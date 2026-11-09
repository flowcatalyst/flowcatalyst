//go:build integration

package pushgw

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/framework/go/fw"
	"github.com/flowcatalyst/framework/go/fwtest"
	"github.com/flowcatalyst/framework/go/push"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
)

// The framework's conformance vectors (conformance/push/, pinned in testdata/conformance/push/)
// run over the mounted gateway: HTTP in, HTTP out, rules and keys read from the platform tables.
// What a vector expresses with a single-application host maps onto application "fulfil" here.

func asStrings(v any) []string {
	var out []string
	l, _ := v.([]any)
	for _, x := range l {
		out = append(out, x.(string))
	}
	return out
}

func str(v any) string { s, _ := v.(string); return s }

// streamClaims decodes the stream token the gateway answered: its client and principal.
func streamClaims(t *testing.T, token string) (client, principal string) {
	t.Helper()
	payload, _, _ := strings.Cut(token, ".")
	raw, err := base64.RawURLEncoding.DecodeString(payload)
	require.NoError(t, err)
	var c struct {
		Client    string `json:"c"`
		Principal string `json:"p"`
	}
	require.NoError(t, json.Unmarshal(raw, &c))
	return c.Client, c.Principal
}

// credsOf makes the bearer credential of a vector's Context, plus the client member to name.
func credsOf(w *world, ctx map[string]any, extra ...string) (header map[string]string, client string) {
	perms := asStrings(ctx["permissions"])
	x := who{scope: principal.ScopeClient, clients: []string{str(ctx["client"])}, perms: perms, id: str(ctx["principal"])}
	return bearerHeader(mustBearer(w, x)), ""
}

func mustBearer(w *world, x who) string {
	tok, _ := w.bearer(x)
	return tok
}

func TestConformanceTickets(t *testing.T) {
	v := readTestdata(t, "tickets.json")
	keys := v["keys"].(map[string]any)
	w := newWorld(t, "fulfil", "orders")
	for _, name := range []string{"current", "previous"} {
		k := keys[name].(map[string]any)
		w.ticketKey("fulfil", str(k["kid"]), map[string]string{"kty": "OKP", "crv": "Ed25519", "x": str(k["x"])})
	}
	now := time.Unix(int64(v["now"].(float64)), 0).UTC()
	n := w.start(nodeOpts{clock: fwtest.NewClock(now)})

	// the gateway mints no tickets and holds none of its own: the keys endpoint is empty
	res := n.do(request{method: "GET", path: pushPath + "/keys"})
	assert.Equal(t, 200, res.status)
	assert.JSONEq(t, `{"keys":[]}`, string(res.body))

	skipped := 0
	for _, raw := range v["cases"].([]any) {
		c := raw.(map[string]any)
		ctx, _ := c["context"].(map[string]any)
		if ctx != nil && ctx["proofKey"] != nil {
			// a ticket bound to a fixed thumbprint needs a field credential proving that exact key;
			// the binding is tested with real device keys in TestFieldTicketKeyBinding
			skipped++
			continue
		}
		t.Run(str(c["name"]), func(t *testing.T) {
			body := map[string]any{"topics": asStrings(c["topics"]), "tickets": asStrings(c["tickets"])}
			var header map[string]string
			wantPrincipal := ""
			if ctx != nil {
				header, _ = credsOf(w, ctx)
				wantPrincipal = str(ctx["principal"])
			}
			res := n.subscribe(body, header)
			want := c["expect"].(map[string]any)
			if code := str(want["error"]); code != "" {
				status := map[string]int{"VALIDATION": 400, "UNAUTHENTICATED": 401, "NO_TOPICS": 403}[code]
				assert.Equal(t, status, res.status, string(res.body))
				assert.Equal(t, code, res.code(), string(res.body))
				return
			}
			require.Equal(t, 200, res.status, string(res.body))
			got := res.json()
			assert.Equal(t, anyList(want["granted"]), got["granted"])
			assert.Equal(t, anyList(want["denied"]), got["denied"])
			assert.Equal(t, want["expiresAt"], got["expiresAt"])
			client, principal := streamClaims(t, str(got["streamToken"]))
			assert.Equal(t, want["streamClient"], client)
			if wantPrincipal == "" {
				wantPrincipal = str(want["principal"])
			}
			assert.Equal(t, wantPrincipal, principal)
		})
	}
	t.Logf("%d vector cases carry a fixed key thumbprint and are covered by TestFieldTicketKeyBinding", skipped)
}

func anyList(v any) any {
	if l, ok := v.([]any); ok && len(l) == 0 {
		return []any{}
	}
	return v
}

