package abi

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestFrameRoundTrip(t *testing.T) {
	f := EncodeFrame([]byte(`{"a":1}`), []byte{0, 1, 2, 0xff})
	if got := binary.LittleEndian.Uint32(f); got != 7 {
		t.Fatalf("meta length prefix = %d, want 7 (little-endian)", got)
	}
	meta, body, err := DecodeFrame(f)
	if err != nil {
		t.Fatal(err)
	}
	if string(meta) != `{"a":1}` || !bytes.Equal(body, []byte{0, 1, 2, 0xff}) {
		t.Fatalf("meta %q body %v", meta, body)
	}
}

func TestFrameEmptyBody(t *testing.T) {
	meta, body, err := DecodeFrame(EncodeFrame([]byte(`{}`), nil))
	if err != nil || string(meta) != `{}` || len(body) != 0 {
		t.Fatalf("meta %q body %v err %v", meta, body, err)
	}
}

func TestFrameMalformed(t *testing.T) {
	for name, b := range map[string][]byte{
		"short":         {1, 0},
		"meta too long": {9, 0, 0, 0, '{', '}'},
	} {
		if _, _, err := DecodeFrame(b); !errors.Is(err, ErrMalformedFrame) {
			t.Errorf("%s: err = %v, want ErrMalformedFrame", name, err)
		}
	}
	if _, err := UnmarshalFrame(EncodeFrame([]byte(`not json`), nil), &Response{}); !errors.Is(err, ErrMalformedFrame) {
		t.Errorf("bad json: err = %v", err)
	}
}

func TestPack(t *testing.T) {
	hi, lo := Unpack(Pack(0xdeadbeef, 42))
	if hi != 0xdeadbeef || lo != 42 {
		t.Fatalf("unpack = %x %d", hi, lo)
	}
}

const validDescribe = `{
  "abi": 1,
  "endpoints": [
    {"method": "POST", "path": "/events/order-created", "auth": "webhook"},
    {"method": "GET", "path": "/api/orders/{id}", "auth": "platform", "cors": {"origins": ["https://app.acme.com"]}},
    {"path": "/files/{rest...}", "auth": "none"},
    {"path": "/healthz", "auth": "none", "maxBodyBytes": 0}
  ],
  "subscriptions": [{"eventType": "orders:order:order:created", "path": "/events/order-created", "mode": "IMMEDIATE", "maxRetries": 3}],
  "schedules": [{"cron": "0 */5 * * * *", "path": "/events/order-created"}],
  "config": ["GREETING"], "secrets": ["STRIPE_KEY"], "db": ["main"],
  "httpAllow": ["api.stripe.com", "*.acme.com", "hooks.acme.com:8080"], "emits": ["orders:order:order:shipped"],
  "dispatches": [{"target": "orders.invoice-logger", "path": "/deliver", "connection": "yard-conn"}]
}`

func TestParseDescribeValid(t *testing.T) {
	d, err := ParseDescribe([]byte(validDescribe))
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Dispatches) != 1 || d.Dispatches[0] != (DispatchTarget{Target: "orders.invoice-logger", Path: "/deliver", Connection: "yard-conn"}) {
		t.Fatalf("dispatches = %+v", d.Dispatches)
	}
	if len(d.Endpoints) != 4 || d.Endpoints[3].MaxBodyBytes == nil || *d.Endpoints[3].MaxBodyBytes != 0 {
		t.Fatalf("endpoints = %+v", d.Endpoints)
	}
}

