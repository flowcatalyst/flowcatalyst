package operations

import (
	"encoding/json"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
)

const (
	FunctionCreatedType = "platform:admin:function:created"
	FunctionUpdatedType = "platform:admin:function:updated"
	FunctionDeletedType = "platform:admin:function:deleted"
	Source              = "platform:admin"
)

func subjectFor(id string) string { return "platform.function." + id }
func groupFor(id string) string   { return "platform:function:" + id }

// FunctionCreated is emitted on successful creation.
type FunctionCreated struct {
	Metadata   usecase.EventMetadata
	FunctionID string
	Address    string
	Name       string
}

func (e FunctionCreated) EventID() string       { return e.Metadata.EventID }
func (e FunctionCreated) EventType() string     { return FunctionCreatedType }
func (e FunctionCreated) SpecVersion() string   { return "1.0" }
func (e FunctionCreated) Source() string        { return Source }
func (e FunctionCreated) Subject() string       { return subjectFor(e.FunctionID) }
func (e FunctionCreated) Time() time.Time       { return e.Metadata.OccurredAt }
func (e FunctionCreated) PrincipalID() string   { return e.Metadata.PrincipalID }
func (e FunctionCreated) CorrelationID() string { return e.Metadata.CorrelationID }
func (e FunctionCreated) CausationID() string   { return e.Metadata.CausationID }
func (e FunctionCreated) ExecutionID() string   { return e.Metadata.ExecutionID }
func (e FunctionCreated) MessageGroup() string  { return groupFor(e.FunctionID) }
func (e FunctionCreated) ToDataJSON() ([]byte, error) {
	return json.Marshal(struct {
		FunctionID string `json:"functionId"`
		Address    string `json:"address"`
		Name       string `json:"name"`
	}{e.FunctionID, e.Address, e.Name})
}

// FunctionUpdated is emitted on a settings update (description, pool, warm,
// limits).
type FunctionUpdated struct {
	Metadata   usecase.EventMetadata
	FunctionID string
	Address    string
}

func (e FunctionUpdated) EventID() string       { return e.Metadata.EventID }
func (e FunctionUpdated) EventType() string     { return FunctionUpdatedType }
func (e FunctionUpdated) SpecVersion() string   { return "1.0" }
func (e FunctionUpdated) Source() string        { return Source }
func (e FunctionUpdated) Subject() string       { return subjectFor(e.FunctionID) }
func (e FunctionUpdated) Time() time.Time       { return e.Metadata.OccurredAt }
func (e FunctionUpdated) PrincipalID() string   { return e.Metadata.PrincipalID }
func (e FunctionUpdated) CorrelationID() string { return e.Metadata.CorrelationID }
func (e FunctionUpdated) CausationID() string   { return e.Metadata.CausationID }
func (e FunctionUpdated) ExecutionID() string   { return e.Metadata.ExecutionID }
func (e FunctionUpdated) MessageGroup() string  { return groupFor(e.FunctionID) }
func (e FunctionUpdated) ToDataJSON() ([]byte, error) {
	return json.Marshal(struct {
		FunctionID string `json:"functionId"`
		Address    string `json:"address"`
	}{e.FunctionID, e.Address})
}

// FunctionDeleted is emitted on deletion.
type FunctionDeleted struct {
	Metadata   usecase.EventMetadata
	FunctionID string
	Address    string
}

func (e FunctionDeleted) EventID() string       { return e.Metadata.EventID }
func (e FunctionDeleted) EventType() string     { return FunctionDeletedType }
func (e FunctionDeleted) SpecVersion() string   { return "1.0" }
func (e FunctionDeleted) Source() string        { return Source }
func (e FunctionDeleted) Subject() string       { return subjectFor(e.FunctionID) }
func (e FunctionDeleted) Time() time.Time       { return e.Metadata.OccurredAt }
func (e FunctionDeleted) PrincipalID() string   { return e.Metadata.PrincipalID }
func (e FunctionDeleted) CorrelationID() string { return e.Metadata.CorrelationID }
func (e FunctionDeleted) CausationID() string   { return e.Metadata.CausationID }
func (e FunctionDeleted) ExecutionID() string   { return e.Metadata.ExecutionID }
func (e FunctionDeleted) MessageGroup() string  { return groupFor(e.FunctionID) }
func (e FunctionDeleted) ToDataJSON() ([]byte, error) {
	return json.Marshal(struct {
		FunctionID string `json:"functionId"`
		Address    string `json:"address"`
	}{e.FunctionID, e.Address})
}
