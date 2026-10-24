//go:build integration

package ingestion_test

import (
	"context"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/ids"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/client"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/connection"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchpool"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/ingestion"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/serviceaccount"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/encryption"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/flowcatalyst-go/internal/tsid"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecasepgx"
)

// TestMain sets FLOWCATALYST_APP_KEY before the shared fixture boots: the
// resolver tests build service accounts, whose repository needs encryption.
func TestMain(m *testing.M) {
	key, err := encryption.GenerateKey()
	if err != nil {
		panic(err)
	}
	_ = os.Setenv("FLOWCATALYST_APP_KEY", key)
	testpg.RunMain(m)
}

// env is one integration application and one client, unique to a test, so
// tests share the database without seeing each other's rows.
type env struct {
	pool   *pgxpool.Pool
	uow    *usecasepgx.UnitOfWork
	repo   *ingestion.Repository
	app    *application.Application
	client *client.Client
	t0     time.Time // the day this environment polls on (see tBase)
}

var envSeq atomic.Int64

func withTx(t *testing.T, uow *usecasepgx.UnitOfWork, fn func(*usecasepgx.DbTx) error) error {
	t.Helper()
	ctx := context.Background()
	tx, err := uow.Pool().Begin(ctx)
	require.NoError(t, err)
	if err := fn(usecasepgx.WrapTxForBootstrap(tx)); err != nil {
		_ = tx.Rollback(ctx)
		return err
	}
	require.NoError(t, tx.Commit(ctx))
	return nil
}

func unique() string { return strings.ToLower(tsid.GenerateUntyped()) }

func newEnv(t *testing.T) *env {
	t.Helper()
	e := &env{pool: testpg.Pool(t), uow: testpg.NewUoW(t), t0: tBase.AddDate(0, 0, int(envSeq.Add(1)))}
	e.repo = ingestion.NewRepository(e.pool)
	e.app = application.NewIntegration("ing-"+unique(), "Ingest")
	e.client = client.New("Acme", "acme-"+unique())
	require.NoError(t, withTx(t, e.uow, func(tx *usecasepgx.DbTx) error {
		if err := application.NewRepository(e.pool).Persist(context.Background(), e.app, tx); err != nil {
			return err
		}
		return client.NewRepository(e.pool).Persist(context.Background(), e.client, tx)
	}))
	return e
}

func (e *env) save(t *testing.T, i *ingestion.Ingestion) error {
	t.Helper()
	return withTx(t, e.uow, func(tx *usecasepgx.DbTx) error { return e.repo.Persist(context.Background(), i, tx) })
}

func (e *env) mustSave(t *testing.T, i *ingestion.Ingestion) {
	t.Helper()
	require.NoError(t, e.save(t, i))
}

func (e *env) newIngestion(code string, def ingestion.Definition, live bool) *ingestion.Ingestion {
	return ingestion.New(e.app.ID, e.client.ID, code, def, live, nil, nil)
}

func (e *env) mustFind(t *testing.T, id string) *ingestion.Ingestion {
	t.Helper()
	i, err := e.repo.FindByID(context.Background(), id)
	require.NoError(t, err)
	require.NotNil(t, i, "ingestion %s not found", id)
	return i
}

func sqsDef(t *testing.T, name, queue string) ingestion.Definition {
	t.Helper()
	d := parse(t, definition(`{"kind":"sqs","region":"eu-west-1","queue":"`+queue+`"}`, `{"kind":"none"}`))
	d.Name = name
	return d
}

func conflictCode(t *testing.T, err error) string {
	t.Helper()
	ue := usecase.AsError(err)
	require.NotNil(t, ue, "want a usecase error, got %v", err)
	assert.Equal(t, usecase.KindConflict, ue.Kind)
	return ue.Code
}

