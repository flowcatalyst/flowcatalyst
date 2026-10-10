// Package assets embeds and serves the UI's static files: the kit stylesheet
// (built from the framework's ui-kit by scripts/ui-kit-sync.sh), the Lucide
// icon sprite, the pinned datastar.js and the licence notices. Nothing is
// loaded from a CDN at run time.
//
// Every file is served under Prefix twice: at a content-hashed name
// ("kit.3f2a9c01de.css") with a one-year immutable cache, and at its plain
// name with `no-cache` + ETag revalidation. Pages always link the hashed URL
// (URL("kit.css")), so a deploy busts caches without any build step.
//
// Text assets (CSS, JavaScript, SVG, notices) are also served compressed:
// each is compressed once, at the best Brotli and gzip levels, on its first
// request in that coding, and the variant is chosen from Accept-Encoding
// (Vary: Accept-Encoding, an ETag per variant). They are mounted outside the
// page group, so the page compression (internal/ui/compress) never sees
// them.
//
// Provenance (kit commit, Tailwind version, Datastar version and hash) is in
// KIT_SOURCE next to this file; the refresh procedure is in docs/ui/README.md.
package assets

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"fmt"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"regexp"
	"strings"
	"sync"

	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/compress"
)

// Prefix is the URL path the assets are served under.
const Prefix = "/ui/assets/"

// DatastarVersion is the vendored Datastar release, matching the kit's
// pinned version (ui-kit README).
const DatastarVersion = "1.0.4"

// DatastarSHA256 is the sha256 of static/datastar.js as downloaded from
// https://cdn.jsdelivr.net/gh/starfederation/datastar@1.0.4/bundles/datastar.js
// (the URL the kit gallery loads). A test fails if the embedded file differs.
const DatastarSHA256 = "727844adfc825ee651fb93c544a2a739986f9a21820a94524b35f0cac470cf91"

//go:embed static
var embedded embed.FS

type file struct {
	name   string // logical name, e.g. "kit.css"
	hashed string // e.g. "kit.3f2a9c01de.css"
	etag   string
	ctype  string
	body   []byte
	// text files are served compressed too; variants holds one encoding per
	// coding, made on first use.
	text     bool
	variants map[string]*variant
}

type variant struct {
	once sync.Once
	body []byte
}

// encoded is the body in coding (compressed once).
func (f *file) encoded(coding string) []byte {
	v := f.variants[coding]
	v.once.Do(func() { v.body = compress.Encode(coding, f.body) })
	return v.body
}

// variantETag is the ETag of the body in coding (a representation of its
// own, RFC 9110 8.8.3).
func (f *file) variantETag(coding string) string {
	return strings.TrimSuffix(f.etag, `"`) + "-" + coding + `"`
}

var (
	byName   = map[string]*file{}
	byHashed = map[string]*file{}
)

func init() {
	sub, err := fs.Sub(embedded, "static")
	if err != nil {
		panic(err)
	}
	err = fs.WalkDir(sub, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		body, err := fs.ReadFile(sub, p)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(body)
		h := hex.EncodeToString(sum[:])
		ext := path.Ext(p)
		f := &file{
			name:   p,
			hashed: strings.TrimSuffix(p, ext) + "." + h[:10] + ext,
			etag:   `"` + h[:16] + `"`,
			ctype:  contentType(ext),
			body:   body,
		}
		if strings.HasPrefix(f.ctype, "text/") || f.ctype == "image/svg+xml" {
			f.text = true
			f.variants = map[string]*variant{compress.Brotli: {}, compress.Gzip: {}}
		}
		byName[f.name] = f
		byHashed[f.hashed] = f
		return nil
	})
	if err != nil {
		panic(fmt.Sprintf("assets: walk embedded files: %v", err))
	}
}

var symbolID = regexp.MustCompile(`<symbol id="i-([a-z0-9-]+)"`)

// HasIcon reports whether the embedded sprite has the Lucide icon name
// (the symbol "i-"+name). Tests use it to check every icon the UI names.
func HasIcon(name string) bool {
	for _, m := range symbolID.FindAllStringSubmatch(string(byName["lucide-sprite.svg"].body), -1) {
		if m[1] == name {
			return true
		}
	}
	return false
}

func contentType(ext string) string {
	switch ext {
	case ".css":
		return "text/css; charset=utf-8"
	case ".js":
		return "text/javascript; charset=utf-8"
	case ".svg":
		return "image/svg+xml"
	case ".md", ".txt":
		return "text/plain; charset=utf-8"
	}
	if t := mime.TypeByExtension(ext); t != "" {
		return t
	}
	return "application/octet-stream"
}

// URL is the long-cache URL of a logical asset name ("kit.css",
// "datastar.js", "lucide-sprite.svg"). An unknown name is a programming
// error and panics, so a typo fails the first render in tests.
func URL(name string) string {
	f, ok := byName[name]
	if !ok {
		panic("assets: unknown asset " + name)
	}
	return Prefix + f.hashed
}

// Handler serves GET/HEAD Prefix+name. Mount it with the prefix intact
// (it reads the last path segment).
func Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		name := strings.TrimPrefix(r.URL.Path, Prefix)
		f, immutable := byHashed[name]
		if !immutable {
			f = byName[name]
		}
		if f == nil {
			http.NotFound(w, r)
			return
		}
		h := w.Header()
		body, etag, coding := f.body, f.etag, ""
		if f.text {
			compress.AddVary(h)
			if coding = compress.Negotiate(r.Header.Get("Accept-Encoding")); coding != "" {
				etag = f.variantETag(coding)
			}
		}
		h.Set("Content-Type", f.ctype)
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("ETag", etag)
		if immutable {
			h.Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			h.Set("Cache-Control", "no-cache")
		}
		if match := r.Header.Get("If-None-Match"); match != "" && strings.Contains(match, etag) {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		if coding != "" {
			body = f.encoded(coding)
			h.Set("Content-Encoding", coding)
		}
		h.Set("Content-Length", fmt.Sprint(len(body)))
		if r.Method == http.MethodHead {
			return
		}
		_, _ = w.Write(body)
	})
}
