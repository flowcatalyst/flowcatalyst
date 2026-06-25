// Package api wires HTTP routes for the client (tenant) subdomain via huma.
package api

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	appops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/application/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/client"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/client/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apicommon"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apiroute"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecaseop"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

// State bundles deps. Applications / ClientConfigs are optional — when
// nil the client→application endpoints surface 501s instead.
type State struct {
	Repo          *client.Repository
	Applications  *application.Repository
	ClientConfigs *application.ClientConfigRepo
	UoW           *usecasepgx.UnitOfWork
}

const tag = "clients"

// Register mounts the client endpoints. Anchor-only.
func Register(api huma.API, s *State) {
	g := apiroute.New(api, tag)
	apiroute.Get(g, "listClients", "/api/clients", "List clients", s.list)
	apiroute.Post(g, "createClient", "/api/clients", "Create a client", http.StatusCreated, s.create)
	apiroute.Post(g, "searchClients", "/api/clients/search", "Search clients", http.StatusOK, s.search)
	// SDK-compatibility alias: the Laravel/Rust client issues
	// GET /api/clients/search?q=<term>. Same search, query-param input.
	apiroute.Get(g, "searchClientsByQuery", "/api/clients/search", "Search clients (SDK alias; ?q=<term>)", s.searchByQuery)
	apiroute.Get(g, "getClientByIdentifier", "/api/clients/by-identifier/{identifier}", "Get a client by identifier", s.byIdentifier)
	apiroute.Get(g, "getClient", "/api/clients/{id}", "Get a client by id", s.getByID)
	apiroute.Put(g, "updateClient", "/api/clients/{id}", "Update a client", http.StatusNoContent, s.update)
	apiroute.Post(g, "activateClient", "/api/clients/{id}/activate", "Activate a client", http.StatusOK, s.activate)
	apiroute.Post(g, "suspendClient", "/api/clients/{id}/suspend", "Suspend a client", http.StatusOK, s.suspend)
	apiroute.Post(g, "addClientNote", "/api/clients/{id}/notes", "Add a note to a client", http.StatusOK, s.addNote)
	apiroute.Delete(g, "deleteClient", "/api/clients/{id}", "Delete a client", http.StatusNoContent, s.delete)
	// Deactivate is an alias for delete (soft-delete with a reason for
	// the audit log). Mirrors Rust's POST /{id}/deactivate handler in
	// crates/fc-platform/src/client/api.rs.
	apiroute.Post(g, "deactivateClient", "/api/clients/{id}/deactivate", "Deactivate a client (soft delete)", http.StatusOK, s.deactivate)
	apiroute.Get(g, "getClientApplications", "/api/clients/{id}/applications", "List applications and their enabled state for the client", s.getApplications)
	apiroute.Put(g, "updateClientApplications", "/api/clients/{id}/applications", "Replace the client's enabled applications (bulk)", http.StatusNoContent, s.updateApplications)
	apiroute.Post(g, "enableClientApplication", "/api/clients/{id}/applications/{applicationId}/enable", "Enable an application for the client", http.StatusNoContent, s.enableApplication)
	apiroute.Post(g, "disableClientApplication", "/api/clients/{id}/applications/{applicationId}/disable", "Disable an application for the client", http.StatusNoContent, s.disableApplication)
}

func (s *State) list(ctx context.Context, _ *apicommon.Empty) (*apicommon.Out[ClientListResponse], error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanReadClients(ac); err != nil {
		return nil, err
	}
	rows, err := s.Repo.FindAll(ctx)
	if err != nil {
		return nil, usecase.Internal("REPO", "find_all failed", err)
	}
	out := apicommon.MapSlice(rows, fromEntity)
	return &apicommon.Out[ClientListResponse]{Body: ClientListResponse{Clients: out, Total: len(out)}}, nil
}

