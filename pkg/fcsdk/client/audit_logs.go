package client

import (
	"context"
	"strings"
)

// ─── Request DTOs ────────────────────────────────────────────────────

// AuditLogFilters — query parameters for GET /api/audit-logs. The listing
// is cursor-paged: pass the previous page's NextCursor as After.
type AuditLogFilters struct {
	EntityType  string
	EntityID    string
	Operation   string
	PrincipalID string
	// ApplicationIDs and ClientIDs filter to those ids (sent as CSV).
	ApplicationIDs []string
	ClientIDs      []string
	// After is the opaque cursor from a previous page's NextCursor.
	After string
	// PageSize defaults to 50 server-side and is capped at 200.
	PageSize *uint32
}

// ─── Response DTOs ───────────────────────────────────────────────────

// AuditLogResponse is a single audit log row.
type AuditLogResponse struct {
	ID         string `json:"id"`
	Operation  string `json:"operation"`
	EntityType string `json:"entityType"`
	EntityID   string `json:"entityId"`
	// OperationJSON is the serialised operation payload, when recorded.
	OperationJSON string `json:"operationJson,omitempty"`
	PrincipalID   string `json:"principalId,omitempty"`
	PrincipalName string `json:"principalName,omitempty"`
	ApplicationID string `json:"applicationId,omitempty"`
	ClientID      string `json:"clientId,omitempty"`
	PerformedAt   string `json:"performedAt"`
}

// AuditLogListResponse — GET /api/audit-logs, cursor-paged: while HasMore
// is true, pass NextCursor as AuditLogFilters.After to fetch the next page.
type AuditLogListResponse struct {
	AuditLogs  []AuditLogResponse `json:"auditLogs"`
	HasMore    bool               `json:"hasMore"`
	NextCursor string             `json:"nextCursor,omitempty"`
}

// ─── Resource ────────────────────────────────────────────────────────

// AuditLogsResource — /api/audit-logs/*.
type AuditLogsResource struct {
	c *FlowCatalystClient
}

// List — GET /api/audit-logs with optional filters.
func (r *AuditLogsResource) List(ctx context.Context, filters *AuditLogFilters) (*AuditLogListResponse, error) {
	q := ""
	if filters != nil {
		q = NewQuery().
			String("entityType", filters.EntityType).
			String("entityId", filters.EntityID).
			String("operation", filters.Operation).
			String("principalId", filters.PrincipalID).
			String("applicationIds", strings.Join(filters.ApplicationIDs, ",")).
			String("clientIds", strings.Join(filters.ClientIDs, ",")).
			String("after", filters.After).
			Uint32("pageSize", filters.PageSize).
			Encode()
	}
	var out AuditLogListResponse
	if err := r.c.Get(ctx, "/api/audit-logs"+q, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Get — GET /api/audit-logs/{id}.
func (r *AuditLogsResource) Get(ctx context.Context, id string) (*AuditLogResponse, error) {
	var out AuditLogResponse
	if err := r.c.Get(ctx, "/api/audit-logs/"+id, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
