// Package payloadtest helps the tests of the payload readers and writers
// (docs/payload-offload-design.md, section 10): a store that counts its reads
// and writes and can be made to fail, services with and without the payload
// table written, and the insert of a msg_payloads row through the payload
// store's own Prepare and Insert. Nothing in it is used outside tests.
package payloadtest

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/payloadstore"
)

// CountingStore is a payloadstore.MemoryStore that counts its range reads and
// its writes and fails either, on demand, with an error of the caller's
// choosing.
type CountingStore struct {
	*payloadstore.MemoryStore
	gets atomic.Int64
	puts atomic.Int64

	mu       sync.Mutex
	readErr  error
	writeErr error
}

// Put counts the write, then fails it with the error FailWrites set, or
// writes the memory store.
func (c *CountingStore) Put(ctx context.Context, key string, data []byte) error {
	c.puts.Add(1)
	c.mu.Lock()
	err := c.writeErr
	c.mu.Unlock()
	if err != nil {
		return err
	}
	return c.MemoryStore.Put(ctx, key, data)
}

// Puts is the number of writes (segments) attempted so far.
func (c *CountingStore) Puts() int64 { return c.puts.Load() }

// FailWrites makes every later write fail with err (nil: write again).
func (c *CountingStore) FailWrites(err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.writeErr = err
}

// NewCountingStore returns an empty CountingStore.
func NewCountingStore() *CountingStore {
	return &CountingStore{MemoryStore: payloadstore.NewMemoryStore()}
}

// GetRange counts the read, then fails it with the error FailReads set, or
// reads the memory store.
func (c *CountingStore) GetRange(ctx context.Context, key string, offset, length int64) ([]byte, error) {
	c.gets.Add(1)
	c.mu.Lock()
	err := c.readErr
	c.mu.Unlock()
	if err != nil {
		return nil, err
	}
	return c.MemoryStore.GetRange(ctx, key, offset, length)
}

// Gets is the number of range reads made so far.
func (c *CountingStore) Gets() int64 { return c.gets.Load() }

// FailReads makes every later read fail with err (nil: read again).
func (c *CountingStore) FailReads(err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.readErr = err
}

// OffloadMinBytes is the threshold Service configures: the smallest the
// configuration allows, so a payload of a few kilobytes is offloaded.
const OffloadMinBytes = payloadstore.MinOffloadMinBytes

// Service builds a payload service on store (nil for none) that offloads a
// payload larger than OffloadMinBytes and caches nothing, so every resolve of
// an offloaded payload is one read of store. It has no database: the readers
// resolve the payload rows they joined themselves (Service.Resolve), never by
// key.
func Service(t *testing.T, store payloadstore.ObjectStore) *payloadstore.Service {
	t.Helper()
	cfg := payloadstore.DefaultConfig()
	cfg.OffloadMinBytes = OffloadMinBytes
	cfg.CacheBytes = 0
	s, err := payloadstore.NewWithStore(cfg, store, nil)
	require.NoError(t, err)
	return s
}

// Store writes data as one payload row under createdAt (the created_at of the
// event or job that will point at it) through svc's Prepare, inserts it on db
// and returns the row: offloaded when svc has a store and data is larger than
// its threshold, held in the database otherwise.
func Store(t *testing.T, svc *payloadstore.Service, db payloadstore.Batcher, clientID string, createdAt time.Time, data []byte) payloadstore.Row {
	t.Helper()
	ctx := context.Background()
	out := svc.Prepare(ctx, []payloadstore.Item{{ClientID: clientID, CreatedAt: createdAt, Data: data}})
	require.Len(t, out, 1)
	require.NoError(t, out[0].Err)
	require.NoError(t, payloadstore.Insert(ctx, db, []payloadstore.Row{out[0].Row}))
	return out[0].Row
}

// WritingService is Service with the payload table written
// (payloadstore.Config.TableWrites, FC_PAYLOAD_TABLE_WRITES on): the writers
// then store payload rows, offloading to store (nil for none) a payload
// larger than OffloadMinBytes. configure, when not nil, adjusts the
// configuration first (a lower InlineMaxBytes or MaxBytes, to reach the
// limits with small payloads). Like Service it has no database and caches
// nothing.
func WritingService(t *testing.T, store payloadstore.ObjectStore, configure func(*payloadstore.Config)) *payloadstore.Service {
	t.Helper()
	cfg := payloadstore.DefaultConfig()
	cfg.OffloadMinBytes = OffloadMinBytes
	cfg.CacheBytes = 0
	cfg.TableWrites = true
	if configure != nil {
		configure(&cfg)
	}
	s, err := payloadstore.NewWithStore(cfg, store, nil)
	require.NoError(t, err)
	return s
}
