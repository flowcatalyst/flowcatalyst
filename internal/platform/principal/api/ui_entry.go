package api

import (
	"context"

	"github.com/flowcatalyst/flowcatalyst-go/internal/ids"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apicommon"
)

// Entry points for the server-rendered UI (internal/ui/pages/users*.go). Each
// runs the route's handler body unchanged, with the caller on ctx, so the
// permission checks, the client scoping, the role and application bounds of a
// client administrator, the role ceiling and the last-administrator guard are
// the API's own and the UI keeps no second copy of them. They add no logic.

// ListQuery is GET /api/principals's query.
type ListQuery = listInput

// ListPrincipals is GET /api/principals.
func (s *State) ListPrincipals(ctx context.Context, q ListQuery) (PrincipalListResponse, error) {
	out, err := s.list(ctx, &q)
	if err != nil {
		return PrincipalListResponse{}, err
	}
	return out.Body, nil
}

// GetPrincipal is GET /api/principals/{id}.
func (s *State) GetPrincipal(ctx context.Context, id string) (PrincipalResponse, error) {
	out, err := s.getByID(ctx, &apicommon.IDInput{ID: id})
	if err != nil {
		return PrincipalResponse{}, err
	}
	return out.Body, nil
}

// CheckEmailDomain is GET /api/principals/check-email-domain.
func (s *State) CheckEmailDomain(ctx context.Context, email string) (CheckEmailDomainResponse, error) {
	out, err := s.checkEmailDomain(ctx, &checkEmailDomainInput{Email: email})
	if err != nil {
		return CheckEmailDomainResponse{}, err
	}
	return out.Body, nil
}

// CreateDomainUser is POST /api/principals/users: the scope is the caller's
// request, backed by the email domain's setup.
func (s *State) CreateDomainUser(ctx context.Context, body CreateUserRequest) (PrincipalResponse, error) {
	out, err := s.createUser(ctx, &apicommon.In[CreateUserRequest]{Body: body})
	if err != nil {
		return PrincipalResponse{}, err
	}
	return out.Body, nil
}

// CreatePrincipal is POST /api/principals.
func (s *State) CreatePrincipal(ctx context.Context, body CreatePrincipalRequest) (CreatePrincipalResponse, error) {
	out, err := s.create(ctx, &apicommon.In[CreatePrincipalRequest]{Body: body})
	if err != nil {
		return CreatePrincipalResponse{}, err
	}
	return out.Body, nil
}

// UpdatePrincipal is PUT /api/principals/{id}.
func (s *State) UpdatePrincipal(ctx context.Context, id string, body UpdatePrincipalRequest) (PrincipalResponse, error) {
	out, err := s.update(ctx, &updateInput{ID: ids.PrincipalID(id), Body: body})
	if err != nil {
		return PrincipalResponse{}, err
	}
	return out.Body, nil
}

// ActivatePrincipal is POST /api/principals/{id}/activate.
func (s *State) ActivatePrincipal(ctx context.Context, id string) error {
	_, err := s.activate(ctx, &apicommon.IDInput{ID: id})
	return err
}

// DeactivatePrincipal is POST /api/principals/{id}/deactivate.
func (s *State) DeactivatePrincipal(ctx context.Context, id string) error {
	_, err := s.deactivate(ctx, &apicommon.IDInput{ID: id})
	return err
}

// ResetPrincipalPassword is POST /api/principals/{id}/reset-password.
func (s *State) ResetPrincipalPassword(ctx context.Context, id string, body ResetPasswordRequest) error {
	_, err := s.resetPassword(ctx, &resetPasswordInput{ID: ids.PrincipalID(id), Body: body})
	return err
}

// SendPrincipalPasswordReset is POST /api/principals/{id}/send-password-reset
// without a body (the 2FA reset is its own action).
func (s *State) SendPrincipalPasswordReset(ctx context.Context, id string) error {
	_, err := s.sendPasswordReset(ctx, &sendPasswordResetInput{ID: ids.PrincipalID(id)})
	return err
}

// ResetPrincipalTwoFactor is POST /api/principals/{id}/reset-2fa.
func (s *State) ResetPrincipalTwoFactor(ctx context.Context, id string) error {
	_, err := s.resetTwoFactor(ctx, &apicommon.IDInput{ID: id})
	return err
}

