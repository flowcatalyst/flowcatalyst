// Package api serves the admin surface of the field-worker identity plane:
// /api/field-workers (with their badges and console sign-in codes),
// /api/field-devices, /api/field-policies and /api/field-tokens/introspect
// (docs/field-worker-identity-p1-spec.md section 3.3,
// docs/field-worker-sign-in-options-spec.md sections 3.4 and 4). The device-facing /field/* protocol is a separate package.
//
// Authorization: every endpoint is client-boxed (platform:iam:field-worker /
// field-device permissions plus access to the target client). Application-
// confined service accounts are limited to workers, devices and applications
// of their own applications by the use cases. A by-id request for a resource
// of a client the caller cannot reach is a 404, not a 403, so ids do not leak.
package api

import (
	"context"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/fieldidentity"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apicommon"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apiroute"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httpcompat"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/jsontime"
	"github.com/flowcatalyst/go-sdk/usecase"
	"github.com/flowcatalyst/go-sdk/usecaseop"
	"github.com/flowcatalyst/go-sdk/usecasepgx"
)

// State bundles the dependencies.
type State struct {
	Ops *fieldidentity.Ops
	UoW *usecasepgx.UnitOfWork
	// BaseURL is the platform's public base URL, embedded in enrolment QR
	// payloads (fcfield:1:<baseUrl>:<code>).
	BaseURL string
	// Verifier verifies field access tokens for introspection. TODO(slice-B):
	// wired when the device protocol's token minting lands; nil makes
	// introspection answer active:false for every token.
	Verifier fieldidentity.TokenVerifier
}

// resolveClient canonicalises a request's client reference (a clt_ id or the
// client's identifier) to the client id. Stored values and responses always
// carry the id.
func (s *State) resolveClient(ctx context.Context, ref string) (string, error) {
	if s.Ops.Clients == nil {
		return ref, nil
	}
	return s.Ops.Clients.ResolveRef(ctx, ref)
}

// clientGate authenticates the caller, canonicalises the request's client
// reference and runs the permission/reach check for it, in that order, so a
// request's answer never depends on whether a client exists until the caller
// has shown it may act on clients:
//   - no AuthContext: the unauthenticated error, whatever the reference;
//   - a caller that lacks the permission, or reach to the client: the same
//     refusal for an unknown reference as for a client it cannot reach (the
//     check runs against the raw reference when nothing resolves);
//   - a caller with permission and reach: 404 for an unknown client.
//
// emptyErr, when non-nil, answers a blank reference once the caller is
// authenticated.
func (s *State) clientGate(ctx context.Context, ref string, emptyErr error, can func(*auth.AuthContext, string) error) (string, error) {
	ac := auth.FromContext(ctx)
	if ac == nil {
		return "", usecase.Authorization("UNAUTHENTICATED", "authentication required")
	}
	ref = strings.TrimSpace(ref)
	if ref == "" && emptyErr != nil {
		return "", emptyErr
	}
	clientID, err := s.resolveClient(ctx, ref)
	if err != nil {
		if ue := usecase.AsError(err); ue != nil && ue.Kind == usecase.KindNotFound {
			if gerr := can(ac, ref); gerr != nil {
				return "", gerr
			}
		}
		return "", err
	}
	if err := can(ac, clientID); err != nil {
		return "", err
	}
	return clientID, nil
}

const (
	workersTag  = "field-workers"
	devicesTag  = "field-devices"
	policiesTag = "field-policies"
	tokensTag   = "field-tokens"
)

// Register mounts the admin endpoints.
func Register(api huma.API, s *State) {
	w := apiroute.New(api, workersTag)
	apiroute.Post(w, "ensureFieldWorker", "/api/field-workers", "Ensure a field worker exists for (client, workerCode) and grant applications", http.StatusCreated, s.ensureWorker)
	apiroute.Get(w, "listFieldWorkers", "/api/field-workers", "Search a client's field workers", s.listWorkers)
	apiroute.Get(w, "getFieldWorker", "/api/field-workers/{id}", "Get a field worker", s.getWorker)
	apiroute.Put(w, "updateFieldWorker", "/api/field-workers/{id}", "Update a field worker's name, email and phone", http.StatusOK, s.updateWorker)
	apiroute.Delete(w, "deleteFieldWorker", "/api/field-workers/{id}", "Delete a field worker (revokes their sessions first)", http.StatusOK, s.deleteWorker)
	apiroute.Post(w, "resetFieldWorkerPin", "/api/field-workers/{id}/pin-reset", "Reset a field worker's PIN (a temporary PIN is returned once when none is supplied)", http.StatusOK, s.resetPin)
	apiroute.Put(w, "linkFieldWorker", "/api/field-workers/{id}/link", "Link a field worker to a platform user (audit correlation only; grants nothing)", http.StatusOK, s.linkWorker)
	apiroute.Delete(w, "unlinkFieldWorker", "/api/field-workers/{id}/link", "Remove a field worker's platform user link", http.StatusOK, s.unlinkWorker)
	apiroute.Post(w, "unlockFieldWorker", "/api/field-workers/{id}/unlock", "Clear a field worker's soft and hard locks", http.StatusOK, s.unlockWorker)
	apiroute.Post(w, "suspendFieldWorker", "/api/field-workers/{id}/suspend", "Suspend a field worker and revoke their sessions", http.StatusOK, s.suspendWorker)
	apiroute.Post(w, "activateFieldWorker", "/api/field-workers/{id}/activate", "Reactivate a suspended field worker", http.StatusOK, s.activateWorker)
	apiroute.Post(w, "grantFieldWorkerApp", "/api/field-workers/{id}/apps", "Grant a field worker access to an application", http.StatusOK, s.grantApp)
	apiroute.Delete(w, "revokeFieldWorkerApp", "/api/field-workers/{id}/apps/{appCode}", "Revoke a field worker's access to an application (and that application's sessions)", http.StatusOK, s.revokeApp)
	apiroute.Get(w, "listFieldWorkerSignIns", "/api/field-workers/{id}/sign-ins", "A field worker's sign-in attempts, newest first", s.signIns)
	apiroute.Post(w, "issueFieldWorkerBadge", "/api/field-workers/{id}/badges", "Issue a printed QR badge for a field worker (the token is shown once; earlier active badges are revoked)", http.StatusCreated, s.issueBadge)
	apiroute.Get(w, "listFieldWorkerBadges", "/api/field-workers/{id}/badges", "A field worker's badges, newest first (never the token)", s.listBadges)
	apiroute.Post(w, "revokeFieldWorkerBadge", "/api/field-workers/{id}/badges/{badgeId}/revoke", "Revoke a field worker's badge", http.StatusOK, s.revokeBadge)
	apiroute.Post(w, "issueFieldWorkerBadges", "/api/field-workers/badges", "Issue badges for up to 200 field workers at once (for printing a sheet); all or nothing", http.StatusCreated, s.issueBadges)
	apiroute.Post(w, "createFieldWorkerSignInCode", "/api/field-workers/{id}/sign-in-codes", "Create a single-use code that signs the worker in to an application on any device (impersonation)", http.StatusCreated, s.createSignInCode)

	d := apiroute.New(api, devicesTag)
	apiroute.Post(d, "createFieldEnrolmentCode", "/api/field-devices/enrolment-codes", "Create a single-use device enrolment code", http.StatusCreated, s.createCode)
	apiroute.Get(d, "listFieldDevices", "/api/field-devices", "List a client's enrolled field devices", s.listDevices)
	apiroute.Get(d, "getFieldDevice", "/api/field-devices/{id}", "Get a field device with its active session count", s.getDevice)
	apiroute.Post(d, "suspendFieldDevice", "/api/field-devices/{id}/suspend", "Suspend a field device and revoke its sessions", http.StatusOK, s.suspendDevice)
	apiroute.Post(d, "activateFieldDevice", "/api/field-devices/{id}/activate", "Reactivate a suspended field device", http.StatusOK, s.activateDevice)
	apiroute.Post(d, "revokeFieldDevice", "/api/field-devices/{id}/revoke", "Permanently revoke a field device and its sessions", http.StatusOK, s.revokeDevice)

	p := apiroute.New(api, policiesTag)
	apiroute.Get(p, "getFieldPolicy", "/api/field-policies/{clientId}", "Get a client's effective field policy", s.getPolicy)
	apiroute.Put(p, "putFieldPolicy", "/api/field-policies/{clientId}", "Set a client's field policy", http.StatusOK, s.putPolicy)

	t := apiroute.New(api, tokensTag)
	apiroute.Post(t, "introspectFieldToken", "/api/field-tokens/introspect", "Online check that a field access token is still usable", http.StatusOK, s.introspect)
}

