// dto.go contains the wire-format types for the principal API.
package api

import (
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httpcompat"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/jsontime"
)

// CreatePrincipalRequest is the wire body for POST /api/principals.
type CreatePrincipalRequest struct {
	Email    string  `json:"email"`
	Name     *string `json:"name,omitempty"`
	Scope    string  `json:"scope" doc:"Principal scope (ANCHOR, PARTNER, CLIENT)"`
	ClientID *string `json:"clientId,omitempty"`
	Password *string `json:"password,omitempty"`
	IDPType  *string `json:"idpType,omitempty"`
}

func (r CreatePrincipalRequest) toCommand() operations.CreateCommand {
	return operations.CreateCommand{
		Email:    r.Email,
		Name:     r.Name,
		Scope:    r.Scope,
		ClientID: r.ClientID,
		Password: r.Password,
		IDPType:  r.IDPType,
	}
}

// UpdatePrincipalRequest is the wire body for PUT /api/principals/{id}.
type UpdatePrincipalRequest struct {
	Name      *string `json:"name,omitempty"`
	FirstName *string `json:"firstName,omitempty"`
	LastName  *string `json:"lastName,omitempty"`
	Phone     *string `json:"phone,omitempty"`
}

func (r UpdatePrincipalRequest) toCommand(id string) operations.UpdateCommand {
	return operations.UpdateCommand{
		ID:        id,
		Name:      r.Name,
		FirstName: r.FirstName,
		LastName:  r.LastName,
		Phone:     r.Phone,
	}
}

// ResetPasswordRequest is the wire body for POST /api/principals/{id}/reset-password.
type ResetPasswordRequest struct {
	NewPassword string `json:"newPassword"`
}

// AssignPrincipalRolesRequest is the wire body for PUT /api/principals/{id}/roles.
type AssignPrincipalRolesRequest struct {
	Roles []string `json:"roles"`
}

// AssignApplicationAccessRequest is the wire body for
// PUT /api/principals/{id}/application-access.
type AssignApplicationAccessRequest struct {
	ApplicationIDs []string `json:"applicationIds"`
}

// GrantClientAccessRequest is the wire body for
// POST /api/principals/{id}/client-access.
type GrantClientAccessRequest struct {
	ClientID string `json:"clientId"`
}

// UserIdentityDTO mirrors principal.UserIdentity.
type UserIdentityDTO struct {
	Email         string           `json:"email"`
	EmailVerified bool             `json:"emailVerified"`
	FirstName     *string          `json:"firstName,omitempty"`
	LastName      *string          `json:"lastName,omitempty"`
	PictureURL    *string          `json:"pictureUrl,omitempty"`
	Phone         *string          `json:"phone,omitempty"`
	ExternalID    *string          `json:"externalId,omitempty"`
	Provider      *string          `json:"provider,omitempty"`
	PasswordHash  *string          `json:"passwordHash,omitempty"`
	LastLoginAt   *httpcompat.Time `json:"lastLoginAt,omitempty"`
}

func userIdentityFromEntity(u *principal.UserIdentity) *UserIdentityDTO {
	if u == nil {
		return nil
	}
	var lastLogin *httpcompat.Time
	if u.LastLoginAt != nil {
		v := jsontime.New(*u.LastLoginAt)
		lastLogin = &v
	}
	return &UserIdentityDTO{
		Email:         u.Email,
		EmailVerified: u.EmailVerified,
		FirstName:     u.FirstName,
		LastName:      u.LastName,
		PictureURL:    u.PictureURL,
		Phone:         u.Phone,
		ExternalID:    u.ExternalID,
		Provider:      u.Provider,
		PasswordHash:  u.PasswordHash,
		LastLoginAt:   lastLogin,
	}
}

// ExternalIdentityDTO mirrors principal.ExternalIdentity.
type ExternalIdentityDTO struct {
	ProviderID string `json:"providerId"`
	ExternalID string `json:"externalId"`
}

// PrincipalRoleAssignmentDTO mirrors serviceaccount.RoleAssignment.
type PrincipalRoleAssignmentDTO struct {
	Role             string          `json:"roleName"`
	ClientID         *string         `json:"clientId,omitempty"`
	AssignmentSource *string         `json:"assignmentSource,omitempty"`
	AssignedAt       httpcompat.Time `json:"assignedAt"`
	AssignedBy       *string         `json:"assignedBy,omitempty"`
}

