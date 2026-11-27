package main

import (
	"archive/tar"
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	embeddedpostgres "github.com/fergusstrange/embedded-postgres"
	"github.com/spf13/cobra"
)

// This file is `fcdev install postgis`: it adds PostGIS to the embedded
// PostgreSQL. zonky's binaries, which embedded-postgres runs, are vanilla
// PostgreSQL; the add-on is PostGIS (core extension only) built for them by
// scripts/postgis-addon and published by release-postgis-addon.yml as release
// assets of this repository. The tag and the checksums fcdev trusts are pinned
// in postgis_addon_pins.go.
//
// The add-on is unpacked over the embedded server's tree
// (embeddedPGBinariesDir: bin/, lib/, share/). Its postgis-addon.json is
// written there last, so its presence means a complete install; while files
// are being moved in, postgis-addon.incomplete says one is under way.
// Installing while the server runs is fine: PostgreSQL reads extension files
// at CREATE EXTENSION. Files are replaced by rename, never rewritten in place,
// so a running backend keeps the module it has mapped.

const (
	postgisAddonDocsURL        = "https://github.com/" + defaultUpgradeRepo + "/blob/main/docs/embedded-postgres-postgis.md"
	postgisAddonManifestName   = "postgis-addon.json"
	postgisAddonIncompleteName = "postgis-addon.incomplete"
	postgisAddonLockName       = "postgis-addon.lock"
	postgisAddonTempPrefix     = ".postgis-addon-"

	// postgisAddonMaxDownload bounds the archive (the add-ons are 5–15 MB) and
	// postgisAddonMaxUnpacked what it may unpack to, so a hostile or corrupt
	// download cannot fill the disk.
	postgisAddonMaxDownload = 128 << 20
	postgisAddonMaxUnpacked = 512 << 20
	postgisAddonMaxEntries  = 10_000
)

// postgisAddonDownloadBase is where release assets are downloaded from:
// <base>/<tag>/<asset>. A variable so tests can point it at an httptest server.
var postgisAddonDownloadBase = "https://github.com/" + defaultUpgradeRepo + "/releases/download"

var (
	errAddonNoBuild   = errors.New("no PostGIS add-on is built for this platform")
	errAddonNotPinned = errors.New("this fcdev build has no PostGIS add-on release pinned")
)

// postgisAddonTagRE: postgis-addon/v<postgis>-pg<major>-<build>.
var postgisAddonTagRE = regexp.MustCompile(`^postgis-addon/v([0-9]+\.[0-9]+\.[0-9]+)-pg([0-9]+)-([0-9]+)$`)

// addonPins is what this fcdev trusts: one release tag and the SHA-256 of each
// platform's asset (keyed goos/goarch).
type addonPins struct {
	Tag    string
	SHA256 map[string]string
}

func pinnedAddon() addonPins { return addonPins{Tag: postgisAddonTag, SHA256: postgisAddonSHA256} }

// addonRelease is one platform's asset of an add-on release.
type addonRelease struct {
	Tag     string
	PostGIS string
	PGMajor string
	GOOS    string
	GOARCH  string
	Asset   string
	SHA256  string // empty for a local archive (--archive)
}

func parseAddonTag(tag string) (postgis, major string, ok bool) {
	m := postgisAddonTagRE.FindStringSubmatch(tag)
	if m == nil {
		return "", "", false
	}
	return m[1], m[2], true
}

func addonAssetName(postgis, major, goos, goarch string) string {
	return fmt.Sprintf("postgis-addon-%s-pg%s-%s-%s.tar.gz", postgis, major, goos, goarch)
}

// addonPlatformKey is goos/goarch when an add-on is built for the platform:
// macOS and glibc Linux, amd64 and arm64. Not for Windows or other systems,
// and not for musl Linux: on Alpine embedded-postgres runs zonky's alpine
// build, which a glibc module cannot load.
func addonPlatformKey(goos, goarch string, musl bool) (string, bool) {
	key := goos + "/" + goarch
	switch key {
	case "darwin/amd64", "darwin/arm64":
		return key, true
	case "linux/amd64", "linux/arm64":
		return key, !musl
	}
	return key, false
}

// isMuslLinux is the test embedded-postgres uses to pick zonky's alpine build.
func isMuslLinux() bool {
	_, err := os.Stat("/etc/alpine-release")
	return err == nil
}

