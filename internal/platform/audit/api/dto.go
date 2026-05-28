// dto.go contains the wire-format types for the audit log API.
package api

import (
	"bytes"
	"encoding/json"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/audit"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httpcompat"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/jsontime"
)

// AuditLogResponse mirrors audit.Log.
//
// OperationJSON is emitted as a JSON STRING (the serialized operation
// payload), not a nested object, because the SPA reads it as
// `operationJson: string | null` and calls JSON.parse on it
// (AuditLogListPage.vue:144 / audit-logs.ts:20). Matches the Rust
// AuditLogDetailResponse.operation_json shape (audit/api.rs:44,80-82).
type AuditLogResponse struct {
	ID            string          `json:"id"`
	EntityType    string          `json:"entityType"`
	EntityID      string          `json:"entityId"`
	Operation     string          `json:"operation"`
	OperationJSON *string         `json:"operationJson,omitempty"`
	PrincipalID   *string         `json:"principalId,omitempty"`
	PrincipalName *string         `json:"principalName,omitempty"`
	ApplicationID *string         `json:"applicationId,omitempty"`
	ClientID      *string         `json:"clientId,omitempty"`
	PerformedAt   httpcompat.Time `json:"performedAt"`
}

func fromEntity(l *audit.Log) AuditLogResponse {
	var opJSON *string
	if len(l.OperationJSON) > 0 && string(l.OperationJSON) != "null" {
		// Compact the raw JSON and emit it as a string so the SPA can
		// JSON.parse it client-side.
		var buf bytes.Buffer
		if err := json.Compact(&buf, l.OperationJSON); err == nil {
			s := buf.String()
			opJSON = &s
		} else {
			s := string(l.OperationJSON)
			opJSON = &s
		}
	}
	return AuditLogResponse{
		ID:            l.ID,
		EntityType:    l.EntityType,
		EntityID:      l.EntityID,
		Operation:     l.Operation,
		OperationJSON: opJSON,
		PrincipalID:   l.PrincipalID,
		PrincipalName: l.PrincipalName,
		ApplicationID: l.ApplicationID,
		ClientID:      l.ClientID,
		PerformedAt:   jsontime.New(l.PerformedAt),
	}
}

// AuditLogListResponse is the wire shape for GET /api/audit-logs.
// Matches the SPA (audit-logs.ts:23-28) which consumes `auditLogs` and
// `hasMore`, and the Rust AuditLogListResponse (audit/api.rs:100-107).
type AuditLogListResponse struct {
	AuditLogs  []AuditLogResponse `json:"auditLogs"`
	HasMore    bool               `json:"hasMore"`
	NextCursor *string            `json:"nextCursor,omitempty"`
}

// AuditLogEntityTypesResponse is the wire shape for GET
// /api/audit-logs/entity-types (SPA expects `{entityTypes}`).
type AuditLogEntityTypesResponse struct {
	EntityTypes []string `json:"entityTypes"`
}

// AuditLogOperationsResponse is the wire shape for GET
// /api/audit-logs/operations (SPA expects `{operations}`).
type AuditLogOperationsResponse struct {
	Operations []string `json:"operations"`
}

// AuditLogApplicationIDsResponse is the wire shape for GET
// /api/audit-logs/application-ids (SPA expects `{applicationIds}`).
type AuditLogApplicationIDsResponse struct {
	ApplicationIDs []string `json:"applicationIds"`
}

// AuditLogClientIDsResponse is the wire shape for GET
// /api/audit-logs/client-ids (SPA expects `{clientIds}`).
type AuditLogClientIDsResponse struct {
	ClientIDs []string `json:"clientIds"`
}
