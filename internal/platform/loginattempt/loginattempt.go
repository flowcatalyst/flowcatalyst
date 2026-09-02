// Package loginattempt tracks user-login and service-account-token
// outcomes for backoff /
// rate-limiting and audit. Writes are infrastructure-processing (the
// auth subdomain inserts rows directly; no UoW commit per the
// conventions in docs/conventions.md §3).
package loginattempt

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/repocommon"
	"github.com/flowcatalyst/flowcatalyst-go/internal/sqlc/dbq"
	"github.com/flowcatalyst/flowcatalyst-go/internal/tsid"
)

// AttemptType identifies the kind of login.
type AttemptType string

const (
	AttemptUserLogin           AttemptType = "USER_LOGIN"
	AttemptServiceAccountToken AttemptType = "SERVICE_ACCOUNT_TOKEN"
	// AttemptDeveloperToken is a client_credentials grant using a USER
	// principal's self-service developer credential (client_id = the
	// principal's own id) — kept distinct from AttemptServiceAccountToken so
	// the audit trail doesn't blur a human's own token minting with actual
	// service-account activity.
	AttemptDeveloperToken AttemptType = "DEVELOPER_TOKEN"
)

// ParseAttemptType is the lenient parser. Unknown → USER_LOGIN.
func ParseAttemptType(s string) AttemptType {
	switch s {
	case string(AttemptServiceAccountToken):
		return AttemptServiceAccountToken
	case string(AttemptDeveloperToken):
		return AttemptDeveloperToken
	default:
		return AttemptUserLogin
	}
}

// Outcome is success/failure.
type Outcome string

const (
	OutcomeSuccess Outcome = "SUCCESS"
	OutcomeFailure Outcome = "FAILURE"
)

// ParseOutcome parses a stored/wire outcome string. Returns ok=false for
// anything other than exactly "SUCCESS" or "FAILURE" — callers MUST NOT
// default an unrecognised value to SUCCESS. That was the X-06 bug: a
// corrupted outcome column silently undercounted lockout failures, because
// the illegible row displayed as a clean success. Follows the (T, bool)
// shape of common.ParseOutboxItemType / serviceaccount.ParseAuthType.
func ParseOutcome(s string) (Outcome, bool) {
	switch Outcome(s) {
	case OutcomeSuccess, OutcomeFailure:
		return Outcome(s), true
	default:
		return "", false
	}
}

// LoginAttempt is a single attempt record.
type LoginAttempt struct {
	ID            string      `json:"id"`
	AttemptType   AttemptType `json:"attemptType"`
	Outcome       Outcome     `json:"outcome"`
	FailureReason *string     `json:"failureReason,omitempty"`
	Identifier    *string     `json:"identifier,omitempty"`
	PrincipalID   *string     `json:"principalId,omitempty"`
	IPAddress     *string     `json:"ipAddress,omitempty"`
	UserAgent     *string     `json:"userAgent,omitempty"`
	AttemptedAt   time.Time   `json:"attemptedAt"`
}

// New constructs a LoginAttempt.
func New(t AttemptType, o Outcome) *LoginAttempt {
	return &LoginAttempt{
		ID:          tsid.Generate(tsid.LoginAttempt),
		AttemptType: t,
		Outcome:     o,
		AttemptedAt: time.Now().UTC(),
	}
}

// Repository writes/reads iam_login_attempts. Direct writes (no UoW).
type Repository struct{ pool *pgxpool.Pool }

// NewRepository wires a repo.
func NewRepository(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }

// Record persists a single attempt. Called by the auth subdomain on every login.
func (r *Repository) Record(ctx context.Context, a *LoginAttempt) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO iam_login_attempts
		     (id, attempt_type, outcome, failure_reason, identifier, principal_id,
		      ip_address, user_agent, attempted_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		a.ID, string(a.AttemptType), string(a.Outcome), a.FailureReason,
		a.Identifier, a.PrincipalID, a.IPAddress, a.UserAgent, a.AttemptedAt)
	return err
}

