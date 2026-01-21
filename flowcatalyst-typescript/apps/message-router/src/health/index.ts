/**
 * Health monitoring services
 *
 * This module provides health monitoring for the message router:
 * - QueueHealthMonitor: Monitors queue backlog and growth, generates warnings
 * - BrokerHealthService: Checks broker connectivity, generates warnings on failures
 *
 * All services use neverthrow for typed error handling.
 */

export * from './errors.js';
export * from './queue-health-monitor.js';
export * from './broker-health-service.js';
