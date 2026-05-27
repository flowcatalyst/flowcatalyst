package operations

import (
	"context"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/emaildomainmapping"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/commit"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

// DeleteCommand is the input DTO.
type DeleteCommand struct {
	ID string `json:"id"`
}

// DeleteMapping removes an email-domain mapping and emits
// EmailDomainMappingDeleted.
func DeleteMapping(
	ctx context.Context,
	repo *emaildomainmapping.Repository,
	uow *usecasepgx.UnitOfWork,
	cmd DeleteCommand,
	ec usecase.ExecutionContext,
) (commit.Committed[EmailDomainMappingDeleted], error) {
	var zero commit.Committed[EmailDomainMappingDeleted]

	if strings.TrimSpace(cmd.ID) == "" {
		return zero, usecase.Validation("ID_REQUIRED", "id is required")
	}

	e, err := repo.FindByID(ctx, cmd.ID)
	if err != nil {
		return zero, usecase.Internal("REPO", "find_by_id failed", err)
	}
	if e == nil {
		return zero, httperror.NotFound("EmailDomainMapping", cmd.ID)
	}
	event := EmailDomainMappingDeleted{
		Metadata:    usecase.NewEventMetadata(ec, EmailDomainMappingDeletedType, Source, subjectFor(e.ID)),
		MappingID:   e.ID,
		EmailDomain: e.EmailDomain,
	}
	return commit.Delete(ctx, uow, e, repo, event, cmd)
}
