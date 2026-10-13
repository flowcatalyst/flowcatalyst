package api

import (
	"context"
	"testing"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apicommon"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecase"
)

// gateRefuses runs call as an anchor principal holding only perm and reports
// whether the controller's permission check refused it. A call that gets past
// the check reaches the (nil) unit of work and panics, which counts as
// admitted.
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

// Each write needs the permission for its operation, not any of the three
// (docs/platform-bug-fixes.md): create needs create; update, deactivate,
// assigning roles, both regenerations and the token mint need update; delete
// needs delete. The last four checked anchor scope only.
func TestWriteGatesCheckTheOperationsPermission(t *testing.T) {
	s := &State{}
	const (
		create = "platform:iam:service-account:create"
		update = "platform:iam:service-account:update"
		del    = "platform:iam:service-account:delete"
	)
	id := &apicommon.IDInput{ID: "sac_x"}
	ops := map[string]struct {
		need string
		call func(ctx context.Context) error
	}{
		"create": {create, func(ctx context.Context) error {
			_, err := s.create(ctx, &apicommon.In[CreateServiceAccountRequest]{Body: CreateServiceAccountRequest{Code: "x", Name: "X"}})
			return err
		}},
		"update": {update, func(ctx context.Context) error {
			_, err := s.update(ctx, &updateInput{ID: "sac_x"})
			return err
		}},
		"deactivate": {update, func(ctx context.Context) error { _, err := s.deactivate(ctx, id); return err }},
		"delete":     {del, func(ctx context.Context) error { _, err := s.delete(ctx, id); return err }},
		"assign roles": {update, func(ctx context.Context) error {
			_, err := s.assignRoles(ctx, &assignRolesInput{ID: "sac_x"})
			return err
		}},
		"regenerate token":  {update, func(ctx context.Context) error { _, err := s.regenerateAuthToken(ctx, id); return err }},
		"regenerate secret": {update, func(ctx context.Context) error { _, err := s.regenerateSigningSecret(ctx, id); return err }},
		"mint token":        {update, func(ctx context.Context) error { _, err := s.mintToken(ctx, id); return err }},
	}
	for name, op := range ops {
		for _, held := range []string{create, update, del} {
			refused := gateRefuses(t, held, op.call)
			if want := held != op.need; refused != want {
				t.Errorf("%s with only %s: refused=%v, want %v", name, held, refused, want)
			}
		}
	}
}
