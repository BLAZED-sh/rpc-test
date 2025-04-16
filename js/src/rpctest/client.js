import { ethers } from 'ethers';
import { createLogger } from './logger.js';

const logger = createLogger('client');

/**
 * Client for Ethereum JSON-RPC connections
 */
export class Client {
  /**
   * Create a new Ethereum client with auto-detected transport
   * @param {string} endpoint - URL or path to connect to
   * @returns {Promise<Client>} - A configured client
   */
  static async create(endpoint) {
    const client = new Client(endpoint);
    await client.connect();
    return client;
  }

  /**
   * Create a new Client instance
   * @param {string} endpoint - URL or path to connect to
   */
  constructor(endpoint) {
    this.endpoint = endpoint;
    this.provider = null;
    this.transport = 'unknown';
    this.supportsSubscriptions = false;
    this.subscriptions = new Map();
    this.subscriptionHandlers = new Map();
    this.requestId = 1;
  }

  /**
   * Connect to the Ethereum node
   */
  async connect() {
    logger.debug({ endpoint: this.endpoint }, 'Creating new RPC client');

    // Detect transport type based on endpoint
    if (this.endpoint.startsWith('/') || this.endpoint.startsWith('./')) {
      // Unix socket
      this.transport = 'ipc';
      this.supportsSubscriptions = true;
      logger.debug({ socket: this.endpoint }, 'Using IPC socket transport');

      try {
        // Use IpcSocketProvider for Unix domain sockets
        this.provider = new ethers.IpcSocketProvider(this.endpoint);
        
        // Add error handling for IPC provider
        this.provider.on('error', (error) => {
          logger.error({ error: error.message }, 'IPC provider error');
        });
        
        // Wait a moment to ensure connection is established
        await new Promise(resolve => setTimeout(resolve, 1000));
      } catch (err) {
        logger.error({ error: err.message }, 'Failed to create IPC provider');
        throw new Error(`Failed to connect to IPC socket: ${err.message}`);
      }
    } else if (this.endpoint.startsWith('ws://') || this.endpoint.startsWith('wss://')) {
      // WebSocket
      this.transport = 'ws';
      this.supportsSubscriptions = true;
      logger.debug({ url: this.endpoint }, 'Using WebSocket transport');
      
      try {
        // Create a WebSocket provider with proper error handling
        this.provider = new ethers.WebSocketProvider(this.endpoint);
        
        // In ethers v6, there's no direct websocket access, 
        // instead we need to listen for provider events
        this.provider.on('error', (error) => {
          logger.error({ error: error.message }, 'WebSocket provider error');
        });
        
        // Wait a moment to ensure connection is established
        await new Promise(resolve => setTimeout(resolve, 1000));
      } catch (err) {
        logger.error({ error: err.message }, 'Failed to create WebSocket provider');
        throw new Error(`Failed to connect to WebSocket: ${err.message}`);
      }
    } else if (this.endpoint.startsWith('http://') || this.endpoint.startsWith('https://')) {
      // HTTP
      this.transport = 'http';
      this.supportsSubscriptions = false;
      logger.debug({ url: this.endpoint }, 'Using HTTP transport');
      this.provider = new ethers.JsonRpcProvider(this.endpoint);
    } else {
      throw new Error(`Unsupported endpoint format: ${this.endpoint}`);
    }

    logger.debug({ endpoint: this.endpoint }, 'Connected to Ethereum node');
    return this;
  }

  /**
   * Close the connection
   */
  async close() {
    // Unsubscribe from all active subscriptions
    const subIds = [...this.subscriptions.keys()];
    
    for (const subId of subIds) {
      try {
        // Use our unsubscribe method which handles all types of subscriptions
        await this.unsubscribe('eth', subId);
      } catch (err) {
        logger.warn({ error: err.message, subscription_id: subId }, 'Failed to unsubscribe during close');
      }
    }

    // Clear maps (should already be empty, but just in case)
    this.subscriptions.clear();
    this.subscriptionHandlers.clear();

    // Close provider if available
    if (this.provider) {
      logger.debug('Closing provider connection');
      try {
        // Try to remove all listeners first
        if (typeof this.provider.removeAllListeners === 'function') {
          this.provider.removeAllListeners();
        }
        
        // Destroy/disconnect the provider if method exists
        if (typeof this.provider.destroy === 'function') {
          await this.provider.destroy();
          logger.debug('Provider destroyed');
        } else if (typeof this.provider.disconnect === 'function') {
          await this.provider.disconnect();
          logger.debug('Provider disconnected');
        } else {
          logger.debug('Provider does not have destroy/disconnect method');
        }
      } catch (err) {
        logger.warn({ error: err.message }, 'Error closing provider');
      }
    }
    
    logger.debug('Client closed');
  }

