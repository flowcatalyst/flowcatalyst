//go:build integration

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"slices"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/outbox"
	outboxpg "github.com/flowcatalyst/flowcatalyst-go/internal/outbox/postgres"
)

// The pollers here run the command itself (`fcdev outbox`), in this process or, to be stopped
// and killed, as a child process, with a lease of 500ms: a standby takes over 0.7s after the
// leader's connection closes and 1.7s after the leader goes silent.

// insertRows writes rows of group as the framework writes them, payload ids "<prefix><n>".
func insertRows(t *testing.T, dbURL, group, prefix string, from, to int) {
	t.Helper()
	conn, err := pgx.Connect(t.Context(), dbURL)
	require.NoError(t, err)
	defer func() { _ = conn.Close(context.Background()) }()
	for i := from; i <= to; i++ {
		_, err := conn.Exec(t.Context(), `INSERT INTO outbox_messages (id, type, message_group, payload, created_at)
			VALUES ($1, 'EVENT', $2, $3, now() + make_interval(secs => $4 / 1000.0))`,
			fmt.Sprintf("%s%0*d", prefix, 26-len(prefix), i), group,
			fmt.Sprintf(`{"id":"%s%d","type":"shop:orders:order:created","source":"shop","data":{}}`, prefix, i), i)
		require.NoError(t, err)
	}
}

func pending(t *testing.T, dbURL string) int {
	t.Helper()
	conn, err := pgx.Connect(t.Context(), dbURL)
	require.NoError(t, err)
	defer func() { _ = conn.Close(context.Background()) }()
	var n int
	require.NoError(t, conn.QueryRow(t.Context(), `SELECT count(*) FROM outbox_messages`).Scan(&n))
	return n
}

// checkHealth runs `fcdev outbox health` against addr: its report and whether it exited 0.
func checkHealth(t *testing.T, addr string) (healthReport, bool) {
	t.Helper()
	cmd := newOutboxCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{"health", "--env-file", "", "--health-addr", addr, "--timeout", "2s"})
	err := cmd.ExecuteContext(context.Background())
	var r healthReport
	_ = json.NewDecoder(&out).Decode(&r)
	return r, err == nil
}

func eventuallyHealth(t *testing.T, addr string, d time.Duration, what string, ok func(healthReport, bool) bool) healthReport {
	t.Helper()
	var r healthReport
	eventually(t, d, what, func() bool {
		var healthy bool
		r, healthy = checkHealth(t, addr)
		return ok(r, healthy)
	})
	return r
}

// Two pollers of one outbox: one sends everything, the other stands by; both are healthy; each
// logs its role once.
func TestOutboxTwoPollersOneSends(t *testing.T) {
	logs := captureLog(t)
	dbURL := emptyDB(t)
	platA, platB := newStubPlatform(t), newStubPlatform(t)
	a := startPoller(t, dbURL, platA.srv.URL)
	eventuallyHealth(t, a.healthAddr(t), 10*time.Second, "a leads", func(r healthReport, ok bool) bool {
		return ok && r.Role == "leader"
	})
	b := startPoller(t, dbURL+"&application_name=poller-b", platB.srv.URL)
	r := eventuallyHealth(t, b.healthAddr(t), 10*time.Second, "b stands by", func(r healthReport, ok bool) bool {
		return ok && r.Role == "standby"
	})
	assert.NotEmpty(t, r.Outbox)
	// the standby keeps its lock connection only: none of its pool's (application_name poller-b)
	sessions := func(app string) int {
		conn, err := pgx.Connect(t.Context(), dbURL)
		require.NoError(t, err)
		defer func() { _ = conn.Close(context.Background()) }()
		var n int
		require.NoError(t, conn.QueryRow(t.Context(),
			`SELECT count(*) FROM pg_stat_activity WHERE application_name = $1 AND datname = current_database()`, app).Scan(&n))
		return n
	}
	eventually(t, 5*time.Second, "b's pool closed", func() bool { return sessions("poller-b") == 0 })
	assert.Equal(t, 2, sessions("fc-outbox-leader"), "one lock connection per poller")

	insertRows(t, dbURL, "orders:1", "TWO", 1, 20)
	eventually(t, 10*time.Second, "every row delivered", func() bool { return pending(t, dbURL) == 0 })
	assert.Len(t, platA.delivered(), 20)
	assert.Empty(t, platB.delivered())
	assert.Equal(t, 1, strings.Count(logs.String(), "outbox standing by"), "standby logged once")
	assert.Equal(t, 1, strings.Count(logs.String(), "outbox leader: dispatching"))
	require.NoError(t, b.stop(t, 5*time.Second))
	require.NoError(t, a.stop(t, 5*time.Second))
}

