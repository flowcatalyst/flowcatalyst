// Package loginattempt is the port of fc-platform/src/login_attempt.
// Tracks user-login and service-account-token outcomes for backoff /
// rate-limiting and audit. Writes are infrastructure-processing (the
// auth subdomain inserts rows directly; no UoW commit per the
// conventions in docs/conventions.md §3).
package loginattempt

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/flowcatalyst/flowcatalyst-go/internal/tsid"
)

// AttemptType identifies the kind of login.
type AttemptType string

const (
	AttemptUserLogin            AttemptType = "USER_LOGIN"
	AttemptServiceAccountToken  AttemptType = "SERVICE_ACCOUNT_TOKEN"
)

// ParseAttemptType is the lenient parser. Unknown → USER_LOGIN.
func ParseAttemptType(s string) AttemptType {
	if s == string(AttemptServiceAccountToken) {
		return AttemptServiceAccountToken
	}
	return AttemptUserLogin
}

// Outcome is success/failure.
type Outcome string

const (
	OutcomeSuccess Outcome = "SUCCESS"
	OutcomeFailure Outcome = "FAILURE"
)

// ParseOutcome is the lenient parser. Unknown → SUCCESS.
func ParseOutcome(s string) Outcome {
	if s == string(OutcomeFailure) {
		return OutcomeFailure
	}
	return OutcomeSuccess
}

// LoginAttempt is a single attempt record.
type LoginAttempt struct {
	ID             string    `json:"id"`
	AttemptType    AttemptType `json:"attemptType"`
	Outcome        Outcome   `json:"outcome"`
	FailureReason  *string   `json:"failureReason,omitempty"`
	Identifier     *string   `json:"identifier,omitempty"`
	PrincipalID    *string   `json:"principalId,omitempty"`
	IPAddress      *string   `json:"ipAddress,omitempty"`
	UserAgent      *string   `json:"userAgent,omitempty"`
	AttemptedAt    time.Time `json:"attemptedAt"`
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
	defer rows.Close()
	var out []LoginAttempt
	for rows.Next() {
		var a LoginAttempt
		var attemptType, outcome string
		if err := rows.Scan(&a.ID, &attemptType, &outcome, &a.FailureReason,
			&a.Identifier, &a.PrincipalID, &a.IPAddress, &a.UserAgent, &a.AttemptedAt); err != nil {
			return nil, err
		}
		a.AttemptType = ParseAttemptType(attemptType)
		a.Outcome = ParseOutcome(outcome)
		out = append(out, a)
	}
	return out, rows.Err()
}
