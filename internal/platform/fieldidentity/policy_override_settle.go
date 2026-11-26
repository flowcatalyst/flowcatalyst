package fieldidentity

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/flowcatalyst/go-sdk/usecase"
	"github.com/flowcatalyst/go-sdk/usecasepgx"
)

// Unlock keys under overrides (docs/field-policy-application-overrides.md
// section 4): a key is the worker's on the device and serves every application
// whose effective unlockKeysEnabled is on. When an application goes from on to
// off, only its key-signed sessions end; the keys are revoked, and every
// key-signed session ends, only when no application of the client has unlock
// keys on.

// unlockKeyState is what decides where unlock keys are on: the client's
// switch, its lock, and the applications whose overrides set it on or off.
// Two states that differ need settleUnlockKeys.
func unlockKeyState(p Policy, overrides []PolicyOverride) string {
	on, off := unlockKeyApplications(p, overrides)
	slices.Sort(on)
	slices.Sort(off)
	return fmt.Sprint(p.UnlockKeysEnabled, p.Locked(MemberUnlockKeysEnabled), on, off)
}

// settleUnlockKeys applies section 4 to the client's policy p and its
// overrides after a change, inside the operation's transaction (the caller
// holds the client's policy lock and the workers with keys of the client,
// lock_order.go).
func (o *Ops) settleUnlockKeys(ctx context.Context, s *usecasepgx.TxScopedUnitOfWork, ec usecase.ExecutionContext,
	p Policy, overrides []PolicyOverride, by string, cmd any,
) error {
	if !UnlockKeysOnSomewhere(p, overrides) {
		// As switching unlock keys off was in 0.31.0: every key revoked, and
		// every session a key of the client signed in ends.
		_, err := o.revokeUnlockKeys(ctx, s, ec, UnlockKeyRevocation{
			Scope: ScopeClient, ClientID: p.ClientID, Reason: UnlockRevokedPolicyDisabled, By: by,
		}, cmd)
		return err
	}
	on, off := unlockKeyApplications(p, overrides)
	at := time.Now().UTC()
	var n int64
	if err := s.WithTx(ctx, func(tx pgx.Tx) error {
		var err error
		if p.UnlockKeysEnabled {
			n, err = o.R.Policies.EndUnlockKeySessionsOf(ctx, tx, p.ClientID, off, at)
		} else {
			n, err = o.R.Policies.EndUnlockKeySessionsOutside(ctx, tx, p.ClientID, on, at)
		}
		return err
	}); err != nil {
		return repoErr("end_unlock_sessions", err)
	}
	return o.noteKeySessionsEnded(ctx, s, ec, UnlockKeyRevocation{ClientID: p.ClientID, Reason: UnlockRevokedPolicyDisabled}, n, cmd)
}

// lockPolicy takes the client's policy lock (lock_order.go rule 1): every
// change of the client's policy, an override or the locks, and every key
// registration of the client, run one after the other.
func (o *Ops) lockPolicy(ctx context.Context, s *usecasepgx.TxScopedUnitOfWork, clientID string) error {
	if err := s.WithTx(ctx, func(tx pgx.Tx) error {
		return o.R.UnlockKeys.LockClientPolicy(ctx, tx, clientID)
	}); err != nil {
		return repoErr("lock_field_policy", err)
	}
	return nil
}

// lockKeyWorkers locks every worker with a key of the client.
func (o *Ops) lockKeyWorkers(ctx context.Context, s *usecasepgx.TxScopedUnitOfWork, clientID string) error {
	workers, err := o.workersWithKeysOfClient(ctx, s, clientID)
	if err != nil {
		return err
	}
	return o.lockWorkers(ctx, s, workers...)
}

// overridesInTx are the client's overrides that count, read in the
// operation's transaction: those of applications active and enabled for the
// client (an override of an inactive, deleted or disabled application keeps no
// key alive).
func (o *Ops) overridesInTx(ctx context.Context, s *usecasepgx.TxScopedUnitOfWork, clientID string) ([]PolicyOverride, error) {
	var out []PolicyOverride
	if err := s.WithTx(ctx, func(tx pgx.Tx) error {
		var err error
		out, err = o.R.Policies.OverridesTx(ctx, tx, clientID)
		return err
	}); err != nil {
		return nil, repoErr("find_field_policy_overrides", err)
	}
	return o.applicableOverrides(ctx, clientID, out)
}
