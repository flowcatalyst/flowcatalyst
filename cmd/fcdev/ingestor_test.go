package main

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ── helpers ────────────────────────────────────────────────────────────

type tarEntry struct {
	name, link string
	typ        byte
	body       string
	mode       int64
}

func makeTarGz(t *testing.T, entries []tarEntry) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, e := range entries {
		typ := e.typ
		if typ == 0 {
			typ = tar.TypeReg
		}
		mode := e.mode
		if mode == 0 {
			mode = 0o644
		}
		h := &tar.Header{Name: e.name, Typeflag: typ, Linkname: e.link, Mode: mode}
		if typ == tar.TypeReg {
			h.Size = int64(len(e.body))
		}
		require.NoError(t, tw.WriteHeader(h))
		if typ == tar.TypeReg {
			_, err := tw.Write([]byte(e.body))
			require.NoError(t, err)
		}
	}
	require.NoError(t, tw.Close())
	require.NoError(t, gz.Close())
	return buf.Bytes()
}

func sum(b []byte) string { s := sha256.Sum256(b); return hex.EncodeToString(s[:]) }

func jreArchive(t *testing.T, mac bool) []byte {
	prefix := "jdk-25.0.5+8-jre/"
	if mac {
		prefix += "Contents/Home/"
	}
	return makeTarGz(t, []tarEntry{
		{name: "jdk-25.0.5+8-jre/", typ: tar.TypeDir},
		{name: prefix + "bin/", typ: tar.TypeDir},
		{name: prefix + "bin/java", body: "#!/bin/sh\n", mode: 0o755},
		{name: prefix + "release", body: "JAVA_VERSION=25.0.5\n"},
	})
}

// fakeAdoptium serves the asset listing and the archive. hits counts requests.
func fakeAdoptium(t *testing.T, archive []byte, checksum string, hits *atomic.Int32) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	var srv *httptest.Server
	mux.HandleFunc("/v3/assets/latest/25/hotspot", func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		q := r.URL.Query()
		assert.Equal(t, "jre", q.Get("image_type"))
		assert.Equal(t, "eclipse", q.Get("vendor"))
		a := adoptiumAsset{ReleaseName: "jdk-25.0.5+8"}
		a.Binary.Package.Name = "OpenJDK25U-jre_" + q.Get("architecture") + "_" + q.Get("os") + "_hotspot_25.0.5_8.tar.gz"
		a.Binary.Package.Link = srv.URL + "/dl/jre.tar.gz"
		a.Binary.Package.Checksum = checksum
		a.Version.OpenJDKVersion = "25.0.5+8-LTS"
		a.Version.Semver = "25.0.5+8.0.LTS"
		_ = json.NewEncoder(w).Encode([]adoptiumAsset{a})
	})
	mux.HandleFunc("/dl/jre.tar.gz", func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		_, _ = w.Write(archive)
	})
	srv = httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func testJRE(t *testing.T, srv *httptest.Server, goos, goarch string) *jreManager {
	m := newJREManager(filepath.Join(t.TempDir(), "fcdev-jre"))
	m.apiURL, m.goos, m.goarch = srv.URL, goos, goarch
	m.client = srv.Client()
	return m
}

// captureLog swaps slog's default for one writing to a buffer.
func captureLog(t *testing.T) *syncBuf {
	b := &syncBuf{}
	old := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(b, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(old) })
	return b
}

// ── JRE ────────────────────────────────────────────────────────────────

func TestAdoptiumPlatform(t *testing.T) {
	for _, c := range []struct{ goos, goarch, os, arch string }{
		{"darwin", "arm64", "mac", "aarch64"},
		{"darwin", "amd64", "mac", "x64"},
		{"linux", "amd64", "linux", "x64"},
		{"windows", "arm64", "windows", "aarch64"},
	} {
		o, a, err := adoptiumPlatform(c.goos, c.goarch)
		require.NoError(t, err)
		assert.Equal(t, c.os, o)
		assert.Equal(t, c.arch, a)
	}
	_, _, err := adoptiumPlatform("plan9", "amd64")
	assert.Error(t, err)
	_, _, err = adoptiumPlatform("linux", "riscv64")
	assert.Error(t, err)
}