func TestRepository_RoundTrip(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	def := parse(t, definition(sourceCases[3].source, sourceCases[3].split)) // sftp
	i := e.newIngestion("orders-sftp", def, true)
	e.mustSave(t, i)

	byID := e.mustFind(t, i.ID)
	assert.Equal(t, e.app.ID, byID.ApplicationID)
	assert.Equal(t, e.client.ID, byID.ClientID)
	assert.Equal(t, "orders-sftp", byID.Code)
	assert.True(t, byID.Active)
	assert.Equal(t, 1, byID.Version)
	assert.Equal(t, 1, byID.LatestVersion)
	assert.Equal(t, def, byID.Definition, "source, split and target survive the JSONB round trip")

	byCode, err := e.repo.FindByCode(ctx, e.app.ID, e.client.ID, "orders-sftp")
	require.NoError(t, err)
	require.NotNil(t, byCode)
	assert.Equal(t, i.ID, byCode.ID)

	missing, err := e.repo.FindByCode(ctx, e.app.ID, e.client.ID, "nope")
	require.NoError(t, err)
	assert.Nil(t, missing, "absence is (nil, nil)")
	gone, err := e.repo.FindByID(ctx, "ing_0000000000000")
	require.NoError(t, err)
	assert.Nil(t, gone)

	versions, err := e.repo.ListVersions(ctx, i.ID)
	require.NoError(t, err)
	require.Len(t, versions, 1)
	assert.Equal(t, 1, versions[0].Number)
	assert.Equal(t, def, versions[0].Definition)
}

func TestRepository_CodeIsUniquePerApplicationAndClient(t *testing.T) {
	e := newEnv(t)
	def := sqsDef(t, "Orders", "orders")
	e.mustSave(t, e.newIngestion("orders", def, true))

	err := e.save(t, e.newIngestion("orders", def, true))
	assert.Equal(t, "INGESTION_CODE_EXISTS", conflictCode(t, err))

	// The same code under another client, or another application, is another ingestion.
	other := newEnv(t)
	otherClient := ingestion.New(e.app.ID, other.client.ID, "orders", def, true, nil, nil)
	e.mustSave(t, otherClient)
	otherApp := ingestion.New(other.app.ID, e.client.ID, "orders", def, true, nil, nil)
	e.mustSave(t, otherApp)
}

func TestRepository_ConcurrentUpdateIsAConflictNotALostUpdate(t *testing.T) {
	e := newEnv(t)
	a := sqsDef(t, "Orders", "orders")
	i := e.newIngestion("orders", a, true)
	e.mustSave(t, i)

	first, second := e.mustFind(t, i.ID), e.mustFind(t, i.ID)
	first.SaveVersion(sqsDef(t, "Orders v2", "orders-2"), true, nil, nil)
	e.mustSave(t, first)

	second.SaveVersion(sqsDef(t, "Orders other v2", "orders-3"), true, nil, nil)
	assert.Equal(t, "INGESTION_VERSION_CONFLICT", conflictCode(t, e.save(t, second)))

	// Nothing of the losing write is stored: the row is the winner's, and
	// there is no second v2.
	got := e.mustFind(t, i.ID)
	assert.Equal(t, "Orders v2", got.Definition.Name)
	assert.Equal(t, 2, got.Version)
	versions, err := e.repo.ListVersions(context.Background(), i.ID)
	require.NoError(t, err)
	assert.Len(t, versions, 2)
}

func TestRepository_PauseAfterAConcurrentPromoteConflicts(t *testing.T) {
	e := newEnv(t)
	i := e.newIngestion("orders", sqsDef(t, "Orders", "orders"), true)
	e.mustSave(t, i)

	pauser, saver := e.mustFind(t, i.ID), e.mustFind(t, i.ID)
	saver.SaveVersion(sqsDef(t, "Orders v2", "orders-2"), false, nil, nil)
	e.mustSave(t, saver) // moves latest_version

	pauser.Pause()
	assert.Equal(t, "INGESTION_VERSION_CONFLICT", conflictCode(t, e.save(t, pauser)))
	assert.True(t, e.mustFind(t, i.ID).Active)
}

