//go:build integration

package api

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/scheduledjob"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/flowcatalyst-go/internal/tsid"
	"github.com/flowcatalyst/go-sdk/usecase"
)

// A job's firings and their logs are the job's tenant's data: a client-tier
// caller holding the scheduled-job view permission reads them only for its own
// clients' jobs; a platform-level job's are anchor-only (as the BFF twins).
func TestJobFiringsAndLogs_ConfinedToTheCallersClients(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	instances := scheduledjob.NewInstanceRepository(pool)
	s := &State{Repo: scheduledjob.NewRepository(pool), Instances: instances}
	sfx := strings.ToLower(tsid.GenerateUntyped())[5:]
	clientA, clientB := "clt_a"+sfx[:8], "clt_b"+sfx[:8]

	type firing struct{ job, instance string }
	seed := func(n string, client *string) firing {
		job, inst := "sjb_"+sfx+n, "sji_"+sfx+n
		_, err := pool.Exec(ctx,
			`INSERT INTO msg_scheduled_jobs (id, code, name, status, crons, client_id)
			 VALUES ($1, $1, $1, 'ACTIVE', '{}', $2)`, job, client)
		require.NoError(t, err)
		require.NoError(t, instances.Insert(ctx, &scheduledjob.ScheduledJobInstance{
			ID: inst, ScheduledJobID: job, ClientID: client, JobCode: job, TriggerKind: scheduledjob.TriggerManual,
			FiredAt: time.Now().UTC(), Status: scheduledjob.InstanceStatusDelivered,
		}))
		return firing{job, inst}
	}
	a, b := clientA, clientB
	fA, fB, fPlatform := seed("a", &a), seed("b", &b), seed("p", nil)

	viewer := testpg.WithAuth(ctx, &auth.AuthContext{
		PrincipalID: "prn_sjviewer", Scope: auth.ScopeClient, Clients: []string{clientA},
		Permissions: []string{"platform:messaging:scheduled-job:view"}, AllApplications: true,
	})
	anchor := testpg.WithAuth(ctx, &auth.AuthContext{
		PrincipalID: "prn_sjanchor", Scope: auth.ScopeAnchor,
		Permissions: []string{"platform:messaging:scheduled-job:view"}, AllApplications: true,
	})

	t.Run("instances", func(t *testing.T) {
		out, err := s.listInstances(viewer, &listInstancesInput{ID: fA.job})
		require.NoError(t, err)
		require.Len(t, out.Body.Data, 1)
		assert.Equal(t, fA.instance, out.Body.Data[0].ID)
		for _, f := range []firing{fB, fPlatform} {
			_, err = s.listInstances(viewer, &listInstancesInput{ID: f.job})
			testpg.RequireUsecaseError(t, err, usecase.KindNotFound, "ScheduledJob_NOT_FOUND")
		}
		_, err = s.listInstances(viewer, &listInstancesInput{ID: "sjb_doesnotexist"})
		testpg.RequireUsecaseError(t, err, usecase.KindNotFound, "ScheduledJob_NOT_FOUND")
		for _, f := range []firing{fA, fB, fPlatform} {
			out, err = s.listInstances(anchor, &listInstancesInput{ID: f.job})
			require.NoError(t, err, "an anchor reads %s", f.job)
			require.Len(t, out.Body.Data, 1)
		}
	})

	t.Run("logs", func(t *testing.T) {
		_, err := s.listInstanceLogs(viewer, &instanceInput{InstanceID: fA.instance})
		require.NoError(t, err)
		for _, f := range []firing{fB, fPlatform} {
			_, err = s.listInstanceLogs(viewer, &instanceInput{InstanceID: f.instance})
			testpg.RequireUsecaseError(t, err, usecase.KindNotFound, "ScheduledJobInstance_NOT_FOUND")
		}
		for _, f := range []firing{fA, fB, fPlatform} {
			_, err = s.listInstanceLogs(anchor, &instanceInput{InstanceID: f.instance})
			require.NoError(t, err, "an anchor reads %s", f.instance)
		}
	})
}
