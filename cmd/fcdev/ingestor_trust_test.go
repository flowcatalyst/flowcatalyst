package main

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// captureSlog routes slog to a buffer for the test.
func captureSlog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	old := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	t.Cleanup(func() { slog.SetDefault(old) })
	return &buf
}

// A jar found by walking up from the working directory is run with a client
// secret, so it is used only when this user alone controls it and the
// directories above it up to the checkout's ingestor directory.
func TestAutoDiscoveredJarMustBeExclusivelyOwned(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("owner and mode bits do not apply on Windows; discovery is unchanged there")
	}
	ctx := context.Background()

	// build makes a user-owned checkout with a jar and returns the pieces.
	build := func(t *testing.T) (root, jar, target, ingestor string) {
		root = checkout(t)
		ingestor = filepath.Join(root, "ingestor")
		target = filepath.Join(ingestor, "ingestor-runtime", "target")
		jar = filepath.Join(target, "ingestor-runtime-0.1.0-all.jar")
		touchJar(t, jar, time.Minute)
		// MkdirAll/umask may leave group bits; make the baseline unambiguous.
		for _, d := range []string{ingestor, filepath.Dir(target), target} {
			require.NoError(t, os.Chmod(d, 0o755))
		}
		require.NoError(t, os.Chmod(jar, 0o644))
		return
	}
	find := func(root string) (string, error) {
		jar, _, err := resolveIngestorJar(ctx, "", []string{root}, ingestorReleaseSpec{}, t.TempDir(), nil)
		return jar, err
	}

	t.Run("user-owned and not writable by others is discovered", func(t *testing.T) {
		root, jar, _, _ := build(t)
		logs := captureSlog(t)
		got, err := find(root)
		require.NoError(t, err)
		assert.Equal(t, jar, got)
		assert.NotContains(t, logs.String(), "ignoring")
	})

	skipped := []struct {
		name   string
		break_ func(t *testing.T, jar, target, ingestor string)
		path   func(jar, target, ingestor string) string
	}{
		{
			"group-writable jar", func(t *testing.T, jar, _, _ string) { require.NoError(t, os.Chmod(jar, 0o664)) },
			func(jar, _, _ string) string { return jar },
		},
		{
			"world-writable target directory", func(t *testing.T, _, target, _ string) { require.NoError(t, os.Chmod(target, 0o777)) },
			func(_, target, _ string) string { return target },
		},
		{
			"world-writable ingestor directory", func(t *testing.T, _, _, ingestor string) { require.NoError(t, os.Chmod(ingestor, 0o777)) },
			func(_, _, ingestor string) string { return ingestor },
		},
	}
	for _, tc := range skipped {
		t.Run(tc.name+" is skipped with one warning", func(t *testing.T) {
			root, jar, target, ingestor := build(t)
			tc.break_(t, jar, target, ingestor)
			logs := captureSlog(t)
			_, err := find(root)
			require.ErrorIs(t, err, errNoIngestorJar, "discovery must continue as if the jar were absent")
			out := logs.String()
			assert.Contains(t, out, "ignoring an auto-discovered ingestor jar")
			assert.Contains(t, out, jar)
			assert.Contains(t, out, tc.path(jar, target, ingestor), "the reason names the offending path")
			assert.Contains(t, out, "writable")

			// Resolving again does not repeat the warning.
			logs.Reset()
			_, _ = find(root)
			assert.NotContains(t, logs.String(), "ignoring")
		})
	}

	t.Run("a symbolic link is skipped", func(t *testing.T) {
		root, jar, _, _ := build(t)
		real := filepath.Join(t.TempDir(), "real.jar")
		require.NoError(t, os.WriteFile(real, []byte("jar"), 0o644))
		require.NoError(t, os.Remove(jar))
		require.NoError(t, os.Symlink(real, jar))
		logs := captureSlog(t)
		_, err := find(root)
		require.ErrorIs(t, err, errNoIngestorJar)
		assert.Contains(t, logs.String(), "symbolic link")
	})

	t.Run("a skipped jar leaves a trusted older one usable", func(t *testing.T) {
		root, jar, target, _ := build(t)
		older := filepath.Join(target, "ingestor-runtime-0.0.9-all.jar")
		touchJar(t, older, time.Hour)
		require.NoError(t, os.Chmod(older, 0o644))
		require.NoError(t, os.Chmod(jar, 0o666))
		captureSlog(t)
		got, err := find(root)
		require.NoError(t, err)
		assert.Equal(t, older, got)
	})

	t.Run("explicit FC_INGESTOR_JAR is used regardless", func(t *testing.T) {
		explicit := filepath.Join(t.TempDir(), "mine.jar")
		require.NoError(t, os.WriteFile(explicit, []byte("jar"), 0o666))
		require.NoError(t, os.Chmod(explicit, 0o666))
		logs := captureSlog(t)
		got, src, err := resolveIngestorJar(ctx, explicit, nil, ingestorReleaseSpec{}, t.TempDir(), nil)
		require.NoError(t, err)
		assert.Equal(t, explicit, got)
		assert.Equal(t, jarExplicit, src)
		assert.NotContains(t, logs.String(), "ignoring")
	})
}
