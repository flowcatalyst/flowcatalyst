package payloadstore

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
)

// ObjectStore is where offloaded payloads are kept: whole objects written
// once under a key, read back a byte range at a time. Keys are relative
// object keys (checkKey); the store adds its own prefix.
type ObjectStore interface {
	// Put writes data as the object key, whole: a reader never observes a
	// partly written object.
	Put(ctx context.Context, key string, data []byte) error
	// GetRange reads at most length bytes of the object key starting at
	// offset. ErrObjectNotFound when there is no such object. A range that
	// runs past the end of the object returns the bytes there are (none when
	// offset is past the end) and no error: the caller checks the length.
	GetRange(ctx context.Context, key string, offset, length int64) ([]byte, error)
	// Describe says where the store is, for logs: its kind ("s3", "file",
	// "memory") and location (bucket and prefix, or directory). Never
	// credentials.
	Describe() (kind, location string)
}

// OpenStore builds the store a URL names (FC_PAYLOAD_STORE):
//
//	""                    no store (nil, nil): nothing is offloaded
//	s3://bucket/prefix    S3Store in bucket, keys under prefix
//	file:///abs/path      FileStore rooted at the absolute directory /abs/path
//
// Anything else is an error.
func OpenStore(ctx context.Context, rawURL string) (ObjectStore, error) {
	switch {
	case rawURL == "":
		return nil, nil
	case strings.HasPrefix(rawURL, "s3://"):
		bucket, prefix, _ := strings.Cut(strings.TrimPrefix(rawURL, "s3://"), "/")
		if bucket == "" {
			return nil, fmt.Errorf("payloadstore: s3:// URL missing bucket: %q", rawURL)
		}
		return asStore(NewS3Store(ctx, bucket, prefix))
	case strings.HasPrefix(rawURL, "file://"):
		dir := strings.TrimPrefix(rawURL, "file://")
		if !filepath.IsAbs(dir) {
			return nil, fmt.Errorf("payloadstore: file:// URL must name an absolute directory (file:///abs/path): %q", rawURL)
		}
		return asStore(NewFileStore(dir))
	default:
		return nil, fmt.Errorf("payloadstore: unsupported store URL %q: s3://bucket/prefix or file:///abs/path", rawURL)
	}
}

// asStore keeps a failed constructor's (nil, err) from becoming a non-nil
// interface holding a nil pointer.
func asStore[T ObjectStore](s T, err error) (ObjectStore, error) {
	if err != nil {
		return nil, err
	}
	return s, nil
}

// MemoryStore keeps objects in memory: for tests. Safe for concurrent use.
type MemoryStore struct {
	mu      sync.Mutex
	objects map[string][]byte
}

// NewMemoryStore returns an empty MemoryStore.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{objects: map[string][]byte{}}
}

// Put stores a copy of data under key.
func (m *MemoryStore) Put(_ context.Context, key string, data []byte) error {
	if err := checkKey(key); err != nil {
		return fmt.Errorf("payloadstore: memory put: %w", err)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.objects[key] = append([]byte(nil), data...)
	return nil
}

// GetRange returns a copy of the bytes of key in [offset, offset+length).
func (m *MemoryStore) GetRange(_ context.Context, key string, offset, length int64) ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	obj, ok := m.objects[key]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrObjectNotFound, key)
	}
	return sliceRange(obj, offset, length), nil
}

// Delete removes key: for tests of a missing object.
func (m *MemoryStore) Delete(key string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.objects, key)
}

// Overwrite replaces the object under key: for tests of a damaged object.
func (m *MemoryStore) Overwrite(key string, data []byte) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.objects[key] = append([]byte(nil), data...)
}

// Keys lists the stored keys: for tests.
func (m *MemoryStore) Keys() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	keys := make([]string, 0, len(m.objects))
	for k := range m.objects {
		keys = append(keys, k)
	}
	return keys
}

// Describe implements ObjectStore.
func (m *MemoryStore) Describe() (kind, location string) { return "memory", "" }

// sliceRange copies obj[offset:offset+length], clipped to obj.
func sliceRange(obj []byte, offset, length int64) []byte {
	if offset < 0 || length <= 0 || offset >= int64(len(obj)) {
		return []byte{}
	}
	end := min(offset+length, int64(len(obj)))
	return append([]byte(nil), obj[offset:end]...)
}
