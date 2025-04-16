package rpctest

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
)

// TestCase represents a single RPC test
type TestCase struct {
	Name        string
	Description string
	Method      string
	Params      []interface{}
	Validator   func(json.RawMessage) (bool, string)
}

// BenchmarkResult contains the results of a benchmark run
type BenchmarkResult struct {
	TestName      string
	Success       bool
	Duration      time.Duration
	Error         string
	ResponseSize  int
	StatusMessage string
}

// TestRunner is responsible for running tests against an Ethereum node
type TestRunner struct {
	client       *Client
	testCases    []TestCase
	subTestCases []SubscriptionTestCase
	results      []BenchmarkResult
	mu           sync.Mutex
}

// SubscriptionTestCase represents a test for subscription methods
type SubscriptionTestCase struct {
	Name        string
	Description string
	Namespace   string
	Method      string
	Params      []interface{}
	// Removed WaitTime field
	// MinEvents is the minimum number of events that should be received for the test to be considered successful
	MinEvents int
	// Validator is called for each event and should return true if the event is valid
	Validator func(json.RawMessage) (bool, string)
}

// NewTestRunner creates a new test runner for the given endpoint (socket path or websocket URL)
func NewTestRunner(endpoint string) (*TestRunner, error) {
	client, err := NewClient(endpoint)
	if err != nil {
		return nil, err
	}

	return &TestRunner{
		client:       client,
		testCases:    GetDefaultTestCases(),
		subTestCases: GetDefaultSubscriptionTestCases(),
	}, nil
}

// Close closes the test runner and its client
func (tr *TestRunner) Close() error {
	return tr.client.Close()
}

// GetDefaultTestCases returns a set of standard Ethereum JSON-RPC tests
func GetDefaultTestCases() []TestCase {
	return []TestCase{
		{
			Name:        "net_version",
			Description: "Get the current network ID",
			Method:      "net_version",
			Params:      []interface{}{},
			Validator: func(result json.RawMessage) (bool, string) {
				var version string
				if err := json.Unmarshal(result, &version); err != nil {
					return false, fmt.Sprintf("Failed to parse result: %v", err)
				}
				return true, fmt.Sprintf("Network version: %s", version)
			},
		},
		{
			Name:        "eth_chainId",
			Description: "Get the chain ID",
			Method:      "eth_chainId",
			Params:      []interface{}{},
			Validator: func(result json.RawMessage) (bool, string) {
				var chainID string
				if err := json.Unmarshal(result, &chainID); err != nil {
					return false, fmt.Sprintf("Failed to parse result: %v", err)
				}
				// Convert hex to decimal
				id, ok := new(big.Int).SetString(chainID[2:], 16)
				if !ok {
					return false, "Failed to convert chain ID to integer"
				}
				return true, fmt.Sprintf("Chain ID: %s", id.String())
			},
		},
		{
			Name:        "eth_getBlockByNumber",
			Description: "Get the latest block data",
			Method:      "eth_getBlockByNumber",
			Params:      []interface{}{"latest", true}, // true to include full transaction objects
			Validator: func(result json.RawMessage) (bool, string) {
				var block map[string]interface{}
				if err := json.Unmarshal(result, &block); err != nil {
					return false, fmt.Sprintf("Failed to parse block: %v", err)
				}

				if block == nil {
					return false, "Received null block"
				}

				// Check for required fields
				requiredFields := []string{
					"number",
					"hash",
					"parentHash",
					"timestamp",
					"gasUsed",
					"gasLimit",
				}
				for _, field := range requiredFields {
					if _, ok := block[field]; !ok {
						return false, fmt.Sprintf("Block missing required field: %s", field)
					}
				}

				// Convert block number to decimal
				blockNumber, ok := block["number"].(string)
				if !ok {
					return false, "Block number is not a string"
				}

				number, ok := new(big.Int).SetString(blockNumber[2:], 16)
				if !ok {
					return false, "Failed to convert block number to integer"
				}

				// Count transactions
				txCount := 0
				if txs, ok := block["transactions"].([]interface{}); ok {
					txCount = len(txs)
				}

				return true, fmt.Sprintf("Block #%s with %d transactions", number.String(), txCount)
			},
		},
		{
			Name:        "eth_blockNumber",
			Description: "Get the current block number",
			Method:      "eth_blockNumber",
			Params:      []interface{}{},
			Validator: func(result json.RawMessage) (bool, string) {
				var blockNumber string
				if err := json.Unmarshal(result, &blockNumber); err != nil {
					return false, fmt.Sprintf("Failed to parse result: %v", err)
				}
				// Convert hex to decimal
				number, ok := new(big.Int).SetString(blockNumber[2:], 16)
				if !ok {
					return false, "Failed to convert block number to integer"
				}
				return true, fmt.Sprintf("Block number: %s", number.String())
			},
		},
		{
			Name:        "eth_call_erc20_totalSupply",
			Description: "Call totalSupply on a popular ERC20 token (USDC)",
			Method:      "eth_call",
			Params: []interface{}{
				map[string]string{
					"to":   "0xa0b86991c6218b36c1d19d4a2e9eb0ce3606eb48", // USDC contract
					"data": "0x18160ddd",                                 // totalSupply()
				},
				"latest",
			},
			Validator: func(result json.RawMessage) (bool, string) {
				var hexResult string
				if err := json.Unmarshal(result, &hexResult); err != nil {
					return false, fmt.Sprintf("Failed to parse result: %v", err)
				}
				if len(hexResult) < 3 {
					return false, "Invalid result length"
				}
				// Convert hex to decimal
				supply, ok := new(big.Int).SetString(hexResult[2:], 16)
				if !ok {
					return false, "Failed to convert supply to integer"
				}
				return true, fmt.Sprintf("Token supply: %s", supply.String())
			},
		},
		{
			Name:        "eth_gasPrice",
			Description: "Get the current gas price",
			Method:      "eth_gasPrice",
			Params:      []interface{}{},
			Validator: func(result json.RawMessage) (bool, string) {
				var gasPrice string
				if err := json.Unmarshal(result, &gasPrice); err != nil {
					return false, fmt.Sprintf("Failed to parse result: %v", err)
				}
				// Convert hex to decimal
				price, ok := new(big.Int).SetString(gasPrice[2:], 16)
				if !ok {
					return false, "Failed to convert gas price to integer"
				}
				return true, fmt.Sprintf("Gas price: %s wei", price.String())
			},
		},
		// Put the heaviest operations last
		{
			Name:        "eth_getBalance",
			Description: "Get the balance of the Ethereum foundation",
			Method:      "eth_getBalance",
			Params:      []interface{}{"0xde0b295669a9fd93d5f28d9ec85e40f4cb697bae", "latest"},
			Validator: func(result json.RawMessage) (bool, string) {
				var balance string
				if err := json.Unmarshal(result, &balance); err != nil {
					return false, fmt.Sprintf("Failed to parse result: %v", err)
				}
				// Convert hex to decimal
				bal, ok := new(big.Int).SetString(balance[2:], 16)
				if !ok {
					return false, "Failed to convert balance to integer"
				}
				return true, fmt.Sprintf("Balance: %s wei", bal.String())
			},
		},
	}
}

