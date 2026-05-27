// Package api wires the audit log read-only HTTP endpoints via huma.
package api

import (
	"context"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/audit"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
)

// State bundles deps.
type State struct {
	Repo *audit.Repository
}

const tag = "audit-logs"

// Register mounts the audit log endpoints.
func Register(api huma.API, s *State) {
	huma.Register(api, huma.Operation{
		OperationID:   "listAuditLogs",
		Method:        http.MethodGet,
		Path:          "/api/audit-logs",
		Summary:       "List audit logs with filters",
		Tags:          []string{tag},
		DefaultStatus: http.StatusOK,
	}, s.list)

	huma.Register(api, huma.Operation{
		OperationID:   "listAuditLogsRecent",
		Method:        http.MethodGet,
		Path:          "/api/audit-logs/recent",
		Summary:       "List recent audit logs (alias for list)",
		Tags:          []string{tag},
		DefaultStatus: http.StatusOK,
	}, s.list)

	huma.Register(api, huma.Operation{
		OperationID:   "auditLogEntityTypes",
		Method:        http.MethodGet,
		Path:          "/api/audit-logs/entity-types",
		Summary:       "Distinct entity types",
		Tags:          []string{tag},
		DefaultStatus: http.StatusOK,
	}, s.entityTypes)

	huma.Register(api, huma.Operation{
		OperationID:   "auditLogOperations",
		Method:        http.MethodGet,
		Path:          "/api/audit-logs/operations",
		Summary:       "Distinct operations",
		Tags:          []string{tag},
		DefaultStatus: http.StatusOK,
	}, s.operations)

	huma.Register(api, huma.Operation{
		OperationID:   "auditLogApplicationIDs",
		Method:        http.MethodGet,
		Path:          "/api/audit-logs/application-ids",
		Summary:       "Distinct application ids",
		Tags:          []string{tag},
		DefaultStatus: http.StatusOK,
	}, s.applicationIDs)

	huma.Register(api, huma.Operation{
		OperationID:   "auditLogClientIDs",
		Method:        http.MethodGet,
		Path:          "/api/audit-logs/client-ids",
		Summary:       "Distinct client ids",
		Tags:          []string{tag},
		DefaultStatus: http.StatusOK,
	}, s.clientIDs)

	huma.Register(api, huma.Operation{
		OperationID:   "getAuditLog",
		Method:        http.MethodGet,
		Path:          "/api/audit-logs/{id}",
		Summary:       "Get an audit log by id",
		Tags:          []string{tag},
		DefaultStatus: http.StatusOK,
	}, s.getByID)

	huma.Register(api, huma.Operation{
		OperationID:   "auditLogsByEntity",
		Method:        http.MethodGet,
		Path:          "/api/audit-logs/entity/{entityId}",
		Summary:       "Audit logs for a specific entity",
		Tags:          []string{tag},
		DefaultStatus: http.StatusOK,
	}, s.byEntity)

	huma.Register(api, huma.Operation{
		OperationID:   "auditLogsByPrincipal",
		Method:        http.MethodGet,
		Path:          "/api/audit-logs/principal/{principalId}",
		Summary:       "Audit logs for a specific principal",
		Tags:          []string{tag},
		DefaultStatus: http.StatusOK,
	}, s.byPrincipal)
}

const viewPerm = "platform:admin:audit-log:view"

type listInput struct {
	EntityType  string `query:"entityType"`
	EntityID    string `query:"entityId"`
	PrincipalID string `query:"principalId"`
	ClientID    string `query:"clientId"`
	Since       string `query:"since" doc:"RFC3339 timestamp"`
	Until       string `query:"until" doc:"RFC3339 timestamp"`
	Limit       int    `query:"limit"`
	Offset      int    `query:"offset"`
}

func (in *listInput) toFilters() audit.FilterParams {
	str := func(v string) *string {
		if v == "" {
			return nil
		}
		s := v
		return &s
	}
	ts := func(v string) *time.Time {
		if v == "" {
			return nil
		}
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			return &t
		}
		return nil
	}
	return audit.FilterParams{
		EntityType:  str(in.EntityType),
		EntityID:    str(in.EntityID),
		PrincipalID: str(in.PrincipalID),
		ClientID:    str(in.ClientID),
		Since:       ts(in.Since),
		Until:       ts(in.Until),
		Limit:       in.Limit,
		Offset:      in.Offset,
	}
}

