package operations

import (
	"context"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth/passwordhash"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

// ResetPasswordCommand is the input DTO. The new plaintext password is
// supplied; we hash it in Execute. The Rust version handles two flows:
// admin-initiated reset (use this) and user-initiated reset via a token
// (which lives in the password_reset subdomain, Wave 3e).
type ResetPasswordCommand struct {
	ID          string `json:"id"`
	NewPassword string `json:"newPassword"`
}

// ResetPasswordUseCase implements UseCase.
type ResetPasswordUseCase struct {
	repo *principal.Repository
	uow  *usecasepgx.UnitOfWork
}

// NewResetPasswordUseCase wires the use case.
func NewResetPasswordUseCase(repo *principal.Repository, uow *usecasepgx.UnitOfWork) *ResetPasswordUseCase {
	return &ResetPasswordUseCase{repo: repo, uow: uow}
}

func (uc *ResetPasswordUseCase) Validate(_ context.Context, cmd ResetPasswordCommand) error {
	if strings.TrimSpace(cmd.ID) == "" {
		return usecase.Validation("ID_REQUIRED", "id is required")
	}
	if len(cmd.NewPassword) < 8 {
		return usecase.Validation("PASSWORD_TOO_SHORT", "newPassword must be at least 8 characters")
	}
	return nil
}

func (uc *ResetPasswordUseCase) Authorize(_ context.Context, _ ResetPasswordCommand, _ usecase.ExecutionContext) error {
	return nil
}

func (uc *ResetPasswordUseCase) Execute(ctx context.Context, cmd ResetPasswordCommand, ec usecase.ExecutionContext) usecase.Result[UserPasswordReset] {
	p, err := uc.repo.FindByID(ctx, cmd.ID)
	if err != nil {
		return usecase.Failure[UserPasswordReset](usecase.Internal("REPO", "find_by_id failed", err))
	}
	if p == nil {
		return usecase.Failure[UserPasswordReset](httperror.NotFound("Principal", cmd.ID))
	}
	if p.Type != principal.TypeUser {
		return usecase.Failure[UserPasswordReset](usecase.Conflict(
			"NOT_A_USER", "Password reset only applies to USER principals"))
	}
	hash, err := passwordhash.Hash(cmd.NewPassword)
	if err != nil {
		return usecase.Failure[UserPasswordReset](usecase.Internal("HASH", "password hash failed", err))
	}
	p.SetPasswordHash(hash)

	event := UserPasswordReset{
		Metadata: usecase.NewEventMetadata(ec, UserPasswordResetType, Source, subjectFor(p.ID)),
		UserID:   p.ID,
	}
	return usecasepgx.Commit[principal.Principal, UserPasswordReset, ResetPasswordCommand](
		ctx, uc.uow, p, uc.repo, event, cmd,
	)
}

var _ usecase.UseCase[ResetPasswordCommand, UserPasswordReset] = (*ResetPasswordUseCase)(nil)