// ── shared plumbing ──────────────────────────────────────────────────────

// fail maps operation errors to the wire: an unconfigured PIN pepper is a 503,
// a link target that cannot be linked a 422, everything else is left to the platform error transformer.
func fail(err error) error {
	if ue := usecase.AsError(err); ue != nil {
		switch ue.Code {
		case fieldidentity.CodeUnconfigured:
			return httpcompat.NewStatusError(http.StatusServiceUnavailable, fieldidentity.CodeUnconfigured, ue.Message)
		case fieldidentity.CodeInvalidLink:
			return httpcompat.NewStatusError(http.StatusUnprocessableEntity, fieldidentity.CodeInvalidLink, ue.Message)
		}
	}
	return err
}

// isScopeForbidden reports the client-box failure (as opposed to a missing
// permission).
func isScopeForbidden(err error) bool {
	ue := usecase.AsError(err)
	return ue != nil && ue.Code == "SCOPE_FORBIDDEN"
}

func opt(t *time.Time) *httpcompat.Time {
	if t == nil {
		return nil
	}
	v := jsontime.New(*t)
	return &v
}

// appIndex indexes every application by id for grant display.
type appIndex map[string]application.Application

func (s *State) appIndex(ctx context.Context) (appIndex, error) {
	apps, err := s.Ops.Apps.FindWithFilters(ctx, nil, nil)
	if err != nil {
		return nil, usecase.Internal("REPO", "list_applications failed", err)
	}
	out := make(appIndex, len(apps))
	for _, a := range apps {
		out[string(a.ID)] = a
	}
	return out, nil
}

// callerAppIDs is the application set an application-confined caller is
// limited to, or nil for an unconfined caller.
func callerAppIDs(ac *auth.AuthContext) []string {
	if !ac.IsApplicationScoped() {
		return nil
	}
	if ac.Applications == nil {
		return []string{}
	}
	return ac.Applications
}

func pageOf(page, size int) (int, int, int, int) {
	page = max(page, 0)
	if size <= 0 {
		size = 100
	}
	size = min(size, apicommon.MaxPageSize)
	return page, size, page * size, size
}

// FieldAppRef names an application.
type FieldAppRef struct {
	ID   string `json:"id"`
	Code string `json:"code"`
	Name string `json:"name"`
}

func (idx appIndex) ref(id string) FieldAppRef {
	if a, ok := idx[id]; ok {
		return FieldAppRef{ID: id, Code: a.Code, Name: a.Name}
	}
	return FieldAppRef{ID: id, Code: id, Name: id}
}

// ── workers ──────────────────────────────────────────────────────────────

// FieldWorkerAppRef is one application a worker is granted.
type FieldWorkerAppRef struct {
	ID        string          `json:"id"`
	Code      string          `json:"code"`
	Name      string          `json:"name"`
	Source    string          `json:"source"`
	GrantedAt httpcompat.Time `json:"grantedAt"`
}

// FieldWorkerResponse is a field worker on the wire. The PIN hash is never
// exposed.
type FieldWorkerResponse struct {
	ID         string `json:"id"`
	ClientID   string `json:"clientId"`
	WorkerCode string `json:"workerCode"`
	Name       string `json:"name"`
	Email      string `json:"email,omitempty"`
	Phone      string `json:"phone,omitempty"`
	// Status is ACTIVE or DISABLED (suspended).
	Status string `json:"status" enum:"ACTIVE,DISABLED"`
	// HasPin is false until a PIN is set (the worker cannot sign in).
	HasPin            bool `json:"hasPin"`
	PinChangeRequired bool `json:"pinChangeRequired"`
	// CanImpersonate lets the worker act as other workers of the client
	// (supervisor act-as). IsTest marks a test identity.
	CanImpersonate bool `json:"canImpersonate"`
	IsTest         bool `json:"isTest"`
	// LinkedPrincipal is the platform user the worker is linked to, null when
	// none. A link whose user was deleted keeps its id with an empty name and
	// email (no foreign key).
	LinkedPrincipal *FieldLinkedPrincipalRef `json:"linkedPrincipal"`
	// Badge is the worker's ACTIVE badge, null when none.
	Badge *FieldWorkerBadgeRef `json:"badge"`
	// Locked is true while a soft or hard lock is in force.
	Locked             bool                `json:"locked"`
	HardLocked         bool                `json:"hardLocked"`
	LockedUntil        *httpcompat.Time    `json:"lockedUntil,omitempty"`
	FailedSinceSuccess int                 `json:"failedSinceSuccess"`
	LastLoginAt        *httpcompat.Time    `json:"lastLoginAt,omitempty"`
	Apps               []FieldWorkerAppRef `json:"apps"`
	CreatedAt          httpcompat.Time     `json:"createdAt"`
	UpdatedAt          httpcompat.Time     `json:"updatedAt"`
	// Created is set only by the ensure endpoint: false when the worker
	// already existed.
	Created *bool `json:"created,omitempty"`
}

// FieldLinkedPrincipalRef is the platform user a worker is linked to.
type FieldLinkedPrincipalRef struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

func linkedRef(i *fieldidentity.Identity, links map[string]fieldidentity.PrincipalRef) *FieldLinkedPrincipalRef {
	if i.LinkedPrincipalID == "" {
		return nil
	}
	p := links[i.LinkedPrincipalID]
	return &FieldLinkedPrincipalRef{ID: i.LinkedPrincipalID, Name: p.Name, Email: p.Email}
}

// linkRefs resolves the platform users the workers are linked to.
func (s *State) linkRefs(ctx context.Context, idents ...fieldidentity.Identity) (map[string]fieldidentity.PrincipalRef, error) {
	var ids []string
	for n := range idents {
		if idents[n].LinkedPrincipalID != "" {
			ids = append(ids, idents[n].LinkedPrincipalID)
		}
	}
	refs, err := s.Ops.R.Identities.PrincipalRefs(ctx, ids)
	if err != nil {
		return nil, usecase.Internal("REPO", "find_principals failed", err)
	}
	return refs, nil
}

// FieldWorkerBadgeRef is a worker's active badge, as the worker shows it.
type FieldWorkerBadgeRef struct {
	ID         string           `json:"id"`
	Status     string           `json:"status" enum:"ACTIVE,REVOKED"`
	CreatedAt  httpcompat.Time  `json:"createdAt"`
	LastUsedAt *httpcompat.Time `json:"lastUsedAt,omitempty"`
}

func badgeRef(b *fieldidentity.Badge) *FieldWorkerBadgeRef {
	if b == nil {
		return nil
	}
	return &FieldWorkerBadgeRef{ID: b.ID, Status: string(b.Status), CreatedAt: jsontime.New(b.CreatedAt), LastUsedAt: opt(b.LastUsedAt)}
}

