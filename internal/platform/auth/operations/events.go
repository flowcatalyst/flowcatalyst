// Package operations holds the 13 auth subdomain admin use cases.
//
// Each event type wires DomainEvent the standard way. To keep the file
// terse, all events use the same Source + group/subject helpers.
package operations

import (
	"encoding/json"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
)

const Source = "platform:admin"

// ── event type constants ──────────────────────────────────────────────────

const (
	OAuthClientCreatedType       = "platform:admin:oauth-client:created"
	OAuthClientUpdatedType       = "platform:admin:oauth-client:updated"
	OAuthClientActivatedType     = "platform:admin:oauth-client:activated"
	OAuthClientDeactivatedType   = "platform:admin:oauth-client:deactivated"
	OAuthClientDeletedType       = "platform:admin:oauth-client:deleted"
	OAuthClientSecretRotatedType = "platform:admin:oauth-client:secret-rotated"

	AnchorDomainCreatedType = "platform:admin:anchor-domain:created"
	AnchorDomainUpdatedType = "platform:admin:anchor-domain:updated"
	AnchorDomainDeletedType = "platform:admin:anchor-domain:deleted"

	AuthConfigCreatedType = "platform:admin:auth-config:created"
	AuthConfigUpdatedType = "platform:admin:auth-config:updated"
	AuthConfigDeletedType = "platform:admin:auth-config:deleted"

	IdpRoleMappingCreatedType = "platform:admin:idp-role-mapping:created"
	IdpRoleMappingDeletedType = "platform:admin:idp-role-mapping:deleted"
)

func oauthSubject(id string) string   { return "platform.oauthclient." + id }
func oauthGroup(id string) string     { return "platform:oauthclient:" + id }
func anchorSubject(id string) string  { return "platform.anchordomain." + id }
func anchorGroup(id string) string    { return "platform:anchordomain:" + id }
func configSubject(id string) string  { return "platform.authconfig." + id }
func configGroup(id string) string    { return "platform:authconfig:" + id }
func mappingSubject(id string) string { return "platform.idprolemapping." + id }
func mappingGroup(id string) string   { return "platform:idprolemapping:" + id }

// ── OAuthClient events ────────────────────────────────────────────────────

// NewOAuthClientCreatedEvent builds the created event with the canonical
// subject. Exported for cross-aggregate orchestrations (application
// provision-service-account) that emit it inside their own transaction.
func NewOAuthClientCreatedEvent(ec usecase.ExecutionContext, oauthID, clientID, clientName string) OAuthClientCreated {
	return OAuthClientCreated{
		Metadata:      usecase.NewEventMetadata(ec, OAuthClientCreatedType, Source, oauthSubject(oauthID)),
		OAuthClientID: oauthID,
		ClientID:      clientID,
		ClientName:    clientName,
	}
}

type OAuthClientCreated struct {
	Metadata      usecase.EventMetadata
	OAuthClientID string
	ClientID      string
	ClientName    string
}

func (e OAuthClientCreated) EventID() string       { return e.Metadata.EventID }
func (e OAuthClientCreated) EventType() string     { return OAuthClientCreatedType }
func (e OAuthClientCreated) SpecVersion() string   { return "1.0" }
func (e OAuthClientCreated) Source() string        { return Source }
func (e OAuthClientCreated) Subject() string       { return oauthSubject(e.OAuthClientID) }
func (e OAuthClientCreated) Time() time.Time       { return e.Metadata.OccurredAt }
func (e OAuthClientCreated) PrincipalID() string   { return e.Metadata.PrincipalID }
func (e OAuthClientCreated) CorrelationID() string { return e.Metadata.CorrelationID }
func (e OAuthClientCreated) CausationID() string   { return e.Metadata.CausationID }
func (e OAuthClientCreated) ExecutionID() string   { return e.Metadata.ExecutionID }
func (e OAuthClientCreated) MessageGroup() string  { return oauthGroup(e.OAuthClientID) }
func (e OAuthClientCreated) ToDataJSON() ([]byte, error) {
	return json.Marshal(struct {
		ID         string `json:"oauthClientId"`
		ClientID   string `json:"clientId"`
		ClientName string `json:"clientName"`
	}{e.OAuthClientID, e.ClientID, e.ClientName})
}

type OAuthClientUpdated struct {
	Metadata      usecase.EventMetadata
	OAuthClientID string
	ClientName    string
}

