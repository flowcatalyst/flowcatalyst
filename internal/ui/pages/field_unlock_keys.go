package pages

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/fieldidentity"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/fieldidentity/attestation"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/framework/go/ui/kit"
	"github.com/flowcatalyst/go-sdk/usecase"
)

// Unlock keys in the console (docs/field-unlock-keys-spec.md section 8): the
// worker drawer lists a worker's keys, the device drawer shows the device's
// live key, and a manager of the worker's client revokes one behind a
// confirmation. A key is made only on the phone; the console never adds one.

const (
	fwUnlockKeyRevokeID      = "fw-unlock-key-revoke"
	fwUnlockKeyEndSessionsID = "fw-unlock-key-end-sessions"
	fdUnlockKeyRevokeID      = "fd-unlock-key-revoke"
	// fwUnlockKeyRows is how many keys the worker drawer lists (the newest);
	// an older revoked key is listed too while it still has live sessions.
	fwUnlockKeyRows = 5
)

// fieldUnlockKeyView is one unlock key as a drawer shows it.
type fieldUnlockKeyView struct {
	ID, WorkerID, WorkerCode, DeviceID, Device string
	Live                                       bool
	Status                                     string
	StatusTag                                  kit.TagSeverity
	Created, LastUsed, PinRequiredBy, Revoked  time.Time
	// PinPeriodic is false when the policy forces no periodic PIN.
	PinPeriodic bool
	// Attestation is the parsed attestation in a phrase; AttestTag its tone.
	Attestation string
	AttestTag   kit.TagSeverity
	// Sessions is how many sessions the key signed in are still live.
	Sessions int64
	// RevokeAction is set when the viewer may revoke the key (a live one);
	// EndSessionsAction when the viewer may end the sessions a revoked key
	// left (the API's revoke route, which on a revoked key ends them).
	RevokeAction, EndSessionsAction string
}

func unlockRevokeReasonLabel(r fieldidentity.UnlockKeyRevokeReason) string {
	switch r {
	case fieldidentity.UnlockRevokedReplaced:
		return "replaced by a newer key"
	case fieldidentity.UnlockRevokedPINChanged:
		return "PIN changed on another device"
	case fieldidentity.UnlockRevokedPINReset:
		return "PIN reset"
	case fieldidentity.UnlockRevokedWorkerSuspended:
		return "worker suspended"
	case fieldidentity.UnlockRevokedWorkerLocked:
		return "worker locked"
	case fieldidentity.UnlockRevokedDeviceRevoked:
		return "device revoked"
	case fieldidentity.UnlockRevokedPolicyDisabled:
		return "unlock keys switched off"
	case fieldidentity.UnlockRevokedPolicyChanged:
		return "attestation now required"
	case fieldidentity.UnlockRevokedAdmin:
		return "by an administrator"
	case fieldidentity.UnlockRevokedDevice:
		return "from the device"
	case fieldidentity.UnlockRevokedAttestationRevoked:
		return "its attestation key is on the status list"
	}
	// A reason written by a newer version: shown as stored.
	return strings.ToLower(string(r))
}

// unlockAttestationSummary says in a phrase what the key's attestation shows.
func unlockAttestationSummary(a attestation.Summary, verified bool) (string, kit.TagSeverity) {
	var parts []string
	switch a.Verdict {
	case attestation.VerdictNone:
		return "None sent (an iPhone sends none)", kit.TagSecondary
	case attestation.VerdictMalformed:
		return "Could not be checked: " + a.Problem, kit.TagDanger
	case attestation.VerdictUntrusted:
		parts = append(parts, "Untrusted root")
	case attestation.VerdictTrusted:
		parts = append(parts, "Trusted")
	default:
		return string(a.Verdict), kit.TagSecondary
	}
	if a.SecurityLevel != "" {
		parts = append(parts, string(a.SecurityLevel))
	}
	switch {
	case a.AuthPerUse && a.BiometricOnly:
		parts = append(parts, "biometric for every use")
	case a.AuthPerUse:
		parts = append(parts, "user authentication for every use")
	default:
		parts = append(parts, "no authentication for every use")
	}
	switch {
	case a.Dates == attestation.DatesInvalid:
		parts = append(parts, "dates: "+a.DatesProblem)
	case a.Revocation == attestation.RevocationRevoked || a.Revocation == attestation.RevocationSuspended:
		parts = append(parts, "a certificate is "+strings.ToLower(string(a.Revocation))+" on the status list")
	case a.Revocation == attestation.RevocationUnavailable:
		parts = append(parts, "status list not available when registered")
	case a.Revocation == attestation.RevocationOff:
		parts = append(parts, "status list not checked")
	}
	if verified {
		return strings.Join(parts, " · ") + " · meets the requirement", kit.TagSuccess
	}
	return strings.Join(parts, " · "), kit.TagWarn
}

