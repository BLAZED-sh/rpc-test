import pino from 'pino';

let logger = null;

/**
 * Configure the global logger
 * @param {Object} options - Logger configuration options
 * @param {string} options.level - Log level (trace, debug, info, warn, error)
 * @param {boolean} options.noColor - Whether to disable colored output
 */
export function configureLogger(options = {}) {
  const { level = 'info', noColor = false } = options;
  
  const prettyPrint = !noColor ? {
    colorize: true,
    translateTime: 'SYS:standard',
    ignore: 'pid,hostname'
  } : {
    colorize: false,
    translateTime: 'SYS:standard',
    ignore: 'pid,hostname'
  };
  
  logger = pino({
    level,
    timestamp: pino.stdTimeFunctions.isoTime,
    transport: {
      target: 'pino-pretty',
      options: prettyPrint
    }
  });
  
  return logger;
}

/**
 * Get a child logger for a specific component
 * @param {string} component - The component name
 * @returns {Object} - A pino logger instance
 */
export function createLogger(component) {
  // Initialize default logger if not already done
  if (!logger) {
    configureLogger();
  }
  
  return logger.child({ component });
}

/**
 * Get the global logger
 * @returns {Object} - The global pino logger
 */
export function getLogger() {
  // Initialize default logger if not already done
  if (!logger) {
    configureLogger();
  }
  
  return logger;
}