func (e OAuthClientUpdated) EventID() string       { return e.Metadata.EventID }
func (e OAuthClientUpdated) EventType() string     { return OAuthClientUpdatedType }
func (e OAuthClientUpdated) SpecVersion() string   { return "1.0" }
func (e OAuthClientUpdated) Source() string        { return Source }
func (e OAuthClientUpdated) Subject() string       { return oauthSubject(e.OAuthClientID) }
func (e OAuthClientUpdated) Time() time.Time       { return e.Metadata.OccurredAt }
func (e OAuthClientUpdated) PrincipalID() string   { return e.Metadata.PrincipalID }
func (e OAuthClientUpdated) CorrelationID() string { return e.Metadata.CorrelationID }
func (e OAuthClientUpdated) CausationID() string   { return e.Metadata.CausationID }
func (e OAuthClientUpdated) ExecutionID() string   { return e.Metadata.ExecutionID }
func (e OAuthClientUpdated) MessageGroup() string  { return oauthGroup(e.OAuthClientID) }
func (e OAuthClientUpdated) ToDataJSON() ([]byte, error) {
	return json.Marshal(struct {
		ID         string `json:"oauthClientId"`
		ClientName string `json:"clientName"`
	}{e.OAuthClientID, e.ClientName})
}

type OAuthClientActivated struct {
	Metadata      usecase.EventMetadata
	OAuthClientID string
}

func (e OAuthClientActivated) EventID() string       { return e.Metadata.EventID }
func (e OAuthClientActivated) EventType() string     { return OAuthClientActivatedType }
func (e OAuthClientActivated) SpecVersion() string   { return "1.0" }
func (e OAuthClientActivated) Source() string        { return Source }
func (e OAuthClientActivated) Subject() string       { return oauthSubject(e.OAuthClientID) }
func (e OAuthClientActivated) Time() time.Time       { return e.Metadata.OccurredAt }
func (e OAuthClientActivated) PrincipalID() string   { return e.Metadata.PrincipalID }
func (e OAuthClientActivated) CorrelationID() string { return e.Metadata.CorrelationID }
func (e OAuthClientActivated) CausationID() string   { return e.Metadata.CausationID }
func (e OAuthClientActivated) ExecutionID() string   { return e.Metadata.ExecutionID }
func (e OAuthClientActivated) MessageGroup() string  { return oauthGroup(e.OAuthClientID) }
func (e OAuthClientActivated) ToDataJSON() ([]byte, error) {
	return json.Marshal(struct {
		ID string `json:"oauthClientId"`
	}{e.OAuthClientID})
}

type OAuthClientDeactivated struct {
	Metadata      usecase.EventMetadata
	OAuthClientID string
}

func (e OAuthClientDeactivated) EventID() string       { return e.Metadata.EventID }
func (e OAuthClientDeactivated) EventType() string     { return OAuthClientDeactivatedType }
func (e OAuthClientDeactivated) SpecVersion() string   { return "1.0" }
func (e OAuthClientDeactivated) Source() string        { return Source }
func (e OAuthClientDeactivated) Subject() string       { return oauthSubject(e.OAuthClientID) }
func (e OAuthClientDeactivated) Time() time.Time       { return e.Metadata.OccurredAt }
func (e OAuthClientDeactivated) PrincipalID() string   { return e.Metadata.PrincipalID }
func (e OAuthClientDeactivated) CorrelationID() string { return e.Metadata.CorrelationID }
func (e OAuthClientDeactivated) CausationID() string   { return e.Metadata.CausationID }
func (e OAuthClientDeactivated) ExecutionID() string   { return e.Metadata.ExecutionID }
func (e OAuthClientDeactivated) MessageGroup() string  { return oauthGroup(e.OAuthClientID) }
func (e OAuthClientDeactivated) ToDataJSON() ([]byte, error) {
	return json.Marshal(struct {
		ID string `json:"oauthClientId"`
	}{e.OAuthClientID})
}

type OAuthClientDeleted struct {
	Metadata      usecase.EventMetadata
	OAuthClientID string
	ClientID      string
}

func (e OAuthClientDeleted) EventID() string       { return e.Metadata.EventID }
func (e OAuthClientDeleted) EventType() string     { return OAuthClientDeletedType }
func (e OAuthClientDeleted) SpecVersion() string   { return "1.0" }
func (e OAuthClientDeleted) Source() string        { return Source }
func (e OAuthClientDeleted) Subject() string       { return oauthSubject(e.OAuthClientID) }
func (e OAuthClientDeleted) Time() time.Time       { return e.Metadata.OccurredAt }
func (e OAuthClientDeleted) PrincipalID() string   { return e.Metadata.PrincipalID }
func (e OAuthClientDeleted) CorrelationID() string { return e.Metadata.CorrelationID }
func (e OAuthClientDeleted) CausationID() string   { return e.Metadata.CausationID }
func (e OAuthClientDeleted) ExecutionID() string   { return e.Metadata.ExecutionID }
func (e OAuthClientDeleted) MessageGroup() string  { return oauthGroup(e.OAuthClientID) }
func (e OAuthClientDeleted) ToDataJSON() ([]byte, error) {
	return json.Marshal(struct {
		ID       string `json:"oauthClientId"`
		ClientID string `json:"clientId"`
	}{e.OAuthClientID, e.ClientID})
}

