//go:build integration

package keyrotation_test

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/ids"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/keyrotation"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/platformconfig"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/encryption"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
)

func TestMain(m *testing.M) { testpg.RunMain(m) }

type keys struct {
	old, rotated, newOnly *encryption.Service
}

func newKeys(t *testing.T) keys {
	t.Helper()
	oldKey, _ := encryption.GenerateKey()
	newKey, _ := encryption.GenerateKey()
	old, err := encryption.New(oldKey)
	require.NoError(t, err)
	rotated, err := encryption.WithPreviousKeys(newKey, []string{oldKey})
	require.NoError(t, err)
	newOnly, err := encryption.New(newKey)
	require.NoError(t, err)
	return keys{old: old, rotated: rotated, newOnly: newOnly}
}

// must unwraps an encryption result; the inputs are fixed, so an error is a
// broken test setup.
func must(s string, err error) string {
	if err != nil {
		panic(err)
	}
	return s
}

func byTarget(rs []keyrotation.Result) map[string]keyrotation.Result {
	m := map[string]keyrotation.Result{}
	for _, r := range rs {
		m[r.Target] = r
	}
	return m
}

// The real targets: every encrypted column is reachable (the SQL of each
// target runs), values sealed under the old key are moved to the new one in
// the form their readers expect (prefix kept, platform-config SECRETs bound to
// their row), and the new key alone then reads them all.
func TestReencryptMovesStoredSecretsToTheCurrentKey(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	k := newKeys(t)
	n := fmt.Sprintf("%d", seq.Add(1))

	clientID := ids.ClientID("clt_kr" + n)
	aadGlobal := platformconfig.SecretAAD("krapp"+n, "sec", "pw", nil)
	aadClient := platformconfig.SecretAAD("krapp"+n, "sec", "pw", &clientID)
	boundOld := "encrypted:" + must(k.old.EncryptBound("cfg-secret", aadGlobal))
	unboundOld := "encrypted:" + must(k.old.Encrypt("cfg-legacy"))
	_, err := pool.Exec(ctx, `INSERT INTO app_platform_configs (id, application_code, section, property, scope, client_id, value_type, value)
		VALUES ($1, $2, 'sec', 'pw', 'GLOBAL', NULL, 'SECRET', $3),
		       ($4, $2, 'sec', 'pw', 'CLIENT', $5, 'SECRET', $6),
		       ($7, $2, 'sec', 'plain', 'GLOBAL', NULL, 'PLAIN', 'not-a-secret')`,
		"apc_kra"+n, "krapp"+n, boundOld, "apc_krb"+n, string(clientID), unboundOld, "apc_krc"+n)
	require.NoError(t, err)

	tokenOld := "encrypted:" + must(k.old.Encrypt("wh-token"))
	_, err = pool.Exec(ctx, `INSERT INTO iam_service_accounts (id, code, name, wh_auth_token_ref, wh_signing_secret_ref)
		VALUES ($1, $2, 'kr', $3, 'legacy-plaintext-signing-secret')`, "sac_kr"+n, "kr-sa-"+n, tokenOld)
	require.NoError(t, err)

	bareOld := must(k.old.Encrypt("idp-secret"))
	_, err = pool.Exec(ctx, `INSERT INTO oauth_identity_providers (id, code, name, type, oidc_client_secret_ref)
		VALUES ($1, $2, 'kr', 'OIDC', $3), ($4, $5, 'kr', 'OIDC', 'aws-sm://kr/idp')`,
		"oip_kra"+n, "kr-idp-a-"+n, bareOld, "oip_krb"+n, "kr-idp-b-"+n)
	require.NoError(t, err)

	read := func(q, id string) string {
		var v string
		require.NoError(t, pool.QueryRow(ctx, q, id).Scan(&v))
		return v
	}
	cfgValue := func(id string) string { return read(`SELECT value FROM app_platform_configs WHERE id = $1`, id) }
	tokenValue := func() string {
		return read(`SELECT wh_auth_token_ref FROM iam_service_accounts WHERE id = $1`, "sac_kr"+n)
	}
	idpValue := func() string {
		return read(`SELECT oidc_client_secret_ref FROM oauth_identity_providers WHERE id = $1`, "oip_kra"+n)
	}

	// Dry run: counted, nothing written.
	rs, err := keyrotation.Run(ctx, pool, k.rotated, keyrotation.Options{DryRun: true})
	require.NoError(t, err)
	require.Len(t, rs, len(keyrotation.Targets()), "every target runs")
	got := byTarget(rs)
	assert.GreaterOrEqual(t, got["app_platform_configs.value"].Reencrypted, 2)
	assert.GreaterOrEqual(t, got["iam_service_accounts.wh_auth_token_ref"].Reencrypted, 1)
	assert.GreaterOrEqual(t, got["iam_service_accounts.wh_signing_secret_ref"].NotEncrypted, 1)
	assert.GreaterOrEqual(t, got["oauth_identity_providers.oidc_client_secret_ref"].External, 1)
	assert.Equal(t, boundOld, cfgValue("apc_kra"+n))
	assert.Equal(t, tokenOld, tokenValue())
	assert.Equal(t, bareOld, idpValue())

	rs, err = keyrotation.Run(ctx, pool, k.rotated, keyrotation.Options{BatchSize: 2})
	require.NoError(t, err)
	got = byTarget(rs)
	assert.GreaterOrEqual(t, got["app_platform_configs.value"].Reencrypted, 2)
	for _, r := range rs {
		assert.Zero(t, r.Unreadable, r.Target)
	}

	// The new key alone reads everything, in the reader's own form.
	for id, want := range map[string]struct {
		pt  string
		aad []byte
	}{"apc_kra" + n: {"cfg-secret", aadGlobal}, "apc_krb" + n: {"cfg-legacy", aadClient}} {
		v := cfgValue(id)
		require.True(t, strings.HasPrefix(v, "encrypted:"), "prefix kept")
		pt, current, err := k.newOnly.DecryptBoundCurrent(v, want.aad)
		require.NoError(t, err, id)
		assert.Equal(t, want.pt, pt)
		assert.True(t, current, "%s: bound to its row under the new key", id)
	}
	assert.Equal(t, "not-a-secret", cfgValue("apc_krc"+n), "PLAIN rows are never touched")
	require.True(t, strings.HasPrefix(tokenValue(), "encrypted:"))
	assert.Equal(t, "wh-token", must(k.newOnly.Decrypt(tokenValue())))
	assert.Equal(t, "legacy-plaintext-signing-secret",
		read(`SELECT wh_signing_secret_ref FROM iam_service_accounts WHERE id = $1`, "sac_kr"+n))
	require.False(t, strings.HasPrefix(idpValue(), "encrypted:"), "a bare value stays bare")
	assert.Equal(t, "idp-secret", must(k.newOnly.Decrypt(idpValue())))

	// Idempotent: a second run finds nothing left to move.
	rs, err = keyrotation.Run(ctx, pool, k.rotated, keyrotation.Options{})
	require.NoError(t, err)
	for _, r := range rs {
		assert.Zero(t, r.Reencrypted, r.Target)
	}
}

