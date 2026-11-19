package apk

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestParseRealAPK reads the APK FC_TEST_APK names (a Gradle build, to check
// this reader against aapt2 and apksigner by hand); skipped without it.
func TestParseRealAPK(t *testing.T) {
	path := os.Getenv("FC_TEST_APK")
	if path == "" {
		t.Skip("FC_TEST_APK names no APK")
	}
	info, err := ParseFile(path)
	require.NoError(t, err)
	t.Logf("package %s, version %d (%q), sdk %d..%d, debuggable %v", info.Package, info.VersionCode, info.VersionName, info.MinSDK, info.TargetSDK, info.Debuggable)
	t.Logf("runtime version %q from %s; update URL %q; channel %q; key id %q", info.RuntimeVersion, info.RuntimeVersionSource, info.UpdateURL, info.Channel(), info.UpdateCodeSigningKeyID)
	t.Logf("signed with v%v by %s (%s), debug certificate %v", info.Signing.Schemes, info.Signing.CertificateSHA256, info.Signing.CertificateSubject, info.Signing.DebugCertificate)
	assert.NotEmpty(t, info.Package)
	assert.Positive(t, info.VersionCode)
	assert.NotEmpty(t, info.Signing.Schemes)
	assert.Len(t, info.Signing.CertificateSHA256, 64)
}
