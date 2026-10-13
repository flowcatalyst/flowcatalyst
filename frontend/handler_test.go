package frontend

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

const testIndex = `<!doctype html><html><head><script type="module" src="/assets/index-abc123.js"></script></head><body><div id="app"></div></body></html>`

// withDist swaps the embedded tree for a fixture for one test.
func withDist(t *testing.T, fsys fs.FS) {
	t.Helper()
	prev := distFS
	distFS = fsys
	resetCache()
	t.Cleanup(func() {
		distFS = prev
		resetCache()
	})
}

func fixtureDist() fstest.MapFS {
	return fstest.MapFS{
		"index.html":              {Data: []byte(testIndex)},
		"favicon.svg":             {Data: []byte(`<svg xmlns="http://www.w3.org/2000/svg"/>`)},
		"assets/index-abc123.js":  {Data: []byte(`console.log("app")`)},
		"assets/index-abc123.css": {Data: []byte(`body{}`)},
	}
}

func get(t *testing.T, method, target string, hdr map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(method, target, nil)
	for k, v := range hdr {
		r.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	Handler().ServeHTTP(w, r)
	return w
}

func isIndex(w *httptest.ResponseRecorder) bool {
	return strings.Contains(w.Body.String(), `<div id="app">`)
}

// The SPA fallback serves index.html for client routes only: extension-less
// paths outside the API prefixes.
func TestSPAFallbackForClientRoutes(t *testing.T) {
	withDist(t, fixtureDist())
	for _, p := range []string{"/", "/dashboard", "/clients/clt_0ABC", "/auth/login", "/authorization/roles/orders:admin", "/portal/login"} {
		w := get(t, http.MethodGet, p, nil)
		if w.Code != http.StatusOK || !isIndex(w) {
			t.Errorf("GET %s: status %d, index=%v; want the SPA shell", p, w.Code, isIndex(w))
		}
	}
}

func TestMissingAssetIs404NotIndex(t *testing.T) {
	withDist(t, fixtureDist())
	for _, p := range []string{
		"/assets/index-OLDHASH.js",
		"/assets/Page-OLD.css",
		"/assets/",
		"/assets/no-extension",
		"/favicon.ico",
		"/robots.txt",
		"/some/deep/chunk-x.js",
		"/manifest.webmanifest",
		"/logo.png",
	} {
		w := get(t, http.MethodGet, p, nil)
		if w.Code != http.StatusNotFound || isIndex(w) {
			t.Errorf("GET %s: status %d, index=%v; want a 404 without the SPA shell", p, w.Code, isIndex(w))
		}
		if cc := w.Header().Get("Cache-Control"); cc != "no-store" {
			t.Errorf("GET %s: Cache-Control %q; a 404 must not be cached (a deploy in progress)", p, cc)
		}
	}
}

func TestUnknownAPIPathIs404JSON(t *testing.T) {
	withDist(t, fixtureDist())
	for _, p := range []string{"/api/nope", "/api", "/bff/nope", "/oauth/nope", "/.well-known/nope", "/control/nope", "/q/nope"} {
		for _, m := range []string{http.MethodGet, http.MethodPost, http.MethodDelete} {
			w := get(t, m, p, map[string]string{"Accept": "text/html"})
			if w.Code != http.StatusNotFound || isIndex(w) {
				t.Errorf("%s %s: status %d, index=%v; want a 404", m, p, w.Code, isIndex(w))
				continue
			}
			if ct := w.Header().Get("Content-Type"); ct != "application/json" {
				t.Errorf("%s %s: Content-Type %q, want the API's JSON envelope", m, p, ct)
			}
			if !strings.Contains(w.Body.String(), `"error":"NOT_FOUND"`) {
				t.Errorf("%s %s: body %s", m, p, w.Body.String())
			}
		}
	}
}

// Prefix matching is by path segment: /apiary is a client route, not /api.
func TestAPIPrefixMatchesWholeSegment(t *testing.T) {
	withDist(t, fixtureDist())
	w := get(t, http.MethodGet, "/apiary", nil)
	if w.Code != http.StatusOK || !isIndex(w) {
		t.Errorf("GET /apiary: status %d; want the SPA shell", w.Code)
	}
}

func TestUnsafeMethodOnClientRouteIsNotServed(t *testing.T) {
	withDist(t, fixtureDist())
	for _, m := range []string{http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodPatch} {
		w := get(t, m, "/dashboard", nil)
		if w.Code != http.StatusMethodNotAllowed || isIndex(w) {
			t.Errorf("%s /dashboard: status %d, index=%v; want 405", m, w.Code, isIndex(w))
		}
		if a := w.Header().Get("Allow"); a != "GET, HEAD" {
			t.Errorf("%s /dashboard: Allow %q", m, a)
		}
	}
}

func TestCacheHeaders(t *testing.T) {
	withDist(t, fixtureDist())

	idx := get(t, http.MethodGet, "/clients", nil)
	if cc := idx.Header().Get("Cache-Control"); cc != "no-cache" {
		t.Errorf("index Cache-Control %q, want no-cache (revalidate)", cc)
	}
	etag := idx.Header().Get("ETag")
	if etag == "" {
		t.Fatal("index has no ETag to revalidate with")
	}
	if root := get(t, http.MethodGet, "/", nil); root.Header().Get("ETag") != etag {
		t.Errorf("/ and /clients serve the same index.html under different ETags")
	}
	nm := get(t, http.MethodGet, "/clients", map[string]string{"If-None-Match": etag})
	if nm.Code != http.StatusNotModified {
		t.Errorf("index revalidation: status %d, want 304", nm.Code)
	}

	js := get(t, http.MethodGet, "/assets/index-abc123.js", nil)
	if js.Code != http.StatusOK {
		t.Fatalf("asset status %d", js.Code)
	}
	if cc := js.Header().Get("Cache-Control"); cc != "public, max-age=31536000, immutable" {
		t.Errorf("hashed asset Cache-Control %q", cc)
	}
	if ct := js.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/javascript") {
		t.Errorf("asset Content-Type %q", ct)
	}

	fav := get(t, http.MethodGet, "/favicon.svg", nil)
	if cc := fav.Header().Get("Cache-Control"); cc != "no-cache" {
		t.Errorf("unhashed root file Cache-Control %q, want no-cache", cc)
	}
	if fav.Header().Get("ETag") == "" {
		t.Error("unhashed root file has no ETag")
	}

	head := get(t, http.MethodHead, "/assets/index-abc123.css", nil)
	if head.Code != http.StatusOK || head.Body.Len() != 0 {
		t.Errorf("HEAD asset: status %d, %d body bytes", head.Code, head.Body.Len())
	}
}

