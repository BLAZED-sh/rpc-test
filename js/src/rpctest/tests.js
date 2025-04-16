import { ethers } from 'ethers';
import { createLogger } from './logger.js';

const logger = createLogger('tests');

/**
 * Test runner for Ethereum JSON-RPC methods
 */
export class TestRunner {
  /**
   * Create a new test runner
   * @param {import('./client.js').Client} client - An initialized client
   */
  constructor(client) {
    this.client = client;
    this.testCases = getDefaultTestCases();
    this.subTestCases = getDefaultSubscriptionTestCases();
    this.results = [];
  }

  /**
   * Close the test runner
   */
  async close() {
    return this.client.close();
  }

  /**
   * Run standard RPC tests
   * @param {boolean} [abortOnError=false] - Whether to abort all tests on first error
   * @returns {Promise<Array<BenchmarkResult>>} - Test results
   */
  async runTests(abortOnError = false) {
    const totalTests = this.testCases.length;
    logger.info({ total: totalTests }, 'Starting tests');
    
    const results = [];
    
    for (let i = 0; i < this.testCases.length; i++) {
      const test = this.testCases[i];
      
      logger.info({
        current: i + 1,
        total: totalTests,
        test: test.name
      }, 'Running test');
      
      // Add a small delay between tests to avoid overwhelming the server
      if (i > 0) {
        await new Promise(resolve => setTimeout(resolve, 100));
      }
      
      try {
        const result = await this.runTest(test);
        results.push(result);
        
        if (result.success) {
          logger.info({
            current: i + 1,
            total: totalTests,
            test: test.name,
            duration: `${result.duration}ms`
          }, 'Test succeeded');
        } else {
          logger.info({
            current: i + 1,
            total: totalTests,
            test: test.name,
            duration: `${result.duration}ms`,
            error: result.error
          }, 'Test failed');
          
          if (abortOnError) {
            logger.warn('Aborting remaining tests due to error');
            break;
          }
        }
      } catch (err) {
        const result = {
          testName: test.name,
          success: false,
          duration: 0,
          error: err.message,
          responseSize: 0,
          statusMessage: 'Test execution error'
        };
        
        results.push(result);
        
        logger.error({
          current: i + 1,
          total: totalTests,
          test: test.name,
          error: err.message
        }, 'Test execution error');
        
        if (abortOnError) {
          logger.warn('Aborting remaining tests due to error');
          break;
        }
      }
    }
    
    this.results = results;
    return results;
  }

  /**
   * Run subscription tests
   * @param {boolean} [shareClient=false] - Whether to use a shared client for all tests
   * @returns {Promise<Array<BenchmarkResult>>} - Test results
   */
  async runSubscriptionTests(shareClient = false) {
    // Check if the transport supports subscriptions
    if (!this.client.supportsSubscriptions) {
      logger.warn(
        'The current transport does not support subscriptions (HTTP endpoints cannot be used for subscriptions)'
      );
      
      // Return empty results marked as failed
      const results = this.subTestCases.map(test => ({
        testName: test.name,
        success: false,
        error: 'Transport does not support subscriptions',
        statusMessage: 'Skipped - transport does not support subscriptions',
        duration: 0,
        responseSize: 0
      }));
      
      return results;
    }
    
    const totalTests = this.subTestCases.length;
    logger.info({ total: totalTests }, 'Starting subscription tests');
    
    const results = [];
    const endpoint = this.client.endpoint;
    
    for (let i = 0; i < this.subTestCases.length; i++) {
      const test = this.subTestCases[i];
      
      logger.info({
        current: i + 1,
        total: totalTests,
        test: test.name
      }, 'Running subscription test');
      
      try {
        let testClient;
        
        // Use shared client or create a fresh one for each test
        if (shareClient) {
          logger.debug({ test: test.name }, 'Using shared client for subscription test');
          testClient = this.client;
        } else {
          logger.debug({ test: test.name }, 'Creating fresh client for subscription test');
          const { Client } = await import('./client.js');
          testClient = await Client.create(endpoint);
        }
        
        // Run the subscription test
        const result = await runSubscriptionTestWithClient(testClient, test);
        results.push(result);
        
        // Close the test-specific client if we didn't use the shared one
        if (!shareClient && testClient !== this.client) {
          await testClient.close();
        }
        
        if (result.success) {
          logger.info({
            current: i + 1,
            total: totalTests,
            test: test.name,
            duration: `${result.duration}ms`
          }, 'Subscription test succeeded');
        } else {
          logger.info({
            current: i + 1,
            total: totalTests,
            test: test.name,
            duration: `${result.duration}ms`,
            error: result.error
          }, 'Subscription test failed');
        }
      } catch (err) {
        const result = {
          testName: test.name,
          success: false,
          duration: 0,
          error: err.message,
          responseSize: 0,
          statusMessage: 'Test initialization failed'
        };
        
        results.push(result);
        
        logger.error({
          test: test.name,
          error: err.message
        }, 'Failed to run subscription test');
      }
    }
    
    // Add subscription results to the overall results
    this.results = [...this.results, ...results];
    return results;
  }

