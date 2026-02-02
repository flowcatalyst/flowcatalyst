package tech.flowcatalyst.platform.common.api;

import org.eclipse.microprofile.openapi.annotations.media.Schema;

import java.util.List;
import java.util.Map;

/**
 * Standard API response DTOs for consistent API design.
 *
 * These records are used across all admin and SDK endpoints to ensure
 * consistent response structures and proper OpenAPI documentation.
 */
public final class ApiResponses {

    private ApiResponses() {} // Prevent instantiation

    // ========================================================================
    // Success Responses
    // ========================================================================

    /**
     * Simple message response for operations that don't return an entity.
     */
    @Schema(description = "Simple message response")
    public record MessageResponse(
        @Schema(description = "Human-readable message", example = "Operation completed successfully")
        String message
    ) {}

    /**
     * Response for delete operations - confirms what was deleted.
     */
    @Schema(description = "Delete operation response")
    public record DeleteResponse(
        @Schema(description = "ID of the deleted resource", example = "app_0ABC123DEF456")
        String id,
        @Schema(description = "Type of resource deleted", example = "application")
        String resourceType,
        @Schema(description = "Human-readable message", example = "Application deleted successfully")
        String message
    ) {
        public DeleteResponse(String id, String resourceType) {
            this(id, resourceType, resourceType.substring(0, 1).toUpperCase() + resourceType.substring(1) + " deleted successfully");
        }
    }

    /**
     * Response for status change operations (activate, deactivate, suspend, etc.)
     */
    @Schema(description = "Status change response")
    public record StatusChangeResponse(
        @Schema(description = "ID of the affected resource", example = "app_0ABC123DEF456")
        String id,
        @Schema(description = "New status of the resource", example = "ACTIVE")
        String status,
        @Schema(description = "Human-readable message", example = "Application activated successfully")
        String message
    ) {}

    // ========================================================================
    // Error Responses
    // ========================================================================

    /**
     * Standard error response for 400 Bad Request errors.
     */
    @Schema(description = "Error response for client errors (400)")
    public record ErrorResponse(
        @Schema(description = "Error code for programmatic handling", example = "VALIDATION_ERROR")
        String code,
        @Schema(description = "Human-readable error message", example = "Invalid input provided")
        String message,
        @Schema(description = "Additional error details")
        Map<String, Object> details
    ) {
        public ErrorResponse(String code, String message) {
            this(code, message, Map.of());
        }

        public ErrorResponse(String message) {
            this("ERROR", message, Map.of());
        }
    }

    /**
     * Validation error response with field-level errors.
     */
    @Schema(description = "Validation error response (400)")
    public record ValidationErrorResponse(
        @Schema(description = "Error code", example = "VALIDATION_ERROR")
        String code,
        @Schema(description = "General error message", example = "Validation failed")
        String message,
        @Schema(description = "Field-level validation errors")
        List<FieldError> errors
    ) {
        public record FieldError(
            @Schema(description = "Field name", example = "email")
            String field,
            @Schema(description = "Error message for this field", example = "must be a valid email address")
            String message
        ) {}
    }

    /**
     * Not found error response for 404 errors.
     */
    @Schema(description = "Not found error response (404)")
    public record NotFoundResponse(
        @Schema(description = "Error code", example = "NOT_FOUND")
        String code,
        @Schema(description = "Human-readable error message", example = "Application not found")
        String message,
        @Schema(description = "Type of resource that was not found", example = "application")
        String resourceType,
        @Schema(description = "ID that was searched for", example = "app_0ABC123DEF456")
        String resourceId
    ) {
        public NotFoundResponse(String resourceType, String resourceId) {
            this("NOT_FOUND",
                 resourceType.substring(0, 1).toUpperCase() + resourceType.substring(1) + " not found",
                 resourceType,
                 resourceId);
        }
    }

    /**
     * Conflict error response for 409 errors.
     */
    @Schema(description = "Conflict error response (409)")
    public record ConflictResponse(
        @Schema(description = "Error code", example = "CONFLICT")
        String code,
        @Schema(description = "Human-readable error message", example = "Resource already exists")
        String message,
        @Schema(description = "Additional conflict details")
        Map<String, Object> details
    ) {
        public ConflictResponse(String code, String message) {
            this(code, message, Map.of());
        }
    }

    /**
     * Unauthorized error response for 401 errors.
     */
    @Schema(description = "Unauthorized error response (401)")
    public record UnauthorizedResponse(
        @Schema(description = "Error code", example = "UNAUTHORIZED")
        String code,
        @Schema(description = "Human-readable error message", example = "Authentication required")
        String message
    ) {
        public UnauthorizedResponse() {
            this("UNAUTHORIZED", "Authentication required");
        }

        public UnauthorizedResponse(String message) {
            this("UNAUTHORIZED", message);
        }
    }

    /**
     * Forbidden error response for 403 errors.
     */
    @Schema(description = "Forbidden error response (403)")
    public record ForbiddenResponse(
        @Schema(description = "Error code", example = "FORBIDDEN")
        String code,
        @Schema(description = "Human-readable error message", example = "Insufficient permissions")
        String message,
        @Schema(description = "Required permission if applicable", example = "application:delete")
        String requiredPermission
    ) {
        public ForbiddenResponse() {
            this("FORBIDDEN", "Insufficient permissions", null);
        }

        public ForbiddenResponse(String message) {
            this("FORBIDDEN", message, null);
        }

        public ForbiddenResponse(String message, String requiredPermission) {
            this("FORBIDDEN", message, requiredPermission);
        }
    }
}
