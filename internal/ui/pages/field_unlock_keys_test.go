package pages

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/fieldidentity"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/fieldidentity/attestation"
	"github.com/flowcatalyst/go-sdk/usecase"
)

func (f *fakeField) UnlockKeys(_ context.Context, identityID string) ([]fieldidentity.UnlockKey, error) {
	var out []fieldidentity.UnlockKey
	for _, k := range f.unlockKeys {
		if k.IdentityID == identityID {
			out = append(out, k)
		}
	}
	return out, nil
}

func (f *fakeField) LiveUnlockKeys(_ context.Context, deviceIDs []string) (map[string]*fieldidentity.UnlockKey, error) {
	out := map[string]*fieldidentity.UnlockKey{}
	for n := range f.unlockKeys {
		k := &f.unlockKeys[n]
		for _, id := range deviceIDs {
			if k.DeviceID == id && k.Live() {
				out[id] = k
			}
		}
	}
	return out, nil
}

func (f *fakeField) UnlockKeySessions(_ context.Context, keyIDs []string) (map[string]int64, error) {
	out := map[string]int64{}
	for _, id := range keyIDs {
		if n := f.unlockKeySessions[id]; n > 0 {
			out[id] = n
		}
	}
	return out, nil
}

func (f *fakeField) DeviceLabels(_ context.Context, deviceIDs []string) (map[string]string, error) {
	out := map[string]string{}
	for _, d := range f.devices {
		for _, id := range deviceIDs {
			if d.ID == id && d.Label != "" {
				out[id] = d.Label
			}
		}
	}
	return out, nil
}

func (f *fakeField) RevokeUnlockKey(_ context.Context, ref fieldidentity.UnlockKeyRef) error {
	if err := f.record("revoke-unlock-key " + ref.ID + " " + ref.UnlockKeyID); err != nil {
		return err
	}
	for n := range f.unlockKeys {
		if f.unlockKeys[n].ID == ref.UnlockKeyID {
			f.unlockKeys[n].Revoke(fieldidentity.UnlockRevokedAdmin, "prn_x", fieldDay)
		}
	}
	return nil
}

// withUnlockKeys gives Ann a live key on her phone (fdv_1, a trusted
// attestation) and an older revoked one.
func withUnlockKeys(f *fakeField) *fakeField {
	used := fieldDay.Add(3 * time.Hour)
	revokedAt := fieldDay.Add(-time.Hour)
	f.unlockKeys = []fieldidentity.UnlockKey{
		{
			ID: "fwk_live", ClientID: "clt_acme", IdentityID: "fwk_ann", DeviceID: "fdv_1",
			Attestation: attestation.Summary{
				Format: attestation.FormatAndroidKey, Verdict: attestation.VerdictTrusted,
				SecurityLevel: attestation.LevelStrongBox, AuthPerUse: true, BiometricOnly: true, SignP256: true, ChallengeMatches: true,
			},
			AttestationVerified: true, PINVerifiedAt: fieldDay, CreatedAt: fieldDay, LastUsedAt: &used,
		},
		{
			ID: "fwk_old", ClientID: "clt_acme", IdentityID: "fwk_ann", DeviceID: "fdv_1",
			Attestation: attestation.None(), PINVerifiedAt: fieldDay.Add(-48 * time.Hour), CreatedAt: fieldDay.Add(-48 * time.Hour),
			RevokedAt: &revokedAt, RevokedReason: fieldidentity.UnlockRevokedReplaced,
		},
	}
	return f
}

func TestFieldWorkerDrawerShowsUnlockKeys(t *testing.T) {
	f := withUnlockKeys(fieldFixture())
	manager := fieldCaller([]string{"clt_acme"}, permFieldWorkerView, permFieldWorkerManage)
	body := fieldBody(t, f, manager, req{target: "/ui/field-workers/fwk_ann"})
	section := between(body, "Unlock keys", "Sign-in history")
	mustContain(t, section, "Ann&#39;s phone", "Active", "Trusted · STRONGBOX · biometric for every use · meets the requirement",
		"PIN needed again by", "Not by time (the policy forces no periodic PIN)", "Revoked: replaced by a newer key", "None sent", "Revoke unlock key",
		"/ui/field-workers/fwk_ann/unlock-keys/fwk_live/revoke")
	if strings.Contains(section, "/unlock-keys/fwk_old/revoke") {
		t.Error("a revoked key offers Revoke")
	}
	mustContain(t, body, `id="`+fwUnlockKeyRevokeID+`"`)

	// A viewer sees the keys and no revoke.
	viewer := fieldCaller([]string{"clt_acme"}, permFieldWorkerView)
	body = fieldBody(t, f, viewer, req{target: "/ui/field-workers/fwk_ann"})
	mustContain(t, between(body, "Unlock keys", "Sign-in history"), "Active")
	mustNotContain(t, body, "Revoke unlock key", `id="`+fwUnlockKeyRevokeID+`"`)

	// A worker without keys says what a key is.
	body = fieldBody(t, f, manager, req{target: "/ui/field-workers/fwk_bob"})
	mustContain(t, body, "No unlock key.")
}

