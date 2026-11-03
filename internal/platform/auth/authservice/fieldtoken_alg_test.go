package authservice

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"errors"
	"testing"

	"github.com/golang-jwt/jwt/v5"
)

// TestValidateFieldTokenPinsTheSigningAlgorithm: the service signs field
// tokens RS256 and must refuse a token of any other alg, including HS256 keyed
// with the public key (the classic confusion) and ES256.
func TestValidateFieldTokenPinsTheSigningAlgorithm(t *testing.T) {
	priv, pub := genRSAPEMs(t)
	svc, err := NewWithRSA(Config{Issuer: "https://fc.example.test"}, priv, pub)
	if err != nil {
		t.Fatal(err)
	}
	good, err := svc.GenerateFieldAccessToken(FieldSubject{WorkerID: "fwu_1", AppCode: "app", ClientID: "clt_1"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ValidateFieldToken(good); err != nil {
		t.Fatalf("own RS256 token refused: %v", err)
	}

	claims := FieldAccessTokenClaims{
		Issuer:   "https://fc.example.test",
		TokenUse: TokenUseField, Aud: "app",
	}
	ec, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	for name, forge := range map[string]func() (string, error){
		"HS256": func() (string, error) {
			tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
			tok.Header["typ"] = FieldTokenType
			return tok.SignedString([]byte(pub))
		},
		"ES256": func() (string, error) {
			tok := jwt.NewWithClaims(jwt.SigningMethodES256, claims)
			tok.Header["typ"] = FieldTokenType
			return tok.SignedString(ec)
		},
		"none": func() (string, error) {
			tok := jwt.NewWithClaims(jwt.SigningMethodNone, claims)
			tok.Header["typ"] = FieldTokenType
			return tok.SignedString(jwt.UnsafeAllowNoneSignatureType)
		},
	} {
		t.Run(name, func(t *testing.T) {
			tok, err := forge()
			if err != nil {
				t.Fatal(err)
			}
			if _, err := svc.ValidateFieldToken(tok); !errors.Is(err, ErrInvalidToken) {
				t.Fatalf("got %v, want ErrInvalidToken", err)
			}
		})
	}
}
