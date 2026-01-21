package serviceaccount

import (
	"time"

	"go.flowcatalyst.tech/internal/platform/principal"
)

// WebhookAuthType defines the authentication type for webhooks
type WebhookAuthType string

const (
	WebhookAuthTypeBearer WebhookAuthType = "BEARER"
	WebhookAuthTypeBasic  WebhookAuthType = "BASIC"
)

// SigningAlgorithm defines the signing algorithm for webhooks
type SigningAlgorithm string

const (
	SigningAlgorithmHMACSHA256 SigningAlgorithm = "HMAC_SHA256"
)

// ServiceAccount represents a service account for API access
// Collection: service_accounts
type ServiceAccount struct {
	ID                 string                        `bson:"_id" json:"id"`
	Code               string                        `bson:"code" json:"code"` // Unique code
	Name               string                        `bson:"name" json:"name"`
	Description        string                        `bson:"description,omitempty" json:"description,omitempty"`
	ClientIDs          []string                      `bson:"clientIds,omitempty" json:"clientIds,omitempty"`
	ApplicationID      string                        `bson:"applicationId,omitempty" json:"applicationId,omitempty"`
	Active             bool                          `bson:"active" json:"active"`
	WebhookCredentials *WebhookCredentials           `bson:"webhookCredentials,omitempty" json:"webhookCredentials,omitempty"`
	Roles              []principal.RoleAssignment    `bson:"roles,omitempty" json:"roles,omitempty"`
	LastUsedAt         time.Time                     `bson:"lastUsedAt,omitempty" json:"lastUsedAt,omitempty"`
	CreatedAt          time.Time                     `bson:"createdAt" json:"createdAt"`
	UpdatedAt          time.Time                     `bson:"updatedAt" json:"updatedAt"`
}

// WebhookCredentials holds credentials for webhook authentication
type WebhookCredentials struct {
	AuthType         WebhookAuthType  `bson:"authType" json:"authType"`
	AuthTokenRef     string           `bson:"authTokenRef,omitempty" json:"-"` // Secret reference
	SigningSecretRef string           `bson:"signingSecretRef,omitempty" json:"-"` // Secret reference
	SigningAlgorithm SigningAlgorithm `bson:"signingAlgorithm,omitempty" json:"signingAlgorithm,omitempty"`
	CreatedAt        time.Time        `bson:"createdAt" json:"createdAt"`
	RegeneratedAt    time.Time        `bson:"regeneratedAt,omitempty" json:"regeneratedAt,omitempty"`
}

// IsActive returns true if the service account is active
func (sa *ServiceAccount) IsActive() bool {
	return sa.Active
}

// HasWebhookCredentials returns true if webhook credentials are configured
func (sa *ServiceAccount) HasWebhookCredentials() bool {
	return sa.WebhookCredentials != nil
}

// HasRole checks if the service account has a specific role
func (sa *ServiceAccount) HasRole(roleName string) bool {
	for _, r := range sa.Roles {
		if r.RoleName == roleName {
			return true
		}
	}
	return false
}

// GetRoleNames returns all role names for this service account
func (sa *ServiceAccount) GetRoleNames() []string {
	names := make([]string, len(sa.Roles))
	for i, r := range sa.Roles {
		names[i] = r.RoleName
	}
	return names
}

// HasAccessToClient checks if the service account has access to a client
func (sa *ServiceAccount) HasAccessToClient(clientID string) bool {
	for _, id := range sa.ClientIDs {
		if id == clientID {
			return true
		}
	}
	return false
}
