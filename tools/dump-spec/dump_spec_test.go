package main

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestOpenAPISpecLocked snapshots the huma-generated OpenAPI spec against
// the committed api/openapi.lock.json. Run `make api-bump` to refresh
// the lockfile when you intentionally change the wire shape.
//
// The CI parity-spec job also runs `make api-diff` for a textual diff
// of the same comparison.
func TestOpenAPISpecLocked(t *testing.T) {
	repoRoot := findRepoRoot(t)
	lockPath := filepath.Join(repoRoot, "api", "openapi.lock.json")

	want, err := os.ReadFile(lockPath)
	require.NoError(t, err, "read lockfile")

	out, err := exec.Command("go", "run", filepath.Join(repoRoot, "tools", "dump-spec")).Output()
	require.NoError(t, err, "dump-spec")

	// Both are JSON; normalise whitespace before compare so a stray
	// newline doesn't fail the snapshot.
	wantNorm := normaliseJSON(t, want)
	gotNorm := normaliseJSON(t, out)
	if !bytes.Equal(wantNorm, gotNorm) {
		t.Fatalf("openapi spec drift — committed lockfile no longer matches code.\n"+
			"Run `make api-bump` and commit the diff (after verifying it's intentional).\n"+
			"Lockfile:  %s\nGenerated: <stdout from `go run ./tools/dump-spec`>",
			lockPath)
	}
}

// TestDumpSpecCoversAllWiredHumaRoutes guards against the lockfile silently
// under-reporting the served API: every aggregate registered against the
// huma API in WirePlatform (any internal/server/*.go file) must also be
// registered here in dump-spec, or its routes won't appear in
// api/openapi.lock.json (and `oasdiff`/parity checks would be blind to
// them). This is exactly how the sdksync + loginattempt routes went
// missing. Compares the set of `<pkg>.Register(humaAPI, …)` idents across
// the server package to `<pkg>.Register(api, …)` idents here.
func TestDumpSpecCoversAllWiredHumaRoutes(t *testing.T) {
	repoRoot := findRepoRoot(t)

	serverFiles, err := filepath.Glob(filepath.Join(repoRoot, "internal", "server", "*.go"))
	require.NoError(t, err, "glob internal/server")
	require.NotEmpty(t, serverFiles, "no files under internal/server — discovery broke")
	var wireSrc []byte
	for _, f := range serverFiles {
		src, err := os.ReadFile(f)
		require.NoError(t, err, "read %s", f)
		wireSrc = append(wireSrc, src...)
		wireSrc = append(wireSrc, '\n')
	}
	dumpSrc, err := os.ReadFile(filepath.Join(repoRoot, "tools", "dump-spec", "main.go"))
	require.NoError(t, err, "read dump-spec/main.go")

	wired := registerIdents(wireSrc, "humaAPI")
	dumped := registerIdents(dumpSrc, "api")
	// An empty wired set means the regex no longer matches how the server
	// registers routes — the guard would pass vacuously. Fail loudly instead.
	require.NotEmpty(t, wired, "no `<pkg>.Register(humaAPI, …)` calls found in internal/server — discovery broke")

	var missing []string
	for pkg := range wired {
		if !dumped[pkg] {
			missing = append(missing, pkg)
		}
	}
	sort.Strings(missing)
	require.Empty(t, missing,
		"these aggregates are registered on the huma API in wire.go but NOT in tools/dump-spec/main.go,\n"+
			"so their routes are missing from api/openapi.lock.json. Add them to dump-spec and run `make api-bump`:\n%v",
		missing)
}

// registerIdents returns the set of package idents X in `X.Register(<apiVar>, …)`.
func registerIdents(src []byte, apiVar string) map[string]bool {
	re := regexp.MustCompile(`(\w+)\.Register\(` + regexp.QuoteMeta(apiVar) + `\b`)
	out := map[string]bool{}
	for _, m := range re.FindAllSubmatch(src, -1) {
		out[string(m[1])] = true
	}
	return out
}

func normaliseJSON(t *testing.T, b []byte) []byte {
	var v any
	require.NoError(t, json.Unmarshal(b, &v))
	out, err := json.Marshal(v)
	require.NoError(t, err)
	return out
}

func findRepoRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	require.NoError(t, err)
	dir := wd
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("could not find go.mod from %s", wd)
		}
		dir = parent
	}
}
