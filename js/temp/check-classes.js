import { ethers } from 'ethers';

// List all provider classes
console.log('Available provider classes in ethers:');
Object.keys(ethers).filter(key => key.includes('Provider')).forEach(key => {
  console.log(`- ${key}`);
});