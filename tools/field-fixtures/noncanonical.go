package main

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"maps"
	"strings"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth/compactjws/compactjwstest"
)

// The non-canonical vectors (noncanonical.json): the valid case's access token
// and DPoP proof, each spelled another way that a lenient base64url decoder
// reads as the same bytes. A token or proof verifies only in the spelling it
// was issued in (docs/wire-contract.md, "Token spelling"; bug-fix ledger F53),
// so every verifier must refuse every vector: a re-spelled access token as
// "malformed" (it is not RFC 7515 base64url, so not a compact JWS) and a
// re-spelled proof as "invalid_proof".
//
// Each vector is otherwise valid, so only the spelling rule can refuse it: a
// re-spelled signature is the same bytes; a re-spelled header, payload or JWK
// coordinate is signed again, over its new text, with the fixture key; and the
// proof of a re-spelled token carries the ath of that exact text.

const (
	expectTokenRespelled = "malformed"
	expectProofRespelled = "invalid_proof"
)

type respelling struct{ name, text string }

// respell returns the spellings of seg (a canonical base64url segment) that a
// lenient decoder reads as the same bytes: each nonzero value of the spare bits
// of its last character, '=' padding (and, where two belong, a single '='), the
// standard alphabet, and a line feed, carriage return or CRLF in the middle.
func respell(seg string) []respelling {
	var out []respelling
	for i, v := range compactjwstest.NonCanonicalSpellings(seg) {
		out = append(out, respelling{fmt.Sprintf("spare_bits_%d", i+1), v})
	}
	switch len(seg) % 4 {
	case 2: // "==" belongs; one '=' is the other padded form a re-padding decoder reads
		out = append(out, respelling{"padded", seg + "=="}, respelling{"padded_once", seg + "="})
	case 3:
		out = append(out, respelling{"padded", seg + "="})
	}
	if std := strings.NewReplacer("-", "+", "_", "/").Replace(seg); std != seg {
		out = append(out, respelling{"standard_alphabet", std})
	}
	mid := len(seg) / 2
	return append(out,
		respelling{"line_feed", seg[:mid] + "\n" + seg[mid:]},
		respelling{"carriage_return", seg[:mid] + "\r" + seg[mid:]},
		respelling{"crlf", seg[:mid] + "\r\n" + seg[mid:]},
	)
}

func rs256Sign(key *rsa.PrivateKey, header, payload string) string {
	input := header + "." + payload
	digest := sha256.Sum256([]byte(input))
	sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, digest[:])
	if err != nil {
		panic(err)
	}
	return input + "." + base64.RawURLEncoding.EncodeToString(sig)
}

func es256Sign(key fixtureKey, header, payload string) string {
	input := header + "." + payload
	return input + "." + base64.RawURLEncoding.EncodeToString(key.sign(input))
}

// nonCanonicalCases builds the vectors from the valid case's token and proof.
func nonCanonicalCases(signer *rsa.PrivateKey, device fixtureKey, token string, po proofOpts) []fixtureCase {
	now := fixedNow.Format(time.RFC3339)
	proofFor := func(tok string) string {
		o := po
		o.ath = ath(tok)
		return dpopProof(device, o)
	}
	mk := func(name, tok, proof, expect string) fixtureCase {
		return fixtureCase{
			Name: name, AccessToken: tok, DPoPProof: proof, Method: po.htm, URL: po.htu, Now: now, Expect: expect,
		}
	}
	var cases []fixtureCase

	// The access token. Whitespace before or after it is not among them: it is
	// outside the token, in the Authorization header, whose framing a verifier
	// parses as the platform does.
	th := splitToken(token)
	for _, r := range respell(th[2]) {
		tok := th[0] + "." + th[1] + "." + r.text
		cases = append(cases, mk("token_signature_"+r.name, tok, proofFor(tok), expectTokenRespelled))
	}
	for i, seg := range []string{"header", "payload"} {
		for _, r := range respell(th[i]) {
			parts := [2]string{th[0], th[1]}
			parts[i] = r.text
			tok := rs256Sign(signer, parts[0], parts[1])
			cases = append(cases, mk("token_"+seg+"_"+r.name, tok, proofFor(tok), expectTokenRespelled))
		}
	}

	// The proof: the whole DPoP header value, so whitespace around it counts.
	proof := proofFor(token)
	ph := splitToken(proof)
	for _, r := range respell(ph[2]) {
		cases = append(cases, mk("proof_signature_"+r.name, token, ph[0]+"."+ph[1]+"."+r.text, expectProofRespelled))
	}
	cases = append(cases,
		mk("proof_leading_space", token, " "+proof, expectProofRespelled),
		mk("proof_trailing_space", token, proof+" ", expectProofRespelled),
		mk("proof_trailing_crlf", token, proof+"\r\n", expectProofRespelled),
	)
	for i, seg := range []string{"header", "payload"} {
		for _, r := range respell(ph[i]) {
			parts := [2]string{ph[0], ph[1]}
			parts[i] = r.text
			cases = append(cases, mk("proof_"+seg+"_"+r.name, token, es256Sign(device, parts[0], parts[1]), expectProofRespelled))
		}
	}
	// The device key's coordinates in the proof header (the thumbprint, and so
	// cnf.jkt, is computed from the decoded key, so a lenient verifier finds it
	// the same key).
	for _, coord := range []string{"x", "y"} {
		for _, r := range respell(device.jwkMap[coord]) {
			jwk := maps.Clone(device.jwkMap)
			jwk[coord] = r.text
			header := b64(map[string]any{"alg": "ES256", "typ": "dpop+jwt", "jwk": jwk})
			cases = append(cases, mk("proof_jwk_"+coord+"_"+r.name, token,
				es256Sign(device, header, ph[1]), expectProofRespelled))
		}
	}
	return cases
}

const nonCanonicalDescription = "Non-canonical field vectors (docs/wire-contract.md, Authentication, \"Token spelling\"; bug-fix ledger F53). " +
	"Each case is the valid case of cases.json with its access token or its DPoP proof spelled another way that a lenient base64url decoder reads as the same bytes: " +
	"a nonzero spare bit in a segment's last character, '=' padding (and a single '=' where two belong), the standard alphabet ('+', '/'), a line feed, carriage return or CRLF inside a segment, " +
	"whitespace around the proof, or such a spelling of the proof key's x or y. " +
	"A re-spelled signature is the same bytes; a re-spelled header, payload or coordinate is signed again over its new text with the fixture key, and the proof of a re-spelled token carries the ath of that exact text, " +
	"so a verifier that decodes leniently accepts every case and only the spelling rule refuses it. " +
	"Every verifier must refuse every case: expect is \"malformed\" for a re-spelled access token (not RFC 7515 base64url, so not a compact JWS) and \"invalid_proof\" for a re-spelled proof. " +
	"Whitespace around the access token is not a case: it is outside the token, in the Authorization header. " +
	"The verifier is configured as for cases.json (issuer, appCode, the JWKS next to this file) and the Authorization header is \"DPoP \" + accessToken. Generated by tools/field-fixtures; do not edit."
