package auth_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/go-sdk/auth"
)

// fixtureDir holds the shared field-token vectors written by
// tools/field-fixtures. It sits outside this module, so a standalone checkout
// of the SDK (the published mirror) does not have it; the test then SKIPS with
// that message rather than passing silently.
var fixtureDir = filepath.Join("..", "..", "..", "api", "field-fixtures")

type fixtureCase struct {
	Name        string            `json:"name"`
	AccessToken string            `json:"accessToken"`
	DPoPProof   string            `json:"dpopProof"`
	Method      string            `json:"method"`
	URL         string            `json:"url"`
	Now         string            `json:"now"`
	Expect      string            `json:"expect"`
	Principal   *fixturePrincipal `json:"principal"`
}

// fixturePrincipal is what a verifier must expose for a valid case that lists
// it (the sign-in-options claims: no device on an unenrolled key, an actor
// when someone is signed in as the worker, test identities).
type fixturePrincipal struct {
	DeviceID   string   `json:"deviceId"`
	DeviceMode string   `json:"deviceMode"`
	AMR        []string `json:"amr"`
	Actor      *struct {
		ID   string `json:"id"`
		Type string `json:"type"`
		Name string `json:"name"`
	} `json:"actor"`
	Test              bool   `json:"test"`
	LinkedPrincipalID string `json:"linkedPrincipalId"`
}

func loadFixtures(t *testing.T) (issuer, appCode string, jwks []byte, cases []fixtureCase) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(fixtureDir, "cases.json"))
	if errors.Is(err, os.ErrNotExist) {
		t.Skipf("field fixtures not present at %s (standalone SDK checkout)", fixtureDir)
	}
	require.NoError(t, err)
	var f struct {
		Issuer  string        `json:"issuer"`
		AppCode string        `json:"appCode"`
		Cases   []fixtureCase `json:"cases"`
	}
	require.NoError(t, json.Unmarshal(raw, &f))
	jwks, err = os.ReadFile(filepath.Join(fixtureDir, "jwks.json"))
	require.NoError(t, err)
	return f.Issuer, f.AppCode, jwks, f.Cases
}

// TestFieldFixtures runs every shared vector through FieldTokenVerifier.
func TestFieldFixtures(t *testing.T) {
	issuer, appCode, jwks, cases := loadFixtures(t)
	require.NotEmpty(t, cases)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(jwks)
	}))
	t.Cleanup(srv.Close)

	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) {
			now, err := time.Parse(time.RFC3339, c.Now)
			require.NoError(t, err)
			v := auth.NewFieldTokenVerifier(auth.FieldTokenVerifierConfig{
				Issuer: issuer, AppCode: appCode, JWKSURL: srv.URL, Now: func() time.Time { return now },
			})
			p, err := v.Verify(context.Background(), "DPoP "+c.AccessToken, c.DPoPProof, c.Method, c.URL)
			if c.Expect == "valid" {
				require.NoError(t, err)
				assert.Equal(t, "fwu_0FIXTUREWORKER", p.WorkerID)
				assert.Equal(t, "fixture-client", p.ClientIdentifier)
				assert.Equal(t, "W001", p.WorkerCode)
				assert.Equal(t, appCode, p.AppCode)
				if want := c.Principal; want != nil {
					assert.Equal(t, want.DeviceID, p.DeviceID)
					assert.Equal(t, want.DeviceMode, p.DeviceMode)
					assert.Equal(t, want.AMR, p.AMR)
					assert.Equal(t, want.Test, p.Test)
					assert.Equal(t, want.LinkedPrincipalID, p.LinkedPrincipalID)
					if want.Actor == nil {
						assert.Nil(t, p.Actor)
					} else if assert.NotNil(t, p.Actor) {
						assert.Equal(t, want.Actor.ID, p.Actor.ID)
						assert.Equal(t, want.Actor.Type, p.Actor.Type)
						assert.Equal(t, want.Actor.Name, p.Actor.Name)
					}
				}
				return
			}
			require.Error(t, err)
			var fe *auth.FieldError
			require.ErrorAs(t, err, &fe)
			assert.Equal(t, c.Expect, fe.Reason)
			assert.ErrorIs(t, err, auth.ErrInvalidToken)
		})
	}
}

