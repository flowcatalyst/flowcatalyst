package config

import (
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds all configuration for FlowCatalyst
type Config struct {
	// HTTP server configuration
	HTTP HTTPConfig

	// MongoDB configuration
	MongoDB MongoDBConfig

	// Queue configuration (NATS or SQS)
	Queue QueueConfig

	// Authentication configuration
	Auth AuthConfig

	// Leader election configuration
	Leader LeaderConfig

	// Data directory for embedded services
	DataDir string

	// Development mode
	DevMode bool
}

// HTTPConfig holds HTTP server configuration
type HTTPConfig struct {
	Port        int
	CORSOrigins []string
}

// MongoDBConfig holds MongoDB connection configuration
type MongoDBConfig struct {
	URI      string
	Database string
}

// QueueConfig holds queue configuration
type QueueConfig struct {
	Type string // "embedded", "nats", "sqs"

	NATS NATSConfig
	SQS  SQSConfig
}

// NATSConfig holds NATS configuration
type NATSConfig struct {
	URL     string
	DataDir string
}

// SQSConfig holds AWS SQS configuration
type SQSConfig struct {
	QueueURL          string
	Region            string
	WaitTimeSeconds   int
	VisibilityTimeout int
}

// AuthConfig holds authentication configuration
type AuthConfig struct {
	Mode         string // "embedded" or "remote"
	ExternalBase string // External base URL for OAuth callbacks

	JWT JWTConfig

	Session SessionConfig

	PKCE PKCEConfig

	// Remote mode configuration
	Remote RemoteAuthConfig
}

// JWTConfig holds JWT configuration
type JWTConfig struct {
	Issuer                   string
	PrivateKeyPath           string
	PublicKeyPath            string
	AccessTokenExpiry        time.Duration
	SessionTokenExpiry       time.Duration
	RefreshTokenExpiry       time.Duration
	AuthorizationCodeExpiry  time.Duration
}

// SessionConfig holds session cookie configuration
type SessionConfig struct {
	CookieName string
	Secure     bool
	SameSite   string // "Strict", "Lax", "None"
}

// PKCEConfig holds PKCE configuration
type PKCEConfig struct {
	Required bool
}

// RemoteAuthConfig holds remote authentication configuration
type RemoteAuthConfig struct {
	JWKSUrl string
	Issuer  string
}

// LeaderConfig holds leader election configuration
type LeaderConfig struct {
	// Enabled controls whether leader election is active
	Enabled bool

	// InstanceID uniquely identifies this instance (defaults to HOSTNAME)
	InstanceID string

	// TTL is how long the lock is valid before expiring
	TTL time.Duration

	// RefreshInterval is how often to refresh the lock while primary
	RefreshInterval time.Duration
}

// Load loads configuration from environment variables with sensible defaults
func Load() (*Config, error) {
	cfg := &Config{
		HTTP: HTTPConfig{
			Port:        getEnvInt("HTTP_PORT", 8080),
			CORSOrigins: getEnvSlice("CORS_ORIGINS", []string{"http://localhost:4200"}),
		},

		MongoDB: MongoDBConfig{
			URI:      getEnv("MONGODB_URI", "mongodb://localhost:27017/?replicaSet=rs0&directConnection=true"),
			Database: getEnv("MONGODB_DATABASE", "flowcatalyst"),
		},

		Queue: QueueConfig{
			Type: getEnv("QUEUE_TYPE", "embedded"),
			NATS: NATSConfig{
				URL:     getEnv("NATS_URL", "nats://localhost:4222"),
				DataDir: getEnv("NATS_DATA_DIR", "./data/nats"),
			},
			SQS: SQSConfig{
				QueueURL:          getEnv("SQS_QUEUE_URL", ""),
				Region:            getEnv("AWS_REGION", "us-east-1"),
				WaitTimeSeconds:   getEnvInt("SQS_WAIT_TIME_SECONDS", 20),
				VisibilityTimeout: getEnvInt("SQS_VISIBILITY_TIMEOUT", 120),
			},
		},

		Auth: AuthConfig{
			Mode:         getEnv("AUTH_MODE", "embedded"),
			ExternalBase: getEnv("AUTH_EXTERNAL_BASE_URL", "http://localhost:4200"),

			JWT: JWTConfig{
				Issuer:                   getEnv("JWT_ISSUER", "flowcatalyst"),
				PrivateKeyPath:           getEnv("JWT_PRIVATE_KEY_PATH", ""),
				PublicKeyPath:            getEnv("JWT_PUBLIC_KEY_PATH", ""),
				AccessTokenExpiry:        getEnvDuration("JWT_ACCESS_TOKEN_EXPIRY", 1*time.Hour),
				SessionTokenExpiry:       getEnvDuration("JWT_SESSION_TOKEN_EXPIRY", 8*time.Hour),
				RefreshTokenExpiry:       getEnvDuration("JWT_REFRESH_TOKEN_EXPIRY", 30*24*time.Hour),
				AuthorizationCodeExpiry:  getEnvDuration("JWT_AUTHORIZATION_CODE_EXPIRY", 10*time.Minute),
			},

			Session: SessionConfig{
				CookieName: getEnv("SESSION_COOKIE_NAME", "FLOWCATALYST_SESSION"),
				Secure:     getEnvBool("SESSION_SECURE", true),
				SameSite:   getEnv("SESSION_SAME_SITE", "Strict"),
			},

			PKCE: PKCEConfig{
				Required: getEnvBool("PKCE_REQUIRED", true),
			},

			Remote: RemoteAuthConfig{
				JWKSUrl: getEnv("AUTH_REMOTE_JWKS_URL", ""),
				Issuer:  getEnv("AUTH_REMOTE_ISSUER", ""),
			},
		},

		Leader: LeaderConfig{
			Enabled:         getEnvBool("LEADER_ELECTION_ENABLED", false),
			InstanceID:      getEnv("HOSTNAME", ""),
			TTL:             getEnvDuration("LEADER_TTL", 30*time.Second),
			RefreshInterval: getEnvDuration("LEADER_REFRESH_INTERVAL", 10*time.Second),
		},

		DataDir: getEnv("DATA_DIR", "./data"),
		DevMode: getEnvBool("FLOWCATALYST_DEV", false),
	}

	return cfg, nil
}

// Helper functions for environment variable parsing

func getEnv(key, defaultValue string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if value, ok := os.LookupEnv(key); ok {
		if intVal, err := strconv.Atoi(value); err == nil {
			return intVal
		}
	}
	return defaultValue
}

func getEnvBool(key string, defaultValue bool) bool {
	if value, ok := os.LookupEnv(key); ok {
		if boolVal, err := strconv.ParseBool(value); err == nil {
			return boolVal
		}
	}
	return defaultValue
}

func getEnvDuration(key string, defaultValue time.Duration) time.Duration {
	if value, ok := os.LookupEnv(key); ok {
		if duration, err := time.ParseDuration(value); err == nil {
			return duration
		}
	}
	return defaultValue
}

func getEnvSlice(key string, defaultValue []string) []string {
	if value, ok := os.LookupEnv(key); ok {
		return strings.Split(value, ",")
	}
	return defaultValue
}
