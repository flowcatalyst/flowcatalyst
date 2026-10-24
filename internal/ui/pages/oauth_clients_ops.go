package pages

import (
	"context"
	"time"

	platformauth "github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth"
	authapi "github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth/api"
	authops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/portalidentity"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecaseop"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecasepgx"
)

// IssuedOAuthClient is a created client with the secret it was issued (empty
// for a PUBLIC client). The secret is read once, from the use case's stash.
type IssuedOAuthClient struct {
	ID       string
	ClientID string
	Secret   string
}

// RotatedOAuthSecret is a rotation's answer: the new secret, and when the
// outgoing one stops being accepted (nil: an immediate cutover).
type RotatedOAuthSecret struct {
	ClientID          string
	Secret            string
	PreviousExpiresAt *time.Time
}

// OAuthClientCommands are the OAuth client operations the page runs. The
// production implementation (OAuthClientOps) runs the same use cases as
// internal/platform/auth/api, so validation, the domain event and the audit
// record are the use case's; the permission gates (anchor scope and the
// operation's permission) are the caller's, as they are the API handlers'.
type OAuthClientCommands interface {
	Create(ctx context.Context, cmd authops.CreateOAuthClientCommand) (IssuedOAuthClient, error)
	Update(ctx context.Context, cmd authops.UpdateOAuthClientCommand) error
	Activate(ctx context.Context, id string) error
	Deactivate(ctx context.Context, id string) error
	Delete(ctx context.Context, id string) error
	// Rotate issues a new secret; a nil grace keeps the outgoing secret for
	// the default 24-hour overlap, zero cuts over at once.
	Rotate(ctx context.Context, id string, graceSeconds *int64) (RotatedOAuthSecret, error)
	// RevokePreviousSecret ends a rotation overlap now.
	RevokePreviousSecret(ctx context.Context, id string) error
}

// OAuthClientOps runs the OAuth client use cases through the unit of work,
// exactly as internal/platform/auth/api does.
type OAuthClientOps struct {
	Repo *platformauth.OAuthClientRepo
	// PortalApps resolves a portal app to its owning client, as the API does.
	PortalApps *portalidentity.AppRepository
	UoW        *usecasepgx.UnitOfWork
}

// Create runs CreateOAuthClient and collects the plaintext secret the use
// case stashed for this client.
func (o OAuthClientOps) Create(ctx context.Context, cmd authops.CreateOAuthClientCommand) (IssuedOAuthClient, error) {
	if err := authapi.ResolvePortalApp(ctx, o.PortalApps, cmd.PortalAppID, &cmd.PortalClientID); err != nil {
		return IssuedOAuthClient{}, err
	}
	event, err := usecaseop.Run(ctx, o.UoW, authops.CreateOAuthClient(o.Repo), cmd, auth.NewExecutionContext(ctx))
	if err != nil {
		return IssuedOAuthClient{}, err
	}
	secret, _ := authops.PopStashedSecret(event.OAuthClientID)
	return IssuedOAuthClient{ID: event.OAuthClientID, ClientID: event.ClientID, Secret: secret}, nil
}

// Update runs UpdateOAuthClient.
func (o OAuthClientOps) Update(ctx context.Context, cmd authops.UpdateOAuthClientCommand) error {
	if err := authapi.ResolvePortalApp(ctx, o.PortalApps, cmd.PortalAppID, &cmd.PortalClientID); err != nil {
		return err
	}
	_, err := usecaseop.Run(ctx, o.UoW, authops.UpdateOAuthClient(o.Repo), cmd, auth.NewExecutionContext(ctx))
	return err
}

// Activate runs ActivateOAuthClient.
func (o OAuthClientOps) Activate(ctx context.Context, id string) error {
	_, err := usecaseop.Run(ctx, o.UoW, authops.ActivateOAuthClient(o.Repo), authops.ActivateOAuthClientCommand{ID: id}, auth.NewExecutionContext(ctx))
	return err
}

// Deactivate runs DeactivateOAuthClient.
func (o OAuthClientOps) Deactivate(ctx context.Context, id string) error {
	_, err := usecaseop.Run(ctx, o.UoW, authops.DeactivateOAuthClient(o.Repo), authops.DeactivateOAuthClientCommand{ID: id}, auth.NewExecutionContext(ctx))
	return err
}

// Delete runs DeleteOAuthClient.
func (o OAuthClientOps) Delete(ctx context.Context, id string) error {
	_, err := usecaseop.Run(ctx, o.UoW, authops.DeleteOAuthClient(o.Repo), authops.DeleteOAuthClientCommand{ID: id}, auth.NewExecutionContext(ctx))
	return err
}

// Rotate runs RotateOAuthClientSecret. The plaintext is collected only after
// the commit: a failed rotation discloses nothing.
func (o OAuthClientOps) Rotate(ctx context.Context, id string, graceSeconds *int64) (RotatedOAuthSecret, error) {
	event, err := usecaseop.Run(ctx, o.UoW, authops.RotateOAuthClientSecret(o.Repo),
		authops.RotateOAuthClientSecretCommand{ID: id, GraceSeconds: graceSeconds}, auth.NewExecutionContext(ctx))
	if err != nil {
		return RotatedOAuthSecret{}, err
	}
	c, err := o.Repo.FindByID(ctx, event.OAuthClientID)
	if err != nil {
		return RotatedOAuthSecret{}, usecase.Internal("REPO", "find_by_id failed", err)
	}
	out := RotatedOAuthSecret{PreviousExpiresAt: event.PreviousSecretExpiresAt}
	if c != nil {
		out.ClientID = c.ClientID
	}
	out.Secret, _ = authops.PopStashedSecret(event.OAuthClientID)
	return out, nil
}

// RevokePreviousSecret runs RevokeOAuthClientPreviousSecret.
func (o OAuthClientOps) RevokePreviousSecret(ctx context.Context, id string) error {
	_, err := usecaseop.Run(ctx, o.UoW, authops.RevokeOAuthClientPreviousSecret(o.Repo),
		authops.RevokeOAuthClientPreviousSecretCommand{ID: id}, auth.NewExecutionContext(ctx))
	return err
}
