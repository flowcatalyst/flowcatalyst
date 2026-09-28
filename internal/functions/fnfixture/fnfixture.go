// Package fnfixture is the raw ABI v1 guest the function runner's tests run:
// a Rust module (source in guest/) whose behaviour is chosen by the request
// path — /echo, /meta, /count, /spin, /grow?mb=N, /trap, /malformed and
// /call?op=N (the request body is passed to fc.call as a host-call frame).
//
// Rebuild after editing guest/src/lib.rs:
//
//	cargo build --release --target wasm32-unknown-unknown --manifest-path internal/functions/fnfixture/guest/Cargo.toml
//	cp internal/functions/fnfixture/guest/target/wasm32-unknown-unknown/release/fixture_guest.wasm internal/functions/fnfixture/fixture.wasm
package fnfixture

import (
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
)

// Wasm is the compiled fixture module.
//
//go:embed fixture.wasm
var Wasm []byte

// Digest is Wasm's sha256, lowercase hex.
func Digest() string {
	s := sha256.Sum256(Wasm)
	return hex.EncodeToString(s[:])
}
