// dto.go contains the wire-format types for the service_account API.
package api

import (
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/serviceaccount"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/serviceaccount/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httpcompat"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/jsontime"
)

// WebhookCredentialsDTO mirrors serviceaccount.WebhookCredentials.
type WebhookCredentialsDTO struct {
	AuthType         string  `json:"authType"`
	Token            *string `json:"token,omitempty"`
	Username         *string `json:"username,omitempty"`
	Password         *string `json:"password,omitempty"`
	HeaderName       *string `json:"headerName,omitempty"`
	SigningSecret    *string `json:"signingSecret,omitempty"`
	SigningAlgorithm *string `json:"signingAlgorithm,omitempty"`
	SignatureHeader  *string `json:"signatureHeader,omitempty"`
}

func (w WebhookCredentialsDTO) toEntity() serviceaccount.WebhookCredentials {
	return serviceaccount.WebhookCredentials{
		AuthType:         serviceaccount.ParseAuthType(w.AuthType),
		Token:            w.Token,
		Username:         w.Username,
		Password:         w.Password,
		HeaderName:       w.HeaderName,
		SigningSecret:    w.SigningSecret,
		SigningAlgorithm: w.SigningAlgorithm,
		SignatureHeader:  w.SignatureHeader,
	}
}

func webhookCredsFromEntity(c serviceaccount.WebhookCredentials) WebhookCredentialsDTO {
	return WebhookCredentialsDTO{
		AuthType:         string(c.AuthType),
		Token:            c.Token,
		Username:         c.Username,
		Password:         c.Password,
		HeaderName:       c.HeaderName,
		SigningSecret:    c.SigningSecret,
		SigningAlgorithm: c.SigningAlgorithm,
		SignatureHeader:  c.SignatureHeader,
	}
}

// RoleAssignmentDTO mirrors serviceaccount.RoleAssignment.
type RoleAssignmentDTO struct {
	Role             string          `json:"roleName"`
	ClientID         *string         `json:"clientId,omitempty"`
	AssignmentSource *string         `json:"assignmentSource,omitempty"`
	AssignedAt       httpcompat.Time `json:"assignedAt"`
	AssignedBy       *string         `json:"assignedBy,omitempty"`
}

// CreateServiceAccountRequest is the wire body for POST /api/service-accounts.
type CreateServiceAccountRequest struct {
	Code               string                 `json:"code"`
	Name               string                 `json:"name"`
	Description        *string                `json:"description,omitempty"`
	Scope              *string                `json:"scope,omitempty"`
	ApplicationID      *string                `json:"applicationId,omitempty"`
	WebhookCredentials *WebhookCredentialsDTO `json:"webhookCredentials,omitempty"`
}

func (r CreateServiceAccountRequest) toCommand() operations.CreateCommand {
	var creds *serviceaccount.WebhookCredentials
	if r.WebhookCredentials != nil {
		c := r.WebhookCredentials.toEntity()
		creds = &c
	}
	return operations.CreateCommand{
		Code:               r.Code,
		Name:               r.Name,
		Description:        r.Description,
		Scope:              r.Scope,
		ApplicationID:      r.ApplicationID,
		WebhookCredentials: creds,
	}
}

// UpdateServiceAccountRequest is the wire body for PUT /api/service-accounts/{id}.
type UpdateServiceAccountRequest struct {
	Name               *string                `json:"name,omitempty"`
	Description        *string                `json:"description,omitempty"`
	WebhookCredentials *WebhookCredentialsDTO `json:"webhookCredentials,omitempty"`
}

func (r UpdateServiceAccountRequest) toCommand(id string) operations.UpdateCommand {
	var creds *serviceaccount.WebhookCredentials
	if r.WebhookCredentials != nil {
		c := r.WebhookCredentials.toEntity()
		creds = &c
	}
	return operations.UpdateCommand{
		ID:                 id,
		Name:               r.Name,
		Description:        r.Description,
		WebhookCredentials: creds,
	}
}

// AssignRolesRequest is the wire body for PUT /api/service-accounts/{id}/roles.
type AssignRolesRequest struct {
	Roles []string `json:"roles"`
}

// ServiceAccountResponse mirrors serviceaccount.ServiceAccount.
type ServiceAccountResponse struct {
	ID                 string                `json:"id"`
	Code               string                `json:"code"`
	Name               string                `json:"name"`
	Description        *string               `json:"description,omitempty"`
	Active             bool                  `json:"active"`
	ClientIDs          []string              `json:"clientIds"`
	Scope              *string               `json:"scope,omitempty"`
	ApplicationID      *string               `json:"applicationId,omitempty"`
	WebhookCredentials WebhookCredentialsDTO `json:"webhookCredentials"`
	Roles              []RoleAssignmentDTO   `json:"roles"`
	LastUsedAt         *httpcompat.Time      `json:"lastUsedAt,omitempty"`
	CreatedAt          httpcompat.Time       `json:"createdAt"`
	UpdatedAt          httpcompat.Time       `json:"updatedAt"`
}

func fromEntity(sa *serviceaccount.ServiceAccount) ServiceAccountResponse {
	clientIDs := sa.ClientIDs
	if clientIDs == nil {
		clientIDs = []string{}
	}
	roles := make([]RoleAssignmentDTO, 0, len(sa.Roles))
	for _, r := range sa.Roles {
		roles = append(roles, RoleAssignmentDTO{
			Role:             r.Role,
			ClientID:         r.ClientID,
			AssignmentSource: r.AssignmentSource,
			AssignedAt:       jsontime.New(r.AssignedAt),
			AssignedBy:       r.AssignedBy,
		})
	}
	var lastUsed *httpcompat.Time
	if sa.LastUsedAt != nil {
		v := jsontime.New(*sa.LastUsedAt)
		lastUsed = &v
	}
	return ServiceAccountResponse{
		ID:                 sa.ID,
		Code:               sa.Code,
		Name:               sa.Name,
		Description:        sa.Description,
		Active:             sa.Active,
		ClientIDs:          clientIDs,
		Scope:              sa.Scope,
		ApplicationID:      sa.ApplicationID,
		WebhookCredentials: webhookCredsFromEntity(sa.WebhookCredentials),
		Roles:              roles,
		LastUsedAt:         lastUsed,
		CreatedAt:          jsontime.New(sa.CreatedAt),
		UpdatedAt:          jsontime.New(sa.UpdatedAt),
	}
}

// ServiceAccountListResponse is the wire shape for GET /api/service-accounts.
type ServiceAccountListResponse struct {
	Items []ServiceAccountResponse `json:"items"`
}

// ServiceAccountRoleListResponse is the wire shape for
// GET /api/service-accounts/{id}/roles.
type ServiceAccountRoleListResponse struct {
	Items []RoleAssignmentDTO `json:"items"`
}

// RegenerateAuthTokenResponse is the wire shape for
// POST /api/service-accounts/{id}/regenerate-auth-token. The plaintext
// auth token is only emitted once on regeneration.
type RegenerateAuthTokenResponse struct {
	ID        string `json:"id"`
	AuthToken string `json:"authToken,omitempty"`
}

// RegenerateSigningSecretResponse is the wire shape for
// POST /api/service-accounts/{id}/regenerate-signing-secret.
type RegenerateSigningSecretResponse struct {
	ID            string `json:"id"`
	SigningSecret string `json:"signingSecret,omitempty"`
}