var seq atomic.Int64

// scratch creates a table with a composite primary key and returns a target
// over it.
func scratch(t *testing.T, prefix keyrotation.Prefix) keyrotation.Target {
	t.Helper()
	table := fmt.Sprintf("kr_scratch_%d", seq.Add(1))
	_, err := testpg.Pool(t).Exec(context.Background(),
		`CREATE TABLE `+table+` (a TEXT NOT NULL, b INT NOT NULL, v TEXT, PRIMARY KEY (a, b))`)
	require.NoError(t, err)
	return keyrotation.Target{Table: table, Column: "v", Keys: []string{"a", "b"}, Prefix: prefix}
}

func insert(t *testing.T, table, a string, b int, v *string) {
	t.Helper()
	_, err := testpg.Pool(t).Exec(context.Background(), `INSERT INTO `+table+` (a, b, v) VALUES ($1, $2, $3)`, a, b, v)
	require.NoError(t, err)
}

func value(t *testing.T, table, a string, b int) string {
	t.Helper()
	var v string
	require.NoError(t, testpg.Pool(t).QueryRow(context.Background(),
		`SELECT v FROM `+table+` WHERE a = $1 AND b = $2`, a, b).Scan(&v))
	return v
}

// Every value lands in exactly one bucket, across batch boundaries of a
// composite key, and a rerun only finds what is genuinely stuck.
func TestReencryptClassifiesEveryValue(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	k := newKeys(t)
	tg := scratch(t, keyrotation.PrefixOptional)

	insert(t, tg.Table, "a", 1, new("encrypted:"+must(k.old.Encrypt("one"))))              // old, prefixed
	insert(t, tg.Table, "a", 2, new(must(k.old.Encrypt("two"))))                           // old, bare
	insert(t, tg.Table, "a", 3, new("encrypted:"+must(k.newOnly.Encrypt("3"))))            // already current
	insert(t, tg.Table, "b", 1, new("hashed:v1:"+strings.Repeat("A", 44)))                 // verify-only
	insert(t, tg.Table, "b", 2, new("vault://kv/x#y"))                                     // external
	insert(t, tg.Table, "b", 3, new("encrypted:bm90LWEtY2lwaGVydGV4dC1hdC1hbGwtcmVhbGx5")) // no key opens it
	insert(t, tg.Table, "c", 1, nil)                                                       // skipped
	insert(t, tg.Table, "c", 2, new(""))                                                   // skipped
	insert(t, tg.Table, "c", 3, new("encrypted:"+must(k.old.Encrypt("six"))))              // last, after the batch edge

	rs, err := keyrotation.RunTargets(ctx, pool, k.rotated, []keyrotation.Target{tg}, keyrotation.Options{BatchSize: 2})
	require.NoError(t, err)
	assert.Equal(t, keyrotation.Result{
		Target: tg.Name(), Scanned: 7, Current: 1, Reencrypted: 3, Hashed: 1, External: 1, Unreadable: 1,
	}, rs[0])
	assert.Equal(t, "one", must(k.newOnly.Decrypt(value(t, tg.Table, "a", 1))))
	assert.Equal(t, "two", must(k.newOnly.Decrypt(value(t, tg.Table, "a", 2))))
	assert.Equal(t, "six", must(k.newOnly.Decrypt(value(t, tg.Table, "c", 3))))

	rs, err = keyrotation.RunTargets(ctx, pool, k.rotated, []keyrotation.Target{tg}, keyrotation.Options{BatchSize: 2})
	require.NoError(t, err)
	assert.Equal(t, keyrotation.Result{
		Target: tg.Name(), Scanned: 7, Current: 4, Hashed: 1, External: 1, Unreadable: 1,
	}, rs[0])

	// With PrefixRequired a bare value is plaintext the column predates
	// encryption with: counted, never decrypted or rewritten.
	req := scratch(t, keyrotation.PrefixRequired)
	bare := must(k.old.Encrypt("bare"))
	insert(t, req.Table, "a", 1, new(bare))
	rs, err = keyrotation.RunTargets(ctx, pool, k.rotated, []keyrotation.Target{req}, keyrotation.Options{})
	require.NoError(t, err)
	assert.Equal(t, 1, rs[0].NotEncrypted)
	assert.Equal(t, bare, value(t, req.Table, "a", 1))
}

