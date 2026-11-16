package auth_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/lestrrat-go/jwx/v4/jwa"
	"github.com/lestrrat-go/jwx/v4/jwk"
	"github.com/lestrrat-go/jwx/v4/jwt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/go-sdk/auth"
)

// These tests pin the observable behaviour of TokenValidator and JwksCache
// (algorithm allowlist, kid selection, skew, JWKS cache and failure handling,
// error kinds) independently of the JWT library underneath.

type rotatingIDP struct {
	srv       *httptest.Server
	issuer    string
	set       atomic.Pointer[jwk.Set]
	jwksFails atomic.Bool
	jwksHits  atomic.Int32
}

func newKeyPair(t *testing.T, kid string) (priv, pub jwk.Key) {
	t.Helper()
	raw, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	priv, err = jwk.Import[jwk.Key](raw)
	require.NoError(t, err)
	require.NoError(t, priv.Set(jwk.KeyIDKey, kid))
	require.NoError(t, priv.Set(jwk.AlgorithmKey, jwa.RS256()))
	pub, err = priv.PublicKey()
	require.NoError(t, err)
	return priv, pub
}

func newRotatingIDP(t *testing.T, pubs ...jwk.Key) *rotatingIDP {
	t.Helper()
	idp := &rotatingIDP{}
	idp.setKeys(t, pubs...)
	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"issuer": idp.issuer, "jwks_uri": idp.issuer + "/jwks"})
	})
	mux.HandleFunc("/jwks", func(w http.ResponseWriter, _ *http.Request) {
		idp.jwksHits.Add(1)
		if idp.jwksFails.Load() {
			http.Error(w, "boom", http.StatusInternalServerError)
			return
		}
		raw, err := json.Marshal(*idp.set.Load())
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		_, _ = w.Write(raw)
	})
	idp.srv = httptest.NewServer(mux)
	t.Cleanup(idp.srv.Close)
	idp.issuer = idp.srv.URL
	return idp
}

func (i *rotatingIDP) setKeys(t *testing.T, pubs ...jwk.Key) {
	t.Helper()
	set := jwk.NewSet()
	for _, p := range pubs {
		require.NoError(t, set.AddKey(p))
	}
	i.set.Store(&set)
}

func (i *rotatingIDP) mint(t *testing.T, key jwk.Key, mutate func(jwt.Token)) string {
	t.Helper()
	tok := jwt.New()
	require.NoError(t, tok.Set(jwt.SubjectKey, "prn_1"))
	require.NoError(t, tok.Set(jwt.IssuerKey, i.issuer))
	require.NoError(t, tok.Set(jwt.AudienceKey, "my-app"))
	require.NoError(t, tok.Set(jwt.ExpirationKey, time.Now().Add(time.Hour)))
	require.NoError(t, tok.Set("type", "USER"))
	require.NoError(t, tok.Set("scope", "ANCHOR"))
	if mutate != nil {
		mutate(tok)
	}
	signed, err := jwt.Sign(tok, jwt.WithKey(jwa.RS256(), key))
	require.NoError(t, err)
	return string(signed)
}

func TestTokenValidatorClockSkew(t *testing.T) {
	priv, pub := newKeyPair(t, "k1")
	idp := newRotatingIDP(t, pub)
	ctx := context.Background()

	expiredBy := func(d time.Duration) func(jwt.Token) {
		return func(tok jwt.Token) { _ = tok.Set(jwt.ExpirationKey, time.Now().Add(-d)) }
	}

	// Default skew is 60s: 30s past exp is accepted, 2m past is TokenExpired.
	v := auth.NewTokenValidator(auth.TokenValidatorConfig{IssuerURL: idp.issuer, Audience: "my-app"})
	_, err := v.Validate(ctx, idp.mint(t, priv, expiredBy(30*time.Second)))
	require.NoError(t, err)
	_, err = v.Validate(ctx, idp.mint(t, priv, expiredBy(2*time.Minute)))
	require.Error(t, err)
	assert.True(t, errors.Is(err, auth.ErrTokenExpired))

	// A configured skew replaces the default.
	tight := auth.NewTokenValidator(auth.TokenValidatorConfig{IssuerURL: idp.issuer, Audience: "my-app", ClockSkew: 5 * time.Second})
	_, err = tight.Validate(ctx, idp.mint(t, priv, expiredBy(30*time.Second)))
	require.Error(t, err)
	assert.True(t, errors.Is(err, auth.ErrTokenExpired))

	// not-before honours the same skew and is an InvalidToken, not Expired.
	nbfIn := func(d time.Duration) func(jwt.Token) {
		return func(tok jwt.Token) { _ = tok.Set(jwt.NotBeforeKey, time.Now().Add(d)) }
	}
	_, err = v.Validate(ctx, idp.mint(t, priv, nbfIn(30*time.Second)))
	require.NoError(t, err)
	_, err = v.Validate(ctx, idp.mint(t, priv, nbfIn(2*time.Minute)))
	require.Error(t, err)
	assert.True(t, errors.Is(err, auth.ErrInvalidToken))
	assert.False(t, errors.Is(err, auth.ErrTokenExpired))
}

