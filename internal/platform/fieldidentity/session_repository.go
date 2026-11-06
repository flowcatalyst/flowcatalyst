package fieldidentity

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/flowcatalyst/flowcatalyst-go/internal/tsid"
)

// SessionRepo stores field_sessions (refresh-token rows), the single-use
// DPoP nonces, and the sign-in attempt log. The admin surface reads from it
// and revokes through the use cases; the device protocol (fieldauth) writes
// sessions, nonces and attempts through it.
type SessionRepo struct{ pool *pgxpool.Pool }

// HashToken is the stored form of a refresh token or enrolment code: the
// lower-case hex SHA-256.
func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

const sessionColumns = `id, family_id, identity_id, device_id, application_id, refresh_hash, amr,
	auth_time, issued_at, idle_expires_at, absolute_expires_at, rotated_at, revoked_at,
	revoked_reason, pin_change_required, jkt, actor_type, actor_id`

// sessionInsertSQL / sessionInsertArgs are shared by the pool and AuthTx inserts.
const sessionInsertSQL = `INSERT INTO field_sessions
	     (id, family_id, identity_id, device_id, application_id, refresh_hash, amr, auth_time,
	      issued_at, idle_expires_at, absolute_expires_at, rotated_at, revoked_at, revoked_reason,
	      pin_change_required, jkt, actor_type, actor_id)
	 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18)`

func sessionInsertArgs(s *Session) []any {
	amr := s.AMR
	if amr == nil {
		amr = []string{}
	}
	return []any{
		s.ID, s.FamilyID, s.IdentityID, emptyToNil(s.DeviceID), s.ApplicationID, s.RefreshHash, amr, s.AuthTime,
		s.IssuedAt, s.IdleExpiresAt, s.AbsoluteExpiresAt, s.RotatedAt, s.RevokedAt,
		emptyToNil(s.RevokedReason), s.PINChangeRequired, emptyToNil(s.JKT), emptyToNil(s.ActorType), emptyToNil(s.ActorID),
	}
}

// NewSessionID returns a fresh session row id.
func NewSessionID() string { return tsid.Generate(tsid.FieldSession) }

// Insert stores a session row.
func (r *SessionRepo) Insert(ctx context.Context, s *Session) error {
	_, err := r.pool.Exec(ctx, sessionInsertSQL, sessionInsertArgs(s)...)
	return err
}

// FindByRefreshHash returns the session for a stored refresh hash, nil if
// absent.
func (r *SessionRepo) FindByRefreshHash(ctx context.Context, hash string) (*Session, error) {
	return scanSession(r.pool.QueryRow(ctx, `SELECT `+sessionColumns+` FROM field_sessions WHERE refresh_hash = $1`, hash))
}

// MarkRotated revokes a session as ROTATED (a refresh consumed it) and stamps
// rotated_at, reporting whether this call won (the row was still live).
func (r *SessionRepo) MarkRotated(ctx context.Context, id string) (bool, error) {
	tag, err := r.pool.Exec(ctx,
		`UPDATE field_sessions SET revoked_at = NOW(), rotated_at = NOW(), revoked_reason = $2
		 WHERE id = $1 AND revoked_at IS NULL`, id, ReasonRotated)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() == 1, nil
}

// RevokeFamily revokes every live session of a login family, returning how
// many it revoked.
func (r *SessionRepo) RevokeFamily(ctx context.Context, familyID, reason string) (int64, error) {
	tag, err := r.pool.Exec(ctx,
		`UPDATE field_sessions SET revoked_at = NOW(), revoked_reason = $2
		 WHERE family_id = $1 AND revoked_at IS NULL`, familyID, reason)
	return tag.RowsAffected(), err
}

// RevokeDevice revokes every live session on a device.
func (r *SessionRepo) RevokeDevice(ctx context.Context, deviceID, reason string) (int64, error) {
	tag, err := r.pool.Exec(ctx,
		`UPDATE field_sessions SET revoked_at = NOW(), revoked_reason = $2
		 WHERE device_id = $1 AND revoked_at IS NULL`, deviceID, reason)
	return tag.RowsAffected(), err
}

// RevokeIdentityExceptFamily revokes the worker's live sessions other than one
// family (a PIN change keeps the session that changed it).
func (r *SessionRepo) RevokeIdentityExceptFamily(ctx context.Context, identityID, keepFamily, reason string) (int64, error) {
	tag, err := r.pool.Exec(ctx,
		`UPDATE field_sessions SET revoked_at = NOW(), revoked_reason = $3
		 WHERE identity_id = $1 AND family_id <> $2 AND revoked_at IS NULL`, identityID, keepFamily, reason)
	return tag.RowsAffected(), err
}

