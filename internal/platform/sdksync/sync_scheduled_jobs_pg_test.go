//go:build integration

package sdksync

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	appops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/application/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/scheduledjob"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/go-sdk/usecase"
	"github.com/flowcatalyst/go-sdk/usecaseop"
)

// A caller confined to one application cannot sync scheduled jobs under
// another application's path, and can under its own.
func TestSyncScheduledJobs_Endpoint_ConfinedToOwnApplication(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	uow := testpg.NewUoW(t)
	apps := application.NewRepository(pool)
	repo := scheduledjob.NewRepository(pool)

	ids := map[string]string{}
	for _, c := range []string{"sjhttpa", "sjhttpb"} {
		ev, err := usecaseop.Run(testpg.AnchorCtx(), uow, appops.CreateApplication(apps),
			appops.CreateCommand{Code: c, Name: c}, testpg.TestEC())
		require.NoError(t, err)
		ids[c] = ev.ApplicationID
	}
	s := &State{Apps: apps, ScheduledJobs: repo, UoW: uow}

	clientID := "cli_sjhttp000001"
	caller := auth.WithContext(ctx, &auth.AuthContext{
		PrincipalID: "prn_sjhttpsa00001", Scope: auth.ScopeClient, PrincipalType: "SERVICE",
		Clients: []string{clientID}, Applications: []string{ids["sjhttpa"]},
		Permissions: []string{"platform:messaging:scheduled-job:sync"},
	})
	body := func(code string) syncScheduledJobsRequest {
		return syncScheduledJobsRequest{
			ClientID: &clientID,
			Jobs: []syncScheduledJobInputRequest{{
				Code: code, Name: code, Crons: []string{"0 0 * * * *"},
				TargetURL: new("https://hooks.example.test/fire"),
			}},
		}
	}

	_, err := s.syncScheduledJobs(caller, &syncScheduledJobsInput{AppCode: "sjhttpb", Body: body("sjhttp-foreign")})
	ue := usecase.AsError(err)
	require.NotNil(t, ue, "want a refusal, got %v", err)
	assert.Equal(t, usecase.KindAuthorization, ue.Kind)
	assert.Equal(t, "Not authorised for application 'sjhttpb'", ue.Message)
	got, err := repo.FindByCode(ctx, "sjhttp-foreign", &clientID)
	require.NoError(t, err)
	assert.Nil(t, got, "a refused sync must write nothing")

	out, err := s.syncScheduledJobs(caller, &syncScheduledJobsInput{AppCode: "sjhttpa", Body: body("sjhttp-own")})
	require.NoError(t, err)
	assert.Len(t, out.Body.Created, 1)
	got, err = repo.FindByCode(ctx, "sjhttp-own", &clientID)
	require.NoError(t, err)
	require.NotNil(t, got)
	require.NotNil(t, got.ApplicationID)
	assert.Equal(t, ids["sjhttpa"], string(*got.ApplicationID))
}
