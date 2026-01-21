package tech.flowcatalyst.platform.authorization.platform;

import tech.flowcatalyst.platform.authorization.Permission;
import tech.flowcatalyst.platform.authorization.PermissionDefinition;

/**
 * Messaging and event management permissions.
 * Controls access to event types, subscriptions, and dispatch jobs.
 */
@Permission
public class PlatformMessagingPermissions {

    // ========================================================================
    // Event Type Management
    // ========================================================================

    // ========================================================================
    // Event Viewing (events_read projection)
    // ========================================================================

    public static final PermissionDefinition EVENT_VIEW = PermissionDefinition.make(
        "platform", "messaging", "event", "view",
        "View events in the event store"
    );

    public static final PermissionDefinition EVENT_VIEW_RAW = PermissionDefinition.make(
        "platform", "messaging", "event", "view-raw",
        "View raw events (debug/admin)"
    );

    // ========================================================================
    // Event Type Management
    // ========================================================================

    public static final PermissionDefinition EVENT_TYPE_VIEW = PermissionDefinition.make(
        "platform", "messaging", "event-type", "view",
        "View event type definitions"
    );

    public static final PermissionDefinition EVENT_TYPE_CREATE = PermissionDefinition.make(
        "platform", "messaging", "event-type", "create",
        "Create new event types"
    );

    public static final PermissionDefinition EVENT_TYPE_UPDATE = PermissionDefinition.make(
        "platform", "messaging", "event-type", "update",
        "Update event type definitions"
    );

    public static final PermissionDefinition EVENT_TYPE_DELETE = PermissionDefinition.make(
        "platform", "messaging", "event-type", "delete",
        "Delete event types"
    );

    // ========================================================================
    // Subscription Management
    // ========================================================================

    public static final PermissionDefinition SUBSCRIPTION_VIEW = PermissionDefinition.make(
        "platform", "messaging", "subscription", "view",
        "View webhook subscriptions"
    );

    public static final PermissionDefinition SUBSCRIPTION_CREATE = PermissionDefinition.make(
        "platform", "messaging", "subscription", "create",
        "Create webhook subscriptions"
    );

    public static final PermissionDefinition SUBSCRIPTION_UPDATE = PermissionDefinition.make(
        "platform", "messaging", "subscription", "update",
        "Update webhook subscriptions"
    );

    public static final PermissionDefinition SUBSCRIPTION_DELETE = PermissionDefinition.make(
        "platform", "messaging", "subscription", "delete",
        "Delete webhook subscriptions"
    );

    // ========================================================================
    // Dispatch Job Management
    // ========================================================================

    public static final PermissionDefinition DISPATCH_JOB_VIEW = PermissionDefinition.make(
        "platform", "messaging", "dispatch-job", "view",
        "View dispatch jobs and delivery status"
    );

    public static final PermissionDefinition DISPATCH_JOB_VIEW_RAW = PermissionDefinition.make(
        "platform", "messaging", "dispatch-job", "view-raw",
        "View raw dispatch jobs (debug/admin)"
    );

    public static final PermissionDefinition DISPATCH_JOB_CREATE = PermissionDefinition.make(
        "platform", "messaging", "dispatch-job", "create",
        "Create new dispatch jobs"
    );

    public static final PermissionDefinition DISPATCH_JOB_RETRY = PermissionDefinition.make(
        "platform", "messaging", "dispatch-job", "retry",
        "Retry failed dispatch jobs"
    );

    // ========================================================================
    // Dispatch Pool Management
    // ========================================================================

    public static final PermissionDefinition DISPATCH_POOL_VIEW = PermissionDefinition.make(
        "platform", "messaging", "dispatch-pool", "view",
        "View dispatch pools and configuration"
    );

    public static final PermissionDefinition DISPATCH_POOL_CREATE = PermissionDefinition.make(
        "platform", "messaging", "dispatch-pool", "create",
        "Create new dispatch pools"
    );

    public static final PermissionDefinition DISPATCH_POOL_UPDATE = PermissionDefinition.make(
        "platform", "messaging", "dispatch-pool", "update",
        "Update dispatch pool configuration"
    );

    public static final PermissionDefinition DISPATCH_POOL_DELETE = PermissionDefinition.make(
        "platform", "messaging", "dispatch-pool", "delete",
        "Delete dispatch pools"
    );

    private PlatformMessagingPermissions() {}
}
