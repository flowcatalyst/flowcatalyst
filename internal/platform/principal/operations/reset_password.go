package operations

import (
	"context"
	"fmt"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth/passwordhash"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth/passwordpolicy"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecaseop"
)

type ResetPasswordCommand struct {
	ID          string `json:"id"`
	NewPassword string `json:"newPassword"`
	// EnforcePasswordComplexity defaults to true when nil: the full
	// passwordpolicy runs (length bounds, common-password blocklist, not
	// derived from the user's email/name). When false the caller owns its own
	// password policy — SDK applications may set any password ≥ 2 chars
	// (owner decision to keep the API fully permissive for external apps).
	EnforcePasswordComplexity *bool `json:"enforcePasswordComplexity,omitempty"`
}

// ResetPassword hashes and sets a user principal's password and emits
// [UserPasswordReset].
//
// Authorize is intentionally Public: this op is invoked by TWO entry points
// with DIFFERENT gating — the admin controller (coarse CanWritePrincipals +
// per-resource requireScopeByID, done in the handler) AND the UNAUTHENTICATED
// token-gated password-reset confirm flow (passwordreset/api, gated by a valid
// single-use reset token). Baking an admin check in here would break the
// self-service password-reset path, so authorization stays at each caller.
func ResetPassword(repo *principal.Repository) usecaseop.Operation[ResetPasswordCommand, UserPasswordReset] {
	return usecaseop.Operation[ResetPasswordCommand, UserPasswordReset]{
		Name: "ResetPassword",
		Validate: func(_ context.Context, cmd ResetPasswordCommand) error {
			if strings.TrimSpace(cmd.ID) == "" {
				return usecase.Validation("ID_REQUIRED", "id is required")
			}
			// The relaxed (SDK) path checks only its minimal floor here; the strict
			// path's cheap length bounds run here too, with the identity-aware
			// checks in Execute once the principal's email/name are loaded.
			if cmd.EnforcePasswordComplexity != nil && !*cmd.EnforcePasswordComplexity {
				if len(cmd.NewPassword) < 2 {
					return usecase.Validation("PASSWORD_TOO_SHORT",
						"newPassword must be at least 2 characters")
				}
				return nil
			}
			if len(cmd.NewPassword) < passwordpolicy.MinLength {
				return usecase.Validation("PASSWORD_TOO_SHORT",
					fmt.Sprintf("newPassword must be at least %d characters", passwordpolicy.MinLength))
			}
			return nil
		},
		Authorize: usecaseop.Public[ResetPasswordCommand],
		Execute: func(ctx context.Context, cmd ResetPasswordCommand, ec usecase.ExecutionContext) (usecaseop.Plan[UserPasswordReset], error) {
			p, err := repo.FindByID(ctx, cmd.ID)
			if err != nil {
				return nil, usecase.Internal("REPO", "find_by_id failed", err)
			}
			if p == nil {
				return nil, httperror.NotFound("Principal", cmd.ID)
			}
			if p.Type != principal.TypeUser {
				return nil, usecase.Conflict("NOT_A_USER", "Password reset only applies to USER principals")
			}
			// Strict path: full policy, now that the principal's identity is
			// known. The relaxed SDK path skips it (the caller owns its policy).
			if cmd.EnforcePasswordComplexity == nil || *cmd.EnforcePasswordComplexity {
				email := ""
				if p.UserIdentity != nil {
					email = p.UserIdentity.Email
				}
				if v := passwordpolicy.Validate(cmd.NewPassword, email, p.Name); v != nil {
					return nil, usecase.Validation(v.Code, v.Message)
				}
			}
			hash, err := passwordhash.Hash(cmd.NewPassword)
			if err != nil {
				return nil, usecase.Internal("HASH", "password hash failed", err)
			}
			p.SetPasswordHash(hash)

			event := UserPasswordReset{
				Metadata: usecase.NewEventMetadata(ec, UserPasswordResetType, Source, subjectFor(p.ID)),
				UserID:   p.ID,
			}
			return usecaseop.Save(p, repo, event), nil
		},
	}
}
