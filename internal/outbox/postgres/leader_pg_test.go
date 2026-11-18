//go:build integration

package postgres_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/outbox"
	outboxpg "github.com/flowcatalyst/flowcatalyst-go/internal/outbox/postgres"
)

// The election of tests is quick: a lease of 500ms confirms every 100ms, the server ends a
// silent holder after 1s and a new holder waits 600ms.
const testLease = 500 * time.Millisecond

// arrival is one request reaching the platform stand-in: which poller sent it, its rows, when.
type arrival struct {
	poller string
	ids    []string
	at     time.Time
}

// recorder is a platform stand-in shared by several pollers: each poller's PlatformURL carries
// its name as the first path segment. It answers SUCCESS for every row after delay; while hang is
// set it does not answer at all (until the request is abandoned).
type recorder struct {
	mu       sync.Mutex
	delay    time.Duration
	hang     bool
	arrivals []arrival
	srv      *httptest.Server
}

func newRecorder(t *testing.T) *recorder {
	r := &recorder{}
	r.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		name, _, _ := strings.Cut(strings.TrimPrefix(req.URL.Path, "/"), "/")
		var body struct {
			Items []struct {
				PushID string `json:"pushId"`
			} `json:"items"`
		}
		_ = json.NewDecoder(req.Body).Decode(&body)
		ids := make([]string, len(body.Items))
		for i, it := range body.Items {
			ids[i] = it.PushID
		}
		r.mu.Lock()
		r.arrivals = append(r.arrivals, arrival{poller: name, ids: ids, at: time.Now()})
		delay, hang := r.delay, r.hang
		r.mu.Unlock()
		if hang {
			<-req.Context().Done()
			return
		}
		select {
		case <-time.After(delay):
		case <-req.Context().Done():
			return
		}
		out := make([]map[string]string, len(ids))
		for i := range ids {
			out[i] = map[string]string{"status": "SUCCESS"}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"results": out})
	}))
	t.Cleanup(r.srv.Close)
	return r
}

func (r *recorder) set(delay time.Duration, hang bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.delay, r.hang = delay, hang
}

func (r *recorder) all() []arrival {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]arrival(nil), r.arrivals...)
}

// senders returns who sent each row id (every sender, in order).
func (r *recorder) senders() map[string][]string {
	out := map[string][]string{}
	for _, a := range r.all() {
		for _, id := range a.ids {
			out[id] = append(out[id], a.poller)
		}
	}
	return out
}

// assertGroupOrder checks that in every group the rows arrived in the order written: no row ever
// arrives after a later row of its group (a repeat of the latest row is allowed). ids are
// "<group>-<seq>".
func assertGroupOrder(t *testing.T, arrivals []arrival) {
	t.Helper()
	latest := map[string]int{}
	for _, a := range arrivals {
		for _, id := range a.ids {
			g, n, ok := strings.Cut(id, "-")
			if !ok {
				continue
			}
			seq, _ := strconv.Atoi(n)
			if prev, seen := latest[g]; seen && seq < prev {
				t.Errorf("group %s: row %d arrived (from %s) after row %d", g, seq, a.poller, prev)
			}
			latest[g] = max(latest[g], seq)
		}
	}
}

// poller is one processor with its election, on a pool of its own.
type poller struct {
	name     string
	pool     *pgxpool.Pool
	proc     *outbox.Processor
	election *outboxpg.Leadership
	cancel   context.CancelFunc
	elected  chan error    // Run's result, once
	ended    chan struct{} // closed when the election's Run returned
	stopped  chan struct{}
}

type pollerOpts struct {
	connect  func(*pgxpool.Config) // e.g. through a proxy
	wrapRepo func(outbox.Repository) outbox.Repository
	tune     func(*outbox.Config)
	stall    time.Duration
}

