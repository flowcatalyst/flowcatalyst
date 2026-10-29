package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// This file runs the ingestor runtime (the Java program built from ./ingestor)
// beside the dev platform: fcdev provisions its service account, makes sure it
// has its own JRE and a jar, starts the JVM as a child process and keeps it
// running. See docs/fcdev-ingestor.md. None of it may stop fcdev from
// starting: every failure here is a WARN and the ingestor is skipped.

// ingestorLocalClientID is the fixed client_id of the dev ingestor runtime's
// OAuth client (see bootstrapLocalCredentials).
const ingestorLocalClientID = "fcdev-ingestor"

// ingestorRole is the whole authority the dev ingestor has.
const ingestorRole = "platform:ingestion-runtime"

const (
	defaultIngestorPort  = 8091
	ingestorReadyWait    = 90 * time.Second
	ingestorPlatformWait = 3 * time.Minute
)

// ingestorConfig is everything one ingestor child needs.
type ingestorConfig struct {
	Port          int
	APIPort       int
	StateDir      string
	JarOverride   string
	ExtensionsDir string
	Creds         routerCredentials
}

// devIngestionEnabled mirrors devEnvCfg: ingestion is on in dev unless
// FC_INGESTION_ENABLED says otherwise.
func devIngestionEnabled() bool { return envBoolDefault("FC_INGESTION_ENABLED", true) }

// ingestorStateDir is the per-instance working directory (config/, the log,
// the pid file). Keyed by the API port so two fcdevs do not collide.
func ingestorStateDir(apiPort int) string {
	if d := os.Getenv("FC_INGESTOR_STATE_DIR"); d != "" {
		return d
	}
	return filepath.Join(userDataDir(), "flowcatalyst", "fcdev-ingestor", "api-"+strconv.Itoa(apiPort))
}

func ingestorConfigFromOpts(opts startOpts, creds routerCredentials) ingestorConfig {
	return ingestorConfig{
		Port:          opts.IngestorPort,
		APIPort:       opts.APIPort,
		StateDir:      ingestorStateDir(opts.APIPort),
		JarOverride:   os.Getenv("FC_INGESTOR_JAR"),
		ExtensionsDir: ingestorExtensionsDir(),
		Creds:         creds,
	}
}

// extensionJars lists the jar files in the extensions folder, by name. A
// folder that is not one is an error naming FC_INGESTOR_EXTENSIONS_DIR, the
// only way it can be wrong: the runtime would refuse it and never start.
func extensionJars(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("the extensions folder %s cannot be read (%w); FC_INGESTOR_EXTENSIONS_DIR must name a folder of extension jars, or be unset for ~/.flowcatalyst/ingestor-extensions", dir, err)
	}
	var jars []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".jar") {
			jars = append(jars, e.Name())
		}
	}
	return jars, nil
}

// ingestorExitCause is why the runtime's latest run ended, from the end of its
// log: the startup failure and the problems it lists, or else the last
// exception line. Empty when the log says nothing recognisable.
func ingestorExitCause(logPath string) string {
	f, err := os.Open(logPath) //nolint:gosec // G304: fcdev's own log file
	if err != nil {
		return ""
	}
	defer func() { _ = f.Close() }()
	const tail = 64 << 10
	if st, err := f.Stat(); err == nil && st.Size() > tail {
		_, _ = f.Seek(st.Size()-tail, io.SeekStart)
	}
	data, err := io.ReadAll(f)
	if err != nil {
		return ""
	}
	text := string(data)
	if i := strings.LastIndex(text, "--- fcdev restarted the ingestor runtime"); i >= 0 {
		text = text[i:]
	}
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		at := strings.Index(line, "StartupException")
		if at < 0 {
			continue
		}
		parts := []string{strings.TrimSpace(line[at:])}
		for _, next := range lines[i+1:] {
			if !strings.HasPrefix(next, "  - ") {
				break
			}
			parts = append(parts, strings.TrimSpace(next[len("  - "):]))
		}
		return strings.Join(parts, "; ")
	}
	for _, line := range slices.Backward(lines) {
		if strings.Contains(line, "Exception") && !strings.HasPrefix(strings.TrimSpace(line), "at ") {
			return strings.TrimSpace(line)
		}
	}
	return ""
}

