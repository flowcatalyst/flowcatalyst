package tech.flowcatalyst.platform.authorization;

import java.util.regex.Pattern;

/**
 * Structured input for permission definition.
 *
 * <p>Permissions follow the structure: {application}:{context}:{aggregate}:{action}
 *
 * <p>Each segment is provided separately to enforce the format.
 * Segments must be lowercase alphanumeric with hyphens or underscores, starting with a letter.
 *
 * @param application Application code (e.g., "platform", "operant")
 * @param context     Bounded context within app (e.g., "iam", "dispatch", "orders")
 * @param aggregate   Resource/entity (e.g., "user", "order", "trip")
 * @param action      Operation (e.g., "view", "create", "update", "delete")
 */
public record PermissionInput(
    String application,
    String context,
    String aggregate,
    String action
) {
    /**
     * Segment format: lowercase alphanumeric with hyphens or underscores, starting with letter.
     */
    private static final Pattern SEGMENT_PATTERN = Pattern.compile("^[a-z][a-z0-9_-]*$");

    /**
     * Build the full permission string from segments.
     *
     * @return The permission in format {application}:{context}:{aggregate}:{action}
     */
    public String buildPermissionString() {
        return String.join(":",
            application != null ? application.toLowerCase() : "",
            context != null ? context.toLowerCase() : "",
            aggregate != null ? aggregate.toLowerCase() : "",
            action != null ? action.toLowerCase() : ""
        );
    }

    /**
     * Validate all segments of this permission.
     *
     * @return null if valid, or an error message if invalid
     */
    public String validate() {
        String appError = validateSegment("application", application);
        if (appError != null) return appError;

        String ctxError = validateSegment("context", context);
        if (ctxError != null) return ctxError;

        String aggError = validateSegment("aggregate", aggregate);
        if (aggError != null) return aggError;

        String actError = validateSegment("action", action);
        if (actError != null) return actError;

        return null;
    }

    /**
     * Check if this permission input is valid.
     */
    public boolean isValid() {
        return validate() == null;
    }

    private String validateSegment(String fieldName, String value) {
        if (value == null || value.isBlank()) {
            return fieldName + " is required";
        }
        if (!SEGMENT_PATTERN.matcher(value).matches()) {
            return fieldName + " must be lowercase alphanumeric with hyphens/underscores, starting with a letter";
        }
        return null;
    }
}
