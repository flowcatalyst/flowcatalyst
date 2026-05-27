// Package api wires the HTTP routes for the application subdomain via huma.
package api

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/client"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apicommon"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

// State bundles deps.
type State struct {
	Repo             *application.Repository
	ClientConfigRepo *application.ClientConfigRepo
	ClientRepo       *client.Repository
	Principals       *principal.Repository
	UoW              *usecasepgx.UnitOfWork
}

const tag = "applications"

// Register mounts the application endpoints.
func Register(api huma.API, s *State) {
	huma.Register(api, huma.Operation{
		OperationID:   "listApplications",
		Method:        http.MethodGet,
		Path:          "/api/applications",
		Summary:       "List applications",
		Tags:          []string{tag},
		DefaultStatus: http.StatusOK,
	}, s.list)

	huma.Register(api, huma.Operation{
		OperationID:   "createApplication",
		Method:        http.MethodPost,
		Path:          "/api/applications",
		Summary:       "Create an application",
		Tags:          []string{tag},
		DefaultStatus: http.StatusCreated,
	}, s.create)

	huma.Register(api, huma.Operation{
		OperationID:   "getApplicationByCode",
		Method:        http.MethodGet,
		Path:          "/api/applications/by-code/{code}",
		Summary:       "Get an application by code",
		Tags:          []string{tag},
		DefaultStatus: http.StatusOK,
	}, s.getByCode)

	huma.Register(api, huma.Operation{
		OperationID:   "getApplication",
		Method:        http.MethodGet,
		Path:          "/api/applications/{id}",
		Summary:       "Get an application by id",
		Tags:          []string{tag},
		DefaultStatus: http.StatusOK,
	}, s.getByID)

	huma.Register(api, huma.Operation{
		OperationID:   "updateApplication",
		Method:        http.MethodPut,
		Path:          "/api/applications/{id}",
		Summary:       "Update an application",
		Tags:          []string{tag},
		DefaultStatus: http.StatusNoContent,
	}, s.update)

	huma.Register(api, huma.Operation{
		OperationID:   "activateApplication",
		Method:        http.MethodPost,
		Path:          "/api/applications/{id}/activate",
		Summary:       "Activate an application",
		Tags:          []string{tag},
		DefaultStatus: http.StatusNoContent,
	}, s.activate)

	huma.Register(api, huma.Operation{
		OperationID:   "deactivateApplication",
		Method:        http.MethodPost,
		Path:          "/api/applications/{id}/deactivate",
		Summary:       "Deactivate an application",
		Tags:          []string{tag},
		DefaultStatus: http.StatusNoContent,
	}, s.deactivate)

	huma.Register(api, huma.Operation{
		OperationID:   "deleteApplication",
		Method:        http.MethodDelete,
		Path:          "/api/applications/{id}",
		Summary:       "Delete an application",
		Tags:          []string{tag},
		DefaultStatus: http.StatusNoContent,
	}, s.delete)

	huma.Register(api, huma.Operation{
		OperationID:   "attachApplicationServiceAccount",
		Method:        http.MethodPost,
		Path:          "/api/applications/{id}/service-account",
		Summary:       "Attach a service account to an application",
		Tags:          []string{tag},
		DefaultStatus: http.StatusNoContent,
	}, s.attachServiceAccount)

	huma.Register(api, huma.Operation{
		OperationID:   "listApplicationClientConfigs",
		Method:        http.MethodGet,
		Path:          "/api/applications/{id}/clients",
		Summary:       "List per-client configurations for an application",
		Tags:          []string{tag},
		DefaultStatus: http.StatusOK,
	}, s.listClientConfigs)

	huma.Register(api, huma.Operation{
		OperationID:   "enableApplicationForClient",
		Method:        http.MethodPost,
		Path:          "/api/applications/{id}/clients/{clientId}/enable",
		Summary:       "Enable an application for a client",
		Tags:          []string{tag},
		DefaultStatus: http.StatusNoContent,
	}, s.enableForClient)

	huma.Register(api, huma.Operation{
		OperationID:   "disableApplicationForClient",
		Method:        http.MethodPost,
		Path:          "/api/applications/{id}/clients/{clientId}/disable",
		Summary:       "Disable an application for a client",
		Tags:          []string{tag},
		DefaultStatus: http.StatusNoContent,
	}, s.disableForClient)
}

