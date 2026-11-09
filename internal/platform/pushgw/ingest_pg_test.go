//go:build integration

package pushgw

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apicommon"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
)

// ── helpers ──────────────────────────────────────────────────────────────

// callerCtx is a request context carrying an AuthContext.
func callerCtx(t *testing.T, ac *auth.AuthContext) context.Context {
	return auth.WithContext(t.Context(), ac)
}

// anchorCaller reaches every client and every application and may ingest.
func anchorCaller(t *testing.T) context.Context {
	return callerCtx(t, &auth.AuthContext{
		PrincipalID: "prn_anchor", Scope: auth.ScopeAnchor, AllApplications: true, Permissions: []string{PermIngest},
	})
}

// appCaller is an application's service account: confined to the given application ids, one client.
func appCaller(t *testing.T, client string, applicationIDs ...string) context.Context {
	return callerCtx(t, &auth.AuthContext{
		PrincipalID: "prn_svc", Scope: auth.ScopeClient, Clients: []string{client}, Applications: applicationIDs,
		Permissions: []string{PermIngest}, PrincipalType: auth.PrincipalTypeService,
	})
}

func item(topic, typ string, data string) PushItem {
	return PushItem{Topic: topic, Type: typ, Data: json.RawMessage(data)}
}

func (i PushItem) withID(id string) PushItem           { i.PushID = id; return i }
func (i PushItem) withClientCode(code string) PushItem { i.ClientCode = &code; return i }

func batch(t *testing.T, in *Ingestor, ctx context.Context, items ...PushItem) ([]PushResult, error) {
	t.Helper()
	out, err := in.batch(ctx, &apicommon.In[PushBatchRequest]{Body: PushBatchRequest{Items: items}})
	if err != nil {
		return nil, err
	}
	require.Len(t, out.Body.Results, len(items), "positional: one result per item")
	return out.Body.Results, nil
}

func (w *world) clientRow(id, identifier string) {
	w.t.Helper()
	_, err := w.pool.Exec(w.t.Context(), `INSERT INTO tnt_clients (id, name, identifier) VALUES ($1, $2, $2)`, id, identifier)
	require.NoError(w.t, err)
}

// pushRows counts the pushes the module holds (written to the outbox or already in the log).
func (w *world) pushRows() int {
	w.t.Helper()
	var n int
	require.NoError(w.t, w.pool.QueryRow(w.t.Context(),
		`SELECT (SELECT count(*) FROM fw_push_outbox) + (SELECT count(*) FROM fw_push_log)`).Scan(&n))
	return n
}

func (w *world) ingestIDs() int {
	w.t.Helper()
	var n int
	require.NoError(w.t, w.pool.QueryRow(w.t.Context(), `SELECT count(*) FROM push_ingest_ids`).Scan(&n))
	return n
}

// platformStream opens a platform-wide stream (every client) on the topics, for an application with
// a public rule.
func (w *world) platformStream(n *node, topics ...string) *stream {
	w.t.Helper()
	tok, _ := w.bearer(who{scope: principal.ScopeAnchor, perms: []string{PermAllClients}})
	res := n.subscribe(withClient(subscribeBody(topics...), "_platform"), bearerHeader(tok))
	require.Equal(w.t, 200, res.status, string(res.body))
	s := n.listen(res.json()["streamToken"].(string), "")
	s.greeting()
	return s
}

// ── ingest ───────────────────────────────────────────────────────────────

// An accepted push reaches a listening stream, once the relay has moved it: ingest is the module's
// own write path.
func TestIngestPublishesToAStream(t *testing.T) {
	w := newWorld(t, "alpha")
	w.rule("alpha", "alpha:feed:{id}", true)
	n := w.start()
	s := w.platformStream(n, "alpha:feed:1")

	res, err := batch(t, n.gw.Ingestor(), anchorCaller(t), item("alpha:feed:1", "order.updated", `{"v": 1}`).withID("p1"))
	require.NoError(t, err)
	assert.Equal(t, PushResult{ID: "p1", Status: "SUCCESS"}, res[0])

	f := s.nextEvent()
	assert.Contains(t, f, "event: order.updated")
	assert.Contains(t, f, `"data":{"v":1}`, "stored as compact JSON")
}