// release selects the platform's asset of the pinned release. errAddonNoBuild:
// the platform has no add-on build. errAddonNotPinned: this fcdev pins no
// release, or no checksum for the platform.
func (p addonPins) release(goos, goarch string, musl bool) (addonRelease, error) {
	key, ok := addonPlatformKey(goos, goarch, musl)
	if !ok {
		if musl {
			key += " (musl)"
		}
		return addonRelease{}, fmt.Errorf("%w (%s)", errAddonNoBuild, key)
	}
	if p.Tag == "" {
		return addonRelease{}, errAddonNotPinned
	}
	postgis, major, ok := parseAddonTag(p.Tag)
	if !ok {
		return addonRelease{}, fmt.Errorf("the pinned PostGIS add-on tag %q is not postgis-addon/v<postgis>-pg<major>-<build>", p.Tag)
	}
	sha := strings.ToLower(strings.TrimSpace(p.SHA256[key]))
	if sha == "" {
		return addonRelease{}, fmt.Errorf("%w for %s", errAddonNotPinned, key)
	}
	return addonRelease{
		Tag: p.Tag, PostGIS: postgis, PGMajor: major, GOOS: goos, GOARCH: goarch,
		Asset: addonAssetName(postgis, major, goos, goarch), SHA256: sha,
	}, nil
}

// addonManifest is postgis-addon.json, at the root of every add-on archive.
type addonManifest struct {
	PostGIS string `json:"postgis"`
	PGMajor string `json:"pgMajor"`
	GOOS    string `json:"goos"`
	GOARCH  string `json:"goarch"`
	Tag     string `json:"tag"`
}

func parseAddonManifest(b []byte) (addonManifest, bool) {
	var m addonManifest
	if json.Unmarshal(b, &m) != nil || m.PostGIS == "" || m.PGMajor == "" || m.GOOS == "" || m.GOARCH == "" || m.Tag == "" {
		return addonManifest{}, false
	}
	return m, true
}

func readAddonManifest(p string) (addonManifest, bool) {
	b, err := os.ReadFile(p)
	if err != nil {
		return addonManifest{}, false
	}
	return parseAddonManifest(b)
}

// check refuses an archive that is not the asset it was fetched as.
func (m addonManifest) check(r addonRelease) error {
	if m.Tag != r.Tag || m.PostGIS != r.PostGIS || m.PGMajor != r.PGMajor || m.GOOS != r.GOOS || m.GOARCH != r.GOARCH {
		return fmt.Errorf("the archive's postgis-addon.json is %s (PostGIS %s, PostgreSQL %s, %s/%s), not %s (PostGIS %s, PostgreSQL %s, %s/%s)",
			m.Tag, m.PostGIS, m.PGMajor, m.GOOS, m.GOARCH, r.Tag, r.PostGIS, r.PGMajor, r.GOOS, r.GOARCH)
	}
	return nil
}

// addonModuleFile is the PostGIS module's file name in lib/postgresql.
func addonModuleFile(goos string) string {
	if goos == "darwin" {
		return "postgis-3.dylib"
	}
	return "postgis-3.so"
}

// addonState is what the embedded tree holds of PostGIS.
type addonState string

const (
	// addonStateMissing: no PostGIS at all.
	addonStateMissing addonState = "missing"
	// addonStateInstalled: an add-on fcdev installed, complete.
	addonStateInstalled addonState = "installed"
	// addonStateIncomplete: an install that did not finish, or an add-on
	// whose files have gone.
	addonStateIncomplete addonState = "incomplete"
	// addonStateForeign: PostGIS that fcdev did not install (copied in by
	// hand, the docs' manual method).
	addonStateForeign addonState = "foreign"
)

// regularFileExists: p exists and is not a directory.
func regularFileExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}

// postgisAddonState reads the tree; the manifest is set for addonStateInstalled.
func postgisAddonState(tree string) (addonState, addonManifest) {
	control := regularFileExists(filepath.Join(tree, "share", "postgresql", "extension", "postgis.control"))
	if regularFileExists(filepath.Join(tree, postgisAddonIncompleteName)) {
		return addonStateIncomplete, addonManifest{}
	}
	m, ok := readAddonManifest(filepath.Join(tree, postgisAddonManifestName))
	switch {
	case ok && control && regularFileExists(filepath.Join(tree, "lib", "postgresql", addonModuleFile(m.GOOS))):
		return addonStateInstalled, m
	case ok:
		return addonStateIncomplete, addonManifest{}
	case control:
		return addonStateForeign, addonManifest{}
	}
	return addonStateMissing, addonManifest{}
}

