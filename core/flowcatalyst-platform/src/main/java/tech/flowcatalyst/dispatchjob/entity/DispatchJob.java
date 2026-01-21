package tech.flowcatalyst.dispatchjob.entity;

import tech.flowcatalyst.dispatch.DispatchMode;
import tech.flowcatalyst.dispatchjob.model.DispatchKind;
import tech.flowcatalyst.dispatchjob.model.DispatchProtocol;
import tech.flowcatalyst.dispatchjob.model.DispatchStatus;
import java.time.Instant;
import java.util.*;

/**
 * Dispatch Job aggregate root stored in MongoDB.
 *
 * <h2>Document Structure</h2>
 * <ul>
 *   <li>metadata: embedded array for flexible key-value pairs</li>
 *   <li>attempts: embedded array for delivery history</li>
 *   <li>serviceAccountId: reference to ServiceAccount for webhook credentials</li>
 * </ul>
 *
 * <h2>Kind and Code Fields</h2>
 * <p>The {@link #kind} and {@link #code} fields work together to classify
 * the dispatch job:</p>
 *
 * <table border="1">
 *   <tr><th>Kind</th><th>Code Contains</th><th>Example Code</th><th>Origin</th></tr>
 *   <tr>
 *     <td>{@code EVENT}</td>
 *     <td>Event type identifier</td>
 *     <td>{@code order.created}</td>
 *     <td>Created when a subscription matches an event</td>
 *   </tr>
 *   <tr>
 *     <td>{@code TASK}</td>
 *     <td>Task code identifier</td>
 *     <td>{@code send-welcome-email}</td>
 *     <td>Created directly via API for async work</td>
 *   </tr>
 * </table>
 *
 * <h2>Event vs Task Dispatch Jobs</h2>
 * <p><b>EVENT dispatch jobs</b> are created automatically when an event matches
 * a subscription. The {@code code} is the event type, {@code eventId} references
 * the source event, and {@code subject} is copied from the event's aggregate reference.</p>
 *
 * <p><b>TASK dispatch jobs</b> are created via API for async work execution.
 * The {@code code} is the task type identifier, {@code eventId} may optionally
 * reference a triggering event, and {@code subject} identifies the target resource.</p>
 *
 * <h2>Payload Delivery Options</h2>
 * <p>The {@link #dataOnly} flag controls how the payload is delivered:</p>
 * <ul>
 *   <li>{@code dataOnly = true}: Raw payload only, no envelope or extra headers</li>
 *   <li>{@code dataOnly = false}: JSON envelope with metadata + FlowCatalyst headers</li>
 * </ul>
 *
 * @see DispatchKind
 */

public class DispatchJob {

    public String id;

    public String externalId;

    // ========================================================================
    // Classification Fields
    // ========================================================================

    /**
     * The origin/source system that created this dispatch job.
     *
     * <p>For subscription-created jobs, this is the event source.
     * For API-created jobs, this identifies the calling system.</p>
     */
    public String source;

    /**
     * The kind (category) of this dispatch job.
     *
     * <p>Determines how the {@link #code} field should be interpreted:</p>
     * <ul>
     *   <li>{@code EVENT} - The {@code code} is an event type (e.g., {@code order.created})</li>
     *   <li>{@code TASK} - The {@code code} is a task identifier (e.g., {@code send-welcome-email})</li>
     * </ul>
     *
     * <p>For subscription-created jobs, this is always {@code EVENT}.
     * For API-created jobs, this can be either {@code EVENT} or {@code TASK}.</p>
     *
     * @see DispatchKind
     * @see #code
     */
    public DispatchKind kind = DispatchKind.EVENT;

    /**
     * The specific type code for this dispatch job.
     *
     * <p>The meaning of this field depends on {@link #kind}:</p>
     *
     * <table border="1">
     *   <tr><th>Kind</th><th>Code Meaning</th><th>Examples</th></tr>
     *   <tr>
     *     <td>{@code EVENT}</td>
     *     <td>Event type identifier from the triggering event</td>
     *     <td>{@code order.created}, {@code user.registered}, {@code payment.failed}</td>
     *   </tr>
     *   <tr>
     *     <td>{@code TASK}</td>
     *     <td>Task code identifying the work to be performed</td>
     *     <td>{@code send-welcome-email}, {@code generate-report}, {@code sync-inventory}</td>
     *   </tr>
     * </table>
     *
     * <p><b>For EVENT kind:</b> This is set automatically when a subscription
     * creates a dispatch job, copied from the event's type field.</p>
     *
     * <p><b>For TASK kind:</b> This is provided by the caller when creating
     * the dispatch job via API.</p>
     *
     * @see #kind
     */
    public String code;

