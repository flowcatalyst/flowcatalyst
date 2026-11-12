// Package secretreftest is a fake AWS backend for tests of code that resolves
// secret references through a real *secretref.Service (no test calls AWS).
package secretreftest

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/smithy-go"

	"github.com/flowcatalyst/flowcatalyst-go/internal/secretref"
)

// AWS serves Secrets Manager secrets (by name) and nothing else.
type AWS map[string]string

// New is a resolver with the allow-list over the fake.
func New(secrets map[string]string, allow ...string) *secretref.Service {
	return secretref.New(secretref.Config{Allow: allow}, AWS(secrets))
}

func (a AWS) SecretsManager(context.Context, string) (secretref.SecretsManagerAPI, error) {
	return a, nil
}

func (a AWS) SSM(context.Context, string) (secretref.SSMAPI, error) { return ssmNone{}, nil }

func (a AWS) GetSecretValue(_ context.Context, in *secretsmanager.GetSecretValueInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.GetSecretValueOutput, error) {
	v, ok := a[*in.SecretId]
	if !ok {
		return nil, &smithy.GenericAPIError{Code: "ResourceNotFoundException", Message: "no such secret"}
	}
	return &secretsmanager.GetSecretValueOutput{SecretString: &v}, nil
}

type ssmNone struct{}

func (ssmNone) GetParameter(context.Context, *ssm.GetParameterInput, ...func(*ssm.Options)) (*ssm.GetParameterOutput, error) {
	return nil, &smithy.GenericAPIError{Code: "ParameterNotFound", Message: "no such parameter"}
}
