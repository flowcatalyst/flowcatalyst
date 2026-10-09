package assets

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The vendored datastar.js must be exactly the pinned upstream file.
func TestDatastarIsThePinnedFile(t *testing.T) {
	f := byName["datastar.js"]
	if f == nil {
		t.Fatal("datastar.js not embedded")
	}
	sum := sha256.Sum256(f.body)
	if got := hex.EncodeToString(sum[:]); got != DatastarSHA256 {
		t.Fatalf("datastar.js sha256 = %s, want %s (re-download the pinned release; see docs/ui/README.md)", got, DatastarSHA256)
	}
	if !strings.HasPrefix(string(f.body), "// Datastar v"+DatastarVersion) {
		t.Fatalf("datastar.js header is not v%s", DatastarVersion)
	}
}

func TestRequiredAssetsAndNotices(t *testing.T) {
	for _, name := range []string{"kit.css", "lucide-sprite.svg", "datastar.js", "LICENSE-datastar.md", "LICENSE-lucide.txt", "KIT-THIRD-PARTY-NOTICES.md"} {
		if byName[name] == nil {
			t.Errorf("%s is not embedded", name)
		}
	}
}

func get(t *testing.T, target string, hdr map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, target, nil)
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	Handler().ServeHTTP(rec, req)
	return rec
}

func TestHashedURLIsImmutable(t *testing.T) {
	u := URL("kit.css")
	if !strings.HasPrefix(u, Prefix+"kit.") || !strings.HasSuffix(u, ".css") || u == Prefix+"kit.css" {
		t.Fatalf("URL(kit.css) = %q, want a hashed name", u)
	}
	rec := get(t, u, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	if cc := rec.Header().Get("Cache-Control"); cc != "public, max-age=31536000, immutable" {
		t.Errorf("Cache-Control = %q", cc)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "text/css; charset=utf-8" {
		t.Errorf("Content-Type = %q", ct)
	}
	if rec.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Error("missing nosniff")
	}
}

func TestPlainNameRevalidates(t *testing.T) {
	rec := get(t, Prefix+"datastar.js", nil)
	if rec.Code != http.StatusOK || rec.Header().Get("Cache-Control") != "no-cache" {
		t.Fatalf("status %d, Cache-Control %q", rec.Code, rec.Header().Get("Cache-Control"))
	}
	etag := rec.Header().Get("ETag")
	if etag == "" {
		t.Fatal("no ETag")
	}
	rec = get(t, Prefix+"datastar.js", map[string]string{"If-None-Match": etag})
	if rec.Code != http.StatusNotModified || rec.Body.Len() != 0 {
		t.Fatalf("revalidation: status %d, %d body bytes", rec.Code, rec.Body.Len())
	}
}

func TestUnknownAndTraversal(t *testing.T) {
	for _, p := range []string{Prefix + "nope.css", Prefix + "../assets.go", Prefix + "static/kit.css", Prefix} {
		if rec := get(t, p, nil); rec.Code != http.StatusNotFound {
			t.Errorf("%s: status %d, want 404", p, rec.Code)
		}
	}
	req := httptest.NewRequest(http.MethodPost, URL("kit.css"), nil)
	rec := httptest.NewRecorder()
	Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST: status %d", rec.Code)
	}
}

func TestURLPanicsOnUnknownName(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("URL(unknown) did not panic")
		}
	}()
	_ = URL("missing.css")
}
