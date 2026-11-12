//go:build integration

package api

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/emaildomainmapping"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/flowcatalyst-go/internal/tsid"
	"github.com/flowcatalyst/go-sdk/usecase"
)

// GET /api/email-domain-mappings/lookup names a domain's identity provider,
// tenants and 2FA policy, so it needs the family's read permission like its
// sibling by-domain. The pre-sign-in lookup is /auth/check-domain, not this.
func TestLookup_NeedsTheReadPermission(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	s := &State{Repo: emaildomainmapping.NewRepository(pool)}
	sfx := strings.ToLower(tsid.GenerateUntyped())[5:]
	domain := "lookup-gate-" + sfx + ".example.test"
	_, err := pool.Exec(ctx,
		`INSERT INTO tnt_email_domain_mappings (id, email_domain, identity_provider_id, scope_type)
		 VALUES ($1, $2, $3, 'ANCHOR')`, "edm_lg"+sfx, domain, "oip_lg"+sfx)
	require.NoError(t, err)

	// No AuthContext at all: refused.
	_, err = s.lookup(ctx, &lookupInput{Domain: domain})
	testpg.RequireUsecaseError(t, err, usecase.KindAuthorization, "UNAUTHENTICATED")

	// Authenticated, without the permission: refused.
	bare := testpg.WithAuth(ctx, &auth.AuthContext{PrincipalID: "prn_edmlookup", Scope: auth.ScopeAnchor})
	_, err = s.lookup(bare, &lookupInput{Domain: domain})
	testpg.RequireUsecaseError(t, err, usecase.KindAuthorization, "PERMISSION_REQUIRED")

	// With the read permission: the mapping.
	reader := testpg.WithAuth(ctx, &auth.AuthContext{
		PrincipalID: "prn_edmlookup", Scope: auth.ScopeAnchor,
		Permissions: []string{"platform:iam:email-domain-mapping:view"},
	})
	out, err := s.lookup(reader, &lookupInput{Domain: domain})
	require.NoError(t, err)
	body, ok := out.Body.(MappingResponse)
	require.True(t, ok, "the mapping, not the not-found envelope: %T", out.Body)
	assert.Equal(t, domain, body.EmailDomain)
	assert.Equal(t, "oip_lg"+sfx, body.IdentityProviderID)
}