// postgisStartWarning is the one warning `fcdev start` logs about PostGIS, or
// "" when there is nothing to say. rel and relErr are this fcdev's pinned
// release for the platform (pinnedAddon().release).
func postgisStartWarning(tree string, rel addonRelease, relErr error) string {
	state, m := postgisAddonState(tree)
	switch state {
	case addonStateInstalled:
		if relErr == nil && m.Tag != rel.Tag {
			return fmt.Sprintf("the embedded Postgres has PostGIS add-on %s and this fcdev pins %s: run `fcdev install postgis` to update it", m.Tag, rel.Tag)
		}
		return ""
	case addonStateForeign:
		return ""
	case addonStateIncomplete:
		return "the PostGIS add-on in the embedded Postgres is incomplete (an install did not finish): run `fcdev install postgis` again"
	case addonStateMissing:
		if relErr == nil {
			return "PostGIS is not installed in the embedded Postgres: run `fcdev install postgis` to add it"
		}
		return "PostGIS is not installed in the embedded Postgres, and this fcdev has no PostGIS add-on for this platform; the docs say how to add it by hand"
	}
	return ""
}

// logPostGISStartWarning is called by `fcdev start` once the embedded server
// is up (so its tree exists). It never fails the start.
func logPostGISStartWarning() {
	rel, err := pinnedAddon().release(runtime.GOOS, runtime.GOARCH, isMuslLinux())
	if msg := postgisStartWarning(embeddedPGBinariesDir(), rel, err); msg != "" {
		slog.Warn(msg, "docs", postgisAddonDocsURL)
	}
}

// embeddedTreeMajor is the PostgreSQL major of the server in tree, from the
// first line of share/postgresql/postgres.bki ("# PostgreSQL 18"), the
// bootstrap file initdb runs, which belongs to exactly one major.
func embeddedTreeMajor(tree string) (string, error) {
	p := filepath.Join(tree, "share", "postgresql", "postgres.bki")
	f, err := os.Open(p)
	if err != nil {
		return "", fmt.Errorf("read the embedded PostgreSQL's version: %w", err)
	}
	defer func() { _ = f.Close() }()
	line, err := bufio.NewReader(io.LimitReader(f, 256)).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", fmt.Errorf("read %s: %w", p, err)
	}
	v, ok := strings.CutPrefix(strings.TrimSpace(line), "# PostgreSQL ")
	if !ok || v == "" || strings.Trim(v, "0123456789") != "" {
		return "", fmt.Errorf("cannot tell the PostgreSQL version from %s (first line %q)", p, strings.TrimSpace(line))
	}
	return v, nil
}

// addonOutcome is what an install did.
type addonOutcome string

const (
	// addonOutcomeInstalled: there was no PostGIS; now there is.
	addonOutcomeInstalled addonOutcome = "installed"
	// addonOutcomeReplaced: PostGIS was there (an add-on, an unfinished
	// install, or a hand-copied one) and has been replaced.
	addonOutcomeReplaced addonOutcome = "replaced"
	// addonOutcomeAlready: this release was installed already; nothing done.
	addonOutcomeAlready addonOutcome = "already-installed"
)

// addonInstaller installs add-ons into one embedded tree.
type addonInstaller struct {
	tree        string // the embedded server's tree (bin/, lib/, share/)
	work        string // lock, download and staging: the directory holding tree, so renames stay on one filesystem
	client      *http.Client
	baseURL     string
	maxDownload int64
	maxUnpacked int64
	// ensureTree makes the tree exist when bin/pg_ctl is missing (a machine
	// where fcdev has never started the embedded server).
	ensureTree func(ctx context.Context, tree string) error
	out        io.Writer
}

func newAddonInstaller(out io.Writer) *addonInstaller {
	tree := embeddedPGBinariesDir()
	return &addonInstaller{
		tree:        tree,
		work:        filepath.Dir(tree),
		client:      &http.Client{Timeout: 10 * time.Minute, CheckRedirect: addonCheckRedirect},
		baseURL:     postgisAddonDownloadBase,
		maxDownload: postgisAddonMaxDownload,
		maxUnpacked: postgisAddonMaxUnpacked,
		ensureTree:  bootstrapEmbeddedTree,
		out:         out,
	}
}

