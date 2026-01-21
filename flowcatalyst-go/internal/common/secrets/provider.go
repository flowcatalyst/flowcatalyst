// Package secrets provides secret management with multiple backend providers.
package secrets

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
)

// Common errors
var (
	ErrSecretNotFound = errors.New("secret not found")
	ErrInvalidKey     = errors.New("invalid encryption key")
	ErrProviderError  = errors.New("provider error")
)

// Provider defines the interface for secret storage backends
type Provider interface {
	// Get retrieves a secret by key
	Get(ctx context.Context, key string) (string, error)

	// Set stores a secret (if supported by the provider)
	Set(ctx context.Context, key, value string) error

	// Delete removes a secret (if supported by the provider)
	Delete(ctx context.Context, key string) error

	// Name returns the provider name for logging
	Name() string
}

// ProviderType represents the type of secret provider
type ProviderType string

const (
	ProviderTypeEncrypted ProviderType = "encrypted"
	ProviderTypeAWSSM     ProviderType = "aws-sm"
	ProviderTypeVault     ProviderType = "vault"
	ProviderTypeGCPSM     ProviderType = "gcp-sm"
	ProviderTypeEnv       ProviderType = "env" // Simple environment variable provider
)

// Config holds configuration for the secrets provider
type Config struct {
	// Provider type
	Provider ProviderType `json:"provider" toml:"provider"`

	// Encrypted provider settings
	EncryptionKey string `json:"encryptionKey" toml:"encryption_key"`
	DataDir       string `json:"dataDir" toml:"data_dir"`

	// AWS Secrets Manager settings
	AWSRegion     string `json:"awsRegion" toml:"aws_region"`
	AWSPrefix     string `json:"awsPrefix" toml:"aws_prefix"`
	AWSEndpoint   string `json:"awsEndpoint" toml:"aws_endpoint"` // For LocalStack
	AWSAccessKey  string `json:"awsAccessKey" toml:"aws_access_key"`
	AWSSecretKey  string `json:"awsSecretKey" toml:"aws_secret_key"`

	// HashiCorp Vault settings
	VaultAddr      string `json:"vaultAddr" toml:"vault_addr"`
	VaultToken     string `json:"vaultToken" toml:"vault_token"`
	VaultPath      string `json:"vaultPath" toml:"vault_path"`
	VaultNamespace string `json:"vaultNamespace" toml:"vault_namespace"`

	// GCP Secret Manager settings
	GCPProject string `json:"gcpProject" toml:"gcp_project"`
	GCPPrefix  string `json:"gcpPrefix" toml:"gcp_prefix"`
}

// DefaultConfig returns a default configuration
func DefaultConfig() *Config {
	return &Config{
		Provider:   ProviderTypeEnv,
		DataDir:    "./data/secrets",
		AWSPrefix:  "/flowcatalyst/",
		VaultPath:  "secret/data/flowcatalyst",
		GCPPrefix:  "flowcatalyst-",
	}
}

// LoadConfigFromEnv loads configuration from environment variables
func LoadConfigFromEnv() *Config {
	cfg := DefaultConfig()

	if p := os.Getenv("FLOWCATALYST_SECRETS_PROVIDER"); p != "" {
		cfg.Provider = ProviderType(strings.ToLower(p))
	}

	// Encrypted provider
	if k := os.Getenv("FLOWCATALYST_SECRETS_ENCRYPTION_KEY"); k != "" {
		cfg.EncryptionKey = k
	}
	if d := os.Getenv("FLOWCATALYST_SECRETS_DATA_DIR"); d != "" {
		cfg.DataDir = d
	}

	// AWS
	if r := os.Getenv("FLOWCATALYST_SECRETS_AWS_REGION"); r != "" {
		cfg.AWSRegion = r
	} else if r := os.Getenv("AWS_REGION"); r != "" {
		cfg.AWSRegion = r
	}
	if p := os.Getenv("FLOWCATALYST_SECRETS_AWS_PREFIX"); p != "" {
		cfg.AWSPrefix = p
	}
	if e := os.Getenv("FLOWCATALYST_SECRETS_AWS_ENDPOINT"); e != "" {
		cfg.AWSEndpoint = e
	}

	// Vault
	if a := os.Getenv("FLOWCATALYST_SECRETS_VAULT_ADDR"); a != "" {
		cfg.VaultAddr = a
	} else if a := os.Getenv("VAULT_ADDR"); a != "" {
		cfg.VaultAddr = a
	}
	if t := os.Getenv("FLOWCATALYST_SECRETS_VAULT_TOKEN"); t != "" {
		cfg.VaultToken = t
	} else if t := os.Getenv("VAULT_TOKEN"); t != "" {
		cfg.VaultToken = t
	}
	if p := os.Getenv("FLOWCATALYST_SECRETS_VAULT_PATH"); p != "" {
		cfg.VaultPath = p
	}
	if n := os.Getenv("FLOWCATALYST_SECRETS_VAULT_NAMESPACE"); n != "" {
		cfg.VaultNamespace = n
	}

	// GCP
	if p := os.Getenv("FLOWCATALYST_SECRETS_GCP_PROJECT"); p != "" {
		cfg.GCPProject = p
	} else if p := os.Getenv("GOOGLE_CLOUD_PROJECT"); p != "" {
		cfg.GCPProject = p
	}
	if p := os.Getenv("FLOWCATALYST_SECRETS_GCP_PREFIX"); p != "" {
		cfg.GCPPrefix = p
	}

	return cfg
}

// NewProvider creates a new secret provider based on configuration
func NewProvider(cfg *Config) (Provider, error) {
	if cfg == nil {
		cfg = LoadConfigFromEnv()
	}

	switch cfg.Provider {
	case ProviderTypeEncrypted:
		return NewEncryptedProvider(cfg.EncryptionKey, cfg.DataDir)
	case ProviderTypeAWSSM:
		return NewAWSSecretsManagerProvider(cfg)
	case ProviderTypeVault:
		return NewVaultProvider(cfg)
	case ProviderTypeGCPSM:
		return NewGCPSecretManagerProvider(cfg)
	case ProviderTypeEnv:
		return NewEnvProvider("FLOWCATALYST_SECRET_"), nil
	default:
		return nil, fmt.Errorf("unknown provider type: %s", cfg.Provider)
	}
}

// EnvProvider reads secrets from environment variables
type EnvProvider struct {
	prefix string
}

// NewEnvProvider creates a new environment variable provider
func NewEnvProvider(prefix string) *EnvProvider {
	return &EnvProvider{prefix: prefix}
}

// Get retrieves a secret from environment variables
func (p *EnvProvider) Get(ctx context.Context, key string) (string, error) {
	envKey := p.prefix + strings.ToUpper(strings.ReplaceAll(key, "-", "_"))
	value := os.Getenv(envKey)
	if value == "" {
		return "", ErrSecretNotFound
	}
	return value, nil
}

// Set is not supported for environment provider
func (p *EnvProvider) Set(ctx context.Context, key, value string) error {
	return fmt.Errorf("environment provider does not support Set")
}

// Delete is not supported for environment provider
func (p *EnvProvider) Delete(ctx context.Context, key string) error {
	return fmt.Errorf("environment provider does not support Delete")
}

// Name returns the provider name
func (p *EnvProvider) Name() string {
	return "env"
}
