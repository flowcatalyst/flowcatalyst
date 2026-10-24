package pages

import (
	"bytes"
	"context"
	"regexp"
	"strconv"
	"testing"

	"github.com/boombuler/barcode/qr"
)

var qrCell = regexp.MustCompile(`M(\d+) (\d+)h1v1h-1z`)

// The drawing is the encoder's matrix, module for module, inside the quiet
// zone, on a white ground; and it carries no text of the code's content.
func TestQRSVGIsTheEncodersMatrix(t *testing.T) {
	c, err := qrSVG(testTOTPURI)
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := c.Render(context.Background(), &buf); err != nil {
		t.Fatal(err)
	}
	svg := buf.String()

	code, err := qr.Encode(testTOTPURI, qr.M, qr.Auto)
	if err != nil {
		t.Fatal(err)
	}
	n := code.Bounds().Dx()
	if want := `viewBox="0 0 ` + strconv.Itoa(n+8) + ` ` + strconv.Itoa(n+8) + `"`; !bytes.Contains([]byte(svg), []byte(want)) {
		t.Errorf("no %s in %.200s", want, svg)
	}
	dark := map[[2]int]bool{}
	for _, m := range qrCell.FindAllStringSubmatch(svg, -1) {
		x, _ := strconv.Atoi(m[1])
		y, _ := strconv.Atoi(m[2])
		dark[[2]int{x - 4, y - 4}] = true
	}
	for y := range n {
		for x := range n {
			r, _, _, _ := code.At(x, y).RGBA()
			if (r == 0) != dark[[2]int{x, y}] {
				t.Fatalf("module (%d,%d): encoder dark=%v, drawing dark=%v", x, y, r == 0, dark[[2]int{x, y}])
			}
		}
	}
	for _, leak := range []string{"otpauth", "JBSWY3DP", "secret"} {
		if bytes.Contains([]byte(svg), []byte(leak)) {
			t.Errorf("the drawing contains %q", leak)
		}
	}
	if !bytes.Contains([]byte(svg), []byte(`fill="#fff"`)) {
		t.Error("no white ground: a dark theme would swallow the code")
	}
}