func TestRepository_VersionsAndTheLivePointer(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	v1def, v2def := sqsDef(t, "Orders", "orders"), sqsDef(t, "Orders v2", "orders-2")
	i := e.newIngestion("orders", v1def, true)
	e.mustSave(t, i)

	// A saved version is stored but not run.
	cur := e.mustFind(t, i.ID)
	note := "try the new queue"
	cur.SaveVersion(v2def, false, &note, nil)
	e.mustSave(t, cur)

	got := e.mustFind(t, i.ID)
	assert.Equal(t, 1, got.Version, "live did not move")
	assert.Equal(t, 2, got.LatestVersion)
	assert.Equal(t, v1def, got.Definition)

	v2, err := e.repo.FindVersion(ctx, i.ID, 2)
	require.NoError(t, err)
	require.NotNil(t, v2)
	assert.Equal(t, v2def, v2.Definition)
	require.NotNil(t, v2.Note)
	assert.Equal(t, note, *v2.Note)
	none, err := e.repo.FindVersion(ctx, i.ID, 3)
	require.NoError(t, err)
	assert.Nil(t, none)

	// Make it live, then roll back to the first.
	require.NoError(t, got.MakeLive(*v2))
	e.mustSave(t, got)
	got = e.mustFind(t, i.ID)
	assert.Equal(t, 2, got.Version)
	assert.Equal(t, v2def, got.Definition)

	v1, err := e.repo.FindVersion(ctx, i.ID, 1)
	require.NoError(t, err)
	require.NoError(t, got.MakeLive(*v1))
	e.mustSave(t, got)
	got = e.mustFind(t, i.ID)
	assert.Equal(t, 1, got.Version)
	assert.Equal(t, 2, got.LatestVersion)
	assert.Equal(t, v1def, got.Definition)

	versions, err := e.repo.ListVersions(ctx, i.ID)
	require.NoError(t, err)
	require.Len(t, versions, 2)
	assert.Equal(t, []int{2, 1}, []int{versions[0].Number, versions[1].Number}, "newest first")
}

func TestRepository_CreatedNotLiveHasNoLiveVersion(t *testing.T) {
	e := newEnv(t)
	def := sqsDef(t, "Orders", "orders")
	i := e.newIngestion("orders", def, false)
	e.mustSave(t, i)

	got := e.mustFind(t, i.ID)
	assert.Equal(t, 0, got.Version)
	assert.Equal(t, 1, got.LatestVersion)
	assert.False(t, got.Live())
	assert.Equal(t, def, got.Definition)

	v1, err := e.repo.FindVersion(context.Background(), i.ID, 1)
	require.NoError(t, err)
	require.NoError(t, got.MakeLive(*v1))
	e.mustSave(t, got)
	assert.True(t, e.mustFind(t, i.ID).Live())
}

func TestRepository_DeleteTakesTheVersionsWithIt(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	i := e.newIngestion("orders", sqsDef(t, "Orders", "orders"), true)
	e.mustSave(t, i)

	require.NoError(t, withTx(t, e.uow, func(tx *usecasepgx.DbTx) error { return e.repo.Delete(ctx, i, tx) }))
	gone, err := e.repo.FindByID(ctx, i.ID)
	require.NoError(t, err)
	assert.Nil(t, gone)
	versions, err := e.repo.ListVersions(ctx, i.ID)
	require.NoError(t, err)
	assert.Empty(t, versions)
}

func TestRepository_CorruptDefinitionFailsTheReadLoudly(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	i := e.newIngestion("orders", sqsDef(t, "Orders", "orders"), true)
	e.mustSave(t, i)
	_, err := e.pool.Exec(ctx, `UPDATE ing_ingestions SET source = '{"kind":"ftp"}' WHERE id = $1`, i.ID)
	require.NoError(t, err)

	got, err := e.repo.FindByID(ctx, i.ID)
	require.Error(t, err, "a row that does not parse must not read as something else")
	assert.Nil(t, got)
	assert.Contains(t, err.Error(), "CORRUPT_INGESTION")
}

func TestRepository_ListPages(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	var want []string
	for _, code := range []string{"a1", "a2", "a3", "a4", "a5"} {
		i := e.newIngestion(code, sqsDef(t, "Orders "+code, "q-"+code), true)
		e.mustSave(t, i)
		want = append(want, i.ID)
	}
	clientID := string(e.client.ID)

	var got []string
	after := ""
	pages := 0
	for {
		page, err := e.repo.List(ctx, ingestion.ListFilter{ClientID: &clientID, Limit: 2, After: after})
		require.NoError(t, err)
		pages++
		for _, item := range page.Items {
			got = append(got, item.ID)
			assert.Equal(t, e.app.Code, item.ApplicationCode)
			assert.Equal(t, e.client.Identifier, item.ClientIdentifier)
		}
		if page.Next == "" {
			break
		}
		assert.Len(t, page.Items, 2, "a page with a next page is full")
		after = page.Next
	}
	assert.Equal(t, 3, pages)
	assert.ElementsMatch(t, want, got)
	assert.Len(t, got, 5, "no row twice, none missed")
	assert.IsIncreasing(t, got, "ordered by id")

	exact, err := e.repo.List(ctx, ingestion.ListFilter{ClientID: &clientID, Limit: 5})
	require.NoError(t, err)
	assert.Len(t, exact.Items, 5)
	assert.Empty(t, exact.Next, "a page that ends exactly at the last row has no next")
}

