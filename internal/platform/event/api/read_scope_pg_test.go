//go:build integration

package api

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/event"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apicommon"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/flowcatalyst-go/internal/tsid"
	"github.com/flowcatalyst/go-sdk/usecase"
)

// The tenant rule of the event reads (list, by-id, the raw debug views, the
// filter options): a client-tier caller holding the view permission reads its
// own clients' events and the client-less events of ordinary applications.
// Another tenant's events are out, and so are the platform's own client-less
// control-plane events (user created, role changed, ...), which are about every
// tenant.
func TestEventReads_ConfinedToTheCallersTenants(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	s := &State{Repo: event.NewRepository(pool)}
	sfx := strings.ToLower(tsid.GenerateUntyped())[5:]
	clientA, clientB := "clt_a"+sfx[:8], "clt_b"+sfx[:8]
	appA, appB, appN := "ra"+sfx[:8], "rb"+sfx[:8], "rn"+sfx[:8]

	at := time.Now().UTC().Add(time.Hour) // newer than anything else in the shared database
	seed := func(n, typ string, client *string) string {
		id := "es" + sfx + n
		app, _, _ := strings.Cut(typ, ":")
		_, err := pool.Exec(ctx,
			`INSERT INTO msg_events (id, type, source, time, deduplication_id, client_id, created_at)
			 VALUES ($1, $2, 'test://scope', NOW(), $1, $3, $4)`, id, typ, client, at)
		require.NoError(t, err)
		_, err = pool.Exec(ctx,
			`INSERT INTO msg_events_read (id, type, source, time, client_id, application, subdomain, aggregate, created_at)
			 VALUES ($1, $2, 'test://scope', NOW(), $3, $4, 'sub', 'agg', $5)`, id, typ, client, app, at)
		require.NoError(t, err)
		return id
	}
	a, b := clientA, clientB
	evA := seed("a", appA+":sub:agg:happened", &a)
	evB := seed("b", appB+":sub:agg:happened", &b)
	evN := seed("n", appN+":sub:agg:happened", nil)    // client-less, ordinary application
	evP := seed("p", "platform:iam:user:created", nil) // client-less, the platform's own
	evPA := seed("q", "platform:iam:user:created", &a) // the platform's, about client A
	all := []string{evA, evB, evN, evP, evPA}

	viewer := func(clients ...string) context.Context {
		return testpg.WithAuth(ctx, &auth.AuthContext{
			PrincipalID: "prn_eventviewer", Scope: auth.ScopeClient, Clients: clients,
			Permissions: []string{"platform:messaging:event:view", "platform:messaging:event:view-raw"}, AllApplications: true,
		})
	}
	anchor := testpg.WithAuth(ctx, &auth.AuthContext{
		PrincipalID: "prn_eventanchor", Scope: auth.ScopeAnchor,
		Permissions: []string{"platform:messaging:event:view", "platform:messaging:event:view-raw"}, AllApplications: true,
	})
	mine := func(got []string) []string {
		var out []string
		for _, id := range got {
			for _, m := range all {
				if id == m {
					out = append(out, id)
				}
			}
		}
		return out
	}

	t.Run("list and list-raw", func(t *testing.T) {
		for name, call := range map[string]func(context.Context) ([]string, error){
			"list": func(c context.Context) ([]string, error) {
				out, err := s.list(c, &listInput{Size: 1000})
				return eventIDs(out, err)
			},
			"list-raw": func(c context.Context) ([]string, error) {
				out, err := s.listRaw(c, &listInput{Size: 1000})
				return eventIDs(out, err)
			},
		} {
			got, err := call(viewer(clientA))
			require.NoError(t, err, name)
			assert.ElementsMatch(t, []string{evA, evN, evPA}, mine(got), "%s: own, and the ordinary client-less one; not B's, not the platform's client-less", name)
			got, err = call(anchor)
			require.NoError(t, err, name)
			assert.ElementsMatch(t, all, mine(got), "%s: an anchor sees every event", name)
		}
	})

	t.Run("by id", func(t *testing.T) {
		for _, id := range []string{evA, evN, evPA} {
			_, err := s.getByID(viewer(clientA), &apicommon.IDInput{ID: id})
			require.NoError(t, err, id)
		}
		// Another tenant's event keeps the 403 this route has always answered;
		// the platform's own client-less event is a 404.
		_, err := s.getByID(viewer(clientA), &apicommon.IDInput{ID: evB})
		testpg.RequireUsecaseError(t, err, usecase.KindAuthorization, "FORBIDDEN")
		_, err = s.getByID(viewer(clientA), &apicommon.IDInput{ID: evP})
		testpg.RequireUsecaseError(t, err, usecase.KindNotFound, "Event_NOT_FOUND")
		for _, id := range all {
			_, err = s.getByID(anchor, &apicommon.IDInput{ID: id})
			require.NoError(t, err, "an anchor reads %s", id)
		}
	})

	t.Run("debug raw list and by id", func(t *testing.T) {
		out, err := s.listDebugRaw(viewer(clientA), &rawListInput{Size: 1000})
		require.NoError(t, err)
		var got []string
		for _, e := range out.Body {
			got = append(got, e.ID)
		}
		assert.ElementsMatch(t, []string{evA, evN, evPA}, mine(got))

		out, err = s.listDebugRaw(anchor, &rawListInput{Size: 1000})
		require.NoError(t, err)
		got = nil
		for _, e := range out.Body {
			got = append(got, e.ID)
		}
		assert.ElementsMatch(t, all, mine(got), "an anchor sees every raw event")

		for _, id := range []string{evA, evN, evPA} {
			_, err := s.getDebugRaw(viewer(clientA), &apicommon.IDInput{ID: id})
			require.NoError(t, err, id)
		}
		for _, id := range []string{evB, evP} {
			_, err := s.getDebugRaw(viewer(clientA), &apicommon.IDInput{ID: id})
			testpg.RequireUsecaseError(t, err, usecase.KindNotFound, "Event_NOT_FOUND")
		}
		for _, id := range all {
			_, err := s.getDebugRaw(anchor, &apicommon.IDInput{ID: id})
			require.NoError(t, err, "an anchor reads %s", id)
		}
	})

	t.Run("filter options (distinct values)", func(t *testing.T) {
		apps := func(c context.Context) []string {
			out, err := s.filterOptions(c, &apicommon.Empty{})
			require.NoError(t, err)
			var v []string
			for _, o := range out.Body.Applications {
				v = append(v, o.Value)
			}
			return v
		}
		got := apps(viewer(clientA))
		assert.Contains(t, got, appA)
		assert.Contains(t, got, appN)
		assert.Contains(t, got, "platform", "the platform's events about client A are its own tenant's")
		assert.NotContains(t, got, appB, "another tenant's values are absent")

		// With A's platform event out of the way, the platform's client-less
		// events alone contribute no value: a viewer of B has none of them.
		got = apps(viewer(clientB))
		assert.Contains(t, got, appB)
		assert.NotContains(t, got, appA)
		assert.NotContains(t, got, "platform", "the platform's client-less events contribute no value to a client viewer")

		got = apps(anchor)
		assert.Subset(t, got, []string{appA, appB, appN, "platform"})
	})
}

func eventIDs(out *apicommon.Out[[]EventRead], err error) ([]string, error) {
	if err != nil {
		return nil, err
	}
	var ids []string
	for _, e := range out.Body {
		ids = append(ids, e.ID)
	}
	return ids, nil
}
