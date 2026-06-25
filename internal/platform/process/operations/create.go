package operations

import (
	"context"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/process"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecaseop"
)

// CreateCommand is the input DTO.
type CreateCommand struct {
	Code        string   `json:"code"`
	Name        string   `json:"name"`
	Description *string  `json:"description,omitempty"`
	Body        string   `json:"body,omitempty"`
	DiagramType string   `json:"diagramType,omitempty"`
	Tags        []string `json:"tags,omitempty"`
}

// CreateProcess validates cmd, enforces code uniqueness, persists the
// process, and emits [ProcessCreated]. The coarse write permission is
// enforced at the controller; process is global (no per-client resource
// dimension), so there is no resource-level authorization here.
func CreateProcess(repo *process.Repository) usecaseop.Operation[CreateCommand, ProcessCreated] {
	return usecaseop.Operation[CreateCommand, ProcessCreated]{
		Name: "CreateProcess",
		Validate: func(_ context.Context, cmd CreateCommand) error {
			if strings.TrimSpace(cmd.Code) == "" {
				return usecase.Validation("CODE_REQUIRED", "Process code is required")
			}
			parts := strings.Split(cmd.Code, ":")
			if len(parts) != 3 {
				return usecase.Validation("INVALID_CODE_FORMAT",
					"Process code must follow format: application:subdomain:process-name")
			}
			for _, p := range parts {
				if strings.TrimSpace(p) == "" {
					return usecase.Validation("INVALID_CODE_FORMAT", "Process code segments cannot be empty")
				}
			}
			if strings.TrimSpace(cmd.Name) == "" {
				return usecase.Validation("NAME_REQUIRED", "Process name is required")
			}
			return nil
		},
		Authorize: usecaseop.Public[CreateCommand],
		Execute: func(ctx context.Context, cmd CreateCommand, ec usecase.ExecutionContext) (usecaseop.Plan[ProcessCreated], error) {
			existing, err := repo.FindByCode(ctx, cmd.Code)
			if err != nil {
				return nil, usecase.Internal("REPO", "find_by_code failed", err)
			}
			if existing != nil {
				return nil, usecase.Conflict(
					"CODE_EXISTS",
					"Process with code '"+cmd.Code+"' already exists")
			}

			p, err := process.New(cmd.Code, strings.TrimSpace(cmd.Name))
			if err != nil {
				return nil, usecase.Validation("INVALID_CODE_FORMAT", err.Error())
			}
			p.Description = cmd.Description
			p.Body = cmd.Body
			if cmd.DiagramType != "" {
				p.DiagramType = cmd.DiagramType
			}
			if cmd.Tags != nil {
				p.Tags = cmd.Tags
			}
			p.CreatedBy = &ec.PrincipalID

			event := ProcessCreated{
				Metadata:  usecase.NewEventMetadata(ec, ProcessCreatedType, Source, subjectFor(p.ID)),
				ProcessID: p.ID,
				Code:      p.Code,
				Name:      p.Name,
			}
			return usecaseop.Save(p, repo, event), nil
		},
	}
}