// addonCheckRedirect follows GitHub's redirect to its asset storage, HTTPS only.
func addonCheckRedirect(req *http.Request, via []*http.Request) error {
	if req.URL.Scheme != "https" {
		return fmt.Errorf("refusing a redirect to %s: not HTTPS", req.URL.Redacted())
	}
	if len(via) >= 10 {
		return errors.New("too many redirects")
	}
	return nil
}

// Install downloads rel, checks its SHA-256 and installs it. A release that is
// installed already is left alone.
func (in *addonInstaller) Install(ctx context.Context, rel addonRelease) (addonOutcome, error) {
	return in.install(ctx, rel, true, func(ctx context.Context, dir string) (string, error) {
		return in.download(ctx, rel, dir)
	})
}

// InstallFile installs a local add-on archive (a local build), always. It has
// no pinned checksum: its postgis-addon.json must name this platform and the
// PostgreSQL major this fcdev embeds.
func (in *addonInstaller) InstallFile(ctx context.Context, archive, goos, goarch string, musl bool) (addonRelease, addonOutcome, error) {
	key, ok := addonPlatformKey(goos, goarch, musl)
	if !ok {
		return addonRelease{}, "", fmt.Errorf("%w (%s)", errAddonNoBuild, key)
	}
	m, err := peekAddonManifest(archive)
	if err != nil {
		return addonRelease{}, "", err
	}
	if m.GOOS+"/"+m.GOARCH != key {
		return addonRelease{}, "", fmt.Errorf("%s is the add-on for %s/%s, and this is %s", filepath.Base(archive), m.GOOS, m.GOARCH, key)
	}
	if m.PGMajor != pinnedPGMajor() {
		return addonRelease{}, "", fmt.Errorf("%s is for PostgreSQL %s, and this fcdev embeds PostgreSQL %s", filepath.Base(archive), m.PGMajor, pinnedPGMajor())
	}
	rel := addonRelease{Tag: m.Tag, PostGIS: m.PostGIS, PGMajor: m.PGMajor, GOOS: m.GOOS, GOARCH: m.GOARCH, Asset: filepath.Base(archive)}
	outcome, err := in.install(ctx, rel, false, func(context.Context, string) (string, error) { return archive, nil })
	return rel, outcome, err
}

func (in *addonInstaller) install(ctx context.Context, rel addonRelease, skipIfCurrent bool,
	fetch func(ctx context.Context, dir string) (string, error),
) (addonOutcome, error) {
	if err := os.MkdirAll(in.work, 0o755); err != nil {
		return "", fmt.Errorf("create %s: %w", in.work, err)
	}
	unlock, err := lockAddonInstall(filepath.Join(in.work, postgisAddonLockName), in.out)
	if err != nil {
		return "", fmt.Errorf("lock %s: %w", filepath.Join(in.work, postgisAddonLockName), err)
	}
	defer unlock()
	in.sweepTemps()

	if !regularFileExists(filepath.Join(in.tree, "bin", "pg_ctl")) {
		fmt.Fprintf(in.out, "The embedded PostgreSQL is not unpacked yet (%s); fetching it first, as fcdev start does.\n", in.tree)
		if err := in.ensureTree(ctx, in.tree); err != nil {
			return "", fmt.Errorf("unpack the embedded PostgreSQL: %w", err)
		}
		if !regularFileExists(filepath.Join(in.tree, "bin", "pg_ctl")) {
			return "", fmt.Errorf("the embedded PostgreSQL at %s has no bin/pg_ctl", in.tree)
		}
	}
	major, err := embeddedTreeMajor(in.tree)
	if err != nil {
		return "", err
	}
	if major != rel.PGMajor {
		return "", fmt.Errorf("the embedded server at %s is PostgreSQL %s, and the PostGIS add-on %s is for PostgreSQL %s; "+
			"nothing was installed", in.tree, major, rel.Tag, rel.PGMajor)
	}

	state, current := postgisAddonState(in.tree)
	if skipIfCurrent && state == addonStateInstalled && current.Tag == rel.Tag && current.GOOS == rel.GOOS && current.GOARCH == rel.GOARCH {
		return addonOutcomeAlready, nil
	}

	tmp, err := os.MkdirTemp(in.work, postgisAddonTempPrefix)
	if err != nil {
		return "", err
	}
	defer func() { _ = os.RemoveAll(tmp) }()
	archive, err := fetch(ctx, tmp)
	if err != nil {
		return "", err
	}
	stage := filepath.Join(tmp, "stage")
	if err := extractAddonArchive(archive, stage, in.maxUnpacked); err != nil {
		return "", fmt.Errorf("unpack %s: %w", filepath.Base(archive), err)
	}
	m, ok := readAddonManifest(filepath.Join(stage, postgisAddonManifestName))
	if !ok {
		return "", fmt.Errorf("%s has no readable %s at its root", filepath.Base(archive), postgisAddonManifestName)
	}
	if err := m.check(rel); err != nil {
		return "", err
	}
	if !regularFileExists(filepath.Join(stage, "lib", "postgresql", addonModuleFile(rel.GOOS))) ||
		!regularFileExists(filepath.Join(stage, "share", "postgresql", "extension", "postgis.control")) {
		return "", fmt.Errorf("%s has no lib/postgresql/%s or share/postgresql/extension/postgis.control", filepath.Base(archive), addonModuleFile(rel.GOOS))
	}
	if err := in.moveIntoTree(stage, tmp); err != nil {
		return "", fmt.Errorf("install into %s: %w (run fcdev install postgis again)", in.tree, err)
	}
	if state == addonStateMissing {
		return addonOutcomeInstalled, nil
	}
	return addonOutcomeReplaced, nil
}