func workerResponse(i *fieldidentity.Identity, idx appIndex, now time.Time, badge *fieldidentity.Badge,
	links map[string]fieldidentity.PrincipalRef,
) FieldWorkerResponse {
	r := FieldWorkerResponse{
		ID: i.ID, ClientID: i.ClientID, WorkerCode: i.WorkerCode, Name: i.Name, Email: i.Email, Phone: i.Phone,
		Status: string(i.Status), HasPin: i.HasPIN(), PinChangeRequired: i.PINChangeRequired,
		CanImpersonate: i.CanImpersonate, IsTest: i.IsTest, LinkedPrincipal: linkedRef(i, links), Badge: badgeRef(badge),
		Locked: i.IsLocked(now), HardLocked: i.HardLocked, LockedUntil: opt(i.LockedUntil),
		FailedSinceSuccess: i.FailedSinceSuccess, LastLoginAt: opt(i.LastLoginAt),
		Apps:      make([]FieldWorkerAppRef, 0, len(i.Apps)),
		CreatedAt: jsontime.New(i.CreatedAt), UpdatedAt: jsontime.New(i.UpdatedAt),
	}
	for _, g := range i.Apps {
		ref := idx.ref(g.AppID)
		r.Apps = append(r.Apps, FieldWorkerAppRef{
			ID: ref.ID, Code: ref.Code, Name: ref.Name,
			Source: g.Source, GrantedAt: jsontime.New(g.GrantedAt),
		})
	}
	return r
}

// loadWorker loads a worker for a by-id endpoint. Missing permission is a
// 403; an unknown id, another client's worker, or (for a confined caller) a
// worker outside its applications is a 404.
func (s *State) loadWorker(ctx context.Context, id string, write bool) (*fieldidentity.Identity, error) {
	gate := auth.CanReadFieldWorkers
	if write {
		gate = auth.CanManageFieldWorkers
	}
	return s.loadWorkerWith(ctx, id, func(ac *auth.AuthContext, _ *fieldidentity.Identity, clientID string) error {
		return gate(ac, clientID)
	})
}

// loadWorkerWith is [State.loadWorker] with the gate chosen by the caller: it
// gets the worker (nil when unknown) and its client id ("" when unknown), so
// the permission may depend on the worker (a test identity).
func (s *State) loadWorkerWith(ctx context.Context, id string,
	gate func(ac *auth.AuthContext, ident *fieldidentity.Identity, clientID string) error,
) (*fieldidentity.Identity, error) {
	ac := auth.FromContext(ctx)
	ident, err := s.Ops.R.Identities.FindByID(ctx, id)
	if err != nil {
		return nil, usecase.Internal("REPO", "find_field_worker failed", err)
	}
	clientID := ""
	if ident != nil {
		clientID = ident.ClientID
	}
	if gerr := gate(ac, ident, clientID); gerr != nil && !isScopeForbidden(gerr) {
		return nil, gerr
	} else if gerr != nil || ident == nil {
		return nil, httperror.NotFound("FieldWorker", id)
	}
	if ac.IsApplicationScoped() && !slices.ContainsFunc(ident.Apps,
		func(g fieldidentity.AppGrant) bool { return ac.CanAccessApplication(g.AppID) }) {
		return nil, httperror.NotFound("FieldWorker", id)
	}
	return ident, nil
}

// EnsureFieldWorkerRequest is the body of POST /api/field-workers.
type EnsureFieldWorkerRequest struct {
	ClientID   string   `json:"clientId" doc:"A clt_ id or the client's identifier"`
	WorkerCode string   `json:"workerCode"`
	Name       string   `json:"name"`
	Email      *string  `json:"email,omitempty"`
	Phone      *string  `json:"phone,omitempty" doc:"Optional contact number, stored as entered; new worker only"`
	AppCodes   []string `json:"appCodes,omitempty" doc:"Applications the worker may sign in to; granted additively"`
	// Pin is a supervisor-entered PIN for a NEW worker; it must pass the
	// client's PIN policy and is stored with pinChangeRequired. Ignored for an
	// existing worker. Omit it and the worker cannot sign in until a PIN reset.
	Pin *string `json:"pin,omitempty"`
	// CanImpersonate (needs platform:iam:field-worker:impersonate) and IsTest
	// apply to a NEW worker only.
	CanImpersonate *bool `json:"canImpersonate,omitempty" doc:"New worker only; needs the impersonate permission"`
	IsTest         *bool `json:"isTest,omitempty" doc:"New worker only: a test identity"`
}

type ensureOutput struct {
	Status int
	Body   FieldWorkerResponse
}

func (s *State) ensureWorker(ctx context.Context, in *apicommon.In[EnsureFieldWorkerRequest]) (*ensureOutput, error) {
	ac := auth.FromContext(ctx)
	clientID, err := s.clientGate(ctx, in.Body.ClientID,
		usecase.Validation("CLIENT_ID_REQUIRED", "clientId is required"), auth.CanManageFieldWorkers)
	if err != nil {
		return nil, err
	}
	cmd := fieldidentity.EnsureWorkerCommand{
		ClientID: clientID, WorkerCode: in.Body.WorkerCode, Name: in.Body.Name, AppCodes: in.Body.AppCodes,
	}
	if in.Body.Email != nil {
		cmd.Email = *in.Body.Email
	}
	if in.Body.Phone != nil {
		cmd.Phone = *in.Body.Phone
	}
	if in.Body.Pin != nil {
		cmd.PIN = *in.Body.Pin
	}
	if in.Body.IsTest != nil {
		cmd.IsTest = *in.Body.IsTest
	}
	if in.Body.CanImpersonate != nil && *in.Body.CanImpersonate {
		// Granting the power to act as others is the impersonate permission's
		// call, in addition to manage.
		if err := auth.CanImpersonateFieldWorkers(ac, clientID); err != nil {
			return nil, err
		}
		cmd.CanImpersonate = true
	}
	ev, err := usecaseop.Run(ctx, s.UoW, s.Ops.EnsureWorker(), cmd, auth.NewExecutionContext(ctx))
	if err != nil {
		return nil, fail(err)
	}
	ident, err := s.Ops.R.Identities.FindByID(ctx, ev.ID)
	if err != nil || ident == nil {
		return nil, usecase.Internal("REPO", "post-ensure worker lookup failed", err)
	}
	idx, err := s.appIndex(ctx)
	if err != nil {
		return nil, err
	}
	badge, err := s.Ops.R.Badges.ActiveByIdentity(ctx, ident.ID)
	if err != nil {
		return nil, usecase.Internal("REPO", "find_field_badge failed", err)
	}
	links, err := s.linkRefs(ctx, *ident)
	if err != nil {
		return nil, err
	}
	resp := workerResponse(ident, idx, time.Now().UTC(), badge, links)
	created := ev.Type == fieldidentity.WorkerCreatedType
	resp.Created = &created
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	return &ensureOutput{Status: status, Body: resp}, nil
}

// FieldWorkerListResponse is the GET /api/field-workers envelope.
type FieldWorkerListResponse struct {
	Workers []FieldWorkerResponse `json:"workers"`
	Total   int64                 `json:"total"`
	Page    int                   `json:"page"`
	Size    int                   `json:"size"`
}

