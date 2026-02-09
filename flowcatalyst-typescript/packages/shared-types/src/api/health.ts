/**
 * GET /health/live - Liveness probe response
 * GET /health/ready - Readiness probe response
 * GET /health/startup - Startup probe response
 */
export interface HealthCheckResponse {
  status: string;
  timestamp: string;
  issues: string[];
}

/**
 * GET /monitoring/health - System health response
 */
export interface MonitoringHealthResponse {
  status: string;
  timestamp: string;
  uptimeMillis: number;
  details: {
    totalQueues: number;
    healthyQueues: number;
    totalPools: number;
    healthyPools: number;
    activeWarnings: number;
    criticalWarnings: number;
    circuitBreakersOpen: number;
    degradationReason: string | null;
  };
}
