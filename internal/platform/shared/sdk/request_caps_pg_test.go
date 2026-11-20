//go:build integration

package sdk

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/payloadstore"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/payloadstore/payloadtest"
	"github.com/flowcatalyst/flowcatalyst-go/internal/tsid"
)

// The request caps of the dispatch-job ingest routes
// (docs/payload-offload-design.md, section 8): the batch takes
// FC_INGEST_BATCH_MAX_BYTES (16 MiB), the singular twice the largest payload
// plus 64 KiB (a payload is a JSON string here, and escaping can nearly
// double it), and each job's payload is held to the largest payload whether
// or not the payload table is written.
func TestDispatchJobRoutes_RequestCaps(t *testing.T) {
	for _, c := range []struct {
		name    string
		svc     func(t *testing.T, store *payloadtest.CountingStore) *payloadstore.Service
		largest int
	}{
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
			rig := newJobRig(t, store, svc)
			code := "sdkcaps:" + strings.ToLower(tsid.GenerateUntyped()) + ":job"
			jobs := func(ids ...string) int {
				t.Helper()
				var n int
				require.NoError(t, rig.pool.QueryRow(context.Background(),
					`SELECT count(*) FROM msg_dispatch_jobs WHERE id = ANY($1)`, ids).Scan(&n))
				return n
			}
			batch := func(payloads ...string) (int, string) {
				items := make([]string, len(payloads))
				for i, p := range payloads {
					items[i] = jobItem(code, p, "")
				}
				resp, body := postJSON(t, rig.api+"/api/dispatch-jobs/batch", `{"items":[`+strings.Join(items, ",")+`]}`)
				return resp.StatusCode, body
			}
			results := func(body string) []BatchResultItem {
				t.Helper()
				var out BatchResponse
				require.NoError(t, json.Unmarshal([]byte(body), &out), body)
				return out.Results
			}

			// Several jobs totalling well over 1 MiB, each under the limit.
			payloads := make([]string, 6)
			for i := range payloads {
				payloads[i] = `{"pad":"` + strings.Repeat("p", 400_000) + `"}`
			}
			st, body := batch(payloads...)
			require.Equal(t, http.StatusCreated, st, "%.300s", body)
			r := results(body)
			require.Len(t, r, 6)
			ids := make([]string, len(r))
			for i := range r {
				require.Equal(t, "SUCCESS", r[i].Status, "item %d: %s", i, r[i].Error)
				ids[i] = r[i].ID
			}
			assert.Equal(t, 6, jobs(ids...))

			// A payload over the largest is refused in its own slot; the rest
			// of the batch is stored.
			st, body = batch(`{"n":1}`, strings.Repeat("o", c.largest+1), `{"n":2}`)
			require.Equal(t, http.StatusCreated, st, "%.300s", body)
			r = results(body)
			require.Len(t, r, 3)
			assert.Equal(t, "SUCCESS", r[0].Status)
			assert.Equal(t, "BAD_REQUEST", r[1].Status)
			assert.True(t, strings.HasPrefix(r[1].Error, "PAYLOAD_TOO_LARGE"), r[1].Error)
			assert.Equal(t, "SUCCESS", r[2].Status)
			assert.Equal(t, 1, jobs(r[0].ID))
			assert.Equal(t, 0, jobs(r[1].ID), "the refused item makes no job")
			assert.Equal(t, 1, jobs(r[2].ID))

			// A batch over the batch cap is refused whole, 413.
			st, body = batch(strings.Repeat("z", int(payloadstore.DefaultIngestBatchMaxBytes)))
			assert.Equal(t, http.StatusRequestEntityTooLarge, st, "%.300s", body)
			assert.Contains(t, body, `"error":"BODY_TOO_LARGE"`)

			// The singular: a payload of the largest size made of quotes, so
			// the request carries it escaped at twice its length, is
			// accepted; one byte more is PAYLOAD_TOO_LARGE; a body over the cap
			// is 413.
			single := func(payload string) (int, string) {
				resp, body := postJSON(t, rig.api+"/api/dispatch-jobs", jobItem(code, payload, ""))
				return resp.StatusCode, body
			}
			puts := store.Puts()
			st, body = single(strings.Repeat(`"`, c.largest))
			require.Equal(t, http.StatusCreated, st, "%.300s", body)
			var created CreatedResponse
			require.NoError(t, json.Unmarshal([]byte(body), &created))
			assert.Equal(t, 1, jobs(created.ID))
			if c.largest > payloadstore.DefaultInlineMaxBytes {
				assert.Greater(t, store.Puts(), puts, "offloaded")
				assert.NotNil(t, readJob(t, rig.pool, created.ID).pRef)
			} else {
				j := readJob(t, rig.pool, created.ID)
				require.NotNil(t, j.payload, "the legacy shape")
				assert.Len(t, *j.payload, c.largest)
			}
			st, body = single(strings.Repeat(`"`, c.largest+1))
			assert.Equal(t, http.StatusBadRequest, st, "%.300s", body)
			assert.Contains(t, body, `"error":"PAYLOAD_TOO_LARGE"`)
			st, body = single(strings.Repeat("x", int(svc.PayloadRequestMaxBytes())))
			assert.Equal(t, http.StatusRequestEntityTooLarge, st, "%.300s", body)
			assert.Contains(t, body, `"error":"BODY_TOO_LARGE"`)
		})
	}
}