type OAuthClientSecretRotated struct {
	Metadata      usecase.EventMetadata
	OAuthClientID string
}

func (e OAuthClientSecretRotated) EventID() string       { return e.Metadata.EventID }
func (e OAuthClientSecretRotated) EventType() string     { return OAuthClientSecretRotatedType }
func (e OAuthClientSecretRotated) SpecVersion() string   { return "1.0" }
func (e OAuthClientSecretRotated) Source() string        { return Source }
func (e OAuthClientSecretRotated) Subject() string       { return oauthSubject(e.OAuthClientID) }
func (e OAuthClientSecretRotated) Time() time.Time       { return e.Metadata.OccurredAt }
func (e OAuthClientSecretRotated) PrincipalID() string   { return e.Metadata.PrincipalID }
func (e OAuthClientSecretRotated) CorrelationID() string { return e.Metadata.CorrelationID }
func (e OAuthClientSecretRotated) CausationID() string   { return e.Metadata.CausationID }
func (e OAuthClientSecretRotated) ExecutionID() string   { return e.Metadata.ExecutionID }
func (e OAuthClientSecretRotated) MessageGroup() string  { return oauthGroup(e.OAuthClientID) }
func (e OAuthClientSecretRotated) ToDataJSON() ([]byte, error) {
	return json.Marshal(struct {
		ID string `json:"oauthClientId"`
	}{e.OAuthClientID})
}

// ── AnchorDomain events ───────────────────────────────────────────────────

type AnchorDomainCreated struct {
	Metadata       usecase.EventMetadata
	AnchorDomainID string
	Domain         string
}

func (e AnchorDomainCreated) EventID() string       { return e.Metadata.EventID }
func (e AnchorDomainCreated) EventType() string     { return AnchorDomainCreatedType }
func (e AnchorDomainCreated) SpecVersion() string   { return "1.0" }
func (e AnchorDomainCreated) Source() string        { return Source }
func (e AnchorDomainCreated) Subject() string       { return anchorSubject(e.AnchorDomainID) }
func (e AnchorDomainCreated) Time() time.Time       { return e.Metadata.OccurredAt }
func (e AnchorDomainCreated) PrincipalID() string   { return e.Metadata.PrincipalID }
func (e AnchorDomainCreated) CorrelationID() string { return e.Metadata.CorrelationID }
func (e AnchorDomainCreated) CausationID() string   { return e.Metadata.CausationID }
func (e AnchorDomainCreated) ExecutionID() string   { return e.Metadata.ExecutionID }
func (e AnchorDomainCreated) MessageGroup() string  { return anchorGroup(e.AnchorDomainID) }
func (e AnchorDomainCreated) ToDataJSON() ([]byte, error) {
	return json.Marshal(struct {
		ID     string `json:"anchorDomainId"`
		Domain string `json:"domain"`
	}{e.AnchorDomainID, e.Domain})
}

type AnchorDomainUpdated struct {
	Metadata       usecase.EventMetadata
	AnchorDomainID string
	Domain         string
}

func (e AnchorDomainUpdated) EventID() string       { return e.Metadata.EventID }
func (e AnchorDomainUpdated) EventType() string     { return AnchorDomainUpdatedType }
func (e AnchorDomainUpdated) SpecVersion() string   { return "1.0" }
func (e AnchorDomainUpdated) Source() string        { return Source }
func (e AnchorDomainUpdated) Subject() string       { return anchorSubject(e.AnchorDomainID) }
func (e AnchorDomainUpdated) Time() time.Time       { return e.Metadata.OccurredAt }
func (e AnchorDomainUpdated) PrincipalID() string   { return e.Metadata.PrincipalID }
func (e AnchorDomainUpdated) CorrelationID() string { return e.Metadata.CorrelationID }
func (e AnchorDomainUpdated) CausationID() string   { return e.Metadata.CausationID }
func (e AnchorDomainUpdated) ExecutionID() string   { return e.Metadata.ExecutionID }
func (e AnchorDomainUpdated) MessageGroup() string  { return anchorGroup(e.AnchorDomainID) }
func (e AnchorDomainUpdated) ToDataJSON() ([]byte, error) {
	return json.Marshal(struct {
		ID     string `json:"anchorDomainId"`
		Domain string `json:"domain"`
	}{e.AnchorDomainID, e.Domain})
}

