# Ethereum RPC Test (JavaScript)

***This is a JavaScript port of the Go-based RPC testing tool***

> **Note**: This JavaScript version uses ethers.js v6 and implements subscription functionality using polling rather than WebSocket subscriptions. This approach is more compatible with a wider range of Ethereum node implementations.

A benchmarking and testing tool for Ethereum RPC node implementations. This tool allows you to run tests against a real-world mainnet Ethereum node to verify functionality and measure performance.

## Features

- Test RPC methods using various transport options:
  - WebSockets (ws:// or wss://)
  - HTTP/HTTPS JSON-RPC
  - Unix domain sockets (IPC)
- Benchmark basic Ethereum JSON-RPC methods
- Support for subscription-based tests (newHeads, logs, etc.)
- Generate reports in multiple formats (text, markdown, JSON)
- Measure response times and request/response sizes

## Requirements

- Node.js 18.x or later
- An Ethereum node with one of the following:
  - WebSocket endpoint
  - HTTP JSON-RPC endpoint

## Installation

```bash
# Clone the repository
git clone https://github.com/fipso/rpc-test.git
cd rpc-test/js

# Install dependencies
npm install

# Make the CLI script executable
chmod +x bin/rpc-test.js
```

## Usage

```bash
./bin/rpc-test.js --ws ws://localhost:8546
```

### Command Line Options

Transport options (one is required):
- `--socket`: Path to the Ethereum node Unix domain socket
- `--ws`: WebSocket URL of the Ethereum node (ws:// or wss://)
- `--http`: HTTP JSON-RPC URL of the Ethereum node (http:// or https://)

Other options:
- `--format`: Output format - text, markdown, or json (default: text)
- `--subscriptions`: Run subscription tests which may take longer (default: false)
- `--output`: Path to write the report (default: stdout)
- `--share-client`: Use a shared client for subscription tests instead of creating fresh connections (default: false)

Note: WebSocket transport is recommended for subscription tests.

### Logging Options

- `--log-level`: Set the log level (default: "info")
  - Valid levels: "trace", "debug", "info", "warn", "error", "fatal"
  - Use "trace" to see detailed socket communication
  - Use "debug" to see detailed test execution
  - Use "info" for normal operation
- `--no-color`: Disable colorized output (default: false)

### Example Commands

Basic test run with WebSocket:
```bash
./bin/rpc-test.js --ws ws://localhost:8546
```

Using HTTP transport:
```bash
./bin/rpc-test.js --http http://localhost:8545
```

Using Unix domain socket (IPC):
```bash
./bin/rpc-test.js --socket /path/to/geth.ipc
```

> **Note**: Unix domain socket (IPC) support is available through ethers.js v6 IpcSocketProvider. This is especially useful for connecting to local nodes like Geth or Erigon.

Run with subscription tests (recommended with WebSocket transport):
```bash
./bin/rpc-test.js --ws wss://mainnet.infura.io/ws/v3/YOUR_PROJECT_ID --subscriptions --format markdown --output report.md
```

Generate JSON report for integration with other tools:
```bash
./bin/rpc-test.js --http https://mainnet.infura.io/v3/YOUR_PROJECT_ID --format json --output report.json
```

Run with detailed logging:
```bash
./bin/rpc-test.js --ws ws://localhost:8546 --log-level trace
```

Run with quiet output (errors only):
```bash
./bin/rpc-test.js --http http://localhost:8545 --log-level error
```

## Test Coverage

The tool tests the following Ethereum RPC methods:

### Regular Methods
- `net_version`
- `eth_chainId`
- `eth_getBlockByNumber`
- `eth_blockNumber`
- `eth_getBalance`
- `eth_gasPrice`
- `eth_call` (testing ERC20 totalSupply)

### Subscription Methods
- `eth_subscribe` with `logs` for USDT and USDC transfers

## Library Usage

You can also use the package as a library in your own JavaScript code:

```javascript
import { createTestRunner, ReportGenerator } from './src/rpctest/index.js';

async function main() {
  try {
    // Create a test runner - can use WebSocket or HTTP endpoint
    const runner = await createTestRunner('ws://localhost:8546'); // or 'http://localhost:8545'
    
    // Run tests
    const results = await runner.runTests();
    
    // Run subscription tests (works best with WebSocket)
    const subResults = await runner.runSubscriptionTests();
    
    // Combine results
    const allResults = [...results, ...subResults];
    
    // Generate report
    const reportGen = new ReportGenerator(allResults);
    const report = reportGen.generateTextReport();
    console.log(report);
    
    // Clean up
    await runner.close();
  } catch (err) {
    console.error('Error:', err);
  }
}

main();
```

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

## License

This project is licensed under the MIT License.