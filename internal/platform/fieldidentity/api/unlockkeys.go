package api

import (
	"context"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/fieldidentity"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apicommon"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httpcompat"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/jsontime"
	"github.com/flowcatalyst/go-sdk/usecase"
	"github.com/flowcatalyst/go-sdk/usecaseop"
)

// ── unlock keys (docs/field-unlock-keys-spec.md section 8) ───────────────

// FieldUnlockKeyAttestation is what the platform parsed from an unlock key's
// attestation. Never the chain.
type FieldUnlockKeyAttestation struct {
	Format                   string           `json:"format" enum:"ANDROID_KEY,NONE"`
	Verdict                  string           `json:"verdict" enum:"NONE,MALFORMED,UNTRUSTED,TRUSTED" doc:"NONE: no chain was sent (iOS has none). MALFORMED: the chain could not be checked. UNTRUSTED: well formed, root not configured. TRUSTED: the root is one of FC_FIELD_ATTESTATION_ROOTS"`
	Problem                  string           `json:"problem,omitempty" doc:"Why a chain is MALFORMED"`
	SecurityLevel            string           `json:"securityLevel,omitempty" enum:"STRONGBOX,TEE,SOFTWARE" doc:"Where Android says the key lives"`
	AttestationSecurityLevel string           `json:"attestationSecurityLevel,omitempty" enum:"STRONGBOX,TEE,SOFTWARE"`
	AttestationVersion       int              `json:"attestationVersion,omitempty"`
	ChallengeMatches         bool             `json:"challengeMatches" doc:"The attestation was made for the session that registered the key"`
	AuthPerUse               bool             `json:"authPerUse" doc:"Secure hardware requires user authentication for every use of the key"`
	BiometricOnly            bool             `json:"biometricOnly" doc:"Only a strong biometric unlocks the key (no device PIN or pattern)"`
	SignP256                 bool             `json:"signP256" doc:"A P-256 signing key in secure hardware"`
	RootFingerprint          string           `json:"rootFingerprint,omitempty" doc:"Hex SHA-256 of the chain root's public key"`
	Provisioning             string           `json:"provisioning,omitempty" enum:"FACTORY,REMOTE" doc:"How the attestation key reached the phone: FACTORY (expired certificates still trusted) or REMOTE (Remote Key Provisioning, dates always checked)"`
	Dates                    string           `json:"dates,omitempty" enum:"VALID,INVALID" doc:"The chain's certificates were within their validity at registration (checked once)"`
	DatesProblem             string           `json:"datesProblem,omitempty"`
	Revocation               string           `json:"revocation,omitempty" enum:"CLEAR,REVOKED,SUSPENDED,NOT_CHECKED_OFF,NOT_CHECKED_UNAVAILABLE" doc:"What the attestation key status list said at registration"`
	CheckedAt                *httpcompat.Time `json:"checkedAt,omitempty" doc:"When the chain was checked (the registration)"`
	// MeetsRequirement is whether the attestation met the policy's REQUIRED rule
	// when the key was registered.
	MeetsRequirement bool `json:"meetsRequirement"`
}

// FieldUnlockKeyResponse is a worker's unlock key on the wire: never the
// public key, its thumbprint or the attestation chain.
type FieldUnlockKeyResponse struct {
	ID            string           `json:"id"`
	WorkerID      string           `json:"workerId"`
	DeviceID      string           `json:"deviceId"`
	Status        string           `json:"status" enum:"ACTIVE,REVOKED"`
	CreatedAt     httpcompat.Time  `json:"createdAt"`
	LastUsedAt    *httpcompat.Time `json:"lastUsedAt,omitempty"`
	PinVerifiedAt httpcompat.Time  `json:"pinVerifiedAt" doc:"The last time the worker proved their PIN on the device"`
	// PinRequiredBy is pinVerifiedAt plus the client's pinReauthDays: after it,
	// the key signs the worker in again only after a PIN sign-in on the device.
	// Absent when the policy forces no periodic PIN (pinReauthDays 0, the default).
	PinRequiredBy *httpcompat.Time `json:"pinRequiredBy,omitempty"`
	RevokedAt     *httpcompat.Time `json:"revokedAt,omitempty"`
	RevokedReason string           `json:"revokedReason,omitempty" enum:"REPLACED,PIN_CHANGED,PIN_RESET,WORKER_SUSPENDED,WORKER_LOCKED,DEVICE_REVOKED,POLICY_DISABLED,POLICY_CHANGED,ADMIN,DEVICE,ATTESTATION_REVOKED"`
	RevokedBy     string           `json:"revokedBy,omitempty"`
	// LiveSessions is how many sessions the key signed in are still live. A key
	// the phone revoked or a newer key replaced keeps its sessions; revoking it
	// again ends them.
	LiveSessions int64                     `json:"liveSessions" doc:"Sessions this key signed in that are still live; revoking the key (also a revoked one) ends them"`
	Attestation  FieldUnlockKeyAttestation `json:"attestation"`
}

