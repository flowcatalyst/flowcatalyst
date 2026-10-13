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

// javaRustFunctionEventTypes are the 14 function event types Java
// (FunctionEvents) and Rust (fc-platform-functions events.rs) emit. The owner
// ruled (2026-10-12) that the function events use this family.
var javaRustFunctionEventTypes = map[string]bool{
	"platform:function:function:created":  true,
	"platform:function:function:updated":  true,
	"platform:function:function:deleted":  true,
	"platform:function:version:published": true,
	"platform:function:version:ready":     true,
	"platform:function:version:retired":   true,
	"platform:function:alias:changed":     true,
	"platform:function:alias:removed":     true,
	"platform:function:policy:updated":    true,
	"platform:function:config:updated":    true,
	"platform:function:secret:set":        true,
	"platform:function:secret:deleted":    true,
	"platform:function:domain:claimed":    true,
	"platform:function:domain:released":   true,
}

// goOnlyFunctionEventTypes are the function event types Go emits that Java
// and Rust have no counterpart for, so they keep their platform:admin:function*
// codes, source and data (docs/platform-bug-fixes.md F11, owner ruling
// 2026-10-13: reshape to Java/Rust where a mapping exists, keep the rest):
//
//   - function:promoted: the summary of the wiring promote reconciles
//     (dispatch pool, subscriptions, scheduled jobs). Java and Rust promote
//     emits only alias:changed, which Go now emits too.
//   - function-schedule:wired: one per scheduled job that reconciliation
//     creates, updates or deletes.
//   - function-route:{set,deleted}: Go's public routes are their own records;
//     Java's and Rust's routes live in the version manifest and emit nothing.
//   - function-setting:{set,deleted}: now only for DB settings. Java and Rust
//     have no DB kind; CONFIG and SECRET settings emit config:updated and
//     secret:{set,deleted}.
//
// The list may only shrink: a new platform:admin:function* code fails, and so
// does an entry no longer emitted.
var goOnlyFunctionEventTypes = map[string]bool{
	"platform:admin:function:promoted":        true,
	"platform:admin:function-schedule:wired":  true,
	"platform:admin:function-route:set":       true,
	"platform:admin:function-route:deleted":   true,
	"platform:admin:function-setting:set":     true,
	"platform:admin:function-setting:deleted": true,
}

// javaRustOnlyFunctionEventTypes are the Java/Rust function events Go has no
// operation for: version:ready (Go marks a version READY from the runner
// heartbeat without an event) and policy:updated (Go has no signer policy).
var javaRustOnlyFunctionEventTypes = map[string]bool{
	"platform:function:version:ready":  true,
	"platform:function:policy:updated": true,
}

// Go's function events stay in step with Java's and Rust's: a code Go emits in
// the platform:function:* family is one of theirs (Go does not invent codes in
// it), Go emits every one of theirs it has an operation for, and the Go-only
// platform:admin:function* family only shrinks (F11).
func TestFunctionEventTypesFollowJavaAndRust(t *testing.T) {
	emitted := emittedEventTypes(t)
	var codes []string
	for code := range emitted {
		codes = append(codes, code)
	}
	sort.Strings(codes)
	for _, code := range codes {
		switch {
		case strings.HasPrefix(code, "platform:function:"):
			if !javaRustFunctionEventTypes[code] {
				t.Errorf("%s (%s) is in the platform:function family but Java and Rust do not emit it", code, emitted[code])
			}
		case strings.HasPrefix(code, "platform:admin:function"):
			if !goOnlyFunctionEventTypes[code] {
				t.Errorf("%s (%s) is a new platform:admin:function* code; function events use platform:function:*", code, emitted[code])
			}
		}
	}
	for code := range goOnlyFunctionEventTypes {
		if _, ok := emitted[code]; !ok {
			t.Errorf("%s is no longer emitted; remove it from goOnlyFunctionEventTypes", code)
		}
	}
	for code := range javaRustFunctionEventTypes {
		_, ok := emitted[code]
		switch {
		case !ok && !javaRustOnlyFunctionEventTypes[code]:
			t.Errorf("Go does not emit %s", code)
		case ok && javaRustOnlyFunctionEventTypes[code]:
			t.Errorf("Go emits %s now; remove it from javaRustOnlyFunctionEventTypes", code)
		}
	}
}
