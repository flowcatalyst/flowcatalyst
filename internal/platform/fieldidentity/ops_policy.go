package fieldidentity

import (
	"context"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/fieldidentity/pin"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/go-sdk/usecase"
	"github.com/flowcatalyst/go-sdk/usecaseop"
	"github.com/flowcatalyst/go-sdk/usecasepgx"
)

// ── policy ───────────────────────────────────────────────────────────────

// PutPolicyCommand replaces a client's field policy. Omitted (nil) numeric
// fields take the defaults; when the minimum PIN length is below the default
// the hard-lock threshold defaults to 5.
type PutPolicyCommand struct {
	ClientID           string `json:"clientId"`
	PINMinLength       *int   `json:"pinMinLength,omitempty"`
	SoftLockThreshold  *int   `json:"softLockThreshold,omitempty"`
	SoftLockSecs       *int   `json:"softLockSecs,omitempty"`
	HardLockThreshold  *int   `json:"hardLockThreshold,omitempty"`
	SharedIdleSecs     *int   `json:"sharedIdleSecs,omitempty"`
	PersonalIdleSecs   *int   `json:"personalIdleSecs,omitempty"`
	ShiftMaxSecs       *int   `json:"shiftMaxSecs,omitempty"`
	RequireAttestation bool   `json:"requireAttestation"`
	PINReauthDays      *int   `json:"pinReauthDays,omitempty"`
	// RequireEnrolledDevice and BadgeOnlyLogin are plain flags (false when
	// omitted: a PUT replaces the policy); the two numbers take their
	// defaults (900, 14400) when omitted.
	RequireEnrolledDevice bool `json:"requireEnrolledDevice"`
	UnenrolledIdleSecs    *int `json:"unenrolledIdleSecs,omitempty"`
	BadgeOnlyLogin        bool `json:"badgeOnlyLogin"`
	ImpersonationMaxSecs  *int `json:"impersonationMaxSecs,omitempty"`
	// WorkerCodeDigits is the number of digits of a generated worker code
	// (default 3); it applies to codes generated afterwards only.
	WorkerCodeDigits *int `json:"workerCodeDigits,omitempty"`
	// PhotosEnabled nil is true (the default: a PUT replaces the policy, so a
	// value left out goes back to its default); WorkerMaySetOwnPhoto false.
	PhotosEnabled        *bool `json:"photosEnabled,omitempty"`
	WorkerMaySetOwnPhoto bool  `json:"workerMaySetOwnPhoto"`
	// UnlockKeysEnabled is false when omitted; UnlockKeyAttestation "" is NONE.
	UnlockKeysEnabled    bool                     `json:"unlockKeysEnabled"`
	UnlockKeyAttestation UnlockKeyAttestationRule `json:"unlockKeyAttestation,omitempty"`
}

func pick(v *int, def int) int {
	if v == nil {
		return def
	}
	return *v
}

