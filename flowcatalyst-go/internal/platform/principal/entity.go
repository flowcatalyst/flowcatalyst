package principal

import (
	"time"
)

// PrincipalType defines the type of principal
type PrincipalType string

const (
	PrincipalTypeUser    PrincipalType = "USER"
	PrincipalTypeService PrincipalType = "SERVICE"
)

// UserScope defines the access scope for a user
type UserScope string

const (
	UserScopeAnchor  UserScope = "ANCHOR"  // Platform admin, access to all clients
	UserScopePartner UserScope = "PARTNER" // Partner user, access to assigned clients
	UserScopeClient  UserScope = "CLIENT"  // Bound to single client
)

// IdpType defines the identity provider type
type IdpType string

const (
	IdpTypeInternal IdpType = "INTERNAL" // Local password auth
	IdpTypeExternal IdpType = "EXTERNAL" // External IDP (OIDC, SAML, etc.)
	IdpTypeOIDC     IdpType = "OIDC"     // External OIDC provider (alias for EXTERNAL)
)

// Principal represents a user or service account
// Collection: auth_principals
type Principal struct {
	ID            string           `bson:"_id" json:"id"`
	Type          PrincipalType    `bson:"type" json:"type"`
	Scope         UserScope        `bson:"scope" json:"scope"`
	ClientID      string           `bson:"clientId,omitempty" json:"clientId,omitempty"`
	ApplicationID string           `bson:"applicationId,omitempty" json:"applicationId,omitempty"`
	Name          string           `bson:"name" json:"name"`
	Active        bool             `bson:"active" json:"active"`
	UserIdentity  *UserIdentity    `bson:"userIdentity,omitempty" json:"userIdentity,omitempty"`
	Roles         []RoleAssignment `bson:"roles,omitempty" json:"roles,omitempty"`
	CreatedAt     time.Time        `bson:"createdAt" json:"createdAt"`
	UpdatedAt     time.Time        `bson:"updatedAt" json:"updatedAt"`
}

// UserIdentity contains authentication details for a user principal
type UserIdentity struct {
	Email         string    `bson:"email" json:"email"`
	EmailDomain   string    `bson:"emailDomain" json:"emailDomain"`
	EmailVerified bool      `bson:"emailVerified" json:"emailVerified"`
	IdpType       IdpType   `bson:"idpType" json:"idpType"`
	IdpIssuer     string    `bson:"idpIssuer,omitempty" json:"idpIssuer,omitempty"`     // External IDP issuer URL
	IdpSubject    string    `bson:"idpSubject,omitempty" json:"idpSubject,omitempty"`   // External IDP subject (user ID)
	ExternalIdpID string    `bson:"externalIdpId,omitempty" json:"externalIdpId,omitempty"`
	PasswordHash  string    `bson:"passwordHash,omitempty" json:"-"` // Never serialize to JSON
	LastLoginAt   time.Time `bson:"lastLoginAt,omitempty" json:"lastLoginAt,omitempty"`
}

// RoleAssignment represents a role assigned to a principal
type RoleAssignment struct {
	RoleID           string    `bson:"roleId" json:"roleId"`
	RoleName         string    `bson:"roleName" json:"roleName"`
	AssignmentSource string    `bson:"assignmentSource,omitempty" json:"assignmentSource,omitempty"`
	AssignedAt       time.Time `bson:"assignedAt" json:"assignedAt"`
}

// HasRole checks if the principal has a specific role
func (p *Principal) HasRole(roleName string) bool {
	for _, r := range p.Roles {
		if r.RoleName == roleName {
			return true
		}
	}
	return false
}

// GetRoleNames returns all role names for this principal
func (p *Principal) GetRoleNames() []string {
	names := make([]string, len(p.Roles))
	for i, r := range p.Roles {
		names[i] = r.RoleName
	}
	return names
}

// IsAnchor returns true if the principal has anchor (platform admin) scope
func (p *Principal) IsAnchor() bool {
	return p.Scope == UserScopeAnchor
}

// IsPartner returns true if the principal has partner scope
func (p *Principal) IsPartner() bool {
	return p.Scope == UserScopePartner
}

// IsClientScoped returns true if the principal is bound to a single client
func (p *Principal) IsClientScoped() bool {
	return p.Scope == UserScopeClient
}
