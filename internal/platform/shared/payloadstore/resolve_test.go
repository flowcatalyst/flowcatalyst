package payloadstore

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// offloadOne stores one payload through Prepare and returns its row as a
// reader would select it.
func offloadOne(t *testing.T, s *Service, payload []byte) (Row, Stored) {
	t.Helper()
	out := s.Prepare(context.Background(), []Item{{ClientID: "clt_A", Data: payload}})
	require.NoError(t, out[0].Err)
	require.True(t, out[0].Row.Offloaded())
	ref := out[0].Row.Ref.String()
	return out[0].Row, Stored{ID: out[0].Row.ID, Ref: &ref, Size: out[0].Row.Size}
}

func TestResolve_Inline(t *testing.T) {
	s := newTestService(t, DefaultConfig(), nil)
	data := `{"a":1}`
	got, err := s.Resolve(context.Background(), Stored{ID: "p", Data: &data, Size: len(data)})
	require.NoError(t, err)
	assert.Equal(t, []byte(data), got)

	_, err = s.Resolve(context.Background(), Stored{ID: "p"})
	assert.ErrorIs(t, err, ErrMissing, "a row with neither data nor ref")
}

func TestResolve_ChecksumMismatchIsMissing(t *testing.T) {
	store := NewMemoryStore()
	cfg := DefaultConfig()
	cfg.CacheBytes = 0
	s := newTestService(t, cfg, store)
	row, st := offloadOne(t, s, payloadOf(9000, 1))

	obj, err := store.GetRange(context.Background(), row.Ref.Key, 0, 1<<20)
	require.NoError(t, err)
	obj[row.Ref.Offset+1] ^= 0xff
	store.Overwrite(row.Ref.Key, obj)

	before := counterValue(missingTotal)
	_, err = s.Resolve(context.Background(), st)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrMissing)
	assert.NotErrorIs(t, err, ErrStoreUnavailable)
	assert.InDelta(t, 1, counterValue(missingTotal)-before, 0)
}

func TestResolve_SizeMismatchIsMissing(t *testing.T) {
	cfg := DefaultConfig()
	cfg.CacheBytes = 0
	s := newTestService(t, cfg, NewMemoryStore())
	_, st := offloadOne(t, s, payloadOf(9000, 1))
	for _, size := range []int{8999, 9001} {
		bad := st
		bad.Size = size
		_, err := s.Resolve(context.Background(), bad)
		assert.ErrorIs(t, err, ErrMissing, size)
	}
	// The codec none path too.
	cfg.Codec = CodecNone
	s = newTestService(t, cfg, NewMemoryStore())
	_, st = offloadOne(t, s, payloadOf(9000, 2))
	st.Size = 8999
	_, err := s.Resolve(context.Background(), st)
	assert.ErrorIs(t, err, ErrMissing)
}

func TestResolve_WrongRangeIsMissing(t *testing.T) {
	cfg := DefaultConfig()
	cfg.CacheBytes = 0
	s := newTestService(t, cfg, NewMemoryStore())
	row, _ := offloadOne(t, s, payloadOf(9000, 1))
	for name, r := range map[string]Ref{
		"past the end":   {Key: row.Ref.Key, Offset: row.Ref.Offset + 1<<20, Length: row.Ref.Length, Codec: row.Ref.Codec, CRC32C: row.Ref.CRC32C},
		"runs past end":  {Key: row.Ref.Key, Offset: row.Ref.Offset, Length: row.Ref.Length + 10, Codec: row.Ref.Codec, CRC32C: row.Ref.CRC32C},
		"shifted by one": {Key: row.Ref.Key, Offset: row.Ref.Offset + 1, Length: row.Ref.Length - 1, Codec: row.Ref.Codec, CRC32C: row.Ref.CRC32C},
	} {
		ref := r.String()
		_, err := s.Resolve(context.Background(), Stored{ID: row.ID, Ref: &ref, Size: row.Size})
		assert.ErrorIs(t, err, ErrMissing, name)
	}
}

func TestResolve_ObjectGoneIsMissing(t *testing.T) {
	store := NewMemoryStore()
	cfg := DefaultConfig()
	cfg.CacheBytes = 0
	s := newTestService(t, cfg, store)
	row, st := offloadOne(t, s, payloadOf(9000, 1))
	store.Delete(row.Ref.Key)
	_, err := s.Resolve(context.Background(), st)
	assert.ErrorIs(t, err, ErrMissing)
}

func TestResolve_MalformedRefIsMissing(t *testing.T) {
	s := newTestService(t, DefaultConfig(), NewMemoryStore())
	ref := "v1:not:a:ref"
	_, err := s.Resolve(context.Background(), Stored{ID: "p", Ref: &ref, Size: 10})
	assert.ErrorIs(t, err, ErrMissing)
}

func TestResolve_StoreFailureIsUnavailable(t *testing.T) {
	failing := &failingStore{err: errors.New("timeout")}
	s := newTestService(t, DefaultConfig(), failing)
	ref := Ref{Key: "v1/_/2026-11-20/K", Offset: 0, Length: 10, Codec: CodecZstd}.String()
	_, err := s.Resolve(context.Background(), Stored{ID: "p", Ref: &ref, Size: 10})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrStoreUnavailable)
	assert.NotErrorIs(t, err, ErrMissing)
}

func TestResolve_RefWithNoStoreIsNotConfigured(t *testing.T) {
	s := newTestService(t, DefaultConfig(), nil)
	ref := Ref{Key: "v1/_/2026-11-20/K", Offset: 0, Length: 10, Codec: CodecZstd}.String()
	_, err := s.Resolve(context.Background(), Stored{ID: "p", Ref: &ref, Size: 10})
	assert.ErrorIs(t, err, ErrStoreNotConfigured)
}

// The cache serves the jobs of one event, and a retry, from one read: the
// second resolve does not touch the store.
func TestResolve_Cache(t *testing.T) {
	store := NewMemoryStore()
	s := newTestService(t, DefaultConfig(), store)
	payload := payloadOf(9000, 1)
	row, st := offloadOne(t, s, payload)

	got, err := s.Resolve(context.Background(), st)
	require.NoError(t, err)
	require.Equal(t, payload, got)
	got[0] = 'X' // the caller's copy

	store.Delete(row.Ref.Key)
	hits := counterValue(cacheHitsTotal)
	got, err = s.Resolve(context.Background(), st)
	require.NoError(t, err)
	assert.Equal(t, payload, got, "served from the cache, unchanged by the caller's edit")
	assert.InDelta(t, 1, counterValue(cacheHitsTotal)-hits, 0)
}
