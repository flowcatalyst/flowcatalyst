//go:build integration

package api

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/humatest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/event"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apicommon"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httpcompat"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
)

// F14 (owner ruling 2026-10-12): an event without a source is not refused;
// the platform stores the type's `{application}:{subdomain}` (its first two
// segments), the source every platform and SDK domain event carries. The
// Java, TypeScript and Laravel SDKs omit an unset source; before the fix the
// batch answered the item BAD_REQUEST "source is required" (terminal for the
// outbox row) and the singular create a 400.
func TestEventIngest_DefaultsAMissingSource(t *testing.T) {
	ctx := anchorCtx()
	pool := testpg.Pool(t)
	s := &State{Repo: event.NewRepository(pool)}
	storedSource := func(id string) string {
		t.Helper()
		var src string
		require.NoError(t, pool.QueryRow(ctx, `SELECT source FROM msg_events WHERE id = $1`, id).Scan(&src))
		return src
	}

	for _, c := range []struct{ raw, want string }{
		{`{"type":"orders:sales:order:created","data":{}}`, "orders:sales"},               // absent
		{`{"type":"orders:sales:order:created","source":null,"data":{}}`, "orders:sales"}, // Java's null
		{`{"type":"orders:sales:order:created","source":"  ","data":{}}`, "orders:sales"}, // blank
		{`{"event_type":"orders:sales:order:created","data":{}}`, "orders:sales"},         // snake_case type
		{`{"type":"orders:sales:order:created","source":"orders","data":{}}`, "orders"},   // given: kept
		{`{"type":" orders:sales:order:created ","data":{}}`, "orders:sales"},             // type trimmed
		{`{"type":"orders:sales","data":{}}`, "orders:sales"},                             // two segments
		{`{"type":"orders","data":{}}`, "orders"},                                         // one segment
	} {
		var item BatchEventItem
		require.NoError(t, json.Unmarshal([]byte(c.raw), &item))
		out, err := s.batchIngest(ctx, &apicommon.In[BatchRequest]{Body: BatchRequest{Items: []BatchEventItem{item}}})
		require.NoError(t, err)
		require.Len(t, out.Body.Results, 1)
		require.Equal(t, "SUCCESS", out.Body.Results[0].Status, "%s: %s", c.raw, out.Body.Results[0].Error)
		assert.Equal(t, c.want, storedSource(out.Body.Results[0].ID), c.raw)
	}

	// A type with no application segment still needs a source.
	out, err := s.batchIngest(ctx, &apicommon.In[BatchRequest]{Body: BatchRequest{Items: []BatchEventItem{
		{Type: ":sales:order:created", Data: json.RawMessage(`{}`)},
	}}})
	require.NoError(t, err)
	assert.Equal(t, BatchResultItem{Status: "BAD_REQUEST", Error: "source is required"}, out.Body.Results[0])

	// The singular POST /api/events: source is optional on the wire and
	// defaulted the same way.
	httpcompat.Init()
	_, hapi := humatest.New(t)
	hapi.UseMiddleware(func(hctx huma.Context, next func(huma.Context)) {
		next(huma.WithContext(hctx, auth.WithContext(hctx.Context(), auth.FromContext(anchorCtx()))))
	})
	Register(hapi, s)
	resp := hapi.Post("/api/events", map[string]any{"eventType": "orders:sales:order:created", "data": map[string]any{"a": 1}})
	require.Equal(t, http.StatusCreated, resp.Code, resp.Body.String())
	var created CreateEventResponse
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &created))
	assert.Equal(t, "orders:sales", created.Event.Source)
	assert.Equal(t, "orders:sales", storedSource(created.Event.ID))

	resp = hapi.Post("/api/events", map[string]any{"eventType": "orders:sales:order:created", "source": "orders", "data": map[string]any{}})
	require.Equal(t, http.StatusCreated, resp.Code, resp.Body.String())
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &created))
	assert.Equal(t, "orders", created.Event.Source)

	resp = hapi.Post("/api/events", map[string]any{"eventType": ":sales:order:created", "data": map[string]any{}})
	require.Equal(t, http.StatusBadRequest, resp.Code, resp.Body.String())
	assert.Contains(t, resp.Body.String(), "source is required")
}
