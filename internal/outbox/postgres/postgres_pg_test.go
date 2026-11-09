//go:build integration

package postgres_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/common"
	"github.com/flowcatalyst/flowcatalyst-go/internal/outbox"
	outboxpg "github.com/flowcatalyst/flowcatalyst-go/internal/outbox/postgres"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
)

func TestMain(m *testing.M) { testpg.RunMain(m) }

// table gives the test an outbox_messages table of its own: a schema named after the test, and a
// pool whose search_path is that schema (the repository's SQL names the table unqualified).
func table(t *testing.T) *pgxpool.Pool {
	t.Helper()
	shared := testpg.Pool(t)
	schema := "ob_" + strings.ToLower(strings.NewReplacer("/", "_", "=", "_", "-", "_").Replace(t.Name()))
	schema = schema[:min(len(schema), 60)]
	_, err := shared.Exec(t.Context(), `DROP SCHEMA IF EXISTS `+schema+` CASCADE; CREATE SCHEMA `+schema)
	require.NoError(t, err)
	cfg := shared.Config().Copy()
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(t.Context(), cfg)
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	require.NoError(t, outboxpg.New(pool).InitSchema(t.Context()))
	return pool
}

// insert writes an outbox row as the SDK does, with a payload carrying its id (as pushId) so the
// platform stand-in can tell rows apart, created at created.
func insert(t *testing.T, pool *pgxpool.Pool, id, group string, created time.Time, status, retries int, msg string) {
	t.Helper()
	var g, m *string
	if group != "" {
		g = &group
	}
	if msg != "" {
		m = &msg
	}
	_, err := pool.Exec(t.Context(), `
INSERT INTO outbox_messages (id, type, message_group, payload, status, retry_count, error_message, created_at, updated_at)
VALUES ($1, 'PUSH', $2, $3, $4, $5, $6, $7, $7)`,
		id, g, fmt.Sprintf(`{"topic":"shop:a","type":"x","data":{},"pushId":%q}`, id), status, retries, m, created)
	require.NoError(t, err)
}

type stored struct {
	status  int
	retries int
	msg     string
}

func row(t *testing.T, pool *pgxpool.Pool, id string) (stored, bool) {
	t.Helper()
	var r stored
	var msg *string
	err := pool.QueryRow(t.Context(), `SELECT status, retry_count, error_message FROM outbox_messages WHERE id = $1`, id).
		Scan(&r.status, &r.retries, &msg)
	if err != nil {
		return stored{}, false
	}
	if msg != nil {
		r.msg = *msg
	}
	return r, true
}

func count(t *testing.T, pool *pgxpool.Pool) int {
	t.Helper()
	var n int
	require.NoError(t, pool.QueryRow(t.Context(), `SELECT count(*) FROM outbox_messages`).Scan(&n))
	return n
}

// platform is a stand-in for the batch ingest endpoint: answer decides each request (nil: every
// item SUCCESS), and it records every delivered id in order and every attempt per id.
type platform struct {
	mu        sync.Mutex
	answer    func(ids []string) (status int, results []string)
	delivered []string
	attempts  map[string]int
	srv       *httptest.Server
}