func unlockKeyViewOf(k *fieldidentity.UnlockKey, pol fieldidentity.Policy, device, workerCode string, sessions int64) fieldUnlockKeyView {
	v := fieldUnlockKeyView{
		ID: k.ID, WorkerID: k.IdentityID, WorkerCode: workerCode, DeviceID: k.DeviceID, Device: device,
		Live: k.Live(), Created: k.CreatedAt, Sessions: sessions,
		Status: "Active", StatusTag: kit.TagSuccess,
	}
	v.PinRequiredBy, v.PinPeriodic = k.PINDeadline(pol)
	if k.LastUsedAt != nil {
		v.LastUsed = *k.LastUsedAt
	}
	if !v.Live {
		v.Status, v.StatusTag = "Revoked: "+unlockRevokeReasonLabel(k.RevokedReason), kit.TagSecondary
		if k.RevokedAt != nil {
			v.Revoked = *k.RevokedAt
		}
	}
	v.Attestation, v.AttestTag = unlockAttestationSummary(k.Attestation, k.AttestationVerified)
	return v
}

// workerUnlockKeys is the worker drawer's keys, newest first: the newest
// fwUnlockKeyRows and any older revoked key that still has live sessions. A
// manager gets Revoke on a live key and End sessions on a revoked one with
// live sessions.
func (h *FieldWorkers) workerUnlockKeys(ctx context.Context, i *fieldidentity.Identity, base string, canManage bool) ([]fieldUnlockKeyView, error) {
	all, err := h.Store.UnlockKeys(ctx, i.ID)
	if err != nil {
		return nil, usecase.Internal("REPO", "list_field_unlock_keys failed", err)
	}
	if len(all) == 0 {
		return nil, nil
	}
	pol, err := h.Store.Policy(ctx, i.ClientID)
	if err != nil {
		return nil, usecase.Internal("REPO", "find_field_policy failed", err)
	}
	keyIDs := make([]string, len(all))
	for n := range all {
		keyIDs[n] = all[n].ID
	}
	sessions, err := h.Store.UnlockKeySessions(ctx, keyIDs)
	if err != nil {
		return nil, usecase.Internal("REPO", "count_unlock_key_sessions failed", err)
	}
	keys := make([]*fieldidentity.UnlockKey, 0, fwUnlockKeyRows)
	for n := range all {
		if n < fwUnlockKeyRows || sessions[all[n].ID] > 0 {
			keys = append(keys, &all[n])
		}
	}
	ids := make([]string, 0, len(keys))
	for _, k := range keys {
		ids = append(ids, k.DeviceID)
	}
	labels, err := h.Store.DeviceLabels(ctx, ids)
	if err != nil {
		return nil, usecase.Internal("REPO", "find_field_devices failed", err)
	}
	out := make([]fieldUnlockKeyView, 0, len(keys))
	for _, k := range keys {
		device := k.DeviceID
		if l, ok := labels[k.DeviceID]; ok {
			device = l
		}
		v := unlockKeyViewOf(k, pol, device, i.WorkerCode, sessions[k.ID])
		if canManage {
			switch {
			case v.Live:
				v.RevokeAction = base + "/unlock-keys/" + k.ID + "/revoke"
			case v.Sessions > 0:
				v.EndSessionsAction = base + "/unlock-keys/" + k.ID + "/end-sessions"
			}
		}
		out = append(out, v)
	}
	return out, nil
}

