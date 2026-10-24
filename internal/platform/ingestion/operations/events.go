package operations

import (
	"encoding/json"
	"time"

	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecase"
)

const (
	IngestionCreatedType      = "platform:ingestion:ingestion:created"
	IngestionUpdatedType      = "platform:ingestion:ingestion:updated"
	IngestionDeletedType      = "platform:ingestion:ingestion:deleted"
	IngestionPausedType       = "platform:ingestion:ingestion:paused"
	IngestionResumedType      = "platform:ingestion:ingestion:resumed"
	IngestionVersionSavedType = "platform:ingestion:ingestion:version-saved"
	IngestionVersionLiveType  = "platform:ingestion:ingestion:version-live"
	Source                    = "platform:ingestion"
)

func subjectFor(id string) string { return "platform.ingestion." + id }
func groupFor(id string) string   { return "platform:ingestion:" + id }

// ingestionEvent is what every ingestion event carries: the envelope and the
// ingestion it is about. Each event adds its type and its data.
type ingestionEvent struct {
	Metadata    usecase.EventMetadata
	IngestionID string
}

func (e ingestionEvent) EventID() string       { return e.Metadata.EventID }
func (e ingestionEvent) SpecVersion() string   { return "1.0" }
func (e ingestionEvent) Source() string        { return Source }
func (e ingestionEvent) Subject() string       { return subjectFor(e.IngestionID) }
func (e ingestionEvent) Time() time.Time       { return e.Metadata.OccurredAt }
func (e ingestionEvent) PrincipalID() string   { return e.Metadata.PrincipalID }
func (e ingestionEvent) CorrelationID() string { return e.Metadata.CorrelationID }
func (e ingestionEvent) CausationID() string   { return e.Metadata.CausationID }
func (e ingestionEvent) ExecutionID() string   { return e.Metadata.ExecutionID }
func (e ingestionEvent) MessageGroup() string  { return groupFor(e.IngestionID) }

func newEvent(ec usecase.ExecutionContext, eventType, id string) ingestionEvent {
	return ingestionEvent{Metadata: usecase.NewEventMetadata(ec, eventType, Source, subjectFor(id)), IngestionID: id}
}

// IngestionCreated is emitted when an ingestion is created, with its first
// version (live or waiting).
type IngestionCreated struct {
	ingestionEvent
	ApplicationID string
	ClientID      string
	Code          string
	Version       int
}

func (IngestionCreated) EventType() string { return IngestionCreatedType }
func (e IngestionCreated) ToDataJSON() ([]byte, error) {
	return json.Marshal(struct {
		IngestionID   string `json:"ingestionId"`
		ApplicationID string `json:"applicationId"`
		ClientID      string `json:"clientId"`
		Code          string `json:"code"`
		Version       int    `json:"version"`
	}{e.IngestionID, e.ApplicationID, e.ClientID, e.Code, e.Version})
}

// IngestionUpdated is emitted when an edit saved a new version and made it
// live; Version is that version.
type IngestionUpdated struct {
	ingestionEvent
	Code    string
	Version int
}

func (IngestionUpdated) EventType() string { return IngestionUpdatedType }
func (e IngestionUpdated) ToDataJSON() ([]byte, error) {
	return json.Marshal(struct {
		IngestionID string `json:"ingestionId"`
		Code        string `json:"code"`
		Version     int    `json:"version"`
	}{e.IngestionID, e.Code, e.Version})
}

// IngestionDeleted is emitted when an ingestion is deleted.
type IngestionDeleted struct {
	ingestionEvent
	Code string
}

func (IngestionDeleted) EventType() string { return IngestionDeletedType }
func (e IngestionDeleted) ToDataJSON() ([]byte, error) {
	return json.Marshal(struct {
		IngestionID string `json:"ingestionId"`
		Code        string `json:"code"`
	}{e.IngestionID, e.Code})
}

// IngestionPaused is emitted when an ingestion is paused.
type IngestionPaused struct {
	ingestionEvent
	Code string
}

func (IngestionPaused) EventType() string { return IngestionPausedType }
func (e IngestionPaused) ToDataJSON() ([]byte, error) {
	return json.Marshal(struct {
		IngestionID string `json:"ingestionId"`
		Code        string `json:"code"`
	}{e.IngestionID, e.Code})
}

// IngestionResumed is emitted when a paused ingestion is resumed.
type IngestionResumed struct {
	ingestionEvent
	Code string
}

func (IngestionResumed) EventType() string { return IngestionResumedType }
func (e IngestionResumed) ToDataJSON() ([]byte, error) {
	return json.Marshal(struct {
		IngestionID string `json:"ingestionId"`
		Code        string `json:"code"`
	}{e.IngestionID, e.Code})
}

// IngestionVersionSaved is emitted when a definition is saved as the next
// version; Live says whether it was also made live.
type IngestionVersionSaved struct {
	ingestionEvent
	Code    string
	Version int
	Live    bool
}

func (IngestionVersionSaved) EventType() string { return IngestionVersionSavedType }
func (e IngestionVersionSaved) ToDataJSON() ([]byte, error) {
	return json.Marshal(struct {
		IngestionID string `json:"ingestionId"`
		Code        string `json:"code"`
		Version     int    `json:"version"`
		Live        bool   `json:"live"`
	}{e.IngestionID, e.Code, e.Version, e.Live})
}

// IngestionVersionLive is emitted when a saved version is made the one
// runtimes receive.
type IngestionVersionLive struct {
	ingestionEvent
	Code    string
	Version int
}

func (IngestionVersionLive) EventType() string { return IngestionVersionLiveType }
func (e IngestionVersionLive) ToDataJSON() ([]byte, error) {
	return json.Marshal(struct {
		IngestionID string `json:"ingestionId"`
		Code        string `json:"code"`
		Version     int    `json:"version"`
	}{e.IngestionID, e.Code, e.Version})
}
