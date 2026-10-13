package client

import (
	"context"
	"encoding/json"
	"fmt"
)

// EventTypeListResponse — GET /api/event-types returns `{ items: [...] }`.
type EventTypeListResponse struct {
	Items []EventTypeResponse `json:"items"`
}

// CreateEventTypeRequest is the body for POST /api/event-types.
type CreateEventTypeRequest struct {
	// Code follows {app}:{domain}:{aggregate}:{event}.
	Code string `json:"code"`
	// Human-readable name.
	Name string `json:"name"`
	// Optional description.
	Description string `json:"description,omitempty"`
	// Optional initial JSON schema. Use json.RawMessage to keep the
	// shape opaque end-to-end.
	Schema json.RawMessage `json:"schema,omitempty"`
	// Client ID for multi-tenant scoping.
	ClientID string `json:"clientId,omitempty"`
	// ClientScoped marks the event type as scoped to ClientID.
	ClientScoped bool `json:"clientScoped,omitempty"`
}

// UpdateEventTypeRequest is the body for PUT /api/event-types/{id}.
type UpdateEventTypeRequest struct {
	// Name is required by the platform on every update.
	Name         string  `json:"name"`
	Description  *string `json:"description,omitempty"`
	ClientScoped *bool   `json:"clientScoped,omitempty"`
}

// AddSchemaVersionRequest is the body for POST /api/event-types/{id}/versions.
type AddSchemaVersionRequest struct {
	// Version names the new schema version (e.g. "1.1"); the platform
	// requires it.
	Version string          `json:"version"`
	Schema  json.RawMessage `json:"schema"`
}

// EventTypeResponse is the platform's event-type representation.
type EventTypeResponse struct {
	ID          string `json:"id"`
	Code        string `json:"code"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Status      string `json:"status"`
	Application string `json:"application"`
	Subdomain   string `json:"subdomain"`
	Aggregate   string `json:"aggregate"`
	// EventName is the last segment of Code.
	EventName string `json:"eventName"`
	// Source is how the event type was created (e.g. UI or SDK).
	Source       string                `json:"source"`
	ClientID     string                `json:"clientId,omitempty"`
	CreatedBy    string                `json:"createdBy,omitempty"`
	SpecVersions []SpecVersionResponse `json:"specVersions"`
	CreatedAt    string                `json:"createdAt"`
	UpdatedAt    string                `json:"updatedAt"`
}

// SpecVersionResponse is one schema version on an event type.
type SpecVersionResponse struct {
	Version   string          `json:"version"`
	Status    string          `json:"status"`
	Schema    json.RawMessage `json:"schema"`
	CreatedAt string          `json:"createdAt"`
}

// SyncEventTypeItem is one event type in the per-app sync payload. The sync
// declares code, name and description only: schemas are added with
// AddSchemaVersion.
type SyncEventTypeItem struct {
	Code        string `json:"code"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

// SyncEventTypesRequest is the body for the per-app sync endpoint.
type SyncEventTypesRequest struct {
	EventTypes []SyncEventTypeItem `json:"eventTypes"`
}

// EventTypesResource is the accessor for /api/event-types/*.
// Construct via FlowCatalystClient.EventTypes.
type EventTypesResource struct {
	c *FlowCatalystClient
}

// Create — POST /api/event-types. Returns the new event type's id; call Get
// for the event type.
func (r *EventTypesResource) Create(ctx context.Context, req *CreateEventTypeRequest) (*CreatedResponse, error) {
	var out CreatedResponse
	if err := r.c.Post(ctx, "/api/event-types", req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Get — GET /api/event-types/{id}.
func (r *EventTypesResource) Get(ctx context.Context, id string) (*EventTypeResponse, error) {
	var out EventTypeResponse
	if err := r.c.Get(ctx, "/api/event-types/"+id, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetByCode — GET /api/event-types/by-code/{code}.
func (r *EventTypesResource) GetByCode(ctx context.Context, code string) (*EventTypeResponse, error) {
	var out EventTypeResponse
	if err := r.c.Get(ctx, "/api/event-types/by-code/"+code, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// EventTypeFilters — query parameters for GET /api/event-types. Empty fields
// are omitted.
type EventTypeFilters struct {
	Application string
	Subdomain   string
	Aggregate   string
	Status      string
	ClientID    string
}

// List — GET /api/event-types with optional filters (nil lists everything).
func (r *EventTypesResource) List(ctx context.Context, filters *EventTypeFilters) (*EventTypeListResponse, error) {
	q := ""
	if filters != nil {
		q = NewQuery().
			String("application", filters.Application).
			String("subdomain", filters.Subdomain).
			String("aggregate", filters.Aggregate).
			String("status", filters.Status).
			String("clientId", filters.ClientID).
			Encode()
	}
	var out EventTypeListResponse
	if err := r.c.Get(ctx, "/api/event-types"+q, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Update — PUT /api/event-types/{id}. The platform answers 204 No Content;
// call Get(id) for the updated event type.
func (r *EventTypesResource) Update(ctx context.Context, id string, req *UpdateEventTypeRequest) error {
	return r.c.Put(ctx, "/api/event-types/"+id, req, nil)
}

// AddSchemaVersion — POST /api/event-types/{id}/versions.
func (r *EventTypesResource) AddSchemaVersion(ctx context.Context, id string, req *AddSchemaVersionRequest) (*EventTypeResponse, error) {
	var out EventTypeResponse
	if err := r.c.Post(ctx, "/api/event-types/"+id+"/versions", req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Archive — DELETE /api/event-types/{id}. The platform soft-archives:
// the row is retained with status flipped to ARCHIVED. Named Archive
// (not Delete) to make that explicit.
func (r *EventTypesResource) Archive(ctx context.Context, id string) error {
	return r.c.Delete(ctx, "/api/event-types/"+id, nil)
}

// Sync — POST /api/applications/{appCode}/event-types/sync. Declarative
// reconciliation: the request's eventTypes list becomes the desired
// state. With removeUnlisted=true the server archives any existing
// event types not in the request.
func (r *EventTypesResource) Sync(ctx context.Context, appCode string, req *SyncEventTypesRequest, removeUnlisted bool) (*SyncResult, error) {
	q := ""
	if removeUnlisted {
		q = "?removeUnlisted=true"
	}
	var out SyncResult
	path := fmt.Sprintf("/api/applications/%s/event-types/sync%s", appCode, q)
	if err := r.c.Post(ctx, path, req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
