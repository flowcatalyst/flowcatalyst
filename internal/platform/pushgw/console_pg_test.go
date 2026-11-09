//go:build integration

package pushgw

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
)

func subscribeToken(t *testing.T, w *world, n *node, client string, topics ...string) string {
	t.Helper()
	tok, _ := w.bearer(who{scope: principal.ScopeClient, clients: []string{client}, perms: []string{"alpha:order:view"}})
	res := n.subscribe(map[string]any{"topics": topics}, bearerHeader(tok))
	require.Equal(t, 200, res.status, string(res.body))
	token, _ := res.json()["streamToken"].(string)
	return token
}

// Every task writes a stats row; the console's sum of the fresh rows counts the open streams per
// application and per client, the pushes the relay logged and their lag, and forgets a task that
// stopped writing.
func TestStatsRowsFromTwoTasks(t *testing.T) {
	old := StatsEvery
	StatsEvery = 100 * time.Millisecond
	t.Cleanup(func() { StatsEvery = old })

	w := newWorld(t, "alpha", "beta")
	w.rule("alpha", "alpha:order:{id}", false, "alpha:order:view")
	a, b := w.start(), w.start()
	repo := &StatsRepo{Pool: w.pool}
	live := func() Live {
		l, err := repo.Live(t.Context(), time.Now())
		require.NoError(t, err)
		return l
	}
	eventually(t, "both tasks to report", 5*time.Second, func() bool { return live().Tasks == 2 })
	assert.Equal(t, "postgres", live().Driver)

	// a stream on one task, another on the other
	s1 := a.listen(subscribeToken(t, w, a, "clt_acme", "alpha:order:1"), "")
	s2 := b.listen(subscribeToken(t, w, b, "clt_other", "alpha:order:2"), "")
	s1.greeting()
	s2.greeting()
	eventually(t, "the open streams to be counted", 5*time.Second, func() bool {
		l := live()
		return l.Open == 2 && l.ByApplication["alpha"] == 2 && l.ByClient["clt_acme"] == 1 && l.ByClient["clt_other"] == 1
	})

	// pushes: the relay's counters and its lag
	for i := range 3 {
		publishTo(t, a, "clt_acme", "alpha:order:1", "order.updated", map[string]any{"i": i})
	}
	eventually(t, "the pushes and their lag to be counted", 5*time.Second, func() bool {
		l := live()
		_, ok := l.Lag.Quantile(0.5)
		return l.Published >= 3 && ok
	})

	// closing a stream takes it out of the count
	s1.cancel()
	eventually(t, "the closed stream to leave the count", 5*time.Second, func() bool {
		l := live()
		return l.Open == 1 && l.ByClient["clt_acme"] == 0
	})

	// a row that stopped being written does not count, and is pruned later
	_, err := w.pool.Exec(t.Context(), `INSERT INTO push_gateway_stats (task_id, at, stats) VALUES ('dead-task', $1, $2)`,
		time.Now().Add(-time.Minute), `{"driver":"postgres","open":99,"byApplication":{"alpha":99}}`)
	require.NoError(t, err)
	assert.Equal(t, 2, live().Tasks)
	assert.Less(t, live().Open, 99)
	n, err := PruneStats(t.Context(), w.pool, time.Now().Add(10*time.Minute))
	require.NoError(t, err)
	assert.GreaterOrEqual(t, n, int64(1))
}

func TestConfiguredApplicationsAndTheirSyncs(t *testing.T) {
	w := newWorld(t, "alpha", "beta", "gamma")
	w.rule("alpha", "alpha:order:{id}", false, "alpha:order:view")
	w.rule("alpha", "alpha:news", true)
	w.ticketKey("alpha", "k1", map[string]string{"kty": "OKP", "crv": "Ed25519", "x": "abc"})
	repo := NewConfigRepo(w.pool)
	require.NoError(t, repo.SetSettings(t.Context(), w.apps["beta"], AppSettings{Paused: true, MaxStreams: new(5), PublishRate: new(10)}, "prn_op"))

	apps, err := repo.ListConfigured(t.Context())
	require.NoError(t, err)
	require.Len(t, apps, 2, "gamma has no push configuration")
	assert.Equal(t, "alpha", apps[0].Code)
	assert.Equal(t, 2, apps[0].Rules)
	assert.Equal(t, 1, apps[0].Keys)
	assert.NotNil(t, apps[0].SyncedAt)
	assert.False(t, apps[0].Paused)
	assert.Equal(t, 0, apps[0].Overrides)
	assert.Equal(t, "beta", apps[1].Code)
	assert.True(t, apps[1].Paused)
	assert.Equal(t, 2, apps[1].Overrides)
	assert.Nil(t, apps[1].SyncedAt, "never synced")

	rules, err := repo.RuleDetails(t.Context(), w.apps["alpha"])
	require.NoError(t, err)
	require.Len(t, rules, 2)
	assert.Equal(t, "alpha:news", rules[0].Pattern)
	assert.True(t, rules[0].Public)
	assert.Equal(t, []string{"alpha:order:view"}, rules[1].Requires)
	assert.False(t, rules[1].SyncedAt.IsZero())
	keys, err := repo.KeyDetails(t.Context(), w.apps["alpha"])
	require.NoError(t, err)
	require.Len(t, keys, 1)
	assert.Equal(t, KeyDetail{Kid: "k1", Alg: "Ed25519 OKP", SyncedAt: keys[0].SyncedAt}, keys[0])
}

