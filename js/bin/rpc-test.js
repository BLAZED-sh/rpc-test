#!/usr/bin/env node

import { Command } from 'commander';
import fs from 'fs/promises';
import { configureLogger, getLogger, createTestRunner, ReportGenerator } from '../src/rpctest/index.js';

// Create the program
const program = new Command();

// Configure the program
program
  .name('rpc-test')
  .description('Ethereum RPC testing and benchmarking tool')
  .version('1.0.0');

// Add transport options
program
  .option('--socket <path>', 'Path to the Ethereum node Unix domain socket')
  .option('--ws <url>', 'WebSocket URL of the Ethereum node (ws:// or wss://)')
  .option('--http <url>', 'HTTP JSON-RPC URL of the Ethereum node (http:// or https://)')
  .option('--format <format>', 'Output format: text or json', 'text')
  .option('--subscriptions', 'Run subscription tests (may take longer)', false)
  .option('--output <path>', 'Path to write the report (optional, default is stdout)')
  .option('--share-client', 'Use a shared client for subscription tests instead of creating fresh connections', false);

// Add logging options
program
  .option('--log-level <level>', 'Log level: trace, debug, info, warn, error', 'info')
  .option('--no-color', 'Disable colorized output');

// Parse arguments
program.parse(process.argv);
const options = program.opts();

// Main function
async function main() {
  try {
    // Configure logger
    configureLogger({
      level: options.logLevel,
      noColor: !options.color
    });
    
    const logger = getLogger();
    
    // Validate endpoint - one of socket path, websocket URL, or HTTP URL is required
    let endpoint = options.socket || '';
    let endpointType = 'socket';
    
    if (!endpoint && !options.ws && !options.http) {
      logger.error('Either socket path, WebSocket URL, or HTTP URL is required');
      program.help();
      process.exit(1);
    }
    
    if (!endpoint && options.ws) {
      endpoint = options.ws;
      endpointType = 'websocket';
    } else if (!endpoint && options.http) {
      endpoint = options.http;
      endpointType = 'http';
    }
    
    // Create test runner
    logger.info({ endpoint, type: endpointType }, 'Creating test runner');
    
    const runner = await createTestRunner(endpoint);
    
    logger.info({ endpoint, type: endpointType }, 'Starting Ethereum RPC tests');
    
    // Run regular RPC tests
    logger.info('Running regular RPC tests');
    const results = await runner.runTests();
    
    // Initialize allResults with the regular RPC test results
    let allResults = results;
    
    // Run subscription tests if requested
    if (options.subscriptions) {
      // Give a warning if using HTTP for subscriptions (they won't work)
      if (endpointType === 'http') {
        logger.warn('Subscription tests were requested but you\'re using HTTP transport, which doesn\'t support subscriptions');
        logger.warn('Consider using WebSocket (--ws) or Unix socket (--socket) instead for subscription tests');
      }
      
      logger.info(
        { share_client: options.shareClient },
        'Running subscription tests (this may take a while)'
      );
      
      // Pass the shareClient flag to control whether to use fresh clients
      const subResults = await runner.runSubscriptionTests(options.shareClient);
      
      logger.info({ count: subResults.length }, 'Completed subscription tests');
      
      // Add subscription results to allResults
      allResults = [...allResults, ...subResults];
    }
    
    // Close the runner
    await runner.close();
    
    // Generate report with all results
    logger.info('Generating report');
    const reportGen = new ReportGenerator(allResults);
    
    let report;
    switch (options.format) {
      case 'text':
        report = reportGen.generateTextReport(options.color);
        break;
      case 'json':
        report = reportGen.generateJSONReport();
        break;
      default:
        logger.fatal({ format: options.format }, 'Unknown output format');
        process.exit(1);
    }
    
    // Output report
    if (!options.output) {
      // Print to stdout
      console.log(report);
    } else {
      // Write to file
      try {
        await fs.writeFile(options.output, report);
        logger.info({ path: options.output }, 'Report written to file');
      } catch (err) {
        logger.fatal({ error: err.message, path: options.output }, 'Failed to write report to file');
        process.exit(1);
      }
    }
  } catch (err) {
    const logger = getLogger();
    logger.fatal({ error: err.message }, 'Fatal error');
    process.exit(1);
  }
}

// Run the main function
main().catch(err => {
  console.error('Unhandled error:', err);
  process.exit(1);
});