func TestParseDescribeRejects(t *testing.T) {
	cases := map[string]struct {
		doc  string
		want string
	}{
		"unknown key":          {`{"abi":1,"endpoints":[{"path":"/x","auth":"none"}],"filter":"x"}`, "unknown field"},
		"wrong abi":            {`{"abi":2,"endpoints":[{"path":"/x","auth":"none"}]}`, "abi: 2"},
		"no endpoints":         {`{"abi":1,"endpoints":[]}`, "at least one endpoint"},
		"missing auth":         {`{"abi":1,"endpoints":[{"path":"/x"}]}`, "auth is required"},
		"bad auth":             {`{"abi":1,"endpoints":[{"path":"/x","auth":"open"}]}`, `auth "open"`},
		"webhook GET":          {`{"abi":1,"endpoints":[{"method":"GET","path":"/x","auth":"webhook"}]}`, "POST only"},
		"relative path":        {`{"abi":1,"endpoints":[{"path":"x","auth":"none"}]}`, "must start with /"},
		"conflicting patterns": {`{"abi":1,"endpoints":[{"path":"/a/{x}","auth":"none"},{"path":"/a/{y}","auth":"none"}]}`, "conflicts"},
		"sub to non-literal":   {`{"abi":1,"endpoints":[{"path":"/e/{x}","auth":"webhook"}],"subscriptions":[{"eventType":"a:b:c:d","path":"/e/{x}"}]}`, "literal path"},
		"sub to platform endpoint": {
			`{"abi":1,"endpoints":[{"path":"/e","auth":"platform"}],"subscriptions":[{"eventType":"a:b:c:d","path":"/e"}]}`,
			"not webhook",
		},
		"sub to nowhere":                            {`{"abi":1,"endpoints":[{"path":"/e","auth":"webhook"}],"subscriptions":[{"eventType":"a:b:c:d","path":"/f"}]}`, "matches no endpoint"},
		"sub bad event type":                        {`{"abi":1,"endpoints":[{"path":"/e","auth":"webhook"}],"subscriptions":[{"eventType":"a:b","path":"/e"}]}`, "app:domain:aggregate:event"},
		"sub bad mode":                              {`{"abi":1,"endpoints":[{"path":"/e","auth":"webhook"}],"subscriptions":[{"eventType":"a:b:c:d","path":"/e","mode":"FAST"}]}`, `mode "FAST"`},
		"schedule no cron":                          {`{"abi":1,"endpoints":[{"path":"/e","auth":"webhook"}],"schedules":[{"cron":" ","path":"/e"}]}`, "cron is required"},
		"duplicate secret":                          {`{"abi":1,"endpoints":[{"path":"/e","auth":"none"}],"secrets":["K","K"]}`, "declared twice"},
		"bad config key":                            {`{"abi":1,"endpoints":[{"path":"/e","auth":"none"}],"config":["1BAD"]}`, `config: "1BAD"`},
		"bad http host":                             {`{"abi":1,"endpoints":[{"path":"/e","auth":"none"}],"httpAllow":["https://x.com"]}`, "httpAllow"},
		"http IP literal":                           {`{"abi":1,"endpoints":[{"path":"/e","auth":"none"}],"httpAllow":["169.254.169.254"]}`, "IP address"},
		"http IP with port":                         {`{"abi":1,"endpoints":[{"path":"/e","auth":"none"}],"httpAllow":["127.0.0.1:8080"]}`, "IP address"},
		"http numeric tld":                          {`{"abi":1,"endpoints":[{"path":"/e","auth":"none"}],"httpAllow":["10.1"]}`, "IP address"},
		"http localhost":                            {`{"abi":1,"endpoints":[{"path":"/e","auth":"none"}],"httpAllow":["localhost:8080"]}`, "loopback"},
		"http sub localhost":                        {`{"abi":1,"endpoints":[{"path":"/e","auth":"none"}],"httpAllow":["*.foo.localhost"]}`, "loopback"},
		"http wildcard tld":                         {`{"abi":1,"endpoints":[{"path":"/e","auth":"none"}],"httpAllow":["*.com"]}`, "too broad"},
		"http wildcard stars":                       {`{"abi":1,"endpoints":[{"path":"/e","auth":"none"}],"httpAllow":["*.*"]}`, "httpAllow"},
		"negative body limit":                       {`{"abi":1,"endpoints":[{"path":"/e","auth":"none","maxBodyBytes":-1}]}`, "maxBodyBytes"},
		"cors wildcard with credentials":            {`{"abi":1,"endpoints":[{"path":"/e","auth":"none","cors":{"origins":["*"],"allowCredentials":true}}]}`, "allowCredentials"},
		"cors listed with wildcard and credentials": {`{"abi":1,"endpoints":[{"path":"/e","auth":"none","cors":{"origins":["https://a.com","*"],"allowCredentials":true}}]}`, "allowCredentials"},
		"dispatch bad target":                       {`{"abi":1,"endpoints":[{"path":"/e","auth":"none"}],"dispatches":[{"target":"https://x.com","path":"/d"}]}`, "not a function address"},
		"dispatch no dot":                           {`{"abi":1,"endpoints":[{"path":"/e","auth":"none"}],"dispatches":[{"target":"logger","path":"/d"}]}`, "not a function address"},
		"dispatch relative path":                    {`{"abi":1,"endpoints":[{"path":"/e","auth":"none"}],"dispatches":[{"target":"a.b","path":"d"}]}`, "must start with /"},
		"dispatch pattern path":                     {`{"abi":1,"endpoints":[{"path":"/e","auth":"none"}],"dispatches":[{"target":"a.b","path":"/d/{x}"}]}`, "literal path"},
		"dispatch query path":                       {`{"abi":1,"endpoints":[{"path":"/e","auth":"none"}],"dispatches":[{"target":"a.b","path":"/d?x=1"}]}`, "literal path"},
		"dispatch dot segment":                      {`{"abi":1,"endpoints":[{"path":"/e","auth":"none"}],"dispatches":[{"target":"a.b","path":"/x/../d"}]}`, "dot segments"},
		"dispatch duplicate":                        {`{"abi":1,"endpoints":[{"path":"/e","auth":"none"}],"dispatches":[{"target":"a.b","path":"/d"},{"target":"a.b","path":"/d"}]}`, "declared twice"},
		"dispatch unknown key":                      {`{"abi":1,"endpoints":[{"path":"/e","auth":"none"}],"dispatches":[{"target":"a.b","path":"/d","url":"x"}]}`, "unknown field"},
		"dispatch missing path":                     {`{"abi":1,"endpoints":[{"path":"/e","auth":"none"}],"dispatches":[{"target":"a.b"}]}`, "must start with /"},
		"dispatch bad pool":                         {`{"abi":1,"endpoints":[{"path":"/e","auth":"none"}],"dispatches":[{"target":"a.b","path":"/d","pool":"Yard Pool"}]}`, "not a dispatch pool code"},
		"dispatch pool upper":                       {`{"abi":1,"endpoints":[{"path":"/e","auth":"none"}],"dispatches":[{"target":"a.b","path":"/d","pool":"Yard"}]}`, "not a dispatch pool code"},
		"dispatch pool digit first":                 {`{"abi":1,"endpoints":[{"path":"/e","auth":"none"}],"dispatches":[{"target":"a.b","path":"/d","pool":"1pool"}]}`, "not a dispatch pool code"},
		"dispatch no connection":                    {`{"abi":1,"endpoints":[{"path":"/e","auth":"none"}],"dispatches":[{"target":"a.b","path":"/d"}]}`, "dispatches[0]: a.b/d declares no connection"},
		"dispatch pool but no connection":           {`{"abi":1,"endpoints":[{"path":"/e","auth":"none"}],"dispatches":[{"target":"a.b","path":"/d","pool":"p1"}]}`, "declares no connection"},
		"dispatch second entry no connection":       {`{"abi":1,"endpoints":[{"path":"/e","auth":"none"}],"dispatches":[{"target":"a.b","path":"/d","connection":"c"},{"target":"a.b","path":"/e"}]}`, "dispatches[1]: a.b/e declares no connection"},
		"dispatch bad connection":                   {`{"abi":1,"endpoints":[{"path":"/e","auth":"none"}],"dispatches":[{"target":"a.b","path":"/d","connection":"my_conn"}]}`, "not a connection code"},
		"dispatch same target and path, two pools":  {`{"abi":1,"endpoints":[{"path":"/e","auth":"none"}],"dispatches":[{"target":"a.b","path":"/d","pool":"p1"},{"target":"a.b","path":"/d","pool":"p2"}]}`, "declared twice"},
		"trailing data":                             {`{"abi":1,"endpoints":[{"path":"/e","auth":"none"}]} {}`, "trailing data"},
	}
	for name, c := range cases {
		_, err := ParseDescribe([]byte(c.doc))
		if _, ok := errors.AsType[*DescribeError](err); !ok {
			t.Errorf("%s: err = %v, want *DescribeError", name, err)
			continue
		}
		if !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: err = %v, want it to mention %q", name, err, c.want)
		}
	}
}

