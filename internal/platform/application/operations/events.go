package operations

import (
	"encoding/json"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
)

const (
	ApplicationCreatedType                 = "platform:iam:application:created"
	ApplicationUpdatedType                 = "platform:iam:application:updated"
	ApplicationActivatedType               = "platform:iam:application:activated"
	ApplicationDeactivatedType             = "platform:iam:application:deactivated"
	ApplicationDeletedType                 = "platform:iam:application:deleted"
	ApplicationServiceAccountProvisioned   = "platform:iam:application:service-account-provisioned"
	ApplicationEnabledForClientType        = "platform:iam:application:enabled-for-client"
	ApplicationDisabledForClientType       = "platform:iam:application:disabled-for-client"
	Source                                 = "platform:iam"
)

func subjectFor(id string) string { return "platform.application." + id }
func groupFor(id string) string   { return "platform:application:" + id }

type ApplicationCreated struct {
	Metadata      usecase.EventMetadata
	ApplicationID string
	Code          string
	Name          string
}

func (e ApplicationCreated) EventID() string       { return e.Metadata.EventID }
func (e ApplicationCreated) EventType() string     { return ApplicationCreatedType }
func (e ApplicationCreated) SpecVersion() string   { return "1.0" }
func (e ApplicationCreated) Source() string        { return Source }
func (e ApplicationCreated) Subject() string       { return subjectFor(e.ApplicationID) }
func (e ApplicationCreated) Time() time.Time       { return e.Metadata.OccurredAt }
func (e ApplicationCreated) PrincipalID() string   { return e.Metadata.PrincipalID }
func (e ApplicationCreated) CorrelationID() string { return e.Metadata.CorrelationID }
func (e ApplicationCreated) CausationID() string   { return e.Metadata.CausationID }
func (e ApplicationCreated) ExecutionID() string   { return e.Metadata.ExecutionID }
func (e ApplicationCreated) MessageGroup() string  { return groupFor(e.ApplicationID) }
func (e ApplicationCreated) ToDataJSON() ([]byte, error) {
	return json.Marshal(struct {
		ApplicationID string `json:"applicationId"`
		Code          string `json:"code"`
		Name          string `json:"name"`
	}{e.ApplicationID, e.Code, e.Name})
}

type ApplicationUpdated struct {
	Metadata      usecase.EventMetadata
	ApplicationID string
	Name          string
}

func (e ApplicationUpdated) EventID() string       { return e.Metadata.EventID }
func (e ApplicationUpdated) EventType() string     { return ApplicationUpdatedType }
func (e ApplicationUpdated) SpecVersion() string   { return "1.0" }
func (e ApplicationUpdated) Source() string        { return Source }
func (e ApplicationUpdated) Subject() string       { return subjectFor(e.ApplicationID) }
func (e ApplicationUpdated) Time() time.Time       { return e.Metadata.OccurredAt }
func (e ApplicationUpdated) PrincipalID() string   { return e.Metadata.PrincipalID }
func (e ApplicationUpdated) CorrelationID() string { return e.Metadata.CorrelationID }
func (e ApplicationUpdated) CausationID() string   { return e.Metadata.CausationID }
func (e ApplicationUpdated) ExecutionID() string   { return e.Metadata.ExecutionID }
func (e ApplicationUpdated) MessageGroup() string  { return groupFor(e.ApplicationID) }
func (e ApplicationUpdated) ToDataJSON() ([]byte, error) {
	return json.Marshal(struct {
		ApplicationID string `json:"applicationId"`
		Name          string `json:"name"`
	}{e.ApplicationID, e.Name})
}

type ApplicationActivated struct {
	Metadata      usecase.EventMetadata
	ApplicationID string
}

func (e ApplicationActivated) EventID() string       { return e.Metadata.EventID }
func (e ApplicationActivated) EventType() string     { return ApplicationActivatedType }
func (e ApplicationActivated) SpecVersion() string   { return "1.0" }
func (e ApplicationActivated) Source() string        { return Source }
func (e ApplicationActivated) Subject() string       { return subjectFor(e.ApplicationID) }
func (e ApplicationActivated) Time() time.Time       { return e.Metadata.OccurredAt }
func (e ApplicationActivated) PrincipalID() string   { return e.Metadata.PrincipalID }
func (e ApplicationActivated) CorrelationID() string { return e.Metadata.CorrelationID }
func (e ApplicationActivated) CausationID() string   { return e.Metadata.CausationID }
func (e ApplicationActivated) ExecutionID() string   { return e.Metadata.ExecutionID }
func (e ApplicationActivated) MessageGroup() string  { return groupFor(e.ApplicationID) }
func (e ApplicationActivated) ToDataJSON() ([]byte, error) {
	return json.Marshal(struct {
		ApplicationID string `json:"applicationId"`
	}{e.ApplicationID})
}

