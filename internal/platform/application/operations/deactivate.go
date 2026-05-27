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

// DeactivateCommand is the input DTO.
type DeactivateCommand struct {
	ID string `json:"id"`
}

// DeactivateApplication marks an application inactive and emits [ApplicationDeactivated].
func DeactivateApplication(
	ctx context.Context,
	repo *application.Repository,
	uow *usecasepgx.UnitOfWork,
	cmd DeactivateCommand,
	ec usecase.ExecutionContext,
) (commit.Committed[ApplicationDeactivated], error) {
	var zero commit.Committed[ApplicationDeactivated]

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
	a.Deactivate()
	event := ApplicationDeactivated{
		Metadata:      usecase.NewEventMetadata(ec, ApplicationDeactivatedType, Source, subjectFor(a.ID)),
		ApplicationID: a.ID,
	}
	return commit.Save(ctx, uow, a, repo, event, cmd)
}
