// Package apk reads what an Android APK says about itself: its manifest
// (package, version, SDK levels, debuggable, the expo-updates meta-data), the
// runtime version expo-updates sends, and its APK Signature Scheme v2/v3
// signer, verified. It is used where a binary is published
// (docs/app-updates-design.md, "App binaries"), so it reads a hostile file:
// every size and offset is bounded and nothing is loaded whole except the
// manifest (at most 4 MiB), resources.arsc (at most 64 MiB) and the signing
// block (at most 16 MiB).
//
// # Where each value comes from
//
// expo-updates 58.0.21 (android/src/main/java/expo/modules/updates/
// UpdatesConfiguration.kt) reads its configuration from the application's
// meta-data through PackageManager: the runtime version is
// expo.modules.updates.EXPO_RUNTIME_VERSION, toString() of the bundle value
// with a leading "string:" removed; when that is "file:fingerprint" it is the
// whole text of the asset "fingerprint" (assets/fingerprint), not trimmed. The
// Expo config plugin writes the meta-data as a @string reference
// (strings.xml expo_runtime_version), which PackageManager resolves to the
// string, so a reference is resolved through resources.arsc here.
//
// # Verified against
//
// Android SDK build-tools 37.0.0 (aapt2 2.20-15087165, apksigner 0.9) and
// keytool of OpenJDK 25.0.4, on 2026-11-19:
//
//   - Two Expo SDK 58 release APKs built by Gradle (v2 signed, one with
//     expo-updates configured and its runtime version a @string reference to
//     "file:fingerprint", one with expo-updates disabled): Parse returned the
//     package, version code and name and SDK levels `aapt2 dump badging`
//     prints, the meta-data `aapt2 dump xmltree --file AndroidManifest.xml`
//     shows (the reference resolved as `aapt2 dump resources` resolves it),
//     the runtime version `unzip -p <apk> assets/fingerprint` prints, and the
//     signer subject and certificate SHA-256 of `apksigner verify -v
//     --print-certs`.
//   - APKs built by package apktest (TestWriteForTools): `apksigner verify -v
//     --print-certs` verifies v2 (and v2 and v3 with Options.V3) with the
//     certificate digest Signer.CertificateSHA256 gives, and `aapt2 dump
//     xmltree` and `aapt2 dump resources` read the manifest and table as
//     written.
//   - An unsigned apktest APK signed by `apksigner sign --ks <keytool RSA 2048
//     keystore> --v1-signing-enabled false --v2-signing-enabled true
//     --v3-signing-enabled true`: Parse verifies schemes [2 3] with the
//     certificate SHA-256 `keytool -list -v` prints.
package apk