func TestJREInstallNotice(t *testing.T) {
	assert.Equal(t, "ingestor runtime needs Java 26; installing it (the Java 25 install stays until you remove it)",
		jreInstallNotice(26, []int{24, 25}))
	assert.Contains(t, jreInstallNotice(25, nil), "installing Eclipse Temurin 25")
	// An install of a HIGHER major than required is not "the older one".
	assert.NotContains(t, jreInstallNotice(25, []int{26}), "stays")
}

func TestJREEnsureInstallsAndCaches(t *testing.T) {
	var hits atomic.Int32
	arc := jreArchive(t, false)
	srv := fakeAdoptium(t, arc, strings.ToUpper(sum(arc)), &hits) // checksum case-insensitive
	m := testJRE(t, srv, "linux", "amd64")
	logs := captureLog(t)
	m.log = slog.Default()

	info, err := m.Ensure(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 25, info.Major)
	assert.Equal(t, "25.0.5+8-LTS", info.Version)
	assert.FileExists(t, info.Java)
	assert.True(t, strings.HasPrefix(info.Java, m.root))

	// The marker records major and full version.
	b, err := os.ReadFile(filepath.Join(m.installDir(25), jreMarkerName))
	require.NoError(t, err)
	var mk jreMarker
	require.NoError(t, json.Unmarshal(b, &mk))
	assert.Equal(t, 25, mk.Major)
	assert.Equal(t, "25.0.5+8-LTS", mk.Version)
	assert.Equal(t, "jdk-25.0.5+8", mk.Release)
	assert.Equal(t, sum(arc), mk.SHA256)

	// Nothing but the install is left in the cache root.
	entries, _ := os.ReadDir(m.root)
	require.Len(t, entries, 1)
	assert.Equal(t, "jre-25", entries[0].Name())

	// A second Ensure uses the install with no network.
	before := hits.Load()
	_, err = m.Ensure(context.Background())
	require.NoError(t, err)
	assert.Equal(t, before, hits.Load())
	assert.Contains(t, logs.String(), "installing Eclipse Temurin 25")
}

func TestJREEnsureMacLayout(t *testing.T) {
	var hits atomic.Int32
	arc := jreArchive(t, true)
	srv := fakeAdoptium(t, arc, sum(arc), &hits)
	m := testJRE(t, srv, "darwin", "arm64")
	info, err := m.Ensure(context.Background())
	require.NoError(t, err)
	assert.True(t, strings.HasSuffix(filepath.ToSlash(info.Home), "Contents/Home"), info.Home)
	assert.FileExists(t, info.Java)
}

func TestJREEnsureChecksumMismatchLeavesNothing(t *testing.T) {
	var hits atomic.Int32
	arc := jreArchive(t, false)
	srv := fakeAdoptium(t, arc, strings.Repeat("0", 64), &hits)
	m := testJRE(t, srv, "linux", "amd64")
	_, err := m.Ensure(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "checksum mismatch")
	entries, _ := os.ReadDir(m.root)
	assert.Empty(t, entries, "no half install, no temp dir")
	assert.Empty(t, m.installedMajors())
}

func TestJREEnsureOfflineFails(t *testing.T) {
	var hits atomic.Int32
	srv := fakeAdoptium(t, nil, "x", &hits)
	m := testJRE(t, srv, "linux", "amd64")
	srv.Close()
	_, err := m.Ensure(context.Background())
	require.Error(t, err)
	entries, _ := os.ReadDir(m.root)
	assert.Empty(t, entries)
}

func TestJREEnsureNoChecksumRefused(t *testing.T) {
	var hits atomic.Int32
	arc := jreArchive(t, false)
	srv := fakeAdoptium(t, arc, "", &hits)
	m := testJRE(t, srv, "linux", "amd64")
	_, err := m.Ensure(context.Background())
	require.Error(t, err)
}