  /**
   * Run a single test
   * @param {TestCase} test - The test case to run
   * @returns {Promise<BenchmarkResult>} - Test result
   */
  async runTest(test) {
    const result = {
      testName: test.name,
      success: false,
      duration: 0,
      error: '',
      responseSize: 0,
      statusMessage: ''
    };
    
    logger.debug({ test: test.name }, 'Executing test');
    
    // Execute the test with timing
    const start = Date.now();
    let response;
    
    try {
      response = await this.client.call(test.method, ...test.params);
      const responseJson = JSON.stringify(response);
      result.responseSize = responseJson.length;
      
      logger.debug({
        test: test.name,
        response_size: result.responseSize
      }, 'Received response');
      
      // Validate the result
      const validation = await test.validator(response);
      result.success = validation.success;
      result.statusMessage = validation.message;
      
      if (!validation.success) {
        result.error = validation.message;
        logger.debug({
          test: test.name,
          error: validation.message
        }, 'Validation failed');
      } else {
        logger.debug({
          test: test.name,
          message: validation.message
        }, 'Validation succeeded');
      }
    } catch (err) {
      result.success = false;
      result.error = err.message;
      logger.debug({
        error: err.message,
        test: test.name
      }, 'Test failed');
    } finally {
      result.duration = Date.now() - start;
    }
    
    return result;
  }
}

/**
 * Run a subscription test with a specific client
 * @param {import('./client.js').Client} client - Ethereum client
 * @param {SubscriptionTestCase} test - The subscription test case
 * @returns {Promise<BenchmarkResult>} - Test result
 */
