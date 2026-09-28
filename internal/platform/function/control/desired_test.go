//go:build integration

package control

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	fncontrol "github.com/flowcatalyst/flowcatalyst-go/internal/functions/control"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/event"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/function"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/function/artifact"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/serviceaccount"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

func describeJSON(t *testing.T, emits ...string) json.RawMessage {
	t.Helper()
	d := map[string]any{
		"abi": 1,
		"endpoints": []map[string]any{
			{"method": "POST", "path": "/events/tick", "auth": "webhook"},
		},
	}
	if len(emits) > 0 {
		d["emits"] = emits
	}
	b, err := json.Marshal(d)
	require.NoError(t, err)
	return b
}

// newTestState builds a State against the shared embedded-PG pool, with a
// fresh service-account credentials cache each call (so a rotated secret in
// one test is never masked by another test's cache).
func newTestState(t *testing.T) *State {
	t.Helper()
	pool := testpg.Pool(t)
	saRepo := serviceaccount.NewRepository(pool)
	store, err := artifact.NewFileStore(t.TempDir())
	require.NoError(t, err)
	return &State{
		Repo:      function.NewRepository(pool),
		Events:    event.NewRepository(pool),
		Artifacts: store,
		Creds:     serviceaccount.NewCachedOutboundCredsResolver(saRepo, time.Millisecond),
		Auth:      fncontrol.TokenAuth{Issuer: "https://issuer.test", Audience: "https://issuer.test"},
	}
}

// TestDesired_RendersFullDocument pins the desired document's shape: config
// and secret/DSN values decrypted, webhookSecret equal to the owning
// application's service-account signing secret, live + candidate roles, a
// RETIRED version excluded, a function in a different pool excluded, and
// Auth populated from State.Auth.
func TestDesired_RendersFullDocument(t *testing.T) {
	t.Parallel()
	s := newTestState(t)
	pool := "pool-" + t.Name()
	appID := "app_" + shortID(t)
	seedApplication(t, testpg.Pool(t), appID, "desiredapp"+shortID(t))

	fn := seedFunction(t, testpg.Pool(t), s.Repo, appID, "desiredapp"+shortID(t), "my-fn", func(f *function.Function) {
		f.Pool = &pool
	})
	seedSetting(t, s.Repo, fn.ID, function.SettingConfig, "GREETING", "hello")
	seedSetting(t, s.Repo, fn.ID, function.SettingSecret, "STRIPE_KEY", "sk_live_secret")
	// "postgres://" looks like an (unsupported) external secret-manager
	// scheme to EncryptSecretRef; "encrypt:" is the documented override to
	// store a plaintext value shaped like a URL as an encrypted secret.
	seedSetting(t, s.Repo, fn.ID, function.SettingDB, "main", "encrypt:postgres://user:pass@host/db")

	live := seedVersion(t, s.Repo, fn.ID, 1, function.VersionReady, "wasm", describeJSON(t))
	seedAlias(t, s.Repo, fn.ID, "live", live.ID)
	seedVersion(t, s.Repo, fn.ID, 2, function.VersionPublished, "wasm", describeJSON(t))
	seedVersion(t, s.Repo, fn.ID, 3, function.VersionRetired, "wasm", describeJSON(t))

	seedServiceAccount(t, testpg.Pool(t), serviceaccount.NewRepository(testpg.Pool(t)), appID, "sa"+shortID(t), "whsec_test123")

	// A function in a different pool must never appear in this pool's
	// document.
	otherPool := "other-" + pool
	seedFunction(t, testpg.Pool(t), s.Repo, appID, "desiredapp"+shortID(t), "other-fn", func(f *function.Function) {
		f.Pool = &otherPool
	})

	doc, err := s.buildDesired(context.Background(), pool)
	require.NoError(t, err)

	assert.Equal(t, pool, doc.Pool)
	assert.Equal(t, fncontrol.TokenAuth{Issuer: "https://issuer.test", Audience: "https://issuer.test"}, doc.Auth)
	require.Len(t, doc.Functions, 1, "only the function in this pool")

	got := doc.Functions[0]
	assert.Equal(t, fn.ID, got.ID)
	assert.Equal(t, fn.Address, got.Address)
	assert.Equal(t, "hello", got.Config["GREETING"])
	assert.Equal(t, "sk_live_secret", got.Secrets["STRIPE_KEY"], "SECRET value must be decrypted, not the ciphertext")
	assert.Equal(t, "postgres://user:pass@host/db", got.DB["main"], "DB DSN must be decrypted")
	assert.Equal(t, "whsec_test123", got.WebhookSecret, "webhookSecret must be the application's SA signing secret")

	byNumber := map[int]fncontrol.Version{}
	for _, v := range got.Versions {
		byNumber[v.Number] = v
	}
	require.Contains(t, byNumber, 1)
	assert.Equal(t, []string{fncontrol.RoleLive}, byNumber[1].Roles)
	require.Contains(t, byNumber, 2)
	assert.Equal(t, []string{fncontrol.RoleCandidate}, byNumber[2].Roles)
	assert.NotContains(t, byNumber, 3, "RETIRED and not aliased must be excluded")
}

