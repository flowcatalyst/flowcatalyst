package control

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Listener fans out Postgres `NOTIFY fng_desired, '<pool>'` wakeups
// (docs/function-runner-plan.md §8.1, §8.3) to the goroutines blocked in a
// long-poll of the desired-state document. One LISTEN connection per
// platform process (Run holds it for the process's lifetime, reconnecting
// with backoff on error) fanned out to per-pool waiters — never one LISTEN
// connection per in-flight request.
//
// If Run is never started (a test that only exercises the HTTP handlers, or
// a platform outage on the Postgres side), Wait's channels simply never
// close on their own: callers must always race them against their own
// re-render timer (see State.desired), never wait on one alone.
type Listener struct {
	pool *pgxpool.Pool
	log  *slog.Logger

	mu      sync.Mutex
	waiters map[string][]chan struct{} // pool -> channels to close on the next wake
}

// NewListener builds a Listener. Call Run to start listening; it does
// nothing until then.
func NewListener(pool *pgxpool.Pool, log *slog.Logger) *Listener {
	if log == nil {
		log = slog.Default()
	}
	return &Listener{pool: pool, log: log, waiters: map[string][]chan struct{}{}}
}

// Wait returns a channel that closes the next time pool's revision is
// bumped. Callers register a fresh Wait for every wait cycle; a channel is
// used (and discarded) at most once.
func (l *Listener) Wait(pool string) <-chan struct{} {
	ch := make(chan struct{})
	l.mu.Lock()
	l.waiters[pool] = append(l.waiters[pool], ch)
	l.mu.Unlock()
	return ch
}

// wake closes and drops every waiter registered for pool. Closed by wake,
// never by a receiver — receivers only ever read from it.
func (l *Listener) wake(pool string) {
	l.mu.Lock()
	chans := l.waiters[pool]
	delete(l.waiters, pool)
	l.mu.Unlock()
	for _, ch := range chans {
		close(ch)
	}
}

// Run holds one LISTEN fng_desired connection for ctx's lifetime, waking
// waiters on every notification. Blocks until ctx is done. A connection
// error (or the initial LISTEN failing) backs off and retries — a Postgres
// blip never crashes the process, it just falls back to every waiter's own
// re-render interval until the connection comes back.
func (l *Listener) Run(ctx context.Context) {
	backoff := time.Second
	for ctx.Err() == nil {
		err := l.listenOnce(ctx)
		if ctx.Err() != nil {
			return // shutdown: listenOnce returned because ctx ended, not a real failure
		}
		l.log.Warn("function control: LISTEN fng_desired failed; falling back to periodic re-render",
			"err", err, "retry_in", backoff)
		select {
		case <-ctx.Done(): // shutdown
			return
		case <-time.After(backoff): // reconnect backoff elapsed
		}
		backoff = min(backoff*2, 30*time.Second)
	}
}

// listenOnce acquires one connection, LISTENs, and waits for notifications
// until ctx ends or the connection fails.
func (l *Listener) listenOnce(ctx context.Context) error {
	conn, err := l.pool.Acquire(ctx)
	if err != nil {
		return err
	}
	defer conn.Release()
	if _, err := conn.Exec(ctx, "LISTEN fng_desired"); err != nil {
		return err
	}
	for {
		n, err := conn.Conn().WaitForNotification(ctx)
		if err != nil {
			return err
		}
		l.wake(n.Payload)
	}
}
