package operations

import (
	"context"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/process"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/commit"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

// DeleteCommand is the input DTO.
type DeleteCommand struct {
	ID string `json:"id"`
}

// DeleteProcess removes a process and emits [ProcessDeleted].
func DeleteProcess(
	ctx context.Context,
	repo *process.Repository,
	uow *usecasepgx.UnitOfWork,
	cmd DeleteCommand,
	ec usecase.ExecutionContext,
) (commit.Committed[ProcessDeleted], error) {
	var zero commit.Committed[ProcessDeleted]

	if strings.TrimSpace(cmd.ID) == "" {
		return zero, usecase.Validation("ID_REQUIRED", "id is required")
	}

	p, err := repo.FindByID(ctx, cmd.ID)
	if err != nil {
		return zero, usecase.Internal("REPO", "find_by_id failed", err)
	}
	if p == nil {
		return zero, httperror.NotFound("Process", cmd.ID)
	}
	event := ProcessDeleted{
		Metadata:  usecase.NewEventMetadata(ec, ProcessDeletedType, Source, subjectFor(p.ID)),
		ProcessID: p.ID,
		Code:      p.Code,
	}
	return commit.Delete(ctx, uow, p, repo, event, cmd)
}
