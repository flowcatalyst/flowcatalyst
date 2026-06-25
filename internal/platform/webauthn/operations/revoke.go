package operations

import (
	"context"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/webauthn"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecaseop"
)

// RevokeCommand is the input DTO.
type RevokeCommand struct {
	ID string `json:"id"`
}

// Revoke deletes a webauthn credential and emits [PasskeyRevoked].
func Revoke(creds *webauthn.Repository) usecaseop.Operation[RevokeCommand, PasskeyRevoked] {
	return usecaseop.Operation[RevokeCommand, PasskeyRevoked]{
		Name: "Revoke",
		Validate: func(_ context.Context, cmd RevokeCommand) error {
			if strings.TrimSpace(cmd.ID) == "" {
				return usecase.Validation("ID_REQUIRED", "id is required")
			}
			return nil
		},
		// Intentionally open: ownership is enforced by the calling handler
		// (deleteCredential), which loads the caller's own credentials and
		// returns NotFound — not Forbidden, to avoid revealing other users'
		// credentials — unless the target id is among them, before invoking
		// Revoke. That anti-enumeration check is handler-specific (it needs the
		// caller's credential list and the NotFound shape), so it stays in the
		// handler; the operation itself has no additional permission gate.
		Authorize: usecaseop.Public[RevokeCommand],
		Execute: func(ctx context.Context, cmd RevokeCommand, ec usecase.ExecutionContext) (usecaseop.Plan[PasskeyRevoked], error) {
			c, err := creds.FindByID(ctx, cmd.ID)
			if err != nil {
				return nil, usecase.Internal("REPO", "find_by_id failed", err)
			}
			if c == nil {
				return nil, httperror.NotFound("WebauthnCredential", cmd.ID)
			}
			event := PasskeyRevoked{
				Metadata:     usecase.NewEventMetadata(ec, PasskeyRevokedType, Source, subjectFor(c.ID)),
				CredentialID: c.ID,
				UserID:       c.PrincipalID,
			}
			return usecaseop.Delete(c, creds, event), nil
		},
	}
}
