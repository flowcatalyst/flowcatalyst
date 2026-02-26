/**
 * Audit Log Entity
 *
 * Tracks operations performed on entities. The audit log provides a complete
 * history of who did what, when, and with what parameters.
 *
 * Audit logs are created atomically with entity changes and domain events
 * by the UnitOfWork, ensuring a complete and accurate audit trail.
 */

/**
 * Audit log entry tracking an operation performed on an entity.
 */
export interface AuditLog {
	/** Unique ID (TSID) */
	readonly id: string;

	/** The type of entity (e.g., "EventType", "Client") */
	readonly entityType: string;

	/** The entity's TSID */
	readonly entityId: string;

	/** The operation name (e.g., "CreateEventType", "ArchiveEventType") */
	readonly operation: string;

	/** The full operation/command record serialized as JSON */
	readonly operationJson: string;

	/** The principal who performed the operation */
	readonly principalId: string;

	/** When the operation was performed */
	readonly performedAt: Date;
}

/**
 * Data required to create an audit log entry.
 */
export interface CreateAuditLogData {
	entityType: string;
	entityId: string;
	operation: string;
	operationJson: string;
	principalId: string;
}
