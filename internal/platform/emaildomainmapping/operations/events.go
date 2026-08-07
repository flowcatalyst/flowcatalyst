package operations

import (
	"encoding/json"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
)

const (
	EmailDomainMappingCreatedType         = "platform:admin:email-domain-mapping:created"
	EmailDomainMappingUpdatedType         = "platform:admin:email-domain-mapping:updated"
	EmailDomainMappingDeletedType         = "platform:admin:email-domain-mapping:deleted"
	EmailDomainMappingProviderChangedType = "platform:admin:email-domain-mapping:provider-changed"
	Source                                = "platform:admin"
)

func subjectFor(id string) string { return "platform.emaildomainmapping." + id }
func groupFor(id string) string   { return "platform:emaildomainmapping:" + id }

// EmailDomainMappingCreated is emitted on create.
type EmailDomainMappingCreated struct {
	Metadata    usecase.EventMetadata
	MappingID   string
	EmailDomain string
}

func (e EmailDomainMappingCreated) EventID() string       { return e.Metadata.EventID }
func (e EmailDomainMappingCreated) EventType() string     { return EmailDomainMappingCreatedType }
func (e EmailDomainMappingCreated) SpecVersion() string   { return "1.0" }
func (e EmailDomainMappingCreated) Source() string        { return Source }
func (e EmailDomainMappingCreated) Subject() string       { return subjectFor(e.MappingID) }
func (e EmailDomainMappingCreated) Time() time.Time       { return e.Metadata.OccurredAt }
func (e EmailDomainMappingCreated) PrincipalID() string   { return e.Metadata.PrincipalID }
func (e EmailDomainMappingCreated) CorrelationID() string { return e.Metadata.CorrelationID }
func (e EmailDomainMappingCreated) CausationID() string   { return e.Metadata.CausationID }
func (e EmailDomainMappingCreated) ExecutionID() string   { return e.Metadata.ExecutionID }
func (e EmailDomainMappingCreated) MessageGroup() string  { return groupFor(e.MappingID) }
func (e EmailDomainMappingCreated) ToDataJSON() ([]byte, error) {
	return json.Marshal(struct {
		MappingID   string `json:"mappingId"`
		EmailDomain string `json:"emailDomain"`
	}{e.MappingID, e.EmailDomain})
}

// NewMappingCreatedEvent builds the created event. Exported for the
// identity-provider orchestration ops, which create mappings for the domains
// listed on a provider inside their own transaction.
func NewMappingCreatedEvent(ec usecase.ExecutionContext, mappingID, emailDomain string) EmailDomainMappingCreated {
	return EmailDomainMappingCreated{
		Metadata:    usecase.NewEventMetadata(ec, EmailDomainMappingCreatedType, Source, subjectFor(mappingID)),
		MappingID:   mappingID,
		EmailDomain: emailDomain,
	}
}

// EmailDomainMappingUpdated is emitted on update.
type EmailDomainMappingUpdated struct {
	Metadata    usecase.EventMetadata
	MappingID   string
	EmailDomain string
}

func (e EmailDomainMappingUpdated) EventID() string       { return e.Metadata.EventID }
func (e EmailDomainMappingUpdated) EventType() string     { return EmailDomainMappingUpdatedType }
func (e EmailDomainMappingUpdated) SpecVersion() string   { return "1.0" }
func (e EmailDomainMappingUpdated) Source() string        { return Source }
func (e EmailDomainMappingUpdated) Subject() string       { return subjectFor(e.MappingID) }
func (e EmailDomainMappingUpdated) Time() time.Time       { return e.Metadata.OccurredAt }
func (e EmailDomainMappingUpdated) PrincipalID() string   { return e.Metadata.PrincipalID }
func (e EmailDomainMappingUpdated) CorrelationID() string { return e.Metadata.CorrelationID }
func (e EmailDomainMappingUpdated) CausationID() string   { return e.Metadata.CausationID }
func (e EmailDomainMappingUpdated) ExecutionID() string   { return e.Metadata.ExecutionID }
func (e EmailDomainMappingUpdated) MessageGroup() string  { return groupFor(e.MappingID) }
func (e EmailDomainMappingUpdated) ToDataJSON() ([]byte, error) {
	return json.Marshal(struct {
		MappingID   string `json:"mappingId"`
		EmailDomain string `json:"emailDomain"`
	}{e.MappingID, e.EmailDomain})
}

