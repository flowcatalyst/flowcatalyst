package postgres

// Leadership of one outbox, held in the source database.
//
// Design note. Every poller of an outbox shares exactly one thing: the database that holds
// outbox_messages. The leader holds a transaction-level advisory lock,
// pg_try_advisory_xact_lock(classid, objid), inside a transaction it keeps open on a
// connection of its own. No table is needed, so the poller still runs no DDL. The leader runs a
// statement in that transaction every Lease/5 to confirm it still holds the lock, and treats
// itself as the leader only until Lease after the last confirmation was sent. The transaction
// carries SET LOCAL idle_in_transaction_session_timeout = 2*Lease, so the server ends it, and
// releases the lock, by its own clock once the leader goes quiet for that long, whatever the
// cause: the process died without closing its socket, a network partition, a paused process,
// a hung confirmation loop. A new holder waits Lease*6/5 after acquiring the lock before it
// dispatches, which covers every other way the lock can go (an administrator ending the
// session, a failover, a pooler closing the server connection): the old leader stopped
// dispatching by then, by its own clock.
//
// Why a transaction lock and not a session lock (pg_try_advisory_lock): a transaction-mode
// connection pooler (PgBouncer pool_mode=transaction, Supavisor, pgcat) can run each statement
// of a session on a different server connection, so a session lock is taken on one server
// connection and outlives the client (PgBouncer lists session advisory locks as unsupported in
// transaction mode). A transaction is always kept on one server connection, by every pooler, so
// an open transaction holding an xact lock works through all of them; RDS Proxy does not pin a
// session for xact locks. Each confirmation also checks that pg_backend_pid() and now() (the
// transaction's start) are the ones the lock was taken with, so a pooler that broke this would
// be caught. A statement-mode pooler refuses transactions: the poller exits with an error.
//
// The open transaction is cheap: in READ COMMITTED with no xid and nothing but function calls
// it holds no snapshot between statements (pg_stat_activity.backend_xmin is NULL), so it holds
// back no vacuum, and it holds no relation lock, so it blocks no DDL (CREATE INDEX CONCURRENTLY
// skips backends without an xmin). transaction_timeout (PostgreSQL 17+) is set to 0 for it.
//
// The lock is keyed to the outbox: classid is a fixed constant, objid a hash of the schema and
// table outbox_messages resolves to. Advisory locks are per database already, so two
// databases on one server never collide, and two schemas of one database get their own keys.

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
)

// LockClassID is the classid of every outbox leadership lock ("fcOB").
const LockClassID int32 = 0x66634f42

// OutboxLock names one outbox and its advisory lock.
type OutboxLock struct {
	Database string
	Schema   string
	Table    string
	ObjID    int32
}

// String is the outbox's name for logs: database.schema.table.
func (k OutboxLock) String() string { return k.Database + "." + k.Schema + "." + k.Table }

// Lock returns the lock of the outbox_messages table this repository's SQL resolves to (the
// connection's search_path). The table must exist.
func (r *Repository) Lock(ctx context.Context) (OutboxLock, error) {
	var k OutboxLock
	err := r.pool.QueryRow(ctx, `
SELECT current_database(), n.nspname, c.relname
  FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
 WHERE c.oid = to_regclass('outbox_messages')`).Scan(&k.Database, &k.Schema, &k.Table)
	if errors.Is(err, pgx.ErrNoRows) {
		return k, errors.New("outbox_messages does not exist")
	}
	if err != nil {
		return k, err
	}
	h := fnv.New32a()
	_, _ = h.Write([]byte(k.Schema + "." + k.Table))
	k.ObjID = int32(h.Sum32()) //nolint:gosec // G115: the hash's bits are the key; wrapping is intended
	return k, nil
}

// Role is where a poller stands in the election.
type Role string

// Roles.
const (
	// RoleConnecting: no usable lock connection (starting, or reconnecting after a loss).
	RoleConnecting Role = "connecting"
	// RoleStandby: another poller holds the lock; this one checks for it every Lease/5.
	RoleStandby Role = "standby"
	// RoleTakingOver: holds the lock and waits out the takeover grace before dispatching.
	RoleTakingOver Role = "taking_over"
	// RoleLeader: holds the lock and dispatches.
	RoleLeader Role = "leader"
)