async function runSubscriptionTestWithClient(client, test) {
  const result = {
    testName: test.name,
    success: false,
    duration: 0,
    error: '',
    responseSize: 0,
    statusMessage: ''
  };
  
  logger.debug({ test: test.name }, 'Testing subscription');
  
  // Variables to track events
  let eventCount = 0;
  let validEvents = 0;
  const events = [];
  
  // Start timing
  const start = Date.now();
  
  try {
    // Create a Promise that will resolve when we've received enough events
    const eventsPromise = new Promise((resolve, reject) => {
      // Set timeout for the subscription test (default: 30 seconds)
      const timeoutId = setTimeout(() => {
        if (validEvents >= test.minEvents) {
          resolve({ timeout: true });
        } else {
          reject(new Error(`Timeout reached with only ${validEvents} valid events (${eventCount} total), needed ${test.minEvents}`));
        }
      }, 30000);
      
      // Create event handler that will be called for each subscription event
      const handler = (data) => {
        try {
          // Parse the data if it's a string
          let parsedData;
          if (typeof data === 'string') {
            try {
              parsedData = JSON.parse(data);
            } catch (err) {
              logger.warn({
                error: err.message,
                test: test.name
              }, 'Failed to parse subscription data');
              return;
            }
          } else {
            parsedData = data;
          }
          
          logger.debug({
            test: test.name,
            data: typeof parsedData === 'object' ? 'object' : data
          }, 'Subscription event received');
          
          eventCount++;
          events.push(parsedData);
          
          // Validate event if validator is provided
          if (test.validator) {
            const valid = test.validator(parsedData);
            if (valid.success) {
              validEvents++;
              logger.debug({
                test: test.name,
                message: valid.message
              }, 'Valid event received');
              
              // If we've received enough valid events, resolve the promise
              if (validEvents >= test.minEvents) {
                clearTimeout(timeoutId);
                resolve({ timeout: false });
              }
            } else {
              logger.warn({
                test: test.name,
                error: valid.message
              }, 'Invalid event received');
            }
          } else {
            // Consider all events valid if no validator
            validEvents++;
            
            // If we've received enough valid events, resolve the promise
            if (validEvents >= test.minEvents) {
              clearTimeout(timeoutId);
              resolve({ timeout: false });
            }
          }
        } catch (err) {
          logger.error({
            error: err.message,
            test: test.name
          }, 'Error processing subscription event');
        }
      };
      
      // Subscribe to the event
      let subscriptionId;
      client.subscribe(test.namespace, test.method, handler, test.params)
        .then(subId => {
          subscriptionId = subId;
          logger.info({
            test: test.name,
            subscription_id: subId,
            min_events: test.minEvents
          }, 'Waiting for subscription events');
          
          // Store the subscription ID for later unsubscribing
          result.subscriptionId = subId;
        })
        .catch(err => {
          clearTimeout(timeoutId);
          reject(err);
        });
    });
    
    // Wait for the events promise to resolve or reject
    const { timeout } = await eventsPromise;
    
    // Unsubscribe if we have a subscription ID
    if (result.subscriptionId) {
      try {
        const unsubResult = await client.unsubscribe(test.namespace, result.subscriptionId);
        if (unsubResult) {
          logger.debug({ subscription_id: result.subscriptionId }, 'Unsubscribed successfully');
        } else {
          logger.warn({ subscription_id: result.subscriptionId }, 'Subscription may not have been fully unsubscribed');
        }
      } catch (err) {
        logger.error({
          error: err.message,
          test: test.name,
          subscription_id: result.subscriptionId
        }, 'Unsubscribe failed');
        
        // Even if unsubscribe fails, we want to make sure we log it but don't fail the test
        // since the cleanup will happen when the client is closed
      }
    }
    
    // Calculate duration
    result.duration = Date.now() - start;
    
    // Check if we received enough events
    if (validEvents >= test.minEvents) {
      result.success = true;
      result.statusMessage = `Received ${validEvents} valid events (${eventCount} total)${timeout ? ' (timed out)' : ''}`;
    } else {
      result.success = false;
      result.error = `Received only ${validEvents} valid events (${eventCount} total), needed ${test.minEvents}`;
    }
    
    // Rough estimate of response size
    result.responseSize = events.reduce((size, event) => {
      return size + JSON.stringify(event).length;
    }, 0);
    
    logger.info({
      test: test.name,
      subscription_id: result.subscriptionId,
      events: eventCount,
      valid_events: validEvents,
      success: result.success
    }, 'Subscription test completed');
    
    return result;
  } catch (err) {
    // Calculate duration even for failures
    result.duration = Date.now() - start;
    result.success = false;
    result.error = err.message;
    
    logger.error({
      error: err.message,
      test: test.name
    }, 'Subscription test failed');
    
    return result;
  }
}

/**
 * Helper function to convert hex string to BigInt
 * Works with different versions of ethers.js
 * @param {string} hexString - Hex string to convert
 * @returns {bigint} - BigInt value
 */
function hexToBigInt(hexString) {
  // Check if we have a valid hex string (should start with 0x)
  if (typeof hexString !== 'string' || !hexString.startsWith('0x')) {
    throw new Error(`Invalid hex string: ${hexString}`);
  }
  
  // Use native BigInt to convert hex to a bigint value
  return BigInt(hexString);
}

/**
 * Get the default set of RPC test cases
 * @returns {Array<TestCase>} - Array of test cases
 */