// ingestorExtensionsDir is the folder of extension jars the runtime loads:
// FC_INGESTOR_EXTENSIONS_DIR, else ~/.flowcatalyst/ingestor-extensions, where
// an extension's build drops its jar (it is created, so the build and fcdev
// meet without either being told). Empty, and no extensions, when neither can
// be had.
func ingestorExtensionsDir() string {
	if d := os.Getenv("FC_INGESTOR_EXTENSIONS_DIR"); d != "" {
		return d
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	d := filepath.Join(home, ".flowcatalyst", "ingestor-extensions")
	if err := os.MkdirAll(d, 0o750); err != nil {
		slog.Warn("no ingestor extensions folder; the runtime starts without extensions", "dir", d, "err", err)
		return ""
	}
	return d
}

// applicationYAML is the config/application.yaml the jar insists on. Every
// value is overridden by the environment fcdev sets, so this carries only the
// shape.
func applicationYAML(cfg ingestorConfig) string {
	return fmt.Sprintf(`# Written by fcdev on every start; edit nothing here. The environment wins.
http:
  port: %d

ingest:
  camel-routes-enabled: false

flowcatalyst:
  base-url: http://localhost:%d
  application: ingestor
  receivers:
    enabled: false
`, cfg.Port, cfg.APIPort)
}

// writeIngestorConfig creates the working directory and its config file.
func writeIngestorConfig(cfg ingestorConfig) error {
	dir := filepath.Join(cfg.StateDir, "config")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "application.yaml"), []byte(applicationYAML(cfg)), 0o600)
}

// inheritedEnv are the variables a child needs from fcdev's own environment.
// Nothing else is inherited: the developer's shell may hold FC_* secrets or
// FLOWCATALYST_* settings of another platform.
var inheritedEnv = []string{
	"PATH", "HOME", "USER", "LOGNAME", "LANG", "LC_ALL", "TMPDIR", "TMP", "TEMP",
	"USERPROFILE", "SystemRoot", "SYSTEMROOT", "APPDATA", "LOCALAPPDATA", "HOMEDRIVE", "HOMEPATH", "HTTP_PROXY", "HTTPS_PROXY", "NO_PROXY",
}

// ingestorEnv builds the child's environment. parent is fcdev's environment
// (KEY=VALUE): the allow-listed variables and any INGEST_* the developer set
// (INGEST_SECRET_* for ingestions that read a secret from the environment)
// pass through; fcdev's own values override them.
func ingestorEnv(parent []string, cfg ingestorConfig, jre jreInfo) []string {
	env := map[string]string{}
	allowed := map[string]bool{}
	for _, k := range inheritedEnv {
		allowed[strings.ToUpper(k)] = true
	}
	for _, kv := range parent {
		k, v, ok := strings.Cut(kv, "=")
		if !ok {
			continue
		}
		if allowed[strings.ToUpper(k)] || strings.HasPrefix(k, "INGEST_") {
			env[k] = v
		}
	}
	// Locally the extensions are always the folder fcdev chose (cfg), never a
	// deployed runtime's S3 manifest or a folder left in the shell.
	delete(env, "INGEST_EXTENSIONS_DIR")
	delete(env, "INGEST_EXTENSIONS_MANIFEST")
	env["JAVA_HOME"] = jre.Home
	env["FLOWCATALYST_BASE_URL"] = fmt.Sprintf("http://localhost:%d", cfg.APIPort)
	env["FLOWCATALYST_CLIENT_ID"] = cfg.Creds.ClientID
	env["FLOWCATALYST_CLIENT_SECRET"] = cfg.Creds.Secret
	env["INGEST_RUNTIME_ID"] = "fcdev-" + strconv.Itoa(cfg.APIPort)
	// The runtime ends itself once fcdev is gone (ParentWatch): off Linux no
	// signal reaches it when fcdev is killed hard.
	env["INGEST_PARENT_PID"] = strconv.Itoa(os.Getpid())
	env["HTTP_PORT"] = strconv.Itoa(cfg.Port)
	env["APP_ENV"] = "local"
	if _, set := env["INGEST_POLL_INTERVAL"]; !set {
		env["INGEST_POLL_INTERVAL"] = "PT15S" // dev wants quick feedback; the lease is 3 minutes
	}
	if cfg.ExtensionsDir != "" {
		env["INGEST_EXTENSIONS_DIR"] = cfg.ExtensionsDir
	}
	out := make([]string, 0, len(env))
	for k, v := range env {
		out = append(out, k+"="+v)
	}
	return out
}

