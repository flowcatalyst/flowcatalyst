//go:build integration

package operations_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/subscription"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/subscription/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/go-sdk/usecase"
	"github.com/flowcatalyst/go-sdk/usecaseop"
)

// A delivery carries the bearer token and signature of the service account
// that signs it, so whoever points a subscription at a URL must be allowed to
// use that account. These tests drive the operations as an ordinary client-tier
// caller (the case the checks exist for) and as an operator (who must keep
// working).

func clientTier(clients ...string) context.Context {
	return testpg.WithAuth(context.Background(), &auth.AuthContext{
		PrincipalID:     "prn_delivcreds1",
		Scope:           auth.ScopeClient,
		Clients:         clients,
		AllApplications: true,
		Permissions:     []string{"platform:messaging:subscription:create", "platform:messaging:subscription:update"},
	})
}

func credBindings() []subscription.EventTypeBinding {
	return []subscription.EventTypeBinding{subscription.NewEventTypeBinding("delivcreds:a:b:c")}
}

func stored(t *testing.T, repo *subscription.Repository, code string, clientID *string) *subscription.Subscription {
	t.Helper()
	s, err := repo.FindByCode(context.Background(), code, nil, clientID)
	require.NoError(t, err)
	return s
}

// T2: the account named on the subscription.
func TestCreateSubscription_ServiceAccountMustBeWieldable(t *testing.T) {
	t.Parallel()
	repo := subscription.NewRepository(testpg.Pool(t))
	uow := testpg.NewUoW(t)
	clientA := "cli_dcsub_a"
	unlinked := testpg.SeedServiceAccount(t, "sva_dcsub_unlink1", testpg.ServiceAccountSpec{})
	linked := testpg.SeedServiceAccount(t, "sva_dcsub_linked1", testpg.ServiceAccountSpec{ClientIDs: []string{clientA}})
	create := func(ctx context.Context, code string, clientID *string, account string) error {
		_, err := usecaseop.Run(ctx, uow, operations.CreateSubscription(repo, credRepos(t)), operations.CreateCommand{
			Code: code, Name: code, Endpoint: "https://dcsub.example.test/hook", ClientID: clientID,
			ServiceAccountID: &account, EventTypes: credBindings(),
		}, testpg.TestEC())
		return err
	}

	err := create(clientTier(clientA), "dcsub-stolen", &clientA, unlinked)
	testpg.RequireUsecaseError(t, err, usecase.KindAuthorization, "SCOPE_FORBIDDEN")
	assert.Nil(t, stored(t, repo, "dcsub-stolen", &clientA), "a refused create stores nothing")

	require.NoError(t, create(clientTier(clientA), "dcsub-own", &clientA, linked))
	require.NotNil(t, stored(t, repo, "dcsub-own", &clientA))

	require.NoError(t, create(testpg.OperatorCtx(), "dcsub-operator", &clientA, unlinked))
	require.NotNil(t, stored(t, repo, "dcsub-operator", &clientA))
}

