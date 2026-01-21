package audit

import (
	"time"
)

// AuditLog represents an audit log entry tracking operations performed on entities.
// Generic audit log that can track any entity type and operation.
// Stores the full operation payload as JSON for complete audit trail.
// Collection: audit_logs
type AuditLog struct {
	ID            string    `bson:"_id" json:"id"`
	EntityType    string    `bson:"entityType" json:"entityType"`       // The type of entity (e.g., "EventType", "Client")
	EntityID      string    `bson:"entityId,omitempty" json:"entityId,omitempty"` // The entity's TSID
	Operation     string    `bson:"operation" json:"operation"`         // The operation name (e.g., "CreateEventType", "UpdateClient")
	OperationJSON string    `bson:"operationJson,omitempty" json:"operationJson,omitempty"` // The full operation record as JSON
	PrincipalID   string    `bson:"principalId,omitempty" json:"principalId,omitempty"`     // The principal who performed the operation
	PerformedAt   time.Time `bson:"performedAt" json:"performedAt"`     // When the operation was performed
}

// AuditLogDTO is the response DTO for audit logs (without full operation JSON)
type AuditLogDTO struct {
	ID            string    `json:"id"`
	EntityType    string    `json:"entityType"`
	EntityID      string    `json:"entityId,omitempty"`
	Operation     string    `json:"operation"`
	PrincipalID   string    `json:"principalId,omitempty"`
	PrincipalName string    `json:"principalName,omitempty"`
	PerformedAt   time.Time `json:"performedAt"`
}

// AuditLogDetailDTO is the detailed response DTO including operation JSON
type AuditLogDetailDTO struct {
	ID            string    `json:"id"`
	EntityType    string    `json:"entityType"`
	EntityID      string    `json:"entityId,omitempty"`
	Operation     string    `json:"operation"`
	OperationJSON string    `json:"operationJson,omitempty"`
	PrincipalID   string    `json:"principalId,omitempty"`
	PrincipalName string    `json:"principalName,omitempty"`
	PerformedAt   time.Time `json:"performedAt"`
}

// AuditLogListResponse is the response for list operations
type AuditLogListResponse struct {
	AuditLogs []AuditLogDTO `json:"auditLogs"`
	Total     int64         `json:"total"`
	Page      int           `json:"page"`
	PageSize  int           `json:"pageSize"`
}

// ToDTO converts an AuditLog to AuditLogDTO
func (a *AuditLog) ToDTO(principalName string) AuditLogDTO {
	return AuditLogDTO{
		ID:            a.ID,
		EntityType:    a.EntityType,
		EntityID:      a.EntityID,
		Operation:     a.Operation,
		PrincipalID:   a.PrincipalID,
		PrincipalName: principalName,
		PerformedAt:   a.PerformedAt,
	}
}

// ToDetailDTO converts an AuditLog to AuditLogDetailDTO
func (a *AuditLog) ToDetailDTO(principalName string) AuditLogDetailDTO {
	return AuditLogDetailDTO{
		ID:            a.ID,
		EntityType:    a.EntityType,
		EntityID:      a.EntityID,
		Operation:     a.Operation,
		OperationJSON: a.OperationJSON,
		PrincipalID:   a.PrincipalID,
		PrincipalName: principalName,
		PerformedAt:   a.PerformedAt,
	}
}
