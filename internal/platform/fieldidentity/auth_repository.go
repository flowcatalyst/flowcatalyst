package fieldidentity

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// AuthTx is one device-protocol transaction (login, refresh, PIN change). The
// hot paths use it instead of the usecase envelope: a sign-in must serialise on
// the worker row (SELECT … FOR UPDATE) so concurrent wrong PINs cannot lose
// increments, and must commit its failure counters even when the attempt is
// refused. Domain-state changes that are not part of that accounting (device
// enrolment, family revocation on reuse, self-service PIN change) go through
// the envelope ([Ops]).
type AuthTx struct{ tx pgx.Tx }

// BeginAuth opens an AuthTx.
func (r *SessionRepo) BeginAuth(ctx context.Context) (*AuthTx, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("field auth begin: %w", err)
	}
	return &AuthTx{tx: tx}, nil
}

// Commit commits the transaction.
func (t *AuthTx) Commit(ctx context.Context) error { return t.tx.Commit(ctx) }

// Rollback aborts the transaction; a no-op after Commit.
func (t *AuthTx) Rollback(ctx context.Context) { _ = t.tx.Rollback(ctx) }

func (t *AuthTx) lockIdentity(ctx context.Context, where string, args ...any) (*Identity, error) {
	rows, err := t.tx.Query(ctx, identitySelect+` WHERE `+where+` FOR UPDATE OF fi`, args...)
	if err != nil {
		return nil, fmt.Errorf("field_identity lock: %w", err)
	}
	all, err := scanIdentities(rows)
	if err != nil || len(all) == 0 {
		return nil, err
	}
	if err := attachAppsQ(ctx, t.tx, all[:1]); err != nil {
		return nil, err
	}
	return &all[0], nil
}

// LockIdentityByCode loads and row-locks the worker for (client, code); nil if
// absent.
func (t *AuthTx) LockIdentityByCode(ctx context.Context, clientID, code string) (*Identity, error) {
	return t.lockIdentity(ctx, `fi.client_id = $1 AND fi.worker_code = $2`, clientID, NormalizeWorkerCode(code))
}

// LockIdentityByID loads and row-locks a worker; nil if absent.
func (t *AuthTx) LockIdentityByID(ctx context.Context, id string) (*Identity, error) {
	return t.lockIdentity(ctx, `fi.id = $1`, id)
}

// RecordFailure counts a wrong PIN against the (row-locked) worker: soft lock
// at the policy's threshold (re-armed by each further failure), hard lock at
// the hard threshold. It updates i in memory too.
func (t *AuthTx) RecordFailure(ctx context.Context, i *Identity, pol Policy, now time.Time) error {
	i.FailedSinceSuccess++
	if pol.SoftLockThreshold > 0 && i.FailedSinceSuccess >= pol.SoftLockThreshold {
		until := now.Add(time.Duration(pol.SoftLockSecs) * time.Second)
		i.LockedUntil = &until
	}
	if pol.HardLockThreshold > 0 && i.FailedSinceSuccess >= pol.HardLockThreshold {
		i.HardLocked = true
	}
	_, err := t.tx.Exec(ctx,
		`UPDATE field_identities SET failed_since_success = $2, locked_until = $3, hard_locked = $4, updated_at = $5
		 WHERE id = $1`, i.ID, i.FailedSinceSuccess, i.LockedUntil, i.HardLocked, now)
	return err
}

// RecordSuccess resets the failure counters and stamps last_login_at. A
// non-nil rehash replaces a LEGACY hash with the PEPPER_V1 one.
func (t *AuthTx) RecordSuccess(ctx context.Context, i *Identity, rehash *string, now time.Time) error {
	i.FailedSinceSuccess, i.LockedUntil = 0, nil
	i.LastLoginAt = &now
	if rehash != nil {
		i.PINHash, i.PINScheme, i.PepperVersion = rehash, "PEPPER_V1", 1
		_, err := t.tx.Exec(ctx,
			`UPDATE field_identities SET failed_since_success = 0, locked_until = NULL, last_login_at = $2,
			     pin_hash = $3, pin_scheme = 'PEPPER_V1', pepper_version = 1, updated_at = $2 WHERE id = $1`,
			i.ID, now, *rehash)
		return err
	}
	_, err := t.tx.Exec(ctx,
		`UPDATE field_identities SET failed_since_success = 0, locked_until = NULL, last_login_at = $2, updated_at = $2
		 WHERE id = $1`, i.ID, now)
	return err
}

