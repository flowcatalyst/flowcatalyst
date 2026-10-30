package operations

import (
	"context"
	"strings"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/audit"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth/authservice"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/role"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/serviceaccount"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/internal/tsid"
	"github.com/flowcatalyst/go-sdk/usecase"
)

// The gates every caller of these operations (the HTTP handlers and the /ui
// pages) applies before running one. They live here so there is one copy.

// RequireAnchorWriter is the gate on service-account create, update and
// delete: the permission, and anchor scope on top of it (owner decision #19).
// A service account's SERVICE principal is anchor-tier, so with the
// permission alone a non-anchor administrator could mint an anchor-tier
// credential for itself — an escalation. Deactivate, assign roles, the
// regenerations and the mint follow the same rule (AuthorizeWrite).
func RequireAnchorWriter(ac *auth.AuthContext, can func(*auth.AuthContext) error) error {
	if err := can(ac); err != nil {
		return err
	}
	return auth.RequireAnchor(ac)
}

// RequireApplicationReach confines an application-scoped caller (one without
// all-applications access, such as an application's own service account)
// to service accounts bound to an application it can access, the rule
// sdksync's requireAppAccess applies to its syncs: it must not manage a
// platform-wide service account or another application's. A caller with
// all-applications access is not confined.
func RequireApplicationReach(ac *auth.AuthContext, applicationID *string) error {
	if !ac.IsApplicationScoped() {
		return nil
	}
	if applicationID != nil && ac.CanAccessApplication(*applicationID) {
		return nil
	}
	return usecase.Authorization("APPLICATION_ACCESS_REQUIRED",
		"not authorised for this service account's application")
}

// AuthorizeWrite is the gate on every write to an existing service account
// bound to applicationID (nil = none): the permission for the operation,
// anchor reach (owner decision #19), and the application confinement.
func AuthorizeWrite(ac *auth.AuthContext, applicationID *string, can func(*auth.AuthContext) error) error {
	if err := RequireAnchorWriter(ac, can); err != nil {
		return err
	}
	return RequireApplicationReach(ac, applicationID)
}

// AuthorizeCreate is the gate on creating a service account for
// applicationID (nil = none): the create permission, anchor reach, the
// application confinement, and — as for assigning application access — only
// a caller that itself holds all-applications access may grant it.
func AuthorizeCreate(ac *auth.AuthContext, allApplications bool, applicationID *string) error {
	if err := RequireAnchorWriter(ac, auth.CanCreateServiceAccounts); err != nil {
		return err
	}
	if allApplications && !ac.AllApplications {
		return httperror.Forbidden("Only an all-applications administrator may grant all-applications access")
	}
	return RequireApplicationReach(ac, applicationID)
}

// MintDeps are what MintAdminToken needs. Auth and Roles are required (nil
// refuses the mint, fail closed); FlattenPermissions and Audit are optional.
type MintDeps struct {
	Repo       *serviceaccount.Repository
	Principals *principal.Repository
	Roles      *role.Repository
	// Auth mints the bearer.
	Auth *authservice.AuthService
	// FlattenPermissions resolves the linked principal's roles into the
	// permission ceiling carried as the minted token's "scope" claim — the
	// same computation the client_credentials grant runs. When nil the token
	// carries no scope claim.
	FlattenPermissions func(ctx context.Context, roleNames []string) ([]string, error)
	// Audit records the mint (an admin obtaining a live credential for
	// another identity must leave a trail).
	Audit *audit.Repository
}

// MintedToken is the short-lived bearer MintAdminToken hands out: a live
// credential, shown once, never logged.
type MintedToken struct {
	AccessToken string
	ExpiresIn   int64
	// Scope is the space-separated permissions the token carries ("" = none).
	Scope string
}