// DeletePrincipal is DELETE /api/principals/{id}.
func (s *State) DeletePrincipal(ctx context.Context, id string) error {
	_, err := s.delete(ctx, &apicommon.IDInput{ID: id})
	return err
}

// ListPrincipalRoles is GET /api/principals/{id}/roles.
func (s *State) ListPrincipalRoles(ctx context.Context, id string) ([]PrincipalRoleAssignmentDTO, error) {
	out, err := s.listRoles(ctx, &apicommon.IDInput{ID: id})
	if err != nil {
		return nil, err
	}
	return out.Body.Roles, nil
}

// AssignPrincipalRoles is PUT /api/principals/{id}/roles: the full role set.
func (s *State) AssignPrincipalRoles(ctx context.Context, id string, roles []string) (RolesAssignedResponse, error) {
	out, err := s.assignRoles(ctx, &assignRolesInput{ID: ids.PrincipalID(id), Body: AssignPrincipalRolesRequest{Roles: roles}})
	if err != nil {
		return RolesAssignedResponse{}, err
	}
	return out.Body, nil
}

// ListPrincipalApplicationAccess is GET /api/principals/{id}/application-access.
func (s *State) ListPrincipalApplicationAccess(ctx context.Context, id string) (ApplicationAccessListResponse, error) {
	out, err := s.listApplicationAccess(ctx, &apicommon.IDInput{ID: id})
	if err != nil {
		return ApplicationAccessListResponse{}, err
	}
	return out.Body, nil
}

// ListPrincipalAvailableApplications is GET /api/principals/{id}/available-applications.
func (s *State) ListPrincipalAvailableApplications(ctx context.Context, id string) ([]PrincipalAvailableApplication, error) {
	out, err := s.listAvailableApplications(ctx, &apicommon.IDInput{ID: id})
	if err != nil {
		return nil, err
	}
	return out.Body.Applications, nil
}

// AssignPrincipalApplicationAccess is PUT /api/principals/{id}/application-access.
func (s *State) AssignPrincipalApplicationAccess(ctx context.Context, id string, body AssignApplicationAccessRequest) (SetApplicationAccessResponse, error) {
	out, err := s.assignApplicationAccess(ctx, &assignAppAccessInput{ID: ids.PrincipalID(id), Body: body})
	if err != nil {
		return SetApplicationAccessResponse{}, err
	}
	return out.Body, nil
}

// ListPrincipalClientAccess is GET /api/principals/{id}/client-access.
func (s *State) ListPrincipalClientAccess(ctx context.Context, id string) ([]ClientAccessGrantResponse, error) {
	out, err := s.listClientAccess(ctx, &apicommon.IDInput{ID: id})
	if err != nil {
		return nil, err
	}
	return out.Body.Grants, nil
}

// GrantPrincipalClientAccess is POST /api/principals/{id}/client-access.
func (s *State) GrantPrincipalClientAccess(ctx context.Context, id, clientID string) error {
	_, err := s.grantClientAccess(ctx, &grantClientAccessInput{ID: ids.PrincipalID(id), Body: GrantClientAccessRequest{ClientID: clientID}})
	return err
}

// RevokePrincipalClientAccess is DELETE /api/principals/{id}/client-access/{clientId}.
func (s *State) RevokePrincipalClientAccess(ctx context.Context, id, clientID string) error {
	_, err := s.revokeClientAccess(ctx, &revokeClientAccessInput{ID: ids.PrincipalID(id), ClientID: ids.ClientID(clientID)})
	return err
}

// SetPrincipalClientAssociation is PUT /api/principals/{id}/client-association.
func (s *State) SetPrincipalClientAssociation(ctx context.Context, id string, body ClientAssociationRequest) (PrincipalResponse, error) {
	out, err := s.setClientAssociation(ctx, &setClientAssociationInput{ID: ids.PrincipalID(id), Body: body})
	if err != nil {
		return PrincipalResponse{}, err
	}
	return out.Body, nil
}

// BulkImportPrincipals is POST /api/principals/bulk-import.
func (s *State) BulkImportPrincipals(ctx context.Context, body BulkImportRequest) (BulkImportResponse, error) {
	out, err := s.bulkImport(ctx, &apicommon.In[BulkImportRequest]{Body: body})
	if err != nil {
		return BulkImportResponse{}, err
	}
	return out.Body, nil
}
