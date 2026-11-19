package apk_test

import (
	"archive/zip"
	"bytes"
	"errors"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/appupdate/apk"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/appupdate/apk/apktest"
)

var signers = sync.OnceValues(func() (map[string]*apktest.Signer, error) {
	out := map[string]*apktest.Signer{}
	for _, cn := range []string{"acme driver-app", "Android Debug"} {
		s, err := apktest.NewSigner(cn)
		if err != nil {
			return nil, err
		}
		out[cn] = s
	}
	return out, nil
})

func signer(t testing.TB, cn string) *apktest.Signer {
	t.Helper()
	m, err := signers()
	require.NoError(t, err)
	return m[cn]
}

func expoOptions(t testing.TB) apktest.Options {
	return apktest.Options{
		Package: "com.example.driver", VersionCode: 42, VersionName: "1.4.0", MinSDK: 24, TargetSDK: 36,
		RuntimeVersion: "1.4.0", UpdateURL: "https://platform.example.com/app-updates/acme/driver-app/manifest",
		Channel: "production", KeyID: "main", Signer: signer(t, "acme driver-app"),
	}
}

func build(t testing.TB, o apktest.Options) []byte {
	t.Helper()
	b, err := apktest.Build(o)
	require.NoError(t, err)
	return b
}

func parse(b []byte) (*apk.Info, error) { return apk.Parse(bytes.NewReader(b), int64(len(b))) }

func requireCode(t *testing.T, err error, code string) {
	t.Helper()
	var e *apk.Error
	require.True(t, errors.As(err, &e), "want a refusal %s, got %v", code, err)
	assert.Equal(t, code, e.Code, e.Message)
}

func TestParseReadsWhatExpoWrites(t *testing.T) {
	t.Parallel()
	o := expoOptions(t)
	info, err := parse(build(t, o))
	require.NoError(t, err)
	assert.Equal(t, "com.example.driver", info.Package)
	assert.Equal(t, int64(42), info.VersionCode)
	assert.Equal(t, "1.4.0", info.VersionName)
	assert.Equal(t, 24, info.MinSDK)
	assert.Equal(t, 36, info.TargetSDK)
	assert.False(t, info.Debuggable)
	assert.Equal(t, "1.4.0", info.RuntimeVersion)
	assert.Equal(t, "resource", info.RuntimeVersionSource)
	assert.Equal(t, o.UpdateURL, info.UpdateURL)
	assert.Equal(t, "production", info.Channel())
	assert.Equal(t, "main", info.UpdateCodeSigningKeyID)
	assert.Equal(t, []int{2}, info.Signing.Schemes)
	assert.Equal(t, o.Signer.CertificateSHA256(), info.Signing.CertificateSHA256)
	assert.Equal(t, "CN=acme driver-app", info.Signing.CertificateSubject)
	assert.False(t, info.Signing.DebugCertificate)
}

func TestParseRuntimeVersionSources(t *testing.T) {
	t.Parallel()
	literal := expoOptions(t)
	literal.RuntimeVersionLiteral = true
	prefixed := expoOptions(t)
	prefixed.RuntimeVersion = "string:2.0"
	fingerprint := expoOptions(t)
	fingerprint.Fingerprint = "8b3dd89ca56b3f5d7b90f20f04ac95d2a6f34bf4\n"
	none := apktest.Options{VersionCode: 1, Signer: signer(t, "acme driver-app")}
	for _, c := range []struct {
		name         string
		o            apktest.Options
		value, from  string
		withExpoData bool
	}{
		{"literal", literal, "1.4.0", "manifest", true},
		{"string: prefix removed", prefixed, "2.0", "resource", true},
		// Read whole, as expo-updates reads it: not trimmed.
		{"fingerprint", fingerprint, "8b3dd89ca56b3f5d7b90f20f04ac95d2a6f34bf4\n", "fingerprint", true},
		{"no expo-updates", none, "", "", false},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			info, err := parse(build(t, c.o))
			require.NoError(t, err)
			assert.Equal(t, c.value, info.RuntimeVersion)
			assert.Equal(t, c.from, info.RuntimeVersionSource)
			if !c.withExpoData {
				assert.Empty(t, info.UpdateURL)
				assert.Nil(t, info.UpdateRequestHeaders)
				assert.Empty(t, info.Channel())
				assert.Empty(t, info.UpdateCodeSigningKeyID)
			}
		})
	}
}