// The start-up line shows where the poller reads, never the password.
func TestOutboxStartupLogHidesThePassword(t *testing.T) {
	logs := captureLog(t)
	dbURL := emptyDB(t)
	u, err := url.Parse(dbURL)
	require.NoError(t, err)
	pw, _ := u.User.Password()
	p := startPoller(t, dbURL, newStubPlatform(t).srv.URL)
	line := waitFor(t, logs, "fcdev outbox started")
	_, line, _ = strings.Cut(line, "fcdev outbox started")
	line, _, _ = strings.Cut(line, "\n")
	assert.Contains(t, line, `source="postgres://`+u.User.Username()+":xxxxx@")
	assert.NotContains(t, line, ":"+pw+"@")
	assert.Contains(t, line, "outbox="+strings.TrimPrefix(u.Path, "/")+".public.outbox_messages")
	require.NoError(t, p.stop(t, 5*time.Second))
}

// --leader=false: no election and no lock connection; the poller delivers as before.
func TestOutboxLeadershipOff(t *testing.T) {
	logs := captureLog(t)
	dbURL := emptyDB(t)
	plat := newStubPlatform(t)
	p := startPoller(t, dbURL, plat.srv.URL, "--leader=false")
	waitFor(t, logs, "outbox leadership is off")
	insertRows(t, dbURL, "orders:1", "OFF", 1, 5)
	eventually(t, 10*time.Second, "every row delivered", func() bool { return pending(t, dbURL) == 0 })
	assert.Len(t, plat.delivered(), 5)
	conn, err := pgx.Connect(t.Context(), dbURL)
	require.NoError(t, err)
	defer func() { _ = conn.Close(context.Background()) }()
	var lockSessions int
	require.NoError(t, conn.QueryRow(t.Context(),
		`SELECT count(*) FROM pg_stat_activity WHERE application_name = 'fc-outbox-leader' AND datname = current_database()`).Scan(&lockSessions))
	assert.Zero(t, lockSessions)
	r, ok := checkHealth(t, p.healthAddr(t))
	assert.True(t, ok)
	assert.Equal(t, "leader", r.Role)
	assert.NotContains(t, logs.String(), "outbox standing by")
	require.NoError(t, p.stop(t, 5*time.Second))
}

// hangingRepo blocks ClaimPending, ignoring its context, once hang is called: a hung poll loop.
type hangingRepo struct {
	outbox.Repository
	mu   sync.Mutex
	gate chan struct{}
}

func (r *hangingRepo) hang() {
	r.mu.Lock()
	r.gate = make(chan struct{})
	r.mu.Unlock()
}

func (r *hangingRepo) release() {
	r.mu.Lock()
	if r.gate != nil {
		close(r.gate)
		r.gate = nil
	}
	r.mu.Unlock()
}

func (r *hangingRepo) ClaimPending(ctx context.Context, n int, f outbox.ClaimFilter) ([]outbox.Item, error) {
	r.mu.Lock()
	g := r.gate
	r.mu.Unlock()
	if g != nil {
		<-g
	}
	return r.Repository.ClaimPending(ctx, n, f)
}

// A hung leader (it holds the lock and never polls): with leadership off the health check turns
// unhealthy; with leadership on the poller also gives the leadership up and exits with an error,
// so that an essential container brings its task down.
func TestOutboxHungLeader(t *testing.T) {
	for _, leader := range []bool{false, true} {
		t.Run(fmt.Sprintf("leader=%v", leader), func(t *testing.T) {
			_ = captureLog(t)
			dbURL := emptyDB(t)
			hung := &hangingRepo{}
			hooks := outboxTestHooks{wrapRepo: func(r outbox.Repository) outbox.Repository { hung.Repository = r; return hung }}
			p := startPollerHooked(t, outboxpg.WaitOptions{}, hooks, dbURL, newStubPlatform(t).srv.URL,
				fmt.Sprintf("--leader=%v", leader))
			t.Cleanup(hung.release)
			addr := p.healthAddr(t)
			eventuallyHealth(t, addr, 10*time.Second, "polling", func(r healthReport, ok bool) bool { return ok && r.Status == "polling" })
			hung.hang()
			if !leader {
				r := eventuallyHealth(t, addr, 10*time.Second, "unhealthy", func(_ healthReport, ok bool) bool { return !ok })
				assert.Contains(t, r.Reason, "the poll loop has not completed a cycle")
				return
			}
			err := p.result(t, 10*time.Second)
			require.ErrorIs(t, err, outboxpg.ErrStalled)
			_, ok := checkHealth(t, addr)
			assert.False(t, ok, "a poller that exited answers no health check")
		})
	}
}

