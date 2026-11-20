package stream

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/payloadstore"
)

// recordingExpirer records the cutoff it is called with.
type recordingExpirer struct {
	cutoffs []time.Time
	err     error
}

func (r *recordingExpirer) RemoveExpired(_ context.Context, cutoff time.Time) (int, error) {
	r.cutoffs = append(r.cutoffs, cutoff)
	return 0, r.err
}

// The cutoff of the stored payloads is the retention of msg_payloads plus
// PayloadExpirySlackDays (docs/payload-offload-design.md, section 7), and a
// failure to clean is logged, not fatal.
func TestPartitionManager_PayloadCutoff(t *testing.T) {
	now := time.Date(2026, 11, 20, 3, 0, 0, 0, time.UTC)
	rec := &recordingExpirer{}
	m := &PartitionManager{PayloadStore: rec}
	m.expirePayloads(context.Background(), now, 90)
	require.Len(t, rec.cutoffs, 1)
	assert.Equal(t, now.AddDate(0, 0, -125), rec.cutoffs[0], "125 days at the default 90-day retention")

	rec.err = errors.New("permission denied")
	m.expirePayloads(context.Background(), now, 30)
	assert.Equal(t, now.AddDate(0, 0, -65), rec.cutoffs[1])

	// No store, or one the platform does not clean: nothing is called.
	(&PartitionManager{}).expirePayloads(context.Background(), now, 90)
}

// With a file payload store, the pass removes the date directories past the
// cutoff from a real directory and keeps the rest.
func TestPartitionManager_RemovesExpiredFilePayloads(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	exp, err := payloadstore.OpenExpirer(ctx, "file://"+dir)
	require.NoError(t, err)
	require.NotNil(t, exp)
	fs, ok := exp.(*payloadstore.FileStore)
	require.True(t, ok)

	now := time.Date(2026, 11, 20, 3, 0, 0, 0, time.UTC)
	old := now.AddDate(0, 0, -126).Format(time.DateOnly)
	edge := now.AddDate(0, 0, -125).Format(time.DateOnly)
	require.NoError(t, fs.Put(ctx, "v1/clt_A/"+old+"/seg1", []byte("x")))
	require.NoError(t, fs.Put(ctx, "v1/clt_A/"+edge+"/seg2", []byte("x")))
	require.NoError(t, fs.Put(ctx, "v1/clt_B/"+old+"/seg3", []byte("x")))

	m := &PartitionManager{PayloadStore: exp}
	m.expirePayloads(ctx, now, 90)

	gone := func(rel string) bool {
		_, err := os.Stat(filepath.Join(dir, filepath.FromSlash(rel)))
		return errors.Is(err, os.ErrNotExist)
	}
	assert.True(t, gone("v1/clt_A/"+old))
	assert.False(t, gone("v1/clt_A/"+edge+"/seg2"), "the cutoff's own day is kept")
	assert.True(t, gone("v1/clt_B"), "a client directory left empty is removed")
}
