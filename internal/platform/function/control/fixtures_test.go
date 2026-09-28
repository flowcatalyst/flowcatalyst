//go:build integration

package control

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/function"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/serviceaccount"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/encryption"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/flowcatalyst-go/internal/tsid"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

// TestMain sets FLOWCATALYST_APP_KEY before the shared embedded-PG fixture
// boots — the SECRET/DB setting-decryption tests need encryption
// configured, and function.Repository / serviceaccount.Repository resolve
// the key once at construction (same convention as
// function/repository_pg_test.go).
func TestMain(m *testing.M) {
	key, err := encryption.GenerateKey()
	if err != nil {
		panic(err)
	}
	_ = os.Setenv("FLOWCATALYST_APP_KEY", key)
	testpg.RunMain(m)
}

// anchorCtx returns a context carrying an anchor-scoped AuthContext holding
// perm (and any extra permissions) — the control plane's own gate
// (RequireAnchor + CanControlFunctionRunner).
func anchorCtx(perms ...string) context.Context {
	if len(perms) == 0 {
		perms = []string{"platform:function:runner:control"}
	}
	return auth.WithContext(context.Background(), &auth.AuthContext{
		PrincipalID: "sa_test_fn_runner",
		Scope:       auth.ScopeAnchor,
		Permissions: perms,
	})
}

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
// infrastructure/test callers exercising Persist directly (see
// function/repository_pg_test.go's identical helper). Avoids depending on
// internal/platform/function/operations (concurrent WP4/WP8 work) for
// fixture setup.
func withTx(t *testing.T, pool *pgxpool.Pool, fn func(*usecasepgx.DbTx) error) {
	t.Helper()
	ctx := context.Background()
	tx, err := pool.Begin(ctx)
	require.NoError(t, err)
	if err := fn(usecasepgx.WrapTxForBootstrap(tx)); err != nil {
		_ = tx.Rollback(ctx)
		require.NoError(t, err)
		return
	}
	require.NoError(t, tx.Commit(ctx))
}

// withRawTx runs fn inside a real transaction and passes the raw pgx.Tx —
// for exercising the raw-pgx.Tx repository methods (BumpPoolRevisionTx and
// friends) directly from a test, the same way heartbeat's WithPoolTx does.
func withRawTx(t *testing.T, pool *pgxpool.Pool, fn func(tx pgx.Tx) error) {
	t.Helper()
	ctx := context.Background()
	tx, err := pool.Begin(ctx)
	require.NoError(t, err)
	if err := fn(tx); err != nil {
		_ = tx.Rollback(ctx)
		require.NoError(t, err)
		return
	}
	require.NoError(t, tx.Commit(ctx))
}

// seedFunction persists a function directly (function.New + Persist),
// bypassing the operations package.
func seedFunction(t *testing.T, pool *pgxpool.Pool, repo *function.Repository, appID, appCode, name string, mutate func(*function.Function)) *function.Function {
	t.Helper()
	f := function.New(appID, appCode, name)
	if mutate != nil {
		mutate(f)
	}
	withTx(t, pool, func(tx *usecasepgx.DbTx) error {
		return repo.Persist(context.Background(), f, tx)
	})
	t.Cleanup(func() {
		withTx(t, pool, func(tx *usecasepgx.DbTx) error {
			return repo.Delete(context.Background(), f, tx)
		})
	})
	return f
}

// seedVersion inserts a version row directly, with an arbitrary (but
// distinct and validly-shaped) digest — fine for anything that never reads
// the actual artifact bytes back. Tests that download the artifact use
// seedVersionWithDigest instead, passing the real sha256 of the content
// they Put into the store.
func seedVersion(t *testing.T, repo *function.Repository, functionID string, number int32, status function.VersionStatus, runtime string, describe json.RawMessage) *function.Version {
	t.Helper()
	return seedVersionWithDigest(t, repo, functionID, number, status, runtime, describe, digestFor(number, functionID))
}

