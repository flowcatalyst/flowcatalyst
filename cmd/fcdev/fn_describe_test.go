package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/flowcatalyst/flowcatalyst-go/internal/functions/abi"
)

// build/describe/deploy --dry-run print the declared pool and connection.
func TestPrintDescribeShowsDeclaredDispatchPlacement(t *testing.T) {
	d := &abi.Describe{ABI: 1, Dispatches: []abi.DispatchTarget{
		{Target: "yard.logger", Path: "/deliver", Pool: "yard-process-pool", Connection: "yard-conn"},
		{Target: "yard.logger", Path: "/pool-only", Pool: "tower-pool"},
		{Target: "yard.logger", Path: "/plain"},
	}}
	var b bytes.Buffer
	printDescribeTo(&b, "f.wasm", d)
	out := b.String()
	for _, want := range []string{
		"yard.logger/deliver (pool yard-process-pool, connection yard-conn)",
		"yard.logger/pool-only (pool tower-pool)\n",
		"yard.logger/plain\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("describe output lacks %q:\n%s", want, out)
		}
	}
}
