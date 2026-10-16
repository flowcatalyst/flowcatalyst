package artifact

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"time"
)

// Describer is implemented by stores that can say where they live (kind,
// bucket, prefix) for diagnostics. Never expose credentials here.
type Describer interface {
	Describe() (kind, bucket, prefix string)
}

// StoreError is a store failure annotated with enough context to diagnose it
// from a log line alone: which store, which operation, which digest. The
// underlying cause is wrapped (%w) with the SDK's text intact. It carries no
// credentials and no presigned URLs.
type StoreError struct {
	Kind   string // "s3" or "file"
	Bucket string // s3 only
	Prefix string
	Op     string // exists | put | open | presign | delete
	Digest string
	Err    error
}

func (e *StoreError) Error() string {
	loc := "store=" + e.Kind
	if e.Bucket != "" {
		loc += " bucket=" + e.Bucket
	}
	if e.Prefix != "" {
		loc += " prefix=" + e.Prefix
	}
	return fmt.Sprintf("artifact store %s op=%s digest=%s: %v", loc, e.Op, e.Digest, e.Err)
}

func (e *StoreError) Unwrap() error { return e.Err }

// LogAttrs exposes the structured diagnostic fields; the platform's central
// 5xx logger picks these up from the error chain.
func (e *StoreError) LogAttrs() []slog.Attr {
	attrs := []slog.Attr{
		slog.String("store_kind", e.Kind),
		slog.String("store_op", e.Op),
		slog.String("digest", e.Digest),
	}
	if e.Bucket != "" {
		attrs = append(attrs, slog.String("bucket", e.Bucket))
	}
	if e.Prefix != "" {
		attrs = append(attrs, slog.String("prefix", e.Prefix))
	}
	return attrs
}

// WithDiagnostics wraps s so every failure that is not a sentinel
// (ErrNotFound, ErrDigestMismatch) comes back as a *StoreError. Sentinels
// pass through untouched so errors.Is keeps working for callers.
func WithDiagnostics(s Store) Store {
	d := &diagStore{Store: s}
	if desc, ok := s.(Describer); ok {
		d.kind, d.bucket, d.prefix = desc.Describe()
	}
	return d
}

type diagStore struct {
	Store
	kind, bucket, prefix string
}

func (d *diagStore) wrap(op, digest string, err error) error {
	if err == nil || errors.Is(err, ErrNotFound) || errors.Is(err, ErrDigestMismatch) {
		return err
	}
	return &StoreError{Kind: d.kind, Bucket: d.bucket, Prefix: d.prefix, Op: op, Digest: digest, Err: err}
}

func (d *diagStore) Put(ctx context.Context, digest string, r io.Reader) error {
	return d.wrap("put", digest, d.Store.Put(ctx, digest, r))
}

func (d *diagStore) Open(ctx context.Context, digest string) (io.ReadCloser, error) {
	rc, err := d.Store.Open(ctx, digest)
	return rc, d.wrap("open", digest, err)
}

func (d *diagStore) Exists(ctx context.Context, digest string) (bool, error) {
	ok, err := d.Store.Exists(ctx, digest)
	return ok, d.wrap("exists", digest, err)
}

func (d *diagStore) Delete(ctx context.Context, digest string) error {
	return d.wrap("delete", digest, d.Store.Delete(ctx, digest))
}

func (d *diagStore) PresignGet(ctx context.Context, digest string, ttl time.Duration) (string, bool, error) {
	u, ok, err := d.Store.PresignGet(ctx, digest, ttl)
	return u, ok, d.wrap("presign", digest, err)
}
