//go:build integration

package api

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchjob"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apicommon"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/flowcatalyst-go/internal/tsid"
	"github.com/flowcatalyst/go-sdk/usecase"
)

const appSvcJobView = "platform:application-service:dispatch-job:view"

type jobFixture struct {
	s                *State
	pool             *pgxpool.Pool
	own, other       string
	ownID, otherID   string
	clientA, clientB string
	ownA, ownB       string
	ownPlatform      string
	foreign          string
	event            string // an event id that spawned both an own and a foreign job
	all              []string
}

func newJobFixture(t *testing.T) *jobFixture {
	t.Helper()
	pool := testpg.Pool(t)
	sfx := strings.ToLower(tsid.GenerateUntyped())[5:]
	f := &jobFixture{
		pool: pool,
		s:    &State{Repo: dispatchjob.NewRepository(pool), Apps: application.NewRepository(pool)},
		own:  "own" + sfx, other: "oth" + sfx,
		ownID: tsid.Generate(tsid.Application), otherID: tsid.Generate(tsid.Application),
		clientA: "clt_a" + sfx[:8], clientB: "clt_b" + sfx[:8],
		event: "evt" + sfx,
	}
	ctx := context.Background()
	for _, a := range [][2]string{{f.ownID, f.own}, {f.otherID, f.other}} {
		_, err := pool.Exec(ctx, `INSERT INTO app_applications (id, code, name) VALUES ($1, $2, $2)`, a[0], a[1])
		require.NoError(t, err)
	}
	base := time.Now().UTC().Add(-time.Hour)
	seed := func(n int, app string, client *string, eventID *string) string {
		id := "djrd" + sfx[:6] + string(rune('0'+n))
		code := app + ":sub:agg:happened"
		at := base.Add(time.Duration(n) * time.Second)
		_, err := pool.Exec(ctx, `INSERT INTO msg_dispatch_jobs (id, code, target_url, status, client_id, event_id, attempt_count, created_at, updated_at)
			VALUES ($1, $2, 'http://example.invalid/hook', 'COMPLETED', $3, $4, 1, $5, $5)`, id, code, client, eventID, at)
		require.NoError(t, err)
		_, err = pool.Exec(ctx, `INSERT INTO msg_dispatch_jobs_read
			(id, code, target_url, client_id, event_id, application, kind, protocol, mode, status, max_retries, attempt_count, created_at, updated_at)
			VALUES ($1, $2, 'http://example.invalid/hook', $3, $4, $5, 'EVENT', 'HTTP_WEBHOOK', 'IMMEDIATE', 'COMPLETED', 3, 1, $6, $6)`,
			id, code, client, eventID, app, at)
		require.NoError(t, err)
		f.all = append(f.all, id)
		return id
	}
	a, b, ev := f.clientA, f.clientB, f.event
	f.ownA = seed(1, f.own, &a, &ev)
	f.ownB = seed(2, f.own, &b, nil)
	f.ownPlatform = seed(3, f.own, nil, nil)
	f.foreign = seed(4, f.other, &a, &ev)
	return f
}

func (f *jobFixture) ctx(ac *auth.AuthContext) context.Context {
	return auth.WithContext(context.Background(), ac)
}

func (f *jobFixture) svc(perms ...string) *auth.AuthContext {
	return &auth.AuthContext{PrincipalID: "prn_svc", Scope: auth.ScopeAnchor, Permissions: perms, Applications: []string{f.ownID}}
}

func (f *jobFixture) list(ctx context.Context, in listInput) ([]string, error) {
	out, err := f.s.list(ctx, &in)
	if err != nil {
		return nil, err
	}
	var ids []string
	for _, j := range out.Body {
		if slices.Contains(f.all, j.ID) {
			ids = append(ids, j.ID)
		}
	}
	return ids, nil
}