func unlockKeyResponse(k *fieldidentity.UnlockKey, pol fieldidentity.Policy, liveSessions int64) FieldUnlockKeyResponse {
	a := k.Attestation
	r := FieldUnlockKeyResponse{
		ID: k.ID, WorkerID: k.IdentityID, DeviceID: k.DeviceID, Status: "ACTIVE", LiveSessions: liveSessions,
		CreatedAt: jsontime.New(k.CreatedAt), LastUsedAt: opt(k.LastUsedAt),
		PinVerifiedAt: jsontime.New(k.PINVerifiedAt),
		RevokedAt:     opt(k.RevokedAt), RevokedReason: string(k.RevokedReason), RevokedBy: k.RevokedBy,
		Attestation: FieldUnlockKeyAttestation{
			Format: string(a.Format), Verdict: string(a.Verdict), Problem: a.Problem,
			SecurityLevel: string(a.SecurityLevel), AttestationSecurityLevel: string(a.AttestationSecurityLevel),
			AttestationVersion: a.AttestationVersion, ChallengeMatches: a.ChallengeMatches, AuthPerUse: a.AuthPerUse,
			BiometricOnly: a.BiometricOnly, SignP256: a.SignP256, RootFingerprint: a.RootFingerprint,
			Provisioning: string(a.Provisioning), Dates: string(a.Dates), DatesProblem: a.DatesProblem,
			Revocation: string(a.Revocation), CheckedAt: opt(a.CheckedAt),
			MeetsRequirement: k.AttestationVerified,
		},
	}
	if d, ok := k.PINDeadline(pol); ok {
		r.PinRequiredBy = opt(&d)
	}
	if !k.Live() {
		r.Status = "REVOKED"
	}
	return r
}

// FieldUnlockKeyListResponse is the GET /api/field-workers/{id}/unlock-keys envelope.
type FieldUnlockKeyListResponse struct {
	UnlockKeys []FieldUnlockKeyResponse `json:"unlockKeys"`
}

func (s *State) listUnlockKeys(ctx context.Context, in *apicommon.IDInput) (*apicommon.Out[FieldUnlockKeyListResponse], error) {
	ident, err := s.loadWorker(ctx, in.ID, false)
	if err != nil {
		return nil, err
	}
	rows, err := s.Ops.R.UnlockKeys.ListByIdentity(ctx, ident.ID)
	if err != nil {
		return nil, usecase.Internal("REPO", "list_field_unlock_keys failed", err)
	}
	pol, err := s.Ops.R.Policies.Effective(ctx, ident.ClientID)
	if err != nil {
		return nil, usecase.Internal("REPO", "find_field_policy failed", err)
	}
	ids := make([]string, len(rows))
	for n := range rows {
		ids[n] = rows[n].ID
	}
	live, err := s.Ops.R.UnlockKeys.LiveSessionCounts(ctx, ids, time.Now().UTC())
	if err != nil {
		return nil, usecase.Internal("REPO", "count_unlock_key_sessions failed", err)
	}
	out := make([]FieldUnlockKeyResponse, 0, len(rows))
	for n := range rows {
		out = append(out, unlockKeyResponse(&rows[n], pol, live[rows[n].ID]))
	}
	return &apicommon.Out[FieldUnlockKeyListResponse]{Body: FieldUnlockKeyListResponse{UnlockKeys: out}}, nil
}

type revokeUnlockKeyInput struct {
	ID          string `path:"id"`
	UnlockKeyID string `path:"unlockKeyId"`
}

func (s *State) revokeUnlockKey(ctx context.Context, in *revokeUnlockKeyInput) (*apicommon.Out[FieldUnlockKeyResponse], error) {
	ident, err := s.loadWorker(ctx, in.ID, true)
	if err != nil {
		return nil, err
	}
	k, err := usecaseop.RunTx(ctx, s.UoW, s.Ops.RevokeUnlockKey(),
		fieldidentity.UnlockKeyRef{WorkerRef: refOf(ident), UnlockKeyID: in.UnlockKeyID}, auth.NewExecutionContext(ctx))
	if err != nil {
		return nil, fail(err)
	}
	pol, err := s.Ops.R.Policies.Effective(ctx, ident.ClientID)
	if err != nil {
		return nil, usecase.Internal("REPO", "find_field_policy failed", err)
	}
	live, err := s.Ops.R.UnlockKeys.LiveSessionCounts(ctx, []string{k.ID}, time.Now().UTC())
	if err != nil {
		return nil, usecase.Internal("REPO", "count_unlock_key_sessions failed", err)
	}
	return &apicommon.Out[FieldUnlockKeyResponse]{Body: unlockKeyResponse(k, pol, live[k.ID])}, nil
}

// deviceUnlockKeys resolves the live unlock key of each device, for the device
// responses (one query for a list).
func (s *State) deviceUnlockKeys(ctx context.Context, clientID string, deviceIDs []string) (map[string]*FieldUnlockKeyResponse, error) {
	keys, err := s.Ops.R.UnlockKeys.LiveByDevices(ctx, deviceIDs)
	if err != nil {
		return nil, usecase.Internal("REPO", "find_field_unlock_keys failed", err)
	}
	out := make(map[string]*FieldUnlockKeyResponse, len(keys))
	if len(keys) == 0 {
		return out, nil
	}
	pol, err := s.Ops.R.Policies.Effective(ctx, clientID)
	if err != nil {
		return nil, usecase.Internal("REPO", "find_field_policy failed", err)
	}
	ids := make([]string, 0, len(keys))
	for _, k := range keys {
		ids = append(ids, k.ID)
	}
	live, err := s.Ops.R.UnlockKeys.LiveSessionCounts(ctx, ids, time.Now().UTC())
	if err != nil {
		return nil, usecase.Internal("REPO", "count_unlock_key_sessions failed", err)
	}
	for id, k := range keys {
		r := unlockKeyResponse(k, pol, live[k.ID])
		out[id] = &r
	}
	return out, nil
}
