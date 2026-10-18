package main

import (
	"encoding/json"
	"strings"
	"testing"

	fn "github.com/flowcatalyst/flowcatalyst/clients/fn-go"
)

func TestDescribeIsOneSignedWebhookAndNothingElse(t *testing.T) {
	var d map[string]json.RawMessage
	if err := json.Unmarshal(fn.DescribeForTest(), &d); err != nil {
		t.Fatal(err)
	}
	if string(d["endpoints"]) != `[{"method":"POST","path":"/deliver","auth":"webhook"}]` {
		t.Fatalf("endpoints = %s", d["endpoints"])
	}
	// No subscription: jobs are dispatched to it directly by dispatch-creator.
	for _, k := range []string{"subscriptions", "schedules", "emits", "dispatches"} {
		if _, ok := d[k]; ok {
			t.Errorf("describe has %q", k)
		}
	}
}

func TestADeliveryIsLoggedAndAcked(t *testing.T) {
	h := &fn.FakeHost{}
	defer fn.SetHost(h)()
	r := fn.InvokeForTest(fn.TestRequest{Method: "POST", Path: "/deliver", Webhook: true, Body: []byte(`{"order":42}`)})
	if r.Status != 200 || string(r.Body) != `{"ack":true}` {
		t.Fatalf("%d %s", r.Status, r.Body)
	}
	logged := false
	for _, c := range h.Calls {
		logged = logged || (c.Op == 1 && strings.Contains(string(c.Meta), "payload received") && strings.Contains(string(c.Meta), "order"))
	}
	if !logged {
		t.Fatalf("no log call: %+v", h.Calls)
	}
}
