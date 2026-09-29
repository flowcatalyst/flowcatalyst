package analyzer_test

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"

	"github.com/flowcatalyst/flowcatalyst-go/tools/analyzer/idconv/analyzer"
)

func TestIDConv(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), analyzer.Analyzer,
		"github.com/flowcatalyst/flowcatalyst-go/internal/platform/portalidentity",
		"github.com/flowcatalyst/flowcatalyst-go/internal/router",
	)
}
