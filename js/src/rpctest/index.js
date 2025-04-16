// Export all modules
export * from './client.js';
export * from './tests.js';
export * from './report.js';
export * from './logger.js';

// Export a factory function to create a TestRunner with a client
export async function createTestRunner(endpoint) {
  const { Client } = await import('./client.js');
  const { TestRunner } = await import('./tests.js');
  
  try {
    const client = await Client.create(endpoint);
    return new TestRunner(client);
  } catch (err) {
    throw new Error(`Failed to create test runner: ${err.message}`);
  }
}