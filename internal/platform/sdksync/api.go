// Package sdksync wires the SDK self-registration ("sync") routes, scoped
// under /api/applications/{appCode}. These are the declarative endpoints an
// application's SDK calls at boot to register its resources (event types,
// roles, subscriptions, dispatch pools, principals, processes, scheduled
// jobs, openapi spec) in one idempotent batch.
//
// Mirrors crates/fc-platform/src/shared/sdk_sync_api.rs (the Rust
// sdk_sync_router nested under /api/applications) exactly: path, method,
// request body shape, and the shared SyncResultResponse
// {applicationCode, created, updated, deleted, syncedCodes} wire shape.
//
// Each handler resolves {appCode} to an Application (404 when unknown),
// checks the resource's sync permission, then delegates to that resource's
// Sync<Resource> use case and maps its rollup event onto the response.
//
// Endpoints land incrementally; event-types is wired first (it reuses the
// existing, tested eventtype Sync use case). The remaining resources
// (roles, subscriptions, dispatch-pools, principals, processes,
// scheduled-jobs, openapi) follow the same shape.
package sdksync

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchpool"
	dispatchpoolops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchpool/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/eventtype"
	eventtypeops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/eventtype/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/openapispecs"
	openapiops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/openapispecs/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/process"
	processops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/process/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/role"
	roleops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/role/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

const tag = "sdk-sync"

// State bundles the deps shared by the SDK sync handlers.
type State struct {
	Apps          *application.Repository
	EventTypes    *eventtype.Repository
	Roles         *role.Repository
	Processes     *process.Repository
	DispatchPools *dispatchpool.Repository
	Specs         *openapispecs.Repository
	UoW           *usecasepgx.UnitOfWork
}

// SyncResultResponse is the shared result for the list-based sync
// endpoints. Mirrors the Rust SyncResultResponse (camelCase wire shape).
type SyncResultResponse struct {
	ApplicationCode string   `json:"applicationCode"`
	Created         uint32   `json:"created"`
	Updated         uint32   `json:"updated"`
	Deleted         uint32   `json:"deleted"`
	SyncedCodes     []string `json:"syncedCodes"`
}

// Register mounts the SDK sync endpoints on the supplied huma API. Paths
// match the Rust sdk_sync_router nested under /api/applications.
func Register(api huma.API, s *State) {
	huma.Register(api, huma.Operation{
		OperationID:   "syncRoles",
		Method:        http.MethodPost,
		Path:          "/api/applications/{appCode}/roles/sync",
		Summary:       "Sync an application's roles (SDK self-registration)",
		Tags:          []string{tag},
		DefaultStatus: http.StatusOK,
	}, s.syncRoles)

	huma.Register(api, huma.Operation{
		OperationID:   "syncEventTypes",
		Method:        http.MethodPost,
		Path:          "/api/applications/{appCode}/event-types/sync",
		Summary:       "Sync an application's event types (SDK self-registration)",
		Tags:          []string{tag},
		DefaultStatus: http.StatusOK,
	}, s.syncEventTypes)

	huma.Register(api, huma.Operation{
		OperationID:   "syncDispatchPools",
		Method:        http.MethodPost,
		Path:          "/api/applications/{appCode}/dispatch-pools/sync",
		Summary:       "Sync dispatch pools (SDK self-registration)",
		Tags:          []string{tag},
		DefaultStatus: http.StatusOK,
	}, s.syncDispatchPools)

	huma.Register(api, huma.Operation{
		OperationID:   "syncProcesses",
		Method:        http.MethodPost,
		Path:          "/api/applications/{appCode}/processes/sync",
		Summary:       "Sync an application's processes (SDK self-registration)",
		Tags:          []string{tag},
		DefaultStatus: http.StatusOK,
	}, s.syncProcesses)

	huma.Register(api, huma.Operation{
		OperationID:   "syncOpenapi",
		Method:        http.MethodPost,
		Path:          "/api/applications/{appCode}/openapi/sync",
		Summary:       "Sync an application's OpenAPI document (SDK self-registration)",
		Tags:          []string{tag},
		DefaultStatus: http.StatusOK,
	}, s.syncOpenapi)
}