func TestFieldWorkerRevokeUnlockKey(t *testing.T) {
	f := withUnlockKeys(fieldFixture())
	manager := fieldCaller([]string{"clt_acme"}, permFieldWorkerView, permFieldWorkerManage)
	rec := serveField(t, f, manager, postForm("/ui/field-workers/fwk_ann/unlock-keys/fwk_live/revoke"))
	if rec.Code >= 400 {
		t.Fatalf("revoke answered %d: %s", rec.Code, rec.Body.String())
	}
	mustContain(t, strings.Join(f.calls, "\n"), "revoke-unlock-key fwk_ann fwk_live")

	// A viewer's forged post runs nothing.
	f2 := withUnlockKeys(fieldFixture())
	viewer := fieldCaller([]string{"clt_acme"}, permFieldWorkerView)
	rec = serveField(t, f2, viewer, nativePost("/ui/field-workers/fwk_ann/unlock-keys/fwk_live/revoke"))
	if rec.Code != 403 {
		t.Errorf("a viewer's revoke answered %d", rec.Code)
	}
	if len(f2.calls) != 0 {
		t.Errorf("a viewer's forged revoke ran %v", f2.calls)
	}
}

// A revoked key that still has open sessions offers End sessions to a
// manager (the API's revoke on a revoked key), also when it is older than the
// newest keys the drawer lists.
func TestFieldWorkerDrawerEndsTheSessionsOfARevokedKey(t *testing.T) {
	f := withUnlockKeys(fieldFixture())
	for n := range fwUnlockKeyRows {
		at := fieldDay.Add(time.Duration(n+1) * time.Minute)
		f.unlockKeys = append([]fieldidentity.UnlockKey{{
			ID: fmt.Sprintf("fwk_new%d", n), ClientID: "clt_acme", IdentityID: "fwk_ann", DeviceID: "fdv_1",
			Attestation: attestation.None(), PINVerifiedAt: at, CreatedAt: at, RevokedAt: &at,
			RevokedReason: fieldidentity.UnlockRevokedDevice,
		}}, f.unlockKeys...)
	}
	f.unlockKeySessions = map[string]int64{"fwk_old": 2, "fwk_live": 1}
	manager := fieldCaller([]string{"clt_acme"}, permFieldWorkerView, permFieldWorkerManage)
	body := fieldBody(t, f, manager, req{target: "/ui/field-workers/fwk_ann"})
	section := between(body, "Unlock keys", "Sign-in history")
	mustContain(t, section, "Open sessions", "End sessions", "/ui/field-workers/fwk_ann/unlock-keys/fwk_old/end-sessions")
	mustContain(t, body, `id="`+fwUnlockKeyEndSessionsID+`"`)
	if strings.Contains(section, "/unlock-keys/fwk_live/end-sessions") || strings.Contains(section, "/unlock-keys/fwk_new0/end-sessions") {
		t.Error("End sessions on a live key or on a revoked key with no session")
	}
	if strings.Contains(section, "fwk_new0/revoke") {
		t.Error("a revoked key offers Revoke")
	}

	rec := serveField(t, f, manager, postForm("/ui/field-workers/fwk_ann/unlock-keys/fwk_old/end-sessions"))
	if rec.Code >= 400 {
		t.Fatalf("end sessions answered %d: %s", rec.Code, rec.Body.String())
	}
	mustContain(t, strings.Join(f.calls, "\n"), "revoke-unlock-key fwk_ann fwk_old")

	// A viewer sees the count and no action.
	viewer := fieldCaller([]string{"clt_acme"}, permFieldWorkerView)
	body = fieldBody(t, f, viewer, req{target: "/ui/field-workers/fwk_ann"})
	mustContain(t, between(body, "Unlock keys", "Sign-in history"), "Open sessions")
	mustNotContain(t, body, "End sessions", `id="`+fwUnlockKeyEndSessionsID+`"`)
}

