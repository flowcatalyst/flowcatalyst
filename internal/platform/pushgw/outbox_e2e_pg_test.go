//go:build integration

package pushgw

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/go-sdk/outbox"
	"github.com/flowcatalyst/go-sdk/outboxpgx"

	platformoutbox "github.com/flowcatalyst/flowcatalyst-go/internal/outbox"
	outboxpg "github.com/flowcatalyst/flowcatalyst-go/internal/outbox/postgres"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
)

// startProcessor runs the platform's outbox processor on the world's database (the outbox table is
// in it), delivering to the node's ingest endpoint with an anchor's token. The poll interval is
// long: what delivers a push is the wake-up, not the poll.
func (w *world) startProcessor(n *node, poll time.Duration) *platformoutbox.Processor {
	w.t.Helper()
	w.outboxSchema()
	tok, _ := w.bearer(who{scope: principal.ScopeAnchor, perms: []string{PermIngest}})
	cfg := platformoutbox.DefaultConfig()
	cfg.PlatformURL = n.srv.URL
	cfg.AuthToken = tok
	cfg.PollInterval = poll
	cfg.BatchSize = 100
	p := platformoutbox.NewProcessor(cfg, outboxpg.New(w.pool))
	ctx, cancel := context.WithCancel(w.t.Context())
	done := make(chan struct{})
	go func() { p.Run(ctx); close(done) }()
	w.t.Cleanup(func() { cancel(); <-done })
	return p
}

// outboxSchema creates the application's outbox table (the SDK's migration) in the world's database.
func (w *world) outboxSchema() {
	w.t.Helper()
	require.NoError(w.t, outboxpg.New(w.pool).InitSchema(w.t.Context()))
}

// writePush writes a PUSH row through the Go SDK in a transaction of its own and commits it.
func (w *world) writePush(sink *outboxpgx.Sink, p outbox.Push) {
	w.t.Helper()
	w.outboxSchema()
	tx, err := w.pool.Begin(w.t.Context())
	require.NoError(w.t, err)
	require.NoError(w.t, sink.WritePush(w.t.Context(), tx, p))
	require.NoError(w.t, tx.Commit(w.t.Context()))
}

// outboxRows counts the PUSH rows still in the outbox, and those that are untouched: no attempt
// spent (retry_count 0) and not failed (PENDING, or claimed for a moment by a poll).
func (w *world) outboxRows() (untouched, total int) {
	w.t.Helper()
	w.outboxSchema()
	require.NoError(w.t, w.pool.QueryRow(w.t.Context(),
		`SELECT count(*) FILTER (WHERE status IN (0, 9) AND retry_count = 0 AND error_message IS NULL), count(*)
		   FROM outbox_messages WHERE type = 'PUSH'`).Scan(&untouched, &total))
	return
}

// The whole path: the SDK writes a PUSH row, the platform's processor wakes on it, posts it to the
// gateway's ingest, the relay logs it and a listening stream hears it. The processor polls every
// 30 seconds, so the latency measured is the wake-up's. The first pushes warm the connections and
// are not counted.
func TestOutboxPushEndToEndWithInstantWakeUp(t *testing.T) {
	w := newWorld(t, "shop")
	w.clientRow("clt_a", "acme")
	w.rule("shop", "shop:orders:{id}", true)
	n := w.start()
	w.startProcessor(n, 30*time.Second)
	sink := outboxpgx.NewSink(outboxpgx.Config{ClientCode: "acme", ApplicationCode: "shop"})

	tok, _ := w.bearer(who{scope: principal.ScopeClient, clients: []string{"clt_a"}})
	res := n.subscribe(subscribeBody("shop:orders:1"), bearerHeader(tok))
	require.Equal(t, 200, res.status, string(res.body))
	s := n.listen(res.json()["streamToken"].(string), "")
	s.greeting()

	const warm, measured = 5, 40
	var lat []time.Duration
	for i := range warm + measured {
		start := time.Now()
		w.writePush(sink, outbox.NewPush("shop:orders:1", "order.updated", map[string]any{"seq": i}))
		f := s.nextEvent()
		took := time.Since(start)
		require.Contains(t, f, fmt.Sprintf(`"seq":%d`, i), "in order")
		require.Contains(t, f, "event: order.updated")
		if i >= warm {
			lat = append(lat, took)
		}
		time.Sleep(20 * time.Millisecond)
	}
	slices.Sort(lat)
	p50, p95, max := lat[len(lat)/2], lat[len(lat)*95/100], lat[len(lat)-1]
	t.Logf("insert to delivered on a stream, %d pushes after %d warm-up: p50 %v, p95 %v, max %v", measured, warm, p50, p95, max)
	assert.Less(t, p50, 100*time.Millisecond, "well under the poll interval")
	assert.Less(t, p95, 500*time.Millisecond)

	// and the delivered rows are gone from the outbox
	eventually(t, "the outbox to be empty", 2*time.Second, func() bool { _, total := w.outboxRows(); return total == 0 })
}

// Without the wake-up the poll is the fallback: a row written by a writer that cannot notify is
// delivered at the next poll.
func TestOutboxPushPollIsTheFallback(t *testing.T) {
	w := newWorld(t, "shop")
	w.clientRow("clt_a", "acme")
	w.rule("shop", "shop:orders:{id}", true)
	n := w.start()
	w.startProcessor(n, 100*time.Millisecond)
	tok, _ := w.bearer(who{scope: principal.ScopeClient, clients: []string{"clt_a"}})
	res := n.subscribe(subscribeBody("shop:orders:1"), bearerHeader(tok))
	require.Equal(t, 200, res.status)
	s := n.listen(res.json()["streamToken"].(string), "")
	s.greeting()

	w.outboxSchema()
	row, err := outbox.PushRow(outbox.Config{ClientCode: "acme"}, outbox.NewPush("shop:orders:1", "order.updated", map[string]any{"quiet": true}))
	require.NoError(t, err)
	stmt, err := outbox.InsertStatement("", outbox.PlaceholderDollar) // the plain insert: no pg_notify
	require.NoError(t, err)
	_, err = w.pool.Exec(t.Context(), stmt, row.Args()...)
	require.NoError(t, err)
	assert.Contains(t, s.nextEvent(), `"quiet":true`)
}