export function getDefaultTestCases() {
  return [
    {
      name: 'net_version',
      description: 'Get the current network ID',
      method: 'net_version',
      params: [],
      validator: async (result) => {
        try {
          const version = result;
          return {
            success: true,
            message: `Network version: ${version}`
          };
        } catch (err) {
          return {
            success: false,
            message: `Failed to parse result: ${err.message}`
          };
        }
      }
    },
    {
      name: 'eth_chainId',
      description: 'Get the chain ID',
      method: 'eth_chainId',
      params: [],
      validator: async (result) => {
        try {
          const chainId = result;
          // Convert hex to decimal
          const id = parseInt(chainId, 16);
          return {
            success: true,
            message: `Chain ID: ${id}`
          };
        } catch (err) {
          return {
            success: false,
            message: `Failed to parse result: ${err.message}`
          };
        }
      }
    },
    {
      name: 'eth_blockNumber',
      description: 'Get the current block number',
      method: 'eth_blockNumber',
      params: [],
      validator: async (result) => {
        try {
          const blockNumber = result;
          // Convert hex to decimal
          const number = parseInt(blockNumber, 16);
          return {
            success: true,
            message: `Block number: ${number}`
          };
        } catch (err) {
          return {
            success: false,
            message: `Failed to parse result: ${err.message}`
          };
        }
      }
    },
    {
      name: 'eth_getBlockByNumber',
      description: 'Get the latest block data',
      method: 'eth_getBlockByNumber',
      params: ['latest', true], // true to include full transaction objects
      validator: async (result) => {
        try {
          const block = result;
          
          if (!block) {
            return {
              success: false,
              message: 'Received null block'
            };
          }
          
          // Check for required fields
          const requiredFields = [
            'number',
            'hash',
            'parentHash',
            'timestamp',
            'gasUsed',
            'gasLimit'
          ];
          
          for (const field of requiredFields) {
            if (!(field in block)) {
              return {
                success: false,
                message: `Block missing required field: ${field}`
              };
            }
          }
          
          // Convert block number to decimal
          const blockNumber = parseInt(block.number, 16);
          
          // Count transactions
          const txCount = Array.isArray(block.transactions) ? block.transactions.length : 0;
          
          return {
            success: true,
            message: `Block #${blockNumber} with ${txCount} transactions`
          };
        } catch (err) {
          return {
            success: false,
            message: `Failed to parse block: ${err.message}`
          };
        }
      }
    },
    {
      name: 'eth_call_erc20_totalSupply',
      description: 'Call totalSupply on a popular ERC20 token (USDC)',
      method: 'eth_call',
      params: [
        {
          to: '0xa0b86991c6218b36c1d19d4a2e9eb0ce3606eb48', // USDC contract
          data: '0x18160ddd' // totalSupply()
        },
        'latest'
      ],
      validator: async (result) => {
        try {
          const hexResult = result;
          
          if (!hexResult || hexResult.length < 3) {
            return {
              success: false,
              message: 'Invalid result length'
            };
          }
          
          // Convert hex to decimal using native BigInt
          const supply = hexToBigInt(hexResult);
          
          return {
            success: true,
            message: `Token supply: ${supply.toString()}`
          };
        } catch (err) {
          return {
            success: false,
            message: `Failed to parse result: ${err.message}`
          };
        }
      }
    },
    {
      name: 'eth_gasPrice',
      description: 'Get the current gas price',
      method: 'eth_gasPrice',
      params: [],
      validator: async (result) => {
        try {
          const gasPrice = result;
          // Convert hex to decimal using native BigInt
          const price = hexToBigInt(gasPrice);
          
          return {
            success: true,
            message: `Gas price: ${price.toString()} wei`
          };
        } catch (err) {
          return {
            success: false,
            message: `Failed to parse result: ${err.message}`
          };
        }
      }
    },
    {
      name: 'eth_getBalance',
      description: 'Get the balance of the Ethereum foundation',
      method: 'eth_getBalance',
      params: ['0xde0b295669a9fd93d5f28d9ec85e40f4cb697bae', 'latest'],
      validator: async (result) => {
        try {
          const balance = result;
          // Convert hex to decimal using native BigInt
          const bal = hexToBigInt(balance);
          
          return {
            success: true,
            message: `Balance: ${bal.toString()} wei`
          };
        } catch (err) {
          return {
            success: false,
            message: `Failed to parse result: ${err.message}`
          };
        }
      }
    }
  ];
}

