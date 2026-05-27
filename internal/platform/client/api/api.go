// Package api wires HTTP routes for the client (tenant) subdomain via huma.
package api

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/client"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/client/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apicommon"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

// State bundles deps.
type State struct {
	Repo *client.Repository
	UoW  *usecasepgx.UnitOfWork
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
	return &listOutput{Body: ClientListResponse{Items: out}}, nil
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
	return &listOutput{Body: ClientListResponse{Items: out}}, nil
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
