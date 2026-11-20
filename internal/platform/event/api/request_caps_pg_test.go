//go:build integration

package api

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/event"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httpcompat"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/payloadstore"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/payloadstore/payloadtest"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
)

// The request caps of the event ingest routes (docs/payload-offload-design.md,
// section 8): the batch routes take FC_INGEST_BATCH_MAX_BYTES (16 MiB), the
// single create twice the largest payload plus 64 KiB, and each event's data
// is held to the largest payload whether or not the payload table is
// written: FC_PAYLOAD_INLINE_MAX_BYTES (1 MiB) with the switch off, whatever
// the store; FC_PAYLOAD_MAX_BYTES (5 MB) with it on, a store and offloading.

// capsServer serves the event routes over HTTP (huma refuses an oversized
// body before the handler), as an anchor.
func capsServer(t *testing.T, s *State) string {
	t.Helper()
	httpcompat.Init()
	mux := chi.NewMux()
	mux.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r.WithContext(auth.WithContext(r.Context(), auth.FromContext(anchorCtx()))))
		})
	})
	api := humachi.New(mux, huma.DefaultConfig("test", "dev"))
	Register(api, s)
	httpcompat.RelaxRequestBodies(api)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv.URL
}

func postBody(t *testing.T, url, body string) (int, string) {
	t.Helper()
	resp, err := http.Post(url, "application/json", strings.NewReader(body))
	require.NoError(t, err)
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(raw)
}

// jsonOf is a JSON string of exactly n bytes, quotes included.
func jsonOf(n int, c string) string { return `"` + strings.Repeat(c, n-2) + `"` }

func batchOf(typ string, data ...string) string {
	items := make([]string, len(data))
	for i, d := range data {
		items[i] = `{"type":"` + typ + `","source":"test://caps","data":` + d + `}`
	}
	return `{"items":[` + strings.Join(items, ",") + `]}`
}

func TestEventRoutes_RequestCaps(t *testing.T) {
	for _, c := range []struct {
		name    string
		svc     func(t *testing.T, store *payloadtest.CountingStore) *payloadstore.Service
		largest int
	}{
		// The switch off with a store configured: every payload is kept in
		// the database, so the largest is the inline maximum.
		{"switch off", func(t *testing.T, store *payloadtest.CountingStore) *payloadstore.Service {
			return payloadtest.Service(t, store)
		}, payloadstore.DefaultInlineMaxBytes},
		{"switch on, offloading", func(t *testing.T, store *payloadtest.CountingStore) *payloadstore.Service {
			return payloadtest.WritingService(t, store, nil)
		}, payloadstore.DefaultMaxBytes},
	} {
		t.Run(c.name, func(t *testing.T) {
			store := payloadtest.NewCountingStore()
			svc := c.svc(t, store)
			require.Equal(t, c.largest, svc.MaxPayloadBytes())
			repo := event.NewRepository(testpg.Pool(t)).UsePayloads(svc)
			base := capsServer(t, &State{Repo: repo})
			typ := pipelineType()

			stored := func(id string) bool {
				t.Helper()
				ev, err := repo.FindRawByID(t.Context(), id)
				require.NoError(t, err)
				return ev != nil
			}
			results := func(body string) []BatchResultItem {
				t.Helper()
				var out BatchResponse
				require.NoError(t, json.Unmarshal([]byte(body), &out), body)
				return out.Results
			}

			// Several items totalling well over 1 MiB, each under the limit, on
			// both batch routes.
			for _, path := range []string{"/api/events/batch", "/bff/events/batch"} {
				data := make([]string, 6)
				for i := range data {
					data[i] = jsonOf(400_000, "b")
				}
				st, body := postBody(t, base+path, batchOf(typ, data...))
				require.Equal(t, http.StatusCreated, st, "%s: %.300s", path, body)
				r := results(body)
				require.Len(t, r, 6)
				for i := range r {
					require.Equal(t, "SUCCESS", r[i].Status, "%s item %d: %s", path, i, r[i].Error)
					assert.True(t, stored(r[i].ID))
				}
			}

			// An item over the largest payload is refused in its own slot,
			// the rest of the batch is stored.
			st, body := postBody(t, base+"/api/events/batch", batchOf(typ, `{"n":1}`, jsonOf(c.largest+1, "o"), `{"n":2}`))
			require.Equal(t, http.StatusCreated, st, "%.300s", body)
			r := results(body)
			require.Len(t, r, 3)
			assert.Equal(t, "SUCCESS", r[0].Status)
			assert.Equal(t, "BAD_REQUEST", r[1].Status)
			assert.True(t, strings.HasPrefix(r[1].Error, "PAYLOAD_TOO_LARGE"), r[1].Error)
			assert.Equal(t, "SUCCESS", r[2].Status)
			assert.True(t, stored(r[0].ID))
			assert.False(t, stored(r[1].ID), "the refused item is not stored")
			assert.True(t, stored(r[2].ID))

			// A batch over the batch cap is refused whole, 413.
			st, body = postBody(t, base+"/api/events/batch", batchOf(typ, jsonOf(int(payloadstore.DefaultIngestBatchMaxBytes)+10, "z")))
			assert.Equal(t, http.StatusRequestEntityTooLarge, st, "%.300s", body)
			assert.Contains(t, body, `"error":"BODY_TOO_LARGE"`)

			// A single event of the largest payload is accepted, one byte more
			// is PAYLOAD_TOO_LARGE, and a body over the single route's cap 413.
			single := func(data string) (int, string) {
				return postBody(t, base+"/api/events", `{"eventType":"`+typ+`","source":"test://caps","data":`+data+`}`)
			}
			puts := store.Puts()
			st, body = single(jsonOf(c.largest, "s"))
			require.Equal(t, http.StatusCreated, st, "%.300s", body)
			if c.largest > payloadstore.DefaultInlineMaxBytes {
				assert.Greater(t, store.Puts(), puts, "offloaded")
			}
			st, body = single(jsonOf(c.largest+1, "s"))
			assert.Equal(t, http.StatusBadRequest, st, "%.300s", body)
			assert.Contains(t, body, `"error":"PAYLOAD_TOO_LARGE"`)
			st, body = single(jsonOf(int(svc.PayloadRequestMaxBytes())+1, "s"))
			assert.Equal(t, http.StatusRequestEntityTooLarge, st, "%.300s", body)
			assert.Contains(t, body, `"error":"BODY_TOO_LARGE"`)
		})
	}
}
