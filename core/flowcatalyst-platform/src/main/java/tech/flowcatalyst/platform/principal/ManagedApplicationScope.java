package tech.flowcatalyst.platform.principal;

/**
 * Scope for application management access.
 *
 * <p>Determines which applications a principal can manage (create roles,
 * permissions, event types, subscriptions, etc. for).
 *
 * <ul>
 *   <li>{@link #ALL} - Can manage all applications (platform admins)</li>
 *   <li>{@link #SPECIFIC} - Can only manage applications in managedApplicationIds</li>
 *   <li>{@link #NONE} - Cannot manage any applications (default)</li>
 * </ul>
 */
public enum ManagedApplicationScope {

    /**
     * Can manage all applications.
     * Typically granted to platform super-admins.
     */
    ALL,

    /**
     * Can only manage specific applications listed in managedApplicationIds.
     * Used for application service accounts and application-scoped admins.
     */
    SPECIFIC,

    /**
     * Cannot manage any applications.
     * Default scope for regular users without application management access.
     */
    NONE
}
