package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"

	"github.com/flowcatalyst/flowcatalyst-go/internal/functions/abi"
)

// devStyle colours output, only when asked to (a terminal).
type devStyle struct{ color bool }

func (s devStyle) wrap(code, t string) string {
	if !s.color {
		return t
	}
	return "\x1b[" + code + "m" + t + "\x1b[0m"
}
func (s devStyle) green(t string) string  { return s.wrap("32", t) }
func (s devStyle) red(t string) string    { return s.wrap("31", t) }
func (s devStyle) yellow(t string) string { return s.wrap("33", t) }
func (s devStyle) dim(t string) string    { return s.wrap("2", t) }

// stdoutStyle colours when stdout is a terminal and NO_COLOR is unset.
func stdoutStyle() devStyle {
	fd := os.Stdout.Fd()
	return devStyle{color: os.Getenv("NO_COLOR") == "" && (isatty.IsTerminal(fd) || isatty.IsCygwinTerminal(fd))}
}

// devUI is the loop's serialised, secret-masking output.
type devUI struct {
	mu      sync.Mutex
	w       io.Writer
	st      devStyle
	secrets []string
}

// maskSecrets hides --secret values of 4+ characters (shorter ones would
// garble ordinary text).
func maskSecrets(s string, secrets []string) string {
	for _, v := range secrets {
		if len(v) >= 4 {
			s = strings.ReplaceAll(s, v, "***")
		}
	}
	return s
}

func (u *devUI) printf(format string, args ...any) {
	s := maskSecrets(fmt.Sprintf(format, args...), u.secrets)
	u.mu.Lock()
	defer u.mu.Unlock()
	_, _ = io.WriteString(u.w, s)
}

func (u *devUI) line(s string) { u.printf("%s\n", s) }

// devOptions are `fn dev`'s settings.
type devOptions struct {
	dir                          string
	address, runtime, whSecret   string
	port, memoryMB               int
	configs, secrets, dbs, perms []string
	noWatch                      bool
	send                         *sendSpec

	pollEvery, debounce time.Duration
}

// devDeps are the loop's seams; tests replace them.
type devDeps struct {
	build func(ctx context.Context, dir string) (string, error)
	out   io.Writer
	in    io.Reader // commands (r / q) when interactive
	tty   bool      // read commands from in
	style devStyle
	ready func(base string) // called once the server listens
}

var fnDevNameClean = regexp.MustCompile(`[^a-z0-9-]+`)

// defaultDevAddress is local.<dirname>: describe carries no address.
func defaultDevAddress(dir string) string {
	abs, err := filepath.Abs(dir)
	if err != nil {
		abs = dir
	}
	n := strings.Trim(fnDevNameClean.ReplaceAllString(strings.ToLower(filepath.Base(abs)), "-"), "-")
	if n == "" {
		n = "fn"
	}
	return "local." + n
}

