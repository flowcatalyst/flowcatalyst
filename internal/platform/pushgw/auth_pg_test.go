//go:build integration

package pushgw

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"maps"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
)

// ticketKey is an application's ticket signing key (the app holds the private half; the platform
// holds the public JWK it synced).
type ticketKey struct {
	kid  string
	priv ed25519.PrivateKey
}

func (w *world) syncTicketKey(app, kid string) *ticketKey {
	w.t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(w.t, err)
	w.ticketKey(app, kid, map[string]string{"kty": "OKP", "crv": "Ed25519", "x": base64.RawURLEncoding.EncodeToString(pub)})
	w.notifyConfig()
	return &ticketKey{kid: kid, priv: priv}
}

// mint signs a ticket (framework contract 6.4).
func (k *ticketKey) mint(t *testing.T, claims map[string]any) string {
	t.Helper()
	b64 := func(v any) string {
		raw, err := json.Marshal(v)
		require.NoError(t, err)
		return base64.RawURLEncoding.EncodeToString(raw)
	}
	now := time.Now().Unix()
	full := map[string]any{"aud": "fw-push", "iat": now, "exp": now + 120}
	maps.Copy(full, claims)
	signing := b64(map[string]any{"alg": "EdDSA", "kid": k.kid}) + "." + b64(full)
	return signing + "." + base64.RawURLEncoding.EncodeToString(ed25519.Sign(k.priv, []byte(signing)))
}

func subscribeBody(topics ...string) map[string]any { return map[string]any{"topics": topics} }

func withClient(body map[string]any, client string) map[string]any {
	body["client"] = client
	return body
}

func withTickets(body map[string]any, tickets ...string) map[string]any {
	body["tickets"] = tickets
	return body
}

