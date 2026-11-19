package public

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"html/template"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/appupdate"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/function/artifact"
)

// The install page and download (docs/app-updates-design.md section 14.5): a
// person who holds a link opens a small page that lists the newest Android
// build of each variant of the link's channel and downloads one. The routes
// are unauthenticated and hostile-facing like the update routes: every
// failure (a wrong, replaced or malformed token, an unknown or inactive
// application, an address that names another app, a channel with no binary)
// is the same 404 page, after the same single indexed read.

// InstallRepository is what the install routes read (*appupdate.LinkRepository).
type InstallRepository interface {
	PublicInstallByToken(ctx context.Context, tokenHash string) (*appupdate.PublicInstall, error)
}

// APKContentType is the content type Android's package installer opens.
const APKContentType = "application/vnd.android.package-archive"

// installCSS is the page's whole stylesheet; the content security policy
// admits it by its hash and nothing else. Colours are the console's (its
// light and dark themes), copied so the page needs nothing from elsewhere.
const installCSS = `
:root{color-scheme:light dark;--bg:#f8fafc;--fg:#1e293b;--card:#fff;--border:#e2e8f0;--muted:#64748b;--primary:#059669;--primary-hover:#047857;--on-primary:#fff}
@media (prefers-color-scheme:dark){:root{--bg:#0f172a;--fg:#e2e8f0;--card:#1e293b;--border:#334155;--muted:#94a3b8;--primary:#10b981;--primary-hover:#34d399;--on-primary:#022c22}}
*{box-sizing:border-box}
body{margin:0;background:var(--bg);color:var(--fg);font:16px/1.5 system-ui,-apple-system,"Segoe UI",Roboto,sans-serif}
main{max-width:44rem;margin:0 auto;padding:1.5rem 1rem 3rem}
h1{font-size:1.5rem;line-height:1.3;margin:0 0 .25rem}
h2{font-size:1.125rem;margin:0 0 .25rem}
p{margin:.25rem 0}
.sub{color:var(--muted);margin-bottom:1.5rem}
.build{background:var(--card);border:1px solid var(--border);border-radius:.75rem;padding:1rem;margin:0 0 1rem}
.build .what{color:var(--muted)}
.tag{display:inline-block;font-size:.8125rem;border:1px solid var(--primary);color:var(--primary);border-radius:999px;padding:0 .5rem;margin-left:.5rem;vertical-align:middle}
.facts{color:var(--muted);font-size:.9375rem;margin:.5rem 0 1rem}
a.button{display:inline-block;background:var(--primary);color:var(--on-primary);text-decoration:none;font-weight:600;border-radius:.5rem;padding:.75rem 1.25rem;min-height:3rem}
a.button:hover,a.button:focus{background:var(--primary-hover)}
a.button:focus-visible{outline:3px solid var(--primary);outline-offset:2px}
a.button.secondary{background:transparent;color:var(--fg);border:1px solid var(--border)}
section{margin-top:2rem}
ol{padding-left:1.25rem}
li{margin:.5rem 0}
.note{border-left:4px solid var(--primary);padding:.5rem .75rem;background:var(--card)}
.qr{display:flex;gap:1rem;align-items:center;flex-wrap:wrap}
.qr svg{width:12rem;height:12rem;border-radius:.5rem;border:1px solid var(--border)}
`

// installCSP is the page's policy: no script, no request for anything (the
// QR code is inline SVG), the one stylesheet by hash, no framing, no forms.
var installCSP = func() string {
	sum := sha256.Sum256([]byte(installCSS))
	return "default-src 'none'; style-src 'sha256-" + base64.StdEncoding.EncodeToString(sum[:]) + "'; " +
		"img-src 'none'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'"
}()

// installBuild is one variant's card.
type installBuild struct {
	Title      string
	Tag        string
	What       string
	Version    string
	Date       string
	Size       string
	Href       string
	FileName   string
	ButtonText string
	Primary    bool
}

