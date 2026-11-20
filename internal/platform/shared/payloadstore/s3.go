package payloadstore

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
	smithyhttp "github.com/aws/smithy-go/transport/http"
)

// s3API is the part of the S3 client the store uses, so the range and put
// logic is tested with a fake client.
type s3API interface {
	PutObject(ctx context.Context, in *s3.PutObjectInput, optFns ...func(*s3.Options)) (*s3.PutObjectOutput, error)
	GetObject(ctx context.Context, in *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error)
}

// S3Store keeps objects in an S3 bucket under a prefix. Configured from the
// default AWS credential chain (environment, instance profile, ECS task
// role), as the platform's other AWS clients are. The bucket's expiry is a
// lifecycle rule the infrastructure code sets (docs/payload-offload-design.md,
// section 7); the platform needs only s3:PutObject and s3:GetObject under
// the prefix. It does not implement Expirer: the platform never deletes from
// the bucket.
type S3Store struct {
	client s3API
	bucket string
	prefix string
	// credsErr is why the credential chain did not resolve at startup (nil
	// when it did, or was not tried): the startup check health records.
	credsErr error
}

// NewS3Store returns an S3Store for bucket, keys under prefix (empty for the
// bucket's root).
func NewS3Store(ctx context.Context, bucket, prefix string) (*S3Store, error) {
	if bucket == "" {
		return nil, errors.New("payloadstore: s3 store bucket must not be empty")
	}
	cfg, err := awsconfig.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("payloadstore: load AWS config: %w", err)
	}
	prefix = strings.Trim(prefix, "/")
	credsErr := logS3SelfCheck(ctx, cfg.Region, bucket, prefix, cfg.Credentials)
	st := newS3StoreWithClient(s3.NewFromConfig(cfg), bucket, prefix)
	st.credsErr = credsErr
	return st, nil
}

// startupCheck reports the credential check NewS3Store made: a failure when
// the chain did not resolve. A chain that resolved proves nothing about the
// bucket, and a request cannot either without writing an object (with only
// s3:GetObject, S3 answers a missing key 403, not 404), so it is not proven:
// health learns whether the bucket is reachable from the first real write or
// read.
func (s *S3Store) startupCheck(context.Context) (bool, error) {
	return false, s.credsErr
}

func newS3StoreWithClient(client s3API, bucket, prefix string) *S3Store {
	return &S3Store{client: client, bucket: bucket, prefix: strings.Trim(prefix, "/")}
}

// logS3SelfCheck logs the store's configuration at startup and whether the
// credential chain resolves; only the provider's source name, never a value.
// It never fails startup: a bad chain is reported here (and returned, for
// health), and a write that fails because of it falls back to the database.
func logS3SelfCheck(ctx context.Context, region, bucket, prefix string, creds aws.CredentialsProvider) error {
	attrs := []any{"store_kind", "s3", "bucket", bucket, "prefix", prefix, "region", region}
	if creds == nil {
		slog.Warn("payload store: s3 configured but no credentials provider", attrs...)
		return errors.New("no AWS credentials provider")
	}
	cctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	c, err := creds.Retrieve(cctx)
	if err != nil {
		slog.Error("payload store: s3 credentials did NOT resolve", append(attrs, "credentials_resolved", false, "err", err)...)
		return fmt.Errorf("AWS credentials did not resolve: %w", err)
	}
	slog.Info("payload store: s3 configured", append(attrs, "credentials_resolved", true,
		"credentials_source", c.Source, "credentials_expire", c.CanExpire)...)
	return nil
}

// s3Err wraps an SDK error with the operation and key, keeping the SDK's text
// and adding a hint for the two classes that are configuration problems.
func (s *S3Store) s3Err(op, key string, err error) error {
	where := fmt.Sprintf("bucket=%s key=%s", s.bucket, key)
	msg := err.Error()
	switch {
	case strings.Contains(msg, "failed to retrieve credentials"), strings.Contains(msg, "NoCredentialProviders"):
		return fmt.Errorf("payloadstore: s3 %s %s (credentials not resolvable by the AWS default chain): %w", op, where, err)
	case strings.Contains(msg, "AccessDenied"), strings.Contains(msg, "StatusCode: 403"):
		return fmt.Errorf("payloadstore: s3 %s %s (access denied: check the role's s3 permissions on the bucket and prefix): %w", op, where, err)
	}
	return fmt.Errorf("payloadstore: s3 %s %s: %w", op, where, err)
}

func (s *S3Store) key(key string) string {
	if s.prefix == "" {
		return key
	}
	return s.prefix + "/" + key
}

// Put uploads data as one object with PutObject.
func (s *S3Store) Put(ctx context.Context, key string, data []byte) error {
	if err := checkKey(key); err != nil {
		return fmt.Errorf("payloadstore: s3 put: %w", err)
	}
	full := s.key(key)
	size := int64(len(data))
	contentType := "application/octet-stream"
	if _, err := s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:        &s.bucket,
		Key:           &full,
		Body:          bytes.NewReader(data),
		ContentLength: &size,
		ContentType:   &contentType,
	}); err != nil {
		return s.s3Err("put_object", full, err)
	}
	return nil
}

// GetRange reads length bytes at offset with a ranged GetObject.
func (s *S3Store) GetRange(ctx context.Context, key string, offset, length int64) ([]byte, error) {
	if err := checkKey(key); err != nil {
		return nil, fmt.Errorf("payloadstore: s3 get: %w", err)
	}
	if offset < 0 || length <= 0 {
		return nil, fmt.Errorf("payloadstore: s3 get %s: invalid range %d+%d", key, offset, length)
	}
	full := s.key(key)
	rng := fmt.Sprintf("bytes=%d-%d", offset, offset+length-1)
	out, err := s.client.GetObject(ctx, &s3.GetObjectInput{Bucket: &s.bucket, Key: &full, Range: &rng})
	if err != nil {
		switch {
		case isNotFound(err):
			return nil, fmt.Errorf("%w: %s", ErrObjectNotFound, full)
		case isRangeNotSatisfiable(err):
			// The object is shorter than offset: no bytes there.
			return []byte{}, nil
		}
		return nil, s.s3Err("get_object", full, err)
	}
	if out.Body == nil {
		return []byte{}, nil
	}
	defer out.Body.Close()
	// Never read more than the range asked for, whatever the body holds.
	b, err := io.ReadAll(io.LimitReader(out.Body, length))
	if err != nil {
		return nil, s.s3Err("get_object read", full, err)
	}
	return b, nil
}

// Describe implements ObjectStore.
func (s *S3Store) Describe() (kind, location string) {
	if s.prefix == "" {
		return "s3", s.bucket
	}
	return "s3", s.bucket + "/" + s.prefix
}

// isNotFound reports an S3 "no such key" or a bare 404.
func isNotFound(err error) bool {
	if _, ok := errors.AsType[*types.NoSuchKey](err); ok {
		return true
	}
	if _, ok := errors.AsType[*types.NotFound](err); ok {
		return true
	}
	if re, ok := errors.AsType[*smithyhttp.ResponseError](err); ok && re.HTTPStatusCode() == 404 {
		return true
	}
	return false
}

// isRangeNotSatisfiable reports S3's answer to a range that starts past the
// end of the object (416 InvalidRange).
func isRangeNotSatisfiable(err error) bool {
	if ae, ok := errors.AsType[smithy.APIError](err); ok && ae.ErrorCode() == "InvalidRange" {
		return true
	}
	if re, ok := errors.AsType[*smithyhttp.ResponseError](err); ok && re.HTTPStatusCode() == 416 {
		return true
	}
	return false
}
