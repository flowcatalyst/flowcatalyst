package operations

import (
	"encoding/json"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
)

const (
	// Aggregate is "serviceaccount" (no hyphen) — matches the
	// platform_event_types.rs catalog. HTTP routes still use /service-accounts.
	ServiceAccountCreatedType            = "platform:iam:serviceaccount:created"
	ServiceAccountUpdatedType            = "platform:iam:serviceaccount:updated"
	ServiceAccountDeactivatedType        = "platform:iam:serviceaccount:deactivated"
	ServiceAccountDeletedType            = "platform:iam:serviceaccount:deleted"
	ServiceAccountRolesAssignedType      = "platform:iam:serviceaccount:roles-assigned"
	ServiceAccountTokenRegeneratedType   = "platform:iam:serviceaccount:token-regenerated"
	ServiceAccountSecretRegeneratedType  = "platform:iam:serviceaccount:secret-regenerated"
	Source                               = "platform:iam"
)

func subjectFor(id string) string { return "platform.serviceaccount." + id }
func groupFor(id string) string   { return "platform:serviceaccount:" + id }

type ServiceAccountCreated struct {
	Metadata         usecase.EventMetadata
	ServiceAccountID string
	Code             string
	Name             string
}

func (e ServiceAccountCreated) EventID() string       { return e.Metadata.EventID }
func (e ServiceAccountCreated) EventType() string     { return ServiceAccountCreatedType }
func (e ServiceAccountCreated) SpecVersion() string   { return "1.0" }
func (e ServiceAccountCreated) Source() string        { return Source }
func (e ServiceAccountCreated) Subject() string       { return subjectFor(e.ServiceAccountID) }
func (e ServiceAccountCreated) Time() time.Time       { return e.Metadata.OccurredAt }
func (e ServiceAccountCreated) PrincipalID() string   { return e.Metadata.PrincipalID }
func (e ServiceAccountCreated) CorrelationID() string { return e.Metadata.CorrelationID }
func (e ServiceAccountCreated) CausationID() string   { return e.Metadata.CausationID }
func (e ServiceAccountCreated) ExecutionID() string   { return e.Metadata.ExecutionID }
func (e ServiceAccountCreated) MessageGroup() string  { return groupFor(e.ServiceAccountID) }
func (e ServiceAccountCreated) ToDataJSON() ([]byte, error) {
	return json.Marshal(struct {
		ServiceAccountID string `json:"serviceAccountId"`
		Code             string `json:"code"`
		Name             string `json:"name"`
	}{e.ServiceAccountID, e.Code, e.Name})
}

type ServiceAccountUpdated struct {
	Metadata         usecase.EventMetadata
	ServiceAccountID string
	Name             string
}

func (e ServiceAccountUpdated) EventID() string       { return e.Metadata.EventID }
func (e ServiceAccountUpdated) EventType() string     { return ServiceAccountUpdatedType }
func (e ServiceAccountUpdated) SpecVersion() string   { return "1.0" }
func (e ServiceAccountUpdated) Source() string        { return Source }
func (e ServiceAccountUpdated) Subject() string       { return subjectFor(e.ServiceAccountID) }
func (e ServiceAccountUpdated) Time() time.Time       { return e.Metadata.OccurredAt }
func (e ServiceAccountUpdated) PrincipalID() string   { return e.Metadata.PrincipalID }
func (e ServiceAccountUpdated) CorrelationID() string { return e.Metadata.CorrelationID }
func (e ServiceAccountUpdated) CausationID() string   { return e.Metadata.CausationID }
func (e ServiceAccountUpdated) ExecutionID() string   { return e.Metadata.ExecutionID }
func (e ServiceAccountUpdated) MessageGroup() string  { return groupFor(e.ServiceAccountID) }
func (e ServiceAccountUpdated) ToDataJSON() ([]byte, error) {
	return json.Marshal(struct {
		ServiceAccountID string `json:"serviceAccountId"`
		Name             string `json:"name"`
	}{e.ServiceAccountID, e.Name})
}

type ServiceAccountDeactivated struct {
	Metadata         usecase.EventMetadata
	ServiceAccountID string
}