// The HTTP surface: the wire is {items} in and {results} out under the module's mount, and a
// caller without the permission is refused.
func TestIngestOverHTTP(t *testing.T) {
	w := newWorld(t, "alpha")
	n := w.start()
	body := map[string]any{"items": []map[string]any{{"topic": "alpha:feed:1", "type": "x.y", "data": map[string]any{"a": 1}, "pushId": "h1"}}}

	res := n.do(request{path: IngestPath, body: body})
	assert.Equal(t, http.StatusForbidden, res.status, "no credential: the platform's handler answer for a missing AuthContext")

	noPerm, _ := w.bearer(who{scope: principal.ScopeAnchor, perms: []string{"platform:messaging:batch:events-write"}})
	res = n.do(request{path: IngestPath, body: body, header: bearerHeader(noPerm)})
	assert.Equal(t, http.StatusForbidden, res.status, string(res.body))

	tok, _ := w.bearer(who{scope: principal.ScopeAnchor, perms: []string{PermIngest}})
	res = n.do(request{path: IngestPath, body: body, header: bearerHeader(tok)})
	require.Equal(t, http.StatusOK, res.status, string(res.body))
	results, _ := res.json()["results"].([]any)
	require.Len(t, results, 1)
	assert.Equal(t, "SUCCESS", results[0].(map[string]any)["status"])
	assert.Equal(t, 1, w.pushRows())

	// the session cookie is not an ingest credential
	cookie, _ := w.cookie(PermIngest)
	res = n.do(request{path: IngestPath, body: body, header: map[string]string{"Cookie": "fc_session=" + cookie}})
	assert.NotEqual(t, http.StatusOK, res.status, "ingest is for bearer tokens; the server route drops the cookie")
}

// An application-scoped caller pushes for its own applications only (TOPIC_NOT_OWNED, FORBIDDEN),
// and nothing of a refused item is written; the rest of the batch lands.
func TestIngestOwnership(t *testing.T) {
	w := newWorld(t, "alpha", "beta")
	n := w.start()
	in := n.gw.Ingestor()
	ctx := appCaller(t, "clt_a", w.apps["alpha"])

	res, err := batch(t, in, ctx,
		item("alpha:feed:1", "x.y", `{}`).withID("own"),
		item("beta:feed:1", "x.y", `{}`).withID("other"),
		item("nosuch:feed:1", "x.y", `{}`).withID("unknown"),
	)
	require.NoError(t, err)
	assert.Equal(t, "SUCCESS", res[0].Status)
	assert.Equal(t, "FORBIDDEN", res[1].Status)
	assert.Equal(t, "TOPIC_NOT_OWNED", res[1].Code)
	assert.Equal(t, "FORBIDDEN", res[2].Status, "unknown and inaccessible are indistinguishable to an app-scoped caller")
	assert.Equal(t, "TOPIC_NOT_OWNED", res[2].Code)
	assert.Equal(t, 1, w.pushRows())
	assert.Equal(t, 1, w.ingestIDs(), "only the accepted push id is recorded")

	// an unscoped caller reaches every application, but an unknown one is no application
	res, err = batch(t, in, anchorCaller(t), item("beta:feed:1", "x.y", `{}`), item("nosuch:feed:1", "x.y", `{}`))
	require.NoError(t, err)
	assert.Equal(t, "SUCCESS", res[0].Status)
	assert.Equal(t, "BAD_REQUEST", res[1].Status)
	assert.Equal(t, "APPLICATION_UNKNOWN", res[1].Code)
}