// addonOwnedDirs are the add-on's own directories in the tree: replaced as a
// whole, so files a previous add-on had and this one has not do not linger.
// Everything else (lib/postgresql, share/postgresql/extension, which hold
// zonky's files too) is replaced file by file.
var addonOwnedDirs = []string{"postgis", "lib/share/proj"}

// moveIntoTree moves the staged add-on into the tree: the incomplete marker
// first, the manifest last.
func (in *addonInstaller) moveIntoTree(stage, tmp string) error {
	incomplete := filepath.Join(in.tree, postgisAddonIncompleteName)
	if err := os.WriteFile(incomplete, []byte("an fcdev install postgis is under way, or did not finish: run it again\n"), 0o600); err != nil {
		return err
	}
	if err := os.Remove(filepath.Join(in.tree, postgisAddonManifestName)); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	for i, d := range addonOwnedDirs {
		src := filepath.Join(stage, filepath.FromSlash(d))
		if st, err := os.Stat(src); err != nil || !st.IsDir() {
			continue
		}
		dst := filepath.Join(in.tree, filepath.FromSlash(d))
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		// The old directory goes into tmp (removed with it); a running server
		// keeps any of its files it has open or mapped.
		if err := os.Rename(dst, filepath.Join(tmp, fmt.Sprintf("old-%d", i))); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		if err := os.Rename(src, dst); err != nil {
			return err
		}
	}
	// The rest, file by file. The stage is this install's own temp directory
	// (0700, made by MkdirTemp) and holds no links (extractAddonArchive).
	var files []string
	err := filepath.WalkDir(stage, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(stage, p)
		if err != nil {
			return err
		}
		if !d.IsDir() && rel != postgisAddonManifestName {
			files = append(files, rel)
		}
		return nil
	})
	if err != nil {
		return err
	}
	for _, rel := range files {
		dst := filepath.Join(in.tree, rel)
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		if err := os.Rename(filepath.Join(stage, rel), dst); err != nil {
			return err
		}
	}
	if err := os.Rename(filepath.Join(stage, postgisAddonManifestName), filepath.Join(in.tree, postgisAddonManifestName)); err != nil {
		return err
	}
	return os.Remove(incomplete)
}

// sweepTemps removes what an interrupted install left beside the tree. It
// runs under the lock, so no install is using them.
func (in *addonInstaller) sweepTemps() {
	entries, _ := os.ReadDir(in.work)
	for _, e := range entries {
		if e.IsDir() && strings.HasPrefix(e.Name(), postgisAddonTempPrefix) {
			_ = os.RemoveAll(filepath.Join(in.work, e.Name()))
		}
	}
}

