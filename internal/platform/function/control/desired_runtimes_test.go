package control

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A runner that names no runtimes gets wasm and js only: a jvm version is
// served to a runner that asks for jvm, never by default.
func TestParseRuntimes_JVMOnlyWhenAsked(t *testing.T) {
	def, err := parseRuntimes("")
	require.NoError(t, err)
	assert.Equal(t, map[string]bool{"wasm": true, "js": true}, def)

	jvm, err := parseRuntimes("jvm")
	require.NoError(t, err)
	assert.Equal(t, map[string]bool{"jvm": true}, jvm)

	both, err := parseRuntimes("wasm, jvm")
	require.NoError(t, err)
	assert.Equal(t, map[string]bool{"wasm": true, "jvm": true}, both)
}