// A paused application's rows wait in the application's own outbox, untouched (no attempt is
// spent however long the pause lasts), and arrive in their order when it is unpaused.
func TestOutboxPushOrderSurvivesAPause(t *testing.T) {
	w := newWorld(t, "shop")
	w.clientRow("clt_a", "acme")
	w.rule("shop", "shop:orders:{id}", true)
	n := w.start()
	repo := NewConfigRepo(w.pool)
	require.NoError(t, repo.SetSettings(t.Context(), w.apps["shop"], AppSettings{Paused: true}, "prn_op"))
	eventually(t, "the pause", time.Second, func() bool { return n.gw.Store().Publish("shop").Paused })
	sink := outboxpgx.NewSink(outboxpgx.Config{ClientCode: "acme"})

	tok, _ := w.bearer(who{scope: principal.ScopeClient, clients: []string{"clt_a"}})
	res := n.subscribe(subscribeBody("shop:orders:1", "shop:orders:2"), bearerHeader(tok))
	require.Equal(t, 200, res.status, string(res.body))
	s := n.listen(res.json()["streamToken"].(string), "")
	s.greeting()

	// six rows in one group, written in order; a seventh on another topic of the same application
	for i := range 6 {
		w.writePush(sink, outbox.NewPush("shop:orders:1", "order.updated", map[string]any{"seq": i}))
	}
	w.writePush(sink, outbox.NewPush("shop:orders:2", "order.updated", map[string]any{"seq": "other"}))
	w.startProcessor(n, 100*time.Millisecond)

	time.Sleep(1500 * time.Millisecond) // many polls, each answered 429
	untouched, total := w.outboxRows()
	assert.Equal(t, 7, total, "nothing was lost")
	assert.Equal(t, 7, untouched, "and no attempt was spent: every row has retry_count 0 and no error")
	assert.Zero(t, w.pushRows(), "nothing reached the gateway")

	require.NoError(t, repo.SetSettings(t.Context(), w.apps["shop"], AppSettings{}, "prn_op"))
	var heard []string
	for range 7 {
		heard = append(heard, s.nextEvent())
	}
	var group []string
	for _, f := range heard {
		if !strings.Contains(f, `"other"`) {
			group = append(group, f)
		}
	}
	require.Len(t, group, 6)
	for i, f := range group {
		assert.Contains(t, f, fmt.Sprintf(`"seq":%d`, i), "the topic's pushes arrive in the order they were written")
	}
	eventually(t, "the outbox to be empty", 3*time.Second, func() bool { _, total := w.outboxRows(); return total == 0 })
}

// The processor's retry of a batch the gateway took but whose answer was lost does not publish the
// pushes twice: the row id is the pushId.
func TestOutboxPushRetryIsDeduplicated(t *testing.T) {
	w := newWorld(t, "shop")
	w.clientRow("clt_a", "acme")
	n := w.start()
	in := n.gw.Ingestor()
	row, err := outbox.PushRow(outbox.Config{ClientCode: "acme"}, outbox.NewPush("shop:orders:1", "order.updated", map[string]any{"v": 1}))
	require.NoError(t, err)
	var payload PushItem
	require.NoError(t, jsonUnmarshal(row.Payload, &payload))
	assert.Equal(t, row.ID, payload.PushID, "the row id is the pushId")

	for range 3 { // the processor sends the same row again and again
		res, err := batch(t, in, anchorCaller(t), payload)
		require.NoError(t, err)
		assert.Equal(t, "SUCCESS", res[0].Status)
	}
	assert.Equal(t, 1, w.pushRows())
}

// The notify statement the SDK writes is one statement with the insert: a rolled-back transaction
// notifies nobody and leaves nothing.
func TestPushRowNotifiesOnCommitOnly(t *testing.T) {
	w := newWorld(t)
	w.outboxSchema()
	pooled, err := w.pool.Acquire(t.Context())
	require.NoError(t, err)
	conn := pooled.Hijack()
	defer func() { _ = conn.Close(t.Context()) }()
	_, err = conn.Exec(t.Context(), "LISTEN "+pgx.Identifier{outbox.PushNotifyChannel}.Sanitize())
	require.NoError(t, err)
	sink := outboxpgx.NewSink(outboxpgx.Config{})

	tx, err := w.pool.Begin(t.Context())
	require.NoError(t, err)
	require.NoError(t, sink.WritePush(t.Context(), tx, outbox.NewPush("shop:a:b", "x.y", map[string]any{})))
	require.NoError(t, tx.Rollback(t.Context()))
	ctx, cancel := context.WithTimeout(t.Context(), 300*time.Millisecond)
	_, err = conn.WaitForNotification(ctx)
	cancel()
	require.Error(t, err, "a rollback notifies nobody")
	_, total := w.outboxRows()
	assert.Zero(t, total)

	w.writePush(sink, outbox.NewPush("shop:a:b", "x.y", map[string]any{}))
	ctx, cancel = context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	nt, err := conn.WaitForNotification(ctx)
	require.NoError(t, err)
	assert.Equal(t, outbox.PushNotifyChannel, nt.Channel)
	_, total = w.outboxRows()
	assert.Equal(t, 1, total)
}

func jsonUnmarshal(b []byte, v any) error { return json.Unmarshal(b, v) }
