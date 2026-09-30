package runner

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/flowcatalyst/flowcatalyst-go/internal/functions/control"
)

// An endpoint that declares an enormous maxBodyBytes is still held to the
// runner-wide ceiling.
func TestBodyCeilingOverridesManifest(t *testing.T) {
	h := start(t, fnDoc(ver(1, control.RoleLive)))
	over := bytes.Repeat([]byte("x"), MaxRequestBodyBytes+1)
	resp, body := h.do("POST", "/fn/app.hello/huge", over, nil)
	if resp.StatusCode != http.StatusRequestEntityTooLarge || !strings.Contains(string(body), "BODY_TOO_LARGE") {
		t.Fatalf("body over the ceiling: %d %.100s", resp.StatusCode, body)
	}
}

// failReader fails the test if anything reads it.
type failReader struct {
	t    *testing.T
	read bool
}

func (f *failReader) Read([]byte) (int, error) {
	f.read = true
	f.t.Error("the request body was read before authentication")
	return 0, http.ErrBodyReadAfterClose
}

func (f *failReader) Close() error { return nil }

// An unauthenticated platform-auth request is refused without touching the body.
func TestPlatformAuthRefusesBeforeReadingBody(t *testing.T) {
	h := start(t, fnDoc(ver(1, control.RoleLive)))
	for _, auth := range []string{"", "Bearer forged"} {
		fr := &failReader{t: t}
		req := httptest.NewRequest("POST", "/fn/app.hello/secure-post", fr)
		if auth != "" {
			req.Header.Set("Authorization", auth)
		}
		rec := httptest.NewRecorder()
		h.r.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized || fr.read {
			t.Fatalf("auth %q: status %d, body read %v", auth, rec.Code, fr.read)
		}
	}
}

// Random addresses from unauthenticated callers must not create metric series.
func TestMetricSeriesBoundedOnMisses(t *testing.T) {
	h := start(t, fnDoc(ver(1, control.RoleLive)))
	count := func() int {
		mfs, err := h.r.metrics.reg.Gather()
		if err != nil {
			t.Fatal(err)
		}
		n := 0
		for _, mf := range mfs {
			n += len(mf.GetMetric())
		}
		return n
	}
	h.do("GET", "/fn/nope.0/echo", nil, nil)
	before := count()
	for i := range 1000 {
		h.do("GET", "/fn/rand"+strconv.Itoa(i)+".x@qa/echo", nil, nil)
	}
	if after := count(); after > before+2 {
		t.Fatalf("series grew from %d to %d across 1000 random addresses", before, after)
	}
}