type AnchorDomainDeleted struct {
	Metadata       usecase.EventMetadata
	AnchorDomainID string
	Domain         string
}

func (e AnchorDomainDeleted) EventID() string       { return e.Metadata.EventID }
func (e AnchorDomainDeleted) EventType() string     { return AnchorDomainDeletedType }
func (e AnchorDomainDeleted) SpecVersion() string   { return "1.0" }
func (e AnchorDomainDeleted) Source() string        { return Source }
func (e AnchorDomainDeleted) Subject() string       { return anchorSubject(e.AnchorDomainID) }
func (e AnchorDomainDeleted) Time() time.Time       { return e.Metadata.OccurredAt }
func (e AnchorDomainDeleted) PrincipalID() string   { return e.Metadata.PrincipalID }
func (e AnchorDomainDeleted) CorrelationID() string { return e.Metadata.CorrelationID }
func (e AnchorDomainDeleted) CausationID() string   { return e.Metadata.CausationID }
func (e AnchorDomainDeleted) ExecutionID() string   { return e.Metadata.ExecutionID }
func (e AnchorDomainDeleted) MessageGroup() string  { return anchorGroup(e.AnchorDomainID) }
func (e AnchorDomainDeleted) ToDataJSON() ([]byte, error) {
	return json.Marshal(struct {
		ID     string `json:"anchorDomainId"`
		Domain string `json:"domain"`
	}{e.AnchorDomainID, e.Domain})
}

// ── AuthConfig events ─────────────────────────────────────────────────────

type AuthConfigCreated struct {
	Metadata     usecase.EventMetadata
	AuthConfigID string
	EmailDomain  string
}

func (e AuthConfigCreated) EventID() string       { return e.Metadata.EventID }
func (e AuthConfigCreated) EventType() string     { return AuthConfigCreatedType }
func (e AuthConfigCreated) SpecVersion() string   { return "1.0" }
func (e AuthConfigCreated) Source() string        { return Source }
func (e AuthConfigCreated) Subject() string       { return configSubject(e.AuthConfigID) }
func (e AuthConfigCreated) Time() time.Time       { return e.Metadata.OccurredAt }
func (e AuthConfigCreated) PrincipalID() string   { return e.Metadata.PrincipalID }
func (e AuthConfigCreated) CorrelationID() string { return e.Metadata.CorrelationID }
func (e AuthConfigCreated) CausationID() string   { return e.Metadata.CausationID }
func (e AuthConfigCreated) ExecutionID() string   { return e.Metadata.ExecutionID }
func (e AuthConfigCreated) MessageGroup() string  { return configGroup(e.AuthConfigID) }
func (e AuthConfigCreated) ToDataJSON() ([]byte, error) {
	return json.Marshal(struct {
		ID          string `json:"authConfigId"`
		EmailDomain string `json:"emailDomain"`
	}{e.AuthConfigID, e.EmailDomain})
}

type AuthConfigUpdated struct {
	Metadata     usecase.EventMetadata
	AuthConfigID string
	EmailDomain  string
}

func (e AuthConfigUpdated) EventID() string       { return e.Metadata.EventID }
func (e AuthConfigUpdated) EventType() string     { return AuthConfigUpdatedType }
func (e AuthConfigUpdated) SpecVersion() string   { return "1.0" }
func (e AuthConfigUpdated) Source() string        { return Source }
func (e AuthConfigUpdated) Subject() string       { return configSubject(e.AuthConfigID) }
func (e AuthConfigUpdated) Time() time.Time       { return e.Metadata.OccurredAt }
func (e AuthConfigUpdated) PrincipalID() string   { return e.Metadata.PrincipalID }
func (e AuthConfigUpdated) CorrelationID() string { return e.Metadata.CorrelationID }
func (e AuthConfigUpdated) CausationID() string   { return e.Metadata.CausationID }
func (e AuthConfigUpdated) ExecutionID() string   { return e.Metadata.ExecutionID }
func (e AuthConfigUpdated) MessageGroup() string  { return configGroup(e.AuthConfigID) }
func (e AuthConfigUpdated) ToDataJSON() ([]byte, error) {
	return json.Marshal(struct {
		ID          string `json:"authConfigId"`
		EmailDomain string `json:"emailDomain"`
	}{e.AuthConfigID, e.EmailDomain})
}

