package credentialpublish

import (
	"context"
	"errors"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	ssmtypes "github.com/aws/aws-sdk-go-v2/service/ssm/types"
	"github.com/aws/smithy-go"
)

// TagKey is the tag every published parameter carries; its value is TagValue.
// The platform overwrites or deletes only a parameter that has it, with the
// owner's value: it never touches a parameter it did not create.
const TagKey = "flowcatalyst:managed"

// TagValue is the tag value for an owner's parameters.
func TagValue(o Owner) string { return string(o.Type) + "/" + o.ID }

// ErrNotManaged is a parameter that FlowCatalyst did not create (no tag, or
// another owner's) at a path it was asked to write or delete.
var ErrNotManaged = errors.New("a parameter FlowCatalyst does not manage exists at that path")

// Store is where credentials are published (spec §9). Only Parameter Store is
// built.
type Store interface {
	// Put writes value at path, creating it tagged with tag, or replacing it
	// when it already carries that tag. ErrNotManaged when it exists without.
	Put(ctx context.Context, path, value, tag string) error
	// Delete removes the parameter at path when it carries tag: nil when there
	// is none, ErrNotManaged when it is somebody else's.
	Delete(ctx context.Context, path, tag string) error
}

// SSMWriteAPI is the slice of the Systems Manager client publishing calls;
// tests supply a fake (credentialpublishtest).
type SSMWriteAPI interface {
	PutParameter(ctx context.Context, in *ssm.PutParameterInput, opts ...func(*ssm.Options)) (*ssm.PutParameterOutput, error)
	DeleteParameter(ctx context.Context, in *ssm.DeleteParameterInput, opts ...func(*ssm.Options)) (*ssm.DeleteParameterOutput, error)
	ListTagsForResource(ctx context.Context, in *ssm.ListTagsForResourceInput, opts ...func(*ssm.Options)) (*ssm.ListTagsForResourceOutput, error)
}

// NewSSMClient builds the Systems Manager client from the default credential
// chain (env, instance profile, task role), as the reference resolver does.
// No call is made until the first publish.
func NewSSMClient(ctx context.Context) (SSMWriteAPI, error) {
	cfg, err := awsconfig.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("load AWS config: %w", err)
	}
	return ssm.NewFromConfig(cfg), nil
}

type parameterStore struct {
	api    SSMWriteAPI
	kmsKey string
}

// NewParameterStore is the Store over Parameter Store, each parameter a
// SecureString under kmsKey (empty: the account's aws/ssm key).
func NewParameterStore(api SSMWriteAPI, kmsKey string) Store {
	return &parameterStore{api: api, kmsKey: kmsKey}
}

func (s *parameterStore) Put(ctx context.Context, path, value, tag string) error {
	in := &ssm.PutParameterInput{
		Name:      aws.String(path),
		Value:     aws.String(value),
		Type:      ssmtypes.ParameterTypeSecureString,
		Overwrite: aws.Bool(false),
		Tags:      []ssmtypes.Tag{{Key: aws.String(TagKey), Value: aws.String(tag)}},
	}
	if s.kmsKey != "" {
		in.KeyId = aws.String(s.kmsKey)
	}
	_, err := s.api.PutParameter(ctx, in)
	if err == nil || ErrorCode(err) != "ParameterAlreadyExists" {
		return err
	}
	managed, err := s.managed(ctx, path, tag)
	if err != nil {
		return err
	}
	if !managed {
		return ErrNotManaged
	}
	// AWS refuses tags together with Overwrite; the tag is already there.
	in.Overwrite = aws.Bool(true)
	in.Tags = nil
	_, err = s.api.PutParameter(ctx, in)
	return err
}

func (s *parameterStore) Delete(ctx context.Context, path, tag string) error {
	managed, err := s.managed(ctx, path, tag)
	if err != nil {
		switch ErrorCode(err) {
		case "InvalidResourceId", "ParameterNotFound":
			return nil
		}
		return err
	}
	if !managed {
		return ErrNotManaged
	}
	_, err = s.api.DeleteParameter(ctx, &ssm.DeleteParameterInput{Name: aws.String(path)})
	if ErrorCode(err) == "ParameterNotFound" {
		return nil
	}
	return err
}

// managed reports whether the parameter carries the managed tag with tag's
// value. Parameter Store answers InvalidResourceId for a name that does not exist.
func (s *parameterStore) managed(ctx context.Context, path, tag string) (bool, error) {
	out, err := s.api.ListTagsForResource(ctx, &ssm.ListTagsForResourceInput{
		ResourceType: ssmtypes.ResourceTypeForTaggingParameter,
		ResourceId:   aws.String(path),
	})
	if err != nil {
		return false, err
	}
	for _, t := range out.TagList {
		if aws.ToString(t.Key) == TagKey && aws.ToString(t.Value) == tag {
			return true, nil
		}
	}
	return false, nil
}

// ErrorCode is the AWS error code in err's chain, or "".
func ErrorCode(err error) string {
	if apiErr, ok := errors.AsType[smithy.APIError](err); ok {
		return apiErr.ErrorCode()
	}
	return ""
}

// errorText is what the item records of a failure: the AWS code and message,
// never a value (the SDK does not echo the request body).
func errorText(err error, path string) string {
	if errors.Is(err, ErrNotManaged) {
		return "a parameter FlowCatalyst does not manage already exists at " + path
	}
	text := err.Error()
	if apiErr, ok := errors.AsType[smithy.APIError](err); ok {
		text = apiErr.ErrorCode() + ": " + apiErr.ErrorMessage()
	}
	if len(text) > 500 {
		text = text[:500]
	}
	return text
}
