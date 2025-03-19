package rpctest

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/rs/zerolog/log"
)

// Transport defines the interface for different transport methods (unix socket, websocket, http)
type Transport interface {
	// Write sends data to the server
	Write(data []byte) error
	
	// Read reads data from the server
	Read(buffer []byte) (int, error)
	
	// SetReadDeadline sets a deadline for read operations
	SetReadDeadline(t time.Time) error
	
	// Close closes the transport
	Close() error
	
	// SupportsSubscriptions returns whether this transport supports subscriptions
	SupportsSubscriptions() bool
}

// UnixSocketTransport implements Transport using a Unix domain socket
type UnixSocketTransport struct {
	conn net.Conn
}

// NewUnixSocketTransport creates a new Unix domain socket transport
func NewUnixSocketTransport(sockPath string) (Transport, error) {
	conn, err := net.Dial("unix", sockPath)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to socket %s: %w", sockPath, err)
	}
	
	return &UnixSocketTransport{conn: conn}, nil
}

func (t *UnixSocketTransport) Write(data []byte) error {
	_, err := t.conn.Write(data)
	return err
}

func (t *UnixSocketTransport) Read(buffer []byte) (int, error) {
	return t.conn.Read(buffer)
}

func (t *UnixSocketTransport) SetReadDeadline(deadline time.Time) error {
	return t.conn.SetReadDeadline(deadline)
}

func (t *UnixSocketTransport) Close() error {
	return t.conn.Close()
}

func (t *UnixSocketTransport) SupportsSubscriptions() bool {
	return true // Unix sockets support subscriptions
}

// WebSocketTransport implements Transport using WebSockets
type WebSocketTransport struct {
	conn        *websocket.Conn
	readLock    sync.Mutex
	writeLock   sync.Mutex
	readTimeout time.Duration
	readBuf     *bytes.Buffer
	
	// Message channel for incoming messages
	msgChan     chan []byte
	// Error channel for reader errors
	errChan     chan error
	// Done channel to signal reader shutdown
	doneChan    chan struct{}
	
	// Transport status
	closed      bool
	closeOnce   sync.Once
}

// NewWebSocketTransport creates a new WebSocket transport
func NewWebSocketTransport(url string) (Transport, error) {
	// Set up a dialer with more reasonable timeouts
	dialer := &websocket.Dialer{
		Proxy:            http.ProxyFromEnvironment,
		HandshakeTimeout: 10 * time.Second,
		ReadBufferSize:   16384,
		WriteBufferSize:  16384,
	}
	
	// Try to establish the websocket connection
	conn, resp, err := dialer.Dial(url, http.Header{
		"User-Agent": []string{"RPC-Test/1.0"},
	})
	
	// Check for connection errors
	if err != nil {
		if resp != nil {
			return nil, fmt.Errorf("failed to connect to WebSocket %s (HTTP status %d): %w", 
				url, resp.StatusCode, err)
		}
		return nil, fmt.Errorf("failed to connect to WebSocket %s: %w", url, err)
	}
	
	// Make sure we got a valid connection
	if conn == nil {
		return nil, fmt.Errorf("failed to establish WebSocket connection to %s: nil connection", url)
	}
	
	// Set up connection close handler to log clean disconnects
	conn.SetCloseHandler(func(code int, text string) error {
		log.Debug().
			Int("code", code).
			Str("reason", text).
			Msg("WebSocket connection closing")
		
		// Let the default handler process the close message
		return nil
	})
	
	// Set reasonable ping/pong handlers to keep the connection alive
	conn.SetPingHandler(func(message string) error {
		err := conn.WriteControl(websocket.PongMessage, []byte(message), time.Now().Add(5*time.Second))
		if err != nil {
			log.Warn().Err(err).Msg("Failed to send pong response")
		}
		return nil
	})
	
	log.Debug().Str("url", url).Msg("WebSocket connection established")
	
	// Create transport with channels
	transport := &WebSocketTransport{
		conn:        conn,
		readBuf:     bytes.NewBuffer(nil),
		readTimeout: 60 * time.Second, // Increased from 10 to 60 seconds
		msgChan:     make(chan []byte, 100),  // Buffer for 100 messages
		errChan:     make(chan error, 10),    // Buffer for 10 errors
		doneChan:    make(chan struct{}),
		closed:      false,
	}
	
	// Start the reader goroutine
	go transport.readPump()
	
	return transport, nil
}