type listInput struct {
	Type   string `query:"type"`
	Active string `query:"active"`
}

type listOutput struct {
	Body ApplicationListResponse
}

func (s *State) list(ctx context.Context, in *listInput) (*listOutput, error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanReadApplications(ac); err != nil {
		return nil, err
	}
	var appType, active *string
	if in.Type != "" {
		appType = &in.Type
	}
	if in.Active != "" {
		active = &in.Active
	}
	rows, err := s.Repo.FindWithFilters(ctx, appType, active)
	if err != nil {
		return nil, usecase.Internal("REPO", "find_with_filters failed", err)
	}
	out := make([]ApplicationResponse, 0, len(rows))
	for i := range rows {
		out = append(out, fromEntity(&rows[i]))
	}
	return &listOutput{Body: ApplicationListResponse{Items: out}}, nil
}

type getInput struct {
	ID string `path:"id"`
}

type getOutput struct {
	Body ApplicationResponse
}

func (s *State) getByID(ctx context.Context, in *getInput) (*getOutput, error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanReadApplications(ac); err != nil {
		return nil, err
	}
	a, err := s.Repo.FindByID(ctx, in.ID)
	if err != nil {
		return nil, usecase.Internal("REPO", "find_by_id failed", err)
	}
	if a == nil {
		return nil, httperror.NotFound("Application", in.ID)
	}
	return &getOutput{Body: fromEntity(a)}, nil
}

type getByCodeInput struct {
	Code string `path:"code"`
}

func (s *State) getByCode(ctx context.Context, in *getByCodeInput) (*getOutput, error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanReadApplications(ac); err != nil {
		return nil, err
	}
	a, err := s.Repo.FindByCode(ctx, in.Code)
	if err != nil {
		return nil, usecase.Internal("REPO", "find_by_code failed", err)
	}
	if a == nil {
		return nil, httperror.NotFound("Application", in.Code)
	}
	return &getOutput{Body: fromEntity(a)}, nil
}

type createInput struct {
	Body CreateApplicationRequest
}

type createOutput struct {
	Body apicommon.CreatedResponse
}

func (s *State) create(ctx context.Context, in *createInput) (*createOutput, error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanWriteApplications(ac); err != nil {
		return nil, err
	}
	ec := usecase.NewExecutionContext(ac.PrincipalID)
	committed, err := operations.CreateApplication(ctx, s.Repo, s.UoW, in.Body.toCommand(), ec)
	if err != nil {
		return nil, err
	}
	return &createOutput{Body: apicommon.CreatedResponse{ID: committed.Event().ApplicationID}}, nil
}

type updateInput struct {
	ID   string `path:"id"`
	Body UpdateApplicationRequest
}

type emptyOutput struct{}

func (s *State) update(ctx context.Context, in *updateInput) (*emptyOutput, error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanWriteApplications(ac); err != nil {
		return nil, err
	}
	ec := usecase.NewExecutionContext(ac.PrincipalID)
	if _, err := operations.UpdateApplication(ctx, s.Repo, s.UoW, in.Body.toCommand(in.ID), ec); err != nil {
		return nil, err
	}
	return &emptyOutput{}, nil
}

type idInput struct {
	ID string `path:"id"`
}

func (s *State) activate(ctx context.Context, in *idInput) (*emptyOutput, error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanWriteApplications(ac); err != nil {
		return nil, err
	}
	ec := usecase.NewExecutionContext(ac.PrincipalID)
	if _, err := operations.ActivateApplication(ctx, s.Repo, s.UoW, operations.ActivateCommand{ID: in.ID}, ec); err != nil {
		return nil, err
	}
	return &emptyOutput{}, nil
}

