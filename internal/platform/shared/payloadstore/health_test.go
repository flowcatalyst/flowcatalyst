package payloadstore

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// switchStore is a MemoryStore whose writes and reads fail while putErr or
// getErr is set.
type switchStore struct {
	*MemoryStore
	putErr, getErr error
}

func (s *switchStore) Put(ctx context.Context, key string, data []byte) error {
	if s.putErr != nil {
		return s.putErr
	}
	return s.MemoryStore.Put(ctx, key, data)
}

func (s *switchStore) GetRange(ctx context.Context, key string, offset, length int64) ([]byte, error) {
	if s.getErr != nil {
		return nil, s.getErr
	}
	return s.MemoryStore.GetRange(ctx, key, offset, length)
}

func reachable(r HealthReport) string {
	if r.Reachable == nil {
		return "unknown"
	}
	if *r.Reachable {
		return "yes"
	}
	return "no"
}

// No store is a healthy state, on a nil service too, and says so.
func TestHealth_NotConfigured(t *testing.T) {
	for name, s := range map[string]*Service{
		"nil service": nil,
		"no store":    newTestService(t, DefaultConfig(), nil),
	} {
		r := s.Health()
		assert.Equal(t, HealthNotConfigured, r.Status, name)
		assert.False(t, r.Configured, name)
		assert.Empty(t, r.Kind, name)
		assert.Nil(t, r.Reachable, name)
		assert.Equal(t, 300, r.WindowSeconds, name)
	}
}

// Health follows the outcomes of real operations: unknown until the first,
// DEGRADED for DegradedWindow after a failed write or read, reachable as the
// last operation found the store. A missing object is an answer.
func TestHealth_FromTheOutcomesOfOperations(t *testing.T) {
	store := &switchStore{MemoryStore: NewMemoryStore()}
	cfg := DefaultConfig()
	cfg.CacheBytes = 0
	cfg.TableWrites = true
	s := newTestService(t, cfg, store)
	now := testNow
	s.now = func() time.Time { return now }
	ctx := context.Background()

	r := s.Health()
	assert.Equal(t, HealthUp, r.Status, "configured, nothing tried yet")
	assert.True(t, r.Configured)
	assert.Equal(t, "memory", r.Kind)
	assert.True(t, r.Offloading)
	assert.Equal(t, "unknown", reachable(r))

	row, stored := offloadOne(t, s, payloadOf(9000, 1))
	r = s.Health()
	assert.Equal(t, HealthUp, r.Status)
	assert.Equal(t, "yes", reachable(r))
	require.NotNil(t, r.LastSuccessAt)
	assert.Equal(t, now, *r.LastSuccessAt)

	// A write fails: its payload is kept in the database, and the store is
	// DEGRADED.
	now = now.Add(time.Minute)
	store.putErr = errors.New("connection refused")
	out := s.Prepare(ctx, []Item{{ClientID: "clt_A", Data: payloadOf(9000, 2)}})
	require.NoError(t, out[0].Err)
	require.False(t, out[0].Row.Offloaded(), "kept in the database")
	r = s.Health()
	assert.Equal(t, HealthDegraded, r.Status)
	assert.Equal(t, "no", reachable(r))
	assert.Equal(t, HealthOpPut, r.LastFailure)
	require.NotNil(t, r.LastFailureAt)
	assert.Equal(t, now, *r.LastFailureAt)

	// Still DEGRADED within the window, though the store answers again.
	store.putErr = nil
	now = now.Add(DegradedWindow - time.Second)
	_, err := s.Resolve(ctx, stored)
	require.NoError(t, err)
	r = s.Health()
	assert.Equal(t, HealthDegraded, r.Status, "a failure in the window")
	assert.Equal(t, "yes", reachable(r), "the last operation found the store")

	// Past the window: UP.
	now = now.Add(2 * time.Second)
	assert.Equal(t, HealthUp, s.Health().Status)

	// A read fails: DEGRADED again.
	store.getErr = errors.New("timeout")
	_, err = s.Resolve(ctx, stored)
	require.ErrorIs(t, err, ErrStoreUnavailable)
	r = s.Health()
	assert.Equal(t, HealthDegraded, r.Status)
	assert.Equal(t, HealthOpGet, r.LastFailure)
	assert.Equal(t, "no", reachable(r))

	// A missing object is an answer: the store is reachable.
	store.getErr = nil
	now = now.Add(time.Second)
	store.Delete(row.Ref.Key)
	_, err = s.Resolve(ctx, stored)
	require.ErrorIs(t, err, ErrMissing)
	assert.Equal(t, "yes", reachable(s.Health()))
}

// The report as served: the fields named, the location never.
func TestHealth_JSONNamesNoLocation(t *testing.T) {
	dir := t.TempDir()
	fs, err := NewFileStore(dir)
	require.NoError(t, err)
	s := newTestService(t, DefaultConfig(), fs)
	b, err := json.Marshal(s.Health())
	require.NoError(t, err)
	assert.JSONEq(t, `{"status":"UP","configured":true,"kind":"file","offloading":false,"windowSeconds":300}`, string(b))
	assert.NotContains(t, string(b), dir)
}

// The one startup check: the file store proves it can write, or is DEGRADED
// from the start; the S3 store records only a credential chain that did not
// resolve. The start never fails on it.
func TestHealth_StartupCheck(t *testing.T) {
	ctx := context.Background()

	dir := filepath.Join(t.TempDir(), "payloads")
	s, err := New(ctx, Config{Store: "file://" + dir}, nil)
	require.NoError(t, err)
	r := s.Health()
	assert.Equal(t, HealthUp, r.Status)
	assert.Equal(t, "yes", reachable(r))
	entries, err := os.ReadDir(dir)
	require.NoError(t, err, "the directory is made")
	assert.Empty(t, entries, "the check leaves nothing behind")

	blocker := filepath.Join(t.TempDir(), "a-file")
	require.NoError(t, os.WriteFile(blocker, nil, 0o600))
	s, err = New(ctx, Config{Store: "file://" + filepath.Join(blocker, "payloads")}, nil)
	require.NoError(t, err, "a store that cannot be used does not stop the start")
	r = s.Health()
	assert.Equal(t, HealthDegraded, r.Status)
	assert.Equal(t, HealthOpStartup, r.LastFailure)
	assert.Equal(t, "no", reachable(r))

	s3 := newS3StoreWithClient(&fakeS3{}, "bucket", "p")
	proven, err := s3.startupCheck(ctx)
	assert.False(t, proven)
	require.NoError(t, err, "credentials resolved: nothing to record")
	s3.credsErr = errors.New("no credentials")
	_, err = s3.startupCheck(ctx)
	require.Error(t, err)
	s = newTestService(t, DefaultConfig(), s3)
	s.checkAtStartup(ctx)
	r = s.Health()
	assert.Equal(t, HealthDegraded, r.Status)
	assert.Equal(t, "s3", r.Kind)
}
