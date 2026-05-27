// Package api wires the HTTP routes for the principal subdomain via huma.
package api

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/client"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/role"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apicommon"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

// State bundles deps. Principal ops need cross-aggregate validation
// against roles, applications, and clients.
type State struct {
	Repo         *principal.Repository
	GrantRepo    *principal.ClientAccessGrantRepo
	Roles        *role.Repository
	Applications *application.Repository
	Clients      *client.Repository
	UoW          *usecasepgx.UnitOfWork
}

const tag = "principals"

// Register mounts the principal endpoints.
func Register(api huma.API, s *State) {
	huma.Register(api, huma.Operation{
		OperationID:   "listPrincipals",
		Method:        http.MethodGet,
		Path:          "/api/principals",
		Summary:       "List principals",
		Tags:          []string{tag},
		DefaultStatus: http.StatusOK,
	}, s.list)

	huma.Register(api, huma.Operation{
		OperationID:   "createPrincipal",
		Method:        http.MethodPost,
		Path:          "/api/principals",
		Summary:       "Create a principal",
		Tags:          []string{tag},
		DefaultStatus: http.StatusCreated,
	}, s.create)

	huma.Register(api, huma.Operation{
		OperationID:   "getPrincipal",
		Method:        http.MethodGet,
		Path:          "/api/principals/{id}",
		Summary:       "Get a principal by id",
		Tags:          []string{tag},
		DefaultStatus: http.StatusOK,
	}, s.getByID)

	huma.Register(api, huma.Operation{
		OperationID:   "updatePrincipal",
		Method:        http.MethodPut,
		Path:          "/api/principals/{id}",
		Summary:       "Update a principal",
		Tags:          []string{tag},
		DefaultStatus: http.StatusNoContent,
	}, s.update)

	huma.Register(api, huma.Operation{
		OperationID:   "activatePrincipal",
		Method:        http.MethodPost,
		Path:          "/api/principals/{id}/activate",
		Summary:       "Activate a principal",
		Tags:          []string{tag},
		DefaultStatus: http.StatusNoContent,
	}, s.activate)

	huma.Register(api, huma.Operation{
		OperationID:   "deactivatePrincipal",
		Method:        http.MethodPost,
		Path:          "/api/principals/{id}/deactivate",
		Summary:       "Deactivate a principal",
		Tags:          []string{tag},
		DefaultStatus: http.StatusNoContent,
	}, s.deactivate)

	huma.Register(api, huma.Operation{
		OperationID:   "resetPrincipalPassword",
		Method:        http.MethodPost,
		Path:          "/api/principals/{id}/reset-password",
		Summary:       "Reset a user's password",
		Tags:          []string{tag},
		DefaultStatus: http.StatusNoContent,
	}, s.resetPassword)

	huma.Register(api, huma.Operation{
		OperationID:   "deletePrincipal",
		Method:        http.MethodDelete,
		Path:          "/api/principals/{id}",
		Summary:       "Delete a principal",
		Tags:          []string{tag},
		DefaultStatus: http.StatusNoContent,
	}, s.delete)

	huma.Register(api, huma.Operation{
		OperationID:   "assignPrincipalRoles",
		Method:        http.MethodPut,
		Path:          "/api/principals/{id}/roles",
		Summary:       "Assign roles to a principal",
		Tags:          []string{tag},
		DefaultStatus: http.StatusNoContent,
	}, s.assignRoles)

	huma.Register(api, huma.Operation{
		OperationID:   "assignPrincipalApplicationAccess",
		Method:        http.MethodPut,
		Path:          "/api/principals/{id}/application-access",
		Summary:       "Assign application access to a principal",
		Tags:          []string{tag},
		DefaultStatus: http.StatusNoContent,
	}, s.assignApplicationAccess)

	huma.Register(api, huma.Operation{
		OperationID:   "listPrincipalClientAccess",
		Method:        http.MethodGet,
		Path:          "/api/principals/{id}/client-access",
		Summary:       "List client-access grants for a principal",
		Tags:          []string{tag},
		DefaultStatus: http.StatusOK,
	}, s.listClientAccess)

	huma.Register(api, huma.Operation{
		OperationID:   "grantPrincipalClientAccess",
		Method:        http.MethodPost,
		Path:          "/api/principals/{id}/client-access",
		Summary:       "Grant a client-access for a principal",
		Tags:          []string{tag},
		DefaultStatus: http.StatusNoContent,
	}, s.grantClientAccess)

	huma.Register(api, huma.Operation{
		OperationID:   "revokePrincipalClientAccess",
		Method:        http.MethodDelete,
		Path:          "/api/principals/{id}/client-access/{clientId}",
		Summary:       "Revoke a client-access grant",
		Tags:          []string{tag},
		DefaultStatus: http.StatusNoContent,
	}, s.revokeClientAccess)
}