func newFnDevCmd() *cobra.Command {
	var o devOptions
	var (
		send, method, path, contentType string
		headers                         []string
	)
	cmd := &cobra.Command{
		Use:   "dev [dir]",
		Short: "Local development loop: build, serve on the real runner, rebuild and hot-reload on change",
		Long: `Builds the function project in dir (default .) the way 'fn build' does, serves
it on the real function runner at 127.0.0.1 without a platform, then watches
the source tree: every change rebuilds and hot-reloads. A failed rebuild keeps
the previous good build serving.

With --send FILE the file is sent to the function after every successful
(re)load and the response, emitted events and would-be dispatch jobs are
printed. On a terminal, 'r'+Enter resends it and 'q'+Enter quits.

Dispatches are checked against the version's declared 'dispatches' but no job
is created, and pools and connections are not resolved: that needs a platform.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			o.dir = "."
			if len(args) == 1 {
				o.dir = args[0]
			}
			if o.address == "" {
				o.address = defaultDevAddress(o.dir)
			}
			if send != "" {
				o.send = &sendSpec{file: send, method: method, path: path, contentType: contentType, headers: headers}
			}
			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			return runDev(ctx, o, devDeps{
				build: buildProject, out: os.Stdout, in: os.Stdin,
				tty: fnStdinIsTerminal(), style: stdoutStyle(),
			})
		},
	}
	f := cmd.Flags()
	f.IntVar(&o.port, "port", 8099, "listen port (127.0.0.1)")
	f.StringVar(&o.address, "address", "", "the address to serve the function at (default local.<directory name>)")
	f.StringVar(&o.runtime, "runtime", "", "wasm or js (default: from the artifact's extension)")
	f.StringArrayVar(&o.configs, "config", nil, "KEY=VALUE config setting (repeatable)")
	f.StringArrayVar(&o.secrets, "secret", nil, "KEY=VALUE secret (repeatable; values are never printed)")
	f.StringArrayVar(&o.dbs, "db", nil, "NAME=DSN database binding (repeatable)")
	f.StringArrayVar(&o.perms, "perms", nil, "permission the dev principal holds (repeatable)")
	f.StringVar(&o.whSecret, "webhook-secret", "dev-webhook-secret", "the signing secret webhook endpoints verify")
	f.IntVar(&o.memoryMB, "memory", 256, "memory budget for the function, in MB")
	f.BoolVar(&o.noWatch, "no-watch", false, "build and serve once; do not watch for changes")
	f.StringVar(&send, "send", "", "send this file to the function after every successful (re)load")
	f.StringVar(&method, "method", "POST", "HTTP method for --send")
	f.StringVar(&path, "path", "/", "function path for --send")
	f.StringArrayVar(&headers, "header", nil, "extra header for --send, 'Key: Value' (repeatable)")
	f.StringVar(&contentType, "content-type", "", "Content-Type for --send (default by file extension)")
	return cmd
}

// devSession is one running `fn dev`.
type devSession struct {
	o    devOptions
	d    devDeps
	ui   *devUI
	rec  *devRecorder
	cp   *devCP
	srv  *localServer
	base string

	runDone        chan struct{}
	runErr         error
	lastVer        int
	shownEndpoints bool
}

func runDev(ctx context.Context, o devOptions, d devDeps) error {
	if _, err := detectLang(o.dir); err != nil {
		return err
	}
	fn, err := newLocalFunction(o.address, o.whSecret, o.configs, o.secrets, o.dbs)
	if err != nil {
		return err
	}
	var body []byte
	if o.send != nil {
		if body, err = os.ReadFile(o.send.file); err != nil {
			return fmt.Errorf("--send: %w", err)
		}
		if _, err := parseHeaders(o.send.headers); err != nil {
			return err
		}
	}
	if o.pollEvery <= 0 {
		o.pollEvery = 300 * time.Millisecond
	}
	if o.debounce <= 0 {
		o.debounce = 150 * time.Millisecond
	}
	secretVals := make([]string, 0, len(fn.Secrets)+1)
	for _, v := range fn.Secrets {
		secretVals = append(secretVals, v)
	}
	if o.whSecret != "dev-webhook-secret" {
		secretVals = append(secretVals, o.whSecret)
	}
	ui := &devUI{w: d.out, st: d.style, secrets: secretVals}
	s := &devSession{o: o, d: d, ui: ui}
	s.rec = &devRecorder{print: func(l string) { ui.printf("  %s\n", l) }}
	s.cp = newDevCP(&localCP{fn: fn, changed: make(chan struct{}, 1)}, s.rec)
	return s.loop(ctx, body)
}

func (s *devSession) loop(ctx context.Context, body []byte) error {
	defer s.shutdown()
	changes := make(chan []string, 1)
	cmds := make(chan string, 4)

	s.ui.printf("fcdev fn dev: %s  address %s\n", s.o.dir, s.o.address)
	s.rebuild(ctx, body)
	if s.srv == nil && s.o.noWatch {
		return errors.New("the first build failed")
	}
	if !s.o.noWatch {
		w := newPollWatcher(s.o.dir, s.o.pollEvery, s.o.debounce)
		go w.run(ctx, func(p []string) {
			select {
			case changes <- p:
			default: // a rebuild is already queued; it will see these too
			}
		})
		s.ui.line(s.d.style.dim("watching for changes (Ctrl-C to stop)"))
	}
	if s.d.tty {
		go readDevCommands(s.d.in, cmds)
		s.ui.line(s.d.style.dim("r+Enter resends the last request, q+Enter quits"))
	}
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-s.runDone:
			return s.runErr
		case p := <-changes:
			if s.o.send != nil && len(p) == 1 && s.sameFile(p[0], s.o.send.file) {
				// The sample request itself changed: resend, no rebuild.
				if b, err := os.ReadFile(s.o.send.file); err == nil {
					body = b
					s.ui.line(s.d.style.dim("request file changed"))
					s.sendOnce(ctx, body)
				}
				continue
			}
			s.ui.line(s.d.style.dim("change: " + summarizePaths(p)))
			s.rebuild(ctx, body)
		case c, ok := <-cmds:
			if !ok {
				cmds = nil // stdin closed
				continue
			}
			switch c {
			case "q", "quit":
				return nil
			case "r", "resend":
				if s.o.send == nil {
					s.ui.line("nothing to resend: start with --send FILE")
					continue
				}
				if b, err := os.ReadFile(s.o.send.file); err == nil {
					body = b
				}
				s.sendOnce(ctx, body)
			}
		}
	}
}

func (s *devSession) sameFile(rel, send string) bool {
	a, err1 := filepath.Abs(filepath.Join(s.o.dir, rel))
	b, err2 := filepath.Abs(send)
	return err1 == nil && err2 == nil && a == b
}

func summarizePaths(p []string) string {
	if len(p) <= 3 {
		return strings.Join(p, ", ")
	}
	return fmt.Sprintf("%s and %d more", strings.Join(p[:3], ", "), len(p)-3)
}

func readDevCommands(in io.Reader, out chan<- string) {
	sc := bufio.NewScanner(in)
	for sc.Scan() {
		out <- strings.ToLower(strings.TrimSpace(sc.Text()))
	}
	close(out)
}

// rebuild builds, loads and, when it all worked, replays the request. Any
// failure is printed and the previous version keeps serving.
func (s *devSession) rebuild(ctx context.Context, body []byte) {
	st := s.d.style
	start := time.Now()
	artifact, err := s.d.build(ctx, s.o.dir)
	if err != nil {
		if ctx.Err() != nil {
			return
		}
		s.failed("build failed: " + err.Error())
		return
	}
	runtime := s.o.runtime
	if runtime == "" {
		runtime = runtimeFor(artifact)
	}
	li, err := s.cp.apply(ctx, artifact, runtime)
	if err != nil {
		s.failed("not loaded: " + err.Error())
		return
	}
	if s.srv == nil {
		if err := s.startServer(ctx); err != nil {
			s.failed("cannot start the runner: " + err.Error())
			return
		}
	}
	if err := s.cp.waitLoaded(ctx, li.Version, 30*time.Second); err != nil {
		if ctx.Err() != nil {
			return
		}
		s.failed("not loaded: " + err.Error())
		return
	}
	s.lastVer = li.Version
	d := li.Describe
	s.ui.printf("%s v%d  %s in %s  size=%s digest=%s  routes=%d subscriptions=%d schedules=%d dispatches=%d\n",
		st.green("ok"), li.Version, runtime, time.Since(start).Round(time.Millisecond), humanSize(li.Size), li.Digest[:12],
		len(d.Endpoints), len(d.Subscriptions), len(d.Schedules), len(d.Dispatches))
	if !s.shownEndpoints {
		s.shownEndpoints = true
		s.printEndpoints(d)
	}
	if s.o.send != nil {
		s.sendOnce(ctx, body)
	}
}

func (s *devSession) failed(msg string) {
	st := s.d.style
	s.ui.printf("%s %s\n", st.red("error"), strings.TrimSpace(msg))
	if s.lastVer > 0 {
		s.ui.line(st.yellow(fmt.Sprintf("  still serving v%d (the last good build)", s.lastVer)))
	} else {
		s.ui.line(st.yellow("  nothing is serving yet: fix the error and save"))
	}
}

func humanSize(n int) string {
	if n >= 1<<20 {
		return fmt.Sprintf("%.1fMB", float64(n)/(1<<20))
	}
	return fmt.Sprintf("%dKB", (n+1023)/1024)
}

func (s *devSession) startServer(ctx context.Context) error {
	srv, err := startLocalServer(ctx, s.cp, s.o.perms, s.o.memoryMB, s.o.port)
	if err != nil {
		return err
	}
	s.srv = srv
	s.base = "http://" + srv.ln.Addr().String()
	s.runDone = make(chan struct{})
	go func() {
		s.runErr = srv.r.Run(ctx)
		close(s.runDone)
	}()
	if s.d.ready != nil {
		s.d.ready(s.base)
	}
	return nil
}

func (s *devSession) shutdown() {
	if s.srv == nil {
		return
	}
	s.srv.Close()
	// The runner stops with the context; give it a moment to drain.
	select {
	case <-s.runDone:
	case <-time.After(5 * time.Second):
	}
}

// printEndpoints lists how to call the function: curl lines per endpoint.
func (s *devSession) printEndpoints(d *abi.Describe) {
	st := s.d.style
	if s.o.whSecret != "dev-webhook-secret" {
		s.ui.line("webhook endpoints verify signatures made with the --webhook-secret you gave; platform endpoints accept any bearer token")
	} else {
		s.ui.line("webhook endpoints verify signatures made with the default dev secret; platform endpoints accept any bearer token")
	}
	if s.o.send != nil {
		return
	}
	for i, e := range d.Endpoints {
		if i == 10 {
			s.ui.line(st.dim(fmt.Sprintf("  … and %d more endpoints", len(d.Endpoints)-i)))
			break
		}
		method := e.Method
		if method == "" {
			method = "POST"
		}
		line := fmt.Sprintf("  curl -X %s http://%s/fn/%s%s", method, s.srv.ln.Addr(), s.o.address, e.Path)
		switch e.Auth {
		case abi.AuthPlatform:
			line += " -H 'Authorization: Bearer dev'"
		case abi.AuthWebhook:
			line += "   " + st.dim("(signed: use --send to have it signed)")
		}
		s.ui.line(line)
	}
}

// sendOnce replays the request and prints the outcome grouped with the events
// and dispatches the call produced.
func (s *devSession) sendOnce(ctx context.Context, body []byte) {
	spec := s.o.send
	req, err := spec.buildRequest(s.base, s.o.address, s.cp.current(), s.o.whSecret, body)
	if err != nil {
		s.failed("request: " + err.Error())
		return
	}
	req = req.WithContext(ctx)
	ct := req.Header.Get("Content-Type")
	s.ui.printf("%s %s %s  (%s, %d bytes from %s)\n", s.d.style.dim("->"), req.Method, req.URL.Path, ct, len(body), spec.file)
	s.rec.begin()
	start := time.Now()
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		s.rec.end()
		s.failed("request failed: " + err.Error())
		return
	}
	defer func() { _ = resp.Body.Close() }()
	rb, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	took := time.Since(start)
	produced := s.rec.end()
	var buf strings.Builder
	formatResponse(&buf, resp, rb, took, produced, s.d.style)
	s.ui.printf("%s", buf.String())
}
