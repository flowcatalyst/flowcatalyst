/**
 * TypedId - Prefixed ID Validation and Utilities
 *
 * Following the Stripe pattern, IDs are stored WITH the prefix in the database:
 * - Format: "{prefix}_{tsid}" (e.g., "clt_0HZXEQ5Y8JY5Z")
 * - Total length: 17 characters (3-char prefix + underscore + 13-char TSID)
 *
 * This provides:
 * - Self-documenting IDs (immediately know the entity type)
 * - Type safety (can't accidentally pass wrong ID type)
 * - Easier debugging and support
 * - No serialization/deserialization overhead
 * - Consistent format across API, database, and logs
 *
 * Example: "clt_0HZXEQ5Y8JY5Z" is stored in the database as-is
 */

import { isValid, generate as generateRawTsid } from './tsid.js';

/**
 * Entity types with their 3-character ID prefixes.
 * Matches Java EntityType enum exactly.
 */
export const EntityType = {
	// Core entities
	CLIENT: 'clt',
	PRINCIPAL: 'prn',
	APPLICATION: 'app',
	SERVICE_ACCOUNT: 'sac',

	// Authorization
	ROLE: 'rol',
	PERMISSION: 'prm',

	// Authentication
	OAUTH_CLIENT: 'oac',
	AUTH_CODE: 'acd',

	// Configuration
	CLIENT_AUTH_CONFIG: 'cac',
	APP_CLIENT_CONFIG: 'apc',
	IDP_ROLE_MAPPING: 'irm',
	CORS_ORIGIN: 'cor',
	ANCHOR_DOMAIN: 'anc',

	// Access management
	CLIENT_ACCESS_GRANT: 'gnt',

	// Events & Messaging
	EVENT_TYPE: 'evt',
	EVENT: 'evn',
	EVENT_READ: 'evr',
	SUBSCRIPTION: 'sub',
	DISPATCH_POOL: 'dpl',
	DISPATCH_JOB: 'djb',
	DISPATCH_JOB_READ: 'djr',
	SCHEMA: 'sch',

	// Audit
	AUDIT_LOG: 'aud',
} as const;

/**
 * Separator between prefix and TSID
 */
export const SEPARATOR = '_';

export type EntityTypeKey = keyof typeof EntityType;
export type EntityTypePrefix = (typeof EntityType)[EntityTypeKey];

// Build reverse lookup map
const PREFIX_TO_TYPE: Record<string, EntityTypeKey> = {};
for (const [key, prefix] of Object.entries(EntityType)) {
	PREFIX_TO_TYPE[prefix] = key as EntityTypeKey;
}

/**
 * Pattern for valid typed IDs (3-char prefix + underscore + 13-char TSID).
 * Example: "clt_0HZXEQ5Y8JY5Z"
 */
const TYPED_ID_PATTERN = /^[a-z]{3}_[0-9A-HJKMNP-TV-Z]{13}$/i;

/**
 * Generate a new typed ID with the specified entity type prefix.
 * This is the primary function for generating new IDs.
 *
 * @param type - The entity type
 * @returns A typed ID (e.g., "clt_0HZXEQ5Y8JY5Z")
 */
export function generate(type: EntityTypeKey): string {
	const prefix = EntityType[type];
	return `${prefix}${SEPARATOR}${generateRawTsid()}`;
}

/**
 * Reason codes for TypedId errors.
 */
export type TypedIdErrorReason =
	| 'empty' // null/blank input
	| 'missing_separator' // no underscore
	| 'unknown_prefix' // unrecognized prefix
	| 'type_mismatch' // wrong entity type
	| 'invalid_tsid'; // TSID format invalid

/**
 * Error thrown when ID serialization/deserialization fails
 */
export class TypedIdError extends Error {
	constructor(
		message: string,
		public readonly reason: TypedIdErrorReason,
		public readonly expectedType?: EntityTypeKey,
		public readonly actualType?: EntityTypeKey,
		public readonly id?: string,
	) {
		super(message);
		this.name = 'TypedIdError';
	}
}

/**
 * Validate that an ID has the correct format and entity type.
 *
 * @param type - The expected entity type
 * @param id - The typed ID (e.g., "clt_0HZXEQ5Y8JY5Z")
 * @throws TypedIdError if the ID is malformed or wrong type
 */
