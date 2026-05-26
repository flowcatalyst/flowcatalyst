package operations

import (
	"encoding/json"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
)

const (
	IdentityProviderCreatedType = "platform:admin:identity-provider:created"
	IdentityProviderUpdatedType = "platform:admin:identity-provider:updated"
	IdentityProviderDeletedType = "platform:admin:identity-provider:deleted"
	Source                      = "platform:admin"
)

func subjectFor(id string) string { return "platform.identityprovider." + id }
func groupFor(id string) string   { return "platform:identityprovider:" + id }

type IdentityProviderCreated struct {
	Metadata           usecase.EventMetadata
	IdentityProviderID string
	Code               string
}

func (e IdentityProviderCreated) EventID() string       { return e.Metadata.EventID }
func (e IdentityProviderCreated) EventType() string     { return IdentityProviderCreatedType }
func (e IdentityProviderCreated) SpecVersion() string   { return "1.0" }
func (e IdentityProviderCreated) Source() string        { return Source }
func (e IdentityProviderCreated) Subject() string       { return subjectFor(e.IdentityProviderID) }
func (e IdentityProviderCreated) Time() time.Time       { return e.Metadata.OccurredAt }
func (e IdentityProviderCreated) PrincipalID() string   { return e.Metadata.PrincipalID }
func (e IdentityProviderCreated) CorrelationID() string { return e.Metadata.CorrelationID }
func (e IdentityProviderCreated) CausationID() string   { return e.Metadata.CausationID }
func (e IdentityProviderCreated) ExecutionID() string   { return e.Metadata.ExecutionID }
func (e IdentityProviderCreated) MessageGroup() string  { return groupFor(e.IdentityProviderID) }
func (e IdentityProviderCreated) ToDataJSON() ([]byte, error) {
	return json.Marshal(struct {
		IdentityProviderID string `json:"identityProviderId"`
		Code               string `json:"code"`
	}{e.IdentityProviderID, e.Code})
}

type IdentityProviderUpdated struct {
	Metadata           usecase.EventMetadata
	IdentityProviderID string
	Code               string
}

func (e IdentityProviderUpdated) EventID() string       { return e.Metadata.EventID }
func (e IdentityProviderUpdated) EventType() string     { return IdentityProviderUpdatedType }
func (e IdentityProviderUpdated) SpecVersion() string   { return "1.0" }
func (e IdentityProviderUpdated) Source() string        { return Source }
func (e IdentityProviderUpdated) Subject() string       { return subjectFor(e.IdentityProviderID) }
func (e IdentityProviderUpdated) Time() time.Time       { return e.Metadata.OccurredAt }
func (e IdentityProviderUpdated) PrincipalID() string   { return e.Metadata.PrincipalID }
func (e IdentityProviderUpdated) CorrelationID() string { return e.Metadata.CorrelationID }
func (e IdentityProviderUpdated) CausationID() string   { return e.Metadata.CausationID }
func (e IdentityProviderUpdated) ExecutionID() string   { return e.Metadata.ExecutionID }
func (e IdentityProviderUpdated) MessageGroup() string  { return groupFor(e.IdentityProviderID) }
func (e IdentityProviderUpdated) ToDataJSON() ([]byte, error) {
	return json.Marshal(struct {
		IdentityProviderID string `json:"identityProviderId"`
		Code               string `json:"code"`
	}{e.IdentityProviderID, e.Code})
}

type IdentityProviderDeleted struct {
	Metadata           usecase.EventMetadata
	IdentityProviderID string
	Code               string
}

func (e IdentityProviderDeleted) EventID() string       { return e.Metadata.EventID }
func (e IdentityProviderDeleted) EventType() string     { return IdentityProviderDeletedType }
func (e IdentityProviderDeleted) SpecVersion() string   { return "1.0" }
func (e IdentityProviderDeleted) Source() string        { return Source }
func (e IdentityProviderDeleted) Subject() string       { return subjectFor(e.IdentityProviderID) }
func (e IdentityProviderDeleted) Time() time.Time       { return e.Metadata.OccurredAt }
func (e IdentityProviderDeleted) PrincipalID() string   { return e.Metadata.PrincipalID }
func (e IdentityProviderDeleted) CorrelationID() string { return e.Metadata.CorrelationID }
func (e IdentityProviderDeleted) CausationID() string   { return e.Metadata.CausationID }
func (e IdentityProviderDeleted) ExecutionID() string   { return e.Metadata.ExecutionID }
func (e IdentityProviderDeleted) MessageGroup() string  { return groupFor(e.IdentityProviderID) }
func (e IdentityProviderDeleted) ToDataJSON() ([]byte, error) {
	return json.Marshal(struct {
		IdentityProviderID string `json:"identityProviderId"`
		Code               string `json:"code"`
	}{e.IdentityProviderID, e.Code})
}
