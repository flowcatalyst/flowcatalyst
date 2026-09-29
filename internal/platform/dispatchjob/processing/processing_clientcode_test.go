package processing

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchjob"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/serviceaccount"
)

// Tests in this file pin docs/spec/webhook-client-code.md's ruling (R1/R2):
// the non-dataOnly envelope's clientCode field, and the X-FlowCatalyst-Client
// header on every delivery whose client resolves. They exercise Handler.deliver
// directly against an httptest subscriber — deliver never touches h.repo, so
// no database is needed; New(nil, nil) is safe here.

// alwaysResolve returns a ClientCodeResolver that resolves every non-empty
// clientID to the given code, and counts how many times it was invoked (used
// to prove a platform-scoped job never even queries the resolver).
func alwaysResolve(code string, calls *atomic.Int32) ClientCodeResolver {
	return func(_ context.Context, clientID string) (string, bool) {
		calls.Add(1)
		if clientID == "" {
			return "", false
		}
		return code, true
	}
}

func neverResolve() ClientCodeResolver {
	return func(context.Context, string) (string, bool) { return "", false }
}

// captured holds what the fake subscriber observed about one delivery.
type captured struct {
	body         []byte
	clientHeader string
	hasHeader    bool
	sig          string
	tstamp       string
}

func fakeSubscriber(t *testing.T, out *atomic.Value) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		_, hasHeader := r.Header[http.CanonicalHeaderKey(clientHeader)]
		out.Store(captured{
			body:         b,
			clientHeader: r.Header.Get(clientHeader),
			hasHeader:    hasHeader,
			sig:          r.Header.Get(signatureHeader),
			tstamp:       r.Header.Get(timestampHeader),
		})
		w.WriteHeader(http.StatusOK)
	}))
}

// T1 — a client-scoped job's envelope carries clientCode = the client's
// identifier, alongside clientId. Mutant: drop the field.
func TestBuildPayload_ClientCode_Present(t *testing.T) {
	job := &dispatchjob.DispatchJob{ID: "dsj_1", Code: "app:evt", ClientID: new("clt_1")}

	var env map[string]any
	require.NoError(t, json.Unmarshal(buildPayload(job, "acme"), &env))

	assert.Equal(t, "clt_1", env["clientId"])
	assert.Equal(t, "acme", env["clientCode"])
}

// T2 (envelope half) — a platform-scoped job (no clientId) carries neither
// key in the envelope, even when a resolved code is available. Mutant: emit
// the header/field with an empty half.
func TestBuildPayload_ClientCode_NoClientID(t *testing.T) {
	job := &dispatchjob.DispatchJob{ID: "dsj_2", Code: "app:evt"} // ClientID nil

	var env map[string]any
	require.NoError(t, json.Unmarshal(buildPayload(job, "acme"), &env))

	_, hasID := env["clientId"]
	_, hasCode := env["clientCode"]
	assert.False(t, hasID, "platform-scoped job must not carry clientId")
	assert.False(t, hasCode, "platform-scoped job must not carry clientCode")
}

// T2/T3/T5 (header decision) — table-tests the pure header-composition rule:
// never a half pair.
func TestClientHeaderValue(t *testing.T) {
	tests := []struct {
		name       string
		clientID   *string
		clientCode string
		wantOK     bool
		wantValue  string
	}{
		{"no clientId at all", nil, "acme", false, ""},
		{"empty clientId", new(""), "acme", false, ""},
		{"resolved client: exact pair", new("clt_1"), "acme", true, "clt_1:acme"},
		{"unresolved client: empty code omits the header", new("clt_1"), "", false, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			job := &dispatchjob.DispatchJob{ClientID: tc.clientID}
			v, ok := clientHeaderValue(job, tc.clientCode)
			assert.Equal(t, tc.wantOK, ok)
			assert.Equal(t, tc.wantValue, v)
		})
	}
}

// T2 (header half, end to end) — a platform-scoped job never carries the
// header, and the resolver is never even queried (nothing to resolve).
// Mutant: emit the header with an empty half.
func TestDeliver_PlatformScopedJob_NoHeaderNoResolverCall(t *testing.T) {
	var out atomic.Value
	sub := fakeSubscriber(t, &out)
	t.Cleanup(sub.Close)

	var calls atomic.Int32
	h := New(nil, nil).WithClientCodeResolver(alwaysResolve("acme", &calls))

	job := &dispatchjob.DispatchJob{ID: "dsj_3", Code: "app:evt", TargetURL: sub.URL, Payload: new(`{"a":1}`)}
	res := h.deliver(context.Background(), job)
	require.True(t, res.success)

	c := out.Load().(captured)
	assert.False(t, c.hasHeader, "platform-scoped delivery must not carry X-FlowCatalyst-Client")
	assert.Equal(t, int32(0), calls.Load(), "no clientId means nothing to resolve")

	var env map[string]any
	require.NoError(t, json.Unmarshal(c.body, &env))
	_, hasCode := env["clientCode"]
	assert.False(t, hasCode)
}

