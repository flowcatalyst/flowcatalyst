package fieldidentity

import (
	"context"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/fieldidentity/attestation"
	"github.com/flowcatalyst/go-sdk/usecase"
	"github.com/flowcatalyst/go-sdk/usecaseop"
	"github.com/flowcatalyst/go-sdk/usecasepgx"
)

// SweepAttestationStatus checks a freshly fetched attestation key status list
// against every key it covers: the keys of clients whose policy has unlock
// keys on and REQUIRED that are live or still have a live session, also keys
// the phone revoked or a replacement ended. Each key whose registered chain
// the list names is revoked (ATTESTATION_REVOKED) and its live sessions end,
// each in its own transaction ([Ops.RevokeUnlockKeyForAttestation], which
// locks the key's worker first). The device protocol makes the same check at
// each challenge and unlock; this one reaches the sessions a key already
// started, which refresh without the key. It returns how many keys it acted on;
// the first error stops it (the next fetch tries again).
func (o *Ops) SweepAttestationStatus(ctx context.Context, uow *usecasepgx.UnitOfWork, l *attestation.StatusList) (int, error) {
	if l.Len() == 0 {
		return 0, nil
	}
	keys, err := o.R.UnlockKeys.UnderRequiredAttestation(ctx)
	if err != nil {
		return 0, repoErr("list_attested_unlock_keys", err)
	}
	acted := 0
	for _, k := range keys {
		serial, listed := attestation.ListedSerial(l, k.Serials)
		if !listed {
			continue
		}
		did, err := usecaseop.RunTx(ctx, uow, o.RevokeUnlockKeyForAttestation(),
			RevokeForAttestationCommand{UnlockKeyID: k.ID, Serial: serial},
			usecase.NewExecutionContext(PrincipalAttestationStatus))
		if err != nil {
			return acted, err
		}
		if did {
			acted++
		}
	}
	return acted, nil
}