// startPoller runs a poller of the outbox in schemaPool's schema, sending to rec as name.
func startPoller(t *testing.T, schemaPool *pgxpool.Pool, rec *recorder, name string, o pollerOpts) *poller {
	t.Helper()
	cfg := schemaPool.Config().Copy()
	if o.connect != nil {
		o.connect(cfg)
	}
	pool, err := pgxpool.NewWithConfig(context.Background(), cfg)
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	repo := outboxpg.New(pool)
	lock, err := repo.Lock(t.Context())
	require.NoError(t, err)

	pcfg := outbox.DefaultConfig()
	pcfg.PlatformURL = rec.srv.URL + "/" + name
	pcfg.PollInterval = 20 * time.Millisecond
	pcfg.RetryBackoffBase = 50 * time.Millisecond
	pcfg.RetryBackoffMax = 200 * time.Millisecond
	pcfg.HTTPTimeout = 2 * time.Second
	pcfg.DBTimeout = 2 * time.Second
	pcfg.TakeoverRecoveryDelay = pcfg.HTTPTimeout + time.Second
	if o.tune != nil {
		o.tune(&pcfg)
	}
	var r outbox.Repository = repo
	if o.wrapRepo != nil {
		r = o.wrapRepo(repo)
	}
	proc := outbox.NewProcessor(pcfg, r)
	el := outboxpg.NewLeadership(pool.Config().ConnConfig, lock, outboxpg.LeaderOptions{
		Lease: testLease, ApplicationName: "test-" + name, Alive: proc.LastIteration, StallAfter: o.stall,
	})
	proc.IsLeader, proc.Term = el.IsLeader, el.Term
	ctx, cancel := context.WithCancel(context.Background())
	p := &poller{
		name: name, pool: pool, proc: proc, election: el, cancel: cancel,
		elected: make(chan error, 1), ended: make(chan struct{}), stopped: make(chan struct{}),
	}
	go func() { p.elected <- el.Run(ctx); close(p.ended) }()
	go func() { proc.Run(ctx); close(p.stopped) }()
	t.Cleanup(p.stop)
	return p
}

func (p *poller) stop() {
	p.cancel()
	<-p.stopped
	<-p.ended
}

// overlapWatch samples every poller's IsLeader until the test ends and fails it if two ever say
// yes at once.
func overlapWatch(t *testing.T, ps ...*poller) {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	var worst atomic.Int32
	go func() {
		defer close(done)
		for ctx.Err() == nil {
			n := int32(0)
			for _, p := range ps {
				if p.election.IsLeader() {
					n++
				}
			}
			if n > worst.Load() {
				worst.Store(n)
			}
			time.Sleep(2 * time.Millisecond)
		}
	}()
	t.Cleanup(func() {
		cancel()
		<-done
		assert.LessOrEqual(t, worst.Load(), int32(1), "two pollers were leaders at once")
	})
}

func leaderOf(ps ...*poller) *poller {
	var found *poller
	for _, p := range ps {
		if p.election.IsLeader() {
			if found != nil {
				return nil
			}
			found = p
		}
	}
	return found
}

func waitLeader(t *testing.T, d time.Duration, ps ...*poller) *poller {
	t.Helper()
	var l *poller
	eventually(t, d, "one leader", func() bool { l = leaderOf(ps...); return l != nil })
	return l
}

// insertGroups writes groups x perGroup rows "<g>-<seq>" in order, ids sortable as written.
func insertGroups(t *testing.T, pool *pgxpool.Pool, prefix string, groups, perGroup int) []string {
	t.Helper()
	at := time.Now().UTC().Add(-time.Minute)
	var ids []string
	for seq := range perGroup {
		for g := range groups {
			id := fmt.Sprintf("%s%d-%03d", prefix, g, seq)
			insert(t, pool, id, fmt.Sprintf("%s%d", prefix, g), at.Add(time.Duration(seq)*time.Millisecond), 0, 0, "")
			ids = append(ids, id)
		}
	}
	return ids
}

// killLock ends the database session holding the outbox's lock (as an administrator, a failover
// or a pooler would) and returns how many it ended.
func killLock(t *testing.T, pool *pgxpool.Pool) int {
	t.Helper()
	lock, err := outboxpg.New(pool).Lock(t.Context())
	require.NoError(t, err)
	var n int
	require.NoError(t, pool.QueryRow(t.Context(), `
SELECT count(pg_terminate_backend(pid)) FROM pg_locks
 WHERE locktype = 'advisory' AND classid = $1::oid AND objid = $2::oid AND objsubid = 2 AND granted`,
		uint32(outboxpg.LockClassID), uint32(lock.ObjID)).Scan(&n)) //nolint:gosec // G115: the key's bits as pg_locks shows them
	return n
}

