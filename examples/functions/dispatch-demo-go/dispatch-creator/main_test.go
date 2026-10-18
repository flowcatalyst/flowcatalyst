package main

import (
	"encoding/json"
	"strings"
	"testing"

	fn "github.com/flowcatalyst/flowcatalyst/clients/fn-go"
)

func allowed() *fn.Principal {
	return &fn.Principal{ID: "prn_1", Type: "SERVICE", Permissions: []string{requiredPermission}}
}

func post(p *fn.Principal, body string) fn.TestResponse {
	return fn.InvokeForTest(fn.TestRequest{Method: "POST", Path: "/dispatch", Principal: p, Body: []byte(body)})
}

// platform scripts the dispatch result and returns the dispatch call it saw.
func platform(t *testing.T, dispatchErr *fn.Error) func() *fn.FakeCall {
	h := &fn.FakeHost{Handler: func(op int32, _, _ []byte) ([]byte, []byte, *fn.Error, error) {
		if op == 11 {
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

func TestDescribeDeclaresTheTarget(t *testing.T) {
	var d struct {
		Endpoints  []struct{ Method, Path, Auth string }
		Emits      []string
		Dispatches []struct{ Target, Path, Connection string }
	}
	if err := json.Unmarshal(fn.DescribeForTest(), &d); err != nil {
		t.Fatal(err)
	}
	if len(d.Dispatches) != 1 || d.Dispatches[0].Target != loggerAddress || d.Dispatches[0].Path != "/deliver" || d.Dispatches[0].Connection != connectionCode {
		t.Fatalf("dispatches = %+v", d.Dispatches)
	}
	if len(d.Emits) != 0 {
		t.Fatalf("emits = %v", d.Emits)
	}
	found := false
	for _, e := range d.Endpoints {
		found = found || (e.Path == "/dispatch" && e.Auth == "platform")
	}
	if !found {
		t.Fatalf("no platform /dispatch endpoint: %+v", d.Endpoints)
	}
}

func TestNames(t *testing.T) {
	if loggerAddress != "dispatchdemo.payload-logger" || requiredPermission != "dispatchdemo:dispatch:job:create" {
		t.Fatalf("%s %s (appCode was edited: update this test)", loggerAddress, requiredPermission)
	}
}

func TestPostCreatesAJobCarryingThePayload(t *testing.T) {
	call := platform(t, nil)
	r := post(allowed(), `{"order":42}`)
	if r.Status != 202 || string(r.Body) != `{"jobId":"djob_1"}` {
		t.Fatalf("%d %s", r.Status, r.Body)
	}
	c := call()
	if c == nil || string(c.Body) != `{"order":42}` {
		t.Fatalf("dispatch call = %+v", c)
	}
	for _, want := range []string{`"target":"` + loggerAddress + `"`, `"path":"/deliver"`, `"messageGroup":"dispatch-demo"`, `"metadata":[{"key":"requestedBy","value":"prn_1"}]`} {
		if !strings.Contains(string(c.Meta), want) {
			t.Errorf("dispatch meta %s lacks %s", c.Meta, want)
		}
	}
}

func TestBadBodiesDispatchNothing(t *testing.T) {
	for body, want := range map[string]string{"not json": "INVALID_JSON", "": "INVALID_JSON", "42": "PAYLOAD_MUST_BE_A_JSON_OBJECT_OR_ARRAY"} {
		call := platform(t, nil)
		r := post(allowed(), body)
		if r.Status != 400 || !strings.Contains(string(r.Body), want) || call() != nil {
			t.Errorf("%q: %d %s", body, r.Status, r.Body)
		}
	}
}

func TestRefusedIs502AndPlatformTroubleIsRetryable503(t *testing.T) {
	platform(t, &fn.Error{Code: "NOT_ALLOWED", Message: "no"})
	if r := post(allowed(), "{}"); r.Status != 502 {
		t.Errorf("refused: %d", r.Status)
	}
	platform(t, &fn.Error{Code: "UNAVAILABLE", Message: "down"})
	if r := post(allowed(), "{}"); r.Status != 503 {
		t.Errorf("unavailable: %d", r.Status)
	}
}

func TestWithoutThePermissionNothingIsDispatched(t *testing.T) {
	call := platform(t, nil)
	other := &fn.Principal{ID: "prn_2", Permissions: []string{"other:x:y:z"}}
	if r := post(other, "{}"); r.Status != 403 {
		t.Errorf("wrong permission: %d", r.Status)
	}
	if r := post(nil, "{}"); r.Status != 403 {
		t.Errorf("anonymous: %d", r.Status)
	}
	if call() != nil {
		t.Error("a dispatch was made")
	}
}
