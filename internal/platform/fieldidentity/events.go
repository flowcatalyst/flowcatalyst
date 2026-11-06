package fieldidentity

import (
	"encoding/json"
	"maps"
	"time"

	"github.com/flowcatalyst/go-sdk/usecase"
)

// Event types of the field-worker identity plane. Codes follow
// platform:{subdomain}:{aggregate}:{event} and are catalogued in the seed.
const (
	WorkerCreatedType    = "platform:iam:field-worker:created"
	WorkerEnsuredType    = "platform:iam:field-worker:ensured"
	WorkerUpdatedType    = "platform:iam:field-worker:updated"
	WorkerDeletedType    = "platform:iam:field-worker:deleted"
	WorkerSuspendedType  = "platform:iam:field-worker:suspended"
	WorkerActivatedType  = "platform:iam:field-worker:activated"
	WorkerPinResetType   = "platform:iam:field-worker:pin-reset"
	WorkerPinChangedType = "platform:iam:field-worker:pin-changed"
	WorkerUnlockedType   = "platform:iam:field-worker:unlocked"
	WorkerAppGrantedType = "platform:iam:field-worker:app-granted"
	WorkerAppRevokedType = "platform:iam:field-worker:app-revoked"

	WorkerBadgeIssuedType       = "platform:iam:field-worker:badge-issued"
	WorkerBadgeRevokedType      = "platform:iam:field-worker:badge-revoked"
	WorkerImpersonatedType      = "platform:iam:field-worker:impersonated"
	WorkerSignInCodeCreatedType = "platform:iam:field-worker:sign-in-code-created"

	EnrolmentCodeCreatedType = "platform:iam:field-device:enrolment-code-created"
	DeviceEnrolledType       = "platform:iam:field-device:enrolled"
	DeviceSuspendedType      = "platform:iam:field-device:suspended"
	DeviceActivatedType      = "platform:iam:field-device:activated"
	DeviceRevokedType        = "platform:iam:field-device:revoked"
	DeviceThrottledType      = "platform:iam:field-device:throttled"

	SessionRevokedType = "platform:iam:field-session:revoked"
	PolicyUpdatedType  = "platform:iam:field-policy:updated"

	// EventSource identifies the field-identity plane.
	EventSource = "platform:field"
)

// Event is the shared shape of every field-identity domain event; Type
// selects which. Data is the event payload (ToDataJSON).
type Event struct {
	Metadata usecase.EventMetadata
	Type     string
	// Aggregate is "field-worker", "field-device", "field-session" or
	// "field-policy"; ID is the aggregate's id (the client id for a policy).
	Aggregate string
	ID        string
	Data      map[string]any
}

// NewEvent builds an event for the aggregate id.
func NewEvent(ec usecase.ExecutionContext, eventType, aggregate, id string, data map[string]any) Event {
	return Event{
		Metadata:  usecase.NewEventMetadata(ec, eventType, EventSource, "platform."+aggregate+"."+id),
		Type:      eventType,
		Aggregate: aggregate,
		ID:        id,
		Data:      data,
	}
}

func (e Event) EventID() string       { return e.Metadata.EventID }
func (e Event) EventType() string     { return e.Type }
func (e Event) SpecVersion() string   { return "1.0" }
func (e Event) Source() string        { return EventSource }
func (e Event) Subject() string       { return "platform." + e.Aggregate + "." + e.ID }
func (e Event) Time() time.Time       { return e.Metadata.OccurredAt }
func (e Event) PrincipalID() string   { return e.Metadata.PrincipalID }
func (e Event) CorrelationID() string { return e.Metadata.CorrelationID }
func (e Event) CausationID() string   { return e.Metadata.CausationID }
func (e Event) ExecutionID() string   { return e.Metadata.ExecutionID }
func (e Event) MessageGroup() string  { return "platform:" + e.Aggregate + ":" + e.ID }
func (e Event) ToDataJSON() ([]byte, error) {
	return json.Marshal(e.Data)
}

// workerData is the payload common to the worker events.
func workerData(i *Identity, extra map[string]any) map[string]any {
	d := map[string]any{"workerId": i.ID, "clientId": i.ClientID, "workerCode": i.WorkerCode}
	maps.Copy(d, extra)
	return d
}

func deviceData(d *Device, extra map[string]any) map[string]any {
	out := map[string]any{"deviceId": d.ID, "clientId": d.ClientID, "mode": string(d.Mode)}
	maps.Copy(out, extra)
	return out
}