// Three pollers of one outbox: one leads and sends everything, the others stand by; the lock
// session holds no snapshot and no relation lock.
func TestLeadershipOnlyOneDispatches(t *testing.T) {
	pool := table(t)
	rec := newRecorder(t)
	ps := []*poller{
		startPoller(t, pool, rec, "a", pollerOpts{}), startPoller(t, pool, rec, "b", pollerOpts{}),
		startPoller(t, pool, rec, "c", pollerOpts{}),
	}
	overlapWatch(t, ps...)
	leader := waitLeader(t, 5*time.Second, ps...)
	ids := insertGroups(t, pool, "g", 4, 10)
	insert(t, pool, "loose-1", "", time.Now().UTC(), 0, 0, "")
	eventually(t, 10*time.Second, "every row delivered", func() bool { return count(t, pool) == 0 })

	senders := rec.senders()
	for _, id := range append(ids, "loose-1") {
		assert.Equal(t, []string{leader.name}, senders[id], "row %s", id)
	}
	assertGroupOrder(t, rec.all())
	for _, p := range ps {
		want := outboxpg.RoleStandby
		if p == leader {
			want = outboxpg.RoleLeader
		}
		assert.Equal(t, want, p.election.Status().Role, p.name)
	}

	var xmin *string
	var relLocks int
	eventually(t, 2*time.Second, "the lock session idle in its transaction", func() bool {
		return pool.QueryRow(t.Context(), `
SELECT a.backend_xmin::text, (SELECT count(*) FROM pg_locks l WHERE l.pid = a.pid AND l.locktype = 'relation')
  FROM pg_stat_activity a WHERE a.application_name = $1 AND a.state = 'idle in transaction'`,
			"test-"+leader.name).Scan(&xmin, &relLocks) == nil
	})
	assert.Nil(t, xmin, "the lock transaction holds no snapshot between statements")
	assert.Zero(t, relLocks, "the lock transaction holds no relation lock")
}

// The leader's lock session is ended while it is sending to a slow platform: another poller takes
// over within a lease and a half, the old leader sends nothing once the new one has started,
// and every group arrives in order.
func TestLeadershipTakeoverKeepsGroupOrder(t *testing.T) {
	pool := table(t)
	rec := newRecorder(t)
	rec.set(80*time.Millisecond, false)
	ps := []*poller{
		startPoller(t, pool, rec, "a", pollerOpts{}), startPoller(t, pool, rec, "b", pollerOpts{}),
		startPoller(t, pool, rec, "c", pollerOpts{}),
	}
	overlapWatch(t, ps...)
	first := waitLeader(t, 5*time.Second, ps...)
	ids := insertGroups(t, pool, "g", 5, 30) // each group's queue outlasts the takeover
	eventually(t, 10*time.Second, "deliveries under way", func() bool { return len(rec.all()) >= 10 })

	killedAt := time.Now()
	require.Equal(t, 1, killLock(t, pool))
	var second *poller
	eventually(t, 3*time.Second, "a new leader", func() bool {
		second = leaderOf(ps...)
		return second != nil && second != first
	})
	took := time.Since(killedAt)
	t.Logf("takeover took %v", took)
	// a check (lease/5) to notice, the grace (1.2 leases), and the poll interval
	assert.Less(t, took, 2*testLease, "takeover within the bound")
	eventually(t, 20*time.Second, "every row delivered", func() bool { return count(t, pool) == 0 })

	arrivals := rec.all()
	assertGroupOrder(t, arrivals)
	var newStart time.Time
	for _, a := range arrivals {
		if a.poller == second.name {
			newStart = a.at
			break
		}
	}
	require.False(t, newStart.IsZero())
	for _, a := range arrivals {
		if a.poller == first.name {
			assert.True(t, a.at.Before(newStart), "the old leader sent after the new leader began")
			// its last confirmation was before the kill: it sends nothing a lease after it
			assert.True(t, a.at.Before(killedAt.Add(testLease)), "the old leader sent %v after losing the lock", a.at.Sub(killedAt))
		}
	}
	senders := rec.senders()
	for _, id := range ids {
		assert.NotEmpty(t, senders[id], "row %s delivered", id)
	}
}

