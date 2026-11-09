//go:build integration

package pushgw

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
)

// A field worker is granted the topics of the application its token was issued for, by rule or by
// ticket, and no other application's (owner ruling 2026-11-09). Other credentials are not narrowed.
func TestFieldWorkerGrantsAreTheTokensApplication(t *testing.T) {
	w := newWorld(t, "alpha", "beta")
	w.rule("alpha", "alpha:worker:{principal}:jobs", false)
	w.rule("beta", "beta:worker:{principal}:jobs", false) // the same shape: the worker's own principal
	w.rule("beta", "beta:board:{id}", false, "p:use")     // a rule a user can hold
	w.rule("beta", "beta:open:{id}", true)                // public: anyone
	ka, kb := w.syncTicketKey("alpha", "ka"), w.syncTicketKey("beta", "kb")
	n := w.start()
	const path = pushPath + "/subscriptions"

	f := w.fieldWorker("alpha", "clt_acme")
	mine := "alpha:worker:" + f.worker + ":jobs"
	theirs := "beta:worker:" + f.worker + ":jobs"

	t.Run("by rule", func(t *testing.T) {
		res := n.subscribe(subscribeBody(mine, theirs, "beta:open:1"), f.header(t, "POST", path))
		require.Equal(t, 200, res.status, string(res.body))
		assert.ElementsMatch(t, []any{mine, "beta:open:1"}, res.json()["granted"],
			"the token's application by rule; another application's public topic is public")
		assert.Equal(t, []any{map[string]any{"topic": theirs, "code": "TOPIC_FORBIDDEN"}}, res.json()["denied"])
	})

	t.Run("a worker of the other application is granted that one's", func(t *testing.T) {
		g := w.fieldWorker("beta", "clt_acme")
		res := n.subscribe(subscribeBody("beta:worker:"+g.worker+":jobs", "alpha:worker:"+g.worker+":jobs"), g.header(t, "POST", path))
		require.Equal(t, 200, res.status, string(res.body))
		assert.Equal(t, []any{"beta:worker:" + g.worker + ":jobs"}, res.json()["granted"])
	})

	t.Run("by ticket", func(t *testing.T) {
		own := ka.mint(t, map[string]any{"iss": "alpha", "client": "clt_acme", "topics": []string{"alpha:ticketed:{id}"}})
		foreign := kb.mint(t, map[string]any{"iss": "beta", "client": "clt_acme", "topics": []string{"beta:ticketed:{id}"}})
		res := n.subscribe(withTickets(subscribeBody("alpha:ticketed:1", "beta:ticketed:1"), own, foreign), f.header(t, "POST", path))
		require.Equal(t, 200, res.status, string(res.body))
		assert.Equal(t, []any{"alpha:ticketed:1"}, res.json()["granted"], "a ticket of another application grants a field worker nothing")
		assert.Equal(t, []any{map[string]any{"topic": "beta:ticketed:1", "code": "TOPIC_FORBIDDEN"}}, res.json()["denied"])

		// only the other application's ticket: nothing is granted
		res = n.subscribe(withTickets(subscribeBody("beta:ticketed:1"), foreign), f.header(t, "POST", path))
		assert.Equal(t, 403, res.status, string(res.body))
		assert.Equal(t, "NO_TOPICS", res.code())

		// the same tickets on a user's credential are not narrowed
		tok, _ := w.bearer(who{scope: principal.ScopeClient, clients: []string{"clt_acme"}, perms: []string{"p:use"}})
		res = n.subscribe(withTickets(subscribeBody("alpha:ticketed:1", "beta:ticketed:1", "beta:board:1"), own, foreign), bearerHeader(tok))
		require.Equal(t, 200, res.status, string(res.body))
		assert.ElementsMatch(t, []any{"alpha:ticketed:1", "beta:ticketed:1", "beta:board:1"}, res.json()["granted"],
			"a platform user's rules and tickets are for every application")
	})

	t.Run("the body keeps its other members", func(t *testing.T) {
		foreign := kb.mint(t, map[string]any{"iss": "beta", "client": "clt_acme", "topics": []string{"beta:ticketed:{id}"}})
		body := withClient(withTickets(subscribeBody(mine, "beta:ticketed:1"), foreign, "not-a-ticket"), "clt_acme")
		res := n.subscribe(body, f.header(t, "POST", path))
		assert.Equal(t, 400, res.status, "a malformed ticket is still the module's 400 (it is not removed)")
		body = withClient(withTickets(subscribeBody(mine, "beta:ticketed:1"), foreign), "clt_other")
		res = n.subscribe(body, f.header(t, "POST", path))
		assert.Equal(t, 403, res.status, "the client member still reaches the module")
		assert.Equal(t, "CLIENT_FORBIDDEN", res.code())
	})
}

