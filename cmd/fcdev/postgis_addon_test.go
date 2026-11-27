package main

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ── helpers ────────────────────────────────────────────────────────────

const testAddonTag = "postgis-addon/v3.6.4-pg18-2"

// fakeEmbeddedTree is a stand-in for zonky's unpacked tree: pg_ctl, the
// bootstrap file that names the major, and two of zonky's own files the
// add-on must leave alone.
func fakeEmbeddedTree(t *testing.T, major string) string {
	t.Helper()
	tree := filepath.Join(t.TempDir(), "embedded-pg", "bin")
	files := map[string]string{
		"bin/pg_ctl":                                  "#!/bin/sh\n",
		"share/postgresql/postgres.bki":               "# PostgreSQL " + major + "\ncreate pg_proc 1255 bootstrap\n",
		"lib/postgresql/plpgsql.so":                   "zonky plpgsql",
		"share/postgresql/extension/plpgsql.control":  "zonky plpgsql control",
		"share/postgresql/extension/plpgsql--1.0.sql": "zonky plpgsql sql",
	}
	for name, body := range files {
		p := filepath.Join(tree, filepath.FromSlash(name))
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
		require.NoError(t, os.WriteFile(p, []byte(body), 0o755))
	}
	return tree
}

func addonManifestJSON(tag, goos, goarch string) string {
	postgis, major, _ := parseAddonTag(tag)
	return fmt.Sprintf(`{"postgis":%q,"pgMajor":%q,"goos":%q,"goarch":%q,"tag":%q}`, postgis, major, goos, goarch, tag)
}

// addonTarball is a well-formed add-on for goos/goarch (linux layout unless
// darwin), with extra entries inserted before the manifest.
func addonTarball(t *testing.T, tag, goos, goarch, version string, extra ...tarEntry) []byte {
	t.Helper()
	entries := []tarEntry{
		{name: "lib/", typ: tar.TypeDir, mode: 0o755},
		{name: "lib/postgresql/", typ: tar.TypeDir, mode: 0o755},
		{name: "lib/postgresql/" + addonModuleFile(goos), body: "module " + version, mode: 0o755},
		{name: "share/", typ: tar.TypeDir, mode: 0o755},
		{name: "share/postgresql/extension/postgis.control", body: "default_version = '" + version + "'"},
		{name: "share/postgresql/extension/postgis--" + version + ".sql", body: "-- install " + version},
		{name: "postgis/share/licenses/postgis/COPYING", body: "GPL"},
	}
	if goos == "darwin" {
		entries = append(entries,
			tarEntry{name: "postgis/lib/libgeos." + version + ".dylib", body: "geos " + version, mode: 0o755},
			tarEntry{name: "postgis/share/proj/proj.db", body: "proj.db " + version})
	} else {
		entries = append(entries, tarEntry{name: "lib/share/proj/proj.db", body: "proj.db " + version})
	}
	entries = append(entries, extra...)
	entries = append(entries, tarEntry{name: postgisAddonManifestName, body: addonManifestJSON(tag, goos, goarch)})
	return makeTarGz(t, entries)
}

// addonServer serves body at /<tag>/<asset> over TLS and counts requests.
type addonServer struct {
	*httptest.Server
	hits atomic.Int32
}

func newAddonServer(t *testing.T, tag, asset string, body []byte) *addonServer {
	t.Helper()
	s := &addonServer{}
	s.Server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.hits.Add(1)
		if r.URL.Path != "/"+tag+"/"+asset {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write(body)
	}))
	t.Cleanup(s.Close)
	return s
}

func testInstaller(tree string, srv *addonServer, out *bytes.Buffer) *addonInstaller {
	client := srv.Client()
	client.CheckRedirect = addonCheckRedirect
	return &addonInstaller{
		tree: tree, work: filepath.Dir(tree), client: client, baseURL: srv.URL,
		maxDownload: postgisAddonMaxDownload, maxUnpacked: postgisAddonMaxUnpacked,
		ensureTree: func(context.Context, string) error { return fmt.Errorf("ensureTree must not be called") },
		out:        out,
	}
}

