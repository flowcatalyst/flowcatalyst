package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
)

// dbSecret is the subset of an RDS-style AWS Secrets Manager secret we read.
// Mirrors the Rust AwsSecretProvider: only username/password/port come from the
// secret JSON — host + database name come from DB_HOST / DB_NAME env vars.
type dbSecret struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Port     *int   `json:"port,omitempty"`
}

// ResolveDBSecretURL builds a Postgres connection string from an AWS Secrets
// Manager secret when DB_SECRET_ARN is configured. Returns (url, true, nil)
// when Secrets-Manager mode applies, ("", false, nil) when it does not (so the
// caller keeps the env-resolved URL), or an error on a genuine fetch/parse
// failure.
//
// SM mode applies only when DB_HOST and DB_SECRET_ARN are both set and no
// explicit FC_DATABASE_URL/DATABASE_URL is present — matching the Rust
// fc-server precedence (full URL > Secrets Manager > explicit DB_* creds).
// DB_SECRET_PROVIDER must be "aws" (the default). Credentials are resolved via
// the standard AWS chain (env, instance profile, ECS task role, …).
//
// Note: this reads the secret once at startup; the Rust DB_SECRET_REFRESH_*
// rotation poller is a tracked follow-up, not yet ported.
func ResolveDBSecretURL(ctx context.Context) (string, bool, error) {
	// An explicit connection string always wins — SM is never consulted.
	if envFirst("FC_DATABASE_URL", "DATABASE_URL", "", "") != "" {
		return "", false, nil
	}
	arn := os.Getenv("DB_SECRET_ARN")
	host := os.Getenv("DB_HOST")
	if arn == "" || host == "" {
		return "", false, nil
	}
	if provider := envOr("DB_SECRET_PROVIDER", "aws"); !strings.EqualFold(provider, "aws") {
		return "", false, fmt.Errorf("DB_SECRET_PROVIDER %q not supported (only \"aws\")", provider)
	}

	awsCfg, err := awsconfig.LoadDefaultConfig(ctx)
	if err != nil {
		return "", false, fmt.Errorf("load AWS config: %w", err)
	}
	sm := secretsmanager.NewFromConfig(awsCfg)
	out, err := sm.GetSecretValue(ctx, &secretsmanager.GetSecretValueInput{SecretId: &arn})
	if err != nil {
		return "", false, fmt.Errorf("get secret %s: %w", arn, err)
	}
	if out.SecretString == nil {
		return "", false, fmt.Errorf("secret %s has no string value", arn)
	}

	var sec dbSecret
	if err := json.Unmarshal([]byte(*out.SecretString), &sec); err != nil {
		return "", false, fmt.Errorf("parse secret %s JSON: %w", arn, err)
	}
	if sec.Username == "" || sec.Password == "" {
		return "", false, fmt.Errorf("secret %s is missing username/password", arn)
	}

	return buildDBSecretDSN(host, envOr("DB_NAME", "flowcatalyst"), os.Getenv("DB_PORT"), sec), true, nil
}

// buildDBSecretDSN assembles the Postgres DSN from the secret + env-supplied
// host/name/port. Pure (no env/network reads) so the parity-critical bits —
// port precedence (secret JSON > DB_PORT > 5432), password URL-escaping, and
// host-already-has-port — are unit-testable. 1:1 with Rust's connection-string
// builder.
func buildDBSecretDSN(host, name, envPort string, sec dbSecret) string {
	port := envPort
	if port == "" {
		port = "5432"
	}
	if sec.Port != nil && *sec.Port > 0 {
		port = strconv.Itoa(*sec.Port)
	}
	hostPort := host
	if !strings.Contains(host, ":") {
		hostPort = host + ":" + port
	}
	return "postgresql://" + sec.Username + ":" + url.QueryEscape(sec.Password) + "@" + hostPort + "/" + name
}
