// Package jsengine embeds the function runner's shared JavaScript engine:
// QuickJS behind ABI v1, built from clients/fn-js-engine (see its build.sh).
// The runner compiles it once and loads each JS function's script into its
// own instances through the LoadExport export, so a JS function costs its
// instances only, never its own copy of the engine.
package jsengine

import (
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
)

// Wasm is the compiled engine.
//
//go:embed engine.wasm
var Wasm []byte

// LoadExport is the engine export that loads a function's script into an
// instance: (ptr, len) of a buffer from fc_alloc → 0 on success.
const LoadExport = "fc_js_load"

// Digest is the engine's sha256, lowercase hex.
func Digest() string {
	s := sha256.Sum256(Wasm)
	return hex.EncodeToString(s[:])
}
