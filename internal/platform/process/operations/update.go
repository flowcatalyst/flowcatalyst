package operations

import (
	"context"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/process"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/go-sdk/usecase"
	"github.com/flowcatalyst/go-sdk/usecaseop"
)

// UpdateCommand is the input DTO.
type UpdateCommand struct {
	ID          string   `json:"id"`
	Name        *string  `json:"name,omitempty"`
	Description *string  `json:"description,omitempty"`
	Body        *string  `json:"body,omitempty"`
	DiagramType *string  `json:"diagramType,omitempty"`
	Tags        []string `json:"tags,omitempty"`
}

// UpdateProcess mutates mutable fields and emits [ProcessUpdated]. An
// archived process is refused (PROCESS_ARCHIVED); a blank description clears
// it.
func UpdateProcess(repo *process.Repository) usecaseop.Operation[UpdateCommand, ProcessUpdated] {
	return usecaseop.Operation[UpdateCommand, ProcessUpdated]{
		Name: "UpdateProcess",
		Validate: func(_ context.Context, cmd UpdateCommand) error {
			if strings.TrimSpace(cmd.ID) == "" {
				return usecase.Validation("ID_REQUIRED", "id is required")
			}
			if cmd.Name != nil && strings.TrimSpace(*cmd.Name) == "" {
				return usecase.Validation("NAME_REQUIRED", "name cannot be empty")
			}
			return nil
		},
		Authorize: usecaseop.Public[UpdateCommand],
		Execute: func(ctx context.Context, cmd UpdateCommand, ec usecase.ExecutionContext) (usecaseop.Plan[ProcessUpdated], error) {
			p, err := repo.FindByID(ctx, cmd.ID)
			if err != nil {
				return nil, usecase.Internal("REPO", "find_by_id failed", err)
			}
			if p == nil {
				return nil, httperror.NotFound("Process", cmd.ID)
			}
			// An archived process is read-only (the SPA drawer's rule).
			if p.Status == process.StatusArchived {
				return nil, usecase.BusinessRule("PROCESS_ARCHIVED", process.CannotEditReason)
			}
			if cmd.Name != nil {
				p.Name = strings.TrimSpace(*cmd.Name)
			}
			if cmd.Description != nil {
				p.Description = optionalDescription(cmd.Description)
			}
			if cmd.Body != nil {
				p.Body = *cmd.Body
			}
			if cmd.DiagramType != nil {
				p.DiagramType = *cmd.DiagramType
			}
			if cmd.Tags != nil {
				p.Tags = cmd.Tags
			}

			event := ProcessUpdated{
				Metadata:  usecase.NewEventMetadata(ec, ProcessUpdatedType, Source, subjectFor(p.ID)),
				ProcessID: p.ID,
				Name:      p.Name,
			}
			return usecaseop.Save(p, repo, event), nil
		},
	}
}

// optionalDescription stores a blank description as none.
func optionalDescription(d *string) *string {
	if d == nil || strings.TrimSpace(*d) == "" {
		return nil
	}
	return d
}
