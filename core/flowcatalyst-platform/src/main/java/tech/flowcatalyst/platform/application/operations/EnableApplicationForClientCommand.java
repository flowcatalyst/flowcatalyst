package tech.flowcatalyst.platform.application.operations;

import java.util.Map;

/**
 * Command to enable an application for a client.
 *
 * @param applicationId The application to enable
 * @param clientId The client to enable it for
 * @param baseUrlOverride Optional URL override for this client
 * @param websiteOverride Optional website URL override for this client
 * @param configJson Optional custom configuration JSON
 */
public record EnableApplicationForClientCommand(
    String applicationId,
    String clientId,
    String baseUrlOverride,
    String websiteOverride,
    Map<String, Object> configJson
) {
    /**
     * Constructor without websiteOverride/configJson for backwards compatibility.
     */
    public EnableApplicationForClientCommand(String applicationId, String clientId, String baseUrlOverride) {
        this(applicationId, clientId, baseUrlOverride, null, null);
    }

    /**
     * Constructor without websiteOverride for backwards compatibility.
     */
    public EnableApplicationForClientCommand(String applicationId, String clientId, String baseUrlOverride,
                                              Map<String, Object> configJson) {
        this(applicationId, clientId, baseUrlOverride, null, configJson);
    }
}
