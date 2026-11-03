// field-fixtures writes the shared field-token test vectors to
// api/field-fixtures/: a JWKS and a list of cases
//
//	{name, accessToken, dpopProof, method, url, now, expect}
//
// where expect is "valid" or the refusal reason ("wrong_typ", "pcr", ...).
// Every SDK's field-token verifier runs every case.
//
// The output is deterministic: fixed keys (derived from fixed seeds), a fixed
// clock and a deterministic ES256 signer, so re-running the generator
// reproduces the committed files byte for byte. The keys are test keys, public
// by design.
//
//	go run ./tools/field-fixtures            # writes api/field-fixtures/
//	go run ./tools/field-fixtures -out DIR
package main

import (
	"crypto/ecdh"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"time"
)

const (
	issuer  = "https://fc.example.test"
	appCode = "fixture-app"
	kid     = "fixture-key"
)

// fixedNow is the verifier clock for every case unless a case overrides it.
var fixedNow = time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

type fixtureKey struct {
	d      []byte
	x, y   string
	jkt    string
	jwkMap map[string]string
}

func newKey(seed string) fixtureKey {
	sum := sha256.Sum256([]byte(seed))
	n := elliptic.P256().Params().N
	d := new(big.Int).SetBytes(sum[:])
	d.Mod(d, new(big.Int).Sub(n, big.NewInt(1)))
	d.Add(d, big.NewInt(1))
	dBytes := d.FillBytes(make([]byte, 32))
	priv, err := ecdh.P256().NewPrivateKey(dBytes)
	if err != nil {
		panic(err)
	}
	pt := priv.PublicKey().Bytes()
	x := base64.RawURLEncoding.EncodeToString(pt[1:33])
	y := base64.RawURLEncoding.EncodeToString(pt[33:65])
	canonical := `{"crv":"P-256","kty":"EC","x":"` + x + `","y":"` + y + `"}`
	tp := sha256.Sum256([]byte(canonical))
	return fixtureKey{
		d: dBytes, x: x, y: y, jkt: base64.RawURLEncoding.EncodeToString(tp[:]),
		jwkMap: map[string]string{"kty": "EC", "crv": "P-256", "x": x, "y": y},
	}
}

// sign returns a deterministic ES256 (r||s) signature over the SHA-256 of
// input. The nonce k is HMAC-SHA256(d, digest) folded into [1, n-1]: stable
// across runs, adequate for public test keys, and never used for real keys.
func (k fixtureKey) sign(input string) []byte {
	n := elliptic.P256().Params().N
	digest := sha256.Sum256([]byte(input))
	z := new(big.Int).SetBytes(digest[:])
	d := new(big.Int).SetBytes(k.d)
	for counter := byte(0); ; counter++ {
		mac := hmac.New(sha256.New, k.d)
		mac.Write(digest[:])
		mac.Write([]byte{counter})
		kk := new(big.Int).SetBytes(mac.Sum(nil))
		kk.Mod(kk, new(big.Int).Sub(n, big.NewInt(1)))
		kk.Add(kk, big.NewInt(1))
		kp, err := ecdh.P256().NewPrivateKey(kk.FillBytes(make([]byte, 32)))
		if err != nil {
			panic(err)
		}
		r := new(big.Int).SetBytes(kp.PublicKey().Bytes()[1:33])
		r.Mod(r, n)
		if r.Sign() == 0 {
			continue
		}
		s := new(big.Int).Mul(r, d)
		s.Add(s, z)
		s.Mul(s, new(big.Int).ModInverse(kk, n))
		s.Mod(s, n)
		if s.Sign() == 0 {
			continue
		}
		sig := append(r.FillBytes(make([]byte, 32)), s.FillBytes(make([]byte, 32))...)
		if !ecdsa.Verify(k.public(), digest[:], r, s) {
			panic("fixture signer produced an invalid signature")
		}
		return sig
	}
}

func (k fixtureKey) public() *ecdsa.PublicKey {
	pt := append([]byte{4}, decode(k.x)...)
	pt = append(pt, decode(k.y)...)
	pub, err := ecdsa.ParseUncompressedPublicKey(elliptic.P256(), pt)
	if err != nil {
		panic(err)
	}
	return pub
}