// A value written between the read and the rewrite (by a process already on
// the new key) is never overwritten: the compare-and-set misses and the row
// keeps the concurrent writer's value.
func TestReencryptNeverOverwritesAConcurrentWrite(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	k := newKeys(t)
	tg := scratch(t, keyrotation.PrefixRequired)
	insert(t, tg.Table, "a", 1, new("encrypted:"+must(k.old.Encrypt("stale"))))
	insert(t, tg.Table, "a", 2, new("encrypted:"+must(k.old.Encrypt("untouched"))))
	concurrent := "encrypted:" + must(k.newOnly.Encrypt("fresh"))

	// The AAD hook runs after the row was read and before it is rewritten:
	// the moment a concurrent writer could step in.
	tg.AADColumns = []string{"b"}
	tg.AAD = func(v []any) ([]byte, error) {
		if v[0] == int32(1) {
			_, err := pool.Exec(ctx, `UPDATE `+tg.Table+` SET v = $1 WHERE a = 'a' AND b = 1`, concurrent)
			require.NoError(t, err)
		}
		return nil, nil
	}
	rs, err := keyrotation.RunTargets(ctx, pool, k.rotated, []keyrotation.Target{tg}, keyrotation.Options{})
	require.NoError(t, err)
	assert.Equal(t, 1, rs[0].Raced)
	assert.Equal(t, 1, rs[0].Reencrypted)
	assert.Equal(t, concurrent, value(t, tg.Table, "a", 1), "the concurrent write survives")
	assert.Equal(t, "untouched", must(k.newOnly.Decrypt(value(t, tg.Table, "a", 2))))
}

func TestReencryptRefusesWithoutAKey(t *testing.T) {
	_, err := keyrotation.Run(context.Background(), testpg.Pool(t), nil, keyrotation.Options{})
	assert.ErrorIs(t, err, encryption.ErrNotConfigured)
}