// download fetches rel's asset into dir over HTTPS, within the size limit, and
// checks its SHA-256 while it streams.
func (in *addonInstaller) download(ctx context.Context, rel addonRelease, dir string) (string, error) {
	if rel.SHA256 == "" {
		return "", errors.New("no checksum to verify the add-on against")
	}
	u := strings.TrimRight(in.baseURL, "/") + "/" + rel.Tag + "/" + rel.Asset
	if !strings.HasPrefix(u, "https://") {
		return "", fmt.Errorf("refusing to download %s: not HTTPS", u)
	}
	fmt.Fprintf(in.out, "Downloading %s\n", u)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "fcdev")
	resp, err := in.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("download the PostGIS add-on: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download the PostGIS add-on: GET %s: %s", u, resp.Status)
	}
	if resp.ContentLength > in.maxDownload {
		return "", fmt.Errorf("the PostGIS add-on is %d bytes, more than the %d allowed", resp.ContentLength, in.maxDownload)
	}
	dest := filepath.Join(dir, rel.Asset)
	f, err := os.OpenFile(dest, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return "", err
	}
	h := sha256.New()
	n, copyErr := io.Copy(io.MultiWriter(f, h), io.LimitReader(resp.Body, in.maxDownload+1))
	if err := errors.Join(copyErr, f.Close()); err != nil {
		return "", fmt.Errorf("download the PostGIS add-on: %w", err)
	}
	if n > in.maxDownload {
		return "", fmt.Errorf("the PostGIS add-on is more than the %d bytes allowed", in.maxDownload)
	}
	if got := hex.EncodeToString(h.Sum(nil)); got != rel.SHA256 {
		return "", fmt.Errorf("checksum mismatch for %s: expected %s, got %s; nothing was installed", rel.Asset, rel.SHA256, got)
	}
	fmt.Fprintf(in.out, "Verified SHA-256 %s\n", rel.SHA256)
	return dest, nil
}

// addonEntryName normalises an archive entry name ("./lib/" → "lib") and
// refuses absolute names, "..", backslashes and anything not in clean form.
// "" is the archive's root directory entry.
func addonEntryName(raw string) (string, error) {
	name := strings.TrimSuffix(strings.TrimPrefix(raw, "./"), "/")
	if name == "" || name == "." {
		return "", nil
	}
	if strings.HasPrefix(name, "/") || strings.ContainsAny(name, "\\\x00") || path.Clean(name) != name {
		return "", fmt.Errorf("unsafe path %q in the archive", raw)
	}
	for el := range strings.SplitSeq(name, "/") {
		if el == ".." || el == "." || el == "" {
			return "", fmt.Errorf("unsafe path %q in the archive", raw)
		}
	}
	return name, nil
}

// addonDirs are the directories an add-on may hold (besides any under its
// own postgis/ and lib/share/proj/).
var addonDirs = map[string]bool{
	"lib": true, "lib/postgresql": true, "lib/share": true, "lib/share/proj": true,
	"share": true, "share/postgresql": true, "share/postgresql/extension": true, "postgis": true,
}

// allowedAddonPath says whether an add-on may put this entry into the tree:
// the manifest, modules directly in lib/postgresql, extension files directly
// in share/postgresql/extension, and anything under postgis/ and
// lib/share/proj/. Never bin/, and never anything else of zonky's.
func allowedAddonPath(name string, dir bool) bool {
	switch {
	case name == postgisAddonManifestName:
		return !dir
	case addonDirs[name]:
		return dir
	case strings.HasPrefix(name, "postgis/"), strings.HasPrefix(name, "lib/share/proj/"):
		return true
	case path.Dir(name) == "lib/postgresql", path.Dir(name) == "share/postgresql/extension":
		return !dir
	}
	return false
}

// extractAddonArchive unpacks an add-on .tar.gz into dest (created). Regular
// files and directories only (no links, devices or other entries), under the
// paths allowedAddonPath accepts; each file once; modes 0644 or 0755 (0755
// for anything executable), directories 0755; at most limit bytes and
// postgisAddonMaxEntries entries.
func extractAddonArchive(archive, dest string, limit int64) error {
	f, err := os.Open(archive)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer func() { _ = gz.Close() }()
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return err
	}
	tr := tar.NewReader(gz)
	var total int64
	for n := 0; ; n++ {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		if n >= postgisAddonMaxEntries {
			return fmt.Errorf("the archive has more than %d entries", postgisAddonMaxEntries)
		}
		if h.Typeflag == tar.TypeXGlobalHeader {
			continue
		}
		name, err := addonEntryName(h.Name)
		if err != nil {
			return err
		}
		switch h.Typeflag {
		case tar.TypeDir:
			if name == "" {
				continue
			}
			if !allowedAddonPath(name, true) {
				return fmt.Errorf("the archive has %q, which an add-on may not install", h.Name)
			}
			if err := os.MkdirAll(filepath.Join(dest, filepath.FromSlash(name)), 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if name == "" || !allowedAddonPath(name, false) {
				return fmt.Errorf("the archive has %q, which an add-on may not install", h.Name)
			}
			if h.Size < 0 || h.Size > limit-total {
				return fmt.Errorf("the archive unpacks to more than the %d bytes allowed", limit)
			}
			if err := writeAddonFile(filepath.Join(dest, filepath.FromSlash(name)), h, tr); err != nil {
				return err
			}
			total += h.Size
		default:
			return fmt.Errorf("refusing %q in the archive: an add-on holds regular files and directories only (entry type %q)", h.Name, string(h.Typeflag))
		}
	}
}

