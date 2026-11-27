// Package api wires the HTTP routes for the principal subdomain via huma.
package api

import (
	"context"
	"net/http"
	"slices"

	"github.com/danielgtaylor/huma/v2"

	"github.com/flowcatalyst/flowcatalyst-go/internal/ids"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/audit"
	platformauth "github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/client"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/emaildomainmapping"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/identityprovider"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/mfa"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/notify"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/role"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/serviceaccount"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apiroute"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/versioncache"
	"github.com/flowcatalyst/go-sdk/usecase"
	"github.com/flowcatalyst/go-sdk/usecasepgx"
)

// State bundles deps. Principal ops need cross-aggregate validation
// against roles, applications, and clients.
type State struct {
	Repo *principal.Repository
	// Versions backs GET /api/principals/{id}/version. Optional: nil falls
	// back to Repo.LookupVersion directly (uncached, still correct) —
	// convenient for tests that don't wire a Reader.
	Versions  *versioncache.Reader
	GrantRepo *principal.ClientAccessGrantRepo
	Roles     *role.Repository
	// ServiceAccounts resolves a SERVICE principal's service account, whose
	// application bounds who may change the principal's application access
	// (saops.AuthorizeWrite). Required for that route to act on a SERVICE
	// principal; nil refuses it.
	ServiceAccounts *serviceaccount.Repository
	Applications    *application.Repository
	// ClientConfigs resolves which applications a client is entitled to (its
	// enabled client-configs). It bounds what a non-anchor (client-admin) may
	// assign: a client-admin can grant a user ANY application the client can
	// access, not merely the apps the admin personally holds.
	ClientConfigs     *application.ClientConfigRepo
	Clients           *client.Repository
	Mappings          *emaildomainmapping.Repository // for /check-email-domain + create-user scope derivation
	IdentityProviders *identityprovider.Repository   // for /check-email-domain + create-user idp-type
	AnchorDomains     *platformauth.AnchorDomainRepo // for create-user anchor-domain check (optional)
	UoW               *usecasepgx.UnitOfWork
	PasswordEmailer   operations.PasswordResetEmailer // optional; gates /send-password-reset

	// InviteEmailer (optional) sends a "set your password" link to a newly
	// created internal user that has no password yet. Notifier (optional) sends
	// the "your account was created" welcome to users created WITH a password.
	InviteEmailer InviteEmailer
	Notifier      *notify.Notifier
	// MFA (optional) backs POST /api/principals/{id}/reset-2fa.
	MFA *mfa.Service
	// Audit (optional) records the admin 2FA reset to the audit trail.
	Audit *audit.Repository
}

// InviteEmailer mints a first-time set-password link for a new user. The
// passwordreset principalEmailer satisfies it.
type InviteEmailer interface {
	// SendInviteRedirect emails the invite; redirectURI (already validated,
	// may be nil) is stored on the token and followed after set-password.
	SendInviteRedirect(ctx context.Context, p *principal.Principal, redirectURI *string) error
	// InviteLink mints the same invite token as SendInvite but returns the
	// link instead of emailing it — backs CreateUserRequest.ReturnInviteLink.
	InviteLink(ctx context.Context, p *principal.Principal, redirectURI *string) (string, error)
}

const tag = "principals"