// Build resolves the command against the defaults and validates ranges.
func (c PutPolicyCommand) Build() (Policy, error) {
	def := DefaultPolicy(c.ClientID)
	p := Policy{
		ClientID:           c.ClientID,
		PINMinLength:       pick(c.PINMinLength, def.PINMinLength),
		SoftLockThreshold:  pick(c.SoftLockThreshold, def.SoftLockThreshold),
		SoftLockSecs:       pick(c.SoftLockSecs, def.SoftLockSecs),
		HardLockThreshold:  pick(c.HardLockThreshold, def.HardLockThreshold),
		SharedIdleSecs:     pick(c.SharedIdleSecs, def.SharedIdleSecs),
		PersonalIdleSecs:   pick(c.PersonalIdleSecs, def.PersonalIdleSecs),
		ShiftMaxSecs:       pick(c.ShiftMaxSecs, def.ShiftMaxSecs),
		RequireAttestation: c.RequireAttestation,
		PINReauthDays:      pick(c.PINReauthDays, def.PINReauthDays),

		RequireEnrolledDevice: c.RequireEnrolledDevice,
		UnenrolledIdleSecs:    pick(c.UnenrolledIdleSecs, def.UnenrolledIdleSecs),
		BadgeOnlyLogin:        c.BadgeOnlyLogin,
		ImpersonationMaxSecs:  pick(c.ImpersonationMaxSecs, def.ImpersonationMaxSecs),
		WorkerCodeDigits:      pick(c.WorkerCodeDigits, def.WorkerCodeDigits),
		PhotosEnabled:         c.PhotosEnabled == nil || *c.PhotosEnabled,
		WorkerMaySetOwnPhoto:  c.WorkerMaySetOwnPhoto,
		UnlockKeysEnabled:     c.UnlockKeysEnabled,
		UnlockKeyAttestation:  c.UnlockKeyAttestation,
	}
	if p.UnlockKeyAttestation == "" {
		p.UnlockKeyAttestation = UnlockAttestationNone
	}
	if c.HardLockThreshold == nil && p.PINMinLength < pin.DefaultMinLength {
		p.HardLockThreshold = 5
	}
	bad := func(field, rng string) error {
		return usecase.Validation("POLICY_INVALID", field+" must be "+rng).
			WithDetails(map[string]any{"field": field})
	}
	switch {
	case !pin.ValidMinLength(p.PINMinLength):
		return p, bad("pinMinLength", "between 4 and 12")
	case p.SoftLockThreshold < 1 || p.SoftLockThreshold > 20:
		return p, bad("softLockThreshold", "between 1 and 20")
	case p.SoftLockSecs < 30 || p.SoftLockSecs > 86400:
		return p, bad("softLockSecs", "between 30 and 86400")
	case p.HardLockThreshold < p.SoftLockThreshold || p.HardLockThreshold > 50:
		return p, bad("hardLockThreshold", "at least softLockThreshold and at most 50")
	case p.PINMinLength < pin.DefaultMinLength && p.HardLockThreshold > 5:
		return p, bad("hardLockThreshold", "at most 5 when pinMinLength is below 6")
	case p.SharedIdleSecs < 60 || p.SharedIdleSecs > 86400:
		return p, bad("sharedIdleSecs", "between 60 and 86400")
	case p.PersonalIdleSecs < 60 || p.PersonalIdleSecs > 86400:
		return p, bad("personalIdleSecs", "between 60 and 86400")
	case p.ShiftMaxSecs < 3600 || p.ShiftMaxSecs > 86400:
		return p, bad("shiftMaxSecs", "between 3600 and 86400")
	case p.PINReauthDays < 0 || p.PINReauthDays > 90:
		return p, bad("pinReauthDays", "between 0 and 90 (0 forces no periodic PIN)")
	case p.UnenrolledIdleSecs < 60 || p.UnenrolledIdleSecs > 86400:
		return p, bad("unenrolledIdleSecs", "between 60 and 86400")
	case p.ImpersonationMaxSecs < 300 || p.ImpersonationMaxSecs > 43200:
		return p, bad("impersonationMaxSecs", "between 300 and 43200")
	case !ValidWorkerCodeDigits(p.WorkerCodeDigits):
		return p, bad("workerCodeDigits", "between 3 and 6")
	case !p.UnlockKeyAttestation.Valid():
		return p, bad("unlockKeyAttestation", "NONE or REQUIRED")
	}
	return p, nil
}