// redactEnv returns env with the value of anything that looks like a secret
// replaced, for the logs.
func redactEnv(env []string) []string {
	out := make([]string, len(env))
	for i, kv := range env {
		k, _, _ := strings.Cut(kv, "=")
		u := strings.ToUpper(k)
		if strings.Contains(u, "SECRET") || strings.Contains(u, "PASSWORD") || strings.Contains(u, "TOKEN") || strings.Contains(u, "KEY") {
			out[i] = k + "=***"
		} else {
			out[i] = kv
		}
	}
	return out
}

func ingestorJVMArgs(jar string) []string {
	return []string{"-XX:MaxRAMPercentage=25", "-jar", jar}
}

// osProc is a real child process.
type osProc struct {
	cmd  *exec.Cmd
	done chan struct{}
}

func (p *osProc) Pid() int              { return p.cmd.Process.Pid }
func (p *osProc) Done() <-chan struct{} { return p.done }
func (p *osProc) Terminate() error      { return terminateProcess(p.cmd.Process) }
func (p *osProc) Kill() error           { return p.cmd.Process.Kill() }

// startJVM starts java directly (no shell) in cfg.StateDir, its output going
// to log (a file the caller owns and closes after the child exits).
func startJVM(java, jar string, env []string, cfg ingestorConfig, log io.Writer) (*osProc, error) {
	cmd := exec.Command(java, ingestorJVMArgs(jar)...) //nolint:gosec // G204: java is fcdev's own installed JRE, jar a path the developer chose
	cmd.Dir = cfg.StateDir
	cmd.Env = env
	cmd.Stdout, cmd.Stderr = log, log
	prepareChild(cmd)
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	p := &osProc{cmd: cmd, done: make(chan struct{})}
	go func() {
		_ = cmd.Wait()
		close(p.done)
	}()
	return p, nil
}

// portFree reports whether nothing listens on the port.
func portFree(port int) bool {
	l, err := net.Listen("tcp", ":"+strconv.Itoa(port))
	if err != nil {
		return false
	}
	_ = l.Close()
	return true
}

// probeOK GETs url and reports a 200.
func probeOK(ctx context.Context, url string) bool {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return false
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false
	}
	_ = resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// waitUntil polls fn every interval until it is true, ctx ends or the timeout
// passes; it reports whether fn became true.
func waitUntil(ctx context.Context, timeout, interval time.Duration, fn func() bool) bool {
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	tick := time.NewTicker(interval)
	defer tick.Stop()
	for {
		if fn() {
			return true
		}
		select {
		case <-ctx.Done():
			return false
		case <-deadline.C:
			return false
		case <-tick.C:
		}
	}
}

// startIngestor runs the ingestor in the background and returns a function
// that waits for it to have stopped (cancel ctx first).
func startIngestor(ctx context.Context, cfg ingestorConfig) (wait func()) {
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer func() {
			if r := recover(); r != nil {
				slog.Warn("the ingestor runtime supervisor failed; continuing without it", "panic", fmt.Sprint(r))
			}
		}()
		runIngestor(ctx, cfg)
	}()
	return func() { <-done }
}