func decode(s string) []byte {
	b, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		panic(err)
	}
	return b
}

func b64(v any) string {
	raw, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return base64.RawURLEncoding.EncodeToString(raw)
}

// jws signs header.claims with key (ES256).
func jws(key fixtureKey, header, claims map[string]any) string {
	input := b64(header) + "." + b64(claims)
	return input + "." + base64.RawURLEncoding.EncodeToString(key.sign(input))
}

type tokenOpts struct {
	typ      string
	tokenUse string
	aud      string
	exp      time.Time
	pcr      bool
	jkt      string
}

func accessToken(signer fixtureKey, o tokenOpts) string {
	claims := map[string]any{
		"iss": issuer, "sub": "fwu_0FIXTUREWORKER", "aud": o.aud,
		"exp": o.exp.Unix(), "iat": fixedNow.Add(-time.Minute).Unix(), "nbf": fixedNow.Add(-time.Minute).Unix(),
		"jti": "fixture-jti", "token_use": o.tokenUse,
		"client_id": "clt_0FIXTURECLIENT", "client_identifier": "fixture-client",
		"worker_code": "W001", "name": "Fixture Worker",
		"device_id": "fwd_0FIXTUREDEVICE", "device_mode": "PERSONAL", "sid": "fws_0FIXTUREFAMILY",
		"amr": []string{"pin"}, "auth_time": fixedNow.Add(-5 * time.Minute).Unix(),
		"cnf": map[string]string{"jkt": o.jkt},
	}
	if o.pcr {
		claims["pcr"] = true
	}
	return jws(signer, map[string]any{"alg": "ES256", "typ": o.typ, "kid": kid}, claims)
}

type proofOpts struct {
	htm, htu string
	iat      time.Time
	ath      string
}

func dpopProof(device fixtureKey, o proofOpts) string {
	return jws(device,
		map[string]any{"alg": "ES256", "typ": "dpop+jwt", "jwk": device.jwkMap},
		map[string]any{"htm": o.htm, "htu": o.htu, "iat": o.iat.Unix(), "jti": "fixture-proof-" + o.ath[:6], "ath": o.ath})
}

