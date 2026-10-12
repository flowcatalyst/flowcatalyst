//go:build integration

package api

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/event"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apicommon"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
)

// rustWireFixture reads one of the Rust SDK's golden payloads, the bytes the
// SDK writes into outbox_messages.payload and the outbox processor sends
// unchanged as one element of the batch's items.
func rustWireFixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "clients", "rust-sdk", "tests", "fixtures", "wire", name))
	require.NoError(t, err)
	return b
}

// F13 (owner ruling 2026-10-12): POST /api/events/batch unwraps an event
// `data` that the SDK outbox writers sent as a JSON string holding an encoded
// object or array, so it is stored (and fanned out, and delivered) as the
// object; a value sent as itself, and any other string, is stored as sent.
// Before the fix msg_events.data held a JSON string.
func TestBatchIngest_UnwrapsEncodedEventData(t *testing.T) {
	ctx := anchorCtx()
	pool := testpg.Pool(t)
	s := &State{Repo: event.NewRepository(pool)}

	ingest := func(raw string) BatchResultItem {
		t.Helper()
		var item BatchEventItem
		require.NoError(t, json.Unmarshal([]byte(raw), &item))
		out, err := s.batchIngest(ctx, &apicommon.In[BatchRequest]{Body: BatchRequest{Items: []BatchEventItem{item}}})
		require.NoError(t, err)
		require.Len(t, out.Body.Results, 1)
		return out.Body.Results[0]
	}
	stored := func(id string) (string, string) {
		t.Helper()
		var typ, data string
		require.NoError(t, pool.QueryRow(ctx, `SELECT jsonb_typeof(data), data::text FROM msg_events WHERE id = $1`, id).Scan(&typ, &data))
		return typ, data
	}

	// The Rust SDK's full event (the Java/TS/Laravel/framework form).
	res := ingest(string(rustWireFixture(t, "event-full.json")))
	require.Equal(t, "SUCCESS", res.Status, res.Error)
	typ, data := stored(res.ID)
	assert.Equal(t, "object", typ)
	assert.JSONEq(t, `{"orderId":"ord_1","total":1500}`, data)

	// The minimal fixture carries no source, which the batch requires.
	assert.Equal(t, BatchResultItem{Status: "BAD_REQUEST", Error: "source is required"},
		ingest(string(rustWireFixture(t, "event-minimal.json"))))
	var minimal map[string]any
	require.NoError(t, json.Unmarshal(rustWireFixture(t, "event-minimal.json"), &minimal))
	minimal["source"] = "orders"
	withSource, err := json.Marshal(minimal)
	require.NoError(t, err)
	res = ingest(string(withSource))
	require.Equal(t, "SUCCESS", res.Status, res.Error)
	typ, data = stored(res.ID)
	assert.Equal(t, "object", typ)
	assert.JSONEq(t, `{}`, data)

	for _, c := range []struct{ data, typ, want string }{
		{`{"orderId":"ord_2"}`, "object", `{"orderId":"ord_2"}`}, // the Go SDK's form: the value itself
		{`"[{\"line\":1}]"`, "array", `[{"line":1}]`},            // an encoded array
		{`"[]"`, "array", `[]`},                                  // PHP's json_encode([])
		{`"hello"`, "string", `"hello"`},                         // a string meant as a string
		{`"null"`, "string", `"null"`},                           // Java's encoding of a null map: kept
		{`"42"`, "string", `"42"`},
		{`"{not json"`, "string", `"{not json"`},
		{`7`, "number", `7`},
	} {
		res := ingest(`{"type":"orders:sales:order:created","source":"orders","data":` + c.data + `}`)
		require.Equal(t, "SUCCESS", res.Status, res.Error)
		typ, data := stored(res.ID)
		assert.Equal(t, c.typ, typ, c.data)
		assert.JSONEq(t, c.want, data, c.data)
	}

	// The singular POST /api/events receives the value itself and is not
	// changed: a string data stays a string.
	out, err := s.create(ctx, &apicommon.In[CreateEventRequest]{Body: CreateEventRequest{
		EventType: "orders:sales:order:created", Source: "orders", Data: json.RawMessage(`"{\"a\":1}"`),
	}})
	require.NoError(t, err)
	typ, data = stored(out.Body.Event.ID)
	assert.Equal(t, "string", typ)
	assert.JSONEq(t, `"{\"a\":1}"`, data)
}
