/**
 * @flowcatalyst/tsid
 *
 * TSID (Time-Sorted ID) generation and TypedId utilities.
 *
 * IDs follow the Stripe pattern where the prefix is stored WITH the ID:
 * - Format: "{prefix}_{tsid}" (e.g., "clt_0HZXEQ5Y8JY5Z")
 * - Total length: 17 characters (3-char prefix + underscore + 13-char TSID)
 *
 * @example
 * ```typescript
 * import { generate, generateRaw, Tsid, EntityType, validate } from '@flowcatalyst/tsid';
 *
 * // Generate a new typed ID (preferred)
 * const id = generate('CLIENT'); // "clt_0HZXEQ5Y8JY5Z"
 *
 * // Validate an ID has the correct type
 * validate('CLIENT', id); // throws if invalid
 *
 * // Generate a raw TSID (for special cases only)
 * const rawId = generateRaw(); // "0HZXEQ5Y8JY5Z"
 *
 * // Work with TSID object
 * const tsid = Tsid.from(rawId);
 * console.log(tsid.getDate()); // Creation timestamp
 * ```
 */

// TSID generation
export {
	Tsid,
	generate as generateRaw, // Renamed to make it clear this generates unprefixed IDs
	toBigInt,
	fromBigInt,
	isValid,
	getTimestamp,
} from './tsid.js';

// TypedId utilities
export {
	EntityType,
	SEPARATOR,
	type EntityTypeKey,
	type EntityTypePrefix,
	type TypedIdErrorReason,
	TypedIdError,
	// Primary API
	generate, // Generates prefixed IDs
	validate,
	validateOrNull,
	isValidTypedId,
	isValidFormat,
	extractRawId,
	extractPrefix,
	parseAny,
	getPrefix,
	getTypeFromPrefix,
	// Deprecated (for backwards compatibility)
	serialize,
	serializeAll,
	deserialize,
	deserializeAll,
	deserializeOrNull,
	stripPrefix,
	ensurePrefix,
} from './typed-id.js';