func testRelease(t *testing.T, tag, goos, goarch string, archive []byte) addonRelease {
	t.Helper()
	rel, err := addonPins{Tag: tag, SHA256: map[string]string{goos + "/" + goarch: sum(archive)}}.release(goos, goarch, false)
	require.NoError(t, err)
	return rel
}

func readTree(t *testing.T, tree, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(tree, filepath.FromSlash(name)))
	require.NoError(t, err, name)
	return string(b)
}

// noAddonLeft: nothing of an add-on reached the tree, and no temp dir is left.
func noAddonLeft(t *testing.T, tree string) {
	t.Helper()
	assert.NoFileExists(t, filepath.Join(tree, postgisAddonManifestName))
	assert.NoFileExists(t, filepath.Join(tree, postgisAddonIncompleteName))
	assert.NoFileExists(t, filepath.Join(tree, "lib", "postgresql", "postgis-3.so"))
	assert.NoFileExists(t, filepath.Join(tree, "share", "postgresql", "extension", "postgis.control"))
	assert.NoDirExists(t, filepath.Join(tree, "postgis"))
	entries, err := os.ReadDir(filepath.Dir(tree))
	require.NoError(t, err)
	for _, e := range entries {
		assert.False(t, strings.HasPrefix(e.Name(), postgisAddonTempPrefix), "temp dir left: %s", e.Name())
	}
	assert.Equal(t, "zonky plpgsql", readTree(t, tree, "lib/postgresql/plpgsql.so"))
}

// ── pins and platform selection ───────────────────────────────────────

func TestAddonPinsSelectThePlatform(t *testing.T) {
	pins := addonPins{Tag: "postgis-addon/v3.6.4-pg18-1", SHA256: map[string]string{
		"darwin/amd64": strings.Repeat("a", 64), "darwin/arm64": strings.Repeat("b", 64),
		"linux/amd64": strings.Repeat("c", 64), "linux/arm64": strings.ToUpper(strings.Repeat("d", 64)),
	}}
	cases := []struct{ goos, goarch, sha string }{
		{"darwin", "amd64", strings.Repeat("a", 64)},
		{"darwin", "arm64", strings.Repeat("b", 64)},
		{"linux", "amd64", strings.Repeat("c", 64)},
		{"linux", "arm64", strings.Repeat("d", 64)}, // upper case pin, lower-cased
	}
	for _, c := range cases {
		rel, err := pins.release(c.goos, c.goarch, false)
		require.NoError(t, err, c.goos+"/"+c.goarch)
		assert.Equal(t, c.sha, rel.SHA256)
		assert.Equal(t, "3.6.4", rel.PostGIS)
		assert.Equal(t, "18", rel.PGMajor)
		assert.Equal(t, "postgis-addon-3.6.4-pg18-"+c.goos+"-"+c.goarch+".tar.gz", rel.Asset)
	}

	for _, p := range [][2]string{{"windows", "amd64"}, {"windows", "arm64"}, {"freebsd", "amd64"}, {"linux", "386"}, {"linux", "riscv64"}} {
		_, err := pins.release(p[0], p[1], false)
		require.ErrorIs(t, err, errAddonNoBuild, p[0]+"/"+p[1])
	}
	_, err := pins.release("linux", "amd64", true)
	require.ErrorIs(t, err, errAddonNoBuild, "musl (Alpine) has no build")
	assert.Contains(t, err.Error(), "musl")

	_, err = addonPins{Tag: "", SHA256: pins.SHA256}.release("linux", "amd64", false)
	require.ErrorIs(t, err, errAddonNotPinned, "no tag")
	_, err = addonPins{Tag: pins.Tag, SHA256: map[string]string{"linux/amd64": " "}}.release("linux", "amd64", false)
	require.ErrorIs(t, err, errAddonNotPinned, "no checksum for the platform")
	_, err = addonPins{Tag: "postgis-addon/v3.6-pg18", SHA256: pins.SHA256}.release("linux", "amd64", false)
	require.Error(t, err)
	assert.NotErrorIs(t, err, errAddonNotPinned)
}