// The authenticator over the real module: bearer tokens, the session cookie with the console's
// CSRF rule, tickets alone, and the client member.
func TestAuthenticatorMatrix(t *testing.T) {
	w := newWorld(t, "alpha")
	w.rule("alpha", "alpha:order:{id}", false, "alpha:order:view")
	key := w.syncTicketKey("alpha", "k1")
	n := w.start()
	const order = "alpha:order:1"
	perms := []string{"alpha:order:view"}

	clientUser := func(client string) string {
		tok, _ := w.bearer(who{scope: principal.ScopeClient, clients: []string{client}, perms: perms})
		return tok
	}
	partner := func(clients ...string) string {
		tok, _ := w.bearer(who{scope: principal.ScopePartner, clients: clients, perms: perms})
		return tok
	}
	anchor := func(extra ...string) string {
		tok, _ := w.bearer(who{scope: principal.ScopeAnchor, perms: append(append([]string{}, perms...), extra...)})
		return tok
	}
	goodTicket := key.mint(t, map[string]any{"iss": "alpha", "client": "clt_acme", "topics": []string{"alpha:order:{id}"}})

	type tc struct {
		name   string
		body   map[string]any
		header map[string]string
		status int
		code   string
	}
	cookie, _ := w.cookie(perms...)
	cookieHeader := func(extra map[string]string) map[string]string {
		h := map[string]string{"Cookie": "fc_session=" + cookie}
		maps.Copy(h, extra)
		return h
	}
	cases := []tc{
		{"bearer, the only client", subscribeBody(order), bearerHeader(clientUser("clt_acme")), 200, ""},
		{"no credentials and no tickets", subscribeBody(order), nil, 401, "UNAUTHENTICATED"},
		{"tickets alone", withTickets(subscribeBody(order), goodTicket), nil, 200, ""},
		{"bad bearer", subscribeBody(order), bearerHeader("not-a-token"), 401, "UNAUTHENTICATED"},
		{"bad bearer with a good ticket is still 401", withTickets(subscribeBody(order), goodTicket), bearerHeader("not-a-token"), 401, "UNAUTHENTICATED"},
		{"a field token presented as a bearer", subscribeBody(order), bearerHeader(w.fieldWorker("alpha", "clt_acme").token), 401, "UNAUTHENTICATED"},
		{"a stale cookie is logged out: the ticket works", withTickets(subscribeBody(order), goodTicket), map[string]string{"Cookie": "fc_session=stale"}, 200, ""},
		{"a stale cookie alone is no credential", subscribeBody(order), map[string]string{"Cookie": "fc_session=stale"}, 401, "UNAUTHENTICATED"},

		// the console's CSRF rule for the session cookie
		{"cookie, same origin", withClient(subscribeBody(order), "clt_acme"), cookieHeader(map[string]string{"Sec-Fetch-Site": "same-origin"}), 200, ""},
		{"cookie, Origin names the public base", withClient(subscribeBody(order), "clt_acme"), cookieHeader(map[string]string{"Origin": publicBase}), 200, ""},
		{"cookie, cross-site refused", withClient(subscribeBody(order), "clt_acme"), cookieHeader(map[string]string{"Sec-Fetch-Site": "cross-site"}), 403, "CROSS_ORIGIN_REQUEST"},
		{"cookie, another origin refused", withClient(subscribeBody(order), "clt_acme"), cookieHeader(map[string]string{"Origin": "https://evil.example"}), 403, "CROSS_ORIGIN_REQUEST"},
		{"cookie, no provenance refused", withClient(subscribeBody(order), "clt_acme"), cookieHeader(nil), 403, "CROSS_ORIGIN_REQUEST"},

		// the client member
		{"partner with several clients, none named", subscribeBody(order), bearerHeader(partner("clt_a", "clt_b")), 400, "VALIDATION"},
		{"partner names one of its clients", withClient(subscribeBody(order), "clt_b"), bearerHeader(partner("clt_a", "clt_b")), 200, ""},
		{"partner names another client", withClient(subscribeBody(order), "clt_z"), bearerHeader(partner("clt_a", "clt_b")), 403, "CLIENT_FORBIDDEN"},
		{"a client user names another client", withClient(subscribeBody(order), "clt_z"), bearerHeader(clientUser("clt_a")), 403, "CLIENT_FORBIDDEN"},
		{"a client user names its client", withClient(subscribeBody(order), "clt_a"), bearerHeader(clientUser("clt_a")), 200, ""},
		{"an anchor reaches all clients: it names one", subscribeBody(order), bearerHeader(anchor()), 400, "VALIDATION"},
		{"an anchor names a client", withClient(subscribeBody(order), "clt_any"), bearerHeader(anchor()), 200, ""},
		{"a client that is not a client id", withClient(subscribeBody(order), "no spaces"), bearerHeader(anchor()), 400, "VALIDATION"},

		// platform-wide streams
		{"platform stream without the permission", withClient(subscribeBody(order), "_platform"), bearerHeader(anchor()), 403, "CLIENT_FORBIDDEN"},
		{"platform stream, a client user without the permission", withClient(subscribeBody(order), "_platform"), bearerHeader(clientUser("clt_a")), 403, "CLIENT_FORBIDDEN"},
		{"platform stream with the permission", withClient(subscribeBody(order), "_platform"), bearerHeader(anchor(PermAllClients)), 200, ""},
		{"the permission alone implies no platform stream", subscribeBody(order), bearerHeader(anchor(PermAllClients)), 400, "VALIDATION"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			res := n.subscribe(c.body, c.header)
			assert.Equal(t, c.status, res.status, string(res.body))
			if c.code != "" {
				assert.Equal(t, c.code, res.code(), string(res.body))
			}
		})
	}

	t.Run("cookie, a body that is not JSON", func(t *testing.T) {
		res := n.do(request{
			path: pushPath + "/subscriptions", raw: "topics=x",
			header: cookieHeader(map[string]string{"Sec-Fetch-Site": "same-origin", "Content-Type": "text/plain"}),
		})
		assert.Equal(t, http.StatusUnsupportedMediaType, res.status, string(res.body))
		assert.Equal(t, "UNSUPPORTED_MEDIA_TYPE", res.code())
	})
}

// A platform-wide stream hears every client; a client's stream hears its own only.
func TestPlatformStreamHearsEveryClient(t *testing.T) {
	w := newWorld(t, "alpha")
	w.rule("alpha", "alpha:feed:{id}", false, "p:use")
	n := w.start()
	wide, _ := w.bearer(who{scope: principal.ScopeAnchor, perms: []string{"p:use", PermAllClients}})
	own, _ := w.bearer(who{scope: principal.ScopeClient, clients: []string{"clt_a"}, perms: []string{"p:use"}})

	open := func(h map[string]string, client string) *stream {
		body := subscribeBody("alpha:feed:1")
		if client != "" {
			body = withClient(body, client)
		}
		res := n.subscribe(body, h)
		require.Equal(t, 200, res.status, string(res.body))
		s := n.listen(res.json()["streamToken"].(string), "")
		s.greeting()
		return s
	}
	platform, mine := open(bearerHeader(wide), "_platform"), open(bearerHeader(own), "")

	publishTo(t, n, "clt_b", "alpha:feed:1", "x.y", map[string]any{"for": "b"})
	assert.Contains(t, platform.nextEvent(), `"for":"b"`)
	publishTo(t, n, "clt_a", "alpha:feed:1", "x.y", map[string]any{"for": "a"})
	assert.Contains(t, mine.nextEvent(), `"for":"a"`, "the client's own stream never heard clt_b's push")
	assert.Contains(t, platform.nextEvent(), `"for":"a"`)
}