// Register mounts the principal endpoints.
func Register(api huma.API, s *State) {
	g := apiroute.New(api, tag)
	apiroute.Get(g, "listPrincipals", "/api/principals", "List principals", s.list)
	apiroute.Post(g, "createPrincipal", "/api/principals", "Create a principal", http.StatusCreated, s.create)
	apiroute.Post(g, "createUser", "/api/principals/users", "Create a user principal (scope optional, default CLIENT; ANCHOR/PARTNER must be backed by the email domain's setup)", http.StatusOK, s.createUser)
	apiroute.Post(g, "bulkImportUsers", "/api/principals/bulk-import", "Bulk-import CLIENT users for a client (CSV onboarding)", http.StatusOK, s.bulkImport)
	apiroute.Post(g, "matchPrincipalEmails", "/api/principals/match-emails", "Match a pasted list of email addresses against the users the caller can see", http.StatusOK, s.matchEmails)
	apiroute.Post(g, "syncUsers", "/api/principals/sync", "Sync users (declarative upsert by email; no application scope)", http.StatusOK, s.syncUsers)
	apiroute.Get(g, "getPrincipal", "/api/principals/{id}", "Get a principal by id", s.getByID)
	apiroute.Get(g, "getPrincipalVersion", "/api/principals/{id}/version", "Get when a principal (or a role it holds) last changed — used by SDKs to detect a revoked/changed session", s.getVersion)
	apiroute.Put(g, "updatePrincipal", "/api/principals/{id}", "Update a principal", http.StatusOK, s.update)
	apiroute.Post(g, "activatePrincipal", "/api/principals/{id}/activate", "Activate a principal", http.StatusOK, s.activate)
	apiroute.Post(g, "deactivatePrincipal", "/api/principals/{id}/deactivate", "Deactivate a principal", http.StatusOK, s.deactivate)
	apiroute.Post(g, "resetPrincipalPassword", "/api/principals/{id}/reset-password", "Reset a user's password", http.StatusOK, s.resetPassword)
	apiroute.Post(g, "sendPrincipalPasswordReset", "/api/principals/{id}/send-password-reset", "Send a password-reset email to a user", http.StatusOK, s.sendPasswordReset)
	apiroute.Post(g, "resetPrincipalTwoFactor", "/api/principals/{id}/reset-2fa", "Clear a user's two-factor methods (forces re-enrollment)", http.StatusOK, s.resetTwoFactor)
	apiroute.Get(g, "checkPrincipalEmailDomain", "/api/principals/check-email-domain", "Resolve auth-method for an email's domain", s.checkEmailDomain)
	apiroute.Delete(g, "deletePrincipal", "/api/principals/{id}", "Delete a principal", http.StatusNoContent, s.delete)
	apiroute.Put(g, "assignPrincipalRoles", "/api/principals/{id}/roles", "Assign roles to a principal (replaces full set)", http.StatusOK, s.assignRoles)
	apiroute.Get(g, "listPrincipalRoles", "/api/principals/{id}/roles", "List a principal's assigned roles", s.listRoles)
	apiroute.Post(g, "addPrincipalRole", "/api/principals/{id}/roles", "Add a single role to a principal", http.StatusOK, s.addRole)
	apiroute.Delete(g, "removePrincipalRole", "/api/principals/{id}/roles/{role}", "Remove a single role from a principal", http.StatusOK, s.removeRole)
	apiroute.Put(g, "assignPrincipalApplicationAccess", "/api/principals/{id}/application-access", "Assign application access to a principal", http.StatusOK, s.assignApplicationAccess)
	apiroute.Get(g, "listPrincipalApplicationAccess", "/api/principals/{id}/application-access", "List application IDs the principal can access", s.listApplicationAccess)
	apiroute.Get(g, "listPrincipalAvailableApplications", "/api/principals/{id}/available-applications", "List applications a principal can be granted access to", s.listAvailableApplications)
	apiroute.Get(g, "listPrincipalClientAccess", "/api/principals/{id}/client-access", "List client-access grants for a principal", s.listClientAccess)
	apiroute.Post(g, "grantPrincipalClientAccess", "/api/principals/{id}/client-access", "Grant a client-access for a principal", http.StatusOK, s.grantClientAccess)
	apiroute.Delete(g, "revokePrincipalClientAccess", "/api/principals/{id}/client-access/{clientId}", "Revoke a client-access grant", http.StatusNoContent, s.revokeClientAccess)
	apiroute.Put(g, "setPrincipalClientAssociation", "/api/principals/{id}/client-association", "Change a principal's scope/client association (anchor-gated)", http.StatusOK, s.setClientAssociation)
	apiroute.Get(g, "listDeveloperUsers", "/api/principals/developer-users", "List USER principals holding the developer role", s.listDeveloperUsers)
	apiroute.Post(g, "setPrincipalDeveloperCredential", "/api/principals/{id}/developer-credential", "Create or rotate a principal's self-service developer API credential", http.StatusOK, s.setDeveloperCredential)
	apiroute.Delete(g, "revokePrincipalDeveloperCredential", "/api/principals/{id}/developer-credential", "Revoke a principal's self-service developer API credential", http.StatusNoContent, s.revokeDeveloperCredential)
}

