package main

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
)

// This file is fcdev's own Java: the ingestor runtime is a Java program, and
// fcdev never depends on whatever Java the machine happens to have. It
// installs a pinned Eclipse Temurin JRE into its cache directory on first use
// and runs the runtime with that, always.

// ingestorJavaMajor is the Java major version the ingestor runtime needs. It is
// the `maven.compiler.release` in ingestor/pom.xml (and the base image in
// ingestor/Dockerfile). Raise it when the runtime's requirement is raised:
// fcdev then installs the new major beside the old one on the next start.
const ingestorJavaMajor = 25

// adoptiumAPIBase is the Adoptium API origin. A variable so tests can point it
// at an httptest server.
var adoptiumAPIBase = "https://api.adoptium.net"

const (
	jreDirPrefix   = "jre-"
	jreMarkerName  = "fcdev-jre.json"
	jreTreeName    = "jre"
	jreInstallTemp = ".install-"

	// extractMaxBytes bounds what one archive may unpack to (a JRE is ~150 MB
	// unpacked), so a hostile or corrupt archive cannot fill the disk.
	extractMaxBytes = 1 << 30
)

// jreMarker is the file recording what a JRE install holds.
type jreMarker struct {
	Major     int       `json:"major"`
	Version   string    `json:"version"`   // full version, e.g. 25.0.5+8-LTS
	Release   string    `json:"release"`   // Adoptium release name, e.g. jdk-25.0.5+8
	Archive   string    `json:"archive"`   // the archive that was installed
	SHA256    string    `json:"sha256"`    // its verified checksum
	JavaHome  string    `json:"javaHome"`  // relative to the install dir
	Installed time.Time `json:"installed"` //
}

// jreInfo is a usable install.
type jreInfo struct {
	Major   int
	Version string
	Home    string // JAVA_HOME
	Java    string // the java executable
}

// jreManager installs and finds fcdev's JRE under root.
type jreManager struct {
	root   string
	major  int
	client *http.Client
	apiURL string
	goos   string
	goarch string
	log    *slog.Logger
}

func newJREManager(root string) *jreManager {
	return &jreManager{
		root:   root,
		major:  ingestorJavaMajor,
		client: &http.Client{Timeout: 15 * time.Minute},
		apiURL: adoptiumAPIBase,
		goos:   runtime.GOOS,
		goarch: runtime.GOARCH,
		log:    slog.Default(),
	}
}

// jreRootDir is where fcdev keeps its JREs: FC_INGESTOR_JRE_DIR, else
// <user-cache-dir>/flowcatalyst/fcdev-jre.
func jreRootDir() string {
	if d := os.Getenv("FC_INGESTOR_JRE_DIR"); d != "" {
		return d
	}
	if c, err := os.UserCacheDir(); err == nil {
		return filepath.Join(c, "flowcatalyst", "fcdev-jre")
	}
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, ".cache", "flowcatalyst", "fcdev-jre")
	}
	return filepath.Join(".", ".flowcatalyst-cache", "fcdev-jre")
}

func (m *jreManager) installDir(major int) string {
	return filepath.Join(m.root, jreDirPrefix+strconv.Itoa(major))
}

func javaExe(goos string) string {
	if goos == "windows" {
		return "java.exe"
	}
	return "java"
}

// readInstall reads the install of one major; ok is false when it is missing
// or not usable (no marker, wrong major, no java binary).
func (m *jreManager) readInstall(major int) (jreInfo, bool) {
	dir := m.installDir(major)
	b, err := os.ReadFile(filepath.Join(dir, jreMarkerName))
	if err != nil {
		return jreInfo{}, false
	}
	var mk jreMarker
	if json.Unmarshal(b, &mk) != nil || mk.Major != major || mk.JavaHome == "" {
		return jreInfo{}, false
	}
	home := filepath.Join(dir, filepath.FromSlash(mk.JavaHome))
	java := filepath.Join(home, "bin", javaExe(m.goos))
	if st, err := os.Stat(java); err != nil || st.IsDir() {
		return jreInfo{}, false
	}
	return jreInfo{Major: mk.Major, Version: mk.Version, Home: home, Java: java}, true
}

