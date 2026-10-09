package ds

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// raw is a minimal Component for tests.
type raw string

func (h raw) Render(_ context.Context, w io.Writer) error {
	_, err := io.WriteString(w, string(h))
	return err
}

type failing struct{}

func (failing) Render(context.Context, io.Writer) error { return errors.New("boom") }

func stream(t *testing.T, fn func(s *SSE) error) (*httptest.ResponseRecorder, string) {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/ui/x", nil)
	req.Header.Set(RequestHeader, "true")
	if err := fn(NewSSE(rec, req)); err != nil {
		t.Fatalf("stream: %v", err)
	}
	return rec, rec.Body.String()
}

// The exact bytes on the wire: these are what Datastar 1.0.4 parses, so a
// change in the SDK's framing must fail here, not in a browser. The SDK ends
// every event with "\n\n" after the last "data:" line's own "\n", so events are
// separated by two blank lines; SSE treats the extra one as an empty event
// boundary, which Datastar ignores.
func TestPatchElementsBytes(t *testing.T) {
	cases := []struct {
		name string
		fn   func(s *SSE) error
		want string
	}{
		{
			name: "outer by id (default)",
			fn:   func(s *SSE) error { return s.Patch(raw(`<div id="a">x</div>`)) },
			want: "event: datastar-patch-elements\ndata: elements <div id=\"a\">x</div>\n\n\n",
		},
		{
			name: "inner with selector, multi-line",
			fn: func(s *SSE) error {
				return s.Patch(raw("<tr><td>1</td></tr>\n<tr><td>2</td></tr>"), Inner("#rows"))
			},
			want: "event: datastar-patch-elements\ndata: selector #rows\ndata: mode inner\n" +
				"data: elements <tr><td>1</td></tr>\ndata: elements <tr><td>2</td></tr>\n\n\n",
		},
		{
			name: "append",
			fn: func(s *SSE) error {
				return s.PatchHTML(`<div role="status">Saved.</div>`, Append("#flash-stack"))
			},
			want: "event: datastar-patch-elements\ndata: selector #flash-stack\ndata: mode append\n" +
				"data: elements <div role=\"status\">Saved.</div>\n\n\n",
		},
		{
			name: "explicit selector and mode",
			fn: func(s *SSE) error {
				return s.PatchHTML(`<li>z</li>`, WithSelector("#list"), WithMode(ModePrepend))
			},
			want: "event: datastar-patch-elements\ndata: selector #list\ndata: mode prepend\ndata: elements <li>z</li>\n\n\n",
		},
		{
			name: "remove",
			fn:   func(s *SSE) error { return s.Remove("#row-42") },
			want: "event: datastar-patch-elements\ndata: selector #row-42\ndata: mode remove\n\n\n",
		},
		{
			name: "patch signals",
			fn:   func(s *SSE) error { return s.PatchSignals(map[string]any{"q": "abc", "n": 2}) },
			want: "event: datastar-patch-signals\ndata: signals {\"n\":2,\"q\":\"abc\"}\n\n\n",
		},
		{
			name: "two events in one response",
			fn: func(s *SSE) error {
				return errors.Join(s.Patch(raw(`<p id="a">1</p>`)), s.PatchSignals(struct {
					Open bool `json:"open"`
				}{true}))
			},
			want: "event: datastar-patch-elements\ndata: elements <p id=\"a\">1</p>\n\n\n" +
				"event: datastar-patch-signals\ndata: signals {\"open\":true}\n\n\n",
		},
		{
			name: "redirect to a local path",
			fn:   func(s *SSE) error { return s.Redirect("/auth/login?redirect=/ui/x&a=<b>") },
			want: "event: datastar-patch-elements\ndata: selector body\ndata: mode append\n" +
				"data: elements <div hidden data-init=\"window.location.assign(&#34;/auth/login?redirect=/ui/x\\u0026a=\\u003cb\\u003e&#34;)\"></div>\n\n\n",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec, got := stream(t, tc.fn)
			if got != tc.want {
				t.Fatalf("bytes:\n got %q\nwant %q", got, tc.want)
			}
			if ct := rec.Header().Get("Content-Type"); ct != "text/event-stream" {
				t.Errorf("Content-Type = %q", ct)
			}
			if cc := rec.Header().Get("Cache-Control"); cc != "no-cache" {
				t.Errorf("Cache-Control = %q", cc)
			}
		})
	}
}

