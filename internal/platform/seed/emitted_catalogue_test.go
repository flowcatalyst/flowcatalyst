package seed

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// eventCodePattern is a platform event-type code:
// platform:{subdomain}:{aggregate}:{event}.
var eventCodePattern = regexp.MustCompile(`^platform:[a-z-]+:[a-z-]+:[a-z-]+$`)

// knownUncataloguedEventTypes are event types the platform emits that the
// seeded catalogue does not list yet. A subscription cannot pick them from the
// catalogue. Empty since the 79 found 2026-10-10 were catalogued
// (docs/platform-bug-fixes.md F9); keep it empty: catalogue a new emitted code
// rather than listing it here. The list may only shrink: an entry that becomes
// catalogued, or stops being emitted, fails the test until removed.
var knownUncataloguedEventTypes = map[string]bool{}

// emittedEventTypes collects the event-type codes the platform's use cases
// emit: every string constant declared in an events.go under
// internal/platform whose value is an event-type code (permission constants,
// which share the shape, are named perm*/…Perm and are skipped).
func emittedEventTypes(t *testing.T) map[string]string {
	t.Helper()
	out := map[string]string{}
	fset := token.NewFileSet()
	err := filepath.WalkDir("..", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || d.Name() != "events.go" {
			return nil
		}
		f, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return err
		}
		for _, decl := range f.Decls {
			gd, ok := decl.(*ast.GenDecl)
			if !ok || gd.Tok != token.CONST {
				continue
			}
			for _, spec := range gd.Specs {
				vs := spec.(*ast.ValueSpec)
				for i, name := range vs.Names {
					if i >= len(vs.Values) || strings.HasPrefix(name.Name, "perm") || strings.HasSuffix(name.Name, "Perm") {
						continue
					}
					lit, ok := vs.Values[i].(*ast.BasicLit)
					if !ok || lit.Kind != token.STRING {
						continue
					}
					code, err := strconv.Unquote(lit.Value)
					if err == nil && eventCodePattern.MatchString(code) {
						out[code] = path + ": " + name.Name
					}
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("scan events.go files: %v", err)
	}
	if len(out) < 100 {
		t.Fatalf("found only %d emitted event types; the scan is broken", len(out))
	}
	return out
}

// Every event type a use case emits is in the seeded catalogue (so a
// subscription can pick it), except the known gaps above (none). The client
// events were once catalogued as platform:iam:client:* while the client use
// cases emit platform:admin:client:* (F3), and 79 other codes were missing (F9).
func TestEveryEmittedEventTypeIsCatalogued(t *testing.T) {
	catalogued := map[string]bool{}
	for _, d := range PlatformEventTypes() {
		catalogued[d.Code] = true
	}
	emitted := emittedEventTypes(t)
	var missing []string
	for code, where := range emitted {
		if !catalogued[code] && !knownUncataloguedEventTypes[code] {
			missing = append(missing, code+" ("+where+")")
		}
	}
	sort.Strings(missing)
	for _, m := range missing {
		t.Errorf("emitted but not in the seeded catalogue: %s", m)
	}
	for code := range knownUncataloguedEventTypes {
		if catalogued[code] {
			t.Errorf("%s is catalogued now; remove it from knownUncataloguedEventTypes", code)
		}
		if _, ok := emitted[code]; !ok {
			t.Errorf("%s is no longer emitted; remove it from knownUncataloguedEventTypes", code)
		}
	}
}

// Every catalogued event type is one a use case emits, so the catalogue offers
// no event that never fires. Older seeds catalogued 16 codes nothing emits
// (platform:iam:role:*, platform:admin:idp:*, ...): a subscription made to one
// never received anything. They were dropped and migration 071 removed them
// from existing databases (docs/platform-bug-fixes.md F9).
func TestEveryCataloguedEventTypeIsEmitted(t *testing.T) {
	emitted := emittedEventTypes(t)
	var dead []string
	for _, d := range PlatformEventTypes() {
		if _, ok := emitted[d.Code]; !ok {
			dead = append(dead, d.Code)
		}
	}
	sort.Strings(dead)
	for _, code := range dead {
		t.Errorf("catalogued but no use case emits it: %s", code)
	}
}