func TestFieldDeviceDrawerShowsTheUnlockKey(t *testing.T) {
	f := withUnlockKeys(fieldFixture())
	both := fieldCaller([]string{"clt_acme"}, permFieldDeviceView, permFieldWorkerView, permFieldWorkerManage)
	body := fieldBody(t, f, both, req{target: "/ui/field-devices/fdv_1"})
	section := between(body, "Unlock key", "Actions")
	mustContain(t, section, "<code>ANN</code>", "Active", "meets the requirement", "Revoke unlock key",
		"/ui/field-devices/fdv_1/unlock-key/revoke")
	mustContain(t, body, `id="`+fdUnlockKeyRevokeID+`"`)

	// Revoking it is the worker manage permission.
	rec := serveField(t, f, both, postForm("/ui/field-devices/fdv_1/unlock-key/revoke"))
	if rec.Code >= 400 {
		t.Fatalf("revoke answered %d: %s", rec.Code, rec.Body.String())
	}
	mustContain(t, strings.Join(f.calls, "\n"), "revoke-unlock-key fwk_ann fwk_live")

	// Device view alone: the key is shown, no revoke, and a forged post runs nothing.
	f2 := withUnlockKeys(fieldFixture())
	devViewer := fieldCaller([]string{"clt_acme"}, permFieldDeviceView, permFieldDeviceManage)
	body = fieldBody(t, f2, devViewer, req{target: "/ui/field-devices/fdv_1"})
	mustContain(t, body, "meets the requirement")
	mustNotContain(t, body, "/unlock-key/revoke", `id="`+fdUnlockKeyRevokeID+`"`)
	serveField(t, f2, devViewer, postForm("/ui/field-devices/fdv_1/unlock-key/revoke"))
	if len(f2.calls) != 0 {
		t.Errorf("a device manager without worker manage revoked: %v", f2.calls)
	}

	// Viewers of both, with no manage permission: no button, and nothing runs.
	f3 := withUnlockKeys(fieldFixture())
	viewer := fieldCaller([]string{"clt_acme"}, permFieldDeviceView, permFieldWorkerView)
	body = fieldBody(t, f3, viewer, req{target: "/ui/field-devices/fdv_1"})
	mustNotContain(t, body, "/unlock-key/revoke")
	if rec := serveField(t, f3, viewer, nativePost("/ui/field-devices/fdv_1/unlock-key/revoke")); rec.Code != 403 {
		t.Errorf("a viewer's forged device-drawer revoke answered %d", rec.Code)
	}
	if len(f3.calls) != 0 {
		t.Errorf("a viewer revoked: %v", f3.calls)
	}

	// A shared device has no unlock-key section.
	body = fieldBody(t, f, both, req{target: "/ui/field-devices/fdv_2"})
	mustNotContain(t, body, "No unlock key.")
}

func TestFieldPolicyUnlockKeySettings(t *testing.T) {
	f := fieldFixture()
	manager := fieldCaller([]string{"clt_acme"}, permFieldDeviceView, permFieldDeviceManage)
	body := fieldBody(t, f, manager, req{target: "/ui/field-policy?client=clt_acme"})
	mustContain(t, body, "Unlock keys", `name="unlockKeysEnabled"`, `name="unlockKeyAttestation"`, `value="REQUIRED"`)

	rec := serveField(t, f, manager, postForm("/ui/field-policy", "client", "clt_acme", "unlockKeysEnabled", "1", "pinReauthDays", "5"))
	mustContain(t, rec.Body.String(), "Field policy saved for Acme Corp")
	p := f.policies["clt_acme"]
	if !p.UnlockKeysEnabled || p.UnlockKeyAttestation != fieldidentity.UnlockAttestationNone || p.PINReauthDays != 5 {
		t.Errorf("saved %+v", p)
	}
	body = fieldBody(t, f, manager, req{target: "/ui/field-policy?client=clt_acme"})
	if !strings.Contains(between(body, `name="unlockKeysEnabled"`, `>`), "checked") {
		t.Error("the unlock-key switch is off after a save")
	}
	// Saving the photo settings keeps the unlock-key settings.
	if cmd := putPolicyCommandOf(p); !cmd.UnlockKeysEnabled || cmd.UnlockKeyAttestation != fieldidentity.UnlockAttestationNone {
		t.Errorf("the photo save would drop the unlock-key settings: %+v", cmd)
	}

	// A refused REQUIRED (no attestation roots) is shown on its switch.
	f.failWith = errPolicyField("unlockKeyAttestation")
	rec = serveField(t, f, manager, postForm("/ui/field-policy", "client", "clt_acme", "unlockKeysEnabled", "1", "unlockKeyAttestation", "REQUIRED"))
	mustContain(t, rec.Body.String(), "no attestation roots configured")
}

// errPolicyField is the operation's refusal of a policy field.
func errPolicyField(field string) error {
	return usecase.Validation("POLICY_INVALID", field+" REQUIRED needs trusted attestation roots").
		WithDetails(map[string]any{"field": field})
}
