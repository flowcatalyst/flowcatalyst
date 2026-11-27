package api

import (
	"context"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/ids"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/serviceaccount"
	saops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/serviceaccount/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apicommon"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/go-sdk/usecase"
	"github.com/flowcatalyst/go-sdk/usecaseop"
)

// assertAssignableRoles bounds a non-anchor (client-admin) role assignment
// (operations.RequireAssignableRoles, shared with the platform principal
// sync): every role must be application-scoped (ApplicationID set, i.e. not a
// platform role) and belong to an application the TARGET CLIENT can access
// (allowed). Anchors are not subject to this (callers skip it).
func (s *State) assertAssignableRoles(ctx context.Context, roleNames []string, allowed map[string]bool) error {
	return operations.RequireAssignableRoles(ctx, s.Roles, roleNames, allowed)
}

// clientAppIDs returns the set of application IDs the given client is entitled
// to (operations.ClientApplicationIDs): the bound a non-anchor (client-admin)
// is held to when assigning roles/applications.
func (s *State) clientAppIDs(ctx context.Context, clientID string) (map[string]bool, error) {
	return operations.ClientApplicationIDs(ctx, s.ClientConfigs, clientID)
}

// clientIDOf returns a principal's client id, or "" when it has none.
func clientIDOf(p *principal.Principal) string {
	if p == nil || p.ClientID == nil {
		return ""
	}
	return string(*p.ClientID)
}

// assertAssignableApplications bounds a non-anchor (client-admin) application
// grant: every application in the requested set must be one the TARGET CLIENT
// can access (allowed). This stops a client-admin from granting a user access to
// an application the client isn't entitled to. Anchors are not subject to this.
func (s *State) assertAssignableApplications(appIDs []string, allowed map[string]bool) error {
	for _, id := range appIDs {
		if !allowed[id] {
			return usecase.Authorization("APP_FORBIDDEN",
				"application the client cannot access: "+id)
		}
	}
	return nil
}

// preservedApplications returns the subset of a user's existing application
// grants that a non-anchor admin may NOT manage (apps outside the CLIENT's
// reach). A client-admin's SET preserves these so it can't strip a user's access
// to applications the client itself can't see — mirrors protectedRoleNames.
func preservedApplications(existing []string, allowed map[string]bool) []string {
	var out []string
	for _, id := range existing {
		if !allowed[id] {
			out = append(out, id)
		}
	}
	return out
}

// protectedRoleNames returns the subset of roleNames a non-anchor admin may NOT
// manage — platform roles (ApplicationID nil), unknown roles, or roles for apps
// the TARGET CLIENT can't access (allowed). A client-admin's role SET preserves
// these so it can't strip a user's platform / other-application roles.
func (s *State) protectedRoleNames(ctx context.Context, roleNames []string, allowed map[string]bool) ([]string, error) {
	var out []string
	for _, name := range roleNames {
		r, err := s.Roles.FindByName(ctx, name)
		if err != nil {
			return nil, usecase.Internal("REPO", "find_role failed", err)
		}
		if r == nil || r.ApplicationID == nil || r.IsPlatform() || !allowed[string(*r.ApplicationID)] {
			out = append(out, name)
		}
	}
	return out, nil
}

func dedupeStrings(xs []string) []string {
	seen := make(map[string]struct{}, len(xs))
	out := make([]string, 0, len(xs))
	for _, x := range xs {
		if _, ok := seen[x]; ok {
			continue
		}
		seen[x] = struct{}{}
		out = append(out, x)
	}
	return out
}

type assignRolesInput struct {
	ID   ids.PrincipalID `path:"id"`
	Body AssignPrincipalRolesRequest
}

