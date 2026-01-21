/**
 * TSID (Time-Sorted ID) Generator
 *
 * Compatible with Java's tsid-creator library (com.github.f4b6a3:tsid-creator).
 *
 * TSID is a 64-bit value composed of:
 * - 42 bits for timestamp (milliseconds since custom epoch)
 * - 22 bits for random component
 *
 * Encoded as 13-character Crockford Base32 strings (e.g., "0HZXEQ5Y8JY5Z").
 *
 * Properties:
 * - Time-sortable (creation order preserved in lexicographic sort)
 * - URL-safe and case-insensitive
 * - No JavaScript number precision issues (stored as string)
 * - Shorter than numeric strings (13 vs ~19 chars)
 */

import crypto from 'node:crypto';

// Crockford Base32 alphabet (excludes I, L, O, U to avoid confusion)
const CROCKFORD_ALPHABET = '0123456789ABCDEFGHJKMNPQRSTVWXYZ';
const CROCKFORD_DECODE: Record<string, number> = {};

// Build decode map (case-insensitive)
for (let i = 0; i < CROCKFORD_ALPHABET.length; i++) {
	const char = CROCKFORD_ALPHABET[i]!;
	CROCKFORD_DECODE[char] = i;
	CROCKFORD_DECODE[char.toLowerCase()] = i;
}
// Handle common substitutions
CROCKFORD_DECODE['i'] = 1;
CROCKFORD_DECODE['I'] = 1;
CROCKFORD_DECODE['l'] = 1;
CROCKFORD_DECODE['L'] = 1;
CROCKFORD_DECODE['o'] = 0;
CROCKFORD_DECODE['O'] = 0;

// TSID epoch: 2020-01-01T00:00:00Z (same as tsid-creator default)
const TSID_EPOCH = 1577836800000n;

// Bit positions
const RANDOM_BITS = 22n;
const RANDOM_MASK = (1n << RANDOM_BITS) - 1n;

// State for monotonic counter (prevents collisions within same millisecond)
let lastTimestamp = 0n;
let counter = 0n;

/**
 * Generate a cryptographically random 22-bit value
 */
function randomBits(): bigint {
	const bytes = crypto.randomBytes(3);
	const value = (BigInt(bytes[0]!) << 16n) | (BigInt(bytes[1]!) << 8n) | BigInt(bytes[2]!);
	return value & RANDOM_MASK;
}

/**
 * Generate a new TSID value as a BigInt
 */
function generateTsidBigInt(): bigint {
	const now = BigInt(Date.now());
	const timestamp = now - TSID_EPOCH;

	if (timestamp === lastTimestamp) {
		// Same millisecond: increment counter
		counter = (counter + 1n) & RANDOM_MASK;
		if (counter === 0n) {
			// Counter overflow: wait for next millisecond
			let nextTimestamp = BigInt(Date.now()) - TSID_EPOCH;
			while (nextTimestamp === lastTimestamp) {
				nextTimestamp = BigInt(Date.now()) - TSID_EPOCH;
			}
			lastTimestamp = nextTimestamp;
			counter = randomBits();
			return (lastTimestamp << RANDOM_BITS) | counter;
		}
	} else {
		// New millisecond: reset counter with random value
		lastTimestamp = timestamp;
		counter = randomBits();
	}

	return (timestamp << RANDOM_BITS) | counter;
}

/**
 * Encode a BigInt as a 13-character Crockford Base32 string
 */
function encodeCrockford(value: bigint): string {
	const chars: string[] = new Array(13);

	for (let i = 12; i >= 0; i--) {
		chars[i] = CROCKFORD_ALPHABET[Number(value & 31n)]!;
		value >>= 5n;
	}

	return chars.join('');
}

/**
 * Decode a 13-character Crockford Base32 string to BigInt
 */
function decodeCrockford(str: string): bigint {
	if (str.length !== 13) {
		throw new Error(`Invalid TSID length: expected 13, got ${str.length}`);
	}

	let value = 0n;

	for (let i = 0; i < 13; i++) {
		const char = str[i]!;
		const digit = CROCKFORD_DECODE[char];
		if (digit === undefined) {
			throw new Error(`Invalid Crockford Base32 character: ${char}`);
		}
		value = (value << 5n) | BigInt(digit);
	}

	return value;
}

/**
 * TSID class for working with Time-Sorted IDs
 */
export class Tsid {
	private readonly value: bigint;

	private constructor(value: bigint) {
		this.value = value;
	}

	/**
	 * Create a new TSID
	 */
	static create(): Tsid {
		return new Tsid(generateTsidBigInt());
	}

	/**
	 * Parse a TSID from a Crockford Base32 string
	 */
	static from(str: string): Tsid {
		return new Tsid(decodeCrockford(str));
	}

	/**
	 * Create a TSID from a BigInt value
	 */
	static fromBigInt(value: bigint): Tsid {
		return new Tsid(value);
	}

	/**
	 * Get the TSID as a 13-character Crockford Base32 string
	 */
	toString(): string {
		return encodeCrockford(this.value);
	}

	/**
	 * Get the TSID as a BigInt
	 */
	toBigInt(): bigint {
		return this.value;
	}

	/**
	 * Get the timestamp component (milliseconds since TSID epoch)
	 */
	getTimestamp(): bigint {
		return this.value >> RANDOM_BITS;
	}

	/**
	 * Get the creation time as a Date
	 */
	getDate(): Date {
		return new Date(Number(this.getTimestamp() + TSID_EPOCH));
	}

	/**
	 * Get the random component
	 */
	getRandom(): bigint {
		return this.value & RANDOM_MASK;
	}
}

/**
 * Generate a new TSID as a Crockford Base32 string.
 * This is the primary function for generating IDs.
 */
export function generate(): string {
	return Tsid.create().toString();
}

/**
 * Convert a TSID string to BigInt.
 * Useful for database queries on legacy numeric fields.
 */
export function toBigInt(tsidString: string): bigint {
	return Tsid.from(tsidString).toBigInt();
}

/**
 * Convert a BigInt to TSID string.
 * Useful for migrating existing numeric IDs to string format.
 */
export function fromBigInt(value: bigint): string {
	return Tsid.fromBigInt(value).toString();
}

/**
 * Validate that a string is a valid TSID format
 */
export function isValid(str: string): boolean {
	if (str.length !== 13) {
		return false;
	}

	const upper = str.toUpperCase();
	for (const char of upper) {
		if (!CROCKFORD_ALPHABET.includes(char)) {
			return false;
		}
	}

	return true;
}

/**
 * Extract the creation timestamp from a TSID string
 */
export function getTimestamp(tsidString: string): Date {
	return Tsid.from(tsidString).getDate();
}
