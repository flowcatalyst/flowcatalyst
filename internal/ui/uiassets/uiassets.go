// Package uiassets is the platform's one assets instance for the server-rendered
// UI. The framework's go/ui module (github.com/flowcatalyst/framework/go/ui/assets)
// carries the kit files (datastar.js, the icon sprite, the islands and
// Mermaid); this package adds the platform's own files and serves everything
// under /ui/assets/:
//
//   - platform.css: the platform's stylesheet. It is the kit's theme plus the
//     classes the platform's pages use, built by scripts/ui-kit-sync.sh
//     (the module's own kit.css lacks those classes, so layouts link
//     platform.css instead);
//   - fc-webauthn.js (+ LICENSE-webauthn.md): the passkey forms' glue, built by
//     tools/vendor-webauthn.
//
// The instance is built once at package initialisation (a missing file is a
// build error caught by the first test run, so it panics); callers use the
// package-level URL, Handler, HasIcon and SpriteURL, which keeps the pages'
// call sites as small as they were with the old internal package.
package uiassets

import (
	"embed"
	"io/fs"
	"net/http"

	"github.com/flowcatalyst/framework/go/ui/assets"
)

// Prefix is the URL path the assets are served under.
const Prefix = "/ui/assets/"

// Stylesheet is the logical name of the platform's stylesheet.
const Stylesheet = "platform.css"

// WebAuthnSHA256 is the sha256 of static/fc-webauthn.js as built by
// tools/vendor-webauthn (@simplewebauthn/browser 13.3.0 and the passkey forms'
// glue; the build is reproducible for the pinned versions). A test fails on a
// hand edit or an unrecorded rebuild.
const WebAuthnSHA256 = "63687571b2db9c295575719e219442ec658c0c825fce69ec0781e0c8c7cb9dce"

//go:embed static
var embedded embed.FS

// Instance is the assets instance (Prefix, the module's files and the
// platform's).
var Instance = mustLoad()

func mustLoad() *assets.Assets {
	sub, err := fs.Sub(embedded, "static")
	if err != nil {
		panic(err)
	}
	a, err := assets.Load(assets.WithPrefix(Prefix),
		assets.WithFiles(sub, Stylesheet, "fc-webauthn.js", "LICENSE-webauthn.md"))
	if err != nil {
		panic("uiassets: " + err.Error())
	}
	return a
}

// URL is the URL of an asset for a <link> or <script> (Prefix+name+"?v=<hash>").
// An unknown name panics.
func URL(name string) string { return Instance.URL(name) }

// SpriteURL is the sprite URL icons reference (kit.WithSpriteURL).
func SpriteURL() string { return Instance.SpriteURL() }

// HasIcon reports whether the sprite has the Lucide icon name.
func HasIcon(name string) bool { return Instance.HasIcon(name) }

// Handler serves the assets; mount it with the prefix intact.
func Handler() http.Handler { return Instance.Handler() }
