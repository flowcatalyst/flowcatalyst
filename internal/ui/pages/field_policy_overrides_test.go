package pages

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/fieldidentity"
	"github.com/flowcatalyst/go-sdk/usecase"
)

func (f *fakeField) PolicyOverrides(_ context.Context, clientID string) ([]fieldidentity.PolicyOverride, error) {
	var out []fieldidentity.PolicyOverride
	for _, o := range f.overrides {
		if o.ClientID == clientID {
			out = append(out, o)
		}
	}
	return out, nil
}

func (f *fakeField) PatchPolicyOverride(_ context.Context, cmd fieldidentity.PatchPolicyOverrideCommand) error {
	line := "override " + cmd.ClientID + " " + cmd.ApplicationID
	for m, v := range cmd.Set {
		line += " set " + string(m) + "=" + memberValue(v)
	}
	for _, m := range cmd.Clear {
		line += " clear " + string(m)
	}
	if cmd.ClearAll {
		line += " clear-all"
	}
	return f.record(line)
}

func (f *fakeField) ApplicationServes(_ context.Context, _, applicationID string) (bool, error) {
	return !slices.Contains(f.notServing, applicationID), nil
}

func (f *fakeField) LockingRevokesKeys(_ context.Context, _ string, locked []fieldidentity.PolicyMember) (bool, error) {
	return f.lockRevokes && slices.Contains(locked, fieldidentity.MemberUnlockKeysEnabled), nil
}

func (f *fakeField) SetPolicyLocks(_ context.Context, cmd fieldidentity.SetPolicyLocksCommand) error {
	line := "locks " + cmd.ClientID
	for _, m := range cmd.LockedMembers {
		line += " " + string(m)
	}
	return f.record(line)
}

func withOverride(f *fakeField) *fakeField {
	f.overrides = []fieldidentity.PolicyOverride{{
		ClientID: "clt_acme", ApplicationID: string(f.apps[0].ID), Values: map[fieldidentity.PolicyMember]any{
			fieldidentity.MemberUnlockKeysEnabled: true, fieldidentity.MemberPersonalIdleSecs: 3600,
		},
	}}
	return f
}

// The policy page lists each application's override, with Clear and the form
// that sets or clears one member; a client administrator sees the locks and
// cannot change them, a platform administrator can.
func TestFieldPolicyShowsApplicationOverrides(t *testing.T) {
	f := withOverride(fieldFixture())
	manager := fieldCaller([]string{"clt_acme"}, permFieldDeviceView, permFieldDeviceManage)
	body := fieldBody(t, f, manager, req{target: "/ui/field-policy?client=clt_acme"})
	section := between(body, "Application overrides", "Worker photos")
	mustContain(t, section, f.apps[0].Code, "Sign in again with a fingerprint or face: On", "Personal device idle timeout: 3600 seconds",
		"Clear override", `action="/ui/field-policy/overrides"`, "Follow the client")
	mustNotContain(t, body, `action="/ui/field-policy/locks"`)

	f.policies = map[string]fieldidentity.Policy{"clt_acme": func() fieldidentity.Policy {
		p := fieldidentity.DefaultPolicy("clt_acme")
		p.LockedMembers = []fieldidentity.PolicyMember{fieldidentity.MemberPersonalIdleSecs}
		return p
	}()}
	body = fieldBody(t, f, manager, req{target: "/ui/field-policy?client=clt_acme"})
	mustContain(t, body, "Personal device idle timeout: 3600 seconds (locked by the client, not applied)", "Locked for applications:")

	anchor := fieldAnchor()
	body = fieldBody(t, f, anchor, req{target: "/ui/field-policy?client=clt_acme"})
	mustContain(t, body, `action="/ui/field-policy/locks"`, `name="locked"`)
	if !strings.Contains(between(body, `id="field-lock-personalIdleSecs"`, ">"), "checked") {
		t.Error("the stored lock is not shown checked")
	}

	// A viewer sees the overrides and changes nothing.
	viewer := fieldCaller([]string{"clt_acme"}, permFieldDeviceView)
	body = fieldBody(t, f, viewer, req{target: "/ui/field-policy?client=clt_acme"})
	mustContain(t, body, "Sign in again with a fingerprint or face: On")
	mustNotContain(t, body, "Clear override", `action="/ui/field-policy/overrides"`)
}

