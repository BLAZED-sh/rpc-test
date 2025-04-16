package rpctest

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/rpc"
	"github.com/rs/zerolog/log"
)

// Client is an Ethereum JSON-RPC client
type Client struct {
	rpcClient       *rpc.Client
	endpoint        string
	requestID       int
	mu              sync.Mutex // Protects requestID, subHandlers, and subscriptions
	subHandlers     map[string]SubscriptionHandler
	subscriptions   map[string]*rpc.ClientSubscription
	ctx             context.Context
	cancel          context.CancelFunc
	supportsSubsVal bool
}

type SubscriptionHandler func(data json.RawMessage)

// NewClient creates a new RPC client with auto-detected transport type
func NewClient(endpoint string) (*Client, error) {
	log.Debug().Str("endpoint", endpoint).Msg("Creating new RPC client")

	// Detect transport type
	transport := "http"
	supportsSubscriptions := false

	// Check if the endpoint is a Unix socket path
	if strings.HasPrefix(endpoint, "/") || strings.HasPrefix(endpoint, "./") {
		transport = "unix"
		supportsSubscriptions = true
		log.Debug().Str("socket", endpoint).Msg("Using Unix socket transport")
	} else if strings.HasPrefix(endpoint, "ws://") || strings.HasPrefix(endpoint, "wss://") {
		transport = "ws"
		supportsSubscriptions = true
		log.Debug().Str("url", endpoint).Msg("Using WebSocket transport")
	} else if strings.HasPrefix(endpoint, "http://") || strings.HasPrefix(endpoint, "https://") {
		log.Debug().Str("url", endpoint).Msg("Using HTTP transport")
		// HTTP already set as default
	} else {
		return nil, fmt.Errorf("unsupported endpoint format: %s", endpoint)
	}

	// Create the client based on transport type
	var rpcClient *rpc.Client
	var err error

	switch transport {
	case "unix":
		// Connect to IPC endpoint
		rpcClient, err = rpc.DialIPC(context.Background(), endpoint)
	case "ws":
		// Connect to WebSocket endpoint
		rpcClient, err = rpc.DialWebsocket(context.Background(), endpoint, "")
	case "http":
		// Connect to HTTP endpoint
		rpcClient, err = rpc.DialHTTP(endpoint)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to connect to %s: %w", endpoint, err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	client := &Client{
		rpcClient:       rpcClient,
		endpoint:        endpoint,
		requestID:       1,
		subHandlers:     make(map[string]SubscriptionHandler),
		subscriptions:   make(map[string]*rpc.ClientSubscription),
		ctx:             ctx,
		cancel:          cancel,
		supportsSubsVal: supportsSubscriptions,
	}

	log.Debug().Str("endpoint", endpoint).Msg("Connected to Ethereum node")
	return client, nil
}

// Close closes the connection to the RPC server
func (c *Client) Close() error {
	// Unsubscribe from all active subscriptions
	c.mu.Lock()
	for _, subscription := range c.subscriptions {
		subscription.Unsubscribe()
	}
	// Clear the maps
	c.subscriptions = make(map[string]*rpc.ClientSubscription)
	c.subHandlers = make(map[string]SubscriptionHandler)
	c.mu.Unlock()

	// Cancel context and close the client
	c.cancel()
	c.rpcClient.Close()
	return nil
}

// Call makes a synchronous RPC call
func (c *Client) Call(method string, result interface{}, params ...interface{}) error {
	log.Debug().Str("method", method).Interface("params", params).Msg("Making RPC call")

	// Track the start time for metrics
	start := time.Now()

	// Make the actual RPC call
	err := c.rpcClient.CallContext(c.ctx, result, method, params...)

	// Log the result
	duration := time.Since(start)
	if err != nil {
		log.Debug().
			Err(err).
			Str("method", method).
			Dur("duration", duration).
			Msg("RPC call failed")
		return fmt.Errorf("RPC call to %s failed: %w", method, err)
	}

	log.Debug().
		Str("method", method).
		Dur("duration", duration).
		Msg("RPC call successful")

	return nil
}