// The client of a push: the credential's only client by default, a named client the credential
// reaches, the platform client for an anchor; a client it cannot reach is CLIENT_FORBIDDEN.
func TestIngestClient(t *testing.T) {
	w := newWorld(t, "alpha")
	w.clientRow("clt_a", "acme")
	w.clientRow("clt_b", "globex")
	w.rule("alpha", "alpha:feed:{id}", true)
	n := w.start()
	in := n.gw.Ingestor()
	ctx := appCaller(t, "clt_a", w.apps["alpha"])

	// listeners of each client
	tokA, _ := w.bearer(who{scope: principal.ScopeClient, clients: []string{"clt_a"}})
	tokB, _ := w.bearer(who{scope: principal.ScopeClient, clients: []string{"clt_b"}})
	open := func(tok string) *stream {
		res := n.subscribe(subscribeBody("alpha:feed:1"), bearerHeader(tok))
		require.Equal(t, 200, res.status, string(res.body))
		s := n.listen(res.json()["streamToken"].(string), "")
		s.greeting()
		return s
	}
	sa, sb := open(tokA), open(tokB)

	res, err := batch(t, in, ctx,
		item("alpha:feed:1", "x.y", `{"to":"default"}`),                         // the credential's only client
		item("alpha:feed:1", "x.y", `{"to":"globex"}`).withClientCode("globex"), // another client: refused
		item("alpha:feed:1", "x.y", `{"to":"acme"}`).withClientCode("acme"),
	)
	require.NoError(t, err)
	assert.Equal(t, "SUCCESS", res[0].Status)
	assert.Equal(t, "FORBIDDEN", res[1].Status)
	assert.Equal(t, "CLIENT_FORBIDDEN", res[1].Code)
	assert.Equal(t, "SUCCESS", res[2].Status)
	assert.Contains(t, sa.nextEvent(), `"to":"default"`)
	assert.Contains(t, sa.nextEvent(), `"to":"acme"`)

	// an anchor names any client; with none it pushes for the platform client (heard by a platform stream only)
	res, err = batch(t, in, anchorCaller(t),
		item("alpha:feed:1", "x.y", `{"to":"globex"}`).withClientCode("globex"),
		item("alpha:feed:1", "x.y", `{"to":"platform"}`),
	)
	require.NoError(t, err)
	assert.Equal(t, "SUCCESS", res[0].Status)
	assert.Equal(t, "SUCCESS", res[1].Status)
	assert.Contains(t, sb.nextEvent(), `"to":"globex"`)

	// a caller with several clients and none named is refused (a push is for one client)
	multi := callerCtx(t, &auth.AuthContext{
		PrincipalID: "prn_multi", Scope: auth.ScopePartner, Clients: []string{"clt_a", "clt_b"},
		AllApplications: true, Permissions: []string{PermIngest},
	})
	res, err = batch(t, in, multi, item("alpha:feed:1", "x.y", `{}`))
	require.NoError(t, err)
	assert.Equal(t, "CLIENT_FORBIDDEN", res[0].Code)
}

