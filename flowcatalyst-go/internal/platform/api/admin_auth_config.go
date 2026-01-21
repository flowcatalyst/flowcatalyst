package api

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"go.flowcatalyst.tech/internal/platform/client"
)

// AuthConfigHandler handles auth config admin API requests
type AuthConfigHandler struct {
	clientRepo client.Repository
}

// NewAuthConfigHandler creates a new auth config handler
func NewAuthConfigHandler(clientRepo client.Repository) *AuthConfigHandler {
	return &AuthConfigHandler{
		clientRepo: clientRepo,
	}
}

// AuthConfigDTO is the response DTO for auth config
type AuthConfigDTO struct {
	ID                  string   `json:"id"`
	EmailDomain         string   `json:"emailDomain"`
	ConfigType          string   `json:"configType"`
	PrimaryClientID     string   `json:"primaryClientId,omitempty"`
	AdditionalClientIDs []string `json:"additionalClientIds,omitempty"`
	GrantedClientIDs    []string `json:"grantedClientIds,omitempty"`
	AuthProvider        string   `json:"authProvider"`
	IdpType             string   `json:"idpType,omitempty"`
	OIDCIssuerURL       string   `json:"oidcIssuerUrl,omitempty"`
	OIDCClientID        string   `json:"oidcClientId,omitempty"`
	OIDCMultiTenant     bool     `json:"oidcMultiTenant,omitempty"`
	OIDCIssuerPattern   string   `json:"oidcIssuerPattern,omitempty"`
	EntraTenantID       string   `json:"entraTenantId,omitempty"`
	GroupsClaim         string   `json:"groupsClaim,omitempty"`
	RolesClaim          string   `json:"rolesClaim,omitempty"`
	CreatedAt           string   `json:"createdAt"`
	UpdatedAt           string   `json:"updatedAt"`
}

// CreateAuthConfigRequest is the request to create an auth config
type CreateAuthConfigRequest struct {
	EmailDomain         string   `json:"emailDomain"`
	ConfigType          string   `json:"configType"`
	PrimaryClientID     string   `json:"primaryClientId,omitempty"`
	AdditionalClientIDs []string `json:"additionalClientIds,omitempty"`
	GrantedClientIDs    []string `json:"grantedClientIds,omitempty"`
	AuthProvider        string   `json:"authProvider"`
	IdpType             string   `json:"idpType,omitempty"`
	OIDCIssuerURL       string   `json:"oidcIssuerUrl,omitempty"`
	OIDCClientID        string   `json:"oidcClientId,omitempty"`
	OIDCClientSecretRef string   `json:"oidcClientSecretRef,omitempty"`
	OIDCMultiTenant     bool     `json:"oidcMultiTenant,omitempty"`
	OIDCIssuerPattern   string   `json:"oidcIssuerPattern,omitempty"`
	EntraTenantID       string   `json:"entraTenantId,omitempty"`
	GroupsClaim         string   `json:"groupsClaim,omitempty"`
	RolesClaim          string   `json:"rolesClaim,omitempty"`
}

// List handles GET /api/admin/platform/auth-configs
//
//	@Summary		List auth configs
//	@Description	Returns all authentication configurations
//	@Tags			Auth Config
//	@Produce		json
//	@Success		200	{array}		AuthConfigDTO
//	@Failure		401	{object}	ErrorResponse
//	@Security		BearerAuth
//	@Router			/admin/platform/auth-configs [get]
func (h *AuthConfigHandler) List(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	configs, err := h.clientRepo.FindAllAuthConfigs(ctx)
	if err != nil {
		WriteInternalError(w, "Failed to fetch auth configs")
		return
	}

	dtos := make([]AuthConfigDTO, len(configs))
	for i, c := range configs {
		dtos[i] = toAuthConfigDTO(c)
	}

	WriteJSON(w, http.StatusOK, dtos)
}

// Create handles POST /api/admin/platform/auth-configs
//
//	@Summary		Create auth config
//	@Description	Creates a new authentication configuration for an email domain
//	@Tags			Auth Config
//	@Accept			json
//	@Produce		json
//	@Param			config	body		CreateAuthConfigRequest	true	"Auth config to create"
//	@Success		201		{object}	AuthConfigDTO
//	@Failure		400		{object}	ErrorResponse
//	@Failure		401		{object}	ErrorResponse
//	@Failure		409		{object}	ErrorResponse
//	@Security		BearerAuth
//	@Router			/admin/platform/auth-configs [post]
func (h *AuthConfigHandler) Create(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req CreateAuthConfigRequest
	if err := DecodeJSON(r, &req); err != nil {
		WriteBadRequest(w, "Invalid request body")
		return
	}

	if req.EmailDomain == "" {
		WriteBadRequest(w, "emailDomain is required")
		return
	}

	config := &client.ClientAuthConfig{
		EmailDomain:         req.EmailDomain,
		ConfigType:          client.AuthConfigType(req.ConfigType),
		PrimaryClientID:     req.PrimaryClientID,
		AdditionalClientIDs: req.AdditionalClientIDs,
		GrantedClientIDs:    req.GrantedClientIDs,
		AuthProvider:        client.AuthProvider(req.AuthProvider),
		IdpType:             req.IdpType,
		OIDCIssuerURL:       req.OIDCIssuerURL,
		OIDCClientID:        req.OIDCClientID,
		OIDCClientSecretRef: req.OIDCClientSecretRef,
		OIDCMultiTenant:     req.OIDCMultiTenant,
		OIDCIssuerPattern:   req.OIDCIssuerPattern,
		EntraTenantID:       req.EntraTenantID,
		GroupsClaim:         req.GroupsClaim,
		RolesClaim:          req.RolesClaim,
	}

	if err := h.clientRepo.InsertAuthConfig(ctx, config); err != nil {
		if errors.Is(err, client.ErrDuplicateDomain) {
			WriteConflict(w, "Auth config for this domain already exists")
			return
		}
		WriteInternalError(w, "Failed to create auth config")
		return
	}

	WriteJSON(w, http.StatusCreated, toAuthConfigDTO(config))
}

