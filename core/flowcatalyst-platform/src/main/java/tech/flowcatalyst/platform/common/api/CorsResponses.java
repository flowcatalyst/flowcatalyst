package tech.flowcatalyst.platform.common.api;

import org.eclipse.microprofile.openapi.annotations.media.Schema;

import java.time.Instant;
import java.util.List;
import java.util.Set;

/**
 * Response DTOs for CORS Admin API endpoints.
 */
public final class CorsResponses {

    private CorsResponses() {}

    // ========================================================================
    // CORS Origin DTOs
    // ========================================================================

    @Schema(description = "CORS allowed origin")
    public record CorsOriginDto(
        @Schema(description = "Origin ID")
        String id,
        @Schema(description = "Origin URL", example = "https://example.com")
        String origin,
        @Schema(description = "Description of why this origin is allowed")
        String description,
        @Schema(description = "Principal ID who created this entry")
        String createdBy,
        @Schema(description = "When this entry was created")
        String createdAt
    ) {}

    @Schema(description = "List of CORS origins")
    public record CorsOriginListResponse(
        @Schema(description = "CORS origin entries")
        List<CorsOriginDto> items,
        @Schema(description = "Total count")
        int total
    ) {}

    @Schema(description = "Cached allowed origins")
    public record AllowedOriginsResponse(
        @Schema(description = "Set of allowed origin URLs")
        Set<String> origins
    ) {}

    @Schema(description = "Response after deleting a CORS origin")
    public record CorsOriginDeletedResponse(
        @Schema(description = "Deleted origin ID")
        String id,
        @Schema(description = "Human-readable message")
        String message
    ) {
        public static CorsOriginDeletedResponse success(String id) {
            return new CorsOriginDeletedResponse(id, "CORS origin deleted successfully");
        }
    }
}
