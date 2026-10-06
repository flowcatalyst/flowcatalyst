// Package api wires the HTTP routes for the event_type subdomain via
// danielgtaylor/huma/v2. The Input/Output structs in dto.go are the
// source of truth for the OpenAPI spec; huma derives the spec from
// them at registration time.
package api

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/flowcatalyst/flowcatalyst-go/internal/ids"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/eventtype"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/eventtype/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apicommon"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apiroute"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecaseop"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

// State bundles deps for the event-type handlers.
type State struct {
	Repo *eventtype.Repository
	UoW  *usecasepgx.UnitOfWork
	// Apps resolves an event type's application code to its id, for confining
	// application service accounts to their own applications' event types.
	Apps ApplicationLookup
}

// ApplicationLookup is the one application read the event-type handlers need.
type ApplicationLookup interface {
	FindByCode(ctx context.Context, code string) (*application.Application, error)
}

// appAccess reports whether the principal is bound to the application with the
// given code. Results are remembered in seen for the life of one request, so a
// list resolves each application once. An unknown application is not accessible.
func (s *State) appAccess(ctx context.Context, ac *auth.AuthContext, code string, seen map[string]bool) (bool, error) {
	if ok, done := seen[code]; done {
		return ok, nil
	}
	app, err := s.Apps.FindByCode(ctx, code)
	if err != nil {
		return false, usecase.Internal("REPO", "find application by code failed", err)
	}
	ok := app != nil && ac.CanAccessApplication(string(app.ID))
	seen[code] = ok
	return ok, nil
}

// requireEventTypeAppAccess refuses an application-confined principal an event
// type that belongs to an application it is not bound to.
func (s *State) requireEventTypeAppAccess(ctx context.Context, ac *auth.AuthContext, et *eventtype.EventType) error {
	ok, err := s.appAccess(ctx, ac, et.Application, map[string]bool{})
	if err != nil {
		return err
	}
	if !ok {
		return httperror.Forbidden("No access to this event type")
	}
	return nil
}

const tag = "event-types"

// Register mounts the event-type endpoints on the supplied huma API.
// Routes preserve the established wire contract exactly (path,
// method, status code).
func Register(api huma.API, s *State) {
	g := apiroute.New(api, tag)
	apiroute.Get(g, "listEventTypes", "/api/event-types", "List event types", s.list)
	apiroute.Post(g, "createEventType", "/api/event-types", "Create an event type", http.StatusCreated, s.create)
	apiroute.Get(g, "getEventType", "/api/event-types/{id}", "Get an event type by id", s.getByID)
	apiroute.Get(g, "getEventTypeByCode", "/api/event-types/by-code/{code}", "Get an event type by code", s.getByCode)
	apiroute.Put(g, "updateEventType", "/api/event-types/{id}", "Update an event type", http.StatusNoContent, s.update)
	apiroute.Delete(g, "deleteEventType", "/api/event-types/{id}", "Archive an event type", http.StatusNoContent, s.delete)
	apiroute.Post(g, "addEventTypeSchema", "/api/event-types/{id}/schemas", "Add a schema version to an event type (Go-historical alias)", http.StatusOK, s.addSchema)
	// /versions is the canonical path. Same handler; both paths
	// remain registered so existing SPA clients on /schemas keep working.
	apiroute.Post(g, "addEventTypeVersion", "/api/event-types/{id}/versions", "Add a schema version to an event type", http.StatusOK, s.addSchema)
}

// ── Handlers ──────────────────────────────────────────────────────────────

type listInput struct {
	Application string `query:"application" doc:"Filter by application code"`
	ClientID    string `query:"clientId" doc:"Filter by client id"`
	Status      string `query:"status" doc:"Filter by status (CURRENT, ARCHIVED)"`
	Subdomain   string `query:"subdomain" doc:"Filter by subdomain"`
	Aggregate   string `query:"aggregate" doc:"Filter by aggregate"`
}

func (s *State) list(ctx context.Context, in *listInput) (*apicommon.Out[EventTypeListResponse], error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanReadEventTypes(ac); err != nil {
		return nil, err
	}

	application := apicommon.OptStr(in.Application)
	clientID := apicommon.OptStr(in.ClientID)
	status := apicommon.OptStr(in.Status)
	subdomain := apicommon.OptStr(in.Subdomain)
	aggregate := apicommon.OptStr(in.Aggregate)
	if application == nil && clientID == nil && status == nil && subdomain == nil && aggregate == nil {
		def := "CURRENT"
		status = &def
	}

	rows, err := s.Repo.FindWithFilters(ctx, application, clientID, status, subdomain, aggregate)
	if err != nil {
		return nil, usecase.Internal("REPO", "find_with_filters failed", err)
	}
	visible := auth.FilterClientScoped(ac, rows, func(et *eventtype.EventType) *string { return ids.StringPtr(et.ClientID) })
	if !auth.CanReadAllEventTypes(ac) {
		// An application service account sees only its own applications' event types.
		seen := map[string]bool{}
		kept := visible[:0:0]
		for i := range visible {
			ok, err := s.appAccess(ctx, ac, visible[i].Application, seen)
			if err != nil {
				return nil, err
			}
			if ok {
				kept = append(kept, visible[i])
			}
		}
		visible = kept
	}
	out := apicommon.MapSlice(visible, fromEntity)
	return &apicommon.Out[EventTypeListResponse]{Body: EventTypeListResponse{Items: out}}, nil
}