func TestRepository_ListFilters(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	otherClient := client.New("Other", "other-"+unique())
	require.NoError(t, withTx(t, e.uow, func(tx *usecasepgx.DbTx) error {
		return client.NewRepository(e.pool).Persist(ctx, otherClient, tx)
	}))

	sftp := parse(t, definition(sourceCases[3].source, sourceCases[3].split))
	sftp.Name = "Warehouse files"
	mix := parse(t, definition(sourceCases[12].source, sourceCases[12].split)) // custom mix
	mix.Name = "Fleet positions"
	sqs := sqsDef(t, "Order 100% queue", "orders")

	ingSftp := e.newIngestion("wh-files", sftp, true)
	ingMix := e.newIngestion("fleet", mix, true)
	ingSqs := e.newIngestion("order-queue", sqs, true)
	paused := e.newIngestion("paused-one", sqsDef(t, "Paused", "paused"), true)
	paused.Pause()
	elsewhere := ingestion.New(e.app.ID, otherClient.ID, "elsewhere", sqs, true, nil, nil)
	for _, i := range []*ingestion.Ingestion{ingSftp, ingMix, ingSqs, paused, elsewhere} {
		e.mustSave(t, i)
	}

	ids := func(f ingestion.ListFilter) []string {
		f.ApplicationCode = &e.app.Code // every case stays inside this test's application
		page, err := e.repo.List(ctx, f)
		require.NoError(t, err)
		out := make([]string, len(page.Items))
		for i, item := range page.Items {
			out[i] = item.Code
		}
		return out
	}
	str := func(s string) *string { return &s }
	mine := str(string(e.client.ID))

	assert.ElementsMatch(t, []string{"wh-files", "fleet", "order-queue", "paused-one", "elsewhere"}, ids(ingestion.ListFilter{}))
	assert.ElementsMatch(t, []string{"elsewhere"}, ids(ingestion.ListFilter{ClientID: str(string(otherClient.ID))}))
	assert.ElementsMatch(t, []string{"wh-files", "fleet", "order-queue", "paused-one"}, ids(ingestion.ListFilter{ClientID: mine}))

	t.Run("application code", func(t *testing.T) {
		page, err := e.repo.List(ctx, ingestion.ListFilter{ApplicationCode: str("no-such-app")})
		require.NoError(t, err)
		assert.Empty(t, page.Items)
	})
	t.Run("q matches code or name, ignoring case", func(t *testing.T) {
		assert.ElementsMatch(t, []string{"wh-files"}, ids(ingestion.ListFilter{Q: str("WAREHOUSE")}))
		assert.ElementsMatch(t, []string{"fleet"}, ids(ingestion.ListFilter{Q: str("fleet")}))
		assert.ElementsMatch(t, []string{"order-queue"}, ids(ingestion.ListFilter{Q: str("order-q")}))
	})
	t.Run("q is not a pattern", func(t *testing.T) {
		assert.ElementsMatch(t, []string{"order-queue", "elsewhere"}, ids(ingestion.ListFilter{Q: str("100%")}))
		assert.ElementsMatch(t, []string{"order-queue", "elsewhere"}, ids(ingestion.ListFilter{Q: str("%")}), "% is a literal, not match-all")
		assert.Empty(t, ids(ingestion.ListFilter{Q: str("wh_files")}), "_ is not a wildcard")
	})
	t.Run("status", func(t *testing.T) {
		assert.ElementsMatch(t, []string{"paused-one"}, ids(ingestion.ListFilter{Status: str("PAUSED")}))
		assert.NotContains(t, ids(ingestion.ListFilter{Status: str("ACTIVE")}), "paused-one")
		assert.Len(t, ids(ingestion.ListFilter{Status: str("ACTIVE")}), 4)
		_, err := e.repo.List(ctx, ingestion.ListFilter{Status: str("RUNNING")})
		require.Error(t, err)
		assert.Equal(t, usecase.KindValidation, usecase.AsError(err).Kind)
	})
	t.Run("kind", func(t *testing.T) {
		assert.ElementsMatch(t, []string{"wh-files"}, ids(ingestion.ListFilter{Kind: str("sftp")}))
		assert.ElementsMatch(t, []string{"order-queue", "paused-one", "elsewhere"}, ids(ingestion.ListFilter{Kind: str("sqs")}))
		assert.ElementsMatch(t, []string{"fleet"}, ids(ingestion.ListFilter{Kind: str("custom")}))
		assert.ElementsMatch(t, []string{"fleet"}, ids(ingestion.ListFilter{Kind: str("mix")}), "a custom source matches by its type")
	})
	t.Run("reach", func(t *testing.T) {
		reach := []string{string(otherClient.ID)}
		assert.ElementsMatch(t, []string{"elsewhere"}, ids(ingestion.ListFilter{AccessibleClientIDs: &reach}))
		none := []string{}
		assert.Empty(t, ids(ingestion.ListFilter{AccessibleClientIDs: &none}), "an empty reach sees nothing")
	})
	t.Run("filters combine", func(t *testing.T) {
		assert.ElementsMatch(t, []string{"order-queue"}, ids(ingestion.ListFilter{ClientID: mine, Kind: str("sqs"), Status: str("ACTIVE")}))
	})
}