// installedMajors lists the majors with a usable install, ascending.
func (m *jreManager) installedMajors() []int {
	entries, err := os.ReadDir(m.root)
	if err != nil {
		return nil
	}
	var majors []int
	for _, e := range entries {
		n, ok := strings.CutPrefix(e.Name(), jreDirPrefix)
		if !ok || !e.IsDir() {
			continue
		}
		major, err := strconv.Atoi(n)
		if err != nil {
			continue
		}
		if _, ok := m.readInstall(major); ok {
			majors = append(majors, major)
		}
	}
	sort.Ints(majors)
	return majors
}

// jreInstallNotice is the single INFO line fcdev prints before installing the
// required Java. When an older major is installed it says so (the requirement
// was raised, the old install stays); otherwise it is the first-use line.
func jreInstallNotice(required int, installed []int) string {
	prev := 0
	for _, m := range installed {
		if m < required && m > prev {
			prev = m
		}
	}
	if prev > 0 {
		return fmt.Sprintf("ingestor runtime needs Java %d; installing it (the Java %d install stays until you remove it)", required, prev)
	}
	return fmt.Sprintf("ingestor runtime needs Java %d; installing Eclipse Temurin %d for fcdev's own use (one-time download)", required, required)
}

// Ensure returns the install of the required major, installing it first when
// it is missing. A failure leaves nothing half-installed behind.
func (m *jreManager) Ensure(ctx context.Context) (jreInfo, error) {
	if info, ok := m.readInstall(m.major); ok {
		return info, nil
	}
	m.log.Info(jreInstallNotice(m.major, m.installedMajors()))

	if err := os.MkdirAll(m.root, 0o755); err != nil {
		return jreInfo{}, fmt.Errorf("create %s: %w", m.root, err)
	}
	// A directory of this major that is not usable is a corrupt cache entry
	// (never left by an install: those are atomic), so replace it.
	if _, err := os.Stat(m.installDir(m.major)); err == nil {
		m.log.Warn("the cached Java is incomplete; replacing it", "dir", m.installDir(m.major))
		if err := os.RemoveAll(m.installDir(m.major)); err != nil {
			return jreInfo{}, err
		}
	}
	m.sweepStaleTemps()

	asset, err := m.lookup(ctx)
	if err != nil {
		return jreInfo{}, err
	}

	tmp, err := os.MkdirTemp(m.root, jreInstallTemp)
	if err != nil {
		return jreInfo{}, err
	}
	defer func() { _ = os.RemoveAll(tmp) }() // a no-op after a successful rename

	archive := filepath.Join(tmp, asset.Binary.Package.Name)
	if err := downloadVerified(ctx, m.client, asset.Binary.Package.Link, archive, asset.Binary.Package.Checksum); err != nil {
		return jreInfo{}, fmt.Errorf("download %s: %w", asset.Binary.Package.Name, err)
	}

	tree := filepath.Join(tmp, jreTreeName)
	if err := extractArchive(archive, tree); err != nil {
		return jreInfo{}, fmt.Errorf("unpack %s: %w", asset.Binary.Package.Name, err)
	}
	if err := os.Remove(archive); err != nil {
		return jreInfo{}, err
	}
	javaHome, err := findJavaHome(tree, m.goos)
	if err != nil {
		return jreInfo{}, err
	}
	rel, err := filepath.Rel(tmp, javaHome)
	if err != nil {
		return jreInfo{}, err
	}
	version := asset.Version.OpenJDKVersion
	if version == "" {
		version = asset.Version.Semver
	}
	mk := jreMarker{
		Major: m.major, Version: version, Release: asset.ReleaseName,
		Archive: asset.Binary.Package.Name, SHA256: strings.ToLower(asset.Binary.Package.Checksum),
		JavaHome: filepath.ToSlash(rel), Installed: time.Now().UTC(),
	}
	b, _ := json.MarshalIndent(mk, "", "  ")
	if err := os.WriteFile(filepath.Join(tmp, jreMarkerName), b, 0o600); err != nil {
		return jreInfo{}, err
	}

	if err := os.Rename(tmp, m.installDir(m.major)); err != nil {
		// A concurrent fcdev may have installed the same major first.
		if info, ok := m.readInstall(m.major); ok {
			return info, nil
		}
		return jreInfo{}, fmt.Errorf("finish install: %w", err)
	}
	info, ok := m.readInstall(m.major)
	if !ok {
		return jreInfo{}, fmt.Errorf("the installed Java at %s has no %s", m.installDir(m.major), javaExe(m.goos))
	}
	m.log.Info("installed Java for the ingestor runtime", "version", info.Version, "dir", m.installDir(m.major))
	return info, nil
}

