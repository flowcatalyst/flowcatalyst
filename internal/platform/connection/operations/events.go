package operations

import (
	"encoding/json"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
)

const (
	ConnectionCreatedType = "platform:admin:connection:created"
	ConnectionUpdatedType = "platform:admin:connection:updated"
	ConnectionDeletedType = "platform:admin:connection:deleted"
	ConnectionsSyncedType = "platform:admin:connection:synced"
	Source                = "platform:admin"
)

func subjectFor(id string) string { return "platform.connection." + id }
func groupFor(id string) string   { return "platform:connection:" + id }

// ConnectionCreated event.
type ConnectionCreated struct {
	Metadata     usecase.EventMetadata
	ConnectionID string
	Code         string
	Name         string
}

func (e ConnectionCreated) EventID() string       { return e.Metadata.EventID }
func (e ConnectionCreated) EventType() string     { return ConnectionCreatedType }
func (e ConnectionCreated) SpecVersion() string   { return "1.0" }
func (e ConnectionCreated) Source() string        { return Source }
func (e ConnectionCreated) Subject() string       { return subjectFor(e.ConnectionID) }
func (e ConnectionCreated) Time() time.Time       { return e.Metadata.OccurredAt }
func (e ConnectionCreated) PrincipalID() string   { return e.Metadata.PrincipalID }
func (e ConnectionCreated) CorrelationID() string { return e.Metadata.CorrelationID }
func (e ConnectionCreated) CausationID() string   { return e.Metadata.CausationID }
func (e ConnectionCreated) ExecutionID() string   { return e.Metadata.ExecutionID }
func (e ConnectionCreated) MessageGroup() string  { return groupFor(e.ConnectionID) }
func (e ConnectionCreated) ToDataJSON() ([]byte, error) {
	return json.Marshal(struct {
		ConnectionID string `json:"connectionId"`
		Code         string `json:"code"`
		Name         string `json:"name"`
	}{e.ConnectionID, e.Code, e.Name})
}

// ConnectionUpdated event.
type ConnectionUpdated struct {
	Metadata     usecase.EventMetadata
	ConnectionID string
	Name         string
}

func (e ConnectionUpdated) EventID() string       { return e.Metadata.EventID }
func (e ConnectionUpdated) EventType() string     { return ConnectionUpdatedType }
func (e ConnectionUpdated) SpecVersion() string   { return "1.0" }
func (e ConnectionUpdated) Source() string        { return Source }
func (e ConnectionUpdated) Subject() string       { return subjectFor(e.ConnectionID) }
func (e ConnectionUpdated) Time() time.Time       { return e.Metadata.OccurredAt }
func (e ConnectionUpdated) PrincipalID() string   { return e.Metadata.PrincipalID }
func (e ConnectionUpdated) CorrelationID() string { return e.Metadata.CorrelationID }
func (e ConnectionUpdated) CausationID() string   { return e.Metadata.CausationID }
func (e ConnectionUpdated) ExecutionID() string   { return e.Metadata.ExecutionID }
func (e ConnectionUpdated) MessageGroup() string  { return groupFor(e.ConnectionID) }
func (e ConnectionUpdated) ToDataJSON() ([]byte, error) {
	return json.Marshal(struct {
		ConnectionID string `json:"connectionId"`
		Name         string `json:"name"`
	}{e.ConnectionID, e.Name})
}

// ConnectionDeleted event.
type ConnectionDeleted struct {
	Metadata     usecase.EventMetadata
	ConnectionID string
	Code         string
}

func (e ConnectionDeleted) EventID() string       { return e.Metadata.EventID }
func (e ConnectionDeleted) EventType() string     { return ConnectionDeletedType }
func (e ConnectionDeleted) SpecVersion() string   { return "1.0" }
func (e ConnectionDeleted) Source() string        { return Source }
func (e ConnectionDeleted) Subject() string       { return subjectFor(e.ConnectionID) }
func (e ConnectionDeleted) Time() time.Time       { return e.Metadata.OccurredAt }
func (e ConnectionDeleted) PrincipalID() string   { return e.Metadata.PrincipalID }
func (e ConnectionDeleted) CorrelationID() string { return e.Metadata.CorrelationID }
func (e ConnectionDeleted) CausationID() string   { return e.Metadata.CausationID }
func (e ConnectionDeleted) ExecutionID() string   { return e.Metadata.ExecutionID }
func (e ConnectionDeleted) MessageGroup() string  { return groupFor(e.ConnectionID) }
func (e ConnectionDeleted) ToDataJSON() ([]byte, error) {
	return json.Marshal(struct {
		ConnectionID string `json:"connectionId"`
		Code         string `json:"code"`
	}{e.ConnectionID, e.Code})
}

// ConnectionsSynced is the rollup emitted by the SDK app-scoped connection
// sync (SyncConnections). Shaped like subscription.SubscriptionsSynced, plus
// ClientID: unlike the subscription sync (application-scoped only), a
// connection sync is also scoped to one client (2026-09-21 ruling) — a nil
// ClientID synced the application's shared, client-less connections.
type ConnectionsSynced struct {
	Metadata        usecase.EventMetadata
	ApplicationCode string
	ClientID        *string
	Created         uint32
	Updated         uint32
	Deleted         uint32
	SyncedCodes     []string
}

func (e ConnectionsSynced) EventID() string       { return e.Metadata.EventID }
func (e ConnectionsSynced) EventType() string     { return ConnectionsSyncedType }
func (e ConnectionsSynced) SpecVersion() string   { return "1.0" }
func (e ConnectionsSynced) Source() string        { return Source }
func (e ConnectionsSynced) Subject() string       { return "platform.connections." + e.ApplicationCode }
func (e ConnectionsSynced) Time() time.Time       { return e.Metadata.OccurredAt }
func (e ConnectionsSynced) PrincipalID() string   { return e.Metadata.PrincipalID }
func (e ConnectionsSynced) CorrelationID() string { return e.Metadata.CorrelationID }
func (e ConnectionsSynced) CausationID() string   { return e.Metadata.CausationID }
func (e ConnectionsSynced) ExecutionID() string   { return e.Metadata.ExecutionID }

// MessageGroup: per-application rollup group, mirroring
// SubscriptionsSynced (X-08 / docs/owner-rulings-todo.md #28) — falling back
// to the bare aggregate group when there's genuinely no application in scope.
func (e ConnectionsSynced) MessageGroup() string {
	if e.ApplicationCode == "" {
		return "platform:connections"
	}
	return "platform:connections:" + e.ApplicationCode
}

func (e ConnectionsSynced) ToDataJSON() ([]byte, error) {
	return json.Marshal(struct {
		ApplicationCode string   `json:"applicationCode"`
		ClientID        *string  `json:"clientId,omitempty"`
		Created         uint32   `json:"created"`
		Updated         uint32   `json:"updated"`
		Deleted         uint32   `json:"deleted"`
		SyncedCodes     []string `json:"syncedCodes"`
	}{e.ApplicationCode, e.ClientID, e.Created, e.Updated, e.Deleted, e.SyncedCodes})
}