func (s *State) search(ctx context.Context, in *apicommon.In[SearchClientRequest]) (*apicommon.Out[ClientListResponse], error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanReadClients(ac); err != nil {
		return nil, err
	}
	rows, err := s.Repo.Search(ctx, in.Body.Term)
	if err != nil {
		return nil, usecase.Internal("REPO", "search failed", err)
	}
	out := apicommon.MapSlice(rows, fromEntity)
	return &apicommon.Out[ClientListResponse]{Body: ClientListResponse{Clients: out, Total: len(out)}}, nil
}

// searchByQuery backs GET /api/clients/search?q=<term> (SDK alias of the POST
// search). Same logic; the term comes from the query string.
type searchQueryInput struct {
	Q string `query:"q"`
}

func (s *State) searchByQuery(ctx context.Context, in *searchQueryInput) (*apicommon.Out[ClientListResponse], error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanReadClients(ac); err != nil {
		return nil, err
	}
	rows, err := s.Repo.Search(ctx, in.Q)
	if err != nil {
		return nil, usecase.Internal("REPO", "search failed", err)
	}
	out := apicommon.MapSlice(rows, fromEntity)
	return &apicommon.Out[ClientListResponse]{Body: ClientListResponse{Clients: out, Total: len(out)}}, nil
}

type byIdentifierInput struct {
	Identifier string `path:"identifier"`
}

func (s *State) byIdentifier(ctx context.Context, in *byIdentifierInput) (*apicommon.Out[ClientResponse], error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanReadClients(ac); err != nil {
		return nil, err
	}
	c, err := s.Repo.FindByIdentifier(ctx, in.Identifier)
	if err != nil {
		return nil, usecase.Internal("REPO", "find_by_identifier failed", err)
	}
	if c == nil {
		return nil, httperror.NotFound("Client", in.Identifier)
	}
	return &apicommon.Out[ClientResponse]{Body: fromEntity(c)}, nil
}

func (s *State) getByID(ctx context.Context, in *apicommon.IDInput) (*apicommon.Out[ClientResponse], error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanReadClients(ac); err != nil {
		return nil, err
	}
	c, err := s.Repo.FindByID(ctx, in.ID)
	if err != nil {
		return nil, usecase.Internal("REPO", "find_by_id failed", err)
	}
	if c == nil {
		return nil, httperror.NotFound("Client", in.ID)
	}
	return &apicommon.Out[ClientResponse]{Body: fromEntity(c)}, nil
}

func (s *State) create(ctx context.Context, in *apicommon.In[CreateClientRequest]) (*apicommon.Out[apicommon.CreatedResponse], error) {
	// Coarse anchor-only permission at the controller; tenant management has no
	// finer per-resource dimension, so the use case stays Public.
	if err := auth.CanCreateClients(auth.FromContext(ctx)); err != nil {
		return nil, err
	}
	ec := auth.NewExecutionContext(ctx)
	event, err := usecaseop.Run(ctx, s.UoW, operations.CreateClient(s.Repo), in.Body.toCommand(), ec)
	if err != nil {
		return nil, err
	}
	return &apicommon.Out[apicommon.CreatedResponse]{Body: apicommon.CreatedResponse{ID: event.ClientID}}, nil
}

type updateInput struct {
	ID   string `path:"id"`
	Body UpdateClientRequest
}

func (s *State) update(ctx context.Context, in *updateInput) (*apicommon.Empty, error) {
	if err := auth.CanUpdateClients(auth.FromContext(ctx)); err != nil {
		return nil, err
	}
	ec := auth.NewExecutionContext(ctx)
	if _, err := usecaseop.Run(ctx, s.UoW, operations.UpdateClient(s.Repo), in.Body.toCommand(in.ID), ec); err != nil {
		return nil, err
	}
	return &apicommon.Empty{}, nil
}

func (s *State) activate(ctx context.Context, in *apicommon.IDInput) (*apicommon.Out[apicommon.StatusChangeResponse], error) {
	if err := auth.CanUpdateClients(auth.FromContext(ctx)); err != nil {
		return nil, err
	}
	ec := auth.NewExecutionContext(ctx)
	if _, err := usecaseop.Run(ctx, s.UoW, operations.ActivateClient(s.Repo), operations.ActivateCommand{ID: in.ID}, ec); err != nil {
		return nil, err
	}
	return &apicommon.Out[apicommon.StatusChangeResponse]{Body: apicommon.StatusChangeResponse{Message: "Client activated"}}, nil
}

