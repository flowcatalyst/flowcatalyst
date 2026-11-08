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
	Phone      *string `json:"phone,omitempty"`
	// AppCodes are the applications the worker may sign in to; granted
	// additively.
	AppCodes []string `json:"appCodes,omitempty"`
	// Pin is a supervisor-entered PIN for a NEW worker (stored with
	// pinChangeRequired); ignored for an existing worker. Omit it and the
	// worker cannot sign in until a PIN reset.
	Pin *string `json:"pin,omitempty"`
	// CanImpersonate (needs the impersonate permission) and IsTest apply to a
	// NEW worker only.
	CanImpersonate *bool `json:"canImpersonate,omitempty"`
	IsTest         *bool `json:"isTest,omitempty"`
}

// UpdateFieldWorkerRequest — PUT /api/field-workers/{id}.
type UpdateFieldWorkerRequest struct {
	Name  string  `json:"name"`
	Email *string `json:"email,omitempty"`
	// Phone nil leaves the number unchanged; "" clears it.
	Phone *string `json:"phone,omitempty"`
	// CanImpersonate (changing it needs the impersonate permission) and IsTest
	// replace the flag when non-nil.
	CanImpersonate *bool `json:"canImpersonate,omitempty"`
	IsTest         *bool `json:"isTest,omitempty"`
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
	// IsTest limits to test identities (true) or real workers (false).
	IsTest *bool
	// LinkedPrincipalID limits to the worker linked to this platform user.
	LinkedPrincipalID string
	// Page is 0-based; PageSize defaults to 100 (max 1000). nil takes the
	// platform default.
	Page     *uint32
	PageSize *uint32
}

// ─── Response DTOs ───────────────────────────────────────────────────

// LinkFieldWorkerRequest — PUT /api/field-workers/{id}/link.
type LinkFieldWorkerRequest struct {
	// PrincipalID is the platform user: a USER (not a service account), active
	// and able to access the worker's client.
	PrincipalID string `json:"principalId"`
}

// FieldLinkedPrincipal is the platform user a worker is linked to. Name and
// Email are empty when the user no longer exists.
type FieldLinkedPrincipal struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

// FieldWorkerBadgeRef is a worker's active badge.
type FieldWorkerBadgeRef struct {
	ID         string `json:"id"`
	Status     string `json:"status"`
	CreatedAt  string `json:"createdAt"`
	LastUsedAt string `json:"lastUsedAt,omitempty"`
}

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
	Phone      string `json:"phone,omitempty"`
	// Status is ACTIVE or DISABLED (suspended).
	Status string `json:"status"`
	// HasPin is false until a PIN is set (the worker cannot sign in).
	HasPin            bool `json:"hasPin"`
	PinChangeRequired bool `json:"pinChangeRequired"`
	// CanImpersonate lets the worker act as other workers (supervisor act-as);
	// IsTest marks a test identity; Badge is the ACTIVE badge, nil when none.
	CanImpersonate bool `json:"canImpersonate"`
	IsTest         bool `json:"isTest"`
	// LinkedPrincipal is the platform user the worker is linked to (audit
	// correlation only; it grants nothing), nil when none.
	LinkedPrincipal *FieldLinkedPrincipal `json:"linkedPrincipal"`
	Badge           *FieldWorkerBadgeRef  `json:"badge"`
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
	// ActorID is the supervisor or principal behind an impersonated sign-in.
	ActorID string `json:"actorId,omitempty"`
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
			Bool("isTest", filters.IsTest).
			String("linkedPrincipalId", filters.LinkedPrincipalID).
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

