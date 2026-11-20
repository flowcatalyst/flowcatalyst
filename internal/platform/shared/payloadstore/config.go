// Package payloadstore owns the payloads of events and dispatch jobs
// (docs/payload-offload-design.md): the msg_payloads table, and the object
// store a payload larger than a configured size is offloaded to.
//
// A payload is one row of msg_payloads, written once and never updated. A row
// holds the payload's bytes as text (data), or, when the payload was
// offloaded, a reference (ref, see Ref) to where its compressed bytes are in
// a segment of the payload store. A batch's offloaded payloads are packed
// into one segment per client (Prepare), the segment is written to the store,
// and only then are the rows returned for the caller to insert beside its own
// event or job rows (QueueInsert, Insert). A reader resolves a row with Get
// (by primary key) or Resolve (a row it selected itself).
//
// Errors a caller branches on are sentinels, checked with errors.Is:
// ErrTooLarge (permanent), ErrStoreUnavailable (retryable), ErrMissing (the
// object is gone or does not match its reference; never retried) and
// ErrStoreNotConfigured (a reference met with no store).
//
// Payload bytes are never logged.
package payloadstore

import (
	"errors"
	"fmt"
	"math"
)

// Defaults and bounds of the configuration (docs/payload-offload-design.md,
// section 4).
const (
	// DefaultOffloadMinBytes: a payload larger than this is offloaded
	// (FC_PAYLOAD_OFFLOAD_MIN_BYTES).
	DefaultOffloadMinBytes = 5120
	// MinOffloadMinBytes is the smallest threshold accepted other than 0:
	// under about 2 KB a store round trip costs more than it saves.
	MinOffloadMinBytes = 2048
	// DefaultInlineMaxBytes is the largest payload kept in PostgreSQL
	// (FC_PAYLOAD_INLINE_MAX_BYTES).
	DefaultInlineMaxBytes = 1 << 20
	// DefaultMaxBytes is the largest payload accepted when a store is
	// configured (FC_PAYLOAD_MAX_BYTES): 5 MB and not 5 MiB, so a delivery in
	// an envelope still fits a receiver's 5 MiB limit (section 8).
	DefaultMaxBytes = 5_000_000
	// DefaultCacheBytes is the size of the in-process cache of resolved
	// payloads (FC_PAYLOAD_CACHE_BYTES).
	DefaultCacheBytes = 64 << 20
	// DefaultIngestBatchMaxBytes is the request body cap of the batch routes
	// (FC_INGEST_BATCH_MAX_BYTES, section 8): 16 MiB.
	DefaultIngestBatchMaxBytes int64 = 16 << 20
	// PayloadRequestOverhead is what a route that carries one payload allows
	// for the rest of its request, over twice the largest payload
	// (PayloadRequestMaxBytes).
	PayloadRequestOverhead int64 = 64 << 10
	// maxSize is the largest value msg_payloads.size (INTEGER) holds.
	maxSize = math.MaxInt32
)