func runIngestor(ctx context.Context, cfg ingestorConfig) {
	jar, src, err := resolveIngestorJar(ctx, cfg.JarOverride, checkoutSearchDirs(), ingestorRelease, ingestorJarCacheDir(), &http.Client{Timeout: 15 * time.Minute})
	if err != nil {
		if errors.Is(err, errNoIngestorJar) {
			slog.Info(err.Error())
		} else {
			slog.Warn("ingestor runtime skipped: no jar", "err", err)
		}
		return
	}

	jre, err := newJREManager(jreRootDir()).Ensure(ctx)
	if err != nil {
		if ctx.Err() != nil {
			return
		}
		slog.Warn("ingestor runtime skipped: could not install Java. Check the network (the download is from api.adoptium.net and github.com) and restart fcdev; "+
			"or set FC_INGESTOR_ENABLED=false to stop trying", "needed_java", ingestorJavaMajor, "err", err)
		return
	}
	if err := writeIngestorConfig(cfg); err != nil {
		slog.Warn("ingestor runtime skipped: could not prepare its working directory", "dir", cfg.StateDir, "err", err)
		return
	}
	if cfg.ExtensionsDir != "" {
		jars, err := extensionJars(cfg.ExtensionsDir)
		if err != nil {
			slog.Warn("ingestor runtime skipped: " + err.Error())
			return
		}
		if len(jars) == 0 {
			slog.Info("no ingestor extensions: an extension's build drops its jar in this folder; restart fcdev after it", "dir", cfg.ExtensionsDir)
		} else {
			slog.Info("ingestor extensions", "dir", cfg.ExtensionsDir, "jars", strings.Join(jars, ", "))
		}
	}
	if !portFree(cfg.Port) {
		slog.Warn("ingestor runtime skipped: its port is already in use (another fcdev, or a runtime left from a crashed one?); change it with --ingestor-port",
			"port", cfg.Port)
		return
	}
	// The runtime polls the platform; let the platform come up first.
	if !waitUntil(ctx, ingestorPlatformWait, time.Second, func() bool {
		return probeOK(ctx, fmt.Sprintf("http://127.0.0.1:%d/health", cfg.APIPort))
	}) {
		if ctx.Err() == nil {
			slog.Warn("ingestor runtime skipped: the platform did not come up", "api_port", cfg.APIPort)
		}
		return
	}

	logPath := filepath.Join(cfg.StateDir, "ingestor.log")
	env := ingestorEnv(os.Environ(), cfg, jre)
	slog.Info("starting the ingestor runtime", "jar", jar, "jar_source", string(src), "java", jre.Version,
		"port", cfg.Port, "dir", cfg.StateDir, "log", logPath)
	slog.Debug("ingestor runtime environment", "env", redactEnv(env))

	first := true
	sup := &ingSupervisor{
		maxRestarts: 5,
		backoff:     defaultIngestorBackoff,
		stableAfter: 5 * time.Minute,
		grace:       15 * time.Second,
		log:         slog.Default(),
		exitCause:   func() string { return ingestorExitCause(logPath) },
		launch: func() (ingProc, error) {
			flags := os.O_WRONLY | os.O_CREATE | os.O_APPEND
			if first {
				flags = os.O_WRONLY | os.O_CREATE | os.O_TRUNC // a fresh log per fcdev start
			}
			lf, err := os.OpenFile(logPath, flags, 0o600)
			if err != nil {
				return nil, err
			}
			if !first {
				fmt.Fprintf(lf, "\n--- fcdev restarted the ingestor runtime at %s ---\n", time.Now().Format(time.RFC3339))
			}
			first = false
			p, err := startJVM(jre.Java, jar, env, cfg, lf)
			if err != nil {
				_ = lf.Close()
				return nil, err
			}
			pidPath := filepath.Join(cfg.StateDir, "ingestor.pid")
			_ = os.WriteFile(pidPath, []byte(strconv.Itoa(p.Pid())+"\n"), 0o600)
			go func() {
				<-p.Done()
				_ = lf.Close()
				_ = os.Remove(pidPath)
			}()
			return p, nil
		},
		onStarted: func(ctx context.Context, p ingProc) {
			ready := waitUntil(ctx, ingestorReadyWait, time.Second, func() bool {
				select {
				case <-p.Done():
					return true // exited; the supervisor reports it
				default:
				}
				return probeOK(ctx, fmt.Sprintf("http://127.0.0.1:%d/health/ready", cfg.Port))
			})
			select {
			case <-p.Done():
				return
			default:
			}
			switch {
			case ready:
				slog.Info(fmt.Sprintf("ingestor runtime ready on :%d", cfg.Port))
			case ctx.Err() == nil:
				slog.Warn("ingestor runtime not ready yet; see its log", "port", cfg.Port, "log", logPath)
			}
		},
	}
	_ = sup.run(ctx)
}

