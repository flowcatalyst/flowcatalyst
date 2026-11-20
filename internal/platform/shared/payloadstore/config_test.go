package payloadstore

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfig_ZeroAndDefaultsAreValid(t *testing.T) {
	require.NoError(t, Config{}.Validate())
	require.NoError(t, DefaultConfig().Validate())
	withStore := DefaultConfig()
	withStore.Store = "file:///tmp/payloads"
	require.NoError(t, withStore.Validate())

	d := DefaultConfig()
	assert.Equal(t, 5120, d.OffloadMinBytes)
	assert.Equal(t, 1048576, d.InlineMaxBytes)
	assert.Equal(t, 5000000, d.MaxBytes)
	assert.Equal(t, CodecZstd, d.Codec)
	assert.Equal(t, int64(64<<20), d.CacheBytes)
}

func TestConfig_Validate(t *testing.T) {
	ok := func(f func(*Config)) Config {
		c := DefaultConfig()
		c.Store = "s3://bucket/prefix"
		f(&c)
		return c
	}
	valid := map[string]Config{
		"threshold 0":          ok(func(c *Config) { c.OffloadMinBytes = 0 }),
		"threshold at minimum": ok(func(c *Config) { c.OffloadMinBytes = MinOffloadMinBytes }),
		"max equals inline":    ok(func(c *Config) { c.MaxBytes = c.InlineMaxBytes }),
		"codec none":           ok(func(c *Config) { c.Codec = CodecNone }),
		"cache off":            ok(func(c *Config) { c.CacheBytes = 0 }),
		// The batch cap is at least twice the largest payload, here the
		// 10 MiB inline maximum.
		"no store, max ignored":         {InlineMaxBytes: 10 << 20, MaxBytes: 1 << 20, IngestBatchMaxBytes: 20 << 20},
		"batch cap twice the largest":   ok(func(c *Config) { c.TableWrites = true; c.IngestBatchMaxBytes = 2 * DefaultMaxBytes }),
		"batch cap with the switch off": ok(func(c *Config) { c.IngestBatchMaxBytes = 2 * DefaultInlineMaxBytes }),
	}
	for name, c := range valid {
		assert.NoError(t, c.Validate(), name)
	}
	invalid := map[string]Config{
		"threshold below 2048": ok(func(c *Config) { c.OffloadMinBytes = MinOffloadMinBytes - 1 }),
		"threshold 1":          ok(func(c *Config) { c.OffloadMinBytes = 1 }),
		"threshold negative":   ok(func(c *Config) { c.OffloadMinBytes = -1 }),
		"threshold >= inline":  ok(func(c *Config) { c.OffloadMinBytes = c.InlineMaxBytes }),
		"max below inline":     ok(func(c *Config) { c.MaxBytes = c.InlineMaxBytes - 1 }),
		"inline negative":      ok(func(c *Config) { c.InlineMaxBytes = -1 }),
		"max over INTEGER":     ok(func(c *Config) { c.MaxBytes = maxSize + 1 }),
		"unknown codec":        ok(func(c *Config) { c.Codec = "gzip" }),
		"cache negative":       ok(func(c *Config) { c.CacheBytes = -1 }),
		"batch cap negative":   ok(func(c *Config) { c.IngestBatchMaxBytes = -1 }),
		// Less than twice the largest payload: 5 MB while offloading, the
		// inline maximum otherwise.
		"batch cap under twice the largest": ok(func(c *Config) { c.TableWrites = true; c.IngestBatchMaxBytes = 2*DefaultMaxBytes - 1 }),
		"batch cap under twice the inline":  ok(func(c *Config) { c.IngestBatchMaxBytes = 2*DefaultInlineMaxBytes - 1 }),
		"no store, batch cap under twice":   {InlineMaxBytes: 10 << 20},
	}
	for name, c := range invalid {
		assert.Error(t, c.Validate(), name)
	}
}

