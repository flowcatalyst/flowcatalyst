// Package resetapproval is the lost-device password-reset approval queue
// (Phase 8 / docs/auth-hardening-plan.md). When a user with no strong factor
// (no authenticator app, no passkey) requests a reset, a request lands here for
// a client-administrator of the user's client to approve in the dashboard.
package resetapproval

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/flowcatalyst/flowcatalyst-go/internal/tsid"
)

// Status is the lifecycle of a request.
type Status string

const (
	StatusPending  Status = "PENDING"
	StatusApproved Status = "APPROVED"
	StatusDenied   Status = "DENIED"
	StatusExpired  Status = "EXPIRED"
)

// Request is a pending lost-device reset awaiting admin approval.
type Request struct {
	ID          string     `json:"id"`
	PrincipalID string     `json:"principalId"`
	ClientID    *string    `json:"clientId,omitempty"`
	Status      Status     `json:"status"`
	Reset2FA    bool       `json:"reset2fa"`
	Note        *string    `json:"note,omitempty"`
	DecidedBy   *string    `json:"decidedBy,omitempty"`
	DecidedAt   *time.Time `json:"decidedAt,omitempty"`
	ExpiresAt   time.Time  `json:"expiresAt"`
	CreatedAt   time.Time  `json:"createdAt"`
}

// New builds a pending request (always clears 2FA on approval — lost device).
func New(principalID string, clientID *string, ttl time.Duration) *Request {
	now := time.Now().UTC()
	return &Request{
		ID:          tsid.Generate(tsid.ResetApprovalRequest),
		PrincipalID: principalID,
		ClientID:    clientID,
		Status:      StatusPending,
		Reset2FA:    true,
		ExpiresAt:   now.Add(ttl),
		CreatedAt:   now,
	}
}

// Repository persists approval requests (plain pgx).
type Repository struct{ pool *pgxpool.Pool }

// NewRepository wires a repo.
func NewRepository(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }

const cols = `id, principal_id, client_id, status, reset_2fa, note, decided_by, decided_at, expires_at, created_at`

func scan(row pgx.Row) (*Request, error) {
	var r Request
	var status string
	if err := row.Scan(&r.ID, &r.PrincipalID, &r.ClientID, &status, &r.Reset2FA,
		&r.Note, &r.DecidedBy, &r.DecidedAt, &r.ExpiresAt, &r.CreatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("scan reset-approval: %w", err)
	}
	r.Status = Status(status)
	return &r, nil
}

// Insert persists a new request.
func (r *Repository) Insert(ctx context.Context, req *Request) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO iam_reset_approval_requests
		     (id, principal_id, client_id, status, reset_2fa, note, decided_by, decided_at, expires_at, created_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`,
		req.ID, req.PrincipalID, req.ClientID, string(req.Status), req.Reset2FA,
		req.Note, req.DecidedBy, req.DecidedAt, req.ExpiresAt, req.CreatedAt)
	return err
}

// FindByID loads a request, or (nil, nil).
func (r *Repository) FindByID(ctx context.Context, id string) (*Request, error) {
	return scan(r.pool.QueryRow(ctx,
		`SELECT `+cols+` FROM iam_reset_approval_requests WHERE id = $1`, id))
}

// HasPending reports whether the principal already has an unexpired pending
// request (so the request endpoint can be idempotent + avoid spamming admins).
func (r *Repository) HasPending(ctx context.Context, principalID string) (bool, error) {
	var n int
	err := r.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM iam_reset_approval_requests
		  WHERE principal_id = $1 AND status = 'PENDING' AND expires_at > NOW()`,
		principalID).Scan(&n)
	return n > 0, err
}

// ListPending returns unexpired pending requests. When clientIDs is nil the
// caller is an anchor (all clients); otherwise it's scoped to those clients.
func (r *Repository) ListPending(ctx context.Context, clientIDs []string) ([]Request, error) {
	var rows pgx.Rows
	var err error
	if clientIDs == nil {
		rows, err = r.pool.Query(ctx,
			`SELECT `+cols+` FROM iam_reset_approval_requests
			  WHERE status = 'PENDING' AND expires_at > NOW() ORDER BY created_at`)
	} else {
		rows, err = r.pool.Query(ctx,
			`SELECT `+cols+` FROM iam_reset_approval_requests
			  WHERE status = 'PENDING' AND expires_at > NOW()
			    AND client_id = ANY($1::varchar[]) ORDER BY created_at`, clientIDs)
	}
	if err != nil {
		return nil, fmt.Errorf("list pending reset-approvals: %w", err)
	}
	defer rows.Close()
	var out []Request
	for rows.Next() {
		req, serr := scanRows(rows)
		if serr != nil {
			return nil, serr
		}
		out = append(out, *req)
	}
	return out, rows.Err()
}

func scanRows(rows pgx.Rows) (*Request, error) {
	var r Request
	var status string
	if err := rows.Scan(&r.ID, &r.PrincipalID, &r.ClientID, &status, &r.Reset2FA,
		&r.Note, &r.DecidedBy, &r.DecidedAt, &r.ExpiresAt, &r.CreatedAt); err != nil {
		return nil, fmt.Errorf("scan reset-approval row: %w", err)
	}
	r.Status = Status(status)
	return &r, nil
}

// Decide transitions a PENDING request to APPROVED/DENIED. The guarded UPDATE is
// race-free: RowsAffected==0 means it was already decided/expired.
func (r *Repository) Decide(ctx context.Context, id string, status Status, decidedBy string) (bool, error) {
	tag, err := r.pool.Exec(ctx,
		`UPDATE iam_reset_approval_requests
		    SET status = $2, decided_by = $3, decided_at = NOW()
		  WHERE id = $1 AND status = 'PENDING' AND expires_at > NOW()`,
		id, string(status), decidedBy)
	if err != nil {
		return false, fmt.Errorf("decide reset-approval: %w", err)
	}
	return tag.RowsAffected() == 1, nil
}
