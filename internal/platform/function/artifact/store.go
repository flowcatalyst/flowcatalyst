// Package artifact is the function-runner artifact store
// (docs/function-runner-plan.md §8.4): content-addressed by sha256, with a
// file:// implementation (fc-dev default) and an s3:// implementation
// (production). The platform never runs guest code except at publish
// (describe-only, no capabilities) — this package only stores and serves
// the raw .wasm bytes; loading/compiling them is the runner's job
// (internal/functions/engine, a later work package).
package artifact

import (
	"context"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"
)

// DigestPattern is the wire/storage digest rule: lowercase hex sha256 (64
// characters).
var DigestPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

// ValidDigest reports whether digest matches DigestPattern.
func ValidDigest(digest string) bool { return DigestPattern.MatchString(digest) }

// ErrDigestMismatch is returned by Put when the streamed content's sha256
// does not equal the requested digest. Callers (the artifacts API handler)
// map this to 400 DIGEST_MISMATCH.
var ErrDigestMismatch = errors.New("artifact: content does not match the requested digest")

// ErrNotFound is returned by Open when no artifact exists for the digest.
var ErrNotFound = errors.New("artifact: not found")

// Store is the artifact store contract. All methods are content-addressed by
// digest (lowercase hex sha256, 64 characters) — callers should validate
// with ValidDigest before calling.
type Store interface {
	// Put streams r into the store under digest, verifying the sha256 of
	// the streamed bytes equals digest (returning ErrDigestMismatch
	// otherwise) and writing atomically (a reader/caller never observes a
	// partially-written artifact). Idempotent: putting the same digest
	// twice with matching content succeeds both times without error.
	Put(ctx context.Context, digest string, r io.Reader) error

	// Open returns a reader for the artifact at digest. Returns ErrNotFound
	// if no artifact exists for that digest. Callers must Close the
	// returned reader.
	Open(ctx context.Context, digest string) (io.ReadCloser, error)

	// Exists reports whether an artifact is stored at digest.
	Exists(ctx context.Context, digest string) (bool, error)

	// Delete removes the artifact at digest. Not an error if it doesn't
	// exist (idempotent).
	Delete(ctx context.Context, digest string) error

	// PresignGet returns a time-limited download URL for the artifact at
	// digest. ok is false when the store has no notion of a presigned URL
	// (the file store: "the runner holds no storage credentials" doesn't
	// apply to a local path, so it has nothing to presign — plan §8.3
	// falls back to streaming from the file store in that case).
	PresignGet(ctx context.Context, digest string, ttl time.Duration) (url string, ok bool, err error)
}

// keyFor returns the content-addressed relative path for digest:
// "sha256/ab/abcdef…" — a two-character fan-out directory keeps any single
// directory from holding an unbounded number of entries.
func keyFor(digest string) string {
	return "sha256/" + digest[:2] + "/" + digest
}

// FromURL constructs a Store from a storage URL:
//
//	file:///abs/path/to/dir   — FileStore rooted at /abs/path/to/dir
//	file://relative/path      — FileStore rooted at the relative path
//	s3://bucket/prefix        — S3Store in bucket, keys under prefix
//	"" (empty)                — FileStore rooted at dataDir (the caller's
//	                            default data directory, e.g. fc-dev's state
//	                            dir), so an unset FC_FUNCTIONS_ARTIFACT_STORE
//	                            still works.
func FromURL(ctx context.Context, rawURL, dataDir string) (Store, error) {
	switch {
	case rawURL == "":
		return NewFileStore(dataDir)
	case strings.HasPrefix(rawURL, "file://"):
		return NewFileStore(strings.TrimPrefix(rawURL, "file://"))
	case strings.HasPrefix(rawURL, "s3://"):
		rest := strings.TrimPrefix(rawURL, "s3://")
		bucket, prefix, _ := strings.Cut(rest, "/")
		if bucket == "" {
			return nil, fmt.Errorf("artifact: s3:// URL missing bucket: %q", rawURL)
		}
		return NewS3Store(ctx, bucket, prefix)
	default:
		return nil, fmt.Errorf("artifact: unsupported store URL scheme: %q", rawURL)
	}
}
