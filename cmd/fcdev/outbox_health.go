package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/spf13/cobra"

	"github.com/flowcatalyst/flowcatalyst-go/internal/outbox"
	outboxpg "github.com/flowcatalyst/flowcatalyst-go/internal/outbox/postgres"
)

// The health check of `fcdev outbox` is a loopback HTTP endpoint the poller serves
// (FC_OUTBOX_HEALTH_ADDR) and `fcdev outbox health` asks: a container health check can run the
// binary alone (no curl, no shell, a read-only root file system), and a frozen process does not
// answer at all. The verdict is computed from what the poll loop and the election last did, not
// from the server answering, so a hung poll loop is reported though the process still serves.

const defaultOutboxHealthAddr = "127.0.0.1:9180"

// outboxPhase is where the command is before its processor runs.
type outboxPhase string

const (
	phaseStarting     outboxPhase = "starting"
	phaseWaitingTable outboxPhase = "waiting_for_table"
	phaseRunning      outboxPhase = "running"
)

// tokenState is the latest platform-token probe.
type tokenState struct {
	checked      bool
	ok           bool
	err          string
	failingSince time.Time
}

// outboxHealth is what the running poller knows about itself, for /health.
type outboxHealth struct {
	stall        time.Duration
	requireToken bool

	mu          sync.Mutex
	phase       outboxPhase
	phaseSince  time.Time
	lastContact time.Time // the table check while waiting for the table
	outboxName  string
	leadership  *outboxpg.Leadership // nil: leadership off
	proc        *outbox.Processor
	token       tokenState
}

func newOutboxHealth(stall time.Duration, requireToken bool) *outboxHealth {
	return &outboxHealth{stall: stall, requireToken: requireToken, phase: phaseStarting, phaseSince: time.Now()}
}

func (h *outboxHealth) setPhase(p outboxPhase) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.phase, h.phaseSince = p, time.Now()
}

func (h *outboxHealth) contacted() {
	h.mu.Lock()
	h.lastContact = time.Now()
	h.mu.Unlock()
}

func (h *outboxHealth) running(name string, l *outboxpg.Leadership, p *outbox.Processor) {
	h.mu.Lock()
	h.outboxName, h.leadership, h.proc = name, l, p
	h.mu.Unlock()
	h.setPhase(phaseRunning)
}

func (h *outboxHealth) noteToken(err error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.token.checked = true
	if err == nil {
		h.token.ok, h.token.err, h.token.failingSince = true, "", time.Time{}
		return
	}
	if h.token.ok || h.token.failingSince.IsZero() {
		h.token.failingSince = time.Now()
	}
	h.token.ok, h.token.err = false, err.Error()
}

// healthReport is the /health body.
type healthReport struct {
	Healthy       bool       `json:"healthy"`
	Status        string     `json:"status"`
	Reason        string     `json:"reason,omitempty"`
	Outbox        string     `json:"outbox,omitempty"`
	Role          string     `json:"role"`
	LastIteration *time.Time `json:"lastIteration,omitempty"`
	LastClaimOK   *time.Time `json:"lastClaimOK,omitempty"`
	ClaimError    string     `json:"claimError,omitempty"`
	LastDBContact *time.Time `json:"lastDBContact,omitempty"`
	Token         string     `json:"token,omitempty"`
}

func tp(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	u := t.UTC()
	return &u
}

// report is the verdict now.
func (h *outboxHealth) report() healthReport {
	h.mu.Lock()
	in := healthInput{
		now: time.Now(), stall: h.stall, requireToken: h.requireToken, phase: h.phase, phaseSince: h.phaseSince,
		lastContact: h.lastContact, outbox: h.outboxName, token: h.token,
	}
	l, p := h.leadership, h.proc
	h.mu.Unlock()
	if l != nil {
		st := l.Status()
		in.leader = &st
	}
	if p != nil {
		in.act = p.Activity()
	}
	return verdict(in)
}

// healthInput is what a verdict is made from.
type healthInput struct {
	now          time.Time
	stall        time.Duration
	requireToken bool
	phase        outboxPhase
	phaseSince   time.Time
	lastContact  time.Time // waiting for the table
	outbox       string
	leader       *outboxpg.LeaderStatus // nil: leadership off
	act          outbox.Activity
	token        tokenState
}

