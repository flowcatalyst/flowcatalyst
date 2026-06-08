// dto.go contains the wire-format types for the principal API.
package api

import (
	"fmt"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/serviceaccount"
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

// CreateUserRequest is the wire body for POST /api/principals/users — the
// SDK/Rust create-user shape. Unlike CreatePrincipalRequest it carries NO
// scope: scope + client association are derived from the email domain
// (anchor-domain check + email-domain-mapping). Mirrors Rust CreateUserRequest.
type CreateUserRequest struct {
	Email                     string  `json:"email"`
	Name                      string  `json:"name"`
	Password                  *string `json:"password,omitempty"`
	ClientID                  *string `json:"clientId,omitempty"`
	EnforcePasswordComplexity *bool   `json:"enforcePasswordComplexity,omitempty"`
}

// UpdatePrincipalRequest is the wire body for PUT /api/principals/{id}. It
// carries only a principal's display + status: the full name (`name`), active
// status (`active`), and `email` as a stable identity assertion. Scope and
// client association are deliberately NOT here — those are sensitive,
// anchor-gated changes served by the /client-access sub-resource (see
// GrantClientAccessRequest's `mode`). A principal in FlowCatalyst is, in
// practice, a full name + email + status, so that's all this endpoint mutates.
type UpdatePrincipalRequest struct {
	Name   *string `json:"name,omitempty"`
	Active *bool   `json:"active,omitempty"`
	Email  *string `json:"email,omitempty" doc:"Optional; asserted against the stored email — a different value is rejected, not treated as a rename"`
}

func (r UpdatePrincipalRequest) toCommand(id string) operations.UpdateCommand {
	return operations.UpdateCommand{
		ID:     id,
		Name:   r.Name,
		Active: r.Active,
		Email:  r.Email,
	}
}

// ResetPasswordRequest is the wire body for POST /api/principals/{id}/reset-password.
type ResetPasswordRequest struct {
	NewPassword string `json:"newPassword"`
	// EnforcePasswordComplexity mirrors the Rust field of the same name (default
	// true). When false, the caller (e.g. an SDK consumer that applies its own
	// policy) opts out of the platform's password rules; Go relaxes the minimum
	// length to match Rust's relaxed() policy. This field MUST exist on the DTO:
	// huma generates schemas with additionalProperties:false, so without it the
	// SDK's body ({newPassword, enforcePasswordComplexity}) is rejected with a
	// "validation failed" 400. Go does not implement the upper/lower/digit/special
	// complexity checks (consistent with create-user, which also only accepts the
	// flag), so enforce=true keeps just the 8-char minimum.
	EnforcePasswordComplexity *bool `json:"enforcePasswordComplexity,omitempty"`
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

// ClientAssociationRequest is the wire body for
// PUT /api/principals/{id}/client-association. It changes a principal's scope +
// client with explicit intent. clientId "*" makes the principal an ANCHOR; for a
// specific clientId, mode disambiguates: CHANGE_CLIENT replaces the home client
// (stays CLIENT scope), TO_PARTNER promotes to PARTNER keeping the old client and
// adding the new one. Anchor-gated.
type ClientAssociationRequest struct {
	ClientID string  `json:"clientId" doc:"Target client id, or \"*\" for anchor (all-client) access"`
	Mode     *string `json:"mode,omitempty" doc:"CHANGE_CLIENT | TO_PARTNER — required for a specific clientId, ignored for \"*\""`
}

// PrincipalResponse is the wire shape for a principal. It is intentionally
// flat (matching the Rust platform + fcsdk client + SPA): email/idpType are
// hoisted out of the identity, roles is a plain name list, and the password
// hash is never exposed. Richer per-assignment data is served by the
// dedicated /roles and /client-access sub-resources.
type PrincipalResponse struct {
	ID               string          `json:"id"`
	Type             string          `json:"type"`
	Scope            string          `json:"scope"`
	ClientID         *string         `json:"clientId,omitempty"`
	Name             string          `json:"name"`
	Active           bool            `json:"active"`
	Email            *string         `json:"email,omitempty"`
	IdpType          *string         `json:"idpType,omitempty"`
	Roles            []string        `json:"roles"`
	IsAnchorUser     bool            `json:"isAnchorUser"`
	GrantedClientIDs []string        `json:"grantedClientIds"`
	CreatedAt        httpcompat.Time `json:"createdAt"`
	UpdatedAt        httpcompat.Time `json:"updatedAt"`
}

func fromEntity(p *principal.Principal) PrincipalResponse {
	var email, idpType *string
	if p.UserIdentity != nil {
		e := p.UserIdentity.Email
		email = &e
		// Report the actual stored provider (INTERNAL / OIDC). The Rust source
		// hardcoded "INTERNAL" here, which mislabels OIDC-linked users in the
		// admin UI; we surface the real value from the principal's identity
		// (idp_type column → UserIdentity.Provider).
		if p.UserIdentity.Provider != nil && *p.UserIdentity.Provider != "" {
			idpType = p.UserIdentity.Provider
		} else {
			internal := "INTERNAL"
			idpType = &internal
		}
	}
	roles := make([]string, 0, len(p.Roles))
	for _, r := range p.Roles {
		roles = append(roles, r.Role)
	}
	granted := p.AssignedClients
	if granted == nil {
		granted = []string{}
	}
	return PrincipalResponse{
		ID:               p.ID,
		Type:             string(p.Type),
		Scope:            string(p.Scope),
		ClientID:         p.ClientID,
		Name:             p.Name,
		Active:           p.Active,
		Email:            email,
		IdpType:          idpType,
		Roles:            roles,
		IsAnchorUser:     p.Scope.IsAnchor(),
		GrantedClientIDs: granted,
		CreatedAt:        jsontime.New(p.CreatedAt),
		UpdatedAt:        jsontime.New(p.UpdatedAt),
	}
}

// PrincipalListResponse is the wire shape for GET /api/principals.
// Matches the Rust shape: `{principals, total}` rather than the
// platform's generic `{items}` envelope. The SPA's UserListPage reads
// `response.principals` + `response.total` directly.
type PrincipalListResponse struct {
	Principals []PrincipalResponse `json:"principals"`
	Total      int                 `json:"total"`
}

// ClientAccessGrantResponse is the wire shape for a single client-access
// grant. Matches the Rust platform + fcsdk client + SPA.
type ClientAccessGrantResponse struct {
	ID        string           `json:"id"`
	ClientID  string           `json:"clientId"`
	GrantedAt httpcompat.Time  `json:"grantedAt"`
	ExpiresAt *httpcompat.Time `json:"expiresAt,omitempty"`
}

func clientAccessGrantFromEntity(g *principal.ClientAccessGrant) ClientAccessGrantResponse {
	return ClientAccessGrantResponse{
		ID:        g.ID,
		ClientID:  g.ClientID,
		GrantedAt: jsontime.New(g.GrantedAt),
	}
}

// ClientAccessGrantListResponse is the wire shape for
// GET /api/principals/{id}/client-access.
type ClientAccessGrantListResponse struct {
	Grants []ClientAccessGrantResponse `json:"grants"`
}

// CheckEmailDomainResponse is the 200 body for
// GET /api/principals/check-email-domain. It is the SUPERSET of two shapes:
//
//   - The slim auth fields (authMethod/loginUrl/idpIssuer) — retained for
//     back-compat. Note the login page does NOT use this endpoint; it checks
//     domains via the separate POST /auth/check-domain handler, so nothing
//     here drives login.
//   - The rich fields the SPA's UserCreatePage needs to preview the new user's
//     derived scope, decide whether a clientId must be supplied, and constrain
//     the client picker. `info`/`warning` are display strings, nullable
//     (serialized as JSON null when absent) to match the SPA's `string | null`.
type CheckEmailDomainResponse struct {
	AuthMethod string `json:"authMethod"` // "internal" | "external"
	LoginURL   string `json:"loginUrl,omitempty"`
	IDPIssuer  string `json:"idpIssuer,omitempty"`

	Domain           string   `json:"domain"`
	AuthProvider     string   `json:"authProvider"` // IdP type: "INTERNAL" | "OIDC" | "SAML"
	IsAnchorDomain   bool     `json:"isAnchorDomain"`
	HasIDPConfig     bool     `json:"hasIdpConfig"`
	EmailExists      bool     `json:"emailExists"`
	Info             *string  `json:"info"`         // display hint; null when none
	Warning          *string  `json:"warning"`      // display warning; null when none
	DerivedScope     string   `json:"derivedScope"` // "ANCHOR" | "PARTNER" | "CLIENT"
	RequiresClientID bool     `json:"requiresClientId"`
	AllowedClientIDs []string `json:"allowedClientIds"`
}

// PrincipalRoleAssignmentDTO is a single role assignment row. Matches the Rust
// RoleAssignmentDto + fcsdk PrincipalRoleResponse + SPA RoleAssignment.
type PrincipalRoleAssignmentDTO struct {
	ID               string          `json:"id"`
	RoleName         string          `json:"roleName"`
	AssignmentSource string          `json:"assignmentSource"`
	AssignedAt       httpcompat.Time `json:"assignedAt"`
}

// roleAssignmentDTOs builds the wire rows for a principal's roles. The id is
// synthetic (principals don't store a per-assignment id), matching Rust's
// "{principalID}-role-{i}" scheme so the SPA has a stable :key.
func roleAssignmentDTOs(principalID string, roles []serviceaccount.RoleAssignment) []PrincipalRoleAssignmentDTO {
	out := make([]PrincipalRoleAssignmentDTO, 0, len(roles))
	for i, r := range roles {
		source := "ADMIN"
		if r.AssignmentSource != nil {
			source = *r.AssignmentSource
		}
		out = append(out, PrincipalRoleAssignmentDTO{
			ID:               fmt.Sprintf("%s-role-%d", principalID, i),
			RoleName:         r.Role,
			AssignmentSource: source,
			AssignedAt:       jsontime.New(r.AssignedAt),
		})
	}
	return out
}

// PrincipalRoleListResponse is the wire shape for
// GET /api/principals/{id}/roles.
type PrincipalRoleListResponse struct {
	Roles []PrincipalRoleAssignmentDTO `json:"roles"`
}

// RolesAssignedResponse is the wire shape for PUT /api/principals/{id}/roles.
type RolesAssignedResponse struct {
	Roles   []PrincipalRoleAssignmentDTO `json:"roles"`
	Added   []string                     `json:"added"`
	Removed []string                     `json:"removed"`
}

// AddRoleRequest is the body for POST /api/principals/{id}/roles.
type AddRoleRequest struct {
	Role string `json:"role"`
}

// ApplicationAccessResponse is a single application-access row.
type ApplicationAccessResponse struct {
	ApplicationID   string `json:"applicationId"`
	ApplicationCode string `json:"applicationCode"`
	ApplicationName string `json:"applicationName"`
}

// ApplicationAccessListResponse is the wire shape for
// GET /api/principals/{id}/application-access.
type ApplicationAccessListResponse struct {
	Applications []ApplicationAccessResponse `json:"applications"`
	Total        int                         `json:"total"`
}

// SetApplicationAccessResponse is the wire shape for
// PUT /api/principals/{id}/application-access.
type SetApplicationAccessResponse struct {
	Applications []ApplicationAccessResponse `json:"applications"`
	Added        int                         `json:"added"`
	Removed      int                         `json:"removed"`
}

// PrincipalAvailableApplication is a row in the available-apps list.
type PrincipalAvailableApplication struct {
	ID   string `json:"id"`
	Code string `json:"code"`
	Name string `json:"name"`
}

// PrincipalAvailableApplicationsResponse is the wire shape for
// GET /api/principals/{id}/available-applications.
type PrincipalAvailableApplicationsResponse struct {
	Applications []PrincipalAvailableApplication `json:"applications"`
}

// BulkImportRequest is the body of POST /api/principals/bulk-import: a list of
// users to onboard under a single client. Each missing user is created as a
// CLIENT-scope user with the listed roles (validated against the client's
// applications); existing users are skipped.
type BulkImportRequest struct {
	ClientID string           `json:"clientId" doc:"Client all imported users are created under"`
	Users    []BulkImportUser `json:"users"`
}

// BulkImportUser is one CSV row: full name, email, and the roles to assign.
type BulkImportUser struct {
	Name  string   `json:"name"`
	Email string   `json:"email"`
	Roles []string `json:"roles,omitempty" doc:"Role names to assign (pipe-separated in the CSV)"`
}

// BulkImportResult is the per-row outcome.
type BulkImportResult struct {
	Row     int    `json:"row"`
	Email   string `json:"email"`
	Status  string `json:"status" doc:"created | exists | error"`
	Message string `json:"message,omitempty"`
}

// BulkImportResponse summarises a bulk import.
type BulkImportResponse struct {
	Created int                `json:"created"`
	Skipped int                `json:"skipped"`
	Failed  int                `json:"failed"`
	Results []BulkImportResult `json:"results"`
}