func ath(token string) string {
	sum := sha256.Sum256([]byte(token))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

type fixtureCase struct {
	Name        string `json:"name"`
	AccessToken string `json:"accessToken"`
	DPoPProof   string `json:"dpopProof"`
	Method      string `json:"method"`
	URL         string `json:"url"`
	Now         string `json:"now"`
	Expect      string `json:"expect"`
}

type fixtureFile struct {
	Description string        `json:"description"`
	Issuer      string        `json:"issuer"`
	AppCode     string        `json:"appCode"`
	Cases       []fixtureCase `json:"cases"`
}

func generate() map[string][]byte {
	signer := newKey("fc-field-fixture-token-signing-key")
	device := newKey("fc-field-fixture-device-key")
	other := newKey("fc-field-fixture-other-device-key")
	const method = "POST"
	const target = "https://app.example.test/api/orders"
	valid := tokenOpts{typ: "fc-field+jwt", tokenUse: "field", aud: appCode, exp: fixedNow.Add(10 * time.Minute), jkt: device.jkt}

	mk := func(name, expect string, o tokenOpts, signWith fixtureKey, po proofOpts, transform func(token, proof string) (string, string)) fixtureCase {
		tok := accessToken(signer, o)
		if po.ath == "" {
			po.ath = ath(tok)
		}
		if po.htm == "" {
			po.htm = method
		}
		if po.htu == "" {
			po.htu = target
		}
		if po.iat.IsZero() {
			po.iat = fixedNow
		}
		proof := dpopProof(signWith, po)
		if transform != nil {
			tok, proof = transform(tok, proof)
		}
		return fixtureCase{
			Name: name, AccessToken: tok, DPoPProof: proof, Method: method, URL: target,
			Now: fixedNow.Format(time.RFC3339), Expect: expect,
		}
	}
	with := func(f func(*tokenOpts)) tokenOpts { o := valid; f(&o); return o }

	cases := []fixtureCase{
		mk("valid", "valid", valid, device, proofOpts{}, nil),
		mk("valid_url_with_query_ignored", "valid", valid, device, proofOpts{}, nil),
		mk("wrong_typ", "wrong_typ", with(func(o *tokenOpts) { o.typ = "JWT" }), device, proofOpts{}, nil),
		mk("wrong_token_use", "wrong_token_use", with(func(o *tokenOpts) { o.tokenUse = "api" }), device, proofOpts{}, nil),
		mk("wrong_aud", "wrong_aud", with(func(o *tokenOpts) { o.aud = "some-other-app" }), device, proofOpts{}, nil),
		mk("expired", "expired", with(func(o *tokenOpts) { o.exp = fixedNow.Add(-time.Hour) }), device, proofOpts{}, nil),
		mk("pcr_pin_change_only", "pcr", with(func(o *tokenOpts) { o.pcr = true }), device, proofOpts{}, nil),
		mk("missing_proof", "missing_proof", valid, device, proofOpts{}, func(t, _ string) (string, string) { return t, "" }),
		mk("jkt_mismatch_proof_from_other_device", "jkt_mismatch", valid, other, proofOpts{}, nil),
		mk("htm_mismatch", "htm_mismatch", valid, device, proofOpts{htm: "GET"}, nil),
		mk("htu_mismatch", "htu_mismatch", valid, device, proofOpts{htu: "https://app.example.test/api/other"}, nil),
		mk("ath_mismatch", "ath_mismatch", valid, device, proofOpts{ath: ath("a different access token")}, nil),
		mk("stale_proof", "stale_proof", valid, device, proofOpts{iat: fixedNow.Add(-time.Hour)}, nil),
		mk("api_token_presented_as_field_token", "wrong_typ",
			with(func(o *tokenOpts) { o.typ = "JWT"; o.tokenUse = "api"; o.aud = "flowcatalyst" }), device, proofOpts{}, nil),
		mk("tampered_token_signature", "invalid_token", valid, device, proofOpts{}, func(t, p string) (string, string) {
			// Re-point the signature at a different value: structurally valid, wrong key.
			parts := splitToken(t)
			return parts[0] + "." + parts[1] + "." + parts[2][:len(parts[2])-4] + "AAAA", p
		}),
	}
	// The query-string case: same token and proof, URL carries a query.
	for i := range cases {
		if cases[i].Name == "valid_url_with_query_ignored" {
			cases[i].URL = target + "?page=2&sort=asc"
		}
	}

	jwks, err := json.MarshalIndent(map[string]any{"keys": []map[string]string{
		{"kty": "EC", "crv": "P-256", "x": signer.x, "y": signer.y, "kid": kid, "use": "sig", "alg": "ES256"},
	}}, "", "  ")
	if err != nil {
		panic(err)
	}
	body, err := json.MarshalIndent(fixtureFile{
		Description: "Field access-token vectors (docs/wire-contract.md, Field identity). The verifier is configured with the issuer, appCode and the JWKS next to this file; each case is verified at its own now. expect is \"valid\" or the refusal reason. Generated by tools/field-fixtures; do not edit.",
		Issuer:      issuer, AppCode: appCode, Cases: cases,
	}, "", "  ")
	if err != nil {
		panic(err)
	}
	return map[string][]byte{"jwks.json": append(jwks, '\n'), "cases.json": append(body, '\n')}
}

func splitToken(t string) [3]string {
	var out [3]string
	i, j := 0, 0
	for n := range 3 {
		for j < len(t) && t[j] != '.' {
			j++
		}
		out[n] = t[i:j]
		j++
		i = j
	}
	return out
}

func main() {
	out := flag.String("out", filepath.Join("api", "field-fixtures"), "output directory")
	flag.Parse()
	if err := os.MkdirAll(*out, 0o750); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	for name, data := range generate() {
		if err := os.WriteFile(filepath.Join(*out, name), data, 0o600); err != nil { //nolint:gosec // public test vectors
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
}
