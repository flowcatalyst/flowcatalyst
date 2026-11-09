//go:build integration

package pushgw

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
)

func jwkOf(t *testing.T) (pub ed25519.PublicKey, priv ed25519.PrivateKey, raw json.RawMessage) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	raw, _ = json.Marshal(map[string]string{"kty": "OKP", "crv": "Ed25519", "x": base64.RawURLEncoding.EncodeToString(pub)})
	return pub, priv, raw
}

// A sync's rules and keys apply on every gateway task within a second (the NOTIFY the repository
// sends in the sync's own transaction), and a replace that drops them removes them again.
func TestSyncReloadsEveryTaskWithinASecond(t *testing.T) {
	w := newWorld(t, "alpha")
	a, b := w.start(), w.start()
	repo := NewConfigRepo(w.pool)
	tok, _ := w.bearer(who{scope: principal.ScopeClient, clients: []string{"clt_acme"}, perms: []string{"alpha:order:view"}})
	subscribe := func(n *node) response {
		return n.subscribe(subscribeBody("alpha:order:1"), bearerHeader(tok))
	}
	require.Equal(t, 403, subscribe(b).status, "no rule yet")

	_, priv, jwk := jwkOf(t)
	res, err := repo.ReplaceRulesAndKeys(t.Context(), w.apps["alpha"],
		[]SyncRule{{Pattern: "alpha:order:{id}", Requires: []string{"alpha:order:view"}}},
		[]SyncKey{{Kid: "k1", JWK: jwk}})
	require.NoError(t, err)
	assert.Equal(t, Counts{Created: 1}, res.Rules)
	assert.Equal(t, Counts{Created: 1}, res.Keys)

	key := &ticketKey{kid: "k1", priv: priv}
	ticket := key.mint(t, map[string]any{"iss": "alpha", "client": "clt_acme", "topics": []string{"alpha:ticketed:{id}"}})
	for _, n := range []*node{a, b} {
		eventually(t, "the synced rule to apply", time.Second, func() bool { return subscribe(n).status == 200 })
		eventually(t, "the synced key to apply", time.Second, func() bool {
			return n.subscribe(withTickets(subscribeBody("alpha:ticketed:1"), ticket), nil).status == 200
		})
	}

	// replace: the rule goes, the key rotates (k2 added, k1 kept), counts say so
	_, _, jwk2 := jwkOf(t)
	res, err = repo.ReplaceRulesAndKeys(t.Context(), w.apps["alpha"], nil, []SyncKey{{Kid: "k1", JWK: jwk}, {Kid: "k2", JWK: jwk2}})
	require.NoError(t, err)
	assert.Equal(t, Counts{Deleted: 1}, res.Rules)
	assert.Equal(t, Counts{Created: 1, Updated: 1}, res.Keys)
	for _, n := range []*node{a, b} {
		eventually(t, "the removed rule to stop applying", time.Second, func() bool { return subscribe(n).status == 403 })
	}
	kids, err := repo.Kids(t.Context(), w.apps["alpha"])
	require.NoError(t, err)
	assert.Equal(t, []string{"k1", "k2"}, kids)

	// the key set is replaced too: k1 dropped, its tickets stop verifying everywhere
	_, err = repo.ReplaceRulesAndKeys(t.Context(), w.apps["alpha"], nil, []SyncKey{{Kid: "k2", JWK: jwk2}})
	require.NoError(t, err)
	for _, n := range []*node{a, b} {
		eventually(t, "the dropped key to stop verifying", time.Second, func() bool {
			return n.subscribe(withTickets(subscribeBody("alpha:ticketed:1"), ticket), nil).status != 200
		})
	}
}

// Sync is all or nothing: a failing write leaves the previous set, and no reload is announced.
func TestSyncIsOneTransaction(t *testing.T) {
	w := newWorld(t, "alpha")
	repo := NewConfigRepo(w.pool)
	_, _, jwk := jwkOf(t)
	_, err := repo.ReplaceRulesAndKeys(t.Context(), w.apps["alpha"], []SyncRule{{Pattern: "alpha:a:{id}", Requires: []string{"x:y"}}}, []SyncKey{{Kid: "k1", JWK: jwk}})
	require.NoError(t, err)

	// a key row that violates the table makes the second write fail after the rules were replaced
	_, err = repo.ReplaceRulesAndKeys(t.Context(), w.apps["alpha"], []SyncRule{{Pattern: "alpha:b:{id}", Requires: []string{"x:y"}}},
		[]SyncKey{{Kid: string(make([]byte, 80)), JWK: jwk}})
	require.Error(t, err)
	rules, err := repo.Rules(t.Context(), w.apps["alpha"])
	require.NoError(t, err)
	require.Len(t, rules, 1)
	assert.Equal(t, "alpha:a:{id}", rules[0].Pattern, "the earlier set stands")
}

