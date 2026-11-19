package public

import (
	"fmt"
	"html"
	"strings"

	"github.com/boombuler/barcode/qr"
)

// qrSVG draws text as an inline SVG QR code: error correction M, the four-
// module quiet zone the standard asks for, one path of unit squares in black
// on a white ground (white in a dark theme too, or a phone's camera loses the
// code). The markup is built from integers only, so it carries nothing of the
// text; it has no class or style attribute (the page's policy allows only its
// own hashed stylesheet, which sizes it).
func qrSVG(text, label string) (string, error) {
	code, err := qr.Encode(text, qr.M, qr.Auto)
	if err != nil {
		return "", err
	}
	const quiet = 4
	b := code.Bounds()
	size := b.Dx() + 2*quiet
	var path strings.Builder
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if r, _, _, _ := code.At(x, y).RGBA(); r == 0 {
				fmt.Fprintf(&path, "M%d %dh1v1h-1z", x-b.Min.X+quiet, y-b.Min.Y+quiet)
			}
		}
	}
	return fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %[1]d %[1]d" width="240" height="240" shape-rendering="crispEdges" role="img" aria-label="%[3]s"><rect width="%[1]d" height="%[1]d" fill="#fff"/><path d="%[2]s" fill="#000"/></svg>`,
		size, path.String(), html.EscapeString(label)), nil
}
