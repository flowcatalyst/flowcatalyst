//go:build integration

package pushgw

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/framework/go/fw"
	"github.com/flowcatalyst/framework/go/push"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
)

// publishTo writes a push for a client through a node's module (ingest is a later step; the relay,
// the log and the hubs are the real ones).
func publishTo(t *testing.T, n *node, client, topic, typ string, data map[string]any) {
	t.Helper()
	p, err := push.New(topic, typ, data)
	require.NoError(t, err)
	cx := fw.ContextFor(fw.MustClientID(client))
	require.NoError(t, n.gw.Pushes().PublishBatchAlone(t.Context(), push.Publication{Context: cx, Push: p}))
}

// Two gateway tasks on one database: a push published through one is heard on the other, a
// subscribe on one is listened to on the other (the stream token is the shared secret's), and only
// the stream's own client hears it.
func TestTwoGatewaysOnOneDatabase(t *testing.T) {
	w := newWorld(t, "alpha")
	w.rule("alpha", "alpha:order:{id}", false, "alpha:order:view")
	a, b := w.start(), w.start()

	tok, _ := w.bearer(who{scope: principal.ScopeClient, clients: []string{"clt_acme"}, perms: []string{"alpha:order:view"}})
	res := a.subscribe(map[string]any{"topics": []string{"alpha:order:1"}}, bearerHeader(tok))
	require.Equal(t, 200, res.status, string(res.body))
	token, _ := res.json()["streamToken"].(string)

	s := b.listen(token, "") // listened on the other task
	retry, subscribed := s.greeting()
	assert.Equal(t, "retry: 3000\n\n", retry)
	assert.Contains(t, subscribed, `"granted":["alpha:order:1"]`)

	publishTo(t, a, "clt_other", "alpha:order:1", "order.updated", map[string]any{"v": 0}) // another client: not heard
	publishTo(t, a, "clt_acme", "alpha:order:1", "order.updated", map[string]any{"v": 1})
	f := s.nextEvent()
	assert.Contains(t, f, "event: order.updated")
	assert.Contains(t, f, `"data":{"v":1}`)

	publishTo(t, b, "clt_acme", "alpha:order:1", "order.updated", map[string]any{"v": 2}) // and the other way
	assert.Contains(t, s.nextEvent(), `"data":{"v":2}`)
}

// A rule edit that is NOTIFYed applies on every task within a second.
func TestSyncedRulesApplyWithinASecond(t *testing.T) {
	w := newWorld(t, "alpha")
	a, b := w.start(), w.start()
	tok, _ := w.bearer(who{scope: principal.ScopeClient, clients: []string{"clt_acme"}, perms: []string{"alpha:order:view"}})

	res := b.subscribe(map[string]any{"topics": []string{"alpha:order:1"}}, bearerHeader(tok))
	require.Equal(t, 403, res.status) // no rule yet: nothing granted
	assert.Equal(t, "NO_TOPICS", res.code())

	w.rule("alpha", "alpha:order:{id}", false, "alpha:order:view")
	w.notifyConfig()
	for _, n := range []*node{a, b} {
		eventually(t, "the rule to apply", time.Second, func() bool {
			return n.subscribe(map[string]any{"topics": []string{"alpha:order:1"}}, bearerHeader(tok)).status == 200
		})
	}
}

// Per-application stream limits: the second stream of an application over its limit is 429
// PUSH_LIMIT naming the application; another application is unaffected.
func TestPerApplicationStreamLimit(t *testing.T) {
	w := newWorld(t, "alpha", "beta")
	w.rule("alpha", "alpha:feed:{id}", false, "p:use")
	w.rule("beta", "beta:feed:{id}", false, "p:use")
	_, err := w.pool.Exec(t.Context(), `INSERT INTO push_app_settings (application_id, max_streams) VALUES ($1, 1)`, w.apps["alpha"])
	require.NoError(t, err)
	n := w.start()
	tok, _ := w.bearer(who{scope: principal.ScopeClient, clients: []string{"clt_a"}, perms: []string{"p:use"}})
	h := bearerHeader(tok)

	open := func(topic string) response {
		res := n.subscribe(map[string]any{"topics": []string{topic}}, h)
		if res.status == 200 {
			s := n.listen(res.json()["streamToken"].(string), "")
			s.greeting()
		}
		return res
	}
	require.Equal(t, 200, open("alpha:feed:1").status)
	res := n.subscribe(map[string]any{"topics": []string{"alpha:feed:2"}}, h)
	require.Equal(t, 429, res.status, string(res.body))
	assert.Equal(t, "PUSH_LIMIT", res.code())
	details, _ := res.json()["details"].(map[string]any)
	assert.Equal(t, "application", details["scope"])
	assert.Equal(t, "alpha", details["application"])
	assert.Equal(t, 200, open("beta:feed:1").status)
}

