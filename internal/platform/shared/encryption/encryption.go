// Package encryption implements field-level AES-256-GCM encryption with
// FLOWCATALYST_APP_KEY rotation support. Used for OAuth client secrets,
// webhook signing keys, and other sensitive column values.
//
// Wire format (mirrors Rust crates/fc-platform/src/shared/encryption_service.rs):
//
//	base64(version_byte(1) || nonce(12) || ciphertext+tag)
//
// version_byte = 0x01 is the current versioned format.
// version_byte = anything else falls back to legacy v0 layout:
//
//	base64(nonce(12) || ciphertext+tag)
//
// TypeScript-era values may also be prefixed "encrypted:" — Decrypt
// strips that prefix transparently.
//
// Key rotation: instantiate with FromEnv (FLOWCATALYST_APP_KEY current,
// FLOWCATALYST_APP_KEY_PREVIOUS optional fallback). Encrypt always uses
// the current key. Decrypt tries current, then each previous key. Use
// ReEncrypt + NeedsReEncryption to migrate.
package encryption

import (
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"strings"

	cryptorand "crypto/rand"
)

// currentVersion is the format version byte for new encryptions.
const currentVersion byte = 1

// Service performs field-level encryption with optional key rotation.
type Service struct {
	current  cipher.AEAD
	previous []cipher.AEAD
}

// New constructs a Service with a single key (no rotation). keyB64 is a
// base64-encoded 32-byte AES-256 key.
func New(keyB64 string) (*Service, error) {
	aead, err := makeAEAD(keyB64)
	if err != nil {
		return nil, err
	}
	return &Service{current: aead}, nil
}

// WithPreviousKeys constructs a Service with rotation: new encryptions
// use currentKeyB64; decryption falls back through previousKeysB64.
func WithPreviousKeys(currentKeyB64 string, previousKeysB64 []string) (*Service, error) {
	current, err := makeAEAD(currentKeyB64)
	if err != nil {
		return nil, err
	}
	prev := make([]cipher.AEAD, 0, len(previousKeysB64))
	for i, k := range previousKeysB64 {
		a, err := makeAEAD(k)
		if err != nil {
			return nil, fmt.Errorf("previous key %d: %w", i, err)
		}
		prev = append(prev, a)
	}
	return &Service{current: current, previous: prev}, nil
}

// FromEnv reads FLOWCATALYST_APP_KEY (required) and
// FLOWCATALYST_APP_KEY_PREVIOUS (optional). Returns nil, nil if the
// current key is unset — callers should treat that as "encryption
// disabled" and refuse to write encrypted fields.
func FromEnv() (*Service, error) {
	current := os.Getenv("FLOWCATALYST_APP_KEY")
	if current == "" {
		return nil, nil
	}
	prev := strings.TrimSpace(os.Getenv("FLOWCATALYST_APP_KEY_PREVIOUS"))
	var prevKeys []string
	if prev != "" {
		prevKeys = []string{prev}
	}
	return WithPreviousKeys(current, prevKeys)
}

// Encrypt returns the base64-encoded versioned envelope for plaintext.
func (s *Service) Encrypt(plaintext string) (string, error) {
	nonce := make([]byte, s.current.NonceSize())
	if _, err := cryptorand.Read(nonce); err != nil {
		return "", fmt.Errorf("encryption: read nonce: %w", err)
	}
	ciphertext := s.current.Seal(nil, nonce, []byte(plaintext), nil)

	out := make([]byte, 0, 1+len(nonce)+len(ciphertext))
	out = append(out, currentVersion)
	out = append(out, nonce...)
	out = append(out, ciphertext...)
	return base64.StdEncoding.EncodeToString(out), nil
}

// Decrypt accepts both v1 versioned envelopes, v0 legacy (no version
// byte), and TypeScript-style "encrypted:" prefixed values. It tries
// the current key first then each previous key.
func (s *Service) Decrypt(encrypted string) (string, error) {
	raw := strings.TrimPrefix(encrypted, "encrypted:")
	data, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return "", fmt.Errorf("encryption: invalid base64: %w", err)
	}
	if len(data) == 0 {
		return "", errors.New("encryption: empty ciphertext")
	}

	nonceSize := s.current.NonceSize()
	if data[0] == currentVersion {
		// v1: version(1) || nonce(12) || ciphertext
		if len(data) < 1+nonceSize+1 {
			return "", errors.New("encryption: ciphertext too short (v1)")
		}
		return s.tryDecrypt(data[1:1+nonceSize], data[1+nonceSize:])
	}
	// v0 legacy: nonce(12) || ciphertext
	if len(data) < nonceSize+1 {
		return "", errors.New("encryption: ciphertext too short (v0)")
	}
	return s.tryDecrypt(data[:nonceSize], data[nonceSize:])
}

func (s *Service) tryDecrypt(nonce, ciphertext []byte) (string, error) {
	if pt, err := s.current.Open(nil, nonce, ciphertext, nil); err == nil {
		return string(pt), nil
	}
	for _, prev := range s.previous {
		if pt, err := prev.Open(nil, nonce, ciphertext, nil); err == nil {
			return string(pt), nil
		}
	}
	return "", errors.New("encryption: decryption failed with all available keys")
}

// ReEncrypt decrypts encrypted (with any available key) and re-encrypts
// using the current key. Used by the rotation migration job.
func (s *Service) ReEncrypt(encrypted string) (string, error) {
	pt, err := s.Decrypt(encrypted)
	if err != nil {
		return "", err
	}
	return s.Encrypt(pt)
}

// NeedsReEncryption returns true if encrypted was produced by an older
// key or older format. False if the current key can decrypt it in v1
// envelope, or if encrypted is malformed (no point in attempting).
func (s *Service) NeedsReEncryption(encrypted string) bool {
	raw := strings.TrimPrefix(encrypted, "encrypted:")
	data, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return false
	}
	if len(data) == 0 || data[0] != currentVersion {
		return true
	}
	nonceSize := s.current.NonceSize()
	if len(data) < 1+nonceSize+1 {
		return true
	}
	_, err = s.current.Open(nil, data[1:1+nonceSize], data[1+nonceSize:], nil)
	return err != nil
}

// GenerateKey returns a freshly-generated 32-byte AES-256 key,
// base64-encoded. Matches Rust generate_key().
func GenerateKey() (string, error) {
	key := make([]byte, 32)
	if _, err := cryptorand.Read(key); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(key), nil
}

func makeAEAD(keyB64 string) (cipher.AEAD, error) {
	keyBytes, err := base64.StdEncoding.DecodeString(keyB64)
	if err != nil {
		return nil, fmt.Errorf("encryption: invalid base64 key: %w", err)
	}
	if len(keyBytes) != 32 {
		return nil, fmt.Errorf("encryption: key must be 32 bytes, got %d", len(keyBytes))
	}
	block, err := aes.NewCipher(keyBytes)
	if err != nil {
		return nil, fmt.Errorf("encryption: aes cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("encryption: gcm: %w", err)
	}
	return gcm, nil
}