// TestDesired_ETagChangesOnSecretRotationWithoutRevisionBump: the ETag is a
// hash of the rendered document, not the pool revision — a rotated
// service-account secret must change it even though nothing calls
// BumpPoolRevision.
func TestDesired_ETagChangesOnSecretRotationWithoutRevisionBump(t *testing.T) {
	t.Parallel()
	s := newTestState(t)
	pool := "pool-" + t.Name()
	appID := "app_" + shortID(t)
	appCode := "rotapp" + shortID(t)
	seedApplication(t, testpg.Pool(t), appID, appCode)
	fn := seedFunction(t, testpg.Pool(t), s.Repo, appID, appCode, "fn", func(f *function.Function) { f.Pool = &pool })
	live := seedVersion(t, s.Repo, fn.ID, 1, function.VersionReady, "wasm", describeJSON(t))
	seedAlias(t, s.Repo, fn.ID, "live", live.ID)

	saRepo := serviceaccount.NewRepository(testpg.Pool(t))
	sa := seedServiceAccount(t, testpg.Pool(t), saRepo, appID, "sa"+shortID(t), "secret-v1")

	revBefore, _, err := s.Repo.GetPoolRevision(context.Background(), pool)
	require.NoError(t, err)

	doc1, err := s.buildDesired(context.Background(), pool)
	require.NoError(t, err)
	etag1, err := etagOf(doc1)
	require.NoError(t, err)

	// Wait out the resolver's cache TTL, then rotate the secret directly —
	// no BumpPoolRevision call anywhere in this test.
	time.Sleep(5 * time.Millisecond)
	sa.WebhookCredentials.SigningSecret = ptr("secret-v2")
	withTx(t, testpg.Pool(t), func(tx *usecasepgx.DbTx) error {
		return saRepo.Persist(context.Background(), sa, tx)
	})

	doc2, err := s.buildDesired(context.Background(), pool)
	require.NoError(t, err)
	etag2, err := etagOf(doc2)
	require.NoError(t, err)

	revAfter, _, err := s.Repo.GetPoolRevision(context.Background(), pool)
	require.NoError(t, err)

	assert.Equal(t, revBefore, revAfter, "no revision bump happened")
	assert.NotEqual(t, etag1, etag2, "a rotated secret must change the ETag even with no revision bump")
	assert.Equal(t, "secret-v2", doc2.Functions[0].WebhookSecret)
}

// ── HTTP-level desired() handler tests ──────────────────────────────────

