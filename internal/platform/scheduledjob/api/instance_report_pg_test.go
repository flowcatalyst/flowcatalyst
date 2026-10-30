//go:build integration

package api

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/scheduledjob"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/go-sdk/usecase"
)

func TestMain(m *testing.M) { testpg.RunMain(m) }

// An application's SDK reports on the firings it receives (log lines, the
// completion) with its own service account, which holds the seeded
// application-service role (scheduled-job-instance:write), not the messaging
// scheduled-job grants. It reports on its own application's jobs only; the
// messaging grants keep reporting on any firing; nothing else is admitted.
func TestInstanceReports_ApplicationServiceAccountConfinedToItsJobs(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	instances := scheduledjob.NewInstanceRepository(pool)
	s := &State{Repo: scheduledjob.NewRepository(pool), Instances: instances}

	const ownApp, otherApp = "app_rptown000001", "app_rptother0001"
	firing := func(jobID, code string, applicationID *string) string {
		t.Helper()
		_, err := pool.Exec(ctx,
			`INSERT INTO msg_scheduled_jobs (id, code, name, status, crons, application_id)
			 VALUES ($1, $2, $2, 'ACTIVE', '{}', $3)`, jobID, code, applicationID)
		require.NoError(t, err)
		instID := "sji_" + jobID[4:]
		require.NoError(t, instances.Insert(ctx, &scheduledjob.ScheduledJobInstance{
			ID: instID, ScheduledJobID: jobID, JobCode: code, TriggerKind: scheduledjob.TriggerManual,
			FiredAt: time.Now().UTC(), Status: scheduledjob.InstanceStatusDelivered,
		}))
		return instID
	}
	own, other := ownApp, otherApp
	ownFiring := firing("sjb_rptown000001", "rpt-own-job", &own)
	otherFiring := firing("sjb_rptother0001", "rpt-other-job", &other)
	noAppFiring := firing("sjb_rptnoapp0001", "rpt-noapp-job", nil)

	appSA := testpg.WithAuth(ctx, &auth.AuthContext{
		PrincipalID:   "prn_rptappsa0001",
		Scope:         auth.ScopeAnchor,
		Applications:  []string{ownApp},
		Permissions:   []string{"platform:application-service:scheduled-job-instance:write"},
		PrincipalType: "SERVICE",
	})
	report := func(ctx context.Context, instID string) (logErr, completeErr error) {
		_, logErr = s.writeInstanceLog(ctx, &writeLogInput{
			InstanceID: instID,
			Body:       WriteInstanceLogRequest{Level: "INFO", Message: "ran"},
		})
		_, completeErr = s.completeInstance(ctx, &completeInstanceInput{
			InstanceID: instID,
			Body:       CompleteInstanceRequest{Status: "SUCCESS"},
		})
		return
	}
	refusedWith := func(t *testing.T, err error, code string) {
		t.Helper()
		ue := usecase.AsError(err)
		require.NotNil(t, ue, "expected a refusal, got %v", err)
		assert.Equal(t, usecase.KindAuthorization, ue.Kind)
		assert.Equal(t, code, ue.Code)
	}

	// Its own application's firing: logged and completed.
	logErr, completeErr := report(appSA, ownFiring)
	require.NoError(t, logErr)
	require.NoError(t, completeErr)
	got, err := instances.FindByID(ctx, ownFiring)
	require.NoError(t, err)
	assert.Equal(t, scheduledjob.InstanceStatusCompleted, got.Status)
	logs, err := instances.ListLogs(ctx, ownFiring, 10)
	require.NoError(t, err)
	require.Len(t, logs, 1)

	// Another application's firing, and a firing of a job with no application.
	for _, instID := range []string{otherFiring, noAppFiring} {
		logErr, completeErr = report(appSA, instID)
		refusedWith(t, logErr, "APPLICATION_ACCESS_REQUIRED")
		refusedWith(t, completeErr, "APPLICATION_ACCESS_REQUIRED")
	}

	// A principal with neither grant (e.g. only the job view) is refused.
	viewer := testpg.WithAuth(ctx, &auth.AuthContext{
		PrincipalID: "prn_rptviewer001", Scope: auth.ScopeAnchor, AllApplications: true,
		Permissions: []string{"platform:messaging:scheduled-job:view"},
	})
	logErr, completeErr = report(viewer, ownFiring)
	refusedWith(t, logErr, "PERMISSION_REQUIRED")
	refusedWith(t, completeErr, "PERMISSION_REQUIRED")

	// The messaging grants report on any firing, as before.
	admin := testpg.WithAuth(ctx, &auth.AuthContext{
		PrincipalID: "prn_rptadmin0001", Scope: auth.ScopeAnchor,
		Permissions: []string{"platform:messaging:scheduled-job:update"},
	})
	for _, instID := range []string{otherFiring, noAppFiring} {
		logErr, completeErr = report(admin, instID)
		require.NoError(t, logErr)
		require.NoError(t, completeErr)
	}
}