// The per-application data limit applies to what the gateway publishes.
func TestPerApplicationDataLimit(t *testing.T) {
	w := newWorld(t, "alpha")
	_, err := w.pool.Exec(t.Context(), `INSERT INTO push_app_settings (application_id, max_data_bytes) VALUES ($1, 20)`, w.apps["alpha"])
	require.NoError(t, err)
	n := w.start()
	p, err := push.New("alpha:feed:1", "x.y", map[string]any{"s": strings.Repeat("a", 30)})
	require.NoError(t, err)
	err = n.gw.Pushes().PublishBatchAlone(t.Context(), push.Publication{Context: fw.ContextFor(fw.MustClientID("clt_a")), Push: p})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "DATA_TOO_LARGE")
}

// Drain: every open stream ends and a subscribe is 503.
func TestDrainEndsStreamsAndRefusesSubscribes(t *testing.T) {
	w := newWorld(t, "alpha")
	w.rule("alpha", "alpha:feed:{id}", false, "p:use")
	n := w.start()
	tok, _ := w.bearer(who{scope: principal.ScopeClient, clients: []string{"clt_a"}, perms: []string{"p:use"}})
	res := n.subscribe(map[string]any{"topics": []string{"alpha:feed:1"}}, bearerHeader(tok))
	require.Equal(t, 200, res.status)
	s := n.listen(res.json()["streamToken"].(string), "")
	s.greeting()

	n.gw.Drain()

	assert.True(t, s.ended(5*time.Second), "the stream ends when the drain begins")
	res = n.subscribe(map[string]any{"topics": []string{"alpha:feed:1"}}, bearerHeader(tok))
	assert.Equal(t, 503, res.status)
	assert.Equal(t, "UNAVAILABLE", res.code())
}

// The ready flag and the module's health checks come up with the component.
func TestReadyAndHealth(t *testing.T) {
	w := newWorld(t)
	n := w.start()
	assert.True(t, n.gw.Ready())
	rep := n.gw.Health(t.Context())
	assert.NotEmpty(t, rep.Results)
	assert.True(t, rep.IsUp())
}

// CORS over the mounted routes, from the platform's allowed-origins table.
func TestCORSOnThePushRoutes(t *testing.T) {
	w := newWorld(t, "alpha")
	w.rule("alpha", "alpha:feed:{id}", false, "p:use")
	_, err := w.pool.Exec(t.Context(),
		`INSERT INTO tnt_cors_allowed_origins (id, origin) VALUES ('cors_pushtest01', 'https://app.example.com')`)
	require.NoError(t, err)
	n := w.start()
	tok, _ := w.bearer(who{scope: principal.ScopeClient, clients: []string{"clt_a"}, perms: []string{"p:use"}})

	pre := n.do(request{method: "OPTIONS", path: pushPath + "/subscriptions", header: map[string]string{
		"Origin": "https://app.example.com", "Access-Control-Request-Method": "POST",
		"Access-Control-Request-Headers": "authorization, content-type",
	}})
	assert.Equal(t, 204, pre.status)
	assert.Equal(t, "https://app.example.com", pre.header.Get("Access-Control-Allow-Origin"))
	assert.Empty(t, pre.header.Get("Access-Control-Allow-Credentials"))

	h := bearerHeader(tok)
	h["Origin"] = "https://app.example.com"
	res := n.subscribe(subscribeBody("alpha:feed:1"), h)
	assert.Equal(t, 200, res.status, string(res.body))
	assert.Equal(t, "https://app.example.com", res.header.Get("Access-Control-Allow-Origin"))
	assert.Empty(t, res.header.Get("Access-Control-Allow-Credentials"))

	h["Origin"] = "https://evil.example"
	res = n.subscribe(subscribeBody("alpha:feed:1"), h)
	assert.Empty(t, res.header.Get("Access-Control-Allow-Origin"))
	pre = n.do(request{method: "OPTIONS", path: pushPath + "/subscriptions", header: map[string]string{
		"Origin": "https://evil.example", "Access-Control-Request-Method": "POST",
	}})
	assert.Equal(t, 403, pre.status)
}