func TestConformanceTopicsAndRules(t *testing.T) {
	v := readTestdata(t, "topics.json")
	w := newWorld(t, "fulfil")
	n := w.start()
	ctx := t.Context()

	// topics: what a subscribe accepts is the contract's topic syntax
	tok := mustBearer(w, who{scope: principal.ScopeClient, clients: []string{"acme"}, perms: []string{"fulfil:x:y"}})
	for _, raw := range v["topics"].([]any) {
		c := raw.(map[string]any)
		topic := str(c["topic"])
		if l, ok := c["topicOfLength"].(float64); ok {
			topic = "fulfil:" + strings.Repeat("x", int(l)-7)
		}
		res := n.subscribe(subscribeBody(topic), bearerHeader(tok))
		if c["accepted"].(bool) {
			assert.NotEqual(t, 400, res.status, "%.40q: %s", topic, res.body)
		} else {
			assert.Equal(t, 400, res.status, "%.40q: %s", topic, res.body)
			assert.Equal(t, "VALIDATION", res.code())
		}
	}

	// types: the contract's type syntax, as the gateway publishes
	for _, raw := range v["types"].([]any) {
		c := raw.(map[string]any)
		typ := str(c["type"])
		if l, ok := c["typeOfLength"].(float64); ok {
			typ = strings.Repeat("a", int(l))
		}
		_, err := push.New("fulfil:order:1", typ, map[string]any{})
		assert.Equal(t, c["accepted"].(bool), err == nil, "type %.30q: %v", typ, err)
	}

	// data: size limits are per application (push_app_settings.max_data_bytes) at the gateway
	for _, raw := range v["data"].([]any) {
		c := raw.(map[string]any)
		text := str(c["json"])
		if l, ok := c["stringOfLength"].(float64); ok {
			text = `{"s":"` + strings.Repeat("a", int(l)) + `"}`
		}
		_, err := w.pool.Exec(ctx, `INSERT INTO push_app_settings (application_id, max_data_bytes) VALUES ($1, $2)
			ON CONFLICT (application_id) DO UPDATE SET max_data_bytes = $2`, w.apps["fulfil"], int(c["maxBytes"].(float64)))
		require.NoError(t, err)
		require.NoError(t, n.gw.Store().Reload(ctx))
		p, err := push.New("fulfil:order:1", "x.y", json.RawMessage(text))
		if err == nil {
			err = n.gw.Pushes().PublishBatchAlone(ctx, push.Publication{Context: fw.ContextFor(fw.MustClientID("acme")), Push: p})
		}
		switch code := str(c["code"]); {
		case code != "":
			require.Error(t, err, "data %.30s", text)
			assert.Contains(t, err.Error(), code, "data %.30s", text)
		default:
			assert.NoError(t, err, "data %.30s", text)
			if s := str(c["stored"]); s != "" {
				assert.Equal(t, s, p.Data())
			}
		}
	}

	// patterns: a rule the module refuses is left out of the application's rules
	for _, raw := range v["patterns"].([]any) {
		c := raw.(map[string]any)
		pattern := str(c["pattern"])
		w.clearRules()
		_, err := w.pool.Exec(ctx, `INSERT INTO push_rules (application_id, pattern, requires, public) VALUES ($1, $2, $3, $4)`,
			w.apps["fulfil"], pattern, nonNil(asStrings(c["requires"])), c["public"] == true)
		require.NoError(t, err)
		require.NoError(t, n.gw.Store().Reload(ctx))
		refused := len(n.gw.Store().Rules("fulfil")) == 0
		assert.Equal(t, c["refused"].(bool), refused, "pattern %s %v", pattern, c["requires"])
	}

	// rules: (rule, context, topic) -> allowed, through a subscribe
	skipped := 0
	for _, raw := range v["rules"].([]any) {
		c := raw.(map[string]any)
		r := c["rule"].(map[string]any)
		cx, _ := c["context"].(map[string]any)
		topic := str(c["topic"])
		if cx == nil && c["allowed"] == true {
			skipped++ // a public topic needs no credential in the module, but a stream has a client: a gateway subscribe without credentials needs a ticket
			continue
		}
		if cx != nil && cx["principal"] == nil {
			skipped++ // every platform credential has a principal
			continue
		}
		w.clearRules()
		_, err := w.pool.Exec(ctx, `INSERT INTO push_rules (application_id, pattern, requires, public) VALUES ($1, $2, $3, $4)`,
			w.apps["fulfil"], str(r["pattern"]), nonNil(asStrings(r["requires"])), r["public"] == true)
		require.NoError(t, err)
		require.NoError(t, n.gw.Store().Reload(ctx))
		var header map[string]string
		if cx != nil {
			header, _ = credsOf(w, cx)
		}
		res := n.subscribe(subscribeBody(topic), header)
		granted := res.status == 200 && slices.Contains(asStrings(res.json()["granted"]), topic)
		assert.Equal(t, c["allowed"].(bool), granted, "rule %v, context %v, topic %s: %d %s", r, cx, topic, res.status, res.body)
	}
	t.Logf("%d rule cases need a credential-less or principal-less caller, which the gateway has no such thing as", skipped)
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

func (w *world) clearRules() {
	w.t.Helper()
	_, err := w.pool.Exec(context.Background(), `DELETE FROM push_rules`)
	require.NoError(w.t, err)
}

func TestConformanceWire(t *testing.T) {
	v := readTestdata(t, "wire.json")
	w := newWorld(t, "fulfil")
	w.rule("fulfil", "fulfil:order:{id}", false, "fulfil:order:view")
	w.rule("fulfil", "fulfil:user:{principal}", false)
	n := w.start()

	tok := mustBearer(w, who{scope: principal.ScopeClient, clients: []string{"acme"}, perms: []string{"fulfil:order:view"}, id: "prn_0ABC"})
	res := n.subscribe(subscribeBody("fulfil:order:ord_1", "fulfil:user:prn_0ABC"), bearerHeader(tok))
	require.Equal(t, 200, res.status, string(res.body))
	s := n.listen(res.json()["streamToken"].(string), "")

	assert.Equal(t, str(v["streamStart"].(map[string]any)["bytes"]), s.next())
	assert.Equal(t, "event: subscribed\ndata: {\"granted\":[\"fulfil:order:ord_1\",\"fulfil:user:prn_0ABC\"],\"latest\":0}\n\n", s.next())
	for _, raw := range v["events"].([]any) {
		e := raw.(map[string]any)
		if e["name"] != "push" {
			continue
		}
		in := e["input"].(map[string]any)
		var data map[string]any
		require.NoError(t, json.Unmarshal([]byte(str(in["data"])), &data))
		publishTo(t, n, "acme", str(in["topic"]), str(in["type"]), data)
		got := s.nextEvent()
		// the log id is the gateway's; the rest of the frame is the vector's bytes
		_, wantRest, _ := strings.Cut(str(e["bytes"]), "\n")
		_, gotRest, _ := strings.Cut(got, "\n")
		assert.Equal(t, wantRest, gotRest)
		assert.Regexp(t, `^id: \d+\n`, got)
		break
	}
	assert.Equal(t, "text/event-stream", s.res.Header.Get("Content-Type"))
}

// Replay and reset: the vector's log is loaded into fw_push_log before the gateway starts, which
// serves every case from the log (push.recent 0).
func TestConformanceReplay(t *testing.T) {
	v := readTestdata(t, "replay.json")
	w := newWorld(t, "fulfil")
	w.rule("fulfil", "fulfil:order:{id}", true)
	w.rule("fulfil", "fulfil:news", true)
	for _, raw := range v["log"].([]any) {
		r := raw.(map[string]any)
		data, _ := json.Marshal(r["data"])
		var client any
		if r["client"] != nil {
			client = r["client"]
		}
		_, err := w.pool.Exec(t.Context(),
			`INSERT INTO fw_push_log (id, topic, type, client, data, logged_at) VALUES ($1, $2, $3, $4, $5, NOW())`,
			int64(r["id"].(float64)), r["topic"], r["type"], client, string(data))
		require.NoError(t, err)
	}
	n := w.start(nodeOpts{env: map[string]string{"PUSH_RECENT": "0"}})
	latest := int64(v["latest"].(float64))
	eventually(t, "the hub to follow the log", 5*time.Second, func() bool { return n.gw.Pushes().Cursor() == strconv.FormatInt(latest, 10) })

	for _, raw := range v["cases"].([]any) {
		c := raw.(map[string]any)
		t.Run(str(c["name"]), func(t *testing.T) {
			client := str(c["client"])
			var header map[string]string
			body := subscribeBody(asStrings(c["topics"])...)
			if client == "_platform" {
				header = bearerHeader(mustBearer(w, who{scope: principal.ScopeAnchor, perms: []string{PermAllClients}}))
				body = withClient(body, client)
			} else {
				header = bearerHeader(mustBearer(w, who{scope: principal.ScopeClient, clients: []string{client}}))
			}
			res := n.subscribe(body, header)
			require.Equal(t, 200, res.status, string(res.body))
			last := ""
			switch l := c["lastEventId"].(type) {
			case float64:
				last = fmt.Sprint(int64(l))
			case string:
				last = l
			}
			s := n.listen(res.json()["streamToken"].(string), last)
			s.greeting()
			// the replay is sent at once; a quiet moment is its end
			ids := []int64{}
			var reset map[string]any
			for {
				select {
				case f := <-s.frames:
					if strings.Contains(f, "event: reset") {
						var id int64
						_, _ = fmt.Sscanf(f, "id: %d", &id)
						reason := "retention"
						if strings.Contains(f, `"reason":"unknown-id"`) {
							reason = "unknown-id"
						}
						reset = map[string]any{"reason": reason, "id": float64(id)}
						continue
					}
					var id int64
					_, _ = fmt.Sscanf(f, "id: %d", &id)
					ids = append(ids, id)
					continue
				case <-time.After(500 * time.Millisecond):
				}
				break
			}
			want := c["expect"].(map[string]any)
			if wr, ok := want["reset"].(map[string]any); ok {
				assert.Equal(t, wr, reset)
				assert.Empty(t, ids)
				return
			}
			assert.Nil(t, reset)
			wantIDs := []int64{}
			for _, x := range want["ids"].([]any) {
				wantIDs = append(wantIDs, int64(x.(float64)))
			}
			assert.Equal(t, wantIDs, ids)
		})
	}
}
