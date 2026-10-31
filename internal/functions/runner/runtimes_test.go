package runner

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/functions/runtimes"
)

// The in-process runner never asks for jvm versions (the ingestor runtime's
// function host serves those), and every runtime it asks for it can prepare.
func TestSupportedRuntimes_NeverJVM(t *testing.T) {
	got := SupportedRuntimes()
	assert.NotContains(t, got, runtimes.JVM)
	require.NotEmpty(t, got)
	for _, r := range got {
		assert.True(t, runtimes.Valid(r), "%s must be preparable here", r)
	}
	assert.False(t, runtimes.Valid(runtimes.JVM), "a jvm version that slips through is skipped, not run")
}
