package fieldidentity

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/fieldidentity/attestation"
	"github.com/flowcatalyst/flowcatalyst-go/internal/sqlc/dbq"
	"github.com/flowcatalyst/go-sdk/usecasepgx"
)

// UnlockKeyRepo stores field_unlock_keys and field_unlock_challenges through
// sqlc. Implements usecasepgx.Persist[UnlockKey].
type UnlockKeyRepo struct {
	pool *pgxpool.Pool
	q    *dbq.Queries
}

func newUnlockKeyRepo(pool *pgxpool.Pool) *UnlockKeyRepo {
	return &UnlockKeyRepo{pool: pool, q: dbq.New(pool)}
}

func unlockKeyFromRow(r dbq.FieldUnlockKey) (*UnlockKey, error) {
	k := &UnlockKey{
		ID: r.ID, ClientID: r.ClientID, IdentityID: r.IdentityID, DeviceID: r.DeviceID, JKT: r.Jkt,
		PublicKeyJWK: r.PublicKeyJwk, AttestationChain: deref(r.AttestationChain),
		AttestationVerified: r.AttestationVerified, PINVerifiedAt: r.PinVerifiedAt, CreatedAt: r.CreatedAt,
		LastUsedAt: r.LastUsedAt, RevokedAt: r.RevokedAt, RevokedReason: UnlockKeyRevokeReason(deref(r.RevokedReason)),
		RevokedBy: deref(r.RevokedBy),
	}
	k.Attestation = attestation.None()
	if len(r.Attestation) > 0 && string(r.Attestation) != "{}" {
		if err := json.Unmarshal(r.Attestation, &k.Attestation); err != nil {
			return nil, fmt.Errorf("field_unlock_key %s attestation: %w", r.ID, err)
		}
	}
	return k, nil
}

func oneUnlockKey(r dbq.FieldUnlockKey, err error) (*UnlockKey, error) {
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("field_unlock_key: %w", err)
	}
	return unlockKeyFromRow(r)
}

func manyUnlockKeys(rows []dbq.FieldUnlockKey, err error) ([]UnlockKey, error) {
	if err != nil {
		return nil, fmt.Errorf("field_unlock_key list: %w", err)
	}
	out := make([]UnlockKey, 0, len(rows))
	for _, r := range rows {
		k, err := unlockKeyFromRow(r)
		if err != nil {
			return nil, err
		}
		out = append(out, *k)
	}
	return out, nil
}

// FindByID returns the key (any status), nil if absent.
func (r *UnlockKeyRepo) FindByID(ctx context.Context, id string) (*UnlockKey, error) {
	return oneUnlockKey(r.q.FieldUnlockKeyFindByID(ctx, id))
}

// LiveByDevice returns the device's live key, nil if none.
func (r *UnlockKeyRepo) LiveByDevice(ctx context.Context, deviceID string) (*UnlockKey, error) {
	return oneUnlockKey(r.q.FieldUnlockKeyLiveByDevice(ctx, deviceID))
}

// LiveByDevices returns the live key of each device that has one.
func (r *UnlockKeyRepo) LiveByDevices(ctx context.Context, deviceIDs []string) (map[string]*UnlockKey, error) {
	out := make(map[string]*UnlockKey, len(deviceIDs))
	if len(deviceIDs) == 0 {
		return out, nil
	}
	keys, err := manyUnlockKeys(r.q.FieldUnlockKeyLiveByDevices(ctx, deviceIDs))
	if err != nil {
		return nil, err
	}
	for n := range keys {
		out[keys[n].DeviceID] = &keys[n]
	}
	return out, nil
}

// ListByIdentity returns every key of a worker (live and revoked), newest first.
func (r *UnlockKeyRepo) ListByIdentity(ctx context.Context, identityID string) ([]UnlockKey, error) {
	return manyUnlockKeys(r.q.FieldUnlockKeyListByIdentity(ctx, identityID))
}

// ExistsByJKT reports whether a key with the thumbprint was ever registered.
func (r *UnlockKeyRepo) ExistsByJKT(ctx context.Context, jkt string) (bool, error) {
	return r.q.FieldUnlockKeyExistsByJKT(ctx, jkt)
}

// errUnlockKeyTaken is Persist's answer when the live-per-device index refused
// the insert: another registration on the device won the race.
// errUnlockKeyRegistered is its answer when the key's thumbprint is taken.
var (
	errUnlockKeyTaken      = errors.New("the device already holds a live unlock key")
	errUnlockKeyRegistered = errors.New("the unlock key is already registered")
)

func uniqueViolation(err error, constraint string) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == constraint
}

