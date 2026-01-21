package tech.flowcatalyst.platform.application.operations.createapplication;

import tech.flowcatalyst.platform.application.Application;

/**
 * Command to create a new Application.
 *
 * @param code           Unique application code (used in role prefixes, e.g., "tms", "wms")
 * @param name           Display name (e.g., "Transport Management System")
 * @param description    Optional description
 * @param defaultBaseUrl Optional default URL for the application
 * @param iconUrl        Optional icon URL
 * @param website        Optional public website URL
 * @param logo           Optional logo content (SVG/vector format)
 * @param logoMimeType   Optional MIME type of the logo (e.g., "image/svg+xml")
 * @param type           Application type (APPLICATION or INTEGRATION), defaults to APPLICATION
 * @param provisionServiceAccount Whether to create a service account for this application
 */
public record CreateApplicationCommand(
    String code,
    String name,
    String description,
    String defaultBaseUrl,
    String iconUrl,
    String website,
    String logo,
    String logoMimeType,
    Application.ApplicationType type,
    boolean provisionServiceAccount
) {
    /**
     * Constructor with defaults for backwards compatibility.
     */
    public CreateApplicationCommand(String code, String name, String description, String defaultBaseUrl, String iconUrl) {
        this(code, name, description, defaultBaseUrl, iconUrl, null, null, null, Application.ApplicationType.APPLICATION, true);
    }

    /**
     * Constructor with type but without website/logo for backwards compatibility.
     */
    public CreateApplicationCommand(String code, String name, String description, String defaultBaseUrl, String iconUrl,
                                    Application.ApplicationType type, boolean provisionServiceAccount) {
        this(code, name, description, defaultBaseUrl, iconUrl, null, null, null, type, provisionServiceAccount);
    }
}
