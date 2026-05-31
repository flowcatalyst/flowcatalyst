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
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
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
	huma.Register(api, huma.Operation{
		OperationID:   "listClients",
		Method:        http.MethodGet,
		Path:          "/api/clients",
		Summary:       "List clients",
		Tags:          []string{tag},
		DefaultStatus: http.StatusOK,
	}, s.list)

	huma.Register(api, huma.Operation{
		OperationID:   "createClient",
		Method:        http.MethodPost,
		Path:          "/api/clients",
		Summary:       "Create a client",
		Tags:          []string{tag},
		DefaultStatus: http.StatusCreated,
	}, s.create)

	huma.Register(api, huma.Operation{
		OperationID:   "searchClients",
		Method:        http.MethodPost,
		Path:          "/api/clients/search",
		Summary:       "Search clients",
		Tags:          []string{tag},
		DefaultStatus: http.StatusOK,
	}, s.search)

	// SDK-compatibility alias: the Laravel/Rust client issues
	// GET /api/clients/search?q=<term>. Same search, query-param input.
	huma.Register(api, huma.Operation{
		OperationID:   "searchClientsByQuery",
		Method:        http.MethodGet,
		Path:          "/api/clients/search",
		Summary:       "Search clients (SDK alias; ?q=<term>)",
		Tags:          []string{tag},
		DefaultStatus: http.StatusOK,
	}, s.searchByQuery)

	huma.Register(api, huma.Operation{
		OperationID:   "getClientByIdentifier",
		Method:        http.MethodGet,
		Path:          "/api/clients/by-identifier/{identifier}",
		Summary:       "Get a client by identifier",
		Tags:          []string{tag},
		DefaultStatus: http.StatusOK,
	}, s.byIdentifier)

	huma.Register(api, huma.Operation{
		OperationID:   "getClient",
		Method:        http.MethodGet,
		Path:          "/api/clients/{id}",
		Summary:       "Get a client by id",
		Tags:          []string{tag},
		DefaultStatus: http.StatusOK,
	}, s.getByID)

	huma.Register(api, huma.Operation{
		OperationID:   "updateClient",
		Method:        http.MethodPut,
		Path:          "/api/clients/{id}",
		Summary:       "Update a client",
		Tags:          []string{tag},
		DefaultStatus: http.StatusNoContent,
	}, s.update)

	huma.Register(api, huma.Operation{
		OperationID:   "activateClient",
		Method:        http.MethodPost,
		Path:          "/api/clients/{id}/activate",
		Summary:       "Activate a client",
		Tags:          []string{tag},
		DefaultStatus: http.StatusNoContent,
	}, s.activate)

	huma.Register(api, huma.Operation{
		OperationID:   "suspendClient",
		Method:        http.MethodPost,
		Path:          "/api/clients/{id}/suspend",
		Summary:       "Suspend a client",
		Tags:          []string{tag},
		DefaultStatus: http.StatusNoContent,
	}, s.suspend)

	huma.Register(api, huma.Operation{
		OperationID:   "addClientNote",
		Method:        http.MethodPost,
		Path:          "/api/clients/{id}/notes",
		Summary:       "Add a note to a client",
		Tags:          []string{tag},
		DefaultStatus: http.StatusNoContent,
	}, s.addNote)

	huma.Register(api, huma.Operation{
		OperationID:   "deleteClient",
		Method:        http.MethodDelete,
		Path:          "/api/clients/{id}",
		Summary:       "Delete a client",
		Tags:          []string{tag},
		DefaultStatus: http.StatusNoContent,
	}, s.delete)

	// Deactivate is an alias for delete (soft-delete with a reason for
	// the audit log). Mirrors Rust's POST /{id}/deactivate handler in
	// crates/fc-platform/src/client/api.rs.
	huma.Register(api, huma.Operation{
		OperationID:   "deactivateClient",
		Method:        http.MethodPost,
		Path:          "/api/clients/{id}/deactivate",
		Summary:       "Deactivate a client (soft delete)",
		Tags:          []string{tag},
		DefaultStatus: http.StatusOK,
	}, s.deactivate)

	huma.Register(api, huma.Operation{
		OperationID:   "getClientApplications",
		Method:        http.MethodGet,
		Path:          "/api/clients/{id}/applications",
		Summary:       "List applications and their enabled state for the client",
		Tags:          []string{tag},
		DefaultStatus: http.StatusOK,
	}, s.getApplications)

	huma.Register(api, huma.Operation{
		OperationID:   "updateClientApplications",
		Method:        http.MethodPut,
		Path:          "/api/clients/{id}/applications",
		Summary:       "Replace the client's enabled applications (bulk)",
		Tags:          []string{tag},
		DefaultStatus: http.StatusNoContent,
	}, s.updateApplications)

	huma.Register(api, huma.Operation{
		OperationID:   "enableClientApplication",
		Method:        http.MethodPost,
		Path:          "/api/clients/{id}/applications/{applicationId}/enable",
		Summary:       "Enable an application for the client",
		Tags:          []string{tag},
		DefaultStatus: http.StatusNoContent,
	}, s.enableApplication)

	huma.Register(api, huma.Operation{
		OperationID:   "disableClientApplication",
		Method:        http.MethodPost,
		Path:          "/api/clients/{id}/applications/{applicationId}/disable",
		Summary:       "Disable an application for the client",
		Tags:          []string{tag},
		DefaultStatus: http.StatusNoContent,
	}, s.disableApplication)
}

