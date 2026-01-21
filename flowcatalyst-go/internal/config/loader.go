package config

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/BurntSushi/toml"
)

// TOMLConfig represents the TOML configuration file structure
type TOMLConfig struct {
	HTTP     TOMLHTTPConfig     `toml:"http"`
	MongoDB  TOMLMongoDBConfig  `toml:"mongodb"`
	Queue    TOMLQueueConfig    `toml:"queue"`
	Auth     TOMLAuthConfig     `toml:"auth"`
	Leader   TOMLLeaderConfig   `toml:"leader"`
	Secrets  TOMLSecretsConfig  `toml:"secrets"`
	DataDir  string             `toml:"data_dir"`
	DevMode  bool               `toml:"dev_mode"`
}

// TOMLHTTPConfig represents HTTP configuration in TOML
type TOMLHTTPConfig struct {
	Port        int      `toml:"port"`
	CORSOrigins []string `toml:"cors_origins"`
}

// TOMLMongoDBConfig represents MongoDB configuration in TOML
type TOMLMongoDBConfig struct {
	URI      string `toml:"uri"`
	Database string `toml:"database"`
}

// TOMLQueueConfig represents queue configuration in TOML
type TOMLQueueConfig struct {
	Type string        `toml:"type"`
	NATS TOMLNATSConfig `toml:"nats"`
	SQS  TOMLSQSConfig  `toml:"sqs"`
}

// TOMLNATSConfig represents NATS configuration in TOML
type TOMLNATSConfig struct {
	URL     string `toml:"url"`
	DataDir string `toml:"data_dir"`
}

// TOMLSQSConfig represents SQS configuration in TOML
type TOMLSQSConfig struct {
	QueueURL          string `toml:"queue_url"`
	Region            string `toml:"region"`
	WaitTimeSeconds   int    `toml:"wait_time_seconds"`
	VisibilityTimeout int    `toml:"visibility_timeout"`
}

// TOMLAuthConfig represents auth configuration in TOML
type TOMLAuthConfig struct {
	Mode         string              `toml:"mode"`
	ExternalBase string              `toml:"external_base"`
	JWT          TOMLJWTConfig       `toml:"jwt"`
	Session      TOMLSessionConfig   `toml:"session"`
	PKCE         TOMLPKCEConfig      `toml:"pkce"`
	Remote       TOMLRemoteAuthConfig `toml:"remote"`
}

// TOMLJWTConfig represents JWT configuration in TOML
type TOMLJWTConfig struct {
	Issuer                   string `toml:"issuer"`
	PrivateKeyPath           string `toml:"private_key_path"`
	PublicKeyPath            string `toml:"public_key_path"`
	AccessTokenExpiry        string `toml:"access_token_expiry"`
	SessionTokenExpiry       string `toml:"session_token_expiry"`
	RefreshTokenExpiry       string `toml:"refresh_token_expiry"`
	AuthorizationCodeExpiry  string `toml:"authorization_code_expiry"`
}

// TOMLSessionConfig represents session configuration in TOML
type TOMLSessionConfig struct {
	CookieName string `toml:"cookie_name"`
	Secure     bool   `toml:"secure"`
	SameSite   string `toml:"same_site"`
}

// TOMLPKCEConfig represents PKCE configuration in TOML
type TOMLPKCEConfig struct {
	Required bool `toml:"required"`
}

// TOMLRemoteAuthConfig represents remote auth configuration in TOML
type TOMLRemoteAuthConfig struct {
	JWKSUrl string `toml:"jwks_url"`
	Issuer  string `toml:"issuer"`
}

// TOMLLeaderConfig represents leader election configuration in TOML
type TOMLLeaderConfig struct {
	Enabled         bool   `toml:"enabled"`
	InstanceID      string `toml:"instance_id"`
	TTL             string `toml:"ttl"`
	RefreshInterval string `toml:"refresh_interval"`
}

// TOMLSecretsConfig represents secrets provider configuration in TOML
type TOMLSecretsConfig struct {
	Provider      string `toml:"provider"`
	EncryptionKey string `toml:"encryption_key"`
	DataDir       string `toml:"data_dir"`

	// AWS
	AWSRegion   string `toml:"aws_region"`
	AWSPrefix   string `toml:"aws_prefix"`
	AWSEndpoint string `toml:"aws_endpoint"`

	// Vault
	VaultAddr      string `toml:"vault_addr"`
	VaultPath      string `toml:"vault_path"`
	VaultNamespace string `toml:"vault_namespace"`

	// GCP
	GCPProject string `toml:"gcp_project"`
	GCPPrefix  string `toml:"gcp_prefix"`
}

