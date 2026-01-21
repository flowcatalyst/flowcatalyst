package tech.flowcatalyst.platform.config;

import jakarta.inject.Inject;
import jakarta.ws.rs.GET;
import jakarta.ws.rs.Path;
import jakarta.ws.rs.Produces;
import jakarta.ws.rs.core.MediaType;
import org.eclipse.microprofile.openapi.annotations.Operation;
import org.eclipse.microprofile.openapi.annotations.media.Content;
import org.eclipse.microprofile.openapi.annotations.media.Schema;
import org.eclipse.microprofile.openapi.annotations.responses.APIResponse;
import org.eclipse.microprofile.openapi.annotations.tags.Tag;

/**
 * Public API for retrieving platform configuration.
 *
 * This endpoint returns feature flags and settings that the UI needs
 * to render appropriately based on the platform deployment configuration.
 */
@Path("/api/config/platform")
@Tag(name = "Platform Config", description = "Platform configuration and feature flags")
@Produces(MediaType.APPLICATION_JSON)
public class PlatformConfigResource {

    @Inject
    PlatformFeaturesConfig featuresConfig;

    @GET
    @Operation(summary = "Get platform configuration", description = "Returns platform feature flags and settings for the UI")
    @APIResponse(
            responseCode = "200",
            description = "Platform configuration",
            content = @Content(schema = @Schema(implementation = PlatformConfigResponse.class))
    )
    public PlatformConfigResponse getConfig() {
        return new PlatformConfigResponse(
                new FeaturesConfig(featuresConfig.messagingEnabled())
        );
    }

    /**
     * Platform configuration response.
     */
    public record PlatformConfigResponse(
            FeaturesConfig features
    ) {}

    /**
     * Feature flags configuration.
     */
    public record FeaturesConfig(
            boolean messagingEnabled
    ) {}
}
