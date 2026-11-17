//go:build integration

package postgres_test

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/flowcatalyst/framework/go/usecase"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	outboxpg "github.com/flowcatalyst/flowcatalyst-go/internal/outbox/postgres"
)

// lockedBuf is a log sink the waiting goroutine and the test share.
type lockedBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (l *lockedBuf) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Write(p)
}
func (l *lockedBuf) String() string { l.mu.Lock(); defer l.mu.Unlock(); return l.b.String() }

func captureLogs(t *testing.T) *lockedBuf {
	b := &lockedBuf{}
	old := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(b, nil)))
	t.Cleanup(func() { slog.SetDefault(old) })
	return b
}

// relations counts what exists in the pool's schema: tables, indexes, anything.
func relations(t *testing.T, pool *pgxpool.Pool) int {
	t.Helper()
	var n int
	require.NoError(t, pool.QueryRow(t.Context(),
		`SELECT count(*) FROM pg_class WHERE relnamespace = current_schema()::regnamespace`).Scan(&n))
	return n
}

// frameworkV0001 is the Go framework's outbox migration V0001 as the framework embeds it.
func frameworkV0001(t *testing.T) string {
	t.Helper()
	b, err := fs.ReadFile(usecase.OutboxMigrations(), "V0001__create_outbox_messages.sql")
	require.NoError(t, err)
	return string(b)
}

func TestTableState(t *testing.T) {
	pool := emptySchema(t)
	repo := outboxpg.New(pool)

	exists, missing, err := repo.TableState(t.Context())
	require.NoError(t, err)
	assert.False(t, exists)
	assert.Nil(t, missing)
	assert.Zero(t, relations(t, pool), "TableState creates nothing")

	_, err = pool.Exec(t.Context(), `CREATE TABLE outbox_messages (id VARCHAR(26) PRIMARY KEY, type VARCHAR(20), payload TEXT)`)
	require.NoError(t, err)
	exists, missing, err = repo.TableState(t.Context())
	require.NoError(t, err)
	assert.True(t, exists)
	assert.Equal(t, []string{"message_group", "status", "retry_count", "error_message", "created_at", "updated_at"}, missing)

	// the framework's own table has every column the processor uses
	_, err = pool.Exec(t.Context(), `DROP TABLE outbox_messages`)
	require.NoError(t, err)
	_, err = pool.Exec(t.Context(), frameworkV0001(t))
	require.NoError(t, err)
	exists, missing, err = repo.TableState(t.Context())
	require.NoError(t, err)
	assert.True(t, exists)
	assert.Empty(t, missing)
}

// WaitForTable runs no DDL while it waits, says so once, keeps waiting while the table lacks
// columns the processor uses, and returns once the table is complete.
func TestWaitForTableWaitsWithoutDDL(t *testing.T) {
	logs := captureLogs(t)
	pool := emptySchema(t)
	done := make(chan error, 1)
	go func() {
		done <- outboxpg.New(pool).WaitForTable(t.Context(),
			outboxpg.WaitOptions{CheckEvery: 20 * time.Millisecond, LogEvery: 150 * time.Millisecond})
	}()

	time.Sleep(400 * time.Millisecond)
	assert.Zero(t, relations(t, pool), "nothing created while waiting")
	select {
	case err := <-done:
		t.Fatalf("WaitForTable returned while there is no table: %v", err)
	default:
	}
	out := logs.String()
	assert.Equal(t, 1, strings.Count(out, "schema creation is off"), "the first message, once")
	assert.Contains(t, out, "still waiting for the application to create outbox_messages", "logged again at LogEvery")

	_, err := pool.Exec(t.Context(), `CREATE TABLE outbox_messages (id VARCHAR(26) PRIMARY KEY)`)
	require.NoError(t, err)
	time.Sleep(300 * time.Millisecond)
	select {
	case err := <-done:
		t.Fatalf("WaitForTable returned for a table without the processor's columns: %v", err)
	default:
	}
	assert.Contains(t, logs.String(), "outbox_messages exists but lacks columns the outbox processor uses")

	_, err = pool.Exec(t.Context(), `DROP TABLE outbox_messages`)
	require.NoError(t, err)
	_, err = pool.Exec(t.Context(), frameworkV0001(t))
	require.NoError(t, err)
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("WaitForTable did not return once the table was created")
	}
	assert.Contains(t, logs.String(), "outbox_messages found; starting the outbox processor")
}

// A table that is already there is no wait and no log line.
func TestWaitForTableReturnsAtOnceWhenTheTableIsThere(t *testing.T) {
	logs := captureLogs(t)
	pool := table(t)
	require.NoError(t, outboxpg.New(pool).WaitForTable(t.Context(), outboxpg.WaitOptions{CheckEvery: time.Minute}))
	assert.Empty(t, logs.String())
}

func TestWaitForTableStopsOnCancel(t *testing.T) {
	_ = captureLogs(t)
	pool := emptySchema(t)
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() {
		done <- outboxpg.New(pool).WaitForTable(ctx, outboxpg.WaitOptions{CheckEvery: time.Minute})
	}()
	time.Sleep(100 * time.Millisecond)
	start := time.Now()
	cancel()
	select {
	case err := <-done:
		assert.ErrorIs(t, err, context.Canceled)
		assert.Less(t, time.Since(start), time.Second)
	case <-time.After(5 * time.Second):
		t.Fatal("WaitForTable did not stop on cancel")
	}
}

// A check that fails is an error at once, not a reason to wait.
func TestWaitForTableFailedCheckIsAnError(t *testing.T) {
	_ = captureLogs(t)
	pool := emptySchema(t)
	pool.Close()
	err := outboxpg.New(pool).WaitForTable(t.Context(), outboxpg.WaitOptions{CheckEvery: time.Minute})
	require.Error(t, err)
	assert.False(t, errors.Is(err, context.Canceled))
	assert.Contains(t, err.Error(), "check for outbox_messages")
}