type ApplicationDeactivated struct {
	Metadata      usecase.EventMetadata
	ApplicationID string
}

func (e ApplicationDeactivated) EventID() string       { return e.Metadata.EventID }
func (e ApplicationDeactivated) EventType() string     { return ApplicationDeactivatedType }
func (e ApplicationDeactivated) SpecVersion() string   { return "1.0" }
func (e ApplicationDeactivated) Source() string        { return Source }
func (e ApplicationDeactivated) Subject() string       { return subjectFor(e.ApplicationID) }
func (e ApplicationDeactivated) Time() time.Time       { return e.Metadata.OccurredAt }
func (e ApplicationDeactivated) PrincipalID() string   { return e.Metadata.PrincipalID }
func (e ApplicationDeactivated) CorrelationID() string { return e.Metadata.CorrelationID }
func (e ApplicationDeactivated) CausationID() string   { return e.Metadata.CausationID }
func (e ApplicationDeactivated) ExecutionID() string   { return e.Metadata.ExecutionID }
func (e ApplicationDeactivated) MessageGroup() string  { return groupFor(e.ApplicationID) }
func (e ApplicationDeactivated) ToDataJSON() ([]byte, error) {
	return json.Marshal(struct {
		ApplicationID string `json:"applicationId"`
	}{e.ApplicationID})
}

type ApplicationDeleted struct {
	Metadata      usecase.EventMetadata
	ApplicationID string
	Code          string
}

func (e ApplicationDeleted) EventID() string       { return e.Metadata.EventID }
func (e ApplicationDeleted) EventType() string     { return ApplicationDeletedType }
func (e ApplicationDeleted) SpecVersion() string   { return "1.0" }
func (e ApplicationDeleted) Source() string        { return Source }
func (e ApplicationDeleted) Subject() string       { return subjectFor(e.ApplicationID) }
func (e ApplicationDeleted) Time() time.Time       { return e.Metadata.OccurredAt }
func (e ApplicationDeleted) PrincipalID() string   { return e.Metadata.PrincipalID }
func (e ApplicationDeleted) CorrelationID() string { return e.Metadata.CorrelationID }
func (e ApplicationDeleted) CausationID() string   { return e.Metadata.CausationID }
func (e ApplicationDeleted) ExecutionID() string   { return e.Metadata.ExecutionID }
func (e ApplicationDeleted) MessageGroup() string  { return groupFor(e.ApplicationID) }
func (e ApplicationDeleted) ToDataJSON() ([]byte, error) {
	return json.Marshal(struct {
		ApplicationID string `json:"applicationId"`
		Code          string `json:"code"`
	}{e.ApplicationID, e.Code})
}

// ApplicationServiceAccountProvisionedEvent — emitted when an
// application gets its dedicated service account attached.
type ApplicationServiceAccountProvisionedEvent struct {
	Metadata           usecase.EventMetadata
	ApplicationID      string
	ApplicationCode    string
	ServiceAccountID   string
	ServiceAccountCode string
}