// A leader dies with requests in flight and rows claimed (its process is gone: nothing it holds
// is written back). The new leader leaves those groups alone until the takeover recovery, then
// delivers them in order; the other groups go on at once.
func TestLeadershipRecoversADeadLeadersClaims(t *testing.T) {
	pool := table(t)
	rec := newRecorder(t)
	a := startPoller(t, pool, rec, "a", pollerOpts{})
	waitLeader(t, 5*time.Second, a)
	rec.set(0, true) // the platform takes the requests and never answers
	insertGroups(t, pool, "s", 2, 5)
	eventually(t, 5*time.Second, "a's requests in flight", func() bool { return len(rec.all()) >= 2 })
	a.cancel() // the process dies: its requests are abandoned, its writes never happen
	<-a.stopped
	require.NoError(t, <-a.elected)
	var stuck int
	require.NoError(t, pool.QueryRow(t.Context(), `SELECT count(*) FROM outbox_messages WHERE status = 9`).Scan(&stuck))
	require.Positive(t, stuck, "a left rows claimed")

	rec.set(0, false)
	b := startPoller(t, pool, rec, "b", pollerOpts{})
	waitLeader(t, 5*time.Second, b)
	leaderAt := time.Now()
	insertGroups(t, pool, "f", 2, 3) // groups a never touched
	eventually(t, 5*time.Second, "the fresh groups delivered", func() bool {
		s := rec.senders()
		return len(s["f0-002"]) > 0 && len(s["f1-002"]) > 0
	})
	for id, who := range rec.senders() {
		if strings.HasPrefix(id, "s") {
			assert.NotContains(t, who, "b", "row %s sent before the takeover recovery", id)
		}
	}
	eventually(t, 10*time.Second, "every row delivered", func() bool { return count(t, pool) == 0 })
	var firstS time.Time
	for _, ar := range rec.all() {
		if ar.poller == "b" && strings.HasPrefix(ar.ids[0], "s") {
			firstS = ar.at
			break
		}
	}
	assert.GreaterOrEqual(t, firstS.Sub(leaderAt), 2*time.Second, "the stuck groups waited for the takeover recovery")
	assertGroupOrder(t, rec.all())
}

// blockingRepo blocks ClaimPending while blocked is set, ignoring its context: a hung poll loop.
type blockingRepo struct {
	outbox.Repository
	mu      sync.Mutex
	blocked chan struct{}
	entered chan struct{}
}

func (r *blockingRepo) block() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.blocked, r.entered = make(chan struct{}), make(chan struct{})
}

func (r *blockingRepo) unblock() {
	r.mu.Lock()
	defer r.mu.Unlock()
	close(r.blocked)
}

func (r *blockingRepo) ClaimPending(ctx context.Context, n int, f outbox.ClaimFilter) ([]outbox.Item, error) {
	r.mu.Lock()
	b, e := r.blocked, r.entered
	r.mu.Unlock()
	if b != nil {
		select {
		case <-e:
		default:
			close(e)
		}
		<-b
	}
	return r.Repository.ClaimPending(context.WithoutCancel(ctx), n, f)
}

// A leader whose poll loop hangs past the stall timeout gives the leadership up (its election
// returns ErrStalled), another takes over, and when the hung loop resumes it sends nothing.
func TestLeadershipStalledLeaderGivesUpAndSendsNothingAfter(t *testing.T) {
	pool := table(t)
	rec := newRecorder(t)
	var hung *blockingRepo
	a := startPoller(t, pool, rec, "a", pollerOpts{stall: time.Second, wrapRepo: func(r outbox.Repository) outbox.Repository {
		hung = &blockingRepo{Repository: r}
		return hung
	}})
	waitLeader(t, 5*time.Second, a)
	b := startPoller(t, pool, rec, "b", pollerOpts{})
	overlapWatch(t, a, b)
	eventually(t, 3*time.Second, "b standing by", func() bool { return b.election.Status().Role == outboxpg.RoleStandby })

	hung.block()
	<-hung.entered
	insertGroups(t, pool, "h", 2, 5)
	select {
	case err := <-a.elected:
		require.ErrorIs(t, err, outboxpg.ErrStalled)
	case <-time.After(5 * time.Second):
		t.Fatal("the stalled leader kept the leadership")
	}
	waitLeader(t, 3*time.Second, a, b)
	require.True(t, b.election.IsLeader())
	eventually(t, 10*time.Second, "b delivers everything", func() bool { return count(t, pool) == 0 })

	insertGroups(t, pool, "late", 1, 3)
	hung.unblock() // the hung loop resumes: its claim runs, after the leadership has gone
	eventually(t, 10*time.Second, "the late rows delivered", func() bool { return count(t, pool) == 0 })
	time.Sleep(200 * time.Millisecond)
	for _, ar := range rec.all() {
		if ar.poller == "a" {
			assert.Fail(t, "the stalled poller sent rows after it lost the leadership", "%v", ar.ids)
		}
	}
	assertGroupOrder(t, rec.all())
}

// proxy forwards TCP to the database and can black-hole it (hold every byte, close nothing): a
// network partition.
type proxy struct {
	ln     net.Listener
	target string
	mu     sync.Mutex
	frozen chan struct{} // non-nil: frozen until closed
}

func newProxy(t *testing.T, target string) *proxy {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	p := &proxy{ln: ln, target: target}
	t.Cleanup(func() { p.heal(); _ = ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go p.serve(c)
		}
	}()
	return p
}

