package portalidentity

import (
	"encoding/json"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
)

const (
	IdentityEnsuredType   = "platform:portal:identity:ensured"
	IdentityStatusSetType = "platform:portal:identity:status-set"
	IdentityDeletedType   = "platform:portal:identity:deleted"
	// EventSource identifies the portal identity plane.
	EventSource = "platform:portal"
)

func subjectFor(id string) string { return "platform.portal-identity." + id }
func groupFor(id string) string   { return "platform:portal-identity:" + id }

// IdentityEnsured — emitted when an identity is created or re-ensured
// (reactivated) in a client's portal context.
type IdentityEnsured struct {
	Metadata   usecase.EventMetadata
	IdentityID string
	ClientID   string
	Email      string
	Created    bool
	Source_    string
}

func (e IdentityEnsured) EventID() string       { return e.Metadata.EventID }
func (e IdentityEnsured) EventType() string     { return IdentityEnsuredType }
func (e IdentityEnsured) SpecVersion() string   { return "1.0" }
func (e IdentityEnsured) Source() string        { return EventSource }
func (e IdentityEnsured) Subject() string       { return subjectFor(e.IdentityID) }
func (e IdentityEnsured) Time() time.Time       { return e.Metadata.OccurredAt }
func (e IdentityEnsured) PrincipalID() string   { return e.Metadata.PrincipalID }
func (e IdentityEnsured) CorrelationID() string { return e.Metadata.CorrelationID }
func (e IdentityEnsured) CausationID() string   { return e.Metadata.CausationID }
func (e IdentityEnsured) ExecutionID() string   { return e.Metadata.ExecutionID }
func (e IdentityEnsured) MessageGroup() string  { return groupFor(e.IdentityID) }
func (e IdentityEnsured) ToDataJSON() ([]byte, error) {
	return json.Marshal(struct {
		IdentityID string `json:"identityId"`
		ClientID   string `json:"clientId"`
		Email      string `json:"email"`
		Created    bool   `json:"created"`
		Source     string `json:"source"`
	}{e.IdentityID, e.ClientID, e.Email, e.Created, e.Source_})
}

// IdentityStatusSet — emitted on suspension/reactivation.
type IdentityStatusSet struct {
	Metadata   usecase.EventMetadata
	IdentityID string
	ClientID   string
	Status     string
}

func (e IdentityStatusSet) EventID() string       { return e.Metadata.EventID }
func (e IdentityStatusSet) EventType() string     { return IdentityStatusSetType }
func (e IdentityStatusSet) SpecVersion() string   { return "1.0" }
func (e IdentityStatusSet) Source() string        { return EventSource }
func (e IdentityStatusSet) Subject() string       { return subjectFor(e.IdentityID) }
func (e IdentityStatusSet) Time() time.Time       { return e.Metadata.OccurredAt }
func (e IdentityStatusSet) PrincipalID() string   { return e.Metadata.PrincipalID }
func (e IdentityStatusSet) CorrelationID() string { return e.Metadata.CorrelationID }
func (e IdentityStatusSet) CausationID() string   { return e.Metadata.CausationID }
func (e IdentityStatusSet) ExecutionID() string   { return e.Metadata.ExecutionID }
func (e IdentityStatusSet) MessageGroup() string  { return groupFor(e.IdentityID) }
func (e IdentityStatusSet) ToDataJSON() ([]byte, error) {
	return json.Marshal(struct {
		IdentityID string `json:"identityId"`
		ClientID   string `json:"clientId"`
		Status     string `json:"status"`
	}{e.IdentityID, e.ClientID, e.Status})
}

// IdentityDeleted — emitted when an identity is removed (offboarding).
type IdentityDeleted struct {
	Metadata   usecase.EventMetadata
	IdentityID string
	ClientID   string
	Email      string
}

func (e IdentityDeleted) EventID() string       { return e.Metadata.EventID }
func (e IdentityDeleted) EventType() string     { return IdentityDeletedType }
func (e IdentityDeleted) SpecVersion() string   { return "1.0" }
func (e IdentityDeleted) Source() string        { return EventSource }
func (e IdentityDeleted) Subject() string       { return subjectFor(e.IdentityID) }
func (e IdentityDeleted) Time() time.Time       { return e.Metadata.OccurredAt }
func (e IdentityDeleted) PrincipalID() string   { return e.Metadata.PrincipalID }
func (e IdentityDeleted) CorrelationID() string { return e.Metadata.CorrelationID }
func (e IdentityDeleted) CausationID() string   { return e.Metadata.CausationID }
func (e IdentityDeleted) ExecutionID() string   { return e.Metadata.ExecutionID }
func (e IdentityDeleted) MessageGroup() string  { return groupFor(e.IdentityID) }
func (e IdentityDeleted) ToDataJSON() ([]byte, error) {
	return json.Marshal(struct {
		IdentityID string `json:"identityId"`
		ClientID   string `json:"clientId"`
		Email      string `json:"email"`
	}{e.IdentityID, e.ClientID, e.Email})
}
