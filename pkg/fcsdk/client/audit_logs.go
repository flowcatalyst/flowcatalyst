package client

import "context"

// ─── Request DTOs ────────────────────────────────────────────────────

// AuditLogFilters — query parameters for GET /api/audit-logs.
type AuditLogFilters struct {
	EntityType  string
	EntityID    string
	Operation   string
	PrincipalID string
	ClientID    string
	From        string
	To          string
	Page        *uint32
	PageSize    *uint32
}

// ─── Response DTOs ───────────────────────────────────────────────────

// AuditLogResponse is a single audit log row.
type AuditLogResponse struct {
	ID            string `json:"id"`
	Operation     string `json:"operation"`
	EntityType    string `json:"entityType"`
	EntityID      string `json:"entityId,omitempty"`
	PrincipalID   string `json:"principalId,omitempty"`
	PrincipalName string `json:"principalName,omitempty"`
	ApplicationID string `json:"applicationId,omitempty"`
	ClientID      string `json:"clientId,omitempty"`
	PerformedAt   string `json:"performedAt"`
}

// AuditLogListResponse — GET /api/audit-logs.
type AuditLogListResponse struct {
	AuditLogs []AuditLogResponse `json:"auditLogs"`
	Total     int64              `json:"total,omitempty"`
	Page      int32              `json:"page,omitempty"`
	PageSize  int32              `json:"pageSize,omitempty"`
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
			String("clientId", filters.ClientID).
			String("from", filters.From).
			String("to", filters.To).
			Uint32("page", filters.Page).
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