func (t *WebSocketTransport) Write(data []byte) error {
	// Use a recovery mechanism to prevent panics from propagating
	var writeErr error
	defer func() {
		if r := recover(); r != nil {
			log.Error().
				Interface("panic", r).
				Msg("Recovered from panic in WebSocketTransport.Write")
			
			if writeErr == nil {
				writeErr = fmt.Errorf("panic occurred during WebSocket write: %v", r)
			}
		}
	}()
	
	t.writeLock.Lock()
	defer t.writeLock.Unlock()
	
	// Add a nil check to avoid panic
	if t.conn == nil {
		return fmt.Errorf("cannot write to nil WebSocket connection")
	}
	
	// Set a write deadline to avoid hanging indefinitely
	if err := t.conn.SetWriteDeadline(time.Now().Add(10 * time.Second)); err != nil {
		return fmt.Errorf("failed to set write deadline: %w", err)
	}
	
	if err := t.conn.WriteMessage(websocket.TextMessage, data); err != nil {
		writeErr = fmt.Errorf("failed to write to WebSocket: %w", err)
		return writeErr
	}
	
	return writeErr
}

func (t *WebSocketTransport) Read(buffer []byte) (int, error) {
	// Use a recovery mechanism to prevent panics from propagating
	defer func() {
		if r := recover(); r != nil {
			log.Error().
				Interface("panic", r).
				Msg("Recovered from panic in WebSocketTransport.Read")
		}
	}()
	
	t.readLock.Lock()
	defer t.readLock.Unlock()
	
	// Check if we're closed
	if t.closed {
		return 0, fmt.Errorf("websocket transport is closed")
	}
	
	// If we have data in buffer, read from there first
	if t.readBuf.Len() > 0 {
		n, err := t.readBuf.Read(buffer)
		if err == nil || err == io.EOF {
			return n, nil // Always return success if we read data
		}
		// Otherwise fall through to channel-based reading
	}
	
	// Calculate the effective timeout - default to 60s if not specified
	readTimeout := t.readTimeout
	if readTimeout < 1*time.Second {
		readTimeout = 60 * time.Second
	}
	timeoutCh := time.After(readTimeout)
	
	// Wait for a message or error from the channels
	select {
	case message, ok := <-t.msgChan:
		if !ok {
			// Channel closed
			return 0, fmt.Errorf("message channel closed")
		}
		
		// Got a message, store in buffer and read from it
		t.readBuf.Write(message)
		return t.readBuf.Read(buffer)
		
	case err, ok := <-t.errChan:
		if !ok {
			// Channel closed
			return 0, fmt.Errorf("error channel closed")
		}
		
		// Return the error
		return 0, err
		
	case <-timeoutCh:
		// Timeout waiting for message
		return 0, fmt.Errorf("timeout waiting for websocket message")
	}
}

func (t *WebSocketTransport) SetReadDeadline(deadline time.Time) error {
	// Use a recovery mechanism to prevent panics from propagating
	defer func() {
		if r := recover(); r != nil {
			log.Error().
				Interface("panic", r).
				Msg("Recovered from panic in WebSocketTransport.SetReadDeadline")
		}
	}()
	
	t.readLock.Lock()
	defer t.readLock.Unlock()
	
	// Store the timeout duration for future reads
	t.readTimeout = time.Until(deadline)
	if t.readTimeout < 0 {
		t.readTimeout = 0
	}
	
	// We don't directly set the deadline on the connection anymore
	// since the connection is managed by the dedicated reader goroutine
	// The timeout is applied when reading from the channel instead
	
	return nil
}

