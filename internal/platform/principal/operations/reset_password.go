package operations

import (
	"context"
	"fmt"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth/passwordhash"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecaseop"
)

type ResetPasswordCommand struct {
	ID          string `json:"id"`
	NewPassword string `json:"newPassword"`
	// EnforcePasswordComplexity defaults to true when nil. When false the caller
	// owns its own password policy, so we relax the minimum length (1:1 with
	// Rust's relaxed() policy). Go has no upper/lower/digit/special complexity
	// checks, so the only effect today is the minimum-length floor.
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
			// Minimum length follows the complexity flag: the strict default requires 8
			// (Rust PasswordPolicy::default min_length), an opt-out relaxes to 2 (Rust
			// PasswordPolicy::relaxed). enforce defaults to true when the flag is absent.
			minLen := 8
			if cmd.EnforcePasswordComplexity != nil && !*cmd.EnforcePasswordComplexity {
				minLen = 2
			}
			if len(cmd.NewPassword) < minLen {
				return usecase.Validation("PASSWORD_TOO_SHORT",
					fmt.Sprintf("newPassword must be at least %d characters", minLen))
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
