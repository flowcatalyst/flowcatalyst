package operations

import (
	"context"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/commit"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

type DeleteCommand struct {
	ID string `json:"id"`
}

func DeleteUser(
	ctx context.Context,
	repo *principal.Repository,
	uow *usecasepgx.UnitOfWork,
	cmd DeleteCommand,
	ec usecase.ExecutionContext,
) (commit.Committed[UserDeleted], error) {
	var zero commit.Committed[UserDeleted]
	if strings.TrimSpace(cmd.ID) == "" {
		return zero, usecase.Validation("ID_REQUIRED", "id is required")
	}
	p, err := repo.FindByID(ctx, cmd.ID)
	if err != nil {
		return zero, usecase.Internal("REPO", "find_by_id failed", err)
	}
	if p == nil {
		return zero, httperror.NotFound("Principal", cmd.ID)
	}
	email := ""
	if p.UserIdentity != nil {
		email = p.UserIdentity.Email
	}
	event := UserDeleted{
		Metadata: usecase.NewEventMetadata(ec, UserDeletedType, Source, subjectFor(p.ID)),
		UserID:   p.ID,
		Email:    email,
	}
	return commit.Delete(ctx, uow, p, repo, event, cmd)
}
