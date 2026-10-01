// routes.go is the function-runner public-routes write path
// (docs/function-runner-plan.md §8, "public routes"): PUT upserts a route
// by (hostname, pathPrefix), DELETE removes one by id. Every write bumps
// the function's runner pool revision in the same transaction — a route
// change must reach runners without waiting for a promote (mirrors
// settings.go's PutSetting/DeleteSetting).
package operations

import (
	"context"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/function"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/functiondomain"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/internal/tsid"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecaseop"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

// PutRouteCommand is the input DTO for PUT /api/functions/{id}/routes.
type PutRouteCommand struct {
	FunctionID string `json:"functionId"`
	Hostname   string `json:"hostname"`
	PathPrefix string `json:"pathPrefix"`
	// Alias names a non-live alias to route to; empty means live.
	Alias string `json:"alias,omitempty"`
	// AliasPrefixes opts the route into alias-prefixed hostnames
	// (`qa-myapp.acme.com` serves alias `qa`). Live routes only.
	AliasPrefixes []string `json:"aliasPrefixes,omitempty"`
}

// PutRoute upserts a route by (hostname, pathPrefix) — matching the
// existing row when one is owned by this SAME function (an update: the
// route keeps its id), refusing ROUTE_TAKEN when it is owned by a
// DIFFERENT function, and validating: a lowercase DNS hostname with no
// port/wildcard, a hostname covered by a zone claimed by the function's
// client (or, for a platform-owned function, a platform zone with null
// client) — ROUTE_HOST_NOT_CLAIMED otherwise — and an alias that either
// names an existing alias of this function or is empty (live).
func PutRoute(repo *function.Repository, domains *functiondomain.Repository) usecaseop.TxOperation[PutRouteCommand, function.Route] {
	return usecaseop.TxOperation[PutRouteCommand, function.Route]{
		Name: "PutFunctionRoute",
		Validate: func(_ context.Context, cmd PutRouteCommand) error {
			if strings.TrimSpace(cmd.FunctionID) == "" {
				return usecase.Validation("FUNCTION_ID_REQUIRED", "functionId is required")
			}
			hostname := function.NormalizeHostname(cmd.Hostname)
			if !function.ValidHostname(hostname) {
				return usecase.Validation("INVALID_HOSTNAME",
					"hostname must be a lowercase DNS name with no port and no wildcard")
			}
			alias := strings.TrimSpace(cmd.Alias)
			if alias != "" && alias != function.LiveAlias && !function.ValidAliasName(alias) {
				return usecase.Validation("INVALID_ALIAS_NAME",
					"alias name must start with a lowercase letter and contain only lowercase alphanumerics and hyphens (max 31 chars)")
			}
			if len(cmd.AliasPrefixes) > 0 && alias != "" && alias != function.LiveAlias {
				return usecase.Validation("ALIAS_PREFIXES_REQUIRE_LIVE",
					"aliasPrefixes can only be set on a route that serves live")
			}
			seen := map[string]bool{}
			for _, p := range cmd.AliasPrefixes {
				if !function.ValidAliasPrefix(p) {
					return usecase.Validation("INVALID_ALIAS_PREFIX",
						"alias prefix '"+p+"' must be a valid alias name without hyphens, and not 'live'")
				}
				if seen[p] {
					return usecase.Validation("INVALID_ALIAS_PREFIX", "duplicate alias prefix '"+p+"'")
				}
				seen[p] = true
			}
			return nil
		},
		// Per-resource authz needs the loaded function; the coarse "may manage
		// routes" permission is on the controller.
		Authorize: usecaseop.Public[PutRouteCommand],
		Execute: func(ctx context.Context, s *usecasepgx.TxScopedUnitOfWork, cmd PutRouteCommand, ec usecase.ExecutionContext) (function.Route, error) {
			var zero function.Route

			f, err := repo.FindByID(ctx, cmd.FunctionID)
			if err != nil {
				return zero, usecase.Internal("REPO", "find_by_id failed", err)
			}
			if f == nil {
				return zero, httperror.NotFound("Function", cmd.FunctionID)
			}
			if err := auth.CheckScopeAccess(auth.FromContext(ctx), f.ClientID); err != nil {
				return zero, err
			}

			hostname := function.NormalizeHostname(cmd.Hostname)
			pathPrefix := function.NormalizePathPrefix(cmd.PathPrefix)

			var aliasPtr *string
			alias := strings.TrimSpace(cmd.Alias)
			if alias != "" && alias != function.LiveAlias {
				aliases, err := repo.ListAliases(ctx, f.ID)
				if err != nil {
					return zero, usecase.Internal("REPO", "list_aliases failed", err)
				}
				found := false
				for _, a := range aliases {
					if a.Name == alias {
						found = true
						break
					}
				}
				if !found {
					return zero, usecase.BusinessRule("ALIAS_NOT_FOUND",
						"function "+f.ID+" has no alias named '"+alias+"'")
				}
				a := alias
				aliasPtr = &a
			}

			_, covered, err := domains.FindCoveringZone(ctx, hostname, f.ClientID)
			if err != nil {
				return zero, usecase.Internal("REPO", "find_covering_zone failed", err)
			}
			if !covered {
				return zero, usecase.BusinessRule("ROUTE_HOST_NOT_CLAIMED",
					"hostname '"+hostname+"' is not covered by any zone claimed by this function's owner")
			}

			existing, err := repo.FindRouteByHostPrefix(ctx, hostname, pathPrefix)
			if err != nil {
				return zero, usecase.Internal("REPO", "find_route_by_host_prefix failed", err)
			}
			if existing != nil && existing.FunctionID != f.ID {
				return zero, usecase.Conflict("ROUTE_TAKEN",
					"hostname '"+hostname+"' path '"+pathPrefix+"' is already routed to another function")
			}

			route := function.Route{
				Hostname:      hostname,
				PathPrefix:    pathPrefix,
				FunctionID:    f.ID,
				Alias:         aliasPtr,
				AliasPrefixes: slices.Sorted(slices.Values(cmd.AliasPrefixes)),
			}
			if existing != nil {
				route.ID = existing.ID
				route.CreatedBy = existing.CreatedBy
				route.CreatedAt = existing.CreatedAt
			} else {
				route.ID = tsid.Generate(tsid.FunctionRoute)
				pid := ec.PrincipalID
				route.CreatedBy = &pid
				route.CreatedAt = time.Now().UTC()
			}
			route.UpdatedAt = time.Now().UTC()

			if err := s.WithTx(ctx, func(tx pgx.Tx) error {
				if rErr := repo.UpsertRouteTx(ctx, &route, tx); rErr != nil {
					return rErr
				}
				_, bumpErr := repo.BumpPoolRevision(ctx, usecasepgx.WrapTxForBootstrap(tx), f.RunnerPool())
				return bumpErr
			}); err != nil {
				return zero, usecase.Internal("REPO", "route write failed", err)
			}

			event := RouteSet{
				Metadata:   usecase.NewEventMetadata(ec, RouteSetType, Source, subjectFor(f.ID)),
				FunctionID: f.ID,
				RouteID:    route.ID,
				Hostname:   route.Hostname,
				PathPrefix: route.PathPrefix,
			}
			if r := usecasepgx.EmitEventScoped(ctx, s, event, cmd); !usecase.IsSuccess(r) {
				_, e := usecase.Into(r)
				return zero, e
			}

			return route, nil
		},
	}
}

// DeleteRouteCommand is the input DTO for DELETE
// /api/functions/{id}/routes/{routeId}.
type DeleteRouteCommand struct {
	FunctionID string `json:"functionId"`
	RouteID    string `json:"routeId"`
}

// DeleteRoute removes one route; the delete and the pool revision bump
// commit atomically.
func DeleteRoute(repo *function.Repository) usecaseop.TxOperation[DeleteRouteCommand, struct{}] {
	return usecaseop.TxOperation[DeleteRouteCommand, struct{}]{
		Name: "DeleteFunctionRoute",
		Validate: func(_ context.Context, cmd DeleteRouteCommand) error {
			if strings.TrimSpace(cmd.FunctionID) == "" {
				return usecase.Validation("FUNCTION_ID_REQUIRED", "functionId is required")
			}
			if strings.TrimSpace(cmd.RouteID) == "" {
				return usecase.Validation("ROUTE_ID_REQUIRED", "routeId is required")
			}
			return nil
		},
		Authorize: usecaseop.Public[DeleteRouteCommand],
		Execute: func(ctx context.Context, s *usecasepgx.TxScopedUnitOfWork, cmd DeleteRouteCommand, ec usecase.ExecutionContext) (struct{}, error) {
			var zero struct{}

			f, err := repo.FindByID(ctx, cmd.FunctionID)
			if err != nil {
				return zero, usecase.Internal("REPO", "find_by_id failed", err)
			}
			if f == nil {
				return zero, httperror.NotFound("Function", cmd.FunctionID)
			}
			if err := auth.CheckScopeAccess(auth.FromContext(ctx), f.ClientID); err != nil {
				return zero, err
			}

			rt, err := repo.FindRouteByID(ctx, cmd.RouteID)
			if err != nil {
				return zero, usecase.Internal("REPO", "find_route_by_id failed", err)
			}
			if rt == nil || rt.FunctionID != f.ID {
				return zero, httperror.NotFound("FunctionRoute", cmd.RouteID)
			}

			if err := s.WithTx(ctx, func(tx pgx.Tx) error {
				if delErr := repo.DeleteRouteTx(ctx, f.ID, rt.ID, tx); delErr != nil {
					return delErr
				}
				_, bumpErr := repo.BumpPoolRevision(ctx, usecasepgx.WrapTxForBootstrap(tx), f.RunnerPool())
				return bumpErr
			}); err != nil {
				return zero, usecase.Internal("REPO", "route delete failed", err)
			}

			event := RouteDeleted{
				Metadata:   usecase.NewEventMetadata(ec, RouteDeletedType, Source, subjectFor(f.ID)),
				FunctionID: f.ID,
				RouteID:    rt.ID,
				Hostname:   rt.Hostname,
				PathPrefix: rt.PathPrefix,
			}
			if r := usecasepgx.EmitEventScoped(ctx, s, event, cmd); !usecase.IsSuccess(r) {
				_, e := usecase.Into(r)
				return zero, e
			}
			return zero, nil
		},
	}
}

// ListRoutes is a pure read (mirrors ListAliases/ListSettings).
func ListRoutes(ctx context.Context, repo *function.Repository, functionID string) (*function.Function, []function.Route, error) {
	f, err := repo.FindByID(ctx, functionID)
	if err != nil {
		return nil, nil, usecase.Internal("REPO", "find_by_id failed", err)
	}
	if f == nil {
		return nil, nil, httperror.NotFound("Function", functionID)
	}
	if err := auth.CheckScopeAccess(auth.FromContext(ctx), f.ClientID); err != nil {
		return nil, nil, err
	}
	routes, err := repo.ListRoutesByFunction(ctx, functionID)
	if err != nil {
		return nil, nil, usecase.Internal("REPO", "list_routes_by_function failed", err)
	}
	return f, routes, nil
}
