package operations

import (
	"encoding/json"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
)

const (
	ProcessCreatedType  = "platform:admin:process:created"
	ProcessUpdatedType  = "platform:admin:process:updated"
	ProcessArchivedType = "platform:admin:process:archived"
	ProcessDeletedType  = "platform:admin:process:deleted"
	Source              = "platform:admin"
)

func subjectFor(id string) string { return "platform.process." + id }
func groupFor(id string) string   { return "platform:process:" + id }

type ProcessCreated struct {
	Metadata  usecase.EventMetadata
	ProcessID string
	Code      string
	Name      string
}

func (e ProcessCreated) EventID() string       { return e.Metadata.EventID }
func (e ProcessCreated) EventType() string     { return ProcessCreatedType }
func (e ProcessCreated) SpecVersion() string   { return "1.0" }
func (e ProcessCreated) Source() string        { return Source }
func (e ProcessCreated) Subject() string       { return subjectFor(e.ProcessID) }
func (e ProcessCreated) Time() time.Time       { return e.Metadata.OccurredAt }
func (e ProcessCreated) PrincipalID() string   { return e.Metadata.PrincipalID }
func (e ProcessCreated) CorrelationID() string { return e.Metadata.CorrelationID }
func (e ProcessCreated) CausationID() string   { return e.Metadata.CausationID }
func (e ProcessCreated) ExecutionID() string   { return e.Metadata.ExecutionID }
func (e ProcessCreated) MessageGroup() string  { return groupFor(e.ProcessID) }
func (e ProcessCreated) ToDataJSON() ([]byte, error) {
	return json.Marshal(struct {
		ProcessID string `json:"processId"`
		Code      string `json:"code"`
		Name      string `json:"name"`
	}{e.ProcessID, e.Code, e.Name})
}

type ProcessUpdated struct {
	Metadata  usecase.EventMetadata
	ProcessID string
	Name      string
}

func (e ProcessUpdated) EventID() string       { return e.Metadata.EventID }
func (e ProcessUpdated) EventType() string     { return ProcessUpdatedType }
func (e ProcessUpdated) SpecVersion() string   { return "1.0" }
func (e ProcessUpdated) Source() string        { return Source }
func (e ProcessUpdated) Subject() string       { return subjectFor(e.ProcessID) }
func (e ProcessUpdated) Time() time.Time       { return e.Metadata.OccurredAt }
func (e ProcessUpdated) PrincipalID() string   { return e.Metadata.PrincipalID }
func (e ProcessUpdated) CorrelationID() string { return e.Metadata.CorrelationID }
func (e ProcessUpdated) CausationID() string   { return e.Metadata.CausationID }
func (e ProcessUpdated) ExecutionID() string   { return e.Metadata.ExecutionID }
func (e ProcessUpdated) MessageGroup() string  { return groupFor(e.ProcessID) }
func (e ProcessUpdated) ToDataJSON() ([]byte, error) {
	return json.Marshal(struct {
		ProcessID string `json:"processId"`
		Name      string `json:"name"`
	}{e.ProcessID, e.Name})
}

type ProcessArchived struct {
	Metadata  usecase.EventMetadata
	ProcessID string
	Code      string
}

func (e ProcessArchived) EventID() string       { return e.Metadata.EventID }
func (e ProcessArchived) EventType() string     { return ProcessArchivedType }
func (e ProcessArchived) SpecVersion() string   { return "1.0" }
func (e ProcessArchived) Source() string        { return Source }
func (e ProcessArchived) Subject() string       { return subjectFor(e.ProcessID) }
func (e ProcessArchived) Time() time.Time       { return e.Metadata.OccurredAt }
func (e ProcessArchived) PrincipalID() string   { return e.Metadata.PrincipalID }
func (e ProcessArchived) CorrelationID() string { return e.Metadata.CorrelationID }
func (e ProcessArchived) CausationID() string   { return e.Metadata.CausationID }
func (e ProcessArchived) ExecutionID() string   { return e.Metadata.ExecutionID }
func (e ProcessArchived) MessageGroup() string  { return groupFor(e.ProcessID) }
func (e ProcessArchived) ToDataJSON() ([]byte, error) {
	return json.Marshal(struct {
		ProcessID string `json:"processId"`
		Code      string `json:"code"`
	}{e.ProcessID, e.Code})
}

type ProcessDeleted struct {
	Metadata  usecase.EventMetadata
	ProcessID string
	Code      string
}

func (e ProcessDeleted) EventID() string       { return e.Metadata.EventID }
func (e ProcessDeleted) EventType() string     { return ProcessDeletedType }
func (e ProcessDeleted) SpecVersion() string   { return "1.0" }
func (e ProcessDeleted) Source() string        { return Source }
func (e ProcessDeleted) Subject() string       { return subjectFor(e.ProcessID) }
func (e ProcessDeleted) Time() time.Time       { return e.Metadata.OccurredAt }
func (e ProcessDeleted) PrincipalID() string   { return e.Metadata.PrincipalID }
func (e ProcessDeleted) CorrelationID() string { return e.Metadata.CausationID }
func (e ProcessDeleted) CausationID() string   { return e.Metadata.CausationID }
func (e ProcessDeleted) ExecutionID() string   { return e.Metadata.ExecutionID }
func (e ProcessDeleted) MessageGroup() string  { return groupFor(e.ProcessID) }
func (e ProcessDeleted) ToDataJSON() ([]byte, error) {
	return json.Marshal(struct {
		ProcessID string `json:"processId"`
		Code      string `json:"code"`
	}{e.ProcessID, e.Code})
}