func TestNew_RefusesBadConfigurationAndURL(t *testing.T) {
	ctx := context.Background()
	_, err := New(ctx, Config{Store: "ftp://x"}, nil)
	assert.Error(t, err)
	_, err = New(ctx, Config{Store: "file://relative"}, nil)
	assert.Error(t, err)
	_, err = New(ctx, Config{OffloadMinBytes: 100}, nil)
	assert.Error(t, err)

	s, err := New(ctx, Config{}, nil)
	require.NoError(t, err)
	assert.False(t, s.StoreConfigured())

	// With a store but the payload table not written, every payload is
	// kept in the database, so the largest accepted is the inline maximum;
	// with the table written it is FC_PAYLOAD_MAX_BYTES.
	s, err = New(ctx, Config{Store: "file://" + t.TempDir(), OffloadMinBytes: DefaultOffloadMinBytes}, nil)
	require.NoError(t, err)
	assert.True(t, s.StoreConfigured())
	assert.Equal(t, DefaultInlineMaxBytes, s.MaxPayloadBytes())
	s, err = New(ctx, Config{Store: "file://" + t.TempDir(), OffloadMinBytes: DefaultOffloadMinBytes, TableWrites: true}, nil)
	require.NoError(t, err)
	assert.Equal(t, DefaultMaxBytes, s.MaxPayloadBytes())

	_, err = s.Get(ctx, "x", testNow)
	assert.Error(t, err, "Get needs a database")
}

// The largest payload accepted (MaxPayloadBytes) is FC_PAYLOAD_MAX_BYTES only
// while payloads are offloaded: a store, a threshold that is not 0 and the
// payload table written. With the switch off it is the inline maximum,
// whatever the store settings. The request caps follow from it.
func TestMaxPayloadBytes_FollowsTheSwitch(t *testing.T) {
	store := NewMemoryStore()
	for _, c := range []struct {
		name        string
		store       ObjectStore
		threshold   int
		tableWrites bool
		want        int
	}{
		{"no store, switch off", nil, DefaultOffloadMinBytes, false, DefaultInlineMaxBytes},
		{"no store, switch on", nil, DefaultOffloadMinBytes, true, DefaultInlineMaxBytes},
		{"store, switch off", store, DefaultOffloadMinBytes, false, DefaultInlineMaxBytes},
		{"store, threshold 0, switch off", store, 0, false, DefaultInlineMaxBytes},
		{"store, threshold 0, switch on", store, 0, true, DefaultInlineMaxBytes},
		{"store, switch on", store, DefaultOffloadMinBytes, true, DefaultMaxBytes},
	} {
		cfg := DefaultConfig()
		cfg.OffloadMinBytes, cfg.TableWrites = c.threshold, c.tableWrites
		s, err := NewWithStore(cfg, c.store, nil)
		require.NoError(t, err, c.name)
		assert.Equal(t, c.want, s.MaxPayloadBytes(), c.name)
		assert.Equal(t, 2*int64(c.want)+PayloadRequestOverhead, s.PayloadRequestMaxBytes(), c.name)
		assert.Equal(t, DefaultIngestBatchMaxBytes, s.BatchRequestMaxBytes(), c.name)
		assert.NoError(t, s.CheckSize(c.want), c.name)
		assert.ErrorIs(t, s.CheckSize(c.want+1), ErrTooLarge, c.name)
	}

	var none *Service
	assert.Equal(t, DefaultInlineMaxBytes, none.MaxPayloadBytes())
	assert.Equal(t, DefaultIngestBatchMaxBytes, none.BatchRequestMaxBytes())
	assert.Equal(t, 2*int64(DefaultInlineMaxBytes)+PayloadRequestOverhead, none.PayloadRequestMaxBytes())
	assert.ErrorIs(t, none.CheckSize(DefaultInlineMaxBytes+1), ErrTooLarge)

	cfg := DefaultConfig()
	cfg.IngestBatchMaxBytes = 32 << 20
	s, err := NewWithStore(cfg, nil, nil)
	require.NoError(t, err)
	assert.Equal(t, int64(32<<20), s.BatchRequestMaxBytes())

	// A batch cap under twice the largest payload stops the service (and so
	// the server) from starting.
	cfg = DefaultConfig()
	cfg.TableWrites = true
	cfg.IngestBatchMaxBytes = 8 << 20
	_, err = NewWithStore(cfg, store, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "FC_INGEST_BATCH_MAX_BYTES")
	cfg.TableWrites = false // the largest is then the 1 MiB inline maximum
	_, err = NewWithStore(cfg, store, nil)
	require.NoError(t, err)
}