// TestFieldVerifierRefusesBearerScheme: a DPoP-bound token must not be
// accepted under the Bearer scheme (that would skip the proof).
func TestFieldVerifierRefusesBearerScheme(t *testing.T) {
	issuer, appCode, jwks, cases := loadFixtures(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(jwks) }))
	t.Cleanup(srv.Close)
	c := cases[0]
	now, _ := time.Parse(time.RFC3339, c.Now)
	v := auth.NewFieldTokenVerifier(auth.FieldTokenVerifierConfig{
		Issuer: issuer, AppCode: appCode, JWKSURL: srv.URL, Now: func() time.Time { return now },
	})
	_, err := v.Verify(context.Background(), "Bearer "+c.AccessToken, c.DPoPProof, c.Method, c.URL)
	var fe *auth.FieldError
	require.ErrorAs(t, err, &fe)
	assert.Equal(t, auth.FieldReasonMalformed, fe.Reason)
}

// TestFieldMiddleware: the valid vector reaches the handler with the principal
// in the context; a refused one is a 401 naming the reason.
func TestFieldMiddleware(t *testing.T) {
	issuer, appCode, jwks, cases := loadFixtures(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(jwks) }))
	t.Cleanup(srv.Close)
	byName := map[string]fixtureCase{}
	for _, c := range cases {
		byName[c.Name] = c
	}
	valid := byName["valid"]
	now, _ := time.Parse(time.RFC3339, valid.Now)
	v := auth.NewFieldTokenVerifier(auth.FieldTokenVerifierConfig{
		Issuer: issuer, AppCode: appCode, JWKSURL: srv.URL, BaseURL: "https://app.example.test",
		Now: func() time.Time { return now },
	})
	var got *auth.FieldPrincipal
	h := auth.FieldMiddleware(v)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got, _ = auth.FieldPrincipalFrom(r.Context())
		w.WriteHeader(http.StatusNoContent)
	}))

	do := func(c fixtureCase) *httptest.ResponseRecorder {
		req := httptest.NewRequest(c.Method, "/api/orders", nil)
		req.Header.Set("Authorization", "DPoP "+c.AccessToken)
		req.Header.Set("DPoP", c.DPoPProof)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}
	rec := do(valid)
	require.Equal(t, http.StatusNoContent, rec.Code)
	require.NotNil(t, got)
	assert.Equal(t, "W001", got.WorkerCode)

	rec = do(byName["pcr_pin_change_only"])
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.JSONEq(t, `{"error":"pcr"}`, rec.Body.String())
}

// TestFieldVerifierPinsRS256: the access-token alg is RS256 only. Tokens whose
// header says HS256 or none are refused as invalid_token before any key is
// consulted (ES256 is the shared alg_not_rs256 vector).
func TestFieldVerifierPinsRS256(t *testing.T) {
	issuer, appCode, jwks, cases := loadFixtures(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(jwks) }))
	t.Cleanup(srv.Close)
	c := cases[0]
	now, _ := time.Parse(time.RFC3339, c.Now)
	v := auth.NewFieldTokenVerifier(auth.FieldTokenVerifierConfig{
		Issuer: issuer, AppCode: appCode, JWKSURL: srv.URL, Now: func() time.Time { return now },
	})
	enc := base64.RawURLEncoding.EncodeToString
	parts := strings.Split(c.AccessToken, ".")
	for _, alg := range []string{"HS256", "none", "ES256"} {
		hdr := enc([]byte(`{"alg":"` + alg + `","typ":"fc-field+jwt","kid":"fixture-key"}`))
		tok := hdr + "." + parts[1] + "." + parts[2]
		_, err := v.Verify(context.Background(), "DPoP "+tok, c.DPoPProof, c.Method, c.URL)
		var fe *auth.FieldError
		require.ErrorAs(t, err, &fe, alg)
		assert.Equal(t, auth.FieldReasonInvalidToken, fe.Reason, alg)
		assert.Contains(t, fe.Error(), "RS256", alg)
	}
}

// TestFieldMiddlewareRequiresBaseURL: no BaseURL is a construction-time panic,
// never a fallback to the Host header.
func TestFieldMiddlewareRequiresBaseURL(t *testing.T) {
	v := auth.NewFieldTokenVerifier(auth.FieldTokenVerifierConfig{Issuer: "https://fc.example.test", AppCode: "a"})
	assert.Panics(t, func() { auth.FieldMiddleware(v) })
}
