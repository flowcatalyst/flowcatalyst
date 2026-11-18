//go:build integration

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"runtime"
	"slices"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	fwdb "github.com/flowcatalyst/framework/go/database"
	"github.com/flowcatalyst/framework/go/fw"
	"github.com/flowcatalyst/framework/go/usecase"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	outboxpg "github.com/flowcatalyst/flowcatalyst-go/internal/outbox/postgres"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
)

// The poller beside an application that owns outbox_messages through its migrations (the Go
// framework's outbox module). The migrations these tests apply are the framework's own:
// usecase.OutboxMigrations() of github.com/flowcatalyst/framework/go (the version in go.mod),
// V0001__create_outbox_messages.sql and V0002__widen_outbox_client_id.sql, applied by the
// framework's own runner (database.ApplyMigrations), as an application applies them at start.

// emptyDB creates a database with nothing in it, as a first deployment's is, and returns its URL.
func emptyDB(t *testing.T) string {
	t.Helper()
	base := testpg.Pool(t)
	name := "obx_" + strings.ToLower(strings.NewReplacer("/", "_", "-", "_", "=", "_").Replace(t.Name()))
	name = name[:min(len(name), 60)]
	_, err := base.Exec(t.Context(), "DROP DATABASE IF EXISTS "+name+" WITH (FORCE)")
	require.NoError(t, err)
	_, err = base.Exec(t.Context(), "CREATE DATABASE "+name)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = base.Exec(context.Background(), "DROP DATABASE IF EXISTS "+name+" WITH (FORCE)")
	})
	c := base.Config().ConnConfig
	return (&url.URL{
		Scheme:   "postgres",
		User:     url.UserPassword(c.User, c.Password),
		Host:     fmt.Sprintf("%s:%d", c.Host, c.Port),
		Path:     "/" + name,
		RawQuery: "sslmode=disable",
	}).String()
}

// tableExists reports whether outbox_messages exists in the database at dbURL.
func tableExists(t *testing.T, dbURL string) bool {
	t.Helper()
	conn, err := pgx.Connect(t.Context(), dbURL)
	require.NoError(t, err)
	defer func() { _ = conn.Close(context.Background()) }()
	var exists bool
	require.NoError(t, conn.QueryRow(t.Context(), `SELECT to_regclass('outbox_messages') IS NOT NULL`).Scan(&exists))
	return exists
}

// migrateApp applies the framework's outbox migrations to dbURL with the framework's runner: what
// a framework application does at start.
func migrateApp(ctx context.Context, dbURL string) error {
	db, err := fwdb.Open(ctx, "default", dbURL)
	if err != nil {
		return err
	}
	defer db.Close()
	return fwdb.ApplyMigrations(ctx, db,
		[]fw.MigrationSet{{Module: usecase.OutboxModuleID, Files: usecase.OutboxMigrations()}},
		slog.New(slog.NewTextHandler(io.Discard, nil)))
}

// stubPlatform answers the events batch endpoint with SUCCESS for every item and records the
// payload ids it was sent.
type stubPlatform struct {
	mu  sync.Mutex
	ids []string
	srv *httptest.Server
}