func verdict(in healthInput) healthReport {
	stale := func(t time.Time) bool { return in.now.Sub(later(t, in.phaseSince)) > in.stall }
	r := healthReport{Outbox: in.outbox}
	switch in.phase {
	case phaseStarting:
		r.Role, r.Status, r.Reason = "starting", "starting", "connecting to the source database"
		return r
	case phaseWaitingTable:
		r.Role, r.LastDBContact = "waiting_for_table", tp(in.lastContact)
		if stale(in.lastContact) {
			r.Status, r.Reason = "unhealthy", "cannot check the source database for outbox_messages"
			return r
		}
		r.Healthy, r.Status = true, "waiting_for_table"
		return r
	case phaseRunning:
	}
	r.Role = "leader"
	if st := in.leader; st != nil {
		r.Role, r.LastDBContact = string(st.Role), tp(st.LastContact)
		if st.Role != outboxpg.RoleLeader {
			// standing by, taking over or reconnecting: alive if the database answers
			if stale(st.LastContact) {
				r.Status = "unhealthy"
				r.Reason = "cannot reach the source database"
				if st.LastError != "" {
					r.Reason += ": " + st.LastError
				}
				return r
			}
			r.Healthy, r.Status = true, r.Role
			return r
		}
	}
	act := in.act
	r.LastIteration, r.LastClaimOK, r.ClaimError = tp(act.LastIteration), tp(act.LastClaimOK), act.ClaimError
	switch {
	case stale(act.LastIteration):
		r.Status, r.Reason = "unhealthy", "the poll loop has not completed a cycle for "+
			in.now.Sub(later(act.LastIteration, in.phaseSince)).Round(time.Second).String()
		return r
	case stale(act.LastClaimOK) && (!act.ClaimTimedOut || !act.LastClaimError.After(act.LastClaimOK)):
		// claims that time out (a locked table, a slow database) do not fail the check: a restart
		// cannot help, and would stop the task whose migration may hold the lock
		r.Status, r.Reason = "unhealthy", "no successful claim: "+act.ClaimError
		return r
	}
	if in.token.checked {
		r.Token = "ok"
		if !in.token.ok {
			r.Token = "failing: " + in.token.err
			if in.requireToken && in.now.Sub(in.token.failingSince) > in.stall {
				r.Status, r.Reason = "unhealthy", "cannot obtain a platform token: "+in.token.err
				return r
			}
		}
	}
	r.Healthy, r.Status = true, "polling"
	return r
}

func later(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}
	return b
}

// ServeHTTP answers GET /health: 200 healthy, 503 not, with the report.
func (h *outboxHealth) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/health" {
		http.NotFound(w, r)
		return
	}
	rep := h.report()
	w.Header().Set("Content-Type", "application/json")
	if !rep.Healthy {
		w.WriteHeader(http.StatusServiceUnavailable)
	}
	_ = json.NewEncoder(w).Encode(rep)
}

// outboxTestHooks, in the command's context (tests only), see inside a running poller.
type outboxTestHooks struct {
	healthListening func(addr string)
	wrapRepo        func(outbox.Repository) outbox.Repository
}

type outboxTestHooksKey struct{}

func testHooks(ctx context.Context) outboxTestHooks {
	h, _ := ctx.Value(outboxTestHooksKey{}).(outboxTestHooks)
	return h
}

// serveOutboxHealth listens on addr at once (so a busy port is a start-up error) and serves h
// until ctx ends.
func serveOutboxHealth(ctx context.Context, addr string, h *outboxHealth) error {
	ln, err := (&net.ListenConfig{}).Listen(ctx, "tcp", addr)
	if err != nil {
		return fmt.Errorf("health check address %s (FC_OUTBOX_HEALTH_ADDR, --health-addr; off disables it): %w", addr, err)
	}
	if f := testHooks(ctx).healthListening; f != nil {
		f(ln.Addr().String())
	}
	srv := &http.Server{Handler: h, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("outbox health server exited", "err", err)
		}
	}()
	go func() { //nolint:gosec // G118: shutdown drain: the parent ctx is already done
		<-ctx.Done()
		c, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Shutdown(c)
	}()
	return nil
}

