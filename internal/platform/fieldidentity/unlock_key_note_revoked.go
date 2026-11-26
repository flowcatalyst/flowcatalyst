package fieldidentity

import (
	"context"

	"github.com/flowcatalyst/go-sdk/usecase"
	"github.com/flowcatalyst/go-sdk/usecaseop"
	"github.com/flowcatalyst/go-sdk/usecasepgx"
)

// UnlockKeysRevokedNotice reports keys the device protocol revoked in its own
// transaction (a worker reaching the hard lock), for the event.
type UnlockKeysRevokedNotice struct {
	ClientID string                `json:"clientId"`
	WorkerID string                `json:"workerId"`
	Reason   UnlockKeyRevokeReason `json:"reason"`
	Count    int64                 `json:"count"`
}

// NoteUnlockKeysRevoked emits field-unlock-key:revoked for a revocation the
// device protocol already committed (its sign-in transaction records failures
// outside the envelope, as throttle notices are).
func (o *Ops) NoteUnlockKeysRevoked() usecaseop.TxOperation[UnlockKeysRevokedNotice, struct{}] {
	return usecaseop.TxOperation[UnlockKeysRevokedNotice, struct{}]{
		Name:      "NoteFieldUnlockKeysRevoked",
		Authorize: usecaseop.Public[UnlockKeysRevokedNotice],
		Execute: func(ctx context.Context, s *usecasepgx.TxScopedUnitOfWork, cmd UnlockKeysRevokedNotice, ec usecase.ExecutionContext) (struct{}, error) {
			ev := unlockKeysRevokedEvent(ec, UnlockKeyRevocation{
				ClientID: cmd.ClientID, WorkerID: cmd.WorkerID, Reason: cmd.Reason,
			}, cmd.Count, "")
			_, err := usecase.Into(usecasepgx.EmitEventScoped(ctx, s, ev, cmd))
			return struct{}{}, err
		},
	}
}
