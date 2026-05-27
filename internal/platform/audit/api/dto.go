// dto.go contains the wire-format types for the audit log API.
package api

import (
	"encoding/json"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/audit"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httpcompat"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/jsontime"
)

// AuditLogResponse mirrors audit.Log.
type AuditLogResponse struct {
	ID            string          `json:"id"`
	EntityType    string          `json:"entityType"`
	EntityID      string          `json:"entityId"`
	Operation     string          `json:"operation"`
	OperationJSON json.RawMessage `json:"operationJson,omitempty"`
	PrincipalID   *string         `json:"principalId,omitempty"`
	PrincipalName *string         `json:"principalName,omitempty"`
	ApplicationID *string         `json:"applicationId,omitempty"`
	ClientID      *string         `json:"clientId,omitempty"`
	PerformedAt   httpcompat.Time `json:"performedAt"`
}

func fromEntity(l *audit.Log) AuditLogResponse {
	return AuditLogResponse{
		ID:            l.ID,
		EntityType:    l.EntityType,
		EntityID:      l.EntityID,
		Operation:     l.Operation,
		OperationJSON: l.OperationJSON,
		PrincipalID:   l.PrincipalID,
		PrincipalName: l.PrincipalName,
		ApplicationID: l.ApplicationID,
		ClientID:      l.ClientID,
		PerformedAt:   jsontime.New(l.PerformedAt),
	}
}

// AuditLogListResponse is the wire shape for GET /api/audit-logs.
type AuditLogListResponse struct {
	Items []AuditLogResponse `json:"items"`
}

// DistinctValuesResponse is the wire shape for facet endpoints.
type DistinctValuesResponse struct {
	Items []string `json:"items"`
}