func TestDesiredHandler_ETagAnd304(t *testing.T) {
	t.Parallel()
	s := newTestState(t)
	pool := "pool-" + t.Name()
	appID := "app_" + shortID(t)
	appCode := "etagapp" + shortID(t)
	seedApplication(t, testpg.Pool(t), appID, appCode)
	fn := seedFunction(t, testpg.Pool(t), s.Repo, appID, appCode, "fn", func(f *function.Function) { f.Pool = &pool })
	live := seedVersion(t, s.Repo, fn.ID, 1, function.VersionReady, "wasm", describeJSON(t))
	seedAlias(t, s.Repo, fn.ID, "live", live.ID)

	ctx := anchorCtx()
	out, err := s.desired(ctx, &desiredInput{Pool: pool})
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, out.Status)
	require.NotEmpty(t, out.ETag)
	require.NotNil(t, out.Body)
	assert.Equal(t, "no-store", out.CacheControl)

	out2, err := s.desired(ctx, &desiredInput{Pool: pool, IfNoneMatch: out.ETag})
	require.NoError(t, err)
	assert.Equal(t, http.StatusNotModified, out2.Status)
	assert.Equal(t, out.ETag, out2.ETag)
	assert.Nil(t, out2.Body)
}

// TestDesiredHandler_WakesOnNotifyWithinASecond proves the long-poll wakes
// promptly on NOTIFY fng_desired rather than waiting for the full `wait`
// duration or the re-render interval.
func TestDesiredHandler_WakesOnNotifyWithinASecond(t *testing.T) {
	t.Parallel()
	pool := testpg.Pool(t)
	s := newTestState(t)
	s.Listener = NewListener(pool, nil)
	s.ReRenderInterval = 10 * time.Second // long enough that only NOTIFY can explain a fast wake

	listenCtx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go s.Listener.Run(listenCtx)
	waitForListener(t, pool)

	poolName := "pool-" + t.Name()
	appID := "app_" + shortID(t)
	appCode := "notifyapp" + shortID(t)
	seedApplication(t, pool, appID, appCode)
	fn := seedFunction(t, pool, s.Repo, appID, appCode, "fn", func(f *function.Function) { f.Pool = &poolName })
	live := seedVersion(t, s.Repo, fn.ID, 1, function.VersionReady, "wasm", describeJSON(t))
	seedAlias(t, s.Repo, fn.ID, "live", live.ID)

	ctx := anchorCtx()
	first, err := s.desired(ctx, &desiredInput{Pool: poolName})
	require.NoError(t, err)

	done := make(chan *desiredOutput, 1)
	go func() {
		out, err := s.desired(ctx, &desiredInput{Pool: poolName, Wait: 30, IfNoneMatch: first.ETag})
		require.NoError(t, err)
		done <- out
	}()

	// Give the goroutine a moment to register its Wait(), then bump the
	// revision (+ NOTIFY) in a separate transaction, as the promote/alias
	// path would.
	time.Sleep(100 * time.Millisecond)
	withRawTx(t, pool, func(tx pgx.Tx) error {
		_, err := s.Repo.BumpPoolRevisionTx(context.Background(), tx, poolName)
		return err
	})

	select {
	case out := <-done:
		// The rendered document embeds the pool's revision (fncontrol.
		// Desired.Revision), so bumping it does change the ETag — 200 with
		// the new document, not 304. What this test actually pins is
		// *promptness*: the wake happens near-instantly on NOTIFY rather
		// than only after ReRenderInterval (set to 10s above) elapses.
		assert.Equal(t, http.StatusOK, out.Status)
		assert.NotEqual(t, first.ETag, out.ETag)
	case <-time.After(1500 * time.Millisecond):
		t.Fatal("desired() did not wake within ~1s of BumpPoolRevision + NOTIFY")
	}
}

func shortID(t *testing.T) string {
	t.Helper()
	h := 0
	for _, r := range t.Name() {
		h = h*31 + int(r)
	}
	if h < 0 {
		h = -h
	}
	return itoaSeq(h % 100000)
}

func ptr[T any](v T) *T { return &v }
