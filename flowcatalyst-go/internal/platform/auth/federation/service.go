package federation

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"sync"

	"log/slog"
)

var (
	ErrAdapterNotFound   = errors.New("adapter not found")
	ErrInvalidState      = errors.New("invalid state")
	ErrProviderNotConfig = errors.New("provider not configured")
)

// Service manages federated authentication with upstream IDPs
type Service struct {
	adapters map[string]Adapter // keyed by domain or IDP ID
	mu       sync.RWMutex
}

// NewService creates a new federation service
func NewService() *Service {
	return &Service{
		adapters: make(map[string]Adapter),
	}
}

// RegisterAdapter registers an adapter for a domain or IDP ID
func (s *Service) RegisterAdapter(key string, adapter Adapter) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.adapters[key] = adapter
}

// GetAdapter returns an adapter for a domain or IDP ID
func (s *Service) GetAdapter(key string) (Adapter, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	adapter, exists := s.adapters[key]
	if !exists {
		return nil, ErrAdapterNotFound
	}
	return adapter, nil
}

// CreateAdapter creates and registers an adapter based on configuration
func (s *Service) CreateAdapter(key string, idpType IdpType, config *Config) error {
	var adapter Adapter
	var err error

	switch idpType {
	case IdpTypeKeycloak:
		adapter, err = NewKeycloakAdapter(config)
	case IdpTypeEntra:
		adapter, err = NewEntraAdapter(&EntraConfig{
			Config:   config,
			TenantID: config.TenantID,
		})
	case IdpTypeOIDC:
		adapter, err = NewOIDCAdapter(config)
	default:
		return fmt.Errorf("unsupported IDP type: %s", idpType)
	}

	if err != nil {
		return fmt.Errorf("failed to create adapter: %w", err)
	}

	s.RegisterAdapter(key, adapter)
	slog.Info("Registered federation adapter", "key", key, "type", string(idpType))
	return nil
}

// HasAdapter checks if an adapter exists for a key
func (s *Service) HasAdapter(key string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, exists := s.adapters[key]
	return exists
}

// InitiateLogin starts the federated login flow
func (s *Service) InitiateLogin(ctx context.Context, key string, req *AuthRequest) (string, error) {
	adapter, err := s.GetAdapter(key)
	if err != nil {
		return "", err
	}

	// Generate state and nonce if not provided
	if req.State == "" {
		req.State, _ = GenerateRandomString(32)
	}
	if req.Nonce == "" {
		req.Nonce, _ = GenerateRandomString(32)
	}

	return adapter.GetAuthorizationURL(ctx, req)
}

// HandleCallback processes the callback from an upstream IDP
func (s *Service) HandleCallback(ctx context.Context, key string, code, redirectURI, codeVerifier, nonce string) (*UserInfo, *TokenSet, error) {
	adapter, err := s.GetAdapter(key)
	if err != nil {
		return nil, nil, err
	}

	// Exchange code for tokens
	tokens, err := adapter.ExchangeCode(ctx, code, redirectURI, codeVerifier)
	if err != nil {
		return nil, nil, fmt.Errorf("code exchange failed: %w", err)
	}

	// Validate ID token and get user info
	var userInfo *UserInfo
	if tokens.IDToken != "" {
		userInfo, err = adapter.ValidateIDToken(ctx, tokens.IDToken, nonce)
		if err != nil {
			return nil, nil, fmt.Errorf("ID token validation failed: %w", err)
		}
	} else {
		// Fall back to userinfo endpoint
		userInfo, err = adapter.GetUserInfo(ctx, tokens.AccessToken)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to get user info: %w", err)
		}
	}

	return userInfo, tokens, nil
}

// GenerateRandomString generates a cryptographically random string
func GenerateRandomString(length int) (string, error) {
	bytes := make([]byte, length)
	if _, err := io.ReadFull(rand.Reader, bytes); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(bytes), nil
}

// GeneratePKCE generates a PKCE code verifier and challenge
func GeneratePKCE() (verifier, challenge string, err error) {
	verifier, err = GenerateRandomString(48)
	if err != nil {
		return "", "", err
	}
	challenge = generateCodeChallenge(verifier)
	return verifier, challenge, nil
}