type listWorkersInput struct {
	ClientID          string `query:"clientId" doc:"Tenant client whose workers to list: a clt_ id or the client's identifier"`
	Q                 string `query:"q" doc:"Case-insensitive prefix matched against name and worker code"`
	AppCode           string `query:"appCode" doc:"Only workers granted this application"`
	Status            string `query:"status" enum:"ACTIVE,DISABLED" doc:"Only workers in this state"`
	IsTest            string `query:"isTest" enum:"true,false" doc:"Only test identities (true) or only real workers (false)"`
	LinkedPrincipalID string `query:"linkedPrincipalId" doc:"Only the worker linked to this platform user"`
	Page              int    `query:"page" doc:"0-based page index (default 0)"`
	PageSize          int    `query:"pageSize" doc:"Page size (default 100, max 1000)"`
}

func (s *State) listWorkers(ctx context.Context, in *listWorkersInput) (*apicommon.Out[FieldWorkerListResponse], error) {
	ac := auth.FromContext(ctx)
	clientID, err := s.clientGate(ctx, in.ClientID,
		usecase.Validation("CLIENT_ID_REQUIRED", "clientId query param is required"), auth.CanReadFieldWorkers)
	if err != nil {
		return nil, err
	}
	f := fieldidentity.IdentityFilter{
		ClientID: clientID, Query: in.Q, Status: fieldidentity.Status(in.Status), AppIDs: callerAppIDs(ac),
		LinkedPrincipalID: strings.TrimSpace(in.LinkedPrincipalID),
	}
	if in.IsTest != "" {
		v := in.IsTest == "true"
		f.IsTest = &v
	}
	if code := strings.TrimSpace(in.AppCode); code != "" {
		app, err := s.Ops.Apps.FindByCode(ctx, code)
		if err != nil {
			return nil, usecase.Internal("REPO", "find_application failed", err)
		}
		if app == nil {
			return nil, usecase.Validation("UNKNOWN_APPLICATION", "unknown application: "+code)
		}
		if !ac.CanAccessApplication(string(app.ID)) {
			return nil, usecase.Authorization("APPLICATION_FORBIDDEN", "no access to application: "+app.Code)
		}
		f.AppID = string(app.ID)
	}
	page, size, off, lim := pageOf(in.Page, in.PageSize)
	f.Offset, f.Limit = off, lim
	rows, total, err := s.Ops.R.Identities.Search(ctx, f)
	if err != nil {
		return nil, usecase.Internal("REPO", "search_field_workers failed", err)
	}
	idx, err := s.appIndex(ctx)
	if err != nil {
		return nil, err
	}
	ids := make([]string, len(rows))
	for n := range rows {
		ids[n] = rows[n].ID
	}
	badges, err := s.Ops.R.Badges.ActiveByIdentities(ctx, ids)
	if err != nil {
		return nil, usecase.Internal("REPO", "find_field_badges failed", err)
	}
	links, err := s.linkRefs(ctx, rows...)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	out := make([]FieldWorkerResponse, 0, len(rows))
	for n := range rows {
		out = append(out, workerResponse(&rows[n], idx, now, badges[rows[n].ID], links))
	}
	return &apicommon.Out[FieldWorkerListResponse]{Body: FieldWorkerListResponse{Workers: out, Total: total, Page: page, Size: size}}, nil
}

func (s *State) getWorker(ctx context.Context, in *apicommon.IDInput) (*apicommon.Out[FieldWorkerResponse], error) {
	ident, err := s.loadWorker(ctx, in.ID, false)
	if err != nil {
		return nil, err
	}
	idx, err := s.appIndex(ctx)
	if err != nil {
		return nil, err
	}
	badge, err := s.Ops.R.Badges.ActiveByIdentity(ctx, ident.ID)
	if err != nil {
		return nil, usecase.Internal("REPO", "find_field_badge failed", err)
	}
	links, err := s.linkRefs(ctx, *ident)
	if err != nil {
		return nil, err
	}
	return &apicommon.Out[FieldWorkerResponse]{Body: workerResponse(ident, idx, time.Now().UTC(), badge, links)}, nil
}

// LinkFieldWorkerRequest is the body of PUT /api/field-workers/{id}/link.
type LinkFieldWorkerRequest struct {
	PrincipalID string `json:"principalId" doc:"The platform user (a USER, active, able to access the worker's client)"`
}

type linkWorkerInput struct {
	ID   string `path:"id"`
	Body LinkFieldWorkerRequest
}

// linkWorker links the worker to a platform user: 422 INVALID_LINK for a
// target that cannot be linked, 409 LINK_EXISTS when the user is already
// linked to another worker of the client.
func (s *State) linkWorker(ctx context.Context, in *linkWorkerInput) (*apicommon.Out[FieldWorkerResponse], error) {
	ident, err := s.loadWorker(ctx, in.ID, true)
	if err != nil {
		return nil, err
	}
	cmd := fieldidentity.LinkWorkerCommand{WorkerRef: refOf(ident), PrincipalID: in.Body.PrincipalID}
	if _, err := usecaseop.Run(ctx, s.UoW, s.Ops.LinkWorker(), cmd, auth.NewExecutionContext(ctx)); err != nil {
		return nil, fail(err)
	}
	return s.getWorker(ctx, &apicommon.IDInput{ID: ident.ID})
}

func (s *State) unlinkWorker(ctx context.Context, in *apicommon.IDInput) (*apicommon.Out[FieldWorkerResponse], error) {
	ident, err := s.loadWorker(ctx, in.ID, true)
	if err != nil {
		return nil, err
	}
	if _, err := usecaseop.Run(ctx, s.UoW, s.Ops.UnlinkWorker(), refOf(ident), auth.NewExecutionContext(ctx)); err != nil {
		return nil, fail(err)
	}
	return s.getWorker(ctx, &apicommon.IDInput{ID: ident.ID})
}

// UpdateFieldWorkerRequest is the body of PUT /api/field-workers/{id}.
type UpdateFieldWorkerRequest struct {
	Name  string  `json:"name"`
	Email *string `json:"email,omitempty"`
	Phone *string `json:"phone,omitempty" doc:"Omit to leave unchanged; an empty string clears it"`
	// CanImpersonate changes need platform:iam:field-worker:impersonate in
	// addition to manage; clearing it ends the sessions the worker holds open
	// as an actor. IsTest marks or unmarks a test identity.
	CanImpersonate *bool `json:"canImpersonate,omitempty" doc:"Changing it needs the impersonate permission; clearing it ends the sessions the worker holds as an actor"`
	IsTest         *bool `json:"isTest,omitempty"`
}

type updateWorkerInput struct {
	ID   string `path:"id"`
	Body UpdateFieldWorkerRequest
}

func (s *State) updateWorker(ctx context.Context, in *updateWorkerInput) (*apicommon.Out[FieldWorkerResponse], error) {
	ident, err := s.loadWorker(ctx, in.ID, true)
	if err != nil {
		return nil, err
	}
	cmd := fieldidentity.UpdateWorkerCommand{
		WorkerRef: refOf(ident),
		Name:      in.Body.Name, Email: in.Body.Email, Phone: in.Body.Phone, IsTest: in.Body.IsTest,
	}
	if v := in.Body.CanImpersonate; v != nil {
		if *v != ident.CanImpersonate {
			if err := auth.CanImpersonateFieldWorkers(auth.FromContext(ctx), ident.ClientID); err != nil {
				return nil, err
			}
		}
		cmd.CanImpersonate = v
	}
	if _, err := usecaseop.RunTx(ctx, s.UoW, s.Ops.UpdateWorker(), cmd, auth.NewExecutionContext(ctx)); err != nil {
		return nil, fail(err)
	}
	return s.getWorker(ctx, &apicommon.IDInput{ID: ident.ID})
}

