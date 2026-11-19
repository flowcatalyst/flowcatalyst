package public

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/appupdate"
)

func TestByteRange(t *testing.T) {
	t.Parallel()
	const size = 1000
	etag := `"abc"`
	for _, c := range []struct {
		header, ifRange    string
		start, end         int64
		partial, satisfies bool
	}{
		{"", "", 0, 999, false, true},
		{"bytes=0-99", "", 0, 99, true, true},
		{"bytes=900-", "", 900, 999, true, true},
		{"bytes=-100", "", 900, 999, true, true},
		{"bytes=-5000", "", 0, 999, true, true},
		{"bytes=990-5000", "", 990, 999, true, true},
		{"bytes=999-999", "", 999, 999, true, true},
		{"bytes=1000-", "", 0, 0, false, false},
		{"bytes=0-99", etag, 0, 99, true, true},
		{"bytes=0-99", `"other"`, 0, 999, false, true},
		{"bytes=0-99", "Wed, 21 Oct 2026 07:28:00 GMT", 0, 999, false, true},
		{"bytes=0-9,20-29", "", 0, 999, false, true},
		{"bytes=9-0", "", 0, 999, false, true},
		{"bytes=x-", "", 0, 999, false, true},
		{"items=0-9", "", 0, 999, false, true},
		{"bytes=-0", "", 0, 999, false, true},
	} {
		s, e, p, ok := byteRange(c.header, c.ifRange, etag, size)
		assert.Equal(t, []any{c.start, c.end, c.partial, c.satisfies}, []any{s, e, p, ok}, "%q if-range %q", c.header, c.ifRange)
	}
}

// Every range of a file that matches its hash is sent; a file that does not
// match is never sent whole, whichever part was asked for.
func TestCopyRangeVerified(t *testing.T) {
	t.Parallel()
	data := make([]byte, 3*verifyChunk+123)
	for i := range data {
		data[i] = byte(i * 7)
	}
	sum := sha256.Sum256(data)
	good := hex.EncodeToString(sum[:])
	size := int64(len(data))
	for _, r := range [][2]int64{{0, size - 1}, {0, 99}, {size - 100, size - 1}, {verifyChunk - 1, 2*verifyChunk + 1}, {5, 5}} {
		var out bytes.Buffer
		require.NoError(t, copyRangeVerified(&out, bytes.NewReader(data), size, r[0], r[1], good), "%v", r)
		assert.Equal(t, data[r[0]:r[1]+1], out.Bytes(), "%v", r)

		bad := bytes.Clone(data)
		bad[len(bad)-1] ^= 1
		out.Reset()
		err := copyRangeVerified(&out, bytes.NewReader(bad), size, r[0], r[1], good)
		assert.True(t, errors.Is(err, errFileMismatch), "%v", r)
		assert.Less(t, int64(out.Len()), r[1]-r[0]+1, "%v: a part of a wrong file is cut short", r)
	}
	var out bytes.Buffer
	assert.ErrorIs(t, copyRangeVerified(&out, bytes.NewReader(data[:100]), size, 0, size-1, good), errFileMismatch, "a short file")
	assert.ErrorIs(t, copyRangeVerified(&out, bytes.NewReader(append(bytes.Clone(data), 1)), size, 0, size-1, good), errFileMismatch, "a long file")
}

func TestSwitchNote(t *testing.T) {
	t.Parallel()
	b := func(variant, pkg, cert string) appupdate.Binary {
		x := appupdate.Binary{Variant: variant}
		x.PackageName, x.SigningCertSHA256 = pkg, cert
		return x
	}
	assert.Empty(t, switchNote([]appupdate.Binary{b("release", "a", "1")}))
	assert.Contains(t, switchNote([]appupdate.Binary{b("release", "a", "1"), b("debug", "a.debug", "2")}), "separate apps")
	assert.Contains(t, switchNote([]appupdate.Binary{b("release", "a", "1"), b("debug", "a", "2")}), "uninstall the app first")
	assert.Empty(t, switchNote([]appupdate.Binary{b("release", "a", "1"), b("debug", "a", "1")}))
}

// The policy admits the page's one stylesheet by its hash and nothing else.
func TestInstallCSPHashesTheStylesheet(t *testing.T) {
	t.Parallel()
	sum := sha256.Sum256([]byte(installCSS))
	assert.Contains(t, installCSP, "style-src 'sha256-"+base64.StdEncoding.EncodeToString(sum[:])+"'")
	assert.Contains(t, installCSP, "default-src 'none'")
	assert.NotContains(t, installCSP, "unsafe")
	assert.NotContains(t, installCSP, "script-src")
	assert.True(t, strings.Contains(string(notFoundPage), "<style>"+installCSS+"</style>"))
}

func TestSizeText(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "48.2 MB", sizeText(48_210_000))
	assert.Equal(t, "512 kB", sizeText(511_600))
	assert.Equal(t, "12 bytes", sizeText(12))
}
