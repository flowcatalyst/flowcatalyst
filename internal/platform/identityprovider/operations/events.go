package operations

import (
	"encoding/json"
	"time"

	"github.com/flowcatalyst/go-sdk/usecase"
)

const (
	IdentityProviderCreatedType = "platform:admin:identity-provider:created"
	IdentityProviderUpdatedType = "platform:admin:identity-provider:updated"
	IdentityProviderDeletedType = "platform:admin:identity-provider:deleted"

	CertificateGeneratedType = "platform:admin:identity-provider:certificate-generated"
	CertificatePromotedType  = "platform:admin:identity-provider:certificate-promoted"
	CertificateDiscardedType = "platform:admin:identity-provider:certificate-discarded"
	Source                   = "platform:admin"
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

// certEventBase carries the envelope accessors the three certificate events
// share (the same shape the provider events above spell out one by one).
type certEventBase struct {
	Metadata           usecase.EventMetadata
	eventType          string
	IdentityProviderID string
}

func (e certEventBase) EventID() string       { return e.Metadata.EventID }
func (e certEventBase) EventType() string     { return e.eventType }
func (e certEventBase) SpecVersion() string   { return "1.0" }
func (e certEventBase) Source() string        { return Source }
func (e certEventBase) Subject() string       { return subjectFor(e.IdentityProviderID) }
func (e certEventBase) Time() time.Time       { return e.Metadata.OccurredAt }
func (e certEventBase) PrincipalID() string   { return e.Metadata.PrincipalID }
func (e certEventBase) CorrelationID() string { return e.Metadata.CorrelationID }
func (e certEventBase) CausationID() string   { return e.Metadata.CausationID }
func (e certEventBase) ExecutionID() string   { return e.Metadata.ExecutionID }
func (e certEventBase) MessageGroup() string  { return groupFor(e.IdentityProviderID) }

// CertificateGenerated is emitted when a client-credential certificate is
// generated (ACTIVE when the provider had none, otherwise NEXT).
type CertificateGenerated struct {
	certEventBase
	Code           string
	CertificateID  string
	Status         string
	ThumbprintSHA1 string
	NotAfter       time.Time
}

func (e CertificateGenerated) ToDataJSON() ([]byte, error) {
	return json.Marshal(struct {
		IdentityProviderID string `json:"identityProviderId"`
		Code               string `json:"code"`
		CertificateID      string `json:"certificateId"`
		Status             string `json:"status"`
		ThumbprintSHA1     string `json:"thumbprintSha1"`
		NotAfter           string `json:"notAfter"`
	}{e.IdentityProviderID, e.Code, e.CertificateID, e.Status, e.ThumbprintSHA1, e.NotAfter.UTC().Format(time.RFC3339)})
}

// CertificatePromoted is emitted when the NEXT certificate became ACTIVE.
type CertificatePromoted struct {
	certEventBase
	Code                 string
	CertificateID        string
	RetiredCertificateID string
}

func (e CertificatePromoted) ToDataJSON() ([]byte, error) {
	return json.Marshal(struct {
		IdentityProviderID   string `json:"identityProviderId"`
		Code                 string `json:"code"`
		CertificateID        string `json:"certificateId"`
		RetiredCertificateID string `json:"retiredCertificateId,omitempty"`
	}{e.IdentityProviderID, e.Code, e.CertificateID, e.RetiredCertificateID})
}

// CertificateDiscarded is emitted when the NEXT certificate was deleted.
type CertificateDiscarded struct {
	certEventBase
	Code          string
	CertificateID string
}

func (e CertificateDiscarded) ToDataJSON() ([]byte, error) {
	return json.Marshal(struct {
		IdentityProviderID string `json:"identityProviderId"`
		Code               string `json:"code"`
		CertificateID      string `json:"certificateId"`
	}{e.IdentityProviderID, e.Code, e.CertificateID})
}