// revokeUnlockKey mirrors POST /api/field-workers/{id}/unlock-keys/{unlockKeyId}/revoke.
func (h *FieldWorkers) revokeUnlockKey(w http.ResponseWriter, r *http.Request) {
	h.runAction(w, r, flashFieldUnlockKeyRevoked, func(ctx context.Context, i *fieldidentity.Identity) (string, error) {
		keyID := chi.URLParam(r, "key")
		err := h.Ops.RevokeUnlockKey(ctx, fieldidentity.UnlockKeyRef{WorkerRef: refOfWorker(i), UnlockKeyID: keyID})
		return i.WorkerCode + "'s unlock key no longer signs in and its sessions have ended; they sign in with their PIN", err
	})
}

// endUnlockKeySessions ends the sessions a revoked key left: the API's
// POST /api/field-workers/{id}/unlock-keys/{unlockKeyId}/revoke on a revoked
// key, under its own label.
func (h *FieldWorkers) endUnlockKeySessions(w http.ResponseWriter, r *http.Request) {
	h.runAction(w, r, flashFieldUnlockKeyEnded, func(ctx context.Context, i *fieldidentity.Identity) (string, error) {
		keyID := chi.URLParam(r, "key")
		err := h.Ops.RevokeUnlockKey(ctx, fieldidentity.UnlockKeyRef{WorkerRef: refOfWorker(i), UnlockKeyID: keyID})
		return "the sessions " + i.WorkerCode + "'s revoked unlock key signed in have ended; they sign in with their PIN", err
	})
}

// deviceUnlockKey is the device drawer's live key, with the revoke action for
// a manager of the worker's client (revoking is the worker's permission, as in
// the API).
func (h *FieldDevices) deviceUnlockKey(ctx context.Context, ac *auth.AuthContext, d *fieldidentity.Device, base string) (*fieldUnlockKeyView, error) {
	keys, err := h.Store.LiveUnlockKeys(ctx, []string{d.ID})
	if err != nil {
		return nil, usecase.Internal("REPO", "find_field_unlock_keys failed", err)
	}
	k := keys[d.ID]
	if k == nil {
		return nil, nil
	}
	pol, err := h.Store.Policy(ctx, d.ClientID)
	if err != nil {
		return nil, usecase.Internal("REPO", "find_field_policy failed", err)
	}
	code := k.IdentityID
	if i, err := h.Store.Worker(ctx, k.IdentityID); err == nil && i != nil {
		code = i.WorkerCode
	}
	sessions, err := h.Store.UnlockKeySessions(ctx, []string{k.ID})
	if err != nil {
		return nil, usecase.Internal("REPO", "count_unlock_key_sessions failed", err)
	}
	v := unlockKeyViewOf(k, pol, d.Label, code, sessions[k.ID])
	if auth.CanManageFieldWorkers(ac, d.ClientID) == nil {
		v.RevokeAction = base + "/unlock-key/revoke"
	}
	return &v, nil
}

// revokeUnlockKey revokes the device's live unlock key: the worker's manage
// permission for the device's client, as the API's route.
func (h *FieldDevices) revokeUnlockKey(w http.ResponseWriter, r *http.Request) {
	h.runDeviceAction(w, r, flashFieldUnlockKeyRevoked, auth.CanManageFieldWorkers, func(ctx context.Context, d *fieldidentity.Device) error {
		keys, err := h.Store.LiveUnlockKeys(ctx, []string{d.ID})
		if err != nil {
			return usecase.Internal("REPO", "find_field_unlock_keys failed", err)
		}
		k := keys[d.ID]
		if k == nil {
			return httperror.NotFound("FieldUnlockKey", d.ID)
		}
		return h.Ops.RevokeUnlockKey(ctx, fieldidentity.UnlockKeyRef{
			ID: k.IdentityID, ClientID: d.ClientID, UnlockKeyID: k.ID,
		})
	})
}
