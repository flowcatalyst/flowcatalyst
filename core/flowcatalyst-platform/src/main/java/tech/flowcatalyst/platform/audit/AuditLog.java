package tech.flowcatalyst.platform.audit;

import java.time.Instant;

/**
 * Audit log entry tracking operations performed on entities.
 *
 * Generic audit log that can track any entity type and operation.
 * Stores the full operation payload as JSON for complete audit trail.
 */

public class AuditLog {

    public String id;

    /**
     * The type of entity (e.g., "EventType", "Tenant").
     */
    public String entityType;

    /**
     * The entity's TSID.
     */
    public String entityId;

    /**
     * The operation name (e.g., "CreateEventType", "AddSchema").
     */
    public String operation;

    /**
     * The full operation record serialized as JSON.
     */
    public String operationJson;

    /**
     * The principal who performed the operation.
     */
    public String principalId;

    /**
     * When the operation was performed.
     */
    public Instant performedAt = Instant.now();

    public AuditLog() {
    }
}
