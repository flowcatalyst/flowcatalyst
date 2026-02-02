package tech.flowcatalyst.platform.common.api;

import org.eclipse.microprofile.openapi.annotations.media.Schema;
import tech.flowcatalyst.platform.client.Client;
import tech.flowcatalyst.platform.client.ClientStatus;
import tech.flowcatalyst.platform.shared.EntityType;
import tech.flowcatalyst.platform.shared.TypedId;

import java.time.Instant;
import java.util.List;

/**
 * Response DTOs for Client Admin API endpoints.
 */
public final class ClientResponses {

    private ClientResponses() {}

    // ========================================================================
    // Client DTOs
    // ========================================================================

    @Schema(description = "Client details")
    public record ClientDto(
        @Schema(description = "Client ID", example = "cli_0ABC123DEF456")
        String id,
        @Schema(description = "Client name", example = "Acme Corporation")
        String name,
        @Schema(description = "Unique client identifier/slug", example = "acme-corp")
        String identifier,
        @Schema(description = "Client status")
        ClientStatus status,
        @Schema(description = "Reason for current status")
        String statusReason,
        @Schema(description = "When status was last changed")
        Instant statusChangedAt,
        @Schema(description = "Creation timestamp")
        Instant createdAt,
        @Schema(description = "Last update timestamp")
        Instant updatedAt
    ) {
        public static ClientDto from(Client client) {
            return new ClientDto(
                TypedId.Ops.serialize(EntityType.CLIENT, client.id),
                client.name,
                client.identifier,
                client.status,
                client.statusReason,
                client.statusChangedAt,
                client.createdAt,
                client.updatedAt
            );
        }
    }

    @Schema(description = "Client list response")
    public record ClientListResponse(
        @Schema(description = "List of clients")
        List<ClientDto> clients,
        @Schema(description = "Total count")
        int total
    ) {}

    // ========================================================================
    // Status Change Responses
    // ========================================================================

    @Schema(description = "Response for client status changes")
    public record ClientStatusResponse(
        @Schema(description = "Client ID")
        String id,
        @Schema(description = "New status", example = "ACTIVE")
        String status,
        @Schema(description = "Human-readable message")
        String message
    ) {
        public static ClientStatusResponse activated(String id) {
            return new ClientStatusResponse(id, "ACTIVE", "Client activated successfully");
        }

        public static ClientStatusResponse suspended(String id) {
            return new ClientStatusResponse(id, "SUSPENDED", "Client suspended successfully");
        }

        public static ClientStatusResponse deactivated(String id) {
            return new ClientStatusResponse(id, "INACTIVE", "Client deactivated successfully");
        }
    }

    // ========================================================================
    // Application Management DTOs
    // ========================================================================

    @Schema(description = "Application with enabled status for a client")
    public record ClientApplicationDto(
        @Schema(description = "Application ID", example = "app_0ABC123DEF456")
        String id,
        @Schema(description = "Application code", example = "my-app")
        String code,
        @Schema(description = "Application name", example = "My Application")
        String name,
        @Schema(description = "Application description")
        String description,
        @Schema(description = "Icon URL")
        String iconUrl,
        @Schema(description = "Default website URL")
        String website,
        @Schema(description = "Effective website URL (override or default)")
        String effectiveWebsite,
        @Schema(description = "Logo MIME type")
        String logoMimeType,
        @Schema(description = "Whether application is globally active")
        boolean active,
        @Schema(description = "Whether application is enabled for this client")
        boolean enabledForClient
    ) {}

    @Schema(description = "Applications for a client")
    public record ClientApplicationsResponse(
        @Schema(description = "List of applications with their enabled status")
        List<ClientApplicationDto> applications,
        @Schema(description = "Total count")
        int total
    ) {}

    @Schema(description = "Response for enabling/disabling application for client")
    public record ClientApplicationStatusResponse(
        @Schema(description = "Client ID")
        String clientId,
        @Schema(description = "Application ID")
        String applicationId,
        @Schema(description = "Whether now enabled")
        boolean enabled,
        @Schema(description = "Human-readable message")
        String message
    ) {
        public static ClientApplicationStatusResponse enabled(String clientId, String applicationId) {
            return new ClientApplicationStatusResponse(clientId, applicationId, true, "Application enabled for client");
        }

        public static ClientApplicationStatusResponse disabled(String clientId, String applicationId) {
            return new ClientApplicationStatusResponse(clientId, applicationId, false, "Application disabled for client");
        }
    }

    // ========================================================================
    // Audit Note Response
    // ========================================================================

    @Schema(description = "Response after adding an audit note")
    public record NoteAddedResponse(
        @Schema(description = "Client ID")
        String clientId,
        @Schema(description = "Note category")
        String category,
        @Schema(description = "Human-readable message")
        String message
    ) {
        public static NoteAddedResponse created(String clientId, String category) {
            return new NoteAddedResponse(clientId, category, "Note added successfully");
        }
    }

    // ========================================================================
    // Bulk Update Response
    // ========================================================================

    @Schema(description = "Response for bulk application update")
    public record ApplicationsUpdatedResponse(
        @Schema(description = "Client ID")
        String clientId,
        @Schema(description = "Number of applications now enabled")
        int enabledCount,
        @Schema(description = "Human-readable message")
        String message
    ) {
        public static ApplicationsUpdatedResponse success(String clientId, int enabledCount) {
            return new ApplicationsUpdatedResponse(
                clientId,
                enabledCount,
                "Applications updated successfully. " + enabledCount + " application(s) enabled."
            );
        }
    }
}
