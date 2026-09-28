//go:build !wasip1

package fn

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestConfigGetFound(t *testing.T) {
	fake := &FakeHost{Handler: func(op int32, meta, body []byte) ([]byte, []byte, *Error, error) {
		if op != int32(opConfigGet) {
			t.Fatalf("op = %d, want %d", op, opConfigGet)
		}
		var m kvGetMeta
		_ = json.Unmarshal(meta, &m)
		if m.Key != "GREETING" {
			t.Fatalf("key = %q, want GREETING", m.Key)
		}
		rm, _ := json.Marshal(kvGetResult{Found: true})
		return rm, []byte("hello"), nil, nil
	}}
	defer SetHost(fake)()

	cv := Config("GREETING")
	val, found, err := cv.Get(context.Background())
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !found || val != "hello" {
		t.Fatalf("Get() = %q, %v, want hello, true", val, found)
	}
}

func TestConfigGetNotFound(t *testing.T) {
	fake := &FakeHost{Handler: func(op int32, meta, body []byte) ([]byte, []byte, *Error, error) {
		rm, _ := json.Marshal(kvGetResult{Found: false})
		return rm, nil, nil, nil
	}}
	defer SetHost(fake)()

	_, found, err := Config("MISSING").Get(context.Background())
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if found {
		t.Fatal("found = true, want false")
	}
}

func TestSecretGetUsesSecretOp(t *testing.T) {
	fake := &FakeHost{Handler: func(op int32, meta, body []byte) ([]byte, []byte, *Error, error) {
		if op != int32(opSecretGet) {
			t.Fatalf("op = %d, want %d", op, opSecretGet)
		}
		rm, _ := json.Marshal(kvGetResult{Found: true})
		return rm, []byte("sk_live_123"), nil, nil
	}}
	defer SetHost(fake)()

	val, found, err := Secret("STRIPE_KEY").Get(context.Background())
	if err != nil || !found || val != "sk_live_123" {
		t.Fatalf("Get() = %q, %v, %v", val, found, err)
	}
}

func TestConfigGetHostError(t *testing.T) {
	fake := &FakeHost{Handler: func(op int32, meta, body []byte) ([]byte, []byte, *Error, error) {
		return nil, nil, &Error{Code: ErrCodeNotDeclared, Message: "key not declared"}, nil
	}}
	defer SetHost(fake)()

	_, _, err := Config("X").Get(context.Background())
	if err == nil {
		t.Fatal("want error")
	}
	var fe *Error
	if !errors.As(err, &fe) || fe.Code != ErrCodeNotDeclared {
		t.Fatalf("err = %v, want *Error{Code: NOT_DECLARED}", err)
	}
}

func TestEmitReturnsEventID(t *testing.T) {
	fake := &FakeHost{Handler: func(op int32, meta, body []byte) ([]byte, []byte, *Error, error) {
		if op != int32(opEventEmit) {
			t.Fatalf("op = %d, want event.emit", op)
		}
		var m eventEmitMeta
		_ = json.Unmarshal(meta, &m)
		if m.Type != "orders:order:order:shipped" {
			t.Fatalf("type = %q", m.Type)
		}
		if string(body) != `{"orderId":1}` {
			t.Fatalf("body = %q", body)
		}
		rm, _ := json.Marshal(eventEmitResult{EventID: "evt_1"})
		return rm, nil, nil, nil
	}}
	defer SetHost(fake)()

	id, err := Emit(context.Background(), Event{Type: "orders:order:order:shipped", Data: []byte(`{"orderId":1}`)})
	if err != nil {
		t.Fatalf("Emit: %v", err)
	}
	if id != "evt_1" {
		t.Fatalf("id = %q, want evt_1", id)
	}
}

func TestEmitRetryable(t *testing.T) {
	fake := &FakeHost{Handler: func(op int32, meta, body []byte) ([]byte, []byte, *Error, error) {
		return nil, nil, &Error{Code: ErrCodeUnavailable, Message: "platform busy"}, nil
	}}
	defer SetHost(fake)()

	_, err := Emit(context.Background(), Event{Type: "a:b:c:d"})
	if err == nil {
		t.Fatal("want error")
	}
	if !errors.Is(err, ErrRetryable) {
		t.Errorf("errors.Is(err, ErrRetryable) = false, want true (err=%v)", err)
	}
}