// T3: the connection named on the subscription.
func TestCreateSubscription_ConnectionMustBeUsable(t *testing.T) {
	t.Parallel()
	pool := testpg.Pool(t)
	repo := subscription.NewRepository(pool)
	uow := testpg.NewUoW(t)
	clientA, clientB := "cli_dcconn_a", "cli_dcconn_b"
	anchorOnly := testpg.SeedServiceAccount(t, "sva_dcconn_anch1", testpg.ServiceAccountSpec{})
	ownAccount := testpg.SeedServiceAccount(t, "sva_dcconn_own01", testpg.ServiceAccountSpec{ClientIDs: []string{clientA}})
	insertRawConnection(t, pool, "con_dcconn_other", "dcconn-other", ownAccount, nil, &clientB)
	insertRawConnection(t, pool, "con_dcconn_global", "dcconn-global", anchorOnly, nil, nil)
	insertRawConnection(t, pool, "con_dcconn_mine01", "dcconn-mine", ownAccount, nil, &clientA)
	create := func(ctx context.Context, code, connectionID string) error {
		_, err := usecaseop.Run(ctx, uow, operations.CreateSubscription(repo, credRepos(t)), operations.CreateCommand{
			Code: code, Name: code, Endpoint: "https://dcconn.example.test/hook", ClientID: &clientA,
			ConnectionID: &connectionID, EventTypes: credBindings(),
		}, testpg.TestEC())
		return err
	}
	caller := clientTier(clientA)

	err := create(caller, "dcconn-s1", "con_dcconn_other")
	testpg.RequireUsecaseError(t, err, usecase.KindValidation, "CONNECTION_SCOPE_MISMATCH")
	assert.Nil(t, stored(t, repo, "dcconn-s1", &clientA))

	err = create(caller, "dcconn-s2", "con_dcconn_global")
	testpg.RequireUsecaseError(t, err, usecase.KindAuthorization, "SCOPE_FORBIDDEN")
	assert.Nil(t, stored(t, repo, "dcconn-s2", &clientA))

	err = create(caller, "dcconn-s3", "con_dcconn_nosuch")
	testpg.RequireUsecaseError(t, err, usecase.KindNotFound, "CONNECTION_NOT_FOUND")

	require.NoError(t, create(caller, "dcconn-s4", "con_dcconn_mine01"))
	require.NoError(t, create(testpg.OperatorCtx(), "dcconn-s5", "con_dcconn_global"))
}

// T4: changing where a delivery goes, or who signs it, on an existing row.
func TestUpdateSubscription_URLAndCredentialsAreChecked(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testpg.Pool(t)
	repo := subscription.NewRepository(pool)
	uow := testpg.NewUoW(t)
	clientA := "cli_dcupd_a"
	anchorOnly := testpg.SeedServiceAccount(t, "sva_dcupd_anch01", testpg.ServiceAccountSpec{})
	ownAccount := testpg.SeedServiceAccount(t, "sva_dcupd_own001", testpg.ServiceAccountSpec{ClientIDs: []string{clientA}})

	// A subscription signed by an account the client-tier caller may not use,
	// created by an operator.
	ev, err := usecaseop.Run(testpg.OperatorCtx(), uow, operations.CreateSubscription(repo, credRepos(t)), operations.CreateCommand{
		Code: "dcupd-sub", Name: "Before", Endpoint: "https://dcupd.example.test/original", ClientID: &clientA,
		ServiceAccountID: &anchorOnly, EventTypes: credBindings(),
	}, testpg.TestEC())
	require.NoError(t, err)
	update := func(c context.Context, cmd operations.UpdateCommand) error {
		cmd.ID = ev.SubscriptionID
		_, err := usecaseop.Run(c, uow, operations.UpdateSubscription(repo, credRepos(t)), cmd, testpg.TestEC())
		return err
	}
	caller := clientTier(clientA)

	err = update(caller, operations.UpdateCommand{Endpoint: new("https://attacker.example.test/hook")})
	testpg.RequireUsecaseError(t, err, usecase.KindAuthorization, "SCOPE_FORBIDDEN")
	got, err := repo.FindByID(ctx, ev.SubscriptionID)
	require.NoError(t, err)
	assert.Equal(t, "https://dcupd.example.test/original", got.Endpoint, "a refused update leaves the URL alone")

	err = update(caller, operations.UpdateCommand{ServiceAccountID: &anchorOnly})
	require.NoError(t, err, "re-sending the stored account is not a change")
	err = update(caller, operations.UpdateCommand{ConnectionID: new("con_dcupd_nosuch")})
	testpg.RequireUsecaseError(t, err, usecase.KindNotFound, "CONNECTION_NOT_FOUND")

	// Fields other than the URL and the credentials are never tripped.
	require.NoError(t, update(caller, operations.UpdateCommand{Name: new("Renamed")}))
	got, err = repo.FindByID(ctx, ev.SubscriptionID)
	require.NoError(t, err)
	assert.Equal(t, "Renamed", got.Name)

	// Pointing it at an account the caller may use is allowed, and then so is
	// moving the URL.
	require.NoError(t, update(caller, operations.UpdateCommand{ServiceAccountID: &ownAccount}))
	require.NoError(t, update(caller, operations.UpdateCommand{Endpoint: new("https://dcupd.example.test/moved")}))

	// The operator may still do any of it.
	require.NoError(t, update(testpg.OperatorCtx(), operations.UpdateCommand{ServiceAccountID: &anchorOnly, Endpoint: new("https://dcupd.example.test/op")}))
}