// PutPolicy sets the client's field policy. Policy is a client-wide setting,
// so an application-confined caller may not change it (an application changes
// its own override instead, PatchPolicyOverride). It never changes the locks. Switching photos off
// only hides them: stored photos are kept (DeleteClientPhotos deletes them).
// Switching unlock keys off revokes the client's live keys, and starting to
// require attestation revokes those that did not meet it, in the same
// transaction (docs/field-unlock-keys-spec.md section 7). REQUIRED needs
// trusted attestation roots on the platform.
func (o *Ops) PutPolicy() usecaseop.TxOperation[PutPolicyCommand, Event] {
	return usecaseop.TxOperation[PutPolicyCommand, Event]{
		Name: "PutFieldPolicy",
		Validate: func(_ context.Context, cmd PutPolicyCommand) error {
			if strings.TrimSpace(cmd.ClientID) == "" {
				return usecase.Validation("CLIENT_ID_REQUIRED", "clientId is required")
			}
			p, err := cmd.Build()
			if err != nil {
				return err
			}
			if p.UnlockKeyAttestation == UnlockAttestationRequired && !o.AttestationRoots.Configured() {
				return usecase.Validation("POLICY_INVALID",
					"unlockKeyAttestation REQUIRED needs trusted attestation roots on the platform (FC_FIELD_ATTESTATION_ROOTS)").
					WithDetails(map[string]any{"field": "unlockKeyAttestation"})
			}
			return nil
		},
		Authorize: func(ctx context.Context, cmd PutPolicyCommand) error {
			ac := auth.FromContext(ctx)
			if err := auth.CheckScopeAccess(ac, &cmd.ClientID); err != nil {
				return err
			}
			if ac.IsApplicationScoped() {
				return usecase.Authorization("APPLICATION_FORBIDDEN", "an application-confined caller cannot change the client's field policy")
			}
			return nil
		},
		Execute: func(ctx context.Context, s *usecasepgx.TxScopedUnitOfWork, cmd PutPolicyCommand, ec usecase.ExecutionContext) (Event, error) {
			if err := requireClient(ctx, o.Clients, cmd.ClientID); err != nil {
				return Event{}, err
			}
			p, err := cmd.Build()
			if err != nil {
				return Event{}, err
			}
			// The policy lock: an unlock-key registration and an override change
			// of the client run before or after this change, never across it.
			if err := o.lockPolicy(ctx, s, cmd.ClientID); err != nil {
				return Event{}, err
			}
			prev, err := o.R.Policies.Effective(ctx, cmd.ClientID)
			if err != nil {
				return Event{}, repoErr("find_field_policy", err)
			}
			overrides, err := o.overridesInTx(ctx, s, cmd.ClientID)
			if err != nil {
				return Event{}, err
			}
			// The locks are not the client-wide PUT's to change.
			p.LockedMembers = prev.LockedMembers
			unlockChanged := unlockKeyState(prev, overrides) != unlockKeyState(p, overrides)
			nowRequired := prev.UnlockKeyAttestation != UnlockAttestationRequired &&
				p.UnlockKeyAttestation == UnlockAttestationRequired && UnlockKeysOnSomewhere(p, overrides)
			if unlockChanged || nowRequired {
				// Then every worker with a key of the client (lock_order.go): a
				// refresh, unlock or revoke of any of them runs before or after
				// this change.
				if err := o.lockKeyWorkers(ctx, s, p.ClientID); err != nil {
					return Event{}, err
				}
			}
			ev := NewEvent(ec, PolicyUpdatedType, "field-policy", p.ClientID, map[string]any{
				"clientId": p.ClientID, "requireEnrolledDevice": p.RequireEnrolledDevice,
				"unenrolledIdleSecs": p.UnenrolledIdleSecs, "badgeOnlyLogin": p.BadgeOnlyLogin,
				"impersonationMaxSecs": p.ImpersonationMaxSecs, "workerCodeDigits": p.WorkerCodeDigits,
				"photosEnabled": p.PhotosEnabled, "workerMaySetOwnPhoto": p.WorkerMaySetOwnPhoto,
				"unlockKeysEnabled": p.UnlockKeysEnabled, "unlockKeyAttestation": string(p.UnlockKeyAttestation),
				"pinReauthDays": p.PINReauthDays,
			})
			out, err := usecase.Into(usecasepgx.CommitScoped(ctx, s, &p, o.R.Policies, ev, cmd))
			if err != nil {
				return Event{}, err
			}
			// Switching unlock keys off client-wide revokes the keys only when
			// no application's override keeps them on
			// (docs/field-policy-application-overrides.md section 4).
			if unlockChanged {
				if err := o.settleUnlockKeys(ctx, s, ec, p, overrides, ec.PrincipalID, cmd); err != nil {
					return Event{}, err
				}
			}
			if nowRequired {
				if _, err := o.revokeUnlockKeys(ctx, s, ec, UnlockKeyRevocation{
					Scope: ScopeClientUnverified, ClientID: p.ClientID, Reason: UnlockRevokedPolicyChanged, By: ec.PrincipalID,
				}, cmd); err != nil {
					return Event{}, err
				}
			}
			return out, nil
		},
	}
}

// validPhone accepts an empty number, or digits with spaces, '-', '.', '(' and
// ')' and at most one leading '+'. It needs 7 to 15 digits (E.164 allows 15)
// and at most 32 characters. The number is stored as entered.
func validPhone(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return true
	}
	if len(s) > 32 {
		return false
	}
	digits := 0
	for i, c := range s {
		switch {
		case c >= '0' && c <= '9':
			digits++
		case c == ' ' || c == '-' || c == '.' || c == '(' || c == ')':
		case c == '+' && i == 0:
		default:
			return false
		}
	}
	return digits >= 7 && digits <= 15
}
