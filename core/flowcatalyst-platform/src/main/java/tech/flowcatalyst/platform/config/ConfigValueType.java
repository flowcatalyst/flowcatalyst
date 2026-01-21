package tech.flowcatalyst.platform.config;

/**
 * Defines how a configuration value is stored.
 */
public enum ConfigValueType {
    /**
     * Value is stored as-is in the database.
     */
    PLAIN,

    /**
     * Value is a secret reference, resolved via SecretService.
     */
    SECRET
}
