package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/appupdate"
)

// writeExport writes an export as @expo/cli 58 lays it out: metadata.json,
// _expo/static/js/<platform>/entry-<hash>.hbc and assets/<md5>.
func writeExport(t *testing.T, dir string, meta any, files map[string]string) {
	t.Helper()
	b, err := json.Marshal(meta)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "metadata.json"), b, 0o600))
	for rel, body := range files {
		p := filepath.Join(dir, filepath.FromSlash(rel))
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o700))
		require.NoError(t, os.WriteFile(p, []byte(body), 0o600))
	}
}

func goodMeta() map[string]any {
	return map[string]any{
		"version": 0, "bundler": "metro",
		"fileMetadata": map[string]any{
			"android": map[string]any{
				"bundle": "_expo/static/js/android/entry-1a2b.hbc",
				"assets": []map[string]string{{"path": "assets/aaaa", "ext": "png"}, {"path": "assets/bbbb", "ext": "ttf"}},
			},
			"ios": map[string]any{
				"bundle": "_expo/static/js/ios/entry-3c4d.hbc",
				"assets": []map[string]string{{"path": "assets/aaaa", "ext": "png"}},
			},
		},
	}
}

func goodFiles() map[string]string {
	return map[string]string{
		"_expo/static/js/android/entry-1a2b.hbc": "android bundle",
		"_expo/static/js/ios/entry-3c4d.hbc":     "ios bundle",
		"assets/aaaa":                            "png bytes",
		"assets/bbbb":                            "ttf bytes",
	}
}

func TestReadExport(t *testing.T) {
	dir := t.TempDir()
	writeExport(t, dir, goodMeta(), goodFiles())
	exp, err := readExport(dir, nil, appupdate.DefaultMaxFileBytes)
	require.NoError(t, err)
	require.Len(t, exp.Platforms, 2)
	assert.Equal(t, "android", exp.Platforms[0].Platform)
	assert.Len(t, exp.Platforms[0].Assets, 2)
	assert.Equal(t, "png", exp.Platforms[0].Assets[0].Ext)
	assert.Len(t, exp.Files, 4, "a file two platforms share is one file")

	exp, err = readExport(dir, []string{"ios"}, appupdate.DefaultMaxFileBytes)
	require.NoError(t, err)
	require.Len(t, exp.Platforms, 1)
	assert.Len(t, exp.Files, 2)
}

// A malformed export publishes nothing: every case fails before anything is sent.
func TestReadExport_Malformed(t *testing.T) {
	mutate := func(f func(m map[string]any, files map[string]string)) (map[string]any, map[string]string) {
		m, fs := goodMeta(), goodFiles()
		f(m, fs)
		return m, fs
	}
	android := func(m map[string]any) map[string]any {
		return m["fileMetadata"].(map[string]any)["android"].(map[string]any)
	}
	type exportCase struct {
		meta  any
		files map[string]string
		os    []string
		want  string
	}
	cases := map[string]exportCase{}
	add := func(name string, meta any, files map[string]string, os []string, want string) {
		cases[name] = exportCase{meta, files, os, want}
	}
	m, f := mutate(func(m map[string]any, _ map[string]string) { m["version"] = 1 })
	add("version 1", m, f, nil, "version must be 0")
	m, f = mutate(func(m map[string]any, _ map[string]string) { delete(m, "version") })
	add("no version", m, f, nil, "version must be 0")
	m, f = mutate(func(m map[string]any, _ map[string]string) { m["bundler"] = "webpack" })
	add("bundler", m, f, nil, "not supported")
	m, f = mutate(func(m map[string]any, _ map[string]string) {
		m["fileMetadata"] = map[string]any{"web": map[string]any{}}
	})
	add("web only", m, f, nil, "no android or ios")
	m, f = mutate(func(m map[string]any, _ map[string]string) { delete(m["fileMetadata"].(map[string]any), "ios") })
	add("platform missing", m, f, []string{"ios"}, "no ios bundle")
	m, f = mutate(func(_ map[string]any, files map[string]string) { delete(files, "assets/bbbb") })
	add("file missing", m, f, nil, "no such file")
	m, f = mutate(func(_ map[string]any, files map[string]string) { files["assets/bbbb"] = "" })
	add("empty file", m, f, nil, "is empty")
	m, f = mutate(func(m map[string]any, _ map[string]string) {
		android(m)["assets"] = []map[string]string{{"path": "../outside", "ext": "png"}}
	})
	add("traversal", m, f, nil, "leaves the export")
	m, f = mutate(func(m map[string]any, _ map[string]string) {
		android(m)["assets"] = []map[string]string{{"path": "/etc/hosts", "ext": "png"}}
	})
	add("absolute", m, f, nil, "not relative")
	m, f = mutate(func(m map[string]any, _ map[string]string) {
		android(m)["assets"] = []map[string]string{{"path": "assets/aaaa", "ext": "p/ng"}}
	})
	add("bad ext", m, f, nil, "extension")
	m, f = mutate(func(m map[string]any, _ map[string]string) { android(m)["bundle"] = "" })
	add("no bundle", m, f, []string{"android"}, "has no bundle")

	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			writeExport(t, dir, c.meta, c.files)
			_, err := readExport(dir, c.os, appupdate.DefaultMaxFileBytes)
			require.Error(t, err)
			assert.Contains(t, err.Error(), c.want)
		})
	}

	_, err := readExport(t.TempDir(), nil, appupdate.DefaultMaxFileBytes)
	require.ErrorContains(t, err, "not an expo export")

	dir := t.TempDir()
	writeExport(t, dir, goodMeta(), goodFiles())
	_, err = readExport(dir, nil, 5)
	require.ErrorContains(t, err, "at most 5")

	dir = t.TempDir()
	writeExport(t, dir, goodMeta(), goodFiles())
	require.NoError(t, os.Remove(filepath.Join(dir, "assets", "bbbb")))
	require.NoError(t, os.Symlink("/etc/hosts", filepath.Join(dir, "assets", "bbbb")))
	_, err = readExport(dir, nil, appupdate.DefaultMaxFileBytes)
	require.ErrorContains(t, err, "not a regular file")
}