// A wildcard origin without credentials, and credentials with listed origins,
// stay valid.
func TestParseDescribeCORSValid(t *testing.T) {
	for _, doc := range []string{
		`{"abi":1,"endpoints":[{"path":"/e","auth":"none","cors":{"origins":["*"]}}]}`,
		`{"abi":1,"endpoints":[{"path":"/e","auth":"none","cors":{"origins":["https://a.com"],"allowCredentials":true}}]}`,
	} {
		if _, err := ParseDescribe([]byte(doc)); err != nil {
			t.Errorf("%s: %v", doc, err)
		}
	}
}

func TestRouterMatch(t *testing.T) {
	d, err := ParseDescribe([]byte(validDescribe))
	if err != nil {
		t.Fatal(err)
	}
	r, err := NewRouter(d.Endpoints)
	if err != nil {
		t.Fatal(err)
	}
	m, err := r.Match("GET", "/api/orders/42")
	if err != nil || m.Index != 1 || m.PathParams["id"] != "42" {
		t.Fatalf("match = %+v, %v", m, err)
	}
	m, err = r.Match("DELETE", "/files/a/b/c.txt")
	if err != nil || m.Index != 2 || m.PathParams["rest"] != "a/b/c.txt" {
		t.Fatalf("rest match = %+v, %v", m, err)
	}
	if _, err := r.Match("POST", "/api/orders/42"); !errors.Is(err, ErrMethodNotAllowed) {
		t.Fatalf("wrong method: err = %v", err)
	}
	if _, err := r.Match("GET", "/nope"); !errors.Is(err, ErrNoRoute) {
		t.Fatalf("no route: err = %v", err)
	}
	// GET patterns also match HEAD, as in net/http.
	if m, err := r.Match("HEAD", "/api/orders/7"); err != nil || m.Index != 1 {
		t.Fatalf("HEAD = %+v, %v", m, err)
	}
}