func TestEmitNotAllowedIsNotRetryable(t *testing.T) {
	fake := &FakeHost{Handler: func(op int32, meta, body []byte) ([]byte, []byte, *Error, error) {
		return nil, nil, &Error{Code: ErrCodeNotAllowed, Message: "type not owned"}, nil
	}}
	defer SetHost(fake)()

	_, err := Emit(context.Background(), Event{Type: "a:b:c:d"})
	if errors.Is(err, ErrRetryable) {
		t.Error("NOT_ALLOWED must not be classified as retryable")
	}
}

func TestLogSendsOp1(t *testing.T) {
	fake := &FakeHost{}
	defer SetHost(fake)()

	Log(context.Background()).Info("hello", "key", "value")

	if len(fake.Calls) != 1 {
		t.Fatalf("calls = %d, want 1", len(fake.Calls))
	}
	var lm logMeta
	if err := json.Unmarshal(fake.Calls[0].Meta, &lm); err != nil {
		t.Fatalf("unmarshal log meta: %v", err)
	}
	if lm.Level != "INFO" || lm.Msg != "hello" {
		t.Fatalf("logMeta = %+v", lm)
	}
	if lm.Attrs["key"] != "value" {
		t.Fatalf("attrs = %+v", lm.Attrs)
	}
}

func TestLogWithInvocationID(t *testing.T) {
	fake := &FakeHost{}
	defer SetHost(fake)()

	ctx := context.WithValue(context.Background(), ctxKeyInvocation, Info{ID: "inv_99"})
	Log(ctx).Warn("careful")

	var lm logMeta
	_ = json.Unmarshal(fake.Calls[0].Meta, &lm)
	if lm.Attrs["invocationId"] != "inv_99" {
		t.Fatalf("attrs = %+v, want invocationId=inv_99", lm.Attrs)
	}
}

func TestHTTPClientRoundTrip(t *testing.T) {
	fake := &FakeHost{Handler: func(op int32, meta, body []byte) ([]byte, []byte, *Error, error) {
		if op != int32(opHTTPFetch) {
			t.Fatalf("op = %d, want http.fetch", op)
		}
		var m httpFetchMeta
		_ = json.Unmarshal(meta, &m)
		if m.Method != "GET" || m.URL != "https://api.stripe.com/v1/charges" {
			t.Fatalf("meta = %+v", m)
		}
		rm, _ := json.Marshal(httpFetchRespMeta{Status: 200, Headers: map[string][]string{"Content-Type": {"application/json"}}})
		return rm, []byte(`{"ok":true}`), nil, nil
	}}
	defer SetHost(fake)()

	resp, err := HTTPClient().Get("https://api.stripe.com/v1/charges")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	b, _ := io.ReadAll(resp.Body)
	if string(b) != `{"ok":true}` {
		t.Fatalf("body = %q", b)
	}
	if resp.Header.Get("Content-Type") != "application/json" {
		t.Fatalf("content-type = %q", resp.Header.Get("Content-Type"))
	}
}

func TestHTTPClientHostError(t *testing.T) {
	fake := &FakeHost{Handler: func(op int32, meta, body []byte) ([]byte, []byte, *Error, error) {
		return nil, nil, &Error{Code: ErrCodeNotAllowed, Message: "host not allowlisted"}, nil
	}}
	defer SetHost(fake)()

	_, err := HTTPClient().Get("https://evil.example.com/")
	if err == nil {
		t.Fatal("want error")
	}
	if !strings.Contains(err.Error(), "NOT_ALLOWED") {
		t.Fatalf("err = %v, want NOT_ALLOWED", err)
	}
}

func TestHTTPClientSendsTimeoutFromContextDeadline(t *testing.T) {
	var gotTimeout int64
	fake := &FakeHost{Handler: func(op int32, meta, body []byte) ([]byte, []byte, *Error, error) {
		var m httpFetchMeta
		_ = json.Unmarshal(meta, &m)
		gotTimeout = m.TimeoutMs
		rm, _ := json.Marshal(httpFetchRespMeta{Status: 200})
		return rm, nil, nil, nil
	}}
	defer SetHost(fake)()

	ctx, cancel := context.WithTimeout(context.Background(), 0) // already-expired-ish deadline
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", "https://api.stripe.com/x", nil)
	_, _ = HTTPClient().Do(req)
	if gotTimeout < 0 {
		t.Fatalf("timeoutMs = %d, must not be negative", gotTimeout)
	}
}