// The leader cannot obtain a platform token: after the stall timeout it is unhealthy.
func TestOutboxHealthTokenUnobtainable(t *testing.T) {
	_ = captureLog(t)
	old := tokenProbeEvery
	tokenProbeEvery = 100 * time.Millisecond
	t.Cleanup(func() { tokenProbeEvery = old })
	tokens := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(tokens.Close)
	dbURL := emptyDB(t)
	p := startPoller(t, dbURL, newStubPlatform(t).srv.URL,
		"--client-id", "svc", "--client-secret", "not-a-secret", "--token-url", tokens.URL)
	addr := p.healthAddr(t)
	r := eventuallyHealth(t, addr, 10*time.Second, "unhealthy", func(r healthReport, ok bool) bool {
		return !ok && r.Role == "leader"
	})
	assert.Contains(t, r.Reason, "cannot obtain a platform token")
	require.NoError(t, p.stop(t, 5*time.Second))
}

// cutProxy forwards TCP to the database until cut, then refuses and drops every connection.
type cutProxy struct {
	ln    net.Listener
	mu    sync.Mutex
	conns []net.Conn
}

func newCutProxy(t *testing.T, target string) *cutProxy {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	p := &cutProxy{ln: ln}
	t.Cleanup(p.cut)
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			up, err := net.Dial("tcp", target)
			if err != nil {
				_ = c.Close()
				continue
			}
			p.mu.Lock()
			p.conns = append(p.conns, c, up)
			p.mu.Unlock()
			go func() { _, _ = io.Copy(up, c); _ = up.Close() }()
			go func() { _, _ = io.Copy(c, up); _ = c.Close() }()
		}
	}()
	return p
}

func (p *cutProxy) cut() {
	_ = p.ln.Close()
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, c := range p.conns {
		_ = c.Close()
	}
	p.conns = nil
}

// A standby that can no longer reach the database turns unhealthy after the stall timeout.
func TestOutboxHealthStandbyDatabaseUnreachable(t *testing.T) {
	_ = captureLog(t)
	dbURL := emptyDB(t)
	a := startPoller(t, dbURL, newStubPlatform(t).srv.URL)
	eventuallyHealth(t, a.healthAddr(t), 10*time.Second, "a leads", func(r healthReport, ok bool) bool { return ok && r.Role == "leader" })

	u, err := url.Parse(dbURL)
	require.NoError(t, err)
	px := newCutProxy(t, u.Host)
	u.Host = px.ln.Addr().String()
	b := startPoller(t, u.String(), newStubPlatform(t).srv.URL)
	addr := b.healthAddr(t)
	eventuallyHealth(t, addr, 10*time.Second, "b stands by", func(r healthReport, ok bool) bool { return ok && r.Role == "standby" })
	px.cut()
	r := eventuallyHealth(t, addr, 10*time.Second, "b unhealthy", func(_ healthReport, ok bool) bool { return !ok })
	assert.Contains(t, r.Reason, "cannot reach the source database")
	require.NoError(t, b.stop(t, 5*time.Second))
	require.NoError(t, a.stop(t, 5*time.Second))
}

// TestOutboxHelperProcess is `fcdev outbox` in a child process (FC_TEST_HELPER_OUTBOX=1), with
// the arguments in FC_TEST_HELPER_OUTBOX_ARGS.
func TestOutboxHelperProcess(t *testing.T) {
	if os.Getenv("FC_TEST_HELPER_OUTBOX") != "1" {
		t.Skip("the child process of the outbox leadership tests")
	}
	var args []string
	require.NoError(t, json.Unmarshal([]byte(os.Getenv("FC_TEST_HELPER_OUTBOX_ARGS")), &args))
	cmd := newOutboxCmd()
	cmd.SetArgs(args)
	if err := cmd.ExecuteContext(context.Background()); err != nil {
		fmt.Fprintln(os.Stderr, "fcdev outbox:", err)
		os.Exit(1)
	}
	os.Exit(0)
}

