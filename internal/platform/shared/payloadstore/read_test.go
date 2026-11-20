package payloadstore

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFromJoin(t *testing.T) {
	assert.Nil(t, FromJoin(nil, nil, nil, nil), "no payload_id: the legacy shape")

	id, data, ref := "p1", `{"a":1}`, "v1:k:0:1:zstd:00000000"
	size := int32(7)
	got := FromJoin(&id, &data, nil, &size)
	require.NotNil(t, got)
	assert.Equal(t, Stored{ID: id, Data: &data, Size: 7}, *got)
	assert.False(t, got.Offloaded())

	got = FromJoin(&id, nil, &ref, &size)
	require.NotNil(t, got)
	assert.True(t, got.Offloaded())

	// The join found no row: neither data nor ref, size 0.
	got = FromJoin(&id, nil, nil, nil)
	require.NotNil(t, got)
	assert.Equal(t, Stored{ID: id}, *got)
}

func TestResolveWith_NoService(t *testing.T) {
	ctx := context.Background()
	data, ref := `{"a":1}`, "v1:k:0:1:zstd:00000000"
	got, err := ResolveWith(ctx, nil, Stored{ID: "p", Data: &data, Size: len(data)})
	require.NoError(t, err)
	assert.Equal(t, []byte(data), got)

	_, err = ResolveWith(ctx, nil, Stored{ID: "p", Ref: &ref, Size: 1})
	assert.ErrorIs(t, err, ErrStoreNotConfigured)

	_, err = ResolveWith(ctx, nil, Stored{ID: "p"})
	assert.ErrorIs(t, err, ErrMissing)
}

func TestResolveWith_Service(t *testing.T) {
	s := newTestService(t, DefaultConfig(), NewMemoryStore())
	payload := payloadOf(9000, 4)
	_, st := offloadOne(t, s, payload)
	got, err := ResolveWith(context.Background(), s, st)
	require.NoError(t, err)
	assert.Equal(t, payload, got)
}
