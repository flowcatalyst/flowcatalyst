package operations

import (
	"context"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/functiondomain"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecaseop"
)

// DeleteCommand is the input DTO for DELETE /api/function-domains/{id}.
type DeleteCommand struct {
	ID string `json:"id"`
}

// RouteHostnames returns every hostname a function route currently uses —
// functiondomain.Repository.ListDistinctRouteHostnames in production. A
// func type (not the concrete repository) so this package never needs to
// import internal/platform/function, which itself imports functiondomain
// to check route coverage — a direct dependency the other way would cycle.
type RouteHostnames func(ctx context.Context) ([]string, error)

// DeleteDomain releases a claim, refusing DOMAIN_IN_USE while any function
// route's hostname is covered by this domain's zone.
func DeleteDomain(repo *functiondomain.Repository, routeHostnames RouteHostnames) usecaseop.Operation[DeleteCommand, DomainReleased] {
	return usecaseop.Operation[DeleteCommand, DomainReleased]{
		Name: "DeleteFunctionDomain",
		Validate: func(_ context.Context, cmd DeleteCommand) error {
			if strings.TrimSpace(cmd.ID) == "" {
				return usecase.Validation("ID_REQUIRED", "id is required")
			}
			return nil
		},
		// Per-resource authz runs post-load in Execute; the coarse "may manage
		// function domains" permission is on the controller.
		Authorize: usecaseop.Public[DeleteCommand],
		Execute: func(ctx context.Context, cmd DeleteCommand, ec usecase.ExecutionContext) (usecaseop.Plan[DomainReleased], error) {
			d, err := repo.FindByID(ctx, cmd.ID)
			if err != nil {
				return nil, usecase.Internal("REPO", "find_by_id failed", err)
			}
			if d == nil {
				return nil, httperror.NotFound("FunctionDomain", cmd.ID)
			}
			if err := auth.CheckScopeAccess(auth.FromContext(ctx), d.ClientID); err != nil {
				return nil, err
			}

			if routeHostnames != nil {
				hosts, err := routeHostnames(ctx)
				if err != nil {
					return nil, usecase.Internal("REPO", "list_distinct_route_hostnames failed", err)
				}
				for _, h := range hosts {
					if functiondomain.Covers(d.Zone, h) {
						return nil, usecase.BusinessRule("DOMAIN_IN_USE",
							"a function route uses a hostname ('"+h+"') covered by this domain; remove the route first")
					}
				}
			}

			event := DomainReleased{
				Metadata: usecase.NewEventMetadata(ec, DomainReleasedType, Source, subjectFor(d.ID)),
				DomainID: d.ID,
				Zone:     d.Zone,
			}
			return usecaseop.Delete(d, repo, event), nil
		},
	}
}
