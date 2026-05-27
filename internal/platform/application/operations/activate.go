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

// ActivateCommand is the input DTO.
type ActivateCommand struct {
	ID string `json:"id"`
}

// ActivateApplication marks an application active and emits [ApplicationActivated].
func ActivateApplication(
	ctx context.Context,
	repo *application.Repository,
	uow *usecasepgx.UnitOfWork,
	cmd ActivateCommand,
	ec usecase.ExecutionContext,
) (commit.Committed[ApplicationActivated], error) {
	var zero commit.Committed[ApplicationActivated]

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
	a.Activate()
	event := ApplicationActivated{
		Metadata:      usecase.NewEventMetadata(ec, ApplicationActivatedType, Source, subjectFor(a.ID)),
		ApplicationID: a.ID,
	}
	return commit.Save(ctx, uow, a, repo, event, cmd)
}
