// Package api wires HTTP routes for functiondomain via huma.
package api

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/functiondomain"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/functiondomain/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apicommon"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apiroute"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecaseop"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

// State bundles deps.
type State struct {
	Repo *functiondomain.Repository
	UoW  *usecasepgx.UnitOfWork
	// RouteHostnames feeds DeleteDomain's DOMAIN_IN_USE check — see
	// operations.RouteHostnames.
	RouteHostnames operations.RouteHostnames
}

const tag = "function-domains"

// Register mounts the function-domain endpoints. Every route is gated on
// platform:function:domain:manage (brief: "claims are an operator action") —
// there is no separate view permission; a caller who may manage domains may
// also list them.
func Register(api huma.API, s *State) {
	g := apiroute.New(api, tag)
	apiroute.Post(g, "createFunctionDomain", "/api/function-domains", "Claim a hostname zone for a client (or the platform)", http.StatusCreated, s.create)
	apiroute.Get(g, "listFunctionDomains", "/api/function-domains", "List claimed hostname zones", s.list)
	apiroute.Delete(g, "deleteFunctionDomain", "/api/function-domains/{id}", "Release a claimed zone", http.StatusNoContent, s.delete)
}

func (s *State) create(ctx context.Context, in *apicommon.In[CreateDomainRequest]) (*apicommon.Out[apicommon.CreatedResponse], error) {
	if err := auth.CanManageFunctionDomains(auth.FromContext(ctx)); err != nil {
		return nil, err
	}
	ec := auth.NewExecutionContext(ctx)
	event, err := usecaseop.Run(ctx, s.UoW, operations.CreateDomain(s.Repo), in.Body.toCommand(), ec)
	if err != nil {
		return nil, err
	}
	return &apicommon.Out[apicommon.CreatedResponse]{Body: apicommon.CreatedResponse{ID: event.DomainID}}, nil
}

type listInput struct {
	ClientID string `query:"clientId" doc:"Filter by owning client id"`
}

func (s *State) list(ctx context.Context, in *listInput) (*apicommon.Out[[]DomainResponse], error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanManageFunctionDomains(ac); err != nil {
		return nil, err
	}
	filters := functiondomain.ListFilters{}
	if !ac.IsAnchor() {
		clients := ac.Clients
		filters.AccessibleClientIDs = &clients
	}
	rows, err := s.Repo.FindWithFilters(ctx, filters)
	if err != nil {
		return nil, usecase.Internal("REPO", "find_with_filters failed", err)
	}
	if in.ClientID != "" {
		filtered := rows[:0]
		for _, d := range rows {
			if d.ClientID != nil && *d.ClientID == in.ClientID {
				filtered = append(filtered, d)
			}
		}
		rows = filtered
	}
	return &apicommon.Out[[]DomainResponse]{Body: apicommon.MapSlice(rows, fromEntity)}, nil
}

func (s *State) delete(ctx context.Context, in *apicommon.IDInput) (*apicommon.Empty, error) {
	if err := auth.CanManageFunctionDomains(auth.FromContext(ctx)); err != nil {
		return nil, err
	}
	ec := auth.NewExecutionContext(ctx)
	if _, err := usecaseop.Run(ctx, s.UoW, operations.DeleteDomain(s.Repo, s.RouteHostnames), operations.DeleteCommand{ID: in.ID}, ec); err != nil {
		return nil, err
	}
	return &apicommon.Empty{}, nil
}
