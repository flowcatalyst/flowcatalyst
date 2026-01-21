package tech.flowcatalyst.messagerouter.model;

/**
 * Result of a mediation attempt.
 *
 * <h2>Result Types</h2>
 * <ul>
 *   <li>{@link #SUCCESS} - Message processed successfully, ACK and remove from queue</li>
 *   <li>{@link #ERROR_CONNECTION} - Connection error (timeout, refused), transient, NACK for retry</li>
 *   <li>{@link #ERROR_PROCESS} - Processing error (5xx, ack=false), transient, NACK for retry</li>
 *   <li>{@link #ERROR_CONFIG} - Configuration error (4xx), permanent, ACK to prevent infinite retry</li>
 * </ul>
 */
public enum MediationResult {
    /** 200 OK - message processed successfully, ACK and remove from queue */
    SUCCESS,

    /** Connection timeout, refused, etc - transient error, NACK for visibility timeout retry */
    ERROR_CONNECTION,

    /** 5xx errors, 200 with ack=false - transient processing issues, NACK for visibility timeout retry */
    ERROR_PROCESS,

    /** 401, 403, 404, 501 - permanent config/auth errors, ACK to prevent infinite retry */
    ERROR_CONFIG
}
