package tech.flowcatalyst.platform.config;

import java.time.Instant;

/**
 * Domain model for platform configuration entries.
 *
 * Configurations are organized hierarchically:
 * - applicationCode: References Application.code (e.g., "platform", "tms", "wms")
 * - section: Logical grouping (e.g., "login", "ui", "api")
 * - property: Specific setting (e.g., "theme", "colors", "logo")
 */
public class PlatformConfig {

    public String id;
    public String applicationCode;
    public String section;
    public String property;
    public ConfigScope scope;
    public String clientId;
    public ConfigValueType valueType;
    public String value;
    public String description;
    public Instant createdAt;
    public Instant updatedAt;

    /**
     * Returns the full key in dot notation: applicationCode.section.property
     */
    public String getFullKey() {
        return applicationCode + "." + section + "." + property;
    }
}