// GetDefaultSubscriptionTestCases returns a set of standard Ethereum subscription tests
func GetDefaultSubscriptionTestCases() []SubscriptionTestCase {
	return []SubscriptionTestCase{
		/*
		{
			Name:        "eth_newHeads",
			Description: "Subscribe to new block headers",
			Namespace:   "eth",
			Method:      "newHeads",
			Params:      []interface{}{},
			MinEvents:   1,
			Validator: func(result json.RawMessage) (bool, string) {
				var header map[string]interface{}
				if err := json.Unmarshal(result, &header); err != nil {
					return false, fmt.Sprintf("Failed to parse header: %v", err)
				}
				if header == nil {
					return false, "Received null header"
				}

				// Check for required fields
				requiredFields := []string{"number", "hash", "parentHash"}
				for _, field := range requiredFields {
					if _, ok := header[field]; !ok {
						return false, fmt.Sprintf("Header missing required field: %s", field)
					}
				}

				return true, fmt.Sprintf("Received valid block header: %v", header["number"])
			},
		},*/
		{
			Name:        "eth_logs_usdt",
			Description: "Subscribe to logs for USDT transfers - high volume token",
			Namespace:   "eth",
			Method:      "logs",
			Params: []interface{}{
				map[string]interface{}{
					"address": "0xdAC17F958D2ee523a2206206994597C13D831ec7",                                        // USDT contract
					"topics":  []interface{}{"0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef"}, // Transfer(address,address,uint256)
				},
			},
			MinEvents: 1,
			Validator: func(result json.RawMessage) (bool, string) {
				var log map[string]interface{}
				if err := json.Unmarshal(result, &log); err != nil {
					return false, fmt.Sprintf("Failed to parse log: %v", err)
				}
				if log == nil {
					return false, "Received null log"
				}

				// Check for required fields
				requiredFields := []string{"address", "topics", "data", "blockNumber"}
				for _, field := range requiredFields {
					if _, ok := log[field]; !ok {
						return false, fmt.Sprintf("Log missing required field: %s", field)
					}
				}

				// Check that the address is USDT
				address, ok := log["address"].(string)
				if !ok || address != "0xdac17f958d2ee523a2206206994597c13d831ec7" {
					return false, fmt.Sprintf("Unexpected contract address: %s", address)
				}

				// Extract some data for better log message
				var blockNumberHex string
				if blockNum, ok := log["blockNumber"].(string); ok {
					blockNumberHex = blockNum
				}

				var txHash string
				if hash, ok := log["transactionHash"].(string); ok {
					txHash = hash
				}

				return true, fmt.Sprintf(
					"Received valid USDT transfer log in block %s, tx: %s",
					blockNumberHex,
					txHash,
				)
			},
		},
		{
			Name:        "eth_logs_usdc",
			Description: "Subscribe to logs for USDC transfers - high volume token",
			Namespace:   "eth",
			Method:      "logs",
			Params: []interface{}{
				map[string]interface{}{
					"address": "0xa0b86991c6218b36c1d19d4a2e9eb0ce3606eb48",                                        // USDC contract
					"topics":  []interface{}{"0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef"}, // Transfer(address,address,uint256)
				},
			},
			MinEvents: 1,
			Validator: func(result json.RawMessage) (bool, string) {
				var log map[string]interface{}
				if err := json.Unmarshal(result, &log); err != nil {
					return false, fmt.Sprintf("Failed to parse log: %v", err)
				}
				if log == nil {
					return false, "Received null log"
				}

				// Check for required fields
				requiredFields := []string{"address", "topics", "data", "blockNumber"}
				for _, field := range requiredFields {
					if _, ok := log[field]; !ok {
						return false, fmt.Sprintf("Log missing required field: %s", field)
					}
				}

				// Check that the address is USDC or that it's a transfer to/from USDC
				logAddr, logAddrOk := log["address"].(string)
				if !logAddrOk {
					return false, fmt.Sprintf("Missing address in log")
				}

				// Try to extract topics
				var toAddr string

				if topicsArray, ok := log["topics"].([]interface{}); ok && len(topicsArray) >= 3 {
					// The 3rd topic (index 2) is usually the destination address in Transfer events
					if topicStr, ok := topicsArray[2].(string); ok {
						// Convert from topic format (0x000000000000000000000000{address}) to address format
						if len(topicStr) >= 42 {
							// Extract the last 40 characters (20 bytes) of the topic and add 0x prefix
							toAddr = "0x" + topicStr[len(topicStr)-40:]
						}
					}
				}

				// Check for USDC address either as the contract address or transfer recipient
				targetContract := "0xa0b86991c6218b36c1d19d4a2e9eb0ce3606eb48"
				if strings.EqualFold(logAddr, targetContract) || (toAddr != "" && strings.EqualFold(toAddr, targetContract)) {
					// Valid - either it's the USDC contract or a transfer to USDC
				} else {
					return false, fmt.Sprintf("Unexpected contract / target address: %s / %s", logAddr, toAddr)
				}

				// Extract some data for better log message
				var blockNumberHex string
				if blockNum, ok := log["blockNumber"].(string); ok {
					blockNumberHex = blockNum
				}

				var txHash string
				if hash, ok := log["transactionHash"].(string); ok {
					txHash = hash
				}

				return true, fmt.Sprintf(
					"Received valid USDC transfer log in block %s, tx: %s",
					blockNumberHex,
					txHash,
				)
			},
		},
	}
}

