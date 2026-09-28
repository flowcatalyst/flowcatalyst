package artifact

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

// FileStore is a content-addressed artifact store rooted at a directory on
// local disk. The fc-dev default (plan §8.4).
type FileStore struct {
	dir string
}

// NewFileStore returns a FileStore rooted at dir. The directory is created
// on the first Put, not here: the platform builds its store at boot, and a
// root it cannot create (the default /var/lib path on a developer machine)
// must fail an upload, not the whole server's startup.
func NewFileStore(dir string) (*FileStore, error) {
	if dir == "" {
		return nil, errors.New("artifact: file store directory must not be empty")
	}
	return &FileStore{dir: dir}, nil
}

func (s *FileStore) path(digest string) string {
	return filepath.Join(s.dir, filepath.FromSlash(keyFor(digest)))
}

// Put streams r to a temp file in the same directory as the final path
// while hashing it, verifies the hash equals digest, then atomically
// renames into place. A digest mismatch leaves no partial file behind. If
// the target already exists, the temp file is discarded (still hashed and
// verified first, so a caller can never "succeed" a Put of content that
// doesn't match digest, existing file or not).
func (s *FileStore) Put(_ context.Context, digest string, r io.Reader) error {
	if !ValidDigest(digest) {
		return fmt.Errorf("artifact: invalid digest %q", digest)
	}
	final := s.path(digest)
	if err := os.MkdirAll(filepath.Dir(final), 0o755); err != nil {
		return fmt.Errorf("artifact: create digest dir: %w", err)
	}

	tmp, err := os.CreateTemp(filepath.Dir(final), ".upload-*")
	if err != nil {
		return fmt.Errorf("artifact: create temp file: %w", err)
	}
	tmpPath := tmp.Name()
	removeTemp := true
	defer func() {
		if removeTemp {
			_ = os.Remove(tmpPath)
		}
	}()

	h := sha256.New()
	if _, err := io.Copy(io.MultiWriter(tmp, h), r); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("artifact: write temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("artifact: close temp file: %w", err)
	}

	got := hex.EncodeToString(h.Sum(nil))
	if got != digest {
		return fmt.Errorf("%w: computed %s, requested %s", ErrDigestMismatch, got, digest)
	}

	if err := os.Rename(tmpPath, final); err != nil {
		return fmt.Errorf("artifact: rename into place: %w", err)
	}
	removeTemp = false
	return nil
}

// Open returns a reader for the artifact at digest.
func (s *FileStore) Open(_ context.Context, digest string) (io.ReadCloser, error) {
	f, err := os.Open(s.path(digest))
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("artifact: open: %w", err)
	}
	return f, nil
}

// Exists reports whether an artifact is stored at digest.
func (s *FileStore) Exists(_ context.Context, digest string) (bool, error) {
	_, err := os.Stat(s.path(digest))
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("artifact: stat: %w", err)
	}
	return true, nil
}

// Delete removes the artifact at digest. Not an error if absent.
func (s *FileStore) Delete(_ context.Context, digest string) error {
	err := os.Remove(s.path(digest))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("artifact: delete: %w", err)
	}
	return nil
}

// PresignGet always returns ok=false: a local path has nothing to presign.
func (s *FileStore) PresignGet(_ context.Context, _ string, _ time.Duration) (string, bool, error) {
	return "", false, nil
}