type listOutput struct {
	Body AuditLogListResponse
}

func (s *State) list(ctx context.Context, in *listInput) (*listOutput, error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanWritePermission(ac, viewPerm); err != nil {
		return nil, err
	}
	rows, err := s.Repo.FindWithFilters(ctx, in.toFilters())
	if err != nil {
		return nil, usecase.Internal("REPO", "find_with_filters failed", err)
	}
	out := make([]AuditLogResponse, 0, len(rows))
	for i := range rows {
		out = append(out, fromEntity(&rows[i]))
	}
	return &listOutput{Body: AuditLogListResponse{Items: out}}, nil
}

type getInput struct {
	ID string `path:"id"`
}

type getOutput struct {
	Body AuditLogResponse
}

func (s *State) getByID(ctx context.Context, in *getInput) (*getOutput, error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanWritePermission(ac, viewPerm); err != nil {
		return nil, err
	}
	l, err := s.Repo.FindByID(ctx, in.ID)
	if err != nil {
		return nil, usecase.Internal("REPO", "find_by_id failed", err)
	}
	if l == nil {
		return nil, httperror.NotFound("AuditLog", in.ID)
	}
	return &getOutput{Body: fromEntity(l)}, nil
}

type byEntityInput struct {
	EntityID string `path:"entityId"`
}

func (s *State) byEntity(ctx context.Context, in *byEntityInput) (*listOutput, error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanWritePermission(ac, viewPerm); err != nil {
		return nil, err
	}
	rows, err := s.Repo.FindWithFilters(ctx, audit.FilterParams{EntityID: &in.EntityID, Limit: 500})
	if err != nil {
		return nil, usecase.Internal("REPO", "by_entity failed", err)
	}
	out := make([]AuditLogResponse, 0, len(rows))
	for i := range rows {
		out = append(out, fromEntity(&rows[i]))
	}
	return &listOutput{Body: AuditLogListResponse{Items: out}}, nil
}

type byPrincipalInput struct {
	PrincipalID string `path:"principalId"`
}

func (s *State) byPrincipal(ctx context.Context, in *byPrincipalInput) (*listOutput, error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanWritePermission(ac, viewPerm); err != nil {
		return nil, err
	}
	rows, err := s.Repo.FindWithFilters(ctx, audit.FilterParams{PrincipalID: &in.PrincipalID, Limit: 500})
	if err != nil {
		return nil, usecase.Internal("REPO", "by_principal failed", err)
	}
	out := make([]AuditLogResponse, 0, len(rows))
	for i := range rows {
		out = append(out, fromEntity(&rows[i]))
	}
	return &listOutput{Body: AuditLogListResponse{Items: out}}, nil
}

// ── facets ──────────────────────────────────────────────────────────────

type emptyInput struct{}

type distinctOutput struct {
	Body DistinctValuesResponse
}

func (s *State) distinct(ctx context.Context, column string) (*distinctOutput, error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanWritePermission(ac, viewPerm); err != nil {
		return nil, err
	}
	out, err := s.Repo.DistinctValues(ctx, column, 500)
	if err != nil {
		return nil, usecase.Internal("REPO", "distinct failed", err)
	}
	return &distinctOutput{Body: DistinctValuesResponse{Items: out}}, nil
}

func (s *State) entityTypes(ctx context.Context, _ *emptyInput) (*distinctOutput, error) {
	return s.distinct(ctx, "entity_type")
}

func (s *State) operations(ctx context.Context, _ *emptyInput) (*distinctOutput, error) {
	return s.distinct(ctx, "operation")
}

func (s *State) applicationIDs(ctx context.Context, _ *emptyInput) (*distinctOutput, error) {
	return s.distinct(ctx, "application_id")
}

func (s *State) clientIDs(ctx context.Context, _ *emptyInput) (*distinctOutput, error) {
	return s.distinct(ctx, "client_id")
}
