# Ethereum RPC Test
  
***This is still in development and may be totally broken. This is 99% claude code generated***
  
  


A benchmarking and testing tool for Ethereum RPC node implementations. This tool allows you to run tests against a real-world mainnet Ethereum node to verify functionality and measure performance.

## Features

- Test RPC methods against Unix domain socket endpoints (support for HTTP/WebSocket to be added)
- Benchmark basic Ethereum JSON-RPC methods
- Support for subscription-based tests (newHeads, logs, etc.)
- Generate reports in multiple formats (text, markdown, JSON)
- Measure response times and request/response sizes

## Requirements

- Go 1.18 or later
- An Ethereum node with a Unix domain socket exposed

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

- `--socket`: Path to the Ethereum node Unix domain socket (required)
- `--format`: Output format - text, markdown, or json (default: text)
- `--timeout`: Timeout in seconds for the entire test run (default: 180)
- `--subscriptions`: Run subscription tests which may take longer (default: false)
- `--output`: Path to write the report (default: stdout)

### Logging Options

- `--log-level`: Set the log level (default: "info")
  - Valid levels: "trace", "debug", "info", "warn", "error", "fatal", "panic"
  - Use "trace" to see detailed socket communication
  - Use "debug" to see detailed test execution
  - Use "info" for normal operation
- `--no-color`: Disable colorized output (default: false)

### Example Commands

Basic test run with text output:
```bash
./rpc-test --socket /path/to/geth.ipc
```

Run with subscription tests and save to markdown file:
```bash
./rpc-test --socket /path/to/geth.ipc --subscriptions --format markdown --output report.md
```

Generate JSON report for integration with other tools:
```bash
./rpc-test --socket /path/to/geth.ipc --format json --output report.json
```

Run with detailed socket logging:
```bash
./rpc-test --socket /path/to/geth.ipc --log-level trace
```

Run with debug-level information:
```bash
./rpc-test --socket /path/to/geth.ipc --log-level debug
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
	"fmt"
	"time"

	"github.com/fipso/rpc-test/pkg/rpctest"
)

func main() {
	// Create a client
	client, err := rpctest.NewClient("/path/to/geth.ipc")
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

	// Create a test runner
	runner, err := rpctest.NewTestRunner("/path/to/geth.ipc")
	if err != nil {
		panic(err)
	}
	defer runner.Close()

	// Run tests with context
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	results := runner.RunTests(ctx)

	// Generate report
	reportGen := rpctest.NewReportGenerator(results)
	report := reportGen.GenerateTextReport()
	fmt.Println(report)
}
```

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

## License

This project is licensed under the MIT License - see the LICENSE file for details.