type installView struct {
	AppName string
	Channel string
	CSS     template.CSS
	Builds  []installBuild
	Switch  string
	QR      template.HTML
}

var installTemplate = template.Must(template.New("install").Parse(`<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<meta name="robots" content="noindex, nofollow">
<meta name="referrer" content="no-referrer">
<title>Install {{.AppName}}</title>
<style>{{.CSS}}</style>
</head>
<body>
<main>
<h1>Install {{.AppName}}</h1>
<p class="sub">Android app, {{.Channel}} channel</p>
{{range .Builds}}<div class="build">
<h2>{{.Title}}{{if .Tag}}<span class="tag">{{.Tag}}</span>{{end}}</h2>
<p class="what">{{.What}}</p>
<p class="facts">Version {{.Version}} · {{.Date}} · {{.Size}}</p>
<a class="button{{if not .Primary}} secondary{{end}}" href="{{.Href}}" download="{{.FileName}}">{{.ButtonText}}</a>
</div>
{{end}}<section>
<h2>How to install</h2>
<ol>
<li>Open this page on the Android phone you want to install the app on.</li>
<li>Tap the download button. If the phone asks, allow your browser to install apps: open Settings, find “Install unknown apps” (under Apps or Security), choose your browser and turn on “Allow from this source”.</li>
<li>Open the downloaded file and tap Install. If the app is already on the phone, it is updated and keeps its data.</li>
<li>If the phone says the app was not installed, the app on the phone is a newer version or came from somewhere else. Uninstall it, then install this one. Uninstalling removes the app’s data from the phone.</li>
</ol>
{{if .Switch}}<p class="note">{{.Switch}}</p>{{end}}
</section>
<section class="qr">
{{.QR}}
<p>Reading this on a computer, or on another phone? Scan the code with the phone’s camera to open this page there.</p>
</section>
</main>
</body>
</html>
`))

// notFoundPage is every failure of the install routes, byte for byte.
var notFoundPage = []byte(`<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<meta name="robots" content="noindex, nofollow">
<title>Link not found</title>
<style>` + installCSS + `</style>
</head>
<body>
<main>
<h1>This link does not work</h1>
<p>It may have been replaced, or there is nothing to install yet. Ask the person who sent it for a new link.</p>
</main>
</body>
</html>
`)

// setInstallHeaders are the headers of every install-route answer: the token
// is in the address, so no referrer, no indexing, and no framing or sniffing.
func setInstallHeaders(h http.Header) {
	h.Set("Content-Security-Policy", installCSP)
	h.Set("Referrer-Policy", "no-referrer")
	h.Set("X-Robots-Tag", "noindex, nofollow")
	h.Set("X-Frame-Options", "DENY")
	h.Set("X-Content-Type-Options", "nosniff")
}

func writeInstallNotFound(w http.ResponseWriter) {
	h := w.Header()
	setInstallHeaders(h)
	h.Set("Content-Type", "text/html; charset=utf-8")
	h.Set("Cache-Control", "no-store")
	h.Set("Content-Length", strconv.Itoa(len(notFoundPage)))
	w.WriteHeader(http.StatusNotFound)
	_, _ = w.Write(notFoundPage)
}

func writeInstallUnavailable(w http.ResponseWriter) {
	h := w.Header()
	setInstallHeaders(h)
	h.Set("Content-Type", "text/plain; charset=utf-8")
	h.Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusInternalServerError)
	_, _ = io.WriteString(w, "Something went wrong. Try again later.\n")
}

// lookupInstall resolves an install address. The token is hashed and the one
// read is by its hash whatever the address holds (a malformed token or
// application code does the same work as a wrong one); the stored hash is then
// compared in constant time and the address must name the link's application
// and app key. nil, nil is every kind of "no".
func (s *Server) lookupInstall(ctx context.Context, code, appKey, token string) (*appupdate.PublicInstall, error) {
	hash := appupdate.HashInstallToken(token)
	inst, err := s.Links.PublicInstallByToken(ctx, hash)
	if err != nil {
		return nil, err
	}
	if inst == nil {
		return nil, nil
	}
	ok := appupdate.InstallTokenMatches(inst.Link.TokenHash, hash)
	ok = ok && appupdate.ValidInstallToken(token) && inst.Link.ApplicationCode == code && inst.Link.AppKey == appKey
	if !ok || len(inst.Binaries) == 0 {
		return nil, nil
	}
	return inst, nil
}

