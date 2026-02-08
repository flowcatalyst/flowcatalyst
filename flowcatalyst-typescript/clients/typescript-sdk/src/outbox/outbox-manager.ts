import type { OutboxDriver, OutboxMessage, MessageType } from './types.js';
import { OutboxStatus } from './types.js';
import type { CreateEventDto } from './create-event-dto.js';
import type { CreateDispatchJobDto } from './create-dispatch-job-dto.js';
import type { CreateAuditLogDto } from './create-audit-log-dto.js';
import { generate } from './tsid.js';

/**
 * Manages outbox message creation for transactional outbox pattern.
 *
 * @example
 * ```typescript
 * const outbox = new OutboxManager(driver, 'client-tsid-123');
 *
 * // Create a single event
 * const eventId = await outbox.createEvent(
 *   CreateEventDto.create('user.registered', { userId: '123' })
 * );
 *
 * // Create a batch of dispatch jobs
 * const jobIds = await outbox.createDispatchJobs([job1, job2, job3]);
 * ```
 */
export class OutboxManager {
  private readonly driver: OutboxDriver;
  private readonly clientId: string;

  constructor(
    driver: OutboxDriver,
    clientId: string,
  ) {
    this.driver = driver;
    this.clientId = clientId;
  }

  /** Create a single event in the outbox. Returns the generated TSID. */
  async createEvent(event: CreateEventDto): Promise<string> {
    this.ensureClientId();

    const id = generate();
    const payload = JSON.stringify(event.toPayload());

    const message = this.buildMessage(
      id,
      'EVENT',
      payload,
      event.messageGroup,
      Object.keys(event.headers).length > 0 ? event.headers : null,
    );

    await this.driver.insert(message);
    return id;
  }

  /** Create multiple events in the outbox (batch). Returns the generated TSIDs. */
  async createEvents(events: CreateEventDto[]): Promise<string[]> {
    if (events.length === 0) return [];
    this.ensureClientId();

    const ids: string[] = [];
    const messages: OutboxMessage[] = [];

    for (const event of events) {
      const id = generate();
      ids.push(id);
      const payload = JSON.stringify(event.toPayload());

      messages.push(
        this.buildMessage(
          id,
          'EVENT',
          payload,
          event.messageGroup,
          Object.keys(event.headers).length > 0 ? event.headers : null,
        ),
      );
    }

    await this.driver.insertBatch(messages);
    return ids;
  }

  /** Create a single dispatch job in the outbox. Returns the generated TSID. */
  async createDispatchJob(job: CreateDispatchJobDto): Promise<string> {
    this.ensureClientId();

    const id = generate();
    const payload = JSON.stringify(job.toPayload());

    const message = this.buildMessage(
      id,
      'DISPATCH_JOB',
      payload,
      job.messageGroup,
      null,
    );

    await this.driver.insert(message);
    return id;
  }

  /** Create multiple dispatch jobs in the outbox (batch). Returns the generated TSIDs. */
  async createDispatchJobs(jobs: CreateDispatchJobDto[]): Promise<string[]> {
    if (jobs.length === 0) return [];
    this.ensureClientId();

    const ids: string[] = [];
    const messages: OutboxMessage[] = [];

    for (const job of jobs) {
      const id = generate();
      ids.push(id);
      const payload = JSON.stringify(job.toPayload());

      messages.push(
        this.buildMessage(
          id,
          'DISPATCH_JOB',
          payload,
          job.messageGroup,
          null,
        ),
      );
    }

    await this.driver.insertBatch(messages);
    return ids;
  }

  /** Create a single audit log in the outbox. Returns the generated TSID. */
  async createAuditLog(auditLog: CreateAuditLogDto): Promise<string> {
    this.ensureClientId();

    const id = generate();
    const payload = JSON.stringify(auditLog.toPayload());

    const message = this.buildMessage(
      id,
      'AUDIT_LOG',
      payload,
      null,
      Object.keys(auditLog.headers).length > 0 ? auditLog.headers : null,
    );

    await this.driver.insert(message);
    return id;
  }

  /** Create multiple audit logs in the outbox (batch). Returns the generated TSIDs. */
  async createAuditLogs(auditLogs: CreateAuditLogDto[]): Promise<string[]> {
    if (auditLogs.length === 0) return [];
    this.ensureClientId();

    const ids: string[] = [];
    const messages: OutboxMessage[] = [];

    for (const auditLog of auditLogs) {
      const id = generate();
      ids.push(id);
      const payload = JSON.stringify(auditLog.toPayload());

      messages.push(
        this.buildMessage(
          id,
          'AUDIT_LOG',
          payload,
          null,
          Object.keys(auditLog.headers).length > 0 ? auditLog.headers : null,
        ),
      );
    }

    await this.driver.insertBatch(messages);
    return ids;
  }

  /** Get the underlying driver. */
  getDriver(): OutboxDriver {
    return this.driver;
  }

  private buildMessage(
    id: string,
    type: MessageType,
    payload: string,
    messageGroup: string | null,
    headers: Record<string, string> | null,
  ): OutboxMessage {
    const now = new Date().toISOString();
    return {
      id,
      type,
      message_group: messageGroup,
      payload,
      status: OutboxStatus.PENDING,
      created_at: now,
      updated_at: now,
      client_id: this.clientId,
      payload_size: new TextEncoder().encode(payload).byteLength,
      headers,
    };
  }

  private ensureClientId(): void {
    if (!this.clientId) {
      throw new Error(
        'OutboxManager: clientId is required. Provide a valid client ID when constructing the OutboxManager.',
      );
    }
  }
}
