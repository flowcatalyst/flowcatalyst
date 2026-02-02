package tech.flowcatalyst.platform.common.api;

import org.eclipse.microprofile.openapi.annotations.media.Schema;
import tech.flowcatalyst.platform.application.Application;
import tech.flowcatalyst.platform.application.ApplicationClientConfig;
import tech.flowcatalyst.platform.client.Client;
import tech.flowcatalyst.platform.shared.EntityType;
import tech.flowcatalyst.platform.shared.TypedId;

import java.time.Instant;
import java.util.List;
import java.util.Map;

/**
 * Response DTOs for Application Admin API endpoints.
 */
public final class ApplicationResponses {

    private ApplicationResponses() {}

    // ========================================================================
    // Application DTOs
    // ========================================================================

    @Schema(description = "Application summary for list views")
    public record ApplicationListItem(
        @Schema(description = "Application ID", example = "app_0ABC123DEF456")
        String id,
        @Schema(description = "Application type", example = "APPLICATION")
        String type,
        @Schema(description = "Unique application code", example = "my-app")
        String code,
        @Schema(description = "Display name", example = "My Application")
        String name,
        @Schema(description = "Description")
        String description,
        @Schema(description = "Default base URL")
        String defaultBaseUrl,
        @Schema(description = "Icon URL")
        String iconUrl,
        @Schema(description = "Website URL")
        String website,
        @Schema(description = "Logo MIME type")
        String logoMimeType,
        @Schema(description = "Service account OAuth client ID")
        String serviceAccountId,
        @Schema(description = "Service account principal ID")
        String serviceAccountPrincipalId,
        @Schema(description = "Whether application is active")
        boolean active,
        @Schema(description = "Creation timestamp")
        Instant createdAt,
        @Schema(description = "Last update timestamp")
        Instant updatedAt
    ) {
        public static ApplicationListItem from(Application app) {
            return new ApplicationListItem(
                TypedId.Ops.serialize(EntityType.APPLICATION, app.id),
                app.type != null ? app.type.name() : "APPLICATION",
                app.code,
                app.name,
                app.description,
                app.defaultBaseUrl,
                app.iconUrl,
                app.website,
                app.logoMimeType,
                app.serviceAccountId,
                TypedId.Ops.serialize(EntityType.PRINCIPAL, app.serviceAccountPrincipalId),
                app.active,
                app.createdAt,
                app.updatedAt
            );
        }
    }

    @Schema(description = "Application list response")
    public record ApplicationListResponse(
        @Schema(description = "List of applications")
        List<ApplicationListItem> applications,
        @Schema(description = "Total count")
        int total
    ) {}

    @Schema(description = "Full application details")
    public record ApplicationResponse(
        @Schema(description = "Application ID", example = "app_0ABC123DEF456")
        String id,
        @Schema(description = "Application type", example = "APPLICATION")
        String type,
        @Schema(description = "Unique application code", example = "my-app")
        String code,
        @Schema(description = "Display name", example = "My Application")
        String name,
        @Schema(description = "Description")
        String description,
        @Schema(description = "Default base URL")
        String defaultBaseUrl,
        @Schema(description = "Icon URL")
        String iconUrl,
        @Schema(description = "Website URL")
        String website,
        @Schema(description = "Logo data (base64)")
        String logo,
        @Schema(description = "Logo MIME type")
        String logoMimeType,
        @Schema(description = "Service account OAuth client ID")
        String serviceAccountId,
        @Schema(description = "Service account principal ID")
        String serviceAccountPrincipalId,
        @Schema(description = "Whether application is active")
        boolean active,
        @Schema(description = "Creation timestamp")
        Instant createdAt,
        @Schema(description = "Last update timestamp")
        Instant updatedAt,
        @Schema(description = "Service account details (only present after provisioning)")
        ServiceAccountInfo serviceAccount,
        @Schema(description = "Warning message if any")
        String warning
    ) {
        public static ApplicationResponse from(Application app) {
            return new ApplicationResponse(
                TypedId.Ops.serialize(EntityType.APPLICATION, app.id),
                app.type != null ? app.type.name() : "APPLICATION",
                app.code,
                app.name,
                app.description,
                app.defaultBaseUrl,
                app.iconUrl,
                app.website,
                app.logo,
                app.logoMimeType,
                app.serviceAccountId,
                TypedId.Ops.serialize(EntityType.PRINCIPAL, app.serviceAccountPrincipalId),
                app.active,
                app.createdAt,
                app.updatedAt,
                null,
                null
            );
        }

        public ApplicationResponse withServiceAccount(ServiceAccountInfo serviceAccount) {
            return new ApplicationResponse(
                id, type, code, name, description, defaultBaseUrl, iconUrl, website,
                logo, logoMimeType, serviceAccountId, serviceAccountPrincipalId,
                active, createdAt, updatedAt, serviceAccount, warning
            );
        }

        public ApplicationResponse withWarning(String warning) {
            return new ApplicationResponse(
                id, type, code, name, description, defaultBaseUrl, iconUrl, website,
                logo, logoMimeType, serviceAccountId, serviceAccountPrincipalId,
                active, createdAt, updatedAt, serviceAccount, warning
            );
        }
    }

    @Schema(description = "Service account information")
    public record ServiceAccountInfo(
        @Schema(description = "Principal ID of the service account")
        String principalId,
        @Schema(description = "Name of the service account")
        String name,
        @Schema(description = "OAuth client details")
        OAuthClientInfo oauthClient
    ) {}

