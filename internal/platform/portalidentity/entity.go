// Package portalidentity is the portal identity plane
// (docs/portal-identity-plan.md Phase 2.5 v2): portal end-users as a
// SEPARATE population from iam_principals — one identity per
// (client, email) context, platform-implemented (password machinery,
// reset tokens, OIDC bridge reuse) but with initiation endpoints
// independent of the employee auth surface.
package portalidentity

import (
	"strings"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/tsid"
)

// Status is the identity state. Only ACTIVE may authenticate.
type Status string

const (
	// StatusActive permits portal login.
	StatusActive Status = "ACTIVE"
	// StatusDisabled suspends portal login but keeps the row (history,
	// reactivation without a fresh invite).
	StatusDisabled Status = "DISABLED"
)

// Source records how the identity came to exist.
type Source string

const (
	// SourceInvite — created by the portal backend via /api/portal-users.
	SourceInvite Source = "INVITE"
	// SourceJIT — created by a first SSO login through the portal plane.
	SourceJIT Source = "JIT"
)

// Identity is one portal end-user identity in one client's portal context.
// The same human at two clients' portals is two Identities with independent
// credentials. Wholly unrelated to iam_principals.
type Identity struct {
	ID       string `json:"id"`
	ClientID string `json:"clientId"`
	Email    string `json:"email"`
	Name     string `json:"name,omitempty"`
	// PasswordHash is nil until the invite completes (or forever, for
	// SSO-only identities).
	PasswordHash *string    `json:"-"`
	Status       Status     `json:"status"`
	Source       Source     `json:"source"`
	LastLoginAt  *time.Time `json:"lastLoginAt,omitempty"`
	CreatedAt    time.Time  `json:"createdAt"`
	UpdatedAt    time.Time  `json:"updatedAt"`
}

// IDStr satisfies usecase.HasID.
func (i Identity) IDStr() string { return i.ID }

// CanSignInWithPassword reports whether a password login is possible (has a
// hash and is ACTIVE).
func (i *Identity) CanSignInWithPassword() bool {
	return i.Status == StatusActive && i.PasswordHash != nil && *i.PasswordHash != ""
}

// New constructs an ACTIVE identity. Email is normalised to lower-case (the
// (client_id, email) uniqueness key assumes it).
func New(clientID, email, name string, source Source) *Identity {
	now := time.Now().UTC()
	return &Identity{
		ID:        tsid.Generate(tsid.PortalUser),
		ClientID:  clientID,
		Email:     strings.ToLower(strings.TrimSpace(email)),
		Name:      name,
		Status:    StatusActive,
		Source:    source,
		CreatedAt: now,
		UpdatedAt: now,
	}
}
