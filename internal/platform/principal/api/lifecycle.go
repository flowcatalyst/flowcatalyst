package api

import (
	"context"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/ids"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/audit"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apicommon"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/jsontime"
	"github.com/flowcatalyst/flowcatalyst-go/internal/tsid"
	"github.com/flowcatalyst/go-sdk/usecase"
	"github.com/flowcatalyst/go-sdk/usecaseop"
)

// getByID returns a single principal. Any authenticated principal may read
// its OWN record (mirrors getVersion's self-bypass below — e.g. the Profile
// page's developer-credential status needs this without granting a plain
// developer CanReadPrincipals); reading anyone else's requires that permission.
func (s *State) getByID(ctx context.Context, in *apicommon.IDInput) (*apicommon.Out[PrincipalResponse], error) {
	ac := auth.FromContext(ctx)
	isSelf := ac != nil && ac.PrincipalID == in.ID
	if !isSelf {
		if err := auth.CanReadPrincipals(ac); err != nil {
			return nil, err
		}
	}
	p, err := s.Repo.FindByID(ctx, ids.PrincipalID(in.ID))
	if err != nil {
		return nil, usecase.Internal("REPO", "find_by_id failed", err)
	}
	if p == nil {
		return nil, httperror.NotFound("Principal", in.ID)
	}
	// Out-of-scope answers the SAME not-found a genuinely missing id would —
	// never 403 — so an unauthorized caller can't use the response to learn
	// whether an id is real (PR-3(b), docs/owner-rulings-todo.md #3).
	if !isSelf && !PrincipalVisible(ac, p) {
		return nil, httperror.NotFound("Principal", in.ID)
	}
	resp := forCaller(ac, fromEntity(p))
	// Enrich with the user's confirmed second factors so the admin detail view
	// can show whether 2FA is enrolled (and which kind). Best-effort: an MFA
	// lookup failure just leaves the list empty rather than failing the read.
	if s.MFA != nil {
		if methods, err := s.MFA.ConfirmedMethods(ctx, string(p.ID)); err == nil {
			out := make([]string, 0, len(methods))
			for _, m := range methods {
				out = append(out, string(m))
			}
			resp.TwoFactorMethods = out
		}
	}
	return &apicommon.Out[PrincipalResponse]{Body: resp}, nil
}

// getVersion reports when the principal (or a role it holds) last changed.
// Any authenticated principal may check its own version — this is what an
// SDK's per-request opt-in revocation check calls — an admin (CanReadPrincipals)
// may check any principal's.
func (s *State) getVersion(ctx context.Context, in *apicommon.IDInput) (*apicommon.Out[PrincipalVersionResponse], error) {
	ac := auth.FromContext(ctx)
	isSelf := ac != nil && ac.PrincipalID == in.ID
	if !isSelf {
		if err := auth.CanReadPrincipals(ac); err != nil {
			return nil, err
		}
		// PR-4: the same client-scope check as the by-id read — an admin
		// checking another tenant's principal answers 404, byte-identical to
		// not-found, never a distinguishing 403.
		p, err := s.Repo.FindByID(ctx, ids.PrincipalID(in.ID))
		if err != nil {
			return nil, usecase.Internal("REPO", "find_by_id failed", err)
		}
		if p == nil {
			return nil, httperror.NotFound("Principal", in.ID)
		}
		if !PrincipalVisible(ac, p) {
			return nil, httperror.NotFound("Principal", in.ID)
		}
	}
	var (
		at  time.Time
		err error
	)
	if s.Versions != nil {
		at, err = s.Versions.Version(ctx, in.ID)
	} else {
		at, err = s.Repo.LookupVersion(ctx, ids.PrincipalID(in.ID))
	}
	if err != nil {
		return nil, usecase.Internal("REPO", "lookup_version failed", err)
	}
	return &apicommon.Out[PrincipalVersionResponse]{Body: PrincipalVersionResponse{UpdatedAt: jsontime.New(at)}}, nil
}

type updateInput struct {
	ID   ids.PrincipalID `path:"id"`
	Body UpdatePrincipalRequest
}

