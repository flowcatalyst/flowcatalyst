//go:build integration

package api

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchjob"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/flowcatalyst-go/internal/tsid"
)

// GET /bff/debug/dispatch-jobs is confined as its sibling list-raw is: a
// client-tier caller holding the view-raw permission reads platform-scoped jobs
// and its own clients' only, never another tenant's.
func TestDebugRawDispatchJobs_ConfinedToTheCallersClients(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	s := &State{Repo: dispatchjob.NewRepository(pool)}
	sfx := strings.ToLower(tsid.GenerateUntyped())[5:]
	clientA, clientB := "clt_a"+sfx[:8], "clt_b"+sfx[:8]

	at := time.Now().UTC().Add(time.Hour) // newer than anything else in the shared database
	seed := func(n string, client *string) string {
		id := "dj" + sfx + n
		_, err := pool.Exec(ctx,
			`INSERT INTO msg_dispatch_jobs (id, code, target_url, status, client_id, created_at, updated_at)
			 VALUES ($1, $2, 'http://example.invalid/hook', 'PENDING', $3, $4, $4)`,
			id, "sec"+sfx+":sub:agg:"+n, client, at)
		require.NoError(t, err)
		return id
	}
	a, b := clientA, clientB
	jobA, jobB, jobPlatform := seed("a", &a), seed("b", &b), seed("p", nil)
	all := []string{jobA, jobB, jobPlatform}

	caller := func(scope auth.Scope, clients ...string) context.Context {
		return testpg.WithAuth(ctx, &auth.AuthContext{
			PrincipalID: "prn_djviewer", Scope: scope, Clients: clients,
			Permissions: []string{viewRawPerm}, AllApplications: true,
		})
	}
	mine := func(c context.Context) []string {
		t.Helper()
		out, err := s.listDebugRaw(c, &rawListInput{Size: 1000})
		require.NoError(t, err)
		var got []string
		for _, j := range out.Body {
			for _, id := range all {
				if j.ID == id {
					got = append(got, id)
				}
			}
		}
		return got
	}

	assert.ElementsMatch(t, []string{jobA, jobPlatform}, mine(caller(auth.ScopeClient, clientA)),
		"own client's job and the platform-scoped one; not another tenant's")
	assert.ElementsMatch(t, []string{jobB, jobPlatform}, mine(caller(auth.ScopeClient, clientB)))
	assert.ElementsMatch(t, all, mine(caller(auth.ScopeAnchor)), "an anchor sees every job")
}