func (p *proxy) serve(c net.Conn) {
	defer func() { _ = c.Close() }()
	up, err := net.Dial("tcp", p.target)
	if err != nil {
		return
	}
	defer func() { _ = up.Close() }()
	done := make(chan struct{}, 2)
	pipe := func(dst, src net.Conn) {
		defer func() { done <- struct{}{} }()
		buf := make([]byte, 32<<10)
		for {
			n, err := src.Read(buf)
			p.mu.Lock()
			f := p.frozen
			p.mu.Unlock()
			if f != nil {
				<-f
			}
			if n > 0 {
				if _, werr := dst.Write(buf[:n]); werr != nil {
					return
				}
			}
			if err != nil { // io.EOF or a closed connection
				return
			}
		}
	}
	go pipe(up, c)
	go pipe(c, up)
	<-done
}

func (p *proxy) freeze() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.frozen == nil {
		p.frozen = make(chan struct{})
	}
}

func (p *proxy) heal() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.frozen != nil {
		close(p.frozen)
		p.frozen = nil
	}
}

func (p *proxy) port() uint16 { return uint16(p.ln.Addr().(*net.TCPAddr).Port) } //nolint:gosec // G115: a TCP port

// The leader is cut off from the database (packets held, nothing closed). It stops dispatching
// within a lease by its own clock; the server ends its silent transaction after two leases and a
// standby takes over; when the network heals the old leader finds its session gone and stands by.
func TestLeadershipPartitionedLeaderStopsBeforeTheTakeover(t *testing.T) {
	pool := table(t)
	rec := newRecorder(t)
	c := pool.Config().ConnConfig
	px := newProxy(t, fmt.Sprintf("%s:%d", c.Host, c.Port))
	a := startPoller(t, pool, rec, "a", pollerOpts{connect: func(cfg *pgxpool.Config) {
		cfg.ConnConfig.Host, cfg.ConnConfig.Port = "127.0.0.1", px.port()
	}})
	waitLeader(t, 5*time.Second, a)
	b := startPoller(t, pool, rec, "b", pollerOpts{})
	overlapWatch(t, a, b)
	eventually(t, 3*time.Second, "b standing by", func() bool { return b.election.Status().Role == outboxpg.RoleStandby })

	px.freeze()
	cut := time.Now()
	eventually(t, 2*testLease, "a stops leading", func() bool { return !a.election.IsLeader() })
	aStopped := time.Since(cut)
	eventually(t, 5*time.Second, "b leads", func() bool { return b.election.IsLeader() })
	bLeads := time.Since(cut)
	t.Logf("a stopped after %v, b leads after %v", aStopped, bLeads)
	assert.LessOrEqual(t, aStopped, testLease+testLease/5)
	assert.GreaterOrEqual(t, bLeads, 2*testLease, "the server released the lock only after two leases")

	insertGroups(t, pool, "p", 2, 3)
	eventually(t, 10*time.Second, "b delivers", func() bool { return count(t, pool) == 0 })
	px.heal()
	eventually(t, 10*time.Second, "a stands by", func() bool { return a.election.Status().Role == outboxpg.RoleStandby })
	for _, ar := range rec.all() {
		assert.NotEqual(t, "a", ar.poller)
	}
}

// Two outboxes in two schemas of one database each elect their own leader.
func TestLeadershipIsPerOutbox(t *testing.T) {
	one := table(t)
	two := func() *pgxpool.Pool {
		cfg := one.Config().Copy()
		schema := cfg.ConnConfig.RuntimeParams["search_path"] + "_two"
		_, err := one.Exec(t.Context(), `DROP SCHEMA IF EXISTS `+schema+` CASCADE; CREATE SCHEMA `+schema)
		require.NoError(t, err)
		cfg.ConnConfig.RuntimeParams["search_path"] = schema
		p, err := pgxpool.NewWithConfig(t.Context(), cfg)
		require.NoError(t, err)
		t.Cleanup(p.Close)
		require.NoError(t, outboxpg.New(p).InitSchema(t.Context()))
		return p
	}()
	k1, err := outboxpg.New(one).Lock(t.Context())
	require.NoError(t, err)
	k2, err := outboxpg.New(two).Lock(t.Context())
	require.NoError(t, err)
	assert.NotEqual(t, k1.ObjID, k2.ObjID)
	assert.Equal(t, k1.Database, k2.Database)

	rec := newRecorder(t)
	a := startPoller(t, one, rec, "a", pollerOpts{})
	b := startPoller(t, two, rec, "b", pollerOpts{})
	waitLeader(t, 5*time.Second, a)
	waitLeader(t, 5*time.Second, b)
}
