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

type ActivateCommand struct {
	ID string `json:"id"`
}

func ActivateUser(
	ctx context.Context,
	repo *principal.Repository,
	uow *usecasepgx.UnitOfWork,
	cmd ActivateCommand,
	ec usecase.ExecutionContext,
) (commit.Committed[UserActivated], error) {
	var zero commit.Committed[UserActivated]
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
	p.Activate()
	event := UserActivated{
		Metadata: usecase.NewEventMetadata(ec, UserActivatedType, Source, subjectFor(p.ID)),
		UserID:   p.ID,
	}
	return commit.Save(ctx, uow, p, repo, event, cmd)
}
