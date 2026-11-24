package bridge

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1" //nolint:gosec // x5t is defined as the SHA-1 thumbprint (RFC 7515); it is an identifier, not a security control.
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/oauth2"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/identityprovider"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/encryption"
)

// RFC 7523 client authentication (private_key_jwt): the token request carries a
// client_assertion JWT where it would otherwise carry client_secret.
const (
	clientAssertionType = "urn:ietf:params:oauth:client-assertion-type:jwt-bearer"
	// assertionTTL is how long a minted assertion is valid. One is minted per
	// exchange, so this only has to outlast the request and clock skew.
	assertionTTL = 5 * time.Minute
	// FederatedAudience is the audience Entra expects of a federated
	// credential's token.
	FederatedAudience = "api://AzureADTokenExchange"
)

// FederatedSubjectPrefix prefixes the subject of a federated assertion; the
// subject is FederatedSubjectPrefix + the identity provider's id.
const FederatedSubjectPrefix = "identity-provider:"

func federatedSubject(idpID string) string { return FederatedSubjectPrefix + idpID }

// AssertionSigner mints the client assertion of a FEDERATED provider with the
// platform's signing key (authservice.AuthService.SignClientAssertion).
type AssertionSigner interface {
	SignClientAssertion(sub, aud string, ttl time.Duration) (string, error)
}

// clientAssertionSource yields the token-request parameters that authenticate
// the client. It is called once per exchange.
type clientAssertionSource interface {
	options(tokenURL string, now time.Time) ([]oauth2.AuthCodeOption, error)
}

func assertionOptions(jwtStr string) []oauth2.AuthCodeOption {
	return []oauth2.AuthCodeOption{
		oauth2.SetAuthURLParam("client_assertion_type", clientAssertionType),
		oauth2.SetAuthURLParam("client_assertion", jwtStr),
	}
}

// federatedAssertion is the platform-is-the-issuer credential.
type federatedAssertion struct {
	signer  AssertionSigner
	subject string
}

func (f federatedAssertion) options(string, time.Time) ([]oauth2.AuthCodeOption, error) {
	tok, err := f.signer.SignClientAssertion(f.subject, FederatedAudience, assertionTTL)
	if err != nil {
		return nil, fmt.Errorf("sign federated client assertion: %w", err)
	}
	return assertionOptions(tok), nil
}

// certificateAssertion is the platform-generated-certificate credential.
type certificateAssertion struct {
	key      *rsa.PrivateKey
	clientID string
	x5t      string // base64url(SHA-1(DER))
	x5tS256  string // base64url(SHA-256(DER))
}

// newCertificateAssertion opens the certificate's sealed key (never logging
// it) and precomputes the header thumbprints.
func newCertificateAssertion(enc *encryption.Service, cert *identityprovider.Certificate, clientID string) (certificateAssertion, error) {
	if enc == nil {
		return certificateAssertion{}, errors.New("certificate client authentication needs an encryption service (FLOWCATALYST_APP_KEY)")
	}
	key, err := cert.PrivateKey(enc)
	if err != nil {
		return certificateAssertion{}, err
	}
	der, err := cert.DER()
	if err != nil {
		return certificateAssertion{}, err
	}
	s1 := sha1.Sum(der) //nolint:gosec // see the import
	s256 := sha256.Sum256(der)
	return certificateAssertion{
		key:      key,
		clientID: clientID,
		x5t:      base64.RawURLEncoding.EncodeToString(s1[:]),
		x5tS256:  base64.RawURLEncoding.EncodeToString(s256[:]),
	}, nil
}

func (c certificateAssertion) options(tokenURL string, now time.Time) ([]oauth2.AuthCodeOption, error) {
	tok, err := mintCertificateAssertion(c, tokenURL, now)
	if err != nil {
		return nil, err
	}
	return assertionOptions(tok), nil
}

// mintCertificateAssertion builds Entra's (and RFC 7523's) certificate
// assertion: header alg RS256, typ JWT, x5t and x5t#S256; claims iss = sub =
// client_id, aud = the token endpoint, a random jti, iat, nbf = now and
// exp = now + 5 minutes.
func mintCertificateAssertion(c certificateAssertion, tokenURL string, now time.Time) (string, error) {
	jti := make([]byte, 16)
	if _, err := rand.Read(jti); err != nil {
		return "", fmt.Errorf("client assertion jti: %w", err)
	}
	now = now.UTC()
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
		"iss": c.clientID,
		"sub": c.clientID,
		"aud": tokenURL,
		"jti": hex.EncodeToString(jti),
		"iat": now.Unix(),
		"nbf": now.Unix(),
		"exp": now.Add(assertionTTL).Unix(),
	})
	tok.Header["typ"] = "JWT"
	tok.Header["x5t"] = c.x5t
	tok.Header["x5t#S256"] = c.x5tS256
	signed, err := tok.SignedString(c.key)
	if err != nil {
		return "", fmt.Errorf("sign certificate client assertion: %w", err)
	}
	return signed, nil
}