func TestPGLogRecent(t *testing.T) {
	w := newWorld(t, "alpha", "beta")
	n := w.start()
	publishTo(t, n, "clt_acme", "alpha:order:1", "order.updated", map[string]any{"v": 1})
	publishTo(t, n, "clt_other", "alpha:order:2", "order.created", map[string]any{"v": 2})
	publishTo(t, n, "_platform", "alpha:news", "news.posted", map[string]any{"v": 3})
	publishTo(t, n, "clt_acme", "beta:thing:1", "thing.moved", map[string]any{"v": 4})
	log := PGLog{Pool: w.pool}
	var all []LogRow
	eventually(t, "the relay to log the pushes", 5*time.Second, func() bool {
		var err error
		all, err = log.Recent(t.Context(), LogQuery{Application: "alpha"})
		require.NoError(t, err)
		return len(all) == 3
	})
	assert.Equal(t, []string{"news.posted", "order.created", "order.updated"}, []string{all[0].Type, all[1].Type, all[2].Type}, "newest first")
	assert.Equal(t, "", all[0].Client, "the platform client is empty")
	assert.Equal(t, "clt_other", all[1].Client)
	assert.JSONEq(t, `{"v":2}`, all[1].Data)

	q := func(f func(*LogQuery)) []LogRow {
		lq := LogQuery{Application: "alpha"}
		f(&lq)
		rows, err := log.Recent(t.Context(), lq)
		require.NoError(t, err)
		return rows
	}
	assert.Len(t, q(func(l *LogQuery) { l.Client = "clt_acme" }), 1)
	assert.Len(t, q(func(l *LogQuery) { l.Client = "_platform" }), 1, "the platform client's pushes")
	assert.Len(t, q(func(l *LogQuery) { l.Type = "order.created" }), 1)
	assert.Len(t, q(func(l *LogQuery) { l.Topic = "order" }), 2)
	assert.Len(t, q(func(l *LogQuery) { l.Topic = "%" }), 0, "LIKE characters are literal")
	paged := q(func(l *LogQuery) { l.Limit = 1; l.BeforeID = all[0].ID })
	require.Len(t, paged, 1)
	assert.Equal(t, all[1].ID, paged[0].ID, "keyset paging by id")
	assert.Len(t, q(func(l *LogQuery) { l.Application = "alph" }), 0, "an application prefix is not the application")
	beta, err := log.Recent(t.Context(), LogQuery{Application: "beta"})
	require.NoError(t, err)
	require.Len(t, beta, 1)
	assert.Equal(t, "thing.moved", beta[0].Type)
}

func TestTraceWindows(t *testing.T) {
	w := newWorld(t, "alpha")
	repo := &TraceRepo{Pool: w.pool}
	now := time.Now().UTC().Truncate(time.Millisecond)
	got, err := repo.Get(t.Context(), w.apps["alpha"])
	require.NoError(t, err)
	assert.Nil(t, got, "never switched on")
	assert.False(t, got.Live(now))

	_, err = repo.Enable(t.Context(), w.apps["alpha"], 7, "prn_op", now)
	require.Error(t, err, "only 60, 300 and 900 seconds")
	win, err := repo.Enable(t.Context(), w.apps["alpha"], 300, "prn_op", now)
	require.NoError(t, err)
	assert.True(t, win.Live(now))
	got, err = repo.Get(t.Context(), w.apps["alpha"])
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.True(t, got.Live(now.Add(299*time.Second)))
	assert.False(t, got.Live(now.Add(301*time.Second)), "it ends by itself")
	assert.Equal(t, 300, got.WindowSecs)

	was, err := repo.Disable(t.Context(), w.apps["alpha"], now.Add(time.Second))
	require.NoError(t, err)
	assert.True(t, was)
	got, _ = repo.Get(t.Context(), w.apps["alpha"])
	assert.False(t, got.Live(now.Add(2*time.Second)))
	was, err = repo.Disable(t.Context(), w.apps["alpha"], now.Add(2*time.Second))
	require.NoError(t, err)
	assert.False(t, was, "already off")
}

// The test send goes through the module's publish path: a stream of the client hears it, and a
// platform push reaches every client.
func TestPublishTestIsHeardOnAStream(t *testing.T) {
	w := newWorld(t, "alpha")
	w.rule("alpha", "alpha:order:{id}", false, "alpha:order:view")
	n := w.start()
	s := n.listen(subscribeToken(t, w, n, "clt_acme", "alpha:order:1"), "")
	s.greeting()

	require.NoError(t, n.gw.PublishTest(t.Context(), "alpha:order:1", "clt_other"))
	require.NoError(t, n.gw.PublishTest(t.Context(), "alpha:order:1", "clt_acme"))
	f := s.nextEvent() // the other client's push was not heard
	assert.Contains(t, f, "event: fw-test.ping")
	assert.Contains(t, f, `"data":{"test":true}`)
	require.NoError(t, n.gw.PublishTest(t.Context(), "alpha:order:1", "_platform"))
	assert.Contains(t, s.nextEvent(), "event: fw-test.ping")

	assert.Error(t, n.gw.PublishTest(t.Context(), "Bad Topic", "clt_acme"))
	assert.Error(t, n.gw.PublishTest(t.Context(), "alpha:order:1", "not a client"))
}