type emptyInput struct{}

type listOutput struct {
	Body PrincipalListResponse
}

func (s *State) list(ctx context.Context, _ *emptyInput) (*listOutput, error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanReadPrincipals(ac); err != nil {
		return nil, err
	}
	rows, err := s.Repo.FindAll(ctx)
	if err != nil {
		return nil, usecase.Internal("REPO", "find_all failed", err)
	}
	out := make([]PrincipalResponse, 0, len(rows))
	for i := range rows {
		p := &rows[i]
		if p.ClientID == nil || ac.CanAccessClient(*p.ClientID) {
			out = append(out, fromEntity(p))
		}
	}
	return &listOutput{Body: PrincipalListResponse{Items: out}}, nil
}

type getInput struct {
	ID string `path:"id"`
}

type getOutput struct {
	Body PrincipalResponse
}

func (s *State) getByID(ctx context.Context, in *getInput) (*getOutput, error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanReadPrincipals(ac); err != nil {
		return nil, err
	}
	p, err := s.Repo.FindByID(ctx, in.ID)
	if err != nil {
		return nil, usecase.Internal("REPO", "find_by_id failed", err)
	}
	if p == nil {
		return nil, httperror.NotFound("Principal", in.ID)
	}
	if p.ClientID != nil && !ac.CanAccessClient(*p.ClientID) {
		return nil, httperror.Forbidden("No access to this principal")
	}
	return &getOutput{Body: fromEntity(p)}, nil
}

type createInput struct {
	Body CreatePrincipalRequest
}

type createOutput struct {
	Body apicommon.CreatedResponse
}

func (s *State) create(ctx context.Context, in *createInput) (*createOutput, error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanWritePrincipals(ac); err != nil {
		return nil, err
	}
	if in.Body.ClientID != nil && !ac.CanAccessClient(*in.Body.ClientID) {
		return nil, httperror.Forbidden("No access to client: " + *in.Body.ClientID)
	}
	ec := usecase.NewExecutionContext(ac.PrincipalID)
	committed, err := operations.CreateUser(ctx, s.Repo, s.UoW, in.Body.toCommand(), ec)
	if err != nil {
		return nil, err
	}
	return &createOutput{Body: apicommon.CreatedResponse{ID: committed.Event().UserID}}, nil
}

type updateInput struct {
	ID   string `path:"id"`
	Body UpdatePrincipalRequest
}

type emptyOutput struct{}

func (s *State) update(ctx context.Context, in *updateInput) (*emptyOutput, error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanWritePrincipals(ac); err != nil {
		return nil, err
	}
	ec := usecase.NewExecutionContext(ac.PrincipalID)
	if _, err := operations.UpdateUser(ctx, s.Repo, s.UoW, in.Body.toCommand(in.ID), ec); err != nil {
		return nil, err
	}
	return &emptyOutput{}, nil
}

type idInput struct {
	ID string `path:"id"`
}

func (s *State) activate(ctx context.Context, in *idInput) (*emptyOutput, error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanWritePrincipals(ac); err != nil {
		return nil, err
	}
	ec := usecase.NewExecutionContext(ac.PrincipalID)
	if _, err := operations.ActivateUser(ctx, s.Repo, s.UoW, operations.ActivateCommand{ID: in.ID}, ec); err != nil {
		return nil, err
	}
	return &emptyOutput{}, nil
}

func (s *State) deactivate(ctx context.Context, in *idInput) (*emptyOutput, error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanWritePrincipals(ac); err != nil {
		return nil, err
	}
	ec := usecase.NewExecutionContext(ac.PrincipalID)
	if _, err := operations.DeactivateUser(ctx, s.Repo, s.UoW, operations.DeactivateCommand{ID: in.ID}, ec); err != nil {
		return nil, err
	}
	return &emptyOutput{}, nil
}

type resetPasswordInput struct {
	ID   string `path:"id"`
	Body ResetPasswordRequest
}

