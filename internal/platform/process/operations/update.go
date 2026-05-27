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

// UpdateCommand is the input DTO.
type UpdateCommand struct {
	ID          string   `json:"id"`
	Name        *string  `json:"name,omitempty"`
	Description *string  `json:"description,omitempty"`
	Body        *string  `json:"body,omitempty"`
	DiagramType *string  `json:"diagramType,omitempty"`
	Tags        []string `json:"tags,omitempty"`
}

// UpdateProcess mutates mutable fields and emits [ProcessUpdated].
func UpdateProcess(
	ctx context.Context,
	repo *process.Repository,
	uow *usecasepgx.UnitOfWork,
	cmd UpdateCommand,
	ec usecase.ExecutionContext,
) (commit.Committed[ProcessUpdated], error) {
	var zero commit.Committed[ProcessUpdated]

	if strings.TrimSpace(cmd.ID) == "" {
		return zero, usecase.Validation("ID_REQUIRED", "id is required")
	}
	if cmd.Name != nil && strings.TrimSpace(*cmd.Name) == "" {
		return zero, usecase.Validation("NAME_REQUIRED", "name cannot be empty")
	}

	p, err := repo.FindByID(ctx, cmd.ID)
	if err != nil {
		return zero, usecase.Internal("REPO", "find_by_id failed", err)
	}
	if p == nil {
		return zero, httperror.NotFound("Process", cmd.ID)
	}
	if cmd.Name != nil {
		p.Name = strings.TrimSpace(*cmd.Name)
	}
	if cmd.Description != nil {
		p.Description = cmd.Description
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
	return commit.Save(ctx, uow, p, repo, event, cmd)
}
