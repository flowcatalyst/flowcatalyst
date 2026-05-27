package operations

import (
	"context"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/webauthn"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/commit"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

// RevokeCommand is the input DTO.
type RevokeCommand struct {
	ID string `json:"id"`
}

// Revoke deletes a webauthn credential and emits [PasskeyRevoked].
func Revoke(
	ctx context.Context,
	creds *webauthn.Repository,
	uow *usecasepgx.UnitOfWork,
	cmd RevokeCommand,
	ec usecase.ExecutionContext,
) (commit.Committed[PasskeyRevoked], error) {
	var zero commit.Committed[PasskeyRevoked]

	if strings.TrimSpace(cmd.ID) == "" {
		return zero, usecase.Validation("ID_REQUIRED", "id is required")
	}

	c, err := creds.FindByID(ctx, cmd.ID)
	if err != nil {
		return zero, usecase.Internal("REPO", "find_by_id failed", err)
	}
	if c == nil {
		return zero, httperror.NotFound("WebauthnCredential", cmd.ID)
	}
	event := PasskeyRevoked{
		Metadata:     usecase.NewEventMetadata(ec, PasskeyRevokedType, Source, subjectFor(c.ID)),
		CredentialID: c.ID,
		UserID:       c.PrincipalID,
	}
	return commit.Delete(ctx, uow, c, creds, event, cmd)
}
