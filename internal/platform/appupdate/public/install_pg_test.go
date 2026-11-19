//go:build integration

package public_test

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/appupdate"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/appupdate/apk/apktest"
	aut "github.com/flowcatalyst/flowcatalyst-go/internal/platform/appupdate/appupdatetest"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/appupdate/public"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/ratelimit"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
)

const rv = "3f2a9c0d1b8e7f6a5c4d3e2f1a0b9c8d7e6f5a4b"

// installWorld is a world with a link on driver-app/production; release and
// debug say which variants are published first.
func installWorld(t *testing.T, release, debug bool) (*aut.World, aut.Link) {
	t.Helper()
	w := aut.NewWorld(t, aut.Options{InstallBase: "https://platform.example.com"})
	_, err := testpg.Pool(t).Exec(context.Background(), `UPDATE app_applications SET name = 'Acme & Co Driver' WHERE id = $1`, w.AppAID)
	require.NoError(t, err)
	if release {
		w.PublishBinary("sa-a", w.AppA, "driver-app", "production", "release",
			aut.APK(t, w, "release", w.AppA, "driver-app", "production", rv, 140, "1.4.0", false))
	}
	if debug {
		w.PublishBinary("sa-a", w.AppA, "driver-app", "production", "debug",
			aut.APK(t, w, "debug", w.AppA, "driver-app", "production", rv, 141, "1.4.1", true))
	}
	return w, w.CreateLink(w.AppA, "driver-app", "production")
}

func pagePath(w *aut.World, token string) string {
	return "/app-updates/" + w.AppA + "/driver-app/install/" + token
}

// saveRendering keeps a page for a person to look at when
// FC_TEST_INSTALL_PAGE_DIR names a directory.
func saveRendering(t *testing.T, name string, body []byte) {
	if dir := os.Getenv("FC_TEST_INSTALL_PAGE_DIR"); dir != "" {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), body, 0o600))
	}
}

func checkInstallHeaders(t *testing.T, h http.Header) {
	t.Helper()
	assert.Equal(t, public.InstallCSP, h.Get("Content-Security-Policy"))
	assert.Equal(t, "no-referrer", h.Get("Referrer-Policy"))
	assert.Equal(t, "noindex, nofollow", h.Get("X-Robots-Tag"))
	assert.Equal(t, "DENY", h.Get("X-Frame-Options"))
	assert.Equal(t, "nosniff", h.Get("X-Content-Type-Options"))
}

var styleBlock = regexp.MustCompile(`(?s)<style>(.*?)</style>`)

// One variant: the page names the app as the application registered it, the
// build with its version, date and size and a download, the steps, and a QR
// code of its own address. Nothing is fetched from elsewhere, nothing runs.
func TestInstallPage_OneVariant(t *testing.T) {
	w, l := installWorld(t, true, false)
	resp, body := w.Get(pagePath(w, l.Token), nil)
	require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
	saveRendering(t, "install-one-variant.html", body)
	page := string(body)
	checkInstallHeaders(t, resp.Header)
	assert.Equal(t, "no-store", resp.Header.Get("Cache-Control"))
	assert.Equal(t, "text/html; charset=utf-8", resp.Header.Get("Content-Type"))

	assert.Contains(t, page, "<title>Install Acme &amp; Co Driver</title>", "the name the application is registered with, escaped")
	assert.Contains(t, page, "<h1>Install Acme &amp; Co Driver</h1>")
	assert.Contains(t, page, "Regular build")
	assert.Contains(t, page, "Most people want this one")
	assert.Contains(t, page, "Version 1.4.0 (140)")
	assert.Contains(t, page, `href="`+l.Token+`/release.apk"`)
	assert.Contains(t, page, `download="driver-app-1.4.0-release.apk"`)
	assert.Contains(t, page, "Install unknown apps")
	assert.NotContains(t, page, "Debug build")
	assert.NotContains(t, page, "uninstall the app first", "no switching note with one build")
	assert.Regexp(t, `· [0-9.]+ kB`, page)

	assert.Equal(t, "https://platform.example.com"+pagePath(w, l.Token), public.DecodeQRInPage(t, page))
	for _, banned := range []string{"<script", "<img", "<link", "<iframe", "<form", "src=", `href="http`, "url(", "@import", "style=", "onload", "onclick"} {
		assert.NotContains(t, strings.ToLower(page), banned)
	}
	m := styleBlock.FindStringSubmatch(page)
	require.NotNil(t, m)
	sum := sha256.Sum256([]byte(m[1]))
	assert.Contains(t, resp.Header.Get("Content-Security-Policy"), "'sha256-"+base64.StdEncoding.EncodeToString(sum[:])+"'",
		"the served stylesheet is the one the policy admits")
}