// The requirement was raised: the old install stays, one INFO line says so,
// and the new one is installed and used.
func TestJREEnsureVersionBump(t *testing.T) {
	var hits atomic.Int32
	arc := jreArchive(t, false)
	srv := fakeAdoptium(t, arc, sum(arc), &hits)
	m := testJRE(t, srv, "linux", "amd64")
	m.major = 25
	// An older install, as an earlier fcdev left it.
	old := m.installDir(24)
	require.NoError(t, os.MkdirAll(filepath.Join(old, "jre", "bin"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(old, "jre", "bin", "java"), []byte("x"), 0o755))
	mk, _ := json.Marshal(jreMarker{Major: 24, Version: "24.0.1", JavaHome: "jre"})
	require.NoError(t, os.WriteFile(filepath.Join(old, jreMarkerName), mk, 0o644))
	require.Equal(t, []int{24}, m.installedMajors())

	logs := captureLog(t)
	m.log = slog.Default()
	info, err := m.Ensure(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 25, info.Major)
	assert.Equal(t, 1, strings.Count(logs.String(), "ingestor runtime needs Java 25; installing it (the Java 24 install stays until you remove it)"))
	assert.Equal(t, []int{24, 25}, m.installedMajors(), "the old install is never deleted")
	assert.FileExists(t, filepath.Join(old, jreMarkerName))
}

func TestJREBrokenInstallIsReplaced(t *testing.T) {
	var hits atomic.Int32
	arc := jreArchive(t, false)
	srv := fakeAdoptium(t, arc, sum(arc), &hits)
	m := testJRE(t, srv, "linux", "amd64")
	require.NoError(t, os.MkdirAll(m.installDir(25), 0o755)) // no marker
	_, err := m.Ensure(context.Background())
	require.NoError(t, err)
}

// ── safe extraction ────────────────────────────────────────────────────

func extractBytes(t *testing.T, name string, data []byte) (string, error) {
	t.Helper()
	d := t.TempDir()
	arc := filepath.Join(d, name)
	require.NoError(t, os.WriteFile(arc, data, 0o644))
	dest := filepath.Join(d, "out")
	return dest, extractArchive(arc, dest)
}

func TestExtractRejectsUnsafeTar(t *testing.T) {
	cases := map[string][]tarEntry{
		"parent traversal":    {{name: "../evil", body: "x"}},
		"nested traversal":    {{name: "a/../../evil", body: "x"}},
		"absolute":            {{name: "/etc/evil", body: "x"}},
		"absolute symlink":    {{name: "l", typ: tar.TypeSymlink, link: "/etc"}},
		"escaping symlink":    {{name: "d/l", typ: tar.TypeSymlink, link: "../../x"}},
		"escaping hardlink":   {{name: "h", typ: tar.TypeLink, link: "../outside"}},
		"device":              {{name: "dev", typ: tar.TypeChar}},
		"write through chain": {{name: "d/", typ: tar.TypeDir}, {name: "d/l1", typ: tar.TypeSymlink, link: ".."}, {name: "d/l2", typ: tar.TypeSymlink, link: "l1/.."}, {name: "d/l2/evil", body: "x"}},
	}
	for name, entries := range cases {
		t.Run(name, func(t *testing.T) {
			dest, err := extractBytes(t, "a.tar.gz", makeTarGz(t, entries))
			require.Error(t, err)
			_, statErr := os.Stat(filepath.Join(filepath.Dir(dest), "evil"))
			assert.True(t, os.IsNotExist(statErr))
			_, statErr = os.Stat(filepath.Join(dest, "..", "..", "evil"))
			assert.True(t, os.IsNotExist(statErr))
		})
	}
}

func TestExtractAcceptsInternalSymlink(t *testing.T) {
	dest, err := extractBytes(t, "a.tar.gz", makeTarGz(t, []tarEntry{
		{name: "top/", typ: tar.TypeDir},
		{name: "top/real", body: "hello"},
		{name: "top/link", typ: tar.TypeSymlink, link: "real"},
	}))
	require.NoError(t, err)
	b, err := os.ReadFile(filepath.Join(dest, "top", "link"))
	require.NoError(t, err)
	assert.Equal(t, "hello", string(b))
}

func TestExtractZip(t *testing.T) {
	build := func(name string) []byte {
		var buf bytes.Buffer
		zw := zip.NewWriter(&buf)
		w, err := zw.Create(name)
		require.NoError(t, err)
		_, _ = w.Write([]byte("x"))
		require.NoError(t, zw.Close())
		return buf.Bytes()
	}
	dest, err := extractBytes(t, "a.zip", build("jdk/bin/java.exe"))
	require.NoError(t, err)
	assert.FileExists(t, filepath.Join(dest, "jdk", "bin", "java.exe"))
	_, err = extractBytes(t, "a.zip", build("../evil"))
	assert.Error(t, err)
	_, err = extractBytes(t, "a.zip", build(`..\evil`))
	assert.Error(t, err)
}

// ── jar resolution ─────────────────────────────────────────────────────

func touchJar(t *testing.T, path string, age time.Duration) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte("jar"), 0o644))
	ts := time.Now().Add(-age)
	require.NoError(t, os.Chtimes(path, ts, ts))
}