func newStubPlatform(t *testing.T) *stubPlatform {
	p := &stubPlatform{}
	p.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/events/batch" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		var body struct {
			Items []struct {
				ID string `json:"id"`
			} `json:"items"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		results := make([]map[string]string, len(body.Items))
		p.mu.Lock()
		for i, it := range body.Items {
			p.ids = append(p.ids, it.ID)
			results[i] = map[string]string{"id": it.ID, "status": "SUCCESS"}
		}
		p.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"results": results})
	}))
	t.Cleanup(p.srv.Close)
	return p
}

func (p *stubPlatform) delivered() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return slices.Clone(p.ids)
}

// poller is a running `fcdev outbox`.
type poller struct {
	cancel context.CancelFunc
	done   chan error
	health chan string // its health address, once it listens
}

// healthAddr is the poller's health address, waiting for it to listen.
func (p *poller) healthAddr(t *testing.T) string {
	t.Helper()
	select {
	case a := <-p.health:
		p.health <- a
		return a
	case <-time.After(10 * time.Second):
		t.Fatal("the poller's health server did not start")
		return ""
	}
}

// stop cancels the poller and returns what the command returned, failing the test if it does
// not return within d.
func (p *poller) stop(t *testing.T, d time.Duration) error {
	t.Helper()
	p.cancel()
	return p.result(t, d)
}

func (p *poller) result(t *testing.T, d time.Duration) error {
	t.Helper()
	select {
	case err := <-p.done:
		p.done <- err // keep it for a later call (the cleanup)
		return err
	case <-time.After(d):
		t.Fatalf("fcdev outbox did not return within %v", d)
		return nil
	}
}

func (p *poller) running() bool {
	select {
	case err := <-p.done:
		p.done <- err
		return false
	default:
		return true
	}
}

// clearOutboxEnv keeps the developer's own FC_OUTBOX_* settings out of the command.
func clearOutboxEnv(t *testing.T) {
	for _, k := range []string{
		"FC_OUTBOX_SOURCE_DB_URL", "FC_OUTBOX_PLATFORM_URL", "FC_OUTBOX_PLATFORM_AUTH_TOKEN",
		"FC_OUTBOX_CLIENT_ID", "FC_OUTBOX_CLIENT_SECRET", "FLOWCATALYST_CLIENT_ID", "FLOWCATALYST_CLIENT_SECRET",
		"FC_OUTBOX_TOKEN_URL", "FC_OUTBOX_SCOPE", "FC_OUTBOX_BATCH_SIZE", "FC_OUTBOX_MAX_IN_FLIGHT",
		"FC_OUTBOX_POLL_INTERVAL_MS", "FC_OUTBOX_INIT_SCHEMA", "FC_OUTBOX_LEADER", "FC_OUTBOX_LEADER_LEASE_MS",
		"FC_OUTBOX_HEALTH_ADDR", "FC_OUTBOX_STALL_TIMEOUT_MS", "FC_OUTBOX_HEALTH_REQUIRE_TOKEN",
	} {
		t.Setenv(k, "")
	}
}

// startPoller runs `fcdev outbox --source-db-url dbURL --target-url platformURL extra...` in the
// background, checking for the table every 50ms.
func startPoller(t *testing.T, dbURL, platformURL string, extra ...string) *poller {
	t.Helper()
	return startPollerWith(t, outboxpg.WaitOptions{CheckEvery: 50 * time.Millisecond, LogEvery: time.Minute},
		dbURL, platformURL, extra...)
}

// startPollerWith is startPoller with the wait for the table paced by wait. The election is
// quick (a lease of 500ms) and the health check listens on a port of its own.
func startPollerWith(t *testing.T, wait outboxpg.WaitOptions, dbURL, platformURL string, extra ...string) *poller {
	t.Helper()
	return startPollerHooked(t, wait, outboxTestHooks{}, dbURL, platformURL, extra...)
}

func startPollerHooked(t *testing.T, wait outboxpg.WaitOptions, hooks outboxTestHooks, dbURL, platformURL string, extra ...string) *poller {
	t.Helper()
	clearOutboxEnv(t)
	old := outboxTableWait
	outboxTableWait = wait
	t.Cleanup(func() { outboxTableWait = old })
	cmd := newOutboxCmd()
	args := []string{
		"--env-file", "", "--source-db-url", dbURL, "--target-url", platformURL, "--poll-interval-ms", "50",
		"--leader-lease-ms", "500", "--stall-timeout-ms", "2000", "--health-addr", "127.0.0.1:0",
	}
	cmd.SetArgs(append(args, extra...))
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	p := &poller{done: make(chan error, 1), health: make(chan string, 1)}
	hooks.healthListening = func(addr string) { p.health <- addr }
	ctx, cancel := context.WithCancel(context.WithValue(context.Background(), outboxTestHooksKey{}, hooks))
	p.cancel = cancel
	go func() { p.done <- cmd.ExecuteContext(ctx) }()
	t.Cleanup(func() { cancel(); _ = p.result(t, 10*time.Second) })
	return p
}

// With --init-schema=false and no table, the poller creates nothing, does not exit and says it is
// waiting; once the application's migration creates the table it starts and delivers.
func TestOutboxNoInitSchemaWaitsForTheApplicationsTable(t *testing.T) {
	logs := captureLog(t)
	dbURL := emptyDB(t)
	plat := newStubPlatform(t)
	p := startPoller(t, dbURL, plat.srv.URL, "--init-schema=false")

	waitFor(t, logs, "waiting for the application to create outbox_messages")
	time.Sleep(500 * time.Millisecond) // ten checks
	assert.False(t, tableExists(t, dbURL), "the poller must not create outbox_messages")
	assert.True(t, p.running(), "the poller must keep waiting, not exit")
	assert.Equal(t, 1, strings.Count(logs.String(), "schema creation is off"),
		"logged once, not at every check")
	assert.NotContains(t, logs.String(), "fcdev outbox started")

	require.NoError(t, migrateApp(t.Context(), dbURL), "the application's migration must succeed")
	waitFor(t, logs, "outbox_messages found; starting the outbox processor")
	waitFor(t, logs, "fcdev outbox started")

	conn, err := pgx.Connect(t.Context(), dbURL)
	require.NoError(t, err)
	defer func() { _ = conn.Close(context.Background()) }()
	var history int
	require.NoError(t, conn.QueryRow(t.Context(),
		`SELECT count(*) FROM fw_schema_history WHERE module = $1`, usecase.OutboxModuleID).Scan(&history))
	assert.Equal(t, 2, history, "V0001 and V0002 recorded")

	// A row written as the framework writes one (type EVENT, the event as the payload).
	_, err = conn.Exec(t.Context(), `INSERT INTO outbox_messages (id, type, message_group, payload, client_id)
		VALUES ('0HZTESTOUTBOXROW000000001', 'EVENT', 'orders:1', $1, 'clt_0HZTEST')`,
		`{"id":"evt-after-migration","type":"shop:orders:order:created","source":"shop","data":{}}`)
	require.NoError(t, err)
	eventually(t, 10*time.Second, "the row to be delivered", func() bool {
		return slices.Contains(plat.delivered(), "evt-after-migration")
	})
	eventually(t, 5*time.Second, "the delivered row to be deleted", func() bool {
		var n int
		require.NoError(t, conn.QueryRow(t.Context(), `SELECT count(*) FROM outbox_messages`).Scan(&n))
		return n == 0
	})
	require.NoError(t, p.stop(t, 5*time.Second))
}

// The defect seen on a first deployment, as a regression test: the poller and the application
// start together on an empty database, the poller first. With --init-schema=false the
// application's first migration succeeds. With the old behaviour (--init-schema, the default)
// the poller has created the table and the migration fails with "already exists", and keeps
// failing on every restart.
func TestOutboxPollerStartedBeforeTheApplicationsFirstMigration(t *testing.T) {
	t.Run("init-schema=false: the migration succeeds", func(t *testing.T) {
		logs := captureLog(t)
		dbURL := emptyDB(t)
		p := startPoller(t, dbURL, newStubPlatform(t).srv.URL, "--init-schema=false")
		waitFor(t, logs, "waiting for the application to create outbox_messages")

		require.NoError(t, migrateApp(t.Context(), dbURL))
		waitFor(t, logs, "fcdev outbox started")
		require.NoError(t, migrateApp(t.Context(), dbURL), "a restart of the application applies nothing and succeeds")
		require.NoError(t, p.stop(t, 5*time.Second))
	})

	t.Run("init-schema=true (the old behaviour): the migration fails", func(t *testing.T) {
		logs := captureLog(t)
		dbURL := emptyDB(t)
		p := startPoller(t, dbURL, newStubPlatform(t).srv.URL) // the default: --init-schema=true
		waitFor(t, logs, "fcdev outbox started")
		require.True(t, tableExists(t, dbURL), "the poller created the table")

		err := migrateApp(t.Context(), dbURL)
		require.Error(t, err)
		assert.Contains(t, err.Error(), `relation "outbox_messages" already exists`)
		err = migrateApp(t.Context(), dbURL)
		require.Error(t, err, "every restart of the application fails the same way")
		assert.Contains(t, err.Error(), `relation "outbox_messages" already exists`)
		require.NoError(t, p.stop(t, 5*time.Second))
	})
}

// A poller waiting for its table stops at once on shutdown, though its next check is a minute
// away: on the command's context, and on SIGTERM (what ECS sends).
func TestOutboxNoInitSchemaStopsPromptlyWhileWaiting(t *testing.T) {
	for _, how := range []string{"context", "SIGTERM"} {
		t.Run(how, func(t *testing.T) {
			if how == "SIGTERM" && runtime.GOOS == "windows" {
				t.Skip("no SIGTERM on Windows")
			}
			logs := captureLog(t)
			dbURL := emptyDB(t)
			// the next check is a minute away: only the shutdown can end the wait in time
			p := startPollerWith(t, outboxpg.WaitOptions{CheckEvery: time.Minute}, dbURL, newStubPlatform(t).srv.URL,
				"--init-schema=false")
			waitFor(t, logs, "waiting for the application to create outbox_messages")

			start := time.Now()
			if how == "SIGTERM" {
				self, err := os.FindProcess(os.Getpid())
				require.NoError(t, err)
				require.NoError(t, self.Signal(syscall.SIGTERM)) // the command has caught it since before it logged
				err = p.result(t, 2*time.Second)
				require.NoError(t, err)
				assert.Contains(t, logs.String(), "shutdown signal received")
			} else {
				require.NoError(t, p.stop(t, 2*time.Second))
			}
			assert.Less(t, time.Since(start), time.Second)
			assert.Contains(t, logs.String(), "fcdev outbox stopped before outbox_messages was created")
			assert.False(t, tableExists(t, dbURL))
		})
	}
}

// With --init-schema=false a database the poller cannot use is still an error, not a wait.
func TestOutboxNoInitSchemaUnusableDatabaseIsAnError(t *testing.T) {
	c := testpg.Pool(t).Config().ConnConfig
	closed := func() int { // a port nothing listens on
		srv := httptest.NewServer(http.NotFoundHandler())
		u, _ := url.Parse(srv.URL)
		srv.Close()
		var port int
		_, _ = fmt.Sscanf(u.Port(), "%d", &port)
		return port
	}()
	for name, dbURL := range map[string]string{
		"nothing listening": fmt.Sprintf("postgres://postgres:postgres@127.0.0.1:%d/flowcatalyst?sslmode=disable&connect_timeout=2", closed),
		"wrong password":    fmt.Sprintf("postgres://%s:not-the-password@%s:%d/flowcatalyst?sslmode=disable", c.User, c.Host, c.Port),
		"no such role":      fmt.Sprintf("postgres://nobody_here:wrong@%s:%d/flowcatalyst?sslmode=disable", c.Host, c.Port),
		"no such database":  fmt.Sprintf("postgres://%s:%s@%s:%d/no_such_db_here?sslmode=disable", c.User, c.Password, c.Host, c.Port),
	} {
		t.Run(name, func(t *testing.T) {
			_ = captureLog(t)
			p := startPoller(t, dbURL, newStubPlatform(t).srv.URL, "--init-schema=false")
			err := p.result(t, 10*time.Second)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "connect source")
		})
	}
}

// eventually polls cond until it holds or d passes.
func eventually(t *testing.T, d time.Duration, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(d)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out after %v waiting for %s", d, what)
		}
		time.Sleep(20 * time.Millisecond)
	}
}