// Config is the payload configuration of a deployment (section 4).
//
// The zero value is valid: no store, so nothing is offloaded, and the
// defaults for InlineMaxBytes, MaxBytes and Codec. OffloadMinBytes and
// CacheBytes are taken as they are, because their zero means something (never
// offload; no cache): start from DefaultConfig to get their defaults.
type Config struct {
	// Store is the payload store URL (FC_PAYLOAD_STORE): empty for none
	// (nothing is offloaded), s3://bucket/prefix or file:///abs/path.
	Store string
	// OffloadMinBytes: a payload larger than this is offloaded when a store
	// is configured. 0 stops offloading (references already written stay
	// readable); otherwise at least MinOffloadMinBytes, and less than
	// InlineMaxBytes.
	OffloadMinBytes int
	// InlineMaxBytes is the largest payload kept in the database: with no
	// store (or offloading stopped) the largest payload accepted, with one
	// the most kept in the database when the store cannot be written.
	// Zero means DefaultInlineMaxBytes.
	InlineMaxBytes int
	// MaxBytes is the largest payload accepted while offloading; at least
	// InlineMaxBytes. Ignored when no store is configured. Zero means
	// DefaultMaxBytes.
	MaxBytes int
	// Codec compresses an offloaded payload. Empty means CodecZstd.
	Codec Codec
	// CacheBytes bounds the cache of resolved offloaded payloads; 0 disables
	// it.
	CacheBytes int64
	// TableWrites is the switch of the design's section 10
	// (FC_PAYLOAD_TABLE_WRITES): when set, the writers store each payload as
	// a msg_payloads row and point the event or job at it; when not, they
	// write the legacy shape (the payload in the row's own column) and
	// enforce no new limit. Turn it on once every instance reads both shapes.
	// Off by default.
	TableWrites bool
	// IngestBatchMaxBytes is the request body cap of the routes that take a
	// batch of payloads (FC_INGEST_BATCH_MAX_BYTES, section 8): the events
	// and dispatch-jobs batch routes. At least twice the largest payload
	// accepted (MaxPayloadBytes): the outbox processor sends data as a JSON
	// string, and escaping lengthens it. Zero means
	// DefaultIngestBatchMaxBytes.
	IngestBatchMaxBytes int64
}

// DefaultConfig is the configuration with every default of section 4 and no
// store. IngestBatchMaxBytes is left zero, which means its default,
// DefaultIngestBatchMaxBytes.
func DefaultConfig() Config {
	return Config{
		OffloadMinBytes: DefaultOffloadMinBytes,
		InlineMaxBytes:  DefaultInlineMaxBytes,
		MaxBytes:        DefaultMaxBytes,
		Codec:           CodecZstd,
		CacheBytes:      DefaultCacheBytes,
	}
}

// withDefaults fills the fields whose zero means "the default".
func (c Config) withDefaults() Config {
	if c.InlineMaxBytes == 0 {
		c.InlineMaxBytes = DefaultInlineMaxBytes
	}
	if c.MaxBytes == 0 {
		c.MaxBytes = DefaultMaxBytes
	}
	if c.Codec == "" {
		c.Codec = CodecZstd
	}
	if c.IngestBatchMaxBytes == 0 {
		c.IngestBatchMaxBytes = DefaultIngestBatchMaxBytes
	}
	return c
}

// largestPayload is the largest payload a deployment configured with c
// accepts (docs/payload-offload-design.md, sections 4 and 8), hasStore saying
// whether a payload store is configured; c has its defaults filled. It is
// MaxBytes only while payloads are offloaded: a store, a threshold that is not
// 0, and the payload table written (TableWrites). Otherwise every payload is
// kept in the database (with the switch off, the legacy way, in the row's own
// column), so it is InlineMaxBytes. This is the one definition: the service's
// MaxPayloadBytes, the request caps derived from it and the check of
// IngestBatchMaxBytes all read it.
func largestPayload(c Config, hasStore bool) int {
	if c.TableWrites && hasStore && c.OffloadMinBytes > 0 {
		return c.MaxBytes
	}
	return c.InlineMaxBytes
}

// Validate refuses a configuration section 4 does not allow. A store is
// taken to be configured when Store is set.
func (c Config) Validate() error {
	return c.validate(c.Store != "")
}

