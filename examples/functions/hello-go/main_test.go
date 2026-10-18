package main

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	fn "github.com/flowcatalyst/flowcatalyst/clients/fn-go"
)

// host scripts the platform side: config values, and the results of emit and dispatch.
func host(t *testing.T, cfg map[string]string, dispatchErr *fn.Error) *fn.FakeHost {
	t.Helper()
	h := &fn.FakeHost{Handler: func(op int32, meta, body []byte) ([]byte, []byte, *fn.Error, error) {
		switch op {
		case 2: // config.get
			var m struct{ Key string }
			_ = json.Unmarshal(meta, &m)
			if v, ok := cfg[m.Key]; ok {
				return []byte(`{"found":true}`), []byte(v), nil, nil
			}
			return []byte(`{"found":false}`), nil, nil, nil
		case 5: // event.emit
			return []byte(`{"eventId":"evt_1"}`), nil, nil, nil
		case 11: // dispatch.create
			if dispatchErr != nil {
				return nil, nil, dispatchErr, nil
			}
			return []byte(`{"jobId":"djob_1"}`), nil, nil, nil
		}
		return nil, nil, nil, nil
	}}
	t.Cleanup(fn.SetHost(h))
	return h
}

func TestDescribe(t *testing.T) {
	var d struct {
		Endpoints  []struct{ Method, Path, Auth string }
		Emits      []string
		Dispatches []struct{ Target, Path, Pool, Connection string }
	}
	if err := json.Unmarshal(fn.DescribeForTest(), &d); err != nil {
		t.Fatal(err)
	}
	if len(d.Endpoints) != 4 || len(d.Emits) != 1 || len(d.Dispatches) != 1 {
		t.Fatalf("describe = %+v", d)
	}
	if g := d.Dispatches[0]; g.Target != "hello.logger" || g.Pool != "hello-pool" || g.Connection != "hello-conn" {
		t.Fatalf("dispatch declaration = %+v", g)
	}
}

func TestHealthz(t *testing.T) {
	host(t, nil, nil)
	r := fn.InvokeForTest(fn.TestRequest{Path: "/healthz"})
	if r.Status != 200 || string(r.Body) != `{"status":"ok"}` {
		t.Fatalf("%d %s", r.Status, r.Body)
	}
}

func TestHelloUsesConfigAndPrincipal(t *testing.T) {
	host(t, map[string]string{"GREETING": "Kia ora"}, nil)
	r := fn.InvokeForTest(fn.TestRequest{Path: "/hello/Aroha", Principal: &fn.Principal{ID: "prn_1"}})
	var got map[string]string
	_ = json.Unmarshal(r.Body, &got)
	if r.Status != 200 || got["message"] != "Kia ora, Aroha" || got["caller"] != "prn_1" {
		t.Fatalf("%d %s", r.Status, r.Body)
	}
}

func TestHelloDefaultsWithoutConfig(t *testing.T) {
	host(t, nil, nil)
	r := fn.InvokeForTest(fn.TestRequest{Path: "/hello/Aroha", Principal: &fn.Principal{ID: "prn_1"}})
	if !strings.Contains(string(r.Body), "Hello, Aroha") {
		t.Fatalf("%d %s", r.Status, r.Body)
	}
}

func TestWebhookEmitsOnceKeyedByEventID(t *testing.T) {
	h := host(t, nil, nil)
	r := fn.InvokeForTest(fn.TestRequest{Method: "POST", Path: "/events/greeting", Webhook: true, Body: []byte(`{"id":"evt_9"}`)})
	if r.Status != 202 {
		t.Fatalf("%d %s", r.Status, r.Body)
	}
	var emit *fn.FakeCall
	for i := range h.Calls {
		if h.Calls[i].Op == 5 {
			emit = &h.Calls[i]
		}
	}
	if emit == nil || !strings.Contains(string(emit.Meta), `"dedupId":"greeting-sent-evt_9"`) {
		t.Fatalf("emit call = %+v", emit)
	}
}

func TestDispatch(t *testing.T) {
	h := host(t, nil, nil)
	r := fn.InvokeForTest(fn.TestRequest{Method: "POST", Path: "/dispatch"})
	if r.Status != http.StatusAccepted || !strings.Contains(string(r.Body), "djob_1") {
		t.Fatalf("%d %s", r.Status, r.Body)
	}
	var last fn.FakeCall
	for _, c := range h.Calls {
		if c.Op == 11 {
			last = c
		}
	}
	if last.Op != 11 || string(last.Body) != `{"hello":"world"}` || !strings.Contains(string(last.Meta), `"metadata":[{"key":"from","value":"hello"}]`) {
		t.Fatalf("dispatch call = %s | %s", last.Meta, last.Body)
	}
}

func TestDispatchRefusalAndRetry(t *testing.T) {
	for _, c := range []struct {
		code string
		want int
	}{{"NOT_ALLOWED", 422}, {"UNAVAILABLE", 429}} {
		host(t, nil, &fn.Error{Code: c.code, Message: "x"})
		if r := fn.InvokeForTest(fn.TestRequest{Method: "POST", Path: "/dispatch"}); r.Status != c.want {
			t.Errorf("%s: %d, want %d", c.code, r.Status, c.want)
		}
	}
}