// Each refusal of framework section 1 is an item BAD_REQUEST with the framework's code, and the
// rest of the batch is written.
func TestIngestValidationCodes(t *testing.T) {
	w := newWorld(t, "alpha")
	n := w.start()
	in := n.gw.Ingestor()
	limit := fmt.Sprintf(`{"s":"%s"}`, strings.Repeat("a", 17000))
	cases := []struct {
		it   PushItem
		code string
	}{
		{item("Alpha:feed", "x.y", `{}`), "TOPIC_INVALID"},
		{item("alpha", "x.y", `{}`), "TOPIC_INVALID"},
		{item("alpha:fe ed", "x.y", `{}`), "TOPIC_INVALID"},
		{item("alpha:feed", "X.y", `{}`), "TYPE_INVALID"},
		{item("alpha:feed", "x..y", `{}`), "TYPE_INVALID"},
		{item("alpha:feed", "reset", `{}`), "TYPE_RESERVED"},
		{item("alpha:feed", "fw.x", `{}`), "TYPE_RESERVED"},
		{item("alpha:feed", "x.y", `[1]`), "DATA_NOT_OBJECT"},
		{item("alpha:feed", "x.y", `null`), "DATA_NOT_OBJECT"},
		{item("alpha:feed", "x.y", `{"a":`), "DATA_INVALID"},
		{item("alpha:feed", "x.y", ``), "DATA_INVALID"},
		{item("alpha:feed", "x.y", limit), "DATA_TOO_LARGE"},
		{item("alpha:feed", "x.y", `{}`).withID(strings.Repeat("i", 65)), "PUSH_ID_INVALID"},
	}
	items := []PushItem{item("alpha:feed", "x.y", `{"ok":true}`)}
	for _, c := range cases {
		items = append(items, c.it)
	}
	res, err := batch(t, in, anchorCaller(t), items...)
	require.NoError(t, err)
	assert.Equal(t, "SUCCESS", res[0].Status)
	for i, c := range cases {
		assert.Equal(t, "BAD_REQUEST", res[i+1].Status, "case %d (%s)", i, c.code)
		assert.Equal(t, c.code, res[i+1].Code, "case %d", i)
	}
	assert.Equal(t, 1, w.pushRows(), "only the valid item was written")

	// the application's own data limit
	_, err = w.pool.Exec(t.Context(), `INSERT INTO push_app_settings (application_id, max_data_bytes) VALUES ($1, 20)`, w.apps["alpha"])
	require.NoError(t, err)
	require.NoError(t, n.gw.Store().Reload(t.Context()))
	res, err = batch(t, in, anchorCaller(t), item("alpha:feed", "x.y", `{"s":"`+strings.Repeat("a", 30)+`"}`), item("alpha:feed", "x.y", `{"s":"a"}`))
	require.NoError(t, err)
	assert.Equal(t, "DATA_TOO_LARGE", res[0].Code)
	assert.Equal(t, "SUCCESS", res[1].Status)

	// too many items refuse the whole request
	_, err = batch(t, in, anchorCaller(t), make([]PushItem, MaxBatchItems+1)...)
	require.Error(t, err)
}

// A pushId is written once: a retried request, and a repeat inside one request, answer SUCCESS and
// write nothing more.
func TestIngestDuplicatePushIDs(t *testing.T) {
	w := newWorld(t, "alpha", "beta")
	w.rule("alpha", "alpha:feed:{id}", true)
	n := w.start()
	in := n.gw.Ingestor()
	s := w.platformStream(n, "alpha:feed:1")

	first := []PushItem{
		item("alpha:feed:1", "x.y", `{"n":1}`).withID("dup"),
		item("alpha:feed:1", "x.y", `{"n":2}`).withID("dup"), // again inside the request
		item("alpha:feed:1", "x.y", `{"n":3}`).withID("other"),
		item("alpha:feed:1", "x.y", `{"n":4}`), // no id: never a duplicate
	}
	res, err := batch(t, in, anchorCaller(t), first...)
	require.NoError(t, err)
	for _, r := range res {
		assert.Equal(t, "SUCCESS", r.Status)
	}
	assert.Equal(t, "DUPLICATE", res[1].Code)
	assert.Equal(t, 3, w.pushRows())

	res, err = batch(t, in, anchorCaller(t), first...) // the processor sends the batch again
	require.NoError(t, err)
	assert.Equal(t, "DUPLICATE", res[0].Code)
	assert.Equal(t, "DUPLICATE", res[2].Code)
	assert.Empty(t, res[3].Code, "an id-less push is written again")
	assert.Equal(t, 4, w.pushRows())

	// the same id of another application is another push
	res, err = batch(t, in, anchorCaller(t), item("beta:feed:1", "x.y", `{}`).withID("dup"))
	require.NoError(t, err)
	assert.Empty(t, res[0].Code)
	assert.Equal(t, 5, w.pushRows())

	// what the stream heard: n1, n3, n4, then n4 again; in order
	var heard []string
	for range 4 {
		heard = append(heard, s.nextEvent())
	}
	for i, want := range []string{`"n":1`, `"n":3`, `"n":4`, `"n":4`} {
		assert.Contains(t, heard[i], want)
	}
}

