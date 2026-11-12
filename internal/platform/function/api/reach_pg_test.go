//go:build integration

package api

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/function"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/function/artifact"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apicommon"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/go-sdk/usecase"
	"github.com/flowcatalyst/go-sdk/usecasepgx"
)

// reachFixture is two applications whose functions belong to one client, a
// caller confined to application A that reaches that client, and an operator.
type reachFixture struct {
	t        *testing.T
	s        *State
	fnA, fnB *function.Function
	confined context.Context
	operator context.Context
	prefixA  string
	prefixB  string
}

func newReachFixture(t *testing.T, seed string) *reachFixture {
	t.Helper()
	pool := testpg.Pool(t)
	repo := function.NewRepository(pool)
	store, err := artifact.NewFileStore(t.TempDir())
	require.NoError(t, err)
	client := "cli_apirch" + seed
	mk := func(letter string) *function.Function {
		appID, code := "app_apirch"+letter+seed, "apirch"+letter+seed
		bg := context.Background()
		_, err := pool.Exec(bg, `INSERT INTO app_applications (id, code, name) VALUES ($1, $2, $2) ON CONFLICT (id) DO NOTHING`, appID, code)
		require.NoError(t, err)
		fn := function.New(appID, code, "fn")
		fn.ClientID = &client
		tx, err := pool.Begin(bg)
		require.NoError(t, err)
		require.NoError(t, repo.Persist(bg, fn, usecasepgx.WrapTxForBootstrap(tx)))
		require.NoError(t, tx.Commit(bg))
		t.Cleanup(func() {
			_, _ = pool.Exec(bg, `DELETE FROM fng_functions WHERE id = $1`, fn.ID)
			_, _ = pool.Exec(bg, `DELETE FROM app_applications WHERE id = $1`, appID)
		})
		return fn
	}
	f := &reachFixture{t: t, s: &State{Repo: repo, UoW: testpg.NewUoW(t), Artifacts: store}}
	f.fnA, f.fnB = mk("a"), mk("b")
	f.prefixA, f.prefixB = "apircha"+seed, "apirchb"+seed
	f.confined = testpg.WithAuth(context.Background(), &auth.AuthContext{
		PrincipalID: "prn_apirchconf" + seed, Scope: auth.ScopeClient, Clients: []string{client},
		AllApplications: false, Applications: []string{f.fnA.ApplicationID},
		Permissions: []string{"platform:function:*:*"},
	})
	f.operator = testpg.WithAuth(context.Background(), &auth.AuthContext{
		PrincipalID: "prn_apirchop" + seed, Scope: auth.ScopeAnchor, AllApplications: true,
		Permissions: []string{"platform:function:*:*"},
	})
	return f
}

func (f *reachFixture) listAddresses(ctx context.Context, prefix string) []string {
	f.t.Helper()
	out, err := f.s.list(ctx, &listInput{AddressPrefix: prefix})
	require.NoError(f.t, err)
	var addrs []string
	for _, r := range out.Body.Data {
		addrs = append(addrs, r.Address)
	}
	return addrs
}

// T3 (API): by-id get, trace log, artifact upload refuse a function of
// another application (403, the routes' existing convention for a reachable
// but forbidden row); list hides it.
func TestFunctionAPI_ApplicationReach(t *testing.T) {
	t.Parallel()
	f := newReachFixture(t, "r3one")

	forbidden := func(err error) {
		t.Helper()
		testpg.RequireUsecaseError(t, err, usecase.KindAuthorization, "SCOPE_FORBIDDEN")
	}

	t.Run("get-by-id", func(t *testing.T) {
		_, err := f.s.getByID(f.confined, &apicommon.IDInput{ID: f.fnB.ID})
		forbidden(err)
		out, err := f.s.getByID(f.confined, &apicommon.IDInput{ID: f.fnA.ID})
		require.NoError(t, err, "its own application's function")
		assert.Equal(t, f.fnA.ID, out.Body.ID)
		_, err = f.s.getByID(f.operator, &apicommon.IDInput{ID: f.fnB.ID})
		require.NoError(t, err, "an all-applications caller reads any application")
	})
	t.Run("trace-log", func(t *testing.T) {
		_, err := f.s.getTraceLog(f.confined, &traceLogInput{ID: f.fnB.ID})
		forbidden(err)
		_, err = f.s.getTraceLog(f.confined, &traceLogInput{ID: f.fnA.ID})
		require.NoError(t, err, "its own application's function")
	})
	t.Run("artifact-upload", func(t *testing.T) {
		_, err := f.s.putArtifact(f.confined, &putArtifactInput{ID: f.fnB.ID, Digest: "00000000000000000000000000000000000000000000000000000000000000aa", RawBody: []byte("x")})
		forbidden(err)
	})
	t.Run("list", func(t *testing.T) {
		assert.Empty(t, f.listAddresses(f.confined, f.prefixB), "B's function is absent for the confined caller")
		assert.Equal(t, []string{f.prefixA + ".fn"}, f.listAddresses(f.confined, f.prefixA), "A's function is listed")
		assert.Equal(t, []string{f.prefixB + ".fn"}, f.listAddresses(f.operator, f.prefixB), "the operator sees B's function")
	})
}
