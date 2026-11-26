package fieldidentity

import (
	"context"
	"strings"

	"github.com/flowcatalyst/go-sdk/usecase"
	"github.com/flowcatalyst/go-sdk/usecaseop"
	"github.com/flowcatalyst/go-sdk/usecasepgx"
)

// RevokeOwnUnlockKeyCommand is the device revoking an unlock key it holds: the
// worker turning biometrics off, the library forgetting a worker, or a key the
// phone invalidated.
type RevokeOwnUnlockKeyCommand struct {
	DeviceID    string `json:"deviceId"`
	UnlockKeyID string `json:"unlockKeyId"`
}

// RevokeOwnUnlockKey revokes a key on the calling device (reason DEVICE). Any
// holder of the device key may: revoking only removes access. An unknown key,
// one of another device and a revoked key change nothing (no oracle) and the
// result is false.
func (o *Ops) RevokeOwnUnlockKey() usecaseop.TxOperation[RevokeOwnUnlockKeyCommand, bool] {
	return usecaseop.TxOperation[RevokeOwnUnlockKeyCommand, bool]{
		Name: "RevokeOwnFieldUnlockKey",
		Validate: func(_ context.Context, cmd RevokeOwnUnlockKeyCommand) error {
			if cmd.DeviceID == "" || strings.TrimSpace(cmd.UnlockKeyID) == "" {
				return usecase.Validation("INVALID_REQUEST", "unlockKeyId is required")
			}
			return nil
		},
		// The protocol layer authenticated the device (proof + enrolled key).
		Authorize: usecaseop.Public[RevokeOwnUnlockKeyCommand],
		Execute: func(ctx context.Context, s *usecasepgx.TxScopedUnitOfWork, cmd RevokeOwnUnlockKeyCommand, ec usecase.ExecutionContext) (bool, error) {
			id := strings.TrimSpace(cmd.UnlockKeyID)
			probe, err := o.R.UnlockKeys.FindByID(ctx, id)
			if err != nil {
				return false, repoErr("find_field_unlock_key", err)
			}
			if probe == nil || probe.DeviceID != cmd.DeviceID {
				return false, nil
			}
			// The worker first (lock_order.go), then the key as it is now.
			if err := o.lockWorkers(ctx, s, probe.IdentityID); err != nil {
				return false, err
			}
			k, err := o.lockedKey(ctx, s, id)
			if err != nil {
				return false, err
			}
			if k == nil || k.DeviceID != cmd.DeviceID || !k.Live() {
				return false, nil
			}
			// The session the worker is in is not ended: they are switching the
			// feature off, not signing out (docs/field-unlock-keys-spec.md 4.7).
			won, _, err := o.revokeOneKey(ctx, s, ec, k, UnlockRevokedDevice, k.IdentityID, cmd)
			return won, err
		},
	}
}