// MintAdminToken mints the same short-lived, authority-bearing bearer the
// account would obtain from the client_credentials grant — scope = the
// linked principal's full permission ceiling — WITHOUT exposing the client
// secret. It hands out the service account's live authority, so the caller
// has passed AuthorizeWrite, and the mint is recorded in the audit trail.
// Nothing is persisted; the token simply expires.
func MintAdminToken(ctx context.Context, d MintDeps, id string) (MintedToken, error) {
	var zero MintedToken
	ac := auth.FromContext(ctx)
	if d.Auth == nil {
		return zero, usecase.Internal("TOKEN", "token minting is not wired", nil)
	}
	sa, err := d.Repo.FindByID(ctx, id)
	if err != nil {
		return zero, usecase.Internal("REPO", "find_by_id failed", err)
	}
	if sa == nil {
		return zero, httperror.NotFound("ServiceAccount", id)
	}
	if !sa.Active {
		return zero, usecase.Validation("SERVICE_ACCOUNT_INACTIVE",
			"the service account is deactivated — reactivate it before minting a token")
	}
	p, err := d.Principals.FindByServiceAccount(ctx, id)
	if err != nil {
		return zero, usecase.Internal("REPO", "find_by_service_account failed", err)
	}
	if p == nil {
		return zero, usecase.Internal("PRINCIPAL", "service account has no linked principal", nil)
	}
	if !p.Active {
		return zero, usecase.Validation("SERVICE_ACCOUNT_INACTIVE",
			"the service account's principal is deactivated")
	}

	names := make([]string, 0, len(p.Roles))
	for _, ra := range p.Roles {
		names = append(names, ra.Role.String())
	}
	// The minted bearer carries the account's whole authority, so the caller
	// must hold it: every platform permission of every role the account holds
	// (the role ceiling, F8). Otherwise a holder of service-account:update
	// could mint a token for an account a super-admin had made stronger than
	// the caller. 403 ROLE_ABOVE_CALLER.
	if err := requireMintWithinCaller(ctx, d.Roles, ac, names); err != nil {
		return zero, err
	}

	// Same grant computation as the client_credentials path: the principal's
	// full permission ceiling, flattened from its roles.
	var granted []string
	if d.FlattenPermissions != nil {
		if granted, err = d.FlattenPermissions(ctx, names); err != nil {
			return zero, usecase.Internal("SCOPE", "permission flattening failed", err)
		}
	}
	token, err := d.Auth.GenerateAccessTokenWithScope(p, granted)
	if err != nil {
		return zero, usecase.Internal("TOKEN", "token mint failed", err)
	}

	// A bearer was handed out for this account — a use of its credentials.
	// Best-effort: a failed stamp must not fail the mint.
	_ = d.Repo.TouchLastUsed(ctx, sa.ID)

	// Best-effort audit: who obtained a credential for which account. Never
	// the token itself.
	if d.Audit != nil {
		actor := ac.PrincipalID
		_ = d.Audit.Insert(ctx, &audit.Log{
			ID:          tsid.Generate(tsid.AuditLog),
			EntityType:  "SERVICE_ACCOUNT",
			EntityID:    sa.ID,
			Operation:   "TOKEN_MINTED_BY_ADMIN",
			PrincipalID: &actor,
			PerformedAt: time.Now().UTC(),
		})
	}

	return MintedToken{
		AccessToken: token,
		// Mirrors the /oauth/token response contract (expires_in 3600).
		ExpiresIn: 3600,
		Scope:     strings.Join(granted, " "),
	}, nil
}

// requireMintWithinCaller refuses a mint for an account holding a role the
// caller could not assign (role.RolesAboveCaller), naming the roles.
func requireMintWithinCaller(ctx context.Context, roles *role.Repository, ac *auth.AuthContext, roleNames []string) error {
	if roles == nil {
		return usecase.Internal("ROLES", "role ceiling is not wired", nil)
	}
	above, err := role.RolesAboveCaller(ctx, ac, roleNames, role.PermissionsFrom(roles))
	if err != nil {
		return err
	}
	if len(above) == 0 {
		return nil
	}
	return usecase.Authorization(role.CeilingCode,
		"You may only mint a token for a service account whose roles' permissions you hold yourself: "+
			strings.Join(above, ", "))
}