// sweepStaleTemps removes install temp dirs an interrupted earlier run left
// (older than a day, so a concurrent install in progress is left alone).
func (m *jreManager) sweepStaleTemps() {
	entries, _ := os.ReadDir(m.root)
	for _, e := range entries {
		if !e.IsDir() || !strings.HasPrefix(e.Name(), jreInstallTemp) {
			continue
		}
		if info, err := e.Info(); err == nil && time.Since(info.ModTime()) > 24*time.Hour {
			_ = os.RemoveAll(filepath.Join(m.root, e.Name()))
		}
	}
}

// adoptiumAsset is the part of an Adoptium asset listing fcdev reads.
type adoptiumAsset struct {
	Binary struct {
		Package struct {
			Name     string `json:"name"`
			Link     string `json:"link"`
			Checksum string `json:"checksum"`
		} `json:"package"`
	} `json:"binary"`
	ReleaseName string `json:"release_name"`
	Version     struct {
		Semver         string `json:"semver"`
		OpenJDKVersion string `json:"openjdk_version"`
	} `json:"version"`
}

// adoptiumPlatform maps Go's GOOS/GOARCH to Adoptium's names.
func adoptiumPlatform(goos, goarch string) (osName, arch string, err error) {
	switch goos {
	case "darwin":
		osName = "mac"
	case "linux", "windows":
		osName = goos
	default:
		return "", "", fmt.Errorf("no Eclipse Temurin build for %s", goos)
	}
	switch goarch {
	case "amd64":
		arch = "x64"
	case "arm64":
		arch = "aarch64"
	default:
		return "", "", fmt.Errorf("no Eclipse Temurin build for %s/%s", goos, goarch)
	}
	return osName, arch, nil
}