func TestParseAddonTag(t *testing.T) {
	postgis, major, ok := parseAddonTag("postgis-addon/v3.6.4-pg18-12")
	require.True(t, ok)
	assert.Equal(t, "3.6.4", postgis)
	assert.Equal(t, "18", major)
	for _, bad := range []string{"", "fcdev/v0.32.0", "postgis-addon/v3.6.4-pg18", "postgis-addon/v3.6.4-pg18-dev", "postgis-addon/3.6.4-pg18-1", "postgis-addon/v3.6.4-18-1"} {
		_, _, ok := parseAddonTag(bad)
		assert.False(t, ok, bad)
	}
}

// The pin file itself: empty until a release is cut; once set, the tag is
// for the major this fcdev embeds and every platform has a checksum.
func TestPinnedAddonIsConsistent(t *testing.T) {
	for _, key := range []string{"darwin/amd64", "darwin/arm64", "linux/amd64", "linux/arm64"} {
		_, ok := postgisAddonSHA256[key]
		assert.True(t, ok, "postgisAddonSHA256 lists %s", key)
	}
	if postgisAddonTag == "" {
		_, err := pinnedAddon().release("linux", "amd64", false)
		require.ErrorIs(t, err, errAddonNotPinned)
		return
	}
	_, major, ok := parseAddonTag(postgisAddonTag)
	require.True(t, ok, postgisAddonTag)
	assert.Equal(t, pinnedPGMajor(), major, "the add-on's PostgreSQL major is the embedded one")
	for key, sha := range postgisAddonSHA256 {
		b, err := hex.DecodeString(sha)
		assert.NoError(t, err, key)
		assert.Len(t, b, 32, key)
	}
}

// ── installing ────────────────────────────────────────────────────────

func TestAddonInstallFreshThenRepeat(t *testing.T) {
	tree := fakeEmbeddedTree(t, "18")
	archive := addonTarball(t, testAddonTag, "linux", "amd64", "3.6.4")
	rel := testRelease(t, testAddonTag, "linux", "amd64", archive)
	srv := newAddonServer(t, testAddonTag, rel.Asset, archive)
	var out bytes.Buffer
	in := testInstaller(tree, srv, &out)

	outcome, err := in.Install(context.Background(), rel)
	require.NoError(t, err, out.String())
	assert.Equal(t, addonOutcomeInstalled, outcome)
	assert.Equal(t, int32(1), srv.hits.Load())

	assert.Equal(t, "module 3.6.4", readTree(t, tree, "lib/postgresql/postgis-3.so"))
	assert.Equal(t, "proj.db 3.6.4", readTree(t, tree, "lib/share/proj/proj.db"))
	assert.Equal(t, "-- install 3.6.4", readTree(t, tree, "share/postgresql/extension/postgis--3.6.4.sql"))
	assert.Equal(t, "zonky plpgsql", readTree(t, tree, "lib/postgresql/plpgsql.so"), "zonky's files are left alone")
	assert.Equal(t, "zonky plpgsql control", readTree(t, tree, "share/postgresql/extension/plpgsql.control"))
	assert.NoFileExists(t, filepath.Join(tree, postgisAddonIncompleteName))
	st, err := os.Stat(filepath.Join(tree, "lib", "postgresql", "postgis-3.so"))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o755), st.Mode().Perm())
	st, err = os.Stat(filepath.Join(tree, "share", "postgresql", "extension", "postgis.control"))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o644), st.Mode().Perm())

	state, m := postgisAddonState(tree)
	assert.Equal(t, addonStateInstalled, state)
	assert.Equal(t, testAddonTag, m.Tag)

	// Again: nothing downloaded, nothing changed.
	outcome, err = in.Install(context.Background(), rel)
	require.NoError(t, err)
	assert.Equal(t, addonOutcomeAlready, outcome)
	assert.Equal(t, int32(1), srv.hits.Load(), "a repeat run downloads nothing")

	var report bytes.Buffer
	reportAddonOutcome(&report, tree, rel, addonOutcomeAlready, 0)
	assert.Contains(t, report.String(), "already installed")
	assert.Contains(t, report.String(), "nothing to do")
}

