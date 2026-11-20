//go:build integration

package processing_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchjob"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchjob/processing"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/payloadstore/payloadtest"
)

// A job of the new shape whose earlier attempt died mid-delivery (left
// PROCESSING past its lease): the call that takes the claim over reloads the
// job and must resolve its payload again (docs/payload-offload-design.md,
// section 7), or it would deliver the reloaded job's empty body.

func TestProcess_TakeoverOfANewShapeJobResolvesThePayloadAgain(t *testing.T) {
	h := newPayloadHarness(t)
	sub := newPayloadSubscriber(t)
	payload := `{"z": 1, "pad":"` + strings.Repeat("t", 3*payloadtest.OffloadMinBytes) + `" ,"a":2}`
	id, row := h.job(t, sub.url, true, payload)
	require.True(t, row.Offloaded())
	markProcessing(t, h.pool, id, 10*time.Minute)

	code, out := callProcess(t, h.base, id, h.auth.Sign(id))
	assert.Equal(t, http.StatusOK, code)
	assert.Equal(t, true, out["ack"])
	assert.EqualValues(t, 1, sub.calls.Load(), "the dead attempt's job is delivered again")
	assert.Equal(t, payload, sub.last(), "byte for byte, from the payload resolved after the takeover")
	assert.EqualValues(t, 2, h.store.Gets(), "read before the claim, and again for the reloaded job")
	status, _, _ := jobRow(t, h.pool, id)
	assert.Equal(t, "COMPLETED", status)
}

// failAfterFirstRead is a payload store whose range reads fail after the
// first, until restored.
type failAfterFirstRead struct {
	*payloadtest.CountingStore
	failing atomic.Bool
}

func (f *failAfterFirstRead) GetRange(ctx context.Context, key string, offset, length int64) ([]byte, error) {
	first := f.Gets() == 0
	b, err := f.CountingStore.GetRange(ctx, key, offset, length)
	if !first && f.failing.Load() {
		return nil, errors.New("connection reset")
	}
	return b, err
}

// The store fails on the read after the takeover: nothing is sent and no
// attempt is recorded; the claim taken over is left to lapse, and a later
// copy past the lease takes it over and delivers.
func TestProcess_TakeoverWithTheStoreDownLeavesTheClaimToLapse(t *testing.T) {
	h := newPayloadHarness(t)
	store := &failAfterFirstRead{CountingStore: payloadtest.NewCountingStore()}
	store.failing.Store(true)
	svc := payloadtest.Service(t, store)
	repo := dispatchjob.NewRepository(h.pool).UsePayloads(svc)
	r := chi.NewRouter()
	processing.New(repo, h.auth).Mount(r)
	ts := httptest.NewServer(r)
	t.Cleanup(ts.Close)

	sub := newPayloadSubscriber(t)
	payload := `{"pad":"` + strings.Repeat("l", 3*payloadtest.OffloadMinBytes) + `"}`
	h.svc = svc // the job's payload is offloaded to this store
	id, row := h.job(t, sub.url, true, payload)
	require.True(t, row.Offloaded())
	markProcessing(t, h.pool, id, 10*time.Minute)

	code, out := callProcess(t, ts.URL, id, h.auth.Sign(id))
	assert.Equal(t, http.StatusServiceUnavailable, code)
	assert.Equal(t, false, out["ack"], "the queue keeps the message")
	assert.Equal(t, "payload store unavailable", out["message"])
	assert.EqualValues(t, 2, store.Gets())
	assert.Zero(t, sub.calls.Load(), "nothing sent")
	status, attempts, _ := jobRow(t, h.pool, id)
	assert.Equal(t, "PROCESSING", status, "the claim taken over lapses")
	assert.EqualValues(t, 0, attempts, "no attempt counted")
	assert.Zero(t, attemptCount(t, h.pool, id), "no attempt recorded")

	// The store is back and the lease has run out: the next copy delivers.
	store.failing.Store(false)
	markProcessing(t, h.pool, id, 10*time.Minute)
	code, out = callProcess(t, ts.URL, id, h.auth.Sign(id))
	assert.Equal(t, http.StatusOK, code)
	assert.Equal(t, true, out["ack"])
	assert.Equal(t, payload, sub.last())
	status, attempts, _ = jobRow(t, h.pool, id)
	assert.Equal(t, "COMPLETED", status)
	assert.EqualValues(t, 1, attempts)
}
