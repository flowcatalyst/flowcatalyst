package runner

import (
	"strings"
	"testing"

	"github.com/flowcatalyst/flowcatalyst-go/internal/functions/abi"
	"github.com/flowcatalyst/flowcatalyst-go/internal/functions/control"
)

// The pool and connection of a dispatch are the describe's. A guest that puts
// either key in the call's meta is refused (BAD_REQUEST), never silently
// dispatched to a pool it did not get, and the platform is not asked.
func TestDispatchCallCannotNameAPoolOrConnection(t *testing.T) {
	h := start(t, fnDoc(ver(1, control.RoleLive)))
	for _, key := range []string{"pool", "connection"} {
		meta := map[string]any{"target": "app.logger", "path": "/deliver", key: "other"}
		code, m, _ := hostCall(t, h, abi.OpDispatch, meta, "{}")
		if code != "1" || !strings.Contains(m, abi.CodeBadRequest) {
			t.Errorf("%s in the call: %s %s, want BAD_REQUEST", key, code, m)
		}
	}
	if n := len(h.cp.dispatches); n != 0 {
		t.Fatalf("the platform was asked %d times for refused calls", n)
	}
}
