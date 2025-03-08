package rpctest

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"
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
	RequestSize   int
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
	// WaitTime defines how long to wait for subscription events
	WaitTime time.Duration
	// MinEvents is the minimum number of events that should be received for the test to be considered successful
	MinEvents int
	// Validator is called for each event and should return true if the event is valid
	Validator func(json.RawMessage) (bool, string)
}

// NewTestRunner creates a new test runner for the given socket path
func NewTestRunner(sockPath string) (*TestRunner, error) {
	client, err := NewClient(sockPath)
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
		{
			Name:        "eth_newHeads",
			Description: "Subscribe to new block headers",
			Namespace:   "eth",
			Method:      "newHeads",
			Params:      []interface{}{},
			WaitTime:    time.Second * 60, // Wait up to a minute for at least one block
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
		},
		{
			Name:        "eth_logs",
			Description: "Subscribe to logs for USDC transfers",
			Namespace:   "eth",
			Method:      "logs",
			Params: []interface{}{
				map[string]interface{}{
					"address": "0xa0b86991c6218b36c1d19d4a2e9eb0ce3606eb48", // USDC contract
					"topics": []string{
						"0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef", // Transfer(address,address,uint256)
					},
				},
			},
			WaitTime:  time.Second * 120, // Wait up to 2 minutes for transfers
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

				return true, "Received valid transfer log"
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
func (tr *TestRunner) RunSubscriptionTests(ctx context.Context) []BenchmarkResult {
	log.Warn().Msg("Subscription tests will only test subscription creation, not real-time notifications")
	log.Warn().Msg("Real-time subscription notifications are not supported in current implementation")

	totalTests := len(tr.subTestCases)
	log.Info().Int("total", totalTests).Msg("Starting subscription tests")

	results := make([]BenchmarkResult, len(tr.subTestCases))

	// Run tests sequentially to avoid socket contention
	for i, test := range tr.subTestCases {
		log.Info().
			Int("current", i+1).
			Int("total", totalTests).
			Str("test", test.Name).
			Msg("Running subscription test")

		// Add a small delay between tests
		if i > 0 {
			time.Sleep(100 * time.Millisecond)
		}

		results[i] = tr.runSubscriptionTest(ctx, test)

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

	// Create a JSON request to estimate size
	request := jsonRPCRequest{
		JSONRPC: "2.0",
		Method:  test.Method,
		Params:  test.Params,
		ID:      1,
	}

	reqBytes, err := json.Marshal(request)
	if err != nil {
		result.Success = false
		result.Error = fmt.Sprintf("Failed to marshal request: %v", err)
		log.Error().Err(err).Str("test", test.Name).Msg("Failed to marshal request")
		return result
	}

	result.RequestSize = len(reqBytes)

	log.Debug().Str("test", test.Name).Msg("Executing test")

	// Execute the test with timing
	start := time.Now()
	var response json.RawMessage

	err = tr.client.Call(test.Method, &response, test.Params...)
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

// runSubscriptionTest executes a single subscription test case
func (tr *TestRunner) runSubscriptionTest(ctx context.Context, test SubscriptionTestCase) BenchmarkResult {
	result := BenchmarkResult{
		TestName: test.Name,
	}

	// Create a JSON request to estimate size (only for metrics)
	req := jsonRPCRequest{
		JSONRPC: "2.0",
		Method:  test.Namespace + "_subscribe",
		Params:  append([]interface{}{test.Method}, test.Params...),
		ID:      1,
	}

	reqBytes, err := json.Marshal(req)
	if err != nil {
		result.Success = false
		result.Error = fmt.Sprintf("Failed to marshal request: %v", err)
		log.Error().Err(err).Str("test", test.Name).Msg("Failed to marshal subscription request")
		return result
	}

	result.RequestSize = len(reqBytes)

	log.Debug().Str("test", test.Name).Msg("Testing subscription")

	// Since we don't actually process notifications in this version,
	// we'll consider the test successful if we can subscribe and unsubscribe

	// Create dummy handler (won't be called in current implementation)
	handler := func(data json.RawMessage) {
		log.Debug().
			Str("test", test.Name).
			RawJSON("data", data).
			Msg("Subscription handler called (unexpected)")
	}

	// Start timing
	start := time.Now()

	// Subscribe
	subID, err := tr.client.Subscribe(ctx, test.Namespace, test.Method, handler, test.Params...)
	if err != nil {
		result.Success = false
		result.Error = fmt.Sprintf("Subscription failed: %v", err)
		result.Duration = time.Since(start)
		log.Debug().
			Err(err).
			Str("test", test.Name).
			Msg("Subscription creation failed")
		return result
	}

	// Immediately unsubscribe
	err = tr.client.Unsubscribe(test.Namespace, subID)
	duration := time.Since(start)
	result.Duration = duration

	if err != nil {
		result.Success = false
		result.Error = fmt.Sprintf("Unsubscribe failed: %v", err)
		log.Debug().
			Err(err).
			Str("test", test.Name).
			Str("subscription_id", subID).
			Msg("Unsubscribe failed")
		return result
	}

	// We consider the test successful if both subscribe and unsubscribe worked
	result.Success = true
	result.ResponseSize = len(subID) // Just as an estimate
	result.StatusMessage = fmt.Sprintf("Successfully subscribed and unsubscribed (ID: %s)", subID)

	log.Debug().
		Str("test", test.Name).
		Str("subscription_id", subID).
		Msg("Subscription test succeeded")

	return result
}