// TestDispatchJobRead_ApplicationConfinement is the F20 confinement matrix for
// the dispatch-job reads: list, by id, attempts, by event.
func TestDispatchJobRead_ApplicationConfinement(t *testing.T) {
	f := newJobFixture(t)

	t.Run("own application only", func(t *testing.T) {
		ids, err := f.list(f.ctx(f.svc(appSvcJobView)), listInput{Size: 1000})
		require.NoError(t, err)
		assert.ElementsMatch(t, []string{f.ownA, f.ownB, f.ownPlatform}, ids)
	})

	t.Run("a caller filter cannot widen it", func(t *testing.T) {
		ctx := f.ctx(f.svc(appSvcJobView))
		for _, in := range []listInput{
			{Applications: f.other},
			{Code: f.other + ":sub:agg:happened"},
			{Codes: f.other + ":sub:agg:happened"},
		} {
			in.Size = 1000
			ids, err := f.list(ctx, in)
			require.NoError(t, err)
			assert.Empty(t, ids)
		}
	})

	t.Run("unbound caller refused everywhere", func(t *testing.T) {
		unbound := f.ctx(&auth.AuthContext{PrincipalID: "prn_unb", Scope: auth.ScopeAnchor, Permissions: []string{appSvcJobView}})
		_, err := f.list(unbound, listInput{})
		testpg.RequireUsecaseError(t, err, usecase.KindAuthorization, "FORBIDDEN")
		_, err = f.s.getByID(unbound, &apicommon.IDInput{ID: f.ownA})
		testpg.RequireUsecaseError(t, err, usecase.KindAuthorization, "FORBIDDEN")
		_, err = f.s.attempts(unbound, &apicommon.IDInput{ID: f.ownA})
		testpg.RequireUsecaseError(t, err, usecase.KindAuthorization, "FORBIDDEN")
		_, err = f.s.byEvent(unbound, &byEventInput{EventID: f.event})
		testpg.RequireUsecaseError(t, err, usecase.KindAuthorization, "FORBIDDEN")
	})

	t.Run("permission required", func(t *testing.T) {
		none := f.ctx(f.svc(appSvcEventViewForTest))
		_, err := f.list(none, listInput{})
		testpg.RequireUsecaseError(t, err, usecase.KindAuthorization, "PERMISSION_REQUIRED")
		_, err = f.s.getByID(none, &apicommon.IDInput{ID: f.ownA})
		testpg.RequireUsecaseError(t, err, usecase.KindAuthorization, "PERMISSION_REQUIRED")
		_, err = f.s.attempts(none, &apicommon.IDInput{ID: f.ownA})
		testpg.RequireUsecaseError(t, err, usecase.KindAuthorization, "PERMISSION_REQUIRED")
		_, err = f.s.byEvent(none, &byEventInput{EventID: f.event})
		testpg.RequireUsecaseError(t, err, usecase.KindAuthorization, "PERMISSION_REQUIRED")
	})

	t.Run("an all-applications caller sees every application", func(t *testing.T) {
		ac := f.svc(appSvcJobView)
		ac.Applications, ac.AllApplications = nil, true
		ids, err := f.list(f.ctx(ac), listInput{Size: 1000})
		require.NoError(t, err)
		assert.ElementsMatch(t, f.all, ids)
	})

	t.Run("the console's messaging view stays unconfined", func(t *testing.T) {
		ids, err := f.list(f.ctx(f.svc(viewPerm)), listInput{Size: 1000})
		require.NoError(t, err)
		assert.ElementsMatch(t, f.all, ids)
	})

	t.Run("client-scoped caller keeps its client reach on top", func(t *testing.T) {
		ac := &auth.AuthContext{
			PrincipalID: "prn_cl", Scope: auth.ScopeClient, Clients: []string{f.clientA},
			Permissions: []string{appSvcJobView}, Applications: []string{f.ownID},
		}
		ids, err := f.list(f.ctx(ac), listInput{Size: 1000})
		require.NoError(t, err)
		assert.ElementsMatch(t, []string{f.ownA, f.ownPlatform}, ids, "own application AND (client A or platform-scoped)")
		_, err = f.s.getByID(f.ctx(ac), &apicommon.IDInput{ID: f.ownB})
		testpg.RequireUsecaseError(t, err, usecase.KindAuthorization, "SCOPE_FORBIDDEN")
	})

	t.Run("by id and attempts: own, other, missing", func(t *testing.T) {
		ctx := f.ctx(f.svc(appSvcJobView))
		got, err := f.s.getByID(ctx, &apicommon.IDInput{ID: f.ownA})
		require.NoError(t, err)
		assert.Equal(t, f.ownA, got.Body.ID)
		_, err = f.s.attempts(ctx, &apicommon.IDInput{ID: f.ownA})
		require.NoError(t, err)

		_, errOther := f.s.getByID(ctx, &apicommon.IDInput{ID: f.foreign})
		_, errMissing := f.s.getByID(ctx, &apicommon.IDInput{ID: "djrd_missing_id"})
		testpg.RequireUsecaseError(t, errOther, usecase.KindNotFound, "DispatchJob_NOT_FOUND")
		testpg.RequireUsecaseError(t, errMissing, usecase.KindNotFound, "DispatchJob_NOT_FOUND")
		assert.Equal(t, strings.ReplaceAll(errMissing.Error(), "djrd_missing_id", "X"),
			strings.ReplaceAll(errOther.Error(), f.foreign, "X"))
		_, err = f.s.attempts(ctx, &apicommon.IDInput{ID: f.foreign})
		testpg.RequireUsecaseError(t, err, usecase.KindNotFound, "DispatchJob_NOT_FOUND")
	})

	t.Run("by event returns only the application's jobs", func(t *testing.T) {
		out, err := f.s.byEvent(f.ctx(f.svc(appSvcJobView)), &byEventInput{EventID: f.event})
		require.NoError(t, err)
		require.Len(t, out.Body, 1)
		assert.Equal(t, f.ownA, out.Body[0].ID)

		all := f.svc(appSvcJobView)
		all.Applications, all.AllApplications = nil, true
		out, err = f.s.byEvent(f.ctx(all), &byEventInput{EventID: f.event})
		require.NoError(t, err)
		assert.Len(t, out.Body, 2)
	})

	t.Run("pagination", func(t *testing.T) {
		ctx := f.ctx(f.svc(appSvcJobView))
		page := func(limit, offset int) []string {
			ids, err := f.list(ctx, listInput{Size: limit, Offset: offset})
			require.NoError(t, err)
			return ids
		}
		assert.Equal(t, []string{f.ownPlatform, f.ownB}, page(2, 0))
		assert.Equal(t, []string{f.ownA}, page(2, 2))
		assert.Empty(t, page(2, 3))
	})
}

const appSvcEventViewForTest = "platform:application-service:event:view"