    @Schema(description = "OAuth client information")
    public record OAuthClientInfo(
        @Schema(description = "OAuth client internal ID")
        String id,
        @Schema(description = "OAuth client_id for authentication")
        String clientId,
        @Schema(description = "OAuth client_secret (only returned at creation time)")
        String clientSecret
    ) {}

    // ========================================================================
    // Status Change Responses
    // ========================================================================

    @Schema(description = "Response for application status changes")
    public record ApplicationStatusResponse(
        @Schema(description = "Application ID")
        String id,
        @Schema(description = "New status", example = "ACTIVE")
        String status,
        @Schema(description = "Human-readable message")
        String message
    ) {
        public static ApplicationStatusResponse activated(String id) {
            return new ApplicationStatusResponse(id, "ACTIVE", "Application activated successfully");
        }

        public static ApplicationStatusResponse deactivated(String id) {
            return new ApplicationStatusResponse(id, "INACTIVE", "Application deactivated successfully");
        }

        public static ApplicationStatusResponse deleted(String id) {
            return new ApplicationStatusResponse(id, "DELETED", "Application deleted successfully");
        }
    }

    // ========================================================================
    // Service Account Responses
    // ========================================================================

    @Schema(description = "Response after provisioning a service account")
    public record ProvisionServiceAccountResponse(
        @Schema(description = "Status message")
        String message,
        @Schema(description = "Service account details")
        ServiceAccountInfo serviceAccount
    ) {}

    // ========================================================================
    // Client Configuration DTOs
    // ========================================================================

    @Schema(description = "Application client configuration")
    public record ClientConfigResponse(
        @Schema(description = "Config ID")
        String id,
        @Schema(description = "Application ID")
        String applicationId,
        @Schema(description = "Client ID")
        String clientId,
        @Schema(description = "Client name")
        String clientName,
        @Schema(description = "Client identifier")
        String clientIdentifier,
        @Schema(description = "Whether enabled for this client")
        boolean enabled,
        @Schema(description = "Base URL override for this client")
        String baseUrlOverride,
        @Schema(description = "Website override for this client")
        String websiteOverride,
        @Schema(description = "Effective base URL (override or default)")
        String effectiveBaseUrl,
        @Schema(description = "Effective website (override or default)")
        String effectiveWebsite,
        @Schema(description = "Additional configuration")
        Map<String, Object> config
    ) {
        public static ClientConfigResponse from(ApplicationClientConfig config, Client client, Application app) {
            String effectiveBaseUrl = (config.baseUrlOverride != null && !config.baseUrlOverride.isBlank())
                ? config.baseUrlOverride
                : (app != null ? app.defaultBaseUrl : null);
            String effectiveWebsite = (config.websiteOverride != null && !config.websiteOverride.isBlank())
                ? config.websiteOverride
                : (app != null ? app.website : null);

            return new ClientConfigResponse(
                TypedId.Ops.serialize(EntityType.APP_CLIENT_CONFIG, config.id),
                TypedId.Ops.serialize(EntityType.APPLICATION, config.applicationId),
                TypedId.Ops.serialize(EntityType.CLIENT, config.clientId),
                client != null ? client.name : null,
                client != null ? client.identifier : null,
                config.enabled,
                config.baseUrlOverride,
                config.websiteOverride,
                effectiveBaseUrl,
                effectiveWebsite,
                config.configJson
            );
        }

        /**
         * Create a ClientConfigResponse for an enabled configuration from event data.
         */
        public static ClientConfigResponse enabled(
                String configId, String applicationId, String clientId,
                String clientName, String clientIdentifier,
                String baseUrlOverride, String websiteOverride,
                String effectiveBaseUrl, String effectiveWebsite,
                Map<String, Object> config) {
            return new ClientConfigResponse(
                configId, applicationId, clientId,
                clientName, clientIdentifier,
                true, baseUrlOverride, websiteOverride,
                effectiveBaseUrl, effectiveWebsite, config
            );
        }

        /**
         * Create a ClientConfigResponse for a disabled configuration from event data.
         */
        public static ClientConfigResponse disabled(
                String configId, String applicationId, String clientId,
                String clientName, String clientIdentifier,
                String effectiveBaseUrl, String effectiveWebsite) {
            return new ClientConfigResponse(
                configId, applicationId, clientId,
                clientName, clientIdentifier,
                false, null, null,
                effectiveBaseUrl, effectiveWebsite, null
            );
        }
    }

    @Schema(description = "List of client configurations")
    public record ClientConfigListResponse(
        @Schema(description = "Client configurations")
        List<ClientConfigResponse> clientConfigs,
        @Schema(description = "Total count")
        int total
    ) {}

    @Schema(description = "Response for enable/disable application for client")
    public record ClientApplicationStatusResponse(
        @Schema(description = "Application ID")
        String applicationId,
        @Schema(description = "Client ID")
        String clientId,
        @Schema(description = "Whether now enabled")
        boolean enabled,
        @Schema(description = "Human-readable message")
        String message
    ) {
        public static ClientApplicationStatusResponse enabled(String applicationId, String clientId) {
            return new ClientApplicationStatusResponse(applicationId, clientId, true, "Application enabled for client");
        }

        public static ClientApplicationStatusResponse disabled(String applicationId, String clientId) {
            return new ClientApplicationStatusResponse(applicationId, clientId, false, "Application disabled for client");
        }
    }

    // ========================================================================
    // Application Roles
    // ========================================================================

    @Schema(description = "Response for application roles query")
    public record ApplicationRolesResponse(
        @Schema(description = "Application code")
        String applicationCode,
        @Schema(description = "Instructions for getting roles")
        String message
    ) {}
}
