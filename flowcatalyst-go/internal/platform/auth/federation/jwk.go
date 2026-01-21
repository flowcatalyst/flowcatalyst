package federation

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rsa"
	"encoding/base64"
	"fmt"
	"math/big"
)

// parseJWK parses a JWK into a public key
func parseJWK(jwk map[string]interface{}) (interface{}, error) {
	kty, _ := jwk["kty"].(string)

	switch kty {
	case "RSA":
		return parseRSAJWK(jwk)
	case "EC":
		return parseECJWK(jwk)
	default:
		return nil, fmt.Errorf("unsupported key type: %s", kty)
	}
}

func parseRSAJWK(jwk map[string]interface{}) (*rsa.PublicKey, error) {
	nStr, _ := jwk["n"].(string)
	eStr, _ := jwk["e"].(string)

	if nStr == "" || eStr == "" {
		return nil, fmt.Errorf("missing n or e in RSA JWK")
	}

	// Decode base64url encoded values
	nBytes, err := base64.RawURLEncoding.DecodeString(nStr)
	if err != nil {
		return nil, fmt.Errorf("failed to decode n: %w", err)
	}

	eBytes, err := base64.RawURLEncoding.DecodeString(eStr)
	if err != nil {
		return nil, fmt.Errorf("failed to decode e: %w", err)
	}

	n := new(big.Int).SetBytes(nBytes)
	e := new(big.Int).SetBytes(eBytes)

	return &rsa.PublicKey{
		N: n,
		E: int(e.Int64()),
	}, nil
}

func parseECJWK(jwk map[string]interface{}) (*ecdsa.PublicKey, error) {
	crv, _ := jwk["crv"].(string)
	xStr, _ := jwk["x"].(string)
	yStr, _ := jwk["y"].(string)

	if xStr == "" || yStr == "" {
		return nil, fmt.Errorf("missing x or y in EC JWK")
	}

	var curve elliptic.Curve
	switch crv {
	case "P-256":
		curve = elliptic.P256()
	case "P-384":
		curve = elliptic.P384()
	case "P-521":
		curve = elliptic.P521()
	default:
		return nil, fmt.Errorf("unsupported curve: %s", crv)
	}

	xBytes, err := base64.RawURLEncoding.DecodeString(xStr)
	if err != nil {
		return nil, fmt.Errorf("failed to decode x: %w", err)
	}

	yBytes, err := base64.RawURLEncoding.DecodeString(yStr)
	if err != nil {
		return nil, fmt.Errorf("failed to decode y: %w", err)
	}

	x := new(big.Int).SetBytes(xBytes)
	y := new(big.Int).SetBytes(yBytes)

	return &ecdsa.PublicKey{
		Curve: curve,
		X:     x,
		Y:     y,
	}, nil
}