type suspendInput struct {
	ID   string `path:"id"`
	Body SuspendClientRequest
}

func (s *State) suspend(ctx context.Context, in *suspendInput) (*apicommon.Out[apicommon.StatusChangeResponse], error) {
	if err := auth.CanUpdateClients(auth.FromContext(ctx)); err != nil {
		return nil, err
	}
	ec := auth.NewExecutionContext(ctx)
	if _, err := usecaseop.Run(ctx, s.UoW, operations.SuspendClient(s.Repo), operations.SuspendCommand{ID: in.ID, Reason: in.Body.Reason}, ec); err != nil {
		return nil, err
	}
	return &apicommon.Out[apicommon.StatusChangeResponse]{Body: apicommon.StatusChangeResponse{Message: "Client suspended"}}, nil
}

type addNoteInput struct {
	ID   string `path:"id"`
	Body AddNoteRequest
}

func (s *State) addNote(ctx context.Context, in *addNoteInput) (*apicommon.Out[apicommon.StatusChangeResponse], error) {
	if err := auth.CanUpdateClients(auth.FromContext(ctx)); err != nil {
		return nil, err
	}
	ec := auth.NewExecutionContext(ctx)
	if _, err := usecaseop.Run(ctx, s.UoW, operations.AddNote(s.Repo), operations.AddNoteCommand{
		ClientID: in.ID,
		Category: in.Body.Category,
		Text:     in.Body.Text,
	}, ec); err != nil {
		return nil, err
	}
	// Rust returns AddNoteResponse {message}; same wire shape as StatusChangeResponse.
	return &apicommon.Out[apicommon.StatusChangeResponse]{Body: apicommon.StatusChangeResponse{Message: "Note added"}}, nil
}

func (s *State) delete(ctx context.Context, in *apicommon.IDInput) (*apicommon.Empty, error) {
	if err := auth.CanDeleteClients(auth.FromContext(ctx)); err != nil {
		return nil, err
	}
	ec := auth.NewExecutionContext(ctx)
	if _, err := usecaseop.Run(ctx, s.UoW, operations.DeleteClient(s.Repo), operations.DeleteCommand{ID: in.ID}, ec); err != nil {
		return nil, err
	}
	return &apicommon.Empty{}, nil
}

// ── deactivate (alias for delete with a reason) ──────────────────────────

type deactivateInput struct {
	ID   string `path:"id"`
	Body StatusChangeRequest
}

func (s *State) deactivate(ctx context.Context, in *deactivateInput) (*apicommon.Out[apicommon.StatusChangeResponse], error) {
	// Deactivate is a soft-delete — same coarse permission as delete.
	if err := auth.CanDeleteClients(auth.FromContext(ctx)); err != nil {
		return nil, err
	}
	ec := auth.NewExecutionContext(ctx)
	if _, err := usecaseop.Run(ctx, s.UoW, operations.DeleteClient(s.Repo), operations.DeleteCommand{ID: in.ID}, ec); err != nil {
		return nil, err
	}
	return &apicommon.Out[apicommon.StatusChangeResponse]{Body: apicommon.StatusChangeResponse{Message: "Client deactivated"}}, nil
}

// ── client→application linking ───────────────────────────────────────────

