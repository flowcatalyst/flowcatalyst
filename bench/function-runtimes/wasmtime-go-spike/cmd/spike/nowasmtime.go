//go:build !wasmtime

// Without the wasmtime tag the command is empty: the spike module then builds
// with CGO_ENABLED=0 and links no wasmtime, which is the isolation the main
// module would rely on (see the README).
package main

import (
	"fmt"
	"os"

	"github.com/flowcatalyst/flowcatalyst-go/bench/function-runtimes/wasmtime-go-spike/wtengine"
)

func main() {
	fmt.Println("built without -tags wasmtime; wtengine.Available =", wtengine.Available)
	os.Exit(2)
}
