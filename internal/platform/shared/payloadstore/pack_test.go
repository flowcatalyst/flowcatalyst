package payloadstore

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var testNow = time.Date(2026, 11, 20, 23, 59, 0, 0, time.UTC)

// failingStore refuses every write and read with err, counting the calls.
type failingStore struct {
	err  error
	puts atomic.Int32
	gets atomic.Int32
}

func (f *failingStore) Put(context.Context, string, []byte) error {
	f.puts.Add(1)
	return f.err
}

func (f *failingStore) GetRange(context.Context, string, int64, int64) ([]byte, error) {
	f.gets.Add(1)
	return nil, f.err
}

func (f *failingStore) Describe() (kind, location string) { return "failing", "" }

// counterValue reads a counter's value.
func counterValue(c prometheus.Counter) float64 {
	var m dto.Metric
	_ = c.Write(&m)
	return m.GetCounter().GetValue()
}

func newTestService(t *testing.T, cfg Config, store ObjectStore) *Service {
	t.Helper()
	s, err := NewWithStore(cfg, store, nil)
	require.NoError(t, err)
	s.now = func() time.Time { return testNow }
	return s
}

// payloadOf makes an n-byte JSON-ish payload that differs by seed.
func payloadOf(n int, seed byte) []byte {
	b := bytes.Repeat([]byte{'a' + seed%26}, n)
	if n >= 2 {
		b[0], b[n-1] = '"', '"'
	}
	return b
}

func resolveRow(t *testing.T, s *Service, r Row) []byte {
	t.Helper()
	st := Stored{ID: r.ID, Size: r.Size}
	if r.Ref != nil {
		ref := r.Ref.String()
		st.Ref = &ref
	} else {
		d := string(r.Data)
		st.Data = &d
	}
	got, err := s.Resolve(context.Background(), st)
	require.NoError(t, err)
	return got
}

// The offloaded payloads of one batch are one segment per client, keyed
// v1/<client or _>/<ingest date UTC>/<segment id>; small ones stay in their
// rows; every row resolves to the payload it was given.
func TestPrepare_OneSegmentPerClient(t *testing.T) {
	store := NewMemoryStore()
	cfg := DefaultConfig()
	s := newTestService(t, cfg, store)
	created := time.Date(2026, 10, 3, 8, 0, 0, 0, time.UTC)

	items := []Item{
		{ClientID: "clt_A", CreatedAt: created, Data: payloadOf(6000, 1)},
		{ClientID: "clt_B", CreatedAt: created, Data: payloadOf(7000, 2)},
		{ClientID: "clt_A", CreatedAt: created, Data: payloadOf(100, 3)}, // inline
		{ClientID: "clt_A", CreatedAt: created, Data: payloadOf(9000, 4)},
		{ClientID: "", CreatedAt: created, Data: payloadOf(8000, 5)},
		{ClientID: "bad/../id", CreatedAt: created, Data: payloadOf(8000, 6)},
	}
	out := s.Prepare(context.Background(), items)
	require.Len(t, out, len(items))

	keysByClient := map[string]map[string]bool{}
	for i, p := range out {
		require.NoError(t, p.Err, i)
		assert.Len(t, p.Row.ID, 13, "untyped TSID")
		assert.Equal(t, created, p.Row.CreatedAt, "the owner's created_at")
		assert.Equal(t, len(items[i].Data), p.Row.Size)
		assert.Equal(t, items[i].Data, resolveRow(t, s, p.Row))
		if i == 2 {
			assert.False(t, p.Row.Offloaded())
			assert.Equal(t, items[i].Data, p.Row.Data)
			continue
		}
		require.True(t, p.Row.Offloaded(), i)
		assert.Nil(t, p.Row.Data, "exactly one of data and ref")
		parts := strings.Split(p.Row.Ref.Key, "/")
		require.Len(t, parts, 4)
		assert.Equal(t, "v1", parts[0])
		assert.Equal(t, "2026-11-20", parts[2], "the ingest date, not the owner's")
		assert.Len(t, parts[3], 13)
		if keysByClient[parts[1]] == nil {
			keysByClient[parts[1]] = map[string]bool{}
		}
		keysByClient[parts[1]][p.Row.Ref.Key] = true
	}
	assert.Len(t, keysByClient["clt_A"], 1, "clt_A's two payloads share one segment")
	assert.Len(t, keysByClient["clt_B"], 1)
	assert.Len(t, keysByClient["_"], 2, "no client, and a client id that cannot be a key segment, under _; one segment each")
	assert.Len(t, store.Keys(), 4, "one object per segment")

	// Within clt_A's segment the payloads follow each other.
	a0, a3 := out[0].Row.Ref, out[3].Row.Ref
	assert.Equal(t, a0.Key, a3.Key)
	assert.Equal(t, int64(0), a0.Offset)
	assert.Equal(t, a0.Offset+a0.Length, a3.Offset)
}

