//go:build integration

package scheduler

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/scheduledjob"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/serviceaccount"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/scheduledjob/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecaseop"
)

func TestMain(m *testing.M) { testpg.RunMain(m) }

// TestDispatcherTick_OrphanInstance_MarkedDeliveryFailed covers the orphan
// path: a QUEUED instance whose job was deleted while it sat in the queue
// must be marked terminally DELIVERY_FAILED by the next dispatcher tick
// (mirrors the Rust tick — "ScheduledJob no longer exists"), not left
// QUEUED forever. No FK/CASCADE exists by design: instances are firing
// history and outlive their job.
func TestDispatcherTick_OrphanInstance_MarkedDeliveryFailed(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	jobs := scheduledjob.NewRepository(pool)
	instances := scheduledjob.NewInstanceRepository(pool)
	uow := testpg.NewUoW(t)
	ec := testpg.TestEC()

	// Create job → FireNow (MANUAL instance, QUEUED) → delete the job, all
	// through the public operations — the same path production uses.
	created, err := usecaseop.Run(testpg.AnchorCtx(), uow, operations.CreateScheduledJob(jobs), operations.CreateCommand{
		Code:  "sjdsp-orphan",
		Name:  "SJ Dispatcher Orphan",
		Crons: []string{"0 0 * * * *"},
	}, ec)
	require.NoError(t, err)
	jobID := created.ScheduledJobID

	fired, err := usecaseop.Run(testpg.AnchorCtx(), uow, operations.FireNow(jobs, instances),
		operations.FireNowCommand{ID: jobID}, ec)
	require.NoError(t, err)
	instanceID := fired.InstanceID

	inst, err := instances.FindByID(ctx, instanceID)
	require.NoError(t, err)
	require.NotNil(t, inst)
	require.Equal(t, scheduledjob.InstanceStatusQueued, inst.Status, "FireNow must enqueue the instance")

	_, err = usecaseop.Run(testpg.AnchorCtx(), uow, operations.DeleteScheduledJob(jobs),
		operations.DeleteCommand{ID: jobID}, ec)
	require.NoError(t, err)
	gone, err := jobs.FindByID(ctx, jobID)
	require.NoError(t, err)
	require.Nil(t, gone, "job row must be gone — the instance is now an orphan")

	// One dispatcher tick. The dispatcher is constructed directly (same shape
	// NewService wires); tick is the unit run claims QUEUED instances with.
	d := &dispatcher{
		cfg:       Config{DispatchInterval: time.Second, DispatchBatchSize: 32, HTTPTimeout: time.Second},
		jobs:      jobs,
		instances: instances,
		http:      &http.Client{Timeout: time.Second},
		isLeader:  func() bool { return true },
	}
	require.NoError(t, d.tick(ctx))

	got, err := instances.FindByID(ctx, instanceID)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, scheduledjob.InstanceStatusDeliveryFailed, got.Status,
		"orphan instance must be terminally DELIVERY_FAILED, not stuck QUEUED")
	require.NotNil(t, got.DeliveryError)
	assert.Equal(t, "ScheduledJob no longer exists", *got.DeliveryError)

	// Terminal means terminal: a second tick must not resurrect or re-touch it.
	require.NoError(t, d.tick(ctx))
	again, err := instances.FindByID(ctx, instanceID)
	require.NoError(t, err)
	require.NotNil(t, again)
	assert.Equal(t, scheduledjob.InstanceStatusDeliveryFailed, again.Status)
}

