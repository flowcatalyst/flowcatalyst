package scheduledjobs_test

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/go-sdk/client"
	"github.com/flowcatalyst/go-sdk/lock"
	"github.com/flowcatalyst/go-sdk/scheduledjobs"
	"github.com/flowcatalyst/go-sdk/webhook"
)

// countingLock counts Acquire calls so a test can prove a refused firing
// never reached the lock.
type countingLock struct {
	acquires atomic.Int32
	inner    lock.Provider
}

func (c *countingLock) Acquire(ctx context.Context, key string, ttl time.Duration) (lock.Handle, error) {
	c.acquires.Add(1)
	return c.inner.Acquire(ctx, key, ttl)
}

func sign(secret, ts string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(ts))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

// captureLogs swaps the default slog logger for one writing to a buffer.
func captureLogs(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(prev) })
	return &buf
}

const (
	explicitSecret = "explicit-secret"
	envSecret      = "env-secret"
)

type secretMode string

const (
	modeExplicit secretMode = "explicit"
	modeEnv      secretMode = "env"
	modeNone     secretMode = "none"
)

func (m secretMode) value() string {
	switch m {
	case modeExplicit:
		return explicitSecret
	case modeEnv:
		return envSecret
	case modeNone:
		return ""
	}
	return ""
}

func buildRunner(t *testing.T, mode secretMode, require bool) (*scheduledjobs.Runner, *atomic.Bool, *countingLock) {
	t.Helper()
	srv, _ := newPlatform(t)
	if mode == modeEnv {
		t.Setenv(webhook.SigningSecretEnvVar, envSecret)
	} else {
		t.Setenv(webhook.SigningSecretEnvVar, "")
	}
	var ran atomic.Bool
	cl := &countingLock{inner: lock.NewNoOp()}
	b := scheduledjobs.NewBuilder(client.New(srv.URL), cl).
		Handler("single", func(context.Context, *scheduledjobs.HandlerContext) (json.RawMessage, error) {
			ran.Store(true)
			return nil, nil
		})
	if mode == modeExplicit {
		b.SigningSecret(explicitSecret)
	}
	if require {
		b.RequireSignature()
	}
	return b.Build(), &ran, cl
}