// T3b: a global connection (no client, no application) is in scope for every
// client, but its account is not the caller's to use: naming it on a
// subscription would send that account's credentials to the caller's URL.
func TestCreateSubscription_GlobalConnectionAccountMustBeWieldable(t *testing.T) {
	t.Parallel()
	pool := testpg.Pool(t)
	repo := subscription.NewRepository(pool)
	uow := testpg.NewUoW(t)
	clientA := "cli_dcgconn_a"
	noClientAccount := testpg.SeedServiceAccount(t, "sva_dcgconn_nc001", testpg.ServiceAccountSpec{})
	insertRawConnection(t, pool, "con_dcgconn_g001", "dcgconn-global", noClientAccount, nil, nil)
	create := func(ctx context.Context, code string) error {
		connectionID := "con_dcgconn_g001"
		_, err := usecaseop.Run(ctx, uow, operations.CreateSubscription(repo, credRepos(t)), operations.CreateCommand{
			Code: code, Name: code, Endpoint: "https://dcgconn.example.test/hook", ClientID: &clientA,
			ConnectionID: &connectionID, EventTypes: credBindings(),
		}, testpg.TestEC())
		return err
	}
	err := create(clientTier(clientA), "dcgconn-client")
	testpg.RequireUsecaseError(t, err, usecase.KindAuthorization, "SCOPE_FORBIDDEN")
	assert.Nil(t, stored(t, repo, "dcgconn-client", &clientA))
	require.NoError(t, create(testpg.OperatorCtx(), "dcgconn-operator"))
}

// Clearing the account is a change of signer: the application's own account
// signs a subscription with an application code and no account or connection,
// so a caller who set its own account and URL must not be able to clear the
// account and keep the URL.
func TestUpdateSubscription_ClearingTheAccountIsChecked(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testpg.Pool(t)
	repo := subscription.NewRepository(pool)
	uow := testpg.NewUoW(t)
	clientA := "cli_dcclr_a"
	appCode := "dcclrapp"
	testpg.EnsureApplication(t, appCode)
	own := testpg.SeedServiceAccount(t, "sva_dcclr_own0001", testpg.ServiceAccountSpec{ClientIDs: []string{clientA}})
	insertRawSubscription(t, pool, "sub_dcclr_000001", "dcclr-sub", "Synced", &appCode, &clientA, "API")
	update := func(c context.Context, cmd operations.UpdateCommand) error {
		cmd.ID = "sub_dcclr_000001"
		_, err := usecaseop.Run(c, uow, operations.UpdateSubscription(repo, credRepos(t)), cmd, testpg.TestEC())
		return err
	}
	caller := clientTier(clientA)

	// Step 1: own account and own URL: allowed.
	require.NoError(t, update(caller, operations.UpdateCommand{ServiceAccountID: &own, Endpoint: new("https://dcclr.example.test/mine")}))
	// Step 2: clear the account, URL unchanged: refused, account kept.
	blank := ""
	err := update(caller, operations.UpdateCommand{ServiceAccountID: &blank})
	testpg.RequireUsecaseError(t, err, usecase.KindAuthorization, "APPLICATION_ACCESS_REQUIRED")
	got, err := repo.FindByID(ctx, "sub_dcclr_000001")
	require.NoError(t, err)
	require.NotNil(t, got.ServiceAccountID)
	assert.Equal(t, own, *got.ServiceAccountID, "a refused clear leaves the account alone")

	// An operator may clear it, and an update that changes none of the three is never checked.
	require.NoError(t, update(testpg.OperatorCtx(), operations.UpdateCommand{ServiceAccountID: &blank}))
	require.NoError(t, update(caller, operations.UpdateCommand{Name: new("Renamed")}))
}
