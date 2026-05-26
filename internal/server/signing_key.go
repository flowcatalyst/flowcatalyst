package server

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"log/slog"
	"os"
)

// LoadSigningKeyOrEphemeral returns the PEM-encoded RSA private key for
// JWT signing. Resolution order:
//
//  1. cfg.JWTSigningKeyPath — read from disk if set.
//  2. FC_JWT_SIGNING_KEY_PEM — read inline from env (handy for ECS).
//  3. Otherwise, generate an ephemeral 2048-bit RSA key and log a warning.
//     Ephemeral keys are fine for dev / first-boot smoke tests but lose
//     every token's signature on restart. Production must supply (1) or (2).
func LoadSigningKeyOrEphemeral(path string) []byte {
	if path != "" {
		if b, err := os.ReadFile(path); err == nil {
			return b
		} else {
			slog.Warn("FC_JWT_SIGNING_KEY_PATH unreadable, falling back", "err", err)
		}
	}
	if pemStr := os.Getenv("FC_JWT_SIGNING_KEY_PEM"); pemStr != "" {
		return []byte(pemStr)
	}
	slog.Warn("no JWT signing key configured — generating ephemeral RSA key (tokens won't survive restart)")
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		panic("rsa generate: " + err.Error())
	}
	der := x509.MarshalPKCS1PrivateKey(key)
	return pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: der})
}
