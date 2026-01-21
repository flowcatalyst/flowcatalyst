package api

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"go.flowcatalyst.tech/internal/platform/audit"
	"go.flowcatalyst.tech/internal/platform/principal"
)

// AuditLogHandler handles audit log admin API requests
type AuditLogHandler struct {
	auditRepo     *audit.Repository
	principalRepo principal.Repository
}

// NewAuditLogHandler creates a new audit log handler
func NewAuditLogHandler(auditRepo *audit.Repository, principalRepo principal.Repository) *AuditLogHandler {
	return &AuditLogHandler{
		auditRepo:     auditRepo,
		principalRepo: principalRepo,
	}
}

// List handles GET /api/admin/platform/audit-logs
//
//	@Summary		List audit logs
//	@Description	Returns audit logs with optional filtering by entity type, entity ID, principal, or operation
//	@Tags			Audit Logs
//	@Accept			json
//	@Produce		json
//	@Param			entityType	query		string	false	"Filter by entity type"
//	@Param			entityId	query		string	false	"Filter by entity ID"
//	@Param			principalId	query		string	false	"Filter by principal ID"
//	@Param			operation	query		string	false	"Filter by operation name"
//	@Param			page		query		int		false	"Page number (0-based)"	default(0)
//	@Param			pageSize	query		int		false	"Page size"					default(50)
//	@Success		200			{object}	audit.AuditLogListResponse
//	@Failure		401			{object}	ErrorResponse
//	@Security		BearerAuth
//	@Router			/admin/platform/audit-logs [get]
func (h *AuditLogHandler) List(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Get query parameters
	entityType := r.URL.Query().Get("entityType")
	entityID := r.URL.Query().Get("entityId")
	principalID := r.URL.Query().Get("principalId")
	operation := r.URL.Query().Get("operation")

	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 0 {
		page = 0
	}
	pageSize, _ := strconv.Atoi(r.URL.Query().Get("pageSize"))
	if pageSize <= 0 {
		pageSize = 50
	}
	if pageSize > 100 {
		pageSize = 100
	}

	var logs []*audit.AuditLog
	var total int64
	var err error

	// Apply filters
	if entityType != "" && entityID != "" {
		logs, err = h.auditRepo.FindByEntity(ctx, entityType, entityID)
		if err == nil {
			total = int64(len(logs))
		}
	} else if entityType != "" {
		logs, err = h.auditRepo.FindByEntityTypePaged(ctx, entityType, page, pageSize)
		if err == nil {
			total, _ = h.auditRepo.CountByEntityType(ctx, entityType)
		}
	} else if principalID != "" {
		logs, err = h.auditRepo.FindByPrincipal(ctx, principalID)
		if err == nil {
			total = int64(len(logs))
		}
	} else if operation != "" {
		logs, err = h.auditRepo.FindByOperation(ctx, operation)
		if err == nil {
			total = int64(len(logs))
		}
	} else {
		logs, err = h.auditRepo.FindPaged(ctx, page, pageSize)
		if err == nil {
			total, _ = h.auditRepo.Count(ctx)
		}
	}

	if err != nil {
		WriteInternalError(w, "Failed to fetch audit logs")
		return
	}

	// Convert to DTOs with principal names
	dtos := make([]audit.AuditLogDTO, len(logs))
	for i, log := range logs {
		principalName := h.resolvePrincipalName(ctx, log.PrincipalID)
		dtos[i] = log.ToDTO(principalName)
	}

	WriteJSON(w, http.StatusOK, audit.AuditLogListResponse{
		AuditLogs: dtos,
		Total:     total,
		Page:      page,
		PageSize:  pageSize,
	})
}

