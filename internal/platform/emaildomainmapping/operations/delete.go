package operations

import (
	"context"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/emaildomainmapping"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecaseop"
)

// DeleteCommand is the input DTO.
type DeleteCommand struct {
	ID string `json:"id"`
}

// DeleteMapping removes an email-domain mapping and emits
// EmailDomainMappingDeleted. The coarse anchor check lives on the controller;
// email-domain mappings have no per-client resource dimension, so the use case
// carries no resource-level authz (Authorize = usecaseop.Public).
func DeleteMapping(repo *emaildomainmapping.Repository) usecaseop.Operation[DeleteCommand, EmailDomainMappingDeleted] {
	return usecaseop.Operation[DeleteCommand, EmailDomainMappingDeleted]{
		Name: "DeleteMapping",
		Validate: func(_ context.Context, cmd DeleteCommand) error {
			if strings.TrimSpace(cmd.ID) == "" {
				return usecase.Validation("ID_REQUIRED", "id is required")
			}
			return nil
		},
		Authorize: usecaseop.Public[DeleteCommand],
		Execute: func(ctx context.Context, cmd DeleteCommand, ec usecase.ExecutionContext) (usecaseop.Plan[EmailDomainMappingDeleted], error) {
			e, err := repo.FindByID(ctx, cmd.ID)
			if err != nil {
				return nil, usecase.Internal("REPO", "find_by_id failed", err)
			}
			if e == nil {
				return nil, httperror.NotFound("EmailDomainMapping", cmd.ID)
			}
			event := EmailDomainMappingDeleted{
				Metadata:    usecase.NewEventMetadata(ec, EmailDomainMappingDeletedType, Source, subjectFor(e.ID)),
				MappingID:   e.ID,
				EmailDomain: e.EmailDomain,
			}
			return usecaseop.Delete(e, repo, event), nil
		},
	}
}