// Everything of a request is written in one transaction: when the write fails, the push ids are not
// recorded either, so the sender's retry is not mistaken for a duplicate.
func TestIngestIsOneTransaction(t *testing.T) {
	w := newWorld(t, "alpha")
	n := w.start()
	in := n.gw.Ingestor()
	_, err := w.pool.Exec(t.Context(), `ALTER TABLE fw_push_outbox RENAME TO fw_push_outbox_gone`)
	require.NoError(t, err)

	_, err = batch(t, in, anchorCaller(t),
		item("alpha:feed:1", "x.y", `{}`).withID("t1"), item("alpha:feed:1", "x.y", `{}`).withID("t2"))
	require.Error(t, err, "the module's write fails")
	assert.Zero(t, w.ingestIDs(), "the push ids roll back with the pushes")

	_, err = w.pool.Exec(t.Context(), `ALTER TABLE fw_push_outbox_gone RENAME TO fw_push_outbox`)
	require.NoError(t, err)
	res, err := batch(t, in, anchorCaller(t),
		item("alpha:feed:1", "x.y", `{}`).withID("t1"), item("alpha:feed:1", "x.y", `{}`).withID("t2"))
	require.NoError(t, err)
	assert.Empty(t, res[0].Code, "the retry is a first send, not a duplicate")
	assert.Equal(t, 2, w.pushRows())
}

// A paused application answers the whole batch 429 with Retry-After, writes nothing, and takes the
// batch again once unpaused; another application is unaffected.
func TestIngestPause(t *testing.T) {
	w := newWorld(t, "alpha", "beta")
	n := w.start()
	repo := NewConfigRepo(w.pool)
	tok, _ := w.bearer(who{scope: principal.ScopeAnchor, perms: []string{PermIngest}})
	send := func(topics ...string) response {
		items := make([]map[string]any, len(topics))
		for i, tp := range topics {
			items[i] = map[string]any{"topic": tp, "type": "x.y", "data": map[string]any{}}
		}
		return n.do(request{path: IngestPath, body: map[string]any{"items": items}, header: bearerHeader(tok)})
	}

	require.NoError(t, repo.SetSettings(t.Context(), w.apps["alpha"], AppSettings{Paused: true}, "prn_op"))
	eventually(t, "the pause to reach the task", time.Second, func() bool { return n.gw.Store().Publish("alpha").Paused })

	res := send("alpha:feed:1", "beta:feed:1")
	require.Equal(t, http.StatusTooManyRequests, res.status, string(res.body))
	assert.Equal(t, "5", res.header.Get("Retry-After"))
	assert.Equal(t, "PUSH_PAUSED", res.code())
	assert.Zero(t, w.pushRows(), "a 429 writes nothing, not even the other application's push")

	assert.Equal(t, http.StatusOK, send("beta:feed:1").status, "another application is unaffected")

	require.NoError(t, repo.SetSettings(t.Context(), w.apps["alpha"], AppSettings{}, "prn_op"))
	eventually(t, "the unpause to reach the task", time.Second, func() bool { return !n.gw.Store().Publish("alpha").Paused })
	assert.Equal(t, http.StatusOK, send("alpha:feed:1", "beta:feed:1").status, "the batch is taken once unpaused")
	assert.Equal(t, 3, w.pushRows())
}