func (e ServiceAccountDeactivated) EventID() string       { return e.Metadata.EventID }
func (e ServiceAccountDeactivated) EventType() string     { return ServiceAccountDeactivatedType }
func (e ServiceAccountDeactivated) SpecVersion() string   { return "1.0" }
func (e ServiceAccountDeactivated) Source() string        { return Source }
func (e ServiceAccountDeactivated) Subject() string       { return subjectFor(e.ServiceAccountID) }
func (e ServiceAccountDeactivated) Time() time.Time       { return e.Metadata.OccurredAt }
func (e ServiceAccountDeactivated) PrincipalID() string   { return e.Metadata.PrincipalID }
func (e ServiceAccountDeactivated) CorrelationID() string { return e.Metadata.CorrelationID }
func (e ServiceAccountDeactivated) CausationID() string   { return e.Metadata.CausationID }
func (e ServiceAccountDeactivated) ExecutionID() string   { return e.Metadata.ExecutionID }
func (e ServiceAccountDeactivated) MessageGroup() string  { return groupFor(e.ServiceAccountID) }
func (e ServiceAccountDeactivated) ToDataJSON() ([]byte, error) {
	return json.Marshal(struct {
		ServiceAccountID string `json:"serviceAccountId"`
	}{e.ServiceAccountID})
}

type ServiceAccountDeleted struct {
	Metadata         usecase.EventMetadata
	ServiceAccountID string
	Code             string
}

func (e ServiceAccountDeleted) EventID() string       { return e.Metadata.EventID }
func (e ServiceAccountDeleted) EventType() string     { return ServiceAccountDeletedType }
func (e ServiceAccountDeleted) SpecVersion() string   { return "1.0" }
func (e ServiceAccountDeleted) Source() string        { return Source }
func (e ServiceAccountDeleted) Subject() string       { return subjectFor(e.ServiceAccountID) }
func (e ServiceAccountDeleted) Time() time.Time       { return e.Metadata.OccurredAt }
func (e ServiceAccountDeleted) PrincipalID() string   { return e.Metadata.PrincipalID }
func (e ServiceAccountDeleted) CorrelationID() string { return e.Metadata.CorrelationID }
func (e ServiceAccountDeleted) CausationID() string   { return e.Metadata.CausationID }
func (e ServiceAccountDeleted) ExecutionID() string   { return e.Metadata.ExecutionID }
func (e ServiceAccountDeleted) MessageGroup() string  { return groupFor(e.ServiceAccountID) }
func (e ServiceAccountDeleted) ToDataJSON() ([]byte, error) {
	return json.Marshal(struct {
		ServiceAccountID string `json:"serviceAccountId"`
		Code             string `json:"code"`
	}{e.ServiceAccountID, e.Code})
}

// ServiceAccountRolesAssigned — emitted by assign_roles after the
// role list is replaced. Payload carries the deltas only (the new full
// list is implicit from the aggregate's state).
type ServiceAccountRolesAssigned struct {
	Metadata         usecase.EventMetadata
	ServiceAccountID string
	RolesAdded       []string
	RolesRemoved     []string
}

func (e ServiceAccountRolesAssigned) EventID() string       { return e.Metadata.EventID }
func (e ServiceAccountRolesAssigned) EventType() string     { return ServiceAccountRolesAssignedType }
func (e ServiceAccountRolesAssigned) SpecVersion() string   { return "1.0" }
func (e ServiceAccountRolesAssigned) Source() string        { return Source }
func (e ServiceAccountRolesAssigned) Subject() string       { return subjectFor(e.ServiceAccountID) }
func (e ServiceAccountRolesAssigned) Time() time.Time       { return e.Metadata.OccurredAt }
func (e ServiceAccountRolesAssigned) PrincipalID() string   { return e.Metadata.PrincipalID }
func (e ServiceAccountRolesAssigned) CorrelationID() string { return e.Metadata.CorrelationID }
func (e ServiceAccountRolesAssigned) CausationID() string   { return e.Metadata.CausationID }
func (e ServiceAccountRolesAssigned) ExecutionID() string   { return e.Metadata.ExecutionID }
func (e ServiceAccountRolesAssigned) MessageGroup() string  { return groupFor(e.ServiceAccountID) }
func (e ServiceAccountRolesAssigned) ToDataJSON() ([]byte, error) {
	return json.Marshal(struct {
		ServiceAccountID string   `json:"serviceAccountId"`
		RolesAdded       []string `json:"rolesAdded"`
		RolesRemoved     []string `json:"rolesRemoved"`
	}{e.ServiceAccountID, defaultEmpty(e.RolesAdded), defaultEmpty(e.RolesRemoved)})
}

