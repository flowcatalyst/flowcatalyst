//go:build integration

package api

import (
	"context"
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

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchjob"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httpcompat"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/payloadstore"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/payloadstore/payloadtest"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/platformsink"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/go-sdk/usecasepgx"
)

// quotesJSON is valid JSON of exactly n bytes (n >= 4), two thirds of it
// quotes, so that sent as a JSON string it is nearly twice as long.
func quotesJSON(n int) string {
	k := (n - 4) / 3
	return "[" + strings.Repeat(`"",`, k) + `""` + strings.Repeat(" ", n-(3*k+4)) + "]"
}

// The payload edit's request cap (docs/payload-offload-design.md, section 8):
// twice the largest payload plus 64 KiB, so an edit up to the largest payload
// is taken even when escaping nearly doubles it; the payload itself is held
// to the largest payload (1 MiB with the switch off whatever the store, 5 MB
// with it on and offloading); a larger body is 413.
func TestEditPayload_RequestCap(t *testing.T) {
	for _, c := range []struct {
		name    string
		svc     func(t *testing.T, store *payloadtest.CountingStore) *payloadstore.Service
		largest int
		prefix  string
	}{
		{"switch off", func(t *testing.T, store *payloadtest.CountingStore) *payloadstore.Service {
			return payloadtest.Service(t, store)
		}, payloadstore.DefaultInlineMaxBytes, "edcpa"},
		{"switch on, offloading", func(t *testing.T, store *payloadtest.CountingStore) *payloadstore.Service {
			return payloadtest.WritingService(t, store, nil)
		}, payloadstore.DefaultMaxBytes, "edcpb"},
	} {
		t.Run(c.name, func(t *testing.T) {
			httpcompat.Init()
			pool := testpg.Pool(t)
			store := payloadtest.NewCountingStore()
			svc := c.svc(t, store)
			require.Equal(t, c.largest, svc.MaxPayloadBytes())
			s := &State{Repo: dispatchjob.NewRepository(pool).UsePayloads(svc), UoW: usecasepgx.New(pool, platformsink.New())}
			mux := chi.NewMux()
			mux.Use(func(next http.Handler) http.Handler {
				return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					next.ServeHTTP(w, r.WithContext(auth.WithContext(r.Context(), auth.FromContext(anchor()))))
				})
			})
			api := humachi.New(mux, huma.DefaultConfig("test", "dev"))
			Register(api, s)
			httpcompat.RelaxRequestBodies(api)
			srv := httptest.NewServer(mux)
			t.Cleanup(srv.Close)

			job := seedJobs(t, pool, c.prefix, "edcaps:job", "FAILED", nil, 1)[0]
			edit := func(base, payload string) (int, string) {
				body, err := json.Marshal(map[string]string{"payload": payload, "payloadContentType": "application/json"})
				require.NoError(t, err)
				req, err := http.NewRequest(http.MethodPut, srv.URL+base+job+"/payload", strings.NewReader(string(body)))
				require.NoError(t, err)
				req.Header.Set("Content-Type", "application/json")
				resp, err := http.DefaultClient.Do(req)
				require.NoError(t, err)
				defer resp.Body.Close()
				raw, _ := io.ReadAll(resp.Body)
				return resp.StatusCode, string(raw)
			}

			largest := quotesJSON(c.largest)
			for _, base := range []string{"/api/dispatch-jobs/", "/bff/dispatch-jobs/"} {
				st, body := edit(base, largest)
				require.Equal(t, http.StatusOK, st, "%s: %.300s", base, body)
			}
			var payload *string
			var payloadID *string
			require.NoError(t, pool.QueryRow(context.Background(),
				`SELECT payload, payload_id FROM msg_dispatch_jobs WHERE id = $1`, job).Scan(&payload, &payloadID))
			if c.largest > payloadstore.DefaultInlineMaxBytes {
				assert.Nil(t, payload)
				assert.NotNil(t, payloadID, "a payload row of the job's own")
				assert.Positive(t, store.Puts(), "offloaded")
			} else {
				require.NotNil(t, payload, "the legacy shape")
				assert.Len(t, *payload, c.largest)
			}

			st, body := edit("/api/dispatch-jobs/", quotesJSON(c.largest+1))
			assert.Equal(t, http.StatusBadRequest, st, "%.300s", body)
			assert.Contains(t, body, `"error":"PAYLOAD_TOO_LARGE"`)
			st, body = edit("/api/dispatch-jobs/", strings.Repeat("x", int(svc.PayloadRequestMaxBytes())))
			assert.Equal(t, http.StatusRequestEntityTooLarge, st, "%.300s", body)
			assert.Contains(t, body, `"error":"BODY_TOO_LARGE"`)
		})
	}
}
