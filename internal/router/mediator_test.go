package router_test

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

	"github.com/flowcatalyst/flowcatalyst-go/internal/common"
	"github.com/flowcatalyst/flowcatalyst-go/internal/router"
)

func TestMediatorPayloadAndSignatureFormat(t *testing.T) {
	// Capture what the mediator sends and verify the HMAC matches the
	// canonical formula. This is the parity test for the at-risk
	// HMAC site flagged in docs/api-parity.md.
	var (
		gotBody    []byte
		gotSig     string
		gotTs      string
		gotAuth    string
	)
	secret := "test-secret-do-not-use-in-prod"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotSig = r.Header.Get(router.SignatureHeader)
		gotTs = r.Header.Get(router.TimestampHeader)
		gotAuth = r.Header.Get("Authorization")
		gotBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	mediator := router.NewHTTPMediator(router.DevMediatorConfig())
	authToken := "abc"
	signing := secret
	msg := &common.Message{
		ID:              "msg_TEST123456",
		MediationType:   common.MediationTypeHTTP,
		MediationTarget: srv.URL,
		AuthToken:       &authToken,
		SigningSecret:   &signing,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out := mediator.Mediate(ctx, msg)
	require.Equal(t, common.MediationSuccess, out.Result, "expected success, got %+v", out)

	assert.Equal(t, `{"messageId":"msg_TEST123456"}`, string(gotBody),
		"payload must be exactly {\"messageId\":\"<id>\"} — this is the HMAC parity-critical byte sequence")
	assert.Equal(t, "Bearer abc", gotAuth)
	require.NotEmpty(t, gotTs)
	require.NotEmpty(t, gotSig)

	// Verify the HMAC manually using the same formula the mediator uses.
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(gotTs))
	mac.Write(gotBody)
	want := hex.EncodeToString(mac.Sum(nil))
	assert.Equal(t, want, gotSig, "HMAC must equal sha256(timestamp || body)")

	// Timestamp shape: millisecond precision, three fractional digits.
	// Format: 2006-01-02T15:04:05.000Z (24 chars).
	assert.Len(t, gotTs, 24)
	assert.Equal(t, "Z", string(gotTs[23]))
}

func TestMediatorBadRequestIsConfigError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer srv.Close()

	out := router.NewHTTPMediator(router.DevMediatorConfig()).Mediate(
		context.Background(),
		&common.Message{ID: "m", MediationType: common.MediationTypeHTTP, MediationTarget: srv.URL},
	)
	assert.Equal(t, common.MediationErrorConfig, out.Result)
	assert.Equal(t, 400, out.StatusCode)
}

func TestMediatorRateLimitedReadsRetryAfter(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Retry-After", "120")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	out := router.NewHTTPMediator(router.DevMediatorConfig()).Mediate(
		context.Background(),
		&common.Message{ID: "m", MediationType: common.MediationTypeHTTP, MediationTarget: srv.URL},
	)
	assert.Equal(t, common.MediationRateLimited, out.Result)
	assert.Equal(t, 120, out.DelaySeconds)
}

func TestMediatorServerErrorRetries(t *testing.T) {
	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts++
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	cfg := router.DevMediatorConfig()
	cfg.MaxRetries = 2
	cfg.RetryDelays = []time.Duration{1 * time.Millisecond, 1 * time.Millisecond}

	out := router.NewHTTPMediator(cfg).Mediate(
		context.Background(),
		&common.Message{ID: "m", MediationType: common.MediationTypeHTTP, MediationTarget: srv.URL},
	)
	assert.Equal(t, common.MediationErrorProcess, out.Result)
	assert.Equal(t, 3, attempts, "should attempt initial + 2 retries")
}

func TestMediatorAckFalseIsTransient(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ack": false, "delaySeconds": 45}`))
	}))
	defer srv.Close()

	cfg := router.DevMediatorConfig()
	cfg.MaxRetries = 0

	out := router.NewHTTPMediator(cfg).Mediate(
		context.Background(),
		&common.Message{ID: "m", MediationType: common.MediationTypeHTTP, MediationTarget: srv.URL},
	)
	assert.Equal(t, common.MediationErrorProcess, out.Result)
	assert.Equal(t, 45, out.DelaySeconds)
}
