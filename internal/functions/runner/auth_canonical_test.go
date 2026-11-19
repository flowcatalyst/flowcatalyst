package runner

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/flowcatalyst/flowcatalyst-go/internal/functions/control"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ids"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth/authservice"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth/compactjws/compactjwstest"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
)

// platformIssuer serves the discovery document and JWKS of a platform signing
// service, as the platform does, and returns the service and its issuer (the
// server's URL).
func platformIssuer(t *testing.T) (*authservice.AuthService, string) {
	t.Helper()
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	keyPEM := string(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}))
	svc, err := authservice.New(authservice.Config{Issuer: srv.URL, Audience: "fc-test", RSAPrivateKeyPEM: keyPEM})
	if err != nil {
		t.Fatal(err)
	}
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"issuer": srv.URL, "jwks_uri": srv.URL + "/.well-known/jwks.json"})
	})
	mux.HandleFunc("/.well-known/jwks.json", func(w http.ResponseWriter, _ *http.Request) {
		keys := []map[string]string{}
		for _, k := range svc.AllJWKSKeys() {
			keys = append(keys, map[string]string{"kty": "RSA", "use": "sig", "alg": "RS256", "kid": k.KeyID, "n": k.Components.N, "e": k.Components.E})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"keys": keys})
	})
	return svc, srv.URL
}

// TestJWKSVerifier_AcceptsOnlyTheIssuedSpelling: the runner verifies platform
// access tokens through the Go SDK, whose JWT library reads padded,
// standard-alphabet and nonzero-spare-bit spellings as the same token; the
// runner refuses all of them, as the platform's own verifiers do.
func TestJWKSVerifier_AcceptsOnlyTheIssuedSpelling(t *testing.T) {
	svc, issuer := platformIssuer(t)
	p := principal.NewUser("alice@example.com", principal.ScopeAnchor)
	p.ID = ids.PrincipalID("prn_0ALICE000000")
	tok, err := svc.GenerateAccessToken(p)
	if err != nil {
		t.Fatal(err)
	}
	j := &jwksVerifier{}
	j.configure(control.TokenAuth{Issuer: issuer, Audience: "fc-test"})
	k := compactjwstest.Kind{
		Token: tok,
		Verify: func(token string) error {
			_, err := j.Verify(context.Background(), "Bearer "+token)
			return err
		},
		Claim: "prn_0ALICE000000", OtherClaim: "prn_0MALLORY0000",
	}
	t.Run("tampered", func(t *testing.T) { compactjwstest.Tampered(t, k) })
	t.Run("respelled", func(t *testing.T) { compactjwstest.NonCanonical(t, k) })
}