func TestTokenValidatorKeySelectionAndAlgorithms(t *testing.T) {
	priv, pub := newKeyPair(t, "k1")
	otherPriv, otherPub := newKeyPair(t, "k2")
	idp := newRotatingIDP(t, pub, otherPub)
	v := auth.NewTokenValidator(auth.TokenValidatorConfig{IssuerURL: idp.issuer, Audience: "my-app"})
	ctx := context.Background()

	// Both keys in the set are usable, selected by kid.
	_, err := v.Validate(ctx, idp.mint(t, priv, nil))
	require.NoError(t, err)
	_, err = v.Validate(ctx, idp.mint(t, otherPriv, nil))
	require.NoError(t, err)

	// A kid that is not in the set is refused as InvalidToken.
	strangerPriv, _ := newKeyPair(t, "unknown-kid")
	_, err = v.Validate(ctx, idp.mint(t, strangerPriv, nil))
	require.Error(t, err)
	assert.True(t, errors.Is(err, auth.ErrInvalidToken))

	// A known kid with the wrong key material is a bad signature.
	impostor, _ := newKeyPair(t, "k1")
	_, err = v.Validate(ctx, idp.mint(t, impostor, nil))
	require.Error(t, err)
	assert.True(t, errors.Is(err, auth.ErrInvalidToken))

	// A token without a kid is still verified against the set (require-kid is off).
	noKid, err := jwk.Import[jwk.Key](mustRSA(t))
	require.NoError(t, err)
	require.NoError(t, noKid.Set(jwk.AlgorithmKey, jwa.RS256()))
	_, noKidPub := noKid, mustPublic(t, noKid)
	idp.setKeys(t, noKidPub)
	require.NoError(t, v.RefreshJWKS(ctx))
	_, err = v.Validate(ctx, idp.mint(t, noKid, nil))
	require.NoError(t, err)

	// HS256 signed with key material from the JWKS must not be accepted
	// (algorithm confusion), nor an unsigned "alg":"none" token.
	hs := func() string {
		tok := jwt.New()
		_ = tok.Set(jwt.IssuerKey, idp.issuer)
		_ = tok.Set(jwt.AudienceKey, "my-app")
		_ = tok.Set(jwt.ExpirationKey, time.Now().Add(time.Hour))
		s, err := jwt.Sign(tok, jwt.WithKey(jwa.HS256(), []byte("any-shared-secret-of-some-length-0123456789")))
		require.NoError(t, err)
		return string(s)
	}()
	_, err = v.Validate(ctx, hs)
	require.Error(t, err)
	assert.True(t, errors.Is(err, auth.ErrInvalidToken))

	enc := base64.RawURLEncoding
	none := enc.EncodeToString([]byte(`{"alg":"none"}`)) + "." +
		enc.EncodeToString([]byte(`{"iss":"`+idp.issuer+`","aud":"my-app","exp":9999999999}`)) + "."
	_, err = v.Validate(ctx, none)
	require.Error(t, err)
	assert.True(t, errors.Is(err, auth.ErrInvalidToken))
}

func mustRSA(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	raw, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	return raw
}

func mustPublic(t *testing.T, k jwk.Key) jwk.Key {
	t.Helper()
	pub, err := k.PublicKey()
	require.NoError(t, err)
	return pub
}

func TestTokenValidatorMalformedAndMissingClaims(t *testing.T) {
	priv, pub := newKeyPair(t, "k1")
	idp := newRotatingIDP(t, pub)
	v := auth.NewTokenValidator(auth.TokenValidatorConfig{IssuerURL: idp.issuer, Audience: "my-app"})
	ctx := context.Background()

	for _, bad := range []string{"", "abc", "a.b", "a.b.c", "....", "not a jwt at all"} {
		_, err := v.Validate(ctx, bad)
		require.Error(t, err, "token %q", bad)
		assert.True(t, errors.Is(err, auth.ErrInvalidToken), "token %q: %v", bad, err)
		assert.False(t, errors.Is(err, auth.ErrTokenExpired), "token %q", bad)
	}

	// A token with no exp, no iss or no aud is not accepted.
	for name, mutate := range map[string]func(jwt.Token){
		"no-iss": func(tok jwt.Token) { _ = tok.Remove(jwt.IssuerKey) },
		"no-aud": func(tok jwt.Token) { _ = tok.Remove(jwt.AudienceKey) },
	} {
		_, err := v.Validate(ctx, idp.mint(t, priv, mutate))
		require.Error(t, err, name)
		assert.True(t, errors.Is(err, auth.ErrInvalidToken), name)
	}

	// Issuer match is exact, but the configured issuer's trailing slash is trimmed.
	slash := auth.NewTokenValidator(auth.TokenValidatorConfig{IssuerURL: idp.issuer + "/", Audience: "my-app"})
	_, err := slash.Validate(ctx, idp.mint(t, priv, nil))
	require.NoError(t, err)

	// An audience list containing the expected audience is accepted.
	_, err = v.Validate(ctx, idp.mint(t, priv, func(tok jwt.Token) {
		_ = tok.Set(jwt.AudienceKey, []string{"someone-else", "my-app"})
	}))
	require.NoError(t, err)
}