// resolveApp loads the application by code, returning a 404 when unknown —
// matching the Rust handlers' 404-on-unknown-application contract.
func (s *State) resolveApp(ctx context.Context, code string) (*application.Application, error) {
	app, err := s.Apps.FindByCode(ctx, code)
	if err != nil {
		return nil, usecase.Internal("REPO", "find_by_code failed", err)
	}
	if app == nil {
		return nil, httperror.NotFound("Application", code)
	}
	return app, nil
}

// ── Event types ─────────────────────────────────────────────────────────

type syncEventTypeInputRequest struct {
	Code        string  `json:"code" doc:"Full code (application:subdomain:aggregate:event)"`
	Name        string  `json:"name"`
	Description *string `json:"description,omitempty"`
}

type syncEventTypesRequest struct {
	EventTypes []syncEventTypeInputRequest `json:"eventTypes"`
}

type syncEventTypesInput struct {
	AppCode        string `path:"appCode" doc:"Application code"`
	RemoveUnlisted bool   `query:"removeUnlisted" doc:"Remove API-sourced event types not in the list"`
	Body           syncEventTypesRequest
}

type syncResultOutput struct {
	Body SyncResultResponse
}

func (s *State) syncEventTypes(ctx context.Context, in *syncEventTypesInput) (*syncResultOutput, error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanSyncEventTypes(ac); err != nil {
		return nil, err
	}
	app, err := s.resolveApp(ctx, in.AppCode)
	if err != nil {
		return nil, err
	}

	inputs := make([]eventtypeops.SyncEventTypeInput, 0, len(in.Body.EventTypes))
	for _, et := range in.Body.EventTypes {
		inputs = append(inputs, eventtypeops.SyncEventTypeInput{
			Code:        et.Code,
			Name:        et.Name,
			Description: et.Description,
		})
	}

	cmd := eventtypeops.SyncEventTypesCommand{
		ApplicationCode: app.Code,
		EventTypes:      inputs,
		RemoveUnlisted:  in.RemoveUnlisted,
	}
	ec := usecase.NewExecutionContext(ac.PrincipalID)
	committed, err := eventtypeops.SyncEventTypes(ctx, s.EventTypes, s.UoW, cmd, ec)
	if err != nil {
		return nil, err
	}
	ev := committed.Event()
	return &syncResultOutput{Body: SyncResultResponse{
		ApplicationCode: ev.ApplicationCode,
		Created:         ev.Created,
		Updated:         ev.Updated,
		Deleted:         ev.Deleted,
		SyncedCodes:     ev.SyncedCodes,
	}}, nil
}

// ── Roles ─────────────────────────────────────────────────────────────────

type syncRoleInputRequest struct {
	Name          string   `json:"name"`
	DisplayName   *string  `json:"displayName,omitempty"`
	Description   *string  `json:"description,omitempty"`
	Permissions   []string `json:"permissions,omitempty"`
	ClientManaged bool     `json:"clientManaged,omitempty"`
}

type syncRolesRequest struct {
	Roles []syncRoleInputRequest `json:"roles"`
}

type syncRolesInput struct {
	AppCode        string `path:"appCode" doc:"Application code"`
	RemoveUnlisted bool   `query:"removeUnlisted" doc:"Remove SDK roles not in the list"`
	Body           syncRolesRequest
}

func (s *State) syncRoles(ctx context.Context, in *syncRolesInput) (*syncResultOutput, error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanSyncRoles(ac); err != nil {
		return nil, err
	}
	app, err := s.resolveApp(ctx, in.AppCode)
	if err != nil {
		return nil, err
	}

	inputs := make([]roleops.SyncRoleInput, 0, len(in.Body.Roles))
	for _, r := range in.Body.Roles {
		inputs = append(inputs, roleops.SyncRoleInput{
			Name:          r.Name,
			DisplayName:   r.DisplayName,
			Description:   r.Description,
			Permissions:   r.Permissions,
			ClientManaged: r.ClientManaged,
		})
	}

	cmd := roleops.SyncRolesCommand{
		ApplicationCode: app.Code,
		ApplicationID:   app.ID,
		Roles:           inputs,
		RemoveUnlisted:  in.RemoveUnlisted,
	}
	ec := usecase.NewExecutionContext(ac.PrincipalID)
	committed, err := roleops.SyncRoles(ctx, s.Roles, s.UoW, cmd, ec)
	if err != nil {
		return nil, err
	}
	ev := committed.Event()
	return &syncResultOutput{Body: SyncResultResponse{
		ApplicationCode: ev.ApplicationCode,
		Created:         ev.Created,
		Updated:         ev.Updated,
		Deleted:         ev.Removed,
		SyncedCodes:     ev.SyncedCodes,
	}}, nil
}

