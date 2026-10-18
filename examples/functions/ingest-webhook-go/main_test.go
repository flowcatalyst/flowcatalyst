package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	fn "github.com/flowcatalyst/flowcatalyst/clients/fn-go"
)

const secret = "s3cr3t"

func sign(body string) string {
	m := hmac.New(sha256.New, []byte(secret))
	m.Write([]byte(body))
	return "sha256=" + hex.EncodeToString(m.Sum(nil))
}

// platform scripts the secret and the dispatch result; dispatched() is the dispatch call, if any.
func platform(t *testing.T, secretValue *string, dispatchErr *fn.Error) (dispatched func() *fn.FakeCall) {
	h := &fn.FakeHost{Handler: func(op int32, _, _ []byte) ([]byte, []byte, *fn.Error, error) {
		switch op {
		case 3:
			if secretValue == nil {
				return []byte(`{"found":false}`), nil, nil, nil
			}
			return []byte(`{"found":true}`), []byte(*secretValue), nil, nil
		case 11:
			if dispatchErr != nil {
				return nil, nil, dispatchErr, nil
			}
			return []byte(`{"jobId":"djob_1"}`), nil, nil, nil
		}
		return nil, nil, nil, nil
	}}
	t.Cleanup(fn.SetHost(h))
	return func() *fn.FakeCall {
		for i := range h.Calls {
			if h.Calls[i].Op == 11 {
				return &h.Calls[i]
			}
		}
		return nil
	}
}

func send(body string, h http.Header) fn.TestResponse {
	return fn.InvokeForTest(fn.TestRequest{Method: "POST", Path: ingestPath, Header: h, Body: []byte(body)})
}

func signed(body string) http.Header { return http.Header{"X-Signature": {sign(body)}} }

func TestDescribe(t *testing.T) {
	var d struct {
		Endpoints  []struct{ Method, Path, Auth string }
		Secrets    []string
		Dispatches []struct{ Target, Path, Pool, Connection string }
	}
	if err := json.Unmarshal(fn.DescribeForTest(), &d); err != nil {
		t.Fatal(err)
	}
	if len(d.Secrets) != 1 || d.Secrets[0] != signingSecretKey {
		t.Fatalf("secrets = %v", d.Secrets)
	}
	if g := d.Dispatches[0]; g.Target != targetAddress || g.Pool != dispatchPool || g.Connection != dispatchConnection {
		t.Fatalf("dispatches = %+v", d.Dispatches)
	}
	for _, e := range d.Endpoints {
		if e.Path == ingestPath && e.Auth != "none" {
			t.Fatalf("ingest auth = %s", e.Auth)
		}
	}
}

func TestSignedPayloadIsDispatchedAsReceived(t *testing.T) {
	s := secret
	call := platform(t, &s, nil)
	body := `{"order": 42,  "note":"keep my bytes"}`
	h := signed(body)
	h.Set("X-Message-Group", "trip-7")
	h.Set("X-Delivery-Id", "d-1")
	r := send(body, h)
	if r.Status != 202 || string(r.Body) != `{"jobId":"djob_1"}` {
		t.Fatalf("%d %s", r.Status, r.Body)
	}
	c := call()
	if c == nil || string(c.Body) != body {
		t.Fatalf("dispatch call = %+v", c)
	}
	for _, want := range []string{`"target":"` + targetAddress + `"`, `"messageGroup":"trip-7"`, `"metadata":[{"key":"deliveryId","value":"d-1"},{"key":"source","value":"webhook"}]`} {
		if !strings.Contains(string(c.Meta), want) {
			t.Errorf("meta %s lacks %s", c.Meta, want)
		}
	}
}

func TestBadSignaturesAreRefusedAndNothingIsDispatched(t *testing.T) {
	s := secret
	call := platform(t, &s, nil)
	body := `{"a":1}`
	for name, h := range map[string]http.Header{
		"missing":        {},
		"wrong prefix":   {"X-Signature": {strings.TrimPrefix(sign(body), "sha256=")}},
		"not hex":        {"X-Signature": {"sha256=zz"}},
		"other body":     signed(`{"a":2}`),
		"truncated":      {"X-Signature": {sign(body)[:20]}},
		"uppercase algo": {"X-Signature": {strings.Replace(sign(body), "sha256", "SHA256", 1)}},
	} {
		if r := send(body, h); r.Status != 401 {
			t.Errorf("%s: %d %s", name, r.Status, r.Body)
		}
	}
	if call() != nil {
		t.Fatal("a job was dispatched")
	}
}

func TestSignedButNotJSONIs400(t *testing.T) {
	s := secret
	call := platform(t, &s, nil)
	for _, body := range []string{"not json", "42", `"s"`, ""} {
		if r := send(body, signed(body)); r.Status != 400 {
			t.Errorf("%q: %d", body, r.Status)
		}
	}
	if call() != nil {
		t.Fatal("a job was dispatched")
	}
}

func TestMissingSecretIs500NotAnOpenDoor(t *testing.T) {
	call := platform(t, nil, nil)
	empty := ""
	for _, h := range []http.Header{signed("{}"), {}} {
		if r := send("{}", h); r.Status != 500 {
			t.Errorf("%d", r.Status)
		}
	}
	platform(t, &empty, nil)
	if r := send("{}", http.Header{"X-Signature": {sign("{}")}}); r.Status != 500 {
		t.Errorf("empty secret: %d", r.Status)
	}
	if call() != nil {
		t.Fatal("a job was dispatched")
	}
}

func TestRefusalIs502AndPlatformTroubleAsksForARetry(t *testing.T) {
	s := secret
	platform(t, &s, &fn.Error{Code: "NOT_ALLOWED", Message: "no"})
	if r := send("{}", signed("{}")); r.Status != 502 {
		t.Errorf("refused: %d", r.Status)
	}
	platform(t, &s, &fn.Error{Code: "UNAVAILABLE", Message: "down"})
	if r := send("{}", signed("{}")); r.Status != 429 || r.Header.Get("Retry-After") != "5" {
		t.Errorf("unavailable: %d %v", r.Status, r.Header)
	}
}

func TestOverlongMessageGroupIs400(t *testing.T) {
	s := secret
	platform(t, &s, nil)
	h := signed("{}")
	h.Set("X-Message-Group", strings.Repeat("g", 129))
	if r := send("{}", h); r.Status != 400 {
		t.Errorf("%d", r.Status)
	}
}
