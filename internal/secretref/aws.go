package secretref

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/smithy-go"
)

// SecretsManagerAPI is the slice of the Secrets Manager client the resolver
// calls; tests supply a fake.
type SecretsManagerAPI interface {
	GetSecretValue(ctx context.Context, in *secretsmanager.GetSecretValueInput, opts ...func(*secretsmanager.Options)) (*secretsmanager.GetSecretValueOutput, error)
}

// SSMAPI is the slice of the Systems Manager client the resolver calls.
type SSMAPI interface {
	GetParameter(ctx context.Context, in *ssm.GetParameterInput, opts ...func(*ssm.Options)) (*ssm.GetParameterOutput, error)
}

// Clients hands out the AWS clients for a region ("" is the default chain's
// region). Production uses NewAWSClients; tests supply fakes.
type Clients interface {
	SecretsManager(ctx context.Context, region string) (SecretsManagerAPI, error)
	SSM(ctx context.Context, region string) (SSMAPI, error)
}

// awsClients builds each region's clients once, lazily, from the default
// credential chain (env, instance profile, ECS task role, ...).
type awsClients struct {
	mu sync.Mutex
	sm map[string]*secretsmanager.Client
	ps map[string]*ssm.Client
}

// NewAWSClients is the production Clients.
func NewAWSClients() Clients {
	return &awsClients{sm: map[string]*secretsmanager.Client{}, ps: map[string]*ssm.Client{}}
}

func (a *awsClients) SecretsManager(ctx context.Context, region string) (SecretsManagerAPI, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if c, ok := a.sm[region]; ok {
		return c, nil
	}
	cfg, err := loadConfig(ctx, region)
	if err != nil {
		return nil, err
	}
	c := secretsmanager.NewFromConfig(cfg)
	a.sm[region] = c
	return c, nil
}

func (a *awsClients) SSM(ctx context.Context, region string) (SSMAPI, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if c, ok := a.ps[region]; ok {
		return c, nil
	}
	cfg, err := loadConfig(ctx, region)
	if err != nil {
		return nil, err
	}
	c := ssm.NewFromConfig(cfg)
	a.ps[region] = c
	return c, nil
}

// loadConfig loads the default AWS config, pinned to region when the
// reference is an ARN (a secret is regional and must be read from the region
// that is encoded in its ARN, even when AWS_REGION is not exported).
func loadConfig(ctx context.Context, region string) (cfg aws.Config, err error) {
	var opts []func(*awsconfig.LoadOptions) error
	if region != "" {
		opts = append(opts, awsconfig.WithRegion(region))
	}
	cfg, err = awsconfig.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return cfg, fmt.Errorf("load AWS config: %w", err)
	}
	return cfg, nil
}

// RegionFromARN extracts the region from an ARN of the form
// arn:partition:service:REGION:account:resource. Returns "" when s is not an
// ARN with a region segment (a bare name).
func RegionFromARN(s string) string {
	parts := strings.Split(s, ":")
	if len(parts) < 4 || parts[0] != "arn" {
		return ""
	}
	return parts[3]
}

// classify maps an AWS failure to a reference error code. Anything that is not
// plainly "no such thing" or "not permitted" (network, throttling, 5xx, a
// credential chain that could not load) is UNAVAILABLE: retryable.
func classify(err error) string {
	if apiErr, ok := errors.AsType[smithy.APIError](err); ok {
		switch apiErr.ErrorCode() {
		case "ResourceNotFoundException", "ParameterNotFound", "ParameterVersionNotFound":
			return CodeNotFound
		case "AccessDeniedException", "AccessDenied", "DecryptionFailure", "InvalidKeyId", "KMSAccessDeniedException":
			return CodeDenied
		}
	}
	return CodeUnavailable
}