func checkout(t *testing.T) string {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "ingestor"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "ingestor", "pom.xml"), []byte("<project/>"), 0o644))
	return root
}

func TestResolveJarOrder(t *testing.T) {
	ctx := context.Background()
	root := checkout(t)
	sub := filepath.Join(root, "cmd", "fcdev")
	require.NoError(t, os.MkdirAll(sub, 0o755))
	cache := t.TempDir()

	// Nothing anywhere and nothing released.
	_, _, err := resolveIngestorJar(ctx, "", []string{sub}, ingestorReleaseSpec{}, cache, nil)
	require.ErrorIs(t, err, errNoIngestorJar)

	// A local build, the newest one.
	oldJar := filepath.Join(root, "ingestor", "ingestor-runtime", "target", "ingestor-runtime-0.1.0-all.jar")
	newJar := filepath.Join(root, "ingestor", "ingestor-runtime", "target", "ingestor-runtime-0.2.0-all.jar")
	touchJar(t, oldJar, time.Hour)
	touchJar(t, newJar, time.Minute)
	touchJar(t, filepath.Join(root, "ingestor", "ingestor-runtime", "target", "ingestor-runtime-0.3.0.jar"), 0) // thin jar ignored
	jar, src, err := resolveIngestorJar(ctx, "", []string{sub}, ingestorReleaseSpec{}, cache, nil)
	require.NoError(t, err)
	assert.Equal(t, newJar, jar)
	assert.Equal(t, jarLocal, src)

	// A checkout without a build is not a source.
	assert.Equal(t, "", localIngestorJar([]string{t.TempDir()}))

	// The explicit path wins over a local build; a missing one is an error.
	explicit := filepath.Join(t.TempDir(), "mine.jar")
	touchJar(t, explicit, 0)
	jar, src, err = resolveIngestorJar(ctx, explicit, []string{sub}, ingestorReleaseSpec{}, cache, nil)
	require.NoError(t, err)
	assert.Equal(t, explicit, jar)
	assert.Equal(t, jarExplicit, src)
	_, _, err = resolveIngestorJar(ctx, filepath.Join(t.TempDir(), "missing.jar"), []string{sub}, ingestorReleaseSpec{}, cache, nil)
	assert.Error(t, err)
}

func TestResolveJarReleaseDownload(t *testing.T) {
	body := []byte("the released jar")
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		assert.Equal(t, "/releases/1.2.3/ingestor.jar", r.URL.Path)
		_, _ = w.Write(body)
	}))
	defer srv.Close()
	cache := t.TempDir()
	rel := ingestorReleaseSpec{Version: "1.2.3", URLTemplate: srv.URL + "/releases/{version}/ingestor.jar", SHA256: sum(body)}
	ctx := context.Background()

	jar, src, err := resolveIngestorJar(ctx, "", nil, rel, cache, srv.Client())
	require.NoError(t, err)
	assert.Equal(t, jarRelease, src)
	got, _ := os.ReadFile(jar)
	assert.Equal(t, body, got)
	// Cached: no second download.
	_, _, err = resolveIngestorJar(ctx, "", nil, rel, cache, srv.Client())
	require.NoError(t, err)
	assert.EqualValues(t, 1, hits.Load())

	// A wrong checksum is refused and leaves nothing.
	cache2 := t.TempDir()
	rel.SHA256 = strings.Repeat("a", 64)
	_, _, err = resolveIngestorJar(ctx, "", nil, rel, cache2, srv.Client())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "checksum mismatch")
	var left []string
	_ = filepath.WalkDir(cache2, func(p string, d os.DirEntry, _ error) error {
		if d != nil && !d.IsDir() {
			left = append(left, p)
		}
		return nil
	})
	assert.Empty(t, left)
}