// Persist implements usecasepgx.Persist[UnlockKey]: a new key revokes the key
// it replaces and is inserted; a loaded key has its revocation written.
func (r *UnlockKeyRepo) Persist(ctx context.Context, k *UnlockKey, tx *usecasepgx.DbTx) error {
	q := r.q.WithTx(tx.Inner())
	if !k.fresh {
		// Revocations are written by RevokeOne before the event, so that two
		// concurrent revokes emit one event; nothing else of a key changes.
		return nil
	}
	if k.replaces != "" {
		at := k.CreatedAt
		if _, err := q.FieldUnlockKeyRevokeByID(ctx, dbq.FieldUnlockKeyRevokeByIDParams{
			At: &at, Reason: new(string(UnlockRevokedReplaced)), RevokedBy: emptyToNil(k.IdentityID), ID: k.replaces,
		}); err != nil {
			return err
		}
	}
	summary, err := json.Marshal(k.Attestation)
	if err != nil {
		return err
	}
	// A savepoint keeps the envelope's transaction usable when the
	// live-per-device index refuses the insert.
	sp, err := tx.Inner().Begin(ctx)
	if err != nil {
		return err
	}
	err = r.q.WithTx(sp).FieldUnlockKeyInsert(ctx, dbq.FieldUnlockKeyInsertParams{
		ID: k.ID, ClientID: k.ClientID, IdentityID: k.IdentityID, DeviceID: k.DeviceID, Jkt: k.JKT,
		PublicKeyJwk: k.PublicKeyJWK, AttestationChain: emptyToNil(k.AttestationChain), Attestation: summary,
		AttestationVerified: k.AttestationVerified, PinVerifiedAt: k.PINVerifiedAt, CreatedAt: k.CreatedAt,
	})
	if err != nil {
		_ = sp.Rollback(ctx)
		switch {
		case uniqueViolation(err, "uq_field_unlock_keys_live_device"):
			return errUnlockKeyTaken
		case uniqueViolation(err, "uq_field_unlock_keys_jkt"):
			return errUnlockKeyRegistered
		}
		return err
	}
	if err := sp.Commit(ctx); err != nil {
		return err
	}
	k.fresh, k.replaces = false, ""
	return nil
}

// Delete is not a path: a key is revoked, and removed with its worker or
// device, or by the purger.
func (r *UnlockKeyRepo) Delete(context.Context, *UnlockKey, *usecasepgx.DbTx) error {
	return errors.New("field unlock keys are revoked, not deleted")
}

// UnlockKeyScope is which live keys a bulk revocation ends.
type UnlockKeyScope int

const (
	// ScopeWorker: every live key of a worker.
	ScopeWorker UnlockKeyScope = iota
	// ScopeWorkerOtherDevices: a worker's live keys except on one device.
	ScopeWorkerOtherDevices
	// ScopeDevice: the live key of a device.
	ScopeDevice
	// ScopeClient: every live key of a client.
	ScopeClient
	// ScopeClientUnverified: a client's live keys that did not meet the
	// attestation requirement when registered.
	ScopeClientUnverified
)

// UnlockKeyRevocation names a bulk revocation: the scope and its ids.
type UnlockKeyRevocation struct {
	Scope    UnlockKeyScope
	WorkerID string
	DeviceID string
	ClientID string
	Reason   UnlockKeyRevokeReason
	By       string
}

// RevokeLive revokes the live keys the revocation names inside tx, returning
// the ids it revoked.
func (r *UnlockKeyRepo) RevokeLive(ctx context.Context, tx pgx.Tx, rv UnlockKeyRevocation, at time.Time) ([]string, error) {
	q := r.q.WithTx(tx)
	reason, by := new(string(rv.Reason)), emptyToNil(rv.By)
	switch rv.Scope {
	case ScopeWorker:
		return q.FieldUnlockKeyRevokeByIdentity(ctx, dbq.FieldUnlockKeyRevokeByIdentityParams{
			At: &at, Reason: reason, RevokedBy: by, IdentityID: rv.WorkerID,
		})
	case ScopeWorkerOtherDevices:
		return q.FieldUnlockKeyRevokeByIdentityExceptDevice(ctx, dbq.FieldUnlockKeyRevokeByIdentityExceptDeviceParams{
			At: &at, Reason: reason, RevokedBy: by, IdentityID: rv.WorkerID, DeviceID: rv.DeviceID,
		})
	case ScopeDevice:
		return q.FieldUnlockKeyRevokeByDevice(ctx, dbq.FieldUnlockKeyRevokeByDeviceParams{
			At: &at, Reason: reason, RevokedBy: by, DeviceID: rv.DeviceID,
		})
	case ScopeClient:
		return q.FieldUnlockKeyRevokeByClient(ctx, dbq.FieldUnlockKeyRevokeByClientParams{
			At: &at, Reason: reason, RevokedBy: by, ClientID: rv.ClientID,
		})
	case ScopeClientUnverified:
		return q.FieldUnlockKeyRevokeUnverifiedByClient(ctx, dbq.FieldUnlockKeyRevokeUnverifiedByClientParams{
			At: &at, Reason: reason, RevokedBy: by, ClientID: rv.ClientID,
		})
	}
	return nil, fmt.Errorf("unknown unlock key scope %d", rv.Scope)
}

