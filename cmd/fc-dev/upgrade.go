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
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// defaultUpgradeRepo is the GitHub repo that publishes fc-dev releases.
// Overridable via FC_DEV_UPGRADE_REPO so the command can be exercised against a
// fork. (`flowcatalyst-go` redirects here — this is the canonical name.)
const defaultUpgradeRepo = "flowcatalyst/flowcatalyst"

// upgradeTagPrefix is the release-tag namespace for the developer binary. The
// repo also tags typescript-sdk/v* and laravel-sdk/v*, so filtering by this
// prefix is mandatory — /releases/latest would return whichever line published
// last, not necessarily fc-dev.
const upgradeTagPrefix = "fc-dev/v"

// newUpgradeCmd builds the `fc-dev upgrade` self-update command. It downloads
// the latest release for the running platform, verifies its SHA256, and
// atomically replaces the live binary. Same artifacts the install.sh /
// install.ps1 scripts use.
func newUpgradeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "upgrade",
		Short: "Update fc-dev to the latest release",
		Long: `Download the latest fc-dev release for this platform from GitHub,
verify its SHA256, and atomically replace the running binary.

These are the same release artifacts that back the install.sh / install.ps1
scripts. Set FC_DEV_UPGRADE_REPO to point at a fork.`,
		Args: cobra.NoArgs,
		RunE: runUpgrade,
	}
	cmd.Flags().Bool("check", false, "only report whether a newer release exists; don't install")
	cmd.Flags().Bool("force", false, "reinstall even if already on the latest version")
	return cmd
}

func runUpgrade(cmd *cobra.Command, _ []string) error {
	check, _ := cmd.Flags().GetBool("check")
	force, _ := cmd.Flags().GetBool("force")
	out := cmd.OutOrStdout()

	repo := envStrDefault("FC_DEV_UPGRADE_REPO", defaultUpgradeRepo)
	current := version()

	fmt.Fprintf(out, "current version: %s\n", current)
	fmt.Fprintln(out, "checking for updates…")

	rel, err := latestRelease(cmd.Context(), repo)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "latest version:  %s\n", rel.version)

	cmp := compareSemver(rel.version, current)
	updateAvailable := cmp > 0

	if check {
		if updateAvailable {
			fmt.Fprintf(out, "update available: %s → %s (run `fc-dev upgrade`)\n", current, rel.version)
		} else {
			fmt.Fprintln(out, "fc-dev is up to date.")
		}
		return nil
	}

	if !updateAvailable && !force {
		fmt.Fprintln(out, "fc-dev is already up to date. Use --force to reinstall.")
		return nil
	}

	goos, goarch := runtime.GOOS, runtime.GOARCH
	ext := "tar.gz"
	binName := "fc-dev"
	if goos == "windows" {
		ext = "zip"
		binName = "fc-dev.exe"
	}
	stem := fmt.Sprintf("fc-dev-v%s-%s-%s", rel.version, goos, goarch)
	assetName := stem + "." + ext

	assetURL, ok := rel.assets[assetName]
	if !ok {
		return fmt.Errorf("release %s has no asset for this platform (%s/%s — looked for %s)",
			rel.version, goos, goarch, assetName)
	}

	fmt.Fprintf(out, "downloading %s…\n", assetName)
	archiveBytes, err := httpGet(cmd.Context(), assetURL)
	if err != nil {
		return fmt.Errorf("download %s: %w", assetName, err)
	}

	if shaURL, has := rel.assets[assetName+".sha256"]; has {
		shaBytes, err := httpGet(cmd.Context(), shaURL)
		if err != nil {
			return fmt.Errorf("download checksum: %w", err)
		}
		if err := verifySHA256(archiveBytes, shaBytes); err != nil {
			return err
		}
		fmt.Fprintln(out, "sha256 verified.")
	} else {
		fmt.Fprintln(out, "warning: no sha256 sidecar published for this asset — skipping checksum verification")
	}

	innerPath := stem + "/" + binName
	newBin, err := extractBinary(archiveBytes, ext, innerPath)
	if err != nil {
		return err
	}

	dest, err := selfPath()
	if err != nil {
		return err
	}
	if err := replaceBinary(dest, newBin); err != nil {
		return err
	}

	fmt.Fprintf(out, "✓ upgraded fc-dev %s → %s (%s)\n", current, rel.version, dest)
	return nil
}

