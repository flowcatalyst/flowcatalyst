// Package api provides HTTP handlers for the platform API
package api

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"log/slog"

	"go.flowcatalyst.tech/internal/common/tsid"
	"go.flowcatalyst.tech/internal/platform/auth/oidc"
)

// OAuthClientAdminHandler handles OAuth client admin operations
type OAuthClientAdminHandler struct {
	repo *oidc.Repository
}

// NewOAuthClientAdminHandler creates a new OAuth client admin handler
func NewOAuthClientAdminHandler(repo *oidc.Repository) *OAuthClientAdminHandler {
	return &OAuthClientAdminHandler{repo: repo}
}

// CreateOAuthClientRequest represents a request to create an OAuth client
type CreateOAuthClientRequest struct {
	ClientName     string   `json:"clientName"`
	ClientType     string   `json:"clientType"` // PUBLIC or CONFIDENTIAL
	RedirectURIs   []string `json:"redirectUris"`
	GrantTypes     []string `json:"grantTypes"`
	DefaultScopes  []string `json:"defaultScopes,omitempty"`
	PKCERequired   *bool    `json:"pkceRequired,omitempty"`
	ApplicationIDs []string `json:"applicationIds,omitempty"`
}

// UpdateOAuthClientRequest represents a request to update an OAuth client
type UpdateOAuthClientRequest struct {
	ClientName     string   `json:"clientName,omitempty"`
	RedirectURIs   []string `json:"redirectUris,omitempty"`
	GrantTypes     []string `json:"grantTypes,omitempty"`
	DefaultScopes  []string `json:"defaultScopes,omitempty"`
	PKCERequired   *bool    `json:"pkceRequired,omitempty"`
	ApplicationIDs []string `json:"applicationIds,omitempty"`
}

// OAuthClientResponse represents an OAuth client in API responses
type OAuthClientResponse struct {
	ID                        string    `json:"id"`
	ClientID                  string    `json:"clientId"`
	ClientName                string    `json:"clientName"`
	ClientType                string    `json:"clientType"`
	RedirectURIs              []string  `json:"redirectUris"`
	GrantTypes                []string  `json:"grantTypes"`
	DefaultScopes             []string  `json:"defaultScopes,omitempty"`
	PKCERequired              bool      `json:"pkceRequired"`
	ApplicationIDs            []string  `json:"applicationIds,omitempty"`
	ServiceAccountPrincipalID string    `json:"serviceAccountPrincipalId,omitempty"`
	Active                    bool      `json:"active"`
	CreatedAt                 time.Time `json:"createdAt"`
	UpdatedAt                 time.Time `json:"updatedAt"`
}

// CreateOAuthClientResponse includes the client secret (only shown on creation)
type CreateOAuthClientResponse struct {
	OAuthClientResponse
	ClientSecret string `json:"clientSecret,omitempty"` // Only returned on creation
}

// RotateSecretResponse contains the new client secret
type RotateSecretResponse struct {
	ClientSecret string `json:"clientSecret"`
}

// toResponse converts an OAuthClient to its API response form
func toOAuthClientResponse(c *oidc.OAuthClient) OAuthClientResponse {
	return OAuthClientResponse{
		ID:                        c.ID,
		ClientID:                  c.ClientID,
		ClientName:                c.ClientName,
		ClientType:                string(c.ClientType),
		RedirectURIs:              c.RedirectURIs,
		GrantTypes:                c.GrantTypes,
		DefaultScopes:             c.DefaultScopes,
		PKCERequired:              c.PKCERequired,
		ApplicationIDs:            c.ApplicationIDs,
		ServiceAccountPrincipalID: c.ServiceAccountPrincipalID,
		Active:                    c.Active,
		CreatedAt:                 c.CreatedAt,
		UpdatedAt:                 c.UpdatedAt,
	}
}

// generateClientID creates a new client ID (public identifier)
func generateClientID() string {
	return tsid.Generate()
}

// generateClientSecret creates a new client secret (32 bytes, base64 encoded)
func generateClientSecret() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(bytes), nil
}

// List handles GET /api/admin/platform/oauth-clients
func (h *OAuthClientAdminHandler) List(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	clients, err := h.repo.FindAllClients(ctx)
	if err != nil {
		slog.Error("Failed to list OAuth clients", "error", err)
		WriteInternalError(w, "Failed to list OAuth clients")
		return
	}

	response := make([]OAuthClientResponse, len(clients))
	for i, c := range clients {
		response[i] = toOAuthClientResponse(c)
	}

	WriteJSON(w, http.StatusOK, response)
}