// CountRecentFailures counts FAILURE attempts for an identifier within
// the supplied window. Used by the backoff middleware.
func (r *Repository) CountRecentFailures(ctx context.Context, identifier string, window time.Duration) (int, error) {
	since := time.Now().Add(-window).UTC()
	var count int
	err := r.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM iam_login_attempts
		   WHERE outcome = 'FAILURE'
		     AND identifier = $1
		     AND attempted_at >= $2`,
		identifier, since).Scan(&count)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return 0, fmt.Errorf("count_recent_failures: %w", err)
	}
	return count, nil
}

// lastSuccessLookback bounds LastSuccessAt's search window. iam_login_attempts
// is range-partitioned by quarter (migration 049); a bare `MAX(attempted_at)`
// with no predicate on attempted_at carries no partition-pruning information,
// so the planner must touch every existing partition on every login. 400
// days (just over a year, with margin) is chosen so any identifier that
// succeeds at least annually sees byte-identical behaviour to the old
// unbounded query.
//
// Tradeoff for anything outside the window: if an identifier's true last
// success is older than this, the query now returns nil — indistinguishable
// from "never succeeded". loginbackoff.Check already treats a nil
// LastSuccessAt by falling back to its own hardcoded 30-day cutoff for the
// per-pair backoff window (see lastSuccessCutoff in Check). That existing
// 30-day fallback already exceeds the point (~12 failures, with the default
// policy) past which the exponential delay saturates at MaxDelaySecs — so
// narrowing a long-dormant identifier's counted window from "years" to
// "capped at 30 days via the existing fallback" only reduces an
// already-saturated failure count; it does not reduce the delay actually
// applied. The global-ceiling window (window 2 in Check) is unaffected
// either way: it never looks back further than
// max(GlobalWindowSecs, GlobalLockSecs), which defaults to 1h — far inside
// this bound regardless of which cutoff window 1 computed.
//
// Owner ruling (2026-09-03): this collapsing is correct as designed and
// stays exactly this way. A dormant identifier (success older than the
// bound) is NEVER-SUCCEEDED as far as the lockout is concerned — dormancy
// must never weaken it — and there must be no unbounded/full-history
// fallback lookup to recover the true stale timestamp: a previously
// proposed fallback of that shape was retracted, since a never-succeeded
// identifier (including every enumeration probe) would then force a full
// partition scan on every single attempt.
const lastSuccessLookback = 400 * 24 * time.Hour

// LastSuccessAt returns the timestamp of the most recent SUCCESS attempt
// for an identifier within the last lastSuccessLookback, or nil when there
// has been none in that window (including "never"). Used by the login
// backoff to bound the failure-counting window. Rewritten as an
// ORDER BY ... LIMIT 1 (was MAX(...)) with an attempted_at lower bound so
// the query is both index-backed on (identifier, attempted_at) and
// partition-pruned — see lastSuccessLookback for the bound's tradeoff.
func (r *Repository) LastSuccessAt(ctx context.Context, identifier string) (*time.Time, error) {
	since := time.Now().Add(-lastSuccessLookback).UTC()
	var t *time.Time
	err := r.pool.QueryRow(ctx,
		`SELECT attempted_at FROM iam_login_attempts
		   WHERE outcome = 'SUCCESS' AND identifier = $1 AND attempted_at >= $2
		   ORDER BY attempted_at DESC
		   LIMIT 1`,
		identifier, since).Scan(&t)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("last_success_at: %w", err)
	}
	return t, nil
}

// FailureStatsByIdentifierIPSince returns the count of FAILURE attempts and
// the most recent failure timestamp for an (identifier, IP) pair since the
// cutoff. Drives the per-pair exponential backoff.
func (r *Repository) FailureStatsByIdentifierIPSince(ctx context.Context, identifier, ip string, since time.Time) (int, *time.Time, error) {
	var count int
	var last *time.Time
	err := r.pool.QueryRow(ctx,
		`SELECT COUNT(*), MAX(attempted_at) FROM iam_login_attempts
		   WHERE outcome = 'FAILURE' AND identifier = $1 AND ip_address = $2 AND attempted_at >= $3`,
		identifier, ip, since).Scan(&count, &last)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return 0, nil, fmt.Errorf("failure_stats_by_identifier_ip_since: %w", err)
	}
	return count, last, nil
}

// FailureCountByIdentifierSince counts FAILURE attempts for an identifier
// (across all IPs) since the cutoff.
func (r *Repository) FailureCountByIdentifierSince(ctx context.Context, identifier string, since time.Time) (int, error) {
	var count int
	err := r.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM iam_login_attempts
		   WHERE outcome = 'FAILURE' AND identifier = $1 AND attempted_at >= $2`,
		identifier, since).Scan(&count)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return 0, fmt.Errorf("failure_count_by_identifier_since: %w", err)
	}
	return count, nil
}