func (s *State) assignRoles(ctx context.Context, in *assignRolesInput) (*apicommon.Out[RolesAssignedResponse], error) {
	ac := auth.FromContext(ctx)
	// Coarse permission gate BEFORE any load (PR-3(a)): no permission means
	// nothing is touched, so an unauthorized caller can't distinguish a real id
	// from an invented one via this route's response. Changing roles needs
	// user:assign-roles itself, not user:update (owner ruling 2026-10-11).
	if err := auth.CanAssignPrincipalRoles(ac); err != nil {
		return nil, err
	}
	// Load the target to shape the desired role set for non-anchor admins; the
	// per-resource authorization (RequireUserAdmin + blockNonClientTarget) is
	// enforced inside the AssignRoles use case post-load.
	p, err := s.Repo.FindByID(ctx, in.ID)
	if err != nil {
		return nil, usecase.Internal("REPO", "find_by_id failed", err)
	}
	if p == nil {
		return nil, httperror.NotFound("Principal", string(in.ID))
	}
	// A non-anchor administrator (client-admin) may only assign
	// application-scoped roles for applications their client can access —
	// never platform roles, never apps outside their reach. Because this is a
	// SET, we also preserve the user's existing protected roles so the client
	// admin can't strip a user's platform / other-application roles.
	effectiveRoles := in.Body.Roles
	if !ac.IsAnchor() {
		allowed, aerr := s.clientAppIDs(ctx, clientIDOf(p))
		if aerr != nil {
			return nil, aerr
		}
		if err := s.assertAssignableRoles(ctx, in.Body.Roles, allowed); err != nil {
			return nil, err
		}
		preserved, perr := s.protectedRoleNames(ctx, roleNamesFrom(p.Roles), allowed)
		if perr != nil {
			return nil, perr
		}
		effectiveRoles = dedupeStrings(append(append([]string{}, in.Body.Roles...), preserved...))
	}
	old := stringSet(roleNamesFrom(p.Roles))
	desired := stringSet(effectiveRoles)
	added := setDifference(desired, old)
	removed := setDifference(old, desired)

	ec := auth.NewExecutionContext(ctx)
	if _, err := usecaseop.Run(ctx, s.UoW, operations.AssignRoles(s.Repo, s.Roles),
		operations.AssignRolesCommand{UserID: in.ID, Roles: effectiveRoles}, ec); err != nil {
		return nil, err
	}
	refreshed, err := s.Repo.FindByID(ctx, in.ID)
	if err != nil {
		return nil, usecase.Internal("REPO", "find_by_id failed", err)
	}
	if refreshed == nil {
		return nil, httperror.NotFound("Principal", string(in.ID))
	}
	return &apicommon.Out[RolesAssignedResponse]{Body: RolesAssignedResponse{
		Roles:   roleAssignmentDTOs(in.ID, refreshed.Roles),
		Added:   added,
		Removed: removed,
	}}, nil
}

type assignAppAccessInput struct {
	ID   ids.PrincipalID `path:"id"`
	Body AssignApplicationAccessRequest
}

