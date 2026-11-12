package runner

import (
	"bytes"
	"encoding/base64"
	"maps"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/functions/abi"
	"github.com/flowcatalyst/flowcatalyst-go/internal/functions/control"
)

// maskFixture is a traced invocation of a version that declares the secrets
// `declared`, with `resolved` as the values the runner holds for them.
func maskFixture(declared []string, resolved map[string]string) (*invTrace, *tracer) {
	tr := newTestTracer(&fakeTraceSink{}, 1000, 65536)
	fn := &function{address: "app.hello"}
	fn.settings.Store(&control.Function{Secrets: resolved})
	ver := &version{number: 1, describe: &abi.Describe{Secrets: declared}}
	return &invTrace{fn: fn, ver: ver, t: tr, address: "app.hello", version: 1, invID: "i", live: []string{"http"}, httpLive: true}, tr
}

func captureOne(inv *invTrace, reqBody, respBody []byte, hdr, respHdr http.Header, query string) control.TraceRecord {
	req := httptest.NewRequest("POST", "/fn/app.hello/x?"+query, bytes.NewReader(reqBody))
	if hdr != nil {
		req.Header = hdr
	}
	cw := &captureWriter{ResponseWriter: httptest.NewRecorder(), limit: 65536}
	maps.Copy(cw.Header(), respHdr)
	_, _ = cw.Write(respBody)
	inv.captureHTTP(req, "/x", reqBody, nil, cw, "2xx", time.Now())
	recs := inv.t.take()
	return recs[0]
}

func TestTraceMasksDeclaredSecretValues(t *testing.T) {
	const secret = "s3cr3t-value-XYZ"
	inv, tr := maskFixture([]string{"API"}, map[string]string{"API": secret, "UNDECLARED": "undeclared-value"})

	rec := captureOne(inv,
		[]byte(`{"k":"`+secret+`","other":"`+"undeclared-value"+`"}`),
		[]byte("echo: "+secret),
		nil, http.Header{"X-Reply": {secret}}, "a="+secret)
	if strings.Contains(string(rec.Data), secret) || strings.Contains(rec.Message, secret) {
		t.Fatalf("secret in http record: %s", rec.Data)
	}
	if !strings.Contains(string(rec.Data), "undeclared-value") {
		t.Errorf("an undeclared secret must not be masked: %s", rec.Data)
	}
	d := httpData(t, rec)
	if !strings.Contains(d.Request.RawQuery, redacted) || d.Response.Headers["x-reply"][0] != redacted {
		t.Errorf("placeholder missing: %+v", d)
	}

	// A request header (not credential-named) carrying the value.
	rec = captureOne(inv, nil, nil, http.Header{"X-Plain": {"before " + secret + " after"}}, nil, "")
	if strings.Contains(string(rec.Data), secret) {
		t.Fatalf("secret in request header: %s", rec.Data)
	}
	if v := httpData(t, rec).Request.Headers["x-plain"]; len(v) != 1 || v[0] != "before "+redacted+" after" {
		t.Errorf("x-plain = %v", v)
	}

	// A binary body is base64 on the record: the value must go before encoding.
	bin := append([]byte{0xff, 0xfe}, []byte(secret)...)
	rec = captureOne(inv, bin, nil, nil, nil, "")
	d = httpData(t, rec)
	raw, err := base64.StdEncoding.DecodeString(*d.Request.Body)
	if err != nil || bytes.Contains(raw, []byte(secret)) || d.Request.BodyTruncated {
		t.Errorf("binary body: err=%v truncated=%v %q", err, d.Request.BodyTruncated, raw)
	}

	// Log lines: log op attrs and message, and guest stdout.
	inv.logRecord("info", "token is "+secret, map[string]any{"v": secret})
	inv.guestOutput("warn", []byte("line with "+secret+"\n"))
	inv.emit("http", control.TraceKindTrace, "info", "msg "+secret, "ref-"+secret, []byte(`{"d":"`+secret+`"}`))
	for _, r := range tr.take() {
		if strings.Contains(string(r.Data), secret) || strings.Contains(r.Message, secret) || strings.Contains(r.Reference, secret) {
			t.Errorf("secret in %s record: %+v %s", r.Kind, r, r.Data)
		}
	}
}

func TestTraceMaskIgnoresShortAndUnresolvedSecrets(t *testing.T) {
	inv, _ := maskFixture([]string{"SHORT", "MISSING"}, map[string]string{"SHORT": "abc"})
	if v := inv.maskValues(); v != nil {
		t.Fatalf("mask values = %v", v)
	}
	rec := captureOne(inv, []byte("abc abc"), nil, nil, nil, "")
	if d := httpData(t, rec); *d.Request.Body != "abc abc" {
		t.Errorf("a 3-character value must not be masked: %q", *d.Request.Body)
	}
	inv, _ = maskFixture([]string{"S"}, map[string]string{"S": "abcd"})
	rec = captureOne(inv, []byte("xabcdx"), nil, nil, nil, "")
	if d := httpData(t, rec); *d.Request.Body != "x"+redacted+"x" {
		t.Errorf("a 4-character value is masked: %q", *d.Request.Body)
	}
}

// With nothing to mask the records are exactly what they were: same bytes as
// an invocation that has no function at all.
func TestTraceNoDeclaredSecretsRecordsUnchanged(t *testing.T) {
	plain, _ := maskFixture(nil, nil)
	declaredButUnset, _ := maskFixture([]string{"A"}, map[string]string{})
	bare := &invTrace{t: plain.t, address: "app.hello", version: 1, invID: "i", live: []string{"http"}, httpLive: true}
	body := []byte(`{"a":"b","n":1.50}`)
	hdr := func() http.Header { return http.Header{"X-A": {"v"}, "Authorization": {"x"}} }
	var out [][]byte
	for _, inv := range []*invTrace{plain, declaredButUnset, bare} {
		rec := captureOne(inv, body, body, hdr(), nil, "q=1&token=t")
		out = append(out, rec.Data)
		inv.emit("http", control.TraceKindTrace, "info", "m", "", []byte(`{"n":1.50,"b":"x"}`))
		rec2 := inv.t.take()[0]
		out = append(out, rec2.Data)
	}
	for k := 2; k < len(out); k++ {
		// Even slots are http records (their duration differs run to run),
		// odd slots are author trace records.
		got, want := string(out[k]), string(out[k%2])
		if k%2 == 0 {
			got, want = strip(got), strip(want)
		}
		if got != want {
			t.Errorf("record %d differs:\n%s\n%s", k, got, want)
		}
	}
}

// strip drops the one run-dependent field.
func strip(s string) string {
	i := strings.Index(s, `"durationMs":`)
	if i < 0 {
		return s
	}
	j := strings.IndexAny(s[i:], ",}")
	return s[:i] + s[i+j:]
}
