package tech.flowcatalyst.dispatchjob.model;

/**
 * Result of a mediation attempt.
 *
 * NOTE: This is a copy of tech.flowcatalyst.messagerouter.model.MediationResult
 * kept in sync for the backend's dispatch job functionality.
 */
public enum MediationResult {
    SUCCESS,              // 200 OK - message processed successfully, ACK
    ERROR_CONNECTION,     // Connection timeout, refused, etc - transient, NACK for visibility timeout retry
    ERROR_SERVER,         // Deprecated: use ERROR_PROCESS for 5xx errors
    ERROR_PROCESS,        // 400, 502-599 (except 501), connection errors - transient issues, NACK for SQS visibility timeout retry
    ERROR_CONFIG          // 401, 403, 404, 501 - permanent config/auth errors, ACK to prevent retry
}