func newPlatform(t *testing.T) *platform {
	p := &platform{attempts: map[string]int{}}
	p.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Items []struct {
				PushID string `json:"pushId"`
			} `json:"items"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		ids := make([]string, len(body.Items))
		for i, it := range body.Items {
			ids[i] = it.PushID
		}
		p.mu.Lock()
		defer p.mu.Unlock()
		for _, id := range ids {
			p.attempts[id]++
		}
		status, results := http.StatusOK, []string(nil)
		if p.answer != nil {
			status, results = p.answer(ids)
		}
		if status != http.StatusOK {
			w.WriteHeader(status)
			return
		}
		if results == nil {
			results = make([]string, len(ids))
			for i := range results {
				results[i] = "SUCCESS"
			}
		}
		out := make([]map[string]string, len(ids))
		for i, id := range ids {
			out[i] = map[string]string{"status": results[i]}
			if results[i] == "SUCCESS" {
				p.delivered = append(p.delivered, id)
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"results": out})
	}))
	t.Cleanup(p.srv.Close)
	return p
}

func (p *platform) setAnswer(f func(ids []string) (int, []string)) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.answer = f
}

func (p *platform) deliveredIDs() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return slices.Clone(p.delivered)
}

func (p *platform) attemptsOf(id string) int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.attempts[id]
}

// run starts a processor against pool and plat; the returned stop ends it and waits.
func run(t *testing.T, pool *pgxpool.Pool, plat *platform, tune func(*outbox.Config)) (*outbox.Processor, func()) {
	t.Helper()
	cfg := outbox.DefaultConfig()
	cfg.PlatformURL = plat.srv.URL
	cfg.PollInterval = 20 * time.Millisecond
	cfg.RetryBackoffBase = 50 * time.Millisecond
	cfg.RetryBackoffMax = 400 * time.Millisecond
	if tune != nil {
		tune(&cfg)
	}
	p := outbox.NewProcessor(cfg, outboxpg.New(pool))
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { p.Run(ctx); close(done) }()
	var once sync.Once
	stop := func() { once.Do(func() { cancel(); <-done }) }
	t.Cleanup(stop)
	return p, stop
}

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

// The platform answers 503 for a few seconds, then 200: every row is delivered, each group in the
// order written, though the outage outlasted the old retry cap many times over.
func TestOutageThenRecoveryDeliversEveryRowInOrder(t *testing.T) {
	pool := table(t)
	at := time.Now().UTC().Add(-time.Minute)
	written := map[string][]string{}
	total := 0
	for _, g := range []string{"a", "b", "c", "d"} {
		for i := range 12 {
			id := fmt.Sprintf("%s-%02d", g, i+1)
			insert(t, pool, id, "grp-"+g, at.Add(time.Duration(i)*time.Millisecond), 0, 0, "")
			written["grp-"+g] = append(written["grp-"+g], id)
			total++
		}
	}
	for i := range 9 {
		insert(t, pool, fmt.Sprintf("u-%02d", i+1), "", at.Add(time.Duration(i)*time.Millisecond), 0, 0, "")
		total++
	}

	plat := newPlatform(t)
	outageEnds := time.Now().Add(3 * time.Second)
	// after the outage, single rows still fail retryably a few times (per-item INTERNAL_ERROR, or
	// the whole request): their group's later rows must wait for them
	failuresLeft := map[string]int{"b-05": 3, "c-01": 2, "d-11": 2, "u-04": 2}
	plat.setAnswer(func(ids []string) (int, []string) {
		if time.Now().Before(outageEnds) {
			return http.StatusServiceUnavailable, nil
		}
		res := make([]string, len(ids))
		for i, id := range ids {
			res[i] = "SUCCESS"
			if failuresLeft[id] > 0 {
				failuresLeft[id]--
				if id == "c-01" {
					return http.StatusBadGateway, nil
				}
				res[i] = "INTERNAL_ERROR"
			}
		}
		return http.StatusOK, res
	})
	p, stop := run(t, pool, plat, func(c *outbox.Config) { c.BatchSize = 10 })

	// during the outage: rows are put back to PENDING with their attempts recorded, never failed
	eventually(t, 5*time.Second, "an attempt recorded during the outage", func() bool {
		r, ok := row(t, pool, "a-01")
		return ok && r.retries > 0
	})
	var terminal int
	require.NoError(t, pool.QueryRow(t.Context(), `SELECT count(*) FROM outbox_messages WHERE status NOT IN (0, 9)`).Scan(&terminal))
	assert.Zero(t, terminal, "no row is left in a failure status during the outage")

	eventually(t, 20*time.Second, "every row delivered", func() bool { return count(t, pool) == 0 })
	stop()

	got := plat.deliveredIDs()
	require.Len(t, got, total, "each row delivered exactly once")
	for g, want := range written {
		var seq []string
		for _, id := range got {
			if "grp-"+id[:1] == g {
				seq = append(seq, id)
			}
		}
		assert.Equal(t, want, seq, "group %s in written order", g)
	}
	assert.Greater(t, plat.attemptsOf("a-01"), 3, "the head row outlived the old 3-attempt cap")
	assert.Greater(t, p.Retried(), uint64(0))
	_, failed := p.Totals()
	assert.Zero(t, failed)
}

// A non-retryable failure blocks its group, and a processor started afterwards (a restart) keeps
// it blocked from the stored row; the operator's skip is stored, so the next restart does not
// block on the row again.
func TestNonRetryableFailureBlocksItsGroupAcrossARestart(t *testing.T) {
	pool := table(t)
	at := time.Now().UTC().Add(-time.Minute)
	for i := range 4 {
		insert(t, pool, fmt.Sprintf("p-%02d", i+1), "grp-p", at.Add(time.Duration(i)*time.Millisecond), 0, 0, "")
		insert(t, pool, fmt.Sprintf("q-%02d", i+1), "grp-q", at.Add(time.Duration(i)*time.Millisecond), 0, 0, "")
	}
	plat := newPlatform(t)
	plat.setAnswer(func(ids []string) (int, []string) {
		res := make([]string, len(ids))
		for i, id := range ids {
			res[i] = "SUCCESS"
			if id == "p-02" {
				res[i] = "BAD_REQUEST"
			}
		}
		return http.StatusOK, res
	})

	first, stop := run(t, pool, plat, nil)
	eventually(t, 10*time.Second, "grp-q delivered and p-02 failed", func() bool {
		r, ok := row(t, pool, "p-02")
		_, q4 := row(t, pool, "q-04")
		return !q4 && ok && r.status == common.OutboxBadRequest.Code()
	})
	require.Len(t, first.BlockedGroups(), 1)
	stop()
	r, _ := row(t, pool, "p-02")
	assert.Equal(t, outbox.BlockingMarker, r.msg, "a refusal by this version is stored behind the blocking marker")

	// the restart: a fresh processor, nothing in memory
	second, stop := run(t, pool, plat, nil)
	eventually(t, 5*time.Second, "the restarted processor blocking grp-p", func() bool {
		return len(second.BlockedGroups()) == 1
	})
	time.Sleep(300 * time.Millisecond) // a dozen polls
	b := second.BlockedGroups()
	require.Len(t, b, 1)
	assert.Equal(t, "grp-p", b[0].Group)
	assert.Equal(t, "p-02", b[0].BlockedItemID)
	for _, id := range []string{"p-03", "p-04"} {
		r, ok := row(t, pool, id)
		require.True(t, ok, "%s was delivered past the blocked row", id)
		assert.Equal(t, 0, r.status, "%s waits PENDING behind the blocked row", id)
		assert.Zero(t, plat.attemptsOf(id), "%s was sent", id)
	}

	// the operator skips the row: the group carries on, the row stays failed, marked
	ok, err := second.SkipGroup(t.Context(), "grp-p")
	require.NoError(t, err)
	require.True(t, ok)
	eventually(t, 5*time.Second, "grp-p's later rows delivered", func() bool {
		_, p3 := row(t, pool, "p-03")
		_, p4 := row(t, pool, "p-04")
		return !p3 && !p4
	})
	stop()
	r, ok = row(t, pool, "p-02")
	require.True(t, ok)
	assert.Equal(t, common.OutboxBadRequest.Code(), r.status)
	assert.Equal(t, outbox.SkippedMarker, r.msg, "the skip marker replaces the blocking marker")

	// a later restart does not block the group on the skipped row
	insert(t, pool, "p-05", "grp-p", time.Now().UTC(), 0, 0, "")
	third, stop := run(t, pool, plat, nil)
	eventually(t, 5*time.Second, "p-05 delivered", func() bool { _, ok := row(t, pool, "p-05"); return !ok })
	assert.Empty(t, third.BlockedGroups())
	stop()
}

// An unblock re-queues the stored poison row, and the group runs again in order from it.
func TestUnblockAfterARestartRetriesTheRowFirst(t *testing.T) {
	pool := table(t)
	at := time.Now().UTC().Add(-time.Minute)
	insert(t, pool, "r-01", "grp-r", at, int(common.OutboxForbidden), 1, outbox.BlockingMarker+"403")
	insert(t, pool, "r-02", "grp-r", at.Add(time.Millisecond), 0, 0, "")
	plat := newPlatform(t)
	p, stop := run(t, pool, plat, nil)
	eventually(t, 5*time.Second, "grp-r blocked from the stored row", func() bool { return len(p.BlockedGroups()) == 1 })
	ok, err := p.UnblockGroup(t.Context(), "grp-r")
	require.NoError(t, err)
	require.True(t, ok)
	eventually(t, 5*time.Second, "grp-r delivered", func() bool { return count(t, pool) == 0 })
	stop()
	assert.Equal(t, []string{"r-01", "r-02"}, plat.deliveredIDs())
}

// Rows an older version left in a retryable failure status (its retry cap used up) are returned
// to PENDING at start and delivered; a non-retryable failure without a group stays as it was.
func TestLegacyRetryableFailedRowsAreSweptBack(t *testing.T) {
	pool := table(t)
	at := time.Now().UTC().Add(-time.Hour)
	insert(t, pool, "l-01", "grp-l", at, int(common.OutboxInternalError), 3, "500")
	insert(t, pool, "l-02", "grp-l", at.Add(time.Millisecond), int(common.OutboxGatewayError), 3, "503")
	insert(t, pool, "l-03", "", at, int(common.OutboxUnauthorized), 3, "401")
	insert(t, pool, "l-04", "", at, int(common.OutboxBadRequest), 1, "400")

	n, err := outboxpg.New(pool).RequeueRetryableFailures(t.Context())
	require.NoError(t, err)
	assert.Equal(t, 3, n)
	for _, id := range []string{"l-01", "l-02", "l-03"} {
		r, _ := row(t, pool, id)
		assert.Equal(t, stored{status: 0, retries: 3, msg: r.msg}, r, "%s back to PENDING, its attempts kept", id)
	}
	_, err = pool.Exec(t.Context(), `UPDATE outbox_messages SET status = $1 WHERE id = 'l-02'`, int(common.OutboxGatewayError))
	require.NoError(t, err)
	_, err = pool.Exec(t.Context(), `UPDATE outbox_messages SET status = $1 WHERE id IN ('l-01', 'l-03')`, int(common.OutboxInternalError))
	require.NoError(t, err)

	plat := newPlatform(t)
	_, stop := run(t, pool, plat, nil)
	eventually(t, 5*time.Second, "the swept rows delivered", func() bool { return count(t, pool) == 1 })
	stop()
	got := plat.deliveredIDs()
	assert.ElementsMatch(t, []string{"l-01", "l-02", "l-03"}, got)
	assert.Less(t, slices.Index(got, "l-01"), slices.Index(got, "l-02"), "grp-l in order")
	r, ok := row(t, pool, "l-04")
	require.True(t, ok)
	assert.Equal(t, int(common.OutboxBadRequest), r.status, "a non-retryable failure is not swept")
	assert.Equal(t, outbox.SkippedBeforeUpgradeMarker+"400", r.msg)
}

// A row an earlier version refused (no marker) does not block its group on first start: it is
// marked skipped before upgrade, once, and the group's later rows flow. A row this version refused
// (the blocking marker) still blocks its group.
func TestLegacyRefusalIsSkippedBeforeUpgrade(t *testing.T) {
	pool := table(t)
	repo := outboxpg.New(pool)
	at := time.Now().UTC().Add(-time.Hour)
	insert(t, pool, "o-01", "grp-o", at, int(common.OutboxBadRequest), 1, "400 old")
	insert(t, pool, "o-02", "grp-o", at.Add(time.Millisecond), 0, 0, "")
	insert(t, pool, "f-01", "grp-f", at, int(common.OutboxForbidden), 1, "")
	insert(t, pool, "f-02", "grp-f", at.Add(time.Millisecond), 0, 0, "")
	insert(t, pool, "n-01", "grp-n", at, int(common.OutboxBadRequest), 1, outbox.BlockingMarker+"400 new")
	insert(t, pool, "n-02", "grp-n", at.Add(time.Millisecond), 0, 0, "")
	insert(t, pool, "s-01", "grp-s", at, int(common.OutboxBadRequest), 1, outbox.SkippedMarker+"400")

	plat := newPlatform(t)
	p, stop := run(t, pool, plat, nil)
	eventually(t, 5*time.Second, "the legacy groups' later rows delivered", func() bool {
		_, o2 := row(t, pool, "o-02")
		_, f2 := row(t, pool, "f-02")
		return !o2 && !f2
	})
	time.Sleep(200 * time.Millisecond)
	stop()

	r, _ := row(t, pool, "o-01")
	assert.Equal(t, stored{status: int(common.OutboxBadRequest), retries: 1, msg: outbox.SkippedBeforeUpgradeMarker + "400 old"}, r)
	r, _ = row(t, pool, "f-01")
	assert.Equal(t, outbox.SkippedBeforeUpgradeMarker, r.msg)
	r, _ = row(t, pool, "s-01")
	assert.Equal(t, outbox.SkippedMarker+"400", r.msg, "an operator skip is left as it is")
	b := p.BlockedGroups()
	require.Len(t, b, 1)
	assert.Equal(t, "grp-n", b[0].Group)
	assert.Equal(t, "400 new", b[0].Error, "shown without the marker")
	_, n2 := row(t, pool, "n-02")
	assert.True(t, n2, "grp-n waits behind this version's refusal")

	n, err := repo.MarkLegacyRefusals(t.Context())
	require.NoError(t, err)
	assert.Zero(t, n, "marked once")
}

// The claim passes over held and blocked groups, the ungrouped rows of a held type, and held rows,
// so a held group with a backlog of a batch or more cannot starve the rest.
func TestClaimPassesOverTheFilter(t *testing.T) {
	pool := table(t)
	repo := outboxpg.New(pool)
	at := time.Now().UTC().Add(-time.Minute)
	for i := range 5 {
		insert(t, pool, fmt.Sprintf("a-%02d", i+1), "aaa", at.Add(time.Duration(i)*time.Millisecond), 0, 0, "")
	}
	insert(t, pool, "b-01", "bbb", at, 0, 0, "")
	insert(t, pool, "u-01", "", at, 0, 0, "")
	insert(t, pool, "u-02", "", at.Add(time.Millisecond), 0, 0, "")

	ids := func(items []outbox.Item) []string {
		out := make([]string, len(items))
		for i, it := range items {
			out[i] = it.ID
		}
		slices.Sort(out)
		return out
	}
	release := func(items []outbox.Item) {
		require.NoError(t, repo.Release(t.Context(), func() []string {
			out := make([]string, len(items))
			for i, it := range items {
				out[i] = it.ID
			}
			return out
		}()))
	}

	// unfiltered, the backlog of aaa fills a batch of 5
	got, err := repo.ClaimPending(t.Context(), 5, outbox.ClaimFilter{})
	require.NoError(t, err)
	assert.Equal(t, []string{"a-01", "a-02", "a-03", "a-04", "a-05"}, ids(got))
	release(got)

	// an ungrouped EVENT row whose message_group is the empty string, not NULL
	_, err = pool.Exec(t.Context(), `INSERT INTO outbox_messages (id, type, message_group, payload) VALUES ('e-01', 'EVENT', '', '{}')`)
	require.NoError(t, err)
	got, err = repo.ClaimPending(t.Context(), 5, outbox.ClaimFilter{
		Groups: []string{"aaa"}, Types: []common.OutboxItemType{common.OutboxItemPush}, IDs: []string{"e-01"},
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"b-01"}, ids(got), "aaa held, ungrouped PUSH held, e-01 held")
	release(got)

	got, err = repo.ClaimPending(t.Context(), 10, outbox.ClaimFilter{Groups: []string{"aaa", "bbb"}, IDs: []string{"u-01"}})
	require.NoError(t, err)
	assert.Equal(t, []string{"e-01", "u-02"}, ids(got), "an empty-string group counts as ungrouped")
}

// The blocking failure of a group is its oldest row carrying the blocking marker; the skip marker
// replaces the blocking marker, once.
func TestBlockingFailuresAndSkipMarker(t *testing.T) {
	pool := table(t)
	repo := outboxpg.New(pool)
	at := time.Now().UTC().Add(-time.Minute)
	insert(t, pool, "x-01", "gx", at, int(common.OutboxBadRequest), 1, outbox.BlockingMarker+"first")
	insert(t, pool, "x-02", "gx", at.Add(time.Millisecond), int(common.OutboxForbidden), 1, outbox.BlockingMarker+"second")
	insert(t, pool, "y-01", "gy", at, int(common.OutboxBadRequest), 1, outbox.BlockingMarker)
	insert(t, pool, "v-01", "gv", at, int(common.OutboxBadRequest), 1, "unmarked: an earlier version's")
	insert(t, pool, "z-01", "", at, int(common.OutboxBadRequest), 1, outbox.BlockingMarker+"ungrouped")
	insert(t, pool, "w-01", "gw", at, int(common.OutboxInternalError), 1, "retryable")

	heads, err := repo.BlockingFailures(t.Context())
	require.NoError(t, err)
	got := map[string]string{}
	for _, h := range heads {
		got[*h.MessageGroup] = h.ID
	}
	assert.Equal(t, map[string]string{"gx": "x-01", "gy": "y-01"}, got)

	require.NoError(t, repo.MarkSkipped(t.Context(), []string{"x-01", "y-01"}))
	require.NoError(t, repo.MarkSkipped(t.Context(), []string{"x-01"}))
	r, _ := row(t, pool, "x-01")
	assert.Equal(t, outbox.SkippedMarker+"first", r.msg, "marked once, in place of the blocking marker")
	assert.Equal(t, int(common.OutboxBadRequest), r.status)
	r, _ = row(t, pool, "y-01")
	assert.Equal(t, outbox.SkippedMarker, r.msg)

	heads, err = repo.BlockingFailures(t.Context())
	require.NoError(t, err)
	require.Len(t, heads, 1)
	assert.Equal(t, "x-02", heads[0].ID, "gx now blocks on its next refusal")
}

// retry_count stops at the SDK column's SMALLINT ceiling instead of failing the update.
func TestRetryCountStopsAtTheColumnCeiling(t *testing.T) {
	pool := table(t)
	insert(t, pool, "c-01", "g", time.Now().UTC(), 9, 32767, "")
	require.NoError(t, outboxpg.New(pool).MarkFailed(t.Context(), []string{"c-01"}, common.OutboxGatewayError, "503", true))
	r, _ := row(t, pool, "c-01")
	assert.Equal(t, stored{status: 0, retries: 32767, msg: "503"}, r)
}
