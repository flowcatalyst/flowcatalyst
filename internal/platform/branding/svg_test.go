package branding

import (
	"errors"
	"strings"
	"testing"
)

func TestCheckLogoSVGAcceptsOrdinaryLogos(t *testing.T) {
	for name, s := range map[string]string{
		"minimal":        `<svg/>`,
		"with namespace": `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 10 10"><circle r="4" fill="#0967d2"/></svg>`,
		"xml prolog and doctype": `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE svg PUBLIC "-//W3C//DTD SVG 1.1//EN" "http://www.w3.org/Graphics/SVG/1.1/DTD/svg11.dtd">
<!-- exported -->
<svg xmlns="http://www.w3.org/2000/svg"><title>Acme</title><g><path d="M0 0h10v10z"/></g></svg>`,
		"style and gradient": `<svg xmlns="http://www.w3.org/2000/svg"><defs><linearGradient id="g"><stop offset="0" stop-color="#fff"/></linearGradient></defs><style>.a{fill:url(#g)}</style><rect class="a" width="10" height="10"/></svg>`,
		"https link":         `<svg xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink"><a xlink:href="https://acme.example"><text>Acme</text></a></svg>`,
		"animation":          `<svg><rect width="1" height="1"><animate attributeName="opacity" values="0;1" dur="1s"/></rect></svg>`,
	} {
		if err := CheckLogoSVG(s); err != nil {
			t.Errorf("%s: refused: %v", name, err)
		}
	}
}

func TestCheckLogoSVGRefusesHostileMarkup(t *testing.T) {
	for name, s := range map[string]string{
		"script":                  `<svg><script>alert(1)</script></svg>`,
		"namespaced script":       `<svg xmlns:h="http://www.w3.org/1999/xhtml"><h:script>alert(1)</h:script></svg>`,
		"foreignObject":           `<svg><foreignObject><div xmlns="http://www.w3.org/1999/xhtml">x</div></foreignObject></svg>`,
		"onload":                  `<svg onload="alert(1)"></svg>`,
		"svg/onload (HTML style)": `<svg/onload=alert(1)>`,
		"image/href/onerror":      `<svg><image/href=x/onerror=alert(1)></svg>`,
		"image onerror":           `<svg><image href="x" onerror="alert(1)"/></svg>`,
		"uppercase handler":       `<svg ONLOAD="alert(1)"/>`,
		"javascript href":         `<svg><a href="javascript:alert(1)"><text>x</text></a></svg>`,
		"javascript entity colon": `<svg><a href="javascript&#58;alert(1)"><text>x</text></a></svg>`,
		"javascript entity tab":   `<svg><a xlink:href="java&#x09;script:alert(1)" xmlns:xlink="http://www.w3.org/1999/xlink"><text>x</text></a></svg>`,
		"javascript mixed case":   `<svg><a href=" JaVaScRiPt:alert(1)"><text>x</text></a></svg>`,
		"data html href":          `<svg><a href="data:text/html,&lt;script&gt;alert(1)&lt;/script&gt;"><text>x</text></a></svg>`,
		"css javascript url":      `<svg><rect style="fill:url(javascript:alert(1))"/></svg>`,
		"set onmouseover":         `<svg><rect><set attributeName="onmouseover" to="alert(1)"/></rect></svg>`,
		"animate href to js":      `<svg><a><animate attributeName="href" values="javascript:alert(1)"/><text>x</text></a></svg>`,
		"iframe":                  `<svg><iframe src="https://evil.example"/></svg>`,
		"handler (SVG Tiny)":      `<svg><handler type="application/ecmascript">alert(1)</handler></svg>`,
		"entity definitions":      `<!DOCTYPE svg [<!ENTITY x "y">]><svg>&x;</svg>`,
		"xml-stylesheet":          `<?xml-stylesheet href="https://evil.example/a.css"?><svg/>`,
		"comment breakout (--!>)": `<svg><!-- --!><img src=x onerror=alert(1)> --></svg>`,
		"style with markup":       `<svg><style><img src=x onerror=alert(1)></style></svg>`,
		"not svg":                 `<img src=x onerror=alert(1)>`,
		"html root":               `<html><body><svg/></body></html>`,
		"two roots":               `<svg/><svg onload="alert(1)"/>`,
		"trailing markup":         `<svg/><img src=x onerror=alert(1)>`,
		"text after root":         `<svg/>hello`,
		"unclosed":                `<svg><g>`,
		"empty":                   ``,
		"too large":               `<svg>` + strings.Repeat(" ", MaxLogoSVGBytes) + `</svg>`,
	} {
		err := CheckLogoSVG(s)
		if err == nil {
			t.Errorf("%s: accepted", name)
			continue
		}
		if !errors.Is(err, ErrUnsafeLogoSVG) {
			t.Errorf("%s: %v does not wrap ErrUnsafeLogoSVG", name, err)
		}
	}
}
