package api

import (
	"context"
	"testing"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apicommon"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/go-sdk/usecase"
)

// gateRefuses runs call as an anchor principal holding only perm and reports
// whether the controller's permission check refused it. A call that gets past
// the check reaches a (nil) repository or unit of work and panics, which
// counts as admitted.
func gateRefuses(t *testing.T, perm string, call func(ctx context.Context) error) (refused bool) {
	t.Helper()
	ctx := auth.WithContext(context.Background(), &auth.AuthContext{
		PrincipalID: "prn_gatetest00001",
		Scope:       auth.ScopeAnchor,
		Permissions: []string{perm},
	})
	defer func() {
		if recover() != nil {
			refused = false
		}
	}()
	err := call(ctx)
	ue := usecase.AsError(err)
	return ue != nil && ue.Kind == usecase.KindAuthorization && ue.Code == "PERMISSION_REQUIRED"
}

// Each user write needs the permission for its operation, not any of
// create/update/delete (docs/platform-bug-fixes.md): creating users needs
// create; every change to an existing user (edit, activate, deactivate,
// passwords, application access, someone else's developer credential) needs
// update; a change to the user's roles (replace, add, remove) needs
// assign-roles, not update (owner ruling 2026-10-11); delete needs delete.
func TestWriteGatesCheckTheOperationsPermission(t *testing.T) {
	s := &State{}
	const (
		create = "platform:iam:user:create"
		update = "platform:iam:user:update"
		del    = "platform:iam:user:delete"
		assign = "platform:iam:user:assign-roles"
	)
	client := "clt_x"
	id := &apicommon.IDInput{ID: "prn_target00001"}
	ops := map[string]struct {
		need string
		call func(ctx context.Context) error
	}{
		"create": {create, func(ctx context.Context) error {
			_, err := s.create(ctx, &apicommon.In[CreatePrincipalRequest]{Body: CreatePrincipalRequest{Scope: "CLIENT", ClientID: &client}})
			return err
		}},
		"bulk import": {create, func(ctx context.Context) error {
			_, err := s.bulkImport(ctx, &apicommon.In[BulkImportRequest]{Body: BulkImportRequest{ClientID: client}})
			return err
		}},
		"update": {update, func(ctx context.Context) error {
			_, err := s.update(ctx, &updateInput{ID: "prn_target00001"})
			return err
		}},
		"activate":   {update, func(ctx context.Context) error { _, err := s.activate(ctx, id); return err }},
		"deactivate": {update, func(ctx context.Context) error { _, err := s.deactivate(ctx, id); return err }},
		"reset password": {update, func(ctx context.Context) error {
			_, err := s.resetPassword(ctx, &resetPasswordInput{ID: "prn_target00001"})
			return err
		}},
		"send password reset": {update, func(ctx context.Context) error {
			_, err := s.sendPasswordReset(ctx, &sendPasswordResetInput{ID: "prn_target00001"})
			return err
		}},
		"assign roles": {assign, func(ctx context.Context) error {
			_, err := s.assignRoles(ctx, &assignRolesInput{ID: "prn_target00001"})
			return err
		}},
		"add role": {assign, func(ctx context.Context) error {
			_, err := s.addRole(ctx, &addRoleInput{ID: "prn_target00001"})
			return err
		}},
		"remove role": {assign, func(ctx context.Context) error {
			_, err := s.removeRole(ctx, &removeRoleInput{ID: "prn_target00001", Role: "app:r"})
			return err
		}},
		"application access": {update, func(ctx context.Context) error {
			_, err := s.assignApplicationAccess(ctx, &assignAppAccessInput{ID: "prn_target00001"})
			return err
		}},
		"someone else's developer credential": {update, func(ctx context.Context) error {
			_, err := s.setDeveloperCredential(ctx, id)
			return err
		}},
		"delete": {del, func(ctx context.Context) error { _, err := s.delete(ctx, id); return err }},
	}
	for name, op := range ops {
		for _, held := range []string{create, update, del, assign} {
			refused := gateRefuses(t, held, op.call)
			if want := held != op.need; refused != want {
				t.Errorf("%s with only %s: refused=%v, want %v", name, held, refused, want)
			}
		}
	}
}
