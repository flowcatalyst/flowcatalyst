/**
 * Lightweight TSID (Time-Sorted ID) generator.
 *
 * Generates 13-character Crockford Base32 strings from a 64-bit value
 * composed of 42 bits of timestamp + 22 bits of randomness.
 */
/**
 * Generate a new TSID as a 13-character Crockford Base32 string.
 */
export declare function generate(): string;
/**
 * Validate that a string is a valid TSID format.
 */
export declare function isValid(tsid: string): boolean;
//# sourceMappingURL=tsid.d.ts.map