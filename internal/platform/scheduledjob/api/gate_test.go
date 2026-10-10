package api

import (
	"context"
	"testing"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apicommon"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
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

// Each job write needs the permission for its operation, not any of the
// three (bug 17 in docs/ui/README.md; docs/platform-bug-fixes.md): create
// needs create; update, pause, resume and archive need update; delete needs
// delete.
func TestWriteGatesCheckTheOperationsPermission(t *testing.T) {
	s := &State{}
	const (
		create = "platform:messaging:scheduled-job:create"
		update = "platform:messaging:scheduled-job:update"
		del    = "platform:messaging:scheduled-job:delete"
	)
	id := &apicommon.IDInput{ID: "sjb_x"}
	ops := map[string]struct {
		need string
		call func(ctx context.Context) error
	}{
		"create": {create, func(ctx context.Context) error {
			_, err := s.create(ctx, &apicommon.In[CreateScheduledJobRequest]{})
			return err
		}},
		"update": {update, func(ctx context.Context) error {
			_, err := s.update(ctx, &updateInput{ID: "sjb_x"})
			return err
		}},
		"pause":   {update, func(ctx context.Context) error { _, err := s.pause(ctx, id); return err }},
		"resume":  {update, func(ctx context.Context) error { _, err := s.resume(ctx, id); return err }},
		"archive": {update, func(ctx context.Context) error { _, err := s.archive(ctx, id); return err }},
		"delete":  {del, func(ctx context.Context) error { _, err := s.delete(ctx, id); return err }},
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
