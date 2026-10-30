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
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/event"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apicommon"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/flowcatalyst-go/internal/tsid"
	"github.com/flowcatalyst/go-sdk/usecase"
)

const appSvcEventView = "platform:application-service:event:view"

type readFixture struct {
	s                *State
	pool             *pgxpool.Pool
	ownID, otherID   string
	own, other       string
	clientA, clientB string
	ownA, ownB       string // own-application event ids at client A / client B
	ownPlatform      string // own-application platform-scoped event
	foreign          string // another application's event
	all              []string
}

func newReadFixture(t *testing.T) *readFixture {
	t.Helper()
	pool := testpg.Pool(t)
	sfx := strings.ToLower(tsid.GenerateUntyped())[5:]
	f := &readFixture{
		s: &State{Repo: event.NewRepository(pool), Apps: application.NewRepository(pool)}, pool: pool,
		own: "own" + sfx, other: "oth" + sfx,
		ownID: tsid.Generate(tsid.Application), otherID: tsid.Generate(tsid.Application),
		clientA: "clt_a" + sfx[:8], clientB: "clt_b" + sfx[:8],
	}
	for _, a := range [][2]string{{f.ownID, f.own}, {f.otherID, f.other}} {
		_, err := pool.Exec(context.Background(), `INSERT INTO app_applications (id, code, name) VALUES ($1, $2, $2)`, a[0], a[1])
		require.NoError(t, err)
	}
	seed := func(id, app string, client *string, at time.Time) string {
		_, err := pool.Exec(context.Background(),
			`INSERT INTO msg_events_read (id, type, source, time, client_id, application, created_at)
			 VALUES ($1, $2, 'test://appread', NOW(), $3, $4, $5)`,
			id, app+":sub:agg:happened", client, app, at)
		require.NoError(t, err)
		f.all = append(f.all, id)
		return id
	}
	base := time.Now().UTC().Add(-time.Hour)
	a, b := f.clientA, f.clientB
	f.ownA = seed("evtrd"+sfx[:6]+"1", f.own, &a, base.Add(1*time.Second))
	f.ownB = seed("evtrd"+sfx[:6]+"2", f.own, &b, base.Add(2*time.Second))
	f.ownPlatform = seed("evtrd"+sfx[:6]+"3", f.own, nil, base.Add(3*time.Second))
	f.foreign = seed("evtrd"+sfx[:6]+"4", f.other, &a, base.Add(4*time.Second))
	return f
}

func (f *readFixture) ctx(ac *auth.AuthContext) context.Context {
	return auth.WithContext(context.Background(), ac)
}

// svc is an application service account bound to the own application.
func (f *readFixture) svc(perms ...string) *auth.AuthContext {
	return &auth.AuthContext{PrincipalID: "prn_svc", Scope: auth.ScopeAnchor, Permissions: perms, Applications: []string{f.ownID}}
}

func (f *readFixture) list(t *testing.T, ctx context.Context, in listInput) ([]string, error) {
	t.Helper()
	out, err := f.s.list(ctx, &in)
	if err != nil {
		return nil, err
	}
	var ids []string
	for _, e := range out.Body {
		if strings.HasPrefix(e.ID, "evtrd") && slices.Contains(f.all, e.ID) {
			ids = append(ids, e.ID)
		}
	}
	return ids, nil
}

