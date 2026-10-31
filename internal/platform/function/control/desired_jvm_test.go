//go:build integration

package control

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/function"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
)

// jvmDescribe is a jvm version's stored describe: the abi document plus the
// jvm block the Java host reads its entrypoint from.
func jvmDescribe(t *testing.T) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(map[string]any{
		"abi":       1,
		"endpoints": []map[string]any{{"method": "GET", "path": "/hello/{name}", "auth": "platform"}},
		"jvm":       map[string]any{"entrypoint": "com.example.HelloFunction"},
	})
	require.NoError(t, err)
	return b
}

// The runtimes filter: a jvm version reaches only a runner whose set includes
// jvm, with its describe (the jvm block) intact; a runner that names no
// runtimes, or only wasm/js, never sees it. A function left with no visible
// version is omitted for that runner.
func TestDesired_JVMVersionsServedOnlyToJVMRunners(t *testing.T) {
	t.Parallel()
	s := newTestState(t)
	pool := "pool-" + t.Name()
	appID := "app_" + shortID(t)
	code := "jvmapp" + shortID(t)
	seedApplication(t, testpg.Pool(t), appID, code)
	fn := seedFunction(t, testpg.Pool(t), s.Repo, appID, code, "jvm-fn", func(f *function.Function) { f.Pool = &pool })
	v := seedVersion(t, s.Repo, fn.ID, 1, function.VersionReady, "jvm", jvmDescribe(t))
	seedAlias(t, s.Repo, fn.ID, "live", v.ID)

	// The in-process Go runner's set, and a runner that sends nothing.
	for _, set := range []map[string]bool{wasmJS, {"wasm": true}} {
		doc, err := s.buildDesired(context.Background(), pool, set)
		require.NoError(t, err)
		assert.Empty(t, doc.Functions, "a jvm-only function is not offered to %v", set)
	}

	jvm, err := parseRuntimes("jvm")
	require.NoError(t, err)
	doc, err := s.buildDesired(context.Background(), pool, jvm)
	require.NoError(t, err)
	require.Len(t, doc.Functions, 1)
	require.Len(t, doc.Functions[0].Versions, 1)
	got := doc.Functions[0].Versions[0]
	assert.Equal(t, "jvm", got.Runtime)
	var d struct {
		JVM struct {
			Entrypoint string `json:"entrypoint"`
		} `json:"jvm"`
	}
	require.NoError(t, json.Unmarshal(got.Describe, &d))
	assert.Equal(t, "com.example.HelloFunction", d.JVM.Entrypoint, "the host reads its entrypoint from the version's describe")

	// A mixed function shows each runner only its runtimes' versions.
	seedVersion(t, s.Repo, fn.ID, 2, function.VersionPublished, "wasm", describeJSON(t))
	doc, err = s.buildDesired(context.Background(), pool, wasmJS)
	require.NoError(t, err)
	require.Len(t, doc.Functions, 1)
	require.Len(t, doc.Functions[0].Versions, 1)
	assert.Equal(t, 2, doc.Functions[0].Versions[0].Number)
}
