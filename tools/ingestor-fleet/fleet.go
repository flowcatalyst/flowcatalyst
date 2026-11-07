package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"syscall"
	"time"
)

// runtimeProc is one ingestor runtime process the test started. Restarting
// keeps its runtime id, as a restarted task would.
type runtimeProc struct {
	n      int
	id     string
	port   int
	dir    string
	cmd    *exec.Cmd
	done   chan struct{}
	start  time.Time
	starts int
}

func (r *runtimeProc) running() bool {
	if r.cmd == nil {
		return false
	}
	select {
	case <-r.done:
		return false
	default:
		return true
	}
}

func (r *runtimeProc) pid() int {
	if r.cmd == nil || r.cmd.Process == nil {
		return 0
	}
	return r.cmd.Process.Pid
}

// fleet owns the runtime processes. Every process it starts is recorded in
// <state>/pids so the script can stop a leftover by PID if this program dies.
type fleet struct {
	cfg   config
	creds runtimeCredentials
	mu    sync.Mutex
	procs []*runtimeProc
}

func newFleet(cfg config, creds runtimeCredentials) (*fleet, error) {
	f := &fleet{cfg: cfg, creds: creds}
	for i, port := range cfg.runtimePorts {
		n := i + 1
		dir := filepath.Join(cfg.state, fmt.Sprintf("rt-%d", n))
		if err := os.MkdirAll(filepath.Join(dir, "config"), 0o755); err != nil {
			return nil, err
		}
		// The jar does not start without config/application.yaml; the
		// environment overrides every value.
		yaml := fmt.Sprintf("http:\n  port: %d\n\ningest:\n  camel-routes-enabled: false\n\nflowcatalyst:\n  base-url: %s\n  application: ingestor\n  receivers:\n    enabled: false\n", port, cfg.apiURL)
		if err := os.WriteFile(filepath.Join(dir, "config", "application.yaml"), []byte(yaml), 0o600); err != nil {
			return nil, err
		}
		f.procs = append(f.procs, &runtimeProc{n: n, id: fmt.Sprintf("fleet-rt-%d", n), port: port, dir: dir})
	}
	return f, nil
}

func (f *fleet) env(r *runtimeProc) []string {
	return []string{
		"PATH=/usr/bin:/bin",
		"HOME=" + filepath.Join(f.cfg.state, "home"),
		"TMPDIR=" + filepath.Join(f.cfg.state, "tmp"),
		"LANG=C.UTF-8",
		"APP_ENV=local",
		"FLOWCATALYST_BASE_URL=" + f.cfg.apiURL,
		"FLOWCATALYST_APPLICATION=ingestor",
		"FLOWCATALYST_CLIENT_ID=" + f.creds.clientID,
		"FLOWCATALYST_CLIENT_SECRET=" + f.creds.secret,
		"INGEST_RUNTIME_ID=" + r.id,
		"INGEST_POLL_INTERVAL=" + isoSeconds(f.cfg.poll),
		"INGEST_FUNCTIONS_ENABLED=false",
		// The runtime ends itself when this program is gone (ParentWatch).
		"INGEST_PARENT_PID=" + strconv.Itoa(os.Getpid()),
		"HTTP_PORT=" + strconv.Itoa(r.port),
		"HTTP_HOST=127.0.0.1",
		// Postgres trusts loopback; the source still names a secret.
		dbSecretEnvVar + "=unused",
	}
}

// start launches runtime n (1-based) again, appending to its log.
func (f *fleet) start(n int) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	r := f.procs[n-1]
	if r.running() {
		return fmt.Errorf("%s is already running (pid %d)", r.id, r.pid())
	}
	logPath := filepath.Join(f.cfg.state, "logs", r.id+".log")
	lf, err := os.OpenFile(logPath, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o600) //nolint:gosec // G304: the test's own log
	if err != nil {
		return err
	}
	fmt.Fprintf(lf, "\n--- ingestor-fleet started %s at %s ---\n", r.id, wallNow().Format(time.RFC3339Nano))
	cmd := exec.Command(f.cfg.java, //nolint:gosec // G204: the java and jar the script chose
		"-Xmx384m", "-XX:+UseCompactObjectHeaders",
		// Resolves the sources' host names (and localhost) to 127.0.0.1 inside
		// this JVM only: the platform refuses a loopback address in a saved
		// definition, the runtime (APP_ENV=local) allows one.
		"-Djdk.net.hosts.file="+filepath.Join(f.cfg.state, "hosts"),
		"-jar", f.cfg.jar)
	cmd.Dir = r.dir
	cmd.Env = f.env(r)
	cmd.Stdout, cmd.Stderr = lf, lf
	if err := cmd.Start(); err != nil {
		_ = lf.Close()
		return err
	}
	r.cmd, r.done, r.start = cmd, make(chan struct{}), wallNow()
	r.starts++
	pidFile := filepath.Join(f.cfg.state, "pids", r.id+".pid")
	_ = os.WriteFile(pidFile, []byte(strconv.Itoa(cmd.Process.Pid)+"\n"), 0o600)
	done := r.done
	go func() {
		_ = cmd.Wait()
		_ = lf.Close()
		_ = os.Remove(pidFile)
		close(done)
	}()
	return nil
}

// kill sends SIGKILL to runtime n and waits for it to be gone.
func (f *fleet) kill(n int) error {
	r := f.procs[n-1]
	if !r.running() {
		return fmt.Errorf("%s is not running", r.id)
	}
	if err := r.cmd.Process.Signal(syscall.SIGKILL); err != nil {
		return err
	}
	select {
	case <-r.done:
		return nil
	case <-time.After(10 * time.Second):
		return fmt.Errorf("%s (pid %d) did not end 10 s after SIGKILL", r.id, r.pid())
	}
}

// terminate sends SIGTERM to runtime n and waits up to within for it to end;
// past that it is killed. It answers how long the stop took and whether it
// ended by itself.
func (f *fleet) terminate(n int, within time.Duration) (time.Duration, bool, error) {
	r := f.procs[n-1]
	if !r.running() {
		return 0, false, fmt.Errorf("%s is not running", r.id)
	}
	at := wallNow()
	if err := r.cmd.Process.Signal(syscall.SIGTERM); err != nil {
		return 0, false, err
	}
	select {
	case <-r.done:
		return time.Since(at), true, nil
	case <-time.After(within):
	}
	_ = r.cmd.Process.Signal(syscall.SIGKILL)
	select {
	case <-r.done:
		return time.Since(at), false, nil
	case <-time.After(10 * time.Second):
		return time.Since(at), false, errors.New(r.id + " did not end after SIGKILL")
	}
}

// running answers the processes that are up.
func (f *fleet) running() []*runtimeProc {
	var out []*runtimeProc
	for _, r := range f.procs {
		if r.running() {
			out = append(out, r)
		}
	}
	return out
}

// stopAll ends every runtime: SIGTERM, then SIGKILL after 20 s.
func (f *fleet) stopAll() {
	var wg sync.WaitGroup
	for _, r := range f.procs {
		if !r.running() {
			continue
		}
		wg.Go(func() { _, _, _ = f.terminate(r.n, 20*time.Second) })
	}
	wg.Wait()
}
