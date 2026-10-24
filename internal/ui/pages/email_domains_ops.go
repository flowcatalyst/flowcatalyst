package pages

import (
	"context"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/emaildomainmapping"
	edmops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/emaildomainmapping/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/identityprovider"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecaseop"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecasepgx"
)

// EmailDomainStore is the slice of the email domain mapping repository the
// pages read (GET /api/email-domain-mappings and its by-id read).
type EmailDomainStore interface {
	FindAll(ctx context.Context) ([]emaildomainmapping.EmailDomainMapping, error)
	FindByID(ctx context.Context, id string) (*emaildomainmapping.EmailDomainMapping, error)
}

// EmailDomainCommands are the mapping operations the page runs; EmailDomainOps
// runs the API's use cases (the permission check is the page's).
type EmailDomainCommands interface {
	Create(ctx context.Context, cmd edmops.CreateCommand) (string, error)
	Update(ctx context.Context, cmd edmops.UpdateCommand) error
	Move(ctx context.Context, cmd edmops.MoveProviderCommand) (edmops.MoveProviderResult, error)
	Delete(ctx context.Context, id string) error
}

// EmailDomainOps runs the email domain mapping use cases through the unit of
// work, exactly as internal/platform/emaildomainmapping/api does.
type EmailDomainOps struct {
	Repo       *emaildomainmapping.Repository
	IDPs       *identityprovider.Repository
	Principals *principal.Repository
	UoW        *usecasepgx.UnitOfWork
}

// Create runs CreateMapping and returns the new mapping's id.
func (o EmailDomainOps) Create(ctx context.Context, cmd edmops.CreateCommand) (string, error) {
	event, err := usecaseop.Run(ctx, o.UoW, edmops.CreateMapping(o.Repo), cmd, auth.NewExecutionContext(ctx))
	if err != nil {
		return "", err
	}
	return event.MappingID, nil
}

// Update runs UpdateMapping.
func (o EmailDomainOps) Update(ctx context.Context, cmd edmops.UpdateCommand) error {
	_, err := usecaseop.Run(ctx, o.UoW, edmops.UpdateMapping(o.Repo), cmd, auth.NewExecutionContext(ctx))
	return err
}

// Move runs MoveMappingToProvider (the direction-aware provider change).
func (o EmailDomainOps) Move(ctx context.Context, cmd edmops.MoveProviderCommand) (edmops.MoveProviderResult, error) {
	deps := edmops.MoveDeps{Mappings: o.Repo, IDPs: o.IDPs, Principals: o.Principals}
	return usecaseop.RunTx(ctx, o.UoW, edmops.MoveMappingToProvider(deps), cmd, auth.NewExecutionContext(ctx))
}

// Delete runs DeleteMapping.
func (o EmailDomainOps) Delete(ctx context.Context, id string) error {
	_, err := usecaseop.Run(ctx, o.UoW, edmops.DeleteMapping(o.Repo), edmops.DeleteCommand{ID: id}, auth.NewExecutionContext(ctx))
	return err
}