// IsFamilyLive reports whether a login family still has a session that is
// neither revoked nor expired at now (the introspection check on sid).
func (r *SessionRepo) IsFamilyLive(ctx context.Context, familyID string, now time.Time) (bool, error) {
	var live bool
	err := r.pool.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM field_sessions
		   WHERE family_id = $1 AND revoked_at IS NULL
		     AND idle_expires_at > $2 AND absolute_expires_at > $2)`, familyID, now).Scan(&live)
	return live, err
}

// FamilyHead returns the live (un-revoked) session of a family, nil if none.
func (r *SessionRepo) FamilyHead(ctx context.Context, familyID string) (*Session, error) {
	return scanSession(r.pool.QueryRow(ctx,
		`SELECT `+sessionColumns+` FROM field_sessions
		 WHERE family_id = $1 AND revoked_at IS NULL ORDER BY issued_at DESC LIMIT 1`, familyID))
}

// ActiveCountByDevice counts live, unexpired sessions per device id.
func (r *SessionRepo) ActiveCountByDevice(ctx context.Context, deviceIDs []string) (map[string]int, error) {
	out := make(map[string]int, len(deviceIDs))
	if len(deviceIDs) == 0 {
		return out, nil
	}
	rows, err := r.pool.Query(ctx,
		`SELECT device_id, COUNT(*) FROM field_sessions
		 WHERE device_id = ANY($1) AND revoked_at IS NULL
		   AND idle_expires_at > NOW() AND absolute_expires_at > NOW()
		 GROUP BY device_id`, deviceIDs)
	if err != nil {
		return nil, fmt.Errorf("field_session count: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		var n int
		if err := rows.Scan(&id, &n); err != nil {
			return nil, err
		}
		out[id] = n
	}
	return out, rows.Err()
}

// ActiveCountByIdentity counts a worker's live, unexpired sessions.
func (r *SessionRepo) ActiveCountByIdentity(ctx context.Context, identityID string) (int, error) {
	var n int
	err := r.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM field_sessions WHERE identity_id = $1 AND revoked_at IS NULL
		   AND idle_expires_at > NOW() AND absolute_expires_at > NOW()`, identityID).Scan(&n)
	return n, err
}

// PrincipalName returns a platform principal's display name and whether it is
// active; found is false for an unknown id. It backs the act claim of a
// session impersonated by a principal (console sign-in code).
func (r *SessionRepo) PrincipalName(ctx context.Context, id string) (name string, active, found bool, err error) {
	err = r.pool.QueryRow(ctx, `SELECT name, active FROM iam_principals WHERE id = $1`, id).Scan(&name, &active)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, false, nil
	}
	return name, active, err == nil, err
}

// ── nonces ───────────────────────────────────────────────────────────────

func nonceHash(nonce string) []byte {
	sum := sha256.Sum256([]byte(nonce))
	return sum[:]
}