func TestAddonInstallReplacesAnEarlierOne(t *testing.T) {
	tree := fakeEmbeddedTree(t, "18")
	ctx := context.Background()
	var out bytes.Buffer

	old := addonTarball(t, "postgis-addon/v3.6.3-pg18-1", "darwin", "arm64", "3.6.3")
	oldRel := testRelease(t, "postgis-addon/v3.6.3-pg18-1", "darwin", "arm64", old)
	outcome, err := testInstaller(tree, newAddonServer(t, oldRel.Tag, oldRel.Asset, old), &out).Install(ctx, oldRel)
	require.NoError(t, err, out.String())
	assert.Equal(t, addonOutcomeInstalled, outcome)
	assert.FileExists(t, filepath.Join(tree, "postgis", "lib", "libgeos.3.6.3.dylib"))

	archive := addonTarball(t, testAddonTag, "darwin", "arm64", "3.6.4")
	rel := testRelease(t, testAddonTag, "darwin", "arm64", archive)
	outcome, err = testInstaller(tree, newAddonServer(t, rel.Tag, rel.Asset, archive), &out).Install(ctx, rel)
	require.NoError(t, err, out.String())
	assert.Equal(t, addonOutcomeReplaced, outcome)

	assert.Equal(t, "module 3.6.4", readTree(t, tree, "lib/postgresql/postgis-3.dylib"))
	assert.Equal(t, "proj.db 3.6.4", readTree(t, tree, "postgis/share/proj/proj.db"))
	assert.NoFileExists(t, filepath.Join(tree, "postgis", "lib", "libgeos.3.6.3.dylib"), "the add-on's own directory is replaced whole")
	assert.FileExists(t, filepath.Join(tree, "share", "postgresql", "extension", "postgis--3.6.3.sql"), "old install scripts stay beside zonky's files")
	state, m := postgisAddonState(tree)
	assert.Equal(t, addonStateInstalled, state)
	assert.Equal(t, testAddonTag, m.Tag)

	var report bytes.Buffer
	reportAddonOutcome(&report, tree, rel, outcome, 4242)
	assert.Contains(t, report.String(), "fcdev is running (pid 4242): restart it")
	assert.Contains(t, report.String(), "ALTER EXTENSION postgis UPDATE")
}

func TestAddonInstallReplacesAHandCopiedPostGIS(t *testing.T) {
	tree := fakeEmbeddedTree(t, "18")
	require.NoError(t, os.WriteFile(filepath.Join(tree, "share", "postgresql", "extension", "postgis.control"), []byte("by hand"), 0o644))
	state, _ := postgisAddonState(tree)
	require.Equal(t, addonStateForeign, state)

	archive := addonTarball(t, testAddonTag, "linux", "arm64", "3.6.4")
	rel := testRelease(t, testAddonTag, "linux", "arm64", archive)
	var out bytes.Buffer
	outcome, err := testInstaller(tree, newAddonServer(t, rel.Tag, rel.Asset, archive), &out).Install(context.Background(), rel)
	require.NoError(t, err)
	assert.Equal(t, addonOutcomeReplaced, outcome, "a hand-copied PostGIS may be loaded: the user is told to restart")
	assert.Equal(t, "default_version = '3.6.4'", readTree(t, tree, "share/postgresql/extension/postgis.control"))
}