func writeAddonFile(p string, h *tar.Header, r io.Reader) error {
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	mode := os.FileMode(0o644)
	if h.Mode&0o111 != 0 {
		mode = 0o755
	}
	// O_EXCL: an archive that names one file twice is refused.
	out, err := os.OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		if errors.Is(err, fs.ErrExist) {
			return fmt.Errorf("the archive has %q twice", h.Name)
		}
		return err
	}
	_, err = io.CopyN(out, r, h.Size)
	return errors.Join(err, out.Close())
}

// peekAddonManifest reads postgis-addon.json from an archive without
// unpacking it.
func peekAddonManifest(archive string) (addonManifest, error) {
	f, err := os.Open(archive)
	if err != nil {
		return addonManifest{}, err
	}
	defer func() { _ = f.Close() }()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return addonManifest{}, fmt.Errorf("%s: %w", filepath.Base(archive), err)
	}
	defer func() { _ = gz.Close() }()
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return addonManifest{}, fmt.Errorf("%s has no %s", filepath.Base(archive), postgisAddonManifestName)
		}
		if err != nil {
			return addonManifest{}, fmt.Errorf("%s: %w", filepath.Base(archive), err)
		}
		if name, err := addonEntryName(h.Name); err == nil && name == postgisAddonManifestName && h.Typeflag == tar.TypeReg {
			b, err := io.ReadAll(io.LimitReader(tr, 64<<10))
			if err != nil {
				return addonManifest{}, err
			}
			m, ok := parseAddonManifest(b)
			if !ok {
				return addonManifest{}, fmt.Errorf("%s: %s is not readable", filepath.Base(archive), postgisAddonManifestName)
			}
			return m, nil
		}
	}
}

// bootstrapEmbeddedTree makes the embedded tree exist the way `fcdev start`
// does, through embedded-postgres itself (its download from Maven Central,
// its checksum, its unpacking): it starts a throwaway server with the tree as
// its binaries directory, in a temporary data and runtime directory on a free
// port, and stops it.
func bootstrapEmbeddedTree(_ context.Context, tree string) error {
	work := filepath.Dir(tree)
	if err := os.MkdirAll(work, 0o755); err != nil {
		return err
	}
	tmp, err := os.MkdirTemp(work, ".pg-bootstrap-")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(tmp) }()
	port, err := freeLoopbackPort()
	if err != nil {
		return err
	}
	var logs bytes.Buffer
	pg := embeddedpostgres.NewDatabase(embeddedpostgres.DefaultConfig().
		Version(embeddedPGVersion).
		Port(uint32(port)).
		DataPath(filepath.Join(tmp, "data")).
		RuntimePath(filepath.Join(tmp, "runtime")).
		BinariesPath(tree).
		Username(embeddedDBUser).
		Password(defaultEmbeddedDBPassword).
		Database("postgres").
		StartParameters(embeddedPGStartParameters).
		StartTimeout(60 * time.Second).
		Logger(&logs))
	if err := pg.Start(); err != nil {
		return fmt.Errorf("%w\n%s", err, logs.String())
	}
	return pg.Stop()
}

// freeLoopbackPort is a free TCP port on 127.0.0.1, never 15432 (the embedded
// database's default) or 8080 (the API's).
func freeLoopbackPort() (int, error) {
	for range 20 {
		l, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			return 0, err
		}
		addr, ok := l.Addr().(*net.TCPAddr)
		_ = l.Close()
		if ok && addr.Port != 15432 && addr.Port != 8080 {
			return addr.Port, nil
		}
	}
	return 0, errors.New("no free port on 127.0.0.1")
}

