package jwt

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"log/slog"
)

var (
	ErrKeyNotFound     = errors.New("key not found")
	ErrInvalidKeyFormat = errors.New("invalid key format")
)

// KeyManager manages RSA key pairs for JWT signing
type KeyManager struct {
	mu         sync.RWMutex
	privateKey *rsa.PrivateKey
	publicKey  *rsa.PublicKey
	keyID      string
}

// NewKeyManager creates a new key manager
func NewKeyManager() *KeyManager {
	return &KeyManager{}
}

// Initialize loads or generates RSA keys
func (km *KeyManager) Initialize(privateKeyPath, publicKeyPath, devKeyDir string) error {
	km.mu.Lock()
	defer km.mu.Unlock()

	// Try to load from configured paths first
	if privateKeyPath != "" && publicKeyPath != "" {
		if err := km.loadFromFiles(privateKeyPath, publicKeyPath); err == nil {
			slog.Info("Loaded JWT keys from configured paths", "keyId", km.keyID)
			return nil
		} else {
			slog.Warn("Failed to load JWT keys from configured paths, will try dev keys", "error", err)
		}
	}

	// Try to load from dev key directory
	if devKeyDir != "" {
		privPath := filepath.Join(devKeyDir, "private.pem")
		pubPath := filepath.Join(devKeyDir, "public.pem")

		if err := km.loadFromFiles(privPath, pubPath); err == nil {
			slog.Info("Loaded JWT keys from dev directory", "keyId", km.keyID, "dir", devKeyDir)
			return nil
		}

		// Generate new keys for dev mode
		slog.Info("Generating new JWT keys for development", "dir", devKeyDir)
		if err := km.generateAndSave(devKeyDir); err != nil {
			return fmt.Errorf("failed to generate dev keys: %w", err)
		}
		slog.Info("Generated new JWT keys", "keyId", km.keyID)
		return nil
	}

	// Generate ephemeral keys (not persisted)
	slog.Warn("Generating ephemeral JWT keys (will be lost on restart)")
	return km.generate()
}

// loadFromFiles loads keys from PEM files
func (km *KeyManager) loadFromFiles(privateKeyPath, publicKeyPath string) error {
	// Load private key
	privPEM, err := os.ReadFile(privateKeyPath)
	if err != nil {
		return fmt.Errorf("failed to read private key: %w", err)
	}

	block, _ := pem.Decode(privPEM)
	if block == nil {
		return ErrInvalidKeyFormat
	}

	privateKey, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		// Try PKCS8 format
		key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err != nil {
			return fmt.Errorf("failed to parse private key: %w", err)
		}
		var ok bool
		privateKey, ok = key.(*rsa.PrivateKey)
		if !ok {
			return ErrInvalidKeyFormat
		}
	}

	// Load public key
	pubPEM, err := os.ReadFile(publicKeyPath)
	if err != nil {
		return fmt.Errorf("failed to read public key: %w", err)
	}

	block, _ = pem.Decode(pubPEM)
	if block == nil {
		return ErrInvalidKeyFormat
	}

	pubInterface, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return fmt.Errorf("failed to parse public key: %w", err)
	}

	publicKey, ok := pubInterface.(*rsa.PublicKey)
	if !ok {
		return ErrInvalidKeyFormat
	}

	km.privateKey = privateKey
	km.publicKey = publicKey
	km.keyID = km.generateKeyID(publicKey)

	return nil
}

// generate creates a new RSA key pair
func (km *KeyManager) generate() error {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return fmt.Errorf("failed to generate RSA key: %w", err)
	}

	km.privateKey = privateKey
	km.publicKey = &privateKey.PublicKey
	km.keyID = km.generateKeyID(km.publicKey)

	return nil
}

// generateAndSave generates keys and saves them to the specified directory
func (km *KeyManager) generateAndSave(dir string) error {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("failed to create key directory: %w", err)
	}

	if err := km.generate(); err != nil {
		return err
	}

	// Save private key
	privPath := filepath.Join(dir, "private.pem")
	privBytes := x509.MarshalPKCS1PrivateKey(km.privateKey)
	privPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: privBytes,
	})
	if err := os.WriteFile(privPath, privPEM, 0600); err != nil {
		return fmt.Errorf("failed to write private key: %w", err)
	}

	// Save public key
	pubPath := filepath.Join(dir, "public.pem")
	pubBytes, err := x509.MarshalPKIXPublicKey(km.publicKey)
	if err != nil {
		return fmt.Errorf("failed to marshal public key: %w", err)
	}
	pubPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: pubBytes,
	})
	if err := os.WriteFile(pubPath, pubPEM, 0644); err != nil {
		return fmt.Errorf("failed to write public key: %w", err)
	}

	return nil
}

// generateKeyID creates a key ID from the public key
func (km *KeyManager) generateKeyID(key *rsa.PublicKey) string {
	pubBytes, _ := x509.MarshalPKIXPublicKey(key)
	hash := sha256.Sum256(pubBytes)
	return base64.RawURLEncoding.EncodeToString(hash[:8])
}

// PrivateKey returns the private key for signing
func (km *KeyManager) PrivateKey() *rsa.PrivateKey {
	km.mu.RLock()
	defer km.mu.RUnlock()
	return km.privateKey
}

// PublicKey returns the public key for verification
func (km *KeyManager) PublicKey() *rsa.PublicKey {
	km.mu.RLock()
	defer km.mu.RUnlock()
	return km.publicKey
}

// KeyID returns the key ID
func (km *KeyManager) KeyID() string {
	km.mu.RLock()
	defer km.mu.RUnlock()
	return km.keyID
}

// JWKS represents a JSON Web Key Set
type JWKS struct {
	Keys []JWK `json:"keys"`
}

// JWK represents a JSON Web Key
type JWK struct {
	Kty string `json:"kty"`           // Key Type (RSA)
	Alg string `json:"alg"`           // Algorithm (RS256)
	Use string `json:"use"`           // Use (sig)
	Kid string `json:"kid"`           // Key ID
	N   string `json:"n"`             // Modulus
	E   string `json:"e"`             // Exponent
}

// GetJWKS returns the JWKS for the current public key
func (km *KeyManager) GetJWKS() *JWKS {
	km.mu.RLock()
	defer km.mu.RUnlock()

	if km.publicKey == nil {
		return &JWKS{Keys: []JWK{}}
	}

	return &JWKS{
		Keys: []JWK{
			{
				Kty: "RSA",
				Alg: "RS256",
				Use: "sig",
				Kid: km.keyID,
				N:   base64.RawURLEncoding.EncodeToString(km.publicKey.N.Bytes()),
				E:   base64.RawURLEncoding.EncodeToString(bigIntToBytes(km.publicKey.E)),
			},
		},
	}
}

// bigIntToBytes converts an int to bytes (for RSA exponent)
func bigIntToBytes(i int) []byte {
	if i == 65537 {
		return []byte{1, 0, 1} // Common case: AQAB in base64
	}
	// Generic case
	b := make([]byte, 4)
	b[0] = byte(i >> 24)
	b[1] = byte(i >> 16)
	b[2] = byte(i >> 8)
	b[3] = byte(i)
	// Trim leading zeros
	for len(b) > 1 && b[0] == 0 {
		b = b[1:]
	}
	return b
}