func TestAddonInstallChecksumMismatch(t *testing.T) {
	tree := fakeEmbeddedTree(t, "18")
	archive := addonTarball(t, testAddonTag, "linux", "amd64", "3.6.4")
	rel := testRelease(t, testAddonTag, "linux", "amd64", archive)
	rel.SHA256 = strings.Repeat("0", 64)
	var out bytes.Buffer
	_, err := testInstaller(tree, newAddonServer(t, rel.Tag, rel.Asset, archive), &out).Install(context.Background(), rel)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "checksum mismatch")
	noAddonLeft(t, tree)
}

func TestAddonInstallRefusesAnOversizedDownload(t *testing.T) {
	tree := fakeEmbeddedTree(t, "18")
	archive := addonTarball(t, testAddonTag, "linux", "amd64", "3.6.4")
	rel := testRelease(t, testAddonTag, "linux", "amd64", archive)
	var out bytes.Buffer
	in := testInstaller(tree, newAddonServer(t, rel.Tag, rel.Asset, archive), &out)
	in.maxDownload = int64(len(archive) - 1)
	_, err := in.Install(context.Background(), rel)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "allowed")
	noAddonLeft(t, tree)
}

func TestAddonInstallRefusesAPlainHTTPRedirect(t *testing.T) {
	tree := fakeEmbeddedTree(t, "18")
	archive := addonTarball(t, testAddonTag, "linux", "amd64", "3.6.4")
	rel := testRelease(t, testAddonTag, "linux", "amd64", archive)
	plain := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(archive) }))
	defer plain.Close()
	srv := &addonServer{}
	srv.Server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, plain.URL+"/asset", http.StatusFound)
	}))
	defer srv.Close()
	var out bytes.Buffer
	_, err := testInstaller(tree, srv, &out).Install(context.Background(), rel)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not HTTPS")
	noAddonLeft(t, tree)
}

func TestAddonInstallRefusesUnsafeArchives(t *testing.T) {
	cases := map[string]tarEntry{
		"symlink":            {name: "lib/postgresql/evil.so", typ: tar.TypeSymlink, link: "/etc/passwd"},
		"hard link":          {name: "postgis/x", typ: tar.TypeLink, link: "lib/postgresql/postgis-3.so"},
		"fifo":               {name: "postgis/fifo", typ: tar.TypeFifo},
		"parent escape":      {name: "../outside", body: "x"},
		"nested escape":      {name: "postgis/../../outside", body: "x"},
		"absolute path":      {name: "/tmp/outside", body: "x"},
		"backslash":          {name: `postgis\..\x`, body: "x"},
		"zonky's server":     {name: "bin/postgres", body: "not postgres"},
		"zonky's data files": {name: "share/postgresql/postgres.bki", body: "x"},
		"module subdir":      {name: "lib/postgresql/sub/x.so", body: "x"},
		"same file twice":    {name: "lib/postgresql/postgis-3.so", body: "again"},
	}
	for name, entry := range cases {
		t.Run(name, func(t *testing.T) {
			tree := fakeEmbeddedTree(t, "18")
			archive := addonTarball(t, testAddonTag, "linux", "amd64", "3.6.4", entry)
			rel := testRelease(t, testAddonTag, "linux", "amd64", archive)
			var out bytes.Buffer
			_, err := testInstaller(tree, newAddonServer(t, rel.Tag, rel.Asset, archive), &out).Install(context.Background(), rel)
			require.Error(t, err)
			noAddonLeft(t, tree)
			assert.NoFileExists(t, filepath.Join(filepath.Dir(filepath.Dir(tree)), "outside"))
			assert.Equal(t, "#!/bin/sh\n", readTree(t, tree, "bin/pg_ctl"))
		})
	}
}

func TestAddonInstallRefusesAnArchiveThatIsNotTheAsset(t *testing.T) {
	tree := fakeEmbeddedTree(t, "18")
	// A darwin archive served as the linux asset: its manifest gives it away.
	archive := addonTarball(t, testAddonTag, "darwin", "arm64", "3.6.4")
	rel := testRelease(t, testAddonTag, "linux", "amd64", archive)
	var out bytes.Buffer
	_, err := testInstaller(tree, newAddonServer(t, rel.Tag, rel.Asset, archive), &out).Install(context.Background(), rel)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "postgis-addon.json")
	noAddonLeft(t, tree)
}