func TestPrepare_ThresholdBoundary(t *testing.T) {
	cfg := DefaultConfig()
	cfg.OffloadMinBytes = MinOffloadMinBytes
	s := newTestService(t, cfg, NewMemoryStore())
	out := s.Prepare(context.Background(), []Item{
		{Data: payloadOf(MinOffloadMinBytes, 1)},
		{Data: payloadOf(MinOffloadMinBytes+1, 2)},
	})
	require.NoError(t, out[0].Err)
	require.NoError(t, out[1].Err)
	assert.False(t, out[0].Row.Offloaded(), "exactly the threshold stays in the database")
	assert.True(t, out[1].Row.Offloaded(), "one byte more is offloaded")
}

func TestPrepare_NoStoreKeepsEverythingUpToInlineMax(t *testing.T) {
	cfg := DefaultConfig()
	cfg.InlineMaxBytes = 64 << 10
	s := newTestService(t, cfg, nil)
	assert.False(t, s.StoreConfigured())
	assert.Equal(t, cfg.InlineMaxBytes, s.MaxPayloadBytes())
	out := s.Prepare(context.Background(), []Item{
		{Data: payloadOf(cfg.InlineMaxBytes, 1)},
		{Data: payloadOf(cfg.InlineMaxBytes+1, 2)},
		{Data: []byte{}},
	})
	require.NoError(t, out[0].Err)
	assert.False(t, out[0].Row.Offloaded())
	assert.Equal(t, cfg.InlineMaxBytes, out[0].Row.Size)
	assert.ErrorIs(t, out[1].Err, ErrTooLarge)
	assert.Empty(t, out[1].Row.ID, "a refused item has no row")
	require.NoError(t, out[2].Err)
	assert.Equal(t, 0, out[2].Row.Size)
	assert.Equal(t, "", *out[2].Row.InsertArgs()[2].(*string), "an empty payload is an empty data, not NULL")
}

// With a store but the threshold at 0, nothing is offloaded, so the largest
// payload accepted is the database's.
func TestPrepare_ThresholdZeroStopsOffloading(t *testing.T) {
	store := NewMemoryStore()
	cfg := DefaultConfig()
	cfg.OffloadMinBytes = 0
	s := newTestService(t, cfg, store)
	assert.Equal(t, cfg.InlineMaxBytes, s.MaxPayloadBytes())
	out := s.Prepare(context.Background(), []Item{
		{Data: payloadOf(cfg.InlineMaxBytes, 1)},
		{Data: payloadOf(cfg.InlineMaxBytes+1, 2)},
	})
	require.NoError(t, out[0].Err)
	assert.False(t, out[0].Row.Offloaded())
	assert.ErrorIs(t, out[1].Err, ErrTooLarge)
	assert.Empty(t, store.Keys())
}

func TestPrepare_TooLargeWithStore(t *testing.T) {
	cfg := DefaultConfig()
	cfg.TableWrites = true // MaxPayloadBytes is FC_PAYLOAD_MAX_BYTES only with the table written
	s := newTestService(t, cfg, NewMemoryStore())
	assert.Equal(t, DefaultMaxBytes, s.MaxPayloadBytes())
	out := s.Prepare(context.Background(), []Item{
		{Data: payloadOf(DefaultMaxBytes, 1)},
		{Data: payloadOf(DefaultMaxBytes+1, 2)},
	})
	require.NoError(t, out[0].Err)
	assert.True(t, out[0].Row.Offloaded())
	require.Error(t, out[1].Err)
	assert.ErrorIs(t, out[1].Err, ErrTooLarge)
	assert.NotErrorIs(t, out[1].Err, ErrStoreUnavailable)
}