// A proof is accepted once: the same proof presented again is refused, on this task and on every
// other; a fresh proof of the same token is fine.
func TestFieldProofReplayIsRefused(t *testing.T) {
	w := newWorld(t, "alpha")
	w.rule("alpha", "alpha:worker:{principal}:jobs", false)
	a, b := w.start(), w.start()
	const path = pushPath + "/subscriptions"

	f := w.fieldWorker("alpha", "clt_acme")
	topic := "alpha:worker:" + f.worker + ":jobs"
	hdr := f.header(t, "POST", path)

	res := a.subscribe(subscribeBody(topic), hdr)
	require.Equal(t, 200, res.status, string(res.body))

	res = a.subscribe(subscribeBody(topic), hdr)
	assert.Equal(t, 401, res.status, "the same proof again")
	assert.Equal(t, "invalid_dpop_proof", res.code(), string(res.body))

	res = b.subscribe(subscribeBody(topic), hdr)
	assert.Equal(t, 401, res.status, "and on another task")
	assert.Equal(t, "invalid_dpop_proof", res.code())

	assert.Equal(t, 200, b.subscribe(subscribeBody(topic), f.header(t, "POST", path)).status, "a fresh proof")

	// a proof that fails for another reason does not take its place in the store
	g := w.fieldWorker("alpha", "clt_acme")
	bad := g.header(t, "GET", path) // wrong method
	assert.Equal(t, 401, a.subscribe(subscribeBody(topic), bad).status)
	var n int
	require.NoError(t, w.pool.QueryRow(t.Context(), `SELECT count(*) FROM push_dpop_proofs`).Scan(&n))
	assert.Equal(t, 2, n, "only the two accepted proofs are remembered")
}

// A proof is remembered for its acceptance window and then forgotten; a stale row does not make a
// later proof a replay.
func TestReplayGuardWindowAndPrune(t *testing.T) {
	w := newWorld(t)
	g := PGReplayGuard{Pool: w.pool}
	now := time.Now()

	replayed, err := g.Seen(t.Context(), "jkt", "jti", now.Add(time.Hour), now)
	require.NoError(t, err)
	assert.False(t, replayed)
	replayed, err = g.Seen(t.Context(), "jkt", "jti", now.Add(time.Hour), now.Add(time.Minute))
	require.NoError(t, err)
	assert.True(t, replayed, "within the window")
	replayed, err = g.Seen(t.Context(), "other-key", "jti", now.Add(time.Hour), now)
	require.NoError(t, err)
	assert.False(t, replayed, "a jti is per key")

	// past its window the entry no longer counts, and the proof is taken again
	replayed, err = g.Seen(t.Context(), "jkt", "jti", now.Add(3*time.Hour), now.Add(2*time.Hour))
	require.NoError(t, err)
	assert.False(t, replayed)

	pruned, err := g.Prune(t.Context(), now.Add(2*time.Hour))
	require.NoError(t, err)
	assert.EqualValues(t, 1, pruned, "the other-key entry (an hour) is past its window; the renewed one is not")
}
