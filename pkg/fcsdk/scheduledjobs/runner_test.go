package scheduledjobs_test

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/go-sdk/client"
	"github.com/flowcatalyst/go-sdk/lock"
	"github.com/flowcatalyst/go-sdk/scheduledjobs"
)

// platformMock records inbound calls to /log and /complete so tests
// can assert on the runner's side-effects.
type platformMock struct {
	mu       sync.Mutex
	logs     []logEntry
	complete *completeEntry
}

type logEntry struct {
	InstanceID string
	Body       client.InstanceLogRequest
}

type completeEntry struct {
	InstanceID string
	Body       client.InstanceCompleteRequest
}

func newPlatform(t *testing.T) (*httptest.Server, *platformMock) {
	t.Helper()
	pm := &platformMock{}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/scheduled-jobs/instances/", func(w http.ResponseWriter, r *http.Request) {
		// path = /api/scheduled-jobs/instances/{id}/(log|complete)
		segs := splitPath(r.URL.Path)
		require.GreaterOrEqual(t, len(segs), 6, "path: %s", r.URL.Path)
		instanceID := segs[4]
		action := segs[5]
		switch action {
		case "log":
			var body client.InstanceLogRequest
			require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
			pm.mu.Lock()
			pm.logs = append(pm.logs, logEntry{InstanceID: instanceID, Body: body})
			pm.mu.Unlock()
			// The platform requires level and message (WriteInstanceLogRequest)
			// and answers 204 No Content.
			if body.Level == "" || body.Message == "" {
				w.WriteHeader(http.StatusUnprocessableEntity)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		case "complete":
			var body client.InstanceCompleteRequest
			require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
			pm.mu.Lock()
			pm.complete = &completeEntry{InstanceID: instanceID, Body: body}
			pm.mu.Unlock()
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"id":"` + instanceID + `","scheduledJobId":"sjb_1","jobCode":"x","triggerKind":"CRON","firedAt":"","status":"COMPLETED","deliveryAttempts":1,"createdAt":""}`))
		default:
			http.NotFound(w, r)
		}
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, pm
}

func splitPath(p string) []string {
	out := []string{}
	cur := ""
	for _, c := range p {
		if c == '/' {
			out = append(out, cur)
			cur = ""
		} else {
			cur += string(c)
		}
	}
	out = append(out, cur)
	return out
}

func makeEnvelope(jobCode, instanceID string, tracksCompletion bool) json.RawMessage {
	env := scheduledjobs.Envelope{
		JobID:            "sjb_1",
		JobCode:          jobCode,
		InstanceID:       instanceID,
		FiredAt:          "2026-01-01T00:00:00Z",
		TriggerKind:      scheduledjobs.TriggerCron,
		TracksCompletion: tracksCompletion,
	}
	raw, _ := json.Marshal(env)
	return raw
}

func TestProcessDispatchesHandlerAndReportsSuccess(t *testing.T) {
	srv, pm := newPlatform(t)
	api := client.New(srv.URL)

	var ran atomic.Bool
	r := scheduledjobs.NewBuilder(api, lock.NewNoOp()).
		Handler("daily", func(ctx context.Context, hctx *scheduledjobs.HandlerContext) (json.RawMessage, error) {
			ran.Store(true)
			hctx.Log(ctx, "starting", nil)
			return json.RawMessage(`{"processed":42}`), nil
		}).
		Build()

	res := r.Process(makeEnvelope("daily", "ins_1", true))
	assert.Equal(t, scheduledjobs.ResultAccepted, res.Kind)

	r.Wait()
	assert.True(t, ran.Load())
	require.NotNil(t, pm.complete)
	assert.Equal(t, "ins_1", pm.complete.InstanceID)
	assert.Equal(t, client.CompletionStatusSuccess, pm.complete.Body.Status)
	assert.JSONEq(t, `{"processed":42}`, string(pm.complete.Body.Result))
	require.Len(t, pm.logs, 1)
	assert.Equal(t, "starting", pm.logs[0].Body.Message)
	assert.Equal(t, client.LogLevelInfo, pm.logs[0].Body.Level, "level is required by the platform; INFO by default")
}

// HandlerContext.Log always sends a level (the platform requires one): INFO
// when no options or an empty level are given, the given level otherwise.
// Before the fix it omitted level, and the platform refused every line.
func TestLogSendsALevel(t *testing.T) {
	srv, pm := newPlatform(t)
	var errs atomic.Int32
	r := scheduledjobs.NewBuilder(client.New(srv.URL), lock.NewNoOp()).
		Handler("daily", func(ctx context.Context, hctx *scheduledjobs.HandlerContext) (json.RawMessage, error) {
			hctx.Log(ctx, "no options", nil)
			hctx.Log(ctx, "empty level", &scheduledjobs.LogOptions{})
			hctx.Log(ctx, "warned", &scheduledjobs.LogOptions{Level: "WARN"})
			return nil, nil
		}).
		OnError(func(error, *scheduledjobs.Envelope) { errs.Add(1) }).
		Build()

	r.Process(makeEnvelope("daily", "ins_lvl", false))
	r.Wait()
	require.Len(t, pm.logs, 3)
	assert.Equal(t, client.LogLevelInfo, pm.logs[0].Body.Level)
	assert.Equal(t, client.LogLevelInfo, pm.logs[1].Body.Level)
	assert.Equal(t, client.LogLevelWarn, pm.logs[2].Body.Level)
	assert.Zero(t, errs.Load(), "no log call was refused")
}

func TestProcessReportsHandlerFailure(t *testing.T) {
	srv, pm := newPlatform(t)
	api := client.New(srv.URL)

	r := scheduledjobs.NewBuilder(api, lock.NewNoOp()).
		Handler("broken", func(_ context.Context, _ *scheduledjobs.HandlerContext) (json.RawMessage, error) {
			return nil, assertErr("boom")
		}).
		Build()

	res := r.Process(makeEnvelope("broken", "ins_2", true))
	require.Equal(t, scheduledjobs.ResultAccepted, res.Kind)

	r.Wait()
	require.NotNil(t, pm.complete)
	assert.Equal(t, client.CompletionStatusFailure, pm.complete.Body.Status)
	assert.Contains(t, string(pm.complete.Body.Result), "boom")
}

func TestProcessUnknownJobCodeIsNotFound(t *testing.T) {
	srv, _ := newPlatform(t)
	api := client.New(srv.URL)

	r := scheduledjobs.NewBuilder(api, lock.NewNoOp()).Build()
	res := r.Process(makeEnvelope("unknown", "ins_x", true))
	assert.Equal(t, scheduledjobs.ResultNotFound, res.Kind)
	assert.Contains(t, res.Message, "unknown")
}

func TestProcessMalformedEnvelopeIsBadRequest(t *testing.T) {
	srv, _ := newPlatform(t)
	api := client.New(srv.URL)

	r := scheduledjobs.NewBuilder(api, lock.NewNoOp()).Build()
	res := r.Process(json.RawMessage(`{"jobCode":"x","triggerKind":"GARBAGE","firedAt":"","jobId":"","instanceId":""}`))
	assert.Equal(t, scheduledjobs.ResultBadRequest, res.Kind)
}

func TestLockContentionReportsLockHeldFailure(t *testing.T) {
	srv, pm := newPlatform(t)
	api := client.New(srv.URL)

	// Memory lock pre-held by an out-of-band caller so the runner finds
	// the key contended on Acquire.
	lp := lock.NewMemory()
	held, err := lp.Acquire(context.Background(), "scheduled-job:single", time.Minute)
	require.NoError(t, err)
	require.NotNil(t, held)
	t.Cleanup(func() { _ = held.Release(context.Background()) })

	var ran atomic.Bool
	r := scheduledjobs.NewBuilder(api, lp).
		Handler("single", func(_ context.Context, _ *scheduledjobs.HandlerContext) (json.RawMessage, error) {
			ran.Store(true)
			return nil, nil
		}).
		Build()

	res := r.Process(makeEnvelope("single", "ins_3", true))
	require.Equal(t, scheduledjobs.ResultAccepted, res.Kind)

	r.Wait()
	assert.False(t, ran.Load(), "handler must not run while lock is contended")
	require.NotNil(t, pm.complete)
	assert.Equal(t, client.CompletionStatusFailure, pm.complete.Body.Status)
	assert.Contains(t, string(pm.complete.Body.Result), "lock-held")
}

func TestNoCompletionCallbackWhenTracksCompletionFalse(t *testing.T) {
	srv, pm := newPlatform(t)
	api := client.New(srv.URL)

	r := scheduledjobs.NewBuilder(api, lock.NewNoOp()).
		Handler("fire-and-forget", func(_ context.Context, _ *scheduledjobs.HandlerContext) (json.RawMessage, error) {
			return nil, nil
		}).
		Build()

	res := r.Process(makeEnvelope("fire-and-forget", "ins_4", false))
	require.Equal(t, scheduledjobs.ResultAccepted, res.Kind)
	r.Wait()
	assert.Nil(t, pm.complete, "no complete callback expected when tracksCompletion=false")
}

func TestHandlerPanicSurfacedViaOnError(t *testing.T) {
	srv, pm := newPlatform(t)
	api := client.New(srv.URL)

	var captured *scheduledjobs.RunnerError
	var mu sync.Mutex
	r := scheduledjobs.NewBuilder(api, lock.NewNoOp()).
		OnError(func(err error, _ *scheduledjobs.Envelope) {
			mu.Lock()
			defer mu.Unlock()
			if re, ok := errors.AsType[*scheduledjobs.RunnerError](err); ok {
				captured = re
			}
		}).
		Handler("panicky", func(_ context.Context, _ *scheduledjobs.HandlerContext) (json.RawMessage, error) {
			panic("kaboom")
		}).
		Build()

	res := r.Process(makeEnvelope("panicky", "ins_5", true))
	require.Equal(t, scheduledjobs.ResultAccepted, res.Kind)
	r.Wait()

	mu.Lock()
	defer mu.Unlock()
	require.NotNil(t, captured, "OnError must fire on panic")
	assert.Equal(t, scheduledjobs.ErrHandlerPanicked, captured.Kind)
	require.NotNil(t, pm.complete)
	assert.Equal(t, client.CompletionStatusFailure, pm.complete.Body.Status)
	assert.Contains(t, string(pm.complete.Body.Result), "kaboom")
}

// assertErr is a tiny error builder used inside tests.
type assertErr string

func (a assertErr) Error() string { return string(a) }

func envelopeJSON(t *testing.T, concurrent bool) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(scheduledjobs.Envelope{
		JobID: "sjb_1", JobCode: "single", InstanceID: "ins_c", FiredAt: "2026-01-01T00:00:00Z",
		TriggerKind: scheduledjobs.TriggerCron, TracksCompletion: true, Concurrent: concurrent,
	})
	require.NoError(t, err)
	return raw
}

func TestConcurrentEnvelopeSkipsLock(t *testing.T) {
	srv, _ := newPlatform(t)
	lp := lock.NewMemory()
	held, err := lp.Acquire(context.Background(), "scheduled-job:single", time.Minute)
	require.NoError(t, err)
	require.NotNil(t, held)
	t.Cleanup(func() { _ = held.Release(context.Background()) })

	run := func(concurrent bool) bool {
		var ran atomic.Bool
		r := scheduledjobs.NewBuilder(client.New(srv.URL), lp).
			Handler("single", func(context.Context, *scheduledjobs.HandlerContext) (json.RawMessage, error) {
				ran.Store(true)
				return nil, nil
			}).Build()
		require.Equal(t, scheduledjobs.ResultAccepted, r.Process(envelopeJSON(t, concurrent)).Kind)
		r.Wait()
		return ran.Load()
	}
	assert.True(t, run(true), "concurrent:true must not take the (contended) lock")
	assert.False(t, run(false), "concurrent:false must still lock")

	// Explicit EnforceLock(true) overrides the envelope flag.
	var ran atomic.Bool
	r := scheduledjobs.NewBuilder(client.New(srv.URL), lp).EnforceLock(true).
		Handler("single", func(context.Context, *scheduledjobs.HandlerContext) (json.RawMessage, error) {
			ran.Store(true)
			return nil, nil
		}).Build()
	r.Process(envelopeJSON(t, true))
	r.Wait()
	assert.False(t, ran.Load())
}

func TestProcessSignedVerifiesSignature(t *testing.T) {
	srv, _ := newPlatform(t)
	const secret = "test-secret"
	body := envelopeJSON(t, true)
	ts := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(ts))
	mac.Write(body)
	sig := hex.EncodeToString(mac.Sum(nil))

	build := func(sec string) (*scheduledjobs.Runner, *atomic.Bool) {
		var ran atomic.Bool
		b := scheduledjobs.NewBuilder(client.New(srv.URL), lock.NewNoOp()).
			Handler("single", func(context.Context, *scheduledjobs.HandlerContext) (json.RawMessage, error) {
				ran.Store(true)
				return nil, nil
			})
		if sec != "" {
			b.SigningSecret(sec)
		}
		return b.Build(), &ran
	}

	r, ran := build(secret)
	assert.Equal(t, scheduledjobs.ResultAccepted, r.ProcessSigned(body, sig, ts).Kind)
	r.Wait()
	assert.True(t, ran.Load())

	for name, c := range map[string][2]string{
		"missing":   {"", ""},
		"bad sig":   {strings.Repeat("0", 64), ts},
		"no ts":     {sig, ""},
		"wrong sec": {"deadbeef", ts},
	} {
		r, ran := build(secret)
		res := r.ProcessSigned(body, c[0], c[1])
		assert.Equal(t, scheduledjobs.ResultUnauthorized, res.Kind, name)
		r.Wait()
		assert.False(t, ran.Load(), name)
	}
	// tampered body
	r, _ = build(secret)
	assert.Equal(t, scheduledjobs.ResultUnauthorized, r.ProcessSigned(append([]byte(" "), body...), sig, ts).Kind)
	// Process (no headers) fails closed when a secret is configured.
	assert.Equal(t, scheduledjobs.ResultUnauthorized, r.Process(body).Kind)

	// No secret: unchanged behaviour, headers ignored.
	r, ran = build("")
	assert.Equal(t, scheduledjobs.ResultAccepted, r.ProcessSigned(body, "", "").Kind)
	r.Wait()
	assert.True(t, ran.Load())
}
