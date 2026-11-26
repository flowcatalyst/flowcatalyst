package fieldidentity

import (
	"context"

	"github.com/flowcatalyst/go-sdk/usecase"
	"github.com/flowcatalyst/go-sdk/usecaseop"
	"github.com/flowcatalyst/go-sdk/usecasepgx"
)

// PrincipalAttestationStatus is the execution-context principal of a
// revocation the attestation key status list caused.
const PrincipalAttestationStatus = "field:attestation-status"

// RevokeForAttestationCommand names a key whose chain the refreshed status
// list names; Serial is the listed certificate (lowercase hex).
type RevokeForAttestationCommand struct {
	UnlockKeyID string `json:"unlockKeyId"`
	Serial      string `json:"serial"`
}

// RevokeUnlockKeyForAttestation revokes a key (ATTESTATION_REVOKED) whose
// registered chain a refreshed attestation key status list names, under the
// policy's REQUIRED: found by the check after each fetch
// ([Ops.SweepAttestationStatus]) or by the device protocol at a challenge or an
// unlock. The key's live sessions end with it, also when the key had been
// revoked already (by the phone or a replacement) and only its sessions were
// left. It reports whether it revoked the key or ended a session.
func (o *Ops) RevokeUnlockKeyForAttestation() usecaseop.TxOperation[RevokeForAttestationCommand, bool] {
	return usecaseop.TxOperation[RevokeForAttestationCommand, bool]{
		Name:      "RevokeFieldUnlockKeyForAttestation",
		Authorize: usecaseop.Public[RevokeForAttestationCommand],
		Execute: func(ctx context.Context, s *usecasepgx.TxScopedUnitOfWork, cmd RevokeForAttestationCommand, ec usecase.ExecutionContext) (bool, error) {
			probe, err := o.R.UnlockKeys.FindByID(ctx, cmd.UnlockKeyID)
			if err != nil {
				return false, repoErr("find_field_unlock_key", err)
			}
			if probe == nil {
				return false, nil
			}
			if err := o.lockWorkers(ctx, s, probe.IdentityID); err != nil {
				return false, err
			}
			k, err := o.lockedKey(ctx, s, cmd.UnlockKeyID)
			if err != nil || k == nil {
				return false, err
			}
			won, ended, err := o.revokeOneKey(ctx, s, ec, k, UnlockRevokedAttestationRevoked, PrincipalAttestationStatus, cmd)
			return won || ended > 0, err
		},
	}
}
