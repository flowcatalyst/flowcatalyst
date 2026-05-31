package server

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
)

// LoadSigningKeyOrEphemeral returns the PEM-encoded RSA private key for
// JWT signing. Resolution order:
//
//  1. cfg.JWTSigningKeyPath — read from disk if set.
//  2. Inline PEM from env: FLOWCATALYST_JWT_PRIVATE_KEY (the name the Rust
//     platform + the deploy IaC use — load-bearing for drop-in token parity)
//     or FC_JWT_SIGNING_KEY_PEM (the Go-native alias).
//  3. Otherwise, generate an ephemeral 2048-bit RSA key and log a warning.
//     Ephemeral keys are fine for dev / first-boot smoke tests but lose every
//     token's signature on restart AND differ per instance (so a multi-replica
//     service rejects each other's tokens). Production must supply (1) or (2).
func LoadSigningKeyOrEphemeral(path string) []byte {
	if path != "" {
		if b, err := os.ReadFile(path); err == nil {
			return b
		} else {
			slog.Warn("FC_JWT_SIGNING_KEY_PATH unreadable, falling back", "err", err)
		}
	}
	// FLOWCATALYST_JWT_PRIVATE_KEY first: it's the key the Rust system signs
	// with, so reading it keeps Go RS256 tokens validating against the same
	// keypair (and stops the silent ephemeral-key fallback that mints tokens
	// no other replica — or the Rust side — can verify).
	for _, env := range []string{"FLOWCATALYST_JWT_PRIVATE_KEY", "FC_JWT_SIGNING_KEY_PEM"} {
		if pemStr := os.Getenv(env); pemStr != "" {
			return []byte(pemStr)
		}
	}
	slog.Warn("no JWT signing key configured — generating ephemeral RSA key (tokens won't survive restart)")
	return generateRSAPEM()
}

// EnsureSigningKeyFile guarantees a PEM-encoded RSA private key exists
// at path. If the file is absent (or empty), generate one and write it
// with 0600. Used by fc-dev so tokens survive restarts without forcing
// engineers to manage a keyring locally. Returns the path that the
// caller should set FC_JWT_SIGNING_KEY_PATH to.
func EnsureSigningKeyFile(path string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("signing key path is empty")
	}
	if st, err := os.Stat(path); err == nil && st.Size() > 0 {
		return path, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return "", fmt.Errorf("create signing key dir: %w", err)
	}
	pemBytes := generateRSAPEM()
	if err := os.WriteFile(path, pemBytes, 0o600); err != nil {
		return "", fmt.Errorf("write signing key: %w", err)
	}
	slog.Info("generated persistent JWT signing key", "path", path)
	return path, nil
}

// generateRSAPEM mints a fresh 2048-bit RSA private key in PKCS#1 PEM.
// Used by both the ephemeral fallback and the dev key-file generator.
func generateRSAPEM() []byte {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		panic("rsa generate: " + err.Error())
	}
	der := x509.MarshalPKCS1PrivateKey(key)
	return pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: der})
}