// ── fcdev ingestor ──────────────────────────────────────────────────────

func newIngestorCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "ingestor",
		Short: "Inspect the ingestor runtime fcdev runs for local development",
	}
	status := &cobra.Command{
		Use:   "status",
		Short: "Show the JRE, jar, process and health of this machine's dev ingestor runtime",
		Args:  cobra.NoArgs,
		RunE:  runIngestorStatus,
	}
	status.Flags().Int("api-port", envIntDefault("FC_API_PORT", 8080), "API port of the fcdev whose ingestor to inspect")
	status.Flags().Int("ingestor-port", envIntDefault("FC_INGESTOR_PORT", defaultIngestorPort), "ingestor runtime port")
	cmd.AddCommand(status)
	return cmd
}

func runIngestorStatus(cmd *cobra.Command, _ []string) error {
	out := cmd.OutOrStdout()
	apiPort, _ := cmd.Flags().GetInt("api-port")
	port, _ := cmd.Flags().GetInt("ingestor-port")

	jm := newJREManager(jreRootDir())
	if info, ok := jm.readInstall(ingestorJavaMajor); ok {
		fmt.Fprintf(out, "java:    %s (%s)\n", info.Java, info.Version)
	} else {
		fmt.Fprintf(out, "java:    Java %d not installed yet (fcdev installs it on first start; cache %s)\n", ingestorJavaMajor, jm.root)
	}
	if others := jm.installedMajors(); len(others) > 0 {
		fmt.Fprintf(out, "         installed majors: %v\n", others)
	}
	jar, src, err := resolveIngestorJar(cmd.Context(), os.Getenv("FC_INGESTOR_JAR"), checkoutSearchDirs(),
		ingestorReleaseSpec{}, ingestorJarCacheDir(), nil)
	switch {
	case err == nil:
		fmt.Fprintf(out, "jar:     %s (%s)\n", jar, src)
	case errors.Is(err, errNoIngestorJar):
		fmt.Fprintf(out, "jar:     none (%s)\n", err)
	default:
		fmt.Fprintf(out, "jar:     %v\n", err)
	}
	state := ingestorStateDir(apiPort)
	fmt.Fprintf(out, "dir:     %s (log %s)\n", state, filepath.Join(state, "ingestor.log"))
	if pid, err := readPIDFile(filepath.Join(state, "ingestor.pid")); err == nil && processAlive(pid) {
		fmt.Fprintf(out, "process: pid %d\n", pid)
	} else {
		fmt.Fprintln(out, "process: not running")
	}
	health := "not answering"
	if probeOK(cmd.Context(), fmt.Sprintf("http://127.0.0.1:%d/health/ready", port)) {
		health = "ready"
	}
	fmt.Fprintf(out, "health:  :%d %s\n", port, health)
	return nil
}
