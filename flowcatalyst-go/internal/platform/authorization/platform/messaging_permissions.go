package platform

import (
	"go.flowcatalyst.tech/internal/platform/authorization"
)

// Messaging and event management permissions.
// Controls access to event types, subscriptions, and dispatch jobs.
var (
	// Event Viewing (events_read projection)
	EventView = authorization.MustPermission(
		"platform", "messaging", "event", "view",
		"View events in the event store",
	)
	EventViewRaw = authorization.MustPermission(
		"platform", "messaging", "event", "view-raw",
		"View raw events (debug/admin)",
	)

	// Event Type Management
	EventTypeView = authorization.MustPermission(
		"platform", "messaging", "event-type", "view",
		"View event type definitions",
	)
	EventTypeCreate = authorization.MustPermission(
		"platform", "messaging", "event-type", "create",
		"Create new event types",
	)
	EventTypeUpdate = authorization.MustPermission(
		"platform", "messaging", "event-type", "update",
		"Update event type definitions",
	)
	EventTypeDelete = authorization.MustPermission(
		"platform", "messaging", "event-type", "delete",
		"Delete event types",
	)

	// Subscription Management
	SubscriptionView = authorization.MustPermission(
		"platform", "messaging", "subscription", "view",
		"View webhook subscriptions",
	)
	SubscriptionCreate = authorization.MustPermission(
		"platform", "messaging", "subscription", "create",
		"Create webhook subscriptions",
	)
	SubscriptionUpdate = authorization.MustPermission(
		"platform", "messaging", "subscription", "update",
		"Update webhook subscriptions",
	)
	SubscriptionDelete = authorization.MustPermission(
		"platform", "messaging", "subscription", "delete",
		"Delete webhook subscriptions",
	)

	// Dispatch Job Management
	DispatchJobView = authorization.MustPermission(
		"platform", "messaging", "dispatch-job", "view",
		"View dispatch jobs and delivery status",
	)
	DispatchJobViewRaw = authorization.MustPermission(
		"platform", "messaging", "dispatch-job", "view-raw",
		"View raw dispatch jobs (debug/admin)",
	)
	DispatchJobCreate = authorization.MustPermission(
		"platform", "messaging", "dispatch-job", "create",
		"Create new dispatch jobs",
	)
	DispatchJobRetry = authorization.MustPermission(
		"platform", "messaging", "dispatch-job", "retry",
		"Retry failed dispatch jobs",
	)

	// Dispatch Pool Management
	DispatchPoolView = authorization.MustPermission(
		"platform", "messaging", "dispatch-pool", "view",
		"View dispatch pools and configuration",
	)
	DispatchPoolCreate = authorization.MustPermission(
		"platform", "messaging", "dispatch-pool", "create",
		"Create new dispatch pools",
	)
	DispatchPoolUpdate = authorization.MustPermission(
		"platform", "messaging", "dispatch-pool", "update",
		"Update dispatch pool configuration",
	)
	DispatchPoolDelete = authorization.MustPermission(
		"platform", "messaging", "dispatch-pool", "delete",
		"Delete dispatch pools",
	)
)

// AllMessagingPermissions returns all messaging permissions for registration.
func AllMessagingPermissions() []*authorization.PermissionRecord {
	return []*authorization.PermissionRecord{
		EventView, EventViewRaw,
		EventTypeView, EventTypeCreate, EventTypeUpdate, EventTypeDelete,
		SubscriptionView, SubscriptionCreate, SubscriptionUpdate, SubscriptionDelete,
		DispatchJobView, DispatchJobViewRaw, DispatchJobCreate, DispatchJobRetry,
		DispatchPoolView, DispatchPoolCreate, DispatchPoolUpdate, DispatchPoolDelete,
	}
}