// Get handles GET /api/admin/platform/audit-logs/{id}
//
//	@Summary		Get audit log by ID
//	@Description	Returns a specific audit log entry including operation JSON
//	@Tags			Audit Logs
//	@Produce		json
//	@Param			id	path		string	true	"Audit log ID"
//	@Success		200	{object}	audit.AuditLogDetailDTO
//	@Failure		404	{object}	ErrorResponse
//	@Failure		401	{object}	ErrorResponse
//	@Security		BearerAuth
//	@Router			/admin/platform/audit-logs/{id} [get]
func (h *AuditLogHandler) Get(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")

	log, err := h.auditRepo.FindByID(ctx, id)
	if err != nil {
		if errors.Is(err, audit.ErrNotFound) {
			WriteNotFound(w, "Audit log not found")
			return
		}
		WriteInternalError(w, "Failed to fetch audit log")
		return
	}

	principalName := h.resolvePrincipalName(ctx, log.PrincipalID)
	WriteJSON(w, http.StatusOK, log.ToDetailDTO(principalName))
}

// GetForEntity handles GET /api/admin/platform/audit-logs/entity/{entityType}/{entityId}
//
//	@Summary		Get audit logs for entity
//	@Description	Returns all audit logs for a specific entity
//	@Tags			Audit Logs
//	@Produce		json
//	@Param			entityType	path		string	true	"Entity type"
//	@Param			entityId	path		string	true	"Entity ID"
//	@Success		200			{object}	map[string]interface{}
//	@Failure		401			{object}	ErrorResponse
//	@Security		BearerAuth
//	@Router			/admin/platform/audit-logs/entity/{entityType}/{entityId} [get]
func (h *AuditLogHandler) GetForEntity(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	entityType := chi.URLParam(r, "entityType")
	entityID := chi.URLParam(r, "entityId")

	logs, err := h.auditRepo.FindByEntity(ctx, entityType, entityID)
	if err != nil {
		WriteInternalError(w, "Failed to fetch audit logs")
		return
	}

	// Convert to DTOs
	dtos := make([]audit.AuditLogDTO, len(logs))
	for i, log := range logs {
		principalName := h.resolvePrincipalName(ctx, log.PrincipalID)
		dtos[i] = log.ToDTO(principalName)
	}

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"auditLogs":  dtos,
		"total":      len(logs),
		"entityType": entityType,
		"entityId":   entityID,
	})
}

// GetEntityTypes handles GET /api/admin/platform/audit-logs/entity-types
//
//	@Summary		Get entity types with audit logs
//	@Description	Returns distinct entity types that have audit log entries
//	@Tags			Audit Logs
//	@Produce		json
//	@Success		200	{object}	map[string]interface{}
//	@Failure		401	{object}	ErrorResponse
//	@Security		BearerAuth
//	@Router			/admin/platform/audit-logs/entity-types [get]
func (h *AuditLogHandler) GetEntityTypes(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	entityTypes, err := h.auditRepo.GetDistinctEntityTypes(ctx)
	if err != nil {
		WriteInternalError(w, "Failed to fetch entity types")
		return
	}

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"entityTypes": entityTypes,
	})
}

// GetOperations handles GET /api/admin/platform/audit-logs/operations
//
//	@Summary		Get operations with audit logs
//	@Description	Returns distinct operation names that have audit log entries
//	@Tags			Audit Logs
//	@Produce		json
//	@Success		200	{object}	map[string]interface{}
//	@Failure		401	{object}	ErrorResponse
//	@Security		BearerAuth
//	@Router			/admin/platform/audit-logs/operations [get]
func (h *AuditLogHandler) GetOperations(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	operations, err := h.auditRepo.GetDistinctOperations(ctx)
	if err != nil {
		WriteInternalError(w, "Failed to fetch operations")
		return
	}

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"operations": operations,
	})
}

// resolvePrincipalName looks up the display name for a principal
func (h *AuditLogHandler) resolvePrincipalName(ctx context.Context, principalID string) string {
	if principalID == "" {
		return ""
	}

	// Handle system principal
	if principalID == audit.SystemPrincipalCode {
		return audit.SystemPrincipalName
	}

	p, err := h.principalRepo.FindByID(ctx, principalID)
	if err != nil || p == nil {
		return ""
	}

	// Prefer name, fall back to email, fall back to "Unknown"
	if p.Name != "" {
		return p.Name
	}
	if p.UserIdentity != nil && p.UserIdentity.Email != "" {
		return p.UserIdentity.Email
	}
	return "Unknown"
}
