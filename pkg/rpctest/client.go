package rpctest

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
)

type Client struct {
	conn        net.Conn
	sockPath    string
	requestID   int
	mu          sync.Mutex // Protects requestID and subHandlers
	subHandlers map[string]SubscriptionHandler
	ctx         context.Context
	cancel      context.CancelFunc
	socketMu    sync.Mutex // Protects socket access
}

type SubscriptionHandler func(data json.RawMessage)

// NewClient creates a new RPC client that connects to an Ethereum node via Unix domain socket
func NewClient(sockPath string) (*Client, error) {
	log.Debug().Str("socket", sockPath).Msg("Connecting to Ethereum node")
	
	conn, err := net.Dial("unix", sockPath)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to socket %s: %w", sockPath, err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	
	client := &Client{
		conn:        conn,
		sockPath:    sockPath,
		requestID:   1,
		subHandlers: make(map[string]SubscriptionHandler),
		ctx:         ctx,
		cancel:      cancel,
	}
	
	// Start listening for subscription responses (placeholder for now)
	go client.listen()

	log.Debug().Str("socket", sockPath).Msg("Connected to Ethereum node")
	return client, nil
}

// Close closes the connection to the RPC server
func (c *Client) Close() error {
	c.cancel()
	return c.conn.Close()
}

// Call makes a synchronous RPC call
func (c *Client) Call(method string, result interface{}, params ...interface{}) error {
	c.mu.Lock()
	id := c.requestID
	c.requestID++
	c.mu.Unlock()

	req := jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      id,
		Method:  method,
		Params:  params,
	}

	reqBytes, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}
	
	// Add newline to properly terminate the JSON request
	reqBytes = append(reqBytes, '\n')

	// Acquire exclusive access to the socket
	log.Debug().Str("method", method).Msg("Acquiring socket lock")
	c.socketMu.Lock()
	defer func() { 
		c.socketMu.Unlock() 
		log.Debug().Str("method", method).Msg("Released socket lock")
	}()
	
	log.Trace().Str("method", method).Str("request", string(reqBytes)).Msg("Writing request")
	log.Debug().Str("method", method).Int("bytes", len(reqBytes)).Msg("Sending request")

	_, err = c.conn.Write(reqBytes)
	if err != nil {
		return fmt.Errorf("failed to write request: %w", err)
	}

	log.Debug().Str("method", method).Msg("Waiting for response")

	// Read response with a larger buffer and handling of partial reads
	var fullResponse []byte
	buffer := make([]byte, 16384) // Use a larger buffer
	
	// Continue reading for a longer time for bigger payloads
	readDeadline := 10 * time.Second
	if method == "eth_getBlockByNumber" {
		// Blocks can be very large, give more time
		readDeadline = 20 * time.Second
	}
	
	// Set read deadline to avoid hanging indefinitely
	if err := c.conn.SetReadDeadline(time.Now().Add(readDeadline)); err != nil {
		return fmt.Errorf("failed to set read deadline: %w", err)
	}
	
	keepReading := true
	for keepReading {
		log.Debug().Str("method", method).Msg("Reading from socket")
		
		n, err := c.conn.Read(buffer)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() && len(fullResponse) > 0 {
				// If we have some data already, try to parse it even after timeout
				log.Debug().
					Str("method", method).
					Int("bytes", len(fullResponse)).
					Msg("Read timeout but have partial data")
				break
			}
			return fmt.Errorf("failed to read response: %w", err)
		}
		
		log.Trace().
			Str("method", method).
			Int("bytes", n).
			Str("data", string(buffer[:n])).
			Msg("Read data")
		
		fullResponse = append(fullResponse, buffer[:n]...)
		
		// Check if we likely have a complete response
		if n < len(buffer) {
			// Got less than buffer size, probably complete
			keepReading = false
		} else {
			// Check if the response might be complete by doing a simple heuristic
			// Look for balanced braces
			openBraces := 0
			closeBraces := 0
			for _, b := range fullResponse {
				if b == '{' {
					openBraces++
				} else if b == '}' {
					closeBraces++
				}
			}
			
			// If braces are balanced and we have a reasonable size, stop reading
			if closeBraces >= openBraces && len(fullResponse) > 128 {
				keepReading = false
			}
		}
		
		if len(fullResponse) > 1024*1024 {
			// Safety limit - don't read more than 1MB
			log.Warn().
				Str("method", method).
				Int("bytes", len(fullResponse)).
				Msg("Response exceeds 1MB, stopping read")
			keepReading = false
		}
		
		log.Debug().
			Str("method", method).
			Int("totalBytes", len(fullResponse)).
			Bool("keepReading", keepReading).
			Msg("Read data chunk")
	}
	
	// Reset read deadline
	if err := c.conn.SetReadDeadline(time.Time{}); err != nil {
		log.Warn().Err(err).Msg("Failed to reset read deadline")
	}
	
	log.Debug().
		Str("method", method).
		Int("bytes", len(fullResponse)).
		Msg("Processing response")
	
	// Check if we have any data to parse
	if len(fullResponse) == 0 {
		return fmt.Errorf("empty response received")
	}
	
	// Try to parse the response as JSON
	var response jsonRPCResponse
	err = json.Unmarshal(fullResponse, &response)
	if err != nil {
		// If that fails, try to find JSON markers and extract just the JSON part
		start := bytes.IndexByte(fullResponse, '{')
		if start == -1 {
			log.Error().
				Str("method", method).
				Int("bytes", len(fullResponse)).
				Msg("No JSON object found in response")
			return fmt.Errorf("no JSON object found in response")
		}
		
		// Try to use the decoder approach as a fallback
		decoder := json.NewDecoder(bytes.NewReader(fullResponse[start:]))
		if decodeErr := decoder.Decode(&response); decodeErr != nil {
			log.Error().
				Err(decodeErr).
				Str("method", method).
				Int("bytes", len(fullResponse)).
				Msg("Failed to decode JSON response")
			return fmt.Errorf("failed to decode JSON response: %w", decodeErr)
		}
	}
	
	log.Debug().
		Str("method", method).
		Msg("Successfully parsed JSON response")

	if response.Error != nil {
		log.Debug().
			Str("method", method).
			Int("code", response.Error.Code).
			Str("message", response.Error.Message).
			Msg("RPC returned error")
		return fmt.Errorf("RPC error: %d %s", response.Error.Code, response.Error.Message)
	}

	log.Debug().
		Str("method", method).
		Msg("Response parsed successfully")

	if result != nil {
		return json.Unmarshal(response.Result, result)
	}

	return nil
}

