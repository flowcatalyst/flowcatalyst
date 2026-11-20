package server

import (
	"encoding/json"
	"net/http"
	"sync/atomic"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/payloadstore"
)

// PayloadStoreHealthPath is where the API listener reports the payload
// store's health (docs/payload-offload-design.md, sections 7 and 13), in a
// process that runs the platform API.
const PayloadStoreHealthPath = "/health/payload-store"

// runningPayloads is the payload service of this process, set by Run once the
// platform is wired (nil when this process runs no platform API). The metrics
// listener's /ready reads it.
var runningPayloads atomic.Pointer[payloadstore.Service]

// payloadStoreHealth answers GET /health/payload-store with the store's
// health report, always 200: a store that is down is a warning, not a reason
// to take the instance out of service, because ingest falls back to the
// database (section 7). The report is derived from the outcomes of the
// store's operations and costs no store call. Like /health it needs no
// credentials, and it names the store's kind, never its bucket or directory.
func payloadStoreHealth(svc *payloadstore.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		_ = json.NewEncoder(w).Encode(svc.Health())
	}
}

// payloadStoreReady is the payload store's status for the metrics listener's
// /ready: ok is false in a process without the platform API.
func payloadStoreReady() (status payloadstore.HealthStatus, ok bool) {
	svc := runningPayloads.Load()
	if svc == nil {
		return "", false
	}
	return svc.Health().Status, true
}
