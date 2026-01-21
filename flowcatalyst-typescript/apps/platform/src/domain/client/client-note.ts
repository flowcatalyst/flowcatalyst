/**
 * Client Note
 *
 * Administrative notes and audit trail for client organizations.
 */

/**
 * A note attached to a client for audit purposes.
 */
export interface ClientNote {
	/** Category of the note (e.g., "STATUS_CHANGE", "ADMIN_NOTE") */
	readonly category: string;

	/** The note text */
	readonly text: string;

	/** Who added the note (principal ID) */
	readonly addedBy: string;

	/** When the note was added */
	readonly addedAt: Date;
}

/**
 * Create a new client note.
 */
export function createClientNote(category: string, text: string, addedBy: string): ClientNote {
	return {
		category,
		text,
		addedBy,
		addedAt: new Date(),
	};
}
