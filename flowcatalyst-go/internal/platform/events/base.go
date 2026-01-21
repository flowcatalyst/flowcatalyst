// Package events defines all domain events for the FlowCatalyst platform.
// These events are emitted when state changes occur in the domain aggregates.
package events

import (
	"fmt"

	"go.flowcatalyst.tech/internal/platform/common"
)

const (
	// SourceControlPlane is the event source for platform control plane events
	SourceControlPlane = "platform:control-plane"

	// Default spec version for all events
	DefaultSpecVersion = "1.0"
)

// Event type codes follow the format: {app}:{domain}:{aggregate}:{action}
// These are used to identify event types in a standardized way

// EventType event codes
const (
	EventTypeEventTypeCreated          = "platform:control-plane:eventtype:created"
	EventTypeEventTypeUpdated          = "platform:control-plane:eventtype:updated"
	EventTypeEventTypeArchived         = "platform:control-plane:eventtype:archived"
	EventTypeEventTypeSchemaAdded      = "platform:control-plane:eventtype:schema-added"
	EventTypeEventTypeSchemaFinalised  = "platform:control-plane:eventtype:schema-finalised"
	EventTypeEventTypeSchemaDeprecated = "platform:control-plane:eventtype:schema-deprecated"
)

// Subscription event codes
const (
	EventTypeSubscriptionCreated = "platform:control-plane:subscription:created"
	EventTypeSubscriptionUpdated = "platform:control-plane:subscription:updated"
	EventTypeSubscriptionPaused  = "platform:control-plane:subscription:paused"
	EventTypeSubscriptionResumed = "platform:control-plane:subscription:resumed"
	EventTypeSubscriptionDeleted = "platform:control-plane:subscription:deleted"
)

// DispatchPool event codes
const (
	EventTypeDispatchPoolCreated   = "platform:control-plane:dispatchpool:created"
	EventTypeDispatchPoolUpdated   = "platform:control-plane:dispatchpool:updated"
	EventTypeDispatchPoolSuspended = "platform:control-plane:dispatchpool:suspended"
	EventTypeDispatchPoolArchived  = "platform:control-plane:dispatchpool:archived"
)

// Principal event codes
const (
	EventTypePrincipalUserCreated           = "platform:control-plane:principal:user-created"
	EventTypePrincipalUserUpdated           = "platform:control-plane:principal:user-updated"
	EventTypePrincipalUserActivated         = "platform:control-plane:principal:user-activated"
	EventTypePrincipalUserDeactivated       = "platform:control-plane:principal:user-deactivated"
	EventTypePrincipalUserDeleted           = "platform:control-plane:principal:user-deleted"
	EventTypePrincipalRolesAssigned         = "platform:control-plane:principal:roles-assigned"
	EventTypePrincipalClientAccessGranted   = "platform:control-plane:principal:client-access-granted"
	EventTypePrincipalClientAccessRevoked   = "platform:control-plane:principal:client-access-revoked"
)

// Client event codes
const (
	EventTypeClientCreated   = "platform:control-plane:client:created"
	EventTypeClientUpdated   = "platform:control-plane:client:updated"
	EventTypeClientSuspended = "platform:control-plane:client:suspended"
	EventTypeClientActivated = "platform:control-plane:client:activated"
)

// Application event codes
const (
	EventTypeApplicationCreated     = "platform:control-plane:application:created"
	EventTypeApplicationUpdated     = "platform:control-plane:application:updated"
	EventTypeApplicationDeactivated = "platform:control-plane:application:deactivated"
	EventTypeApplicationProvisioned = "platform:control-plane:application:provisioned"
)

// Role event codes
const (
	EventTypeRoleCreated = "platform:control-plane:role:created"
	EventTypeRoleUpdated = "platform:control-plane:role:updated"
	EventTypeRoleDeleted = "platform:control-plane:role:deleted"
)

// ServiceAccount event codes
const (
	EventTypeServiceAccountCreated            = "platform:control-plane:serviceaccount:created"
	EventTypeServiceAccountCredentialsRotated = "platform:control-plane:serviceaccount:credentials-rotated"
	EventTypeServiceAccountDeleted            = "platform:control-plane:serviceaccount:deleted"
)

// subject builds a subject string for domain events
// Format: {domain}.{aggregate}.{id}
func subject(domain, aggregate, id string) string {
	return fmt.Sprintf("%s.%s.%s", domain, aggregate, id)
}

// messageGroup builds a message group key for FIFO ordering
// Format: {domain}:{aggregate}:{id}
func messageGroup(domain, aggregate, id string) string {
	return fmt.Sprintf("%s:%s:%s", domain, aggregate, id)
}

// newBase creates a BaseDomainEvent with standard settings
func newBase(ctx *common.ExecutionContext, eventType, domain, aggregate, id string) common.BaseDomainEvent {
	return common.NewBaseDomainEvent(
		ctx,
		eventType,
		subject(domain, aggregate, id),
		messageGroup(domain, aggregate, id),
	)
}