export function validate(type: EntityTypeKey, id: string): void {
	if (!id || id.trim() === '') {
		throw new TypedIdError('ID cannot be null or blank', 'empty', type, undefined, id);
	}

	const separatorIndex = id.indexOf(SEPARATOR);
	if (separatorIndex === -1) {
		throw new TypedIdError(
			`Invalid ID format: expected '${EntityType[type]}_<id>' but got '${id}'`,
			'missing_separator',
			type,
			undefined,
			id,
		);
	}

	const prefix = id.slice(0, separatorIndex);
	const rawId = id.slice(separatorIndex + 1);

	const actualType = PREFIX_TO_TYPE[prefix];
	if (!actualType) {
		throw new TypedIdError(`Unknown ID prefix '${prefix}'`, 'unknown_prefix', type, undefined, id);
	}

	if (actualType !== type) {
		throw new TypedIdError(
			`ID type mismatch. Expected '${EntityType[type]}' but got '${prefix}'`,
			'type_mismatch',
			type,
			actualType,
			id,
		);
	}

	if (!isValid(rawId)) {
		throw new TypedIdError(`Invalid TSID format in ID '${id}'`, 'invalid_tsid', type, undefined, id);
	}
}

/**
 * Validate an ID if present, returning null for null/blank inputs.
 *
 * @param type - The expected entity type
 * @param id - The typed ID, may be null/undefined
 * @returns The ID if valid, or null if input is null/blank
 * @throws TypedIdError if the ID is present but invalid
 */
export function validateOrNull(type: EntityTypeKey, id: string | null | undefined): string | null {
	if (id === null || id === undefined || id.trim() === '') {
		return null;
	}
	validate(type, id);
	return id;
}

/**
 * @deprecated IDs are now stored with prefixes. Use generate() for new IDs.
 *
 * Serialize an internal TSID to external prefixed format.
 * Kept for backwards compatibility during migration.
 */
export function serialize(type: EntityTypeKey, id: string): string;
export function serialize(type: EntityTypeKey, id: null | undefined): null;
export function serialize(type: EntityTypeKey, id: string | null | undefined): string | null;
export function serialize(type: EntityTypeKey, id: string | null | undefined): string | null {
	if (id === null || id === undefined) {
		return null;
	}
	// If ID already has a prefix, return as-is
	if (id.includes(SEPARATOR)) {
		return id;
	}
	const prefix = EntityType[type];
	return `${prefix}${SEPARATOR}${id}`;
}

/**
 * @deprecated IDs are now stored with prefixes. Use validate() to verify IDs.
 *
 * Deserialize an external prefixed ID to internal TSID.
 * Kept for backwards compatibility during migration.
 */
export function deserialize(type: EntityTypeKey, externalId: string): string {
	validate(type, externalId);
	// Extract raw ID (though in new code, the prefixed ID is the canonical form)
	const separatorIndex = externalId.indexOf(SEPARATOR);
	return externalId.slice(separatorIndex + 1);
}

/**
 * Deserialize an external prefixed ID, returning null if invalid.
 *
 * @param type - The expected entity type
 * @param externalId - The external prefixed ID
 * @returns The internal TSID string, or null if invalid
 */
export function deserializeOrNull(type: EntityTypeKey, externalId: string | null | undefined): string | null {
	if (externalId === null || externalId === undefined) {
		return null;
	}

	try {
		return deserialize(type, externalId);
	} catch {
		return null;
	}
}

/**
 * Check if a typed ID is valid for the given type.
 *
 * @param type - The expected entity type
 * @param id - The typed ID
 * @returns true if valid, false otherwise
 */
export function isValidTypedId(type: EntityTypeKey, id: string): boolean {
	try {
		validate(type, id);
		return true;
	} catch {
		return false;
	}
}

/**
 * Check if a string is a valid typed ID format (any type).
 *
 * @param id - The potential typed ID
 * @returns true if valid format, false otherwise
 */
export function isValidFormat(id: string): boolean {
	return TYPED_ID_PATTERN.test(id);
}

