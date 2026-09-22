package stream

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/flowcatalyst/flowcatalyst-go/internal/common"
	"github.com/flowcatalyst/flowcatalyst-go/internal/tsid"
)

// FanOut matches each new event against the active subscriptions and
// inserts a dispatch job per match.
//
// Subscription set is loaded with a small projection query and cached
// locally; the cache TTL controls how stale subscription edits can be
// before fanout picks them up. Cache stays in this package to keep this
// loop independent of the platform layer.
//
// At-least-once semantics: dispatch-job inserts and `fanned_out_at`
// stamps land in one transaction. FOR UPDATE SKIP LOCKED on the claim
// makes it safe to run multiple stream nodes against the same DB.
type FanOut struct {
	pool            *pgxpool.Pool
	subscriptionTTL time.Duration

	cacheMu       sync.Mutex
	subs          []cachedSubscription
	lastCacheLoad time.Time
}

// FanOutConfig tunes the subscription cache.
type FanOutConfig struct {
	// SubscriptionTTL controls how long the cached subscription set is
	// reused before being refetched. Default 5s.
	SubscriptionTTL time.Duration
}

// DefaultFanOutConfig returns the standard defaults.
func DefaultFanOutConfig() FanOutConfig {
	return FanOutConfig{SubscriptionTTL: 5 * time.Second}
}

// NewFanOut wires the fan-out processor.
func NewFanOut(pool *pgxpool.Pool) *FanOut {
	return NewFanOutWithConfig(pool, DefaultFanOutConfig())
}

// NewFanOutWithConfig wires the fan-out processor with an explicit config.
func NewFanOutWithConfig(pool *pgxpool.Pool, cfg FanOutConfig) *FanOut {
	if cfg.SubscriptionTTL <= 0 {
		cfg.SubscriptionTTL = 5 * time.Second
	}
	return &FanOut{pool: pool, subscriptionTTL: cfg.SubscriptionTTL}
}

// Projector returns the configured Projector ready to Run.
func (f *FanOut) Projector(cfg ProjectorConfig) *Projector {
	return &Projector{
		Name: "event_fan_out",
		Pool: f.pool,
		Cfg:  cfg,
		Step: f.step,
	}
}

