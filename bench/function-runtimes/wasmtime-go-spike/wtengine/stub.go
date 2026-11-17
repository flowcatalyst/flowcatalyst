//go:build !wasmtime

// Package wtengine is the wasmtime engine of the spike. This file is what a
// build without the wasmtime tag gets: nothing, and no cgo. It is how the
// main module would keep fcdev and the default `go build` pure Go.
package wtengine

// Available reports whether the binary was built with wasmtime.
const Available = false