func TestValidateSync(t *testing.T) {
	_, _, jwk := jwkOf(t)
	_, _, jwk2 := jwkOf(t)
	rule := func(p string, public bool, req ...string) SyncRule {
		return SyncRule{Pattern: p, Public: public, Requires: req}
	}
	key := func(kid string) SyncKey { return SyncKey{Kid: kid, JWK: jwk} }
	priv, _ := json.Marshal(map[string]string{"kty": "OKP", "crv": "Ed25519", "x": "AAAA", "d": "BBBB"})
	rsa, _ := json.Marshal(map[string]string{"kty": "RSA", "n": "x", "e": "AQAB"})

	cases := []struct {
		name  string
		rules []SyncRule
		keys  []SyncKey
		code  string
	}{
		{"valid", []SyncRule{rule("alpha:a:{principal}", false), rule("alpha:b:{id}", false, "x:y"), rule("alpha:c", true)}, []SyncKey{key("k1"), {Kid: "k2", JWK: jwk2}}, ""},
		{"another application's pattern", []SyncRule{rule("beta:a:{id}", false, "x:y")}, nil, "PATTERN_NOT_OWNED"},
		{"a pattern that only starts like the code", []SyncRule{rule("alphabet:a:{id}", false, "x:y")}, nil, "PATTERN_NOT_OWNED"},
		{"an open rule", []SyncRule{rule("alpha:a:{id}", false)}, nil, "RULE_OPEN"},
		{"an open rule, declared public", []SyncRule{rule("alpha:a:{id}", true)}, nil, ""},
		{"public with requires", []SyncRule{rule("alpha:a:{id}", true, "x:y")}, nil, "RULE_INVALID"},
		{"a pattern that is not one", []SyncRule{rule("alpha", false, "x:y")}, nil, "RULE_INVALID"},
		{"a bad segment", []SyncRule{rule("alpha:a b", false, "x:y")}, nil, "RULE_INVALID"},
		{"a blank permission", []SyncRule{rule("alpha:a:{id}", false, "")}, nil, "RULE_INVALID"},
		{"duplicate patterns", []SyncRule{rule("alpha:a:{id}", false, "x:y"), rule("alpha:a:{id}", false, "z:y")}, nil, "PATTERN_DUPLICATE"},
		{"five keys", nil, []SyncKey{key("1"), key("2"), key("3"), key("4"), key("5")}, "TOO_MANY_KEYS"},
		{"duplicate kids", nil, []SyncKey{key("k"), key("k")}, "KEY_DUPLICATE"},
		{"no kid", nil, []SyncKey{key("")}, "KEY_INVALID"},
		{"private key material", nil, []SyncKey{{Kid: "k", JWK: priv}}, "KEY_INVALID"},
		{"not an Ed25519 key", nil, []SyncKey{{Kid: "k", JWK: rsa}}, "KEY_INVALID"},
		{"garbage", nil, []SyncKey{{Kid: "k", JWK: json.RawMessage(`"x"`)}}, "KEY_INVALID"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := ValidateSync("alpha", c.rules, c.keys)
			if c.code == "" {
				assert.NoError(t, err)
				return
			}
			var ve *ValidationError
			require.ErrorAs(t, err, &ve)
			assert.Equal(t, c.code, ve.Code, ve.Message)
		})
	}
}

func TestValidateSettings(t *testing.T) {
	i := func(n int) *int { return &n }
	assert.NoError(t, ValidateSettings(AppSettings{}))
	assert.NoError(t, ValidateSettings(AppSettings{PublishRate: i(3000)}), "a rate over the default burst gets burst = rate")
	assert.NoError(t, ValidateSettings(AppSettings{MaxStreams: i(10), PublishRate: i(5), PublishBurst: i(5), MaxDataBytes: i(65536)}))
	for name, s := range map[string]AppSettings{
		"zero streams":          {MaxStreams: i(0)},
		"negative rate":         {PublishRate: i(-1)},
		"data over the ceiling": {MaxDataBytes: i(65537)},
		"burst under the rate":  {PublishRate: i(100), PublishBurst: i(50)},
	} {
		assert.Error(t, ValidateSettings(s), name)
	}
}

// The overrides are stored and read back; a missing row is all defaults.
func TestSettingsRoundTrip(t *testing.T) {
	w := newWorld(t, "alpha")
	repo := NewConfigRepo(w.pool)
	s, err := repo.Settings(t.Context(), w.apps["alpha"])
	require.NoError(t, err)
	assert.Equal(t, AppSettings{}, s)

	i := func(n int) *int { return &n }
	require.NoError(t, repo.SetSettings(t.Context(), w.apps["alpha"], AppSettings{MaxStreams: i(7), PublishRate: i(3), PublishBurst: i(9), Paused: true}, "prn_op"))
	s, err = repo.Settings(t.Context(), w.apps["alpha"])
	require.NoError(t, err)
	assert.Equal(t, 7, *s.MaxStreams)
	assert.Equal(t, 3, *s.PublishRate)
	assert.Equal(t, 9, *s.PublishBurst)
	assert.Nil(t, s.MaxDataBytes)
	assert.True(t, s.Paused)
	assert.Equal(t, "prn_op", *s.UpdatedBy)

	n := w.start()
	assert.Equal(t, PublishSettings{Rate: 3, Burst: 9, Paused: true}, n.gw.Store().Publish("alpha"))
	assert.Equal(t, PublishSettings{Rate: DefaultPublishRate, Burst: DefaultPublishBurst}, n.gw.Store().Publish("beta-unknown"))
}
