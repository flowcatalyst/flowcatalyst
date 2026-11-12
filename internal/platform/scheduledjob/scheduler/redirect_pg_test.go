//go:build integration

package scheduler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/scheduledjob"
)

// A target that answers a redirect is a failed firing (HTTP 3xx, expected
// 2xx) and the redirect target is never contacted. Uses the client the
// service wires, not a hand-built one.
func TestDispatcherTick_RedirectIsAFailedFiring(t *testing.T) {
	ctx := context.Background()
	var second atomic.Int32
	elsewhere := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		second.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer elsewhere.Close()
	first := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, elsewhere.URL, http.StatusTemporaryRedirect)
	}))
	defer first.Close()

	id, d, instances := fireAgainst(t, "sjdsp-redirect", first.URL)
	d.http = NewService(Config{}, nil, nil, nil, nil).dispatcher.http
	require.NoError(t, d.tick(ctx))

	got, err := instances.FindByID(ctx, id)
	require.NoError(t, err)
	assert.NotEqual(t, scheduledjob.InstanceStatusDelivered, got.Status)
	require.NotNil(t, got.DeliveryError)
	assert.Contains(t, *got.DeliveryError, "HTTP 307 (expected 2xx)")
	assert.EqualValues(t, 0, second.Load(), "the redirect target must receive no request")
}