// InsertSession stores a session row in the transaction.
func (t *AuthTx) InsertSession(ctx context.Context, s *Session) error {
	amr := s.AMR
	if amr == nil {
		amr = []string{}
	}
	_, err := t.tx.Exec(ctx,
		`INSERT INTO field_sessions
		     (id, family_id, identity_id, device_id, application_id, refresh_hash, amr, auth_time,
		      issued_at, idle_expires_at, absolute_expires_at, rotated_at, revoked_at, revoked_reason,
		      pin_change_required)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)`,
		s.ID, s.FamilyID, s.IdentityID, s.DeviceID, s.ApplicationID, s.RefreshHash, amr, s.AuthTime,
		s.IssuedAt, s.IdleExpiresAt, s.AbsoluteExpiresAt, s.RotatedAt, s.RevokedAt,
		emptyToNil(s.RevokedReason), s.PINChangeRequired)
	return err
}

func scanSession(row pgx.Row) (*Session, error) {
	var s Session
	var reason *string
	err := row.Scan(&s.ID, &s.FamilyID, &s.IdentityID, &s.DeviceID, &s.ApplicationID, &s.RefreshHash, &s.AMR,
		&s.AuthTime, &s.IssuedAt, &s.IdleExpiresAt, &s.AbsoluteExpiresAt, &s.RotatedAt, &s.RevokedAt,
		&reason, &s.PINChangeRequired)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("field_session scan: %w", err)
	}
	s.RevokedReason = deref(reason)
	return &s, nil
}

// LockSessionByHash loads and row-locks the session for a refresh hash; nil if
// absent.
func (t *AuthTx) LockSessionByHash(ctx context.Context, hash string) (*Session, error) {
	return scanSession(t.tx.QueryRow(ctx,
		`SELECT `+sessionColumns+` FROM field_sessions WHERE refresh_hash = $1 FOR UPDATE`, hash))
}

// LockFamilyHead loads and row-locks the live session of a family; nil if none.
func (t *AuthTx) LockFamilyHead(ctx context.Context, familyID string) (*Session, error) {
	return scanSession(t.tx.QueryRow(ctx,
		`SELECT `+sessionColumns+` FROM field_sessions
		 WHERE family_id = $1 AND revoked_at IS NULL ORDER BY issued_at DESC LIMIT 1 FOR UPDATE`, familyID))
}

// Rotate marks a live session ROTATED at the given time.
func (t *AuthTx) Rotate(ctx context.Context, id string, at time.Time) error {
	_, err := t.tx.Exec(ctx,
		`UPDATE field_sessions SET revoked_at = $2, rotated_at = $2, revoked_reason = $3
		 WHERE id = $1 AND revoked_at IS NULL`, id, at, ReasonRotated)
	return err
}

// RevokeSession marks a live session revoked for reason.
func (t *AuthTx) RevokeSession(ctx context.Context, id, reason string, at time.Time) error {
	_, err := t.tx.Exec(ctx,
		`UPDATE field_sessions SET revoked_at = $2, revoked_reason = $3 WHERE id = $1 AND revoked_at IS NULL`,
		id, at, reason)
	return err
}

// RevokeDeviceSessions revokes every live session on a device, returning the
// number revoked (the logout device-all scope).
func (t *AuthTx) RevokeDeviceSessions(ctx context.Context, deviceID, reason string, at time.Time) (int64, error) {
	tag, err := t.tx.Exec(ctx,
		`UPDATE field_sessions SET revoked_at = $2, revoked_reason = $3 WHERE device_id = $1 AND revoked_at IS NULL`,
		deviceID, at, reason)
	return tag.RowsAffected(), err
}

// RevokeFamilySessions revokes every live session of a family (logout scope
// session).
func (t *AuthTx) RevokeFamilySessions(ctx context.Context, familyID, reason string, at time.Time) (int64, error) {
	tag, err := t.tx.Exec(ctx,
		`UPDATE field_sessions SET revoked_at = $2, revoked_reason = $3 WHERE family_id = $1 AND revoked_at IS NULL`,
		familyID, at, reason)
	return tag.RowsAffected(), err
}

// ClearFamilyPINChange clears pin_change_required on a family's live session
// (after the worker has changed their PIN on it).
func (t *AuthTx) ClearFamilyPINChange(ctx context.Context, familyID string) error {
	_, err := t.tx.Exec(ctx,
		`UPDATE field_sessions SET pin_change_required = FALSE WHERE family_id = $1 AND revoked_at IS NULL`, familyID)
	return err
}

// ResetFailures clears the failure counter and soft lock after a verified PIN
// that is not a sign-in (the PIN-change endpoint).
func (t *AuthTx) ResetFailures(ctx context.Context, i *Identity, now time.Time) error {
	i.FailedSinceSuccess, i.LockedUntil = 0, nil
	_, err := t.tx.Exec(ctx,
		`UPDATE field_identities SET failed_since_success = 0, locked_until = NULL, updated_at = $2 WHERE id = $1`,
		i.ID, now)
	return err
}