// Subscribe creates a subscription to an event
func (c *Client) Subscribe(
	ctx context.Context,
	namespace, method string,
	handler SubscriptionHandler,
	params ...interface{},
) (string, error) {
	log.Debug().
		Str("namespace", namespace).
		Str("method", method).
		Interface("params", params).
		Msg("Creating subscription")

	// Create a channel to receive subscription notifications
	notificationChan := make(chan interface{})
	
	// Variables to hold the subscription and any error
	var subscription *rpc.ClientSubscription
	var err error

	// For "newHeads" subscription, we only need to pass the method name
	if method == "newHeads" {
		subscription, err = c.rpcClient.EthSubscribe(ctx, notificationChan, method)
	} else if method == "logs" {
		// For logs subscription, check if we already have a filter object as the first parameter
		if len(params) > 0 {
			if filterObj, ok := params[0].(map[string]interface{}); ok {
				// Use the filter object directly
				log.Debug().Interface("filter", filterObj).Msg("Using provided filter object for logs subscription")
				subscription, err = c.rpcClient.EthSubscribe(ctx, notificationChan, method, filterObj)
			} else {
				// If not a map, pass the parameter as-is
				subscription, err = c.rpcClient.EthSubscribe(ctx, notificationChan, method, params[0])
			}
		} else {
			// No parameters, subscribe to all logs
			subscription, err = c.rpcClient.EthSubscribe(ctx, notificationChan, method)
		}
	} else {
		// For other types of subscriptions
		if len(params) > 0 {
			subscription, err = c.rpcClient.EthSubscribe(ctx, notificationChan, method, params[0])
		} else {
			subscription, err = c.rpcClient.EthSubscribe(ctx, notificationChan, method)
		}
	}
	
	if err != nil {
		return "", fmt.Errorf("subscription failed: %w", err)
	}

	// The subscription has already been created in the conditional logic above

	// Get the subscription ID (using time-based ID as we can't directly get it from the ClientSubscription)
	subID := fmt.Sprintf("%x", time.Now().UnixNano())

	// Store the handler and subscription
	c.mu.Lock()
	c.subHandlers[subID] = handler
	c.subscriptions[subID] = subscription
	c.mu.Unlock()

	// Start a goroutine to process incoming notifications
	go func() {
		for {
			select {
			case notification := <-notificationChan:
				if notification == nil {
					log.Debug().Str("subscription", subID).Msg("Subscription channel closed")
					return
				}
				// Process the notification by calling the handler
				if rawMsg, err := json.Marshal(notification); err == nil {
					handler(rawMsg)
				} else {
					log.Warn().Err(err).Str("subscription", subID).Msg("Failed to marshal notification")
				}
			case err := <-subscription.Err():
				if err != nil {
					log.Warn().Err(err).Str("subscription", subID).Msg("Subscription error")
				}
				return
			case <-ctx.Done():
				log.Debug().Str("subscription", subID).Msg("Context cancelled, stopping subscription")
				subscription.Unsubscribe()
				return
			}
		}
	}()

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

	// Get the subscription object
	c.mu.Lock()
	subscription, exists := c.subscriptions[subID]
	c.mu.Unlock()

	if !exists {
		return fmt.Errorf("subscription not found: %s", subID)
	}

	// Use the subscription object to unsubscribe
	subscription.Unsubscribe()

	// Clean up the handler and subscription
	c.mu.Lock()
	delete(c.subHandlers, subID)
	delete(c.subscriptions, subID)
	c.mu.Unlock()

	log.Debug().
		Str("subscription_id", subID).
		Msg("Unsubscribed successfully")

	return nil
}

// SupportsSubscriptions returns whether this transport type supports subscriptions
func (c *Client) SupportsSubscriptions() bool {
	return c.supportsSubsVal
}

// Helper method to determine if transport supports subscriptions
func (c *Client) transport() Transport {
	return &transportAdapter{client: c}
}

// Transport is an interface for compatibility with existing code
type Transport interface {
	SupportsSubscriptions() bool
}

// transportAdapter is a simple adapter to provide the Transport interface
type transportAdapter struct {
	client *Client
}

func (t *transportAdapter) SupportsSubscriptions() bool {
	return t.client.supportsSubsVal
}
