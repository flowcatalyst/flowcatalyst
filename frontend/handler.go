package frontend

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"slices"
	"strings"
	"sync"
	"time"
)

// Handler returns an http.Handler that serves the embedded SPA. It is the
// server's fallback (chi NotFound), so it sees every request no route
// matched:
//
//  1. Paths under the API prefixes (/api, /bff, /oauth, /.well-known,
//     /control, /q): 404 with the API's JSON error envelope, never the SPA
//     shell (a client calling a mistyped or removed route must not get 200
//     and HTML).
//  2. Methods other than GET and HEAD: 405.
//  3. An exact file under dist/: served with its MIME type and an ETag.
//     Files under /assets/ (Vite's hash-suffixed names) get `Cache-Control:
//     public, max-age=31536000, immutable`; any other file `no-cache`
//     (revalidate with the ETag).
//  4. A missing file (anything under /assets/, or a path whose extension is a
//     static-asset type): 404, `no-store`. A stale hashed chunk must fail as
//     a 404, not load index.html as JavaScript.
//  5. Anything else (an extension-less client route): index.html,
//     `no-cache` with an ETag, so Vue Router's history mode can take over and
//     a new deploy's shell is picked up on the next load.
//
// Every response carries nosniff, Referrer-Policy: same-origin, framing
// denial (X-Frame-Options and CSP frame-ancestors), object-src 'none' and
// base-uri 'self'. The CSP deliberately does not restrict scripts, styles or
// connections (docs/platform-bug-fixes.md, F16).
//
// Returns a "frontend not built" handler if `frontend/dist/` was empty at
// compile time — distinguishes "we forgot to run pnpm build" from a genuine
// 404 for clarity.
func Handler() http.Handler {
	if distFS == nil {
		return http.HandlerFunc(missingFrontend)
	}
	return http.HandlerFunc(serveEmbedded)
}

// IsAvailable reports whether the embedded SPA has anything to serve.
// Callers can use this to skip mounting the frontend route when the
// binary was built without `make frontend` (e.g. for backend-only
// iteration) — the user sees an honest "no frontend wired" instead of
// a generic 404.
func IsAvailable() bool { return distFS != nil }

// apiPrefixes are the first path segments the platform's APIs own. The SPA
// has no client routes under them.
var apiPrefixes = []string{"api", "bff", "oauth", ".well-known", "control", "q"}

// IsAPIPath reports whether p (a URL path) lies under one of the API
// prefixes, matched by whole segment (/apiary is not /api).
func IsAPIPath(p string) bool {
	first, _, _ := strings.Cut(strings.TrimPrefix(p, "/"), "/")
	return slices.Contains(apiPrefixes, first)
}

// assetExtensions are the file types a missing path is a 404 for, rather
// than a client route. An allow-list, not "has a dot": client routes carry
// identifiers that may contain dots.
var assetExtensions = map[string]bool{
	".js": true, ".mjs": true, ".cjs": true, ".css": true, ".map": true,
	".json": true, ".webmanifest": true, ".txt": true, ".xml": true, ".wasm": true,
	".html": true, ".htm": true,
	".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".svg": true, ".ico": true,
	".webp": true, ".avif": true, ".bmp": true,
	".woff": true, ".woff2": true, ".ttf": true, ".otf": true, ".eot": true,
}

// contentSecurityPolicy restricts framing, plugins and <base> only. A policy
// on scripts and styles is not set: PrimeVue injects <style> elements at
// runtime and the Swagger UI chunk uses the Function constructor, and the SPA
// has not been run under such a policy.
const contentSecurityPolicy = "frame-ancestors 'none'; object-src 'none'; base-uri 'self'"

func setSecurityHeaders(h http.Header) {
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Referrer-Policy", "same-origin")
	h.Set("X-Frame-Options", "DENY")
	h.Set("Content-Security-Policy", contentSecurityPolicy)
}

func serveEmbedded(w http.ResponseWriter, r *http.Request) {
	setSecurityHeaders(w.Header())

	if IsAPIPath(r.URL.Path) {
		apiNotFound(w, r)
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		plainStatus(w, http.StatusMethodNotAllowed)
		return
	}

	// Clean against a rooted path so ".." can never climb out of dist/
	// (fs.FS rejects such names anyway; this keeps the answer a 404).
	reqPath := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
	if reqPath == "" || reqPath == "index.html" {
		serveFile(w, r, "index.html", "no-cache")
		return
	}

	if info, err := fs.Stat(distFS, reqPath); err == nil && !info.IsDir() {
		cc := "no-cache"
		if strings.HasPrefix(reqPath, "assets/") {
			cc = "public, max-age=31536000, immutable"
		}
		serveFile(w, r, reqPath, cc)
		return
	}

	if strings.HasPrefix(reqPath, "assets/") || reqPath == "assets" || assetExtensions[strings.ToLower(path.Ext(reqPath))] {
		plainStatus(w, http.StatusNotFound)
		return
	}
	serveFile(w, r, "index.html", "no-cache")
}

// serveFile serves name from dist with its MIME type, the given
// Cache-Control and a content-hash ETag (embedded files have no modification
// time, so the ETag is what conditional requests revalidate against).
func serveFile(w http.ResponseWriter, r *http.Request, name, cacheControl string) {
	f, err := distFS.Open(name)
	if err != nil {
		if name == "index.html" {
			// Impossible: distFS init verified index.html exists.
			http.Error(w, "index.html missing from embedded frontend", http.StatusInternalServerError)
			return
		}
		plainStatus(w, http.StatusNotFound)
		return
	}
	defer func() { _ = f.Close() }()
	rs, ok := f.(io.ReadSeeker)
	if !ok {
		http.Error(w, "embedded file is not seekable", http.StatusInternalServerError)
		return
	}
	etag, err := etagFor(name, rs)
	if err != nil {
		http.Error(w, "embedded file unreadable", http.StatusInternalServerError)
		return
	}

	h := w.Header()
	ct := mime.TypeByExtension(path.Ext(name))
	if ct == "" {
		ct = "application/octet-stream"
	}
	h.Set("Content-Type", ct)
	h.Set("Cache-Control", cacheControl)
	h.Set("ETag", etag)
	http.ServeContent(w, r, name, time.Time{}, rs)
}

// etags caches each embedded file's ETag (the content never changes for the
// life of the binary).
var etags sync.Map

func etagFor(name string, rs io.ReadSeeker) (string, error) {
	if v, ok := etags.Load(name); ok {
		return v.(string), nil
	}
	sum := sha256.New()
	if _, err := io.Copy(sum, rs); err != nil {
		return "", err
	}
	if _, err := rs.Seek(0, io.SeekStart); err != nil {
		return "", err
	}
	tag := `"` + hex.EncodeToString(sum.Sum(nil)[:16]) + `"`
	etags.Store(name, tag)
	return tag, nil
}

// resetCache forgets the cached ETags (tests swap distFS).
func resetCache() { etags.Clear() }

func plainStatus(w http.ResponseWriter, status int) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_, _ = io.WriteString(w, http.StatusText(status)+"\n")
}

// apiNotFound answers an unknown API path with the platform's JSON error
// envelope ({error, message}).
func apiNotFound(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusNotFound)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"error":   "NOT_FOUND",
		"message": "No route for " + r.Method + " " + r.URL.Path,
	})
}

func missingFrontend(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusNotFound)
	_, _ = w.Write([]byte(
		"Frontend not built into this binary.\n" +
			"Run `make frontend` (or `cd frontend && pnpm install && pnpm build`)\n" +
			"and rebuild the binary.\n"))
}
