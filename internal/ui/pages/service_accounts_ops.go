package pages

import (
	"context"

	"github.com/flowcatalyst/flowcatalyst-go/internal/ids"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/audit"
	platformauth "github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth/authservice"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/client"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	principalops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/role"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/serviceaccount"
	saops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/serviceaccount/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/go-sdk/usecaseop"
	"github.com/flowcatalyst/go-sdk/usecasepgx"
)

// ServiceAccountCommands are the service-account operations the page runs.
// The production implementation (ServiceAccountOps) runs the same use cases
// as internal/platform/serviceaccount/api (and the principal API's
// application access), so validation, the role ceiling, the domain event and
// the audit record are the use case's. The permission and reach gates
// (saops.AuthorizeWrite and friends) are the caller's, as they are the API
// handlers'.
type ServiceAccountCommands interface {
	Create(ctx context.Context, cmd saops.CreateCommand) (saops.CreateWithCredentialsResult, error)
	Update(ctx context.Context, cmd saops.UpdateCommand) error
	Delete(ctx context.Context, id string) error
	AssignRoles(ctx context.Context, id string, roles []string) error
	// RegenerateToken and RegenerateSecret return the new plaintext, once.
	RegenerateToken(ctx context.Context, id string) (string, error)
	RegenerateSecret(ctx context.Context, id string) (string, error)
	MintToken(ctx context.Context, id string) (saops.MintedToken, error)
	// SetApplicationAccess replaces the applications of the account's linked
	// SERVICE principal; all, when set, sets its all-applications flag.
	SetApplicationAccess(ctx context.Context, principalID string, applicationIDs []string, all *bool) error
}

// ServiceAccountOps runs the service-account use cases through the unit of
// work, exactly as internal/platform/serviceaccount/api does.
type ServiceAccountOps struct {
	Repo         *serviceaccount.Repository
	Principals   *principal.Repository
	OAuthClients *platformauth.OAuthClientRepo
	Clients      *client.Repository
	Grants       *principal.ClientAccessGrantRepo
	Roles        *role.Repository
	Applications *application.Repository
	UoW          *usecasepgx.UnitOfWork
	// Auth, FlattenPermissions and Audit back the admin token mint (see
	// saops.MintDeps).
	Auth               *authservice.AuthService
	FlattenPermissions func(ctx context.Context, roleNames []string) ([]string, error)
	Audit              *audit.Repository
}

// Create runs CreateServiceAccountWithCredentials (account, linked principal
// and OAuth client in one transaction).
func (o ServiceAccountOps) Create(ctx context.Context, cmd saops.CreateCommand) (saops.CreateWithCredentialsResult, error) {
	return usecaseop.RunTx(ctx, o.UoW, saops.CreateServiceAccountWithCredentials(o.Repo, o.Principals, o.OAuthClients, o.Clients, o.Grants),
		cmd, auth.NewExecutionContext(ctx))
}

// Update runs UpdateServiceAccount.
func (o ServiceAccountOps) Update(ctx context.Context, cmd saops.UpdateCommand) error {
	_, err := usecaseop.RunTx(ctx, o.UoW, saops.UpdateServiceAccount(o.Repo, o.Principals, o.Clients, o.Grants), cmd, auth.NewExecutionContext(ctx))
	return err
}

// Delete runs DeleteServiceAccount.
func (o ServiceAccountOps) Delete(ctx context.Context, id string) error {
	_, err := usecaseop.Run(ctx, o.UoW, saops.DeleteServiceAccount(o.Repo), saops.DeleteCommand{ID: id}, auth.NewExecutionContext(ctx))
	return err
}

// AssignRoles runs AssignRolesToServiceAccount (the role ceiling is its).
func (o ServiceAccountOps) AssignRoles(ctx context.Context, id string, roles []string) error {
	_, err := usecaseop.Run(ctx, o.UoW, saops.AssignRolesToServiceAccount(o.Repo, o.Principals, o.Roles),
		saops.AssignRolesCommand{ServiceAccountID: id, Roles: roles}, auth.NewExecutionContext(ctx))
	return err
}

// RegenerateToken runs RegenerateAuthToken. The plaintext is read from a
// local sink, only on the success path: a failed commit discloses nothing.
func (o ServiceAccountOps) RegenerateToken(ctx context.Context, id string) (string, error) {
	var token string
	_, err := usecaseop.Run(ctx, o.UoW, saops.RegenerateAuthToken(o.Repo),
		saops.RegenerateAuthTokenCommand{ServiceAccountID: id, Disclose: func(p string) { token = p }}, auth.NewExecutionContext(ctx))
	if err != nil {
		return "", err
	}
	return token, nil
}

// RegenerateSecret runs RegenerateSigningSecret (see RegenerateToken).
func (o ServiceAccountOps) RegenerateSecret(ctx context.Context, id string) (string, error) {
	var secret string
	_, err := usecaseop.Run(ctx, o.UoW, saops.RegenerateSigningSecret(o.Repo),
		saops.RegenerateSigningSecretCommand{ServiceAccountID: id, Disclose: func(p string) { secret = p }}, auth.NewExecutionContext(ctx))
	if err != nil {
		return "", err
	}
	return secret, nil
}

// MintToken runs MintAdminToken (audited).
func (o ServiceAccountOps) MintToken(ctx context.Context, id string) (saops.MintedToken, error) {
	return saops.MintAdminToken(ctx, saops.MintDeps{
		Repo: o.Repo, Principals: o.Principals, Roles: o.Roles, Auth: o.Auth,
		FlattenPermissions: o.FlattenPermissions, Audit: o.Audit,
	}, id)
}

// SetApplicationAccess runs AssignApplicationAccess on the account's linked
// principal, as PUT /api/principals/{id}/application-access does.
func (o ServiceAccountOps) SetApplicationAccess(ctx context.Context, principalID string, applicationIDs []string, all *bool) error {
	_, err := usecaseop.Run(ctx, o.UoW, principalops.AssignApplicationAccess(o.Principals, o.Applications),
		principalops.AssignApplicationAccessCommand{UserID: ids.PrincipalID(principalID), ApplicationIDs: applicationIDs, AllApplications: all},
		auth.NewExecutionContext(ctx))
	return err
}
