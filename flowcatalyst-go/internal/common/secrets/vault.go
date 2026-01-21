package secrets

import (
	"context"
	"fmt"
	"strings"

	vault "github.com/hashicorp/vault/api"
)

// VaultProvider uses HashiCorp Vault as the backend
type VaultProvider struct {
	client    *vault.Client
	path      string
	namespace string
}

// NewVaultProvider creates a new HashiCorp Vault provider
func NewVaultProvider(cfg *Config) (*VaultProvider, error) {
	if cfg.VaultAddr == "" {
		return nil, fmt.Errorf("%w: vault address is required", ErrProviderError)
	}

	vaultCfg := vault.DefaultConfig()
	vaultCfg.Address = cfg.VaultAddr

	client, err := vault.NewClient(vaultCfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create Vault client: %w", err)
	}

	// Set token
	if cfg.VaultToken != "" {
		client.SetToken(cfg.VaultToken)
	}

	// Set namespace if provided
	if cfg.VaultNamespace != "" {
		client.SetNamespace(cfg.VaultNamespace)
	}

	path := cfg.VaultPath
	if path == "" {
		path = "secret/data/flowcatalyst"
	}
	// Ensure path doesn't end with /
	path = strings.TrimSuffix(path, "/")

	return &VaultProvider{
		client:    client,
		path:      path,
		namespace: cfg.VaultNamespace,
	}, nil
}

// Get retrieves a secret from Vault
func (p *VaultProvider) Get(ctx context.Context, key string) (string, error) {
	secretPath := p.path + "/" + key

	secret, err := p.client.KVv2("secret").Get(ctx, p.stripSecretPrefix(secretPath))
	if err != nil {
		// Check for not found
		if strings.Contains(err.Error(), "secret not found") {
			return "", ErrSecretNotFound
		}
		return "", fmt.Errorf("%w: %v", ErrProviderError, err)
	}

	if secret == nil || secret.Data == nil {
		return "", ErrSecretNotFound
	}

	// Try to get the "value" key from the data
	if value, ok := secret.Data["value"]; ok {
		if strVal, ok := value.(string); ok {
			return strVal, nil
		}
	}

	return "", ErrSecretNotFound
}

// Set stores a secret in Vault
func (p *VaultProvider) Set(ctx context.Context, key, value string) error {
	secretPath := p.path + "/" + key

	data := map[string]interface{}{
		"value": value,
	}

	_, err := p.client.KVv2("secret").Put(ctx, p.stripSecretPrefix(secretPath), data)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrProviderError, err)
	}

	return nil
}

// Delete removes a secret from Vault
func (p *VaultProvider) Delete(ctx context.Context, key string) error {
	secretPath := p.path + "/" + key

	err := p.client.KVv2("secret").DeleteMetadata(ctx, p.stripSecretPrefix(secretPath))
	if err != nil {
		if strings.Contains(err.Error(), "secret not found") {
			return ErrSecretNotFound
		}
		return fmt.Errorf("%w: %v", ErrProviderError, err)
	}

	return nil
}

// Name returns the provider name
func (p *VaultProvider) Name() string {
	return "vault"
}

// stripSecretPrefix removes the "secret/data/" prefix if present
// This is needed because KVv2 methods add the prefix automatically
func (p *VaultProvider) stripSecretPrefix(path string) string {
	path = strings.TrimPrefix(path, "secret/data/")
	path = strings.TrimPrefix(path, "secret/")
	return path
}
