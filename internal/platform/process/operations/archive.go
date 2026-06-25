package operations

import (
	"context"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/process"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecaseop"
)

// ArchiveCommand is the input DTO.
type ArchiveCommand struct {
	ID string `json:"id"`
}

// ArchiveProcess marks a process archived and emits [ProcessArchived].
func ArchiveProcess(repo *process.Repository) usecaseop.Operation[ArchiveCommand, ProcessArchived] {
	return usecaseop.Operation[ArchiveCommand, ProcessArchived]{
		Name: "ArchiveProcess",
		Validate: func(_ context.Context, cmd ArchiveCommand) error {
			if strings.TrimSpace(cmd.ID) == "" {
				return usecase.Validation("ID_REQUIRED", "id is required")
			}
			return nil
		},
		Authorize: usecaseop.Public[ArchiveCommand],
		Execute: func(ctx context.Context, cmd ArchiveCommand, ec usecase.ExecutionContext) (usecaseop.Plan[ProcessArchived], error) {
			p, err := repo.FindByID(ctx, cmd.ID)
			if err != nil {
				return nil, usecase.Internal("REPO", "find_by_id failed", err)
			}
			if p == nil {
				return nil, httperror.NotFound("Process", cmd.ID)
			}
			p.Archive()
			event := ProcessArchived{
				Metadata:  usecase.NewEventMetadata(ec, ProcessArchivedType, Source, subjectFor(p.ID)),
				ProcessID: p.ID,
				Code:      p.Code,
			}
			return usecaseop.Save(p, repo, event), nil
		},
	}
}
