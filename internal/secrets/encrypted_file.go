package secrets

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	cryptorand "crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// EncryptedFileProvider stores AES-256-GCM-encrypted secrets in a JSON
// file. The encryption key is supplied at construction time (typically
// from a master KMS or boot-time env var).
//
// Wire format (JSON):
//
//	{ "version": 1, "entries": { "key": { "n": "<base64 nonce>", "c": "<base64 ciphertext+tag>" } } }
type EncryptedFileProvider struct {
	path string
	gcm  cipher.AEAD

	mu      sync.Mutex
	entries map[string]encEntry
}

type encEntry struct {
	Nonce      string `json:"n"`
	Ciphertext string `json:"c"`
}

type encFile struct {
	Version int                 `json:"version"`
	Entries map[string]encEntry `json:"entries"`
}

// NewEncryptedFileProvider opens (or creates) an encrypted secrets file.
// key must be 32 bytes (AES-256).
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
	p := &EncryptedFileProvider{path: path, gcm: gcm, entries: make(map[string]encEntry)}
	if err := p.load(); err != nil {
		return nil, err
	}
	return p, nil
}

// Name returns the scheme.
func (*EncryptedFileProvider) Name() string { return "enc" }

func (p *EncryptedFileProvider) load() error {
	data, err := os.ReadFile(p.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("read %s: %w", p.path, err)
	}
	var f encFile
	if err := json.Unmarshal(data, &f); err != nil {
		return fmt.Errorf("decode %s: %w", p.path, err)
	}
	p.entries = f.Entries
	if p.entries == nil {
		p.entries = make(map[string]encEntry)
	}
	return nil
}

func (p *EncryptedFileProvider) save() error {
	if err := os.MkdirAll(filepath.Dir(p.path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(encFile{Version: 1, Entries: p.entries}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p.path, data, 0o600)
}

// Get returns the plaintext for the supplied key.
func (p *EncryptedFileProvider) Get(_ context.Context, key string) (string, error) {
	p.mu.Lock()
	e, ok := p.entries[key]
	p.mu.Unlock()
	if !ok {
		return "", ErrNotFound
	}
	nonce, err := base64.StdEncoding.DecodeString(e.Nonce)
	if err != nil {
		return "", fmt.Errorf("bad nonce: %w", err)
	}
	ct, err := base64.StdEncoding.DecodeString(e.Ciphertext)
	if err != nil {
		return "", fmt.Errorf("bad ciphertext: %w", err)
	}
	pt, err := p.gcm.Open(nil, nonce, ct, []byte(key))
	if err != nil {
		return "", fmt.Errorf("decrypt: %w", err)
	}
	return string(pt), nil
}

// Set encrypts and persists value under key.
func (p *EncryptedFileProvider) Set(_ context.Context, key, value string) error {
	nonce := make([]byte, p.gcm.NonceSize())
	if _, err := cryptorand.Read(nonce); err != nil {
		return err
	}
	ct := p.gcm.Seal(nil, nonce, []byte(value), []byte(key))

	p.mu.Lock()
	p.entries[key] = encEntry{
		Nonce:      base64.StdEncoding.EncodeToString(nonce),
		Ciphertext: base64.StdEncoding.EncodeToString(ct),
	}
	err := p.save()
	p.mu.Unlock()
	return err
}

// Delete removes the entry.
func (p *EncryptedFileProvider) Delete(_ context.Context, key string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.entries, key)
	return p.save()
}