// PrincipalResponse mirrors principal.Principal.
type PrincipalResponse struct {
	ID                       string               `json:"id"`
	Type                     string               `json:"type"`
	Scope                    string               `json:"scope"`
	ClientID                 *string              `json:"clientId,omitempty"`
	ApplicationID            *string              `json:"applicationId,omitempty"`
	Name                     string               `json:"name"`
	Active                   bool                 `json:"active"`
	UserIdentity             *UserIdentityDTO     `json:"userIdentity,omitempty"`
	ServiceAccountID         *string              `json:"serviceAccountId,omitempty"`
	Roles                    []PrincipalRoleAssignmentDTO  `json:"roles"`
	AssignedClients          []string             `json:"assignedClients"`
	ClientIdentifierMap      map[string]string    `json:"clientIdentifierMap,omitempty"`
	AccessibleApplicationIDs []string             `json:"accessibleApplicationIds"`
	ExternalIdentity         *ExternalIdentityDTO `json:"externalIdentity,omitempty"`
	CreatedAt                httpcompat.Time      `json:"createdAt"`
	UpdatedAt                httpcompat.Time      `json:"updatedAt"`
}

func fromEntity(p *principal.Principal) PrincipalResponse {
	roles := make([]PrincipalRoleAssignmentDTO, 0, len(p.Roles))
	for _, r := range p.Roles {
		roles = append(roles, PrincipalRoleAssignmentDTO{
			Role:             r.Role,
			ClientID:         r.ClientID,
			AssignmentSource: r.AssignmentSource,
			AssignedAt:       jsontime.New(r.AssignedAt),
			AssignedBy:       r.AssignedBy,
		})
	}
	assigned := p.AssignedClients
	if assigned == nil {
		assigned = []string{}
	}
	apps := p.AccessibleApplicationIDs
	if apps == nil {
		apps = []string{}
	}
	var ext *ExternalIdentityDTO
	if p.ExternalIdentity != nil {
		ext = &ExternalIdentityDTO{
			ProviderID: p.ExternalIdentity.ProviderID,
			ExternalID: p.ExternalIdentity.ExternalID,
		}
	}
	return PrincipalResponse{
		ID:                       p.ID,
		Type:                     string(p.Type),
		Scope:                    string(p.Scope),
		ClientID:                 p.ClientID,
		ApplicationID:            p.ApplicationID,
		Name:                     p.Name,
		Active:                   p.Active,
		UserIdentity:             userIdentityFromEntity(p.UserIdentity),
		ServiceAccountID:         p.ServiceAccountID,
		Roles:                    roles,
		AssignedClients:          assigned,
		ClientIdentifierMap:      p.ClientIdentifierMap,
		AccessibleApplicationIDs: apps,
		ExternalIdentity:         ext,
		CreatedAt:                jsontime.New(p.CreatedAt),
		UpdatedAt:                jsontime.New(p.UpdatedAt),
	}
}

// PrincipalListResponse is the wire shape for GET /api/principals.
type PrincipalListResponse struct {
	Items []PrincipalResponse `json:"items"`
}

// ClientAccessGrantResponse mirrors principal.ClientAccessGrant.
type ClientAccessGrantResponse struct {
	ID          string          `json:"id"`
	PrincipalID string          `json:"principalId"`
	ClientID    string          `json:"clientId"`
	GrantedBy   string          `json:"grantedBy"`
	GrantedAt   httpcompat.Time `json:"grantedAt"`
}

func clientAccessGrantFromEntity(g *principal.ClientAccessGrant) ClientAccessGrantResponse {
	return ClientAccessGrantResponse{
		ID:          g.ID,
		PrincipalID: g.PrincipalID,
		ClientID:    g.ClientID,
		GrantedBy:   g.GrantedBy,
		GrantedAt:   jsontime.New(g.GrantedAt),
	}
}

// ClientAccessGrantListResponse is the wire shape for
// GET /api/principals/{id}/client-access.
type ClientAccessGrantListResponse struct {
	Items []ClientAccessGrantResponse `json:"items"`
}
