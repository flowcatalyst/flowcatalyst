//go:build integration

package operations_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth/authservice"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/client"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
)

// TestToPartner_TokensCarryEveryGrantedClient follows a CLIENT user flipped to
// PARTNER through to the tokens an OIDC app (Integral, via the Laravel SDK)
// receives: tier PARTNER, no home client_id, and a `clients` entry
// "id:identifier" for the old home client and the new one. The SDK admits a
// login to a tenant when the identifier half matches the tenant code, so both
// identifiers must be there, read back from the database rather than hand-built.
func TestToPartner_TokensCarryEveryGrantedClient(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := principal.NewRepository(testpg.Pool(t))
	clients := client.NewRepository(testpg.Pool(t))
	grants := principal.NewClientAccessGrantRepo(testpg.Pool(t))
	uow := testpg.NewUoW(t)

	home := mustCreateClient(t, uow, "Prn Partner Login Home", "prn-partner-login-home")
	added := mustCreateClient(t, uow, "Prn Partner Login Added", "prn-partner-login-added")
	other := mustCreateClient(t, uow, "Prn Partner Login Other", "prn-partner-login-other")
	seeded := mustCreateUser(t, repo, uow, "prn-partner-login@example.com", "CLIENT", &home)

	_, err := runAuthorized(uow, operations.SetClientAssociation(repo, clients, grants),
		operations.SetClientAssociationCommand{UserID: seeded.UserID, ClientID: added, Mode: operations.ModeToPartner})
	require.NoError(t, err)

	// Login resolves the user by email (password and SSO alike).
	p, err := repo.FindByEmail(ctx, "prn-partner-login@example.com")
	require.NoError(t, err)
	require.NotNil(t, p)

	svc := authservice.NewWithSecret(authservice.Config{
		SecretKey: "partner-login-test-secret-0123456789abcdef",
		Issuer:    "https://fc.test",
		Audience:  "flowcatalyst",
	})
	access, err := svc.GenerateAccessToken(p)
	require.NoError(t, err)
	idToken, err := svc.GenerateIDToken(p, "integral", nil)
	require.NoError(t, err)

	want := []string{home + ":prn-partner-login-home", added + ":prn-partner-login-added"}
	for name, tok := range map[string]string{"access": access, "id": idToken} {
		c := jwtClaims(t, tok)
		assert.Equal(t, "PARTNER", c["tier"], name)
		assert.NotContains(t, c, "client_id", "%s: a partner has no home client", name)
		got := stringsOf(c["clients"])
		assert.ElementsMatch(t, want, got, name)

		// The Laravel SDK's hasClientAccessByIdentifier: the part after ":".
		idents := make([]string, 0, len(got))
		for _, e := range got {
			_, ident, _ := strings.Cut(e, ":")
			idents = append(idents, ident)
		}
		assert.Contains(t, idents, "prn-partner-login-home", name)
		assert.Contains(t, idents, "prn-partner-login-added", name)
		assert.NotContains(t, idents, "prn-partner-login-other", name)
		assert.NotContains(t, got, other, name)
	}
}

func jwtClaims(t *testing.T, tok string) map[string]any {
	t.Helper()
	parts := strings.Split(tok, ".")
	require.Len(t, parts, 3)
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	require.NoError(t, err)
	var c map[string]any
	require.NoError(t, json.Unmarshal(raw, &c))
	return c
}

func stringsOf(v any) []string {
	arr, _ := v.([]any)
	out := make([]string, 0, len(arr))
	for _, e := range arr {
		if s, ok := e.(string); ok {
			out = append(out, s)
		}
	}
	return out
}
