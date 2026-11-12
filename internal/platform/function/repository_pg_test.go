//go:build integration

package function_test

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/function"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/encryption"
	"github.com/flowcatalyst/flowcatalyst-go/internal/secretref"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/go-sdk/usecasepgx"
)

// TestMain sets FLOWCATALYST_APP_KEY before the shared embedded-PG fixture
// boots, same convention as serviceaccount/repository_pg_test.go: the
// SECRET/DB setting-encryption tests need encryption configured, and
// repository.NewRepository resolves the key once at construction.
func TestMain(m *testing.M) {
	key, err := encryption.GenerateKey()
	if err != nil {
		panic(err)
	}
	_ = os.Setenv("FLOWCATALYST_APP_KEY", key)
	testpg.RunMain(m)
}

// seedApplication inserts a bare app_applications row (fng_functions has no
// FK to it — see migration 059's header comment — so this is only needed
// for FindByID/FindWithFilters' read-side application_code join to have
// something to find).
func seedApplication(t *testing.T, pool *pgxpool.Pool, id, code string) {
	t.Helper()
	ctx := context.Background()
	_, err := pool.Exec(ctx, `INSERT INTO app_applications (id, code, name)
		VALUES ($1, $2, $2) ON CONFLICT (id) DO NOTHING`, id, code)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM app_applications WHERE id = $1`, id)
	})
}

// withTx runs fn inside a real transaction, wrapped as a *usecasepgx.DbTx
// via WrapTxForBootstrap — the SDK's documented escape hatch for
// infrastructure/test callers exercising a repository's Persist/Delete
// directly, outside the full use-case envelope (see db_tx.go's doc
// comment). Commits on success, rolls back and fails the test otherwise.
func withTx(t *testing.T, uow *usecasepgx.UnitOfWork, fn func(*usecasepgx.DbTx) error) {
	t.Helper()
	ctx := context.Background()
	tx, err := uow.Pool().Begin(ctx)
	require.NoError(t, err)
	if err := fn(usecasepgx.WrapTxForBootstrap(tx)); err != nil {
		_ = tx.Rollback(ctx)
		require.NoError(t, err)
		return
	}
	require.NoError(t, tx.Commit(ctx))
}

// mustPersist saves f through repo.Persist inside a real transaction — the
// same write path CreateFunction/UpdateFunction use via commit.Save.
func mustPersist(t *testing.T, uow *usecasepgx.UnitOfWork, repo *function.Repository, f *function.Function) {
	t.Helper()
	withTx(t, uow, func(tx *usecasepgx.DbTx) error { return repo.Persist(context.Background(), f, tx) })
}

func TestRepository_Function_RoundTrip(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testpg.Pool(t)
	repo := function.NewRepository(pool)
	uow := testpg.NewUoW(t)

	seedApplication(t, pool, "app_repofnroundtr", "repofnroundtripapp")

	f := function.New("app_repofnroundtr", "repofnroundtripapp", "my-fn")
	client := "cli_repofnround01"
	f.ClientID = &client
	desc := "a test function"
	f.Description = &desc
	poolOverride := "custom-pool"
	f.Pool = &poolOverride
	f.Warm = true
	f.Limits.MemoryMB = 128
	mustPersist(t, uow, repo, f)

	got, err := repo.FindByID(ctx, f.ID)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, f.ID, got.ID)
	assert.Equal(t, "repofnroundtripapp", got.ApplicationCode, "read-side application code must be populated via join")
	assert.Equal(t, client, *got.ClientID)
	assert.Equal(t, desc, *got.Description)
	assert.Equal(t, "custom-pool", *got.Pool)
	assert.True(t, got.Warm)
	assert.Equal(t, int32(128), got.Limits.MemoryMB)
	assert.Equal(t, "repofnroundtripapp.my-fn", got.Address)

	byAddr, err := repo.FindByAddress(ctx, "repofnroundtripapp.my-fn")
	require.NoError(t, err)
	require.NotNil(t, byAddr)
	assert.Equal(t, f.ID, byAddr.ID)

	missing, err := repo.FindByID(ctx, "fnc_doesnotexist1")
	require.NoError(t, err)
	assert.Nil(t, missing)

	// Persist again (update path): description changes, code/address don't.
	newDesc := "updated"
	got.Description = &newDesc
	mustPersist(t, uow, repo, got)
	got2, err := repo.FindByID(ctx, f.ID)
	require.NoError(t, err)
	require.NotNil(t, got2)
	assert.Equal(t, newDesc, *got2.Description)
}

func TestRepository_Function_ListFilters(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testpg.Pool(t)
	repo := function.NewRepository(pool)
	uow := testpg.NewUoW(t)

	seedApplication(t, pool, "app_repofnlist001", "repofnlistapp")

	for _, name := range []string{"aaa", "bbb", "ccc"} {
		mustPersist(t, uow, repo, function.New("app_repofnlist001", "repofnlistapp", name))
	}

	appID := "app_repofnlist001"
	total, err := repo.CountWithFilters(ctx, function.ListFilters{ApplicationID: &appID})
	require.NoError(t, err)
	assert.Equal(t, int64(3), total)

	limit, offset := int64(2), int64(0)
	page1, err := repo.FindWithFilters(ctx, function.ListFilters{ApplicationID: &appID, Limit: &limit, Offset: &offset})
	require.NoError(t, err)
	assert.Len(t, page1, 2)

	prefix := "repofnlistapp.b"
	byPrefix, err := repo.FindWithFilters(ctx, function.ListFilters{AddressPrefix: &prefix})
	require.NoError(t, err)
	require.Len(t, byPrefix, 1)
	assert.Equal(t, "repofnlistapp.bbb", byPrefix[0].Address)

	// AccessibleClientIDs: a platform-owned function (nil client) is
	// visible; a client-scoped one only if its client is in the set.
	client := "cli_repofnlistac1"
	scoped := function.New("app_repofnlist001", "repofnlistapp", "ddd")
	scoped.ClientID = &client
	mustPersist(t, uow, repo, scoped)

	accessible := []string{"cli_someother_id1"}
	visible, err := repo.FindWithFilters(ctx, function.ListFilters{ApplicationID: &appID, AccessibleClientIDs: &accessible})
	require.NoError(t, err)
	for _, v := range visible {
		assert.NotEqual(t, "ddd", v.Name, "a client-scoped function outside the accessible set must not appear")
	}

	accessible = []string{client}
	visible, err = repo.FindWithFilters(ctx, function.ListFilters{ApplicationID: &appID, AccessibleClientIDs: &accessible})
	require.NoError(t, err)
	found := false
	for _, v := range visible {
		if v.Name == "ddd" {
			found = true
		}
	}
	assert.True(t, found, "a client-scoped function in the accessible set must appear")
}

func TestRepository_Function_Delete(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testpg.Pool(t)
	repo := function.NewRepository(pool)
	uow := testpg.NewUoW(t)
	seedApplication(t, pool, "app_repofndelete1", "repofndeleteapp")

	f := function.New("app_repofndelete1", "repofndeleteapp", "doomed")
	mustPersist(t, uow, repo, f)

	withTx(t, uow, func(tx *usecasepgx.DbTx) error { return repo.Delete(ctx, f, tx) })

	got, err := repo.FindByID(ctx, f.ID)
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestRepository_Version_RoundTrip(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testpg.Pool(t)
	repo := function.NewRepository(pool)
	uow := testpg.NewUoW(t)
	seedApplication(t, pool, "app_repofnver0001", "repofnverapp")

	f := function.New("app_repofnver0001", "repofnverapp", "ver-fn")
	mustPersist(t, uow, repo, f)

	v := &function.Version{
		ID:         "fnv_repotest00001",
		FunctionID: f.ID,
		Number:     1,
		Digest:     digest64("a"),
		SizeBytes:  1024,
		ABI:        1,
		Describe:   json.RawMessage(`{"abi":1}`),
		Status:     function.VersionPublished,
		CreatedAt:  time.Now().UTC(),
	}
	require.NoError(t, repo.InsertVersion(ctx, v))

	byNumber, err := repo.GetVersionByNumber(ctx, f.ID, 1)
	require.NoError(t, err)
	require.NotNil(t, byNumber)
	assert.Equal(t, v.Digest, byNumber.Digest)
	assert.Equal(t, function.VersionPublished, byNumber.Status)

	byDigest, err := repo.GetVersionByDigest(ctx, f.ID, v.Digest)
	require.NoError(t, err)
	require.NotNil(t, byDigest)
	assert.Equal(t, v.ID, byDigest.ID)

	readyAt := time.Now().UTC()
	require.NoError(t, repo.SetVersionReady(ctx, v.ID, readyAt))
	got, err := repo.GetVersionByNumber(ctx, f.ID, 1)
	require.NoError(t, err)
	assert.Equal(t, function.VersionReady, got.Status)
	require.NotNil(t, got.ReadyAt)

	require.NoError(t, repo.SetVersionFailure(ctx, v.ID, json.RawMessage(`{"reason":"boom"}`)))
	got, err = repo.GetVersionByNumber(ctx, f.ID, 1)
	require.NoError(t, err)
	assert.Equal(t, function.VersionFailed, got.Status)
	assert.JSONEq(t, `{"reason":"boom"}`, string(got.Failure))

	require.NoError(t, repo.SetVersionStatus(ctx, v.ID, function.VersionRetired))
	got, err = repo.GetVersionByNumber(ctx, f.ID, 1)
	require.NoError(t, err)
	assert.Equal(t, function.VersionRetired, got.Status)

	list, err := repo.ListVersionsByFunction(ctx, f.ID)
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Equal(t, v.ID, list[0].ID)
}

func TestRepository_Alias_RoundTrip(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testpg.Pool(t)
	repo := function.NewRepository(pool)
	uow := testpg.NewUoW(t)
	seedApplication(t, pool, "app_repofnalias01", "repofnaliasapp")

	f := function.New("app_repofnalias01", "repofnaliasapp", "alias-fn")
	mustPersist(t, uow, repo, f)

	v := &function.Version{
		ID: "fnv_repoaliastst1", FunctionID: f.ID, Number: 1,
		Digest: digest64("b"), SizeBytes: 1, ABI: 1, Describe: json.RawMessage(`{}`),
		Status: function.VersionReady, CreatedAt: time.Now().UTC(),
	}
	require.NoError(t, repo.InsertVersion(ctx, v))

	updatedBy := "prn_repoaliastest"
	a := &function.Alias{FunctionID: f.ID, Name: "live", VersionID: v.ID, UpdatedAt: time.Now().UTC(), UpdatedBy: &updatedBy}
	require.NoError(t, repo.UpsertAlias(ctx, a))

	list, err := repo.ListAliases(ctx, f.ID)
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Equal(t, "live", list[0].Name)
	assert.Equal(t, v.ID, list[0].VersionID)

	// Upsert again with a different version — proves ON CONFLICT DO UPDATE.
	v2 := &function.Version{
		ID: "fnv_repoaliastst2", FunctionID: f.ID, Number: 2,
		Digest: digest64("c"), SizeBytes: 1, ABI: 1, Describe: json.RawMessage(`{}`),
		Status: function.VersionReady, CreatedAt: time.Now().UTC(),
	}
	require.NoError(t, repo.InsertVersion(ctx, v2))
	a.VersionID = v2.ID
	require.NoError(t, repo.UpsertAlias(ctx, a))
	list, err = repo.ListAliases(ctx, f.ID)
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Equal(t, v2.ID, list[0].VersionID)

	require.NoError(t, repo.DeleteAlias(ctx, f.ID, "live"))
	list, err = repo.ListAliases(ctx, f.ID)
	require.NoError(t, err)
	assert.Empty(t, list)
}

// TestRepository_Setting_ConfigRoundTrip proves CONFIG values round-trip in
// plaintext.
func TestRepository_Setting_ConfigRoundTrip(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testpg.Pool(t)
	repo := function.NewRepository(pool)
	uow := testpg.NewUoW(t)
	seedApplication(t, pool, "app_reposetcfg001", "reposetcfgapp")

	f := function.New("app_reposetcfg001", "reposetcfgapp", "cfg-fn")
	mustPersist(t, uow, repo, f)

	require.NoError(t, repo.UpsertSetting(ctx, &function.Setting{
		FunctionID: f.ID, Kind: function.SettingConfig, Key: "GREETING", Value: "hello",
	}))

	list, err := repo.ListSettings(ctx, f.ID)
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Equal(t, "hello", list[0].Value, "CONFIG values are returned as stored")

	require.NoError(t, repo.DeleteSetting(ctx, f.ID, function.SettingConfig, "GREETING"))
	list, err = repo.ListSettings(ctx, f.ID)
	require.NoError(t, err)
	assert.Empty(t, list)
}

// TestRepository_Setting_SecretEncryptedAtRestAndNeverListed proves a
// SECRET value is (a) stored encrypted (not the plaintext) and (b) never
// returned by ListSettings — plan §8.2: "Secrets are write-only and never
// returned."
func TestRepository_Setting_SecretEncryptedAtRestAndNeverListed(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testpg.Pool(t)
	repo := function.NewRepository(pool)
	uow := testpg.NewUoW(t)
	seedApplication(t, pool, "app_reposetsec001", "reposetsecapp")

	f := function.New("app_reposetsec001", "reposetsecapp", "sec-fn")
	mustPersist(t, uow, repo, f)

	const plaintext = "sk_live_super_secret_value"
	require.NoError(t, repo.UpsertSetting(ctx, &function.Setting{
		FunctionID: f.ID, Kind: function.SettingSecret, Key: "STRIPE_KEY", Value: plaintext,
	}))

	var stored string
	row := pool.QueryRow(ctx, `SELECT value FROM fng_settings WHERE function_id = $1 AND kind = 'SECRET' AND key = 'STRIPE_KEY'`, f.ID)
	require.NoError(t, row.Scan(&stored))
	assert.NotEqual(t, plaintext, stored, "the raw DB column must not hold the plaintext secret")
	assert.NotContains(t, stored, plaintext)

	list, err := repo.ListSettings(ctx, f.ID)
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Equal(t, function.SettingSecret, list[0].Kind)
	assert.Empty(t, list[0].Value, "ListSettings must never return a SECRET value")
}

func TestRepository_Runner_Heartbeat(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testpg.Pool(t)
	repo := function.NewRepository(pool)

	report := json.RawMessage(`{"loaded":3}`)
	require.NoError(t, repo.UpsertRunnerHeartbeat(ctx, "fnr_repotest00001", "default", report))
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM fng_runners WHERE id = 'fnr_repotest00001'`)
	})

	var gotPool string
	var gotReport json.RawMessage
	row := pool.QueryRow(ctx, `SELECT pool, report FROM fng_runners WHERE id = $1`, "fnr_repotest00001")
	require.NoError(t, row.Scan(&gotPool, &gotReport))
	assert.Equal(t, "default", gotPool)
	assert.JSONEq(t, `{"loaded":3}`, string(gotReport))

	// Upsert again with a different pool — proves ON CONFLICT DO UPDATE.
	require.NoError(t, repo.UpsertRunnerHeartbeat(ctx, "fnr_repotest00001", "other", json.RawMessage(`{"loaded":4}`)))
	row = pool.QueryRow(ctx, `SELECT pool, report FROM fng_runners WHERE id = $1`, "fnr_repotest00001")
	require.NoError(t, row.Scan(&gotPool, &gotReport))
	assert.Equal(t, "other", gotPool)
}

