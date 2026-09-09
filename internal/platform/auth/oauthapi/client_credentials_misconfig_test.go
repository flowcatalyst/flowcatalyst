package oauthapi

import (
	"testing"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth"
)

// TestClientCredentialsNilPrincipalID is the regression guard for the
// client_credentials grant against a confidential client with no linked
// principal: RFC 6749 §5.2 treats this as a client-side misconfiguration
// (unauthorized_client, 400), not a server fault (server_error, 500) —
// there's nothing wrong on FlowCatalyst's end that a client can't itself fix
// by asking an operator to link the client to a principal.
func TestClientCredentialsNilPrincipalID(t *testing.T) {
	s := overlapState(t)
	c := &auth.OAuthClient{
		ClientID:   "oac_nolink",
		Active:     true,
		ClientType: auth.OAuthClientConfidential,
		// PrincipalID intentionally left nil.
	}
	c.SetSecretRef(encrypted(t, s, "s3cr3t"))
	s.OAuthClients = fakeClientFinder{client: c}

	status, body := postToken(t, s,
		"grant_type=client_credentials&client_id=oac_nolink&client_secret=s3cr3t")

	if status != 400 {
		t.Fatalf("status = %d, want 400: %v", status, body)
	}
	if body["error"] != "unauthorized_client" {
		t.Fatalf("error = %q, want unauthorized_client: %v", body["error"], body)
	}
}