/**
 * Extract the raw TSID portion from a typed ID.
 *
 * @param id - The typed ID (e.g., "clt_0HZXEQ5Y8JY5Z")
 * @returns The raw TSID (e.g., "0HZXEQ5Y8JY5Z")
 * @throws Error if the ID format is invalid
 */
export function extractRawId(id: string): string {
	if (!id || id.trim() === '') {
		throw new Error('ID cannot be null or blank');
	}
	const separatorIndex = id.indexOf(SEPARATOR);
	if (separatorIndex === -1) {
		throw new Error('Invalid typed ID format: missing separator');
	}
	return id.slice(separatorIndex + 1);
}

/**
 * Extract the prefix from a typed ID.
 *
 * @param id - The typed ID (e.g., "clt_0HZXEQ5Y8JY5Z")
 * @returns The prefix (e.g., "clt")
 * @throws Error if the ID format is invalid
 */
export function extractPrefix(id: string): string {
	if (!id || id.trim() === '') {
		throw new Error('ID cannot be null or blank');
	}
	const separatorIndex = id.indexOf(SEPARATOR);
	if (separatorIndex === -1) {
		throw new Error('Invalid typed ID format: missing separator');
	}
	return id.slice(0, separatorIndex);
}

/**
 * Parse any prefixed ID without type validation.
 *
 * @param externalId - The external prefixed ID
 * @returns Object with parsed type and internal ID
 */
export function parseAny(externalId: string): { type: EntityTypeKey | null; id: string } {
	const separatorIndex = externalId.indexOf('_');

	if (separatorIndex === -1) {
		// No prefix, return as-is (might be an internal ID)
		return { type: null, id: externalId };
	}

	const prefix = externalId.slice(0, separatorIndex);
	const id = externalId.slice(separatorIndex + 1);

	const type = PREFIX_TO_TYPE[prefix] ?? null;

	return { type, id };
}

/**
 * Get the prefix for an entity type.
 *
 * @param type - The entity type
 * @returns The prefix string
 */
export function getPrefix(type: EntityTypeKey): EntityTypePrefix {
	return EntityType[type];
}

/**
 * Get the entity type from a prefix.
 *
 * @param prefix - The prefix string
 * @returns The entity type, or null if not found
 */
export function getTypeFromPrefix(prefix: string): EntityTypeKey | null {
	return PREFIX_TO_TYPE[prefix] ?? null;
}

/**
 * Strip the prefix from an ID if present, regardless of type.
 * Useful for accepting both prefixed and unprefixed IDs.
 *
 * @param externalId - The ID (may or may not have prefix)
 * @returns The internal TSID string
 */
export function stripPrefix(externalId: string): string {
	const parsed = parseAny(externalId);
	return parsed.id;
}

/**
 * Ensure an ID has the correct prefix, adding it if missing.
 *
 * @param type - The entity type
 * @param id - The ID (may or may not have prefix)
 * @returns The external prefixed ID
 */
export function ensurePrefix(type: EntityTypeKey, id: string): string {
	const parsed = parseAny(id);

	if (parsed.type === type) {
		// Already has correct prefix
		return id;
	}

	if (parsed.type !== null) {
		// Has wrong prefix - error
		throw new TypedIdError(
			`Cannot add ${type} prefix to ID with ${parsed.type} prefix`,
			'type_mismatch',
			type,
			parsed.type,
			id,
		);
	}

	// No prefix - add it
	return serialize(type, parsed.id)!;
}

/**
 * Serialize multiple internal TSIDs to external prefixed format.
 *
 * @param type - The entity type
 * @param ids - Array of internal TSID strings
 * @returns Array of external prefixed IDs
 */
export function serializeAll(type: EntityTypeKey, ids: string[]): string[] {
	return ids.map((id) => serialize(type, id)!);
}

/**
 * Deserialize multiple external prefixed IDs to internal TSIDs.
 *
 * @param type - The expected entity type
 * @param externalIds - Array of external prefixed IDs
 * @returns Array of internal TSID strings
 * @throws TypedIdError if any ID format is invalid or type doesn't match
 */
export function deserializeAll(type: EntityTypeKey, externalIds: string[]): string[] {
	return externalIds.map((externalId) => deserialize(type, externalId));
}
