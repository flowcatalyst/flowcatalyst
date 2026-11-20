package payloadstore

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// FileStore keeps objects as files under a directory: for fcdev, tests and a
// single node (or several sharing the directory). The directory is made by
// the service's startup check or the first Put, not here, and a root that
// cannot be made fails a write, which falls back to the database, and never
// the server's start.
//
// Unlike the S3 store, the file store is cleaned by the platform: the
// partition manager's daily pass removes its expired date directories
// (RemoveExpired).
type FileStore struct {
	dir string
}

// NewFileStore returns a FileStore rooted at dir.
func NewFileStore(dir string) (*FileStore, error) {
	if dir == "" {
		return nil, errors.New("payloadstore: file store directory must not be empty")
	}
	return &FileStore{dir: filepath.Clean(dir)}, nil
}

func (s *FileStore) path(key string) (string, error) {
	if err := checkKey(key); err != nil {
		return "", err
	}
	return filepath.Join(s.dir, filepath.FromSlash(key)), nil
}

// Put writes data to a temporary file beside the final path, flushes it to
// disk and renames it into place, so a reader sees the whole object or none.
func (s *FileStore) Put(_ context.Context, key string, data []byte) error {
	final, err := s.path(key)
	if err != nil {
		return fmt.Errorf("payloadstore: file put: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(final), 0o755); err != nil {
		return fmt.Errorf("payloadstore: file put %s: create directory: %w", key, err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(final), ".segment-*")
	if err != nil {
		return fmt.Errorf("payloadstore: file put %s: create temp file: %w", key, err)
	}
	tmpPath := tmp.Name()
	keep := false
	defer func() {
		if !keep {
			_ = os.Remove(tmpPath)
		}
	}()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("payloadstore: file put %s: write: %w", key, err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("payloadstore: file put %s: sync: %w", key, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("payloadstore: file put %s: close: %w", key, err)
	}
	if err := os.Rename(tmpPath, final); err != nil {
		return fmt.Errorf("payloadstore: file put %s: rename into place: %w", key, err)
	}
	keep = true
	return nil
}

// GetRange reads at most length bytes of key at offset.
func (s *FileStore) GetRange(_ context.Context, key string, offset, length int64) ([]byte, error) {
	p, err := s.path(key)
	if err != nil {
		return nil, fmt.Errorf("payloadstore: file get: %w", err)
	}
	if offset < 0 || length <= 0 {
		return nil, fmt.Errorf("payloadstore: file get %s: invalid range %d+%d", key, offset, length)
	}
	f, err := os.Open(p)
	if errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("%w: %s", ErrObjectNotFound, key)
	}
	if err != nil {
		return nil, fmt.Errorf("payloadstore: file get %s: open: %w", key, err)
	}
	defer func() { _ = f.Close() }()
	info, err := f.Stat()
	if err != nil {
		return nil, fmt.Errorf("payloadstore: file get %s: stat: %w", key, err)
	}
	if offset >= info.Size() {
		return []byte{}, nil
	}
	buf := make([]byte, min(length, info.Size()-offset))
	n, err := f.ReadAt(buf, offset)
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("payloadstore: file get %s: read: %w", key, err)
	}
	return buf[:n], nil
}

// Describe implements ObjectStore.
func (s *FileStore) Describe() (kind, location string) { return "file", s.dir }

// startupCheck makes the directory and writes and removes a temporary file in
// it: a cheap proof that a write can succeed.
func (s *FileStore) startupCheck(context.Context) (bool, error) {
	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		return false, fmt.Errorf("create directory %s: %w", s.dir, err)
	}
	f, err := os.CreateTemp(s.dir, ".health-*")
	if err != nil {
		return false, fmt.Errorf("write in %s: %w", s.dir, err)
	}
	name := f.Name()
	closeErr := f.Close()
	removeErr := os.Remove(name)
	if err := errors.Join(closeErr, removeErr); err != nil {
		return false, fmt.Errorf("write in %s: %w", s.dir, err)
	}
	return true, nil
}
