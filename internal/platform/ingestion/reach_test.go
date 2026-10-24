package ingestion

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecase"
)

func reachTarget() *Ingestion {
	return &Ingestion{ID: "ing_x", ApplicationID: "app_one", ClientID: "clt_a"}
}

// Reach is the client axis below anchor tier and the application axis for an
// application-scoped principal; neither is authority, which is a permission.
func TestCheckReach(t *testing.T) {
	cases := []struct {
		name    string
		ac      *auth.AuthContext
		allowed bool
	}{
		{"anchor, all applications", &auth.AuthContext{Scope: auth.ScopeAnchor, AllApplications: true}, true},
		{"client tier with the client", &auth.AuthContext{Scope: auth.ScopeClient, Clients: []string{"clt_a"}, AllApplications: true}, true},
		{"client tier without the client", &auth.AuthContext{Scope: auth.ScopeClient, Clients: []string{"clt_b"}, AllApplications: true}, false},
		{"client tier with no clients", &auth.AuthContext{Scope: auth.ScopeClient, AllApplications: true}, false},
		{"application-scoped to the application", &auth.AuthContext{Scope: auth.ScopeAnchor, Applications: []string{"app_one"}}, true},
		{"application-scoped to another", &auth.AuthContext{Scope: auth.ScopeAnchor, Applications: []string{"app_two"}}, false},
		{"anchor without application access", &auth.AuthContext{Scope: auth.ScopeAnchor}, false},
		{"unauthenticated", nil, false},
	}
	for _, c := range cases {
		err := CheckReach(c.ac, reachTarget())
		if c.allowed {
			assert.NoError(t, err, c.name)
			continue
		}
		require.Error(t, err, c.name)
		assert.Equal(t, usecase.KindAuthorization, usecase.AsError(err).Kind, c.name)
	}
}

func TestListReach(t *testing.T) {
	clients, apps := ListReach(&auth.AuthContext{Scope: auth.ScopeAnchor, AllApplications: true})
	assert.Nil(t, clients, "anchor tier reaches every client")
	assert.Nil(t, apps, "all-applications reaches every application")

	clients, apps = ListReach(&auth.AuthContext{Scope: auth.ScopeClient, Clients: []string{"clt_a"}, AllApplications: true})
	require.NotNil(t, clients)
	assert.Equal(t, []string{"clt_a"}, *clients)
	assert.Nil(t, apps)

	clients, apps = ListReach(&auth.AuthContext{Scope: auth.ScopeClient})
	require.NotNil(t, clients)
	assert.Empty(t, *clients, "a client-tier caller with no clients reaches nothing, not everything")
	require.NotNil(t, apps)
	assert.Empty(t, *apps)

	clients, apps = ListReach(&auth.AuthContext{Scope: auth.ScopeAnchor, Applications: []string{"app_one"}})
	assert.Nil(t, clients)
	require.NotNil(t, apps)
	assert.Equal(t, []string{"app_one"}, *apps)
}