// LeaderOptions tunes a [Leadership]. Zero values take the defaults.
type LeaderOptions struct {
	// Lease is how long a leader keeps dispatching after it last confirmed the lock (default
	// 5s). The lock is confirmed every Lease/5; the server ends a holder silent for 2*Lease; a
	// new holder waits Lease*6/5 before dispatching.
	Lease time.Duration
	// ApplicationName is the lock connection's application_name (default fc-outbox-leader).
	ApplicationName string
	// Alive, when set, reports when the processor's poll loop last completed an iteration. A
	// holder whose loop has not done so for StallAfter gives up the lock and Run returns
	// [ErrStalled]: nothing else would ever replace a leader that holds the lock and never polls.
	Alive      func() time.Time
	StallAfter time.Duration
	// OnRole is called (from Run's goroutine) at every change of role.
	OnRole func(Role)
}

// Fatal errors of [Leadership.Run]: the poller cannot carry on and should exit.
var (
	ErrStalled          = errors.New("the outbox poll loop stalled while this poller held the leadership; gave the leadership up")
	ErrStatementPooling = errors.New("the source database connection refuses transactions (a statement-mode pooler?); outbox leadership needs a direct connection, a session- or transaction-mode pooler, or leadership off")
	ErrNotOneBackend    = errors.New("the lock connection's transaction moved to another server connection; outbox leadership cannot work through this pooler: connect directly, or switch leadership off")
)

// LeaderStatus is a snapshot of a [Leadership].
type LeaderStatus struct {
	Outbox string
	Role   Role
	// Since is when the current role began.
	Since time.Time
	// LastContact is when a statement on the lock connection last succeeded (zero: never).
	LastContact time.Time
	// LastError is the last lock-connection error, "" after a success.
	LastError string
	Term      uint64
}

// Leadership elects one leader among the pollers of one outbox (see the design note above).
type Leadership struct {
	cfg  *pgx.ConnConfig
	lock OutboxLock
	opts LeaderOptions

	check, serverTimeout, grace time.Duration

	mu          sync.Mutex
	role        Role
	since       time.Time
	holding     bool
	graceUntil  time.Time
	deadline    time.Time
	lastContact time.Time
	lastErr     string
	term        uint64
}

// NewLeadership prepares an election for lock among pollers connecting with cfg (copied; the
// copy uses the simple protocol, so that no prepared statement outlives a pooler's transaction).
func NewLeadership(cfg *pgx.ConnConfig, lock OutboxLock, opts LeaderOptions) *Leadership {
	if opts.Lease <= 0 {
		opts.Lease = 5 * time.Second
	}
	if opts.ApplicationName == "" {
		opts.ApplicationName = "fc-outbox-leader"
	}
	c := cfg.Copy()
	c.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol
	c.RuntimeParams["application_name"] = opts.ApplicationName
	return &Leadership{
		cfg: c, lock: lock, opts: opts,
		check:         max(opts.Lease/5, 20*time.Millisecond),
		serverTimeout: 2 * opts.Lease,
		grace:         opts.Lease + opts.Lease/5,
		role:          RoleConnecting,
		since:         time.Now(),
	}
}

// Grace is how long a new holder waits before it dispatches.
func (l *Leadership) Grace() time.Duration { return l.grace }

// IsLeader reports whether this poller may dispatch now: it holds the lock, has waited out the
// grace, and confirmed the lock less than Lease ago. It goes false by itself when confirmations
// stop, whatever the goroutine confirming them is doing.
func (l *Leadership) IsLeader() bool {
	now := time.Now()
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.holding && !now.Before(l.graceUntil) && now.Before(l.deadline)
}

// Term numbers the leaderships this poller has taken; it changes at each acquisition.
func (l *Leadership) Term() uint64 {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.term
}

// Status returns a snapshot.
func (l *Leadership) Status() LeaderStatus {
	l.mu.Lock()
	defer l.mu.Unlock()
	role := l.role
	if role == RoleLeader && !time.Now().Before(l.deadline) {
		role = RoleTakingOver // holds the lock but cannot confirm it: not dispatching
	}
	return LeaderStatus{
		Outbox: l.lock.String(), Role: role, Since: l.since, LastContact: l.lastContact,
		LastError: l.lastErr, Term: l.term,
	}
}