// PrincipalVisible is the list's per-row access rule, shared with
// match-emails, the developer-users list and the console's Developer Users
// page so they cannot drift: anchors see all; non-anchors see the
// principals homed at a client they can access, and the partners granted one.
// Other platform-level principals (anchors, partners of other clients) are
// hidden from non-anchors. The by-id read and its sub-routes (version, roles,
// application access, available applications) apply the same rule, answering
// the not-found a missing id would (owner ruling PR-3/PR-4; the earlier
// leniency for principals with no home client let a client administrator read
// an anchor user by id).
func PrincipalVisible(ac *auth.AuthContext, p *principal.Principal) bool {
	if ac.IsAnchor() {
		return true
	}
	if p.ClientID != nil {
		return ac.CanAccessClient(string(*p.ClientID))
	}
	return p.Scope == principal.ScopePartner && slices.ContainsFunc(p.AssignedClients, func(c ids.ClientID) bool {
		return ac.CanAccessClient(string(c))
	})
}

// forCaller trims a principal response to what the caller may know: a
// non-anchor sees only the granted clients it can access itself, so a client
// administrator looking at a partner does not learn the partner's other
// clients.
func forCaller(ac *auth.AuthContext, r PrincipalResponse) PrincipalResponse {
	if ac.IsAnchor() {
		return r
	}
	r.GrantedClientIDs = slices.DeleteFunc(slices.Clone(r.GrantedClientIDs), func(c string) bool {
		return !ac.CanAccessClient(c)
	})
	return r
}

// loadScoped loads the principal and enforces per-resource scope (A2) on top of
// the coarse permission already checked: a non-anchor principal must not mutate
// another tenant's principal by id. (UpdatePrincipalRequest deliberately
// doesn't expose scope/client_id at all, so the scope/client-escalation vector
// can't exist here — no extra gate needed.)
//
// An out-of-scope target answers the SAME not-found error a genuinely missing
// id would — never 403 — so an unauthorized caller can't use the response to
// learn whether an id is real (PR-3(b)). blockNonClientTarget stays a 403: a
// distinct "wrong kind of administrator" decision, not a tenancy boundary.
func (s *State) loadScoped(ctx context.Context, ac *auth.AuthContext, id ids.PrincipalID) (*principal.Principal, error) {
	p, err := s.Repo.FindByID(ctx, id)
	if err != nil {
		return nil, usecase.Internal("REPO", "find_by_id failed", err)
	}
	if p == nil {
		return nil, httperror.NotFound("Principal", string(id))
	}
	if err := blockNonClientTarget(ac, p); err != nil {
		return nil, err
	}
	if !auth.CanAccessScope(ac, ids.StringPtr(p.ClientID)) {
		return nil, httperror.NotFound("Principal", string(id))
	}
	return p, nil
}

// blockNonClientTarget stops a non-anchor administrator (client-admin) from
// acting on an ANCHOR- or PARTNER-scoped principal. Anchors are unrestricted.
// Client access alone is not enough: a PARTNER user's home client may be one the
// admin can reach, but partner/anchor users are out of a client-admin's remit —
// they manage only CLIENT-scope users. Pairs with CanAccessClient/CheckScopeAccess,
// which bound *which* client; this bounds *which kind* of user.
func blockNonClientTarget(ac *auth.AuthContext, p *principal.Principal) error {
	if ac != nil && !ac.IsAnchor() && p != nil && p.Scope != principal.ScopeClient {
		return httperror.Forbidden("Client administrators can only manage client-scope users")
	}
	return nil
}
