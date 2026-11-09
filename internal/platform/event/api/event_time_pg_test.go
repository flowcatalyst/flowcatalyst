//go:build integration

package api

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/event"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apicommon"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
)

// A batch item's optional time is the event's time, as the producer recorded
// it; createdAt stays the ingest time. A bad or far-future time is that item's
// BAD_REQUEST, and the rest of the batch lands. (Wire form parsed through
// UnmarshalJSON, as the SDK outbox sends it.)
func TestBatchIngest_EventTime(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	s := &State{Repo: event.NewRepository(pool)}
	caller := auth.WithContext(ctx, &auth.AuthContext{
		PrincipalID: "prn_evtime", Scope: auth.ScopeAnchor,
		AllApplications: true, Permissions: []string{"platform:messaging:batch:events-write"},
	})

	recorded := time.Now().UTC().Add(-3 * time.Hour).Truncate(time.Millisecond)
	var items []BatchEventItem
	require.NoError(t, json.Unmarshal([]byte(`[
		{"id":"evtime0000001","type":"evtime:a:b:c","data":{"x":1},"time":"`+recorded.Format(time.RFC3339Nano)+`"},
		{"id":"evtime0000002","type":"evtime:a:b:c","data":{"x":2}},
		{"id":"evtime0000003","type":"evtime:a:b:c","data":{"x":3},"time":"yesterday"},
		{"id":"evtime0000004","type":"evtime:a:b:c","data":{"x":4},"time":"`+time.Now().UTC().Add(time.Hour).Format(time.RFC3339)+`"}
	]`), &items))
	before := time.Now().UTC().Add(-time.Second)
	out, err := s.batchIngest(caller, &apicommon.In[BatchRequest]{Body: BatchRequest{Items: items}})
	require.NoError(t, err)
	require.Len(t, out.Body.Results, 4)
	assert.Equal(t, "SUCCESS", out.Body.Results[0].Status)
	assert.Equal(t, "SUCCESS", out.Body.Results[1].Status)
	assert.Equal(t, "BAD_REQUEST", out.Body.Results[2].Status)
	assert.Equal(t, "BAD_REQUEST", out.Body.Results[3].Status)

	read := func(id string) (tm, created time.Time) {
		require.NoError(t, pool.QueryRow(ctx, `SELECT time, created_at FROM msg_events WHERE id = $1`, id).Scan(&tm, &created))
		return
	}
	tm, created := read("evtime0000001")
	assert.True(t, tm.Equal(recorded), "the producer's time: got %s want %s", tm, recorded)
	assert.True(t, created.After(before), "createdAt is the ingest time")
	tm, created = read("evtime0000002")
	assert.True(t, tm.Equal(created), "absent: the ingest time, as before")
}
