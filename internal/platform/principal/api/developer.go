package api

import (
	"context"

	"github.com/flowcatalyst/flowcatalyst-go/internal/ids"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apicommon"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/go-sdk/usecase"
	"github.com/flowcatalyst/go-sdk/usecaseop"
)

// developerRoleName is the seeded role (internal/platform/seed/roles.go)
// that gates the self-service developer client_credentials flow — matches
// the literal duplicated in principal/operations and oauthapi.
const developerRoleName = "platform:developer"

// ── developer API credentials (self-service client_credentials) ─────────

// listDeveloperUsers backs the Developer Users admin page: every USER
// principal currently holding the developer role, so an admin can grant the
// role to more users and manage existing developers' credentials. Gated the
// same as the general principal list (CanReadPrincipals), and scoped the same
// way (PrincipalVisible, forCaller) — set/revoke on a specific user go through
// the self-or-admin use cases below, not this read.
func (s *State) listDeveloperUsers(ctx context.Context, _ *apicommon.Empty) (*apicommon.Out[DeveloperUserListResponse], error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanReadPrincipals(ac); err != nil {
		return nil, err
	}
	rows, err := s.Repo.FindByRole(ctx, developerRoleName)
	if err != nil {
		return nil, usecase.Internal("REPO", "find_by_role failed", err)
	}
	out := make([]PrincipalResponse, 0, len(rows))
	for i := range rows {
		if !PrincipalVisible(ac, &rows[i]) {
			continue
		}
		out = append(out, forCaller(ac, fromEntity(&rows[i])))
	}
	return &apicommon.Out[DeveloperUserListResponse]{Body: DeveloperUserListResponse{Principals: out, Total: len(out)}}, nil
}

// setDeveloperCredential creates or rotates the target principal's
// self-service developer client_credentials secret. Coarse permission gate
// BEFORE any load (PR-3(a)): a caller acting on their own id (no load needed
// to know that — it's the authenticated principal's own id) needs the
// self-service developer-credential permission; a caller acting on someone
// else's id needs the general user-admin write permission. The
// SetDeveloperCredential use case's requireSelfOrUserAdmin then enforces the
// finer per-resource rule (the developer role check, and — for the admin
// branch — client scope) post-load.
func (s *State) setDeveloperCredential(ctx context.Context, in *apicommon.IDInput) (*apicommon.Out[SetDeveloperCredentialResponse], error) {
	ac := auth.FromContext(ctx)
	if err := requireDeveloperCredentialAccess(ac, in.ID); err != nil {
		return nil, err
	}
	ec := auth.NewExecutionContext(ctx)
	ev, err := usecaseop.Run(ctx, s.UoW, operations.SetDeveloperCredential(s.Repo, s.Roles),
		operations.SetDeveloperCredentialCommand{PrincipalID: ids.PrincipalID(in.ID)}, ec)
	if err != nil {
		return nil, err
	}
	resp := SetDeveloperCredentialResponse{ID: string(ev.UserID)}
	if secret, ok := operations.PopDevClientSecret(string(ev.UserID)); ok {
		resp.ClientSecret = secret
	}
	return &apicommon.Out[SetDeveloperCredentialResponse]{Body: resp}, nil
}

// revokeDeveloperCredential clears the target principal's developer
// client_credentials secret without touching their role assignment. Same
// self-or-admin shape as setDeveloperCredential, including the coarse gate.
func (s *State) revokeDeveloperCredential(ctx context.Context, in *apicommon.IDInput) (*apicommon.Empty, error) {
	ac := auth.FromContext(ctx)
	if err := requireDeveloperCredentialAccess(ac, in.ID); err != nil {
		return nil, err
	}
	ec := auth.NewExecutionContext(ctx)
	if _, err := usecaseop.Run(ctx, s.UoW, operations.RevokeDeveloperCredential(s.Repo),
		operations.RevokeDeveloperCredentialCommand{PrincipalID: ids.PrincipalID(in.ID)}, ec); err != nil {
		return nil, err
	}
	return &apicommon.Empty{}, nil
}

// requireDeveloperCredentialAccess is the coarse, pre-load gate shared by
// setDeveloperCredential/revokeDeveloperCredential (PR-3(a)): self-targeting
// needs no load (the id is compared to the caller's own, which is already on
// the AuthContext) and passes with the self-service developer-credential
// permission; targeting someone else needs the general user-admin write
// permission. Either way SOME permission is required before the target is
// ever loaded — the per-resource fine-grained checks (developer role,
// requester-vs-admin client scope) stay in the use case's Execute, post-load.
func requireDeveloperCredentialAccess(ac *auth.AuthContext, targetID string) error {
	if ac != nil && ac.PrincipalID == targetID {
		return auth.CanManageOwnDeveloperCredential(ac)
	}
	return auth.CanUpdatePrincipals(ac)
}