func (s *State) deleteWorker(ctx context.Context, in *apicommon.IDInput) (*apicommon.Out[apicommon.StatusChangeResponse], error) {
	ident, err := s.loadWorker(ctx, in.ID, true)
	if err != nil {
		return nil, err
	}
	if _, err := usecaseop.RunTx(ctx, s.UoW, s.Ops.DeleteWorker(),
		fieldidentity.WorkerRef{ID: ident.ID, ClientID: ident.ClientID}, auth.NewExecutionContext(ctx)); err != nil {
		return nil, fail(err)
	}
	return statusMsg("Field worker deleted"), nil
}

// refOf addresses a loaded worker, boxed to its own client.
func refOf(i *fieldidentity.Identity) fieldidentity.WorkerRef {
	return fieldidentity.WorkerRef{ID: i.ID, ClientID: i.ClientID}
}

func statusMsg(m string) *apicommon.Out[apicommon.StatusChangeResponse] {
	return &apicommon.Out[apicommon.StatusChangeResponse]{Body: apicommon.StatusChangeResponse{Message: m}}
}

// ResetFieldWorkerPinRequest is the body of POST /api/field-workers/{id}/pin-reset.
type ResetFieldWorkerPinRequest struct {
	// Pin is the PIN the supervisor chose; it must pass the client's policy.
	// Omit it for a random temporary PIN, returned once.
	Pin *string `json:"pin,omitempty"`
}

// ResetFieldWorkerPinResponse is the outcome of a PIN reset.
type ResetFieldWorkerPinResponse struct {
	// TemporaryPin is present only when the platform generated the PIN. It is
	// shown once and cannot be retrieved again.
	TemporaryPin      *string `json:"temporaryPin,omitempty"`
	PinChangeRequired bool    `json:"pinChangeRequired"`
}

type resetPinInput struct {
	ID   string `path:"id"`
	Body ResetFieldWorkerPinRequest
}

func (s *State) resetPin(ctx context.Context, in *resetPinInput) (*apicommon.Out[ResetFieldWorkerPinResponse], error) {
	ident, err := s.loadWorker(ctx, in.ID, true)
	if err != nil {
		return nil, err
	}
	cmd := fieldidentity.ResetPinCommand{WorkerRef: refOf(ident)}
	if in.Body.Pin != nil {
		cmd.PIN = *in.Body.Pin
	}
	res, err := usecaseop.RunTx(ctx, s.UoW, s.Ops.ResetWorkerPin(), cmd, auth.NewExecutionContext(ctx))
	if err != nil {
		return nil, fail(err)
	}
	out := ResetFieldWorkerPinResponse{PinChangeRequired: res.PinChangeRequired}
	if res.TemporaryPIN != "" {
		out.TemporaryPin = &res.TemporaryPIN
	}
	return &apicommon.Out[ResetFieldWorkerPinResponse]{Body: out}, nil
}

func (s *State) unlockWorker(ctx context.Context, in *apicommon.IDInput) (*apicommon.Out[apicommon.StatusChangeResponse], error) {
	ident, err := s.loadWorker(ctx, in.ID, true)
	if err != nil {
		return nil, err
	}
	if _, err := usecaseop.Run(ctx, s.UoW, s.Ops.UnlockWorker(),
		fieldidentity.WorkerRef{ID: ident.ID, ClientID: ident.ClientID}, auth.NewExecutionContext(ctx)); err != nil {
		return nil, fail(err)
	}
	return statusMsg("Field worker unlocked"), nil
}

func (s *State) suspendWorker(ctx context.Context, in *apicommon.IDInput) (*apicommon.Out[apicommon.StatusChangeResponse], error) {
	ident, err := s.loadWorker(ctx, in.ID, true)
	if err != nil {
		return nil, err
	}
	if _, err := usecaseop.RunTx(ctx, s.UoW, s.Ops.SuspendWorker(),
		fieldidentity.WorkerRef{ID: ident.ID, ClientID: ident.ClientID}, auth.NewExecutionContext(ctx)); err != nil {
		return nil, fail(err)
	}
	return statusMsg("Field worker suspended"), nil
}

func (s *State) activateWorker(ctx context.Context, in *apicommon.IDInput) (*apicommon.Out[apicommon.StatusChangeResponse], error) {
	ident, err := s.loadWorker(ctx, in.ID, true)
	if err != nil {
		return nil, err
	}
	if _, err := usecaseop.Run(ctx, s.UoW, s.Ops.ActivateWorker(),
		fieldidentity.WorkerRef{ID: ident.ID, ClientID: ident.ClientID}, auth.NewExecutionContext(ctx)); err != nil {
		return nil, fail(err)
	}
	return statusMsg("Field worker activated"), nil
}

// GrantFieldWorkerAppRequest names the application to grant.
type GrantFieldWorkerAppRequest struct {
	AppCode string `json:"appCode"`
}

type grantAppInput struct {
	ID   string `path:"id"`
	Body GrantFieldWorkerAppRequest
}

// grantApp does not use loadWorker's application confinement: a confined
// caller may grant its own application to a worker it cannot yet see.
func (s *State) grantApp(ctx context.Context, in *grantAppInput) (*apicommon.Out[apicommon.StatusChangeResponse], error) {
	ac := auth.FromContext(ctx)
	ident, err := s.Ops.R.Identities.FindByID(ctx, in.ID)
	if err != nil {
		return nil, usecase.Internal("REPO", "find_field_worker failed", err)
	}
	clientID := ""
	if ident != nil {
		clientID = ident.ClientID
	}
	if gerr := auth.CanManageFieldWorkers(ac, clientID); gerr != nil && !isScopeForbidden(gerr) {
		return nil, gerr
	} else if gerr != nil || ident == nil {
		return nil, httperror.NotFound("FieldWorker", in.ID)
	}
	if _, err := usecaseop.Run(ctx, s.UoW, s.Ops.GrantWorkerApp(),
		fieldidentity.WorkerAppCommand{WorkerRef: refOf(ident), AppCode: in.Body.AppCode},
		auth.NewExecutionContext(ctx)); err != nil {
		return nil, fail(err)
	}
	return statusMsg("Application access granted"), nil
}

type revokeAppInput struct {
	ID      string `path:"id"`
	AppCode string `path:"appCode"`
}

func (s *State) revokeApp(ctx context.Context, in *revokeAppInput) (*apicommon.Out[apicommon.StatusChangeResponse], error) {
	ident, err := s.loadWorker(ctx, in.ID, true)
	if err != nil {
		return nil, err
	}
	if _, err := usecaseop.RunTx(ctx, s.UoW, s.Ops.RevokeWorkerApp(),
		fieldidentity.WorkerAppCommand{WorkerRef: refOf(ident), AppCode: in.AppCode},
		auth.NewExecutionContext(ctx)); err != nil {
		return nil, fail(err)
	}
	return statusMsg("Application access revoked"), nil
}

// FieldWorkerSignInResponse is one sign-in attempt.
type FieldWorkerSignInResponse struct {
	ID            string `json:"id"`
	Outcome       string `json:"outcome" enum:"SUCCESS,FAILURE"`
	FailureReason string `json:"failureReason,omitempty"`
	AuthMethod    string `json:"authMethod"`
	DeviceID      string `json:"deviceId,omitempty"`
	AppCode       string `json:"appCode,omitempty"`
	IPAddress     string `json:"ipAddress,omitempty"`
	// ActorID is the supervisor or principal behind an impersonated sign-in
	// (authMethod ACT_AS or SIGN_IN_CODE).
	ActorID     string          `json:"actorId,omitempty"`
	AttemptedAt httpcompat.Time `json:"attemptedAt"`
}

