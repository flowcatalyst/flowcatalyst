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
	ID     string  `json:"id"`
	Name   *string `json:"name,omitempty"`
	Active *bool   `json:"active,omitempty"`
	Email  *string `json:"email,omitempty"`
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
	if cmd.Active != nil {
		p.Active = *cmd.Active
	}
	if cmd.Email != nil {
		// Email is the identity key (login, scope derivation, and the sync's own
		// findByEmail match key). Accept it so callers can PUT a full object, but
		// only as a stable assertion: a different value is an identity change this
		// endpoint deliberately does not perform.
		got := strings.ToLower(strings.TrimSpace(*cmd.Email))
		cur := ""
		if p.UserIdentity != nil {
			cur = strings.ToLower(strings.TrimSpace(p.UserIdentity.Email))
		}
		if got != "" && got != cur {
			return zero, usecase.Validation("EMAIL_IMMUTABLE",
				"email cannot be changed here; it is the principal's identity")
		}
	}

	event := UserUpdated{
		Metadata: usecase.NewEventMetadata(ec, UserUpdatedType, Source, subjectFor(p.ID)),
		UserID:   p.ID,
		Name:     p.Name,
	}
	return commit.Save(ctx, uow, p, repo, event, cmd)
}
