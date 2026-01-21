/**
 * Client Domain
 *
 * Exports for the client entity and related types.
 */

export { ClientStatus } from './client-status.js';
export { type ClientNote, createClientNote } from './client-note.js';
export {
	type Client,
	type NewClient,
	createClient,
	addClientNote,
	changeClientStatus,
} from './client.js';
export {
	type ClientCreatedData,
	ClientCreated,
	type ClientUpdatedData,
	ClientUpdated,
	type ClientStatusChangedData,
	ClientStatusChanged,
	type ClientDeletedData,
	ClientDeleted,
	type ClientNoteAddedData,
	ClientNoteAdded,
} from './events.js';
