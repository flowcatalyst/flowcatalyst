package payloadstore

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// PrepareInline (the platform's own events, inside a transaction) never
// writes the store: every row holds its bytes, whatever the threshold, and a
// payload over InlineMaxBytes is refused.
func TestPrepareInline(t *testing.T) {
	store := &failingStore{}
	cfg := DefaultConfig()
	cfg.OffloadMinBytes = MinOffloadMinBytes
	cfg.InlineMaxBytes = 8192
	cfg.MaxBytes = 16384
	s := newTestService(t, cfg, store)

	out := s.PrepareInline([]Item{
		{CreatedAt: testNow, Data: payloadOf(100, 1)},
		{CreatedAt: testNow, Data: payloadOf(5000, 2)}, // over the threshold: still inline
		{CreatedAt: testNow, Data: payloadOf(8193, 3)}, // over the inline maximum
		{CreatedAt: testNow, Data: []byte("a\xffb")},
	})
	require.Len(t, out, 4)
	require.NoError(t, out[0].Err)
	require.NoError(t, out[1].Err)
	assert.False(t, out[1].Row.Offloaded())
	assert.Equal(t, payloadOf(5000, 2), out[1].Row.Data)
	assert.Equal(t, 5000, out[1].Row.Size)
	assert.Equal(t, testNow, out[1].Row.CreatedAt)
	assert.ErrorIs(t, out[2].Err, ErrTooLarge)
	assert.ErrorIs(t, out[3].Err, ErrNotText)
	assert.Zero(t, store.puts.Load(), "the store is never written")
}

// A payload the payload table cannot hold as text is refused by Prepare,
// whether it would be kept in the database or offloaded.
func TestPrepare_RefusesWhatIsNotText(t *testing.T) {
	s := newTestService(t, Config{OffloadMinBytes: MinOffloadMinBytes}, NewMemoryStore())
	big := payloadOf(3*MinOffloadMinBytes, 4)
	big[10] = 0
	out := s.Prepare(context.Background(), []Item{
		{CreatedAt: testNow, Data: []byte("a\xffb")},
		{CreatedAt: testNow, Data: []byte("a\x00b")},
		{CreatedAt: testNow, Data: big},
		{CreatedAt: testNow, Data: []byte(`{"ok":"é"}`)},
	})
	for i := range 3 {
		assert.ErrorIs(t, out[i].Err, ErrNotText, i)
	}
	require.NoError(t, out[3].Err)
}

// The switch is read in one place, and a nil service writes the legacy shape
// with the default limit.
func TestTableWritesAndNilService(t *testing.T) {
	var none *Service
	assert.False(t, none.TableWrites())
	assert.Equal(t, DefaultInlineMaxBytes, none.MaxPayloadBytes())
	assert.False(t, newTestService(t, DefaultConfig(), nil).TableWrites())
	on := DefaultConfig()
	on.TableWrites = true
	assert.True(t, newTestService(t, on, nil).TableWrites())
}