// ── Dispatch pools ────────────────────────────────────────────────────────

type syncDispatchPoolInputRequest struct {
	Code        string  `json:"code"`
	Name        string  `json:"name"`
	Description *string `json:"description,omitempty"`
	RateLimit   *int32  `json:"rateLimit,omitempty" doc:"Messages per minute; omit for concurrency-only"`
	// Concurrency defaults to 10 when omitted (matches the Rust default).
	Concurrency *int32 `json:"concurrency,omitempty"`
}

type syncDispatchPoolsRequest struct {
	Pools []syncDispatchPoolInputRequest `json:"pools"`
}

type syncDispatchPoolsInput struct {
	AppCode        string `path:"appCode" doc:"Application code"`
	RemoveUnlisted bool   `query:"removeUnlisted" doc:"Archive pools not in the list"`
	Body           syncDispatchPoolsRequest
}

func (s *State) syncDispatchPools(ctx context.Context, in *syncDispatchPoolsInput) (*syncResultOutput, error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanSyncDispatchPools(ac); err != nil {
		return nil, err
	}
	app, err := s.resolveApp(ctx, in.AppCode)
	if err != nil {
		return nil, err
	}

	inputs := make([]dispatchpoolops.SyncDispatchPoolInput, 0, len(in.Body.Pools))
	for _, p := range in.Body.Pools {
		concurrency := int32(10) // serde default_concurrency
		if p.Concurrency != nil {
			concurrency = *p.Concurrency
		}
		inputs = append(inputs, dispatchpoolops.SyncDispatchPoolInput{
			Code:        p.Code,
			Name:        p.Name,
			Description: p.Description,
			RateLimit:   p.RateLimit,
			Concurrency: concurrency,
		})
	}

	cmd := dispatchpoolops.SyncDispatchPoolsCommand{
		ApplicationCode: app.Code,
		Pools:           inputs,
		RemoveUnlisted:  in.RemoveUnlisted,
	}
	ec := usecase.NewExecutionContext(ac.PrincipalID)
	committed, err := dispatchpoolops.SyncDispatchPools(ctx, s.DispatchPools, s.UoW, cmd, ec)
	if err != nil {
		return nil, err
	}
	ev := committed.Event()
	return &syncResultOutput{Body: SyncResultResponse{
		ApplicationCode: ev.ApplicationCode,
		Created:         ev.Created,
		Updated:         ev.Updated,
		Deleted:         ev.Deleted,
		SyncedCodes:     ev.SyncedCodes,
	}}, nil
}

// ── Processes ─────────────────────────────────────────────────────────────

type syncProcessInputRequest struct {
	Code        string   `json:"code" doc:"Full code (application:subdomain:process-name)"`
	Name        string   `json:"name"`
	Description *string  `json:"description,omitempty"`
	Body        string   `json:"body,omitempty" doc:"Diagram body (typically Mermaid source)"`
	DiagramType *string  `json:"diagramType,omitempty"`
	Tags        []string `json:"tags,omitempty"`
}

type syncProcessesRequest struct {
	Processes []syncProcessInputRequest `json:"processes"`
}

type syncProcessesInput struct {
	AppCode        string `path:"appCode" doc:"Application code"`
	RemoveUnlisted bool   `query:"removeUnlisted" doc:"Remove API/CODE processes not in the list"`
	Body           syncProcessesRequest
}

