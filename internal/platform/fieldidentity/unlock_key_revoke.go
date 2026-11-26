package fieldidentity

import (
	"context"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/go-sdk/usecase"
	"github.com/flowcatalyst/go-sdk/usecaseop"
	"github.com/flowcatalyst/go-sdk/usecasepgx"
)

// UnlockKeyRef addresses one unlock key of a worker.
type UnlockKeyRef struct {
	WorkerRef
	UnlockKeyID string `json:"unlockKeyId"`
}

// RevokeUnlockKey is an administrator revoking one of a worker's unlock keys
// (reason ADMIN); the live sessions the key signed in end with it. On a key
// that is already revoked (by the phone, a replacement, or anything else) it
// changes nothing on the key and ends the sessions the key signed in that are
// still live: it is also how an administrator signs out what a revoked key
// left behind. The worker is loaded through the client box and application
// confinement; a key of another worker is 404. The worker's row is locked
// first (lock_order.go), so a refresh or another revoke of the worker runs
// before or after this one.
func (o *Ops) RevokeUnlockKey() usecaseop.TxOperation[UnlockKeyRef, *UnlockKey] {
	return usecaseop.TxOperation[UnlockKeyRef, *UnlockKey]{
		Name: "RevokeFieldUnlockKey",
		Validate: func(ctx context.Context, cmd UnlockKeyRef) error {
			if err := validateRef(ctx, cmd.WorkerRef); err != nil {
				return err
			}
			if strings.TrimSpace(cmd.UnlockKeyID) == "" {
				return usecase.Validation("UNLOCK_KEY_ID_REQUIRED", "unlockKeyId is required")
			}
			return nil
		},
		Authorize: usecaseop.Public[UnlockKeyRef],
		Execute: func(ctx context.Context, s *usecasepgx.TxScopedUnitOfWork, cmd UnlockKeyRef, ec usecase.ExecutionContext) (*UnlockKey, error) {
			ident, err := o.loadWorker(ctx, cmd.ID, cmd.ClientID)
			if err != nil {
				return nil, err
			}
			if err := o.lockWorkers(ctx, s, ident.ID); err != nil {
				return nil, err
			}
			k, err := o.lockedKey(ctx, s, cmd.UnlockKeyID)
			if err != nil {
				return nil, err
			}
			if k == nil || k.IdentityID != ident.ID {
				return nil, httperror.NotFound("FieldUnlockKey", cmd.UnlockKeyID)
			}
			if _, _, err := o.revokeOneKey(ctx, s, ec, k, UnlockRevokedAdmin, ec.PrincipalID, cmd); err != nil {
				return nil, err
			}
			return k, nil
		},
	}
}
