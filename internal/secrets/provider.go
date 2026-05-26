// Package secrets implements a pluggable secrets provider registry.
//
// Backends register themselves via Register. The SecretService routes
// references (e.g. "aws-sm://my-secret") to the right backend.
//
// Mirrors the Rust fc-secrets crate. Phase 1 ships the in-process
// backends (env, encrypted file). AWS Secrets Manager, AWS SSM, and
// Vault land in Phase 1.5+ when the team picks the AWS SDK version.
package secrets

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
)

// Provider is a single secrets backend.
type Provider interface {
	Get(ctx context.Context, key string) (string, error)
	Set(ctx context.Context, key, value string) error
	Delete(ctx context.Context, key string) error
	Name() string
}

// ErrNotFound is returned when a key isn't present in the backend.
var ErrNotFound = errors.New("secret not found")

// Service routes references to the appropriate provider. A reference
// is either:
//   - "env://VAR_NAME"          → env provider
//   - "aws-sm://name"           → AWS Secrets Manager
//   - "aws-ps://parameter"      → AWS Parameter Store / SSM
//   - "vault://path#field"      → HashiCorp Vault
//   - "enc://key"               → encrypted local file
//   - "literal:value"           → bypass; return value as-is (for dev)
//   - bare "value"              → also bypass (assume literal)
type Service struct {
	mu        sync.RWMutex
	providers map[string]Provider
	defaultP  string
}

// NewService constructs a service. defaultProvider is the name used
// when a reference has no scheme.
func NewService(defaultProvider string) *Service {
	return &Service{providers: make(map[string]Provider), defaultP: defaultProvider}
}

// Register adds a provider keyed by its name. Names match the scheme
// in references ("env", "aws-sm", "aws-ps", "vault", "enc").
func (s *Service) Register(p Provider) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.providers[p.Name()] = p
}

// Resolve takes a reference and returns the plaintext secret. Only
// system processes should call this; admin/validation paths should use
// Validate instead.
func (s *Service) Resolve(ctx context.Context, ref string) (string, error) {
	scheme, key, isRef := parseRef(ref)
	if !isRef {
		// Literal or bare value; `key` is the unwrapped payload.
		return key, nil
	}
	s.mu.RLock()
	p, ok := s.providers[scheme]
	s.mu.RUnlock()
	if !ok {
		return "", fmt.Errorf("no provider for scheme %q", scheme)
	}
	return p.Get(ctx, key)
}

// Validate checks that a reference is resolvable without revealing
// plaintext. Used in admin paths where the caller has permission to
// validate but not to read.
func (s *Service) Validate(ctx context.Context, ref string) error {
	scheme, key, isRef := parseRef(ref)
	if !isRef {
		return nil
	}
	s.mu.RLock()
	p, ok := s.providers[scheme]
	s.mu.RUnlock()
	if !ok {
		return fmt.Errorf("no provider for scheme %q", scheme)
	}
	if _, err := p.Get(ctx, key); err != nil {
		return err
	}
	return nil
}

// parseRef extracts the scheme + key from a reference. Returns
// isRef=false for bare literals.
func parseRef(ref string) (scheme, key string, isRef bool) {
	if strings.HasPrefix(ref, "literal:") {
		return "", strings.TrimPrefix(ref, "literal:"), false
	}
	idx := strings.Index(ref, "://")
	if idx < 0 {
		return "", ref, false
	}
	return ref[:idx], ref[idx+3:], true
}