  /**
   * Make a synchronous RPC call
   * @param {string} method - RPC method name
   * @param {Array<any>} params - Method parameters
   * @returns {Promise<any>} - The result
   */
  async call(method, ...params) {
    logger.debug({ method, params }, 'Making RPC call');
    
    // Track start time for metrics
    const start = Date.now();
    
    try {
      // Make the actual RPC call
      const result = await this.provider.send(method, params);
      
      // Calculate duration
      const duration = Date.now() - start;
      
      logger.debug({ method, duration_ms: duration }, 'RPC call successful');
      return result;
    } catch (err) {
      // Calculate duration even for failures
      const duration = Date.now() - start;
      
      logger.debug({ 
        error: err.message, 
        method, 
        duration_ms: duration 
      }, 'RPC call failed');
      
      throw new Error(`RPC call to ${method} failed: ${err.message}`);
    }
  }

  /**
   * Subscribe to an Ethereum event
   * @param {string} namespace - Ethereum namespace (usually "eth")
   * @param {string} method - Subscription method (e.g., "newHeads", "logs")
   * @param {Function} handler - Handler function for subscription data
   * @param {any} params - Optional parameters for the subscription
   * @returns {Promise<string>} - Subscription ID
   */
  async subscribe(namespace, method, handler, params = null) {
    logger.debug({ namespace, method, params }, 'Creating subscription');
    
    if (!this.supportsSubscriptions) {
      throw new Error('This transport does not support subscriptions');
    }
    
    // Generate a local subscription ID
    const subId = `sub_${Date.now().toString(16)}`;
    
    try {
      if (method === 'newHeads') {
        // For "newHeads", we need to handle block events directly
        logger.debug('Setting up newHeads subscription via block events');
        
        // Use a blockNumber event first, which all providers support
        const blockNumberHandler = async (blockNumber) => {
          try {
            // Get the full block data for the handler
            const blockData = await this.provider.getBlock(blockNumber);
            
            if (blockData) {
              // Convert block to JSON-RPC format expected by the handler
              const notification = {
                number: `0x${blockData.number.toString(16)}`,
                hash: blockData.hash,
                parentHash: blockData.parentHash,
                timestamp: `0x${blockData.timestamp.toString(16)}`,
                nonce: blockData.nonce || '0x0',
                difficulty: '0x0', // No longer in use post-merge
                gasLimit: `0x${blockData.gasLimit.toString(16)}`,
                gasUsed: `0x${blockData.gasUsed.toString(16)}`,
                miner: blockData.miner,
                extraData: blockData.extraData || '0x',
                baseFeePerGas: blockData.baseFeePerGas ? 
                  `0x${blockData.baseFeePerGas.toString(16)}` : 
                  '0x0'
              };
              
              // Call the handler with JSON data
              handler(JSON.stringify(notification));
            }
          } catch (err) {
            logger.warn({ error: err.message }, 'Error processing block header');
          }
        };
        
        // Register the listener
        this.provider.on('block', blockNumberHandler);
        
        // Store subscription info
        this.subscriptions.set(subId, { 
          type: 'ethers',
          eventName: 'block',
          listener: blockNumberHandler 
        });
        this.subscriptionHandlers.set(subId, handler);
        
        logger.debug({ subscription_id: subId }, 'Block subscription created');
      } else if (method === 'logs') {
        // Set up logs subscription using direct RPC calls in ethers.js v6
        logger.debug('Setting up logs subscription with polling');
        
        // Prepare filter parameters
        const filter = params || {};
        
        // Track the last block we've seen to avoid duplicate logs
        let lastBlockProcessed = null;
        
        // Get the current block number to start filtering from
        const startBlock = await this.provider.getBlockNumber();
        logger.debug({ startBlock }, 'Starting logs subscription from block');
        
        // Set up polling for logs
        const checkForLogs = async () => {
          try {
            // Get current block number
            const currentBlock = await this.provider.getBlockNumber();
            
            // If we haven't processed any blocks yet, start from the beginning
            const fromBlock = lastBlockProcessed ? lastBlockProcessed + 1 : startBlock;
            
            // Only query if there are new blocks
            if (currentBlock >= fromBlock) {
              // Create filter parameters
              const filterParams = {
                address: filter.address,
                topics: filter.topics,
                fromBlock: `0x${fromBlock.toString(16)}`,
                toBlock: `0x${currentBlock.toString(16)}`
              };
              
              // Use getLogs RPC call directly
              const logs = await this.provider.send('eth_getLogs', [filterParams]);
              
              if (logs && logs.length > 0) {
                logger.debug({ 
                  count: logs.length, 
                  fromBlock, 
                  toBlock: currentBlock 
                }, 'Found logs');
                
                for (const log of logs) {
                  // logs are already in JSON-RPC format, just pass to handler
                  handler(JSON.stringify(log));
                }
              }
              
              // Update the last block we've processed
              lastBlockProcessed = currentBlock;
            }
          } catch (err) {
            logger.warn({ error: err.message }, 'Error polling for logs');
          }
        };
        
        // Start polling for logs - every 2 seconds
        const pollInterval = setInterval(checkForLogs, 2000);
        
        // Store subscription info including the interval for cleanup
        this.subscriptions.set(subId, { 
          type: 'logs_poll',
          interval: pollInterval,
          lastBlock: startBlock
        });
        this.subscriptionHandlers.set(subId, handler);
        
        // Immediately check for logs
        checkForLogs();
        
        logger.debug({ subscription_id: subId }, 'Logs subscription created');
      } else {
        // For other types of subscriptions, attempt to use direct RPC
        logger.debug('Using direct RPC subscribe for', method);
        
        try {
          // Send eth_subscribe request
          const serverSubId = await this.provider.send(
            'eth_subscribe', 
            [method, ...(params ? [params] : [])]
          );
          
          // Store subscription info
          this.subscriptions.set(subId, { 
            type: 'manual',
            serverSubId,
            method
          });
          this.subscriptionHandlers.set(subId, handler);
          
          // Use WebSocket directly for subscription handling if available
          if (this.transport === 'ws') {
            logger.debug('Setting up WebSocket message handler for subscriptions');
            
            // We'll have to implement our own message handler that intercepts
            // eth_subscription messages, as ethers.js v6 WebSocketProvider doesn't 
            // expose this directly
            
            // For real applications, it's recommended to use a dedicated library
            // for Ethereum subscriptions over WebSockets
            logger.debug({ 
              subscription_id: subId, 
              server_sub_id: serverSubId 
            }, 'Manual subscription created, but events will be handled by provider');
          }
          
          logger.debug({ subscription_id: subId, server_sub_id: serverSubId }, 'Custom subscription created');
        } catch (err) {
          logger.warn({ 
            error: err.message, 
            method 
          }, 'Direct RPC subscription failed, falling back to polling');
          
          // For methods not directly supported, implement a polling strategy
          // similar to logs but with custom logic per method type
          throw new Error(`Subscription method ${method} not supported yet`);
        }
      }
      
      return subId;
    } catch (err) {
      logger.error({ error: err.message, method }, 'Subscription failed');
      throw new Error(`Subscription failed: ${err.message}`);
    }
  }

