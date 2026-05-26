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
