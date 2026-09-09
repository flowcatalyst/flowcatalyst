// Package encryption implements field-level AES-256-GCM encryption with
// FLOWCATALYST_APP_KEY rotation support, plus a keyed-hash form for secrets
// that are only ever verified (never resent or re-signed by the platform).
// Used for OAuth client secrets, webhook signing keys, and other sensitive
// column values.
//
// Wire format:
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
// A verify-only secret is instead stored as "hashed:v1:<base64 MAC>" where
// MAC = HMAC-SHA256(current key, plaintext) — see Hash and VerifySecret.
// A hashed: value never falls back to decrypt-and-compare on mismatch: the
// prefix is a closed claim, same as the encrypted: one.
//
// Key rotation: instantiate with FromEnv (FLOWCATALYST_APP_KEY current,
// FLOWCATALYST_APP_KEY_PREVIOUS optional fallback). Encrypt and Hash always
// use the current key. Decrypt and VerifySecret try current, then each
// previous key. Use ReEncrypt + NeedsReEncryption to migrate encrypted
// values across a rotation; VerifySecret reports the equivalent signal
// (rehash) for hashed ones inline, since a hash can only be recomputed from
// the plaintext a caller just supplied, not from the stored value.
package encryption

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"strings"

	cryptorand "crypto/rand"
)

// currentVersion is the format version byte for new encryptions.
const currentVersion byte = 1

// hashPrefix marks a keyed-hash, verify-only secret: hashed:v1:<base64 MAC>.
// A closed claim like encrypted: — a value carrying this prefix is only ever
// checked by VerifySecret's hash branch, never decrypted.
const hashPrefix = "hashed:v1:"

// literalPrefix marks a dev-bypass value that is its own plaintext.
const literalPrefix = "literal:"

// Service performs field-level encryption with optional key rotation.
type Service struct {
	current  cipher.AEAD
	previous []cipher.AEAD

	// currentKeyBytes / previousKeyBytes are the same raw key bytes behind
	// current / previous, kept alongside the AEADs so Hash and VerifySecret
	// can key an HMAC with "the same key bytes the encryption service uses
	// for AES-GCM" without re-deriving anything.
	currentKeyBytes  []byte
	previousKeyBytes [][]byte
}

// New constructs a Service with a single key (no rotation). keyB64 is a
// base64-encoded 32-byte AES-256 key.
func New(keyB64 string) (*Service, error) {
	keyBytes, err := decodeKey(keyB64)
	if err != nil {
		return nil, err
	}
	aead, err := aeadFromKey(keyBytes)
	if err != nil {
		return nil, err
	}
	return &Service{current: aead, currentKeyBytes: keyBytes}, nil
}