// TestSigningMatrix walks {explicit, env, none} x RequireSignature x
// {valid, tampered body, bad signature, missing headers, stale timestamp}
// through ProcessSigned, and Process, asserting the Result, that the handler
// runs only on acceptance, and that a refusal never takes the lock.
func TestSigningMatrix(t *testing.T) {
	body := envelopeJSON(t, false)
	fresh := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	stale := time.Now().UTC().Add(-time.Hour).Format("2006-01-02T15:04:05.000Z")

	type scenario struct {
		name      string
		body      []byte
		sig, ts   func(secret string) string
		validOnly bool // accepted when a secret is resolved
	}
	scenarios := []scenario{
		{"valid", body, func(s string) string { return sign(s, fresh, body) }, func(string) string { return fresh }, true},
		{"tampered body", append([]byte(" "), body...), func(s string) string { return sign(s, fresh, body) }, func(string) string { return fresh }, false},
		{"bad signature", body, func(string) string { return strings.Repeat("0", 64) }, func(string) string { return fresh }, false},
		{"missing headers", body, func(string) string { return "" }, func(string) string { return "" }, false},
		{"stale timestamp", body, func(s string) string { return sign(s, stale, body) }, func(string) string { return stale }, false},
	}

	for _, mode := range []secretMode{modeExplicit, modeEnv, modeNone} {
		for _, req := range []bool{false, true} {
			for _, sc := range scenarios {
				name := string(mode) + "/require=" + map[bool]string{false: "no", true: "yes"}[req] + "/" + sc.name
				t.Run("ProcessSigned/"+name, func(t *testing.T) {
					captureLogs(t)
					r, ran, cl := buildRunner(t, mode, req)
					res := r.ProcessSigned(sc.body, sc.sig(mode.value()), sc.ts(mode.value()))
					r.Wait()

					var want scheduledjobs.ResultKind
					switch {
					case mode != modeNone:
						want = scheduledjobs.ResultUnauthorized
						if sc.validOnly {
							want = scheduledjobs.ResultAccepted
						}
					case req:
						want = scheduledjobs.ResultUnauthorized
					default:
						want = scheduledjobs.ResultAccepted // no secret: unchanged, headers ignored
					}
					assert.Equal(t, want, res.Kind, res.Message)
					if want == scheduledjobs.ResultUnauthorized {
						assert.False(t, ran.Load(), "handler must not run on refusal")
						assert.Zero(t, cl.acquires.Load(), "lock must not be taken on refusal")
					} else {
						assert.Equal(t, int32(1), cl.acquires.Load(), "accepted firing reaches the lock")
						assert.True(t, ran.Load(), "accepted firing runs the handler")
					}
				})
			}
		}
		for _, req := range []bool{false, true} {
			name := string(mode) + "/require=" + map[bool]string{false: "no", true: "yes"}[req]
			t.Run("Process/"+name, func(t *testing.T) {
				captureLogs(t)
				r, ran, cl := buildRunner(t, mode, req)
				res := r.Process(body)
				r.Wait()
				want := scheduledjobs.ResultAccepted
				// Process cannot verify: it refuses for an explicit secret
				// (historical) and under RequireSignature; an env-only
				// secret does not make it refuse.
				if mode == modeExplicit || req {
					want = scheduledjobs.ResultUnauthorized
				}
				assert.Equal(t, want, res.Kind, res.Message)
				if want == scheduledjobs.ResultUnauthorized {
					assert.False(t, ran.Load(), "handler must not run on refusal")
					assert.Zero(t, cl.acquires.Load(), "lock must not be taken on refusal")
				}
			})
		}
	}
}

func TestUnverifiedWarningLoggedOncePerRunner(t *testing.T) {
	logs := captureLogs(t)
	r, _, _ := buildRunner(t, modeNone, false)
	body := envelopeJSON(t, false)

	for range 3 {
		require.Equal(t, scheduledjobs.ResultAccepted, r.ProcessSigned(body, "", "").Kind)
	}
	for range 2 {
		require.Equal(t, scheduledjobs.ResultAccepted, r.Process(body).Kind)
	}
	r.Wait()
	assert.Equal(t, 1, strings.Count(logs.String(), "WITHOUT verifying"), logs.String())
	assert.Contains(t, logs.String(), webhook.SigningSecretEnvVar)

	// A second Runner warns on its own.
	r2, _, _ := buildRunner(t, modeNone, false)
	r2.Process(body)
	r2.Wait()
	assert.Equal(t, 2, strings.Count(logs.String(), "WITHOUT verifying"))
}

func TestNoWarningWhenVerifying(t *testing.T) {
	logs := captureLogs(t)
	r, _, _ := buildRunner(t, modeEnv, false)
	body := envelopeJSON(t, false)
	ts := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	require.Equal(t, scheduledjobs.ResultAccepted, r.ProcessSigned(body, sign(envSecret, ts, body), ts).Kind)
	r.Wait()
	assert.NotContains(t, logs.String(), "WITHOUT verifying")
}

// An explicit secret wins over the environment one.
func TestExplicitSecretBeatsEnv(t *testing.T) {
	captureLogs(t)
	r, _, _ := buildRunner(t, modeExplicit, false)
	t.Setenv(webhook.SigningSecretEnvVar, envSecret)
	body := envelopeJSON(t, false)
	ts := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	assert.Equal(t, scheduledjobs.ResultUnauthorized, r.ProcessSigned(body, sign(envSecret, ts, body), ts).Kind)
	assert.Equal(t, scheduledjobs.ResultAccepted, r.ProcessSigned(body, sign(explicitSecret, ts, body), ts).Kind)
	r.Wait()
}
