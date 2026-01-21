/**
 * Audit Log Entity
 *
 * Represents an audit log entry recording a state change.
 */

/**
 * Audit log entry.
 */
export interface AuditLog {
	/** TSID primary key */
	readonly id: string;

	/** Type of entity that was modified */
	readonly entityType: string;

	/** ID of the entity that was modified */
	readonly entityId: string;

	/** Name of the operation performed */
	readonly operation: string;

	/** Full operation payload as JSON */
	readonly operationJson: unknown | null;

	/** ID of the principal who performed the operation */
	readonly principalId: string | null;

	/** When the operation was performed */
	readonly performedAt: Date;
}

/**
 * Audit log with resolved principal name.
 */
export interface AuditLogWithPrincipal extends AuditLog {
	/** Name of the principal (resolved from principalId) */
	readonly principalName: string | null;
}