type getByIDInput struct {
	ID string `path:"id" doc:"Event type id (TSID)"`
}

func (s *State) getByID(ctx context.Context, in *getByIDInput) (*apicommon.Out[EventTypeResponse], error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanReadEventTypes(ac); err != nil {
		return nil, err
	}
	et, err := s.Repo.FindByID(ctx, in.ID)
	if err != nil {
		return nil, usecase.Internal("REPO", "find_by_id failed", err)
	}
	if et == nil {
		return nil, httperror.NotFound("EventType", in.ID)
	}
	if et.ClientID != nil && !ac.CanAccessClient(string(*et.ClientID)) {
		return nil, httperror.Forbidden("No access to this event type")
	}
	if !auth.CanReadAllEventTypes(ac) {
		if err := s.requireEventTypeAppAccess(ctx, ac, et); err != nil {
			return nil, err
		}
	}
	return &apicommon.Out[EventTypeResponse]{Body: fromEntity(et)}, nil
}

type getByCodeInput struct {
	Code string `path:"code" doc:"Event type code (e.g. platform:iam:user:created)"`
}

func (s *State) getByCode(ctx context.Context, in *getByCodeInput) (*apicommon.Out[EventTypeResponse], error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanReadEventTypes(ac); err != nil {
		return nil, err
	}
	et, err := s.Repo.FindByCode(ctx, in.Code)
	if err != nil {
		return nil, usecase.Internal("REPO", "find_by_code failed", err)
	}
	if et == nil {
		return nil, httperror.NotFound("EventType", in.Code)
	}
	if et.ClientID != nil && !ac.CanAccessClient(string(*et.ClientID)) {
		return nil, httperror.Forbidden("No access to this event type")
	}
	if !auth.CanReadAllEventTypes(ac) {
		if err := s.requireEventTypeAppAccess(ctx, ac, et); err != nil {
			return nil, err
		}
	}
	return &apicommon.Out[EventTypeResponse]{Body: fromEntity(et)}, nil
}

func (s *State) create(ctx context.Context, in *apicommon.In[CreateEventTypeRequest]) (*apicommon.Out[apicommon.CreatedResponse], error) {
	// Coarse permission at the controller; the use case enforces per-client
	// resource access (you may only bind an event type to a client you can
	// access; platform-wide requires anchor).
	if err := auth.CanWriteEventTypes(auth.FromContext(ctx)); err != nil {
		return nil, err
	}
	ec := auth.NewExecutionContext(ctx)
	event, err := usecaseop.Run(ctx, s.UoW, operations.CreateEventType(s.Repo), in.Body.toCommand(), ec)
	if err != nil {
		return nil, err
	}
	return &apicommon.Out[apicommon.CreatedResponse]{Body: apicommon.CreatedResponse{ID: event.EventTypeID}}, nil
}

type updateInput struct {
	ID   string `path:"id"`
	Body UpdateEventTypeRequest
}

func (s *State) update(ctx context.Context, in *updateInput) (*apicommon.Empty, error) {
	if err := auth.CanWriteEventTypes(auth.FromContext(ctx)); err != nil {
		return nil, err
	}
	ec := auth.NewExecutionContext(ctx)
	if _, err := usecaseop.Run(ctx, s.UoW, operations.UpdateEventType(s.Repo), in.Body.toCommand(in.ID), ec); err != nil {
		return nil, err
	}
	return &apicommon.Empty{}, nil
}

func (s *State) delete(ctx context.Context, in *apicommon.IDInput) (*apicommon.Empty, error) {
	if err := auth.CanDeleteEventTypes(auth.FromContext(ctx)); err != nil {
		return nil, err
	}
	ec := auth.NewExecutionContext(ctx)
	if _, err := usecaseop.Run(ctx, s.UoW, operations.DeleteEventType(s.Repo), operations.DeleteCommand{ID: in.ID}, ec); err != nil {
		return nil, err
	}
	return &apicommon.Empty{}, nil
}

type addSchemaInput struct {
	ID   string `path:"id"`
	Body AddSchemaRequest
}

func (s *State) addSchema(ctx context.Context, in *addSchemaInput) (*apicommon.Out[EventTypeResponse], error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanAddEventTypeSchema(ac); err != nil {
		return nil, err
	}
	if !auth.CanWriteAllEventTypes(ac) {
		// An application service account may version only its own applications' event types.
		target, err := s.Repo.FindByID(ctx, in.ID)
		if err != nil {
			return nil, usecase.Internal("REPO", "find_by_id failed", err)
		}
		if target == nil {
			return nil, httperror.NotFound("EventType", in.ID)
		}
		if err := s.requireEventTypeAppAccess(ctx, ac, target); err != nil {
			return nil, err
		}
	}
	ec := auth.NewExecutionContext(ctx)
	if _, err := usecaseop.Run(ctx, s.UoW, operations.AddSchema(s.Repo), in.Body.toCommand(in.ID), ec); err != nil {
		return nil, err
	}
	et, err := s.Repo.FindByID(ctx, in.ID)
	if err != nil {
		return nil, usecase.Internal("REPO", "find_by_id failed", err)
	}
	if et == nil {
		return nil, httperror.NotFound("EventType", in.ID)
	}
	// Return the updated event type (full EventTypeResponse).
	return &apicommon.Out[EventTypeResponse]{Body: fromEntity(et)}, nil
}
