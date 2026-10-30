//go:build integration

package operations_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/ids"
	platformauth "github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/client"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/serviceaccount"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/serviceaccount/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/go-sdk/usecase"
	"github.com/flowcatalyst/go-sdk/usecaseop"
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

	p, err := principals.FindByID(context.Background(), ids.PrincipalID(res.PrincipalID))
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
		assert.Equal(t, "clt_reach_one", string(*p.ClientID))
	})

	t.Run("several links become a partner with a grant each", func(t *testing.T) {
		p, _ := provisionWithClients(t, "sareach-two", []string{"clt_reach_one", "clt_reach_two"})
		assert.Equal(t, principal.ScopePartner, p.Scope)
		assert.Nil(t, p.ClientID, "a partner reaches through its grants, not a single client")
		assert.ElementsMatch(t, []string{"clt_reach_one", "clt_reach_two"}, ids.Strings(p.AssignedClients))
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

	update := func(clientIDs []string) *principal.Principal {
		t.Helper()
		_, err := usecaseop.RunTx(testpg.AnchorCtx(), uow,
			operations.UpdateServiceAccount(serviceaccount.NewRepository(pool), principals,
				client.NewRepository(pool), principal.NewClientAccessGrantRepo(pool)),
			operations.UpdateCommand{ID: res.ServiceAccount.ID, ClientIDs: clientIDs}, testpg.TestEC())
		require.NoError(t, err)
		got, err := principals.FindByID(context.Background(), ids.PrincipalID(res.PrincipalID))
		require.NoError(t, err)
		return got
	}

	// One → two: becomes a partner.
	got := update([]string{"clt_reach_upd_a", "clt_reach_upd_b"})
	assert.Equal(t, principal.ScopePartner, got.Scope)
	assert.ElementsMatch(t, []string{"clt_reach_upd_a", "clt_reach_upd_b"}, ids.Strings(got.AssignedClients))

	// Two → one: back to a single client.
	got = update([]string{"clt_reach_upd_b"})
	assert.Equal(t, principal.ScopeClient, got.Scope)
	require.NotNil(t, got.ClientID)
	assert.Equal(t, "clt_reach_upd_b", string(*got.ClientID))

	// An update that does not mention the links leaves reach alone.
	name := "Renamed"
	_, err := usecaseop.RunTx(testpg.AnchorCtx(), uow,
		operations.UpdateServiceAccount(serviceaccount.NewRepository(pool), principals,
			client.NewRepository(pool), principal.NewClientAccessGrantRepo(pool)),
		operations.UpdateCommand{ID: res.ServiceAccount.ID, Name: &name}, testpg.TestEC())
	require.NoError(t, err)
	got, err = principals.FindByID(context.Background(), ids.PrincipalID(res.PrincipalID))
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

// An account created for one application keeps its client links too. The
// app-scoped path used to persist only the application binding, so a partner
// account came out with no grants and reached no tenant.
func TestServiceAccountReach_AppScopedKeepsClientGrants(t *testing.T) {
	pool := testpg.Pool(t)
	uow := testpg.NewUoW(t)
	principals := principal.NewRepository(pool)
	seedClient(t, pool, "clt_reach_app_a", "reach-app-a")
	seedClient(t, pool, "clt_reach_app_b", "reach-app-b")

	appID := "app_reachscoped"
	res, err := usecaseop.RunTx(testpg.AnchorCtx(), uow,
		operations.CreateServiceAccountWithCredentials(
			serviceaccount.NewRepository(pool), principals,
			platformauth.NewRepository(pool).OAuthClients,
			client.NewRepository(pool), principal.NewClientAccessGrantRepo(pool)),
		operations.CreateCommand{
			Code:          "sareach-appscoped",
			Name:          "sareach-appscoped",
			ApplicationID: &appID,
			ClientIDs:     []string{"clt_reach_app_a", "clt_reach_app_b"},
		}, testpg.TestEC())
	require.NoError(t, err)

	p, err := principals.FindByID(context.Background(), ids.PrincipalID(res.PrincipalID))
	require.NoError(t, err)
	require.NotNil(t, p)
	assert.Equal(t, principal.ScopePartner, p.Scope)
	assert.ElementsMatch(t, []string{"clt_reach_app_a", "clt_reach_app_b"}, ids.Strings(p.AssignedClients))
	assert.False(t, p.AllApplications)
	assert.Equal(t, []string{appID}, ids.Strings(p.AccessibleApplicationIDs))
}

// F28: the stored scope and the issued token tier must agree. PUT scope=ANCHOR
// beside client links was accepted and echoed while the token stayed PARTNER.
func TestServiceAccountScope_MustMatchReach(t *testing.T) {
	pool := testpg.Pool(t)
	uow := testpg.NewUoW(t)
	principals := principal.NewRepository(pool)
	saRepo := serviceaccount.NewRepository(pool)
	seedClient(t, pool, "clt_f28_a", "f28-a")
	seedClient(t, pool, "clt_f28_b", "f28-b")
	str := func(s string) *string { return &s }

	create := func(code string, scope *string, clientIDs []string) error {
		_, err := usecaseop.RunTx(testpg.AnchorCtx(), uow,
			operations.CreateServiceAccountWithCredentials(saRepo, principals,
				platformauth.NewRepository(pool).OAuthClients,
				client.NewRepository(pool), principal.NewClientAccessGrantRepo(pool)),
			operations.CreateCommand{Code: code, Name: code, Scope: scope, ClientIDs: clientIDs}, testpg.TestEC())
		return err
	}
	update := func(id string, cmd operations.UpdateCommand) error {
		cmd.ID = id
		_, err := usecaseop.RunTx(testpg.AnchorCtx(), uow,
			operations.UpdateServiceAccount(saRepo, principals, client.NewRepository(pool), principal.NewClientAccessGrantRepo(pool)),
			cmd, testpg.TestEC())
		return err
	}

	t.Run("create: ANCHOR with client links is refused and nothing is stored", func(t *testing.T) {
		err := create("f28-create-bad", str("ANCHOR"), []string{"clt_f28_a", "clt_f28_b"})
		testpg.RequireUsecaseError(t, err, usecase.KindValidation, "SCOPE_REACH_MISMATCH")
		got, ferr := saRepo.FindByCode(context.Background(), "f28-create-bad")
		require.NoError(t, ferr)
		assert.Nil(t, got)
	})
	t.Run("create: CLIENT with no links is refused (the token would be ANCHOR)", func(t *testing.T) {
		err := create("f28-create-wide", str("CLIENT"), nil)
		testpg.RequireUsecaseError(t, err, usecase.KindValidation, "SCOPE_REACH_MISMATCH")
	})
	t.Run("create: matching scopes are accepted, case-insensitively, and the token tier is that scope", func(t *testing.T) {
		require.NoError(t, create("f28-ok-anchor", str("ANCHOR"), nil))
		require.NoError(t, create("f28-ok-client", str("client"), []string{"clt_f28_a"}))
		require.NoError(t, create("f28-ok-partner", str("PARTNER"), []string{"clt_f28_a", "clt_f28_b"}))
		require.NoError(t, create("f28-ok-unset", nil, []string{"clt_f28_a"}))
	})

	p, res := provisionWithClients(t, "f28-upd", []string{"clt_f28_a", "clt_f28_b"})
	require.Equal(t, principal.ScopePartner, p.Scope)
	tier := func() principal.UserScope {
		got, err := principals.FindByID(context.Background(), ids.PrincipalID(res.PrincipalID))
		require.NoError(t, err)
		return got.Scope
	}
	storedScope := func() *string {
		got, err := saRepo.FindByID(context.Background(), res.ServiceAccount.ID)
		require.NoError(t, err)
		return got.Scope
	}

	t.Run("update: scope ANCHOR while links remain is refused, stored scope and token unchanged", func(t *testing.T) {
		err := update(res.ServiceAccount.ID, operations.UpdateCommand{Scope: str("ANCHOR")})
		testpg.RequireUsecaseError(t, err, usecase.KindValidation, "SCOPE_REACH_MISMATCH")
		assert.Nil(t, storedScope())
		assert.Equal(t, principal.ScopePartner, tier())
	})
	t.Run("update: scope and links together, ANCHOR with none, is accepted and the token follows", func(t *testing.T) {
		require.NoError(t, update(res.ServiceAccount.ID, operations.UpdateCommand{Scope: str("ANCHOR"), ClientIDs: []string{}}))
		require.NotNil(t, storedScope())
		assert.Equal(t, "ANCHOR", *storedScope())
		assert.Equal(t, principal.ScopeAnchor, tier())
	})
	t.Run("update: adding links while the stored scope is ANCHOR is refused", func(t *testing.T) {
		err := update(res.ServiceAccount.ID, operations.UpdateCommand{ClientIDs: []string{"clt_f28_a"}})
		testpg.RequireUsecaseError(t, err, usecase.KindValidation, "SCOPE_REACH_MISMATCH")
		assert.Equal(t, principal.ScopeAnchor, tier(), "a refused update must not widen or narrow the token")
	})
	t.Run("update: a rename does not re-check the scope", func(t *testing.T) {
		name := "renamed"
		require.NoError(t, update(res.ServiceAccount.ID, operations.UpdateCommand{Name: &name}))
	})
	t.Run("update: both together to CLIENT with one link", func(t *testing.T) {
		require.NoError(t, update(res.ServiceAccount.ID, operations.UpdateCommand{Scope: str("CLIENT"), ClientIDs: []string{"clt_f28_b"}}))
		assert.Equal(t, principal.ScopeClient, tier())
		assert.Equal(t, "CLIENT", *storedScope())
	})
}