// startChildPoller runs `fcdev outbox` as a child process.
func startChildPoller(t *testing.T, dbURL, platformURL string) (*exec.Cmd, *syncBuf) {
	t.Helper()
	args, err := json.Marshal([]string{
		"--env-file", "", "--source-db-url", dbURL, "--target-url", platformURL,
		"--poll-interval-ms", "50", "--leader-lease-ms", "500", "--stall-timeout-ms", "2000", "--health-addr", "off",
	})
	require.NoError(t, err)
	c := exec.Command(os.Args[0], "-test.run=^TestOutboxHelperProcess$") //nolint:gosec // G204: this test binary
	c.Env = append(os.Environ(), "FC_TEST_HELPER_OUTBOX=1", "FC_TEST_HELPER_OUTBOX_ARGS="+string(args))
	out := &syncBuf{}
	c.Stdout, c.Stderr = out, out
	require.NoError(t, c.Start())
	t.Cleanup(func() {
		_ = c.Process.Signal(syscall.SIGCONT)
		_ = c.Process.Kill()
		_ = c.Wait()
	})
	return c, out
}

func since(t0 time.Time) string { return time.Since(t0).Round(10 * time.Millisecond).String() }

// The leader's process is stopped (SIGSTOP: a stop-the-world of the whole process). The server
// ends its silent lock transaction after two leases and the standby takes over; when the
// stopped process resumes it sends nothing and stands by.
func TestOutboxPausedLeaderLosesLeadershipAndSendsNothingOnResume(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("no SIGSTOP on Windows")
	}
	_ = captureLog(t)
	dbURL := emptyDB(t)
	platA, platB := newStubPlatform(t), newStubPlatform(t)
	a, aOut := startChildPoller(t, dbURL, platA.srv.URL)
	eventually(t, 30*time.Second, "the child to create the table", func() bool { return tableExists(t, dbURL) })
	insertRows(t, dbURL, "orders:1", "PAUSE", 1, 1)
	eventually(t, 20*time.Second, "the child leads and delivers", func() bool { return len(platA.delivered()) == 1 })
	b := startPoller(t, dbURL, platB.srv.URL)
	eventuallyHealth(t, b.healthAddr(t), 10*time.Second, "b stands by", func(r healthReport, ok bool) bool { return ok && r.Role == "standby" })

	require.NoError(t, a.Process.Signal(syscall.SIGSTOP))
	stopped := time.Now()
	insertRows(t, dbURL, "orders:1", "PAUSE", 2, 6)
	eventually(t, 10*time.Second, "b delivers", func() bool { return len(platB.delivered()) == 5 })
	took := time.Since(stopped)
	t.Logf("b took over %s after the leader stopped", since(stopped))
	assert.GreaterOrEqual(t, took, time.Second, "the server ends the stopped leader's transaction only after two leases")
	assert.Less(t, took, 5*time.Second)

	require.NoError(t, a.Process.Signal(syscall.SIGCONT))
	insertRows(t, dbURL, "orders:1", "PAUSE", 7, 9)
	eventually(t, 10*time.Second, "b delivers after the resume", func() bool { return len(platB.delivered()) == 8 })
	waitFor(t, aOut, "outbox standing by")
	time.Sleep(300 * time.Millisecond)
	assert.Len(t, platA.delivered(), 1, "the resumed process sent nothing")
	assert.Contains(t, aOut.String(), "outbox leadership lost")
	require.NoError(t, b.stop(t, 5*time.Second))
}

// The leader's process is killed: its connection closes with it and the standby takes over within
// a check and the grace.
func TestOutboxKilledLeaderIsReplacedWithinSeconds(t *testing.T) {
	_ = captureLog(t)
	dbURL := emptyDB(t)
	platA, platB := newStubPlatform(t), newStubPlatform(t)
	a, _ := startChildPoller(t, dbURL, platA.srv.URL)
	eventually(t, 30*time.Second, "the child to create the table", func() bool { return tableExists(t, dbURL) })
	insertRows(t, dbURL, "orders:1", "KILL", 1, 1)
	eventually(t, 20*time.Second, "the child leads and delivers", func() bool { return len(platA.delivered()) == 1 })
	b := startPoller(t, dbURL, platB.srv.URL)
	eventuallyHealth(t, b.healthAddr(t), 10*time.Second, "b stands by", func(r healthReport, ok bool) bool { return ok && r.Role == "standby" })

	require.NoError(t, a.Process.Kill())
	killed := time.Now()
	insertRows(t, dbURL, "orders:1", "KILL", 2, 4)
	eventually(t, 10*time.Second, "b delivers", func() bool { return len(platB.delivered()) == 3 })
	t.Logf("b took over %s after the leader was killed", since(killed))
	assert.Less(t, time.Since(killed), 2*time.Second)
	assert.True(t, slices.Equal(platB.delivered(), []string{"KILL2", "KILL3", "KILL4"}), "in order: %v", platB.delivered())
	require.NoError(t, b.stop(t, 5*time.Second))
}
