// Command sizeprobe is the smallest program that compiles and instantiates
// the shipped JS engine: on wazero without the wasmtime tag, on wasmtime with
// it. The two binaries' sizes are what each engine adds to a server image.
package main

import (
	"context"
	"fmt"
	"os"
)

func main() {
	if err := probe(context.Background()); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