func (s *State) assignApplicationAccess(ctx context.Context, in *assignAppAccessInput) (*apicommon.Out[SetApplicationAccessResponse], error) {
	ac := auth.FromContext(ctx)
	// Coarse permission gate BEFORE any load (PR-3(a)): no permission means
	// nothing is touched, so an unauthorized caller can't distinguish a real id
	// from an invented one via this route's response.
	// A service account's application access is also writable with the
	// service-account update permission (the SPA's service-account page drives
	// this route), so either opens the route; the target's type decides below.
	if err := canUpdatePrincipalsOrServiceAccounts(ac); err != nil {
		return nil, err
	}
	// Load the target to bound the desired application set for non-anchor admins;
	// the per-resource authorization (RequireUserAdmin + blockNonClientTarget) is
	// enforced inside the AssignApplicationAccess use case post-load.
	p, err := s.Repo.FindByID(ctx, in.ID)
	if err != nil {
		return nil, usecase.Internal("REPO", "find_by_id failed", err)
	}
	if p == nil {
		return nil, httperror.NotFound("Principal", string(in.ID))
	}
	if err := s.authorizeApplicationAccessTarget(ctx, ac, p); err != nil {
		return nil, err
	}
	// Granting all-applications access exceeds any client-bounded reach, so only
	// an assigner that itself holds all-applications access may set it true,
	// and only an anchor-tier one: a client administrator never widens a
	// principal to every application. Clearing it (false) or leaving it
	// unchanged (nil) is open.
	if in.Body.AllApplications != nil && *in.Body.AllApplications {
		if !ac.AllApplications {
			return nil, httperror.Forbidden("Only an all-applications administrator may grant all-applications access")
		}
		if !ac.IsAnchor() {
			return nil, httperror.Forbidden("Only an anchor administrator may grant all-applications access")
		}
	}
	// Removing your OWN all-applications flag strips your ability to grant it to
	// anyone (including yourself back), so it needs an explicit confirmation.
	if ac.PrincipalID == string(p.ID) && p.AllApplications &&
		in.Body.AllApplications != nil && !*in.Body.AllApplications &&
		(in.Body.ConfirmSelfRestriction == nil || !*in.Body.ConfirmSelfRestriction) {
		return nil, usecase.Conflict("SELF_RESTRICTION_REQUIRES_CONFIRMATION",
			"You are removing your own all-applications access: you will no longer be able to grant "+
				"all-applications access to anyone (including yourself). Resend with confirmSelfRestriction=true to proceed.")
	}
	desiredIDs := in.Body.ApplicationIDs
	if !ac.IsAnchor() {
		allowed, aerr := s.clientAppIDs(ctx, clientIDOf(p))
		if aerr != nil {
			return nil, aerr
		}
		if err := s.assertAssignableApplications(in.Body.ApplicationIDs, allowed); err != nil {
			return nil, err
		}
		preserved := preservedApplications(ids.Strings(p.AccessibleApplicationIDs), allowed)
		desiredIDs = dedupeStrings(append(append([]string{}, in.Body.ApplicationIDs...), preserved...))
	}
	old := stringSet(ids.Strings(p.AccessibleApplicationIDs))
	desired := stringSet(desiredIDs)
	added := len(setDifference(desired, old))
	removed := len(setDifference(old, desired))

	ec := auth.NewExecutionContext(ctx)
	if _, err := usecaseop.Run(ctx, s.UoW, operations.AssignApplicationAccess(s.Repo, s.Applications),
		operations.AssignApplicationAccessCommand{
			UserID:          in.ID,
			ApplicationIDs:  desiredIDs,
			AllApplications: in.Body.AllApplications,
		}, ec); err != nil {
		return nil, err
	}
	apps, err := s.resolveApplications(ctx, desiredIDs)
	if err != nil {
		return nil, err
	}
	// Echo the effective all-applications state: the requested value when the
	// caller set it, otherwise the principal's unchanged value.
	effectiveAllApps := p.AllApplications
	if in.Body.AllApplications != nil {
		effectiveAllApps = *in.Body.AllApplications
	}
	return &apicommon.Out[SetApplicationAccessResponse]{Body: SetApplicationAccessResponse{
		Applications:    apps,
		Added:           added,
		Removed:         removed,
		AllApplications: effectiveAllApps,
	}}, nil
}

func (s *State) listClientAccess(ctx context.Context, in *apicommon.IDInput) (*apicommon.Out[ClientAccessGrantListResponse], error) {
	ac := auth.FromContext(ctx)
	if err := auth.RequireAnchor(ac); err != nil {
		return nil, err
	}
	grants, err := s.GrantRepo.FindByPrincipal(ctx, ids.PrincipalID(in.ID))
	if err != nil {
		return nil, usecase.Internal("REPO", "list grants failed", err)
	}
	out := apicommon.MapSlice(grants, clientAccessGrantFromEntity)
	return &apicommon.Out[ClientAccessGrantListResponse]{Body: ClientAccessGrantListResponse{Grants: out}}, nil
}

type grantClientAccessInput struct {
	ID   ids.PrincipalID `path:"id"`
	Body GrantClientAccessRequest
}

func (s *State) grantClientAccess(ctx context.Context, in *grantClientAccessInput) (*apicommon.Out[ClientAccessGrantResponse], error) {
	// Anchor tier and user:manage-access, checked here and again by the use
	// case's Authorize; the use case enforces the domain invariants
	// (USER/PARTNER, client exists, no duplicate grant).
	if err := auth.CanManagePrincipalAccess(auth.FromContext(ctx)); err != nil {
		return nil, err
	}
	ec := auth.NewExecutionContext(ctx)
	if _, err := usecaseop.Run(ctx, s.UoW, operations.GrantClientAccess(s.Repo, s.Clients, s.GrantRepo),
		operations.GrantClientAccessCommand{UserID: in.ID, ClientID: in.Body.ClientID}, ec); err != nil {
		return nil, err
	}
	g, err := s.GrantRepo.FindByPrincipalAndClient(ctx, in.ID, ids.ClientID(in.Body.ClientID))
	if err != nil {
		return nil, usecase.Internal("REPO", "find grant failed", err)
	}
	if g == nil {
		return nil, usecase.Internal("REPO", "grant not found after create", nil)
	}
	return &apicommon.Out[ClientAccessGrantResponse]{Body: clientAccessGrantFromEntity(g)}, nil
}