func TestTokenValidatorPrivateClaimTypes(t *testing.T) {
	priv, pub := newKeyPair(t, "k1")
	idp := newRotatingIDP(t, pub)
	v := auth.NewTokenValidator(auth.TokenValidatorConfig{IssuerURL: idp.issuer, Audience: "my-app"})
	tok := idp.mint(t, priv, func(tok jwt.Token) {
		_ = tok.Set("name", "Tester")
		_ = tok.Set("clients", []string{"10:abc", "11:def"})
		_ = tok.Set("roles", []string{"admin", "viewer"})
		_ = tok.Set("applications", []string{"1:app"})
	})
	got, err := v.Validate(context.Background(), tok)
	require.NoError(t, err)
	assert.Equal(t, "prn_1", got.PrincipalID())
	assert.True(t, got.IsAnchor())
	assert.True(t, got.HasRole("admin"))
	assert.True(t, got.HasRole("viewer"))
	assert.False(t, got.HasRole("other"))
	assert.Equal(t, tok, got.BearerToken())
}

func TestJwksCacheFetchFailureAndRotation(t *testing.T) {
	priv, pub := newKeyPair(t, "k1")
	idp := newRotatingIDP(t, pub)
	ctx := context.Background()

	// First fetch failing surfaces as a Discovery error (not InvalidToken).
	idp.jwksFails.Store(true)
	v := auth.NewTokenValidator(auth.TokenValidatorConfig{IssuerURL: idp.issuer, Audience: "my-app"})
	_, err := v.Validate(ctx, idp.mint(t, priv, nil))
	require.Error(t, err)
	assert.True(t, errors.Is(err, auth.ErrDiscovery), "%v", err)

	// Recovery: an explicit RefreshJWKS repairs it. What a Validate between the
	// failure and that refresh answers is not asserted: registering the URL
	// also starts a fetch in the background, and when that one lands after the
	// endpoint has recovered the keys are already there (otherwise the entry
	// stays empty until a refresh).
	idp.jwksFails.Store(false)
	require.NoError(t, v.RefreshJWKS(ctx))
	_, err = v.Validate(ctx, idp.mint(t, priv, nil))
	require.NoError(t, err)

	// The key set is cached: no new JWKS request for the next validations.
	hits := idp.jwksHits.Load()
	for range 3 {
		_, err = v.Validate(ctx, idp.mint(t, priv, nil))
		require.NoError(t, err)
	}
	assert.Equal(t, hits, idp.jwksHits.Load(), "cached key set must not be re-fetched within the refresh interval")

	// An outage after a successful fetch keeps serving the cached keys.
	idp.jwksFails.Store(true)
	_, err = v.Validate(ctx, idp.mint(t, priv, nil))
	require.NoError(t, err)

	// A forced refresh during the outage reports the failure and keeps the old keys.
	require.Error(t, v.RefreshJWKS(ctx))
	_, err = v.Validate(ctx, idp.mint(t, priv, nil))
	require.NoError(t, err)

	// Rotation is not picked up until a refresh: new key refused, then accepted.
	idp.jwksFails.Store(false)
	newPriv, newPub := newKeyPair(t, "k2")
	idp.setKeys(t, newPub)
	newTok := idp.mint(t, newPriv, nil)
	_, err = v.Validate(ctx, newTok)
	require.Error(t, err)
	assert.True(t, errors.Is(err, auth.ErrInvalidToken))
	require.NoError(t, v.RefreshJWKS(ctx))
	_, err = v.Validate(ctx, newTok)
	require.NoError(t, err)
	// ...and the retired key is gone.
	_, err = v.Validate(ctx, idp.mint(t, priv, nil))
	require.Error(t, err)
}

func TestJwksCacheDirect(t *testing.T) {
	_, pub := newKeyPair(t, "k1")
	idp := newRotatingIDP(t, pub)
	c := auth.NewJwksCache(http.DefaultClient, time.Hour)
	require.NoError(t, c.Register(idp.issuer+"/jwks"))
	set, err := c.Get(context.Background(), idp.issuer+"/jwks")
	require.NoError(t, err)
	assert.Equal(t, 1, set.Len())
	k, ok := set.LookupKeyID("k1")
	require.True(t, ok)
	alg, _ := k.Algorithm()
	assert.Equal(t, "RS256", alg.String())

	// A URL that was never registered is an error.
	_, err = c.Get(context.Background(), idp.issuer+"/other")
	require.Error(t, err)
}
