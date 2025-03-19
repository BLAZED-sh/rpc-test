# Ethereum RPC Test
  
***This is still in development and may be totally broken. This is 99% claude code generated***
  
  


A benchmarking and testing tool for Ethereum RPC node implementations. This tool allows you to run tests against a real-world mainnet Ethereum node to verify functionality and measure performance.

## Features

- Test RPC methods using various transport options:
  - Unix domain sockets
  - WebSockets (ws:// or wss://)
  - HTTP/HTTPS JSON-RPC
- Benchmark basic Ethereum JSON-RPC methods
- Support for subscription-based tests (newHeads, logs, etc.)
- Generate reports in multiple formats (text, markdown, JSON)
- Measure response times and request/response sizes

## Requirements

- Go 1.18 or later
- An Ethereum node with one of the following:
  - Unix domain socket exposed
  - WebSocket endpoint
  - HTTP JSON-RPC endpoint

## Installation

```bash
go get github.com/fipso/rpc-test
```

Or clone and build:

```bash
git clone https://github.com/fipso/rpc-test.git
cd rpc-test
go build -o rpc-test ./cmd
```

## Usage

```bash
./rpc-test --socket /path/to/geth.ipc
```

### Command Line Options

Transport options (one is required):
- `--socket`: Path to the Ethereum node Unix domain socket
- `--ws`: WebSocket URL of the Ethereum node (ws:// or wss://)
- `--http`: HTTP JSON-RPC URL of the Ethereum node (http:// or https://)

Other options:
- `--format`: Output format - text, markdown, or json (default: text)
- `--timeout`: Timeout in seconds for the entire test run (default: 180)
- `--subscriptions`: Run subscription tests which may take longer (default: false)
- `--output`: Path to write the report (default: stdout)

Note: WebSocket transport is recommended for subscription tests.

### Logging Options

- `--log-level`: Set the log level (default: "info")
  - Valid levels: "trace", "debug", "info", "warn", "error", "fatal", "panic"
  - Use "trace" to see detailed socket communication
  - Use "debug" to see detailed test execution
  - Use "info" for normal operation
- `--no-color`: Disable colorized output (default: false)

### Example Commands

Basic test run with Unix socket:
```bash
./rpc-test --socket /path/to/geth.ipc
```

Using WebSocket transport:
```bash
./rpc-test --ws ws://localhost:8546
```

Using HTTP transport:
```bash
./rpc-test --http http://localhost:8545
```

Run with subscription tests (recommended with WebSocket transport):
```bash
./rpc-test --ws wss://mainnet.infura.io/ws/v3/YOUR_PROJECT_ID --subscriptions --format markdown --output report.md
```

Generate JSON report for integration with other tools:
```bash
./rpc-test --http https://mainnet.infura.io/v3/YOUR_PROJECT_ID --format json --output report.json
```

Run with detailed logging:
```bash
./rpc-test --ws ws://localhost:8546 --log-level trace
```

Run with debug-level information:
```bash
./rpc-test --http http://localhost:8545 --log-level debug
```

Run with quiet output (errors only):
```bash
./rpc-test --socket /path/to/geth.ipc --log-level error
```

## Test Coverage

The tool tests the following Ethereum RPC methods:

### Regular Methods
- `net_version`
- `eth_blockNumber`
- `eth_gasPrice`
- `eth_getBalance`
- `eth_chainId`
- `eth_call` (testing ERC20 totalSupply)

Note: `eth_getBlockByNumber` was removed from the default tests because it generates
very large responses that can be difficult to parse reliably over Unix domain sockets.
For large responses, consider using HTTP or WebSocket transport in production.

### Subscription Methods
- `eth_subscribe` with `newHeads`
- `eth_subscribe` with `logs`

## Output Example

```
ETHEREUM RPC BENCHMARK RESULTS
============================

Total tests:     7
Successful:      7 (100.0%)
Failed:          0 (0.0%)
Total duration:  543.21ms
Avg. duration:   77.60ms
Total request:   893 bytes
Total response:  2871 bytes

DETAILED RESULTS
================

✅ eth_blockNumber - SUCCESS
   Duration:     38.12ms
   Request size: 52 bytes
   Response size: 21 bytes
   Result: Block number: 18693412

✅ eth_chainId - SUCCESS
   Duration:     35.45ms
   Request size: 48 bytes
   Response size: 7 bytes
   Result: Chain ID: 1

...
```

## Library Usage

You can also use the package as a library in your own Go code:

```go
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/fipso/rpc-test/pkg/rpctest"
)

func main() {
	// Create a client - can use Unix socket, WebSocket or HTTP endpoint
	client, err := rpctest.NewClient("/path/to/geth.ipc") // or "ws://localhost:8546" or "http://localhost:8545"
	if err != nil {
		panic(err)
	}
	defer client.Close()

	// Make a simple call
	var blockNumber string
	err = client.Call("eth_blockNumber", &blockNumber)
	if err != nil {
		panic(err)
	}
	fmt.Println("Latest block:", blockNumber)
	
	// Use subscription (works best with Unix socket or WebSocket)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	
	handler := func(data json.RawMessage) {
		fmt.Printf("New block header: %s\n", string(data))
	}
	
	subID, err := client.Subscribe(ctx, "eth", "newHeads", handler)
	if err != nil {
		panic(err)
	}
	
	// Unsubscribe after a while
	time.Sleep(10 * time.Second)
	err = client.Unsubscribe("eth", subID)
	if err != nil {
		panic(err)
	}

	// Create a test runner 
	runner, err := rpctest.NewTestRunner("ws://localhost:8546")
	if err != nil {
		panic(err)
	}
	defer runner.Close()

	// Run tests with context
	results := runner.RunTests(ctx)
	
	// Run subscription tests
	subResults := runner.RunSubscriptionTests(ctx)
	
	// Combine results
	allResults := append(results, subResults...)

	// Generate report
	reportGen := rpctest.NewReportGenerator(allResults)
	report := reportGen.GenerateTextReport()
	fmt.Println(report)
}
```

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

## License

This project is licensed under the MIT License - see the LICENSE file for details.
