package tech.flowcatalyst.platform.shared;

import com.github.f4b6a3.tsid.Tsid;
import com.github.f4b6a3.tsid.TsidCreator;

import java.util.Objects;

/**
 * Centralized TSID generation for all entities.
 * TSID (Time-Sorted ID) provides:
 * - Time-sortable (creation order preserved)
 * - 64-bit efficiency (vs 128-bit UUID)
 * - Sequential-ish (better for indexing than random UUIDs)
 * - Monotonic (no collisions in distributed systems)
 *
 * IDs are stored and transmitted as typed IDs with 3-character prefixes.
 * Format: "{prefix}_{tsid}" (e.g., "clt_0HZXEQ5Y8JY5Z")
 * Total length: 17 characters (3-char prefix + underscore + 13-char TSID)
 *
 * This format is:
 * - Self-documenting (prefix indicates entity type)
 * - URL-safe and case-insensitive
 * - Lexicographically sortable within type
 * - Safe from JavaScript number precision issues
 * - No conversion needed between API and database
 */
public class TsidGenerator {

    /**
     * Separator between prefix and TSID.
     */
    public static final String SEPARATOR = "_";

    /**
     * Generate a new ID for the given entity type.
     * High-volume entity types (events, dispatch jobs) return raw TSIDs without prefix.
     * Other entity types return typed IDs with prefix (e.g., "clt_0HZXEQ5Y8JY5Z").
     *
     * @param type the entity type
     * @return the ID (with or without prefix depending on entity type)
     */
    public static String generate(EntityType type) {
        Objects.requireNonNull(type, "EntityType must not be null");
        String tsid = TsidCreator.getTsid().toString();
        return type.usePrefix() ? type.prefix() + SEPARATOR + tsid : tsid;
    }

    /**
     * Generate a raw TSID without prefix.
     * Use this for non-entity IDs (execution IDs, trace IDs, etc.)
     * or during migration period.
     *
     * @return the raw TSID (e.g., "0HZXEQ5Y8JY5Z")
     */
    public static String generateRaw() {
        return TsidCreator.getTsid().toString();
    }

    /**
     * Generate a raw TSID without prefix.
     *
     * @return the raw TSID (e.g., "0HZXEQ5Y8JY5Z")
     * @deprecated Use {@link #generate(EntityType)} for entity IDs to get prefixed IDs,
     *             or {@link #generateRaw()} explicitly for non-entity IDs.
     */
    @Deprecated
    public static String generate() {
        return generateRaw();
    }

    /**
     * Extract the raw TSID portion from a typed ID.
     *
     * @param typedId the typed ID (e.g., "clt_0HZXEQ5Y8JY5Z")
     * @return the raw TSID (e.g., "0HZXEQ5Y8JY5Z")
     * @throws IllegalArgumentException if the ID format is invalid
     */
    public static String extractRawId(String typedId) {
        if (typedId == null || typedId.isBlank()) {
            throw new IllegalArgumentException("Typed ID cannot be null or blank");
        }
        int separatorIndex = typedId.indexOf(SEPARATOR);
        if (separatorIndex == -1) {
            throw new IllegalArgumentException("Invalid typed ID format: missing separator");
        }
        return typedId.substring(separatorIndex + 1);
    }

    /**
     * Extract the prefix from a typed ID.
     *
     * @param typedId the typed ID (e.g., "clt_0HZXEQ5Y8JY5Z")
     * @return the prefix (e.g., "clt")
     * @throws IllegalArgumentException if the ID format is invalid
     */
    public static String extractPrefix(String typedId) {
        if (typedId == null || typedId.isBlank()) {
            throw new IllegalArgumentException("Typed ID cannot be null or blank");
        }
        int separatorIndex = typedId.indexOf(SEPARATOR);
        if (separatorIndex == -1) {
            throw new IllegalArgumentException("Invalid typed ID format: missing separator");
        }
        return typedId.substring(0, separatorIndex);
    }

    /**
     * Convert a TSID string to Long.
     * Works with both raw TSIDs and typed IDs (extracts TSID portion).
     */
    public static Long toLong(String tsidString) {
        String rawId = tsidString.contains(SEPARATOR) ? extractRawId(tsidString) : tsidString;
        return Tsid.from(rawId).toLong();
    }

    /**
     * Convert a Long to raw TSID string.
     * Useful for migrating existing Long IDs.
     */
    public static String toString(Long tsidLong) {
        return Tsid.from(tsidLong).toString();
    }

    /**
     * Convert a Long to typed ID string.
     *
     * @param type the entity type
     * @param tsidLong the TSID as Long
     * @return the typed ID (e.g., "clt_0HZXEQ5Y8JY5Z")
     */
    public static String toTypedString(EntityType type, Long tsidLong) {
        Objects.requireNonNull(type, "EntityType must not be null");
        return type.prefix() + SEPARATOR + Tsid.from(tsidLong).toString();
    }

    private TsidGenerator() {
        // Utility class
    }
}