func TestRepository_PoolRevision_BumpAndGet(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testpg.Pool(t)
	repo := function.NewRepository(pool)
	uow := testpg.NewUoW(t)
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM fng_pool_revisions WHERE pool = 'repo-test-pool'`)
	})

	_, ok, err := repo.GetPoolRevision(ctx, "repo-test-pool")
	require.NoError(t, err)
	assert.False(t, ok, "an unbumped pool has no revision row")

	var rev int64
	withTx(t, uow, func(tx *usecasepgx.DbTx) error {
		var err error
		rev, err = repo.BumpPoolRevision(ctx, tx, "repo-test-pool")
		return err
	})
	assert.Equal(t, int64(1), rev)

	got, ok, err := repo.GetPoolRevision(ctx, "repo-test-pool")
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, int64(1), got)

	var rev2 int64
	withTx(t, uow, func(tx *usecasepgx.DbTx) error {
		var err error
		rev2, err = repo.BumpPoolRevision(ctx, tx, "repo-test-pool")
		return err
	})
	assert.Equal(t, int64(2), rev2)
}

// digest64 pads s to a 64-character lowercase-hex-looking digest for test
// fixtures. Not a real sha256 — the DB layer doesn't verify digest
// well-formedness (that's the artifact.Store's job, tested separately).
func digest64(s string) string {
	out := make([]byte, 64)
	for i := range out {
		out[i] = s[0]
	}
	return string(out)
}

// fakeRefs is a secretref.Resolver over a map: a reference absent from it
// fails with its error code.
type fakeRefs struct {
	values map[string]string
	errs   map[string]string // reference -> secretref code
}

func (f fakeRefs) Resolve(_ context.Context, ref string, _ secretref.Owner) (string, error) {
	if code, ok := f.errs[ref]; ok {
		return "", &secretref.Error{Code: code, Ref: ref, Msg: "fake failure"}
	}
	return f.values[ref], nil
}

// TestRepository_ResolveSettingsForControl_SecretReferences proves a SECRET/DB
// value that is an aws-sm:// / aws-ps:// reference is stored verbatim, listed
// as the reference (not blanked), resolved in the desired document's settings,
// and, when it does not resolve, left out and reported (never the reference in
// place of the value).
func TestRepository_ResolveSettingsForControl_SecretReferences(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testpg.Pool(t)
	repo := function.NewRepository(pool).WithSecretResolver(fakeRefs{
		values: map[string]string{
			"aws-sm://prod/stripe#key":      "sk_from_aws",
			"aws-ps:///inhance/np/dsn":      "postgres://u:p@h/db",
			"aws-sm://prod/other#something": "ignored",
		},
		errs: map[string]string{"aws-sm://prod/gone": secretref.CodeNotFound},
	})
	uow := testpg.NewUoW(t)
	seedApplication(t, pool, "app_reporefs0001", "reporefsapp")
	f := function.New("app_reporefs0001", "reporefsapp", "refs-fn")
	mustPersist(t, uow, repo, f)

	for _, s := range []function.Setting{
		{Kind: function.SettingSecret, Key: "STRIPE_KEY", Value: "aws-sm://prod/stripe#key"},
		{Kind: function.SettingSecret, Key: "GONE", Value: "aws-sm://prod/gone"},
		{Kind: function.SettingSecret, Key: "PLAIN", Value: "plain-secret"},
		{Kind: function.SettingDB, Key: "main", Value: "aws-ps:///inhance/np/dsn"},
		{Kind: function.SettingConfig, Key: "GREETING", Value: "hello"},
	} {
		s.FunctionID = f.ID
		require.NoError(t, repo.UpsertSetting(ctx, &s))
	}

	var stored string
	require.NoError(t, pool.QueryRow(ctx, `SELECT value FROM fng_settings WHERE function_id = $1 AND key = 'STRIPE_KEY'`, f.ID).Scan(&stored))
	assert.Equal(t, "aws-sm://prod/stripe#key", stored, "a reference is stored verbatim, never encrypted")

	list, err := repo.ListSettings(ctx, f.ID)
	require.NoError(t, err)
	shown := map[string]string{}
	for _, s := range list {
		shown[s.Key] = s.Value
	}
	assert.Equal(t, "aws-sm://prod/stripe#key", shown["STRIPE_KEY"], "a reference is shown, it is not a secret")
	assert.Empty(t, shown["PLAIN"], "a real secret stays write-only")

	got, err := repo.ResolveSettingsForControl(ctx, f.ID, f.SecretOwner())
	require.NoError(t, err)
	assert.Equal(t, "sk_from_aws", got.Secrets["STRIPE_KEY"])
	assert.Equal(t, "plain-secret", got.Secrets["PLAIN"])
	assert.Equal(t, "postgres://u:p@h/db", got.DB["main"])
	assert.Equal(t, "hello", got.Config["GREETING"])
	assert.NotContains(t, got.Secrets, "GONE", "an unresolved key is left out, not replaced by its reference")
	assert.Equal(t, []function.UnresolvedSetting{
		{Kind: function.SettingSecret, Key: "GONE", Reference: "aws-sm://prod/gone", Code: secretref.CodeNotFound},
	}, got.Unresolved)
}
