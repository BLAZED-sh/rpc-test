import { Client } from '../src/rpctest/client.js';
import { createLogger } from '../src/rpctest/logger.js';

const logger = createLogger('test-ipc');
const ipcPath = process.argv[2];

if (!ipcPath) {
  console.error('Please provide an IPC socket path as argument');
  process.exit(1);
}

async function testIpc() {
  try {
    logger.info({ path: ipcPath }, 'Testing IPC connection');
    
    // Create client
    const client = await Client.create(ipcPath);
    logger.info('Connected successfully!');
    
    // Test a simple RPC call
    const blockNumber = await client.call('eth_blockNumber');
    logger.info({ blockNumber }, 'Current block number');
    
    // Close client
    await client.close();
    logger.info('Connection closed');
  } catch (err) {
    logger.error({ error: err.message }, 'IPC test failed');
  }
}

testIpc();