func TestFieldPolicyOverrideActions(t *testing.T) {
	f := withOverride(fieldFixture())
	app := string(f.apps[0].ID)
	manager := fieldCaller([]string{"clt_acme"}, permFieldDeviceView, permFieldDeviceManage)
	rec := serveField(t, f, manager, postForm("/ui/field-policy/overrides", "client", "clt_acme", "application", app,
		"member", "unlockKeysEnabled", "setting", "off"))
	mustContain(t, rec.Body.String(), "Override saved")
	rec = serveField(t, f, manager, postForm("/ui/field-policy/overrides", "client", "clt_acme", "application", app,
		"member", "shiftMaxSecs", "setting", "seconds", "seconds", "7200"))
	mustContain(t, rec.Body.String(), "Override saved")
	serveField(t, f, manager, postForm("/ui/field-policy/overrides", "client", "clt_acme", "application", app,
		"member", "personalIdleSecs", "setting", "follow"))
	serveField(t, f, manager, postForm("/ui/field-policy/overrides/clear", "client", "clt_acme", "application", app))
	calls := strings.Join(f.calls, "\n")
	mustContain(t, calls, "override clt_acme "+app+" set unlockKeysEnabled=Off", "set shiftMaxSecs=7200 seconds",
		"clear personalIdleSecs", "clear-all")

	// A wrong pairing and a refusal are shown over the form, not as saved.
	rec = serveField(t, f, manager, postForm("/ui/field-policy/overrides", "client", "clt_acme", "application", app,
		"member", "unlockKeysEnabled", "setting", "seconds", "seconds", "60"))
	mustContain(t, rec.Body.String(), "The change was not saved: Sign in again with a fingerprint or face is on or off")
	mustNotContain(t, rec.Body.String(), "Override saved")
	f.failWith = usecase.Conflict(fieldidentity.CodePolicyMemberLocked, "the client does not let its applications override badgeOnlyLogin")
	rec = serveField(t, f, manager, postForm("/ui/field-policy/overrides", "client", "clt_acme", "application", app,
		"member", "badgeOnlyLogin", "setting", "on"))
	mustContain(t, rec.Body.String(), "The change was not saved: The client does not let its applications override badgeOnlyLogin")

	// Locks: the posted members, for a platform administrator (the use case
	// refuses anyone else).
	serveField(t, f, fieldAnchor(), postForm("/ui/field-policy/locks", "client", "clt_acme", "locked", "badgeOnlyLogin", "locked", "photosEnabled", "locked", "pinMinLength"))
	mustContain(t, strings.Join(f.calls, "\n"), "locks clt_acme badgeOnlyLogin photosEnabled")

	// A viewer's forged post runs nothing.
	f2 := withOverride(fieldFixture())
	viewer := fieldCaller([]string{"clt_acme"}, permFieldDeviceView)
	serveField(t, f2, viewer, nativePost("/ui/field-policy/overrides", "client", "clt_acme", "application", app, "member", "unlockKeysEnabled", "setting", "on"))
	if len(f2.calls) != 0 {
		t.Errorf("a viewer's forged post ran %v", f2.calls)
	}
}

// Locking can revoke every key: the page asks first, showing the form as
// submitted; only the confirming post saves. And the lock needs the platform
// administrator's permission, not anchor scope.
func TestFieldPolicyLocksAskBeforeRevokingKeys(t *testing.T) {
	f := withOverride(fieldFixture())
	f.lockRevokes = true
	admin := fieldAnchor()
	rec := serveField(t, f, admin, postForm("/ui/field-policy/locks", "client", "clt_acme", "locked", "unlockKeysEnabled"))
	b := rec.Body.String()
	mustContain(t, b, "Nothing is saved yet", "saving revokes every unlock key of the client", "Lock and revoke keys", `name="confirmRevoke"`)
	if !strings.Contains(between(b, `id="field-lock-unlockKeysEnabled"`, ">"), "checked") {
		t.Error("the form does not keep what was submitted")
	}
	mustNotContain(t, strings.Join(f.calls, "\n"), "locks clt_acme")
	serveField(t, f, admin, postForm("/ui/field-policy/locks", "client", "clt_acme", "locked", "unlockKeysEnabled", "confirmRevoke", "1"))
	mustContain(t, strings.Join(f.calls, "\n"), "locks clt_acme unlockKeysEnabled")

	// An anchor principal with field-device manage but not the lock permission
	// sees no lock form and cannot post one.
	anchorField := fieldAnchor()
	anchorField.Permissions = []string{permFieldDeviceView, permFieldDeviceManage}
	body := fieldBody(t, f, anchorField, req{target: "/ui/field-policy?client=clt_acme"})
	mustNotContain(t, body, `action="/ui/field-policy/locks"`)
	f2 := withOverride(fieldFixture())
	serveField(t, f2, anchorField, nativePost("/ui/field-policy/locks", "client", "clt_acme", "locked", "badgeOnlyLogin"))
	if len(f2.calls) != 0 {
		t.Errorf("a post without the lock permission ran %v", f2.calls)
	}
}

// An override of an application that does not serve the client is shown as
// not applied.
func TestFieldPolicyShowsAnOverrideThatDoesNotApply(t *testing.T) {
	f := withOverride(fieldFixture())
	f.notServing = []string{string(f.apps[0].ID)}
	manager := fieldCaller([]string{"clt_acme"}, permFieldDeviceView, permFieldDeviceManage)
	body := fieldBody(t, f, manager, req{target: "/ui/field-policy?client=clt_acme"})
	mustContain(t, body, "(not applied: the application is not active and enabled for this client)")
}