// resolver ----------------------------------------------------------------

type resolverEnv struct {
	*env
	resolver *ingestion.RepoResolver
	pools    *dispatchpool.Repository
}

func newResolverEnv(t *testing.T) *resolverEnv {
	e := newEnv(t)
	pools := dispatchpool.NewRepository(e.pool)
	return &resolverEnv{
		env: e,
		resolver: ingestion.NewResolver(application.NewRepository(e.pool), client.NewRepository(e.pool), pools,
			connection.NewRepository(e.pool), serviceaccount.NewRepository(e.pool), nil),
		pools: pools,
	}
}

func (r *resolverEnv) makePool(t *testing.T, code string, clientID *ids.ClientID, mutate func(*dispatchpool.DispatchPool)) *dispatchpool.DispatchPool {
	p := dispatchpool.New(code, code)
	p.ClientID = clientID
	if mutate != nil {
		mutate(p)
	}
	require.NoError(t, withTx(t, r.uow, func(tx *usecasepgx.DbTx) error { return r.pools.Persist(context.Background(), p, tx) }))
	return p
}

func (r *resolverEnv) serviceAccount(t *testing.T, mutate func(*serviceaccount.ServiceAccount)) *serviceaccount.ServiceAccount {
	sa := serviceaccount.New("sa-"+unique(), "Signer")
	if mutate != nil {
		mutate(sa)
	}
	require.NoError(t, withTx(t, r.uow, func(tx *usecasepgx.DbTx) error {
		return serviceaccount.NewRepository(r.pool).Persist(context.Background(), sa, tx)
	}))
	return sa
}

func (r *resolverEnv) connection(t *testing.T, code string, appCode, clientID *string, saID string) *connection.Connection {
	c := connection.New(code, code, saID)
	c.ApplicationCode = appCode
	if clientID != nil {
		cid := ids.ClientID(*clientID)
		c.ClientID = &cid
	}
	require.NoError(t, withTx(t, r.uow, func(tx *usecasepgx.DbTx) error {
		return connection.NewRepository(r.pool).Persist(context.Background(), c, tx)
	}))
	return c
}

func target(pool string) ingestion.Target {
	return ingestion.Target{
		Code: "acme:orders:order:received", TargetURL: "https://hooks.example.com/orders",
		Pool: pool, Mode: ingestion.ModeImmediate, BatchSize: 100,
	}
}

func invalidTarget(t *testing.T, err error, contains string) {
	t.Helper()
	ue := usecase.AsError(err)
	require.NotNil(t, ue, "want a usecase error, got %v", err)
	assert.Equal(t, usecase.KindValidation, ue.Kind)
	assert.Equal(t, "INGESTION_TARGET_INVALID", ue.Code)
	assert.Contains(t, ue.Message, contains)
}

