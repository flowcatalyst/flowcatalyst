//go:build integration

package oauthapi

import (
	"encoding/base64"
	"net/http"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/encryption"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
)

// TestClientCredentialsDanglingPrincipalID is the regression guard for the
// "principal lookup returns nil" half of the client_credentials
// misconfiguration fix: a confidential client whose PrincipalID points at a
// row that doesn't exist (dangling reference) must fail the same way as a
// client with no PrincipalID at all — 400 unauthorized_client, not 500
// server_error — since it's the same "this client isn't configured for this
// grant" condition from the caller's point of view.
func TestClientCredentialsDanglingPrincipalID(t *testing.T) {
	// A self-contained encryption key (not FLOWCATALYST_APP_KEY from the
	// environment) so this test doesn't depend on external setup — same
	// approach as overlapState in secret_overlap_test.go.
	key := base64.StdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef"))
	enc, err := encryption.New(key)
	require.NoError(t, err)

	dangling := "prn_doesnotexist000"
	c := &auth.OAuthClient{
		ClientID:    "oac_dangling",
		Active:      true,
		ClientType:  auth.OAuthClientConfidential,
		PrincipalID: &dangling,
	}

	s := &State{
		OAuthClients: fakeClientFinder{client: c},
		Principals:   principal.NewRepository(testpg.Pool(t)),
		Auth:         testAuthService(t),
		Encryption:   enc,
	}
	c.SetSecretRef(encrypted(t, s, "s3cr3t"))

	rr := doTokenRequest(t, s, url.Values{
		"grant_type":    {"client_credentials"},
		"client_id":     {"oac_dangling"},
		"client_secret": {"s3cr3t"},
	})

	require.Equal(t, http.StatusBadRequest, rr.Code, rr.Body.String())
	assert.Contains(t, rr.Body.String(), "unauthorized_client")
}