    /**
     * The subject/aggregate reference for this dispatch job.
     *
     * <p>Identifies what entity or resource this dispatch job relates to.
     * Uses CloudEvents subject semantics.</p>
     *
     * <p><b>For EVENT kind:</b> Copied from the triggering event's subject
     * (e.g., {@code order:12345}, {@code user:67890}).</p>
     *
     * <p><b>For TASK kind:</b> Identifies the entity the task operates on
     * (e.g., {@code report:abc123}, {@code batch:xyz789}).</p>
     */
    public String subject;

    /**
     * The ID of the event that triggered this dispatch job.
     *
     * <p><b>For EVENT kind:</b> Required. References the event that matched
     * the subscription and caused this dispatch job to be created.</p>
     *
     * <p><b>For TASK kind:</b> Optional. May reference an event that triggered
     * the task creation, useful for tracing causation.</p>
     */
    public String eventId;

    /**
     * Correlation ID for distributed tracing.
     *
     * <p>Used to correlate this dispatch job with other operations in a
     * distributed system. Typically propagated from incoming requests or
     * set when creating the dispatch job.</p>
     */
    public String correlationId;

    // Metadata stored as embedded array
    public List<DispatchJobMetadata> metadata = new ArrayList<>();

    // Target Information
    public String targetUrl;

    public DispatchProtocol protocol = DispatchProtocol.HTTP_WEBHOOK;

    // ========================================================================
    // Payload & Delivery Options
    // ========================================================================

    /** The payload content to be delivered */
    public String payload;

    /** Content type of the payload (default: application/json) */
    public String payloadContentType = "application/json";

    /**
     * Controls how the payload is delivered to the target.
     *
     * <p>When {@code dataOnly = true} (default):</p>
     * <ul>
     *   <li>Only the raw payload is sent in the request body</li>
     *   <li>No FlowCatalyst envelope wrapping</li>
     *   <li>Standard FlowCatalyst headers are still sent (X-FlowCatalyst-ID, etc.)</li>
     * </ul>
     *
     * <p>When {@code dataOnly = false}:</p>
     * <ul>
     *   <li>Payload is wrapped in a JSON envelope with metadata</li>
     *   <li>Envelope includes: id, kind, code, subject, eventId, timestamp, data</li>
     *   <li>FlowCatalyst headers are also included</li>
     * </ul>
     *
     * <p>Example envelope ({@code dataOnly = false}):</p>
     * <pre>{@code
     * {
     *   "id": "0HZXEQ5Y8JY5Z",
     *   "kind": "EVENT",
     *   "code": "order.created",
     *   "subject": "order:12345",
     *   "eventId": "0HZXEQ5Y8JY00",
     *   "timestamp": "2024-01-15T10:30:00Z",
     *   "data": { ... original payload ... }
     * }
     * }</pre>
     */
    public boolean dataOnly = true;

    // ========================================================================
    // Credentials Reference
    // ========================================================================

    /**
     * Reference to ServiceAccount entity for webhook credentials.
     *
     * <p>The ServiceAccount contains embedded webhook credentials
     * (auth token, signing secret) used for authenticating the webhook request.</p>
     *
     * @see tech.flowcatalyst.serviceaccount.entity.ServiceAccount
     */
    public String serviceAccountId;

    // Context - Client and Subscription
    /** Client this job belongs to (nullable - null means anchor-level) */
    public String clientId;

    /** Subscription that created this job (nullable - jobs can be created directly) */
    public String subscriptionId;

    // Dispatch Behavior
    /** Processing mode (IMMEDIATE, NEXT_ON_ERROR, BLOCK_ON_ERROR) */
    public DispatchMode mode = DispatchMode.IMMEDIATE;

    /** Dispatch pool for rate limiting */
    public String dispatchPoolId;

    /** Message group for FIFO ordering (e.g., subscriptionName:eventMessageGroup) */
    public String messageGroup;

    /** Sequence number for ordering within message group (default 99) */
    public int sequence = 99;

    /** Timeout in seconds for target to respond */
    public int timeoutSeconds = 30;

    // Schema Reference
    /** Optional schema ID for payload validation (not tied to eventType) */
    public String schemaId;

    // Execution Control
    public DispatchStatus status = DispatchStatus.PENDING;

    public Integer maxRetries = 3;

    public String retryStrategy = "exponential";

    public Instant scheduledFor;

    public Instant expiresAt;

    // Tracking & Observability
    public Integer attemptCount = 0;

    public Instant lastAttemptAt;

    public Instant completedAt;

    public Long durationMillis;

    public String lastError;

    // Idempotency
    public String idempotencyKey;

    // Attempts stored as embedded array
    public List<DispatchAttempt> attempts = new ArrayList<>();

    // Timestamps
    public Instant createdAt = Instant.now();

    public Instant updatedAt = Instant.now();

    public DispatchJob() {
    }
}
