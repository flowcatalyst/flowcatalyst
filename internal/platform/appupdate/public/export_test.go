package public

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// DecodeQRInPage reads the QR code drawn in an HTML page back to its text,
// with the test's own decoder (qr_test.go), for the install page's tests.
func DecodeQRInPage(t *testing.T, page string) string {
	t.Helper()
	i := strings.Index(page, "<svg")
	require.GreaterOrEqual(t, i, 0, "no QR code in the page")
	j := strings.Index(page[i:], "</svg>")
	require.Greater(t, j, 0)
	text, _, err := decodeQR(matrixOfSVG(t, page[i:i+j+len("</svg>")]))
	require.NoError(t, err)
	return text
}

// InstallCSP is the install page's content security policy.
var InstallCSP = installCSP