func (t *WebSocketTransport) Close() error {
	// Use a recovery mechanism to prevent panics from propagating
	var closeErr error
	defer func() {
		if r := recover(); r != nil {
			log.Error().
				Interface("panic", r).
				Msg("Recovered from panic in WebSocketTransport.Close")
			
			if closeErr == nil {
				closeErr = fmt.Errorf("panic occurred during WebSocket close: %v", r)
			}
		}
	}()
	
	// Use closeOnce to ensure we only close once
	t.closeOnce.Do(func() {
		log.Debug().Msg("Closing WebSocket transport")
		
		// Mark as closed first to prevent new reads
		t.readLock.Lock()
		t.writeLock.Lock()
		t.closed = true
		t.readLock.Unlock()
		t.writeLock.Unlock()
		
		// Signal reader goroutine to stop
		close(t.doneChan)
		
		// Give the reader goroutine a bit of time to clean up
		time.Sleep(100 * time.Millisecond)
		
		// Check if we have a valid connection to close
		if t.conn == nil {
			log.Debug().Msg("WebSocket connection already nil, not closing")
			return // Already closed or never initialized
		}
		
		// Set a short deadline for the close operation
		func() {
			defer func() {
				if r := recover(); r != nil {
					log.Warn().Interface("panic", r).Msg("Panic while setting write deadline for close")
				}
			}()
			_ = t.conn.SetWriteDeadline(time.Now().Add(1 * time.Second))
		}()
		
		// Try to send close message - but don't fail if it doesn't work
		// This helps avoid issues with already closed connections
		func() {
			defer func() {
				if r := recover(); r != nil {
					log.Warn().Interface("panic", r).Msg("Panic while sending close message")
				}
			}()
			_ = t.conn.WriteMessage(websocket.CloseMessage,
				websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
		}()
		
		// Always attempt to close the connection
		func() {
			defer func() {
				if r := recover(); r != nil {
					log.Warn().Interface("panic", r).Msg("Panic while closing connection")
					closeErr = fmt.Errorf("panic during connection close: %v", r)
				}
			}()
			
			if err := t.conn.Close(); err != nil {
				closeErr = err
			}
		}()
		
		// Close the channels
		close(t.msgChan)
		close(t.errChan)
		
		// Signal that the connection is now invalid
		t.conn = nil
		
		log.Debug().Msg("WebSocket transport closed successfully")
	})
	
	return closeErr
}

func (t *WebSocketTransport) SupportsSubscriptions() bool {
	return true // WebSockets support subscriptions
}

// readPump continuously reads messages from the WebSocket connection
func (t *WebSocketTransport) readPump() {
	// Handle panics
	defer func() {
		if r := recover(); r != nil {
			log.Error().
				Interface("panic", r).
				Msg("Recovered from panic in WebSocket reader goroutine")
				
			// Send the panic as an error
			select {
			case t.errChan <- fmt.Errorf("reader panic: %v", r):
			default:
				// Channel full or closed, can't send error
			}
		}
	}()
	
	// Set ping handler
	t.conn.SetPingHandler(func(data string) error {
		// Log ping received
		log.Debug().Msg("WebSocket ping received, sending pong")
		// Send the pong message
		if err := t.conn.WriteControl(
			websocket.PongMessage, 
			[]byte(data), 
			time.Now().Add(5*time.Second),
		); err != nil {
			log.Warn().Err(err).Msg("Failed to send pong response")
			return err
		}
		return nil
	})
	
	// Main read loop
	for {
		select {
		case <-t.doneChan:
			// Transport is shutting down
			log.Debug().Msg("WebSocket reader shutting down")
			return
		default:
			// Continue reading
		}
		
		// Set a read deadline - use a longer timeout
		err := t.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		if err != nil {
			log.Warn().Err(err).Msg("Failed to set read deadline in WebSocket reader")
			// Try to continue anyway
		}
		
		// Read the next message
		messageType, message, err := t.conn.ReadMessage()
		
		// Process based on the result
		if err != nil {
			// If connection is closed normally, just exit
			if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
				log.Debug().Msg("WebSocket connection closed normally")
				return
			}
			
			// Check for timeout - this is expected and shouldn't cause an error
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				// Just a timeout, keep going
				continue
			}
			
			// For all other errors
			log.Warn().Err(err).Msg("Error reading from WebSocket connection")
			
			// Send the error to the channel, but don't block if it's full
			select {
			case t.errChan <- err:
			default:
				// Channel is full, just log
				log.Warn().Msg("Error channel full, dropping error")
			}
			
			// Back off before trying again
			time.Sleep(100 * time.Millisecond)
			continue
		}
		
		// Check message type
		if messageType != websocket.TextMessage && messageType != websocket.BinaryMessage {
			log.Warn().Int("type", messageType).Msg("Received non-text/binary WebSocket message")
			continue
		}
		
		// Send the message to the channel, without blocking
		select {
		case t.msgChan <- message:
			log.Debug().Int("bytes", len(message)).Msg("WebSocket message received and queued")
		default:
			// Channel is full, log warning
			log.Warn().Msg("Message channel full, dropping message")
		}
	}
}