func TestSecurityHeaders(t *testing.T) {
	withDist(t, fixtureDist())
	for _, p := range []string{"/", "/dashboard", "/assets/index-abc123.js", "/assets/missing.js", "/api/nope"} {
		w := get(t, http.MethodGet, p, nil)
		h := w.Header()
		if h.Get("X-Content-Type-Options") != "nosniff" {
			t.Errorf("%s: X-Content-Type-Options %q", p, h.Get("X-Content-Type-Options"))
		}
		if h.Get("X-Frame-Options") != "DENY" {
			t.Errorf("%s: X-Frame-Options %q", p, h.Get("X-Frame-Options"))
		}
		if h.Get("Referrer-Policy") != "same-origin" {
			t.Errorf("%s: Referrer-Policy %q", p, h.Get("Referrer-Policy"))
		}
		csp := h.Get("Content-Security-Policy")
		for _, d := range []string{"frame-ancestors 'none'", "object-src 'none'", "base-uri 'self'"} {
			if !strings.Contains(csp, d) {
				t.Errorf("%s: CSP %q lacks %q", p, csp, d)
			}
		}
		// The policy must not restrict scripts, styles or connections: the
		// SPA has not been run under one (docs/platform-bug-fixes.md F16).
		for _, d := range []string{"default-src", "script-src", "style-src", "connect-src"} {
			if strings.Contains(csp, d) {
				t.Errorf("%s: CSP %q restricts %s", p, csp, d)
			}
		}
	}
}

func TestPathTraversalIsNotServed(t *testing.T) {
	withDist(t, fixtureDist())
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.URL.Path = "/assets/../../embed.go"
	w := httptest.NewRecorder()
	Handler().ServeHTTP(w, r)
	if strings.Contains(w.Body.String(), "package frontend") {
		t.Fatal("served a file outside dist")
	}
}

// The real embedded build (when `make frontend` ran before the test build):
// its index.html references hashed assets that must be served as immutable,
// and a stale hash must 404.
func TestEmbeddedBuild(t *testing.T) {
	if !IsAvailable() {
		t.Skip("frontend/dist not built into this test binary")
	}
	w := get(t, http.MethodGet, "/", nil)
	if w.Code != http.StatusOK || w.Header().Get("Cache-Control") != "no-cache" {
		t.Fatalf("index: status %d, Cache-Control %q", w.Code, w.Header().Get("Cache-Control"))
	}
	body := w.Body.String()
	i := strings.Index(body, `src="/assets/`)
	if i < 0 {
		t.Fatalf("index.html references no /assets/ script: %s", body)
	}
	src := body[i+len(`src="`):]
	src = src[:strings.IndexByte(src, '"')]
	a := get(t, http.MethodGet, src, nil)
	if a.Code != http.StatusOK || a.Header().Get("Cache-Control") != "public, max-age=31536000, immutable" {
		t.Fatalf("%s: status %d, Cache-Control %q", src, a.Code, a.Header().Get("Cache-Control"))
	}
	if m := get(t, http.MethodGet, "/assets/index-STALEHASH.js", nil); m.Code != http.StatusNotFound {
		t.Fatalf("stale asset: status %d, want 404", m.Code)
	}
}
