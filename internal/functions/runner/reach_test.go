package runner

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/flowcatalyst/flowcatalyst-go/internal/functions/abi"
	"github.com/flowcatalyst/flowcatalyst-go/internal/functions/control"
)

// reachVer is describeDoc with /meta turned into a platform endpoint that
// declares reach "any" (the fixture echoes the request, caller included).
func reachVer(n int, roles ...string) control.Version {
	v := ver(n, roles...)
	v.Describe = json.RawMessage(strings.Replace(describeDoc,
		`{"path":"/meta","auth":"none"}`, `{"path":"/meta","auth":"platform","reach":"any"}`, 1))
	return v
}

func authHdr(tok string) map[string]string {
	return map[string]string{"Authorization": "Bearer " + tok}
}

// A platform endpoint with default reach answers a caller holding the
// function's client and an anchor caller, and refuses everyone else (403)
// without invoking the guest.
func TestPlatformEndpointReach(t *testing.T) {
	h := start(t, fnDoc(ver(1, control.RoleLive)))

	// The fixture has no /secure path: its own 404 is a guest answer.
	for _, tok := range []string{"plain", "anchor"} {
		resp, b := h.do("GET", "/fn/app.hello/secure", nil, authHdr(tok))
		if resp.StatusCode != 404 || string(b) != "no such fixture path" {
			t.Fatalf("%s: %d %s, want the guest's answer", tok, resp.StatusCode, b)
		}
	}

	resp, b := h.do("GET", "/fn/app.hello/secure", nil, authHdr("other"))
	if resp.StatusCode != http.StatusForbidden || !strings.Contains(string(b), `"FORBIDDEN"`) ||
		!strings.Contains(string(b), "the caller cannot reach this function") {
		t.Fatalf("other client: %d %s", resp.StatusCode, b)
	}
	if got := outcomeCount(h, "forbidden"); got != 1 {
		t.Fatalf("forbidden outcome counted %v times, want 1", got)
	}
}

// The reach check runs before the body is read and before the guest is
// acquired.
func TestPlatformReachRefusesBeforeReadingBody(t *testing.T) {
	h := start(t, fnDoc(ver(1, control.RoleLive)))
	fr := &failReader{t: t}
	req := httptest.NewRequest("POST", "/fn/app.hello/secure-post", fr)
	req.Header.Set("Authorization", "Bearer other")
	rec := httptest.NewRecorder()
	h.r.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden || fr.read {
		t.Fatalf("status %d, body read %v", rec.Code, fr.read)
	}
}

// A function with no client is reached by anchor callers only.
func TestPlatformReachFunctionWithoutClient(t *testing.T) {
	fn := fnDoc(ver(1, control.RoleLive))
	fn.ClientID = nil
	h := start(t, fn)
	if resp, b := h.do("GET", "/fn/app.hello/secure", nil, authHdr("plain")); resp.StatusCode != http.StatusForbidden {
		t.Fatalf("client caller of a clientless function: %d %s", resp.StatusCode, b)
	}
	if resp, b := h.do("GET", "/fn/app.hello/secure", nil, authHdr("anchor")); resp.StatusCode != 404 || string(b) != "no such fixture path" {
		t.Fatalf("anchor caller: %d %s", resp.StatusCode, b)
	}
}

// reach "any" opens the endpoint to a caller of another client, and the
// function receives that caller as before.
func TestPlatformReachAny(t *testing.T) {
	h := start(t, fnDoc(reachVer(1, control.RoleLive)))
	resp, b := h.do("GET", "/fn/app.hello/meta", nil, authHdr("other"))
	if resp.StatusCode != 200 {
		t.Fatalf("status %d %s", resp.StatusCode, b)
	}
	var m abi.Request
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	if m.Caller.Kind != abi.CallerPrincipal || m.Caller.ID != "prn_3" || len(m.Caller.Clients) != 1 || m.Caller.Clients[0] != "clt_2" {
		t.Fatalf("caller = %+v", m.Caller)
	}
	// A bad token is still refused.
	if resp, _ := h.do("GET", "/fn/app.hello/meta", nil, authHdr("forged")); resp.StatusCode != 401 {
		t.Fatalf("forged token on a reach any endpoint: %d", resp.StatusCode)
	}
}

// An explicit-version call needs reach whatever the endpoint declares.
func TestExplicitVersionStillNeedsReachOnReachAny(t *testing.T) {
	h := start(t, fnDoc(reachVer(1, control.RoleLive), reachVer(2)))
	h.waitVersionReady("app.hello", 2)
	if resp, b := h.do("GET", "/fn/app.hello@v2/meta", nil, authHdr("other-invoke")); resp.StatusCode != http.StatusForbidden {
		t.Fatalf("explicit version by a caller that cannot reach: %d %s", resp.StatusCode, b)
	}
	if resp, b := h.do("GET", "/fn/app.hello@v2/meta", nil, authHdr("good")); resp.StatusCode != 200 {
		t.Fatalf("explicit version by a caller that reaches: %d %s", resp.StatusCode, b)
	}
}