// When the store cannot be written, a payload the database takes is kept in
// its row (and counted); a larger one gets the retryable error, not "too
// large".
func TestPrepare_StoreFailure(t *testing.T) {
	store := &failingStore{err: errors.New("connection refused")}
	cfg := DefaultConfig()
	cfg.InlineMaxBytes = 64 << 10
	cfg.MaxBytes = 256 << 10
	s := newTestService(t, cfg, store)
	before := counterValue(inlineFallbackTotal)
	putErrorsBefore := counterValue(storeErrors.WithLabelValues(opPut))

	small := payloadOf(cfg.InlineMaxBytes, 1)
	large := payloadOf(cfg.InlineMaxBytes+1, 2)
	out := s.Prepare(context.Background(), []Item{
		{ClientID: "clt_A", Data: small},
		{ClientID: "clt_A", Data: large},
		{ClientID: "clt_B", Data: payloadOf(6000, 3)},
		{ClientID: "clt_B", Data: payloadOf(10, 4)},
	})
	require.NoError(t, out[0].Err)
	assert.False(t, out[0].Row.Offloaded())
	assert.Equal(t, small, out[0].Row.Data)

	require.Error(t, out[1].Err)
	assert.ErrorIs(t, out[1].Err, ErrStoreUnavailable)
	assert.NotErrorIs(t, out[1].Err, ErrTooLarge)
	assert.Empty(t, out[1].Row.ID)

	require.NoError(t, out[2].Err)
	assert.False(t, out[2].Row.Offloaded())
	require.NoError(t, out[3].Err)

	assert.Equal(t, int32(2), store.puts.Load(), "one write per client segment")
	assert.InDelta(t, 2, counterValue(inlineFallbackTotal)-before, 0)
	assert.InDelta(t, 2, counterValue(storeErrors.WithLabelValues(opPut))-putErrorsBefore, 0)
}

// A reference is only returned once its object is in the store.
func TestPrepare_StoreWrittenBeforeRowsReturned(t *testing.T) {
	store := NewMemoryStore()
	s := newTestService(t, DefaultConfig(), store)
	out := s.Prepare(context.Background(), []Item{{ClientID: "clt_A", Data: payloadOf(20_000, 1)}})
	require.NoError(t, out[0].Err)
	require.True(t, out[0].Row.Offloaded())
	_, err := store.GetRange(context.Background(), out[0].Row.Ref.Key, out[0].Row.Ref.Offset, out[0].Row.Ref.Length)
	require.NoError(t, err)
}

func TestPrepare_CodecNone(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Codec = CodecNone
	s := newTestService(t, cfg, NewMemoryStore())
	p := payloadOf(6000, 1)
	out := s.Prepare(context.Background(), []Item{{Data: p}})
	require.NoError(t, out[0].Err)
	require.True(t, out[0].Row.Offloaded())
	assert.Equal(t, CodecNone, out[0].Row.Ref.Codec)
	assert.Equal(t, int64(len(p)), out[0].Row.Ref.Length)
	assert.Equal(t, p, resolveRow(t, s, out[0].Row))
}

func TestRow_InsertArgs(t *testing.T) {
	ref := Ref{Key: "v1/_/2026-11-20/K", Offset: 0, Length: 5, Codec: CodecZstd, CRC32C: 7}
	off := Row{ID: "0ABCDEFGHJKMN", CreatedAt: testNow, Ref: &ref, Size: 9}.InsertArgs()
	assert.Nil(t, off[2], "data is NULL when offloaded")
	assert.Equal(t, ref.String(), *off[3].(*string))
	assert.Equal(t, int32(9), off[4])

	in := Row{ID: "0ABCDEFGHJKMN", CreatedAt: testNow, Data: []byte(`{"a":1}`), Size: 7}.InsertArgs()
	assert.Equal(t, `{"a":1}`, *in[2].(*string))
	assert.Nil(t, in[3], "ref is NULL when inline")
}
