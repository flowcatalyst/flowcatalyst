// Package analyzer implements the uowseal go vet analyzer.
//
// It enforces the one part of the use-case envelope that the type system
// cannot: that every [usecaseop.Operation] literal sets an Authorize phase.
// A struct literal that omits Authorize compiles fine (the field zeroes to
// nil) and would fail closed at runtime — but a silently-unauthorized write
// operation is exactly the mistake worth catching at build time. Declaring an
// operation intentionally open is still required, just explicit: set
// Authorize to usecaseop.Public.
//
// The check is type-resolved, not name-based: it matches composite literals
// whose type is the named type Operation declared in
// pkg/fcsdk/usecaseop, so it cannot be fooled by an unrelated local type
// called "Operation".
package analyzer

import (
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"
)

// operationPkgPath is the import path of the package that declares Operation.
const operationPkgPath = "github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecaseop"

var Analyzer = &analysis.Analyzer{
	Name:     "uowseal",
	Doc:      "checks that every usecaseop.Operation literal sets an Authorize phase (use usecaseop.Public to declare it intentionally open)",
	Requires: []*analysis.Analyzer{inspect.Analyzer},
	Run:      run,
}

func run(pass *analysis.Pass) (any, error) {
	insp := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)

	filter := []ast.Node{(*ast.CompositeLit)(nil)}
	insp.Preorder(filter, func(n ast.Node) {
		lit := n.(*ast.CompositeLit)
		if !isOperationLiteral(pass, lit) {
			return
		}
		if !setsAuthorize(lit) {
			pass.Reportf(lit.Pos(),
				"usecaseop.Operation literal must set Authorize (a real resource check, or usecaseop.Public to declare it intentionally open)")
		}
	})

	return nil, nil
}

// isOperationLiteral reports whether lit constructs a usecaseop.Operation or
// usecaseop.TxOperation, resolved through type information so a same-named
// local type is not matched.
func isOperationLiteral(pass *analysis.Pass, lit *ast.CompositeLit) bool {
	t := pass.TypesInfo.TypeOf(lit)
	if t == nil {
		return false
	}
	named, ok := t.(*types.Named)
	if !ok {
		return false
	}
	obj := named.Obj()
	if obj == nil || obj.Pkg() == nil {
		return false
	}
	if obj.Name() != "Operation" && obj.Name() != "TxOperation" {
		return false
	}
	return obj.Pkg().Path() == operationPkgPath
}

// setsAuthorize reports whether the literal assigns the Authorize field. Keyed
// literals (the only form used in practice for a cross-package struct) must
// carry an `Authorize:` key; a positional literal sets fields in declaration
// order, where Authorize is the third field (index 2).
func setsAuthorize(lit *ast.CompositeLit) bool {
	if len(lit.Elts) == 0 {
		return false
	}
	if _, keyed := lit.Elts[0].(*ast.KeyValueExpr); keyed {
		for _, elt := range lit.Elts {
			kv, ok := elt.(*ast.KeyValueExpr)
			if !ok {
				continue
			}
			if id, ok := kv.Key.(*ast.Ident); ok && id.Name == "Authorize" {
				return true
			}
		}
		return false
	}
	// Positional: Name, Validate, Authorize, Execute → index 2 must be present.
	return len(lit.Elts) >= 3
}