func (f *FanOut) step(ctx context.Context, batchSize int) (int, error) {
	subs, err := f.subscriptions(ctx)
	if err != nil {
		return 0, fmt.Errorf("load subscriptions: %w", err)
	}

	// Fast path: no active subscriptions. Stamp events as fanned-out
	// without opening a long transaction.
	if len(subs) == 0 {
		tag, err := f.pool.Exec(ctx,
			`WITH batch AS (
			    SELECT id, created_at
			      FROM msg_events
			     WHERE fanned_out_at IS NULL
			     ORDER BY created_at
			     LIMIT $1
			 )
			 UPDATE msg_events e
			    SET fanned_out_at = NOW()
			   FROM batch b
			  WHERE e.id = b.id AND e.created_at = b.created_at`, batchSize)
		if err != nil {
			return 0, fmt.Errorf("stamp no-subs: %w", err)
		}
		return int(tag.RowsAffected()), nil
	}

	tx, err := f.pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	claimed, err := claimUnfannedEvents(ctx, tx, batchSize)
	if err != nil {
		return 0, fmt.Errorf("claim: %w", err)
	}
	if len(claimed) == 0 {
		return 0, nil
	}

	jobs := buildJobs(claimed, subs)
	if len(jobs) > 0 {
		if err := insertJobsInTx(ctx, tx, jobs); err != nil {
			return 0, fmt.Errorf("insert jobs: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("commit: %w", err)
	}
	return len(claimed), nil
}

// ── Event claim ──────────────────────────────────────────────────────────

// claimedEvent carries just the columns fanout needs from msg_events.
type claimedEvent struct {
	ID            string
	EventType     string
	Source        string
	Subject       *string
	Data          json.RawMessage
	CorrelationID *string
	MessageGroup  *string
	ClientID      *string
	CreatedAt     time.Time
	// ContextData is the event's key/value tags (msg_events.context_data,
	// a JSON array of {key,value}) — copied verbatim onto each raised job's
	// metadata, which has the same shape, so a job shows the same
	// "additional data" its event does (2026-09-22). nil when the event
	// has none.
	ContextData json.RawMessage
}

// claimUnfannedEvents stamps `fanned_out_at` and returns the claimed
// rows in one shot via a single CTE.
func claimUnfannedEvents(ctx context.Context, tx pgx.Tx, batchSize int) ([]claimedEvent, error) {
	rows, err := tx.Query(ctx,
		`WITH batch AS (
		    SELECT id, created_at
		      FROM msg_events
		     WHERE fanned_out_at IS NULL
		     ORDER BY created_at
		     LIMIT $1
		     FOR UPDATE SKIP LOCKED
		 )
		 UPDATE msg_events e
		    SET fanned_out_at = NOW()
		   FROM batch b
		  WHERE e.id = b.id AND e.created_at = b.created_at
		 RETURNING e.id, e.type, e.source, e.subject, e.data,
		           e.correlation_id, e.message_group, e.client_id, e.created_at,
		           e.context_data`,
		batchSize)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []claimedEvent
	for rows.Next() {
		var e claimedEvent
		var data, contextData []byte
		if err := rows.Scan(&e.ID, &e.EventType, &e.Source, &e.Subject, &data,
			&e.CorrelationID, &e.MessageGroup, &e.ClientID, &e.CreatedAt,
			&contextData); err != nil {
			return nil, err
		}
		if len(data) > 0 {
			e.Data = data
		}
		if len(contextData) > 0 {
			e.ContextData = contextData
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// ── Subscription cache ───────────────────────────────────────────────────

// cachedSubscription is the minimal field set fanout needs. Loaded by
// `loadActiveSubscriptions` and refreshed every SubscriptionTTL.
type cachedSubscription struct {
	ID               string
	ClientID         *string
	Target           string
	Mode             common.DispatchMode
	DataOnly         bool
	DispatchPoolID   *string
	ServiceAccountID *string
	MaxRetries       int32
	TimeoutSeconds   int32
	Sequence         int32
	// Queue is the subscription's raw stored dispatch priority — copied
	// verbatim onto a raised job's own queue column (R2). nil when the
	// subscription has none set.
	Queue *string
	// Name becomes each raised job's descriptor — what the job IS, in
	// words, on the dispatch-jobs grid (2026-09-22).
	Name              string
	EventTypePatterns []string
}

func (s *cachedSubscription) matchesEventType(code string) bool {
	for _, p := range s.EventTypePatterns {
		if patternMatches(p, code) {
			return true
		}
	}
	return false
}

func (s *cachedSubscription) matchesClient(eventClient *string) bool {
	if s.ClientID == nil {
		return true
	}
	if eventClient == nil {
		return false
	}
	return *s.ClientID == *eventClient
}

// patternMatches is the `:`-separated wildcard match. Segment
// count must agree; `*` matches a single segment.
func patternMatches(pattern, code string) bool {
	pp := strings.Split(pattern, ":")
	cp := strings.Split(code, ":")
	if len(pp) != len(cp) {
		return false
	}
	for i := range pp {
		if pp[i] != "*" && pp[i] != cp[i] {
			return false
		}
	}
	return true
}

// subscriptions returns the current cache, refreshing if stale.
func (f *FanOut) subscriptions(ctx context.Context) ([]cachedSubscription, error) {
	f.cacheMu.Lock()
	defer f.cacheMu.Unlock()
	if time.Since(f.lastCacheLoad) < f.subscriptionTTL {
		return f.subs, nil
	}
	subs, err := loadActiveSubscriptions(ctx, f.pool)
	if err != nil {
		// Keep the stale cache rather than failing the cycle.
		if !f.lastCacheLoad.IsZero() {
			return f.subs, nil
		}
		return nil, err
	}
	f.subs = subs
	f.lastCacheLoad = time.Now()
	return f.subs, nil
}

func loadActiveSubscriptions(ctx context.Context, pool *pgxpool.Pool) ([]cachedSubscription, error) {
	rows, err := pool.Query(ctx,
		`SELECT s.id, s.client_id, s.target, s.mode, s.data_only,
		        s.dispatch_pool_id, s.service_account_id, s.max_retries,
		        s.timeout_seconds, s.sequence, s.queue, s.name, e.event_type_code
		   FROM msg_subscriptions s
		   LEFT JOIN msg_subscription_event_types e ON e.subscription_id = s.id
		  WHERE s.status = 'ACTIVE'
		  ORDER BY s.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	byID := map[string]*cachedSubscription{}
	var order []string
	for rows.Next() {
		var (
			id, target, mode, name                        string
			clientID, dispatchPoolID, saID, queue, etCode *string
			dataOnly                                      bool
			maxRetries, timeoutSeconds, sequence          int32
		)
		if err := rows.Scan(&id, &clientID, &target, &mode, &dataOnly,
			&dispatchPoolID, &saID, &maxRetries, &timeoutSeconds,
			&sequence, &queue, &name, &etCode); err != nil {
			return nil, err
		}
		entry, ok := byID[id]
		if !ok {
			entry = &cachedSubscription{
				ID:               id,
				ClientID:         clientID,
				Target:           target,
				Mode:             common.ParseDispatchMode(mode),
				DataOnly:         dataOnly,
				DispatchPoolID:   dispatchPoolID,
				ServiceAccountID: saID,
				MaxRetries:       maxRetries,
				TimeoutSeconds:   timeoutSeconds,
				Sequence:         sequence,
				Queue:            queue,
				Name:             name,
			}
			byID[id] = entry
			order = append(order, id)
		}
		if etCode != nil {
			entry.EventTypePatterns = append(entry.EventTypePatterns, *etCode)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := make([]cachedSubscription, 0, len(order))
	for _, id := range order {
		out = append(out, *byID[id])
	}
	return out, nil
}

// ── Dispatch job assembly + insert ───────────────────────────────────────

// newJob is the subset of msg_dispatch_jobs columns fanout sets. Other
// columns take the table default (kind='EVENT', retry_strategy='exponential',
// etc.).
type newJob struct {
	ID             string
	Code           string
	Source         string
	Subject        *string
	EventID        string
	CorrelationID  *string
	TargetURL      string
	Payload        string
	DataOnly       bool
	ServiceAcctID  *string
	ClientID       *string
	SubscriptionID string
	Mode           string
	DispatchPoolID *string
	MessageGroup   *string
	Sequence       int32
	TimeoutSeconds int32
	Status         string
	MaxRetries     int32
	IdempotencyKey string
	CreatedAt      time.Time
	// Descriptor is the raising subscription's name; Metadata the raising
	// event's context_data, verbatim (2026-09-22). nil when absent.
	Descriptor *string
	Metadata   json.RawMessage
	// Queue is the raising subscription's queue value, copied verbatim
	// (R2) — nil when the subscription has none set.
	Queue *string
}

func buildJobs(events []claimedEvent, subs []cachedSubscription) []newJob {
	var jobs []newJob
	for _, e := range events {
		for i := range subs {
			s := &subs[i]
			if !s.matchesEventType(e.EventType) {
				continue
			}
			if !s.matchesClient(e.ClientID) {
				continue
			}
			payload := "null"
			if len(e.Data) > 0 {
				payload = string(e.Data)
			}
			jobs = append(jobs, newJob{
				// 13-char untyped TSID — `msg_dispatch_jobs.id` is
				// VARCHAR(13). Using a typed prefix (`djb_...`) overflows
				// the column.
				ID:             tsid.GenerateUntyped(),
				Code:           e.EventType,
				Source:         e.Source,
				Subject:        e.Subject,
				EventID:        e.ID,
				CorrelationID:  e.CorrelationID,
				TargetURL:      s.Target,
				Payload:        payload,
				DataOnly:       s.DataOnly,
				ServiceAcctID:  s.ServiceAccountID,
				ClientID:       e.ClientID,
				SubscriptionID: s.ID,
				Mode:           dispatchModeStr(s.Mode),
				DispatchPoolID: s.DispatchPoolID,
				MessageGroup:   e.MessageGroup,
				Sequence:       s.Sequence,
				TimeoutSeconds: s.TimeoutSeconds,
				Status:         string(common.DispatchPending),
				MaxRetries:     s.MaxRetries,
				IdempotencyKey: fmt.Sprintf("%s:%s", e.ID, s.ID),
				CreatedAt:      e.CreatedAt,
				Queue:          s.Queue,
				Descriptor:     descriptorFor(s.Name),
				Metadata:       e.ContextData,
			})
		}
	}
	return jobs
}

// descriptorFor is the raised job's descriptor: the subscription's name,
// nil when it has none (the column is nullable and absent is the legacy
// state, not an empty string). Clipped to the column's width.
func descriptorFor(name string) *string {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil
	}
	if len(name) > 255 {
		name = name[:255]
	}
	return &name
}

// dispatchModeStr renders the subscription's mode for the job row. Each mode
// is spelled out: a default arm that swallowed IMMEDIATE along with anything
// unrecognised would silently rewrite an explicit choice the day the default
// changed.
func dispatchModeStr(m common.DispatchMode) string {
	switch m {
	case common.DispatchImmediate:
		return "IMMEDIATE"
	case common.DispatchBlockOnError:
		return "BLOCK_ON_ERROR"
	case common.DispatchNextOnError:
		return "NEXT_ON_ERROR"
	default:
		return string(common.DefaultDispatchMode)
	}
}

// insertJobsInTx writes the fanout-produced jobs in the same transaction
// that stamped fanned_out_at. Uses pgx.Batch — same shape as the
// dispatchjob repository's InsertBatch, but scoped to the columns
// fanout actually sets (everything else takes the table default).
func insertJobsInTx(ctx context.Context, tx pgx.Tx, jobs []newJob) error {
	if len(jobs) == 0 {
		return nil
	}
	batch := &pgx.Batch{}
	for _, j := range jobs {
		batch.Queue(
			`INSERT INTO msg_dispatch_jobs (
			    id, code, source, subject, event_id, correlation_id,
			    target_url, protocol, payload, data_only, service_account_id,
			    client_id, subscription_id, mode, dispatch_pool_id, message_group,
			    sequence, timeout_seconds, status, max_retries, idempotency_key,
			    queue, descriptor, metadata, created_at, updated_at)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, 'HTTP_WEBHOOK', $8, $9,
			         $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20,
			         $21, $22, COALESCE($23::jsonb, '[]'::jsonb), $24, $24)
			 ON CONFLICT (id, created_at) DO NOTHING`,
			j.ID, j.Code, j.Source, j.Subject, j.EventID, j.CorrelationID,
			j.TargetURL, j.Payload, j.DataOnly, j.ServiceAcctID,
			j.ClientID, j.SubscriptionID, j.Mode, j.DispatchPoolID,
			j.MessageGroup, j.Sequence, j.TimeoutSeconds, j.Status,
			j.MaxRetries, j.IdempotencyKey, j.Queue, j.Descriptor,
			nullableJSON(j.Metadata), j.CreatedAt)
	}
	br := tx.SendBatch(ctx, batch)
	defer br.Close()
	for range jobs {
		if _, err := br.Exec(); err != nil {
			return err
		}
	}
	return nil
}

// nullableJSON passes a raw JSON document as a nullable text parameter —
// nil, not an empty string, when there is none, so the SQL COALESCE can
// fall back to the column default.
func nullableJSON(raw json.RawMessage) *string {
	if len(raw) == 0 {
		return nil
	}
	s := string(raw)
	return &s
}