func TestParseSchemesDebuggableAndDebugCertificate(t *testing.T) {
	t.Parallel()
	v3 := expoOptions(t)
	v3.V3 = true
	info, err := parse(build(t, v3))
	require.NoError(t, err)
	assert.Equal(t, []int{2, 3}, info.Signing.Schemes)
	assert.Equal(t, v3.Signer.CertificateSHA256(), info.Signing.CertificateSHA256)

	debug := expoOptions(t)
	debug.Debuggable = true
	debug.Signer = signer(t, "Android Debug")
	info, err = parse(build(t, debug))
	require.NoError(t, err)
	assert.True(t, info.Debuggable)
	assert.True(t, info.Signing.DebugCertificate)
	assert.Equal(t, "CN=Android Debug,O=Android,C=US", info.Signing.CertificateSubject)
}

func TestParseRefuses(t *testing.T) {
	t.Parallel()
	unsigned := expoOptions(t)
	unsigned.Signer = nil

	big := expoOptions(t)
	big.Filler = 3 << 20
	changed := build(t, big)
	changed[100_000] ^= 0xff // inside the stored filler, after signing

	v1 := expoOptions(t)
	v1.Signer = nil
	v1.ExtraFiles = map[string][]byte{"META-INF/MANIFEST.MF": []byte("Manifest-Version: 1.0\n"), "META-INF/CERT.RSA": {0x30, 0x00}}

	hugeManifest := expoOptions(t)
	hugeManifest.ExtraFiles = map[string][]byte{"AndroidManifest.xml": make([]byte, 5<<20)}

	notXML := expoOptions(t)
	notXML.ExtraFiles = map[string][]byte{"AndroidManifest.xml": []byte("<manifest package=\"x\"/>")}

	badTable := expoOptions(t)
	badTable.ExtraFiles = map[string][]byte{"resources.arsc": []byte("not a resource table")}

	hugeFingerprint := expoOptions(t)
	hugeFingerprint.Fingerprint = "abc"
	hugeFingerprint.ExtraFiles = map[string][]byte{"assets/fingerprint": bytes.Repeat([]byte("f"), 5000)}

	good := build(t, expoOptions(t))
	var plainZip bytes.Buffer
	zw := zip.NewWriter(&plainZip)
	w, err := zw.Create("hello.txt")
	require.NoError(t, err)
	_, _ = w.Write([]byte("hello"))
	require.NoError(t, zw.Close())

	for _, c := range []struct {
		name string
		b    []byte
		code string
	}{
		{"unsigned", build(t, unsigned), apk.CodeUnsigned},
		{"changed after signing", changed, apk.CodeSignatureInvalid},
		{"v1 only", build(t, v1), apk.CodeUnsupportedSignature},
		{"manifest over 4 MiB", build(t, hugeManifest), apk.CodeMalformedManifest},
		{"manifest not binary XML", build(t, notXML), apk.CodeMalformedManifest},
		{"runtime version reference unresolvable", build(t, badTable), apk.CodeMalformedRuntimeVersion},
		{"fingerprint over 4 KiB", build(t, hugeFingerprint), apk.CodeMalformedRuntimeVersion},
		{"empty", nil, apk.CodeNotAnAPK},
		{"not a zip", bytes.Repeat([]byte("not an apk "), 100), apk.CodeNotAnAPK},
		{"truncated", good[:len(good)-10], apk.CodeNotAnAPK},
		{"a zip without a manifest", plainZip.Bytes(), apk.CodeNotAnAPK},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			_, err := parse(c.b)
			requireCode(t, err, c.code)
		})
	}
}

// Every byte of the signing block and central directory matters: a change
// anywhere after the entries is refused, never accepted or a panic.
func TestParseRefusesAnyChangedByteAfterTheEntries(t *testing.T) {
	t.Parallel()
	o := expoOptions(t)
	o.V3 = true
	good := build(t, o)
	for i := len(good) - 1; i >= len(good)-1500 && i >= 0; i -= 7 {
		b := bytes.Clone(good)
		b[i] ^= 0x01
		if _, err := parse(b); err == nil {
			// A byte of the zip comment length or an unused field can be
			// harmless only if the result is still the same signed APK.
			info, _ := parse(good)
			got, _ := parse(b)
			assert.Equal(t, info, got, "byte %d", i)
		}
	}
}

func FuzzParse(f *testing.F) {
	o := expoOptions(f)
	f.Add(build(f, o))
	o.V3 = true
	f.Add(build(f, o))
	o.Signer = nil
	f.Add(build(f, o))
	o.Fingerprint = "abc"
	f.Add(build(f, o))
	f.Fuzz(func(_ *testing.T, b []byte) {
		_, _ = parse(b)
	})
}