// RevokeOne revokes one live key inside tx, reporting whether this call
// revoked it (false when it was already revoked: a concurrent revoke won).
func (r *UnlockKeyRepo) RevokeOne(ctx context.Context, tx pgx.Tx, id string, reason UnlockKeyRevokeReason, by string, at time.Time) (bool, error) {
	ids, err := r.q.WithTx(tx).FieldUnlockKeyRevokeByID(ctx, dbq.FieldUnlockKeyRevokeByIDParams{
		At: &at, Reason: new(string(reason)), RevokedBy: emptyToNil(by), ID: id,
	})
	return len(ids) == 1, err
}

// EndSessions ends the live login families the keys signed in
// (UNLOCK_KEY_REVOKED), returning how many sessions it ended.
func (r *UnlockKeyRepo) EndSessions(ctx context.Context, tx pgx.Tx, keyIDs []string, at time.Time) (int64, error) {
	if len(keyIDs) == 0 {
		return 0, nil
	}
	return r.q.WithTx(tx).FieldSessionEndForUnlockKeys(ctx, dbq.FieldSessionEndForUnlockKeysParams{
		At: &at, UnlockKeyIds: keyIDs,
	})
}

// EndClientSessions ends the live sessions signed in by any unlock key of the
// client, live or already revoked (unverifiedOnly: only keys registered
// without a verified attestation), returning how many it ended.
func (r *UnlockKeyRepo) EndClientSessions(ctx context.Context, tx pgx.Tx, clientID string, unverifiedOnly bool, at time.Time) (int64, error) {
	return r.q.WithTx(tx).FieldSessionEndForClientUnlockKeys(ctx, dbq.FieldSessionEndForClientUnlockKeysParams{
		At: &at, ClientID: clientID, UnverifiedOnly: unverifiedOnly,
	})
}

// LiveSessionCounts is how many unexpired live sessions each key signed in
// (keys with none are absent).
func (r *UnlockKeyRepo) LiveSessionCounts(ctx context.Context, keyIDs []string, now time.Time) (map[string]int64, error) {
	out := make(map[string]int64, len(keyIDs))
	if len(keyIDs) == 0 {
		return out, nil
	}
	rows, err := r.q.FieldUnlockKeyLiveSessionCounts(ctx, dbq.FieldUnlockKeyLiveSessionCountsParams{
		UnlockKeyIds: keyIDs, Now: now,
	})
	if err != nil {
		return nil, fmt.Errorf("field_unlock_key sessions: %w", err)
	}
	for _, row := range rows {
		out[row.UnlockKeyID] = row.Sessions
	}
	return out, nil
}

// AttestedKey is a key the attestation status list is checked against: its id
// and the certificate serials of the chain it was registered with.
type AttestedKey struct {
	ID      string
	Serials []string
}

// UnderRequiredAttestation lists the keys of clients whose policy has unlock
// keys on and REQUIRED that are live or still have a live session.
func (r *UnlockKeyRepo) UnderRequiredAttestation(ctx context.Context) ([]AttestedKey, error) {
	rows, err := r.q.FieldUnlockKeysUnderRequiredAttestation(ctx)
	if err != nil {
		return nil, fmt.Errorf("field_unlock_key attested: %w", err)
	}
	out := make([]AttestedKey, 0, len(rows))
	for _, row := range rows {
		var sum attestation.Summary
		if len(row.Attestation) > 0 {
			if err := json.Unmarshal(row.Attestation, &sum); err != nil {
				return nil, fmt.Errorf("field_unlock_key %s attestation: %w", row.ID, err)
			}
		}
		out = append(out, AttestedKey{ID: row.ID, Serials: sum.Serials})
	}
	return out, nil
}

// LockClientPolicy takes the client's policy lock for the transaction: a
// policy change and an unlock-key registration of the client run one after the
// other, so a registration always reads the policy that is in force.
func (r *UnlockKeyRepo) LockClientPolicy(ctx context.Context, tx pgx.Tx, clientID string) error {
	return r.q.WithTx(tx).FieldPolicyLock(ctx, clientID)
}

