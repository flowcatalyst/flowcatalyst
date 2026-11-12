//go:build integration

package operations_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/scheduledjob"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/scheduledjob/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/go-sdk/usecase"
	"github.com/flowcatalyst/go-sdk/usecaseop"
)

// A job stamped with an application is signed by that application's own
// service account, so choosing the job's application, or the URL it fires at,
// is choosing where those credentials go. T6.
func TestScheduledJob_ApplicationIdentityMustBeWieldable(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := scheduledjob.NewRepository(testpg.Pool(t))
	uow := testpg.NewUoW(t)
	clientA := "cli_dcsj_a"
	appA, appB := "app_dcsj_a00001", "app_dcsj_b00001"
	clientTier := testpg.WithAuth(ctx, &auth.AuthContext{
		PrincipalID: "prn_dcsj1", Scope: auth.ScopeClient, Clients: []string{clientA}, AllApplications: true,
	})
	appAnchor := testpg.WithAuth(ctx, &auth.AuthContext{
		PrincipalID: "prn_dcsj2", Scope: auth.ScopeAnchor, Applications: []string{appA},
	})
	create := func(c context.Context, code string, clientID, applicationID, url *string) error {
		_, err := usecaseop.Run(c, uow, operations.CreateScheduledJob(repo), operations.CreateCommand{
			Code: code, Name: code, Crons: validCrons, ClientID: clientID, ApplicationID: applicationID, TargetURL: url,
		}, testpg.TestEC())
		return err
	}
	hook := "https://dcsj.example.test/hook"

	// Create: a client-tier caller cannot choose an application.
	testpg.RequireUsecaseError(t, create(clientTier, "dcsj-stolen", &clientA, &appA, &hook), usecase.KindAuthorization, "APPLICATION_ACCESS_REQUIRED")
	got, err := repo.FindByCode(ctx, "dcsj-stolen", &clientA)
	require.NoError(t, err)
	assert.Nil(t, got, "a refused create stores nothing")
	// ... but a job that names no application is unaffected.
	require.NoError(t, create(clientTier, "dcsj-plain", &clientA, nil, &hook))

	// An application-scoped anchor is confined to its own application.
	testpg.RequireUsecaseError(t, create(appAnchor, "dcsj-other-app", nil, &appB, &hook), usecase.KindAuthorization, "APPLICATION_ACCESS_REQUIRED")
	require.NoError(t, create(appAnchor, "dcsj-own-app", nil, &appA, &hook))
	require.NoError(t, create(testpg.OperatorCtx(), "dcsj-operator", &clientA, &appB, &hook))

	// Update: moving an application-stamped job's URL needs the application.
	stamped, err := repo.FindByCode(ctx, "dcsj-operator", &clientA)
	require.NoError(t, err)
	require.NotNil(t, stamped)
	update := func(c context.Context, cmd operations.UpdateCommand) error {
		cmd.ID = stamped.ID
		_, err := usecaseop.Run(c, uow, operations.UpdateScheduledJob(repo), cmd, testpg.TestEC())
		return err
	}
	attacker := "https://attacker.example.test/hook"
	testpg.RequireUsecaseError(t, update(clientTier, operations.UpdateCommand{TargetURL: &attacker}), usecase.KindAuthorization, "APPLICATION_ACCESS_REQUIRED")
	got, err = repo.FindByID(ctx, stamped.ID)
	require.NoError(t, err)
	require.NotNil(t, got.TargetURL)
	assert.Equal(t, hook, *got.TargetURL, "a refused update leaves the URL alone")

	require.NoError(t, update(clientTier, operations.UpdateCommand{TargetURL: &hook}), "re-sending the stored URL is not a change")
	require.NoError(t, update(clientTier, operations.UpdateCommand{Name: new("Renamed")}))
	moved := "https://dcsj.example.test/moved"
	require.NoError(t, update(testpg.OperatorCtx(), operations.UpdateCommand{TargetURL: &moved}))

	// A job with no application can be re-pointed by its own client.
	plain, err := repo.FindByCode(ctx, "dcsj-plain", &clientA)
	require.NoError(t, err)
	_, err = usecaseop.Run(clientTier, uow, operations.UpdateScheduledJob(repo),
		operations.UpdateCommand{ID: plain.ID, TargetURL: &moved}, testpg.TestEC())
	require.NoError(t, err)
}