func TestResolver_Application(t *testing.T) {
	ctx := context.Background()
	r := newResolverEnv(t)

	app, err := r.resolver.Application(ctx, r.app.Code)
	require.NoError(t, err)
	assert.Equal(t, r.app.ID, app.ID)

	_, err = r.resolver.Application(ctx, "no-such-app-"+unique())
	assert.Equal(t, "INGESTION_APPLICATION_INVALID", usecase.AsError(err).Code)

	plain := application.New("plain-"+unique(), "Plain")
	require.NoError(t, withTx(t, r.uow, func(tx *usecasepgx.DbTx) error {
		return application.NewRepository(r.pool).Persist(ctx, plain, tx)
	}))
	_, err = r.resolver.Application(ctx, plain.Code)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not of type INTEGRATION")
}

func TestResolver_Client(t *testing.T) {
	ctx := context.Background()
	r := newResolverEnv(t)

	byID, err := r.resolver.Client(ctx, string(r.client.ID))
	require.NoError(t, err)
	assert.Equal(t, r.client.ID, byID.ID)
	byIdentifier, err := r.resolver.Client(ctx, r.client.Identifier)
	require.NoError(t, err)
	assert.Equal(t, r.client.ID, byIdentifier.ID)

	_, err = r.resolver.Client(ctx, "nobody-"+unique())
	assert.Equal(t, "INGESTION_CLIENT_INVALID", usecase.AsError(err).Code)
}

func TestResolver_TargetResolvesCodesToIds(t *testing.T) {
	ctx := context.Background()
	r := newResolverEnv(t)
	cid := r.client.ID
	pool := r.makePool(t, "pool-"+unique(), &cid, nil)
	sa := r.serviceAccount(t, nil)
	conn := r.connection(t, "conn-"+unique(), &r.app.Code, nil, sa.ID)

	tg := target(pool.Code)
	got, err := r.resolver.Target(ctx, r.app.Code, r.client.ID, tg)
	require.NoError(t, err)
	assert.Equal(t, pool.ID, got.PoolID)
	assert.Nil(t, got.ConnectionID)
	assert.Nil(t, got.ServiceAccountID)

	tg.Connection = &conn.Code
	got, err = r.resolver.Target(ctx, r.app.Code, r.client.ID, tg)
	require.NoError(t, err)
	require.NotNil(t, got.ConnectionID)
	assert.Equal(t, conn.ID, *got.ConnectionID)

	tg.Connection, tg.ServiceAccount = nil, &sa.Code
	got, err = r.resolver.Target(ctx, r.app.Code, r.client.ID, tg)
	require.NoError(t, err)
	require.NotNil(t, got.ServiceAccountID)
	assert.Equal(t, sa.ID, *got.ServiceAccountID)
}

func TestResolver_PoolOfNoClientServesEveryClient(t *testing.T) {
	r := newResolverEnv(t)
	shared := r.makePool(t, "shared-"+unique(), nil, nil)
	got, err := r.resolver.Target(context.Background(), r.app.Code, r.client.ID, target(shared.Code))
	require.NoError(t, err)
	assert.Equal(t, shared.ID, got.PoolID)
}

func TestResolver_PoolErrors(t *testing.T) {
	ctx := context.Background()
	r := newResolverEnv(t)

	_, err := r.resolver.Target(ctx, r.app.Code, r.client.ID, target("no-such-pool-"+unique()))
	invalidTarget(t, err, "no dispatch pool with code")

	other := newEnv(t)
	theirs := r.makePool(t, "theirs-"+unique(), &other.client.ID, nil)
	_, err = r.resolver.Target(ctx, r.app.Code, r.client.ID, target(theirs.Code))
	invalidTarget(t, err, "no dispatch pool with code")

	cid := r.client.ID
	suspended := r.makePool(t, "susp-"+unique(), &cid, func(p *dispatchpool.DispatchPool) { p.Suspend() })
	_, err = r.resolver.Target(ctx, r.app.Code, r.client.ID, target(suspended.Code))
	invalidTarget(t, err, "suspended")
}