// StoreNonce records a freshly issued nonce until expiresAt.
func (r *SessionRepo) StoreNonce(ctx context.Context, nonce string, expiresAt time.Time) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO field_nonces (nonce_hash, expires_at) VALUES ($1, $2) ON CONFLICT DO NOTHING`,
		nonceHash(nonce), expiresAt)
	return err
}

// ConsumeNonce atomically deletes the nonce, reporting whether it existed and
// had not expired. A nonce is valid for exactly one proof.
func (r *SessionRepo) ConsumeNonce(ctx context.Context, nonce string) (bool, error) {
	var exp time.Time
	err := r.pool.QueryRow(ctx,
		`DELETE FROM field_nonces WHERE nonce_hash = $1 RETURNING expires_at`, nonceHash(nonce)).Scan(&exp)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return exp.After(time.Now()), nil
}

// ── purge ────────────────────────────────────────────────────────────────

// Purge deletes expired nonces, used/expired enrolment codes and sign-in codes
// (the latter counted with the former) older than a day, and sessions whose absolute expiry or revocation is older than 30 days. It
// returns the rows removed per kind.
func (r *SessionRepo) Purge(ctx context.Context) (nonces, codes, sessions int64, err error) {
	tag, err := r.pool.Exec(ctx, `DELETE FROM field_nonces WHERE expires_at < NOW()`)
	if err != nil {
		return 0, 0, 0, err
	}
	nonces = tag.RowsAffected()
	tag, err = r.pool.Exec(ctx,
		`DELETE FROM field_enrolment_codes
		 WHERE (used_at IS NOT NULL AND used_at < NOW() - INTERVAL '1 day')
		    OR expires_at < NOW() - INTERVAL '1 day'`)
	if err != nil {
		return nonces, 0, 0, err
	}
	codes = tag.RowsAffected()
	tag, err = r.pool.Exec(ctx,
		`DELETE FROM field_sign_in_codes WHERE expires_at < NOW() - INTERVAL '1 day'`)
	if err != nil {
		return nonces, codes, 0, err
	}
	codes += tag.RowsAffected()
	tag, err = r.pool.Exec(ctx,
		`DELETE FROM field_sessions
		 WHERE absolute_expires_at < NOW() - INTERVAL '30 days'
		    OR revoked_at < NOW() - INTERVAL '30 days'`)
	if err != nil {
		return nonces, codes, 0, err
	}
	return nonces, codes, tag.RowsAffected(), nil
}

// loginAttemptPurgeBatch bounds each DELETE so a purge tick never holds a long
// lock or produces a huge WAL burst on a large sign-in log.
const loginAttemptPurgeBatch = 5000

// PurgeLoginAttempts deletes sign-in attempts older than retentionDays, in
// batches of loginAttemptPurgeBatch rows (the attempted_at index makes the
// scan cheap). retentionDays <= 0 keeps everything. It returns the rows
// removed; a cancelled context stops between batches.
func (r *SessionRepo) PurgeLoginAttempts(ctx context.Context, retentionDays int) (int64, error) {
	if retentionDays <= 0 {
		return 0, nil
	}
	var total int64
	for {
		tag, err := r.pool.Exec(ctx,
			`DELETE FROM field_login_attempts WHERE ctid IN (
			   SELECT ctid FROM field_login_attempts
			   WHERE attempted_at < NOW() - make_interval(days => $1)
			   LIMIT $2)`, retentionDays, loginAttemptPurgeBatch)
		if err != nil {
			return total, err
		}
		n := tag.RowsAffected()
		total += n
		if n < loginAttemptPurgeBatch {
			return total, nil
		}
		if err := ctx.Err(); err != nil {
			return total, err
		}
	}
}

// ── sign-in attempts ─────────────────────────────────────────────────────

// LoginAttempt is one field sign-in attempt, success or failure.
type LoginAttempt struct {
	ID            string
	ClientID      string
	IdentityID    string
	WorkerCode    string
	DeviceID      string
	ApplicationID string
	AuthMethod    string
	Outcome       string
	FailureReason string
	IPAddress     string
	AttemptedAt   time.Time
	// ActorID is the principal or supervisor behind an impersonated sign-in.
	ActorID string
}

// Outcomes of a sign-in attempt.
const (
	OutcomeSuccess = "SUCCESS"
	OutcomeFailure = "FAILURE"
)

// RecordLoginAttempt appends a sign-in attempt. A zero ID/AttemptedAt/AuthMethod
// is filled in.
func (r *SessionRepo) RecordLoginAttempt(ctx context.Context, a LoginAttempt) error {
	if a.ID == "" {
		a.ID = tsid.GenerateWithPrefix("fwl")
	}
	if a.AttemptedAt.IsZero() {
		a.AttemptedAt = time.Now().UTC()
	}
	if a.AuthMethod == "" {
		a.AuthMethod = MethodPIN
	}
	_, err := r.pool.Exec(ctx,
		`INSERT INTO field_login_attempts
		     (id, client_id, identity_id, worker_code, device_id, application_id, auth_method, outcome,
		      failure_reason, ip_address, attempted_at, actor_id)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`,
		a.ID, emptyToNil(a.ClientID), emptyToNil(a.IdentityID), emptyToNil(a.WorkerCode), emptyToNil(a.DeviceID),
		emptyToNil(a.ApplicationID), a.AuthMethod, a.Outcome, emptyToNil(a.FailureReason),
		emptyToNil(a.IPAddress), a.AttemptedAt, emptyToNil(a.ActorID))
	return err
}

// SignIns lists a worker's sign-in attempts, newest first, with the total.
func (r *SessionRepo) SignIns(ctx context.Context, identityID string, offset, limit int) ([]LoginAttempt, int64, error) {
	var total int64
	if err := r.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM field_login_attempts WHERE identity_id = $1`, identityID).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("field sign-ins count: %w", err)
	}
	rows, err := r.pool.Query(ctx,
		`SELECT id, client_id, identity_id, worker_code, device_id, application_id, auth_method, outcome,
		        failure_reason, ip_address, attempted_at, actor_id
		 FROM field_login_attempts WHERE identity_id = $1
		 ORDER BY attempted_at DESC, id DESC LIMIT $2 OFFSET $3`, identityID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("field sign-ins: %w", err)
	}
	defer rows.Close()
	out := []LoginAttempt{}
	for rows.Next() {
		var a LoginAttempt
		var clientID, identity, code, device, app, reason, ip, actor *string
		if err := rows.Scan(&a.ID, &clientID, &identity, &code, &device, &app, &a.AuthMethod, &a.Outcome,
			&reason, &ip, &a.AttemptedAt, &actor); err != nil {
			return nil, 0, err
		}
		a.ClientID, a.IdentityID, a.WorkerCode = deref(clientID), deref(identity), deref(code)
		a.DeviceID, a.ApplicationID, a.FailureReason, a.IPAddress = deref(device), deref(app), deref(reason), deref(ip)
		a.ActorID = deref(actor)
		out = append(out, a)
	}
	return out, total, rows.Err()
}