func TestAddonInstallRefusesAnotherMajor(t *testing.T) {
	tree := fakeEmbeddedTree(t, "17")
	archive := addonTarball(t, testAddonTag, "linux", "amd64", "3.6.4")
	rel := testRelease(t, testAddonTag, "linux", "amd64", archive)
	srv := newAddonServer(t, rel.Tag, rel.Asset, archive)
	var out bytes.Buffer
	_, err := testInstaller(tree, srv, &out).Install(context.Background(), rel)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "PostgreSQL 17")
	assert.Contains(t, err.Error(), "for PostgreSQL 18")
	assert.Equal(t, int32(0), srv.hits.Load(), "refused before downloading")
	noAddonLeft(t, tree)
}

func TestAddonInstallUnpacksTheTreeFirst(t *testing.T) {
	tree := filepath.Join(t.TempDir(), "embedded-pg", "bin") // a fresh machine: nothing there
	archive := addonTarball(t, testAddonTag, "linux", "amd64", "3.6.4")
	rel := testRelease(t, testAddonTag, "linux", "amd64", archive)
	var out bytes.Buffer
	in := testInstaller(tree, newAddonServer(t, rel.Tag, rel.Asset, archive), &out)
	called := 0
	in.ensureTree = func(_ context.Context, dir string) error {
		called++
		assert.Equal(t, tree, dir)
		real := fakeEmbeddedTree(t, "18")
		return os.Rename(real, dir)
	}
	outcome, err := in.Install(context.Background(), rel)
	require.NoError(t, err, out.String())
	assert.Equal(t, 1, called)
	assert.Equal(t, addonOutcomeInstalled, outcome)
	assert.Contains(t, out.String(), "not unpacked yet")
}

// A failure while files are moved in leaves the incomplete marker and no
// manifest: the tree reads as incomplete, start warns, and the next install
// (not skipped as a repeat) finishes the job.
func TestAddonInstallInterruptedLeavesNoManifest(t *testing.T) {
	tree := fakeEmbeddedTree(t, "18")
	archive := addonTarball(t, testAddonTag, "linux", "amd64", "3.6.4")
	rel := testRelease(t, testAddonTag, "linux", "amd64", archive)
	srv := newAddonServer(t, rel.Tag, rel.Asset, archive)
	var out bytes.Buffer
	in := testInstaller(tree, srv, &out)

	// A directory where postgis.control goes makes that rename fail.
	blocker := filepath.Join(tree, "share", "postgresql", "extension", "postgis.control")
	require.NoError(t, os.MkdirAll(filepath.Join(blocker, "x"), 0o755))
	_, err := in.Install(context.Background(), rel)
	require.Error(t, err)
	assert.NoFileExists(t, filepath.Join(tree, postgisAddonManifestName))
	assert.FileExists(t, filepath.Join(tree, postgisAddonIncompleteName))
	state, _ := postgisAddonState(tree)
	assert.Equal(t, addonStateIncomplete, state)
	assert.Contains(t, postgisStartWarning(tree, rel, nil), "incomplete")

	require.NoError(t, os.RemoveAll(blocker))
	outcome, err := in.Install(context.Background(), rel)
	require.NoError(t, err)
	assert.Equal(t, addonOutcomeReplaced, outcome)
	state, _ = postgisAddonState(tree)
	assert.Equal(t, addonStateInstalled, state)
	assert.NoFileExists(t, filepath.Join(tree, postgisAddonIncompleteName))
}