// The field plane: the token, its DPoP proof, and the live state of the login family.
func TestFieldAuthentication(t *testing.T) {
	w := newWorld(t, "alpha")
	w.rule("alpha", "alpha:worker:{principal}:jobs", false)
	n := w.start()
	const path = pushPath + "/subscriptions"

	f := w.fieldWorker("alpha", "clt_acme")
	topic := "alpha:worker:" + f.worker + ":jobs"
	body := func() map[string]any { return subscribeBody(topic) }
	other := newDevice(t)

	tc := []struct {
		name   string
		header func() map[string]string
		body   map[string]any
		status int
		code   string
	}{
		{"valid token and proof", func() map[string]string { return f.header(t, "POST", path) }, body(), 200, ""},
		{"the named client is the token's", func() map[string]string { return f.header(t, "POST", path) }, withClient(body(), "clt_acme"), 200, ""},
		{"another client named", func() map[string]string { return f.header(t, "POST", path) }, withClient(body(), "clt_other"), 403, "CLIENT_FORBIDDEN"},
		{"no proof", func() map[string]string { return map[string]string{"Authorization": "DPoP " + f.token} }, body(), 401, "invalid_dpop_proof"},
		{"a proof for another method", func() map[string]string { return f.header(t, "GET", path) }, body(), 401, "invalid_dpop_proof"},
		{"a proof for another URL", func() map[string]string { return f.header(t, "POST", pushPath+"/stream") }, body(), 401, "invalid_dpop_proof"},
		{"a proof that names another access token", func() map[string]string {
			return map[string]string{
				"Authorization": "DPoP " + f.token,
				"DPoP":          f.dev.proof(t, "POST", publicBase+path, w.fieldWorker("alpha", "clt_acme").token, time.Now()),
			}
		}, body(), 401, "invalid_dpop_proof"},
		{"a proof signed by another key than the token's", func() map[string]string {
			return map[string]string{"Authorization": "DPoP " + f.token, "DPoP": other.proof(t, "POST", publicBase+path, f.token, time.Now())}
		}, body(), 401, "invalid_token"},
		{"a token that is not a token", func() map[string]string {
			return map[string]string{"Authorization": "DPoP garbage", "DPoP": f.dev.proof(t, "POST", publicBase+path, "garbage", time.Now())}
		}, body(), 401, "invalid_token"},
	}
	for _, c := range tc {
		t.Run(c.name, func(t *testing.T) {
			res := n.subscribe(c.body, c.header())
			assert.Equal(t, c.status, res.status, string(res.body))
			if c.code != "" {
				assert.Equal(t, c.code, res.code(), string(res.body))
			}
		})
	}

	t.Run("a revoked login family", func(t *testing.T) {
		g := w.fieldWorker("alpha", "clt_acme")
		gTopic := "alpha:worker:" + g.worker + ":jobs"
		assert.Equal(t, 200, n.subscribe(subscribeBody(gTopic), g.header(t, "POST", path)).status)
		_, err := w.pool.Exec(t.Context(), `UPDATE field_sessions SET revoked_at = NOW(), revoked_reason = 'LOGOUT' WHERE family_id = $1`, g.family)
		require.NoError(t, err)
		res := n.subscribe(subscribeBody(gTopic), g.header(t, "POST", path))
		assert.Equal(t, 401, res.status, string(res.body))
		assert.Equal(t, "invalid_token", res.code())
	})
}

