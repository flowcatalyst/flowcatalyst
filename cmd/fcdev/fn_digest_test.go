package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestFnBuildIsReproducible proves the property CI relies on: two builds of
// the same function source give the same bundle bytes, hence the same digest
// (docs/fcdev-profiles.md "What changes the digest"). It runs the example's
// own pinned esbuild with build.mjs's settings, outputting to a temp dir so
// the tree is untouched. Skipped when node or the example's deps are absent.
func TestFnBuildIsReproducible(t *testing.T) {
	proj, err := filepath.Abs("../../examples/functions/dispatch-demo/payload-logger")
	require.NoError(t, err)
	esbuild := filepath.Join(proj, "node_modules", ".bin", "esbuild")
	if _, err := os.Stat(esbuild); err != nil {
		t.Skipf("skipping: %s is missing (run `npm ci` in %s to enable this check)", esbuild, proj)
	}
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("skipping: node is not on PATH")
	}
	build := func(out string) []byte {
		t.Helper()
		// #nosec G204 -- the repo's own pinned esbuild, fixed arguments.
		cmd := exec.Command(esbuild, "src/index.ts", "--bundle", "--format=iife", "--platform=neutral",
			"--target=es2020", `--define:__APP_CODE__="dispatchdemo"`, "--outfile="+out, "--log-level=warning")
		cmd.Dir = proj
		if b, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("esbuild: %v\n%s", err, b)
		}
		data, err := os.ReadFile(out)
		require.NoError(t, err)
		return data
	}
	dir := t.TempDir()
	a := build(filepath.Join(dir, "one", "function.js"))
	b := build(filepath.Join(dir, "two", "deeper", "function.js"))
	require.Equal(t, digestHex(a), digestHex(b), "two builds of the same source must give the same digest")
	for _, leak := range []string{dir, proj, os.Getenv("HOME")} {
		if len(leak) < 2 {
			continue
		}
		require.False(t, bytes.Contains(a, []byte(leak)), "the bundle embeds the absolute path %q", leak)
	}
}