func (s *State) resetPassword(ctx context.Context, in *resetPasswordInput) (*emptyOutput, error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanWritePrincipals(ac); err != nil {
		return nil, err
	}
	ec := usecase.NewExecutionContext(ac.PrincipalID)
	if _, err := operations.ResetPassword(ctx, s.Repo, s.UoW,
		operations.ResetPasswordCommand{ID: in.ID, NewPassword: in.Body.NewPassword}, ec); err != nil {
		return nil, err
	}
	return &emptyOutput{}, nil
}

func (s *State) delete(ctx context.Context, in *idInput) (*emptyOutput, error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanDeletePrincipals(ac); err != nil {
		return nil, err
	}
	ec := usecase.NewExecutionContext(ac.PrincipalID)
	if _, err := operations.DeleteUser(ctx, s.Repo, s.UoW, operations.DeleteCommand{ID: in.ID}, ec); err != nil {
		return nil, err
	}
	return &emptyOutput{}, nil
}

type assignRolesInput struct {
	ID   string `path:"id"`
	Body AssignPrincipalRolesRequest
}

func (s *State) assignRoles(ctx context.Context, in *assignRolesInput) (*emptyOutput, error) {
	ac := auth.FromContext(ctx)
	if err := auth.RequireAnchor(ac); err != nil {
		return nil, err
	}
	ec := usecase.NewExecutionContext(ac.PrincipalID)
	if _, err := operations.AssignRoles(ctx, s.Repo, s.Roles, s.UoW,
		operations.AssignRolesCommand{UserID: in.ID, Roles: in.Body.Roles}, ec); err != nil {
		return nil, err
	}
	return &emptyOutput{}, nil
}

type assignAppAccessInput struct {
	ID   string `path:"id"`
	Body AssignApplicationAccessRequest
}

func (s *State) assignApplicationAccess(ctx context.Context, in *assignAppAccessInput) (*emptyOutput, error) {
	ac := auth.FromContext(ctx)
	if err := auth.RequireAnchor(ac); err != nil {
		return nil, err
	}
	ec := usecase.NewExecutionContext(ac.PrincipalID)
	if _, err := operations.AssignApplicationAccess(ctx, s.Repo, s.Applications, s.UoW,
		operations.AssignApplicationAccessCommand{UserID: in.ID, ApplicationIDs: in.Body.ApplicationIDs}, ec); err != nil {
		return nil, err
	}
	return &emptyOutput{}, nil
}

type listClientAccessOutput struct {
	Body ClientAccessGrantListResponse
}

func (s *State) listClientAccess(ctx context.Context, in *idInput) (*listClientAccessOutput, error) {
	ac := auth.FromContext(ctx)
	if err := auth.RequireAnchor(ac); err != nil {
		return nil, err
	}
	grants, err := s.GrantRepo.FindByPrincipal(ctx, in.ID)
	if err != nil {
		return nil, usecase.Internal("REPO", "list grants failed", err)
	}
	out := make([]ClientAccessGrantResponse, 0, len(grants))
	for i := range grants {
		out = append(out, clientAccessGrantFromEntity(&grants[i]))
	}
	return &listClientAccessOutput{Body: ClientAccessGrantListResponse{Items: out}}, nil
}

type grantClientAccessInput struct {
	ID   string `path:"id"`
	Body GrantClientAccessRequest
}

func (s *State) grantClientAccess(ctx context.Context, in *grantClientAccessInput) (*emptyOutput, error) {
	ac := auth.FromContext(ctx)
	if err := auth.RequireAnchor(ac); err != nil {
		return nil, err
	}
	ec := usecase.NewExecutionContext(ac.PrincipalID)
	if _, err := operations.GrantClientAccess(ctx, s.Repo, s.Clients, s.GrantRepo, s.UoW,
		operations.GrantClientAccessCommand{UserID: in.ID, ClientID: in.Body.ClientID}, ec); err != nil {
		return nil, err
	}
	return &emptyOutput{}, nil
}

type revokeClientAccessInput struct {
	ID       string `path:"id"`
	ClientID string `path:"clientId"`
}

func (s *State) revokeClientAccess(ctx context.Context, in *revokeClientAccessInput) (*emptyOutput, error) {
	ac := auth.FromContext(ctx)
	if err := auth.RequireAnchor(ac); err != nil {
		return nil, err
	}
	ec := usecase.NewExecutionContext(ac.PrincipalID)
	if _, err := operations.RevokeClientAccess(ctx, s.Repo, s.GrantRepo, s.UoW,
		operations.RevokeClientAccessCommand{UserID: in.ID, ClientID: in.ClientID}, ec); err != nil {
		return nil, err
	}
	return &emptyOutput{}, nil
}