func (l *Leadership) currentRole() Role {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.role
}

func (l *Leadership) setRole(r Role) {
	l.mu.Lock()
	changed := l.role != r
	if changed {
		l.role, l.since = r, time.Now()
	}
	l.mu.Unlock()
	if changed && l.opts.OnRole != nil {
		l.opts.OnRole(r)
	}
}

func (l *Leadership) contact(err error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if err != nil {
		l.lastErr = err.Error()
		return
	}
	l.lastContact, l.lastErr = time.Now(), ""
}

// drop forgets the lock (before it is released, so nothing in this process passes IsLeader
// after the release).
func (l *Leadership) drop() {
	l.mu.Lock()
	l.holding = false
	l.mu.Unlock()
}

// Run takes part in the election until ctx ends, then gives the lock up if it holds it and
// returns nil. It returns early only with a fatal error ([ErrStalled], [ErrStatementPooling],
// [ErrNotOneBackend]); a database it cannot reach is retried for ever (the health check
// reports it).
func (l *Leadership) Run(ctx context.Context) error {
	log := slog.With("outbox", l.lock.String())
	pause := l.check
	var failingSince, lastLogged time.Time
	for ctx.Err() == nil {
		conn, err := l.connect(ctx)
		if err != nil {
			if ctx.Err() != nil {
				break
			}
			l.contact(err)
			l.setRole(RoleConnecting)
			now := time.Now()
			if failingSince.IsZero() {
				failingSince = now
			}
			if lastLogged.IsZero() || now.Sub(lastLogged) >= time.Minute {
				log.Warn("outbox leadership: cannot reach the source database; retrying", "err", err,
					"failing_for", now.Sub(failingSince).Round(time.Second).String())
				lastLogged = now
			}
			if !sleep(ctx, pause) {
				break
			}
			pause = min(pause*2, 5*time.Second)
			continue
		}
		failingSince, lastLogged, pause = time.Time{}, time.Time{}, l.check
		err = l.session(ctx, conn, log)
		l.drop()
		closeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Second)
		_ = conn.Close(closeCtx)
		cancel()
		if errors.Is(err, ErrStalled) || errors.Is(err, ErrStatementPooling) || errors.Is(err, ErrNotOneBackend) {
			l.setRole(RoleConnecting)
			return err
		}
		if ctx.Err() != nil {
			break
		}
		l.contact(err)
		l.setRole(RoleConnecting)
		if !sleep(ctx, l.check) {
			break
		}
	}
	l.drop()
	l.setRole(RoleConnecting)
	return nil
}

func (l *Leadership) connect(ctx context.Context) (*pgx.Conn, error) {
	c, cancel := context.WithTimeout(ctx, max(l.opts.Lease, 5*time.Second))
	defer cancel()
	return pgx.ConnectConfig(c, l.cfg)
}