// GlobalCeilingTrippedAt returns the timestamp of the ceiling-th most recent
// FAILURE attempt for identifier at or after since — the failure whose
// arrival pushed the in-window count up to ceiling — or nil when fewer than
// ceiling failures exist in that range (the ceiling has never tripped,
// within the searched range).
//
// Drives the login backoff's global lock (see loginbackoff.Check): both the
// lock's expiry and the point the count would naturally fall back below the
// ceiling are computed from this one timestamp, rather than kept in a
// separate lock record.
func (r *Repository) GlobalCeilingTrippedAt(ctx context.Context, identifier string, since time.Time, ceiling int64) (*time.Time, error) {
	if ceiling <= 0 {
		return nil, nil
	}
	var t *time.Time
	err := r.pool.QueryRow(ctx,
		`SELECT attempted_at FROM iam_login_attempts
		   WHERE outcome = 'FAILURE' AND identifier = $1 AND attempted_at >= $2
		   ORDER BY attempted_at DESC
		   OFFSET $3 LIMIT 1`,
		identifier, since, ceiling-1).Scan(&t)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("global_ceiling_tripped_at: %w", err)
	}
	return t, nil
}

// ListParams filters a cursor-paginated query. AfterTime+AfterID together
// form the keyset cursor (exclusive) for the next page.
type ListParams struct {
	AttemptType *string
	Outcome     *string
	Identifier  *string
	PrincipalID *string
	DateFrom    *time.Time
	DateTo      *time.Time
	AfterTime   *time.Time
	AfterID     *string
	Limit       int
}

// FindPage returns attempts ordered by (attempted_at, id) DESC, applying the
// optional filters + keyset cursor. The caller fetches Limit+1 to detect
// whether more pages exist.
func (r *Repository) FindPage(ctx context.Context, p ListParams) ([]LoginAttempt, error) {
	var f repocommon.Filter
	f.EqPtr("attempt_type", p.AttemptType)
	f.EqPtr("outcome", p.Outcome)
	f.EqPtr("identifier", p.Identifier)
	f.EqPtr("principal_id", p.PrincipalID)
	if p.DateFrom != nil {
		f.Clause("attempted_at >= $%d", *p.DateFrom)
	}
	if p.DateTo != nil {
		f.Clause("attempted_at <= $%d", *p.DateTo)
	}
	if p.AfterTime != nil && p.AfterID != nil {
		// Two-argument keyset condition: push AfterTime first, then let
		// Clause append AfterID and fill in its positional index.
		t := f.Arg(*p.AfterTime)
		f.Clause(fmt.Sprintf("(attempted_at, id) < ($%d, $%%d)", t), *p.AfterID)
	}
	limit := f.Arg(p.Limit)
	q := `SELECT id, attempt_type, outcome, failure_reason, identifier, principal_id,
	             ip_address, user_agent, attempted_at
	        FROM iam_login_attempts` + f.Where() +
		fmt.Sprintf(" ORDER BY attempted_at DESC, id DESC LIMIT $%d", limit)

	rows, err := r.pool.Query(ctx, q, f.Args()...)
	if err != nil {
		return nil, err
	}
	collected, err := pgx.CollectRows(rows, pgx.RowToStructByName[dbq.IamLoginAttempt])
	if err != nil {
		return nil, err
	}
	var out []LoginAttempt
	for _, row := range collected {
		out = append(out, rowToLoginAttempt(row))
	}
	return out, nil
}

