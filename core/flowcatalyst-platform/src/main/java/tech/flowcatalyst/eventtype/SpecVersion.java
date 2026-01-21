package tech.flowcatalyst.eventtype;

/**
 * A versioned schema specification for an EventType.
 *
 * Stored as embedded document within the EventType.
 *
 * Version format: {MAJOR}.{MINOR}
 * - Minor versions (1.0 → 1.1) are backwards compatible
 * - Major versions (1.x → 2.0) are breaking changes
 */
public record SpecVersion(
    /**
     * Version string in format "MAJOR.MINOR" (e.g., "1.0", "1.1", "2.0")
     */
    String version,

    /**
     * MIME type for the event payload (e.g., "application/json", "application/protobuf")
     */
    String mimeType,

    /**
     * The schema definition content (JSON Schema, Proto, or XSD)
     */
    String schema,

    /**
     * The type of schema (JSON_SCHEMA, PROTO, XSD)
     */
    SchemaType schemaType,

    /**
     * Current status of this spec version (FINALISING, CURRENT, DEPRECATED)
     */
    SpecVersionStatus status
) {

    /**
     * Extract the major version number from the version string.
     * e.g., "1.2" → 1
     */
    public int majorVersion() {
        if (version == null || !version.contains(".")) {
            throw new IllegalStateException("Invalid version format: " + version);
        }
        return Integer.parseInt(version.split("\\.")[0]);
    }

    /**
     * Extract the minor version number from the version string.
     * e.g., "1.2" → 2
     */
    public int minorVersion() {
        if (version == null || !version.contains(".")) {
            throw new IllegalStateException("Invalid version format: " + version);
        }
        return Integer.parseInt(version.split("\\.")[1]);
    }

    /**
     * Create a copy with a new status.
     */
    public SpecVersion withStatus(SpecVersionStatus newStatus) {
        return new SpecVersion(version, mimeType, schema, schemaType, newStatus);
    }
}
