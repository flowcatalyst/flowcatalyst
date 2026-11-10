package auth_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/go-sdk/auth"
)

func makeClaims(scope, principalType string, clients, roles []string) auth.AccessTokenClaims {
	return auth.AccessTokenClaims{
		Sub:           "prn_test123",
		Iss:           "flowcatalyst",
		Aud:           "flowcatalyst",
		Exp:           9999999999,
		Iat:           1000000000,
		Nbf:           1000000000,
		Jti:           "jti_abc",
		PrincipalType: principalType,
		Scope:         scope,
		Email:         "user@example.com",
		Name:          "Test User",
		Clients:       clients,
		Roles:         roles,
	}
}

func TestClaimsHasClientAccessWildcardAndSpecific(t *testing.T) {
	wc := makeClaims("ANCHOR", "USER", []string{"*"}, nil)
	assert.True(t, wc.HasClientAccess("clt_anything"))

	scoped := makeClaims("CLIENT", "USER", []string{"clt_a", "clt_b"}, nil)
	assert.True(t, scoped.HasClientAccess("clt_a"))
	assert.False(t, scoped.HasClientAccess("clt_c"))
}

func TestClaimsScopeFlags(t *testing.T) {
	anchor := makeClaims("ANCHOR", "USER", []string{"*"}, nil)
	assert.True(t, anchor.IsAnchor())
	assert.False(t, anchor.IsService())

	svc := makeClaims("CLIENT", "SERVICE", []string{"clt_1"}, nil)
	assert.True(t, svc.IsService())
	assert.False(t, svc.IsAnchor())
}

func TestClaimsHasRole(t *testing.T) {
	c := makeClaims("CLIENT", "USER", []string{"clt_1"}, []string{"admin", "editor"})
	assert.True(t, c.HasRole("admin"))
	assert.True(t, c.HasRole("editor"))
	assert.False(t, c.HasRole("viewer"))
}

func TestClaimsSerdeRoundTripPreservesTypeRename(t *testing.T) {
	c := makeClaims("ANCHOR", "SERVICE", []string{"*"}, []string{"admin"})
	raw, err := json.Marshal(c)
	require.NoError(t, err)
	assert.Contains(t, string(raw), `"type":"SERVICE"`)
	assert.NotContains(t, string(raw), `"principal_type":`)

	var back auth.AccessTokenClaims
	require.NoError(t, json.Unmarshal(raw, &back))
	assert.Equal(t, "SERVICE", back.PrincipalType)
}

func TestClaimsDeserializesWithoutEmail(t *testing.T) {
	raw := `{
		"sub":"prn_1","iss":"fc","aud":"fc","exp":9999999999,"iat":0,"nbf":0,
		"jti":"j1","type":"SERVICE","scope":"CLIENT","name":"svc","clients":["clt_1"]
	}`
	var c auth.AccessTokenClaims
	require.NoError(t, json.Unmarshal([]byte(raw), &c))
	assert.Empty(t, c.Email)
	assert.Equal(t, "SERVICE", c.PrincipalType)
}

func TestAuthContextDelegatesToClaims(t *testing.T) {
	c := makeClaims("ANCHOR", "USER", []string{"*"}, []string{"admin"})
	ctx := auth.NewAuthContext(c, "eyJtoken")

	assert.Equal(t, "prn_test123", ctx.PrincipalID())
	assert.Equal(t, "user@example.com", ctx.Email())
	assert.Equal(t, "Test User", ctx.Name())
	assert.True(t, ctx.IsAnchor())
	assert.True(t, ctx.HasClientAccess("clt_anything"))
	assert.True(t, ctx.HasRole("admin"))
	assert.Equal(t, []string{"*"}, ctx.ClientIDs())
	assert.Equal(t, []string{"admin"}, ctx.Roles())
	assert.Equal(t, "eyJtoken", ctx.BearerToken())
}

func TestClaimsModernTokenTierAndPermissions(t *testing.T) {
	raw := `{"sub":"p","type":"USER","tier":"ANCHOR","scope":"app:read app:write","clients":["clt_1:acme","*"]}`
	var c auth.AccessTokenClaims
	require.NoError(t, json.Unmarshal([]byte(raw), &c))
	assert.True(t, c.IsAnchor())
	assert.Equal(t, "ANCHOR", c.TierName())
	assert.Equal(t, []string{"app:read", "app:write"}, c.Permissions())
	assert.True(t, c.HasPermission("app:write"))
	assert.False(t, c.HasPermission("app:admin"))

	// tier wins over a legacy-looking scope
	c2 := auth.AccessTokenClaims{Tier: "CLIENT", Scope: "ANCHOR"}
	assert.False(t, c2.IsAnchor())
	assert.True(t, c2.IsClient())
}

func TestClaimsLegacyScopeTierFallback(t *testing.T) {
	c := auth.AccessTokenClaims{Scope: "PARTNER"}
	assert.True(t, c.IsPartner())
	assert.Empty(t, c.Permissions())
	// not exactly a tier name -> not a tier
	d := auth.AccessTokenClaims{Scope: "ANCHOR other:perm"}
	assert.False(t, d.IsAnchor())
	assert.Equal(t, []string{"ANCHOR", "other:perm"}, d.Permissions())
}

func TestClaimsClientPairs(t *testing.T) {
	c := auth.AccessTokenClaims{Clients: []string{"clt_a:acme", "clt_b:beta:x", "clt_c"}}
	assert.True(t, c.HasClientAccess("clt_a"))
	assert.True(t, c.HasClientAccess("clt_b"))
	assert.True(t, c.HasClientAccess("clt_c"))
	assert.False(t, c.HasClientAccess("acme"))
	assert.False(t, c.HasClientAccess("clt_d"))
	assert.False(t, c.HasClientAccess(""))
	assert.Equal(t, []string{"clt_a", "clt_b", "clt_c"}, c.ClientIDs())
	assert.Equal(t, []string{"acme", "beta:x"}, c.ClientCodes())
}
