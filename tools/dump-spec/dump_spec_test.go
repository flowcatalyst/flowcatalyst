package main

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
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
