//go:build !wasip1

package fn

import (
	"net/http"
	"strings"
	"testing"
)

func TestInvokeForTest(t *testing.T) {
	resetRegistry()
	Platform("POST /echo/{id}", func(w http.ResponseWriter, r *http.Request) {
		who := CallerFrom(r.Context())
		p := who.Principal()
		w.Header().Set("X-Seen", r.Header.Get("x-token"))
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(r.PathValue("id") + ":" + p.ID + ":" + r.URL.RawQuery))
	})
	Webhook("POST /hook", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Kind", CallerFrom(r.Context()).Kind())
	})

	resp := InvokeForTest(TestRequest{
		Method: "POST", Path: "/echo/7", Query: "a=1", Header: http.Header{"x-token": {"t"}},
		Principal: &Principal{ID: "prn_1", Permissions: []string{"a:b:c:d"}},
	})
	if resp.Status != 202 || string(resp.Body) != "7:prn_1:a=1" || resp.Header.Get("X-Seen") != "t" {
		t.Fatalf("got %d %q %v", resp.Status, resp.Body, resp.Header)
	}
	if r := InvokeForTest(TestRequest{Method: "POST", Path: "/hook", Webhook: true}); r.Header.Get("X-Kind") != "webhook" {
		t.Fatalf("kind header %q", r.Header.Get("X-Kind"))
	}
	if r := InvokeForTest(TestRequest{Path: "/nope"}); r.Status != 404 {
		t.Fatalf("status %d", r.Status)
	}
	if !strings.Contains(string(DescribeForTest()), `"/echo/{id}"`) {
		t.Fatalf("describe %s", DescribeForTest())
	}
}