// ── env and logging ────────────────────────────────────────────────────

func TestIngestorEnv(t *testing.T) {
	cfg := ingestorConfig{Port: 18091, APIPort: 18080, ExtensionsDir: "/ext", Creds: routerCredentials{ClientID: "fcdev-ingestor", Secret: "s3cr3t-value"}}
	parent := []string{
		"PATH=/usr/bin", "HOME=/home/x", "FC_APP_KEY=leak", "FLOWCATALYST_APP_KEY=leak2",
		"FLOWCATALYST_CLIENT_SECRET=other", "FLOWCATALYST_BASE_URL=http://prod", "INGEST_SECRET_ACME__X=pw", "HTTP_PORT=1", "APP_ENV=production",
	}
	env := ingestorEnv(parent, cfg, jreInfo{Home: "/jre"})
	m := map[string]string{}
	for _, kv := range env {
		k, v, _ := strings.Cut(kv, "=")
		m[k] = v
	}
	assert.Equal(t, "http://localhost:18080", m["FLOWCATALYST_BASE_URL"])
	assert.Equal(t, "fcdev-ingestor", m["FLOWCATALYST_CLIENT_ID"])
	assert.Equal(t, "s3cr3t-value", m["FLOWCATALYST_CLIENT_SECRET"])
	assert.Equal(t, "fcdev-18080", m["INGEST_RUNTIME_ID"])
	assert.Equal(t, strconv.Itoa(os.Getpid()), m["INGEST_PARENT_PID"], "the runtime ends itself once fcdev is gone")
	assert.Equal(t, "18091", m["HTTP_PORT"])
	assert.Equal(t, "local", m["APP_ENV"])
	assert.Equal(t, "/ext", m["INGEST_EXTENSIONS_DIR"])
	assert.Equal(t, "/jre", m["JAVA_HOME"])
	assert.Equal(t, "pw", m["INGEST_SECRET_ACME__X"], "the developer's own INGEST_* pass through")
	assert.Equal(t, "PT15S", m["INGEST_POLL_INTERVAL"])
	assert.NotContains(t, m, "FC_APP_KEY")
	assert.NotContains(t, m, "FLOWCATALYST_APP_KEY")

	// The shell's extension settings never reach the runtime: fcdev's folder does.
	env = ingestorEnv([]string{"INGEST_EXTENSIONS_DIR=/stale/*.jar", "INGEST_EXTENSIONS_MANIFEST=s3://b/manifest.json"}, cfg, jreInfo{})
	assert.Contains(t, env, "INGEST_EXTENSIONS_DIR=/ext")
	for _, kv := range env {
		assert.False(t, strings.HasPrefix(kv, "INGEST_EXTENSIONS_MANIFEST="))
	}
	env = ingestorEnv(parent, cfg, jreInfo{Home: "/jre"})

	// Unset extensions dir: not passed.
	cfg.ExtensionsDir = ""
	for _, kv := range ingestorEnv(nil, cfg, jreInfo{}) {
		assert.False(t, strings.HasPrefix(kv, "INGEST_EXTENSIONS_DIR="))
	}

	red := strings.Join(redactEnv(env), "\n")
	assert.NotContains(t, red, "s3cr3t-value")
	assert.NotContains(t, red, "=pw")
	assert.Contains(t, red, "FLOWCATALYST_CLIENT_ID=fcdev-ingestor")
}

func TestExtensionJars(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "ingest-extension-mix.jar"), []byte("x"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("x"), 0o600))
	jars, err := extensionJars(dir)
	require.NoError(t, err)
	assert.Equal(t, []string{"ingest-extension-mix.jar"}, jars)

	_, err = extensionJars(filepath.Join(dir, "ingest-extension-*.jar"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "FC_INGESTOR_EXTENSIONS_DIR")
}