func (s *Server) installPage(w http.ResponseWriter, r *http.Request) {
	code, appKey, token := chi.URLParam(r, "applicationCode"), chi.URLParam(r, "appKey"), chi.URLParam(r, "token")
	inst, err := s.lookupInstall(r.Context(), code, appKey, token)
	if err != nil {
		slog.Error("app binaries: install page read failed", "err", err)
		writeInstallUnavailable(w)
		return
	}
	if inst == nil {
		writeInstallNotFound(w)
		return
	}
	addr := s.installAddress(r, code, appKey, token)
	qr, err := qrSVG(addr, "QR code of this page’s address")
	if err != nil {
		slog.Error("app binaries: QR code not drawn", "err", err)
	}
	view := installView{
		AppName: inst.ApplicationName, Channel: inst.Link.Channel, CSS: template.CSS(installCSS), //nolint:gosec // G203: a constant
		QR:     template.HTML(qr), //nolint:gosec // G203: built from integers and an escaped constant label (qrSVG)
		Switch: switchNote(inst.Binaries),
	}
	for i := range inst.Binaries {
		view.Builds = append(view.Builds, buildCard(&inst.Binaries[i], token))
	}
	var body bytes.Buffer
	if err := installTemplate.Execute(&body, view); err != nil {
		slog.Error("app binaries: install page render failed", "err", err)
		writeInstallUnavailable(w)
		return
	}
	h := w.Header()
	setInstallHeaders(h)
	h.Set("Content-Type", "text/html; charset=utf-8")
	h.Set("Cache-Control", "no-store")
	h.Set("Content-Length", strconv.Itoa(body.Len()))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body.Bytes())
}

// installAddress is the page's own address: under the configured base when
// there is one (the address the link was handed out as), else as the request
// reached the platform.
func (s *Server) installAddress(r *http.Request, code, appKey, token string) string {
	base := s.InstallBase
	if base == "" {
		scheme := "http"
		if r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
			scheme = "https"
		}
		base = scheme + "://" + r.Host
	}
	return appupdate.InstallURL(base, code, appKey, token)
}

func buildCard(b *appupdate.Binary, token string) installBuild {
	c := installBuild{
		Version: versionText(b), Date: b.CreatedAt.UTC().Format("2 Jan 2006"), Size: sizeText(b.SizeBytes),
		Href: token + "/" + b.Variant + ".apk", FileName: b.FileName(),
	}
	switch b.Variant {
	case appupdate.VariantRelease:
		c.Title, c.Tag, c.Primary = "Regular build", "Most people want this one", true
		c.What = "The app as everyone uses it."
		c.ButtonText = "Download the regular build"
	case appupdate.VariantDebug:
		c.Title = "Debug build"
		c.What = "For testers and developers: it shows extra information for finding problems."
		c.ButtonText = "Download the debug build"
	default:
		c.Title = "Build “" + b.Variant + "”"
		c.What = "A special build. Use it only if you were asked to."
		c.ButtonText = "Download the " + b.Variant + " build"
	}
	return c
}

func versionText(b *appupdate.Binary) string {
	code := strconv.FormatInt(b.VersionCode, 10)
	if b.VersionName == nil || *b.VersionName == "" {
		return code
	}
	return *b.VersionName + " (" + code + ")"
}

func sizeText(n int64) string {
	switch {
	case n >= 1_000_000:
		return strconv.FormatFloat(float64(n)/1_000_000, 'f', 1, 64) + " MB"
	case n >= 1_000:
		return strconv.FormatInt((n+500)/1_000, 10) + " kB"
	}
	return strconv.FormatInt(n, 10) + " bytes"
}