// FieldWorkerSignInListResponse is the GET /api/field-workers/{id}/sign-ins envelope.
type FieldWorkerSignInListResponse struct {
	SignIns []FieldWorkerSignInResponse `json:"signIns"`
	Total   int64                       `json:"total"`
	Page    int                         `json:"page"`
	Size    int                         `json:"size"`
}

type signInsInput struct {
	ID       string `path:"id"`
	Page     int    `query:"page" doc:"0-based page index (default 0)"`
	PageSize int    `query:"pageSize" doc:"Page size (default 100, max 1000)"`
}

func (s *State) signIns(ctx context.Context, in *signInsInput) (*apicommon.Out[FieldWorkerSignInListResponse], error) {
	ident, err := s.loadWorker(ctx, in.ID, false)
	if err != nil {
		return nil, err
	}
	page, size, off, lim := pageOf(in.Page, in.PageSize)
	rows, total, err := s.Ops.R.Sessions.SignIns(ctx, ident.ID, off, lim)
	if err != nil {
		return nil, usecase.Internal("REPO", "list_field_sign_ins failed", err)
	}
	idx, err := s.appIndex(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]FieldWorkerSignInResponse, 0, len(rows))
	for _, a := range rows {
		r := FieldWorkerSignInResponse{
			ID: a.ID, Outcome: a.Outcome, FailureReason: a.FailureReason, AuthMethod: a.AuthMethod,
			DeviceID: a.DeviceID, IPAddress: a.IPAddress, AttemptedAt: jsontime.New(a.AttemptedAt), ActorID: a.ActorID,
		}
		if a.ApplicationID != "" {
			r.AppCode = idx.ref(a.ApplicationID).Code
		}
		out = append(out, r)
	}
	return &apicommon.Out[FieldWorkerSignInListResponse]{Body: FieldWorkerSignInListResponse{SignIns: out, Total: total, Page: page, Size: size}}, nil
}

// ── devices ──────────────────────────────────────────────────────────────

// CreateEnrolmentCodeRequest is the body of POST /api/field-devices/enrolment-codes.
type CreateEnrolmentCodeRequest struct {
	ClientID         string   `json:"clientId" doc:"A clt_ id or the client's identifier"`
	Mode             string   `json:"mode" enum:"PERSONAL,SHARED"`
	AppCodes         []string `json:"appCodes"`
	AssignedWorkerID *string  `json:"assignedWorkerId,omitempty" doc:"PERSONAL devices only"`
	Label            *string  `json:"label,omitempty"`
}

// FieldEnrolmentCodeResponse carries the code, shown once.
type FieldEnrolmentCodeResponse struct {
	// Code is the single-use enrolment code (10 characters). It is shown once.
	Code string `json:"code"`
	// QrPayload is what the QR code encodes: fcfield:1:<platformBaseUrl>:<code>.
	QrPayload string          `json:"qrPayload"`
	ExpiresAt httpcompat.Time `json:"expiresAt"`
}

func (s *State) createCode(ctx context.Context, in *apicommon.In[CreateEnrolmentCodeRequest]) (*apicommon.Out[FieldEnrolmentCodeResponse], error) {
	clientID, err := s.clientGate(ctx, in.Body.ClientID,
		usecase.Validation("CLIENT_ID_REQUIRED", "clientId is required"), auth.CanManageFieldDevices)
	if err != nil {
		return nil, err
	}
	code, err := fieldidentity.NewEnrolmentCodeValue()
	if err != nil {
		return nil, usecase.Internal("CODE_GENERATE", "could not generate an enrolment code", err)
	}
	cmd := fieldidentity.CreateEnrolmentCodeCommand{
		ClientID: clientID, Mode: in.Body.Mode, AppCodes: in.Body.AppCodes, Code: code,
	}
	if in.Body.AssignedWorkerID != nil {
		cmd.AssignedWorkerID = *in.Body.AssignedWorkerID
	}
	if in.Body.Label != nil {
		cmd.Label = *in.Body.Label
	}
	res, err := usecaseop.RunTx(ctx, s.UoW, s.Ops.CreateEnrolmentCode(), cmd, auth.NewExecutionContext(ctx))
	if err != nil {
		return nil, fail(err)
	}
	return &apicommon.Out[FieldEnrolmentCodeResponse]{Body: FieldEnrolmentCodeResponse{
		Code:      code,
		QrPayload: "fcfield:1:" + strings.TrimRight(s.BaseURL, "/") + ":" + code,
		ExpiresAt: jsontime.New(res.ExpiresAt),
	}}, nil
}

// FieldDeviceResponse is an enrolled device on the wire.
type FieldDeviceResponse struct {
	ID               string `json:"id"`
	ClientID         string `json:"clientId"`
	Mode             string `json:"mode" enum:"PERSONAL,SHARED"`
	Label            string `json:"label,omitempty"`
	Platform         string `json:"platform,omitempty"`
	Model            string `json:"model,omitempty"`
	Jkt              string `json:"jkt" doc:"RFC 7638 thumbprint of the device key"`
	AttestationLevel string `json:"attestationLevel" enum:"STRONGBOX,TEE,SOFTWARE,NONE"`
	// AssignedWorkerID is the worker a PERSONAL device is bound to, if any.
	AssignedWorkerID *string          `json:"assignedWorkerId,omitempty"`
	Status           string           `json:"status" enum:"ACTIVE,SUSPENDED,REVOKED"`
	Apps             []FieldAppRef    `json:"apps"`
	ActiveSessions   int              `json:"activeSessions"`
	EnrolledAt       httpcompat.Time  `json:"enrolledAt"`
	LastSeenAt       *httpcompat.Time `json:"lastSeenAt,omitempty"`
}

func deviceResponse(d *fieldidentity.Device, idx appIndex, sessions map[string]int) FieldDeviceResponse {
	r := FieldDeviceResponse{
		ID: d.ID, ClientID: d.ClientID, Mode: string(d.Mode), Label: d.Label, Platform: d.Platform, Model: d.Model,
		Jkt: d.JKT, AttestationLevel: d.AttestationLevel, AssignedWorkerID: d.AssignedIdentityID,
		Status: string(d.Status), Apps: make([]FieldAppRef, 0, len(d.AppIDs)), ActiveSessions: sessions[d.ID],
		EnrolledAt: jsontime.New(d.EnrolledAt), LastSeenAt: opt(d.LastSeenAt),
	}
	for _, id := range d.AppIDs {
		r.Apps = append(r.Apps, idx.ref(id))
	}
	return r
}

func (s *State) loadDevice(ctx context.Context, id string, write bool) (*fieldidentity.Device, error) {
	ac := auth.FromContext(ctx)
	gate := auth.CanReadFieldDevices
	if write {
		gate = auth.CanManageFieldDevices
	}
	d, err := s.Ops.R.Devices.FindByID(ctx, id)
	if err != nil {
		return nil, usecase.Internal("REPO", "find_field_device failed", err)
	}
	clientID := ""
	if d != nil {
		clientID = d.ClientID
	}
	if gerr := gate(ac, clientID); gerr != nil && !isScopeForbidden(gerr) {
		return nil, gerr
	} else if gerr != nil || d == nil {
		return nil, httperror.NotFound("FieldDevice", id)
	}
	if ac.IsApplicationScoped() && !slices.ContainsFunc(d.AppIDs, ac.CanAccessApplication) {
		return nil, httperror.NotFound("FieldDevice", id)
	}
	return d, nil
}

// FieldDeviceListResponse is the GET /api/field-devices envelope.
type FieldDeviceListResponse struct {
	Devices []FieldDeviceResponse `json:"devices"`
	Total   int64                 `json:"total"`
	Page    int                   `json:"page"`
	Size    int                   `json:"size"`
}