// HTTPTransport implements Transport using HTTP POST requests
type HTTPTransport struct {
	url           string
	client        *http.Client
	responseQueue []byte
	mu            sync.Mutex
	timeout       time.Duration
}

// NewHTTPTransport creates a new HTTP transport
func NewHTTPTransport(url string) (Transport, error) {
	return &HTTPTransport{
		url:    url,
		client: &http.Client{Timeout: 30 * time.Second},
	}, nil
}

func (t *HTTPTransport) Write(data []byte) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	
	req, err := http.NewRequest("POST", t.url, bytes.NewReader(data))
	if err != nil {
		return err
	}
	
	req.Header.Set("Content-Type", "application/json")
	
	resp, err := t.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP error: %s", resp.Status)
	}
	
	// Read the entire response body and store it in the response queue
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	
	t.responseQueue = responseBody
	return nil
}

func (t *HTTPTransport) Read(buffer []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	
	if len(t.responseQueue) == 0 {
		if t.timeout > 0 && time.Now().After(time.Now().Add(t.timeout)) {
			return 0, fmt.Errorf("timeout")
		}
		return 0, io.EOF
	}
	
	n := copy(buffer, t.responseQueue)
	t.responseQueue = t.responseQueue[n:]
	return n, nil
}

func (t *HTTPTransport) SetReadDeadline(deadline time.Time) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	
	t.timeout = time.Until(deadline)
	return nil
}

func (t *HTTPTransport) Close() error {
	// HTTP client doesn't need explicit closing
	return nil
}

func (t *HTTPTransport) SupportsSubscriptions() bool {
	return false // HTTP does not support subscriptions
}

// Client is an Ethereum JSON-RPC client
type Client struct {
	transport   Transport
	endpoint    string
	requestID   int
	mu          sync.Mutex // Protects requestID and subHandlers
	subHandlers map[string]SubscriptionHandler
	ctx         context.Context
	cancel      context.CancelFunc
	socketMu    sync.Mutex // Protects socket access
}

type SubscriptionHandler func(data json.RawMessage)

// NewClient creates a new RPC client with auto-detected transport type
func NewClient(endpoint string) (*Client, error) {
	log.Debug().Str("endpoint", endpoint).Msg("Creating new RPC client")
	
	var transport Transport
	var err error
	
	// Check if the endpoint is a Unix socket path
	if strings.HasPrefix(endpoint, "/") || strings.HasPrefix(endpoint, "./") {
		log.Debug().Str("socket", endpoint).Msg("Using Unix socket transport")
		transport, err = NewUnixSocketTransport(endpoint)
	} else {
		// Assume it's a URL
		endpointURL, urlErr := url.Parse(endpoint)
		if urlErr != nil {
			return nil, fmt.Errorf("invalid endpoint URL %s: %w", endpoint, urlErr)
		}
		
		switch endpointURL.Scheme {
		case "ws", "wss":
			log.Debug().Str("url", endpoint).Msg("Using WebSocket transport")
			transport, err = NewWebSocketTransport(endpoint)
		case "http", "https":
			log.Debug().Str("url", endpoint).Msg("Using HTTP transport")
			transport, err = NewHTTPTransport(endpoint)
		default:
			return nil, fmt.Errorf("unsupported URL scheme: %s", endpointURL.Scheme)
		}
	}
	
	if err != nil {
		return nil, err
	}
	
	ctx, cancel := context.WithCancel(context.Background())
	
	client := &Client{
		transport:   transport,
		endpoint:    endpoint,
		requestID:   1,
		subHandlers: make(map[string]SubscriptionHandler),
		ctx:         ctx,
		cancel:      cancel,
	}
	
	// Start listening for subscription responses
	go client.listen()
	
	log.Debug().Str("endpoint", endpoint).Msg("Connected to Ethereum node")
	return client, nil
}

