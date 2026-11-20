package payloadstore

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The file store's clean-up removes the date directories of its layout before
// the cutoff's date, then the client directories left empty, and nothing that
// does not match the layout.
func TestFileStore_RemoveExpired(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	fs, err := NewFileStore(dir)
	require.NoError(t, err)
	put := func(key string) {
		t.Helper()
		require.NoError(t, fs.Put(ctx, key, []byte("x")))
	}
	mkfile := func(rel string) {
		t.Helper()
		p := filepath.Join(dir, filepath.FromSlash(rel))
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
		require.NoError(t, os.WriteFile(p, []byte("keep"), 0o600))
	}
	exists := func(rel string) bool {
		_, err := os.Lstat(filepath.Join(dir, filepath.FromSlash(rel)))
		return err == nil
	}

	put("v1/clt_A/2026-07-01/seg1")
	put("v1/clt_A/2026-07-01/seg2")
	put("v1/clt_A/2026-08-14/seg3") // the day before the cutoff
	put("v1/clt_A/2026-08-15/seg4") // the cutoff's own day: kept
	put("v1/_/2026-01-31/seg5")     // the only directory of "_": both go
	put("v1/clt_B/2026-09-01/seg6") // after the cutoff
	// Outside the layout: kept whatever their dates.
	mkfile("v1/clt_A/2026-7-1/seg")   // not yyyy-mm-dd
	mkfile("v1/clt_A/2026-02-30/seg") // not a date
	mkfile("v1/clt_A/2026-01-01")     // a file, not a directory
	mkfile("v1/clt.C/2026-01-01/seg") // not a client segment
	mkfile("v1/a-very-long-client-id-that-is-longer-than-sixty-four-characters-0001/2026-01-01/seg")
	mkfile("v2/clt_A/2026-01-01/seg")     // another version
	mkfile("other/clt_A/2026-01-01/seg")  // outside v1
	mkfile("v1/stray-file")               // a file under v1
	mkfile("v1/clt_D/2026-01-01.bak/seg") // a date with a suffix
	require.NoError(t, os.Symlink(filepath.Join(dir, "other"), filepath.Join(dir, "v1", "clt_E")))

	cutoff := time.Date(2026, 8, 15, 13, 0, 0, 0, time.UTC)
	removed, err := fs.RemoveExpired(ctx, cutoff)
	require.NoError(t, err)
	assert.Equal(t, 3, removed, "2026-07-01 and 2026-08-14 of clt_A, 2026-01-31 of _")

	for _, gone := range []string{"v1/clt_A/2026-07-01", "v1/clt_A/2026-08-14", "v1/_/2026-01-31", "v1/_"} {
		assert.False(t, exists(gone), gone)
	}
	for _, kept := range []string{
		"v1/clt_A/2026-08-15/seg4", "v1/clt_B/2026-09-01/seg6",
		"v1/clt_A/2026-7-1/seg", "v1/clt_A/2026-02-30/seg", "v1/clt_A/2026-01-01",
		"v1/clt.C/2026-01-01/seg",
		"v1/a-very-long-client-id-that-is-longer-than-sixty-four-characters-0001/2026-01-01/seg",
		"v2/clt_A/2026-01-01/seg", "other/clt_A/2026-01-01/seg", "v1/stray-file",
		"v1/clt_D/2026-01-01.bak/seg", "v1/clt_E",
	} {
		assert.True(t, exists(kept), kept)
	}

	// Nothing left to remove: a second pass removes nothing.
	removed, err = fs.RemoveExpired(ctx, cutoff)
	require.NoError(t, err)
	assert.Zero(t, removed)

	// A store nothing was written to yet.
	empty, err := NewFileStore(filepath.Join(t.TempDir(), "never"))
	require.NoError(t, err)
	removed, err = empty.RemoveExpired(ctx, cutoff)
	require.NoError(t, err)
	assert.Zero(t, removed)
}

// Only a file store is cleaned by the platform: OpenExpirer gives none for no
// store and for S3, whose bucket's lifecycle rule expires it.
func TestOpenExpirer(t *testing.T) {
	ctx := context.Background()
	e, err := OpenExpirer(ctx, "")
	require.NoError(t, err)
	assert.Nil(t, e)
	e, err = OpenExpirer(ctx, "s3://bucket/prefix")
	require.NoError(t, err)
	assert.Nil(t, e, "S3 is never cleaned by the platform")
	e, err = OpenExpirer(ctx, "file://"+t.TempDir())
	require.NoError(t, err)
	assert.IsType(t, &FileStore{}, e)
	_, err = OpenExpirer(ctx, "file://relative")
	require.Error(t, err)

	var _ Expirer = (*FileStore)(nil)
	_, isExpirer := any(&S3Store{}).(Expirer)
	assert.False(t, isExpirer)
}
