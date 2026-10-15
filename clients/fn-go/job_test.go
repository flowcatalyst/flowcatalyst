//go:build !wasip1

package fn

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

func TestDispatchSendsTheJobAndReturnsTheID(t *testing.T) {
	retries := 0
	fake := &FakeHost{Handler: func(op int32, meta, body []byte) ([]byte, []byte, *Error, error) {
		if op != int32(opDispatch) {
			t.Fatalf("op = %d, want dispatch (11)", op)
		}
		var m map[string]any
		if err := json.Unmarshal(meta, &m); err != nil {
			t.Fatal(err)
		}
		if m["target"] != "orders.logger" || m["path"] != "/deliver" || m["subject"] != "s" || m["messageGroup"] != "g" {
			t.Fatalf("meta = %s", meta)
		}
		if v, ok := m["maxRetries"]; !ok || v != float64(0) {
			t.Fatalf("an explicit zero maxRetries must be sent: %s", meta)
		}
		if m["dataOnly"] != false {
			t.Fatalf("an explicit dataOnly=false must be sent: %s", meta)
		}
		md, _ := m["metadata"].([]any)
		if len(md) != 2 || md[0].(map[string]any)["key"] != "a" || md[1].(map[string]any)["key"] != "b" {
			t.Fatalf("metadata must be a key-sorted array of {key,value}: %s", meta)
		}
		if string(body) != `{"order":42}` {
			t.Fatalf("body = %q", body)
		}
		rm, _ := json.Marshal(dispatchResult{JobID: "djob_1"})
		return rm, nil, nil, nil
	}}
	defer SetHost(fake)()

	no := false
	id, err := Dispatch(context.Background(), Job{
		Target: "orders.logger", Path: "/deliver", Payload: []byte(`{"order":42}`),
		Subject: "s", MessageGroup: "g", MaxRetries: &retries, DataOnly: &no,
		Metadata: map[string]string{"b": "2", "a": "1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if id != "djob_1" {
		t.Fatalf("id = %q", id)
	}
}

func TestDispatchOmitsUnsetFields(t *testing.T) {
	fake := &FakeHost{Handler: func(op int32, meta, body []byte) ([]byte, []byte, *Error, error) {
		var m map[string]any
		_ = json.Unmarshal(meta, &m)
		if len(m) != 2 {
			t.Fatalf("only target and path expected, got %s", meta)
		}
		return nil, nil, nil, nil
	}}
	defer SetHost(fake)()
	if _, err := Dispatch(context.Background(), Job{Target: "a.b", Path: "/d"}); err != nil {
		t.Fatal(err)
	}
}

func TestDispatchMetadataEntriesKeepOrder(t *testing.T) {
	j := Job{MetadataEntries: []MetadataEntry{{"z", "1"}, {"a", "2"}}, Metadata: map[string]string{"ignored": "x"}}
	got := j.metadataEntries()
	if len(got) != 2 || got[0].Key != "z" || got[1].Key != "a" {
		t.Fatalf("entries = %+v", got)
	}
}

func TestDispatchErrorClasses(t *testing.T) {
	for code, retryable := range map[string]bool{
		ErrCodeUnavailable: true, ErrCodeNotDeclared: false, ErrCodeNotAllowed: false,
		ErrCodeBadRequest: false, ErrCodeTooLarge: false,
	} {
		fake := &FakeHost{Handler: func(int32, []byte, []byte) ([]byte, []byte, *Error, error) {
			return nil, nil, &Error{Code: code, Message: "x"}, nil
		}}
		restore := SetHost(fake)
		_, err := Dispatch(context.Background(), Job{Target: "a.b", Path: "/d"})
		restore()
		var fe *Error
		if !errors.As(err, &fe) || fe.Code != code {
			t.Fatalf("%s: err = %v", code, err)
		}
		if errors.Is(err, ErrRetryable) != retryable {
			t.Errorf("%s: retryable = %v, want %v", code, !retryable, retryable)
		}
	}
}
