// Package serviceaccount implements machine-to-machine principals with
// webhook credentials and roles.
//
// TODO(wave-3c-follow-up): assign_roles, regenerate_secret, regenerate_token
// ops — they have tight coupling with principal/role and are best ported
// alongside those subdomains.
package serviceaccount

import (
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/tsid"
)

// WebhookAuthType is the authentication scheme for outbound webhooks.
type WebhookAuthType string

const (
	AuthNone   WebhookAuthType = "NONE"
	AuthBearer WebhookAuthType = "BEARER_TOKEN"
	AuthBasic  WebhookAuthType = "BASIC_AUTH"
	AuthAPIKey WebhookAuthType = "API_KEY"
	AuthHMAC   WebhookAuthType = "HMAC_SIGNATURE"
)

// ParseAuthType parses a wire-format auth type string into a
// WebhookAuthType. Empty string maps to AuthNone — "no auth type given"
// means "no credentials", matching NoCredentials() — but any other
// unrecognised value returns ok=false. Callers MUST reject on ok=false
// rather than coerce it to NONE: a typo silently downgrading a webhook to
// unauthenticated is the bug this parser exists to prevent. Follows the
// (T, bool) shape of common.ParseOutboxItemType.
func ParseAuthType(s string) (WebhookAuthType, bool) {
	switch WebhookAuthType(s) {
	case "":
		return AuthNone, true
	case AuthNone, AuthBearer, AuthBasic, AuthAPIKey, AuthHMAC:
		return WebhookAuthType(s), true
	default:
		return "", false
	}
}

// WebhookCredentials carries the per-account auth details used by outbound calls.
type WebhookCredentials struct {
	AuthType         WebhookAuthType `json:"authType"`
	Token            *string         `json:"token,omitempty"`
	Username         *string         `json:"username,omitempty"`
	Password         *string         `json:"password,omitempty"`
	HeaderName       *string         `json:"headerName,omitempty"`
	SigningSecret    *string         `json:"signingSecret,omitempty"`
	SigningAlgorithm *string         `json:"signingAlgorithm,omitempty"`
	SignatureHeader  *string         `json:"signatureHeader,omitempty"`
}

// NoCredentials returns a credentials value with AuthType=NONE.
func NoCredentials() WebhookCredentials {
	return WebhookCredentials{AuthType: AuthNone}
}

// RoleAssignment is a role granted to this account (optionally scoped to a client).
type RoleAssignment struct {
	Role             string    `json:"roleName"`
	ClientID         *string   `json:"clientId,omitempty"`
	AssignmentSource *string   `json:"assignmentSource,omitempty"`
	AssignedAt       time.Time `json:"assignedAt"`
	AssignedBy       *string   `json:"assignedBy,omitempty"`
}

// ServiceAccount is the aggregate root.
type ServiceAccount struct {
	ID                    string             `json:"id"`
	Code                  string             `json:"code"`
	Name                  string             `json:"name"`
	Description           *string            `json:"description,omitempty"`
	Active                bool               `json:"active"`
	ClientIDs             []string           `json:"clientIds"`
	Scope                 *string            `json:"scope,omitempty"`
	ApplicationID         *string            `json:"applicationId,omitempty"`
	WebhookCredentials    WebhookCredentials `json:"webhookCredentials"`
	ServiceAccountTableID *string            `json:"-"`
	Roles                 []RoleAssignment   `json:"roles"`
	LastUsedAt            *time.Time         `json:"lastUsedAt,omitempty"`
	CreatedAt             time.Time          `json:"createdAt"`
	UpdatedAt             time.Time          `json:"updatedAt"`
}

// IDStr satisfies usecase.HasID.
func (s ServiceAccount) IDStr() string { return s.ID }

// New constructs a ServiceAccount.
func New(code, name string) *ServiceAccount {
	now := time.Now().UTC()
	return &ServiceAccount{
		ID:                 tsid.Generate(tsid.ServiceAccount),
		Code:               code,
		Name:               name,
		Active:             true,
		ClientIDs:          []string{},
		WebhookCredentials: NoCredentials(),
		Roles:              []RoleAssignment{},
		CreatedAt:          now,
		UpdatedAt:          now,
	}
}

// Deactivate flips Active=false and bumps UpdatedAt.
func (s *ServiceAccount) Deactivate() {
	s.Active = false
	s.UpdatedAt = time.Now().UTC()
}

// Activate flips Active=true and bumps UpdatedAt.
func (s *ServiceAccount) Activate() {
	s.Active = true
	s.UpdatedAt = time.Now().UTC()
}
