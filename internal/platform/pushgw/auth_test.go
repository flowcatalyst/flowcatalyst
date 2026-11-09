package pushgw

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/framework/go/httpfw"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
)

func statusOf(t *testing.T, err error) (int, string) {
	t.Helper()
	var he *httpfw.Error
	require.ErrorAs(t, err, &he)
	return he.Status(), he.Code()
}

// The stream's client for a platform credential (spec 4).
func TestChooseClient(t *testing.T) {
	user := func(scope auth.Scope, perms []string, clients ...string) *auth.AuthContext {
		return &auth.AuthContext{PrincipalID: "prn_1", Scope: scope, Clients: clients, Permissions: perms}
	}
	cases := []struct {
		name   string
		ac     *auth.AuthContext
		named  string
		want   string
		status int
		code   string
	}{
		{"the only client is the default", user(auth.ScopeClient, nil, "c1"), "", "c1", 0, ""},
		{"the only client, named", user(auth.ScopeClient, nil, "c1"), "c1", "c1", 0, ""},
		{"another client named", user(auth.ScopeClient, nil, "c1"), "c2", "", http.StatusForbidden, "CLIENT_FORBIDDEN"},
		{"several clients, none named", user(auth.ScopePartner, nil, "c1", "c2"), "", "", http.StatusBadRequest, "VALIDATION"},
		{"several clients, one named", user(auth.ScopePartner, nil, "c1", "c2"), "c2", "c2", 0, ""},
		{"an anchor reaches all: it must name one", user(auth.ScopeAnchor, nil, "*"), "", "", http.StatusBadRequest, "VALIDATION"},
		{"an anchor names any client", user(auth.ScopeAnchor, nil, "*"), "c9", "c9", 0, ""},
		{"no client at all", user(auth.ScopeClient, nil), "", "", http.StatusForbidden, "CLIENT_FORBIDDEN"},
		{"platform stream needs the permission", user(auth.ScopeAnchor, nil, "*"), "_platform", "", http.StatusForbidden, "CLIENT_FORBIDDEN"},
		{"platform stream with the permission", user(auth.ScopeAnchor, []string{PermAllClients}, "*"), "_platform", "_platform", 0, ""},
		{"platform stream with a covering wildcard", user(auth.ScopeAnchor, []string{"platform:*:*:*"}, "*"), "_platform", "_platform", 0, ""},
		{"the permission implies nothing", user(auth.ScopeAnchor, []string{PermAllClients}, "*"), "", "", http.StatusBadRequest, "VALIDATION"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := chooseClient(c.ac, c.named)
			if c.status == 0 {
				require.NoError(t, err)
				assert.Equal(t, c.want, got)
				return
			}
			status, code := statusOf(t, err)
			assert.Equal(t, c.status, status)
			assert.Equal(t, c.code, code)
		})
	}
}

func TestAuthorizationScheme(t *testing.T) {
	r, _ := http.NewRequest(http.MethodPost, "/", nil)
	r.Header.Set("Authorization", "DPoP  abc ")
	scheme, token := authorizationScheme(r)
	assert.Equal(t, "DPoP", scheme)
	assert.Equal(t, "abc", token)
}
