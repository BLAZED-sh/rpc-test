import chalk from 'chalk';
import { createLogger } from './logger.js';

const logger = createLogger('report');

/**
 * Report generator for benchmark results
 */
export class ReportGenerator {
  /**
   * Create a new report generator
   * @param {Array<Object>} results - Benchmark results
   */
  constructor(results) {
    this.results = results || [];
  }

  /**
   * Generate a plain text report
   * @param {boolean} [useColors=true] - Whether to use colors in the output
   * @returns {string} - Formatted text report
   */
  generateTextReport(useColors = true) {
    if (this.results.length === 0) {
      return 'No benchmark results to report.';
    }
    
    // Sort results: standard RPC tests first, then subscription tests
    const sortedResults = [...this.results].sort((a, b) => {
      // If one is a subscription test and the other isn't, put standard tests first
      const isSub1 = a.testName.startsWith('eth_') && 
                     (a.testName.includes('logs') || a.testName.includes('newHeads'));
      const isSub2 = b.testName.startsWith('eth_') && 
                     (b.testName.includes('logs') || b.testName.includes('newHeads'));
      
      if (isSub1 && !isSub2) {
        return 1; // a is subscription, b is not, so b comes first
      }
      if (!isSub1 && isSub2) {
        return -1; // a is not subscription, b is, so a comes first
      }
      
      // Otherwise sort by name
      return a.testName.localeCompare(b.testName);
    });
    
    // Prepare color functions (or no-op if colors disabled)
    const c = useColors ? {
      header: chalk.bold.blue,
      success: chalk.green,
      error: chalk.red,
      highlight: chalk.yellow,
      dim: chalk.gray,
      checkmark: chalk.green('✅'),
      xmark: chalk.red('❌')
    } : {
      header: (s) => s,
      success: (s) => s,
      error: (s) => s,
      highlight: (s) => s,
      dim: (s) => s,
      checkmark: '✅',
      xmark: '❌'
    };
    
    let output = [];
    
    // Header
    output.push(c.header('ETHEREUM RPC BENCHMARK RESULTS'));
    output.push(c.header('============================'));
    output.push('');
    
    // Calculate overall stats
    const total = sortedResults.length;
    const successful = sortedResults.filter(r => r.success).length;
    const totalDuration = sortedResults.reduce((sum, r) => sum + r.duration, 0);
    const avgDuration = total > 0 ? totalDuration / total : 0;
    const totalResponseSize = sortedResults.reduce((sum, r) => sum + r.responseSize, 0);
    
    // Write summary
    output.push(`Total tests:     ${total}`);
    output.push(`Successful:      ${successful} (${(successful / total * 100).toFixed(1)}%)`);
    output.push(`Failed:          ${total - successful} (${((total - successful) / total * 100).toFixed(1)}%)`);
    output.push(`Total duration:  ${totalDuration.toFixed(2)}ms`);
    output.push(`Avg. duration:   ${avgDuration.toFixed(2)}ms`);
    output.push(`Total response:  ${totalResponseSize} bytes`);
    output.push('');
    
    // Write detailed results
    output.push(c.header('DETAILED RESULTS'));
    output.push(c.header('================'));
    output.push('');
    
    // First write standard RPC tests
    output.push(c.header('Standard RPC Tests'));
    output.push(c.header('-----------------'));
    output.push('');
    
    let standardTestCount = 0;
    for (const result of sortedResults) {
      // Skip subscription tests
      if (result.testName.startsWith('eth_') && 
          (result.testName.includes('logs_') || result.testName.includes('newHeads'))) {
        continue;
      }
      
      standardTestCount++;
      
      // Test name and status
      if (result.success) {
        output.push(`${c.checkmark} ${c.success(result.testName)} - SUCCESS`);
      } else {
        output.push(`${c.xmark} ${c.error(result.testName)} - FAILED`);
      }
      
      // Metrics
      output.push(`   Duration:     ${result.duration}ms`);
      output.push(`   Response size: ${result.responseSize} bytes`);
      
      // Status message or error
      if (result.success) {
        output.push(`   Result: ${result.statusMessage}`);
      } else {
        output.push(`   Error: ${c.error(result.error)}`);
      }
      
      output.push('');
    }
    
    // Then write subscription tests
    output.push(c.header('Subscription Tests'));
    output.push(c.header('-----------------'));
    output.push('');
    
    let subscriptionTestCount = 0;
    for (const result of sortedResults) {
      // Only include subscription tests
      if (!result.testName.startsWith('eth_') || 
          (!result.testName.includes('logs_') && !result.testName.includes('newHeads'))) {
        continue;
      }
      
      subscriptionTestCount++;
      
      // Test name and status
      if (result.success) {
        output.push(`${c.checkmark} ${c.success(result.testName)} - SUCCESS`);
      } else {
        output.push(`${c.xmark} ${c.error(result.testName)} - FAILED`);
      }
      
      // Metrics
      output.push(`   Duration:     ${result.duration}ms`);
      
      // Status message or error
      if (result.success) {
        output.push(`   Result: ${result.statusMessage}`);
      } else {
        output.push(`   Error: ${c.error(result.error)}`);
      }
      
      output.push('');
    }
    
    // If no subscription tests were found, mention it
    if (subscriptionTestCount === 0) {
      output.push('No subscription tests run.');
      output.push('');
    }
    
    return output.join('\n');
  }

  /**
   * Generate a JSON report
   * @returns {string} - JSON string
   */
  generateJSONReport() {
    // Calculate overall stats
    const total = this.results.length;
    const successful = this.results.filter(r => r.success).length;
    const totalDuration = this.results.reduce((sum, r) => sum + r.duration, 0);
    const totalResponseSize = this.results.reduce((sum, r) => sum + r.responseSize, 0);
    
    const report = {
      summary: {
        totalTests: total,
        successfulTests: successful,
        failedTests: total - successful,
        totalDurationMs: totalDuration,
        avgDurationMs: total > 0 ? totalDuration / total : 0,
        totalResponseBytes: totalResponseSize
      },
      tests: this.results.map(result => {
        const test = {
          name: result.testName,
          success: result.success,
          durationMs: result.duration,
          responseBytes: result.responseSize
        };
        
        if (result.success) {
          test.message = result.statusMessage;
        } else {
          test.error = result.error;
        }
        
        return test;
      })
    };
    
    return JSON.stringify(report, null, 2);
  }

}