func TestParseDescribeDispatchPoolAndConnection(t *testing.T) {
	d, err := ParseDescribe([]byte(`{"abi":1,"endpoints":[{"path":"/e","auth":"none"}],"dispatches":[` +
		`{"target":"a.b","path":"/d","pool":"yard-process-pool","connection":"yard-conn"},` +
		`{"target":"a.b","path":"/e","connection":"yard-conn"},{"target":"a.c","path":"/d","pool":"tower_pool","connection":"c2"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	got, ok := d.DispatchFor("a.b", "/d")
	if !ok || got.Pool != "yard-process-pool" || got.Connection != "yard-conn" {
		t.Fatalf("DispatchFor = %+v, %v", got, ok)
	}
	if got, _ := d.DispatchFor("a.b", "/e"); got.Pool != "" || got.Connection != "yard-conn" {
		t.Fatalf("an undeclared pool must stay empty, the connection must be kept: %+v", got)
	}
	if _, ok := d.DispatchFor("a.b", "/nope"); ok {
		t.Fatal("DispatchFor matched an undeclared path")
	}
}

// A describe document that declares a pool or connection, handed to a
// platform that predates them, must fail at publish (unknown key) rather than
// be accepted and dispatch to the default pool. oldDescribe stands for that
// platform: today's strict decode with the previous DispatchTarget shape.
func TestOldPlatformRefusesPoolAndConnectionDeclarations(t *testing.T) {
	type oldTarget struct {
		Target string `json:"target"`
		Path   string `json:"path"`
	}
	type oldDescribe struct {
		ABI        int         `json:"abi"`
		Endpoints  []Endpoint  `json:"endpoints"`
		Dispatches []oldTarget `json:"dispatches"`
	}
	for _, key := range []string{"pool", "connection"} {
		extra := ""
		if key == "pool" {
			extra = `,"connection":"c"` // required by the current platform
		}
		doc := `{"abi":1,"endpoints":[{"path":"/e","auth":"none"}],"dispatches":[{"target":"a.b","path":"/d","` + key + `":"x"` + extra + `}]}`
		dec := json.NewDecoder(strings.NewReader(doc))
		dec.DisallowUnknownFields()
		var old oldDescribe
		if err := dec.Decode(&old); err == nil || !strings.Contains(err.Error(), "unknown field") {
			t.Errorf("%s: an old strict decoder must refuse the key, got %v", key, err)
		}
		if _, err := ParseDescribe([]byte(doc)); err != nil {
			t.Errorf("%s: the current platform must accept the declaration: %v", key, err)
		}
	}
}
