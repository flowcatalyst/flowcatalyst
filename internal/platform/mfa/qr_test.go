package mfa

import (
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"strings"
	"testing"

	"github.com/pquerna/otp"
)

// The QR renderer (pquerna/otp over boombuler/barcode) must keep producing the
// exact same image; the hash was taken with the 2019 barcode pin and matched
// by v1.1.0, so a dependency bump that changes the rendering fails here.
func TestQRDataURI_Stable(t *testing.T) {
	k, err := otp.NewKeyFromURL("otpauth://totp/FlowCatalyst:user@example.com?secret=JBSWY3DPEHPK3PXPJBSWY3DPEHPK3PXP&issuer=FlowCatalyst&period=30&digits=6&algorithm=SHA1")
	if err != nil {
		t.Fatal(err)
	}
	uri, err := qrDataURI(k, 200)
	if err != nil {
		t.Fatal(err)
	}
	const prefix = "data:image/png;base64,"
	if !strings.HasPrefix(uri, prefix) {
		t.Fatalf("not a PNG data URI: %.40s", uri)
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(uri, prefix))
	if err != nil {
		t.Fatal(err)
	}
	if got := fmt.Sprintf("%x", sha256.Sum256(raw)); got != "43cdac951551a0005b28551e8071db3527f5f5270a8213e6f9cf93e033490a07" {
		t.Fatalf("QR rendering changed: %s", got)
	}
}