// A ticket bound to a device key (cnf.jkt) is honoured for the field credential holding that key,
// and for nobody else.
func TestFieldTicketKeyBinding(t *testing.T) {
	w := newWorld(t, "alpha")
	w.rule("alpha", "alpha:feed:{id}", false, "p:use")
	key := w.syncTicketKey("alpha", "k1")
	n := w.start()
	const path = pushPath + "/subscriptions"

	f := w.fieldWorker("alpha", "clt_acme")
	bound := key.mint(t, map[string]any{
		"iss": "alpha", "client": "clt_acme", "topics": []string{"alpha:bound:{id}"},
		"cnf": map[string]string{"jkt": f.dev.jkt()},
	})

	res := n.subscribe(withTickets(subscribeBody("alpha:bound:1"), bound), f.header(t, "POST", path))
	require.Equal(t, 200, res.status, string(res.body))
	assert.Equal(t, []any{"alpha:bound:1"}, res.json()["granted"])

	// the same ticket on a bearer credential (no proof key) grants nothing
	tok, _ := w.bearer(who{scope: principal.ScopeClient, clients: []string{"clt_acme"}, perms: []string{"p:use"}})
	res = n.subscribe(withTickets(subscribeBody("alpha:feed:1", "alpha:bound:1"), bound), bearerHeader(tok))
	require.Equal(t, 200, res.status, string(res.body))
	assert.Equal(t, []any{"alpha:feed:1"}, res.json()["granted"])
	assert.Equal(t, []any{map[string]any{"topic": "alpha:bound:1", "code": "TICKET_INVALID"}}, res.json()["denied"])

	// and on a field credential with another key
	g := w.fieldWorker("alpha", "clt_acme")
	res = n.subscribe(withTickets(subscribeBody("alpha:bound:1"), bound), g.header(t, "POST", path))
	assert.Equal(t, 403, res.status, string(res.body))
}

// A ticket for app B's topic signed by app A grants nothing; a ticket of another client grants
// nothing; tickets of two applications compose in one subscribe.
func TestTicketsAcrossApplications(t *testing.T) {
	w := newWorld(t, "alpha", "beta")
	ka, kb := w.syncTicketKey("alpha", "ka"), w.syncTicketKey("beta", "kb")
	n := w.start()
	mint := func(k *ticketKey, iss, client string, topics ...string) string {
		return k.mint(t, map[string]any{"iss": iss, "client": client, "topics": topics})
	}
	ta := mint(ka, "alpha", "clt_acme", "alpha:a:{id}")
	tb := mint(kb, "beta", "clt_acme", "beta:b:{id}")

	res := n.subscribe(withTickets(subscribeBody("alpha:a:1", "beta:b:1"), ta, tb), nil)
	require.Equal(t, 200, res.status, string(res.body))
	assert.ElementsMatch(t, []any{"alpha:a:1", "beta:b:1"}, res.json()["granted"])

	// alpha's key signing a beta ticket: the issuer's keys decide, and the topic must be the issuer's
	forged := mint(ka, "beta", "clt_acme", "beta:b:{id}")
	res = n.subscribe(withTickets(subscribeBody("alpha:a:1", "beta:b:1"), ta, forged), nil)
	require.Equal(t, 200, res.status, string(res.body))
	assert.Equal(t, []any{"alpha:a:1"}, res.json()["granted"])
	assert.Equal(t, []any{map[string]any{"topic": "beta:b:1", "code": "TICKET_INVALID"}}, res.json()["denied"])

	// a ticket naming another client than the stream's
	tOther := mint(kb, "beta", "clt_other", "beta:b:{id}")
	res = n.subscribe(withTickets(subscribeBody("alpha:a:1", "beta:b:1"), ta, tOther), nil)
	require.Equal(t, 200, res.status, string(res.body))
	assert.Equal(t, []any{"alpha:a:1"}, res.json()["granted"])
	assert.Equal(t, []any{map[string]any{"topic": "beta:b:1", "code": "TICKET_INVALID"}}, res.json()["denied"])

	// a ticket of an application the platform has no keys for grants nothing
	_, stranger, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	ts := (&ticketKey{kid: "kx", priv: stranger}).mint(t, map[string]any{"iss": "gamma", "client": "clt_acme", "topics": []string{"gamma:g:{id}"}})
	res = n.subscribe(withTickets(subscribeBody("alpha:a:1", "gamma:g:1"), ta, ts), nil)
	require.Equal(t, 200, res.status, string(res.body))
	assert.Equal(t, []any{"alpha:a:1"}, res.json()["granted"])
	assert.Equal(t, []any{map[string]any{"topic": "gamma:g:1", "code": "TICKET_INVALID"}}, res.json()["denied"])
}
