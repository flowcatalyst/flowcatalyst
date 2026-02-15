/**
 * Dispatch Scheduler Configuration
 */

/** Minimal logger interface compatible with pino and FastifyBaseLogger. */
export interface SchedulerLogger {
  info(obj: unknown, msg?: string): void;
  warn(obj: unknown, msg?: string): void;
  error(obj: unknown, msg?: string): void;
  debug(obj: unknown, msg?: string): void;
}

export interface DispatchSchedulerConfig {
  readonly pollIntervalMs: number;
  readonly batchSize: number;
  readonly maxConcurrentGroups: number;
  readonly processingEndpoint: string;
  readonly defaultDispatchPoolCode: string;
  readonly staleQueuedThresholdMinutes: number;
  readonly staleQueuedPollIntervalMs: number;
}

export const DEFAULT_DISPATCH_SCHEDULER_CONFIG: DispatchSchedulerConfig = {
  pollIntervalMs: 5000,
  batchSize: 20,
  maxConcurrentGroups: 10,
  processingEndpoint: 'http://localhost:8080/api/dispatch/process',
  defaultDispatchPoolCode: 'DISPATCH-POOL',
  staleQueuedThresholdMinutes: 15,
  staleQueuedPollIntervalMs: 60000,
};
