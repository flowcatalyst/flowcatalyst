package tech.flowcatalyst.sdk.enums;

/**
 * Status of a schema spec version.
 */
public enum SpecVersionStatus {
    /** Schema is being finalized, can still be modified */
    FINALISING,

    /** Schema is finalized and in use */
    CURRENT,

    /** Schema is deprecated and should not be used for new events */
    DEPRECATED
}
