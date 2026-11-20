package payloadstore

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/sqlc/dbq"
	"github.com/flowcatalyst/flowcatalyst-go/internal/tsid"
)

// Service packs, stores and resolves payloads. Built once at startup and
// shared; safe for concurrent use.
type Service struct {
	cfg   Config      // validated, with defaults filled
	store ObjectStore // nil: no store, nothing is offloaded
	q     *dbq.Queries
	cache *cache
	// health records the outcomes of the store's operations (Health).
	health *health
	now    func() time.Time
	newID  func() string
}

// New validates cfg, opens the store its URL names (none when empty) and
// returns the service. db is where Get reads msg_payloads (a pool); nil
// leaves Get unusable, for a process that only packs. An invalid
// configuration or an unusable store URL is an error, so the server does not
// start. A store that is configured but cannot be used is not: the one check
// New makes of it (checkAtStartup) is recorded for Health, never fatal.
func New(ctx context.Context, cfg Config, db dbq.DBTX) (*Service, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	store, err := OpenStore(ctx, cfg.Store)
	if err != nil {
		return nil, err
	}
	s, err := NewWithStore(cfg, store, db)
	if err != nil {
		return nil, err
	}
	s.checkAtStartup(ctx)
	return s, nil
}

// NewWithStore is New with the store given (nil for none) instead of opened
// from cfg.Store: for tests, with a MemoryStore or a failing store.
func NewWithStore(cfg Config, store ObjectStore, db dbq.DBTX) (*Service, error) {
	if err := cfg.validate(store != nil); err != nil {
		return nil, err
	}
	cfg = cfg.withDefaults()
	s := &Service{
		cfg:    cfg,
		store:  store,
		cache:  newCache(cfg.CacheBytes),
		health: &health{},
		now:    func() time.Time { return time.Now().UTC() },
		newID:  tsid.GenerateUntyped,
	}
	if db != nil {
		s.q = dbq.New(db)
	}
	s.logConfig()
	return s, nil
}

func (s *Service) logConfig() {
	if s.store == nil {
		slog.Info("payload store: none configured; payloads are kept in the database",
			"inline_max_bytes", s.cfg.InlineMaxBytes, "table_writes", s.cfg.TableWrites,
			"ingest_batch_max_bytes", s.cfg.IngestBatchMaxBytes)
		return
	}
	kind, location := s.store.Describe()
	slog.Info("payload store: configured", "store_kind", kind, "location", location,
		"offload_min_bytes", s.cfg.OffloadMinBytes, "inline_max_bytes", s.cfg.InlineMaxBytes,
		"max_bytes", s.cfg.MaxBytes, "codec", string(s.cfg.Codec), "cache_bytes", s.cfg.CacheBytes,
		"table_writes", s.cfg.TableWrites, "largest_payload_bytes", s.MaxPayloadBytes(),
		"ingest_batch_max_bytes", s.cfg.IngestBatchMaxBytes)
}

// TableWrites reports whether the writers store payloads as msg_payloads rows
// (FC_PAYLOAD_TABLE_WRITES, the switch of docs/payload-offload-design.md,
// section 10). This is the one place the switch is read. False for a nil
// service: a writer built without one writes the legacy shape.
func (s *Service) TableWrites() bool { return s != nil && s.cfg.TableWrites }

// StoreConfigured reports whether a payload store is configured. References
// are readable only when it is.
func (s *Service) StoreConfigured() bool { return s.store != nil }

// offloading reports whether payloads over the threshold are written to the
// store: a store is configured and the threshold is not 0.
func (s *Service) offloading() bool { return s.store != nil && s.cfg.OffloadMinBytes > 0 }

// MaxPayloadBytes is the largest payload accepted (largestPayload):
// MaxBytes while payloads are offloaded, which needs the payload table written
// (TableWrites), a store and a threshold that is not 0; InlineMaxBytes
// otherwise, because every payload is then kept in the database (with the
// switch off, the legacy way). DefaultInlineMaxBytes for a nil service.
//
// It holds whatever the switch says: a route refuses a larger payload with
// PAYLOAD_TOO_LARGE (CheckSize) before it writes either shape.
func (s *Service) MaxPayloadBytes() int {
	if s == nil {
		return DefaultInlineMaxBytes
	}
	return largestPayload(s.cfg, s.store != nil)
}

// storableBytes is the largest payload Prepare stores: largestPayload as if
// the payload table were written. Prepare is called only while it is, where
// this is MaxPayloadBytes.
func (s *Service) storableBytes() int {
	c := s.cfg
	c.TableWrites = true
	return largestPayload(c, s.store != nil)
}

// CheckSize refuses a payload of n bytes larger than MaxPayloadBytes with an
// error wrapping ErrTooLarge (PAYLOAD_TOO_LARGE); nil when it is not. Safe on
// a nil service (the default inline limit).
func (s *Service) CheckSize(n int) error {
	if limit := s.MaxPayloadBytes(); n > limit {
		return fmt.Errorf("%w: %d bytes, the largest accepted is %d", ErrTooLarge, n, limit)
	}
	return nil
}

// BatchRequestMaxBytes is the request body cap of a route that takes a batch
// of payloads (FC_INGEST_BATCH_MAX_BYTES; DefaultIngestBatchMaxBytes for a nil
// service). It is at least twice MaxPayloadBytes (Config.Validate).
func (s *Service) BatchRequestMaxBytes() int64 {
	if s == nil {
		return DefaultIngestBatchMaxBytes
	}
	return s.cfg.IngestBatchMaxBytes
}

// PayloadRequestMaxBytes is the request body cap of a route that carries one
// payload (a single event, a single job, a payload edit): twice
// MaxPayloadBytes, because a payload sent as a JSON string can nearly double
// when escaped, plus PayloadRequestOverhead for the rest of the request.
func (s *Service) PayloadRequestMaxBytes() int64 {
	return 2*int64(s.MaxPayloadBytes()) + PayloadRequestOverhead
}
