package fieldidentity

import (
	"context"
	"slices"

	"github.com/jackc/pgx/v5"

	"github.com/flowcatalyst/flowcatalyst-go/internal/sqlc/dbq"
	"github.com/flowcatalyst/go-sdk/usecasepgx"
)

// The lock order of a field worker's credentials.
//
// Every transaction that changes a worker's sessions or unlock keys (an
// administrator's suspension, PIN reset, update, deletion, a device's
// revocation or deletion, a key's revocation by anyone, a policy change that
// revokes keys, the attestation status list's revocations, a PIN change, a
// sign-in, a console sign-in code's redemption, an act-as, a refresh, an
// unlock, a wrong PIN that hard-locks, a sign-out) takes its locks in this
// order:
//
//  1. The client's policy lock (pg_advisory_xact_lock on 'field-policy:' and
//     the client id), only in the operations that act on the client's policy:
//     a key's registration, a change of the client-wide policy, of an
//     application's override and of the client's locks, and an application's
//     enabling, disabling or configuration removal for the client
//     (application.ClientConfigRepo).
//  2. The worker rows (field_identities FOR UPDATE) of every worker whose
//     sessions or keys the transaction will change, in ascending id, in one
//     statement ([lockWorkers], [AuthTx.LockWorkers]), before any other row.
//     An operation that touches more than one worker learns the set first,
//     without locks, and locks it at once: a policy, override or lock change
//     that moves where unlock keys are on, every worker with a key of the
//     client, a device's revocation every worker with a key or a
//     live session on it, a suspension, update or deletion the worker and
//     those whose sessions they hold open as an actor, a sign-in code's
//     redemption the code's worker (before the code row). No transaction takes
//     a worker row after another row or after a later worker.
//
//     An act-as runs in two transactions so that no other worker's row is held
//     while a PIN is hashed: the target is resolved first, without a lock and
//     only within the supervisor's client (a worker of another client resolves
//     to nothing and is never locked); the first transaction locks the
//     supervisor alone, verifies their PIN and commits a wrong PIN's failure,
//     or rolls back; the second locks the supervisor and the target in id
//     order and checks again what the first decided on (the supervisor may
//     still act, their stored PIN is the one that verified) before the
//     target's own checks and the session.
//  3. Then the rows of those workers, in whatever order the operation needs:
//     the device row, unlock keys, unlock challenges, sessions. The worker
//     locks serialise every transaction that touches them, so these rows
//     cannot close a cycle.
//
// One narrow gap is known: a set learnt without locks can miss a worker that
// joins it in between (an act-as begun between the read and the lock); that
// worker's session is then ended without its lock. The
// purger is outside the order: it deletes only rows revoked or expired long
// ago, which no other transaction changes.
//
// A deadlock (SQLSTATE 40P01) between two of these transactions is a breach of
// the order, to be fixed here and not retried. The ordering tests are in
// internal/platform/fieldauth/lock_order_pg_test.go.

// lockWorkers row-locks the workers in ascending id inside tx (rule 2). Ids
// that name no worker lock nothing.
func lockWorkers(ctx context.Context, q *dbq.Queries, tx pgx.Tx, ids ...string) error {
	set := make([]string, 0, len(ids))
	for _, id := range ids {
		if id != "" {
			set = append(set, id)
		}
	}
	if len(set) == 0 {
		return nil
	}
	slices.Sort(set)
	_, err := q.WithTx(tx).FieldWorkersLock(ctx, slices.Compact(set))
	return err
}

// lockWorkers takes rule 2's worker locks in the operation's transaction.
func (o *Ops) lockWorkers(ctx context.Context, s *usecasepgx.TxScopedUnitOfWork, ids ...string) error {
	if err := s.WithTx(ctx, func(tx pgx.Tx) error {
		return lockWorkers(ctx, o.R.UnlockKeys.q, tx, ids...)
	}); err != nil {
		return repoErr("lock_field_workers", err)
	}
	return nil
}

// workersOnDevice is the worker set of a device's revocation or deletion: the
// workers with an unlock key on it and those with a live session on it.
func (o *Ops) workersOnDevice(ctx context.Context, s *usecasepgx.TxScopedUnitOfWork, deviceID string) ([]string, error) {
	var ids []string
	err := s.WithTx(ctx, func(tx pgx.Tx) error {
		var err error
		ids, err = o.R.UnlockKeys.q.WithTx(tx).FieldWorkersOnDevice(ctx, deviceID)
		return err
	})
	if err != nil {
		return nil, repoErr("find_device_workers", err)
	}
	return ids, nil
}

// lockWorkerAndActedAs locks a worker being suspended or deleted and the
// workers whose sessions they hold open as an actor.
func (o *Ops) lockWorkerAndActedAs(ctx context.Context, s *usecasepgx.TxScopedUnitOfWork, workerID string) error {
	ids, err := o.workerAndActedAs(ctx, s, workerID)
	if err != nil {
		return err
	}
	return o.lockWorkers(ctx, s, ids...)
}

// lockDeviceWorkers locks the workers of a device being revoked, suspended or
// deleted.
func (o *Ops) lockDeviceWorkers(ctx context.Context, s *usecasepgx.TxScopedUnitOfWork, deviceID string) error {
	ids, err := o.workersOnDevice(ctx, s, deviceID)
	if err != nil {
		return err
	}
	return o.lockWorkers(ctx, s, ids...)
}

// workerAndActedAs is the worker set of a suspension or deletion: the worker
// and the workers whose sessions they hold open as an actor.
func (o *Ops) workerAndActedAs(ctx context.Context, s *usecasepgx.TxScopedUnitOfWork, workerID string) ([]string, error) {
	ids := []string{workerID}
	err := s.WithTx(ctx, func(tx pgx.Tx) error {
		acted, err := o.R.UnlockKeys.q.WithTx(tx).FieldWorkersActedAsBy(ctx, &workerID)
		ids = append(ids, acted...)
		return err
	})
	if err != nil {
		return nil, repoErr("find_acted_as", err)
	}
	return ids, nil
}

// workersWithKeysOfClient is the worker set of a policy change that revokes
// keys: every worker with an unlock key of the client, live or not.
func (o *Ops) workersWithKeysOfClient(ctx context.Context, s *usecasepgx.TxScopedUnitOfWork, clientID string) ([]string, error) {
	var ids []string
	err := s.WithTx(ctx, func(tx pgx.Tx) error {
		var err error
		ids, err = o.R.UnlockKeys.q.WithTx(tx).FieldUnlockKeyWorkersOfClient(ctx, clientID)
		return err
	})
	if err != nil {
		return nil, repoErr("find_key_workers", err)
	}
	return ids, nil
}