func (c Config) validate(hasStore bool) error {
	c = c.withDefaults()
	var errs []error
	switch {
	case c.OffloadMinBytes < 0:
		errs = append(errs, fmt.Errorf("FC_PAYLOAD_OFFLOAD_MIN_BYTES=%d is negative", c.OffloadMinBytes))
	case c.OffloadMinBytes > 0 && c.OffloadMinBytes < MinOffloadMinBytes:
		errs = append(errs, fmt.Errorf("FC_PAYLOAD_OFFLOAD_MIN_BYTES=%d is too small: 0 (never offload) or at least %d (under about 2 KB a store round trip costs more than it saves)",
			c.OffloadMinBytes, MinOffloadMinBytes))
	}
	if c.InlineMaxBytes < 0 || c.InlineMaxBytes > maxSize {
		errs = append(errs, fmt.Errorf("FC_PAYLOAD_INLINE_MAX_BYTES=%d is out of range: 1 to %d", c.InlineMaxBytes, maxSize))
	}
	if c.MaxBytes < 0 || c.MaxBytes > maxSize {
		errs = append(errs, fmt.Errorf("FC_PAYLOAD_MAX_BYTES=%d is out of range: 1 to %d", c.MaxBytes, maxSize))
	}
	if _, err := ParseCodec(string(c.Codec)); err != nil {
		errs = append(errs, fmt.Errorf("FC_PAYLOAD_CODEC: %w", err))
	}
	if c.CacheBytes < 0 {
		errs = append(errs, fmt.Errorf("FC_PAYLOAD_CACHE_BYTES=%d is negative", c.CacheBytes))
	}
	// A batch must carry at least one payload of the largest size: the
	// outbox processor sends data as a JSON string, and escaping makes it
	// longer, up to about twice as long.
	if largest := int64(largestPayload(c, hasStore)); c.IngestBatchMaxBytes < 2*largest {
		errs = append(errs, fmt.Errorf("FC_INGEST_BATCH_MAX_BYTES=%d is less than twice the largest payload accepted (%d bytes): a payload sent as a JSON string can nearly double when escaped",
			c.IngestBatchMaxBytes, largest))
	}
	if hasStore {
		if c.MaxBytes < c.InlineMaxBytes {
			errs = append(errs, fmt.Errorf("FC_PAYLOAD_MAX_BYTES=%d is less than FC_PAYLOAD_INLINE_MAX_BYTES=%d", c.MaxBytes, c.InlineMaxBytes))
		}
		// A payload above the inline maximum must be offloaded, so the
		// threshold must lie below it.
		if c.OffloadMinBytes > 0 && c.OffloadMinBytes >= c.InlineMaxBytes {
			errs = append(errs, fmt.Errorf("FC_PAYLOAD_OFFLOAD_MIN_BYTES=%d must be less than FC_PAYLOAD_INLINE_MAX_BYTES=%d", c.OffloadMinBytes, c.InlineMaxBytes))
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("payloadstore: invalid configuration: %w", errors.Join(errs...))
	}
	return nil
}

// Errors a caller branches on (errors.Is). Each is returned wrapped with
// context.
var (
	// ErrTooLarge: the payload is larger than the largest accepted
	// (MaxBytes while offloading, InlineMaxBytes otherwise). Permanent: the
	// same payload is refused every time (PAYLOAD_TOO_LARGE).
	ErrTooLarge = errors.New("payloadstore: payload too large")
	// ErrStoreUnavailable: the store could not be written (a payload too
	// large for the database) or read. Retryable: nothing is wrong with the
	// payload.
	ErrStoreUnavailable = errors.New("payloadstore: payload store unavailable")
	// ErrMissing: a stored payload cannot be produced: its row or object is
	// gone, or the bytes read do not match the reference (checksum, length,
	// decompression, size). Not retried (PAYLOAD_MISSING); logged at error.
	ErrMissing = errors.New("payloadstore: payload missing")
	// ErrNotText: the payload is not text the payload table can hold: not
	// valid UTF-8, or it contains a NUL byte. Permanent: the same payload is
	// refused every time (a per-item BAD_REQUEST on the batch routes).
	ErrNotText = errors.New("payloadstore: payload is not UTF-8 text")
	// ErrStoreNotConfigured: a row holds a reference but no store is
	// configured (PAYLOAD_STORE_NOT_CONFIGURED).
	ErrStoreNotConfigured = errors.New("payloadstore: payload store not configured")
	// ErrObjectNotFound is returned by an ObjectStore when no object exists
	// under the key.
	ErrObjectNotFound = errors.New("payloadstore: object not found")
)