// TouchPINVerified moves the PIN proof of the worker's live key on the device
// to at (a PIN sign-in or PIN change there).
func (r *UnlockKeyRepo) TouchPINVerified(ctx context.Context, tx pgx.Tx, deviceID, identityID string, at time.Time) error {
	_, err := r.q.WithTx(tx).FieldUnlockKeyTouchPinVerified(ctx, dbq.FieldUnlockKeyTouchPinVerifiedParams{
		At: at, DeviceID: deviceID, IdentityID: identityID,
	})
	return err
}

// LockByID loads and row-locks a key inside tx; nil if absent.
func (r *UnlockKeyRepo) LockByID(ctx context.Context, tx pgx.Tx, id string) (*UnlockKey, error) {
	return oneUnlockKey(r.q.WithTx(tx).FieldUnlockKeyLockByID(ctx, id))
}

// TouchUsed stamps last_used_at on a key that just signed a worker in.
func (r *UnlockKeyRepo) TouchUsed(ctx context.Context, tx pgx.Tx, id string, at time.Time) error {
	return r.q.WithTx(tx).FieldUnlockKeyTouchUsed(ctx, dbq.FieldUnlockKeyTouchUsedParams{At: &at, ID: id})
}

// DeleteForIdentity removes a deleted worker's keys and their challenges.
func (r *UnlockKeyRepo) DeleteForIdentity(ctx context.Context, tx pgx.Tx, identityID string) error {
	q := r.q.WithTx(tx)
	if err := q.FieldUnlockChallengeDeleteByIdentity(ctx, identityID); err != nil {
		return err
	}
	return q.FieldUnlockKeyDeleteByIdentity(ctx, identityID)
}

// DeleteForDevice removes a deleted device's keys and challenges.
func (r *UnlockKeyRepo) DeleteForDevice(ctx context.Context, tx pgx.Tx, deviceID string) error {
	q := r.q.WithTx(tx)
	if err := q.FieldUnlockChallengeDeleteByDevice(ctx, deviceID); err != nil {
		return err
	}
	return q.FieldUnlockKeyDeleteByDevice(ctx, deviceID)
}

// ── challenges ───────────────────────────────────────────────────────────

func challengeHash(challenge string) []byte {
	sum := sha256.Sum256([]byte(challenge))
	return sum[:]
}

// StoreChallenge records an issued challenge for one key on one device.
func (r *UnlockKeyRepo) StoreChallenge(ctx context.Context, challenge, keyID, deviceID string, expiresAt time.Time) error {
	return r.q.FieldUnlockChallengeInsert(ctx, dbq.FieldUnlockChallengeInsertParams{
		ChallengeHash: challengeHash(challenge), UnlockKeyID: keyID, DeviceID: deviceID, ExpiresAt: expiresAt,
	})
}

// ConsumedChallenge is a challenge taken out of the store.
type ConsumedChallenge struct {
	UnlockKeyID string
	DeviceID    string
	ExpiresAt   time.Time
}

// ConsumeChallenge deletes the challenge inside tx and returns what it was
// issued for; nil when it is unknown or already used. Expiry is the caller's
// to judge (an expired challenge is consumed too).
func (r *UnlockKeyRepo) ConsumeChallenge(ctx context.Context, tx pgx.Tx, challenge string) (*ConsumedChallenge, error) {
	row, err := r.q.WithTx(tx).FieldUnlockChallengeConsume(ctx, challengeHash(challenge))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &ConsumedChallenge{UnlockKeyID: row.UnlockKeyID, DeviceID: row.DeviceID, ExpiresAt: row.ExpiresAt}, nil
}

// Purge deletes expired challenges and keys revoked before revokedBefore,
// returning how many of each.
func (r *UnlockKeyRepo) Purge(ctx context.Context, now, revokedBefore time.Time) (challenges, keys int64, err error) {
	if challenges, err = r.q.FieldUnlockChallengePurgeExpired(ctx, now); err != nil {
		return 0, 0, err
	}
	keys, err = r.q.FieldUnlockKeyPurgeRevoked(ctx, &revokedBefore)
	return challenges, keys, err
}

// DeviceLabels names the devices keys are on (id to label; a device with no
// label, or gone, is absent), for the console's list of a worker's keys.
func (r *UnlockKeyRepo) DeviceLabels(ctx context.Context, deviceIDs []string) (map[string]string, error) {
	out := make(map[string]string, len(deviceIDs))
	if len(deviceIDs) == 0 {
		return out, nil
	}
	rows, err := r.q.FieldUnlockKeyDeviceLabels(ctx, deviceIDs)
	if err != nil {
		return nil, fmt.Errorf("field_unlock_key device labels: %w", err)
	}
	for _, row := range rows {
		if l := deref(row.Label); l != "" {
			out[row.ID] = l
		}
	}
	return out, nil
}
