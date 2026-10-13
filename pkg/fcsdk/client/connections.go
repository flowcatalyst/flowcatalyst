package client

import "context"

// ─── Request DTOs ────────────────────────────────────────────────────

// CreateConnectionRequest — POST /api/connections.
type CreateConnectionRequest struct {
	Code             string `json:"code"`
	Name             string `json:"name"`
	Description      string `json:"description,omitempty"`
	ServiceAccountID string `json:"serviceAccountId"`
	ExternalID       string `json:"externalId,omitempty"`
	ClientID         string `json:"clientId,omitempty"`
	// ApplicationCode owns the connection to that application; empty
	// leaves it unowned.
	ApplicationCode string `json:"applicationCode,omitempty"`
}

// UpdateConnectionRequest — PUT /api/connections/{id}.
type UpdateConnectionRequest struct {
	Name        *string `json:"name,omitempty"`
	Description *string `json:"description,omitempty"`
	ExternalID  *string `json:"externalId,omitempty"`
	Status      *string `json:"status,omitempty"`
	// ApplicationCode re-homes the connection to that application.
	ApplicationCode *string `json:"applicationCode,omitempty"`
}

// ─── Response DTOs ───────────────────────────────────────────────────

// ConnectionResponse is the platform's connection aggregate.
type ConnectionResponse struct {
	ID               string `json:"id"`
	Code             string `json:"code"`
	Name             string `json:"name"`
	Description      string `json:"description,omitempty"`
	ExternalID       string `json:"externalId,omitempty"`
	Status           string `json:"status"`
	ServiceAccountID string `json:"serviceAccountId"`
	ClientID         string `json:"clientId,omitempty"`
	ClientIdentifier string `json:"clientIdentifier,omitempty"`
	// ApplicationCode is the owning application, empty for a shared connection.
	ApplicationCode string `json:"applicationCode,omitempty"`
	// Source is how the connection was created (e.g. UI or SDK).
	Source    string `json:"source"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

// ConnectionsListResponse — GET /api/connections.
type ConnectionsListResponse struct {
	Connections []ConnectionResponse `json:"connections"`
	Total       uint64               `json:"total,omitempty"`
}

// SyncConnectionItem — one connection in the per-app sync payload. A
// connection carries nothing environment-specific: the platform assigns the
// application's own service account, so one definition serves every
// environment. It exists so a subscription's ConnectionCode resolves.
type SyncConnectionItem struct {
	Code        string `json:"code"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	ExternalID  string `json:"externalId,omitempty"`
}

// SyncConnectionsRequest — body for the per-app sync endpoint. ClientID
// scopes the batch to one client: its id or its identifier slug (prefer the
// slug). Empty syncs global connections.
type SyncConnectionsRequest struct {
	ClientID    string               `json:"clientId,omitempty"`
	Connections []SyncConnectionItem `json:"connections"`
}

// ─── Resource ────────────────────────────────────────────────────────

// ConnectionsResource — /api/connections/*.
type ConnectionsResource struct {
	c *FlowCatalystClient
}

// Create — POST /api/connections. Returns the new connection's id.
func (r *ConnectionsResource) Create(ctx context.Context, req *CreateConnectionRequest) (*CreatedResponse, error) {
	var out CreatedResponse
	if err := r.c.Post(ctx, "/api/connections", req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Get — GET /api/connections/{id}.
func (r *ConnectionsResource) Get(ctx context.Context, id string) (*ConnectionResponse, error) {
	var out ConnectionResponse
	if err := r.c.Get(ctx, "/api/connections/"+id, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// List — GET /api/connections with optional filters.
func (r *ConnectionsResource) List(ctx context.Context, clientID, status, serviceAccountID string) (*ConnectionsListResponse, error) {
	q := NewQuery().
		String("clientId", clientID).
		String("status", status).
		String("serviceAccountId", serviceAccountID).
		Encode()
	var out ConnectionsListResponse
	if err := r.c.Get(ctx, "/api/connections"+q, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Update — PUT /api/connections/{id}.
func (r *ConnectionsResource) Update(ctx context.Context, id string, req *UpdateConnectionRequest) error {
	return r.c.Put(ctx, "/api/connections/"+id, req, nil)
}

// Delete — DELETE /api/connections/{id}.
func (r *ConnectionsResource) Delete(ctx context.Context, id string) error {
	return r.c.Delete(ctx, "/api/connections/"+id, nil)
}

// Pause — POST /api/connections/{id}/pause.
func (r *ConnectionsResource) Pause(ctx context.Context, id string) error {
	return r.c.Post(ctx, "/api/connections/"+id+"/pause", nil, nil)
}

// Activate — POST /api/connections/{id}/activate.
func (r *ConnectionsResource) Activate(ctx context.Context, id string) error {
	return r.c.Post(ctx, "/api/connections/"+id+"/activate", nil, nil)
}

// Sync — POST /api/applications/{appCode}/connections/sync. Sync
// connections before the subscriptions that name them by ConnectionCode.
func (r *ConnectionsResource) Sync(ctx context.Context, appCode string, req *SyncConnectionsRequest, removeUnlisted bool) (*SyncResult, error) {
	var out SyncResult
	if err := r.c.Post(ctx, syncPath(appCode, "connections", removeUnlisted), req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
