package client

import (
	"context"
	"net/url"
)

// ─── Request DTOs ────────────────────────────────────────────────────

// EnsureFieldWorkerRequest — POST /api/field-workers.
type EnsureFieldWorkerRequest struct {
	ClientID   string  `json:"clientId"`
	WorkerCode string  `json:"workerCode"`
	Name       string  `json:"name"`
	Email      *string `json:"email,omitempty"`
	// AppCodes are the applications the worker may sign in to; granted
	// additively.
	AppCodes []string `json:"appCodes,omitempty"`
	// Pin is a supervisor-entered PIN for a NEW worker (stored with
	// pinChangeRequired); ignored for an existing worker. Omit it and the
	// worker cannot sign in until a PIN reset.
	Pin *string `json:"pin,omitempty"`
}

// UpdateFieldWorkerRequest — PUT /api/field-workers/{id}.
type UpdateFieldWorkerRequest struct {
	Name  string  `json:"name"`
	Email *string `json:"email,omitempty"`
}

// ResetFieldWorkerPinRequest — POST /api/field-workers/{id}/pin-reset. Omit
// Pin for a random temporary PIN, returned once.
type ResetFieldWorkerPinRequest struct {
	Pin *string `json:"pin,omitempty"`
}

// GrantFieldWorkerAppRequest — POST /api/field-workers/{id}/apps.
type GrantFieldWorkerAppRequest struct {
	AppCode string `json:"appCode"`
}

// FieldWorkerFilters — query parameters for GET /api/field-workers.
type FieldWorkerFilters struct {
	// Q is a case-insensitive prefix matched against name and worker code.
	Q string
	// AppCode limits to workers granted this application.
	AppCode string
	// Status is ACTIVE or DISABLED.
	Status string
	// Page is 0-based; PageSize defaults to 100 (max 1000). nil takes the
	// platform default.
	Page     *uint32
	PageSize *uint32
}

// ─── Response DTOs ───────────────────────────────────────────────────

// FieldWorkerAppRef is one application a worker is granted.
type FieldWorkerAppRef struct {
	ID        string `json:"id"`
	Code      string `json:"code"`
	Name      string `json:"name"`
	Source    string `json:"source"`
	GrantedAt string `json:"grantedAt"`
}

// FieldWorkerResponse is a field worker. The PIN hash is never exposed.
type FieldWorkerResponse struct {
	ID         string `json:"id"`
	ClientID   string `json:"clientId"`
	WorkerCode string `json:"workerCode"`
	Name       string `json:"name"`
	Email      string `json:"email,omitempty"`
	// Status is ACTIVE or DISABLED (suspended).
	Status string `json:"status"`
	// HasPin is false until a PIN is set (the worker cannot sign in).
	HasPin            bool `json:"hasPin"`
	PinChangeRequired bool `json:"pinChangeRequired"`
	// Locked is true while a soft or hard lock is in force.
	Locked             bool                `json:"locked"`
	HardLocked         bool                `json:"hardLocked"`
	LockedUntil        string              `json:"lockedUntil,omitempty"`
	FailedSinceSuccess int                 `json:"failedSinceSuccess"`
	LastLoginAt        string              `json:"lastLoginAt,omitempty"`
	Apps               []FieldWorkerAppRef `json:"apps"`
	CreatedAt          string              `json:"createdAt"`
	UpdatedAt          string              `json:"updatedAt"`
	// Created is set only on the ensure response: false when the worker
	// already existed.
	Created *bool `json:"created,omitempty"`
}

// FieldWorkerListResponse — GET /api/field-workers.
type FieldWorkerListResponse struct {
	Workers []FieldWorkerResponse `json:"workers"`
	Total   int64                 `json:"total"`
	Page    int                   `json:"page"`
	Size    int                   `json:"size"`
}

// ResetFieldWorkerPinResponse is the outcome of a PIN reset.
type ResetFieldWorkerPinResponse struct {
	// TemporaryPin is present only when the platform generated the PIN. It is
	// shown once and cannot be retrieved again.
	TemporaryPin      *string `json:"temporaryPin,omitempty"`
	PinChangeRequired bool    `json:"pinChangeRequired"`
}

// FieldWorkerSignInResponse is one sign-in attempt.
type FieldWorkerSignInResponse struct {
	ID string `json:"id"`
	// Outcome is SUCCESS or FAILURE.
	Outcome       string `json:"outcome"`
	FailureReason string `json:"failureReason,omitempty"`
	AuthMethod    string `json:"authMethod"`
	DeviceID      string `json:"deviceId,omitempty"`
	AppCode       string `json:"appCode,omitempty"`
	IPAddress     string `json:"ipAddress,omitempty"`
	AttemptedAt   string `json:"attemptedAt"`
}

// FieldWorkerSignInListResponse — GET /api/field-workers/{id}/sign-ins.
type FieldWorkerSignInListResponse struct {
	SignIns []FieldWorkerSignInResponse `json:"signIns"`
	Total   int64                       `json:"total"`
	Page    int                         `json:"page"`
	Size    int                         `json:"size"`
}