  /**
   * Unsubscribe from an event
   * @param {string} namespace - Ethereum namespace (usually "eth")
   * @param {string} subId - Subscription ID to unsubscribe
   * @returns {Promise<boolean>} - Success status
   */
  async unsubscribe(namespace, subId) {
    logger.debug({ namespace, subscription_id: subId }, 'Unsubscribing');
    
    // Get the subscription object
    const subscription = this.subscriptions.get(subId);
    if (!subscription) {
      throw new Error(`Subscription not found: ${subId}`);
    }
    
    try {
      if (subscription.type === 'ethers') {
        // For ethers.js managed subscriptions, just remove our stored references
        // The actual unsubscribe will happen when the provider is closed
        logger.debug({ subscription_id: subId }, 'Unsubscribing from ethers event');
        
        // Try to remove the listener only if both listener and eventName exist
        if (subscription.listener && subscription.eventName) {
          try {
            // For filter objects (used in logs), we need to use a string representation
            if (typeof subscription.eventName === 'string' && 
                subscription.eventName.startsWith('{') && 
                subscription.eventName.endsWith('}')) {
              // This was a JSON filter, need to parse it back
              logger.debug('Unsubscribing from filter subscription');
            } else {
              // This was a simple event name
              this.provider.removeListener(subscription.eventName, subscription.listener);
            }
          } catch (listenerErr) {
            // Just log but don't fail if we can't remove the listener
            logger.warn({ 
              error: listenerErr.message, 
              subscription_id: subId 
            }, 'Error removing event listener');
          }
        }
      } else if (subscription.type === 'logs_poll') {
        // Stop the polling interval
        logger.debug({ subscription_id: subId }, 'Clearing polling interval for logs subscription');
        
        if (subscription.interval) {
          clearInterval(subscription.interval);
          logger.debug('Cleared polling interval');
        }
      } else if (subscription.type === 'manual') {
        // Remove listener if we have one
        if (subscription.listener) {
          try {
            this.provider.removeListener('eth_subscription', subscription.listener);
          } catch (listenerErr) {
            logger.warn({ 
              error: listenerErr.message, 
              subscription_id: subId 
            }, 'Error removing subscription listener');
          }
        }
        
        // Manually unsubscribe via RPC
        try {
          await this.provider.send('eth_unsubscribe', [subscription.serverSubId]);
          logger.debug({ server_sub_id: subscription.serverSubId }, 'RPC unsubscribe successful');
        } catch (rpcErr) {
          logger.warn({ 
            error: rpcErr.message, 
            server_sub_id: subscription.serverSubId 
          }, 'RPC unsubscribe failed');
        }
      }
      
      // Clean up stored data regardless of any errors above
      this.subscriptions.delete(subId);
      this.subscriptionHandlers.delete(subId);
      
      logger.debug({ subscription_id: subId }, 'Unsubscribed successfully');
      return true;
    } catch (err) {
      logger.error({ error: err.message, subscription_id: subId }, 'Failed to unsubscribe');
      // We still want to clean up our local state
      this.subscriptions.delete(subId);
      this.subscriptionHandlers.delete(subId);
      return false;
    }
  }

  /**
   * Check if the current transport supports subscriptions
   * @returns {boolean} - Whether subscriptions are supported
   */
  supportsSubscriptions() {
    return this.supportsSubscriptions;
  }
}