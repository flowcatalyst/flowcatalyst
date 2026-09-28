package artifact

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func digestOf(content []byte) string {
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}

func TestFileStore_PutOpenExistsDelete(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store, err := NewFileStore(t.TempDir())
	require.NoError(t, err)

	content := []byte("a wasm module, allegedly")
	digest := digestOf(content)

	ok, err := store.Exists(ctx, digest)
	require.NoError(t, err)
	assert.False(t, ok, "must not exist before Put")

	require.NoError(t, store.Put(ctx, digest, bytes.NewReader(content)))

	ok, err = store.Exists(ctx, digest)
	require.NoError(t, err)
	assert.True(t, ok)

	rc, err := store.Open(ctx, digest)
	require.NoError(t, err)
	got, err := io.ReadAll(rc)
	require.NoError(t, rc.Close())
	require.NoError(t, err)
	assert.Equal(t, content, got)

	require.NoError(t, store.Delete(ctx, digest))
	ok, err = store.Exists(ctx, digest)
	require.NoError(t, err)
	assert.False(t, ok, "must not exist after Delete")

	// Delete is idempotent.
	require.NoError(t, store.Delete(ctx, digest))
}

func TestFileStore_Open_NotFound(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store, err := NewFileStore(t.TempDir())
	require.NoError(t, err)

	_, err = store.Open(ctx, digestOf([]byte("never written")))
	assert.ErrorIs(t, err, ErrNotFound)
}

// TestFileStore_Put_DigestMismatchRejected pins the core safety property:
// content whose sha256 doesn't match the claimed digest is refused, and no
// file is left behind at the target path (no partial/wrong-content
// artifact).
func TestFileStore_Put_DigestMismatchRejected(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	dir := t.TempDir()
	store, err := NewFileStore(dir)
	require.NoError(t, err)

	content := []byte("real content")
	wrongDigest := digestOf([]byte("something else entirely"))

	err = store.Put(ctx, wrongDigest, bytes.NewReader(content))
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrDigestMismatch)

	ok, err := store.Exists(ctx, wrongDigest)
	require.NoError(t, err)
	assert.False(t, ok, "a digest-mismatched Put must leave nothing stored")

	// No stray temp file left in the digest's fan-out directory either.
	sub := filepath.Join(dir, "sha256", wrongDigest[:2])
	entries, statErr := os.ReadDir(sub)
	if statErr == nil {
		for _, e := range entries {
			assert.NotEqual(t, wrongDigest, e.Name(), "no file must exist at the rejected digest's path")
		}
	}
}

// TestFileStore_Put_Idempotent proves putting the same (digest, content)
// twice succeeds both times and the content is unchanged.
func TestFileStore_Put_Idempotent(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store, err := NewFileStore(t.TempDir())
	require.NoError(t, err)

	content := []byte("idempotent content")
	digest := digestOf(content)

	require.NoError(t, store.Put(ctx, digest, bytes.NewReader(content)))
	require.NoError(t, store.Put(ctx, digest, bytes.NewReader(content)), "a second identical Put must succeed")

	rc, err := store.Open(ctx, digest)
	require.NoError(t, err)
	got, err := io.ReadAll(rc)
	require.NoError(t, rc.Close())
	require.NoError(t, err)
	assert.Equal(t, content, got)
}

// TestFileStore_Put_AtomicWrite proves a reader can never observe a
// partially-written file: Put writes to a temp file in the same directory
// and renames into place, so mid-write there is no file at the final path
// yet, and once Put returns the final path has the complete content.
func TestFileStore_Put_AtomicWrite(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	dir := t.TempDir()
	store, err := NewFileStore(dir)
	require.NoError(t, err)

	content := bytes.Repeat([]byte("x"), 1<<20) // 1 MiB
	digest := digestOf(content)

	// Use a reader that blocks until told to proceed, so we can assert the
	// final path doesn't exist while the write is in flight.
	pr, pw, perr := os.Pipe()
	require.NoError(t, perr)
	done := make(chan error, 1)
	go func() {
		done <- store.Put(ctx, digest, pr)
	}()

	// Nothing written yet — the final path must not exist.
	final := store.path(digest)
	_, statErr := os.Stat(final)
	assert.True(t, errors.Is(statErr, os.ErrNotExist), "final path must not exist before the write completes")

	_, werr := pw.Write(content)
	require.NoError(t, werr)
	require.NoError(t, pw.Close())

	require.NoError(t, <-done)

	got, err := os.ReadFile(final)
	require.NoError(t, err)
	assert.Equal(t, content, got)
}

func TestValidDigest(t *testing.T) {
	t.Parallel()
	assert.True(t, ValidDigest(digestOf([]byte("x"))))
	assert.False(t, ValidDigest("not-hex"))
	assert.False(t, ValidDigest("ABCD"))
	assert.False(t, ValidDigest(""))
	assert.False(t, ValidDigest("aa")) // too short
}

// TestNewFileStore_UnwritableRootFailsOnPutNotConstruction pins that the
// platform can boot with the default store root it cannot create (a
// developer machine's /var/lib): construction succeeds, an upload fails.
func TestNewFileStore_UnwritableRootFailsOnPutNotConstruction(t *testing.T) {
	parent := t.TempDir()
	if err := os.Chmod(parent, 0o500); err != nil {
		t.Skip("cannot make a read-only directory here")
	}
	t.Cleanup(func() { _ = os.Chmod(parent, 0o700) })
	s, err := NewFileStore(filepath.Join(parent, "artifacts"))
	if err != nil {
		t.Fatalf("construction failed on an uncreatable root: %v", err)
	}
	data := []byte("x")
	sum := sha256.Sum256(data)
	if err := s.Put(context.Background(), hex.EncodeToString(sum[:]), bytes.NewReader(data)); err == nil {
		t.Fatal("a Put into an uncreatable root succeeded")
	}
}
