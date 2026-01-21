package secrets

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
)

// EncryptedProvider stores secrets encrypted on the local filesystem
type EncryptedProvider struct {
	key     []byte
	dataDir string
	mu      sync.RWMutex
	cache   map[string]string
}

// NewEncryptedProvider creates a new encrypted file provider
func NewEncryptedProvider(encryptionKey, dataDir string) (*EncryptedProvider, error) {
	if encryptionKey == "" {
		return nil, fmt.Errorf("%w: encryption key is required", ErrInvalidKey)
	}

	// Decode the base64 key
	key, err := base64.StdEncoding.DecodeString(encryptionKey)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to decode encryption key: %v", ErrInvalidKey, err)
	}

	// Key must be 32 bytes for AES-256
	if len(key) != 32 {
		return nil, fmt.Errorf("%w: encryption key must be 32 bytes (256 bits), got %d", ErrInvalidKey, len(key))
	}

	// Ensure data directory exists
	if err := os.MkdirAll(dataDir, 0700); err != nil {
		return nil, fmt.Errorf("failed to create secrets directory: %w", err)
	}

	p := &EncryptedProvider{
		key:     key,
		dataDir: dataDir,
		cache:   make(map[string]string),
	}

	// Load existing secrets into cache
	if err := p.loadCache(); err != nil {
		return nil, fmt.Errorf("failed to load secrets cache: %w", err)
	}

	return p, nil
}

// Get retrieves a secret by key
func (p *EncryptedProvider) Get(ctx context.Context, key string) (string, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	value, ok := p.cache[key]
	if !ok {
		return "", ErrSecretNotFound
	}
	return value, nil
}

// Set stores a secret
func (p *EncryptedProvider) Set(ctx context.Context, key, value string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.cache[key] = value
	return p.saveCache()
}

// Delete removes a secret
func (p *EncryptedProvider) Delete(ctx context.Context, key string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if _, ok := p.cache[key]; !ok {
		return ErrSecretNotFound
	}

	delete(p.cache, key)
	return p.saveCache()
}

// Name returns the provider name
func (p *EncryptedProvider) Name() string {
	return "encrypted"
}

// secretsFile returns the path to the secrets file
func (p *EncryptedProvider) secretsFile() string {
	return filepath.Join(p.dataDir, "secrets.enc")
}

// loadCache loads secrets from the encrypted file
func (p *EncryptedProvider) loadCache() error {
	data, err := os.ReadFile(p.secretsFile())
	if os.IsNotExist(err) {
		// No secrets file yet, that's fine
		return nil
	}
	if err != nil {
		return err
	}

	// Decrypt
	plaintext, err := p.decrypt(data)
	if err != nil {
		return fmt.Errorf("failed to decrypt secrets: %w", err)
	}

	// Unmarshal
	if err := json.Unmarshal(plaintext, &p.cache); err != nil {
		return fmt.Errorf("failed to parse secrets: %w", err)
	}

	return nil
}

// saveCache saves secrets to the encrypted file
func (p *EncryptedProvider) saveCache() error {
	// Marshal
	plaintext, err := json.Marshal(p.cache)
	if err != nil {
		return fmt.Errorf("failed to serialize secrets: %w", err)
	}

	// Encrypt
	ciphertext, err := p.encrypt(plaintext)
	if err != nil {
		return fmt.Errorf("failed to encrypt secrets: %w", err)
	}

	// Write to file atomically
	tmpFile := p.secretsFile() + ".tmp"
	if err := os.WriteFile(tmpFile, ciphertext, 0600); err != nil {
		return fmt.Errorf("failed to write secrets file: %w", err)
	}

	if err := os.Rename(tmpFile, p.secretsFile()); err != nil {
		os.Remove(tmpFile)
		return fmt.Errorf("failed to rename secrets file: %w", err)
	}

	return nil
}

// encrypt encrypts data using AES-256-GCM
func (p *EncryptedProvider) encrypt(plaintext []byte) ([]byte, error) {
	block, err := aes.NewCipher(p.key)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}

	// Prepend nonce to ciphertext
	ciphertext := gcm.Seal(nonce, nonce, plaintext, nil)
	return ciphertext, nil
}

// decrypt decrypts data using AES-256-GCM
func (p *EncryptedProvider) decrypt(ciphertext []byte) ([]byte, error) {
	block, err := aes.NewCipher(p.key)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, fmt.Errorf("ciphertext too short")
	}

	nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, err
	}

	return plaintext, nil
}

// GenerateKey generates a new 256-bit encryption key
func GenerateKey() (string, error) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(key), nil
}