func (s *State) getApplications(ctx context.Context, in *apicommon.IDInput) (*apicommon.Out[ClientApplicationsResponse], error) {
	ac := auth.FromContext(ctx)
	if !ac.IsAnchor() && !ac.CanAccessClient(in.ID) {
		return nil, httperror.Forbidden("No access to this client")
	}
	if s.Applications == nil {
		return nil, usecase.Internal("WIRING", "applications repo not configured", nil)
	}

	c, err := s.Repo.FindByID(ctx, in.ID)
	if err != nil {
		return nil, usecase.Internal("REPO", "find_client failed", err)
	}
	if c == nil {
		return nil, httperror.NotFound("Client", in.ID)
	}

	allApps, err := s.Applications.FindWithFilters(ctx, nil, nil)
	if err != nil {
		return nil, usecase.Internal("REPO", "find_all applications failed", err)
	}

	enabledByApp := make(map[string]bool, len(allApps))
	if s.ClientConfigs != nil {
		cfgs, err := s.ClientConfigs.FindByClient(ctx, in.ID)
		if err != nil {
			return nil, usecase.Internal("REPO", "find_configs failed", err)
		}
		for _, cfg := range cfgs {
			enabledByApp[cfg.ApplicationID] = cfg.Enabled
		}
	}

	out := apicommon.MapSlice(allApps, func(a *application.Application) ClientApplicationResponse {
		return ClientApplicationResponse{
			ID:               a.ID,
			Code:             a.Code,
			Name:             a.Name,
			Description:      a.Description,
			IconURL:          a.IconURL,
			Active:           a.Active,
			EnabledForClient: enabledByApp[a.ID],
		}
	})
	return &apicommon.Out[ClientApplicationsResponse]{Body: ClientApplicationsResponse{
		Applications: out,
		Total:        len(out),
	}}, nil
}

type updateApplicationsInput struct {
	ID   string `path:"id"`
	Body UpdateClientApplicationsRequest
}

func (s *State) updateApplications(ctx context.Context, in *updateApplicationsInput) (*apicommon.Empty, error) {
	ac := auth.FromContext(ctx)
	if !ac.IsAnchor() {
		return nil, httperror.Forbidden("Anchor scope required")
	}
	if s.Applications == nil || s.ClientConfigs == nil {
		return nil, usecase.Internal("WIRING", "application repos not configured", nil)
	}
	ec := usecase.NewExecutionContext(ac.PrincipalID)
	cmd := appops.UpdateClientApplicationsCommand{
		ClientID:              in.ID,
		EnabledApplicationIDs: in.Body.EnabledApplicationIDs,
	}
	if _, err := usecaseop.Run(ctx, s.UoW, appops.UpdateClientApplications(s.Applications, s.Repo, s.ClientConfigs), cmd, ec); err != nil {
		return nil, err
	}
	return &apicommon.Empty{}, nil
}

type appLinkInput struct {
	ID            string `path:"id"`
	ApplicationID string `path:"applicationId"`
}

func (s *State) enableApplication(ctx context.Context, in *appLinkInput) (*apicommon.Empty, error) {
	ac := auth.FromContext(ctx)
	if !ac.IsAnchor() {
		return nil, httperror.Forbidden("Anchor scope required")
	}
	if s.Applications == nil || s.ClientConfigs == nil {
		return nil, usecase.Internal("WIRING", "application repos not configured", nil)
	}
	ec := usecase.NewExecutionContext(ac.PrincipalID)
	cmd := appops.EnableForClientCommand{
		ApplicationID: in.ApplicationID,
		ClientID:      in.ID,
	}
	if _, err := usecaseop.Run(ctx, s.UoW, appops.EnableApplicationForClient(s.Applications, s.Repo, s.ClientConfigs), cmd, ec); err != nil {
		return nil, err
	}
	return &apicommon.Empty{}, nil
}

func (s *State) disableApplication(ctx context.Context, in *appLinkInput) (*apicommon.Empty, error) {
	ac := auth.FromContext(ctx)
	if !ac.IsAnchor() {
		return nil, httperror.Forbidden("Anchor scope required")
	}
	if s.ClientConfigs == nil {
		return nil, usecase.Internal("WIRING", "client_configs repo not configured", nil)
	}
	ec := usecase.NewExecutionContext(ac.PrincipalID)
	cmd := appops.DisableForClientCommand{
		ApplicationID: in.ApplicationID,
		ClientID:      in.ID,
	}
	if _, err := usecaseop.Run(ctx, s.UoW, appops.DisableApplicationForClient(s.ClientConfigs), cmd, ec); err != nil {
		return nil, err
	}
	return &apicommon.Empty{}, nil
}