// A publish rate with a burst: the burst passes, the next batch is 429 with a Retry-After of the
// wait, and it passes once the bucket has refilled.
func TestIngestPublishRate(t *testing.T) {
	w := newWorld(t, "alpha")
	n := w.start()
	in := n.gw.Ingestor()
	now := time.Now()
	in.Now = func() time.Time { return now }
	r, b := 2, 4
	require.NoError(t, NewConfigRepo(w.pool).SetSettings(t.Context(), w.apps["alpha"], AppSettings{PublishRate: &r, PublishBurst: &b}, "prn_op"))
	eventually(t, "the limits to reach the task", time.Second, func() bool { return n.gw.Store().Publish("alpha").Burst == 4 })

	four := []PushItem{item("alpha:f:1", "x.y", `{}`), item("alpha:f:1", "x.y", `{}`), item("alpha:f:1", "x.y", `{}`), item("alpha:f:1", "x.y", `{}`)}
	_, err := batch(t, in, anchorCaller(t), four...)
	require.NoError(t, err, "the burst passes")

	_, err = batch(t, in, anchorCaller(t), four[:2]...)
	var limited *rateLimited
	require.ErrorAs(t, err, &limited, "over the rate: the whole batch is 429")
	assert.Equal(t, "1", limited.GetHeaders().Get("Retry-After"), "two tokens at 2/s: one second")
	assert.Equal(t, 4, w.pushRows())

	now = now.Add(time.Second)
	_, err = batch(t, in, anchorCaller(t), four[:2]...)
	require.NoError(t, err, "refilled")
	assert.Equal(t, 6, w.pushRows())

	// a batch larger than the burst is not starved: it takes the whole bucket
	now = now.Add(10 * time.Second)
	_, err = batch(t, in, anchorCaller(t), append(four, four...)...)
	require.NoError(t, err)
}

// The ingest ids are pruned past the retention.
func TestPruneIngestIDs(t *testing.T) {
	w := newWorld(t, "alpha")
	_, err := w.pool.Exec(t.Context(), `INSERT INTO push_ingest_ids (application_id, push_id, created_at) VALUES ($1, 'old', NOW() - interval '2 hours'), ($1, 'new', NOW())`, w.apps["alpha"])
	require.NoError(t, err)
	n, err := PruneIngestIDs(t.Context(), w.pool, time.Hour, time.Now())
	require.NoError(t, err)
	assert.EqualValues(t, 1, n)
	assert.Equal(t, 1, w.ingestIDs())
}

// Items that leave out members the SDKs always write are answered item by item (not 422 for the
// whole batch), and the gateway's counters are on the framework registry the host exposes.
func TestIngestMissingMembersAndMetrics(t *testing.T) {
	w := newWorld(t, "alpha")
	n := w.start()
	tok, _ := w.bearer(who{scope: principal.ScopeAnchor, perms: []string{PermIngest}})
	res := n.do(request{path: IngestPath, header: bearerHeader(tok), raw: `{"items":[{"type":"x.y","data":{}},{"topic":"alpha:a"},{"topic":"alpha:a","type":"x.y"},{"topic":"alpha:a","type":"x.y","data":{"ok":1}}]}`})
	require.Equal(t, http.StatusOK, res.status, string(res.body))
	results := res.json()["results"].([]any)
	require.Len(t, results, 4)
	var codes []string
	for _, r := range results {
		m := r.(map[string]any)
		c, _ := m["code"].(string)
		codes = append(codes, m["status"].(string)+" "+c)
	}
	assert.Equal(t, []string{"BAD_REQUEST TOPIC_INVALID", "BAD_REQUEST TYPE_INVALID", "BAD_REQUEST DATA_INVALID", "SUCCESS "}, codes)

	eventually(t, "the relay to log the push", 5*time.Second, func() bool {
		var found bool
		for _, f := range n.gw.Metrics().Snapshot().Families {
			if f.Name == "fw_push_published_total" {
				found = len(f.Samples) > 0
			}
		}
		return found
	})
	got := map[string]float64{}
	for _, f := range n.gw.Metrics().Snapshot().Families {
		for _, s := range f.Samples {
			got[f.Name+"/"+strings.Join(s.LabelValues, ",")] = s.Value
		}
	}
	assert.EqualValues(t, 1, got["fc_push_ingest_items_total/alpha,accepted"])
	assert.EqualValues(t, 2, got["fc_push_ingest_items_total/alpha,refused"], "the item without a topic has no application to count")
	assert.EqualValues(t, 1, got["fc_push_ingest_rejected_total/alpha,TYPE_INVALID"])
	assert.EqualValues(t, 1, got["fw_push_published_total/"])
}