func TestRuntimeVersionFor(t *testing.T) {
	cfg := json.RawMessage(`{"name":"x","runtimeVersion":"1.2.0","ios":{"runtimeVersion":"1.2.1"}}`)
	v, err := runtimeVersionFor("android", "", "", cfg)
	require.NoError(t, err)
	assert.Equal(t, "1.2.0", v)
	v, err = runtimeVersionFor("ios", "", "", cfg)
	require.NoError(t, err)
	assert.Equal(t, "1.2.1", v, "the platform's own runtimeVersion wins")
	v, err = runtimeVersionFor("ios", "", "9.9.9", cfg)
	require.NoError(t, err)
	assert.Equal(t, "9.9.9", v, "a flag wins")
	v, err = runtimeVersionFor("ios", "3.0.0", "9.9.9", cfg)
	require.NoError(t, err)
	assert.Equal(t, "3.0.0", v, "the per-platform flag wins")

	_, err = runtimeVersionFor("android", "", "", nil)
	require.ErrorContains(t, err, "no runtime version")
	_, err = runtimeVersionFor("android", "", "", json.RawMessage(`{"name":"x"}`))
	require.ErrorContains(t, err, "no runtime version")
	_, err = runtimeVersionFor("android", "", "", json.RawMessage(`{"runtimeVersion":{"policy":"fingerprint"}}`))
	require.ErrorContains(t, err, "runtimeversion:resolve")
	_, err = runtimeVersionFor("android", "", "1 0", nil)
	require.ErrorContains(t, err, "not valid")
}

func TestReadExpoConfig(t *testing.T) {
	dir := t.TempDir()
	raw, path, err := readExpoConfig(dir, "")
	require.NoError(t, err)
	assert.Nil(t, raw)
	assert.Empty(t, path)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "expoConfig.json"), []byte(` {"name":"Driver"} `), 0o600))
	raw, path, err = readExpoConfig(dir, "")
	require.NoError(t, err)
	assert.JSONEq(t, `{"name":"Driver"}`, string(raw))
	assert.True(t, strings.HasSuffix(path, "expoConfig.json"))
	bad := filepath.Join(dir, "bad.json")
	require.NoError(t, os.WriteFile(bad, []byte(`[1]`), 0o600))
	_, _, err = readExpoConfig(dir, bad)
	require.ErrorContains(t, err, "not a JSON object")
}

func TestNewSigningKeyAndCertificate(t *testing.T) {
	keyPEM, certPEM, err := newSigningKeyAndCertificate("acme updates", 10)
	require.NoError(t, err)
	key, err := appupdate.ParsePrivateKeyPEM(keyPEM)
	require.NoError(t, err)
	cert, err := appupdate.CheckCodeSigningCertificate(certPEM, &key.PublicKey, time.Now())
	require.NoError(t, err)
	assert.Equal(t, "acme updates", cert.Subject.CommonName)

	dir := t.TempDir()
	k1, c1, err := ensureDevSigningKey(dir)
	require.NoError(t, err)
	k2, c2, err := ensureDevSigningKey(dir)
	require.NoError(t, err)
	assert.Equal(t, k1, k2)
	assert.Equal(t, c1, c2)
	st, err := os.Stat(k1)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), st.Mode().Perm())
}
