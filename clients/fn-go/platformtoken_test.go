//go:build !wasip1

package fn

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func tokenHost(t *testing.T, sent *[]string) *FakeHost {
	return &FakeHost{Handler: func(op int32, meta, body []byte) ([]byte, []byte, *Error, error) {
		if op != int32(opPlatformToken) {
			t.Fatalf("op = %d, want platform.token (15)", op)
		}
		if len(body) != 0 {
			t.Fatalf("body = %q, want none", body)
		}
		*sent = append(*sent, string(meta))
		rm, _ := json.Marshal(platformTokenResult{AccessToken: "fake-token-1", ExpiresAt: 1793736000000})
		return rm, nil, nil, nil
	}}
}

func TestPlatformTokenReturnsTheToken(t *testing.T) {
	var sent []string
	defer SetHost(tokenHost(t, &sent))()
	tok, err := PlatformToken(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if tok != "fake-token-1" {
		t.Fatalf("token = %q", tok)
	}
	if sent[0] != `{}` {
		t.Fatalf("meta = %s, want {}", sent[0])
	}
}

func TestPlatformTokenRefreshSendsTheFlag(t *testing.T) {
	var sent []string
	defer SetHost(tokenHost(t, &sent))()
	if _, err := PlatformToken(context.Background(), Refresh()); err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(sent[0]), &m); err != nil || m["refresh"] != true {
		t.Fatalf("meta = %s", sent[0])
	}
}

func TestPlatformTokenErrorClasses(t *testing.T) {
	for code, retryable := range map[string]bool{
		ErrCodeUnavailable: true, ErrCodeNotDeclared: false, ErrCodeNotAllowed: false, ErrCodeDeadline: false,
	} {
		fake := &FakeHost{Handler: func(int32, []byte, []byte) ([]byte, []byte, *Error, error) {
			return nil, nil, &Error{Code: code, Message: "PLATFORM_TOKEN_NOT_DECLARED"}, nil
		}}
		restore := SetHost(fake)
		_, err := PlatformToken(context.Background())
		restore()
		var fe *Error
		if !errors.As(err, &fe) || fe.Code != code || fe.Message != "PLATFORM_TOKEN_NOT_DECLARED" {
			t.Fatalf("%s: err = %v", code, err)
		}
		if errors.Is(err, ErrRetryable) != retryable {
			t.Errorf("%s: retryable = %v, want %v", code, !retryable, retryable)
		}
	}
}

func TestPlatformTokenUnknownOpIsAClearError(t *testing.T) {
	fake := &FakeHost{Handler: func(int32, []byte, []byte) ([]byte, []byte, *Error, error) {
		return nil, nil, &Error{Code: "UNKNOWN_OP", Message: "op 15"}, nil
	}}
	defer SetHost(fake)()
	_, err := PlatformToken(context.Background())
	var fe *Error
	if !errors.As(err, &fe) || fe.Code != ErrCodeUnknownOp {
		t.Fatalf("err = %v", err)
	}
	if !strings.Contains(err.Error(), "runner does not support platform tokens") {
		t.Fatalf("message = %q", err.Error())
	}
	if errors.Is(err, ErrRetryable) {
		t.Error("UNKNOWN_OP must not be retryable")
	}
}

func TestDescribePlatformToken(t *testing.T) {
	resetRegistry()
	Open("GET /healthz", noopHandler)
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(describeJSON(), &doc); err != nil {
		t.Fatal(err)
	}
	if _, ok := doc["platformToken"]; ok {
		t.Error("platformToken present without the declaration")
	}

	UsesPlatformToken()
	doc = nil
	if err := json.Unmarshal(describeJSON(), &doc); err != nil {
		t.Fatal(err)
	}
	if string(doc["platformToken"]) != "true" {
		t.Errorf("platformToken = %s, want true", doc["platformToken"])
	}
}
