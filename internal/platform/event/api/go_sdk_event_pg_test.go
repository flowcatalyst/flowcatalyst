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
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/flowcatalyst-go/internal/tsid"
	"github.com/flowcatalyst/go-sdk/outboxpgx"
	"github.com/flowcatalyst/go-sdk/usecasepgx"
)

type goSDKEvent struct{}

// goSDKEventID is fresh per run: the SDK's deduplicationId is the event id,
// and the batch endpoint drops an event whose deduplicationId is stored
// (the Rust fixtures in this package use 0HZXEQ5Y8JY5Z).
var goSDKEventID = tsid.GenerateUntyped()

func (goSDKEvent) EventID() string       { return goSDKEventID }
func (goSDKEvent) EventType() string     { return "gosdk:sales:order:placed" }
func (goSDKEvent) SpecVersion() string   { return "1.0" }
func (goSDKEvent) Source() string        { return "gosdk:sales" }
func (goSDKEvent) Subject() string       { return "gosdk.order.ord_1" }
func (goSDKEvent) Time() time.Time       { return time.Unix(0, 0) }
func (goSDKEvent) PrincipalID() string   { return "prn_0HZXEQ5Y8JY5A" }
func (goSDKEvent) CorrelationID() string { return "corr-1" }
func (goSDKEvent) CausationID() string   { return "" }
func (goSDKEvent) ExecutionID() string   { return "" }
func (goSDKEvent) MessageGroup() string  { return "orders:ord_1" }
func (goSDKEvent) ToDataJSON() ([]byte, error) {
	return []byte(`{"orderId":"ord_1","lines":["a"]}`), nil
}

// F13: the Go SDK's outbox event (data sent as the object itself, the batch
// item's camelCase members) goes through POST /api/events/batch with its data
// stored as sent; the unwrap at ingest leaves it alone.
func TestBatchIngest_GoSDKOutboxEventDataStoredAsSent(t *testing.T) {
	ctx := anchorCtx()
	pool := testpg.Pool(t)
	require.NoError(t, outboxpgx.InitSchema(ctx, pool))
	tx, err := pool.Begin(ctx)
	require.NoError(t, err)
	require.NoError(t, outboxpgx.NewSink(outboxpgx.DefaultConfig()).WriteEvent(ctx, usecasepgx.WrapTxForBootstrap(tx), goSDKEvent{}))
	require.NoError(t, tx.Commit(ctx))
	var payload string
	require.NoError(t, pool.QueryRow(context.Background(),
		`SELECT payload FROM outbox_messages WHERE type = 'EVENT' ORDER BY created_at DESC LIMIT 1`).Scan(&payload))

	var item BatchEventItem
	require.NoError(t, json.Unmarshal([]byte(payload), &item))
	s := &State{Repo: event.NewRepository(pool)}
	out, err := s.batchIngest(ctx, &apicommon.In[BatchRequest]{Body: BatchRequest{Items: []BatchEventItem{item}}})
	require.NoError(t, err)
	require.Equal(t, "SUCCESS", out.Body.Results[0].Status, out.Body.Results[0].Error)

	row := fetchEventRow(t, ctx, pool, out.Body.Results[0].ID)
	assert.JSONEq(t, `{"orderId":"ord_1","lines":["a"]}`, row.Data)
	assert.Equal(t, "gosdk:sales:order:placed", row.Type)
}