type listDevicesInput struct {
	ClientID string `query:"clientId" doc:"Tenant client whose devices to list: a clt_ id or the client's identifier"`
	Status   string `query:"status" enum:"ACTIVE,SUSPENDED,REVOKED"`
	Mode     string `query:"mode" enum:"PERSONAL,SHARED"`
	Page     int    `query:"page" doc:"0-based page index (default 0)"`
	PageSize int    `query:"pageSize" doc:"Page size (default 100, max 1000)"`
}

func (s *State) listDevices(ctx context.Context, in *listDevicesInput) (*apicommon.Out[FieldDeviceListResponse], error) {
	ac := auth.FromContext(ctx)
	clientID, err := s.clientGate(ctx, in.ClientID,
		usecase.Validation("CLIENT_ID_REQUIRED", "clientId query param is required"), auth.CanReadFieldDevices)
	if err != nil {
		return nil, err
	}
	page, size, off, lim := pageOf(in.Page, in.PageSize)
	rows, total, err := s.Ops.R.Devices.Search(ctx, fieldidentity.DeviceFilter{
		ClientID: clientID, Status: fieldidentity.DeviceStatus(in.Status), Mode: fieldidentity.Mode(in.Mode),
		AppIDs: callerAppIDs(ac), Offset: off, Limit: lim,
	})
	if err != nil {
		return nil, usecase.Internal("REPO", "search_field_devices failed", err)
	}
	ids := make([]string, len(rows))
	for n := range rows {
		ids[n] = rows[n].ID
	}
	sessions, err := s.Ops.R.Sessions.ActiveCountByDevice(ctx, ids)
	if err != nil {
		return nil, usecase.Internal("REPO", "count_field_sessions failed", err)
	}
	idx, err := s.appIndex(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]FieldDeviceResponse, 0, len(rows))
	for n := range rows {
		out = append(out, deviceResponse(&rows[n], idx, sessions))
	}
	return &apicommon.Out[FieldDeviceListResponse]{Body: FieldDeviceListResponse{Devices: out, Total: total, Page: page, Size: size}}, nil
}

func (s *State) getDevice(ctx context.Context, in *apicommon.IDInput) (*apicommon.Out[FieldDeviceResponse], error) {
	d, err := s.loadDevice(ctx, in.ID, false)
	if err != nil {
		return nil, err
	}
	sessions, err := s.Ops.R.Sessions.ActiveCountByDevice(ctx, []string{d.ID})
	if err != nil {
		return nil, usecase.Internal("REPO", "count_field_sessions failed", err)
	}
	idx, err := s.appIndex(ctx)
	if err != nil {
		return nil, err
	}
	return &apicommon.Out[FieldDeviceResponse]{Body: deviceResponse(d, idx, sessions)}, nil
}

func (s *State) suspendDevice(ctx context.Context, in *apicommon.IDInput) (*apicommon.Out[apicommon.StatusChangeResponse], error) {
	d, err := s.loadDevice(ctx, in.ID, true)
	if err != nil {
		return nil, err
	}
	if _, err := usecaseop.RunTx(ctx, s.UoW, s.Ops.SuspendDevice(),
		fieldidentity.DeviceRef{ID: d.ID, ClientID: d.ClientID}, auth.NewExecutionContext(ctx)); err != nil {
		return nil, fail(err)
	}
	return statusMsg("Field device suspended"), nil
}

func (s *State) activateDevice(ctx context.Context, in *apicommon.IDInput) (*apicommon.Out[apicommon.StatusChangeResponse], error) {
	d, err := s.loadDevice(ctx, in.ID, true)
	if err != nil {
		return nil, err
	}
	if _, err := usecaseop.Run(ctx, s.UoW, s.Ops.ActivateDevice(),
		fieldidentity.DeviceRef{ID: d.ID, ClientID: d.ClientID}, auth.NewExecutionContext(ctx)); err != nil {
		return nil, fail(err)
	}
	return statusMsg("Field device activated"), nil
}

func (s *State) revokeDevice(ctx context.Context, in *apicommon.IDInput) (*apicommon.Out[apicommon.StatusChangeResponse], error) {
	d, err := s.loadDevice(ctx, in.ID, true)
	if err != nil {
		return nil, err
	}
	if _, err := usecaseop.RunTx(ctx, s.UoW, s.Ops.RevokeDevice(),
		fieldidentity.DeviceRef{ID: d.ID, ClientID: d.ClientID}, auth.NewExecutionContext(ctx)); err != nil {
		return nil, fail(err)
	}
	return statusMsg("Field device revoked"), nil
}

// ── policy ───────────────────────────────────────────────────────────────

// FieldPolicyResponse is a client's effective field policy.
type FieldPolicyResponse struct {
	ClientID           string `json:"clientId"`
	PinMinLength       int    `json:"pinMinLength"`
	SoftLockThreshold  int    `json:"softLockThreshold"`
	SoftLockSecs       int    `json:"softLockSecs"`
	HardLockThreshold  int    `json:"hardLockThreshold"`
	SharedIdleSecs     int    `json:"sharedIdleSecs"`
	PersonalIdleSecs   int    `json:"personalIdleSecs"`
	ShiftMaxSecs       int    `json:"shiftMaxSecs"`
	RequireAttestation bool   `json:"requireAttestation"`
	PinReauthDays      int    `json:"pinReauthDays"`
	// RequireEnrolledDevice refuses sign-in from a key that is not an enrolled
	// device. UnenrolledIdleSecs is the idle limit of an unenrolled session.
	// BadgeOnlyLogin lets a badge sign in without a PIN on an enrolled device.
	// ImpersonationMaxSecs caps an impersonated session.
	RequireEnrolledDevice bool `json:"requireEnrolledDevice"`
	UnenrolledIdleSecs    int  `json:"unenrolledIdleSecs"`
	BadgeOnlyLogin        bool `json:"badgeOnlyLogin"`
	ImpersonationMaxSecs  int  `json:"impersonationMaxSecs"`
	// IsDefault is true when the client has no policy row and these are the
	// platform defaults.
	IsDefault bool             `json:"isDefault"`
	UpdatedAt *httpcompat.Time `json:"updatedAt,omitempty"`
}

func policyResponse(p fieldidentity.Policy) FieldPolicyResponse {
	r := FieldPolicyResponse{
		ClientID: p.ClientID, PinMinLength: p.PINMinLength, SoftLockThreshold: p.SoftLockThreshold,
		SoftLockSecs: p.SoftLockSecs, HardLockThreshold: p.HardLockThreshold, SharedIdleSecs: p.SharedIdleSecs,
		PersonalIdleSecs: p.PersonalIdleSecs, ShiftMaxSecs: p.ShiftMaxSecs, RequireAttestation: p.RequireAttestation,
		PinReauthDays: p.PINReauthDays, IsDefault: p.IsDefault,
		RequireEnrolledDevice: p.RequireEnrolledDevice, UnenrolledIdleSecs: p.UnenrolledIdleSecs,
		BadgeOnlyLogin: p.BadgeOnlyLogin, ImpersonationMaxSecs: p.ImpersonationMaxSecs,
	}
	if !p.IsDefault {
		r.UpdatedAt = opt(&p.UpdatedAt)
	}
	return r
}

type policyInput struct {
	ClientID string `path:"clientId" doc:"A clt_ id or the client's identifier"`
}

func (s *State) getPolicy(ctx context.Context, in *policyInput) (*apicommon.Out[FieldPolicyResponse], error) {
	clientID, err := s.clientGate(ctx, in.ClientID, nil, auth.CanReadFieldDevices)
	if err != nil {
		return nil, err
	}
	p, err := s.Ops.R.Policies.Effective(ctx, clientID)
	if err != nil {
		return nil, usecase.Internal("REPO", "find_field_policy failed", err)
	}
	return &apicommon.Out[FieldPolicyResponse]{Body: policyResponse(p)}, nil
}