// Get handles GET /api/admin/platform/oauth-clients/{id}
func (h *OAuthClientAdminHandler) Get(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")

	client, err := h.repo.FindClientByID(ctx, id)
	if err != nil {
		if err == oidc.ErrNotFound {
			WriteNotFound(w, "OAuth client not found")
			return
		}
		slog.Error("Failed to get OAuth client", "error", err, "id", id)
		WriteInternalError(w, "Failed to get OAuth client")
		return
	}

	WriteJSON(w, http.StatusOK, toOAuthClientResponse(client))
}

// GetByClientID handles GET /api/admin/platform/oauth-clients/by-client-id/{clientId}
func (h *OAuthClientAdminHandler) GetByClientID(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	clientID := chi.URLParam(r, "clientId")

	client, err := h.repo.FindClientByClientID(ctx, clientID)
	if err != nil {
		if err == oidc.ErrNotFound {
			WriteNotFound(w, "OAuth client not found")
			return
		}
		slog.Error("Failed to get OAuth client", "error", err, "clientId", clientID)
		WriteInternalError(w, "Failed to get OAuth client")
		return
	}

	WriteJSON(w, http.StatusOK, toOAuthClientResponse(client))
}

// Create handles POST /api/admin/platform/oauth-clients
func (h *OAuthClientAdminHandler) Create(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req CreateOAuthClientRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteBadRequest(w, "Invalid request body")
		return
	}

	// Validate required fields
	if req.ClientName == "" {
		WriteBadRequest(w, "clientName is required")
		return
	}
	if req.ClientType != "PUBLIC" && req.ClientType != "CONFIDENTIAL" {
		WriteBadRequest(w, "clientType must be PUBLIC or CONFIDENTIAL")
		return
	}
	if len(req.RedirectURIs) == 0 {
		WriteBadRequest(w, "at least one redirectUri is required")
		return
	}
	if len(req.GrantTypes) == 0 {
		WriteBadRequest(w, "at least one grantType is required")
		return
	}

	// Generate client credentials
	clientID := generateClientID()

	var clientSecretRef string
	var clientSecret string

	// Only confidential clients get a secret
	if req.ClientType == "CONFIDENTIAL" {
		secret, err := generateClientSecret()
		if err != nil {
			slog.Error("Failed to generate client secret", "error", err)
			WriteInternalError(w, "Failed to generate client secret")
			return
		}
		clientSecret = secret
		// TODO: Encrypt secret before storing (will be done in Phase 3 with secrets package)
		clientSecretRef = secret
	}

	// Determine PKCE requirement
	pkceRequired := req.ClientType == "PUBLIC" // Default to true for public clients
	if req.PKCERequired != nil {
		pkceRequired = *req.PKCERequired
	}

	client := &oidc.OAuthClient{
		ID:              tsid.Generate(),
		ClientID:        clientID,
		ClientName:      req.ClientName,
		ClientType:      oidc.OAuthClientType(req.ClientType),
		ClientSecretRef: clientSecretRef,
		RedirectURIs:    req.RedirectURIs,
		GrantTypes:      req.GrantTypes,
		DefaultScopes:   req.DefaultScopes,
		PKCERequired:    pkceRequired,
		ApplicationIDs:  req.ApplicationIDs,
		Active:          true,
	}

	if err := h.repo.InsertClient(ctx, client); err != nil {
		slog.Error("Failed to create OAuth client", "error", err)
		WriteInternalError(w, "Failed to create OAuth client")
		return
	}

	slog.Info("OAuth client created", "id", client.ID, "clientId", client.ClientID, "clientName", client.ClientName, "clientType", string(client.ClientType))

	response := CreateOAuthClientResponse{
		OAuthClientResponse: toOAuthClientResponse(client),
		ClientSecret:        clientSecret,
	}

	WriteJSON(w, http.StatusCreated, response)
}

// Update handles PUT /api/admin/platform/oauth-clients/{id}
func (h *OAuthClientAdminHandler) Update(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")

	client, err := h.repo.FindClientByID(ctx, id)
	if err != nil {
		if err == oidc.ErrNotFound {
			WriteNotFound(w, "OAuth client not found")
			return
		}
		slog.Error("Failed to get OAuth client", "error", err, "id", id)
		WriteInternalError(w, "Failed to get OAuth client")
		return
	}

	var req UpdateOAuthClientRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteBadRequest(w, "Invalid request body")
		return
	}

	// Apply updates
	if req.ClientName != "" {
		client.ClientName = req.ClientName
	}
	if req.RedirectURIs != nil {
		client.RedirectURIs = req.RedirectURIs
	}
	if req.GrantTypes != nil {
		client.GrantTypes = req.GrantTypes
	}
	if req.DefaultScopes != nil {
		client.DefaultScopes = req.DefaultScopes
	}
	if req.PKCERequired != nil {
		client.PKCERequired = *req.PKCERequired
	}
	if req.ApplicationIDs != nil {
		client.ApplicationIDs = req.ApplicationIDs
	}

	if err := h.repo.UpdateClient(ctx, client); err != nil {
		slog.Error("Failed to update OAuth client", "error", err, "id", id)
		WriteInternalError(w, "Failed to update OAuth client")
		return
	}

	slog.Info("OAuth client updated", "id", client.ID, "clientId", client.ClientID)

	WriteJSON(w, http.StatusOK, toOAuthClientResponse(client))
}