type emptyInput struct{}

type listOutput struct {
	Body ClientListResponse
}

func (s *State) list(ctx context.Context, _ *emptyInput) (*listOutput, error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanReadClients(ac); err != nil {
		return nil, err
	}
	rows, err := s.Repo.FindAll(ctx)
	if err != nil {
		return nil, usecase.Internal("REPO", "find_all failed", err)
	}
	out := make([]ClientResponse, 0, len(rows))
	for i := range rows {
		out = append(out, fromEntity(&rows[i]))
	}
	return &listOutput{Body: ClientListResponse{Clients: out, Total: len(out)}}, nil
}

type searchInput struct {
	Body SearchClientRequest
}

func (s *State) search(ctx context.Context, in *searchInput) (*listOutput, error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanReadClients(ac); err != nil {
		return nil, err
	}
	rows, err := s.Repo.Search(ctx, in.Body.Term)
	if err != nil {
		return nil, usecase.Internal("REPO", "search failed", err)
	}
	out := make([]ClientResponse, 0, len(rows))
	for i := range rows {
		out = append(out, fromEntity(&rows[i]))
	}
	return &listOutput{Body: ClientListResponse{Clients: out, Total: len(out)}}, nil
}

// searchByQuery backs GET /api/clients/search?q=<term> (SDK alias of the POST
// search). Same logic; the term comes from the query string.
type searchQueryInput struct {
	Q string `query:"q"`
}

func (s *State) searchByQuery(ctx context.Context, in *searchQueryInput) (*listOutput, error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanReadClients(ac); err != nil {
		return nil, err
	}
	rows, err := s.Repo.Search(ctx, in.Q)
	if err != nil {
		return nil, usecase.Internal("REPO", "search failed", err)
	}
	out := make([]ClientResponse, 0, len(rows))
	for i := range rows {
		out = append(out, fromEntity(&rows[i]))
	}
	return &listOutput{Body: ClientListResponse{Clients: out, Total: len(out)}}, nil
}

type byIdentifierInput struct {
	Identifier string `path:"identifier"`
}

type getOutput struct {
	Body ClientResponse
}

func (s *State) byIdentifier(ctx context.Context, in *byIdentifierInput) (*getOutput, error) {
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
	return &getOutput{Body: fromEntity(c)}, nil
}

type getInput struct {
	ID string `path:"id"`
}

func (s *State) getByID(ctx context.Context, in *getInput) (*getOutput, error) {
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
	return &getOutput{Body: fromEntity(c)}, nil
}

type createInput struct {
	Body CreateClientRequest
}

type createOutput struct {
	Body apicommon.CreatedResponse
}

func (s *State) create(ctx context.Context, in *createInput) (*createOutput, error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanCreateClients(ac); err != nil {
		return nil, err
	}
	ec := usecase.NewExecutionContext(ac.PrincipalID)
	committed, err := operations.CreateClient(ctx, s.Repo, s.UoW, in.Body.toCommand(), ec)
	if err != nil {
		return nil, err
	}
	return &createOutput{Body: apicommon.CreatedResponse{ID: committed.Event().ClientID}}, nil
}

type updateInput struct {
	ID   string `path:"id"`
	Body UpdateClientRequest
}

type emptyOutput struct{}

func (s *State) update(ctx context.Context, in *updateInput) (*emptyOutput, error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanUpdateClients(ac); err != nil {
		return nil, err
	}
	ec := usecase.NewExecutionContext(ac.PrincipalID)
	if _, err := operations.UpdateClient(ctx, s.Repo, s.UoW, in.Body.toCommand(in.ID), ec); err != nil {
		return nil, err
	}
	return &emptyOutput{}, nil
}

type idInput struct {
	ID string `path:"id"`
}

func (s *State) activate(ctx context.Context, in *idInput) (*emptyOutput, error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanUpdateClients(ac); err != nil {
		return nil, err
	}
	ec := usecase.NewExecutionContext(ac.PrincipalID)
	if _, err := operations.ActivateClient(ctx, s.Repo, s.UoW, operations.ActivateCommand{ID: in.ID}, ec); err != nil {
		return nil, err
	}
	return &emptyOutput{}, nil
}