func (s *State) syncProcesses(ctx context.Context, in *syncProcessesInput) (*syncResultOutput, error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanSyncProcesses(ac); err != nil {
		return nil, err
	}
	app, err := s.resolveApp(ctx, in.AppCode)
	if err != nil {
		return nil, err
	}

	inputs := make([]processops.SyncProcessInput, 0, len(in.Body.Processes))
	for _, p := range in.Body.Processes {
		inputs = append(inputs, processops.SyncProcessInput{
			Code:        p.Code,
			Name:        p.Name,
			Description: p.Description,
			Body:        p.Body,
			DiagramType: p.DiagramType,
			Tags:        p.Tags,
		})
	}

	cmd := processops.SyncProcessesCommand{
		ApplicationCode: app.Code,
		Processes:       inputs,
		RemoveUnlisted:  in.RemoveUnlisted,
	}
	ec := usecase.NewExecutionContext(ac.PrincipalID)
	committed, err := processops.SyncProcesses(ctx, s.Processes, s.UoW, cmd, ec)
	if err != nil {
		return nil, err
	}
	ev := committed.Event()
	return &syncResultOutput{Body: SyncResultResponse{
		ApplicationCode: ev.ApplicationCode,
		Created:         ev.Created,
		Updated:         ev.Updated,
		Deleted:         ev.Deleted,
		SyncedCodes:     ev.SyncedCodes,
	}}, nil
}

// ── OpenAPI document ──────────────────────────────────────────────────────

// SyncOpenApiSpecResponse is the openapi sync result. Unlike the list-based
// resources this is a single-document, versioned sync, so it has its own
// shape (mirrors the Rust SyncOpenApiSpecResponse).
type SyncOpenApiSpecResponse struct {
	ApplicationCode      string  `json:"applicationCode"`
	SpecID               string  `json:"specId"`
	Version              string  `json:"version"`
	Status               string  `json:"status"`
	ArchivedPriorVersion *string `json:"archivedPriorVersion,omitempty"`
	HasBreaking          bool    `json:"hasBreaking"`
	Unchanged            bool    `json:"unchanged"`
}

type syncOpenapiRequest struct {
	Spec json.RawMessage `json:"spec" doc:"The OpenAPI document (OpenAPI 3.x or Swagger 2.x)"`
}

type syncOpenapiInput struct {
	AppCode string `path:"appCode" doc:"Application code"`
	Body    syncOpenapiRequest
}

type syncOpenapiOutput struct {
	Body SyncOpenApiSpecResponse
}

func (s *State) syncOpenapi(ctx context.Context, in *syncOpenapiInput) (*syncOpenapiOutput, error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanSyncApplicationOpenAPI(ac); err != nil {
		return nil, err
	}
	app, err := s.resolveApp(ctx, in.AppCode)
	if err != nil {
		return nil, err
	}

	// Resource-level guard: anchor / super-admin may sync any application;
	// otherwise the caller must BE this application's bound service account.
	// Mirrors the Rust handler's per-application gate.
	isAppServiceAccount := app.ServiceAccountID != nil && *app.ServiceAccountID == ac.PrincipalID
	if !(ac.IsAnchor() || ac.IsSuperAdmin() || isAppServiceAccount) {
		return nil, httperror.Forbidden("Service account is not authorised for application '" + app.Code + "'")
	}

	cmd := openapiops.SyncOpenApiSpecCommand{
		ApplicationID:   app.ID,
		ApplicationCode: app.Code,
		Spec:            in.Body.Spec,
	}
	ec := usecase.NewExecutionContext(ac.PrincipalID)
	committed, err := openapiops.SyncOpenApiSpec(ctx, s.Specs, s.UoW, cmd, ec)
	if err != nil {
		return nil, err
	}
	ev := committed.Event()
	status := "CURRENT"
	if ev.Unchanged {
		status = "UNCHANGED"
	}
	return &syncOpenapiOutput{Body: SyncOpenApiSpecResponse{
		ApplicationCode:      ev.ApplicationCode,
		SpecID:               ev.SpecID,
		Version:              ev.Version,
		Status:               status,
		ArchivedPriorVersion: ev.ArchivedPriorVersion,
		HasBreaking:          ev.HasBreaking,
		Unchanged:            ev.Unchanged,
	}}, nil
}
