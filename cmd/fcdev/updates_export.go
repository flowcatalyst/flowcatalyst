package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/appupdate"
)

// An `expo export` directory, as @expo/cli 58 writes it
// (export/createMetadataJson.js): metadata.json {version: 0, bundler: "metro",
// fileMetadata: {<platform>: {bundle, assets: [{path, ext}]}}}, the bundle and
// asset files under the directory. It carries no runtime version.

// exportMetadata is metadata.json.
type exportMetadata struct {
	Version      *int                              `json:"version"`
	Bundler      string                            `json:"bundler"`
	FileMetadata map[string]exportPlatformMetadata `json:"fileMetadata"`
}

type exportPlatformMetadata struct {
	Bundle string        `json:"bundle"`
	Assets []exportAsset `json:"assets"`
}

type exportAsset struct {
	Path string `json:"path"`
	Ext  string `json:"ext"`
}

// exportFile is one file of the export, hashed.
type exportFile struct {
	Path   string // absolute
	SHA256 string
	Size   int64
}

// exportPlatform is what one platform of the export publishes.
type exportPlatform struct {
	Platform string
	Launch   exportFile
	Assets   []exportAssetFile
}

type exportAssetFile struct {
	exportFile
	Ext string
}

// exportBundle is a read and checked export.
type exportBundle struct {
	Dir       string
	Platforms []exportPlatform
	// Files is every distinct file, by SHA-256.
	Files map[string]exportFile
	// ExpoConfig is the public app config when one was found or given.
	ExpoConfig json.RawMessage
}

// readExport reads and checks an `expo export` directory for platforms (nil:
// every native platform in it). It fails, publishing nothing, on anything
// malformed: no metadata.json, a version or bundler it does not know, a
// platform missing, a path outside the directory, a missing or oversized file,
// an asset without a usable extension.
func readExport(dir string, platforms []string, maxFileBytes int64) (*exportBundle, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(filepath.Join(abs, "metadata.json"))
	if err != nil {
		return nil, fmt.Errorf("%s is not an expo export: %w (run `npx expo export` first)", dir, err)
	}
	var meta exportMetadata
	dec := json.NewDecoder(bytes.NewReader(raw))
	if err := dec.Decode(&meta); err != nil {
		return nil, fmt.Errorf("metadata.json: %w", err)
	}
	if meta.Version == nil || *meta.Version != 0 {
		return nil, errors.New("metadata.json: version must be 0 (the format @expo/cli writes); this export's format is not supported")
	}
	if meta.Bundler != "metro" {
		return nil, fmt.Errorf("metadata.json: bundler %q is not supported (metro only)", meta.Bundler)
	}
	if len(platforms) == 0 {
		for p := range meta.FileMetadata {
			if appupdate.ValidPlatform(p) {
				platforms = append(platforms, p)
			}
		}
		sort.Strings(platforms)
		if len(platforms) == 0 {
			return nil, errors.New("metadata.json names no android or ios bundle")
		}
	}
	b := &exportBundle{Dir: abs, Files: map[string]exportFile{}}
	for _, p := range platforms {
		if !appupdate.ValidPlatform(p) {
			return nil, fmt.Errorf("platform %q: must be android or ios", p)
		}
		pm, ok := meta.FileMetadata[p]
		if !ok {
			return nil, fmt.Errorf("metadata.json has no %s bundle (export with --platform %s)", p, p)
		}
		if pm.Bundle == "" {
			return nil, fmt.Errorf("metadata.json: %s has no bundle", p)
		}
		launch, err := b.file(pm.Bundle, maxFileBytes)
		if err != nil {
			return nil, fmt.Errorf("%s bundle: %w", p, err)
		}
		ep := exportPlatform{Platform: p, Launch: launch}
		if len(pm.Assets)+1 > appupdate.MaxAssets {
			return nil, fmt.Errorf("%s: %d files; an update may have at most %d", p, len(pm.Assets)+1, appupdate.MaxAssets)
		}
		for i, a := range pm.Assets {
			if _, ok := appupdate.NormalizeFileExtension(a.Ext); !ok {
				return nil, fmt.Errorf("%s asset %d (%s): extension %q is not 1-16 letters and digits", p, i, a.Path, a.Ext)
			}
			f, err := b.file(a.Path, maxFileBytes)
			if err != nil {
				return nil, fmt.Errorf("%s asset %d: %w", p, i, err)
			}
			ep.Assets = append(ep.Assets, exportAssetFile{exportFile: f, Ext: a.Ext})
		}
		b.Platforms = append(b.Platforms, ep)
	}
	return b, nil
}

