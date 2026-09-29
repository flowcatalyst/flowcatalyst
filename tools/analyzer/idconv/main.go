// Command idconv runs the idconv analyzer as a standalone tool.
//
//	go run ./tools/analyzer/idconv ./...
package main

import (
	"golang.org/x/tools/go/analysis/singlechecker"

	"github.com/flowcatalyst/flowcatalyst-go/tools/analyzer/idconv/analyzer"
)

func main() {
	singlechecker.Main(analyzer.Analyzer)
}
