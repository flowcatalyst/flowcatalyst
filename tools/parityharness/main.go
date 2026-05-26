// Command parityharness replays HTTP requests against both the Rust
// and Go FlowCatalyst binaries and diffs their responses.
//
// Phase 0: skeleton. Wired up properly in Phase 3 with the first
// subdomain port (event_type).
//
// Usage:
//
//	parityharness -rust=http://localhost:3001 -go=http://localhost:3002 -dir=tests/parity/requests
package main

import (
	"flag"
	"fmt"
	"log/slog"
	"os"
)

func main() {
	rustURL := flag.String("rust", "", "URL of the Rust binary (required)")
	goURL := flag.String("go", "", "URL of the Go binary (required)")
	dir := flag.String("dir", "tests/parity/requests", "directory of request YAML files")
	flag.Parse()

	if *rustURL == "" || *goURL == "" {
		flag.Usage()
		os.Exit(2)
	}

	slog.Info("parityharness scaffold", "rust", *rustURL, "go", *goURL, "dir", *dir)
	fmt.Println("TODO(phase-3a): implement YAML loader + request replay + JSON diff")
	os.Exit(0)
}
