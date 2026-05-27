package operations

import (
	"context"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/commit"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

type UpdateCommand struct {
	ID        string  `json:"id"`
	Name      *string `json:"name,omitempty"`
	FirstName *string `json:"firstName,omitempty"`
	LastName  *string `json:"lastName,omitempty"`
	Phone     *string `json:"phone,omitempty"`
}

func UpdateUser(
	ctx context.Context,
	repo *principal.Repository,
	uow *usecasepgx.UnitOfWork,
	cmd UpdateCommand,
	ec usecase.ExecutionContext,
) (commit.Committed[UserUpdated], error) {
	var zero commit.Committed[UserUpdated]
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
		return zero, httperror.NotFound("Principal", cmd.ID)
	}
	if cmd.Name != nil {
		p.Name = strings.TrimSpace(*cmd.Name)
	}
	if p.UserIdentity != nil {
		if cmd.FirstName != nil {
			p.UserIdentity.FirstName = cmd.FirstName
		}
		if cmd.LastName != nil {
			p.UserIdentity.LastName = cmd.LastName
		}
		if cmd.Phone != nil {
			p.UserIdentity.Phone = cmd.Phone
		}
	}

	event := UserUpdated{
		Metadata: usecase.NewEventMetadata(ec, UserUpdatedType, Source, subjectFor(p.ID)),
		UserID:   p.ID,
		Name:     p.Name,
	}
	return commit.Save(ctx, uow, p, repo, event, cmd)
}