// ConfigPaths lists the paths to search for config files
var ConfigPaths = []string{
	"config.toml",
	"application.toml",
	"flowcatalyst.toml",
	"./config/config.toml",
	"./config/application.toml",
	"/etc/flowcatalyst/config.toml",
}

// LoadFromFile loads configuration from a TOML file
func LoadFromFile(path string) (*Config, error) {
	var tomlCfg TOMLConfig

	if _, err := toml.DecodeFile(path, &tomlCfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	return tomlConfigToConfig(&tomlCfg)
}

// LoadWithFile loads configuration from file first, then overrides with env vars
func LoadWithFile() (*Config, error) {
	// Start with defaults from environment
	cfg, err := Load()
	if err != nil {
		return nil, err
	}

	// Check for explicit config file path
	configPath := os.Getenv("FLOWCATALYST_CONFIG")
	if configPath == "" {
		// Search for config file in standard locations
		for _, path := range ConfigPaths {
			if _, err := os.Stat(path); err == nil {
				configPath = path
				break
			}
		}
	}

	// If no config file found, just use env vars
	if configPath == "" {
		return cfg, nil
	}

	// Load from file
	fileCfg, err := LoadFromFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load config from %s: %w", configPath, err)
	}

	// Merge: file config as base, env vars override
	return mergeConfigs(fileCfg, cfg), nil
}

// tomlConfigToConfig converts TOML config to the internal Config struct
func tomlConfigToConfig(tc *TOMLConfig) (*Config, error) {
	cfg := &Config{
		HTTP: HTTPConfig{
			Port:        tc.HTTP.Port,
			CORSOrigins: tc.HTTP.CORSOrigins,
		},
		MongoDB: MongoDBConfig{
			URI:      tc.MongoDB.URI,
			Database: tc.MongoDB.Database,
		},
		Queue: QueueConfig{
			Type: tc.Queue.Type,
			NATS: NATSConfig{
				URL:     tc.Queue.NATS.URL,
				DataDir: tc.Queue.NATS.DataDir,
			},
			SQS: SQSConfig{
				QueueURL:          tc.Queue.SQS.QueueURL,
				Region:            tc.Queue.SQS.Region,
				WaitTimeSeconds:   tc.Queue.SQS.WaitTimeSeconds,
				VisibilityTimeout: tc.Queue.SQS.VisibilityTimeout,
			},
		},
		Auth: AuthConfig{
			Mode:         tc.Auth.Mode,
			ExternalBase: tc.Auth.ExternalBase,
			JWT: JWTConfig{
				Issuer:         tc.Auth.JWT.Issuer,
				PrivateKeyPath: tc.Auth.JWT.PrivateKeyPath,
				PublicKeyPath:  tc.Auth.JWT.PublicKeyPath,
			},
			Session: SessionConfig{
				CookieName: tc.Auth.Session.CookieName,
				Secure:     tc.Auth.Session.Secure,
				SameSite:   tc.Auth.Session.SameSite,
			},
			PKCE: PKCEConfig{
				Required: tc.Auth.PKCE.Required,
			},
			Remote: RemoteAuthConfig{
				JWKSUrl: tc.Auth.Remote.JWKSUrl,
				Issuer:  tc.Auth.Remote.Issuer,
			},
		},
		Leader: LeaderConfig{
			Enabled:    tc.Leader.Enabled,
			InstanceID: tc.Leader.InstanceID,
		},
		DataDir: tc.DataDir,
		DevMode: tc.DevMode,
	}

	// Parse durations
	if tc.Auth.JWT.AccessTokenExpiry != "" {
		if d, err := time.ParseDuration(tc.Auth.JWT.AccessTokenExpiry); err == nil {
			cfg.Auth.JWT.AccessTokenExpiry = d
		}
	}
	if tc.Auth.JWT.SessionTokenExpiry != "" {
		if d, err := time.ParseDuration(tc.Auth.JWT.SessionTokenExpiry); err == nil {
			cfg.Auth.JWT.SessionTokenExpiry = d
		}
	}
	if tc.Auth.JWT.RefreshTokenExpiry != "" {
		if d, err := time.ParseDuration(tc.Auth.JWT.RefreshTokenExpiry); err == nil {
			cfg.Auth.JWT.RefreshTokenExpiry = d
		}
	}
	if tc.Auth.JWT.AuthorizationCodeExpiry != "" {
		if d, err := time.ParseDuration(tc.Auth.JWT.AuthorizationCodeExpiry); err == nil {
			cfg.Auth.JWT.AuthorizationCodeExpiry = d
		}
	}
	if tc.Leader.TTL != "" {
		if d, err := time.ParseDuration(tc.Leader.TTL); err == nil {
			cfg.Leader.TTL = d
		}
	}
	if tc.Leader.RefreshInterval != "" {
		if d, err := time.ParseDuration(tc.Leader.RefreshInterval); err == nil {
			cfg.Leader.RefreshInterval = d
		}
	}

	return cfg, nil
}

