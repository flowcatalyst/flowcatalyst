// Command uowseal runs the uowseal analyzer as a standalone tool.
//
//	go run ./tools/analyzer/uowseal ./internal/platform/...
//
// Or as a go vet plugin:
//
//	go build -o $GOBIN/uowseal ./tools/analyzer/uowseal
//	go vet -vettool=$(which uowseal) ./internal/platform/...
package main

import (
	"golang.org/x/tools/go/analysis/singlechecker"

	"github.com/flowcatalyst/flowcatalyst-go/tools/analyzer/uowseal/analyzer"
)

func main() {
	singlechecker.Main(analyzer.Analyzer)
}
