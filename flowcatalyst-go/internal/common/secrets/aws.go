package secrets

import (
	"context"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager/types"
)

// AWSSecretsManagerProvider uses AWS Secrets Manager as the backend
type AWSSecretsManagerProvider struct {
	client *secretsmanager.Client
	prefix string
}

// NewAWSSecretsManagerProvider creates a new AWS Secrets Manager provider
func NewAWSSecretsManagerProvider(cfg *Config) (*AWSSecretsManagerProvider, error) {
	ctx := context.Background()

	// Build AWS config options
	var opts []func(*config.LoadOptions) error

	if cfg.AWSRegion != "" {
		opts = append(opts, config.WithRegion(cfg.AWSRegion))
	}

	// Use explicit credentials if provided
	if cfg.AWSAccessKey != "" && cfg.AWSSecretKey != "" {
		opts = append(opts, config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(cfg.AWSAccessKey, cfg.AWSSecretKey, ""),
		))
	}

	awsCfg, err := config.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	// Create Secrets Manager client
	var smOpts []func(*secretsmanager.Options)
	if cfg.AWSEndpoint != "" {
		smOpts = append(smOpts, func(o *secretsmanager.Options) {
			o.BaseEndpoint = aws.String(cfg.AWSEndpoint)
		})
	}

	client := secretsmanager.NewFromConfig(awsCfg, smOpts...)

	prefix := cfg.AWSPrefix
	if prefix == "" {
		prefix = "/flowcatalyst/"
	}
	// Ensure prefix ends with /
	if !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}

	return &AWSSecretsManagerProvider{
		client: client,
		prefix: prefix,
	}, nil
}

// Get retrieves a secret from AWS Secrets Manager
func (p *AWSSecretsManagerProvider) Get(ctx context.Context, key string) (string, error) {
	secretName := p.prefix + key

	input := &secretsmanager.GetSecretValueInput{
		SecretId: aws.String(secretName),
	}

	result, err := p.client.GetSecretValue(ctx, input)
	if err != nil {
		// Check if it's a not found error
		var notFoundErr *types.ResourceNotFoundException
		if ok := isAWSNotFoundError(err, &notFoundErr); ok {
			return "", ErrSecretNotFound
		}
		return "", fmt.Errorf("%w: %v", ErrProviderError, err)
	}

	if result.SecretString != nil {
		return *result.SecretString, nil
	}

	return "", ErrSecretNotFound
}

// Set stores a secret in AWS Secrets Manager
func (p *AWSSecretsManagerProvider) Set(ctx context.Context, key, value string) error {
	secretName := p.prefix + key

	// Try to update first
	updateInput := &secretsmanager.PutSecretValueInput{
		SecretId:     aws.String(secretName),
		SecretString: aws.String(value),
	}

	_, err := p.client.PutSecretValue(ctx, updateInput)
	if err != nil {
		// If secret doesn't exist, create it
		var notFoundErr *types.ResourceNotFoundException
		if ok := isAWSNotFoundError(err, &notFoundErr); ok {
			createInput := &secretsmanager.CreateSecretInput{
				Name:         aws.String(secretName),
				SecretString: aws.String(value),
			}
			_, err = p.client.CreateSecret(ctx, createInput)
			if err != nil {
				return fmt.Errorf("%w: failed to create secret: %v", ErrProviderError, err)
			}
			return nil
		}
		return fmt.Errorf("%w: failed to update secret: %v", ErrProviderError, err)
	}

	return nil
}

// Delete removes a secret from AWS Secrets Manager
func (p *AWSSecretsManagerProvider) Delete(ctx context.Context, key string) error {
	secretName := p.prefix + key

	input := &secretsmanager.DeleteSecretInput{
		SecretId:                   aws.String(secretName),
		ForceDeleteWithoutRecovery: aws.Bool(true),
	}

	_, err := p.client.DeleteSecret(ctx, input)
	if err != nil {
		var notFoundErr *types.ResourceNotFoundException
		if ok := isAWSNotFoundError(err, &notFoundErr); ok {
			return ErrSecretNotFound
		}
		return fmt.Errorf("%w: %v", ErrProviderError, err)
	}

	return nil
}

// Name returns the provider name
func (p *AWSSecretsManagerProvider) Name() string {
	return "aws-sm"
}

// isAWSNotFoundError checks if the error is an AWS ResourceNotFoundException
func isAWSNotFoundError(err error, target **types.ResourceNotFoundException) bool {
	if err == nil {
		return false
	}
	// Type assertion for the specific error type
	if e, ok := err.(*types.ResourceNotFoundException); ok {
		*target = e
		return true
	}
	return false
}
