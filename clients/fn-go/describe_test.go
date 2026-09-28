//go:build !wasip1

package fn

import (
	"encoding/json"
	"testing"
)

// TestDescribeGolden pins the exact describe JSON shape (plan §5.4) for a
// function exercising every declaration kind, including the optional
// endpoint fields and an explicit maxBodyBytes: 0 (which must be present,
// not omitted, distinguishing "declared zero" from "not declared").
func TestDescribeGolden(t *testing.T) {
	resetRegistry()

	Webhook("POST /events/order-created", noopHandler)
	Platform("GET /api/orders/{id}", noopHandler, CORS(CORSConfig{Origins: []string{"https://app.acme.com"}}))
	Open("GET /healthz", noopHandler, MaxBody(0))

	Subscribe("orders:order:order:created", "/events/order-created", Mode("IMMEDIATE"), MaxRetries(3))
	Schedule("*/5 * * * *", "/events/tick", Timezone("UTC"))

	Config("GREETING")
	Secret("STRIPE_KEY")
	DB("main")
	HTTPAllow("api.stripe.com", "*.acme.com")
	Emits("orders:order:order:shipped")

	got := describeJSON()

	want := `{
	  "abi": 1,
	  "endpoints": [
	    {"method": "POST", "path": "/events/order-created", "auth": "webhook"},
	    {"method": "GET", "path": "/api/orders/{id}", "auth": "platform",
	     "cors": {"origins": ["https://app.acme.com"]}},
	    {"method": "GET", "path": "/healthz", "auth": "none", "maxBodyBytes": 0}
	  ],
	  "subscriptions": [
	    {"eventType": "orders:order:order:created", "path": "/events/order-created",
	     "mode": "IMMEDIATE", "maxRetries": 3}
	  ],
	  "schedules": [{"cron": "*/5 * * * *", "timezone": "UTC", "path": "/events/tick"}],
	  "config": ["GREETING"], "secrets": ["STRIPE_KEY"], "db": ["main"],
	  "httpAllow": ["api.stripe.com", "*.acme.com"], "emits": ["orders:order:order:shipped"]
	}`

	assertJSONEqual(t, got, []byte(want))
}

// TestDescribeOmitsEmptyOptionalArrays checks that a function with only
// endpoints declared omits subscriptions/schedules/config/secrets/db/
// httpAllow/emits entirely, while endpoints is still present (possibly
// empty) per the task's decision: "Omit empty optional fields; arrays may
// be empty" — endpoints is the one required, always-present array.
func TestDescribeOmitsEmptyOptionalArrays(t *testing.T) {
	resetRegistry()
	Open("GET /healthz", noopHandler)

	var doc map[string]json.RawMessage
	if err := json.Unmarshal(describeJSON(), &doc); err != nil {
		t.Fatalf("unmarshal describe: %v", err)
	}
	for _, key := range []string{"subscriptions", "schedules", "config", "secrets", "db", "httpAllow", "emits"} {
		if _, ok := doc[key]; ok {
			t.Errorf("describe: field %q present but should be omitted when empty", key)
		}
	}
	if _, ok := doc["endpoints"]; !ok {
		t.Error("describe: endpoints field missing")
	}
}

func TestDescribeNoEndpointsIsEmptyArray(t *testing.T) {
	resetRegistry()
	var doc struct {
		Endpoints []json.RawMessage `json:"endpoints"`
	}
	if err := json.Unmarshal(describeJSON(), &doc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if doc.Endpoints == nil {
		t.Error("describe: endpoints should be an empty array, not null/omitted, when no endpoints are declared")
	}
}

func TestWebhookNonPostPanics(t *testing.T) {
	resetRegistry()
	defer func() {
		if recover() == nil {
			t.Error("Webhook with a non-POST method: want panic, got none")
		}
	}()
	Webhook("GET /events/x", noopHandler)
}

func TestWebhookPostOK(t *testing.T) {
	resetRegistry()
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("Webhook with POST: unexpected panic: %v", r)
		}
	}()
	Webhook("POST /events/x", noopHandler)
}

func TestEndpointPatternWithoutMethodPanics(t *testing.T) {
	resetRegistry()
	defer func() {
		if recover() == nil {
			t.Error("Open with a method-less pattern: want panic, got none")
		}
	}()
	Open("/healthz", noopHandler)
}

func TestEndpointNilHandlerPanics(t *testing.T) {
	resetRegistry()
	defer func() {
		if recover() == nil {
			t.Error("Open with a nil handler: want panic, got none")
		}
	}()
	Open("GET /healthz", nil)
}

// assertJSONEqual compares two JSON documents for structural equality
// (ignoring key order and formatting).
func assertJSONEqual(t *testing.T, got, want []byte) {
	t.Helper()
	var gv, wv any
	if err := json.Unmarshal(got, &gv); err != nil {
		t.Fatalf("got is not valid JSON: %v\n%s", err, got)
	}
	if err := json.Unmarshal(want, &wv); err != nil {
		t.Fatalf("want is not valid JSON: %v\n%s", err, want)
	}
	gb, _ := json.Marshal(gv)
	wb, _ := json.Marshal(wv)
	if string(gb) != string(wb) {
		t.Errorf("describe JSON mismatch:\n got: %s\nwant: %s", got, want)
	}
}