func TestResolver_ConnectionAndServiceAccountTogether(t *testing.T) {
	r := newResolverEnv(t)
	cid := r.client.ID
	pool := r.makePool(t, "pool-"+unique(), &cid, nil)
	sa := r.serviceAccount(t, nil)
	conn := r.connection(t, "conn-"+unique(), nil, nil, sa.ID)

	tg := target(pool.Code)
	tg.Connection, tg.ServiceAccount = &conn.Code, &sa.Code
	_, err := r.resolver.Target(context.Background(), r.app.Code, r.client.ID, tg)
	invalidTarget(t, err, "may not be set with a connection")
}

func TestResolver_BadURLs(t *testing.T) {
	r := newResolverEnv(t)
	cid := r.client.ID
	pool := r.makePool(t, "pool-"+unique(), &cid, nil)
	for _, url := range []string{
		"http://169.254.169.254/latest/meta-data",
		"http://127.0.0.1:8080/hook",
		"http://10.1.2.3/hook",
		"http://localhost/hook",
		"https://user:pw@hooks.example.com/x",
		"ftp://hooks.example.com/x",
		"not a url",
	} {
		t.Run(url, func(t *testing.T) {
			tg := target(pool.Code)
			tg.TargetURL = url
			_, err := r.resolver.Target(context.Background(), r.app.Code, r.client.ID, tg)
			invalidTarget(t, err, "target.targetUrl")
		})
	}
}

func TestResolver_ConnectionLookup(t *testing.T) {
	ctx := context.Background()
	r := newResolverEnv(t)
	cid := r.client.ID
	pool := r.makePool(t, "pool-"+unique(), &cid, nil)
	sa := r.serviceAccount(t, nil)

	tg := target(pool.Code)
	missing := "no-such-conn-" + unique()
	tg.Connection = &missing
	_, err := r.resolver.Target(ctx, r.app.Code, r.client.ID, tg)
	invalidTarget(t, err, "no connection with code")

	// A connection of another application or of another client is not usable.
	otherApp, otherClient := "other-app-"+unique(), string(newEnv(t).client.ID)
	foreignApp := r.connection(t, "fa-"+unique(), &otherApp, nil, sa.ID)
	tg.Connection = &foreignApp.Code
	_, err = r.resolver.Target(ctx, r.app.Code, r.client.ID, tg)
	invalidTarget(t, err, "no connection with code")
	foreignClient := r.connection(t, "fc-"+unique(), nil, &otherClient, sa.ID)
	tg.Connection = &foreignClient.Code
	_, err = r.resolver.Target(ctx, r.app.Code, r.client.ID, tg)
	invalidTarget(t, err, "no connection with code")

	// A shared connection (no application, no client) is.
	shared := r.connection(t, "sh-"+unique(), nil, nil, sa.ID)
	tg.Connection = &shared.Code
	got, err := r.resolver.Target(ctx, r.app.Code, r.client.ID, tg)
	require.NoError(t, err)
	assert.Equal(t, shared.ID, *got.ConnectionID)
}

func TestResolver_ServiceAccountErrors(t *testing.T) {
	ctx := context.Background()
	r := newResolverEnv(t)
	cid := r.client.ID
	pool := r.makePool(t, "pool-"+unique(), &cid, nil)
	tg := target(pool.Code)

	missing := "no-such-sa-" + unique()
	tg.ServiceAccount = &missing
	_, err := r.resolver.Target(ctx, r.app.Code, r.client.ID, tg)
	invalidTarget(t, err, "no service account with code")

	inactive := r.serviceAccount(t, func(sa *serviceaccount.ServiceAccount) { sa.Deactivate() })
	tg.ServiceAccount = &inactive.Code
	_, err = r.resolver.Target(ctx, r.app.Code, r.client.ID, tg)
	invalidTarget(t, err, "not active")

	other := newEnv(t)
	narrow := r.serviceAccount(t, func(sa *serviceaccount.ServiceAccount) { sa.ClientIDs = []string{string(other.client.ID)} })
	tg.ServiceAccount = &narrow.Code
	_, err = r.resolver.Target(ctx, r.app.Code, r.client.ID, tg)
	invalidTarget(t, err, "does not reach this client")

	reaching := r.serviceAccount(t, func(sa *serviceaccount.ServiceAccount) { sa.ClientIDs = []string{string(r.client.ID)} })
	tg.ServiceAccount = &reaching.Code
	_, err = r.resolver.Target(ctx, r.app.Code, r.client.ID, tg)
	require.NoError(t, err)
}