// Close closes the connection to the RPC server
func (c *Client) Close() error {
	c.cancel()
	return c.transport.Close()
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

	err = c.transport.Write(reqBytes)
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
	if err := c.transport.SetReadDeadline(time.Now().Add(readDeadline)); err != nil {
		return fmt.Errorf("failed to set read deadline: %w", err)
	}
	
	keepReading := true
	for keepReading {
		log.Debug().Str("method", method).Msg("Reading from socket")
		
		n, err := c.transport.Read(buffer)
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
	if err := c.transport.SetReadDeadline(time.Time{}); err != nil {
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
		Msg("Subscription created")

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
	// Add recovery for the entire listener goroutine
	defer func() {
		if r := recover(); r != nil {
			log.Error().
				Interface("panic", r).
				Msg("Fatal error in subscription listener - subscription functionality will be unavailable")
		}
	}()
	
	log.Debug().Msg("Starting subscription listener")
	
	buffer := make([]byte, 16384)
	connectionErrorCount := 0 // Track consecutive connection errors
	maxErrors := 3 // Maximum number of errors before backing off significantly
	
	for {
		select {
		case <-c.ctx.Done():
			log.Debug().Msg("Subscription listener shutting down")
			return
		default:
			// Only try to read if we have active subscriptions
			c.mu.Lock()
			hasSubscriptions := len(c.subHandlers) > 0
			c.mu.Unlock()
			
			if !hasSubscriptions {
				// Sleep a bit to avoid busy waiting when no subscriptions
				time.Sleep(100 * time.Millisecond)
				continue
			}
			
			// Set a short read deadline to allow checking for context cancellation
			if err := c.transport.SetReadDeadline(time.Now().Add(500 * time.Millisecond)); err != nil {
				log.Warn().Err(err).Msg("Failed to set read deadline for subscription listener")
				
				// Possible closed connection, back off more aggressively
				connectionErrorCount++
				if connectionErrorCount > maxErrors {
					log.Warn().Msg("Multiple connection errors detected, backing off longer")
					time.Sleep(2 * time.Second)
				} else {
					time.Sleep(100 * time.Millisecond)
				}
				continue
			}
			
			c.socketMu.Lock()
			n, err := c.transport.Read(buffer)
			c.socketMu.Unlock()
			
			if err != nil {
				// Check for timeout errors - treat them specially since they're common
				if strings.Contains(err.Error(), "timeout waiting for websocket message") {
					// Only log occasionally to avoid flooding logs
					if connectionErrorCount == 0 || connectionErrorCount % 10 == 0 {
						log.Debug().Err(err).Msg("Timeout waiting for WebSocket messages (normal during quiet periods)")
					}
					connectionErrorCount++
					
					// Don't back off too aggressively for timeouts
					time.Sleep(100 * time.Millisecond)
					continue
				}
				
				if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
					// This is expected, just a timeout
					connectionErrorCount = 0 // Reset error count on successful timeout
					continue
				}
				
				// Check if it's a websocket error that indicates a closed connection
				if strings.Contains(err.Error(), "websocket read error") || 
				   strings.Contains(err.Error(), "use of closed network connection") ||
				   strings.Contains(err.Error(), "connection reset by peer") {
					
					connectionErrorCount++
					
					if connectionErrorCount > maxErrors {
						log.Error().Err(err).Msg("Persistent connection error in subscription listener, backing off")
						time.Sleep(5 * time.Second)
					} else {
						log.Warn().Err(err).Msg("Connection error in subscription listener")
						time.Sleep(500 * time.Millisecond)
					}
					
					// No need to try processing this message
					continue
				}
				
				// Other errors
				log.Warn().Err(err).Msg("Error reading from transport in subscription listener")
				// Don't exit, try again after a small delay
				time.Sleep(100 * time.Millisecond)
				continue
			}
			
			// Successfully read data, reset error counter
			connectionErrorCount = 0
			
			// Process the notification
			data := buffer[:n]
			log.Debug().Int("bytes", n).Msg("Received potential subscription notification")
			
			var notification jsonRPCNotification
			if err := json.Unmarshal(data, &notification); err != nil {
				log.Warn().Err(err).Str("data", string(data)).Msg("Failed to parse subscription notification")
				continue
			}
			
			// Check if this is a subscription notification
			if notification.Method == "eth_subscription" {
				var subResult subscriptionResult
				if err := json.Unmarshal(notification.Params, &subResult); err != nil {
					log.Warn().Err(err).Msg("Failed to parse subscription result")
					continue
				}
				
				log.Debug().
					Str("subscription", subResult.Subscription).
					RawJSON("result", subResult.Result).
					Msg("Received subscription notification")
				
				// Forward to the appropriate handler
				c.mu.Lock()
				handler, exists := c.subHandlers[subResult.Subscription]
				c.mu.Unlock()
				
				if exists {
					handler(subResult.Result)
				} else {
					log.Warn().
						Str("subscription", subResult.Subscription).
						Msg("Received notification for unknown subscription")
				}
			}
		}
	}
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