func TestAddonInstallFile(t *testing.T) {
	tree := fakeEmbeddedTree(t, pinnedPGMajor())
	tag := "postgis-addon/v3.6.4-pg" + pinnedPGMajor() + "-7"
	p := filepath.Join(t.TempDir(), "local.tar.gz")
	require.NoError(t, os.WriteFile(p, addonTarball(t, tag, "linux", "arm64", "3.6.4"), 0o644))
	var out bytes.Buffer
	in := testInstaller(tree, newAddonServer(t, "", "", nil), &out)

	_, _, err := in.InstallFile(context.Background(), p, "linux", "amd64", false)
	require.Error(t, err, "another architecture")
	_, _, err = in.InstallFile(context.Background(), p, "windows", "amd64", false)
	require.ErrorIs(t, err, errAddonNoBuild)

	rel, outcome, err := in.InstallFile(context.Background(), p, "linux", "arm64", false)
	require.NoError(t, err, out.String())
	assert.Equal(t, addonOutcomeInstalled, outcome)
	assert.Equal(t, tag, rel.Tag)
	_, outcome, err = in.InstallFile(context.Background(), p, "linux", "arm64", false)
	require.NoError(t, err)
	assert.Equal(t, addonOutcomeReplaced, outcome, "a local archive is always installed")
}

// ── what fcdev start says ────────────────────────────────────────────

func TestPostGISStartWarning(t *testing.T) {
	rel := addonRelease{Tag: testAddonTag}
	tree := fakeEmbeddedTree(t, "18")

	msg := postgisStartWarning(tree, rel, nil)
	assert.Contains(t, msg, "PostGIS is not installed")
	assert.Contains(t, msg, "fcdev install postgis")

	msg = postgisStartWarning(tree, addonRelease{}, fmt.Errorf("%w (windows/amd64)", errAddonNoBuild))
	assert.Contains(t, msg, "PostGIS is not installed")
	assert.Contains(t, msg, "by hand")
	assert.NotContains(t, msg, "fcdev install postgis")

	ext := filepath.Join(tree, "share", "postgresql", "extension")
	require.NoError(t, os.WriteFile(filepath.Join(ext, "postgis.control"), []byte("by hand"), 0o644))
	assert.Empty(t, postgisStartWarning(tree, rel, nil), "a hand-copied PostGIS gets no warning")

	require.NoError(t, os.WriteFile(filepath.Join(tree, "lib", "postgresql", "postgis-3.so"), []byte("m"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(tree, postgisAddonManifestName), []byte(addonManifestJSON(testAddonTag, "linux", "amd64")), 0o644))
	assert.Empty(t, postgisStartWarning(tree, rel, nil), "the pinned add-on, installed")
	assert.Contains(t, postgisStartWarning(tree, addonRelease{Tag: "postgis-addon/v3.7.0-pg18-1"}, nil), "to update it")

	require.NoError(t, os.Remove(filepath.Join(tree, "lib", "postgresql", "postgis-3.so")))
	assert.Contains(t, postgisStartWarning(tree, rel, nil), "incomplete", "a manifest without its module")

	// The tree deleted by hand and unpacked again by embedded-postgres: the
	// warning is back.
	require.NoError(t, os.RemoveAll(tree))
	vanilla := fakeEmbeddedTree(t, "18")
	assert.Contains(t, postgisStartWarning(vanilla, rel, nil), "PostGIS is not installed")
}

func TestEmbeddedTreeMajor(t *testing.T) {
	tree := fakeEmbeddedTree(t, "18")
	major, err := embeddedTreeMajor(tree)
	require.NoError(t, err)
	assert.Equal(t, "18", major)

	require.NoError(t, os.WriteFile(filepath.Join(tree, "share", "postgresql", "postgres.bki"), []byte("create pg_proc\n"), 0o644))
	_, err = embeddedTreeMajor(tree)
	require.Error(t, err)
	_, err = embeddedTreeMajor(t.TempDir())
	require.Error(t, err)
}

func TestFreeLoopbackPortAvoidsTheDefaults(t *testing.T) {
	for range 5 {
		p, err := freeLoopbackPort()
		require.NoError(t, err)
		assert.NotContains(t, []int{15432, 8080}, p)
	}
}