// mergeConfigs merges two configs, with override taking precedence for non-zero values
func mergeConfigs(base, override *Config) *Config {
	result := *base

	// HTTP
	if override.HTTP.Port != 0 && override.HTTP.Port != 8080 {
		result.HTTP.Port = override.HTTP.Port
	}
	if len(override.HTTP.CORSOrigins) > 0 {
		result.HTTP.CORSOrigins = override.HTTP.CORSOrigins
	}

	// MongoDB
	if override.MongoDB.URI != "" && override.MongoDB.URI != "mongodb://localhost:27017/?replicaSet=rs0&directConnection=true" {
		result.MongoDB.URI = override.MongoDB.URI
	}
	if override.MongoDB.Database != "" && override.MongoDB.Database != "flowcatalyst" {
		result.MongoDB.Database = override.MongoDB.Database
	}

	// Queue
	if override.Queue.Type != "" && override.Queue.Type != "embedded" {
		result.Queue.Type = override.Queue.Type
	}
	if override.Queue.NATS.URL != "" {
		result.Queue.NATS.URL = override.Queue.NATS.URL
	}
	if override.Queue.NATS.DataDir != "" {
		result.Queue.NATS.DataDir = override.Queue.NATS.DataDir
	}
	if override.Queue.SQS.QueueURL != "" {
		result.Queue.SQS.QueueURL = override.Queue.SQS.QueueURL
	}
	if override.Queue.SQS.Region != "" {
		result.Queue.SQS.Region = override.Queue.SQS.Region
	}

	// Auth
	if override.Auth.Mode != "" && override.Auth.Mode != "embedded" {
		result.Auth.Mode = override.Auth.Mode
	}
	if override.Auth.ExternalBase != "" {
		result.Auth.ExternalBase = override.Auth.ExternalBase
	}
	if override.Auth.JWT.Issuer != "" {
		result.Auth.JWT.Issuer = override.Auth.JWT.Issuer
	}
	if override.Auth.Session.CookieName != "" {
		result.Auth.Session.CookieName = override.Auth.Session.CookieName
	}

	// Leader
	if override.Leader.Enabled {
		result.Leader.Enabled = true
	}
	if override.Leader.InstanceID != "" {
		result.Leader.InstanceID = override.Leader.InstanceID
	}

	// General
	if override.DataDir != "" && override.DataDir != "./data" {
		result.DataDir = override.DataDir
	}
	if override.DevMode {
		result.DevMode = true
	}

	return &result
}

// WriteExampleConfig writes an example configuration file
func WriteExampleConfig(path string) error {
	example := `# FlowCatalyst Configuration
# Environment variables override these settings

[http]
port = 8080
cors_origins = ["http://localhost:4200"]

[mongodb]
uri = "mongodb://localhost:27017/?replicaSet=rs0&directConnection=true"
database = "flowcatalyst"

[queue]
type = "embedded"  # embedded, nats, or sqs

[queue.nats]
url = "nats://localhost:4222"
data_dir = "./data/nats"

[queue.sqs]
queue_url = ""
region = "us-east-1"
wait_time_seconds = 20
visibility_timeout = 120

[auth]
mode = "embedded"
external_base = "http://localhost:4200"

[auth.jwt]
issuer = "flowcatalyst"
private_key_path = ""
public_key_path = ""
access_token_expiry = "1h"
session_token_expiry = "8h"
refresh_token_expiry = "720h"
authorization_code_expiry = "10m"

[auth.session]
cookie_name = "FLOWCATALYST_SESSION"
secure = true
same_site = "Strict"

[auth.pkce]
required = true

[auth.remote]
jwks_url = ""
issuer = ""

[leader]
enabled = false
instance_id = ""
ttl = "30s"
refresh_interval = "10s"

[secrets]
provider = "env"  # env, encrypted, aws-sm, vault, gcp-sm

# Encrypted provider
encryption_key = ""
data_dir = "./data/secrets"

# AWS Secrets Manager
aws_region = ""
aws_prefix = "/flowcatalyst/"
aws_endpoint = ""

# HashiCorp Vault
vault_addr = ""
vault_path = "secret/data/flowcatalyst"
vault_namespace = ""

# GCP Secret Manager
gcp_project = ""
gcp_prefix = "flowcatalyst-"

data_dir = "./data"
dev_mode = false
`

	// Ensure directory exists
	dir := filepath.Dir(path)
	if dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create directory: %w", err)
		}
	}

	return os.WriteFile(path, []byte(example), 0644)
}