// RunTests executes all registered RPC test cases and returns results
func (tr *TestRunner) RunTests(ctx context.Context) []BenchmarkResult {
	tr.results = []BenchmarkResult{}

	totalTests := len(tr.testCases)
	log.Info().Int("total", totalTests).Msg("Starting tests")

	// Run regular RPC tests
	for i, test := range tr.testCases {
		log.Info().
			Int("current", i+1).
			Int("total", totalTests).
			Str("test", test.Name).
			Msg("Running test")

		// Add a small delay between tests to avoid overwhelming the server
		if i > 0 {
			time.Sleep(100 * time.Millisecond)
		}

		result := tr.runTest(ctx, test)
		tr.mu.Lock()
		tr.results = append(tr.results, result)
		tr.mu.Unlock()

		// Print progress
		if result.Success {
			log.Info().
				Int("current", i+1).
				Int("total", totalTests).
				Str("test", test.Name).
				Str("duration", result.Duration.String()).
				Msg("Test succeeded")
		} else {
			log.Info().
				Int("current", i+1).
				Int("total", totalTests).
				Str("test", test.Name).
				Str("duration", result.Duration.String()).
				Str("error", result.Error).
				Msg("Test failed")
		}
	}

	return tr.results
}

// RunSubscriptionTests executes all registered subscription test cases
func (tr *TestRunner) RunSubscriptionTests(
	ctx context.Context,
	shareClient bool,
) []BenchmarkResult {
	// Check if the transport supports subscriptions
	if !tr.client.SupportsSubscriptions() {
		log.Warn().
			Msg("The current transport does not support subscriptions (HTTP endpoints cannot be used for subscriptions)")

		// Return empty results marked as failed
		results := make([]BenchmarkResult, len(tr.subTestCases))
		for i, test := range tr.subTestCases {
			results[i] = BenchmarkResult{
				TestName:      test.Name,
				Success:       false,
				Error:         "Transport does not support subscriptions",
				StatusMessage: "Skipped - transport does not support subscriptions",
				Duration:      0,
			}
		}
		return results
	}

	totalTests := len(tr.subTestCases)
	log.Info().Int("total", totalTests).Msg("Starting subscription tests")

	results := make([]BenchmarkResult, len(tr.subTestCases))

	// Get the endpoint to create fresh clients for each test if needed
	endpoint := tr.client.endpoint

	// Run tests sequentially to avoid socket contention
	for i, test := range tr.subTestCases {
		log.Info().
			Int("current", i+1).
			Int("total", totalTests).
			Str("test", test.Name).
			Msg("Running subscription test")

		var testClient *Client
		var err error

		// If sharing client, use the existing one, otherwise create a fresh client
		if shareClient {
			log.Debug().Str("test", test.Name).Msg("Using shared client for subscription test")
			testClient = tr.client
		} else {
			log.Debug().Str("test", test.Name).Msg("Creating fresh client for subscription test")
			testClient, err = NewClient(endpoint)
			if err != nil {
				log.Error().Err(err).Msg("Failed to create client for subscription test")
				results[i] = BenchmarkResult{
					TestName:      test.Name,
					Success:       false,
					Error:         fmt.Sprintf("Client creation failed: %v", err),
					Duration:      0,
					StatusMessage: "Test initialization failed",
				}
				continue
			}
		}

		// Create a context without timeout for this test
		testCtx, cancel := context.WithCancel(ctx)

		// Run the test with the dedicated client
		results[i] = runSubscriptionTestWithClient(testCtx, testClient, test)

		// Only close the test-specific client if we didn't use the shared one
		if !shareClient {
			if err := testClient.Close(); err != nil {
				log.Warn().Err(err).Msg("Error closing test client")
			}
		}

		// Cancel context immediately after test completes
		cancel()

		// Print progress
		if results[i].Success {
			log.Info().
				Int("current", i+1).
				Int("total", totalTests).
				Str("test", test.Name).
				Str("duration", results[i].Duration.String()).
				Msg("Subscription test succeeded")
		} else {
			log.Info().
				Int("current", i+1).
				Int("total", totalTests).
				Str("test", test.Name).
				Str("duration", results[i].Duration.String()).
				Str("error", results[i].Error).
				Msg("Subscription test failed")
		}
	}

	tr.mu.Lock()
	tr.results = append(tr.results, results...)
	tr.mu.Unlock()

	return results
}

