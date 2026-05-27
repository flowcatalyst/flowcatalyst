package operations

import (
	"context"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/platformconfig"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/commit"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

// RevokeAccessCommand is the input DTO.
type RevokeAccessCommand struct {
	ID string `json:"id"`
}

// RevokeAccess removes a platform-config access grant and emits [AccessRevoked].
func RevokeAccess(
	ctx context.Context,
	repo *platformconfig.Repository,
	uow *usecasepgx.UnitOfWork,
	cmd RevokeAccessCommand,
	ec usecase.ExecutionContext,
) (commit.Committed[AccessRevoked], error) {
	var zero commit.Committed[AccessRevoked]

	if strings.TrimSpace(cmd.ID) == "" {
		return zero, usecase.Validation("ID_REQUIRED", "id is required")
	}

	a, err := repo.FindAccessByID(ctx, cmd.ID)
	if err != nil {
		return zero, usecase.Internal("REPO", "find_access_by_id failed", err)
	}
	if a == nil {
		return zero, httperror.NotFound("PlatformConfigAccess", cmd.ID)
	}
	event := AccessRevoked{
		Metadata:        usecase.NewEventMetadata(ec, AccessRevokedType, Source, subjectFor(a.ID)),
		AccessID:        a.ID,
		ApplicationCode: a.ApplicationCode,
		RoleCode:        a.RoleCode,
	}
	return commit.Delete(ctx, uow, a, newAccessRepo(repo), event, cmd)
}