// TestEventRead_ApplicationConfinement is the F20 confinement matrix for
// GET /api/events and GET /api/events/{id}.
func TestEventRead_ApplicationConfinement(t *testing.T) {
	f := newReadFixture(t)

	t.Run("own application only", func(t *testing.T) {
		ids, err := f.list(t, f.ctx(f.svc(appSvcEventView)), listInput{Size: 1000})
		require.NoError(t, err)
		assert.ElementsMatch(t, []string{f.ownA, f.ownB, f.ownPlatform}, ids)
		assert.NotContains(t, ids, f.foreign)
	})

	t.Run("a caller filter cannot widen it", func(t *testing.T) {
		ids, err := f.list(t, f.ctx(f.svc(appSvcEventView)), listInput{Applications: f.other, Size: 1000})
		require.NoError(t, err)
		assert.Empty(t, ids, "asking for another application narrows to nothing, never reaches it")
		ids, err = f.list(t, f.ctx(f.svc(appSvcEventView)), listInput{Applications: f.own + "," + f.other, Size: 1000})
		require.NoError(t, err)
		assert.ElementsMatch(t, []string{f.ownA, f.ownB, f.ownPlatform}, ids)
		ids, err = f.list(t, f.ctx(f.svc(appSvcEventView)), listInput{Types: f.other + ":sub:agg:happened", Size: 1000})
		require.NoError(t, err)
		assert.Empty(t, ids)
	})

	t.Run("unbound caller refused", func(t *testing.T) {
		unbound := &auth.AuthContext{PrincipalID: "prn_unb", Scope: auth.ScopeAnchor, Permissions: []string{appSvcEventView}}
		_, err := f.list(t, f.ctx(unbound), listInput{})
		testpg.RequireUsecaseError(t, err, usecase.KindAuthorization, "FORBIDDEN")
		_, err = f.s.getByID(f.ctx(unbound), &apicommon.IDInput{ID: f.ownA})
		testpg.RequireUsecaseError(t, err, usecase.KindAuthorization, "FORBIDDEN")
		// Bound to an id that names no application: also refused, not blind.
		ghost := &auth.AuthContext{PrincipalID: "prn_ghost", Scope: auth.ScopeAnchor, Permissions: []string{appSvcEventView}, Applications: []string{"app_doesnotexist"}}
		_, err = f.list(t, f.ctx(ghost), listInput{})
		testpg.RequireUsecaseError(t, err, usecase.KindAuthorization, "FORBIDDEN")
	})

	t.Run("a caller bound to the other application sees only that one", func(t *testing.T) {
		oth := &auth.AuthContext{PrincipalID: "prn_oth", Scope: auth.ScopeAnchor, Permissions: []string{appSvcEventView}, Applications: []string{f.otherID}}
		ids, err := f.list(t, f.ctx(oth), listInput{Size: 1000})
		require.NoError(t, err)
		assert.ElementsMatch(t, []string{f.foreign}, ids)
	})

	t.Run("permission required", func(t *testing.T) {
		none := f.svc("platform:application-service:dispatch-job:view")
		_, err := f.list(t, f.ctx(none), listInput{})
		testpg.RequireUsecaseError(t, err, usecase.KindAuthorization, "PERMISSION_REQUIRED")
		_, err = f.s.getByID(f.ctx(none), &apicommon.IDInput{ID: f.ownA})
		testpg.RequireUsecaseError(t, err, usecase.KindAuthorization, "PERMISSION_REQUIRED")
		_, err = f.list(t, f.ctx(nil), listInput{})
		require.Error(t, err)
	})

	t.Run("an all-applications caller sees every application", func(t *testing.T) {
		ac := f.svc(appSvcEventView)
		ac.Applications, ac.AllApplications = nil, true
		ids, err := f.list(t, f.ctx(ac), listInput{Size: 1000})
		require.NoError(t, err)
		assert.ElementsMatch(t, f.all, ids)
	})

	t.Run("the console's messaging view stays unconfined", func(t *testing.T) {
		ac := f.svc("platform:messaging:event:view") // bound to own, but the console permission
		ids, err := f.list(t, f.ctx(ac), listInput{Size: 1000})
		require.NoError(t, err)
		assert.ElementsMatch(t, f.all, ids)
	})

	t.Run("client-scoped caller keeps its client reach on top", func(t *testing.T) {
		ac := &auth.AuthContext{
			PrincipalID: "prn_cl", Scope: auth.ScopeClient, Clients: []string{f.clientA},
			Permissions: []string{appSvcEventView}, Applications: []string{f.ownID},
		}
		ids, err := f.list(t, f.ctx(ac), listInput{Size: 1000})
		require.NoError(t, err)
		// own application AND (client A or platform-scoped); not client B, not the other application.
		assert.ElementsMatch(t, []string{f.ownA, f.ownPlatform}, ids)
		_, err = f.s.getByID(f.ctx(ac), &apicommon.IDInput{ID: f.ownB})
		testpg.RequireUsecaseError(t, err, usecase.KindAuthorization, "FORBIDDEN")
	})

	t.Run("get by id: own, other and missing", func(t *testing.T) {
		ctx := f.ctx(f.svc(appSvcEventView))
		got, err := f.s.getByID(ctx, &apicommon.IDInput{ID: f.ownA})
		require.NoError(t, err)
		assert.Equal(t, f.ownA, got.Body.ID)

		_, errOther := f.s.getByID(ctx, &apicommon.IDInput{ID: f.foreign})
		_, errMissing := f.s.getByID(ctx, &apicommon.IDInput{ID: "evtrd_missing_id"})
		testpg.RequireUsecaseError(t, errOther, usecase.KindNotFound, "Event_NOT_FOUND")
		testpg.RequireUsecaseError(t, errMissing, usecase.KindNotFound, "Event_NOT_FOUND")
		// No existence leak: only the id echoed in the message differs.
		assert.Equal(t, strings.ReplaceAll(errMissing.Error(), "evtrd_missing_id", "X"),
			strings.ReplaceAll(errOther.Error(), f.foreign, "X"))
	})

	t.Run("pagination", func(t *testing.T) {
		ctx := f.ctx(f.svc(appSvcEventView))
		page := func(limit, offset int) []string {
			ids, err := f.list(t, ctx, listInput{Size: limit, Offset: offset})
			require.NoError(t, err)
			return ids
		}
		// newest first: platform(3), B(2), A(1)
		assert.Equal(t, []string{f.ownPlatform, f.ownB}, page(2, 0))
		assert.Equal(t, []string{f.ownA}, page(2, 2))
		assert.Empty(t, page(2, 3))
		assert.Equal(t, []string{f.ownPlatform}, page(1, 0))
	})
}