type setClientAssociationInput struct {
	ID   ids.PrincipalID `path:"id"`
	Body ClientAssociationRequest
}

// setClientAssociation changes a principal's scope + client association with
// explicit intent (anchor tier + user:manage-access). clientId "*" → ANCHOR; mode CHANGE_CLIENT →
// new home client; mode TO_PARTNER → promote to PARTNER (old + new client).
// Returns the updated principal.
func (s *State) setClientAssociation(ctx context.Context, in *setClientAssociationInput) (*apicommon.Out[PrincipalResponse], error) {
	// Anchor tier and user:manage-access (owner ruling 2026-10-25), checked
	// here and again by the use case's Authorize; the use case applies the
	// scope change + partner grants atomically.
	if err := auth.CanManagePrincipalAccess(auth.FromContext(ctx)); err != nil {
		return nil, err
	}
	ec := auth.NewExecutionContext(ctx)
	mode := ""
	if in.Body.Mode != nil {
		mode = strings.ToUpper(strings.TrimSpace(*in.Body.Mode))
	}
	if _, err := usecaseop.Run(ctx, s.UoW, operations.SetClientAssociation(s.Repo, s.Clients, s.GrantRepo),
		operations.SetClientAssociationCommand{
			UserID:   in.ID,
			ClientID: in.Body.ClientID,
			Mode:     operations.ClientAssociationMode(mode),
		}, ec); err != nil {
		return nil, err
	}
	p, err := s.Repo.FindByID(ctx, in.ID)
	if err != nil {
		return nil, usecase.Internal("REPO", "find_by_id failed", err)
	}
	if p == nil {
		return nil, httperror.NotFound("Principal", string(in.ID))
	}
	return &apicommon.Out[PrincipalResponse]{Body: fromEntity(p)}, nil
}

type revokeClientAccessInput struct {
	ID       ids.PrincipalID `path:"id"`
	ClientID ids.ClientID    `path:"clientId"`
}

func (s *State) revokeClientAccess(ctx context.Context, in *revokeClientAccessInput) (*apicommon.Empty, error) {
	// Anchor tier and user:manage-access, checked here and again by the use
	// case's Authorize; the use case enforces the domain invariants (USER
	// type, grant exists).
	if err := auth.CanManagePrincipalAccess(auth.FromContext(ctx)); err != nil {
		return nil, err
	}
	ec := auth.NewExecutionContext(ctx)
	if _, err := usecaseop.Run(ctx, s.UoW, operations.RevokeClientAccess(s.Repo, s.GrantRepo),
		operations.RevokeClientAccessCommand{UserID: in.ID, ClientID: string(in.ClientID)}, ec); err != nil {
		return nil, err
	}
	return &apicommon.Empty{}, nil
}

// ── granular role endpoints ──────────────────────────────────────────────

func (s *State) listRoles(ctx context.Context, in *apicommon.IDInput) (*apicommon.Out[PrincipalRoleListResponse], error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanReadPrincipals(ac); err != nil {
		return nil, err
	}
	p, err := s.Repo.FindByID(ctx, ids.PrincipalID(in.ID))
	if err != nil {
		return nil, usecase.Internal("REPO", "find_by_id failed", err)
	}
	if p == nil {
		return nil, httperror.NotFound("Principal", in.ID)
	}
	// PR-4: the same client-scope check as the by-id read — an out-of-scope
	// target answers 404, byte-identical to not-found, never 403.
	if !PrincipalVisible(ac, p) {
		return nil, httperror.NotFound("Principal", in.ID)
	}
	return &apicommon.Out[PrincipalRoleListResponse]{Body: PrincipalRoleListResponse{Roles: roleAssignmentDTOs(ids.PrincipalID(in.ID), p.Roles)}}, nil
}

