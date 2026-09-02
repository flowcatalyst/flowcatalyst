// dto.go contains the wire-format types for the service_account API.
package api

import (
	"fmt"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/serviceaccount"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/serviceaccount/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httpcompat"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/jsontime"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
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

// toEntity converts the wire DTO to the entity, rejecting an unrecognised
// authType loudly (usecase.Validation → 400) rather than silently coercing
// it to NONE — a typo here would otherwise ship an unauthenticated webhook.
func (w WebhookCredentialsDTO) toEntity() (serviceaccount.WebhookCredentials, error) {
	authType, ok := serviceaccount.ParseAuthType(w.AuthType)
	if !ok {
		return serviceaccount.WebhookCredentials{}, usecase.Validation(
			"INVALID_AUTH_TYPE", fmt.Sprintf("unknown webhook auth type %q", w.AuthType))
	}
	return serviceaccount.WebhookCredentials{
		AuthType:         authType,
		Token:            w.Token,
		Username:         w.Username,
		Password:         w.Password,
		HeaderName:       w.HeaderName,
		SigningSecret:    w.SigningSecret,
		SigningAlgorithm: w.SigningAlgorithm,
		SignatureHeader:  w.SignatureHeader,
	}, nil
}

// RoleAssignmentDTO mirrors serviceaccount.RoleAssignment.
type RoleAssignmentDTO struct {
	Role             string          `json:"roleName"`
	ClientID         *string         `json:"clientId,omitempty"`
	AssignmentSource *string         `json:"assignmentSource,omitempty"`
	AssignedAt       httpcompat.Time `json:"assignedAt"`
	AssignedBy       *string         `json:"assignedBy,omitempty"`
}

// roleDTOs maps entity role assignments to wire rows.
func roleDTOs(roles []serviceaccount.RoleAssignment) []RoleAssignmentDTO {
	out := make([]RoleAssignmentDTO, 0, len(roles))
	for _, r := range roles {
		out = append(out, RoleAssignmentDTO{
			Role:             r.Role,
			ClientID:         r.ClientID,
			AssignmentSource: r.AssignmentSource,
			AssignedAt:       jsontime.New(r.AssignedAt),
			AssignedBy:       r.AssignedBy,
		})
	}
	return out
}

// CreateServiceAccountRequest is the wire body for POST /api/service-accounts.
type CreateServiceAccountRequest struct {
	Code               string                 `json:"code"`
	Name               string                 `json:"name"`
	Description        *string                `json:"description,omitempty"`
	Scope              *string                `json:"scope,omitempty"`
	ClientIDs          []string               `json:"clientIds,omitempty"`
	ApplicationID      *string                `json:"applicationId,omitempty"`
	WebhookCredentials *WebhookCredentialsDTO `json:"webhookCredentials,omitempty"`
}

func (r CreateServiceAccountRequest) toCommand() (operations.CreateCommand, error) {
	var creds *serviceaccount.WebhookCredentials
	if r.WebhookCredentials != nil {
		c, err := r.WebhookCredentials.toEntity()
		if err != nil {
			return operations.CreateCommand{}, err
		}
		creds = &c
	}
	return operations.CreateCommand{
		Code:               r.Code,
		Name:               r.Name,
		Description:        r.Description,
		Scope:              r.Scope,
		ClientIDs:          r.ClientIDs,
		ApplicationID:      r.ApplicationID,
		WebhookCredentials: creds,
	}, nil
}

// UpdateServiceAccountRequest is the wire body for PUT /api/service-accounts/{id}.
type UpdateServiceAccountRequest struct {
	Name               *string                `json:"name,omitempty"`
	Description        *string                `json:"description,omitempty"`
	Scope              *string                `json:"scope,omitempty"`
	ClientIDs          []string               `json:"clientIds,omitempty"`
	WebhookCredentials *WebhookCredentialsDTO `json:"webhookCredentials,omitempty"`
}

func (r UpdateServiceAccountRequest) toCommand(id string) (operations.UpdateCommand, error) {
	var creds *serviceaccount.WebhookCredentials
	if r.WebhookCredentials != nil {
		c, err := r.WebhookCredentials.toEntity()
		if err != nil {
			return operations.UpdateCommand{}, err
		}
		creds = &c
	}
	return operations.UpdateCommand{
		ID:                 id,
		Name:               r.Name,
		Description:        r.Description,
		Scope:              r.Scope,
		ClientIDs:          r.ClientIDs,
		WebhookCredentials: creds,
	}, nil
}

// AssignRolesRequest is the wire body for PUT /api/service-accounts/{id}/roles.
type AssignRolesRequest struct {
	Roles []string `json:"roles"`
}

// ServiceAccountOAuthSecrets carries the one-time OAuth client credentials.
type ServiceAccountOAuthSecrets struct {
	ClientID     string `json:"clientId"`
	ClientSecret string `json:"clientSecret"`
}

// ServiceAccountWebhookSecrets carries the one-time webhook credentials.
type ServiceAccountWebhookSecrets struct {
	AuthToken     string `json:"authToken"`
	SigningSecret string `json:"signingSecret"`
}

// CreateServiceAccountResponse is the 201 body for POST /api/service-accounts.
// The plaintext secrets are returned exactly once, at creation.
type CreateServiceAccountResponse struct {
	ServiceAccount ServiceAccountResponse       `json:"serviceAccount"`
	PrincipalID    string                       `json:"principalId"`
	OAuth          ServiceAccountOAuthSecrets   `json:"oauth"`
	Webhook        ServiceAccountWebhookSecrets `json:"webhook"`
}

// ServiceAccountResponse is the wire shape the SPA + fcsdk expect: flat,
// with `authType` hoisted out of the webhook credentials and `roles` as a
// plain name list. Webhook secrets (token/signingSecret/password) are NEVER
// exposed here — they are only returned once at create/regenerate time.
type ServiceAccountResponse struct {
	ID            string   `json:"id"`
	Code          string   `json:"code"`
	Name          string   `json:"name"`
	Description   *string  `json:"description,omitempty"`
	Active        bool     `json:"active"`
	ClientIDs     []string `json:"clientIds"`
	Scope         *string  `json:"scope,omitempty"`
	ApplicationID *string  `json:"applicationId,omitempty"`
	AuthType      string   `json:"authType"`
	Roles         []string `json:"roles"`
	// PrincipalID is the id of the linked SERVICE principal that actually owns
	// this account's roles and application access (a service account is not its
	// own principal row). Populated on the single-account read so the UI can
	// drive the shared /api/principals/{id}/application-access endpoints; omitted
	// from list responses to avoid a per-row lookup.
	PrincipalID *string          `json:"principalId,omitempty"`
	LastUsedAt  *httpcompat.Time `json:"lastUsedAt,omitempty"`
	CreatedAt   httpcompat.Time  `json:"createdAt"`
	UpdatedAt   httpcompat.Time  `json:"updatedAt"`
}

func fromEntity(sa *serviceaccount.ServiceAccount) ServiceAccountResponse {
	clientIDs := sa.ClientIDs
	if clientIDs == nil {
		clientIDs = []string{}
	}
	roles := make([]string, 0, len(sa.Roles))
	for _, r := range sa.Roles {
		roles = append(roles, r.Role)
	}
	var lastUsed *httpcompat.Time
	if sa.LastUsedAt != nil {
		v := jsontime.New(*sa.LastUsedAt)
		lastUsed = &v
	}
	return ServiceAccountResponse{
		ID:            sa.ID,
		Code:          sa.Code,
		Name:          sa.Name,
		Description:   sa.Description,
		Active:        sa.Active,
		ClientIDs:     clientIDs,
		Scope:         sa.Scope,
		ApplicationID: sa.ApplicationID,
		AuthType:      string(sa.WebhookCredentials.AuthType),
		Roles:         roles,
		LastUsedAt:    lastUsed,
		CreatedAt:     jsontime.New(sa.CreatedAt),
		UpdatedAt:     jsontime.New(sa.UpdatedAt),
	}
}

// ServiceAccountListResponse is the wire shape for GET /api/service-accounts:
// `{serviceAccounts, total}`. The SPA's
// ServiceAccountListPage reads `response.serviceAccounts` directly.
type ServiceAccountListResponse struct {
	ServiceAccounts []ServiceAccountResponse `json:"serviceAccounts"`
	Total           int                      `json:"total"`
}

// ServiceAccountRoleListResponse is the wire shape for
// GET /api/service-accounts/{id}/roles.
type ServiceAccountRoleListResponse struct {
	Roles []RoleAssignmentDTO `json:"roles"`
}

// ServiceAccountRolesAssignedResponse is the wire shape for
// PUT /api/service-accounts/{id}/roles. Note the SPA uses addedRoles /
// removedRoles here (distinct from the principal endpoint's added/removed).
type ServiceAccountRolesAssignedResponse struct {
	Roles        []RoleAssignmentDTO `json:"roles"`
	AddedRoles   []string            `json:"addedRoles"`
	RemovedRoles []string            `json:"removedRoles"`
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

// ServiceAccountTokenResponse is the wire shape for
// POST /api/service-accounts/{id}/token — an admin-minted bearer with the
// same claims a client_credentials exchange would yield for this account.
// The token is a live credential: shown once, never logged.
type ServiceAccountTokenResponse struct {
	AccessToken string  `json:"accessToken"`
	TokenType   string  `json:"tokenType"`
	ExpiresIn   int64   `json:"expiresIn"`
	Scope       *string `json:"scope,omitempty"`
}