// file hashes one file named relative to the export directory.
func (b *exportBundle) file(rel string, maxFileBytes int64) (exportFile, error) {
	if rel == "" || filepath.IsAbs(rel) || strings.Contains(rel, "\\") {
		return exportFile{}, fmt.Errorf("path %q is not relative to the export", rel)
	}
	p := filepath.Join(b.Dir, filepath.FromSlash(rel))
	if r, err := filepath.Rel(b.Dir, p); err != nil || r == ".." || strings.HasPrefix(r, ".."+string(filepath.Separator)) {
		return exportFile{}, fmt.Errorf("path %q leaves the export directory", rel)
	}
	st, err := os.Lstat(p)
	if err != nil {
		return exportFile{}, err
	}
	if !st.Mode().IsRegular() {
		return exportFile{}, fmt.Errorf("%s is not a regular file", rel)
	}
	if st.Size() == 0 {
		return exportFile{}, fmt.Errorf("%s is empty", rel)
	}
	if st.Size() > maxFileBytes {
		return exportFile{}, fmt.Errorf("%s is %d bytes; a file may be at most %d", rel, st.Size(), maxFileBytes)
	}
	f, err := os.Open(p)
	if err != nil {
		return exportFile{}, err
	}
	defer func() { _ = f.Close() }()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return exportFile{}, err
	}
	ef := exportFile{Path: p, SHA256: hex.EncodeToString(h.Sum(nil)), Size: st.Size()}
	b.Files[ef.SHA256] = ef
	return ef, nil
}

// readExpoConfig reads the public app config (`npx expo config --type public
// --json`): path when given, else <export>/expoConfig.json when present; nil
// when neither.
func readExpoConfig(exportDir, path string) (json.RawMessage, string, error) {
	explicit := path != ""
	if !explicit {
		path = filepath.Join(exportDir, "expoConfig.json")
		if !fileExists(path) {
			return nil, "", nil
		}
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, "", err
	}
	raw = bytes.TrimSpace(raw)
	var obj map[string]json.RawMessage
	if len(raw) == 0 || raw[0] != '{' || json.Unmarshal(raw, &obj) != nil {
		return nil, "", fmt.Errorf("%s is not a JSON object (write it with `npx expo config --type public --json`)", path)
	}
	if len(raw) > appupdate.MaxExpoClientBytes {
		return nil, "", fmt.Errorf("%s is %d bytes; the app config may be at most %d", path, len(raw), appupdate.MaxExpoClientBytes)
	}
	return raw, path, nil
}

// runtimeVersionFor settles one platform's runtime version: the flag for
// that platform, else the common flag, else the app config's
// <platform>.runtimeVersion or runtimeVersion when it is a plain string. A
// runtime-version policy object must be resolved first; no runtime version at
// all is an error (an export does not carry one).
func runtimeVersionFor(platform, perPlatformFlag, flag string, expoConfig json.RawMessage) (string, error) {
	v := firstNonEmpty(perPlatformFlag, flag)
	if v == "" && len(expoConfig) > 0 {
		var cfg struct {
			RuntimeVersion json.RawMessage            `json:"runtimeVersion"`
			Android        map[string]json.RawMessage `json:"android"`
			IOS            map[string]json.RawMessage `json:"ios"`
		}
		_ = json.Unmarshal(expoConfig, &cfg)
		pick := cfg.RuntimeVersion
		if pp := map[string]map[string]json.RawMessage{"android": cfg.Android, "ios": cfg.IOS}[platform]; pp != nil && len(pp["runtimeVersion"]) > 0 {
			pick = pp["runtimeVersion"]
		}
		if len(pick) > 0 {
			var s string
			if json.Unmarshal(pick, &s) != nil {
				return "", fmt.Errorf("%s: the app config's runtimeVersion is a policy, not a value; resolve it with `npx expo-updates runtimeversion:resolve --platform %s` and pass --runtime-version", platform, platform)
			}
			v = s
		}
	}
	if v == "" {
		return "", fmt.Errorf("%s: no runtime version: an expo export does not record one; pass --runtime-version (or --runtime-version-%s), or --expo-config with a runtimeVersion string", platform, platform)
	}
	if !appupdate.ValidRuntimeVersion(v) {
		return "", fmt.Errorf("%s: runtime version %q is not valid (letters, digits and . _ : + -, at most 128)", platform, v)
	}
	return v, nil
}
