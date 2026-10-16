package artifact

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
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
	smithyhttp "github.com/aws/smithy-go/transport/http"
)

// S3Store is a content-addressed artifact store backed by an S3 bucket.
// Configured from the default AWS credential chain (env, instance profile,
// ECS task role, …) — same convention as the platform's other AWS clients
// (internal/server/dbsecret.go, internal/queue/sqs).
type S3Store struct {
	client *s3.Client
	bucket string
	prefix string
}

// NewS3Store returns an S3Store for bucket, storing keys under prefix
// (empty prefix means the bucket root).
func NewS3Store(ctx context.Context, bucket, prefix string) (*S3Store, error) {
	if bucket == "" {
		return nil, errors.New("artifact: s3 store bucket must not be empty")
	}
	cfg, err := awsconfig.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("artifact: load AWS config: %w", err)
	}
	prefix = strings.Trim(prefix, "/")
	logS3SelfCheck(ctx, cfg.Region, bucket, prefix, cfg.Credentials)
	return &S3Store{
		client: s3.NewFromConfig(cfg),
		bucket: bucket,
		prefix: prefix,
	}, nil
}

// Describe implements Describer.
func (s *S3Store) Describe() (kind, bucket, prefix string) { return "s3", s.bucket, s.prefix }

// logS3SelfCheck logs the effective S3 configuration at startup and whether
// the credential chain resolves. Values are never logged — only the provider
// source name. It never fails startup: a bad credential chain is reported
// here and again, with full detail, on the first request that needs S3.
func logS3SelfCheck(ctx context.Context, region, bucket, prefix string, creds aws.CredentialsProvider) {
	attrs := []any{"store_kind", "s3", "bucket", bucket, "prefix", prefix, "region", region}
	if creds == nil {
		slog.Warn("artifact store: s3 configured but no credentials provider", attrs...)
		return
	}
	cctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	c, err := creds.Retrieve(cctx)
	if err != nil {
		slog.Error("artifact store: s3 credentials did NOT resolve", append(attrs, "credentials_resolved", false, "err", err)...)
		return
	}
	slog.Info("artifact store: s3 configured", append(attrs, "credentials_resolved", true,
		"credentials_source", c.Source, "credentials_expire", c.CanExpire)...)
}

// s3Err wraps an SDK error with the operation, keeping the SDK's text (which
// names AccessDenied / failed to retrieve credentials / the HTTP status) and
// adding a short hint for the two classes that are configuration problems.
func s3Err(op string, err error) error {
	msg := err.Error()
	switch {
	case strings.Contains(msg, "failed to retrieve credentials"), strings.Contains(msg, "NoCredentialProviders"):
		return fmt.Errorf("artifact: s3 %s (credentials not resolvable by the AWS default chain): %w", op, err)
	case strings.Contains(msg, "AccessDenied"), strings.Contains(msg, "StatusCode: 403"):
		return fmt.Errorf("artifact: s3 %s (access denied: check the task role's s3 permissions on the bucket/prefix): %w", op, err)
	}
	return fmt.Errorf("artifact: s3 %s: %w", op, err)
}

func (s *S3Store) key(digest string) string {
	if s.prefix == "" {
		return keyFor(digest)
	}
	return s.prefix + "/" + keyFor(digest)
}

// Put buffers r (functions are capped at 64 MiB — plan §8.2 — so buffering
// in memory to compute the digest before upload is simple and safe) while
// hashing it, verifies the hash equals digest, then uploads with PutObject.
// Idempotent: S3 PutObject is itself an overwrite of the same key, and
// content-addressing means an overwrite of a matching digest is a no-op in
// substance.
func (s *S3Store) Put(ctx context.Context, digest string, r io.Reader) error {
	if !ValidDigest(digest) {
		return fmt.Errorf("artifact: invalid digest %q", digest)
	}

	var buf bytes.Buffer
	h := sha256.New()
	if _, err := io.Copy(io.MultiWriter(&buf, h), r); err != nil {
		return fmt.Errorf("artifact: read upload body: %w", err)
	}
	got := hex.EncodeToString(h.Sum(nil))
	if got != digest {
		return fmt.Errorf("%w: computed %s, requested %s", ErrDigestMismatch, got, digest)
	}

	key := s.key(digest)
	size := int64(buf.Len())
	_, err := s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:        &s.bucket,
		Key:           &key,
		Body:          bytes.NewReader(buf.Bytes()),
		ContentLength: &size,
	})
	if err != nil {
		return s3Err("put_object", err)
	}
	return nil
}

// Open returns a reader for the artifact at digest.
func (s *S3Store) Open(ctx context.Context, digest string) (io.ReadCloser, error) {
	key := s.key(digest)
	out, err := s.client.GetObject(ctx, &s3.GetObjectInput{Bucket: &s.bucket, Key: &key})
	if err != nil {
		if isNotFound(err) {
			return nil, ErrNotFound
		}
		return nil, s3Err("get_object", err)
	}
	return out.Body, nil
}

// Exists reports whether an artifact is stored at digest.
func (s *S3Store) Exists(ctx context.Context, digest string) (bool, error) {
	key := s.key(digest)
	_, err := s.client.HeadObject(ctx, &s3.HeadObjectInput{Bucket: &s.bucket, Key: &key})
	if err != nil {
		if isNotFound(err) {
			return false, nil
		}
		return false, s3Err("head_object", err)
	}
	return true, nil
}

// Delete removes the artifact at digest. Not an error if absent (S3
// DeleteObject is itself idempotent).
func (s *S3Store) Delete(ctx context.Context, digest string) error {
	key := s.key(digest)
	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{Bucket: &s.bucket, Key: &key})
	if err != nil {
		return s3Err("delete_object", err)
	}
	return nil
}

// PresignGet returns a presigned GET URL valid for ttl. The runner "holds
// no storage credentials" (plan §8.3) — it follows this URL instead.
func (s *S3Store) PresignGet(ctx context.Context, digest string, ttl time.Duration) (string, bool, error) {
	key := s.key(digest)
	presigner := s3.NewPresignClient(s.client)
	req, err := presigner.PresignGetObject(ctx, &s3.GetObjectInput{Bucket: &s.bucket, Key: &key},
		s3.WithPresignExpires(ttl))
	if err != nil {
		return "", false, s3Err("presign get_object", err)
	}
	return req.URL, true, nil
}

// isNotFound reports whether err is an S3 "no such key"/404 response —
// covers both the typed NoSuchKey error and a bare HTTP 404 (HeadObject
// returns the latter, not a typed error, per the AWS SDK v2 S3 client).
func isNotFound(err error) bool {
	if _, ok := errors.AsType[*types.NoSuchKey](err); ok {
		return true
	}
	if re, ok := errors.AsType[*smithyhttp.ResponseError](err); ok && re.HTTPStatusCode() == 404 {
		return true
	}
	return false
}