func TestPatchRenderErrorIsReturned(t *testing.T) {
	rec := httptest.NewRecorder()
	s := NewSSE(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if err := s.Patch(failing{}); err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("err = %v, want the render error", err)
	}
	if strings.Contains(rec.Body.String(), "event:") {
		t.Fatalf("a failed render must not emit an event, got %q", rec.Body.String())
	}
}

func TestRedirectRefusesOtherOrigins(t *testing.T) {
	for _, target := range []string{"https://evil.example/x", "//evil.example/x", `/\evil.example`, "ui/x", "javascript:alert(1)", "/ui/\nx"} {
		rec := httptest.NewRecorder()
		s := NewSSE(rec, httptest.NewRequest(http.MethodGet, "/", nil))
		if err := s.Redirect(target); !errors.Is(err, ErrUnsafeRedirect) {
			t.Errorf("Redirect(%q) err = %v, want ErrUnsafeRedirect", target, err)
		}
		if rec.Body.Len() != 0 {
			t.Errorf("Redirect(%q) wrote %q", target, rec.Body.String())
		}
	}
}

func TestIsDatastar(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	if IsDatastar(r) {
		t.Fatal("plain request reported as Datastar")
	}
	r.Header.Set(RequestHeader, "true")
	if !IsDatastar(r) {
		t.Fatal("Datastar-Request: true not detected")
	}
}

type filters struct {
	Q      string `json:"q"`
	Status string `json:"status"`
}

func TestReadSignals(t *testing.T) {
	t.Run("GET reads the datastar query parameter", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/ui/x?datastar="+url.QueryEscape(`{"q":"abc","status":"ACTIVE"}`), nil)
		var f filters
		if err := ReadSignals(r, &f); err != nil {
			t.Fatal(err)
		}
		if f != (filters{Q: "abc", Status: "ACTIVE"}) {
			t.Fatalf("got %+v", f)
		}
	})
	t.Run("DELETE reads the query too (Datastar 1.0.4)", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodDelete, "/ui/x?datastar="+url.QueryEscape(`{"q":"d"}`), strings.NewReader(`{"q":"body"}`))
		var f filters
		if err := ReadSignals(r, &f); err != nil {
			t.Fatal(err)
		}
		if f.Q != "d" {
			t.Fatalf("got %+v, want the query value", f)
		}
	})
	t.Run("POST reads the JSON body", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodPost, "/ui/x?datastar="+url.QueryEscape(`{"q":"query"}`), strings.NewReader(`{"q":"body","status":"SUSPENDED"}`))
		var f filters
		if err := ReadSignals(r, &f); err != nil {
			t.Fatal(err)
		}
		if f != (filters{Q: "body", Status: "SUSPENDED"}) {
			t.Fatalf("got %+v", f)
		}
	})
	t.Run("no signals leaves the target untouched", func(t *testing.T) {
		for _, r := range []*http.Request{
			httptest.NewRequest(http.MethodGet, "/ui/x", nil),
			httptest.NewRequest(http.MethodPost, "/ui/x", nil),
			httptest.NewRequest(http.MethodPut, "/ui/x", strings.NewReader("  ")),
		} {
			f := filters{Q: "keep"}
			if err := ReadSignals(r, &f); err != nil {
				t.Fatalf("%s: %v", r.Method, err)
			}
			if f.Q != "keep" {
				t.Fatalf("%s: target changed to %+v", r.Method, f)
			}
		}
	})
	t.Run("malformed JSON is an error", func(t *testing.T) {
		for _, r := range []*http.Request{
			httptest.NewRequest(http.MethodGet, "/ui/x?datastar=%7Bnope", nil),
			httptest.NewRequest(http.MethodPost, "/ui/x", strings.NewReader("{nope")),
		} {
			var f filters
			if err := ReadSignals(r, &f); err == nil {
				t.Fatalf("%s: malformed signals accepted", r.Method)
			}
		}
	})
	t.Run("oversized body is refused", func(t *testing.T) {
		big := `{"q":"` + strings.Repeat("a", MaxSignalBytes) + `"}`
		r := httptest.NewRequest(http.MethodPost, "/ui/x", strings.NewReader(big))
		var f filters
		err := ReadSignals(r, &f)
		if _, ok := errors.AsType[*http.MaxBytesError](err); !ok {
			t.Fatalf("err = %v, want *http.MaxBytesError", err)
		}
	})
}
