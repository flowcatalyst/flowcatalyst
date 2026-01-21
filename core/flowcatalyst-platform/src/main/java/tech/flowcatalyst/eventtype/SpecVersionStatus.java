package tech.flowcatalyst.eventtype;

/**
 * Status of a schema spec version.
 */
public enum SpecVersionStatus {
    /**
     * Schema is being finalized - not yet ready for production use.
     * Can be modified or deleted.
     */
    FINALISING,

    /**
     * Schema is the current active version for its major version line.
     * Only one schema per major version can be CURRENT.
     */
    CURRENT,

    /**
     * Schema has been superseded by a newer version.
     * Still valid for reading existing events but new events should use CURRENT.
     */
    DEPRECATED
}
