/**
 * Client Entity
 *
 * Client organization. Only customers get clients (partners don't).
 */

import { generate } from '@flowcatalyst/tsid';
import type { ClientStatus } from './client-status.js';
import type { ClientNote } from './client-note.js';
import { createClientNote } from './client-note.js';

/**
 * Client organization entity.
 */
export interface Client {
	/** TSID primary key */
	readonly id: string;

	/** Client name */
	readonly name: string;

	/**
	 * Unique client identifier/slug.
	 * Max 60 characters, lowercase, may include hyphens and underscores.
	 * Example: "acme-corp", "flowcatalyst_demo"
	 */
	readonly identifier: string;

	/** Client status */
	readonly status: ClientStatus;

	/**
	 * Free-form reason for current status (e.g., "ACCOUNT_NOT_PAID", "TRIAL_EXPIRED").
	 * Applications can use their own codes.
	 */
	readonly statusReason: string | null;

	/** When the status was last changed */
	readonly statusChangedAt: Date | null;

	/** Administrative notes and audit trail */
	readonly notes: readonly ClientNote[];

	/** When the client was created */
	readonly createdAt: Date;

	/** When the client was last updated */
	readonly updatedAt: Date;
}

/**
 * Input for creating a new Client.
 */
export type NewClient = Omit<Client, 'createdAt' | 'updatedAt'> & {
	createdAt?: Date;
	updatedAt?: Date;
};

/**
 * Create a new client.
 */
export function createClient(params: {
	name: string;
	identifier: string;
}): NewClient {
	return {
		id: generate('CLIENT'),
		name: params.name,
		identifier: params.identifier.toLowerCase(),
		status: 'ACTIVE',
		statusReason: null,
		statusChangedAt: null,
		notes: [],
	};
}

/**
 * Add a note to a client's audit trail.
 */
export function addClientNote(
	client: Client,
	category: string,
	text: string,
	addedBy: string,
): Client {
	const note = createClientNote(category, text, addedBy);
	return {
		...client,
		notes: [...client.notes, note],
		updatedAt: new Date(),
	};
}

/**
 * Change client status with reason and optional note.
 */
export function changeClientStatus(
	client: Client,
	newStatus: ClientStatus,
	reason: string | null,
	changeNote: string | null,
	changedBy: string,
): Client {
	const now = new Date();
	let notes = client.notes;

	if (changeNote) {
		const note = createClientNote('STATUS_CHANGE', changeNote, changedBy);
		notes = [...notes, note];
	}

	return {
		...client,
		status: newStatus,
		statusReason: reason,
		statusChangedAt: now,
		notes,
		updatedAt: now,
	};
}
