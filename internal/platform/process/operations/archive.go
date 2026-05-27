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

// ArchiveCommand is the input DTO.
type ArchiveCommand struct {
	ID string `json:"id"`
}

// ArchiveProcess marks a process archived and emits [ProcessArchived].
func ArchiveProcess(
	ctx context.Context,
	repo *process.Repository,
	uow *usecasepgx.UnitOfWork,
	cmd ArchiveCommand,
	ec usecase.ExecutionContext,
) (commit.Committed[ProcessArchived], error) {
	var zero commit.Committed[ProcessArchived]

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
	p.Archive()
	event := ProcessArchived{
		Metadata:  usecase.NewEventMetadata(ec, ProcessArchivedType, Source, subjectFor(p.ID)),
		ProcessID: p.ID,
		Code:      p.Code,
	}
	return commit.Save(ctx, uow, p, repo, event, cmd)
}