// PutFieldPolicyRequest is the body of PUT /api/field-policies/{clientId}.
// Omitted numeric fields take the defaults; a pinMinLength below 6 makes the
// hardLockThreshold default 5 (and it may not exceed 5).
type PutFieldPolicyRequest struct {
	PinMinLength       *int `json:"pinMinLength,omitempty" doc:"4 to 12 (default 6)"`
	SoftLockThreshold  *int `json:"softLockThreshold,omitempty" doc:"1 to 20 (default 5)"`
	SoftLockSecs       *int `json:"softLockSecs,omitempty" doc:"30 to 86400 (default 900)"`
	HardLockThreshold  *int `json:"hardLockThreshold,omitempty" doc:"softLockThreshold to 50 (default 10)"`
	SharedIdleSecs     *int `json:"sharedIdleSecs,omitempty" doc:"60 to 86400 (default 900)"`
	PersonalIdleSecs   *int `json:"personalIdleSecs,omitempty" doc:"60 to 86400 (default 7200)"`
	ShiftMaxSecs       *int `json:"shiftMaxSecs,omitempty" doc:"3600 to 86400 (default 50400)"`
	RequireAttestation bool `json:"requireAttestation"`
	PinReauthDays      *int `json:"pinReauthDays,omitempty" doc:"1 to 90 (default 7)"`
	// A PUT replaces the policy: the two flags are false when omitted.
	RequireEnrolledDevice bool `json:"requireEnrolledDevice,omitempty" doc:"Refuse sign-in from a key that is not an enrolled device (default false)"`
	UnenrolledIdleSecs    *int `json:"unenrolledIdleSecs,omitempty" doc:"60 to 86400 (default 900)"`
	BadgeOnlyLogin        bool `json:"badgeOnlyLogin,omitempty" doc:"Let a badge sign in without a PIN on an enrolled device (default false)"`
	ImpersonationMaxSecs  *int `json:"impersonationMaxSecs,omitempty" doc:"300 to 43200 (default 14400)"`
}

type putPolicyInput struct {
	ClientID string `path:"clientId" doc:"A clt_ id or the client's identifier"`
	Body     PutFieldPolicyRequest
}

func (s *State) putPolicy(ctx context.Context, in *putPolicyInput) (*apicommon.Out[FieldPolicyResponse], error) {
	clientID, err := s.clientGate(ctx, in.ClientID, nil, auth.CanManageFieldDevices)
	if err != nil {
		return nil, err
	}
	b := in.Body
	cmd := fieldidentity.PutPolicyCommand{
		ClientID: clientID, PINMinLength: b.PinMinLength, SoftLockThreshold: b.SoftLockThreshold,
		SoftLockSecs: b.SoftLockSecs, HardLockThreshold: b.HardLockThreshold, SharedIdleSecs: b.SharedIdleSecs,
		PersonalIdleSecs: b.PersonalIdleSecs, ShiftMaxSecs: b.ShiftMaxSecs, RequireAttestation: b.RequireAttestation,
		PINReauthDays: b.PinReauthDays, RequireEnrolledDevice: b.RequireEnrolledDevice,
		UnenrolledIdleSecs: b.UnenrolledIdleSecs, BadgeOnlyLogin: b.BadgeOnlyLogin, ImpersonationMaxSecs: b.ImpersonationMaxSecs,
	}
	if _, err := usecaseop.Run(ctx, s.UoW, s.Ops.PutPolicy(), cmd, auth.NewExecutionContext(ctx)); err != nil {
		return nil, fail(err)
	}
	return s.getPolicy(ctx, &policyInput{ClientID: clientID})
}

// ── introspection ────────────────────────────────────────────────────────

// FieldTokenIntrospectRequest is the body of POST /api/field-tokens/introspect.
type FieldTokenIntrospectRequest struct {
	Token string `json:"token"`
}

// FieldTokenIntrospectResponse is the introspection result. A token that fails any
// check is just {"active": false}: introspection never errors on a bad token.
type FieldTokenIntrospectResponse struct {
	Active     bool             `json:"active"`
	Sub        string           `json:"sub,omitempty"`
	ClientID   string           `json:"clientId,omitempty"`
	WorkerCode string           `json:"workerCode,omitempty"`
	DeviceID   string           `json:"deviceId,omitempty"`
	AppCode    string           `json:"appCode,omitempty"`
	Amr        []string         `json:"amr,omitempty"`
	AuthTime   *httpcompat.Time `json:"authTime,omitempty"`
	Exp        *httpcompat.Time `json:"exp,omitempty"`
	// Act is present when someone other than the worker is signed in as them.
	Act *FieldTokenActor `json:"act,omitempty"`
	// Test is true for a test identity.
	Test bool `json:"test,omitempty"`
	// LinkedPrincipalID is the platform user the worker is linked to, when
	// linked. Audit correlation only: it grants nothing.
	LinkedPrincipalID string `json:"linkedPrincipalId,omitempty"`
}

// FieldTokenActor is the act claim: who is acting as the worker. Typ is
// "field_worker" (a supervisor) or "principal" (a platform user).
type FieldTokenActor struct {
	Sub string `json:"sub"`
	Typ string `json:"typ"`
}

func (s *State) introspect(ctx context.Context, in *apicommon.In[FieldTokenIntrospectRequest]) (*apicommon.Out[FieldTokenIntrospectResponse], error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanIntrospectFieldTokens(ac); err != nil {
		return nil, err
	}
	inactive := &apicommon.Out[FieldTokenIntrospectResponse]{Body: FieldTokenIntrospectResponse{Active: false}}
	// TODO(slice-B): the verifier (signature, typ=fc-field+jwt, exp) arrives
	// with the device protocol's token minting. Until it is wired every token
	// is reported inactive.
	if s.Verifier == nil || strings.TrimSpace(in.Body.Token) == "" {
		return inactive, nil
	}
	claims, err := s.Verifier.VerifyFieldAccessToken(ctx, in.Body.Token)
	if err != nil || claims == nil {
		return inactive, nil
	}
	// Client box and application confinement apply to the token's subject: a
	// caller learns nothing about tokens it could not manage.
	if !ac.CanAccessClient(claims.ClientID) {
		return inactive, nil
	}
	if ac.IsApplicationScoped() {
		app, err := s.Ops.Apps.FindByCode(ctx, claims.AppCode)
		if err != nil || app == nil || !ac.CanAccessApplication(string(app.ID)) {
			return inactive, nil
		}
	}
	active, err := s.Ops.TokenActive(ctx, claims, time.Now().UTC())
	if err != nil {
		return nil, usecase.Internal("REPO", "introspect_field_token failed", err)
	}
	if !active {
		return inactive, nil
	}
	authTime, exp := jsontime.New(claims.AuthTime), jsontime.New(claims.Expires)
	out := FieldTokenIntrospectResponse{
		Active: true, Sub: claims.Subject, ClientID: claims.ClientID, WorkerCode: claims.WorkerCode,
		DeviceID: claims.DeviceID, AppCode: claims.AppCode, Amr: claims.AMR, AuthTime: &authTime, Exp: &exp,
		Test: claims.Test, LinkedPrincipalID: claims.LinkedPrincipalID,
	}
	if claims.ActorSub != "" {
		out.Act = &FieldTokenActor{Sub: claims.ActorSub, Typ: claims.ActorType}
	}
	return &apicommon.Out[FieldTokenIntrospectResponse]{Body: out}, nil
}