type AuthConfigDeleted struct {
	Metadata     usecase.EventMetadata
	AuthConfigID string
	EmailDomain  string
}

func (e AuthConfigDeleted) EventID() string       { return e.Metadata.EventID }
func (e AuthConfigDeleted) EventType() string     { return AuthConfigDeletedType }
func (e AuthConfigDeleted) SpecVersion() string   { return "1.0" }
func (e AuthConfigDeleted) Source() string        { return Source }
func (e AuthConfigDeleted) Subject() string       { return configSubject(e.AuthConfigID) }
func (e AuthConfigDeleted) Time() time.Time       { return e.Metadata.OccurredAt }
func (e AuthConfigDeleted) PrincipalID() string   { return e.Metadata.PrincipalID }
func (e AuthConfigDeleted) CorrelationID() string { return e.Metadata.CorrelationID }
func (e AuthConfigDeleted) CausationID() string   { return e.Metadata.CausationID }
func (e AuthConfigDeleted) ExecutionID() string   { return e.Metadata.ExecutionID }
func (e AuthConfigDeleted) MessageGroup() string  { return configGroup(e.AuthConfigID) }
func (e AuthConfigDeleted) ToDataJSON() ([]byte, error) {
	return json.Marshal(struct {
		ID          string `json:"authConfigId"`
		EmailDomain string `json:"emailDomain"`
	}{e.AuthConfigID, e.EmailDomain})
}

// ── IdpRoleMapping events ─────────────────────────────────────────────────

type IdpRoleMappingCreated struct {
	Metadata         usecase.EventMetadata
	MappingID        string
	IdpType          string
	IdpRoleName      string
	PlatformRoleName string
}

func (e IdpRoleMappingCreated) EventID() string       { return e.Metadata.EventID }
func (e IdpRoleMappingCreated) EventType() string     { return IdpRoleMappingCreatedType }
func (e IdpRoleMappingCreated) SpecVersion() string   { return "1.0" }
func (e IdpRoleMappingCreated) Source() string        { return Source }
func (e IdpRoleMappingCreated) Subject() string       { return mappingSubject(e.MappingID) }
func (e IdpRoleMappingCreated) Time() time.Time       { return e.Metadata.OccurredAt }
func (e IdpRoleMappingCreated) PrincipalID() string   { return e.Metadata.PrincipalID }
func (e IdpRoleMappingCreated) CorrelationID() string { return e.Metadata.CorrelationID }
func (e IdpRoleMappingCreated) CausationID() string   { return e.Metadata.CausationID }
func (e IdpRoleMappingCreated) ExecutionID() string   { return e.Metadata.ExecutionID }
func (e IdpRoleMappingCreated) MessageGroup() string  { return mappingGroup(e.MappingID) }
func (e IdpRoleMappingCreated) ToDataJSON() ([]byte, error) {
	return json.Marshal(struct {
		ID               string `json:"mappingId"`
		IdpType          string `json:"idpType"`
		IdpRoleName      string `json:"idpRoleName"`
		PlatformRoleName string `json:"platformRoleName"`
	}{e.MappingID, e.IdpType, e.IdpRoleName, e.PlatformRoleName})
}

type IdpRoleMappingDeleted struct {
	Metadata    usecase.EventMetadata
	MappingID   string
	IdpRoleName string
}

func (e IdpRoleMappingDeleted) EventID() string       { return e.Metadata.EventID }
func (e IdpRoleMappingDeleted) EventType() string     { return IdpRoleMappingDeletedType }
func (e IdpRoleMappingDeleted) SpecVersion() string   { return "1.0" }
func (e IdpRoleMappingDeleted) Source() string        { return Source }
func (e IdpRoleMappingDeleted) Subject() string       { return mappingSubject(e.MappingID) }
func (e IdpRoleMappingDeleted) Time() time.Time       { return e.Metadata.OccurredAt }
func (e IdpRoleMappingDeleted) PrincipalID() string   { return e.Metadata.PrincipalID }
func (e IdpRoleMappingDeleted) CorrelationID() string { return e.Metadata.CorrelationID }
func (e IdpRoleMappingDeleted) CausationID() string   { return e.Metadata.CausationID }
func (e IdpRoleMappingDeleted) ExecutionID() string   { return e.Metadata.ExecutionID }
func (e IdpRoleMappingDeleted) MessageGroup() string  { return mappingGroup(e.MappingID) }
func (e IdpRoleMappingDeleted) ToDataJSON() ([]byte, error) {
	return json.Marshal(struct {
		ID          string `json:"mappingId"`
		IdpRoleName string `json:"idpRoleName"`
	}{e.MappingID, e.IdpRoleName})
}
