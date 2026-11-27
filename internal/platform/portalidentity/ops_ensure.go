package portalidentity

import (
	"context"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/client"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/go-sdk/usecase"
	"github.com/flowcatalyst/go-sdk/usecaseop"
)

// Operations are Authorize-Public: the portal-users admin API gates on the
// client-delegable portal permissions at the controller, and the JIT path
// runs as the system actor during an authenticated SSO callback. There is no
// per-resource scope narrower than the client id the controller already
// checked.

type EnsureCommand struct {
	ClientID string  `json:"clientId"`
	Email    string  `json:"email"`
	Name     *string `json:"name,omitempty"`
	Source   string  `json:"source"`
	// PortalAppID, when set, grants the identity that portal app (which must
	// belong to ClientID and be active). The grant's source is Source.
	PortalAppID string `json:"portalAppId,omitempty"`
}

// Ensure idempotently creates — or reactivates — the (client, email) portal
// identity, grants the named portal app, and emits [IdentityEnsured].
// Re-ensuring keeps the original id/source/created_at, any set password and
// any existing grants; a DISABLED identity converges back to ACTIVE
// (suspend-then-reinvite must work). apps may be nil when no caller ever
// passes PortalAppID.
func Ensure(repo *Repository, clients *client.Repository, apps *AppRepository) usecaseop.Operation[EnsureCommand, IdentityEnsured] {
	return usecaseop.Operation[EnsureCommand, IdentityEnsured]{
		Name: "EnsurePortalIdentity",
		Validate: func(_ context.Context, cmd EnsureCommand) error {
			if strings.TrimSpace(cmd.ClientID) == "" {
				return usecase.Validation("CLIENT_ID_REQUIRED", "clientId is required")
			}
			email := strings.TrimSpace(cmd.Email)
			if email == "" {
				return usecase.Validation("EMAIL_REQUIRED", "email is required")
			}
			at := strings.IndexByte(email, '@')
			if at <= 0 || at == len(email)-1 {
				return usecase.Validation("EMAIL_INVALID", "email is not valid")
			}
			return nil
		},
		Authorize: usecaseop.Public[EnsureCommand],
		Execute: func(ctx context.Context, cmd EnsureCommand, ec usecase.ExecutionContext) (usecaseop.Plan[IdentityEnsured], error) {
			c, err := clients.FindByID(ctx, cmd.ClientID)
			if err != nil {
				return nil, usecase.Internal("REPO", "find_client failed", err)
			}
			if c == nil {
				return nil, httperror.NotFound("Client", cmd.ClientID)
			}
			var app *App
			if cmd.PortalAppID != "" {
				if app, err = loadClientApp(ctx, apps, cmd.ClientID, cmd.PortalAppID); err != nil {
					return nil, err
				}
			}

			source := Source(cmd.Source)
			if source != SourceJIT {
				source = SourceInvite
			}
			existing, err := repo.FindByClientAndEmail(ctx, cmd.ClientID, cmd.Email)
			if err != nil {
				return nil, usecase.Internal("REPO", "find_identity failed", err)
			}
			created := existing == nil
			ident := existing
			if ident == nil {
				name := ""
				if cmd.Name != nil {
					name = strings.TrimSpace(*cmd.Name)
				}
				ident = New(cmd.ClientID, cmd.Email, name, source)
			} else {
				ident.Status = StatusActive
				if cmd.Name != nil && strings.TrimSpace(*cmd.Name) != "" {
					ident.Name = strings.TrimSpace(*cmd.Name)
				}
			}
			event := IdentityEnsured{
				Metadata:   usecase.NewEventMetadata(ec, IdentityEnsuredType, EventSource, subjectFor(ident.ID)),
				IdentityID: ident.ID,
				ClientID:   ident.ClientID,
				Email:      ident.Email,
				Created:    created,
				Source_:    string(ident.Source),
			}
			if app != nil {
				ident.Grant(app.ID, source)
				event.AppID, event.AppCode = app.ID, app.Code
			}
			return usecaseop.Save(ident, repo, event), nil
		},
	}
}