// switchNote says, when the page offers both the regular and the debug build,
// what switching between them takes: they install side by side when their
// packages differ; otherwise, signed differently, one cannot replace the other
// without uninstalling first.
func switchNote(builds []appupdate.Binary) string {
	var rel, dbg *appupdate.Binary
	for i := range builds {
		switch builds[i].Variant {
		case appupdate.VariantRelease:
			rel = &builds[i]
		case appupdate.VariantDebug:
			dbg = &builds[i]
		}
	}
	switch {
	case rel == nil || dbg == nil:
		return ""
	case rel.PackageName != dbg.PackageName:
		return "The regular and debug builds are separate apps: you can have both on the phone."
	case rel.SigningCertSHA256 != dbg.SigningCertSHA256:
		return "The regular and debug builds are signed differently, so one cannot replace the other. " +
			"To switch, uninstall the app first, then install the other build. Uninstalling removes the app’s data from the phone."
	}
	return ""
}

// download streams the newest binary of a variant of the link's channel:
// whole, or one byte range (a phone on a poor connection resumes), verified
// against its SHA-256 as it is read (see copyRangeVerified).
func (s *Server) download(w http.ResponseWriter, r *http.Request) {
	code, appKey, token := chi.URLParam(r, "applicationCode"), chi.URLParam(r, "appKey"), chi.URLParam(r, "token")
	variant, isAPK := strings.CutSuffix(chi.URLParam(r, "file"), ".apk")
	inst, err := s.lookupInstall(r.Context(), code, appKey, token)
	if err != nil {
		slog.Error("app binaries: download read failed", "err", err)
		writeInstallUnavailable(w)
		return
	}
	var b *appupdate.Binary
	if inst != nil && isAPK {
		for i := range inst.Binaries {
			if inst.Binaries[i].Variant == variant {
				b = &inst.Binaries[i]
			}
		}
	}
	if b == nil {
		writeInstallNotFound(w)
		return
	}

	h := w.Header()
	setInstallHeaders(h)
	etag := `"` + b.SHA256 + `"`
	h.Set("ETag", etag)
	h.Set("Accept-Ranges", "bytes")
	// The address is the link's, not the file's: a newer build replaces what
	// it gives. A client may keep the file and revalidate by its hash.
	h.Set("Cache-Control", "private, no-cache")
	h.Set("Last-Modified", b.CreatedAt.UTC().Format(http.TimeFormat))
	if m := r.Header.Get("If-Match"); m != "" && m != "*" && !etagListHas(m, etag) {
		h.Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusPreconditionFailed)
		_, _ = io.WriteString(w, "A newer build is available. Download it again.\n")
		return
	}
	if m := r.Header.Get("If-None-Match"); m != "" && etagListHas(m, etag) {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	start, end, partial, ok := byteRange(r.Header.Get("Range"), r.Header.Get("If-Range"), etag, b.SizeBytes)
	if !ok {
		h.Set("Content-Range", "bytes */"+strconv.FormatInt(b.SizeBytes, 10))
		h.Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusRequestedRangeNotSatisfiable)
		return
	}

	rc, err := s.Store.Open(r.Context(), b.SHA256)
	if err != nil {
		if errors.Is(err, artifact.ErrNotFound) {
			slog.Error("app binaries: a published binary is missing from the store", "binary", b.ID, "sha256", b.SHA256)
		} else {
			slog.Error("app binaries: store open failed", "binary", b.ID, "err", err)
		}
		writeInstallUnavailable(w)
		return
	}
	defer rc.Close()
	h.Set("Content-Type", APKContentType)
	h.Set("Content-Disposition", `attachment; filename="`+b.FileName()+`"`)
	h.Set("Content-Length", strconv.FormatInt(end-start+1, 10))
	status := http.StatusOK
	if partial {
		h.Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, b.SizeBytes))
		status = http.StatusPartialContent
	}
	w.WriteHeader(status)
	if err := copyRangeVerified(w, rc, b.SizeBytes, start, end, b.SHA256); err != nil {
		if errors.Is(err, errFileMismatch) {
			slog.Error("app binaries: a stored binary does not match its SHA-256; its download is cut off", "binary", b.ID, "sha256", b.SHA256)
		} else {
			slog.Warn("app binaries: download ended early", "binary", b.ID, "err", err)
		}
		// Fewer bytes than Content-Length: a failed download, never a complete wrong one.
		panic(http.ErrAbortHandler)
	}
}