type addRoleInput struct {
	ID   ids.PrincipalID `path:"id"`
	Body AddRoleRequest
}

func (s *State) addRole(ctx context.Context, in *addRoleInput) (*apicommon.Out[PrincipalResponse], error) {
	ac := auth.FromContext(ctx)
	// Coarse permission gate BEFORE any load (PR-3(a)): no permission means
	// nothing is touched, so an unauthorized caller can't distinguish a real id
	// from an invented one via this route's response. Changing roles needs
	// user:assign-roles itself, not user:update (owner ruling 2026-10-11).
	if err := auth.CanAssignPrincipalRoles(ac); err != nil {
		return nil, err
	}
	// Load the target to bound the role for non-anchor admins and to apply the
	// idempotent skip.
	p, err := s.Repo.FindByID(ctx, in.ID)
	if err != nil {
		return nil, usecase.Internal("REPO", "find_by_id failed", err)
	}
	if p == nil {
		return nil, httperror.NotFound("Principal", string(in.ID))
	}
	// Per-resource authorization (the use-case-side requireUserAdmin shape:
	// blockNonClientTarget + CanAccessScope, out-of-scope answers the same
	// not-found a missing id would, PR-4). This MUST run before the idempotent
	// early-return below — the role-already-present branch never reaches
	// AssignRoles (and therefore never reaches its own post-load
	// requireUserAdmin check), so without this a cross-tenant caller could
	// "add" a role the target already holds and read back another tenant's
	// full PrincipalResponse for free.
	if err := blockNonClientTarget(ac, p); err != nil {
		return nil, err
	}
	if !auth.CanAccessScope(ac, ids.StringPtr(p.ClientID)) {
		return nil, httperror.NotFound("Principal", string(in.ID))
	}
	if !ac.IsAnchor() {
		allowed, aerr := s.clientAppIDs(ctx, clientIDOf(p))
		if aerr != nil {
			return nil, aerr
		}
		if err := s.assertAssignableRoles(ctx, []string{in.Body.Role}, allowed); err != nil {
			return nil, err
		}
	}
	roles := uniqueRoleNames(p.Roles)
	if _, ok := roles[in.Body.Role]; !ok { // skip mutation when already present (idempotent)
		desired := append(roleNamesFrom(p.Roles), in.Body.Role)
		ec := auth.NewExecutionContext(ctx)
		if _, err := usecaseop.Run(ctx, s.UoW, operations.AssignRoles(s.Repo, s.Roles),
			operations.AssignRolesCommand{UserID: in.ID, Roles: desired}, ec); err != nil {
			return nil, err
		}
		if p, err = s.Repo.FindByID(ctx, in.ID); err != nil {
			return nil, usecase.Internal("REPO", "find_by_id failed", err)
		} else if p == nil {
			return nil, httperror.NotFound("Principal", string(in.ID))
		}
	}
	// Return the updated principal (full PrincipalResponse).
	return &apicommon.Out[PrincipalResponse]{Body: fromEntity(p)}, nil
}

type removeRoleInput struct {
	ID   ids.PrincipalID `path:"id"`
	Role string          `path:"role"`
}