type suspendInput struct {
	ID   string `path:"id"`
	Body SuspendClientRequest
}

func (s *State) suspend(ctx context.Context, in *suspendInput) (*emptyOutput, error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanUpdateClients(ac); err != nil {
		return nil, err
	}
	ec := usecase.NewExecutionContext(ac.PrincipalID)
	if _, err := operations.SuspendClient(ctx, s.Repo, s.UoW, operations.SuspendCommand{ID: in.ID, Reason: in.Body.Reason}, ec); err != nil {
		return nil, err
	}
	return &emptyOutput{}, nil
}

type addNoteInput struct {
	ID   string `path:"id"`
	Body AddNoteRequest
}

func (s *State) addNote(ctx context.Context, in *addNoteInput) (*emptyOutput, error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanUpdateClients(ac); err != nil {
		return nil, err
	}
	ec := usecase.NewExecutionContext(ac.PrincipalID)
	if _, err := operations.AddNote(ctx, s.Repo, s.UoW, operations.AddNoteCommand{
		ClientID: in.ID,
		Category: in.Body.Category,
		Text:     in.Body.Text,
	}, ec); err != nil {
		return nil, err
	}
	return &emptyOutput{}, nil
}

func (s *State) delete(ctx context.Context, in *idInput) (*emptyOutput, error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanDeleteClients(ac); err != nil {
		return nil, err
	}
	ec := usecase.NewExecutionContext(ac.PrincipalID)
	if _, err := operations.DeleteClient(ctx, s.Repo, s.UoW, operations.DeleteCommand{ID: in.ID}, ec); err != nil {
		return nil, err
	}
	return &emptyOutput{}, nil
}

// ── deactivate (alias for delete with a reason) ──────────────────────────

type deactivateInput struct {
	ID   string `path:"id"`
	Body StatusChangeRequest
}

type statusChangeOutput struct {
	Body apicommon.StatusChangeResponse
}

func (s *State) deactivate(ctx context.Context, in *deactivateInput) (*statusChangeOutput, error) {
	ac := auth.FromContext(ctx)
	// Deactivate is a soft-delete — same permission as delete.
	if err := auth.CanDeleteClients(ac); err != nil {
		return nil, err
	}
	ec := usecase.NewExecutionContext(ac.PrincipalID)
	if _, err := operations.DeleteClient(ctx, s.Repo, s.UoW, operations.DeleteCommand{ID: in.ID}, ec); err != nil {
		return nil, err
	}
	return &statusChangeOutput{Body: apicommon.StatusChangeResponse{Message: "Client deactivated"}}, nil
}

// ── client→application linking ───────────────────────────────────────────

type clientApplicationsOutput struct {
	Body ClientApplicationsResponse
}

func (s *State) getApplications(ctx context.Context, in *idInput) (*clientApplicationsOutput, error) {
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

	out := make([]ClientApplicationResponse, 0, len(allApps))
	for i := range allApps {
		a := &allApps[i]
		out = append(out, ClientApplicationResponse{
			ID:               a.ID,
			Code:             a.Code,
			Name:             a.Name,
			Description:      a.Description,
			IconURL:          a.IconURL,
			Active:           a.Active,
			EnabledForClient: enabledByApp[a.ID],
		})
	}
	return &clientApplicationsOutput{Body: ClientApplicationsResponse{
		Applications: out,
		Total:        len(out),
	}}, nil
}

type updateApplicationsInput struct {
	ID   string `path:"id"`
	Body UpdateClientApplicationsRequest
}

func (s *State) updateApplications(ctx context.Context, in *updateApplicationsInput) (*emptyOutput, error) {
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
	if _, err := appops.UpdateClientApplications(ctx, s.Applications, s.Repo, s.ClientConfigs, s.UoW, cmd, ec); err != nil {
		return nil, err
	}
	return &emptyOutput{}, nil
}

type appLinkInput struct {
	ID            string `path:"id"`
	ApplicationID string `path:"applicationId"`
}

func (s *State) enableApplication(ctx context.Context, in *appLinkInput) (*emptyOutput, error) {
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
	if _, err := appops.EnableApplicationForClient(ctx, s.Applications, s.Repo, s.ClientConfigs, s.UoW, cmd, ec); err != nil {
		return nil, err
	}
	return &emptyOutput{}, nil
}

func (s *State) disableApplication(ctx context.Context, in *appLinkInput) (*emptyOutput, error) {
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
	if _, err := appops.DisableApplicationForClient(ctx, s.ClientConfigs, s.UoW, cmd, ec); err != nil {
		return nil, err
	}
	return &emptyOutput{}, nil
}