// TestDispatcherTick_Accepts2xxNotJust202 covers the fix for a real reported
// bug: a consumer endpoint that returns a plain 200 OK (not 202) was having
// every firing marked DELIVERY_FAILED, even though the delivery itself
// succeeded. The dispatcher must accept any 2xx, not hard-require 202.
func TestDispatcherTick_Accepts2xxNotJust202(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	jobs := scheduledjob.NewRepository(pool)
	instances := scheduledjob.NewInstanceRepository(pool)
	uow := testpg.NewUoW(t)
	ec := testpg.TestEC()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK) // 200, not 202 — this is the case under test
		_, _ = w.Write([]byte(`{"attempted":0,"released":0,"skipped":0,"failed":0,"failures":[]}`))
	}))
	defer server.Close()
	targetURL := server.URL

	created, err := usecaseop.Run(testpg.AnchorCtx(), uow, operations.CreateScheduledJob(jobs), operations.CreateCommand{
		Code:      "sjdsp-200ok",
		Name:      "SJ Dispatcher 200 OK",
		Crons:     []string{"0 0 * * * *"},
		TargetURL: &targetURL,
	}, ec)
	require.NoError(t, err)
	jobID := created.ScheduledJobID

	fired, err := usecaseop.Run(testpg.AnchorCtx(), uow, operations.FireNow(jobs, instances),
		operations.FireNowCommand{ID: jobID}, ec)
	require.NoError(t, err)
	instanceID := fired.InstanceID

	d := &dispatcher{
		cfg:       Config{DispatchInterval: time.Second, DispatchBatchSize: 32, HTTPTimeout: time.Second},
		jobs:      jobs,
		instances: instances,
		http:      &http.Client{Timeout: time.Second},
		isLeader:  func() bool { return true },
	}
	require.NoError(t, d.tick(ctx))

	got, err := instances.FindByID(ctx, instanceID)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, scheduledjob.InstanceStatusDelivered, got.Status,
		"a plain 200 OK from the target must be accepted as delivered, not treated as a failure")
	assert.Nil(t, got.DeliveryError)
}

// TestDispatcherTick_SignsFiringWithApplicationSecret pins the delivery
// signature contract: HMAC-SHA256 over `timestamp + rawBody`, hex-encoded, in
// X-FlowCatalyst-Signature with the millisecond-ISO8601 timestamp in
// X-FlowCatalyst-Timestamp — byte-compatible with the SDK's WebhookValidator
// and the router's webhook signing. Jobs without an application (or whose
// application resolves no secret) deliver unsigned.
func TestDispatcherTick_SignsFiringWithApplicationSecret(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	jobs := scheduledjob.NewRepository(pool)
	instances := scheduledjob.NewInstanceRepository(pool)
	uow := testpg.NewUoW(t)
	ec := testpg.TestEC()

	const secret = "sjdsp-signing-secret-1"

	type capture struct {
		sig, ts, bearer string
		body            []byte
	}
	got := make(chan capture, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		got <- capture{
			sig:    r.Header.Get("X-FlowCatalyst-Signature"),
			ts:     r.Header.Get("X-FlowCatalyst-Timestamp"),
			bearer: r.Header.Get("Authorization"),
			body:   body,
		}
		w.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()
	targetURL := server.URL

	appID := "app_sjdspsign0001"
	created, err := usecaseop.Run(testpg.AnchorCtx(), uow, operations.CreateScheduledJob(jobs), operations.CreateCommand{
		Code:          "sjdsp-signed",
		Name:          "SJ Dispatcher Signed",
		Crons:         []string{"0 0 * * * *"},
		TargetURL:     &targetURL,
		ApplicationID: &appID,
	}, ec)
	require.NoError(t, err)

	_, err = usecaseop.Run(testpg.AnchorCtx(), uow, operations.FireNow(jobs, instances),
		operations.FireNowCommand{ID: created.ScheduledJobID}, ec)
	require.NoError(t, err)

	d := &dispatcher{
		cfg:       Config{DispatchInterval: time.Second, DispatchBatchSize: 32, HTTPTimeout: time.Second},
		jobs:      jobs,
		instances: instances,
		http:      &http.Client{Timeout: time.Second},
		isLeader:  func() bool { return true },
		creds: func(_ context.Context, gotAppID string) (serviceaccount.OutboundCreds, error) {
			require.Equal(t, appID, gotAppID, "resolver must receive the job's application id")
			return serviceaccount.OutboundCreds{BearerToken: "fc_testbearer", SigningSecret: secret}, nil
		},
	}
	require.NoError(t, d.tick(ctx))

	c := <-got
	assert.Equal(t, "Bearer fc_testbearer", c.bearer,
		"delivery must carry the SA bearer token, same as router webhooks")
	require.NotEmpty(t, c.sig, "delivery must carry X-FlowCatalyst-Signature")
	require.NotEmpty(t, c.ts, "delivery must carry X-FlowCatalyst-Timestamp")
	_, terr := time.Parse("2006-01-02T15:04:05.000Z", c.ts)
	require.NoError(t, terr, "timestamp must be millisecond-precision ISO8601 UTC")

	// Recompute exactly as the SDK's WebhookValidator does:
	// hash_hmac('sha256', timestamp . rawBody, secret), hex.
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(c.ts))
	mac.Write(c.body)
	assert.Equal(t, hex.EncodeToString(mac.Sum(nil)), c.sig,
		"signature must verify against timestamp+body with the application secret")
}
