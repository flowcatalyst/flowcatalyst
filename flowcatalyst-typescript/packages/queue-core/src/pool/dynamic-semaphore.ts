/**
 * Dynamic Semaphore - permit-based concurrency limiter with in-place limit adjustment.
 *
 * Unlike p-limit, this allows changing the concurrency limit without replacing
 * the limiter instance. Active tasks continue to hold their permits; only the
 * total available permits change. This prevents the "leaky" concurrency spike
 * that occurs when replacing p-limit instances.
 *
 * Mirrors the semantics of Java's Semaphore with release(n)/tryAcquire(n).
 */
export class DynamicSemaphore {
  private _limit: number;
  private _active = 0;
  private readonly _waiting: Array<() => void> = [];

  constructor(limit: number) {
    this._limit = limit;
  }

  /**
   * Number of tasks currently holding a permit.
   */
  get activeCount(): number {
    return this._active;
  }

  /**
   * Number of tasks waiting for a permit.
   */
  get pendingCount(): number {
    return this._waiting.length;
  }

  /**
   * Current concurrency limit.
   */
  get limit(): number {
    return this._limit;
  }

  /**
   * Update the concurrency limit in-place.
   *
   * - Increasing the limit immediately releases waiters up to the new capacity.
   * - Decreasing the limit takes effect as active tasks complete; no tasks are
   *   interrupted. The active count may temporarily exceed the new limit until
   *   enough permits are released.
   */
  setLimit(newLimit: number): void {
    this._limit = Math.max(1, newLimit);
    this.drain();
  }

  /**
   * Acquire a permit. Resolves immediately if a permit is available,
   * otherwise queues until one is released.
   */
  async acquire(): Promise<void> {
    if (this._active < this._limit) {
      this._active++;
      return;
    }
    return new Promise<void>((resolve) => {
      this._waiting.push(resolve);
    });
  }

  /**
   * Release a permit and wake the next waiter if capacity allows.
   */
  release(): void {
    this._active--;
    this.drain();
  }

  /**
   * Execute a function with a permit, automatically releasing on completion.
   */
  async run<T>(fn: () => Promise<T>): Promise<T> {
    await this.acquire();
    try {
      return await fn();
    } finally {
      this.release();
    }
  }

  /**
   * Release waiting tasks up to available capacity.
   */
  private drain(): void {
    while (this._waiting.length > 0 && this._active < this._limit) {
      this._active++;
      this._waiting.shift()!();
    }
  }
}
