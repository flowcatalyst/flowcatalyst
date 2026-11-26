package fieldidentity

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/flowcatalyst/go-sdk/usecase"
	"github.com/flowcatalyst/go-sdk/usecasepgx"
)

// unlockKeysRevokedEvent is field-unlock-key:revoked: one per revoking action,
// naming the key of an explicit revoke and carrying the count of a bulk one.
func unlockKeysRevokedEvent(ec usecase.ExecutionContext, rv UnlockKeyRevocation, count int64, keyID string) Event {
	data := map[string]any{"clientId": rv.ClientID, "reason": string(rv.Reason), "count": count}
	id := keyID
	if keyID != "" {
		data["unlockKeyId"] = keyID
	}
	if rv.WorkerID != "" {
		data["workerId"] = rv.WorkerID
		if id == "" {
			id = rv.WorkerID
		}
	}
	if rv.DeviceID != "" {
		data["deviceId"] = rv.DeviceID
		if id == "" {
			id = rv.DeviceID
		}
	}
	if id == "" {
		id = rv.ClientID
	}
	return NewEvent(ec, UnlockKeyRevokedType, "field-unlock-key", id, data)
}

// endKeySessions ends the live sessions the keys signed in, whether the keys
// are still live or were revoked earlier and by whom, and emits
// field-session:revoked when it ended any, in the operation's transaction.
// The caller holds the keys' workers (lock_order.go).
func (o *Ops) endKeySessions(ctx context.Context, s *usecasepgx.TxScopedUnitOfWork, ec usecase.ExecutionContext,
	rv UnlockKeyRevocation, keyIDs []string, at time.Time, cmd any,
) (int64, error) {
	if len(keyIDs) == 0 {
		return 0, nil
	}
	var n int64
	if err := s.WithTx(ctx, func(tx pgx.Tx) error {
		var err error
		n, err = o.R.UnlockKeys.EndSessions(ctx, tx, keyIDs, at)
		return err
	}); err != nil {
		return 0, repoErr("end_unlock_sessions", err)
	}
	return n, o.noteKeySessionsEnded(ctx, s, ec, rv, n, cmd)
}

// endClientKeySessions ends the live sessions signed in by any key of the
// client the revocation's rule covers (every key for ScopeClient, those
// registered without a verified attestation for ScopeClientUnverified), live
// or already revoked.
func (o *Ops) endClientKeySessions(ctx context.Context, s *usecasepgx.TxScopedUnitOfWork, ec usecase.ExecutionContext,
	rv UnlockKeyRevocation, at time.Time, cmd any,
) error {
	var n int64
	if err := s.WithTx(ctx, func(tx pgx.Tx) error {
		var err error
		n, err = o.R.UnlockKeys.EndClientSessions(ctx, tx, rv.ClientID, rv.Scope == ScopeClientUnverified, at)
		return err
	}); err != nil {
		return repoErr("end_unlock_sessions", err)
	}
	return o.noteKeySessionsEnded(ctx, s, ec, rv, n, cmd)
}

func (o *Ops) noteKeySessionsEnded(ctx context.Context, s *usecasepgx.TxScopedUnitOfWork, ec usecase.ExecutionContext,
	rv UnlockKeyRevocation, n int64, cmd any,
) error {
	if n == 0 {
		return nil
	}
	_, err := usecase.Into(usecasepgx.EmitEventScoped(ctx, s,
		sessionsRevokedEvent(ec, rv.ClientID, ReasonUnlockKeyRevoked, int(n), rv.WorkerID, rv.DeviceID, ""), cmd))
	return err
}

// revokeUnlockKeys revokes the live keys rv names inside the operation's
// transaction and, when it revoked any, emits field-unlock-key:revoked with the
// count. When the reason ends sessions it ends those the rule covers, also of
// keys revoked before. It is how the operations that end a worker's or a
// device's credentials end their unlock keys too
// (docs/field-unlock-keys-spec.md section 7). The caller holds the workers
// (lock_order.go).
func (o *Ops) revokeUnlockKeys(ctx context.Context, s *usecasepgx.TxScopedUnitOfWork, ec usecase.ExecutionContext,
	rv UnlockKeyRevocation, cmd any,
) (int64, error) {
	var ids []string
	at := time.Now().UTC()
	if err := s.WithTx(ctx, func(tx pgx.Tx) error {
		var err error
		ids, err = o.R.UnlockKeys.RevokeLive(ctx, tx, rv, at)
		return err
	}); err != nil {
		return 0, repoErr("revoke_unlock_keys", err)
	}
	if len(ids) > 0 {
		if _, err := usecase.Into(usecasepgx.EmitEventScoped(ctx, s, unlockKeysRevokedEvent(ec, rv, int64(len(ids)), ""), cmd)); err != nil {
			return 0, err
		}
	}
	if !rv.Reason.EndsSessions() {
		return int64(len(ids)), nil
	}
	switch rv.Scope {
	case ScopeClient, ScopeClientUnverified:
		if err := o.endClientKeySessions(ctx, s, ec, rv, at, cmd); err != nil {
			return 0, err
		}
	case ScopeWorker, ScopeWorkerOtherDevices, ScopeDevice:
		if _, err := o.endKeySessions(ctx, s, ec, rv, ids, at, cmd); err != nil {
			return 0, err
		}
	}
	return int64(len(ids)), nil
}

// lockedKey loads and row-locks a key in the operation's transaction (after
// its worker, lock_order.go); nil if absent.
func (o *Ops) lockedKey(ctx context.Context, s *usecasepgx.TxScopedUnitOfWork, id string) (*UnlockKey, error) {
	var k *UnlockKey
	if err := s.WithTx(ctx, func(tx pgx.Tx) error {
		var err error
		k, err = o.R.UnlockKeys.LockByID(ctx, tx, id)
		return err
	}); err != nil {
		return nil, repoErr("lock_field_unlock_key", err)
	}
	return k, nil
}

// revokeOneKey revokes key k (read under its worker's lock) in the
// operation's transaction and emits field-unlock-key:revoked when this call
// revoked it. When the reason ends sessions it ends the key's live sessions
// whether or not the key was still live: an administrator can end the
// sessions of a key the phone revoked or a newer key replaced. It reports
// whether it revoked the key and how many sessions it ended.
func (o *Ops) revokeOneKey(ctx context.Context, s *usecasepgx.TxScopedUnitOfWork, ec usecase.ExecutionContext,
	k *UnlockKey, reason UnlockKeyRevokeReason, by string, cmd any,
) (bool, int64, error) {
	at := time.Now().UTC().Truncate(time.Microsecond)
	rv := UnlockKeyRevocation{ClientID: k.ClientID, WorkerID: k.IdentityID, DeviceID: k.DeviceID, Reason: reason}
	won := false
	if k.Live() {
		if err := s.WithTx(ctx, func(tx pgx.Tx) error {
			var err error
			won, err = o.R.UnlockKeys.RevokeOne(ctx, tx, k.ID, reason, by, at)
			return err
		}); err != nil {
			return false, 0, repoErr("revoke_unlock_key", err)
		}
	}
	if won {
		k.Revoke(reason, by, at)
		if _, err := usecase.Into(usecasepgx.EmitEventScoped(ctx, s, unlockKeysRevokedEvent(ec, rv, 1, k.ID), cmd)); err != nil {
			return false, 0, err
		}
	}
	if !reason.EndsSessions() {
		return won, 0, nil
	}
	n, err := o.endKeySessions(ctx, s, ec, rv, []string{k.ID}, at, cmd)
	if err != nil {
		return false, 0, err
	}
	return won, n, nil
}
