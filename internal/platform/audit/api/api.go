// Package api wires the audit log read-only HTTP endpoints via huma.
package api

import (
	"context"
	"encoding/base64"
	"errors"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/audit"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apicommon"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apiroute"
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
	g := apiroute.New(api, tag)
	apiroute.Get(g, "listAuditLogs", "/api/audit-logs", "List audit logs with filters", s.list)
	apiroute.Get(g, "listAuditLogsRecent", "/api/audit-logs/recent", "List recent audit logs (alias for list)", s.list)
	apiroute.Get(g, "auditLogEntityTypes", "/api/audit-logs/entity-types", "Distinct entity types", s.entityTypes)
	apiroute.Get(g, "auditLogOperations", "/api/audit-logs/operations", "Distinct operations", s.operations)
	apiroute.Get(g, "auditLogApplicationIDs", "/api/audit-logs/application-ids", "Distinct application ids", s.applicationIDs)
	apiroute.Get(g, "auditLogClientIDs", "/api/audit-logs/client-ids", "Distinct client ids", s.clientIDs)
	apiroute.Get(g, "getAuditLog", "/api/audit-logs/{id}", "Get an audit log by id", s.getByID)
	apiroute.Get(g, "auditLogsByEntity", "/api/audit-logs/entity/{entityType}/{entityId}", "Audit logs for a specific entity", s.byEntity)
	apiroute.Get(g, "auditLogsByPrincipal", "/api/audit-logs/principal/{principalId}", "Audit logs for a specific principal", s.byPrincipal)
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

func (s *State) list(ctx context.Context, in *listInput) (*apicommon.Out[AuditLogListResponse], error) {
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
		EntityType:     apicommon.OptStr(in.EntityType),
		EntityID:       apicommon.OptStr(in.EntityID),
		PrincipalID:    apicommon.OptStr(in.PrincipalID),
		Operation:      apicommon.OptStr(in.Operation),
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

	body := AuditLogListResponse{AuditLogs: apicommon.MapSlice(rows, fromEntity), HasMore: hasMore}
	if hasMore && len(rows) > 0 {
		last := rows[len(rows)-1]
		cur := encodeCursor(audit.Cursor{PerformedAt: last.PerformedAt, ID: last.ID})
		body.NextCursor = &cur
	}
	return &apicommon.Out[AuditLogListResponse]{Body: body}, nil
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

func (s *State) getByID(ctx context.Context, in *apicommon.IDInput) (*apicommon.Out[AuditLogResponse], error) {
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
	return &apicommon.Out[AuditLogResponse]{Body: fromEntity(l)}, nil
}

type byEntityInput struct {
	EntityType string `path:"entityType"`
	EntityID   string `path:"entityId"`
}

func (s *State) byEntity(ctx context.Context, in *byEntityInput) (*apicommon.Out[AuditLogListResponse], error) {
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
	out := apicommon.MapSlice(rows, fromEntity)
	return &apicommon.Out[AuditLogListResponse]{Body: AuditLogListResponse{AuditLogs: out}}, nil
}

type byPrincipalInput struct {
	PrincipalID string `path:"principalId"`
}

func (s *State) byPrincipal(ctx context.Context, in *byPrincipalInput) (*apicommon.Out[AuditLogListResponse], error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanWritePermission(ac, viewPerm); err != nil {
		return nil, err
	}
	rows, err := s.Repo.FindWithFilters(ctx, audit.FilterParams{PrincipalID: &in.PrincipalID, Limit: 500})
	if err != nil {
		return nil, usecase.Internal("REPO", "by_principal failed", err)
	}
	out := apicommon.MapSlice(rows, fromEntity)
	return &apicommon.Out[AuditLogListResponse]{Body: AuditLogListResponse{AuditLogs: out}}, nil
}

// ── facets ──────────────────────────────────────────────────────────────

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

func (s *State) entityTypes(ctx context.Context, _ *apicommon.Empty) (*apicommon.Out[AuditLogEntityTypesResponse], error) {
	vals, err := s.distinct(ctx, "entity_type")
	if err != nil {
		return nil, err
	}
	return &apicommon.Out[AuditLogEntityTypesResponse]{Body: AuditLogEntityTypesResponse{EntityTypes: vals}}, nil
}

func (s *State) operations(ctx context.Context, _ *apicommon.Empty) (*apicommon.Out[AuditLogOperationsResponse], error) {
	vals, err := s.distinct(ctx, "operation")
	if err != nil {
		return nil, err
	}
	return &apicommon.Out[AuditLogOperationsResponse]{Body: AuditLogOperationsResponse{Operations: vals}}, nil
}

func (s *State) applicationIDs(ctx context.Context, _ *apicommon.Empty) (*apicommon.Out[AuditLogApplicationIDsResponse], error) {
	vals, err := s.distinct(ctx, "application_id")
	if err != nil {
		return nil, err
	}
	return &apicommon.Out[AuditLogApplicationIDsResponse]{Body: AuditLogApplicationIDsResponse{ApplicationIDs: vals}}, nil
}

func (s *State) clientIDs(ctx context.Context, _ *apicommon.Empty) (*apicommon.Out[AuditLogClientIDsResponse], error) {
	vals, err := s.distinct(ctx, "client_id")
	if err != nil {
		return nil, err
	}
	return &apicommon.Out[AuditLogClientIDsResponse]{Body: AuditLogClientIDsResponse{ClientIDs: vals}}, nil
}