func TestIngestorExitCause(t *testing.T) {
	log := filepath.Join(t.TempDir(), "ingestor.log")
	require.NoError(t, os.WriteFile(log, []byte(`[main] INFO started
Exception in thread "main" io.flowcatalyst.framework.core.StartupException: Application failed to start: 1 problem
  - module 'ingest-runtime': provider failed: IllegalStateException: an old problem

--- fcdev restarted the ingestor runtime at 2026-10-29T16:15:20Z ---
[main] INFO org.apache.camel - starting
Exception in thread "main" io.flowcatalyst.framework.core.StartupException: Application failed to start: 1 problem
  - module 'ingest-runtime': provider failed: IllegalStateException: the extensions directory /x is not a directory
	at io.flowcatalyst.framework.core.AppBuilder.configureAndBuild(AppBuilder.java:489)
`), 0o600))
	assert.Equal(t, "StartupException: Application failed to start: 1 problem; module 'ingest-runtime': provider failed: IllegalStateException: the extensions directory /x is not a directory",
		ingestorExitCause(log), "the latest run's failure, not an earlier one")

	require.NoError(t, os.WriteFile(log, []byte("java.lang.OutOfMemoryError: Java heap space\nException in thread \"x\" java.lang.RuntimeException: boom\n\tat a.b(C.java:1)\n"), 0o600))
	assert.Equal(t, `Exception in thread "x" java.lang.RuntimeException: boom`, ingestorExitCause(log))
	assert.Empty(t, ingestorExitCause(filepath.Join(t.TempDir(), "missing.log")))
}

func TestIngestorExtensionsDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("FC_INGESTOR_EXTENSIONS_DIR", "")
	want := filepath.Join(home, ".flowcatalyst", "ingestor-extensions")
	assert.Equal(t, want, ingestorExtensionsDir(), "the folder an extension's build drops its jar in")
	assert.DirExists(t, want)

	t.Setenv("FC_INGESTOR_EXTENSIONS_DIR", "/elsewhere")
	assert.Equal(t, "/elsewhere", ingestorExtensionsDir())
}

func TestApplicationYAMLAndConfigDir(t *testing.T) {
	cfg := ingestorConfig{Port: 18091, APIPort: 18080, StateDir: filepath.Join(t.TempDir(), "state")}
	require.NoError(t, writeIngestorConfig(cfg))
	b, err := os.ReadFile(filepath.Join(cfg.StateDir, "config", "application.yaml"))
	require.NoError(t, err)
	assert.Contains(t, string(b), "port: 18091")
	assert.Contains(t, string(b), "base-url: http://localhost:18080")
	assert.NotContains(t, string(b), "secret")
}

func TestIngestorStateDirPerInstance(t *testing.T) {
	t.Setenv("FC_INGESTOR_STATE_DIR", "")
	assert.NotEqual(t, ingestorStateDir(8080), ingestorStateDir(18080))
	t.Setenv("FC_INGESTOR_STATE_DIR", "/x/y")
	assert.Equal(t, "/x/y", ingestorStateDir(8080))
}

func TestJRERootDirOverride(t *testing.T) {
	t.Setenv("FC_INGESTOR_JRE_DIR", "/custom")
	assert.Equal(t, "/custom", jreRootDir())
	t.Setenv("FC_INGESTOR_JRE_DIR", "")
	assert.True(t, strings.HasSuffix(filepath.ToSlash(jreRootDir()), "flowcatalyst/fcdev-jre"))
}

func TestIngestorJavaMajorMatchesPom(t *testing.T) {
	b, err := os.ReadFile("../../ingestor/pom.xml")
	if err != nil {
		t.Skip("not in a checkout")
	}
	assert.Contains(t, string(b), fmt.Sprintf("<maven.compiler.release>%d</maven.compiler.release>", ingestorJavaMajor))
}

func TestFlagsDefaults(t *testing.T) {
	cmd := newStartCmd()
	o := optsFromFlags(cmd)
	assert.Equal(t, defaultIngestorPort, o.IngestorPort)
	assert.True(t, o.IngestorEnabled)
}