// lookup asks Adoptium for the latest GA JRE of the required major for this
// machine: the archive's link and its SHA-256.
func (m *jreManager) lookup(ctx context.Context) (adoptiumAsset, error) {
	osName, arch, err := adoptiumPlatform(m.goos, m.goarch)
	if err != nil {
		return adoptiumAsset{}, err
	}
	q := url.Values{"architecture": {arch}, "image_type": {"jre"}, "os": {osName}, "vendor": {"eclipse"}}
	u := fmt.Sprintf("%s/v3/assets/latest/%d/hotspot?%s", strings.TrimRight(m.apiURL, "/"), m.major, q.Encode())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return adoptiumAsset{}, err
	}
	req.Header.Set("User-Agent", "fcdev")
	resp, err := m.client.Do(req)
	if err != nil {
		return adoptiumAsset{}, fmt.Errorf("ask Adoptium for Java %d: %w", m.major, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return adoptiumAsset{}, fmt.Errorf("ask Adoptium for Java %d: %s", m.major, resp.Status)
	}
	var assets []adoptiumAsset
	if err := json.NewDecoder(io.LimitReader(resp.Body, 8<<20)).Decode(&assets); err != nil {
		return adoptiumAsset{}, fmt.Errorf("read Adoptium's answer: %w", err)
	}
	for _, a := range assets {
		p := a.Binary.Package
		wantExt := ".tar.gz"
		if m.goos == "windows" {
			wantExt = ".zip"
		}
		if p.Link != "" && p.Checksum != "" && strings.HasSuffix(p.Name, wantExt) && !strings.Contains(a.ReleaseName, "-ea") {
			if strings.ContainsAny(p.Name, `/\`) {
				return adoptiumAsset{}, fmt.Errorf("unexpected archive name %q", p.Name)
			}
			return a, nil
		}
	}
	return adoptiumAsset{}, fmt.Errorf("adoptium lists no Java %d JRE (%s/%s) release", m.major, osName, arch)
}

// downloadVerified downloads url to dest and checks its SHA-256 (hex, either
// case) against want while it streams; on any failure dest is removed. An empty
// want is refused: nothing fcdev runs is fetched unverified.
func downloadVerified(ctx context.Context, client *http.Client, rawURL, dest, want string) error {
	want = strings.ToLower(strings.TrimSpace(want))
	if want == "" {
		return errors.New("no checksum to verify against")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "fcdev")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GET %s: %s", redactURL(rawURL), resp.Status)
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(dest, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	h := sha256.New()
	_, copyErr := io.Copy(io.MultiWriter(f, h), resp.Body)
	closeErr := f.Close()
	if err := errors.Join(copyErr, closeErr); err != nil {
		_ = os.Remove(dest)
		return err
	}
	if got := hex.EncodeToString(h.Sum(nil)); got != want {
		_ = os.Remove(dest)
		return fmt.Errorf("checksum mismatch (expected %s, got %s)", want, got)
	}
	return nil
}

// findJavaHome locates the directory holding bin/java under an extracted
// Temurin tree: <top>/Contents/Home on macOS, <top> elsewhere.
func findJavaHome(tree, goos string) (string, error) {
	entries, err := os.ReadDir(tree)
	if err != nil {
		return "", err
	}
	exe := javaExe(goos)
	cands := []string{tree}
	for _, e := range entries {
		if e.IsDir() {
			cands = append(cands, filepath.Join(tree, e.Name()), filepath.Join(tree, e.Name(), "Contents", "Home"))
		}
	}
	for _, c := range cands {
		if st, err := os.Stat(filepath.Join(c, "bin", exe)); err == nil && !st.IsDir() {
			return c, nil
		}
	}
	return "", fmt.Errorf("the archive holds no bin/%s", exe)
}

// extractArchive unpacks a .tar.gz or .zip into dest (created). It rejects
// absolute and parent-escaping paths, and links whose target leaves dest.
func extractArchive(archive, dest string) error {
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return err
	}
	absDest, err := filepath.EvalSymlinks(dest)
	if err != nil {
		return err
	}
	x := &extractor{dest: absDest}
	switch {
	case strings.HasSuffix(archive, ".zip"):
		return x.zip(archive)
	case strings.HasSuffix(archive, ".tar.gz"), strings.HasSuffix(archive, ".tgz"):
		return x.tarGz(archive)
	}
	return fmt.Errorf("unsupported archive %q", filepath.Base(archive))
}

type extractor struct {
	dest  string // absolute, symlink-resolved
	total int64
}

// target returns the absolute path an entry name would be written to, or an
// error when the name is absolute or leaves dest.
func (x *extractor) target(name string) (string, error) {
	n := filepath.FromSlash(strings.ReplaceAll(name, `\`, "/"))
	if n == "" || filepath.IsAbs(n) || strings.HasPrefix(name, "/") || filepath.VolumeName(n) != "" {
		return "", fmt.Errorf("unsafe path %q in archive", name)
	}
	p := filepath.Join(x.dest, n)
	if !within(x.dest, p) {
		return "", fmt.Errorf("unsafe path %q in archive", name)
	}
	return p, nil
}

func within(base, p string) bool {
	rel, err := filepath.Rel(base, p)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}

// parentOK checks that the entry's parent, with every symlink in it resolved,
// is still inside dest (a symlink entry cannot be used to write outside).
func (x *extractor) parentOK(p string) error {
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	real, err := filepath.EvalSymlinks(filepath.Dir(p))
	if err != nil {
		return err
	}
	if !within(x.dest, real) {
		return fmt.Errorf("unsafe path %q in archive (through a link)", p)
	}
	return nil
}

func (x *extractor) symlink(name, linkTarget string) error {
	p, err := x.target(name)
	if err != nil {
		return err
	}
	if filepath.IsAbs(linkTarget) || strings.HasPrefix(linkTarget, "/") {
		return fmt.Errorf("link %q points outside the archive (%q)", name, linkTarget)
	}
	if !within(x.dest, filepath.Join(filepath.Dir(p), filepath.FromSlash(linkTarget))) {
		return fmt.Errorf("link %q points outside the archive (%q)", name, linkTarget)
	}
	if err := x.parentOK(p); err != nil {
		return err
	}
	return os.Symlink(linkTarget, p)
}

func (x *extractor) file(name string, mode os.FileMode, r io.Reader) error {
	p, err := x.target(name)
	if err != nil {
		return err
	}
	if err := x.parentOK(p); err != nil {
		return err
	}
	f, err := os.OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode.Perm()|0o200)
	if err != nil {
		return err
	}
	n, err := io.Copy(f, io.LimitReader(r, extractMaxBytes-x.total+1))
	x.total += n
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return err
	}
	if x.total > extractMaxBytes {
		return errors.New("archive unpacks to more than the allowed size")
	}
	return nil
}

func (x *extractor) dir(name string) error {
	p, err := x.target(name)
	if err != nil {
		return err
	}
	if err := x.parentOK(p); err != nil {
		return err
	}
	return os.MkdirAll(p, 0o755)
}

func (x *extractor) tarGz(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer func() { _ = gz.Close() }()
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		switch h.Typeflag {
		case tar.TypeDir:
			err = x.dir(h.Name)
		case tar.TypeReg:
			err = x.file(h.Name, os.FileMode(h.Mode), tr)
		case tar.TypeSymlink:
			err = x.symlink(h.Name, h.Linkname)
		case tar.TypeLink:
			// A hard link is a second name for a file already extracted;
			// copy it, after checking both names are inside dest.
			err = x.hardlink(h.Name, h.Linkname, os.FileMode(h.Mode))
		case tar.TypeXGlobalHeader:
		default:
			err = fmt.Errorf("unsupported entry %q (type %c) in archive", h.Name, h.Typeflag)
		}
		if err != nil {
			return err
		}
	}
}

func (x *extractor) hardlink(name, linkname string, mode os.FileMode) error {
	src, err := x.target(linkname)
	if err != nil {
		return err
	}
	if err := x.parentOK(src); err != nil {
		return err
	}
	real, err := filepath.EvalSymlinks(src)
	if err != nil || !within(x.dest, real) {
		return fmt.Errorf("link %q points outside the archive (%q)", name, linkname)
	}
	in, err := os.Open(real)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()
	return x.file(name, mode, in)
}

func (x *extractor) zip(path string) error {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return err
	}
	defer func() { _ = zr.Close() }()
	for _, zf := range zr.File {
		if zf.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("unsupported link %q in archive", zf.Name)
		}
		if zf.FileInfo().IsDir() {
			if err := x.dir(strings.TrimRight(zf.Name, "/")); err != nil {
				return err
			}
			continue
		}
		rc, err := zf.Open()
		if err != nil {
			return err
		}
		err = x.file(zf.Name, zf.Mode(), rc)
		rc.Close()
		if err != nil {
			return err
		}
	}
	return nil
}
