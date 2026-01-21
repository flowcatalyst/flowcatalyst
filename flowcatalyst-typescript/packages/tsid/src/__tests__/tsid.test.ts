import { describe, it, expect } from 'vitest';
import { generate, Tsid, isValid, toBigInt, fromBigInt, getTimestamp } from '../tsid.js';

describe('TSID Generation', () => {
	it('should generate a 13-character string', () => {
		const id = generate();
		expect(id).toHaveLength(13);
	});

	it('should generate valid Crockford Base32 characters', () => {
		const id = generate();
		// Crockford Base32 excludes I, L, O, U
		expect(id).toMatch(/^[0-9A-HJKMNP-TV-Z]+$/i);
	});

	it('should generate unique IDs', () => {
		const ids = new Set<string>();
		for (let i = 0; i < 1000; i++) {
			ids.add(generate());
		}
		expect(ids.size).toBe(1000);
	});

	it('should generate time-sortable IDs', () => {
		const ids: string[] = [];
		for (let i = 0; i < 100; i++) {
			ids.push(generate());
		}

		// IDs should already be in sorted order
		const sorted = [...ids].sort();
		expect(ids).toEqual(sorted);
	});

	it('should handle rapid generation without collision', () => {
		const ids = new Set<string>();
		for (let i = 0; i < 10000; i++) {
			ids.add(generate());
		}
		expect(ids.size).toBe(10000);
	});
});

describe('Tsid class', () => {
	it('should parse and round-trip a TSID string', () => {
		const original = generate();
		const tsid = Tsid.from(original);
		expect(tsid.toString()).toBe(original);
	});

	it('should extract timestamp', () => {
		const tsid = Tsid.create();
		const date = tsid.getDate();
		const now = Date.now();

		// Timestamp should be within 1 second of now
		expect(Math.abs(date.getTime() - now)).toBeLessThan(1000);
	});

	it('should convert to and from BigInt', () => {
		const original = generate();
		const bigint = toBigInt(original);
		const restored = fromBigInt(bigint);
		expect(restored).toBe(original);
	});

	it('should handle BigInt operations correctly', () => {
		const tsid = Tsid.create();
		const bigint = tsid.toBigInt();

		// BigInt should be positive
		expect(bigint).toBeGreaterThan(0n);

		// Should be 64-bit (less than 2^64)
		expect(bigint).toBeLessThan(2n ** 64n);
	});
});

describe('isValid', () => {
	it('should validate correct TSID format', () => {
		const id = generate();
		expect(isValid(id)).toBe(true);
	});

	it('should reject strings that are too short', () => {
		expect(isValid('0HZXEQ5Y8JY')).toBe(false);
	});

	it('should reject strings that are too long', () => {
		expect(isValid('0HZXEQ5Y8JY5ZZ')).toBe(false);
	});

	it('should reject strings with invalid characters', () => {
		expect(isValid('0HZXEQ5Y8JY5I')).toBe(false); // I is not valid
		expect(isValid('0HZXEQ5Y8JY5L')).toBe(false); // L is not valid
		expect(isValid('0HZXEQ5Y8JY5O')).toBe(false); // O is not valid
		expect(isValid('0HZXEQ5Y8JY5U')).toBe(false); // U is not valid
	});

	it('should accept case-insensitive input for validation', () => {
		const id = generate();
		expect(isValid(id.toLowerCase())).toBe(true);
	});
});

describe('getTimestamp', () => {
	it('should extract creation time from TSID', () => {
		const id = generate();
		const timestamp = getTimestamp(id);
		const now = Date.now();

		expect(Math.abs(timestamp.getTime() - now)).toBeLessThan(1000);
	});

	it('should preserve timestamp through serialization', () => {
		const tsid = Tsid.create();
		const date1 = tsid.getDate();

		const str = tsid.toString();
		const restored = Tsid.from(str);
		const date2 = restored.getDate();

		expect(date1.getTime()).toBe(date2.getTime());
	});
});

describe('Crockford Base32 decoding', () => {
	it('should handle lowercase input', () => {
		const upper = generate();
		const lower = upper.toLowerCase();
		const tsid = Tsid.from(lower);
		expect(tsid.toString()).toBe(upper);
	});

	it('should reject invalid length', () => {
		expect(() => Tsid.from('0HZXEQ5Y8JY')).toThrow('Invalid TSID length');
	});

	it('should reject invalid characters', () => {
		expect(() => Tsid.from('0HZXEQ5Y8JY5!')).toThrow('Invalid Crockford Base32 character');
	});
});
