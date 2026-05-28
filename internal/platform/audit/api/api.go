// Package api wires the audit log read-only HTTP endpoints via huma.
package api

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"strings"
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
		Path:          "/api/audit-logs/entity/{entityType}/{entityId}",
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

// listInput is the cursor-paginated query for GET /api/audit-logs. Matches
// the params the SPA sends (audit-logs.ts:50-60): after (opaque cursor),
// pageSize, entityType, operation, principalId, applicationIds/clientIds
// (CSV). Rust ref: audit/api.rs:148-171.
type listInput struct {
	After          string `query:"after" doc:"Opaque cursor from a previous page's nextCursor"`
	PageSize       int    `query:"pageSize" doc:"Page size (default 50, capped at 200)"`
	EntityType     string `query:"entityType"`
	EntityID       string `query:"entityId"`
	PrincipalID    string `query:"principalId"`
	Operation      string `query:"operation"`
	ApplicationIDs string `query:"applicationIds" doc:"CSV of application ids"`
	ClientIDs      string `query:"clientIds" doc:"CSV of client ids"`
}

func strPtr(v string) *string {
	if v == "" {
		return nil
	}
	s := v
	return &s
}

// csv splits a comma-separated query value, trimming blanks.
func csv(v string) []string {
	if v == "" {
		return nil
	}
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

type listOutput struct {
	Body AuditLogListResponse
}

func (s *State) list(ctx context.Context, in *listInput) (*listOutput, error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanWritePermission(ac, viewPerm); err != nil {
		return nil, err
	}

	size := in.PageSize
	if size < 1 || size > 200 {
		size = 50
	}

	var after *audit.Cursor
	if in.After != "" {
		c, err := decodeCursor(in.After)
		if err != nil {
			return nil, usecase.Validation("CURSOR", "invalid cursor")
		}
		after = c
	}

	rows, err := s.Repo.FindWithCursor(ctx, audit.CursorFilterParams{
		EntityType:     strPtr(in.EntityType),
		EntityID:       strPtr(in.EntityID),
		PrincipalID:    strPtr(in.PrincipalID),
		Operation:      strPtr(in.Operation),
		ApplicationIDs: csv(in.ApplicationIDs),
		ClientIDs:      csv(in.ClientIDs),
		After:          after,
		Limit:          size + 1,
	})
	if err != nil {
		return nil, usecase.Internal("REPO", "find_with_cursor failed", err)
	}

	hasMore := len(rows) > size
	if hasMore {
		rows = rows[:size]
	}

	out := make([]AuditLogResponse, 0, len(rows))
	for i := range rows {
		out = append(out, fromEntity(&rows[i]))
	}

	body := AuditLogListResponse{AuditLogs: out, HasMore: hasMore}
	if hasMore && len(rows) > 0 {
		last := rows[len(rows)-1]
		cur := encodeCursor(audit.Cursor{PerformedAt: last.PerformedAt, ID: last.ID})
		body.NextCursor = &cur
	}
	return &listOutput{Body: body}, nil
}

// encodeCursor serializes a keyset position into an opaque base64 token of
// the form "<rfc3339nano>|<id>".
func encodeCursor(c audit.Cursor) string {
	raw := c.PerformedAt.UTC().Format(time.RFC3339Nano) + "|" + c.ID
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

// decodeCursor reverses encodeCursor.
func decodeCursor(s string) (*audit.Cursor, error) {
	b, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return nil, err
	}
	parts := strings.SplitN(string(b), "|", 2)
	if len(parts) != 2 {
		return nil, errBadCursor
	}
	t, err := time.Parse(time.RFC3339Nano, parts[0])
	if err != nil {
		return nil, err
	}
	return &audit.Cursor{PerformedAt: t, ID: parts[1]}, nil
}

var errBadCursor = errors.New("malformed cursor")

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
	EntityType string `path:"entityType"`
	EntityID   string `path:"entityId"`
}

func (s *State) byEntity(ctx context.Context, in *byEntityInput) (*listOutput, error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanWritePermission(ac, viewPerm); err != nil {
		return nil, err
	}
	rows, err := s.Repo.FindWithFilters(ctx, audit.FilterParams{
		EntityType: &in.EntityType,
		EntityID:   &in.EntityID,
		Limit:      500,
	})
	if err != nil {
		return nil, usecase.Internal("REPO", "by_entity failed", err)
	}
	out := make([]AuditLogResponse, 0, len(rows))
	for i := range rows {
		out = append(out, fromEntity(&rows[i]))
	}
	return &listOutput{Body: AuditLogListResponse{AuditLogs: out}}, nil
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
	return &listOutput{Body: AuditLogListResponse{AuditLogs: out}}, nil
}

// ── facets ──────────────────────────────────────────────────────────────

type emptyInput struct{}

// distinct fetches whitelisted distinct column values for the facet endpoints.
func (s *State) distinct(ctx context.Context, column string) ([]string, error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanWritePermission(ac, viewPerm); err != nil {
		return nil, err
	}
	out, err := s.Repo.DistinctValues(ctx, column, 500)
	if err != nil {
		return nil, usecase.Internal("REPO", "distinct failed", err)
	}
	return out, nil
}

type entityTypesOutput struct {
	Body AuditLogEntityTypesResponse
}

func (s *State) entityTypes(ctx context.Context, _ *emptyInput) (*entityTypesOutput, error) {
	vals, err := s.distinct(ctx, "entity_type")
	if err != nil {
		return nil, err
	}
	return &entityTypesOutput{Body: AuditLogEntityTypesResponse{EntityTypes: vals}}, nil
}

type operationsOutput struct {
	Body AuditLogOperationsResponse
}

func (s *State) operations(ctx context.Context, _ *emptyInput) (*operationsOutput, error) {
	vals, err := s.distinct(ctx, "operation")
	if err != nil {
		return nil, err
	}
	return &operationsOutput{Body: AuditLogOperationsResponse{Operations: vals}}, nil
}

type applicationIDsOutput struct {
	Body AuditLogApplicationIDsResponse
}

func (s *State) applicationIDs(ctx context.Context, _ *emptyInput) (*applicationIDsOutput, error) {
	vals, err := s.distinct(ctx, "application_id")
	if err != nil {
		return nil, err
	}
	return &applicationIDsOutput{Body: AuditLogApplicationIDsResponse{ApplicationIDs: vals}}, nil
}

type clientIDsOutput struct {
	Body AuditLogClientIDsResponse
}

func (s *State) clientIDs(ctx context.Context, _ *emptyInput) (*clientIDsOutput, error) {
	vals, err := s.distinct(ctx, "client_id")
	if err != nil {
		return nil, err
	}
	return &clientIDsOutput{Body: AuditLogClientIDsResponse{ClientIDs: vals}}, nil
}