// ServiceAccountTokenRegenerated — bearer token rotation. The plaintext
// token is returned out-of-band via a sync.Map (see stashToken below).
type ServiceAccountTokenRegenerated struct {
	Metadata         usecase.EventMetadata
	ServiceAccountID string
	Code             string
}

func (e ServiceAccountTokenRegenerated) EventID() string       { return e.Metadata.EventID }
func (e ServiceAccountTokenRegenerated) EventType() string     { return ServiceAccountTokenRegeneratedType }
func (e ServiceAccountTokenRegenerated) SpecVersion() string   { return "1.0" }
func (e ServiceAccountTokenRegenerated) Source() string        { return Source }
func (e ServiceAccountTokenRegenerated) Subject() string       { return subjectFor(e.ServiceAccountID) }
func (e ServiceAccountTokenRegenerated) Time() time.Time       { return e.Metadata.OccurredAt }
func (e ServiceAccountTokenRegenerated) PrincipalID() string   { return e.Metadata.PrincipalID }
func (e ServiceAccountTokenRegenerated) CorrelationID() string { return e.Metadata.CorrelationID }
func (e ServiceAccountTokenRegenerated) CausationID() string   { return e.Metadata.CausationID }
func (e ServiceAccountTokenRegenerated) ExecutionID() string   { return e.Metadata.ExecutionID }
func (e ServiceAccountTokenRegenerated) MessageGroup() string  { return groupFor(e.ServiceAccountID) }
func (e ServiceAccountTokenRegenerated) ToDataJSON() ([]byte, error) {
	return json.Marshal(struct {
		ServiceAccountID string `json:"serviceAccountId"`
		Code             string `json:"code"`
	}{e.ServiceAccountID, e.Code})
}

// ServiceAccountSecretRegenerated — signing-secret rotation. Plaintext
// returned out-of-band like the token.
type ServiceAccountSecretRegenerated struct {
	Metadata         usecase.EventMetadata
	ServiceAccountID string
	Code             string
}

func (e ServiceAccountSecretRegenerated) EventID() string       { return e.Metadata.EventID }
func (e ServiceAccountSecretRegenerated) EventType() string     { return ServiceAccountSecretRegeneratedType }
func (e ServiceAccountSecretRegenerated) SpecVersion() string   { return "1.0" }
func (e ServiceAccountSecretRegenerated) Source() string        { return Source }
func (e ServiceAccountSecretRegenerated) Subject() string       { return subjectFor(e.ServiceAccountID) }
func (e ServiceAccountSecretRegenerated) Time() time.Time       { return e.Metadata.OccurredAt }
func (e ServiceAccountSecretRegenerated) PrincipalID() string   { return e.Metadata.PrincipalID }
func (e ServiceAccountSecretRegenerated) CorrelationID() string { return e.Metadata.CorrelationID }
func (e ServiceAccountSecretRegenerated) CausationID() string   { return e.Metadata.CausationID }
func (e ServiceAccountSecretRegenerated) ExecutionID() string   { return e.Metadata.ExecutionID }
func (e ServiceAccountSecretRegenerated) MessageGroup() string  { return groupFor(e.ServiceAccountID) }
func (e ServiceAccountSecretRegenerated) ToDataJSON() ([]byte, error) {
	return json.Marshal(struct {
		ServiceAccountID string `json:"serviceAccountId"`
		Code             string `json:"code"`
	}{e.ServiceAccountID, e.Code})
}

func defaultEmpty(xs []string) []string {
	if xs == nil {
		return []string{}
	}
	return xs
}
