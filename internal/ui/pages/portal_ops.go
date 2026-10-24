package pages

import (
	"context"

	platformauth "github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/client"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/portalidentity"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecaseop"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecasepgx"
)

// PortalStore is the slice of the portal repositories the portal pages read
// (GET /api/portal-apps and GET /api/portal-users).
type PortalStore interface {
	Apps(ctx context.Context, clientID string) ([]portalidentity.App, error)
	AppByCode(ctx context.Context, clientID, code string) (*portalidentity.App, error)
	GrantCounts(ctx context.Context, appIDs []string) (map[string]int, error)
	LinkedOAuthClients(ctx context.Context, appIDs []string) (map[string][]portalidentity.LinkedOAuthClient, error)
	CountUnassigned(ctx context.Context, clientID string) (int64, error)
	Search(ctx context.Context, f portalidentity.SearchFilter) ([]portalidentity.Identity, int64, error)
	Identity(ctx context.Context, id string) (*portalidentity.Identity, error)
}

// PortalReader reads the portal identity and portal app repositories.
type PortalReader struct {
	Identities *portalidentity.Repository
	PortalApps *portalidentity.AppRepository
}

// Apps lists a client's portal apps.
func (r PortalReader) Apps(ctx context.Context, clientID string) ([]portalidentity.App, error) {
	return r.PortalApps.FindByClient(ctx, clientID)
}

// AppByCode finds one of a client's portal apps by code.
func (r PortalReader) AppByCode(ctx context.Context, clientID, code string) (*portalidentity.App, error) {
	return r.PortalApps.FindByClientAndCode(ctx, clientID, code)
}

// GrantCounts counts the identities granted each app.
func (r PortalReader) GrantCounts(ctx context.Context, appIDs []string) (map[string]int, error) {
	return r.PortalApps.GrantCounts(ctx, appIDs)
}

// LinkedOAuthClients lists the OAuth clients fronting each app.
func (r PortalReader) LinkedOAuthClients(ctx context.Context, appIDs []string) (map[string][]portalidentity.LinkedOAuthClient, error) {
	return r.PortalApps.LinkedOAuthClients(ctx, appIDs)
}

// CountUnassigned counts a client's identities granted no portal app.
func (r PortalReader) CountUnassigned(ctx context.Context, clientID string) (int64, error) {
	return r.Identities.CountUnassigned(ctx, clientID)
}

// Search lists a client's portal identities.
func (r PortalReader) Search(ctx context.Context, f portalidentity.SearchFilter) ([]portalidentity.Identity, int64, error) {
	return r.Identities.Search(ctx, f)
}

// Identity finds one portal identity by id.
func (r PortalReader) Identity(ctx context.Context, id string) (*portalidentity.Identity, error) {
	return r.Identities.FindByID(ctx, id)
}

// PortalCommands are the portal operations the pages run; PortalOps runs the
// API's use cases. The permission check (CanManagePortalUsers for the
// client) is the page's, as it is the API handler's: the use cases are
// Authorize-Public.
type PortalCommands interface {
	CreateApp(ctx context.Context, cmd portalidentity.CreateAppWithOAuthClientCommand) (portalidentity.CreateAppWithOAuthClientResult, error)
	UpdateApp(ctx context.Context, cmd portalidentity.UpdateAppCommand) error
	DeleteApp(ctx context.Context, cmd portalidentity.DeleteAppCommand) error
	AssignUnassigned(ctx context.Context, cmd portalidentity.AssignUnassignedCommand) (portalidentity.AssignUnassignedResult, error)
	SetStatus(ctx context.Context, cmd portalidentity.SetStatusCommand) error
	DeleteUser(ctx context.Context, cmd portalidentity.DeleteCommand) error
	GrantApp(ctx context.Context, cmd portalidentity.AppGrantCommand) error
	RevokeApp(ctx context.Context, cmd portalidentity.AppGrantCommand) error
}

// PortalOps runs the portal use cases through the unit of work, exactly as
// internal/platform/portalidentity/api does.
type PortalOps struct {
	Identities   *portalidentity.Repository
	Apps         *portalidentity.AppRepository
	Clients      *client.Repository
	OAuthClients *platformauth.OAuthClientRepo
	UoW          *usecasepgx.UnitOfWork
}

// CreateApp runs CreatePortalAppWithOAuthClient.
func (o PortalOps) CreateApp(ctx context.Context, cmd portalidentity.CreateAppWithOAuthClientCommand) (portalidentity.CreateAppWithOAuthClientResult, error) {
	return usecaseop.RunTx(ctx, o.UoW, portalidentity.CreateAppWithOAuthClient(o.Apps, o.Clients, o.OAuthClients), cmd, auth.NewExecutionContext(ctx))
}

// UpdateApp runs UpdatePortalApp.
func (o PortalOps) UpdateApp(ctx context.Context, cmd portalidentity.UpdateAppCommand) error {
	_, err := usecaseop.Run(ctx, o.UoW, portalidentity.UpdateApp(o.Apps), cmd, auth.NewExecutionContext(ctx))
	return err
}

// DeleteApp runs DeletePortalApp (the app's OAuth clients go with it).
func (o PortalOps) DeleteApp(ctx context.Context, cmd portalidentity.DeleteAppCommand) error {
	_, err := usecaseop.RunTx(ctx, o.UoW, portalidentity.DeleteApp(o.Apps, o.OAuthClients), cmd, auth.NewExecutionContext(ctx))
	return err
}

// AssignUnassigned runs AssignUnassignedPortalIdentitiesToApp.
func (o PortalOps) AssignUnassigned(ctx context.Context, cmd portalidentity.AssignUnassignedCommand) (portalidentity.AssignUnassignedResult, error) {
	return usecaseop.RunTx(ctx, o.UoW, portalidentity.AssignUnassignedToApp(o.Identities, o.Apps), cmd, auth.NewExecutionContext(ctx))
}

// SetStatus runs the portal identity's suspend and reactivate.
func (o PortalOps) SetStatus(ctx context.Context, cmd portalidentity.SetStatusCommand) error {
	_, err := usecaseop.Run(ctx, o.UoW, portalidentity.SetStatus(o.Identities), cmd, auth.NewExecutionContext(ctx))
	return err
}

// DeleteUser runs DeletePortalIdentity.
func (o PortalOps) DeleteUser(ctx context.Context, cmd portalidentity.DeleteCommand) error {
	_, err := usecaseop.Run(ctx, o.UoW, portalidentity.Delete(o.Identities), cmd, auth.NewExecutionContext(ctx))
	return err
}

// GrantApp runs the grant of a portal app to an identity.
func (o PortalOps) GrantApp(ctx context.Context, cmd portalidentity.AppGrantCommand) error {
	_, err := usecaseop.Run(ctx, o.UoW, portalidentity.GrantApp(o.Identities, o.Apps), cmd, auth.NewExecutionContext(ctx))
	return err
}

// RevokeApp runs the revoke of an identity's grant.
func (o PortalOps) RevokeApp(ctx context.Context, cmd portalidentity.AppGrantCommand) error {
	_, err := usecaseop.Run(ctx, o.UoW, portalidentity.RevokeApp(o.Identities, o.Apps), cmd, auth.NewExecutionContext(ctx))
	return err
}
