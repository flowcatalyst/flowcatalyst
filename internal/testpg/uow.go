//go:build integration

package testpg

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/platformsink"
	"github.com/flowcatalyst/flowcatalyst-go/internal/tsid"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecasepgx"
)

// NewUoW returns a UnitOfWork wired to the shared embedded-PG pool and
// the production platform sink (events → msg_events, audit → aud_logs),
// exactly as the server wires it. Operation integration tests pass this
// into the use case under test.
func NewUoW(t *testing.T) *usecasepgx.UnitOfWork {
	t.Helper()
	return usecasepgx.New(Pool(t), platformsink.New())
}

// TestEC returns an ExecutionContext for an arbitrary test principal.
// msg_events / aud_logs carry the principal id without a foreign key,
// so no iam_principals row is needed.
func TestEC() usecase.ExecutionContext {
	return usecase.NewExecutionContext("prn_optestrunner1")
}

// AnchorCtx returns a context carrying an anchor-scoped AuthContext for the
// same principal as TestEC. Operations whose Authorize phase reads
// auth.FromContext(ctx) (every write that goes through usecaseop.Run) need an
// authorized context; this is the test-side analogue of the auth middleware.
// Use AnchorCtxFor / WithAuth when a test needs a narrower principal.
func AnchorCtx() context.Context { return AnchorCtxFor(context.Background()) }

// AnchorCtxFor attaches an anchor-scoped AuthContext to the given context.
//
// It carries the super-admin wildcard because it stands in for a full platform
// administrator, and scope alone grants nothing: anchor scope is reach — which
// tenants a principal may act for — while every permission gate reads the
// permissions its roles granted. A test that wants to exercise a gate builds
// its own narrower principal with WithAuth.
func AnchorCtxFor(ctx context.Context) context.Context {
	return WithAuth(ctx, &auth.AuthContext{
		PrincipalID: "prn_optestrunner1",
		Scope:       auth.ScopeAnchor,
		Permissions: []string{"platform:*:*:*"},
	})
}

// WithAuth attaches an arbitrary AuthContext — for tests that exercise the
// authorization phase with a non-anchor or unauthorized principal.
func WithAuth(ctx context.Context, ac *auth.AuthContext) context.Context {
	return auth.WithContext(ctx, ac)
}

// RequireUsecaseError asserts err is a *usecase.Error with the given
// kind and code. httperror.NotFound also lands here (it returns a
// *usecase.Error with Code "<Resource>_NOT_FOUND").
func RequireUsecaseError(t *testing.T, err error, kind usecase.Kind, code string) {
	t.Helper()
	ue := usecase.AsError(err)
	require.NotNil(t, ue, "expected *usecase.Error, got: %v", err)
	assert.Equal(t, kind, ue.Kind, "error kind (full err: %v)", err)
	assert.Equal(t, code, ue.Code, "error code (full err: %v)", err)
}

// EnsureApplication makes sure an application row with code exists and returns
// its id. A role may only be created for an application that exists (F18), so
// tests that create roles under a hand-picked code call this first.
func EnsureApplication(t *testing.T, code string) string {
	t.Helper()
	ctx := context.Background()
	pool := Pool(t)
	var id string
	if err := pool.QueryRow(ctx, `SELECT id FROM app_applications WHERE code = $1`, code).Scan(&id); err == nil {
		return id
	}
	id = tsid.Generate(tsid.Application)
	if _, err := pool.Exec(ctx,
		`INSERT INTO app_applications (id, code, name) VALUES ($1, $2, $2) ON CONFLICT (code) DO NOTHING`,
		id, code); err != nil {
		t.Fatalf("testpg: ensure application %q: %v", code, err)
	}
	if err := pool.QueryRow(ctx, `SELECT id FROM app_applications WHERE code = $1`, code).Scan(&id); err != nil {
		t.Fatalf("testpg: look up application %q: %v", code, err)
	}
	return id
}
