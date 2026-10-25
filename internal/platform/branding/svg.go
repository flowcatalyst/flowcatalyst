package branding

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"strings"
)

// MaxLogoSVGBytes caps an inline logo (the theme editor's 64 kB limit).
const MaxLogoSVGBytes = 64 << 10

// ErrUnsafeLogoSVG is CheckLogoSVG's refusal; the wrapped detail says why.
var ErrUnsafeLogoSVG = errors.New("unsafe logo SVG")

// svgForbiddenElements can run script, embed a document, or pull markup in.
// Names are compared lower-cased and without namespace prefix.
var svgForbiddenElements = map[string]bool{
	"script": true, "foreignobject": true, "iframe": true, "frame": true, "frameset": true,
	"embed": true, "object": true, "applet": true, "base": true, "link": true, "meta": true,
	"handler": true, "listener": true, // SVG Tiny event handling
}

// svgForbiddenSchemes are URL schemes that run script or render a document.
var svgForbiddenSchemes = []string{"javascript:", "vbscript:", "data:text/html", "data:application/xhtml"}

// CheckLogoSVG parses an inline logo as XML (not by pattern) and refuses
// what could run script wherever it is shown: a root other than <svg>, any
// element that can script or embed (script, foreignObject, iframe, ...), any
// on* attribute, an animation that targets an on* attribute, and any
// attribute value whose scheme is javascript:/vbscript:/data:text/html
// (entities are decoded by the parser, so javascript&#58; is seen as what it
// is; whitespace and control characters a browser would skip are removed
// before comparing). A DOCTYPE with an internal subset (entity definitions)
// and xml-stylesheet instructions are refused too. Markup that is not
// well-formed XML is refused: what a browser's HTML parser would make of it
// cannot be judged here.
//
// Rendering stays the first defence: every place the platform shows a logo
// SVG uses <img src="data:image/svg+xml;base64,...">, where no script runs.
func CheckLogoSVG(s string) error {
	if len(s) > MaxLogoSVGBytes {
		return fmt.Errorf("%w: larger than %d bytes", ErrUnsafeLogoSVG, MaxLogoSVGBytes)
	}
	dec := xml.NewDecoder(strings.NewReader(s))
	dec.Strict = true
	depth, roots := 0, 0
	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("%w: not well-formed XML: %w", ErrUnsafeLogoSVG, err)
		}
		switch t := tok.(type) {
		case xml.StartElement:
			name := strings.ToLower(t.Name.Local)
			if depth == 0 {
				roots++
				if name != "svg" || roots > 1 {
					return fmt.Errorf("%w: the document must be one <svg> element", ErrUnsafeLogoSVG)
				}
			}
			if svgForbiddenElements[name] {
				return fmt.Errorf("%w: <%s> is not allowed", ErrUnsafeLogoSVG, t.Name.Local)
			}
			isAnimation := name == "set" || name == "animate" || name == "animatetransform" || name == "animatemotion"
			for _, a := range t.Attr {
				an := strings.ToLower(a.Name.Local)
				if strings.HasPrefix(an, "on") {
					return fmt.Errorf("%w: event handler attribute %s", ErrUnsafeLogoSVG, a.Name.Local)
				}
				if isAnimation && an == "attributename" && strings.HasPrefix(strings.ToLower(strings.TrimSpace(a.Value)), "on") {
					return fmt.Errorf("%w: an animation of an event handler", ErrUnsafeLogoSVG)
				}
				if v := squeeze(a.Value); hasForbiddenScheme(v) {
					return fmt.Errorf("%w: a script URL in %s", ErrUnsafeLogoSVG, a.Name.Local)
				}
			}
			depth++
		case xml.EndElement:
			depth--
		case xml.CharData:
			if depth == 0 && len(bytes.TrimSpace(t)) > 0 {
				return fmt.Errorf("%w: text outside the <svg> element", ErrUnsafeLogoSVG)
			}
		case xml.Directive:
			d := strings.ToUpper(string(t))
			if strings.Contains(d, "[") || strings.Contains(d, "ENTITY") {
				return fmt.Errorf("%w: a DOCTYPE with entity definitions", ErrUnsafeLogoSVG)
			}
		case xml.ProcInst:
			if !strings.EqualFold(t.Target, "xml") {
				return fmt.Errorf("%w: processing instruction <?%s?>", ErrUnsafeLogoSVG, t.Target)
			}
		case xml.Comment:
			// Comments are inert in both XML and HTML parsing; the strict
			// decoder already refuses "--" inside one.
		}
	}
	if roots != 1 {
		return fmt.Errorf("%w: the document must be one <svg> element", ErrUnsafeLogoSVG)
	}
	return nil
}

// squeeze lower-cases v and drops the whitespace and control characters a
// browser ignores inside a URL scheme ("java\tscript:").
func squeeze(v string) string {
	var b strings.Builder
	for _, r := range v {
		if r <= 0x20 || r == 0x7f {
			continue
		}
		b.WriteRune(r)
	}
	return strings.ToLower(b.String())
}

func hasForbiddenScheme(v string) bool {
	for _, s := range svgForbiddenSchemes {
		if strings.Contains(v, s) {
			return true
		}
	}
	return false
}
