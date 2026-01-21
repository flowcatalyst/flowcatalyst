package tech.flowcatalyst.platform.application.operations.updateapplication;

/**
 * Command to update an Application.
 *
 * @param applicationId  The ID of the application to update
 * @param name           New name (null to keep existing)
 * @param description    New description (null to keep existing)
 * @param defaultBaseUrl New base URL (null to keep existing)
 * @param iconUrl        New icon URL (null to keep existing)
 * @param website        New website URL (null to keep existing)
 * @param logo           New logo content (null to keep existing)
 * @param logoMimeType   New logo MIME type (null to keep existing)
 */
public record UpdateApplicationCommand(
    String applicationId,
    String name,
    String description,
    String defaultBaseUrl,
    String iconUrl,
    String website,
    String logo,
    String logoMimeType
) {
    /**
     * Constructor without website/logo for backwards compatibility.
     */
    public UpdateApplicationCommand(String applicationId, String name, String description,
                                    String defaultBaseUrl, String iconUrl) {
        this(applicationId, name, description, defaultBaseUrl, iconUrl, null, null, null);
    }
}
