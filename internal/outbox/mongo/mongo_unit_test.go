package mongo

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/flowcatalyst/flowcatalyst-go/internal/common"
	"github.com/flowcatalyst/flowcatalyst-go/internal/outbox"
)

// These need no MongoDB server: they pin the BSON the repository reads and
// writes (the server-backed tests are mongo_integration_test.go).

func TestDocDecodesTheSDKShapes(t *testing.T) {
	cases := []struct {
		name      string
		in        bson.D
		wantErr   bool
		status    int32
		retry     int32
		group     *string
		errorText *string
	}{
		{
			name: "int32 fields", in: bson.D{{Key: "id", Value: "a"}, {Key: "status", Value: int32(9)}, {Key: "retry_count", Value: int32(3)}, {Key: "message_group", Value: "g"}, {Key: "error_message", Value: ""}},
			status: 9, retry: 3, group: new("g"), errorText: new(""),
		},
		{
			name: "int64 and whole doubles narrow", in: bson.D{{Key: "id", Value: "b"}, {Key: "status", Value: int64(4)}, {Key: "retry_count", Value: float64(2)}, {Key: "message_group", Value: nil}, {Key: "error_message", Value: nil}},
			status: 4, retry: 2,
		},
		{name: "unknown fields ignored", in: bson.D{{Key: "id", Value: "c"}, {Key: "client_id", Value: "z"}, {Key: "headers", Value: "{}"}, {Key: "payload_size", Value: int32(4)}, {Key: "extra", Value: bson.D{{Key: "x", Value: 1}}}}},
		{name: "string status refused", in: bson.D{{Key: "id", Value: "d"}, {Key: "status", Value: "oops"}}, wantErr: true},
		{name: "fractional double refused", in: bson.D{{Key: "id", Value: "e"}, {Key: "status", Value: 1.5}}, wantErr: true},
		{name: "overflowing int64 refused", in: bson.D{{Key: "id", Value: "f"}, {Key: "retry_count", Value: int64(1 << 40)}}, wantErr: true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			raw, err := bson.Marshal(c.in)
			require.NoError(t, err)
			var d doc
			err = bson.Unmarshal(raw, &d)
			if c.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, c.status, d.Status)
			assert.Equal(t, c.retry, d.RetryCount)
			assert.Equal(t, c.group, d.MessageGroup)
			assert.Equal(t, c.errorText, d.ErrorMessage)
		})
	}
}

func TestFilterEncodesCodesAsInt32(t *testing.T) {
	raw, err := bson.Marshal(claimFilter(outbox.ClaimFilter{Groups: []string{"g"}, IDs: []string{"i"}}))
	require.NoError(t, err)
	v := bson.Raw(raw).Lookup("status")
	assert.Equal(t, bson.TypeInt32, v.Type)
	assert.Equal(t, int32(common.OutboxPending), v.Int32())

	raw, err = bson.Marshal(bson.M{"s": bson.M{"$in": terminalFailureCodes()}})
	require.NoError(t, err)
	arr := bson.Raw(raw).Lookup("s", "$in").Array()
	vals, err := arr.Values()
	require.NoError(t, err)
	require.Len(t, vals, 2)
	for _, e := range vals {
		assert.Equal(t, bson.TypeInt32, e.Type)
	}

	raw, err = bson.Marshal(bson.M{"m": bson.M{"$in": ungroupedValues()}})
	require.NoError(t, err)
	vals, err = bson.Raw(raw).Lookup("m", "$in").Array().Values()
	require.NoError(t, err)
	require.Len(t, vals, 2)
	assert.Equal(t, bson.TypeNull, vals[0].Type)
	assert.Equal(t, bson.TypeString, vals[1].Type)

	raw, err = bson.Marshal(bson.M{"e": markerPattern(outbox.BlockingMarker, outbox.SkippedMarker)})
	require.NoError(t, err)
	assert.Equal(t, bson.TypeRegex, bson.Raw(raw).Lookup("e").Type)
}
