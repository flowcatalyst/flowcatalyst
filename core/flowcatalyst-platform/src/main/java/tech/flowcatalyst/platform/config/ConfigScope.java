package tech.flowcatalyst.platform.config;

/**
 * Defines the scope of a platform configuration value.
 */
public enum ConfigScope {
    /**
     * Platform-wide configuration (clientId is null).
     */
    GLOBAL,

    /**
     * Client-specific configuration override (clientId is required).
     */
    CLIENT
}