// WithPreviousKeys constructs a Service with rotation: new encryptions
// use currentKeyB64; decryption falls back through previousKeysB64.
func WithPreviousKeys(currentKeyB64 string, previousKeysB64 []string) (*Service, error) {
	currentKeyBytes, err := decodeKey(currentKeyB64)
	if err != nil {
		return nil, err
	}
	current, err := aeadFromKey(currentKeyBytes)
	if err != nil {
		return nil, err
	}
	prev := make([]cipher.AEAD, 0, len(previousKeysB64))
	prevKeyBytes := make([][]byte, 0, len(previousKeysB64))
	for i, k := range previousKeysB64 {
		kb, err := decodeKey(k)
		if err != nil {
			return nil, fmt.Errorf("previous key %d: %w", i, err)
		}
		a, err := aeadFromKey(kb)
		if err != nil {
			return nil, fmt.Errorf("previous key %d: %w", i, err)
		}
		prev = append(prev, a)
		prevKeyBytes = append(prevKeyBytes, kb)
	}
	return &Service{
		current:          current,
		previous:         prev,
		currentKeyBytes:  currentKeyBytes,
		previousKeyBytes: prevKeyBytes,
	}, nil
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

// MustFromEnv is FromEnv for the startup paths that have no error to return.
// A *malformed* key is a boot-time misconfiguration and is fatal (owner ruling
// 2026-09-08): carrying on with encryption silently disabled means the process
// comes up, fails closed on every read and refuses every secret write, with
// nothing pointing at the fat-fingered key. An unset key is still not an
// error — that is the documented "encryption disabled" state and returns nil.
func MustFromEnv() *Service {
	svc, err := FromEnv()
	if err != nil {
		panic(fmt.Sprintf("encryption init: %v", err))
	}
	return svc
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
	// "literal:<value>" means the value IS the plaintext (owner ruling
	// 2026-09-08). One shape, one meaning, wherever it is read: previously
	// only secrets.Service.Resolve honoured it and Decrypt reported it as
	// invalid base64.
	if lit := strings.TrimSpace(encrypted); strings.HasPrefix(lit, literalPrefix) {
		return strings.TrimPrefix(lit, literalPrefix), nil
	}
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
		if len(data) >= 1+nonceSize+1 {
			if pt, err := s.tryDecrypt(data[1:1+nonceSize], data[1+nonceSize:]); err == nil {
				return pt, nil
			}
		}
		// A v0 envelope whose random nonce happens to begin 0x01 (1 in 256)
		// reads as v1 and fails under every key. Retrying the v0 layout is
		// safe because GCM authenticates: a wrong layout cannot yield a false
		// positive, only a failure (owner ruling 2026-09-08). Falls through.
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
// base64-encoded.
func GenerateKey() (string, error) {
	key := make([]byte, 32)
	if _, err := cryptorand.Read(key); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(key), nil
}

func decodeKey(keyB64 string) ([]byte, error) {
	keyBytes, err := base64.StdEncoding.DecodeString(keyB64)
	if err != nil {
		return nil, fmt.Errorf("encryption: invalid base64 key: %w", err)
	}
	if len(keyBytes) != 32 {
		return nil, fmt.Errorf("encryption: key must be 32 bytes, got %d", len(keyBytes))
	}
	return keyBytes, nil
}

func aeadFromKey(keyBytes []byte) (cipher.AEAD, error) {
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

// Hash returns the keyed-hash at-rest form of plaintext for a verify-only
// secret: "hashed:v1:" + base64(HMAC-SHA256(current key, plaintext)). Unlike
// Encrypt, this is deterministic and irreversible — there is no plaintext to
// recover from the stored value, only a caller-supplied guess to check
// against it (see VerifySecret). Always keyed with the current key.
func (s *Service) Hash(plaintext string) string {
	return hashPrefix + base64.StdEncoding.EncodeToString(macFor(s.currentKeyBytes, plaintext))
}

// VerifySecret checks provided against stored, a verify-only secret ref in
// either shape found in the wild:
//
//   - "hashed:v1:<mac>" — the current form. Compared with the keyed MAC only
//     (hmac.Equal, constant-time); current key first, then each previous key
//     in the same order Decrypt tries them. A hashed: ref never falls back
//     to decrypt-and-compare — the prefix is a closed claim, so a mismatch
//     here is failure, not "try the other shape".
//   - anything else — the legacy shape: Decrypt, then compare in constant
//     time (subtle.ConstantTimeCompare), exactly as before this type existed.
//
// ok reports whether provided matched. rehash reports whether the caller
// should rewrite stored to Hash(provided) under the current key: true for
// every legacy-shape match (lazy migration to the hashed form) and for a
// hashed: match that only a previous key could verify (rotation catch-up);
// false for a hashed: match already keyed under the current key, and for no
// match at all.
func (s *Service) VerifySecret(stored, provided string) (ok, rehash bool) {
	if raw, isHash := strings.CutPrefix(stored, hashPrefix); isHash {
		mac, err := base64.StdEncoding.DecodeString(raw)
		if err != nil {
			return false, false
		}
		if hmac.Equal(mac, macFor(s.currentKeyBytes, provided)) {
			return true, false
		}
		for _, prevKey := range s.previousKeyBytes {
			if hmac.Equal(mac, macFor(prevKey, provided)) {
				return true, true
			}
		}
		return false, false
	}

	decrypted, err := s.Decrypt(stored)
	if err != nil {
		return false, false
	}
	if subtle.ConstantTimeCompare([]byte(decrypted), []byte(provided)) == 1 {
		return true, true
	}
	return false, false
}

// macFor computes HMAC-SHA256(key, plaintext).
func macFor(key []byte, plaintext string) []byte {
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(plaintext))
	return mac.Sum(nil)
}
