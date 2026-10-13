package api

import (
	"context"
	"testing"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apicommon"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecase"
)

// gateRefuses runs call as a principal holding only perm and reports whether
// the controller's permission check refused it. A call that gets past the
// check reaches the (nil) unit of work and panics, which counts as admitted.
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
// (bug 13 in docs/ui/README.md; docs/platform-bug-fixes.md). Activate and
// deactivate are updates (there is no lifecycle permission for applications).
func TestWriteGatesCheckTheOperationsPermission(t *testing.T) {
	s := &State{}
	const (
		create = "platform:admin:application:create"
		update = "platform:admin:application:update"
		del    = "platform:admin:application:delete"
	)
	ops := map[string]struct {
		need string
		call func(ctx context.Context) error
	}{
		"create": {create, func(ctx context.Context) error {
			_, err := s.create(ctx, &apicommon.In[CreateApplicationRequest]{})
			return err
		}},
		"update": {update, func(ctx context.Context) error {
			_, err := s.update(ctx, &updateInput{ID: "app_x"})
			return err
		}},
		"activate": {update, func(ctx context.Context) error {
			_, err := s.activate(ctx, &apicommon.IDInput{ID: "app_x"})
			return err
		}},
		"deactivate": {update, func(ctx context.Context) error {
			_, err := s.deactivate(ctx, &apicommon.IDInput{ID: "app_x"})
			return err
		}},
		"delete": {del, func(ctx context.Context) error {
			_, err := s.delete(ctx, &apicommon.IDInput{ID: "app_x"})
			return err
		}},
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