import (
	"archive/zip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

// Refusal codes.
const (
	CodeNotAnAPK                = "NOT_AN_APK"
	CodeMalformedManifest       = "MALFORMED_MANIFEST"
	CodeUnsigned                = "UNSIGNED"
	CodeSignatureInvalid        = "SIGNATURE_INVALID"
	CodeUnsupportedSignature    = "UNSUPPORTED_SIGNATURE"
	CodeMalformedRuntimeVersion = "MALFORMED_RUNTIME_VERSION"
)

// Limits on what a hostile file can make this package read.
const (
	maxManifestBytes    = 4 << 20
	maxResourcesBytes   = 64 << 20
	maxFingerprintBytes = 4 << 10
	maxEntries          = 100_000
	maxCentralDirectory = 64 << 20
)

// The meta-data keys expo-updates reads.
const (
	metaRuntimeVersion  = "expo.modules.updates.EXPO_RUNTIME_VERSION"
	metaUpdateURL       = "expo.modules.updates.EXPO_UPDATE_URL"
	metaRequestHeaders  = "expo.modules.updates.UPDATES_CONFIGURATION_REQUEST_HEADERS_KEY"
	metaCodeSigning     = "expo.modules.updates.CODE_SIGNING_METADATA"
	fingerprintSentinel = "file:fingerprint"
	fingerprintAsset    = "assets/fingerprint"
)

// Info is what an APK says about itself.
type Info struct {
	Package     string
	VersionCode int64
	VersionName string
	MinSDK      int
	TargetSDK   int
	Debuggable  bool
	// RuntimeVersion is what expo-updates sends as expo-runtime-version.
	RuntimeVersion string
	// RuntimeVersionSource is "manifest" (a literal), "resource" (a @string
	// reference), "fingerprint" (assets/fingerprint) or "" (absent).
	RuntimeVersionSource   string
	UpdateURL              string
	UpdateRequestHeaders   map[string]string
	UpdateCodeSigningKeyID string
	Signing                Signing
}

// Channel is the expo-channel-name request header the app sends, "" when none.
func (i *Info) Channel() string {
	for k, v := range i.UpdateRequestHeaders {
		if strings.EqualFold(strings.TrimSpace(k), "expo-channel-name") {
			return v
		}
	}
	return ""
}

// Signing is the APK's verified signer.
type Signing struct {
	// Schemes are the verified APK Signature Schemes, ascending (2 and/or 3).
	Schemes []int
	// CertificateSHA256 is the lower-case hex SHA-256 of the DER certificate
	// Android uses: v3's when a v3 block verified, else v2's.
	CertificateSHA256  string
	CertificateSubject string
	// DebugCertificate: the subject's CN is "Android Debug" (the Android
	// SDK's and the React Native template's debug keystore).
	DebugCertificate bool
}

// Error is a refusal.
type Error struct{ Code, Message string }

func (e *Error) Error() string { return e.Code + ": " + e.Message }

func refuse(code, format string, args ...any) *Error {
	return &Error{Code: code, Message: fmt.Sprintf(format, args...)}
}

// ParseFile opens path and calls Parse.
func ParseFile(path string) (*Info, error) {
	f, err := os.Open(path) //nolint:gosec // G304: the caller names the file to read
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	st, err := f.Stat()
	if err != nil {
		return nil, err
	}
	return Parse(f, st.Size())
}

// Parse reads the APK in r (size bytes).
func Parse(r io.ReaderAt, size int64) (*Info, error) {
	layout, err := findEOCD(r, size)
	if err != nil {
		return nil, err
	}
	if layout.entries > maxEntries {
		return nil, refuse(CodeNotAnAPK, "the file has more than %d entries", maxEntries)
	}
	if layout.cdSize > maxCentralDirectory {
		return nil, refuse(CodeNotAnAPK, "the central directory is larger than %d bytes", maxCentralDirectory)
	}
	zr, err := zip.NewReader(io.NewSectionReader(r, 0, size), size)
	if err != nil && !errors.Is(err, zip.ErrInsecurePath) {
		return nil, refuse(CodeNotAnAPK, "not a zip file: %v", err)
	}
	if len(zr.File) > maxEntries {
		return nil, refuse(CodeNotAnAPK, "the file has more than %d entries", maxEntries)
	}
	files := make(map[string]*zip.File, len(zr.File))
	for _, f := range zr.File {
		if _, dup := files[f.Name]; !dup {
			files[f.Name] = f
		}
	}
	if files["AndroidManifest.xml"] == nil {
		return nil, refuse(CodeNotAnAPK, "the file has no AndroidManifest.xml")
	}

	signing, err := verifySigning(r, layout, hasV1Signature(files))
	if err != nil {
		return nil, err
	}

	raw, err := readEntry(files["AndroidManifest.xml"], maxManifestBytes)
	if err != nil {
		return nil, refuse(CodeMalformedManifest, "AndroidManifest.xml: %v", err)
	}
	m, err := parseManifest(raw)
	if err != nil {
		return nil, refuse(CodeMalformedManifest, "AndroidManifest.xml: %v", err)
	}
	res := &resources{file: files["resources.arsc"]}

	info := &Info{Signing: *signing}
	info.Package = m.pkg
	if info.Package == "" {
		return nil, refuse(CodeMalformedManifest, "the manifest names no package")
	}
	vc, ok := m.versionCode.uint32Value(res)
	if !m.versionCode.present || !ok {
		return nil, refuse(CodeMalformedManifest, "the manifest has no versionCode")
	}
	info.VersionCode = int64(vc)
	if m.versionCodeMajor.present {
		major, ok := m.versionCodeMajor.uint32Value(res)
		if !ok {
			return nil, refuse(CodeMalformedManifest, "the manifest's versionCodeMajor is not a number")
		}
		info.VersionCode |= int64(major) << 32
	}
	if s, ok := m.versionName.stringValue(res); ok {
		info.VersionName = s
	}
	info.MinSDK = m.minSDK.sdkLevel(res)
	info.TargetSDK = m.targetSDK.sdkLevel(res)
	if b, ok := m.debuggable.boolValue(res); ok {
		info.Debuggable = b
	}

	if v, ok := m.meta[metaRuntimeVersion]; ok {
		if err := info.readRuntimeVersion(v, res, files[fingerprintAsset]); err != nil {
			return nil, err
		}
	}
	if v, ok := m.meta[metaUpdateURL]; ok {
		info.UpdateURL, _ = v.stringValue(res)
	}
	if v, ok := m.meta[metaRequestHeaders]; ok {
		if s, ok := v.stringValue(res); ok {
			var h map[string]string
			if json.Unmarshal([]byte(s), &h) == nil {
				info.UpdateRequestHeaders = h
			}
		}
	}
	if v, ok := m.meta[metaCodeSigning]; ok {
		if s, ok := v.stringValue(res); ok {
			var cs struct {
				KeyID string `json:"keyid"`
			}
			if json.Unmarshal([]byte(s), &cs) == nil {
				info.UpdateCodeSigningKeyID = cs.KeyID
			}
		}
	}
	return info, nil
}

// readRuntimeVersion does what UpdatesConfiguration.getRuntimeVersion does.
func (i *Info) readRuntimeVersion(v attrValue, res *resources, fingerprint *zip.File) error {
	source := "manifest"
	if v.isReference() {
		source = "resource"
		rv, err := res.resolve(v.data)
		if err != nil {
			return refuse(CodeMalformedRuntimeVersion, "%s names resource 0x%08x: %v", metaRuntimeVersion, v.data, err)
		}
		v = rv
	}
	var s string
	switch v.typ {
	case typeString:
		s = v.str
	case typeIntDec, typeIntHex:
		s = strconv.FormatInt(int64(int32(v.data)), 10) //nolint:gosec // G115: Java's Integer is the signed 32 bits
	case typeIntBoolean:
		s = strconv.FormatBool(v.data != 0)
	default:
		return refuse(CodeMalformedRuntimeVersion, "%s is a value of type 0x%02x, not a string", metaRuntimeVersion, v.typ)
	}
	s = strings.TrimPrefix(s, "string:")
	if s == fingerprintSentinel {
		if fingerprint == nil {
			return refuse(CodeMalformedRuntimeVersion, "the runtime version is read from %s, which the APK does not have", fingerprintAsset)
		}
		b, err := readEntry(fingerprint, maxFingerprintBytes)
		if err != nil {
			return refuse(CodeMalformedRuntimeVersion, "%s: %v", fingerprintAsset, err)
		}
		s, source = string(b), "fingerprint"
	}
	if s == "" {
		return nil
	}
	i.RuntimeVersion, i.RuntimeVersionSource = s, source
	return nil
}

// readEntry reads a whole zip entry of at most limit bytes.
func readEntry(f *zip.File, limit int64) ([]byte, error) {
	if f.UncompressedSize64 > uint64(limit) {
		return nil, fmt.Errorf("larger than %d bytes", limit)
	}
	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer func() { _ = rc.Close() }()
	b, err := io.ReadAll(io.LimitReader(rc, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > limit {
		return nil, fmt.Errorf("larger than %d bytes", limit)
	}
	return b, nil
}

// hasV1Signature reports whether the zip carries a JAR signature.
func hasV1Signature(files map[string]*zip.File) bool {
	if files["META-INF/MANIFEST.MF"] == nil {
		return false
	}
	for name := range files {
		upper := strings.ToUpper(name)
		if strings.HasPrefix(upper, "META-INF/") && strings.Count(name, "/") == 1 &&
			(strings.HasSuffix(upper, ".RSA") || strings.HasSuffix(upper, ".DSA") || strings.HasSuffix(upper, ".EC")) {
			return true
		}
	}
	return false
}