func (s *State) deactivate(ctx context.Context, in *idInput) (*emptyOutput, error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanWriteApplications(ac); err != nil {
		return nil, err
	}
	ec := usecase.NewExecutionContext(ac.PrincipalID)
	if _, err := operations.DeactivateApplication(ctx, s.Repo, s.UoW, operations.DeactivateCommand{ID: in.ID}, ec); err != nil {
		return nil, err
	}
	return &emptyOutput{}, nil
}

func (s *State) delete(ctx context.Context, in *idInput) (*emptyOutput, error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanDeleteApplications(ac); err != nil {
		return nil, err
	}
	ec := usecase.NewExecutionContext(ac.PrincipalID)
	if _, err := operations.DeleteApplication(ctx, s.Repo, s.UoW, operations.DeleteCommand{ID: in.ID}, ec); err != nil {
		return nil, err
	}
	return &emptyOutput{}, nil
}

type attachSAInput struct {
	ID   string `path:"id"`
	Body AttachServiceAccountRequest
}

func (s *State) attachServiceAccount(ctx context.Context, in *attachSAInput) (*emptyOutput, error) {
	ac := auth.FromContext(ctx)
	if err := auth.RequireAnchor(ac); err != nil {
		return nil, err
	}
	ec := usecase.NewExecutionContext(ac.PrincipalID)
	if _, err := operations.AttachServiceAccount(ctx, s.Repo, s.Principals, s.UoW,
		operations.AttachServiceAccountCommand{
			ApplicationID:      in.ID,
			ServiceAccountID:   in.Body.ServiceAccountID,
			ServiceAccountCode: in.Body.ServiceAccountCode,
		}, ec); err != nil {
		return nil, err
	}
	return &emptyOutput{}, nil
}

type listConfigsInput struct {
	ID string `path:"id"`
}

type listConfigsOutput struct {
	Body ClientConfigListResponse
}

func (s *State) listClientConfigs(ctx context.Context, in *listConfigsInput) (*listConfigsOutput, error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanReadApplications(ac); err != nil {
		return nil, err
	}
	rows, err := s.ClientConfigRepo.FindByApplication(ctx, in.ID)
	if err != nil {
		return nil, usecase.Internal("REPO", "list client configs failed", err)
	}
	out := make([]ClientConfigResponse, 0, len(rows))
	for i := range rows {
		out = append(out, clientConfigFromEntity(&rows[i]))
	}
	return &listConfigsOutput{Body: ClientConfigListResponse{Items: out}}, nil
}

type clientToggleInput struct {
	ID       string `path:"id"`
	ClientID string `path:"clientId"`
}

func (s *State) enableForClient(ctx context.Context, in *clientToggleInput) (*emptyOutput, error) {
	ac := auth.FromContext(ctx)
	if err := auth.RequireAnchor(ac); err != nil {
		return nil, err
	}
	ec := usecase.NewExecutionContext(ac.PrincipalID)
	if _, err := operations.EnableApplicationForClient(ctx, s.Repo, s.ClientRepo, s.ClientConfigRepo, s.UoW,
		operations.EnableForClientCommand{ApplicationID: in.ID, ClientID: in.ClientID}, ec); err != nil {
		return nil, err
	}
	return &emptyOutput{}, nil
}

func (s *State) disableForClient(ctx context.Context, in *clientToggleInput) (*emptyOutput, error) {
	ac := auth.FromContext(ctx)
	if err := auth.RequireAnchor(ac); err != nil {
		return nil, err
	}
	ec := usecase.NewExecutionContext(ac.PrincipalID)
	if _, err := operations.DisableApplicationForClient(ctx, s.ClientConfigRepo, s.UoW,
		operations.DisableForClientCommand{ApplicationID: in.ID, ClientID: in.ClientID}, ec); err != nil {
		return nil, err
	}
	return &emptyOutput{}, nil
}