// session runs the election on one connection until it fails, ctx ends or a fatal error.
func (l *Leadership) session(ctx context.Context, conn *pgx.Conn, log *slog.Logger) error {
	var pid int32
	var xstart time.Time
	holding := false
	pg17 := serverMajor(conn) >= 17
	for {
		if !holding {
			sent := time.Now()
			got, p, x, err := l.tryAcquire(ctx, conn, pg17)
			if err != nil {
				return err
			}
			l.contact(nil)
			if !got {
				if l.currentRole() != RoleStandby {
					log.Info("outbox standing by: another poller is the leader")
				}
				l.setRole(RoleStandby)
			} else {
				holding, pid, xstart = true, p, x
				now := time.Now()
				l.mu.Lock()
				l.holding, l.graceUntil, l.deadline = true, now.Add(l.grace), sent.Add(l.opts.Lease)
				l.term++
				l.mu.Unlock()
				l.setRole(RoleTakingOver)
				log.Info("outbox leadership acquired; dispatching starts after the takeover grace",
					"grace", l.grace.String(), "lock_classid", LockClassID, "lock_objid", l.lock.ObjID)
			}
		} else {
			if l.opts.Alive != nil && l.opts.StallAfter > 0 && l.currentRole() == RoleLeader {
				if alive := l.opts.Alive(); !alive.IsZero() && time.Since(alive) > l.opts.StallAfter {
					since := time.Since(alive)
					l.drop()
					log.Error("outbox poll loop has not completed a cycle; giving the leadership up",
						"stalled_for", since.Round(time.Second).String())
					return ErrStalled
				}
			}
			sent := time.Now()
			c, cancel := context.WithTimeout(ctx, l.opts.Lease)
			var p int32
			var x time.Time
			err := conn.QueryRow(c, `SELECT pg_backend_pid(), now()`).Scan(&p, &x)
			cancel()
			if err != nil {
				if ctx.Err() != nil {
					return l.release(conn, log)
				}
				l.drop()
				log.Warn("outbox leadership lost; dispatching stopped", "err", err)
				return err
			}
			if p != pid || !x.Equal(xstart) {
				l.drop()
				log.Error("outbox leadership: the lock transaction is not the one the lock was taken in",
					"pid", pid, "now_pid", p)
				return ErrNotOneBackend
			}
			l.contact(nil)
			l.mu.Lock()
			l.deadline = sent.Add(l.opts.Lease)
			ready := !time.Now().Before(l.graceUntil)
			l.mu.Unlock()
			if ready && l.currentRole() == RoleTakingOver {
				l.setRole(RoleLeader)
				log.Info("outbox leader: dispatching")
			}
		}
		select {
		case <-ctx.Done(): // shutdown
			if holding {
				return l.release(conn, log)
			}
			return nil
		case <-time.After(l.check): // next check
		}
	}
}

// tryAcquire opens a transaction and tries the lock in it. Holding, it bounds the transaction
// with the server's idle-in-transaction timeout and returns the backend pid and the
// transaction's start; not holding, it rolls back.
func (l *Leadership) tryAcquire(ctx context.Context, conn *pgx.Conn, pg17 bool) (bool, int32, time.Time, error) {
	c, cancel := context.WithTimeout(ctx, l.opts.Lease)
	defer cancel()
	if _, err := conn.Exec(c, "BEGIN"); err != nil {
		if s := strings.ToLower(err.Error()); strings.Contains(s, "statement pooling") || strings.Contains(s, "transaction blocks not allowed") {
			return false, 0, time.Time{}, fmt.Errorf("%w: %w", ErrStatementPooling, err)
		}
		return false, 0, time.Time{}, err
	}
	var got bool
	var pid int32
	var xstart time.Time
	if err := conn.QueryRow(c, `SELECT pg_try_advisory_xact_lock($1, $2), pg_backend_pid(), now()`,
		LockClassID, l.lock.ObjID).Scan(&got, &pid, &xstart); err != nil {
		return false, 0, time.Time{}, err
	}
	if !got {
		_, err := conn.Exec(c, "ROLLBACK")
		return false, 0, time.Time{}, err
	}
	q := `SELECT set_config('idle_in_transaction_session_timeout', $1, true), set_config('statement_timeout', $2, true)`
	if pg17 {
		q += `, set_config('transaction_timeout', '0', true)`
	}
	if _, err := conn.Exec(c, q, strconv.FormatInt(l.serverTimeout.Milliseconds(), 10),
		strconv.FormatInt(l.opts.Lease.Milliseconds(), 10)); err != nil {
		return false, 0, time.Time{}, err
	}
	return true, pid, xstart, nil
}

// release gives the lock up at shutdown: forget it first, then end the transaction.
func (l *Leadership) release(conn *pgx.Conn, log *slog.Logger) error {
	l.drop()
	c, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, err := conn.Exec(c, "ROLLBACK")
	log.Info("outbox leadership released")
	return err
}

func serverMajor(conn *pgx.Conn) int {
	v := conn.PgConn().ParameterStatus("server_version")
	n, _ := strconv.Atoi(strings.SplitN(strings.SplitN(v, ".", 2)[0], " ", 2)[0])
	return n
}

func sleep(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done(): // shutdown
		return false
	case <-t.C: // waited
		return true
	}
}
