package operations

import (
	"context"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/commit"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

// DeleteCommand is the input DTO.
type DeleteCommand struct {
	ID string `json:"id"`
}

// DeleteApplication removes an application and emits [ApplicationDeleted].
func DeleteApplication(
	ctx context.Context,
	repo *application.Repository,
	uow *usecasepgx.UnitOfWork,
	cmd DeleteCommand,
	ec usecase.ExecutionContext,
) (commit.Committed[ApplicationDeleted], error) {
	var zero commit.Committed[ApplicationDeleted]

	if strings.TrimSpace(cmd.ID) == "" {
		return zero, usecase.Validation("ID_REQUIRED", "id is required")
	}

	a, err := repo.FindByID(ctx, cmd.ID)
	if err != nil {
		return zero, usecase.Internal("REPO", "find_by_id failed", err)
	}
	if a == nil {
		return zero, httperror.NotFound("Application", cmd.ID)
	}
	event := ApplicationDeleted{
		Metadata:      usecase.NewEventMetadata(ec, ApplicationDeletedType, Source, subjectFor(a.ID)),
		ApplicationID: a.ID,
		Code:          a.Code,
	}
	return commit.Delete(ctx, uow, a, repo, event, cmd)
}