// ── the command ───────────────────────────────────────────────────────────

func newInstallCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "install",
		Short: "Add optional parts to the embedded dev database",
	}
	cmd.AddCommand(newInstallPostGISCmd())
	return cmd
}

func newInstallPostGISCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "postgis",
		Short: "Add PostGIS to the embedded Postgres",
		Long: `Download the PostGIS add-on this fcdev pins (PostGIS built for the embedded
PostgreSQL ` + pinnedPGMajor() + `, core extension only), check its SHA-256, and unpack it into the
embedded server's tree. Then enable it in a database with CREATE EXTENSION postgis.

It may run while fcdev start is running: PostgreSQL reads an extension's files when
CREATE EXTENSION runs. When it replaces a PostGIS that a running server may have
loaded, restart fcdev. Running it again when this add-on is installed does nothing.

Add-ons are built for macOS (arm64, amd64) and Linux with glibc (amd64, arm64).
On other platforms the docs describe adding PostGIS by hand:
` + postgisAddonDocsURL,
		Args: cobra.NoArgs,
		RunE: runInstallPostGIS,
	}
	cmd.Flags().String("archive", "", "install this add-on tarball (a local build of scripts/postgis-addon) instead of the pinned release; "+
		"its postgis-addon.json must name this platform, and there is no pinned checksum to check it against")
	return cmd
}

func runInstallPostGIS(cmd *cobra.Command, _ []string) error {
	out := cmd.OutOrStdout()
	in := newAddonInstaller(out)
	archive, _ := cmd.Flags().GetString("archive")

	var rel addonRelease
	var outcome addonOutcome
	var err error
	if archive != "" {
		rel, outcome, err = in.InstallFile(cmd.Context(), archive, runtime.GOOS, runtime.GOARCH, isMuslLinux())
	} else {
		rel, err = pinnedAddon().release(runtime.GOOS, runtime.GOARCH, isMuslLinux())
		if err == nil && rel.PGMajor != pinnedPGMajor() {
			err = fmt.Errorf("this fcdev pins the PostGIS add-on %s, for PostgreSQL %s, but embeds PostgreSQL %s", rel.Tag, rel.PGMajor, pinnedPGMajor())
		}
		if err == nil {
			outcome, err = in.Install(cmd.Context(), rel)
		}
	}
	if errors.Is(err, errAddonNoBuild) || errors.Is(err, errAddonNotPinned) {
		fmt.Fprintf(cmd.ErrOrStderr(), "fcdev install postgis: %v.\nTo add PostGIS by hand, see %s\n", err, postgisAddonDocsURL)
		return &exitError{Code: 1}
	}
	if err != nil {
		return err
	}
	reportAddonOutcome(out, in.tree, rel, outcome, fcdevRunningPID())
	return nil
}

func reportAddonOutcome(out io.Writer, tree string, rel addonRelease, outcome addonOutcome, runningPID int) {
	what := fmt.Sprintf("PostGIS %s (%s)", rel.PostGIS, rel.Tag)
	switch outcome {
	case addonOutcomeAlready:
		fmt.Fprintf(out, "%s is already installed in the embedded PostgreSQL at %s; nothing to do.\n", what, tree)
		return
	case addonOutcomeInstalled:
		fmt.Fprintf(out, "Installed %s into the embedded PostgreSQL %s at %s.\n", what, rel.PGMajor, tree)
	case addonOutcomeReplaced:
		fmt.Fprintf(out, "Installed %s into the embedded PostgreSQL %s at %s, replacing the PostGIS that was there.\n", what, rel.PGMajor, tree)
		running := "If fcdev is running, restart it"
		if runningPID > 0 {
			running = fmt.Sprintf("fcdev is running (pid %d): restart it", runningPID)
		}
		fmt.Fprintf(out, "%s (fcdev stop, then fcdev start): a session that loaded the old PostGIS keeps it until then.\n"+
			"Then run ALTER EXTENSION postgis UPDATE; in each database that has PostGIS, if its version changed.\n", running)
	}
	fmt.Fprintln(out, "Enable it in a database with: CREATE EXTENSION postgis;")
}

// fcdevRunningPID is the pid of a running `fcdev start` (from its default pid
// file), or 0.
func fcdevRunningPID() int {
	pid, err := readPIDFile(pidFilePath())
	if err != nil || !processAlive(pid) {
		return 0
	}
	return pid
}
