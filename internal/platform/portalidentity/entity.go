// Package portalidentity is the portal identity plane
// (docs/portal-identity-plan.md Phase 2.5 v2): portal end-users as a
// SEPARATE population from iam_principals — one identity per
// (client, email) context, platform-implemented (password machinery,
// reset tokens, OIDC bridge reuse) but with initiation endpoints
// independent of the employee auth surface.
//
// A client may run several portal apps ([App]). The identity stays one per
// (client, email) — one password across that client's portals — and is
// GRANTED per app; a login through an app-linked OAuth client requires the
// grant.
package portalidentity

import (
	"slices"
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

// Source records how the identity (or an app grant) came to exist.
type Source string

const (
	// SourceInvite — created by the portal backend via /api/portal-users.
	SourceInvite Source = "INVITE"
	// SourceJIT — created by a first SSO login through the portal plane.
	SourceJIT Source = "JIT"
	// SourceAdmin — an app grant added directly (POST /api/portal-users/{id}/apps).
	SourceAdmin Source = "ADMIN"
)

// AccessState is the admin-facing lifecycle of an identity, derived — never
// stored — from status, credentials, logins, and the invite dates, so it can
// never drift from the facts it summarises.
type AccessState string

const (
	// StateInvited — invited, has not yet set a password or signed in; the
	// invite is still usable (SSO invites never expire).
	StateInvited AccessState = "INVITED"
	// StateInviteExpired — invited, never completed, and the set-password
	// link has lapsed. Re-ensuring re-sends it.
	StateInviteExpired AccessState = "INVITE_EXPIRED"
	// StateActive — has created a password or signed in at least once.
	StateActive AccessState = "ACTIVE"
	// StateSuspended — status DISABLED.
	StateSuspended AccessState = "SUSPENDED"
)

// AppGrant is one identity's access to one portal app.
type AppGrant struct {
	AppID     string    `json:"appId"`
	Source    Source    `json:"source"`
	GrantedAt time.Time `json:"grantedAt"`
}

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
	// InvitedAt / InviteExpiresAt record the latest invite. They are
	// infrastructure bookkeeping written by the invite path (MarkInvited),
	// not by Persist. A nil expiry with InvitedAt set is an SSO invite.
	InvitedAt       *time.Time `json:"invitedAt,omitempty"`
	InviteExpiresAt *time.Time `json:"inviteExpiresAt,omitempty"`
	// Apps are the portal apps this identity may sign in to. Persist inserts
	// any grant here that is missing and deletes only the grants explicitly
	// revoked (revoked) — never "whatever isn't in Apps", so a grant added
	// concurrently by another request is not silently lost.
	Apps      []AppGrant `json:"apps"`
	CreatedAt time.Time  `json:"createdAt"`
	UpdatedAt time.Time  `json:"updatedAt"`

	// revoked are the app ids Revoke removed since load — the only grant
	// rows Persist deletes.
	revoked []string
}

// IDStr satisfies usecase.HasID.
func (i Identity) IDStr() string { return i.ID }

// HasPassword reports whether a password has been set.
func (i *Identity) HasPassword() bool {
	return i.PasswordHash != nil && *i.PasswordHash != ""
}

// CanSignInWithPassword reports whether a password login is possible (has a
// hash and is ACTIVE).
func (i *Identity) CanSignInWithPassword() bool {
	return i.Status == StatusActive && i.HasPassword()
}

// HasApp reports whether the identity is granted the portal app.
func (i *Identity) HasApp(appID string) bool {
	return slices.ContainsFunc(i.Apps, func(g AppGrant) bool { return g.AppID == appID })
}

// Grant adds the app grant, reporting whether it was new.
func (i *Identity) Grant(appID string, source Source) bool {
	if appID == "" || i.HasApp(appID) {
		return false
	}
	i.revoked = slices.DeleteFunc(i.revoked, func(id string) bool { return id == appID })
	i.Apps = append(i.Apps, AppGrant{AppID: appID, Source: source, GrantedAt: time.Now().UTC()})
	return true
}

// Revoke removes the app grant, reporting whether one existed. The removal
// is recorded so Persist deletes exactly this grant row.
func (i *Identity) Revoke(appID string) bool {
	before := len(i.Apps)
	i.Apps = slices.DeleteFunc(i.Apps, func(g AppGrant) bool { return g.AppID == appID })
	if len(i.Apps) == before {
		return false
	}
	i.revoked = append(i.revoked, appID)
	return true
}

// RevokedApps are the app ids revoked since load (Persist deletes these).
func (i *Identity) RevokedApps() []string { return slices.Clone(i.revoked) }

// State derives the admin-facing lifecycle at time now.
func (i *Identity) State(now time.Time) AccessState {
	switch {
	case i.Status == StatusDisabled:
		return StateSuspended
	case i.HasPassword(), i.LastLoginAt != nil, i.Source == SourceJIT:
		return StateActive
	case i.InviteExpiresAt != nil && !now.Before(*i.InviteExpiresAt):
		return StateInviteExpired
	default:
		return StateInvited
	}
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
		Apps:      []AppGrant{},
		CreatedAt: now,
		UpdatedAt: now,
	}
}
