package fieldidentity

import (
	"context"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/go-sdk/usecase"
	"github.com/flowcatalyst/go-sdk/usecaseop"
	"github.com/flowcatalyst/go-sdk/usecasepgx"
)

// SetPolicyLocksCommand replaces the overridable members a client does not let
// its applications override.
type SetPolicyLocksCommand struct {
	ClientID      string         `json:"clientId"`
	LockedMembers []PolicyMember `json:"lockedMembers"`
}

// SetPolicyLocks replaces the client's locks: platform administrators only
// (platform:iam:field-policy:lock). An override of a member that becomes locked is kept and stops
// applying; unlocking restores it. A change of the unlockKeysEnabled lock is
// settled as any other change (section 4).
func (o *Ops) SetPolicyLocks() usecaseop.TxOperation[SetPolicyLocksCommand, Policy] {
	return usecaseop.TxOperation[SetPolicyLocksCommand, Policy]{
		Name: "SetFieldPolicyLocks",
		Validate: func(_ context.Context, cmd SetPolicyLocksCommand) error {
			if strings.TrimSpace(cmd.ClientID) == "" {
				return usecase.Validation("CLIENT_ID_REQUIRED", "clientId is required")
			}
			for _, m := range cmd.LockedMembers {
				if !m.Valid() {
					return usecase.Validation("POLICY_INVALID", string(m)+" is not a member applications can override").
						WithDetails(map[string]any{"field": string(m)})
				}
			}
			return nil
		},
		// The lock is a platform administrator's authority, not a matter of
		// reach: the dedicated permission platform:iam:field-policy:lock, which
		// only the platform administrator roles hold.
		Authorize: func(ctx context.Context, cmd SetPolicyLocksCommand) error {
			return auth.CanLockFieldPolicy(auth.FromContext(ctx), cmd.ClientID)
		},
		Execute: func(ctx context.Context, s *usecasepgx.TxScopedUnitOfWork, cmd SetPolicyLocksCommand, ec usecase.ExecutionContext) (Policy, error) {
			if err := requireClient(ctx, o.Clients, cmd.ClientID); err != nil {
				return Policy{}, err
			}
			if err := o.lockPolicy(ctx, s, cmd.ClientID); err != nil {
				return Policy{}, err
			}
			prev, err := o.R.Policies.Effective(ctx, cmd.ClientID)
			if err != nil {
				return Policy{}, repoErr("find_field_policy", err)
			}
			overrides, err := o.overridesInTx(ctx, s, cmd.ClientID)
			if err != nil {
				return Policy{}, err
			}
			locked := make([]PolicyMember, 0, len(cmd.LockedMembers))
			for _, m := range OverridableMembers() {
				if slices.Contains(cmd.LockedMembers, m) {
					locked = append(locked, m)
				}
			}
			next := prev
			next.LockedMembers = locked
			unlockChanged := unlockKeyState(prev, overrides) != unlockKeyState(next, overrides)
			if unlockChanged {
				if err := o.lockKeyWorkers(ctx, s, cmd.ClientID); err != nil {
					return Policy{}, err
				}
			}
			if err := s.WithTx(ctx, func(tx pgx.Tx) error {
				return o.R.Policies.SetLocks(ctx, tx, cmd.ClientID, locked, ec.PrincipalID, time.Now().UTC())
			}); err != nil {
				return Policy{}, repoErr("set_field_policy_locks", err)
			}
			names := make([]string, 0, len(locked))
			for _, m := range locked {
				names = append(names, string(m))
			}
			ev := NewEvent(ec, PolicyLocksChangedType, "field-policy", cmd.ClientID, map[string]any{
				"clientId": cmd.ClientID, "lockedMembers": names,
			})
			if _, err := usecase.Into(usecasepgx.EmitEventScoped(ctx, s, ev, cmd)); err != nil {
				return Policy{}, err
			}
			if unlockChanged {
				if err := o.settleUnlockKeys(ctx, s, ec, next, overrides, ec.PrincipalID, cmd); err != nil {
					return Policy{}, err
				}
			}
			return next, nil
		},
	}
}