// Two variants: the regular build first and marked, the debug build named
// for what it is, and a note on switching (they are signed differently).
func TestInstallPage_TwoVariants(t *testing.T) {
	w, l := installWorld(t, true, true)
	resp, body := w.Get(pagePath(w, l.Token), nil)
	require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
	saveRendering(t, "install-two-variants.html", body)
	page := string(body)
	rel, dbg := strings.Index(page, "Regular build"), strings.Index(page, "Debug build")
	require.Positive(t, rel)
	require.Positive(t, dbg)
	assert.Less(t, rel, dbg, "the regular build first")
	assert.Equal(t, 1, strings.Count(page, "Most people want this one"))
	assert.Contains(t, page, "For testers and developers")
	assert.Contains(t, page, `href="`+l.Token+`/debug.apk"`)
	assert.Contains(t, page, "Version 1.4.1 (141)")
	assert.Contains(t, page, "signed differently")
	assert.Contains(t, page, "uninstall the app first")
}

// Every way of not holding a working link gets the same answer, byte for
// byte: a wrong, replaced or malformed token, an unknown application, an
// address naming another app key, an inactive application, and a link whose
// channel has no binary.
func TestInstallPage_EveryNoIsTheSame(t *testing.T) {
	w, l := installWorld(t, true, false)
	replaced := l.Token
	l = w.ReplaceLink(l.ID)
	empty := w.CreateLink(w.AppA, "driver-app", "staging")
	other := aut.NewWorld(t, aut.Options{})
	otherLink := other.CreateLink(other.AppA, "driver-app", "production")
	other.PublishBinary("sa-a", other.AppA, "driver-app", "production", "release",
		aut.APK(t, other, "release", other.AppA, "driver-app", "production", rv, 1, "1.0.0", false))
	_, err := testpg.Pool(t).Exec(context.Background(), `UPDATE app_applications SET active = false WHERE id = $1`, other.AppAID)
	require.NoError(t, err)
	wrong, _, err := appupdate.NewInstallToken()
	require.NoError(t, err)

	cases := map[string]string{
		"wrong token":          pagePath(w, wrong),
		"replaced token":       pagePath(w, replaced),
		"malformed token":      pagePath(w, "not-a-token"),
		"long token":           pagePath(w, strings.Repeat("A", 2000)),
		"unknown application":  "/app-updates/nobody-here/driver-app/install/" + l.Token,
		"malformed app code":   "/app-updates/NOPE!/driver-app/install/" + l.Token,
		"another app key":      "/app-updates/" + w.AppA + "/other-app/install/" + l.Token,
		"another application":  "/app-updates/" + w.AppB + "/driver-app/install/" + l.Token,
		"no binary yet":        pagePath(w, empty.Token),
		"inactive application": "/app-updates/" + other.AppA + "/driver-app/install/" + otherLink.Token,
		"download, wrong":      pagePath(w, wrong) + "/release.apk",
		"download, no variant": pagePath(w, l.Token) + "/debug.apk",
		"download, not apk":    pagePath(w, l.Token) + "/release.zip",
	}
	var first []byte
	var firstHeader http.Header
	for name, path := range cases {
		srv := w
		if strings.Contains(path, other.AppA) {
			srv = other
		}
		resp, body := srv.Get(path, nil)
		require.Equal(t, http.StatusNotFound, resp.StatusCode, "%s: %s", name, body)
		checkInstallHeaders(t, resp.Header)
		h := resp.Header.Clone()
		h.Del("Date")
		if first == nil {
			first, firstHeader = body, h
			saveRendering(t, "install-not-found.html", body)
			continue
		}
		assert.Equal(t, string(first), string(body), name)
		assert.Equal(t, firstHeader, h, name)
	}
	resp, _ := w.Get(pagePath(w, l.Token), nil)
	assert.Equal(t, http.StatusOK, resp.StatusCode, "the current token works")
}

func publishedFile(t *testing.T, w *aut.World, filler int) ([]byte, aut.Link) {
	t.Helper()
	b, err := apktest.Build(apktest.Options{
		Package: "com.example.driver", VersionCode: 7, VersionName: "2.0.0", Fingerprint: rv,
		UpdateURL: w.Server.URL + "/app-updates/" + w.AppA + "/driver-app/manifest", Channel: "production",
		Filler: filler, Signer: aut.Signer(t, "release"),
	})
	require.NoError(t, err)
	w.PublishBinary("sa-a", w.AppA, "driver-app", "production", "release", b)
	return b, w.CreateLink(w.AppA, "driver-app", "production")
}

