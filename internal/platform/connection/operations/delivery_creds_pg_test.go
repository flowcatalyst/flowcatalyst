//go:build integration

package operations_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/connection"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/connection/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/go-sdk/usecase"
	"github.com/flowcatalyst/go-sdk/usecaseop"
)

// A connection signs its subscriptions' deliveries with its service account,
// so naming that account hands its credentials to whatever URL a subscription
// then points at. T5: the caller must be allowed to use the account they name.
func TestConnection_ServiceAccountMustBeWieldable(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testpg.Pool(t)
	repo := connection.NewRepository(pool)
	apps := application.NewRepository(pool)
	uow := testpg.NewUoW(t)
	clientA := "cli_dccon_a"
	caller := testpg.WithAuth(ctx, &auth.AuthContext{
		PrincipalID: "prn_dccon1", Scope: auth.ScopeClient, Clients: []string{clientA}, AllApplications: true,
	})
	anchorOnly := testpg.SeedServiceAccount(t, "sva_dccon_anch001", testpg.ServiceAccountSpec{})
	own := testpg.SeedServiceAccount(t, "sva_dccon_own0001", testpg.ServiceAccountSpec{ClientIDs: []string{clientA}})
	create := func(c context.Context, code, account string) error {
		_, err := usecaseop.Run(c, uow, operations.CreateConnection(repo, apps, accts(t)),
			operations.CreateCommand{Code: code, Name: code, ServiceAccountID: account, ClientID: &clientA}, testpg.TestEC())
		return err
	}

	// Create.
	testpg.RequireUsecaseError(t, create(caller, "dccon-stolen", anchorOnly), usecase.KindAuthorization, "SCOPE_FORBIDDEN")
	existing, err := repo.FindByCode(ctx, "dccon-stolen", nil, &clientA)
	require.NoError(t, err)
	assert.Nil(t, existing, "a refused create stores nothing")
	testpg.RequireUsecaseError(t, create(caller, "dccon-nosuch", "sva_dccon_nosuch1"), usecase.KindValidation, "SERVICE_ACCOUNT_NOT_FOUND")
	require.NoError(t, create(caller, "dccon-own", own))
	require.NoError(t, create(testpg.OperatorCtx(), "dccon-operator", anchorOnly))

	// Update.
	conn, err := repo.FindByCode(ctx, "dccon-own", nil, &clientA)
	require.NoError(t, err)
	require.NotNil(t, conn)
	update := func(c context.Context, account string) error {
		_, err := usecaseop.Run(c, uow, operations.UpdateConnection(repo, apps, accts(t)),
			operations.UpdateCommand{ID: conn.ID, Name: "dccon-own", ServiceAccountID: &account}, testpg.TestEC())
		return err
	}
	testpg.RequireUsecaseError(t, update(caller, anchorOnly), usecase.KindAuthorization, "SCOPE_FORBIDDEN")
	got, err := repo.FindByID(ctx, conn.ID)
	require.NoError(t, err)
	assert.Equal(t, own, got.ServiceAccountID, "a refused update leaves the account alone")
	require.NoError(t, update(caller, own), "re-sending the stored account is not a change")
	require.NoError(t, update(testpg.OperatorCtx(), anchorOnly))
	got, err = repo.FindByID(ctx, conn.ID)
	require.NoError(t, err)
	assert.Equal(t, anchorOnly, got.ServiceAccountID)
}
