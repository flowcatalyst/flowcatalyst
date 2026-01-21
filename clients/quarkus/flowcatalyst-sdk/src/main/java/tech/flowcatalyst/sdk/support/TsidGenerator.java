package tech.flowcatalyst.sdk.support;

import com.github.f4b6a3.tsid.Tsid;
import com.github.f4b6a3.tsid.TsidCreator;

/**
 * Utility for generating Time-Sorted IDs (TSIDs).
 *
 * <p>TSIDs are 13-character Crockford Base32 strings that are:
 * <ul>
 *   <li>Lexicographically sortable (newer IDs sort after older ones)</li>
 *   <li>URL-safe and case-insensitive</li>
 *   <li>Shorter than numeric strings (13 vs ~19 chars)</li>
 *   <li>Safe from JavaScript number precision issues</li>
 * </ul>
 */
public final class TsidGenerator {

    private TsidGenerator() {}

    /**
     * Generate a new TSID string.
     *
     * @return A 13-character Crockford Base32 string (e.g., "0HZXEQ5Y8JY5Z")
     */
    public static String generate() {
        return TsidCreator.getTsid().toString();
    }

    /**
     * Convert a TSID string to its numeric representation.
     *
     * @param tsid The TSID string
     * @return The numeric value
     */
    public static long toLong(String tsid) {
        return Tsid.from(tsid).toLong();
    }

    /**
     * Convert a numeric TSID to its string representation.
     *
     * @param value The numeric value
     * @return The TSID string
     */
    public static String toString(long value) {
        return Tsid.from(value).toString();
    }
}