// release is a single fc-dev GitHub release, reduced to what upgrade needs.
type release struct {
	version string            // clean X.Y.Z (prefix stripped)
	assets  map[string]string // asset filename → browser_download_url
}

// latestRelease returns the highest fc-dev/vX.Y.Z release published for repo.
// Anonymous GitHub API requests are rate-limited to 60/hour per IP, which is
// ample for a manual upgrade.
func latestRelease(ctx context.Context, repo string) (*release, error) {
	api := fmt.Sprintf("https://api.github.com/repos/%s/releases?per_page=100", repo)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, api, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "fc-dev-upgrade")
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := upgradeHTTPClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("query GitHub API: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("GitHub API %s returned %s: %s", api, resp.Status, strings.TrimSpace(string(body)))
	}

	var raw []struct {
		TagName    string `json:"tag_name"`
		Draft      bool   `json:"draft"`
		Prerelease bool   `json:"prerelease"`
		Assets     []struct {
			Name string `json:"name"`
			URL  string `json:"browser_download_url"`
		} `json:"assets"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("decode GitHub API response: %w", err)
	}

	var best *release
	for _, r := range raw {
		if r.Draft || r.Prerelease || !strings.HasPrefix(r.TagName, upgradeTagPrefix) {
			continue
		}
		ver := strings.TrimPrefix(r.TagName, upgradeTagPrefix)
		if !isCleanSemver(ver) {
			continue
		}
		if best != nil && compareSemver(ver, best.version) <= 0 {
			continue
		}
		assets := make(map[string]string, len(r.Assets))
		for _, a := range r.Assets {
			assets[a.Name] = a.URL
		}
		best = &release{version: ver, assets: assets}
	}
	if best == nil {
		return nil, fmt.Errorf("no %sX.Y.Z releases found for %s", upgradeTagPrefix, repo)
	}
	return best, nil
}

func upgradeHTTPClient() *http.Client {
	// One client for both the (tiny) API call and the (few-MB) asset download.
	// Generous timeout so slow links don't abort a legitimate download.
	return &http.Client{Timeout: 5 * time.Minute}
}

func httpGet(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "fc-dev-upgrade")
	resp, err := upgradeHTTPClient().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s returned %s", url, resp.Status)
	}
	return io.ReadAll(resp.Body)
}

// verifySHA256 checks data against a `<hex>  <filename>` sidecar (sha256sum /
// shasum format — only the leading hex digest matters).
func verifySHA256(data, sidecar []byte) error {
	fields := strings.Fields(string(sidecar))
	if len(fields) == 0 {
		return errors.New("empty sha256 sidecar")
	}
	expected := strings.ToLower(fields[0])
	sum := sha256.Sum256(data)
	actual := hex.EncodeToString(sum[:])
	if expected != actual {
		return fmt.Errorf("checksum mismatch (expected %s, got %s)", expected, actual)
	}
	return nil
}

// extractBinary pulls the single file at innerPath out of the release archive.
func extractBinary(archive []byte, ext, innerPath string) ([]byte, error) {
	switch ext {
	case "tar.gz":
		return extractFromTarGz(archive, innerPath)
	case "zip":
		return extractFromZip(archive, innerPath)
	default:
		return nil, fmt.Errorf("unsupported archive type %q", ext)
	}
}

func extractFromTarGz(archive []byte, innerPath string) ([]byte, error) {
	gz, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return nil, fmt.Errorf("open gzip: %w", err)
	}
	defer func() { _ = gz.Close() }()
	want := filepath.ToSlash(filepath.Clean(innerPath))
	base := path.Base(want)
	var fallback []byte
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read tar: %w", err)
		}
		name := filepath.ToSlash(filepath.Clean(hdr.Name))
		if name == want {
			return io.ReadAll(tr)
		}
		// Fallback: match by basename so a flat archive (binary at the root,
		// no <stem>/ prefix) still works regardless of how it was packaged.
		if fallback == nil && hdr.Typeflag == tar.TypeReg && path.Base(name) == base {
			if fallback, err = io.ReadAll(tr); err != nil {
				return nil, fmt.Errorf("read tar: %w", err)
			}
		}
	}
	if fallback != nil {
		return fallback, nil
	}
	return nil, fmt.Errorf("binary %q not found in archive", innerPath)
}

func extractFromZip(archive []byte, innerPath string) ([]byte, error) {
	zr, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
	if err != nil {
		return nil, fmt.Errorf("open zip: %w", err)
	}
	want := filepath.ToSlash(filepath.Clean(innerPath))
	base := path.Base(want)
	var fallback *zip.File
	for _, f := range zr.File {
		name := filepath.ToSlash(filepath.Clean(f.Name))
		if name == want {
			return readZipEntry(f)
		}
		// Fallback: match by basename (flat archive — see extractFromTarGz).
		if fallback == nil && !f.FileInfo().IsDir() && path.Base(name) == base {
			fallback = f
		}
	}
	if fallback != nil {
		return readZipEntry(fallback)
	}
	return nil, fmt.Errorf("binary %q not found in archive", innerPath)
}

// readZipEntry reads one zip file entry fully.
func readZipEntry(f *zip.File) ([]byte, error) {
	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer func() { _ = rc.Close() }()
	return io.ReadAll(rc)
}

// selfPath resolves the on-disk path of the running binary, following symlinks
// so we replace the real file rather than a launcher symlink.
func selfPath() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("locate running binary: %w", err)
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	return exe, nil
}

// replaceBinary atomically swaps the binary at dest for newBin. The temp file
// is created in dest's directory so the rename stays on one filesystem (atomic).
func replaceBinary(dest string, newBin []byte) error {
	dir := filepath.Dir(dest)
	tmp, err := os.CreateTemp(dir, ".fc-dev-upgrade-*")
	if err != nil {
		return fmt.Errorf("cannot write to %s — re-run with elevated permissions or use the install script: %w", dir, err)
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }() // harmless no-op once the rename consumes it

	if _, err := tmp.Write(newBin); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write new binary: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("flush new binary: %w", err)
	}
	if err := os.Chmod(tmpName, 0o755); err != nil { // #nosec G302 -- upgrade target is an executable binary; 0755 is required
		return fmt.Errorf("chmod new binary: %w", err)
	}

	if runtime.GOOS == "windows" {
		// Windows refuses to overwrite a running .exe. Move the live binary
		// aside, slot the new one in, then best-effort delete the aside copy
		// (it stays locked until this process exits — Windows cleans it up,
		// or the next upgrade's os.Remove does).
		old := dest + ".old"
		_ = os.Remove(old)
		if err := os.Rename(dest, old); err != nil {
			return fmt.Errorf("move current binary aside: %w", err)
		}
		if err := os.Rename(tmpName, dest); err != nil {
			_ = os.Rename(old, dest) // roll back
			return fmt.Errorf("install new binary: %w", err)
		}
		_ = os.Remove(old)
		return nil
	}

	if err := os.Rename(tmpName, dest); err != nil {
		return fmt.Errorf("install new binary to %s — re-run with elevated permissions: %w", dest, err)
	}
	return nil
}

// isCleanSemver reports whether s is exactly X.Y.Z with numeric parts (no
// prerelease/build suffix).
func isCleanSemver(s string) bool {
	parts := strings.Split(s, ".")
	if len(parts) != 3 {
		return false
	}
	for _, p := range parts {
		if p == "" {
			return false
		}
		if _, err := strconv.Atoi(p); err != nil {
			return false
		}
	}
	return true
}

// compareSemver returns -1 if a<b, 0 if a==b, +1 if a>b for clean X.Y.Z inputs.
// Non-numeric parts collapse to 0 — a safe fallback since callers filter to
// clean semver before comparing release tags.
func compareSemver(a, b string) int {
	pa := strings.SplitN(a, ".", 3)
	pb := strings.SplitN(b, ".", 3)
	for i := 0; i < 3; i++ {
		na, nb := 0, 0
		if i < len(pa) {
			na, _ = strconv.Atoi(pa[i])
		}
		if i < len(pb) {
			nb, _ = strconv.Atoi(pb[i])
		}
		if na != nb {
			if na < nb {
				return -1
			}
			return 1
		}
	}
	return 0
}
