//go:build integration

package control

import (
	"bytes"
	"context"
	"log/slog"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/function"
	"github.com/flowcatalyst/flowcatalyst-go/internal/secretref"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
)

// refsFake resolves from a map; a reference absent from it is NOT_FOUND. The
// calls counter shows the document is rendered through the resolver each time
// (which caches in production).
type refsFake struct {
	values map[string]string
	calls  atomic.Int64
}

func (f *refsFake) Resolve(_ context.Context, ref string, _ secretref.Owner) (string, error) {
	f.calls.Add(1)
	v, ok := f.values[ref]
	if !ok {
		return "", &secretref.Error{Code: secretref.CodeNotFound, Ref: ref, Msg: "no such secret"}
	}
	return v, nil
}

// The document carries the resolved value, and its ETag is stable across
// re-renders (the runner long-polls on it: a reference must not make it flip).
func TestDesired_SecretReferenceResolvedAndETagStable(t *testing.T) {
	t.Parallel()
	s := newTestState(t)
	refs := &refsFake{values: map[string]string{"aws-sm://prod/stripe#key": "sk_from_aws"}}
	s.Repo.WithSecretResolver(refs)
	pool := "pool-" + t.Name()
	appID := "app_" + shortID(t)
	appCode := "refapp" + shortID(t)
	seedApplication(t, testpg.Pool(t), appID, appCode)
	fn := seedFunction(t, testpg.Pool(t), s.Repo, appID, appCode, "fn", func(f *function.Function) { f.Pool = &pool })
	seedSetting(t, s.Repo, fn.ID, function.SettingSecret, "STRIPE_KEY", "aws-sm://prod/stripe#key")
	live := seedVersion(t, s.Repo, fn.ID, 1, function.VersionReady, "wasm", describeJSON(t))
	seedAlias(t, s.Repo, fn.ID, "live", live.ID)

	doc1, err := s.buildDesired(context.Background(), pool, wasmJS)
	require.NoError(t, err)
	require.Len(t, doc1.Functions, 1)
	assert.Equal(t, "sk_from_aws", doc1.Functions[0].Secrets["STRIPE_KEY"])
	etag1, err := etagOf(doc1)
	require.NoError(t, err)

	doc2, err := s.buildDesired(context.Background(), pool, wasmJS)
	require.NoError(t, err)
	etag2, err := etagOf(doc2)
	require.NoError(t, err)
	assert.Equal(t, etag1, etag2, "re-rendering with a resolvable reference does not change the ETag")
	assert.GreaterOrEqual(t, refs.calls.Load(), int64(2), "each render goes through the resolver, which caches")
}

// An unresolved reference leaves its key out of the document (the guest's
// secret(...) answers not-set), keeps the function in it, and logs ERROR once
// per (function, key, code), naming the reference and never a value. Not
// parallel: it swaps the process's slog default.
func TestDesired_UnresolvedSecretReferenceLeftOutAndLoggedOnce(t *testing.T) {
	s := newTestState(t)
	const secretValue = "sk_resolved_must_not_be_logged"
	refs := &refsFake{values: map[string]string{"aws-sm://prod/ok": secretValue}}
	s.Repo.WithSecretResolver(refs)
	pool := "pool-" + t.Name()
	appID := "app_" + shortID(t)
	appCode := "unrefapp" + shortID(t)
	seedApplication(t, testpg.Pool(t), appID, appCode)
	fn := seedFunction(t, testpg.Pool(t), s.Repo, appID, appCode, "fn", func(f *function.Function) { f.Pool = &pool })
	seedSetting(t, s.Repo, fn.ID, function.SettingSecret, "GOOD", "aws-sm://prod/ok")
	seedSetting(t, s.Repo, fn.ID, function.SettingSecret, "GONE", "aws-sm://prod/gone")
	seedSetting(t, s.Repo, fn.ID, function.SettingDB, "main", "aws-ps:///missing/dsn")
	live := seedVersion(t, s.Repo, fn.ID, 1, function.VersionReady, "wasm", describeJSON(t))
	seedAlias(t, s.Repo, fn.ID, "live", live.ID)

	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	defer slog.SetDefault(prev)

	for range 3 {
		doc, err := s.buildDesired(context.Background(), pool, wasmJS)
		require.NoError(t, err)
		require.Len(t, doc.Functions, 1, "the function stays in the document")
		got := doc.Functions[0]
		assert.Equal(t, secretValue, got.Secrets["GOOD"])
		assert.NotContains(t, got.Secrets, "GONE")
		assert.NotContains(t, got.DB, "main")
	}

	out := buf.String()
	assert.Equal(t, 1, bytes.Count([]byte(out), []byte("key=GONE")), "ERROR once per (function, key, code) per TTL")
	assert.Equal(t, 1, bytes.Count([]byte(out), []byte("key=main")))
	assert.Contains(t, out, "level=ERROR")
	assert.Contains(t, out, "reference=aws-sm://prod/gone")
	assert.Contains(t, out, "code=SECRET_REFERENCE_NOT_FOUND")
	assert.Contains(t, out, fn.Address)
	assert.NotContains(t, out, secretValue)
}