// EmailDomainMappingProviderChanged is emitted when a domain is re-pointed to
// a different identity provider (the move use case, or an IdP claiming /
// releasing the domain via the IdP orchestration ops).
type EmailDomainMappingProviderChanged struct {
	Metadata               usecase.EventMetadata
	MappingID              string
	EmailDomain            string
	FromIdentityProviderID string
	ToIdentityProviderID   string
}

func (e EmailDomainMappingProviderChanged) EventID() string { return e.Metadata.EventID }
func (e EmailDomainMappingProviderChanged) EventType() string {
	return EmailDomainMappingProviderChangedType
}
func (e EmailDomainMappingProviderChanged) SpecVersion() string { return "1.0" }
func (e EmailDomainMappingProviderChanged) Source() string      { return Source }
func (e EmailDomainMappingProviderChanged) Subject() string     { return subjectFor(e.MappingID) }
func (e EmailDomainMappingProviderChanged) Time() time.Time     { return e.Metadata.OccurredAt }
func (e EmailDomainMappingProviderChanged) PrincipalID() string { return e.Metadata.PrincipalID }
func (e EmailDomainMappingProviderChanged) CorrelationID() string {
	return e.Metadata.CorrelationID
}
func (e EmailDomainMappingProviderChanged) CausationID() string  { return e.Metadata.CausationID }
func (e EmailDomainMappingProviderChanged) ExecutionID() string  { return e.Metadata.ExecutionID }
func (e EmailDomainMappingProviderChanged) MessageGroup() string { return groupFor(e.MappingID) }
func (e EmailDomainMappingProviderChanged) ToDataJSON() ([]byte, error) {
	return json.Marshal(struct {
		MappingID              string `json:"mappingId"`
		EmailDomain            string `json:"emailDomain"`
		FromIdentityProviderID string `json:"fromIdentityProviderId"`
		ToIdentityProviderID   string `json:"toIdentityProviderId"`
	}{e.MappingID, e.EmailDomain, e.FromIdentityProviderID, e.ToIdentityProviderID})
}

// EmailDomainMappingDeleted is emitted on delete.
type EmailDomainMappingDeleted struct {
	Metadata    usecase.EventMetadata
	MappingID   string
	EmailDomain string
}

func (e EmailDomainMappingDeleted) EventID() string       { return e.Metadata.EventID }
func (e EmailDomainMappingDeleted) EventType() string     { return EmailDomainMappingDeletedType }
func (e EmailDomainMappingDeleted) SpecVersion() string   { return "1.0" }
func (e EmailDomainMappingDeleted) Source() string        { return Source }
func (e EmailDomainMappingDeleted) Subject() string       { return subjectFor(e.MappingID) }
func (e EmailDomainMappingDeleted) Time() time.Time       { return e.Metadata.OccurredAt }
func (e EmailDomainMappingDeleted) PrincipalID() string   { return e.Metadata.PrincipalID }
func (e EmailDomainMappingDeleted) CorrelationID() string { return e.Metadata.CorrelationID }
func (e EmailDomainMappingDeleted) CausationID() string   { return e.Metadata.CausationID }
func (e EmailDomainMappingDeleted) ExecutionID() string   { return e.Metadata.ExecutionID }
func (e EmailDomainMappingDeleted) MessageGroup() string  { return groupFor(e.MappingID) }
func (e EmailDomainMappingDeleted) ToDataJSON() ([]byte, error) {
	return json.Marshal(struct {
		MappingID   string `json:"mappingId"`
		EmailDomain string `json:"emailDomain"`
	}{e.MappingID, e.EmailDomain})
}
