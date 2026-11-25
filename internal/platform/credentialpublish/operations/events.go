package operations

import (
	"encoding/json"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/credentialpublish"
	"github.com/flowcatalyst/go-sdk/usecase"
)

const (
	CredentialPublishingEnabledType  = "platform:iam:credential-publishing:enabled"
	CredentialPublishingDisabledType = "platform:iam:credential-publishing:disabled"
	Source                           = "platform:iam"
)

func subjectFor(o credentialpublish.Owner) string {
	return "platform.credentialpublishing." + string(o.Type) + "." + o.ID
}

func groupFor(o credentialpublish.Owner) string {
	return "platform:credentialpublishing:" + string(o.Type) + ":" + o.ID
}

// CredentialPublishingEnabled records who switched publishing on for an owner,
// and where the parameters go. The credentials' values are never in it.
type CredentialPublishingEnabled struct {
	Metadata  usecase.EventMetadata
	OwnerType string
	OwnerID   string
	BasePath  string
}

func (e CredentialPublishingEnabled) owner() credentialpublish.Owner {
	return credentialpublish.Owner{Type: credentialpublish.OwnerType(e.OwnerType), ID: e.OwnerID}
}
func (e CredentialPublishingEnabled) EventID() string       { return e.Metadata.EventID }
func (e CredentialPublishingEnabled) EventType() string     { return CredentialPublishingEnabledType }
func (e CredentialPublishingEnabled) SpecVersion() string   { return "1.0" }
func (e CredentialPublishingEnabled) Source() string        { return Source }
func (e CredentialPublishingEnabled) Subject() string       { return subjectFor(e.owner()) }
func (e CredentialPublishingEnabled) Time() time.Time       { return e.Metadata.OccurredAt }
func (e CredentialPublishingEnabled) PrincipalID() string   { return e.Metadata.PrincipalID }
func (e CredentialPublishingEnabled) CorrelationID() string { return e.Metadata.CorrelationID }
func (e CredentialPublishingEnabled) CausationID() string   { return e.Metadata.CausationID }
func (e CredentialPublishingEnabled) ExecutionID() string   { return e.Metadata.ExecutionID }
func (e CredentialPublishingEnabled) MessageGroup() string  { return groupFor(e.owner()) }
func (e CredentialPublishingEnabled) ToDataJSON() ([]byte, error) {
	return json.Marshal(struct {
		OwnerType string `json:"ownerType"`
		OwnerID   string `json:"ownerId"`
		BasePath  string `json:"basePath"`
	}{e.OwnerType, e.OwnerID, e.BasePath})
}

// CredentialPublishingDisabled records that publishing was switched off and
// which parameters were queued for deletion.
type CredentialPublishingDisabled struct {
	Metadata  usecase.EventMetadata
	OwnerType string
	OwnerID   string
	Paths     []string
}

func (e CredentialPublishingDisabled) owner() credentialpublish.Owner {
	return credentialpublish.Owner{Type: credentialpublish.OwnerType(e.OwnerType), ID: e.OwnerID}
}
func (e CredentialPublishingDisabled) EventID() string       { return e.Metadata.EventID }
func (e CredentialPublishingDisabled) EventType() string     { return CredentialPublishingDisabledType }
func (e CredentialPublishingDisabled) SpecVersion() string   { return "1.0" }
func (e CredentialPublishingDisabled) Source() string        { return Source }
func (e CredentialPublishingDisabled) Subject() string       { return subjectFor(e.owner()) }
func (e CredentialPublishingDisabled) Time() time.Time       { return e.Metadata.OccurredAt }
func (e CredentialPublishingDisabled) PrincipalID() string   { return e.Metadata.PrincipalID }
func (e CredentialPublishingDisabled) CorrelationID() string { return e.Metadata.CorrelationID }
func (e CredentialPublishingDisabled) CausationID() string   { return e.Metadata.CausationID }
func (e CredentialPublishingDisabled) ExecutionID() string   { return e.Metadata.ExecutionID }
func (e CredentialPublishingDisabled) MessageGroup() string  { return groupFor(e.owner()) }
func (e CredentialPublishingDisabled) ToDataJSON() ([]byte, error) {
	paths := e.Paths
	if paths == nil {
		paths = []string{}
	}
	return json.Marshal(struct {
		OwnerType string   `json:"ownerType"`
		OwnerID   string   `json:"ownerId"`
		Paths     []string `json:"paths"`
	}{e.OwnerType, e.OwnerID, paths})
}