// Subscribe creates a subscription to an event
func (c *Client) Subscribe(ctx context.Context, namespace, method string, handler SubscriptionHandler, params ...interface{}) (string, error) {
	log.Debug().
		Str("namespace", namespace).
		Str("method", method).
		Msg("Creating subscription")

	// For eth_subscribe, the method is the first param, and other params follow
	actualParams := make([]interface{}, 0, len(params)+1)
	actualParams = append(actualParams, method)
	actualParams = append(actualParams, params...)
	
	var subID string
	err := c.Call(namespace+"_subscribe", &subID, actualParams...)
	if err != nil {
		return "", fmt.Errorf("subscription failed: %w", err)
	}

	c.mu.Lock()
	c.subHandlers[subID] = handler
	c.mu.Unlock()

	log.Debug().
		Str("subscription_id", subID).
		Msg("Subscription created (notifications not supported in current implementation)")

	// In the current implementation, we won't actually receive any notifications
	// This function is kept for future implementation of proper subscription handling

	return subID, nil
}

// Unsubscribe removes a subscription
func (c *Client) Unsubscribe(namespace, subID string) error {
	log.Debug().
		Str("namespace", namespace).
		Str("subscription_id", subID).
		Msg("Unsubscribing")
	
	var result bool
	err := c.Call(namespace+"_unsubscribe", &result, subID)
	if err != nil {
		return fmt.Errorf("unsubscribe failed: %w", err)
	}

	if !result {
		return errors.New("unsubscribe returned false")
	}

	c.mu.Lock()
	delete(c.subHandlers, subID)
	c.mu.Unlock()

	log.Debug().
		Str("subscription_id", subID).
		Msg("Unsubscribed successfully")

	return nil
}

// listen handles incoming subscription notifications
func (c *Client) listen() {
	// Actually disable the listener when not using subscriptions
	// This is just a placeholder for the future
	log.Debug().Msg("Subscription listener is disabled in current implementation")
	
	// Just wait for shutdown signal and do nothing
	<-c.ctx.Done()
	log.Debug().Msg("Subscription listener shutting down")
}

// JSON-RPC request, response and notification structures
type jsonRPCRequest struct {
	JSONRPC string        `json:"jsonrpc"`
	Method  string        `json:"method"`
	Params  []interface{} `json:"params"`
	ID      int           `json:"id"`
}

type jsonRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type jsonRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int             `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *jsonRPCError   `json:"error,omitempty"`
}

type jsonRPCNotification struct {
	JSONRPC string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

type subscriptionResult struct {
	Subscription string          `json:"subscription"`
	Result       json.RawMessage `json:"result"`
}