// ─── Resource ────────────────────────────────────────────────────────

// FieldWorkersResource — /api/field-workers/*.
type FieldWorkersResource struct {
	c *FlowCatalystClient
}

// Ensure — POST /api/field-workers. Creates the worker for (clientId,
// workerCode) if absent and grants the listed applications; the response's
// Created says which happened.
func (r *FieldWorkersResource) Ensure(ctx context.Context, req *EnsureFieldWorkerRequest) (*FieldWorkerResponse, error) {
	var out FieldWorkerResponse
	if err := r.c.Post(ctx, "/api/field-workers", req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// List — GET /api/field-workers for one client, with optional filters.
func (r *FieldWorkersResource) List(ctx context.Context, clientID string, filters *FieldWorkerFilters) (*FieldWorkerListResponse, error) {
	q := NewQuery().String("clientId", clientID)
	if filters != nil {
		q = q.String("q", filters.Q).
			String("appCode", filters.AppCode).
			String("status", filters.Status).
			Uint32("page", filters.Page).
			Uint32("pageSize", filters.PageSize)
	}
	var out FieldWorkerListResponse
	if err := r.c.Get(ctx, "/api/field-workers"+q.Encode(), &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Get — GET /api/field-workers/{id}.
func (r *FieldWorkersResource) Get(ctx context.Context, id string) (*FieldWorkerResponse, error) {
	var out FieldWorkerResponse
	if err := r.c.Get(ctx, "/api/field-workers/"+url.PathEscape(id), &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Update — PUT /api/field-workers/{id}. Returns the refreshed worker.
func (r *FieldWorkersResource) Update(ctx context.Context, id string, req *UpdateFieldWorkerRequest) (*FieldWorkerResponse, error) {
	var out FieldWorkerResponse
	if err := r.c.Put(ctx, "/api/field-workers/"+url.PathEscape(id), req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Delete — DELETE /api/field-workers/{id}. Revokes the worker's sessions
// first. The { message } response is discarded.
func (r *FieldWorkersResource) Delete(ctx context.Context, id string) error {
	return r.c.Delete(ctx, "/api/field-workers/"+url.PathEscape(id), nil)
}

// ResetPin — POST /api/field-workers/{id}/pin-reset. A nil req (or nil Pin)
// makes the platform generate a temporary PIN, returned once in
// TemporaryPin.
func (r *FieldWorkersResource) ResetPin(ctx context.Context, id string, req *ResetFieldWorkerPinRequest) (*ResetFieldWorkerPinResponse, error) {
	if req == nil {
		req = &ResetFieldWorkerPinRequest{}
	}
	var out ResetFieldWorkerPinResponse
	if err := r.c.Post(ctx, "/api/field-workers/"+url.PathEscape(id)+"/pin-reset", req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Unlock — POST /api/field-workers/{id}/unlock. Clears soft and hard locks.
func (r *FieldWorkersResource) Unlock(ctx context.Context, id string) error {
	return r.c.Post(ctx, "/api/field-workers/"+url.PathEscape(id)+"/unlock", nil, nil)
}

// Suspend — POST /api/field-workers/{id}/suspend. Revokes the worker's
// sessions.
func (r *FieldWorkersResource) Suspend(ctx context.Context, id string) error {
	return r.c.Post(ctx, "/api/field-workers/"+url.PathEscape(id)+"/suspend", nil, nil)
}

// Activate — POST /api/field-workers/{id}/activate.
func (r *FieldWorkersResource) Activate(ctx context.Context, id string) error {
	return r.c.Post(ctx, "/api/field-workers/"+url.PathEscape(id)+"/activate", nil, nil)
}

// GrantApp — POST /api/field-workers/{id}/apps.
func (r *FieldWorkersResource) GrantApp(ctx context.Context, id, appCode string) error {
	return r.c.Post(ctx, "/api/field-workers/"+url.PathEscape(id)+"/apps",
		&GrantFieldWorkerAppRequest{AppCode: appCode}, nil)
}

// RevokeApp — DELETE /api/field-workers/{id}/apps/{appCode}. Also revokes
// that application's sessions.
func (r *FieldWorkersResource) RevokeApp(ctx context.Context, id, appCode string) error {
	return r.c.Delete(ctx, "/api/field-workers/"+url.PathEscape(id)+"/apps/"+url.PathEscape(appCode), nil)
}

// SignIns — GET /api/field-workers/{id}/sign-ins, newest first. page and
// pageSize are optional (nil takes the platform default).
func (r *FieldWorkersResource) SignIns(ctx context.Context, id string, page, pageSize *uint32) (*FieldWorkerSignInListResponse, error) {
	q := NewQuery().Uint32("page", page).Uint32("pageSize", pageSize).Encode()
	var out FieldWorkerSignInListResponse
	if err := r.c.Get(ctx, "/api/field-workers/"+url.PathEscape(id)+"/sign-ins"+q, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
