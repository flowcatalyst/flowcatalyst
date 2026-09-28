package runner

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/functions/budget"
	"github.com/flowcatalyst/flowcatalyst-go/internal/functions/control"
)

// TestLoad serves the fixture function through the runner over real HTTP and
// drives it with wrk: the runner's own overhead per call (the fixture's echo
// does no work). Opt-in, and directional — the load generator shares the
// machine:
//
//	FN_BENCH=1 go test ./internal/functions/runner -run TestLoad -v
//	FN_BENCH=serve FN_BENCH_ADDR=0.0.0.0:18195 go test … -run TestLoad   # serve only (drive wrk from elsewhere)
func TestLoad(t *testing.T) {
	mode := os.Getenv("FN_BENCH")
	if mode == "" {
		t.Skip("FN_BENCH not set")
	}
	fn := fnDoc(ver(1, control.RoleLive))
	fn.Limits.MaxConcurrency = 4096
	b, _ := budget.New(1<<30, 0)
	cp := newFakeCP()
	cp.push(control.Desired{Pool: "default", Functions: []control.Function{fn}})
	r, err := New(t.Context(), Config{Pool: "default", ControlPlane: cp, Budget: b, Tokens: fakeTokens{}})
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = r.Run(t.Context()) }()
	h := &harness{t: t, cp: cp, r: r}
	h.waitServing("app.hello", 1)

	addr := os.Getenv("FN_BENCH_ADDR")
	if addr == "" {
		addr = "127.0.0.1:0"
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	srv := &http.Server{Handler: r.Handler(), ReadHeaderTimeout: 10 * time.Second}
	go func() { _ = srv.Serve(ln) }()
	defer func() { _ = srv.Close() }()
	url := fmt.Sprintf("http://%s/fn/app.hello/echo", ln.Addr())
	if mode == "serve" {
		t.Logf("serving %s for 5 minutes", url)
		time.Sleep(5 * time.Minute)
		return
	}
	for _, c := range []int{64, 256} {
		out, err := exec.Command("wrk", "-t4", fmt.Sprintf("-c%d", c), "-d10s", "--latency", url).CombinedOutput()
		if err != nil {
			t.Fatalf("wrk: %v\n%s", err, out)
		}
		for line := range strings.SplitSeq(string(out), "\n") {
			if strings.Contains(line, "Requests/sec") || strings.Contains(line, "50%") || strings.Contains(line, "99%") || strings.Contains(line, "Non-2xx") {
				t.Logf("c=%d %s", c, strings.TrimSpace(line))
			}
		}
	}
	s := b.Stats()
	t.Logf("guest memory committed after load: %d MB", s.UsedBytes>>20)
}