// T3 — the header is exactly "{clientId}:{clientCode}", observed on the
// actual wire request. Mutant: send the code alone.
func TestDeliver_ClientHeader_ExactFormat(t *testing.T) {
	var out atomic.Value
	sub := fakeSubscriber(t, &out)
	t.Cleanup(sub.Close)

	var calls atomic.Int32
	h := New(nil, nil).WithClientCodeResolver(alwaysResolve("acme-corp", &calls))

	job := &dispatchjob.DispatchJob{ID: "dsj_4", Code: "app:evt", TargetURL: sub.URL, ClientID: new("clt_42")}
	res := h.deliver(context.Background(), job)
	require.True(t, res.success)

	c := out.Load().(captured)
	require.True(t, c.hasHeader)
	assert.Equal(t, "clt_42:acme-corp", c.clientHeader, "must be exactly {clientId}:{clientCode}, not the code alone")
}

// T4 — a dataOnly delivery: the body is the raw payload byte-for-byte
// (unchanged), and the header is present. Mutant: skip the header in
// dataOnly mode.
func TestDeliver_DataOnly_BodyUnchangedHeaderPresent(t *testing.T) {
	var out atomic.Value
	sub := fakeSubscriber(t, &out)
	t.Cleanup(sub.Close)

	var calls atomic.Int32
	h := New(nil, nil).WithClientCodeResolver(alwaysResolve("acme", &calls))

	rawPayload := `{"raw":true,"n":7}`
	job := &dispatchjob.DispatchJob{
		ID: "dsj_5", Code: "app:evt", TargetURL: sub.URL,
		DataOnly: true, ClientID: new("clt_1"), Payload: new(rawPayload),
	}
	res := h.deliver(context.Background(), job)
	require.True(t, res.success)

	c := out.Load().(captured)
	assert.Equal(t, rawPayload, string(c.body), "dataOnly body must be the raw payload byte-for-byte")
	require.True(t, c.hasHeader, "dataOnly delivery must still carry X-FlowCatalyst-Client")
	assert.Equal(t, "clt_1:acme", c.clientHeader)
}

// T5 — an unresolvable client: delivery still happens, no header, no
// clientCode. Mutant: fail or block the delivery.
func TestDeliver_UnresolvableClient_StillDelivers(t *testing.T) {
	var out atomic.Value
	sub := fakeSubscriber(t, &out)
	t.Cleanup(sub.Close)

	h := New(nil, nil).WithClientCodeResolver(neverResolve())

	job := &dispatchjob.DispatchJob{ID: "dsj_6", Code: "app:evt", TargetURL: sub.URL, ClientID: new("clt_ghost")}
	res := h.deliver(context.Background(), job)

	require.True(t, res.success, "an unresolvable client must not fail or block the delivery")

	c := out.Load().(captured)
	assert.False(t, c.hasHeader, "unresolved client must omit the header")

	var env map[string]any
	require.NoError(t, json.Unmarshal(c.body, &env))
	assert.Equal(t, "clt_ghost", env["clientId"], "clientId is unaffected by resolution failure")
	_, hasCode := env["clientCode"]
	assert.False(t, hasCode, "unresolved client must omit clientCode")
}

// T7 — X-FlowCatalyst-Signature still verifies over the body alone with the
// client header present. Mutant: fold the header into the signed material
// (e.g. sign timestamp+body+header) — recomputing over timestamp+body alone
// would then no longer match.
func TestDeliver_SignatureCoversBodyOnly_WithClientHeaderPresent(t *testing.T) {
	var out atomic.Value
	sub := fakeSubscriber(t, &out)
	t.Cleanup(sub.Close)

	const secret = "sig-secret-1"
	var calls atomic.Int32
	h := New(nil, nil).
		WithClientCodeResolver(alwaysResolve("acme", &calls)).
		WithDeliveryCredsResolver(func(context.Context, *dispatchjob.DispatchJob) (serviceaccount.OutboundCreds, error) {
			return serviceaccount.OutboundCreds{SigningSecret: secret}, nil
		})

	job := &dispatchjob.DispatchJob{ID: "dsj_7", Code: "app:evt", TargetURL: sub.URL, ClientID: new("clt_9")}
	res := h.deliver(context.Background(), job)
	require.True(t, res.success)

	c := out.Load().(captured)
	require.True(t, c.hasHeader, "sanity: the header this test is about must actually be present")
	assert.Equal(t, "clt_9:acme", c.clientHeader)
	require.NotEmpty(t, c.sig)

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(c.tstamp))
	mac.Write(c.body)
	assert.Equal(t, hex.EncodeToString(mac.Sum(nil)), c.sig,
		"signature must verify over timestamp+body alone; the client header must not be folded in")

	_, terr := time.Parse("2006-01-02T15:04:05.000Z", c.tstamp)
	require.NoError(t, terr)
}