func (e ApplicationServiceAccountProvisionedEvent) EventID() string {
	return e.Metadata.EventID
}
func (e ApplicationServiceAccountProvisionedEvent) EventType() string {
	return ApplicationServiceAccountProvisioned
}
func (e ApplicationServiceAccountProvisionedEvent) SpecVersion() string   { return "1.0" }
func (e ApplicationServiceAccountProvisionedEvent) Source() string        { return Source }
func (e ApplicationServiceAccountProvisionedEvent) Subject() string       { return subjectFor(e.ApplicationID) }
func (e ApplicationServiceAccountProvisionedEvent) Time() time.Time       { return e.Metadata.OccurredAt }
func (e ApplicationServiceAccountProvisionedEvent) PrincipalID() string   { return e.Metadata.PrincipalID }
func (e ApplicationServiceAccountProvisionedEvent) CorrelationID() string { return e.Metadata.CorrelationID }
func (e ApplicationServiceAccountProvisionedEvent) CausationID() string   { return e.Metadata.CausationID }
func (e ApplicationServiceAccountProvisionedEvent) ExecutionID() string   { return e.Metadata.ExecutionID }
func (e ApplicationServiceAccountProvisionedEvent) MessageGroup() string  { return groupFor(e.ApplicationID) }
func (e ApplicationServiceAccountProvisionedEvent) ToDataJSON() ([]byte, error) {
	return json.Marshal(struct {
		ApplicationID      string `json:"applicationId"`
		ApplicationCode    string `json:"applicationCode"`
		ServiceAccountID   string `json:"serviceAccountId"`
		ServiceAccountCode string `json:"serviceAccountCode"`
	}{e.ApplicationID, e.ApplicationCode, e.ServiceAccountID, e.ServiceAccountCode})
}

// ApplicationEnabledForClient — emitted on enable_for_client.
type ApplicationEnabledForClient struct {
	Metadata      usecase.EventMetadata
	ApplicationID string
	ClientID      string
	ConfigID      string
}

func (e ApplicationEnabledForClient) EventID() string       { return e.Metadata.EventID }
func (e ApplicationEnabledForClient) EventType() string     { return ApplicationEnabledForClientType }
func (e ApplicationEnabledForClient) SpecVersion() string   { return "1.0" }
func (e ApplicationEnabledForClient) Source() string        { return Source }
func (e ApplicationEnabledForClient) Subject() string       { return subjectFor(e.ApplicationID) }
func (e ApplicationEnabledForClient) Time() time.Time       { return e.Metadata.OccurredAt }
func (e ApplicationEnabledForClient) PrincipalID() string   { return e.Metadata.PrincipalID }
func (e ApplicationEnabledForClient) CorrelationID() string { return e.Metadata.CorrelationID }
func (e ApplicationEnabledForClient) CausationID() string   { return e.Metadata.CausationID }
func (e ApplicationEnabledForClient) ExecutionID() string   { return e.Metadata.ExecutionID }
func (e ApplicationEnabledForClient) MessageGroup() string  { return groupFor(e.ApplicationID) }
func (e ApplicationEnabledForClient) ToDataJSON() ([]byte, error) {
	return json.Marshal(struct {
		ApplicationID string `json:"applicationId"`
		ClientID      string `json:"clientId"`
		ConfigID      string `json:"configId"`
	}{e.ApplicationID, e.ClientID, e.ConfigID})
}

// ApplicationDisabledForClient — emitted on disable_for_client.
type ApplicationDisabledForClient struct {
	Metadata      usecase.EventMetadata
	ApplicationID string
	ClientID      string
	ConfigID      string
}

func (e ApplicationDisabledForClient) EventID() string       { return e.Metadata.EventID }
func (e ApplicationDisabledForClient) EventType() string     { return ApplicationDisabledForClientType }
func (e ApplicationDisabledForClient) SpecVersion() string   { return "1.0" }
func (e ApplicationDisabledForClient) Source() string        { return Source }
func (e ApplicationDisabledForClient) Subject() string       { return subjectFor(e.ApplicationID) }
func (e ApplicationDisabledForClient) Time() time.Time       { return e.Metadata.OccurredAt }
func (e ApplicationDisabledForClient) PrincipalID() string   { return e.Metadata.PrincipalID }
func (e ApplicationDisabledForClient) CorrelationID() string { return e.Metadata.CorrelationID }
func (e ApplicationDisabledForClient) CausationID() string   { return e.Metadata.CausationID }
func (e ApplicationDisabledForClient) ExecutionID() string   { return e.Metadata.ExecutionID }
func (e ApplicationDisabledForClient) MessageGroup() string  { return groupFor(e.ApplicationID) }
func (e ApplicationDisabledForClient) ToDataJSON() ([]byte, error) {
	return json.Marshal(struct {
		ApplicationID string `json:"applicationId"`
		ClientID      string `json:"clientId"`
		ConfigID      string `json:"configId"`
	}{e.ApplicationID, e.ClientID, e.ConfigID})
}