// Link — PUT /api/field-workers/{id}/link. Links the worker to a platform user
// for audit correlation (the token then carries linked_principal_id); it
// grants nothing. 422 INVALID_LINK when the user cannot be linked (not a
// user, inactive, or without access to the worker's client), 409 LINK_EXISTS
// when another worker of the client already holds the link. Returns the
// refreshed worker.
func (r *FieldWorkersResource) Link(ctx context.Context, id, principalID string) (*FieldWorkerResponse, error) {
	var out FieldWorkerResponse
	if err := r.c.Put(ctx, "/api/field-workers/"+url.PathEscape(id)+"/link", &LinkFieldWorkerRequest{PrincipalID: principalID}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Unlink — DELETE /api/field-workers/{id}/link. Idempotent. Returns the
// refreshed worker.
func (r *FieldWorkersResource) Unlink(ctx context.Context, id string) (*FieldWorkerResponse, error) {
	var out FieldWorkerResponse
	if err := r.c.Delete(ctx, "/api/field-workers/"+url.PathEscape(id)+"/link", &out); err != nil {
		return nil, err
	}
	return &out, nil
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

// ─── Badges and console sign-in codes ────────────────────────────────

// IssueFieldBadgeRequest — POST /api/field-workers/{id}/badges.
type IssueFieldBadgeRequest struct {
	// Label is a note for the printed badge (at most 255 characters).
	Label string `json:"label,omitempty"`
}

// BulkIssueFieldBadgesRequest — POST /api/field-workers/badges.
type BulkIssueFieldBadgesRequest struct {
	// WorkerIDs is 1 to 200 workers, all in clients the caller can access.
	WorkerIDs []string `json:"workerIds"`
}

// CreateFieldSignInCodeRequest — POST /api/field-workers/{id}/sign-in-codes.
type CreateFieldSignInCodeRequest struct {
	// AppCode is the application the worker is signed in to.
	AppCode string `json:"appCode"`
}

// FieldBadgeResponse is one badge. The token is never part of it.
type FieldBadgeResponse struct {
	ID         string `json:"id"`
	WorkerID   string `json:"workerId"`
	Label      string `json:"label,omitempty"`
	Status     string `json:"status"`
	CreatedAt  string `json:"createdAt"`
	CreatedBy  string `json:"createdBy,omitempty"`
	LastUsedAt string `json:"lastUsedAt,omitempty"`
	RevokedAt  string `json:"revokedAt,omitempty"`
}

// FieldBadgeIssuedResponse is a freshly issued badge. Token is the secret to
// print (as a QR code); it is shown once and cannot be retrieved again.
type FieldBadgeIssuedResponse struct {
	Badge FieldBadgeResponse `json:"badge"`
	Token string             `json:"token"`
}

// FieldBadgeListResponse — GET /api/field-workers/{id}/badges.
type FieldBadgeListResponse struct {
	Badges []FieldBadgeResponse `json:"badges"`
}

// BulkIssuedFieldBadge is one worker's new badge in a bulk issue.
type BulkIssuedFieldBadge struct {
	WorkerID   string             `json:"workerId"`
	WorkerCode string             `json:"workerCode"`
	Name       string             `json:"name"`
	Badge      FieldBadgeResponse `json:"badge"`
	Token      string             `json:"token"`
}

// BulkIssueFieldBadgesResponse — the result of a bulk issue.
type BulkIssueFieldBadgesResponse struct {
	Issued []BulkIssuedFieldBadge `json:"issued"`
}

// FieldSignInCodeResponse is a single-use code a console admin uses to sign in
// as the worker on a device.
type FieldSignInCodeResponse struct {
	Code      string `json:"code"`
	WorkerID  string `json:"workerId"`
	AppCode   string `json:"appCode"`
	ExpiresAt string `json:"expiresAt"`
}

// IssueBadge — POST /api/field-workers/{id}/badges. Replaces the worker's
// active badge; the response's Token is shown once.
func (r *FieldWorkersResource) IssueBadge(ctx context.Context, id string, req *IssueFieldBadgeRequest) (*FieldBadgeIssuedResponse, error) {
	if req == nil {
		req = &IssueFieldBadgeRequest{}
	}
	var out FieldBadgeIssuedResponse
	if err := r.c.Post(ctx, "/api/field-workers/"+url.PathEscape(id)+"/badges", req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ListBadges — GET /api/field-workers/{id}/badges.
func (r *FieldWorkersResource) ListBadges(ctx context.Context, id string) (*FieldBadgeListResponse, error) {
	var out FieldBadgeListResponse
	if err := r.c.Get(ctx, "/api/field-workers/"+url.PathEscape(id)+"/badges", &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// RevokeBadge — POST /api/field-workers/{id}/badges/{badgeId}/revoke.
func (r *FieldWorkersResource) RevokeBadge(ctx context.Context, id, badgeID string) error {
	return r.c.Post(ctx, "/api/field-workers/"+url.PathEscape(id)+"/badges/"+url.PathEscape(badgeID)+"/revoke", nil, nil)
}

// BulkIssueBadges — POST /api/field-workers/badges. Issues a badge for each
// of 1 to 200 workers.
func (r *FieldWorkersResource) BulkIssueBadges(ctx context.Context, workerIDs []string) (*BulkIssueFieldBadgesResponse, error) {
	var out BulkIssueFieldBadgesResponse
	if err := r.c.Post(ctx, "/api/field-workers/badges", &BulkIssueFieldBadgesRequest{WorkerIDs: workerIDs}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// CreateSignInCode — POST /api/field-workers/{id}/sign-in-codes. The code is
// redeemed at the platform's field sign-in to start an impersonated session.
func (r *FieldWorkersResource) CreateSignInCode(ctx context.Context, id, appCode string) (*FieldSignInCodeResponse, error) {
	var out FieldSignInCodeResponse
	if err := r.c.Post(ctx, "/api/field-workers/"+url.PathEscape(id)+"/sign-in-codes", &CreateFieldSignInCodeRequest{AppCode: appCode}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
