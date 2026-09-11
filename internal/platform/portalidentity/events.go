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
	// AppID/AppCode name the portal app granted by this ensure, if any.
	AppID   string
	AppCode string
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
		IdentityID    string `json:"identityId"`
		ClientID      string `json:"clientId"`
		Email         string `json:"email"`
		Created       bool   `json:"created"`
		Source        string `json:"source"`
		PortalAppID   string `json:"portalAppId,omitempty"`
		PortalAppCode string `json:"portalAppCode,omitempty"`
	}{e.IdentityID, e.ClientID, e.Email, e.Created, e.Source_, e.AppID, e.AppCode})
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

const (
	IdentityAppGrantedType = "platform:portal:identity:app-granted"
	IdentityAppRevokedType = "platform:portal:identity:app-revoked"
	AppCreatedType         = "platform:portal:app:created"
	AppUpdatedType         = "platform:portal:app:updated"
	AppDeletedType         = "platform:portal:app:deleted"
)

func appSubjectFor(id string) string { return "platform.portal-app." + id }
func appGroupFor(id string) string   { return "platform:portal-app:" + id }

// IdentityAppGranted — an identity gained access to a portal app.
type IdentityAppGranted struct {
	Metadata   usecase.EventMetadata
	IdentityID string
	ClientID   string
	AppID      string
	AppCode    string
	Source_    string
}

func (e IdentityAppGranted) EventID() string       { return e.Metadata.EventID }
func (e IdentityAppGranted) EventType() string     { return IdentityAppGrantedType }
func (e IdentityAppGranted) SpecVersion() string   { return "1.0" }
func (e IdentityAppGranted) Source() string        { return EventSource }
func (e IdentityAppGranted) Subject() string       { return subjectFor(e.IdentityID) }
func (e IdentityAppGranted) Time() time.Time       { return e.Metadata.OccurredAt }
func (e IdentityAppGranted) PrincipalID() string   { return e.Metadata.PrincipalID }
func (e IdentityAppGranted) CorrelationID() string { return e.Metadata.CorrelationID }
func (e IdentityAppGranted) CausationID() string   { return e.Metadata.CausationID }
func (e IdentityAppGranted) ExecutionID() string   { return e.Metadata.ExecutionID }
func (e IdentityAppGranted) MessageGroup() string  { return groupFor(e.IdentityID) }
func (e IdentityAppGranted) ToDataJSON() ([]byte, error) {
	return json.Marshal(struct {
		IdentityID    string `json:"identityId"`
		ClientID      string `json:"clientId"`
		PortalAppID   string `json:"portalAppId"`
		PortalAppCode string `json:"portalAppCode"`
		Source        string `json:"source"`
	}{e.IdentityID, e.ClientID, e.AppID, e.AppCode, e.Source_})
}

// IdentityAppRevoked — an identity lost access to a portal app.
type IdentityAppRevoked struct {
	Metadata   usecase.EventMetadata
	IdentityID string
	ClientID   string
	AppID      string
	AppCode    string
}

func (e IdentityAppRevoked) EventID() string       { return e.Metadata.EventID }
func (e IdentityAppRevoked) EventType() string     { return IdentityAppRevokedType }
func (e IdentityAppRevoked) SpecVersion() string   { return "1.0" }
func (e IdentityAppRevoked) Source() string        { return EventSource }
func (e IdentityAppRevoked) Subject() string       { return subjectFor(e.IdentityID) }
func (e IdentityAppRevoked) Time() time.Time       { return e.Metadata.OccurredAt }
func (e IdentityAppRevoked) PrincipalID() string   { return e.Metadata.PrincipalID }
func (e IdentityAppRevoked) CorrelationID() string { return e.Metadata.CorrelationID }
func (e IdentityAppRevoked) CausationID() string   { return e.Metadata.CausationID }
func (e IdentityAppRevoked) ExecutionID() string   { return e.Metadata.ExecutionID }
func (e IdentityAppRevoked) MessageGroup() string  { return groupFor(e.IdentityID) }
func (e IdentityAppRevoked) ToDataJSON() ([]byte, error) {
	return json.Marshal(struct {
		IdentityID    string `json:"identityId"`
		ClientID      string `json:"clientId"`
		PortalAppID   string `json:"portalAppId"`
		PortalAppCode string `json:"portalAppCode"`
	}{e.IdentityID, e.ClientID, e.AppID, e.AppCode})
}

// AppChanged is the shared shape of the portal-app lifecycle events
// (created / updated / deleted); Type selects which.
type AppChanged struct {
	Metadata usecase.EventMetadata
	Type     string
	AppID    string
	ClientID string
	Code     string
	Name     string
}

func (e AppChanged) EventID() string       { return e.Metadata.EventID }
func (e AppChanged) EventType() string     { return e.Type }
func (e AppChanged) SpecVersion() string   { return "1.0" }
func (e AppChanged) Source() string        { return EventSource }
func (e AppChanged) Subject() string       { return appSubjectFor(e.AppID) }
func (e AppChanged) Time() time.Time       { return e.Metadata.OccurredAt }
func (e AppChanged) PrincipalID() string   { return e.Metadata.PrincipalID }
func (e AppChanged) CorrelationID() string { return e.Metadata.CorrelationID }
func (e AppChanged) CausationID() string   { return e.Metadata.CausationID }
func (e AppChanged) ExecutionID() string   { return e.Metadata.ExecutionID }
func (e AppChanged) MessageGroup() string  { return appGroupFor(e.AppID) }
func (e AppChanged) ToDataJSON() ([]byte, error) {
	return json.Marshal(struct {
		PortalAppID string `json:"portalAppId"`
		ClientID    string `json:"clientId"`
		Code        string `json:"code"`
		Name        string `json:"name"`
	}{e.AppID, e.ClientID, e.Code, e.Name})
}