// etagListHas reports whether an If-Match / If-None-Match list names etag
// (weak or strong; the binary's tag is its content hash).
func etagListHas(list, etag string) bool {
	for t := range strings.SplitSeq(list, ",") {
		t = strings.TrimPrefix(strings.TrimSpace(t), "W/")
		if t == etag || t == "*" {
			return true
		}
	}
	return false
}

// byteRange reads a Range header for a file of size bytes: the bytes to send
// [start, end] and whether that is a part (206). One range of the forms
// "bytes=a-b", "bytes=a-" and "bytes=-n" is honoured; a header that is absent,
// malformed, asks for several ranges, or carries an If-Range that is not the
// file's tag, is the whole file (RFC 9110 allows ignoring it). ok is false for
// a range that starts past the end (416).
func byteRange(header, ifRange, etag string, size int64) (start, end int64, partial, ok bool) {
	whole := func() (int64, int64, bool, bool) { return 0, size - 1, false, true }
	spec, found := strings.CutPrefix(strings.TrimSpace(header), "bytes=")
	if !found || strings.Contains(spec, ",") || (ifRange != "" && strings.TrimSpace(ifRange) != etag) {
		return whole()
	}
	a, b, found := strings.Cut(strings.TrimSpace(spec), "-")
	if !found {
		return whole()
	}
	if a == "" {
		n, err := strconv.ParseInt(b, 10, 64)
		if err != nil || n <= 0 {
			return whole()
		}
		if n > size {
			n = size
		}
		return size - n, size - 1, true, true
	}
	s, err := strconv.ParseInt(a, 10, 64)
	if err != nil || s < 0 {
		return whole()
	}
	if s >= size {
		return 0, 0, false, false
	}
	e := size - 1
	if b != "" {
		v, err := strconv.ParseInt(b, 10, 64)
		if err != nil || v < s {
			return whole()
		}
		if v < e {
			e = v
		}
	}
	return s, e, true, true
}

// copyRangeVerified writes bytes [start, end] of a file of size bytes to w,
// reading the whole file to hash it, and holds back the last verifyChunk bytes
// it would write until the whole file has been read and its SHA-256 and size
// are wantHex and size. A file that does not match is never sent whole: the
// caller aborts the response short of its Content-Length. For a range that
// ends before the file does, the rest is read (hashed, not sent): a resumed
// download costs a full read from the store, which keeps every answer
// verified without keeping per-chunk hashes.
func copyRangeVerified(w io.Writer, r io.Reader, size, start, end int64, wantHex string) error {
	h := sha256.New()
	buf := make([]byte, verifyChunk)
	pending := make([]byte, 0, 2*verifyChunk)
	var pos int64
	lr := io.LimitReader(r, size+1)
	for {
		k, err := lr.Read(buf)
		if k > 0 {
			chunk := buf[:k]
			h.Write(chunk)
			lo, hi := max(start-pos, 0), min(end+1-pos, int64(k))
			if lo < hi {
				pending = append(pending, chunk[lo:hi]...)
				if over := len(pending) - verifyChunk; over > 0 {
					if _, werr := w.Write(pending[:over]); werr != nil {
						return werr
					}
					pending = append(pending[:0], pending[over:]...)
				}
			}
			pos += int64(k)
		}
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}
	}
	if pos != size || fmt.Sprintf("%x", h.Sum(nil)) != wantHex {
		return errFileMismatch
	}
	_, err := w.Write(pending)
	return err
}