// Get handles GET /api/admin/platform/auth-configs/{id}
//
//	@Summary		Get auth config by ID
//	@Description	Returns a specific authentication configuration
//	@Tags			Auth Config
//	@Produce		json
//	@Param			id	path		string	true	"Auth config ID"
//	@Success		200	{object}	AuthConfigDTO
//	@Failure		404	{object}	ErrorResponse
//	@Failure		401	{object}	ErrorResponse
//	@Security		BearerAuth
//	@Router			/admin/platform/auth-configs/{id} [get]
func (h *AuthConfigHandler) Get(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")

	// Try to find by ID first, then by domain
	configs, err := h.clientRepo.FindAllAuthConfigs(ctx)
	if err != nil {
		WriteInternalError(w, "Failed to fetch auth config")
		return
	}

	for _, c := range configs {
		if c.ID == id || c.EmailDomain == id {
			WriteJSON(w, http.StatusOK, toAuthConfigDTO(c))
			return
		}
	}

	WriteNotFound(w, "Auth config not found")
}

// Update handles PUT /api/admin/platform/auth-configs/{id}
//
//	@Summary		Update auth config
//	@Description	Updates an existing authentication configuration
//	@Tags			Auth Config
//	@Accept			json
//	@Produce		json
//	@Param			id		path		string					true	"Auth config ID"
//	@Param			config	body		CreateAuthConfigRequest	true	"Auth config updates"
//	@Success		200		{object}	AuthConfigDTO
//	@Failure		400		{object}	ErrorResponse
//	@Failure		404		{object}	ErrorResponse
//	@Failure		401		{object}	ErrorResponse
//	@Security		BearerAuth
//	@Router			/admin/platform/auth-configs/{id} [put]
func (h *AuthConfigHandler) Update(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")

	var req CreateAuthConfigRequest
	if err := DecodeJSON(r, &req); err != nil {
		WriteBadRequest(w, "Invalid request body")
		return
	}

	config := &client.ClientAuthConfig{
		ID:                  id,
		EmailDomain:         req.EmailDomain,
		ConfigType:          client.AuthConfigType(req.ConfigType),
		PrimaryClientID:     req.PrimaryClientID,
		AdditionalClientIDs: req.AdditionalClientIDs,
		GrantedClientIDs:    req.GrantedClientIDs,
		AuthProvider:        client.AuthProvider(req.AuthProvider),
		IdpType:             req.IdpType,
		OIDCIssuerURL:       req.OIDCIssuerURL,
		OIDCClientID:        req.OIDCClientID,
		OIDCClientSecretRef: req.OIDCClientSecretRef,
		OIDCMultiTenant:     req.OIDCMultiTenant,
		OIDCIssuerPattern:   req.OIDCIssuerPattern,
		EntraTenantID:       req.EntraTenantID,
		GroupsClaim:         req.GroupsClaim,
		RolesClaim:          req.RolesClaim,
	}

	if err := h.clientRepo.UpdateAuthConfig(ctx, config); err != nil {
		if errors.Is(err, client.ErrNotFound) {
			WriteNotFound(w, "Auth config not found")
			return
		}
		WriteInternalError(w, "Failed to update auth config")
		return
	}

	WriteJSON(w, http.StatusOK, toAuthConfigDTO(config))
}

// Delete handles DELETE /api/admin/platform/auth-configs/{id}
//
//	@Summary		Delete auth config
//	@Description	Deletes an authentication configuration
//	@Tags			Auth Config
//	@Param			id	path	string	true	"Auth config ID"
//	@Success		204	"No content"
//	@Failure		404	{object}	ErrorResponse
//	@Failure		401	{object}	ErrorResponse
//	@Security		BearerAuth
//	@Router			/admin/platform/auth-configs/{id} [delete]
func (h *AuthConfigHandler) Delete(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")

	if err := h.clientRepo.DeleteAuthConfig(ctx, id); err != nil {
		if errors.Is(err, client.ErrNotFound) {
			WriteNotFound(w, "Auth config not found")
			return
		}
		WriteInternalError(w, "Failed to delete auth config")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func toAuthConfigDTO(c *client.ClientAuthConfig) AuthConfigDTO {
	return AuthConfigDTO{
		ID:                  c.ID,
		EmailDomain:         c.EmailDomain,
		ConfigType:          string(c.ConfigType),
		PrimaryClientID:     c.PrimaryClientID,
		AdditionalClientIDs: c.AdditionalClientIDs,
		GrantedClientIDs:    c.GrantedClientIDs,
		AuthProvider:        string(c.AuthProvider),
		IdpType:             c.IdpType,
		OIDCIssuerURL:       c.OIDCIssuerURL,
		OIDCClientID:        c.OIDCClientID,
		OIDCMultiTenant:     c.OIDCMultiTenant,
		OIDCIssuerPattern:   c.OIDCIssuerPattern,
		EntraTenantID:       c.EntraTenantID,
		GroupsClaim:         c.GroupsClaim,
		RolesClaim:          c.RolesClaim,
		CreatedAt:           c.CreatedAt.Format("2006-01-02T15:04:05Z"),
		UpdatedAt:           c.UpdatedAt.Format("2006-01-02T15:04:05Z"),
	}
}
