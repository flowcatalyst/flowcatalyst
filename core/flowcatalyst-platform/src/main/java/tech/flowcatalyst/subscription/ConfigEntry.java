package tech.flowcatalyst.subscription;

/**
 * A key-value configuration entry for subscription custom config.
 *
 * @param key Configuration key
 * @param value Configuration value
 */
public record ConfigEntry(
    String key,
    String value
) {}