// ── supervisor ─────────────────────────────────────────────────────────

type fakeProc struct {
	pid                int
	done               chan struct{}
	once               sync.Once
	terminated, killed atomic.Bool
	ignoreTerm         bool
}

func newFakeProc(pid int) *fakeProc { return &fakeProc{pid: pid, done: make(chan struct{})} }
func (f *fakeProc) exit()           { f.once.Do(func() { close(f.done) }) }
func (f *fakeProc) Pid() int        { return f.pid }
func (f *fakeProc) Done() <-chan struct{} {
	return f.done
}

func (f *fakeProc) Terminate() error {
	f.terminated.Store(true)
	if !f.ignoreTerm {
		f.exit()
	}
	return nil
}
func (f *fakeProc) Kill() error { f.killed.Store(true); f.exit(); return nil }

func testSupervisor(launch func() (ingProc, error)) *ingSupervisor {
	return &ingSupervisor{
		launch: launch, maxRestarts: 3, backoff: func(int) time.Duration { return time.Millisecond },
		stableAfter: time.Hour, grace: 50 * time.Millisecond, log: slog.Default(),
	}
}

func TestSupervisorRestartCap(t *testing.T) {
	logs := captureLog(t)
	var launches atomic.Int32
	s := testSupervisor(func() (ingProc, error) {
		launches.Add(1)
		p := newFakeProc(int(launches.Load()))
		p.exit() // dies at once
		return p, nil
	})
	s.log = slog.Default()
	err := s.run(context.Background())
	require.ErrorIs(t, err, errIngestorGaveUp)
	assert.EqualValues(t, 4, launches.Load(), "the first start plus 3 restarts")
	assert.Equal(t, 3, strings.Count(logs.String(), "restarting the ingestor runtime"))
}

func TestSupervisorStableRunResetsCount(t *testing.T) {
	var launches atomic.Int32
	clock := time.Unix(0, 0)
	s := testSupervisor(func() (ingProc, error) {
		n := launches.Add(1)
		p := newFakeProc(int(n))
		p.exit()
		return p, nil
	})
	s.stableAfter = 10 * time.Second
	// Each process "ran" a minute: every exit is a stable one, so the cap is never reached.
	s.now = func() time.Time { clock = clock.Add(time.Minute); return clock }
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		for launches.Load() < 8 {
			time.Sleep(time.Millisecond)
		}
		cancel()
	}()
	require.NoError(t, s.run(ctx))
	assert.GreaterOrEqual(t, launches.Load(), int32(8))
}

func TestSupervisorLaunchErrorsCount(t *testing.T) {
	var n atomic.Int32
	s := testSupervisor(func() (ingProc, error) { n.Add(1); return nil, fmt.Errorf("no java") })
	require.ErrorIs(t, s.run(context.Background()), errIngestorGaveUp)
	assert.EqualValues(t, 4, n.Load())
}

func TestSupervisorStopsChildOnCancel(t *testing.T) {
	p := newFakeProc(7)
	s := testSupervisor(func() (ingProc, error) { return p, nil })
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error)
	go func() { done <- s.run(ctx) }()
	time.Sleep(20 * time.Millisecond)
	cancel()
	require.NoError(t, <-done)
	assert.True(t, p.terminated.Load())
	assert.False(t, p.killed.Load(), "a child that exits on SIGTERM is not killed")
}

func TestSupervisorKillsAfterGrace(t *testing.T) {
	p := newFakeProc(8)
	p.ignoreTerm = true
	s := testSupervisor(func() (ingProc, error) { return p, nil })
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error)
	go func() { done <- s.run(ctx) }()
	time.Sleep(20 * time.Millisecond)
	cancel()
	require.NoError(t, <-done)
	assert.True(t, p.terminated.Load())
	assert.True(t, p.killed.Load())
}

func TestDefaultBackoffGrowsAndCaps(t *testing.T) {
	assert.Equal(t, 2*time.Second, defaultIngestorBackoff(1))
	assert.Equal(t, 4*time.Second, defaultIngestorBackoff(2))
	assert.LessOrEqual(t, defaultIngestorBackoff(50), time.Minute)
}
