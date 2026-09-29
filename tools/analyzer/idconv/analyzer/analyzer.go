// Package analyzer implements the idconv analyzer.
//
// internal/ids gives each entity identifier its own string type, so a
// ClientID cannot be passed where a PrincipalID is expected. But in Go
// ids.ClientID(anyString) always compiles, and a conversion checks nothing:
// it is the one place a wrong id can still be made to look right. idconv
// reviews every such conversion for the two mistakes that actually happen:
//
//  1. Converting between two id kinds: ids.PrincipalID(clientID). The types
//     exist to stop exactly this.
//  2. Converting another entity's ID field: ids.PrincipalID(ident.ID) where
//     ident is a portal identity. An entity's ID field is the id of that
//     entity, so it may only become the id kind that entity owns
//     (principal.Principal -> PrincipalID, and so on).
//
// It also keeps entity ids out of infrastructure: outside internal/platform,
// internal/server and cmd (the layers that read and write entities), any
// conversion is reported, since the router, queues and stream code have no
// business minting them.
//
// A conversion that is genuinely right but trips a rule is marked
// "//idconv:ok <reason>" on its line or the line above; the reason is
// required. Test files are not checked.
package analyzer

import (
	"go/ast"
	"go/types"
	"strings"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"
)

const modulePath = "github.com/flowcatalyst/flowcatalyst-go/"

const idsPkgPath = modulePath + "internal/ids"

// conversionKinds are the id types whose conversions are reviewed.
var conversionKinds = map[string]bool{
	"ClientID": true, "PrincipalID": true, "ApplicationID": true, "OAuthClientID": true,
}

// owners maps an id kind to the one entity whose ID field it may be built from
// ("import path.Type"). Kinds not listed have no ID-field rule.
var owners = map[string]string{
	"PrincipalID":   modulePath + "internal/platform/principal.Principal",
	"ClientID":      modulePath + "internal/platform/client.Client",
	"ApplicationID": modulePath + "internal/platform/application.Application",
}

// allowedLayers may convert to id types at all.
var allowedLayers = []string{
	modulePath + "internal/ids",
	modulePath + "internal/platform",
	modulePath + "internal/server",
	modulePath + "cmd/",
}

var Analyzer = &analysis.Analyzer{
	Name:     "idconv",
	Doc:      "reviews conversions to internal/ids types: no id-kind to id-kind, no other entity's ID field, none outside the platform layers",
	Requires: []*analysis.Analyzer{inspect.Analyzer},
	Run:      run,
}

func run(pass *analysis.Pass) (any, error) {
	insp := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)
	layerOK := inAllowedLayer(pass.Pkg.Path())

	insp.Preorder([]ast.Node{(*ast.CallExpr)(nil)}, func(n ast.Node) {
		call := n.(*ast.CallExpr)
		kind, ok := conversionTarget(pass, call)
		if !ok {
			return
		}
		pos := pass.Fset.Position(call.Pos())
		if strings.HasSuffix(pos.Filename, "_test.go") || waived(pass, call) {
			return
		}
		if !layerOK {
			pass.Reportf(call.Pos(), "ids.%s conversion outside the platform layers: parse or receive the typed id instead (or //idconv:ok <reason>)", kind)
			return
		}
		arg := call.Args[0]
		if from := idKindOf(pass.TypesInfo.TypeOf(arg)); from != "" && from != kind {
			pass.Reportf(call.Pos(), "converting ids.%s to ids.%s: the types exist to stop this", from, kind)
			return
		}
		if owner, ok := owners[kind]; ok {
			if sel, ok := arg.(*ast.SelectorExpr); ok && sel.Sel.Name == "ID" {
				if src := namedStruct(pass.TypesInfo.TypeOf(sel.X)); src != "" && src != owner && !isRequestShape(src) {
					pass.Reportf(call.Pos(), "ids.%s built from %s.ID: that is not a %s (or //idconv:ok <reason>)", kind, shortName(src), shortName(owner))
				}
			}
		}
	})
	return nil, nil
}

func inAllowedLayer(pkgPath string) bool {
	for _, p := range allowedLayers {
		if pkgPath == strings.TrimSuffix(p, "/") || strings.HasPrefix(pkgPath, p) || strings.HasPrefix(pkgPath, strings.TrimSuffix(p, "/")+"/") {
			return true
		}
	}
	return false
}

// conversionTarget reports the id kind when call is a conversion to one of
// the reviewed ids types.
func conversionTarget(pass *analysis.Pass, call *ast.CallExpr) (string, bool) {
	if len(call.Args) != 1 {
		return "", false
	}
	tv, ok := pass.TypesInfo.Types[call.Fun]
	if !ok || !tv.IsType() {
		return "", false
	}
	if k := idKindOf(tv.Type); k != "" {
		return k, true
	}
	return "", false
}

// idKindOf returns the ids type name of t, or "".
func idKindOf(t types.Type) string {
	named, ok := t.(*types.Named)
	if !ok || named.Obj().Pkg() == nil || named.Obj().Pkg().Path() != idsPkgPath {
		return ""
	}
	if conversionKinds[named.Obj().Name()] {
		return named.Obj().Name()
	}
	return ""
}

// namedStruct returns "pkgpath.Type" for a (pointer to a) named struct type.
func namedStruct(t types.Type) string {
	if t == nil {
		return ""
	}
	if p, ok := t.(*types.Pointer); ok {
		t = p.Elem()
	}
	named, ok := t.(*types.Named)
	if !ok || named.Obj().Pkg() == nil {
		return ""
	}
	if _, ok := named.Underlying().(*types.Struct); !ok {
		return ""
	}
	return named.Obj().Pkg().Path() + "." + named.Obj().Name()
}

// requestSuffixes name the shapes that carry an id in from the wire — a path
// or body field, a command, a DTO. Their ID is whatever the caller sent, and
// converting it is the point where a string becomes a typed id, so rule 2
// (which protects entity ID fields) does not apply to them.
var requestSuffixes = []string{"Input", "Request", "Command", "Body", "DTO", "Response", "Params", "Cmd"}

func isRequestShape(qualified string) bool {
	name := qualified[strings.LastIndex(qualified, ".")+1:]
	for _, s := range requestSuffixes {
		if strings.HasSuffix(name, s) {
			return true
		}
	}
	return false
}

func shortName(q string) string {
	if i := strings.LastIndex(q, "/"); i >= 0 {
		return q[i+1:]
	}
	return q
}

// waived reports whether the conversion carries "//idconv:ok <reason>" on its
// own line or the line above.
func waived(pass *analysis.Pass, call *ast.CallExpr) bool {
	line := pass.Fset.Position(call.Pos()).Line
	file := pass.Fset.File(call.Pos())
	for _, f := range pass.Files {
		if pass.Fset.File(f.Pos()) != file {
			continue
		}
		for _, cg := range f.Comments {
			for _, c := range cg.List {
				text, ok := strings.CutPrefix(c.Text, "//idconv:ok")
				if !ok || strings.TrimSpace(text) == "" {
					continue
				}
				if l := pass.Fset.Position(c.Pos()).Line; l == line || l == line-1 {
					return true
				}
			}
		}
	}
	return false
}
