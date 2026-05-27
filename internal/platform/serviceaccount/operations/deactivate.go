package operations

import (
	"context"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/serviceaccount"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/commit"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

// DeactivateCommand is the input DTO.
type DeactivateCommand struct {
	ID string `json:"id"`
}

// DeactivateServiceAccount marks a service account inactive and emits
// [ServiceAccountDeactivated].
func DeactivateServiceAccount(
	ctx context.Context,
	repo *serviceaccount.Repository,
	uow *usecasepgx.UnitOfWork,
	cmd DeactivateCommand,
	ec usecase.ExecutionContext,
) (commit.Committed[ServiceAccountDeactivated], error) {
	var zero commit.Committed[ServiceAccountDeactivated]

	if strings.TrimSpace(cmd.ID) == "" {
		return zero, usecase.Validation("ID_REQUIRED", "id is required")
	}

	sa, err := repo.FindByID(ctx, cmd.ID)
	if err != nil {
		return zero, usecase.Internal("REPO", "find_by_id failed", err)
	}
	if sa == nil {
		return zero, httperror.NotFound("ServiceAccount", cmd.ID)
	}
	sa.Deactivate()
	event := ServiceAccountDeactivated{
		Metadata:         usecase.NewEventMetadata(ec, ServiceAccountDeactivatedType, Source, subjectFor(sa.ID)),
		ServiceAccountID: sa.ID,
	}
	return commit.Save(ctx, uow, sa, repo, event, cmd)
}