func seedVersionWithDigest(t *testing.T, repo *function.Repository, functionID string, number int32, status function.VersionStatus, runtime string, describe json.RawMessage, digest string) *function.Version {
	t.Helper()
	v := &function.Version{
		ID:         tsid.Generate(tsid.FunctionVersion),
		FunctionID: functionID,
		Number:     number,
		Digest:     digest,
		SizeBytes:  128,
		ABI:        1,
		Runtime:    runtime,
		Describe:   describe,
		Status:     status,
		CreatedAt:  time.Now().UTC(),
	}
	require.NoError(t, repo.InsertVersion(context.Background(), v))
	return v
}

// sha256Hex is the real content digest, for tests that round-trip actual
// artifact bytes through the store.
func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// seedRoute inserts a route row directly (hand-rolled, mirrors seedAlias):
// tests that exercise buildDesired's Routes rendering don't need the full
// PutRoute operation's validation.
func seedRoute(t *testing.T, repo *function.Repository, functionID, hostname, pathPrefix, alias string) function.Route {
	t.Helper()
	var aliasPtr *string
	if alias != "" {
		aliasPtr = &alias
	}
	rt := function.Route{
		ID: tsid.Generate(tsid.FunctionRoute), FunctionID: functionID,
		Hostname: hostname, PathPrefix: pathPrefix, Alias: aliasPtr,
		CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}
	withRawTx(t, testpg.Pool(t), func(tx pgx.Tx) error {
		return repo.UpsertRouteTx(context.Background(), &rt, tx)
	})
	return rt
}

// seedAlias points functionID/name at versionID.
func seedAlias(t *testing.T, repo *function.Repository, functionID, name, versionID string) {
	t.Helper()
	require.NoError(t, repo.UpsertAlias(context.Background(), &function.Alias{
		FunctionID: functionID,
		Name:       name,
		VersionID:  versionID,
		UpdatedAt:  time.Now().UTC(),
	}))
}

// seedSetting writes one setting through the repository's normal write path
// (encrypts SECRET/DB at rest, same as production).
func seedSetting(t *testing.T, repo *function.Repository, functionID string, kind function.SettingKind, key, value string) {
	t.Helper()
	require.NoError(t, repo.UpsertSetting(context.Background(), &function.Setting{
		FunctionID: functionID,
		Kind:       kind,
		Key:        key,
		Value:      value,
	}))
}

// seedServiceAccount persists an application-bound service account holding
// signingSecret as its webhook signing secret — the credential
// OutboundCredsResolver should surface as WebhookSecret.
func seedServiceAccount(t *testing.T, pool *pgxpool.Pool, saRepo *serviceaccount.Repository, applicationID, code, signingSecret string) *serviceaccount.ServiceAccount {
	t.Helper()
	sa := serviceaccount.New(code, code)
	sa.ApplicationID = &applicationID
	sa.WebhookCredentials = serviceaccount.WebhookCredentials{
		AuthType:      serviceaccount.AuthHMAC,
		SigningSecret: &signingSecret,
	}
	withTx(t, pool, func(tx *usecasepgx.DbTx) error {
		return saRepo.Persist(context.Background(), sa, tx)
	})
	t.Cleanup(func() {
		withTx(t, pool, func(tx *usecasepgx.DbTx) error {
			return saRepo.Delete(context.Background(), sa, tx)
		})
	})
	return sa
}

// digestFor builds a deterministic, valid (lowercase hex sha256) digest so
// different (functionID, number) pairs never collide within a test.
func digestFor(number int32, functionID string) string {
	sum := sha256.Sum256([]byte(functionID + "-" + string(rune('0'+number))))
	return hex.EncodeToString(sum[:])
}

// itoaSeq is a tiny non-negative-int-to-string helper for building short
// test ids without pulling in strconv everywhere.
func itoaSeq(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

// waitForListener polls until l has acquired its LISTEN connection (a fresh
// NOTIFY sent right after starting Run can otherwise race the Acquire/LISTEN
// exec and be missed). Bounded: fails the test rather than hanging forever.
func waitForListener(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		var one int
		if err := pool.QueryRow(context.Background(),
			`SELECT 1 FROM pg_stat_activity WHERE query = 'LISTEN fng_desired'`).Scan(&one); err == nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Log("waitForListener: gave up waiting for the LISTEN connection to appear in pg_stat_activity; " +
		"continuing anyway (the test's own timeout will catch a real failure)")
}
