package pages

import (
	"fmt"
	"strings"

	"github.com/a-h/templ"
	"github.com/boombuler/barcode/qr"
)

// qrSVG draws text as a QR code (error correction M, the level the current
// UI's PNG uses) in an inline SVG: one path of unit squares on a white
// ground with the four-module quiet zone the standard asks for. The markup
// is built from integers only, so it carries nothing of the text.
func qrSVG(text string) (templ.Component, error) {
	code, err := qr.Encode(text, qr.M, qr.Auto)
	if err != nil {
		return nil, err
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
	svg := fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %[1]d %[1]d" width="200" height="200" shape-rendering="crispEdges" role="img" aria-label="Authenticator QR code" class="mx-auto rounded-lg border border-border"><rect width="%[1]d" height="%[1]d" fill="#fff"/><path d="%[2]s" fill="#000"/></svg>`, size, path.String())
	return templ.Raw(svg), nil
}
