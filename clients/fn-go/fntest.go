//go:build !wasip1

package fn

import (
	"encoding/json"
	"net/http"
	"time"
)

// TestRequest is one request for InvokeForTest. Exactly one caller kind
// applies: Principal (auth "platform"), Webhook (auth "webhook"), or neither
// (auth "none", an anonymous caller).
type TestRequest struct {
	Method string // default GET
	Path   string // default /
	Query  string // raw query, no leading "?"
	Header http.Header
	Body   []byte

	// Principal, when set, makes the caller a verified principal, exactly as
	// the runner presents one to a "platform" endpoint.
	Principal *Principal
	// Webhook marks the caller as a signature-verified webhook delivery.
	Webhook bool
}

// TestResponse is what a handler produced.
type TestResponse struct {
	Status int
	Header http.Header
	Body   []byte
}

// InvokeForTest runs req through the same path a real invocation takes: the
// request frame is built and decoded, routed by the endpoints this package's
// init() declarations registered, and the handler's response is read back from
// the response frame. Host calls (config, secret, emit, dispatch, ...) go to
// the active host, so install a FakeHost with SetHost first. It exists for
// tests on the host OS and is absent from wasip1 builds.
func InvokeForTest(req TestRequest) TestResponse {
	method := req.Method
	if method == "" {
		method = http.MethodGet
	}
	path := req.Path
	if path == "" {
		path = "/"
	}
	headers := http.Header{}
	for k, vs := range req.Header {
		for _, v := range vs {
			headers.Add(k, v) // canonical keys, as the runner sends them
		}
	}
	meta := requestMeta{
		ID: "inv_test", Address: "test.fn", Version: 1,
		Method: method, Path: path, RawQuery: req.Query,
		Headers:        map[string][]string(headers),
		Caller:         callerMeta{Kind: "anonymous"},
		DeadlineUnixMs: time.Now().Add(30 * time.Second).UnixMilli(),
	}
	switch {
	case req.Principal != nil:
		p := req.Principal
		meta.Caller = callerMeta{
			Kind: "principal", ID: p.ID, Type: p.Type, Tier: p.Tier, Clients: p.Clients, ClientCodes: p.ClientCodes, Roles: p.Roles,
			Applications: p.Applications, AllApplications: p.AllApplications, Permissions: p.Permissions,
		}
	case req.Webhook:
		meta.Caller = callerMeta{Kind: "webhook"}
	}
	mb, err := json.Marshal(meta)
	if err != nil {
		panic("fn: InvokeForTest: " + err.Error())
	}
	respFrame := handleRequestFrame(encodeFrame(mb, req.Body))
	rmb, body, err := decodeFrame(respFrame)
	if err != nil {
		panic("fn: InvokeForTest: " + err.Error())
	}
	var rm responseMeta
	if err := json.Unmarshal(rmb, &rm); err != nil {
		panic("fn: InvokeForTest: " + err.Error())
	}
	return TestResponse{Status: rm.Status, Header: http.Header(rm.Headers), Body: body}
}

// DescribeForTest returns the describe document (plan section 5.4) for the
// declarations registered so far, as publish would read it.
func DescribeForTest() []byte { return describeJSON() }