// The download: the whole file, a range, a resume, revalidation, and the
// headers a person's phone needs.
func TestInstallDownload(t *testing.T) {
	w := aut.NewWorld(t, aut.Options{})
	b, l := publishedFile(t, w, 300<<10)
	path := pagePath(w, l.Token) + "/release.apk"
	etag := `"` + aut.SHA256Hex(b) + `"`

	resp, body := w.Get(path, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, b, body)
	checkInstallHeaders(t, resp.Header)
	assert.Equal(t, public.APKContentType, resp.Header.Get("Content-Type"))
	assert.Equal(t, `attachment; filename="driver-app-2.0.0-release.apk"`, resp.Header.Get("Content-Disposition"))
	assert.Equal(t, strconv.Itoa(len(b)), resp.Header.Get("Content-Length"))
	assert.Equal(t, "bytes", resp.Header.Get("Accept-Ranges"))
	assert.Equal(t, etag, resp.Header.Get("ETag"))
	assert.Equal(t, "private, no-cache", resp.Header.Get("Cache-Control"))

	resp, body = w.Get(path, map[string]string{"Range": "bytes=1000-1999"})
	require.Equal(t, http.StatusPartialContent, resp.StatusCode)
	assert.Equal(t, b[1000:2000], body)
	assert.Equal(t, fmt.Sprintf("bytes 1000-1999/%d", len(b)), resp.Header.Get("Content-Range"))
	assert.Equal(t, "1000", resp.Header.Get("Content-Length"))

	// A phone that got half the file resumes from there and ends with the whole of it.
	half := len(b) / 2
	resp, body = w.Get(path, map[string]string{"Range": fmt.Sprintf("bytes=%d-", half), "If-Range": etag})
	require.Equal(t, http.StatusPartialContent, resp.StatusCode)
	assert.Equal(t, b, append(b[:half:half], body...))

	resp, body = w.Get(path, map[string]string{"Range": "bytes=0-9", "If-Range": `"an older build"`})
	require.Equal(t, http.StatusOK, resp.StatusCode, "a resume of another build gets the whole new one")
	assert.Equal(t, b, body)
	resp, _ = w.Get(path, map[string]string{"If-Match": `"an older build"`})
	assert.Equal(t, http.StatusPreconditionFailed, resp.StatusCode)
	resp, _ = w.Get(path, map[string]string{"Range": fmt.Sprintf("bytes=%d-", len(b))})
	assert.Equal(t, http.StatusRequestedRangeNotSatisfiable, resp.StatusCode)
	assert.Equal(t, fmt.Sprintf("bytes */%d", len(b)), resp.Header.Get("Content-Range"))
	resp, body = w.Get(path, map[string]string{"If-None-Match": etag})
	assert.Equal(t, http.StatusNotModified, resp.StatusCode)
	assert.Empty(t, body)
}

// A stored binary that does not match its hash is never sent whole: the
// response ends short of its Content-Length, whole or by range.
func TestInstallDownload_CorruptedFileIsCutOff(t *testing.T) {
	w := aut.NewWorld(t, aut.Options{})
	b, l := publishedFile(t, w, 200<<10)
	w.CorruptStored(aut.SHA256Hex(b), int64(len(b)-10))
	path := pagePath(w, l.Token) + "/release.apk"
	for _, rng := range []string{"", "bytes=0-99", "bytes=1000-"} {
		req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, w.Server.URL+path, nil)
		require.NoError(t, err)
		if rng != "" {
			req.Header.Set("Range", rng)
		}
		resp, err := w.Server.Client().Do(req)
		if err != nil {
			continue // cut off before the headers left: nothing was received
		}
		got, err := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		require.Error(t, err, "%q: the body must end early, not complete", rng)
		assert.Less(t, int64(len(got)), resp.ContentLength, rng)
	}
}

// The install routes are behind their own per-address governors.
func TestInstallRoutes_AreGoverned(t *testing.T) {
	w, l := installWorld(t, true, false)
	r := chi.NewMux()
	gov := func() []func(http.Handler) http.Handler {
		g := ratelimit.NewGovernor(ratelimit.GovernorConfig{PerMinute: 1, Burst: 1})
		return []func(http.Handler) http.Handler{ratelimit.GovernorMiddleware(g, "Too many requests")}
	}
	(&public.Server{Repo: w.Repo, Store: w.Store, Keys: w.Keys, Links: w.Links}).Mount(r, public.Middlewares{Install: gov(), Download: gov()})
	for _, path := range []string{pagePath(w, l.Token), pagePath(w, l.Token) + "/release.apk"} {
		first := httpDo(t, r, path)
		assert.Equal(t, http.StatusOK, first, path)
		assert.Equal(t, http.StatusTooManyRequests, httpDo(t, r, path), path)
	}
}

func httpDo(t *testing.T, h http.Handler, path string) int {
	t.Helper()
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, path, nil)
	req.RemoteAddr = "192.0.2.10:40000"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec.Code
}