func (s *State) removeRole(ctx context.Context, in *removeRoleInput) (*apicommon.Out[PrincipalResponse], error) {
	ac := auth.FromContext(ctx)
	// Coarse permission gate BEFORE any load (PR-3(a)): no permission means
	// nothing is touched, so an unauthorized caller can't distinguish a real id
	// from an invented one via this route's response. Changing roles needs
	// user:assign-roles itself, not user:update (owner ruling 2026-10-11).
	if err := auth.CanAssignPrincipalRoles(ac); err != nil {
		return nil, err
	}
	// Load the target to bound the role for non-anchor admins and to apply the
	// idempotent skip.
	p, err := s.Repo.FindByID(ctx, in.ID)
	if err != nil {
		return nil, usecase.Internal("REPO", "find_by_id failed", err)
	}
	if p == nil {
		return nil, httperror.NotFound("Principal", string(in.ID))
	}
	// Per-resource authorization (the use-case-side requireUserAdmin shape:
	// blockNonClientTarget + CanAccessScope, out-of-scope answers the same
	// not-found a missing id would, PR-4). This MUST run before the idempotent
	// early-return below — the role-not-present branch never reaches
	// AssignRoles (and therefore never reaches its own post-load
	// requireUserAdmin check), so without this a cross-tenant caller could
	// "remove" a role the target never had and read back another tenant's
	// full PrincipalResponse for free.
	if err := blockNonClientTarget(ac, p); err != nil {
		return nil, err
	}
	if !auth.CanAccessScope(ac, ids.StringPtr(p.ClientID)) {
		return nil, httperror.NotFound("Principal", string(in.ID))
	}
	// A non-anchor admin may only remove roles they could also assign — so they
	// can't strip a user's platform / other-application roles.
	if !ac.IsAnchor() {
		allowed, aerr := s.clientAppIDs(ctx, clientIDOf(p))
		if aerr != nil {
			return nil, aerr
		}
		if err := s.assertAssignableRoles(ctx, []string{in.Role}, allowed); err != nil {
			return nil, err
		}
	}
	current := roleNamesFrom(p.Roles)
	desired := make([]string, 0, len(current))
	found := false
	for _, r := range current {
		if r == in.Role {
			found = true
			continue
		}
		desired = append(desired, r)
	}
	if found { // skip mutation when absent (idempotent)
		ec := auth.NewExecutionContext(ctx)
		if _, err := usecaseop.Run(ctx, s.UoW, operations.AssignRoles(s.Repo, s.Roles),
			operations.AssignRolesCommand{UserID: in.ID, Roles: desired}, ec); err != nil {
			return nil, err
		}
		if p, err = s.Repo.FindByID(ctx, in.ID); err != nil {
			return nil, usecase.Internal("REPO", "find_by_id failed", err)
		} else if p == nil {
			return nil, httperror.NotFound("Principal", string(in.ID))
		}
	}
	// Return the updated principal (full PrincipalResponse).
	return &apicommon.Out[PrincipalResponse]{Body: fromEntity(p)}, nil
}

func roleNamesFrom(rs []serviceaccount.RoleAssignment) []string {
	return apicommon.MapSlice(rs, func(r *serviceaccount.RoleAssignment) string { return r.Role.String() })
}

func uniqueRoleNames(rs []serviceaccount.RoleAssignment) map[string]struct{} {
	out := make(map[string]struct{}, len(rs))
	for _, r := range rs {
		out[r.Role.String()] = struct{}{}
	}
	return out
}

func stringSet(vs []string) map[string]struct{} {
	out := make(map[string]struct{}, len(vs))
	for _, v := range vs {
		out[v] = struct{}{}
	}
	return out
}

// setDifference returns members of a not present in b (unordered).
func setDifference(a, b map[string]struct{}) []string {
	out := make([]string, 0)
	for v := range a {
		if _, ok := b[v]; !ok {
			out = append(out, v)
		}
	}
	return out
}

// ── application-access listing ───────────────────────────────────────────

func (s *State) listApplicationAccess(ctx context.Context, in *apicommon.IDInput) (*apicommon.Out[ApplicationAccessListResponse], error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanReadPrincipals(ac); err != nil {
		return nil, err
	}
	p, err := s.Repo.FindByID(ctx, ids.PrincipalID(in.ID))
	if err != nil {
		return nil, usecase.Internal("REPO", "find_by_id failed", err)
	}
	if p == nil {
		return nil, httperror.NotFound("Principal", in.ID)
	}
	// PR-4: the same client-scope check as the by-id read.
	if !PrincipalVisible(ac, p) {
		return nil, httperror.NotFound("Principal", in.ID)
	}
	apps, err := s.resolveApplications(ctx, ids.Strings(p.AccessibleApplicationIDs))
	if err != nil {
		return nil, err
	}
	return &apicommon.Out[ApplicationAccessListResponse]{Body: ApplicationAccessListResponse{
		Applications:    apps,
		Total:           len(apps),
		AllApplications: p.AllApplications,
	}}, nil
}