/**
 * Get the default set of subscription test cases
 * @returns {Array<SubscriptionTestCase>} - Array of subscription test cases
 */
export function getDefaultSubscriptionTestCases() {
  return [
    {
      name: 'eth_logs_usdt',
      description: 'Subscribe to logs for USDT transfers - high volume token',
      namespace: 'eth',
      method: 'logs',
      params: {
        address: '0xdAC17F958D2ee523a2206206994597C13D831ec7', // USDT contract
        topics: ['0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef'] // Transfer(address,address,uint256)
      },
      minEvents: 1,
      validator: (log) => {
        try {
          if (!log) {
            return {
              success: false,
              message: 'Received null log'
            };
          }
          
          // Check for required fields
          const requiredFields = ['address', 'topics', 'data', 'blockNumber'];
          for (const field of requiredFields) {
            if (!(field in log)) {
              return {
                success: false,
                message: `Log missing required field: ${field}`
              };
            }
          }
          
          // Check that the address is USDT (case insensitive)
          const address = typeof log.address === 'string' ? log.address.toLowerCase() : '';
          if (address !== '0xdac17f958d2ee523a2206206994597c13d831ec7') {
            return {
              success: false,
              message: `Unexpected contract address: ${address}`
            };
          }
          
          // Extract some data for better log message
          const blockNumberHex = log.blockNumber || 'unknown';
          const txHash = log.transactionHash || 'unknown';
          
          return {
            success: true,
            message: `Received valid USDT transfer log in block ${blockNumberHex}, tx: ${txHash}`
          };
        } catch (err) {
          return {
            success: false,
            message: `Failed to validate log: ${err.message}`
          };
        }
      }
    },
    {
      name: 'eth_logs_usdc',
      description: 'Subscribe to logs for USDC transfers - high volume token',
      namespace: 'eth',
      method: 'logs',
      params: {
        address: '0xa0b86991c6218b36c1d19d4a2e9eb0ce3606eb48', // USDC contract
        topics: ['0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef'] // Transfer(address,address,uint256)
      },
      minEvents: 1,
      validator: (log) => {
        try {
          if (!log) {
            return {
              success: false,
              message: 'Received null log'
            };
          }
          
          // Check for required fields
          const requiredFields = ['address', 'topics', 'data', 'blockNumber'];
          for (const field of requiredFields) {
            if (!(field in log)) {
              return {
                success: false,
                message: `Log missing required field: ${field}`
              };
            }
          }
          
          // Check that the address is USDC (case insensitive)
          const logAddr = typeof log.address === 'string' ? log.address.toLowerCase() : '';
          const targetContract = '0xa0b86991c6218b36c1d19d4a2e9eb0ce3606eb48'.toLowerCase();
          
          // Try to extract topics for a potential "transfer to" check
          let toAddr = '';
          
          if (Array.isArray(log.topics) && log.topics.length >= 3) {
            // The 3rd topic (index 2) is usually the destination address in Transfer events
            const topicStr = log.topics[2];
            if (typeof topicStr === 'string' && topicStr.length >= 42) {
              // Extract the last 40 characters (20 bytes) of the topic and add 0x prefix
              toAddr = '0x' + topicStr.slice(-40).toLowerCase();
            }
          }
          
          // Check for USDC address either as the contract address or transfer recipient
          if (logAddr !== targetContract && toAddr !== targetContract) {
            return {
              success: false,
              message: `Unexpected contract / target address: ${logAddr} / ${toAddr}`
            };
          }
          
          // Extract some data for better log message
          const blockNumberHex = log.blockNumber || 'unknown';
          const txHash = log.transactionHash || 'unknown';
          
          return {
            success: true,
            message: `Received valid USDC transfer log in block ${blockNumberHex}, tx: ${txHash}`
          };
        } catch (err) {
          return {
            success: false, 
            message: `Failed to validate log: ${err.message}`
          };
        }
      }
    }
  ];
}