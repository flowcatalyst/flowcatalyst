// Command parityharness replays a directory of YAML request files
// against the Rust and Go FlowCatalyst binaries and reports any
// wire-format divergence between them. Highest-leverage tool for
// verifying the drop-in compatibility claim.
//
// # Usage
//
//	parityharness -rust=http://localhost:3001 -go=http://localhost:3000 \
//	              -dir=tests/parity/requests
//
// Both binaries must be running against equivalent state (same
// migrations, same seed data). The operator brings them up; the
// harness only diffs responses.
//
// # YAML schema
//
// Each file under -dir describes one request:
//
//	name: "Human-readable description"
//	request:
//	  method: GET
//	  path: /api/event-types
//	  headers:
//	    Authorization: "Bearer ${ANCHOR_TOKEN}"   # ${VAR} substitution
//	  body: |
//	    {...}                                      # optional
//	expect:
//	  status: 200                                  # optional; both must match if present
//	  body_shape:                                  # optional; placeholder-typed shape
//	    items: any-array
//
// Headers (and the body, and path) support `${VAR}` substitution from
// the harness's environment. Missing vars cause the case to skip with
// a clear message — not fail — so a partial parity run (no token) still
// exercises the unauthenticated cases.
//
// # Exit codes
//
//	0 — every case matched (or was deliberately skipped)
//	1 — at least one case diverged
//	2 — bad flags / load error
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"
)

func main() {
	rustURL := flag.String("rust", "", "Base URL of the Rust binary (required)")
	goURL := flag.String("go", "", "Base URL of the Go binary (required)")
	dir := flag.String("dir", "tests/parity/requests", "Directory of *.yaml request files (recursive)")
	only := flag.String("only", "", "Only run cases whose name matches this substring")
	verbose := flag.Bool("v", false, "Print every case, not just failures")
	timeout := flag.Duration("timeout", 10*time.Second, "Per-request timeout")
	flag.Parse()

	if *rustURL == "" || *goURL == "" {
		flag.Usage()
		os.Exit(2)
	}

	cases, err := loadCases(*dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load cases: %v\n", err)
		os.Exit(2)
	}
	if len(cases) == 0 {
		slog.Warn("no parity cases found", "dir", *dir)
		os.Exit(0)
	}

	client := &http.Client{Timeout: *timeout}
	r := &runner{
		rustURL: strings.TrimRight(*rustURL, "/"),
		goURL:   strings.TrimRight(*goURL, "/"),
		client:  client,
	}

	ctx := context.Background()
	var passed, failed, skipped int
	var failures []caseResult
	for _, c := range cases {
		if *only != "" && !strings.Contains(c.Name, *only) {
			continue
		}
		res := r.run(ctx, c)
		switch res.status {
		case statusPassed:
			passed++
			if *verbose {
				fmt.Printf("PASS  %s\n", c.Name)
			}
		case statusSkipped:
			skipped++
			if *verbose {
				fmt.Printf("SKIP  %s — %s\n", c.Name, res.detail)
			}
		case statusFailed:
			failed++
			failures = append(failures, res)
			fmt.Printf("FAIL  %s\n%s\n", c.Name, indent(res.detail, "    "))
		}
	}

	// Stable failure summary at the end so the failures are easy to
	// re-find when running in CI logs.
	if len(failures) > 0 {
		fmt.Println()
		fmt.Println("== Failures ==")
		sort.Slice(failures, func(i, j int) bool { return failures[i].name < failures[j].name })
		for _, f := range failures {
			fmt.Println(" -", f.name)
		}
	}
	fmt.Printf("\n%d passed, %d failed, %d skipped (of %d total)\n",
		passed, failed, skipped, len(cases))
	if failed > 0 {
		os.Exit(1)
	}
}

func indent(s, prefix string) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	for i, l := range lines {
		lines[i] = prefix + l
	}
	return strings.Join(lines, "\n")
}
