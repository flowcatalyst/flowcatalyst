package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/payloadstore"
)

// failingPuts is a memory store whose writes fail.
type failingPuts struct{ *payloadstore.MemoryStore }

func (failingPuts) Put(context.Context, string, []byte) error {
	return errors.New("connection refused")
}

// degradedService is a payload service whose one store write has just failed,
// its payload kept in the database.
func degradedService(t *testing.T) *payloadstore.Service {
	t.Helper()
	cfg := payloadstore.DefaultConfig()
	cfg.TableWrites = true
	cfg.OffloadMinBytes = payloadstore.MinOffloadMinBytes
	s, err := payloadstore.NewWithStore(cfg, failingPuts{payloadstore.NewMemoryStore()}, nil)
	require.NoError(t, err)
	big := []byte(strings.Repeat("a", 4*payloadstore.MinOffloadMinBytes))
	res := s.Prepare(context.Background(), []payloadstore.Item{{Data: big}})
	require.NoError(t, res[0].Err, "kept in the database")
	return s
}

func getJSON(t *testing.T, h http.Handler, path string) (int, map[string]any) {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	var out map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out), rec.Body.String())
	return rec.Code, out
}

// GET /health/payload-store reports the store, always 200: no store is a
// healthy NOT_CONFIGURED, a store whose write failed is DEGRADED, and neither
// fails the request.
func TestPayloadStoreHealth(t *testing.T) {
	none, err := payloadstore.NewWithStore(payloadstore.DefaultConfig(), nil, nil)
	require.NoError(t, err)
	code, out := getJSON(t, payloadStoreHealth(none), PayloadStoreHealthPath)
	assert.Equal(t, http.StatusOK, code)
	assert.Equal(t, "NOT_CONFIGURED", out["status"])
	assert.Equal(t, false, out["configured"])

	down := degradedService(t)
	code, out = getJSON(t, payloadStoreHealth(down), PayloadStoreHealthPath)
	assert.Equal(t, http.StatusOK, code, "a store that is down is a warning, not a failed probe")
	assert.Equal(t, "DEGRADED", out["status"])
	assert.Equal(t, false, out["reachable"])
	assert.Equal(t, "put", out["lastFailure"])
	assert.Equal(t, "memory", out["kind"])
}

// The metrics listener's /ready says the payload store's status where the
// platform API runs, and stays "ready" whatever it is.
func TestReadyReportsThePayloadStore(t *testing.T) {
	t.Cleanup(func() { runningPayloads.Store(nil) })
	h := metricsRouter(EnvCfg{PlatformEnabled: true})

	runningPayloads.Store(nil)
	code, out := getJSON(t, h, "/ready")
	assert.Equal(t, http.StatusOK, code)
	assert.NotContains(t, out, "payload_store", "no platform API in this process")

	runningPayloads.Store(degradedService(t))
	code, out = getJSON(t, h, "/ready")
	assert.Equal(t, http.StatusOK, code, "a degraded store never makes the process unready")
	assert.Equal(t, "ready", out["status"])
	assert.Equal(t, "DEGRADED", out["payload_store"])
}
