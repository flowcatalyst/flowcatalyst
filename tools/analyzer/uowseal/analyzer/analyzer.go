// Package analyzer implements the uowseal go vet analyzer.
//
// It asserts that every method named Execute on a *UseCase struct ends
// in either:
//   - a call to a usecase.UnitOfWork method (Commit, CommitDelete,
//     CommitAll, EmitEvent) — the happy path; or
//   - a usecase.Failure(...) call — the error path.
//
// This is belt-and-suspenders alongside the compile-time seal pattern.
// The seal already prevents constructing a Success outside the usecase
// package; this analyzer additionally catches "all code paths return
// Failure and you forgot to call UoW at all" — which compiles but means
// no event/audit row ever gets written.
package analyzer

import (
	"go/ast"
	"strings"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"
)

var Analyzer = &analysis.Analyzer{
	Name:     "uowseal",
	Doc:      "checks that *UseCase.Execute methods end in uow.Commit* or usecase.Failure",
	Requires: []*analysis.Analyzer{inspect.Analyzer},
	Run:      run,
}

func run(pass *analysis.Pass) (any, error) {
	insp := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)

	filter := []ast.Node{(*ast.FuncDecl)(nil)}
	insp.Preorder(filter, func(n ast.Node) {
		fn := n.(*ast.FuncDecl)
		if fn.Name == nil || fn.Name.Name != "Execute" {
			return
		}
		if fn.Recv == nil || len(fn.Recv.List) == 0 {
			return
		}
		// receiver must be *XxxUseCase
		recvType := receiverTypeName(fn.Recv.List[0])
		if !strings.HasSuffix(recvType, "UseCase") {
			return
		}
		if fn.Body == nil {
			return
		}

		// Examine every terminal return statement in the function body.
		// Each must be one of:
		//   return <something>.Commit*(...)   (UoW happy path)
		//   return usecase.Failure[E](...)    (explicit failure)
		// We collect violations and report them.
		ast.Inspect(fn.Body, func(node ast.Node) bool {
			ret, ok := node.(*ast.ReturnStmt)
			if !ok {
				return true
			}
			if len(ret.Results) == 0 {
				return true // bare return — fine
			}
			expr := ret.Results[0]
			if isLegalUseCaseReturn(expr) {
				return true
			}
			pass.Reportf(ret.Pos(),
				"UseCase.Execute must return usecase.Failure(...) or uow.Commit*(...); got %T", expr)
			return true
		})
	})

	return nil, nil
}

func isLegalUseCaseReturn(expr ast.Expr) bool {
	call, ok := expr.(*ast.CallExpr)
	if !ok {
		return false
	}
	switch fn := call.Fun.(type) {
	case *ast.SelectorExpr:
		// e.g. uc.uow.Commit(...) or usecase.Failure(...) or pkg.Failure(...)
		name := fn.Sel.Name
		if strings.HasPrefix(name, "Commit") || name == "EmitEvent" || name == "Failure" {
			return true
		}
	case *ast.IndexExpr, *ast.IndexListExpr:
		// Generic call: usecase.Failure[E](...) — the Fun is X[T]
		// Recurse into the indexed expression.
		var inner ast.Expr
		switch f := fn.(type) {
		case *ast.IndexExpr:
			inner = f.X
		case *ast.IndexListExpr:
			inner = f.X
		}
		if sel, ok := inner.(*ast.SelectorExpr); ok {
			name := sel.Sel.Name
			if strings.HasPrefix(name, "Commit") || name == "EmitEvent" || name == "Failure" {
				return true
			}
		}
	}
	return false
}

func receiverTypeName(field *ast.Field) string {
	switch t := field.Type.(type) {
	case *ast.StarExpr:
		if id, ok := t.X.(*ast.Ident); ok {
			return id.Name
		}
	case *ast.Ident:
		return t.Name
	}
	return ""
}
