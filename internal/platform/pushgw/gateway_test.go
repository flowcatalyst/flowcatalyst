package pushgw

import (
	"bytes"
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWithPoolMaxBoundsTheModulesPool(t *testing.T) {
	got, err := withPoolMax("postgres://u:p@h:5432/db?sslmode=disable", 0)
	require.NoError(t, err)
	assert.Contains(t, got, "pool_max_conns=10")
	assert.Contains(t, got, "sslmode=disable")

	got, err = withPoolMax("postgres://u:p@h/db", 4)
	require.NoError(t, err)
	assert.Contains(t, got, "pool_max_conns=4")

	got, err = withPoolMax("postgres://u:p@h/db?pool_max_conns=25", 4)
	require.NoError(t, err)
	assert.Equal(t, "postgres://u:p@h/db?pool_max_conns=25", got, "the URL's own setting wins")

	_, err = withPoolMax("://bad", 1)
	assert.Error(t, err)
}

// fieldScope touches only a field worker's subscribe, removes only the tickets of other
// applications, and leaves every other body as it was.
func TestFieldScopeNarrowsOnlyAFieldWorkersTickets(t *testing.T) {
	jwt := func(claims string) string {
		enc := func(s string) string {
			return strings.TrimRight(strings.NewReplacer("+", "-", "/", "_").Replace(b64(s)), "=")
		}
		return enc(`{"alg":"EdDSA"}`) + "." + enc(claims) + ".c2ln"
	}
	token := jwt(`{"aud":"alpha"}`)
	own, foreign := jwt(`{"iss":"alpha"}`), jwt(`{"iss":"beta"}`)

	seen := func(method, path, auth, body string) string {
		var got string
		h := fieldScope(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
			b, _ := io.ReadAll(r.Body)
			got = string(b)
			if r.ContentLength >= 0 {
				assert.Equal(t, int64(len(b)), r.ContentLength, "the length follows the body")
			}
		}))
		req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
		if auth != "" {
			req.Header.Set("Authorization", auth)
		}
		h.ServeHTTP(httptest.NewRecorder(), req)
		return got
	}
	body := `{"topics":["alpha:a:1"],"tickets":["` + own + `","` + foreign + `","not-a-jwt"],"client":"c1"}`

	got := seen(http.MethodPost, "/api/push/subscriptions", "DPoP "+token, body)
	assert.NotContains(t, got, foreign)
	assert.Contains(t, got, own)
	assert.Contains(t, got, "not-a-jwt", "an unreadable ticket stays: the module answers it")
	assert.Contains(t, got, `"client":"c1"`)
	assert.Contains(t, got, `"topics":["alpha:a:1"]`)

	assert.Equal(t, body, seen(http.MethodPost, "/api/push/subscriptions", "Bearer "+token, body), "not a field worker")
	assert.Equal(t, body, seen(http.MethodPost, "/api/push/subscriptions", "", body), "no credential")
	assert.Equal(t, body, seen(http.MethodPost, "/api/push/other", "DPoP "+token, body), "another route")
	assert.Equal(t, body, seen(http.MethodGet, "/api/push/subscriptions", "DPoP "+token, body), "another method")
	assert.Equal(t, body, seen(http.MethodPost, "/api/push/subscriptions", "DPoP garbage", body), "a token with no readable application")
	assert.Equal(t, "not json", seen(http.MethodPost, "/api/push/subscriptions", "DPoP "+token, "not json"))
	noTickets := `{"topics":["alpha:a:1"]}`
	assert.Equal(t, noTickets, seen(http.MethodPost, "/api/push/subscriptions", "DPoP "+token, noTickets))
	allOwn := `{"topics":["alpha:a:1"],"tickets":["` + own + `"]}`
	assert.Equal(t, allOwn, seen(http.MethodPost, "/api/push/subscriptions", "DPoP "+token, allOwn), "nothing to remove: byte for byte")
}

func TestAppAccessPermission(t *testing.T) {
	assert.Equal(t, "fc:push:app:alpha", AppAccessPermission("alpha"))
	assert.Equal(t, 4, len(strings.Split(AppAccessPermission("alpha"), ":")), "four segments, so a wildcard holder matches")
}

func b64(s string) string { return base64.StdEncoding.EncodeToString([]byte(s)) }
