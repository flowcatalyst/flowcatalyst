package secrets

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	cryptorand "crypto/rand"
)

// EncryptedFileProvider stores AES-256-GCM-encrypted secrets in a single
// binary file. The wire format mirrors Rust's fc-secrets EncryptedProvider
// (crates/fc-secrets/src/encrypted.rs):
//
//	file bytes = nonce(12) || ciphertext+tag
//	plaintext  = JSON-serialised map[string]string of all entries
//
// No AAD is used. The whole map is re-encrypted on every write.
type EncryptedFileProvider struct {
	path string
	gcm  cipher.AEAD

	mu      sync.Mutex
	entries map[string]string
}

// NewEncryptedFileProvider opens (or creates) the encrypted secrets file
// at `path`. key must be exactly 32 bytes (AES-256).
func NewEncryptedFileProvider(path string, key []byte) (*EncryptedFileProvider, error) {
	if len(key) != 32 {
		return nil, errors.New("encryption key must be 32 bytes (AES-256)")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("aes cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("aes-gcm: %w", err)
	}
	p := &EncryptedFileProvider{path: path, gcm: gcm, entries: make(map[string]string)}
	if err := p.load(); err != nil {
		return nil, err
	}
	return p, nil
}

// NewEncryptedFileProviderFromBase64Key is the Rust-style constructor:
// caller supplies a base64-encoded 32-byte key and a data directory; the
// file is data_dir/secrets.enc.
func NewEncryptedFileProviderFromBase64Key(dataDir, b64Key string) (*EncryptedFileProvider, error) {
	key, err := base64.StdEncoding.DecodeString(b64Key)
	if err != nil {
		return nil, fmt.Errorf("invalid base64 key: %w", err)
	}
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}
	return NewEncryptedFileProvider(filepath.Join(dataDir, "secrets.enc"), key)
}

// Name returns the scheme. Matches Rust EncryptedProvider::name.
func (*EncryptedFileProvider) Name() string { return "encrypted" }

func (p *EncryptedFileProvider) load() error {
	data, err := os.ReadFile(p.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("read %s: %w", p.path, err)
	}
	if len(data) < p.gcm.NonceSize() {
		// Treat undersized file as empty (matches Rust: silently returns Ok).
		return nil
	}
	nonce := data[:p.gcm.NonceSize()]
	ciphertext := data[p.gcm.NonceSize():]
	plaintext, err := p.gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return fmt.Errorf("decrypt %s: %w", p.path, err)
	}
	if len(plaintext) == 0 {
		return nil
	}
	if err := json.Unmarshal(plaintext, &p.entries); err != nil {
		return fmt.Errorf("decode %s: %w", p.path, err)
	}
	if p.entries == nil {
		p.entries = make(map[string]string)
	}
	return nil
}

func (p *EncryptedFileProvider) save() error {
	plaintext, err := json.Marshal(p.entries)
	if err != nil {
		return err
	}
	nonce := make([]byte, p.gcm.NonceSize())
	if _, err := cryptorand.Read(nonce); err != nil {
		return err
	}
	ciphertext := p.gcm.Seal(nil, nonce, plaintext, nil)

	out := make([]byte, 0, len(nonce)+len(ciphertext))
	out = append(out, nonce...)
	out = append(out, ciphertext...)

	if err := os.MkdirAll(filepath.Dir(p.path), 0o700); err != nil {
		return err
	}
	tmp := p.path + ".tmp"
	if err := os.WriteFile(tmp, out, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, p.path)
}

// Get returns the plaintext for the supplied key.
func (p *EncryptedFileProvider) Get(_ context.Context, key string) (string, error) {
	p.mu.Lock()
	v, ok := p.entries[key]
	p.mu.Unlock()
	if !ok {
		return "", ErrNotFound
	}
	return v, nil
}

// Set persists value under key.
func (p *EncryptedFileProvider) Set(_ context.Context, key, value string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.entries[key] = value
	return p.save()
}

// Delete removes the entry. Returns ErrNotFound if absent.
func (p *EncryptedFileProvider) Delete(_ context.Context, key string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if _, ok := p.entries[key]; !ok {
		return ErrNotFound
	}
	delete(p.entries, key)
	return p.save()
}

// GenerateKey returns a freshly-generated 32-byte AES-256 key,
// base64-encoded. Matches Rust fc_secrets::generate_key.
func GenerateKey() (string, error) {
	key := make([]byte, 32)
	if _, err := cryptorand.Read(key); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(key), nil
}
