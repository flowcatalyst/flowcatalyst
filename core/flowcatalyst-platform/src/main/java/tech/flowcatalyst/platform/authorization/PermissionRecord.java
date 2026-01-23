package tech.flowcatalyst.platform.authorization;

import java.util.regex.Pattern;

/**
 * Concrete permission record implementation.
 *
 * Format: {application}:{context}:{aggregate}:{action}
 */
public record PermissionRecord(
    String application,
    String context,
    String aggregate,
    String action,
    String description
) implements PermissionDefinition {

    private static final Pattern VALID_PART = Pattern.compile("^[a-z0-9][a-z0-9_-]*[a-z0-9]$|^[a-z0-9]$");

    public PermissionRecord {
        // Validate all parts
        validatePart(application, "application");
        validatePart(context, "context");
        validatePart(aggregate, "aggregate");
        validatePart(action, "action");
        // Description is optional
    }

    /**
     * Validate that a part follows the naming rules:
     * - Lowercase letters, numbers, hyphens, and underscores only
     * - Cannot start or end with a hyphen or underscore
     * - At least 1 character
     */
    private void validatePart(String part, String partName) {
        if (part == null || part.isBlank()) {
            throw new IllegalArgumentException(partName + " cannot be null or empty");
        }

        if (!VALID_PART.matcher(part).matches()) {
            throw new IllegalArgumentException(
                partName + " must be lowercase alphanumeric with hyphens/underscores (cannot start/end with hyphen or underscore): " + part
            );
        }
    }

    @Override
    public String toString() {
        return description != null
            ? toPermissionString() + " (" + description + ")"
            : toPermissionString();
    }
}