func (s *State) update(ctx context.Context, in *updateInput) (*apicommon.Out[PrincipalResponse], error) {
	ac := auth.FromContext(ctx)
	// Coarse permission at the controller; the use case enforces the per-resource
	// scope check (blockNonClientTarget + CheckScopeAccess) post-load.
	if err := auth.CanUpdatePrincipals(ac); err != nil {
		return nil, err
	}
	ec := auth.NewExecutionContext(ctx)
	if _, err := usecaseop.Run(ctx, s.UoW, operations.UpdateUser(s.Repo), in.Body.toCommand(in.ID), ec); err != nil {
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

func (s *State) activate(ctx context.Context, in *apicommon.IDInput) (*apicommon.Out[apicommon.StatusChangeResponse], error) {
	// Coarse permission at the controller; the use case enforces the per-resource
	// scope check post-load.
	if err := auth.CanUpdatePrincipals(auth.FromContext(ctx)); err != nil {
		return nil, err
	}
	ec := auth.NewExecutionContext(ctx)
	if _, err := usecaseop.Run(ctx, s.UoW, operations.ActivateUser(s.Repo), operations.ActivateCommand{ID: ids.PrincipalID(in.ID)}, ec); err != nil {
		return nil, err
	}
	return &apicommon.Out[apicommon.StatusChangeResponse]{Body: apicommon.StatusChangeResponse{Message: "Principal activated"}}, nil
}

func (s *State) deactivate(ctx context.Context, in *apicommon.IDInput) (*apicommon.Out[apicommon.StatusChangeResponse], error) {
	// Coarse permission at the controller; the use case enforces the per-resource
	// scope check post-load.
	if err := auth.CanUpdatePrincipals(auth.FromContext(ctx)); err != nil {
		return nil, err
	}
	ec := auth.NewExecutionContext(ctx)
	if _, err := usecaseop.Run(ctx, s.UoW, operations.DeactivateUser(s.Repo), operations.DeactivateCommand{ID: ids.PrincipalID(in.ID)}, ec); err != nil {
		return nil, err
	}
	return &apicommon.Out[apicommon.StatusChangeResponse]{Body: apicommon.StatusChangeResponse{Message: "Principal deactivated"}}, nil
}

type resetPasswordInput struct {
	ID   ids.PrincipalID `path:"id"`
	Body ResetPasswordRequest
}

func (s *State) resetPassword(ctx context.Context, in *resetPasswordInput) (*apicommon.Out[apicommon.StatusChangeResponse], error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanUpdatePrincipals(ac); err != nil {
		return nil, err
	}
	target, err := s.loadScoped(ctx, ac, in.ID)
	if err != nil {
		return nil, err
	}
	// The new password signs in as the target: the role ceiling applies (a
	// service principal is refused by the operation, NOT_A_USER).
	if target.Type == principal.TypeUser {
		if err := operations.RequireTargetRolesWithinCaller(ctx, s.Roles, target); err != nil {
			return nil, err
		}
	}
	ec := auth.NewExecutionContext(ctx)
	if _, err := usecaseop.Run(ctx, s.UoW, operations.ResetPassword(s.Repo),
		operations.ResetPasswordCommand{
			ID:                        in.ID,
			NewPassword:               in.Body.NewPassword,
			EnforcePasswordComplexity: in.Body.EnforcePasswordComplexity,
		}, ec); err != nil {
		return nil, err
	}
	return &apicommon.Out[apicommon.StatusChangeResponse]{Body: apicommon.StatusChangeResponse{Message: "Password reset successfully"}}, nil
}

func (s *State) delete(ctx context.Context, in *apicommon.IDInput) (*apicommon.Empty, error) {
	// Coarse permission at the controller; the use case enforces the per-resource
	// scope check post-load.
	if err := auth.CanDeletePrincipals(auth.FromContext(ctx)); err != nil {
		return nil, err
	}
	ec := auth.NewExecutionContext(ctx)
	if _, err := usecaseop.Run(ctx, s.UoW, operations.DeleteUser(s.Repo), operations.DeleteCommand{ID: ids.PrincipalID(in.ID)}, ec); err != nil {
		return nil, err
	}
	return &apicommon.Empty{}, nil
}

// ── send-password-reset (admin trigger) ──────────────────────────────────

// sendPasswordResetInput carries the path id plus an optional body asking to
// also reset the user's 2FA as part of the reset (lost-device recovery). Body
// is a POINTER so the common "just send the reset email" action can POST with
// no body at all: with a non-pointer Body, huma marks the request body
// required and rejects a body-less call with "request body is required".
type sendPasswordResetInput struct {
	ID   ids.PrincipalID `path:"id"`
	Body *struct {
		Reset2FA bool `json:"reset2fa,omitempty"`
	}
}

func (s *State) sendPasswordReset(ctx context.Context, in *sendPasswordResetInput) (*apicommon.Out[apicommon.StatusChangeResponse], error) {
	ac := auth.FromContext(ctx)
	// Coarse permission gate BEFORE any load (PR-3(a)).
	if err := auth.CanUpdatePrincipals(ac); err != nil {
		return nil, err
	}
	p, err := s.Repo.FindByID(ctx, in.ID)
	if err != nil {
		return nil, usecase.Internal("REPO", "find_by_id failed", err)
	}
	if p == nil {
		return nil, httperror.NotFound("Principal", string(in.ID))
	}
	if err := blockNonClientTarget(ac, p); err != nil {
		return nil, err
	}
	// Out-of-scope answers the same not-found error a missing id would (PR-3(b)).
	if !auth.CanAccessScope(ac, ids.StringPtr(p.ClientID)) {
		return nil, httperror.NotFound("Principal", string(in.ID))
	}
	reset2FA := false
	if in.Body != nil {
		reset2FA = in.Body.Reset2FA
	}
	// A reset that also clears the user's 2FA needs the reset-2fa permission
	// on top of user:update.
	if reset2FA {
		if err := auth.CanResetUserTwoFactor(ac); err != nil {
			return nil, err
		}
		// Clearing the second factor lets whoever controls the mailbox sign
		// in as the target: the role ceiling applies.
		if p.IsUser() {
			if err := operations.RequireTargetRolesWithinCaller(ctx, s.Roles, p); err != nil {
				return nil, err
			}
		}
	}
	ec := usecase.NewExecutionContext(ac.PrincipalID)
	if err := operations.SendPasswordReset(ctx, s.Repo, s.PasswordEmailer,
		operations.SendPasswordResetCommand{ID: in.ID, Reset2FA: reset2FA}, ec); err != nil {
		return nil, err
	}
	return &apicommon.Out[apicommon.StatusChangeResponse]{Body: apicommon.StatusChangeResponse{Message: "Password reset email sent"}}, nil
}

// resetTwoFactor clears a user's enrolled 2FA (factors, recovery codes, pending
// PINs, trusted devices). Needs user:reset-2fa and reach to the user's client
// (an anchor, or a client-administrator of that client).
// The user must re-enroll at next sign-in if their domain requires 2FA.
func (s *State) resetTwoFactor(ctx context.Context, in *apicommon.IDInput) (*apicommon.Out[apicommon.StatusChangeResponse], error) {
	ac := auth.FromContext(ctx)
	if s.MFA == nil {
		return nil, usecase.Internal("MFA_NOT_CONFIGURED", "Two-factor service not configured", nil)
	}
	// Coarse permission gate BEFORE any load (PR-3(a)): user:reset-2fa.
	if err := auth.CanResetUserTwoFactor(ac); err != nil {
		return nil, err
	}
	p, err := s.Repo.FindByID(ctx, ids.PrincipalID(in.ID))
	if err != nil {
		return nil, usecase.Internal("REPO", "find_by_id failed", err)
	}
	if p == nil {
		return nil, httperror.NotFound("Principal", in.ID)
	}
	if err := blockNonClientTarget(ac, p); err != nil {
		return nil, err
	}
	// Out-of-scope answers the same not-found error a missing id would (PR-3(b)).
	if !auth.CanAccessScope(ac, ids.StringPtr(p.ClientID)) {
		return nil, httperror.NotFound("Principal", in.ID)
	}
	if !p.IsUser() {
		return nil, usecase.Validation("NOT_USER", "Two-factor reset only applies to user accounts")
	}
	// Clearing the second factor takes the account over for whoever holds its
	// password: the role ceiling applies.
	if err := operations.RequireTargetRolesWithinCaller(ctx, s.Roles, p); err != nil {
		return nil, err
	}
	if err := s.MFA.ResetAll(ctx, string(p.ID)); err != nil {
		return nil, usecase.Internal("MFA", "reset failed", err)
	}
	// A lost-device reset must also end sessions already open under the old
	// factors. The factors are gone, so a failure here is reported, not hidden.
	if _, err := s.Repo.InvalidateSessions(ctx, p.ID); err != nil {
		return nil, usecase.Internal("SESSIONS", "invalidate sessions failed", err)
	}
	if p.UserIdentity != nil {
		s.Notifier.TwoFactorReset(ctx, p.UserIdentity.Email)
	}
	if s.Audit != nil {
		actor := ac.PrincipalID
		_ = s.Audit.Insert(ctx, &audit.Log{
			ID:          tsid.Generate(tsid.AuditLog),
			EntityType:  "PRINCIPAL",
			EntityID:    string(p.ID),
			Operation:   "2FA_RESET_BY_ADMIN",
			PrincipalID: &actor,
			PerformedAt: time.Now().UTC(),
		})
	}
	return &apicommon.Out[apicommon.StatusChangeResponse]{Body: apicommon.StatusChangeResponse{Message: "Two-factor authentication reset"}}, nil
}
