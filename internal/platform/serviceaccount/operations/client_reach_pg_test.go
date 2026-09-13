//go:build integration

package operations_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	platformauth "github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/client"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/serviceaccount"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/serviceaccount/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecaseop"
)

func seedClient(t *testing.T, pool *pgxpool.Pool, id, identifier string) {
	t.Helper()
	_, err := pool.Exec(context.Background(),
		`INSERT INTO tnt_clients (id, name, identifier) VALUES ($1, $2, $2)
		 ON CONFLICT (id) DO NOTHING`, id, identifier)
	require.NoError(t, err)
}

// provisionWithClients creates an account with credentials — the operation
// that also mints the linked SERVICE principal — carrying clientIDs.
func provisionWithClients(t *testing.T, code string, clientIDs []string) (*principal.Principal, operations.CreateWithCredentialsResult) {
	t.Helper()
	pool := testpg.Pool(t)
	uow := testpg.NewUoW(t)
	principals := principal.NewRepository(pool)

	res, err := usecaseop.RunTx(testpg.AnchorCtx(), uow,
		operations.CreateServiceAccountWithCredentials(
			serviceaccount.NewRepository(pool), principals,
			platformauth.NewRepository(pool).OAuthClients,
			client.NewRepository(pool), principal.NewClientAccessGrantRepo(pool)),
		operations.CreateCommand{Code: code, Name: code, ClientIDs: clientIDs}, testpg.TestEC())
	require.NoError(t, err)

	p, err := principals.FindByID(context.Background(), res.PrincipalID)
	require.NoError(t, err)
	require.NotNil(t, p)
	return p, res
}

// A service account's reach is its client links. The links were stored on the
// account but the linked principal was always ANCHOR, and a token is built
// from the principal — so every service account reached every tenant whatever
// it was linked to.
func TestServiceAccountReach_DerivedFromClientLinks(t *testing.T) {
	pool := testpg.Pool(t)
	seedClient(t, pool, "clt_reach_one", "reach-one")
	seedClient(t, pool, "clt_reach_two", "reach-two")

	t.Run("no links stays anchor", func(t *testing.T) {
		p, _ := provisionWithClients(t, "sareach-none", nil)
		assert.Equal(t, principal.ScopeAnchor, p.Scope)
		assert.Nil(t, p.ClientID)
		assert.Empty(t, p.AssignedClients)
	})

	t.Run("one link confines to that client", func(t *testing.T) {
		p, _ := provisionWithClients(t, "sareach-one", []string{"clt_reach_one"})
		assert.Equal(t, principal.ScopeClient, p.Scope)
		require.NotNil(t, p.ClientID)
		assert.Equal(t, "clt_reach_one", *p.ClientID)
	})

	t.Run("several links become a partner with a grant each", func(t *testing.T) {
		p, _ := provisionWithClients(t, "sareach-two", []string{"clt_reach_one", "clt_reach_two"})
		assert.Equal(t, principal.ScopePartner, p.Scope)
		assert.Nil(t, p.ClientID, "a partner reaches through its grants, not a single client")
		assert.ElementsMatch(t, []string{"clt_reach_one", "clt_reach_two"}, p.AssignedClients)
	})
}

// Changing the links re-derives the principal, so an account moved between
// clients cannot keep the reach it had before.
func TestServiceAccountReach_UpdateReDerives(t *testing.T) {
	pool := testpg.Pool(t)
	uow := testpg.NewUoW(t)
	principals := principal.NewRepository(pool)
	seedClient(t, pool, "clt_reach_upd_a", "reach-upd-a")
	seedClient(t, pool, "clt_reach_upd_b", "reach-upd-b")

	p, res := provisionWithClients(t, "sareach-upd", []string{"clt_reach_upd_a"})
	require.Equal(t, principal.ScopeClient, p.Scope)

	update := func(ids []string) *principal.Principal {
		t.Helper()
		_, err := usecaseop.RunTx(testpg.AnchorCtx(), uow,
			operations.UpdateServiceAccount(serviceaccount.NewRepository(pool), principals,
				client.NewRepository(pool), principal.NewClientAccessGrantRepo(pool)),
			operations.UpdateCommand{ID: res.ServiceAccount.ID, ClientIDs: ids}, testpg.TestEC())
		require.NoError(t, err)
		got, err := principals.FindByID(context.Background(), res.PrincipalID)
		require.NoError(t, err)
		return got
	}

	// One → two: becomes a partner.
	got := update([]string{"clt_reach_upd_a", "clt_reach_upd_b"})
	assert.Equal(t, principal.ScopePartner, got.Scope)
	assert.ElementsMatch(t, []string{"clt_reach_upd_a", "clt_reach_upd_b"}, got.AssignedClients)

	// Two → one: back to a single client.
	got = update([]string{"clt_reach_upd_b"})
	assert.Equal(t, principal.ScopeClient, got.Scope)
	require.NotNil(t, got.ClientID)
	assert.Equal(t, "clt_reach_upd_b", *got.ClientID)

	// An update that does not mention the links leaves reach alone.
	name := "Renamed"
	_, err := usecaseop.RunTx(testpg.AnchorCtx(), uow,
		operations.UpdateServiceAccount(serviceaccount.NewRepository(pool), principals,
			client.NewRepository(pool), principal.NewClientAccessGrantRepo(pool)),
		operations.UpdateCommand{ID: res.ServiceAccount.ID, Name: &name}, testpg.TestEC())
	require.NoError(t, err)
	got, err = principals.FindByID(context.Background(), res.PrincipalID)
	require.NoError(t, err)
	assert.Equal(t, principal.ScopeClient, got.Scope)
}

// A typo must not silently confine a principal to a client that is not there.
func TestServiceAccountReach_UnknownClientRefused(t *testing.T) {
	pool := testpg.Pool(t)
	uow := testpg.NewUoW(t)
	principals := principal.NewRepository(pool)

	_, err := usecaseop.RunTx(testpg.AnchorCtx(), uow,
		operations.CreateServiceAccountWithCredentials(
			serviceaccount.NewRepository(pool), principals,
			platformauth.NewRepository(pool).OAuthClients,
			client.NewRepository(pool), principal.NewClientAccessGrantRepo(pool)),
		operations.CreateCommand{
			Code: "sareach-badclient", Name: "Bad Client", ClientIDs: []string{"clt_does_not_exist"},
		}, testpg.TestEC())
	testpg.RequireUsecaseError(t, err, usecase.KindNotFound, "Client_NOT_FOUND")

	seedClient(t, pool, "clt_reach_ok", "reach-ok")
	_, res := provisionWithClients(t, "sareach-updbad", []string{"clt_reach_ok"})
	_, err = usecaseop.RunTx(testpg.AnchorCtx(), uow,
		operations.UpdateServiceAccount(serviceaccount.NewRepository(pool), principals,
			client.NewRepository(pool), principal.NewClientAccessGrantRepo(pool)),
		operations.UpdateCommand{ID: res.ServiceAccount.ID, ClientIDs: []string{"clt_still_missing"}},
		testpg.TestEC())
	testpg.RequireUsecaseError(t, err, usecase.KindNotFound, "Client_NOT_FOUND")
}
