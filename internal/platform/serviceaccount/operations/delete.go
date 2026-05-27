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

// DeleteCommand is the input DTO.
type DeleteCommand struct {
	ID string `json:"id"`
}

// DeleteServiceAccount removes a service account and emits [ServiceAccountDeleted].
func DeleteServiceAccount(
	ctx context.Context,
	repo *serviceaccount.Repository,
	uow *usecasepgx.UnitOfWork,
	cmd DeleteCommand,
	ec usecase.ExecutionContext,
) (commit.Committed[ServiceAccountDeleted], error) {
	var zero commit.Committed[ServiceAccountDeleted]

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
	event := ServiceAccountDeleted{
		Metadata:         usecase.NewEventMetadata(ec, ServiceAccountDeletedType, Source, subjectFor(sa.ID)),
		ServiceAccountID: sa.ID,
		Code:             sa.Code,
	}
	return commit.Delete(ctx, uow, sa, repo, event, cmd)
}
