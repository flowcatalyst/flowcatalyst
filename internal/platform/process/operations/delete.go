package operations

import (
	"context"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/process"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecaseop"
)

// DeleteCommand is the input DTO.
type DeleteCommand struct {
	ID string `json:"id"`
}

// DeleteProcess removes a process and emits [ProcessDeleted].
func DeleteProcess(repo *process.Repository) usecaseop.Operation[DeleteCommand, ProcessDeleted] {
	return usecaseop.Operation[DeleteCommand, ProcessDeleted]{
		Name: "DeleteProcess",
		Validate: func(_ context.Context, cmd DeleteCommand) error {
			if strings.TrimSpace(cmd.ID) == "" {
				return usecase.Validation("ID_REQUIRED", "id is required")
			}
			return nil
		},
		// The coarse delete permission is enforced at the controller; process
		// is global (no per-client resource dimension), so there is no
		// resource-level authorization here.
		Authorize: usecaseop.Public[DeleteCommand],
		Execute: func(ctx context.Context, cmd DeleteCommand, ec usecase.ExecutionContext) (usecaseop.Plan[ProcessDeleted], error) {
			p, err := repo.FindByID(ctx, cmd.ID)
			if err != nil {
				return nil, usecase.Internal("REPO", "find_by_id failed", err)
			}
			if p == nil {
				return nil, httperror.NotFound("Process", cmd.ID)
			}
			event := ProcessDeleted{
				Metadata:  usecase.NewEventMetadata(ec, ProcessDeletedType, Source, subjectFor(p.ID)),
				ProcessID: p.ID,
				Code:      p.Code,
			}
			return usecaseop.Delete(p, repo, event), nil
		},
	}
}