// RotateSecret handles POST /api/admin/platform/oauth-clients/{id}/rotate-secret
func (h *OAuthClientAdminHandler) RotateSecret(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")

	client, err := h.repo.FindClientByID(ctx, id)
	if err != nil {
		if err == oidc.ErrNotFound {
			WriteNotFound(w, "OAuth client not found")
			return
		}
		slog.Error("Failed to get OAuth client", "error", err, "id", id)
		WriteInternalError(w, "Failed to get OAuth client")
		return
	}

	// Only confidential clients have secrets
	if client.ClientType == oidc.OAuthClientTypePublic {
		WriteBadRequest(w, "Public clients do not have secrets")
		return
	}

	// Generate new secret
	newSecret, err := generateClientSecret()
	if err != nil {
		slog.Error("Failed to generate new client secret", "error", err)
		WriteInternalError(w, "Failed to generate new client secret")
		return
	}

	// TODO: Encrypt secret before storing (will be done in Phase 3 with secrets package)
	client.ClientSecretRef = newSecret

	if err := h.repo.UpdateClient(ctx, client); err != nil {
		slog.Error("Failed to update OAuth client secret", "error", err, "id", id)
		WriteInternalError(w, "Failed to update OAuth client secret")
		return
	}

	slog.Info("OAuth client secret rotated", "id", client.ID, "clientId", client.ClientID)

	WriteJSON(w, http.StatusOK, RotateSecretResponse{ClientSecret: newSecret})
}

// Activate handles POST /api/admin/platform/oauth-clients/{id}/activate
func (h *OAuthClientAdminHandler) Activate(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")

	client, err := h.repo.FindClientByID(ctx, id)
	if err != nil {
		if err == oidc.ErrNotFound {
			WriteNotFound(w, "OAuth client not found")
			return
		}
		slog.Error("Failed to get OAuth client", "error", err, "id", id)
		WriteInternalError(w, "Failed to get OAuth client")
		return
	}

	if client.Active {
		WriteJSON(w, http.StatusOK, toOAuthClientResponse(client))
		return
	}

	client.Active = true
	if err := h.repo.UpdateClient(ctx, client); err != nil {
		slog.Error("Failed to activate OAuth client", "error", err, "id", id)
		WriteInternalError(w, "Failed to activate OAuth client")
		return
	}

	slog.Info("OAuth client activated", "id", client.ID, "clientId", client.ClientID)

	WriteJSON(w, http.StatusOK, toOAuthClientResponse(client))
}

// Deactivate handles POST /api/admin/platform/oauth-clients/{id}/deactivate
func (h *OAuthClientAdminHandler) Deactivate(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")

	client, err := h.repo.FindClientByID(ctx, id)
	if err != nil {
		if err == oidc.ErrNotFound {
			WriteNotFound(w, "OAuth client not found")
			return
		}
		slog.Error("Failed to get OAuth client", "error", err, "id", id)
		WriteInternalError(w, "Failed to get OAuth client")
		return
	}

	if !client.Active {
		WriteJSON(w, http.StatusOK, toOAuthClientResponse(client))
		return
	}

	client.Active = false
	if err := h.repo.UpdateClient(ctx, client); err != nil {
		slog.Error("Failed to deactivate OAuth client", "error", err, "id", id)
		WriteInternalError(w, "Failed to deactivate OAuth client")
		return
	}

	slog.Info("OAuth client deactivated", "id", client.ID, "clientId", client.ClientID)

	WriteJSON(w, http.StatusOK, toOAuthClientResponse(client))
}

// Delete handles DELETE /api/admin/platform/oauth-clients/{id}
func (h *OAuthClientAdminHandler) Delete(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")

	err := h.repo.DeleteClient(ctx, id)
	if err != nil {
		if err == oidc.ErrNotFound {
			WriteNotFound(w, "OAuth client not found")
			return
		}
		slog.Error("Failed to delete OAuth client", "error", err, "id", id)
		WriteInternalError(w, "Failed to delete OAuth client")
		return
	}

	slog.Info("OAuth client deleted", "id", id)

	w.WriteHeader(http.StatusNoContent)
}