// tokenProbeEvery paces the leader's platform-token probe; tests shorten it.
var tokenProbeEvery = 30 * time.Second

// probeToken obtains the platform token every tokenProbeEvery while this poller dispatches, and
// records the result (the source caches the token, so this mints only when it is due).
func probeToken(ctx context.Context, h *outboxHealth, src outbox.TokenSource, dispatching func() bool) {
	t := time.NewTicker(tokenProbeEvery)
	defer t.Stop()
	for {
		if dispatching() {
			c, cancel := context.WithTimeout(ctx, 10*time.Second)
			_, err := src.Token(c)
			cancel()
			if ctx.Err() != nil {
				return
			}
			h.noteToken(err)
		}
		select {
		case <-ctx.Done(): // shutdown
			return
		case <-t.C: // next probe
		}
	}
}

func newOutboxHealthCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "health",
		Short: "Ask the running outbox poller in this container whether it is healthy (exit 0) or not (exit 1)",
		Long: `Ask the poller running in this container (fcdev outbox, on FC_OUTBOX_HEALTH_ADDR) for its health
and print its report. Exit status 0: healthy; 1: unhealthy, or the poller does not answer. For a
container health check: ["CMD", "/fcdev", "outbox", "health"].

Healthy is: the leader is polling (its poll loop completed a cycle and a claim succeeded within
FC_OUTBOX_STALL_TIMEOUT_MS, and, unless FC_OUTBOX_HEALTH_REQUIRE_TOKEN=false, the platform token
could be obtained within that time); a standby (or a poller waiting for outbox_messages) reached
the database within that time.`,
		Args:         cobra.NoArgs,
		SilenceUsage: true, // a failed check prints the report, not the usage
		RunE: func(c *cobra.Command, _ []string) error {
			addr := resolveEnvFlag(c, "health-addr", "FC_OUTBOX_HEALTH_ADDR")
			if strings.EqualFold(addr, "off") || addr == "" {
				return errors.New("the health check is off (FC_OUTBOX_HEALTH_ADDR=off)")
			}
			timeout, _ := c.Flags().GetDuration("timeout")
			ctx, cancel := context.WithTimeout(c.Context(), timeout)
			defer cancel()
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+addr+"/health", nil)
			if err != nil {
				return err
			}
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				return fmt.Errorf("the outbox poller does not answer on %s: %w", addr, err)
			}
			defer resp.Body.Close()
			body, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
			_, _ = c.OutOrStdout().Write(body)
			if resp.StatusCode != http.StatusOK {
				return fmt.Errorf("outbox poller unhealthy (%s)", resp.Status)
			}
			return nil
		},
	}
	cmd.Flags().String("health-addr", envStrDefault("FC_OUTBOX_HEALTH_ADDR", defaultOutboxHealthAddr), "the poller's health address (env FC_OUTBOX_HEALTH_ADDR)")
	cmd.Flags().Duration("timeout", 3*time.Second, "how long to wait for the answer")
	return cmd
}

var (
	dsnPassword  = regexp.MustCompile(`(?i)(password\s*=\s*)('(?:[^'\\]|\\.)*'|\S+)`)
	urlUserinfo  = regexp.MustCompile(`(://[^:/@]*:)[^/]*@`)
	urlPasswordQ = regexp.MustCompile(`(?i)([?&]password=)[^&]*`)
)

// redactDBURL returns a database address for a log line: a URL's password (in its userinfo or a
// password query parameter) and a keyword string's password=... replaced by xxxxx.
func redactDBURL(s string) string {
	if u, err := url.Parse(s); err == nil && u.Scheme != "" && u.Host != "" {
		if q := u.Query(); q.Has("password") {
			q.Set("password", "xxxxx")
			u.RawQuery = q.Encode()
		}
		return u.Redacted()
	}
	// not a URL Go can parse (or a keyword string): replace what looks like a password
	s = urlUserinfo.ReplaceAllString(s, "${1}xxxxx@")
	s = urlPasswordQ.ReplaceAllString(s, "${1}xxxxx")
	return dsnPassword.ReplaceAllString(s, "${1}xxxxx")
}
