package tech.flowcatalyst.platform.authorization;

import java.util.Collections;
import java.util.HashSet;
import java.util.Set;
import java.util.regex.Pattern;

/**
 * Concrete role record implementation.
 *
 * Format: {application}:{role-name}
 */
public record RoleRecord(
    String application,
    String roleName,
    Set<PermissionRecord> permissions,
    String description
) implements RoleDefinition {

    private static final Pattern VALID_PART = Pattern.compile("^[a-z0-9][a-z0-9-]*[a-z0-9]$|^[a-z0-9]$");

    public RoleRecord {
        // Validate parts
        validatePart(application, "application");
        validatePart(roleName, "roleName");

        if (description == null || description.isBlank()) {
            throw new IllegalArgumentException("Description cannot be null or empty");
        }

        // Make permissions immutable (empty set is valid - role can exist without permissions)
        permissions = permissions == null
            ? Collections.emptySet()
            : Collections.unmodifiableSet(new HashSet<>(permissions));

        // Validate that all permissions are valid (they validate themselves in their constructors)
        for (PermissionRecord permission : permissions) {
            if (permission == null) {
                throw new IllegalArgumentException("Permission cannot be null");
            }
        }
    }

    /**
     * Validate that a part follows the naming rules:
     * - Lowercase letters, numbers, and hyphens only
     * - Cannot start or end with a hyphen
     * - At least 1 character
     */
    private void validatePart(String part, String partName) {
        if (part == null || part.isBlank()) {
            throw new IllegalArgumentException(partName + " cannot be null or empty");
        }

        if (!VALID_PART.matcher(part).matches()) {
            throw new IllegalArgumentException(
                partName + " must be lowercase alphanumeric with hyphens (cannot start/end with hyphen): " + part
            );
        }
    }

    @Override
    public String toString() {
        return toRoleString() + " (" + permissions.size() + " permissions: " + description + ")";
    }
}
