package tech.flowcatalyst.platform.authorization;

/**
 * Definition of a permission in the FlowCatalyst system.
 *
 * Permissions follow the structure: {application}:{context}:{aggregate}:{action}
 *
 * Where:
 * - application: The registered application (e.g., "platform", "tms", "operant")
 * - context: Bounded context within the app (e.g., "iam", "admin", "messaging", "dispatch")
 * - aggregate: The entity/resource being accessed (e.g., "user", "role", "order")
 * - action: The operation (e.g., "view", "create", "update", "delete")
 *
 * Examples:
 * - platform:iam:user:create
 * - platform:admin:client:view
 * - platform:messaging:event-type:create
 * - tms:dispatch:order:update
 *
 * All parts must be lowercase alphanumeric with hyphens allowed.
 * Permissions are defined in code using the @Permission annotation.
 */
public interface PermissionDefinition {

    String application();  // Application code (e.g., "platform", "tms")
    String context();      // Bounded context within app (e.g., "iam", "admin", "messaging")
    String aggregate();    // Resource/entity (e.g., "user", "role", "order")
    String action();       // Operation (e.g., "view", "create", "update", "delete")
    String description();  // Human-readable description

    /**
     * Generate the string representation of this permission.
     * Format: {application}:{context}:{aggregate}:{action}
     *
     * @return Permission string (e.g., "platform:iam:user:create")
     */
    default String toPermissionString() {
        return String.format("%s:%s:%s:%s", application(), context(), aggregate(), action());
    }

    /**
     * Static factory method to create a permission.
     *
     * @param application Application code
     * @param context Bounded context
     * @param aggregate Resource/entity
     * @param action Operation
     * @param description Human-readable description
     * @return Permission instance
     */
    static PermissionRecord make(String application, String context, String aggregate,
                          String action, String description) {
        return new PermissionRecord(application, context, aggregate, action, description);
    }
}
