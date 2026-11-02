package uiassets

import (
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// fc-webauthn.js must be the recorded tools/vendor-webauthn build.
func TestWebAuthnBundleIsPinned(t *testing.T) {
	body, err := os.ReadFile("static/fc-webauthn.js")
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(body)
	if got := hex.EncodeToString(sum[:]); got != WebAuthnSHA256 {
		t.Errorf("fc-webauthn.js sha256 = %s, want %s (see docs/ui/README.md, Islands)", got, WebAuthnSHA256)
	}
}

// The module's files and the platform's are all served under the platform prefix.
func TestInstanceServesModuleAndPlatformFiles(t *testing.T) {
	for _, name := range []string{"platform.css", "fc-webauthn.js", "LICENSE-webauthn.md", "kit.css", "datastar.js", "lucide-sprite.svg", "mermaid.min.js", "fc-diagram.js", "fc-code-editor.js"} {
		u := URL(name)
		if !strings.HasPrefix(u, Prefix+name+"?v=") {
			t.Errorf("URL(%s) = %q", name, u)
			continue
		}
		rec := httptest.NewRecorder()
		Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, u, nil))
		if rec.Code != http.StatusOK || rec.Body.Len() == 0 {
			t.Errorf("GET %s: %d, %d bytes", u, rec.Code, rec.Body.Len())
		}
	}
	if SpriteURL() != Prefix+"lucide-sprite.svg" {
		t.Errorf("SpriteURL = %q", SpriteURL())
	}
}

// platform.css is the built stylesheet (not the empty placeholder) and has the
// kit's theme and a class only the platform's pages use.
func TestPlatformStylesheetIsBuilt(t *testing.T) {
	body, err := os.ReadFile("static/platform.css")
	if err != nil {
		t.Fatal(err)
	}
	if len(body) < 20000 {
		t.Fatalf("platform.css is %d bytes: run make ui-kit-sync", len(body))
	}
}

// Every icon the UI names must be in the sprite: a missing symbol renders as
// an empty box, silently.
func TestEveryIconUsedIsInTheSprite(t *testing.T) {
	uses := regexp.MustCompile(`(?:[Ii]con: |IconProps\{Name: )"([a-z0-9-]+)"`)
	missing := map[string][]string{}
	seen := 0
	err := filepath.WalkDir("..", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && (d.Name() == "testdata" || d.Name() == "uiassets") {
			return filepath.SkipDir
		}
		if d.IsDir() || strings.HasSuffix(p, "_templ.go") || strings.HasSuffix(p, "_test.go") ||
			(!strings.HasSuffix(p, ".templ") && !strings.HasSuffix(p, ".go")) {
			return nil
		}
		src, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		for _, m := range uses.FindAllStringSubmatch(string(src), -1) {
			seen++
			if !HasIcon(m[1]) {
				missing[m[1]] = append(missing[m[1]], p)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if seen < 40 {
		t.Fatalf("found only %d icon references; the scan pattern is broken", seen)
	}
	var names []string
	for n, files := range missing {
		names = append(names, n+" ("+strings.Join(files, ", ")+")")
	}
	sort.Strings(names)
	if len(names) > 0 {
		t.Fatalf("icons not in the sprite (add them to the framework kit's icons): %s", strings.Join(names, "; "))
	}
}