// resolveApplications hydrates application IDs into {id, code, name} rows,
// skipping IDs that no longer resolve (lenient by design).
func (s *State) resolveApplications(ctx context.Context, ids []string) ([]ApplicationAccessResponse, error) {
	out := make([]ApplicationAccessResponse, 0, len(ids))
	for _, id := range ids {
		a, err := s.Applications.FindByID(ctx, id)
		if err != nil {
			return nil, usecase.Internal("REPO", "find_app_by_id failed", err)
		}
		if a == nil {
			continue
		}
		out = append(out, ApplicationAccessResponse{
			ApplicationID:   string(a.ID),
			ApplicationCode: a.Code,
			ApplicationName: a.Name,
		})
	}
	return out, nil
}

func (s *State) listAvailableApplications(ctx context.Context, in *apicommon.IDInput) (*apicommon.Out[PrincipalAvailableApplicationsResponse], error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanReadPrincipals(ac); err != nil {
		return nil, err
	}
	p, err := s.Repo.FindByID(ctx, ids.PrincipalID(in.ID))
	if err != nil {
		return nil, usecase.Internal("REPO", "find_by_id failed", err)
	}
	if p == nil {
		return nil, httperror.NotFound("Principal", in.ID)
	}
	// PR-4: the same client-scope check as the by-id read.
	if !PrincipalVisible(ac, p) {
		return nil, httperror.NotFound("Principal", in.ID)
	}
	// Available = all active applications the system knows about — the
	// simpler choice over filtering by what's already enabled in the
	// principal's clients, pending product confirmation.
	// For a non-anchor administrator (client-admin) we bound the menu to the
	// applications the caller's client can access, so they can only grant what
	// the client is entitled to (mirrors the role-assignment bounding).
	apps, err := s.Applications.FindActive(ctx)
	if err != nil {
		return nil, usecase.Internal("REPO", "find_active_apps failed", err)
	}
	// For a non-anchor admin, bound the menu to applications the target user's
	// CLIENT is entitled to — so a client-admin can grant any app the client can
	// access, not just the apps the admin personally holds.
	allowed := map[string]bool{}
	if !ac.IsAnchor() {
		allowed, err = s.clientAppIDs(ctx, clientIDOf(p))
		if err != nil {
			return nil, err
		}
	}
	out := make([]PrincipalAvailableApplication, 0, len(apps))
	for i := range apps {
		a := &apps[i]
		if !ac.IsAnchor() && !allowed[string(a.ID)] {
			continue
		}
		out = append(out, PrincipalAvailableApplication{
			ID:   string(a.ID),
			Code: a.Code,
			Name: a.Name,
		})
	}
	return &apicommon.Out[PrincipalAvailableApplicationsResponse]{Body: PrincipalAvailableApplicationsResponse{
		Applications: out,
	}}, nil
}

// canUpdatePrincipalsOrServiceAccounts passes a caller holding user:update or
// service-account:update (the permission of the account's own API).
func canUpdatePrincipalsOrServiceAccounts(ac *auth.AuthContext) error {
	if err := auth.CanUpdatePrincipals(ac); err == nil {
		return nil
	}
	return auth.CanUpdateServiceAccounts(ac)
}

// authorizeApplicationAccessTarget is the per-target permission gate of the
// application-access route. A USER needs user:update. A SERVICE principal is a
// service account, so the write needs what the service-account API requires
// (saops.AuthorizeWrite: the permission, anchor reach, the account's
// application confinement); a client administrator holding user:update must
// not widen or narrow it. Other principal types are refused by the operation.
func (s *State) authorizeApplicationAccessTarget(ctx context.Context, ac *auth.AuthContext, p *principal.Principal) error {
	if p.Type != principal.TypeService {
		return auth.CanUpdatePrincipals(ac)
	}
	var appID *string
	if p.ServiceAccountID != nil {
		if s.ServiceAccounts == nil {
			return usecase.Internal("SERVICE_ACCOUNTS", "service account lookup is not wired", nil)
		}
		sa, err := s.ServiceAccounts.FindByID(ctx, *p.ServiceAccountID)
		if err != nil {
			return usecase.Internal("REPO", "find_by_id failed", err)
		}
		if sa != nil {
			appID = ids.StringPtr(sa.ApplicationID)
		}
	}
	return saops.AuthorizeWrite(ac, appID, canUpdatePrincipalsOrServiceAccounts)
}
