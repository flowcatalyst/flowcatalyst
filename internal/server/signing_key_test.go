package server

import (
	"encoding/base64"
	"encoding/pem"
	"strings"
	"testing"
)

func TestNormalizePEM(t *testing.T) {
	clean := string(generateRSAPEM()) // valid multi-line PKCS#1 PEM

	// parses returns true iff s decodes to a non-nil PEM block — exactly what
	// the provider's parseRSAPrivateKey does first ("no PEM block found" is the
	// production failure this normalization fixes).
	parses := func(s string) bool {
		block, _ := pem.Decode([]byte(s))
		return block != nil
	}

	t.Run("clean PEM unchanged", func(t *testing.T) {
		if got := NormalizePEM(clean); got != strings.TrimSpace(clean) {
			t.Fatal("clean PEM should pass through unchanged")
		}
	})

	t.Run("literal backslash-n escapes are repaired", func(t *testing.T) {
		escaped := strings.ReplaceAll(clean, "\n", `\n`) // single-line with literal \n
		if parses(escaped) {
			t.Fatal("precondition: escaped PEM should NOT parse before normalize")
		}
		if !parses(NormalizePEM(escaped)) {
			t.Fatal("normalized escaped PEM should decode to a PEM block")
		}
	})

	t.Run("surrounding quotes trimmed", func(t *testing.T) {
		if !parses(NormalizePEM(`"` + clean + `"`)) {
			t.Fatal("quoted PEM should decode after normalize")
		}
	})

	t.Run("base64-wrapped PEM is decoded", func(t *testing.T) {
		wrapped := base64.StdEncoding.EncodeToString([]byte(clean))
		if !parses(NormalizePEM(wrapped)) {
			t.Fatal("base64-wrapped PEM should decode after normalize")
		}
	})
}