// FindRecentByIdentifier returns the most recent attempts for an identifier.
func (r *Repository) FindRecentByIdentifier(ctx context.Context, identifier string, limit int) ([]LoginAttempt, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, attempt_type, outcome, failure_reason, identifier, principal_id,
		        ip_address, user_agent, attempted_at
		   FROM iam_login_attempts WHERE identifier = $1
		   ORDER BY attempted_at DESC LIMIT $2`, identifier, limit)
	if err != nil {
		return nil, err
	}
	collected, err := pgx.CollectRows(rows, pgx.RowToStructByName[dbq.IamLoginAttempt])
	if err != nil {
		return nil, err
	}
	var out []LoginAttempt
	for _, row := range collected {
		out = append(out, rowToLoginAttempt(row))
	}
	return out, nil
}

// rowToLoginAttempt hydrates the entity from its row.
//
// This feeds the admin/self-service attempt-history display (FindPage,
// FindRecentByIdentifier), NOT the backoff counters above — those run raw
// `outcome = 'FAILURE'` / `outcome = 'SUCCESS'` SQL directly against the
// column and are unaffected by this Go-level parse. Even so, an outcome
// this code can't confidently recognise as SUCCESS must never render as
// one: that display would be the same lie the old lenient-default bug told
// the counters. Fail-SAFE, not fail-closed: log loudly with the row id (so
// the corrupt row can be found and fixed) and show it as FAILURE, rather
// than failing the whole page for one bad row.
func rowToLoginAttempt(row dbq.IamLoginAttempt) LoginAttempt {
	outcome, ok := ParseOutcome(row.Outcome)
	if !ok {
		slog.Error("login attempt row has unrecognised outcome",
			"id", row.ID, "outcome", row.Outcome)
		outcome = OutcomeFailure
	}
	return LoginAttempt{
		ID:            row.ID,
		AttemptType:   ParseAttemptType(row.AttemptType),
		Outcome:       outcome,
		FailureReason: row.FailureReason,
		Identifier:    row.Identifier,
		PrincipalID:   row.PrincipalID,
		IPAddress:     row.IpAddress,
		UserAgent:     row.UserAgent,
		AttemptedAt:   row.AttemptedAt,
	}
}

// ─── Partition maintenance (owner ruling X-03) ──────────────────────────────
//
// iam_login_attempts is range-partitioned by quarter on attempted_at as of
// migration 049. Forward and retention maintenance from here on is a Go-side
// housekeeping sweep (StartPurger in internal/server/subsystems.go) rather
// than a Postgres extension — same reasoning as internal/stream's
// PartitionManager for the messaging tables: no dependency on an extension
// allowlist, identical behaviour in dev and prod. Retention drops whole
// partitions; it never issues a row DELETE.

// loginAttemptsPartitionPrefix names every quarterly partition; the DEFAULT
// partition (created by migration 049) doesn't match it and is deliberately
// left alone by both methods below.
const loginAttemptsPartitionPrefix = "iam_login_attempts_"

// isPartitioned reports whether iam_login_attempts is a partitioned table
// (relkind 'p'). false (not an error) if the table doesn't exist at all or
// is a plain table — a drop-in over a pre-migration-049 schema shouldn't
// error on every housekeeping tick.
func (r *Repository) isPartitioned(ctx context.Context) (bool, error) {
	var relkind string
	err := r.pool.QueryRow(ctx,
		`SELECT relkind::text FROM pg_class WHERE relname = 'iam_login_attempts'`).Scan(&relkind)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		return false, fmt.Errorf("is_partitioned: %w", err)
	}
	return relkind == "p", nil
}

// EnsureQuarterlyPartition creates the iam_login_attempts partition covering
// at's calendar quarter, if it doesn't already exist. Uses CREATE TABLE IF
// NOT EXISTS … PARTITION OF, so it's idempotent without a separate
// existence check. No-op against an unpartitioned table.
func (r *Repository) EnsureQuarterlyPartition(ctx context.Context, at time.Time) error {
	partitioned, err := r.isPartitioned(ctx)
	if err != nil {
		return err
	}
	if !partitioned {
		return nil
	}
	start := quarterStart(at)
	end := start.AddDate(0, 3, 0)
	name := quarterPartitionName(start)
	_, err = r.pool.Exec(ctx, fmt.Sprintf(
		`CREATE TABLE IF NOT EXISTS %s PARTITION OF iam_login_attempts FOR VALUES FROM ('%s') TO ('%s')`,
		pgQuoteIdent(name), start.Format("2006-01-02"), end.Format("2006-01-02")))
	if err != nil {
		return fmt.Errorf("ensure_quarterly_partition %s: %w", name, err)
	}
	return nil
}

// DropPartitionsOlderThan drops every iam_login_attempts quarterly partition
// whose entire range ends before cutoff. The DEFAULT partition and any
// child whose name doesn't match the `..._YYYY_qN` convention are left
// alone. Returns the dropped partition names so the caller can log what
// happened; a schema-level DROP TABLE, never a row DELETE.
func (r *Repository) DropPartitionsOlderThan(ctx context.Context, cutoff time.Time) ([]string, error) {
	partitioned, err := r.isPartitioned(ctx)
	if err != nil {
		return nil, err
	}
	if !partitioned {
		return nil, nil
	}
	rows, err := r.pool.Query(ctx, `
		SELECT child.relname
		  FROM pg_inherits i
		  JOIN pg_class parent ON i.inhparent = parent.oid
		  JOIN pg_class child  ON i.inhrelid  = child.oid
		 WHERE parent.relname = 'iam_login_attempts'`)
	if err != nil {
		return nil, fmt.Errorf("list_partitions: %w", err)
	}
	var names []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			rows.Close()
			return nil, err
		}
		names = append(names, n)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	var dropped []string
	for _, name := range names {
		end, ok := quarterPartitionEnd(name)
		if !ok {
			continue // not a quarterly partition (e.g. the DEFAULT) — leave it
		}
		if !end.After(cutoff) { // whole range is before the cutoff
			if _, err := r.pool.Exec(ctx, fmt.Sprintf(`DROP TABLE IF EXISTS %s`, pgQuoteIdent(name))); err != nil {
				return dropped, fmt.Errorf("drop partition %s: %w", name, err)
			}
			dropped = append(dropped, name)
		}
	}
	return dropped, nil
}

// quarterStart truncates t (in UTC) to the first instant of its calendar
// quarter.
func quarterStart(t time.Time) time.Time {
	t = t.UTC()
	m := ((int(t.Month())-1)/3)*3 + 1
	return time.Date(t.Year(), time.Month(m), 1, 0, 0, 0, 0, time.UTC)
}

// quarterPartitionName builds the `iam_login_attempts_YYYY_qN` name for the
// quarter starting at start (must already be quarter-aligned).
func quarterPartitionName(start time.Time) string {
	q := (int(start.Month())-1)/3 + 1
	return fmt.Sprintf("%s%04d_q%d", loginAttemptsPartitionPrefix, start.Year(), q)
}

// quarterPartitionEnd parses a `iam_login_attempts_YYYY_qN` partition name
// and returns its exclusive end (the start of the following quarter).
// ok=false for anything that doesn't match (e.g. the DEFAULT partition).
func quarterPartitionEnd(name string) (time.Time, bool) {
	suffix := strings.TrimPrefix(name, loginAttemptsPartitionPrefix)
	if suffix == name {
		return time.Time{}, false
	}
	parts := strings.SplitN(suffix, "_q", 2)
	if len(parts) != 2 {
		return time.Time{}, false
	}
	year, err1 := strconv.Atoi(parts[0])
	q, err2 := strconv.Atoi(parts[1])
	if err1 != nil || err2 != nil || q < 1 || q > 4 {
		return time.Time{}, false
	}
	start := time.Date(year, time.Month((q-1)*3+1), 1, 0, 0, 0, 0, time.UTC)
	return start.AddDate(0, 3, 0), true
}

// pgQuoteIdent double-quotes a Postgres identifier. The names passed through
// here are internal constants plus derived YYYY/quarter suffixes, but quote
// them defensively (same pattern as internal/stream's PartitionManager).
func pgQuoteIdent(ident string) string {
	return `"` + strings.ReplaceAll(ident, `"`, `""`) + `"`
}
