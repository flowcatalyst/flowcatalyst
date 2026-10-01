package router

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/netguard"
)

// The router applies no delivery policy (owner decision #56): its targets are
// first-party, and the endpoint that receives a pointer checks its own target.
// Under the strict production policy, a loopback target (httptest listens on
// 127.0.0.1) is still delivered to.
func TestMediatorIgnoresTheDeliveryPolicy(t *testing.T) {
	if netguard.Default.AllowLoopback || netguard.Default.AllowPrivate {
		t.Skip("the environment relaxes the policy; this test needs the strict default")
	}
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	cfg := DefaultMediatorConfig()
	cfg.HTTPVersion = HTTPVersion1
	cfg.Timeout = 5 * time.Second
	client := newClientBuilder(cfg)()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, srv.URL+"/hook", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("delivery to a loopback target failed: %v", err)
	}
	resp.Body.Close()
	if hits.Load() != 1 {
		t.Fatalf("target hit %d times, want 1", hits.Load())
	}
}
