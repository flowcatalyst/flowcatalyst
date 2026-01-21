package secrets

import (
	"context"
	"fmt"

	secretmanager "cloud.google.com/go/secretmanager/apiv1"
	"cloud.google.com/go/secretmanager/apiv1/secretmanagerpb"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// GCPSecretManagerProvider uses GCP Secret Manager as the backend
type GCPSecretManagerProvider struct {
	client  *secretmanager.Client
	project string
	prefix  string
}

// NewGCPSecretManagerProvider creates a new GCP Secret Manager provider
func NewGCPSecretManagerProvider(cfg *Config) (*GCPSecretManagerProvider, error) {
	if cfg.GCPProject == "" {
		return nil, fmt.Errorf("%w: GCP project is required", ErrProviderError)
	}

	ctx := context.Background()
	client, err := secretmanager.NewClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCP Secret Manager client: %w", err)
	}

	prefix := cfg.GCPPrefix
	if prefix == "" {
		prefix = "flowcatalyst-"
	}

	return &GCPSecretManagerProvider{
		client:  client,
		project: cfg.GCPProject,
		prefix:  prefix,
	}, nil
}

// Get retrieves a secret from GCP Secret Manager
func (p *GCPSecretManagerProvider) Get(ctx context.Context, key string) (string, error) {
	secretName := p.secretName(key)
	versionName := secretName + "/versions/latest"

	req := &secretmanagerpb.AccessSecretVersionRequest{
		Name: versionName,
	}

	result, err := p.client.AccessSecretVersion(ctx, req)
	if err != nil {
		if isGCPNotFoundError(err) {
			return "", ErrSecretNotFound
		}
		return "", fmt.Errorf("%w: %v", ErrProviderError, err)
	}

	return string(result.Payload.Data), nil
}

// Set stores a secret in GCP Secret Manager
func (p *GCPSecretManagerProvider) Set(ctx context.Context, key, value string) error {
	secretName := p.secretName(key)

	// Try to create the secret first
	createReq := &secretmanagerpb.CreateSecretRequest{
		Parent:   fmt.Sprintf("projects/%s", p.project),
		SecretId: p.prefix + key,
		Secret: &secretmanagerpb.Secret{
			Replication: &secretmanagerpb.Replication{
				Replication: &secretmanagerpb.Replication_Automatic_{
					Automatic: &secretmanagerpb.Replication_Automatic{},
				},
			},
		},
	}

	_, err := p.client.CreateSecret(ctx, createReq)
	if err != nil && !isGCPAlreadyExistsError(err) {
		return fmt.Errorf("%w: failed to create secret: %v", ErrProviderError, err)
	}

	// Add a new version with the secret value
	addVersionReq := &secretmanagerpb.AddSecretVersionRequest{
		Parent: secretName,
		Payload: &secretmanagerpb.SecretPayload{
			Data: []byte(value),
		},
	}

	_, err = p.client.AddSecretVersion(ctx, addVersionReq)
	if err != nil {
		return fmt.Errorf("%w: failed to add secret version: %v", ErrProviderError, err)
	}

	return nil
}

// Delete removes a secret from GCP Secret Manager
func (p *GCPSecretManagerProvider) Delete(ctx context.Context, key string) error {
	secretName := p.secretName(key)

	req := &secretmanagerpb.DeleteSecretRequest{
		Name: secretName,
	}

	err := p.client.DeleteSecret(ctx, req)
	if err != nil {
		if isGCPNotFoundError(err) {
			return ErrSecretNotFound
		}
		return fmt.Errorf("%w: %v", ErrProviderError, err)
	}

	return nil
}

// Name returns the provider name
func (p *GCPSecretManagerProvider) Name() string {
	return "gcp-sm"
}

// Close closes the GCP client
func (p *GCPSecretManagerProvider) Close() error {
	return p.client.Close()
}

// secretName returns the full secret name for a key
func (p *GCPSecretManagerProvider) secretName(key string) string {
	return fmt.Sprintf("projects/%s/secrets/%s%s", p.project, p.prefix, key)
}

// isGCPNotFoundError checks if the error is a GCP not found error
func isGCPNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	st, ok := status.FromError(err)
	if !ok {
		return false
	}
	return st.Code() == codes.NotFound
}

// isGCPAlreadyExistsError checks if the error is a GCP already exists error
func isGCPAlreadyExistsError(err error) bool {
	if err == nil {
		return false
	}
	st, ok := status.FromError(err)
	if !ok {
		return false
	}
	return st.Code() == codes.AlreadyExists
}