// runTest executes a single RPC test case
func (tr *TestRunner) runTest(ctx context.Context, test TestCase) BenchmarkResult {
	result := BenchmarkResult{
		TestName: test.Name,
	}

	log.Debug().Str("test", test.Name).Msg("Executing test")

	// Execute the test with timing
	start := time.Now()
	var response json.RawMessage

	err := tr.client.Call(test.Method, &response, test.Params...)
	duration := time.Since(start)
	result.Duration = duration

	if err != nil {
		result.Success = false
		result.Error = err.Error()
		log.Debug().Err(err).Str("test", test.Name).Msg("Test failed")
		return result
	}

	result.ResponseSize = len(response)

	log.Debug().
		Str("test", test.Name).
		Int("response_size", len(response)).
		Msg("Received response")

	// Validate the result
	success, msg := test.Validator(response)
	result.Success = success
	result.StatusMessage = msg

	if !success {
		result.Error = msg
		log.Debug().
			Str("test", test.Name).
			Str("error", msg).
			Msg("Validation failed")
	} else {
		log.Debug().
			Str("test", test.Name).
			Str("message", msg).
			Msg("Validation succeeded")
	}

	return result
}

// runSubscriptionTestWithClient executes a single subscription test case with a dedicated client
func runSubscriptionTestWithClient(
	ctx context.Context,
	client *Client,
	test SubscriptionTestCase,
) BenchmarkResult {
	result := BenchmarkResult{
		TestName: test.Name,
	}

	log.Debug().Str("test", test.Name).Msg("Testing subscription")

	// Don't reset between subscriptions - a healthy WebSocket should handle multiple subscriptions

	// Create a channel to collect events
	eventCh := make(chan json.RawMessage, 100)
	eventCount := 0
	eventsMu := sync.Mutex{}
	validEvents := 0

	// Start timing
	start := time.Now()

	// Create event handler
	handler := func(data json.RawMessage) {
		log.Debug().
			Str("test", test.Name).
			RawJSON("data", data).
			Msg("Subscription event received")

		select {
		case eventCh <- data:
			eventsMu.Lock()
			eventCount++

			// Validate event if validator is provided
			if test.Validator != nil {
				valid, msg := test.Validator(data)
				if valid {
					validEvents++
					log.Debug().
						Str("test", test.Name).
						Str("message", msg).
						Msg("Valid event received")
				} else {
					log.Warn().
						Str("test", test.Name).
						Str("error", msg).
						Msg("Invalid event received")
				}
			} else {
				validEvents++ // Consider all events valid if no validator
			}
			eventsMu.Unlock()
		default:
			log.Warn().Str("test", test.Name).Msg("Event channel full, dropping event")
		}
	}

	// Subscribe
	subID, err := client.Subscribe(ctx, test.Namespace, test.Method, handler, test.Params...)
	if err != nil {
		result.Success = false
		result.Error = fmt.Sprintf("Subscription failed: %s", err)
		result.Duration = time.Since(start)
		log.Error().
			Err(err).
			Str("test", test.Name).
			Msg("All subscription attempts failed")
		return result
	}

	log.Info().
		Str("test", test.Name).
		Str("subscription_id", subID).
		Int("min_events", test.MinEvents).
		Msg("Waiting for subscription events")

	// Wait for events (removed timeout)
	finished := false

	// This used to use a timeout, now it only checks for context cancellation
	// or minimum events received
	for !finished {
		select {
		case <-ctx.Done():
			finished = true
		default:
			// Check for minimum events without delay
			eventsMu.Lock()
			count := eventCount
			valid := validEvents
			eventsMu.Unlock()

			// log.Debug().
			// 	Str("test", test.Name).
			// 	Int("events", count).
			// 	Int("valid_events", valid).
			// 	Int("min_events", test.MinEvents).
			// 	Msg("Subscription status")

			if valid >= test.MinEvents {
				log.Info().
					Str("test", test.Name).
					Int("events", count).
					Int("valid_events", valid).
					Int("min_events", test.MinEvents).
					Msg("Minimum events received, finishing test early")
				finished = true
			}
		}
	}

	// Get final event counts
	eventsMu.Lock()
	finalCount := eventCount
	finalValid := validEvents
	eventsMu.Unlock()

	// Unsubscribe
	err = client.Unsubscribe(test.Namespace, subID)
	if err != nil {
		log.Error().
			Err(err).
			Str("test", test.Name).
			Str("subscription_id", subID).
			Msg("Unsubscribe failed")
	}

	duration := time.Since(start)
	result.Duration = duration

	// Check if we received enough events
	if finalValid >= test.MinEvents {
		result.Success = true
		result.StatusMessage = fmt.Sprintf(
			"Received %d valid events (%d total)",
			finalValid,
			finalCount,
		)
	} else {
		result.Success = false
		result.Error = fmt.Sprintf("Received only %d valid events (%d total), needed %d",
			finalValid, finalCount, test.MinEvents)
	}

	result.ResponseSize = finalCount * 100 // Rough estimate of response size

	log.Info().
		Str("test", test.Name).
		Str("subscription_id", subID).
		Int("events", finalCount).
		Int("valid_events", finalValid).
		Bool("success", result.Success).
		Msg("Subscription test completed")

	// Drain the event channel
	close(eventCh)
	for range eventCh {
		// Just drain
	}

	return result
}
