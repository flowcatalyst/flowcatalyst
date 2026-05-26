package operations

import (
	"encoding/json"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
)

const (
	RoleCreatedType = "platform:admin:role:created"
	RoleUpdatedType = "platform:admin:role:updated"
	RoleDeletedType = "platform:admin:role:deleted"
	Source          = "platform:admin"
)

func subjectFor(id string) string { return "platform.role." + id }
func groupFor(id string) string   { return "platform:role:" + id }

type RoleCreated struct {
	Metadata usecase.EventMetadata
	RoleID   string
	Name     string
}

func (e RoleCreated) EventID() string       { return e.Metadata.EventID }
func (e RoleCreated) EventType() string     { return RoleCreatedType }
func (e RoleCreated) SpecVersion() string   { return "1.0" }
func (e RoleCreated) Source() string        { return Source }
func (e RoleCreated) Subject() string       { return subjectFor(e.RoleID) }
func (e RoleCreated) Time() time.Time       { return e.Metadata.OccurredAt }
func (e RoleCreated) PrincipalID() string   { return e.Metadata.PrincipalID }
func (e RoleCreated) CorrelationID() string { return e.Metadata.CorrelationID }
func (e RoleCreated) CausationID() string   { return e.Metadata.CausationID }
func (e RoleCreated) ExecutionID() string   { return e.Metadata.ExecutionID }
func (e RoleCreated) MessageGroup() string  { return groupFor(e.RoleID) }
func (e RoleCreated) ToDataJSON() ([]byte, error) {
	return json.Marshal(struct {
		RoleID string `json:"roleId"`
		Name   string `json:"name"`
	}{e.RoleID, e.Name})
}

type RoleUpdated struct {
	Metadata usecase.EventMetadata
	RoleID   string
	Name     string
}

func (e RoleUpdated) EventID() string       { return e.Metadata.EventID }
func (e RoleUpdated) EventType() string     { return RoleUpdatedType }
func (e RoleUpdated) SpecVersion() string   { return "1.0" }
func (e RoleUpdated) Source() string        { return Source }
func (e RoleUpdated) Subject() string       { return subjectFor(e.RoleID) }
func (e RoleUpdated) Time() time.Time       { return e.Metadata.OccurredAt }
func (e RoleUpdated) PrincipalID() string   { return e.Metadata.PrincipalID }
func (e RoleUpdated) CorrelationID() string { return e.Metadata.CorrelationID }
func (e RoleUpdated) CausationID() string   { return e.Metadata.CausationID }
func (e RoleUpdated) ExecutionID() string   { return e.Metadata.ExecutionID }
func (e RoleUpdated) MessageGroup() string  { return groupFor(e.RoleID) }
func (e RoleUpdated) ToDataJSON() ([]byte, error) {
	return json.Marshal(struct {
		RoleID string `json:"roleId"`
		Name   string `json:"name"`
	}{e.RoleID, e.Name})
}

type RoleDeleted struct {
	Metadata usecase.EventMetadata
	RoleID   string
	Name     string
}

func (e RoleDeleted) EventID() string       { return e.Metadata.EventID }
func (e RoleDeleted) EventType() string     { return RoleDeletedType }
func (e RoleDeleted) SpecVersion() string   { return "1.0" }
func (e RoleDeleted) Source() string        { return Source }
func (e RoleDeleted) Subject() string       { return subjectFor(e.RoleID) }
func (e RoleDeleted) Time() time.Time       { return e.Metadata.OccurredAt }
func (e RoleDeleted) PrincipalID() string   { return e.Metadata.PrincipalID }
func (e RoleDeleted) CorrelationID() string { return e.Metadata.CorrelationID }
func (e RoleDeleted) CausationID() string   { return e.Metadata.CausationID }
func (e RoleDeleted) ExecutionID() string   { return e.Metadata.ExecutionID }
func (e RoleDeleted) MessageGroup() string  { return groupFor(e.RoleID) }
func (e RoleDeleted) ToDataJSON() ([]byte, error) {
	return json.Marshal(struct {
		RoleID string `json:"roleId"`
		Name   string `json:"name"`
	}{e.RoleID, e.Name})
}
