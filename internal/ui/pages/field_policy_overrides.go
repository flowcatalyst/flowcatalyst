package pages

import (
	"context"
	"log/slog"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/fieldidentity"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/framework/go/ui/ds"
	"github.com/flowcatalyst/framework/go/ui/kit"
	"github.com/flowcatalyst/go-sdk/usecase"
)

// The applications' overrides of the client's field policy and the client's
// locks (docs/field-policy-application-overrides.md section 5): the policy
// page lists each application's override with Clear, sets or clears one
// member of an application's override, and, for a platform administrator,
// locks members.

const (
	fieldOverrideForm  = "field-policy-override-form"
	fieldLocksForm     = "field-policy-locks-form"
	fieldOverrideClear = "field-policy-override-clear"
)

// memberLabel is a member's name on the page.
func memberLabel(m fieldidentity.PolicyMember) string {
	switch m {
	case fieldidentity.MemberUnlockKeysEnabled:
		return "Sign in again with a fingerprint or face"
	case fieldidentity.MemberRequireEnrolledDevice:
		return "Require an enrolled device"
	case fieldidentity.MemberBadgeOnlyLogin:
		return "Badge-only sign-in"
	case fieldidentity.MemberSharedIdleSecs:
		return "Shared device idle timeout"
	case fieldidentity.MemberPersonalIdleSecs:
		return "Personal device idle timeout"
	case fieldidentity.MemberShiftMaxSecs:
		return "Maximum shift length"
	case fieldidentity.MemberUnenrolledIdleSecs:
		return "Idle timeout when the device is not enrolled"
	case fieldidentity.MemberPhotosEnabled:
		return "Photos"
	}
	return string(m)
}

// memberValue is a value as the page shows it.
func memberValue(v any) string {
	switch x := v.(type) {
	case bool:
		if x {
			return "On"
		}
		return "Off"
	case int:
		return strconv.Itoa(x) + " seconds"
	}
	return "—"
}

// policyOverrideRow is one application's override on the page.
type policyOverrideRow struct {
	AppID, AppCode, AppName string
	// Items are "member: value", in the members' order; an overruled one
	// (locked by the client) says so.
	Items []string
	// ClearDialog is the id of its Clear confirmation.
	ClearDialog string
}

// policyOverridesView is the overrides part of the page.
type policyOverridesView struct {
	Rows []policyOverrideRow
	// AppOptions are the applications the viewer may set an override for.
	AppOptions []kit.Option
	// CanOverride: the viewer may set and clear overrides; CanLock: the viewer
	// is a platform administrator and may lock members.
	CanOverride, CanLock bool
	Locked               map[fieldidentity.PolicyMember]bool
	// Error is a refused override or lock change, shown over its form.
	Error string
	// ConfirmRevoke is set when the submitted locks would revoke every unlock
	// key of the client: the form is shown again as submitted with the warning
	// and a confirming button.
	ConfirmRevoke bool
}

func (v policyOverridesView) memberOptions() []kit.Option {
	out := make([]kit.Option, 0, len(fieldidentity.OverridableMembers()))
	for _, m := range fieldidentity.OverridableMembers() {
		out = append(out, kit.Option{Value: string(m), Label: memberLabel(m)})
	}
	return out
}

func overrideSettingOptions() []kit.Option {
	return []kit.Option{
		{Value: "on", Label: "On (switches)"},
		{Value: "off", Label: "Off (switches)"},
		{Value: "seconds", Label: "A number of seconds (limits)"},
		{Value: "follow", Label: "Follow the client"},
	}
}

// readOverrides fills the overrides part of the page for the client.
func (h *FieldPolicy) readOverrides(ctx context.Context, ac *auth.AuthContext, p fieldidentity.Policy) (policyOverridesView, error) {
	v := policyOverridesView{
		CanOverride: auth.CanManageFieldPolicyOverrides(ac, p.ClientID) == nil,
		CanLock:     auth.CanLockFieldPolicy(ac, p.ClientID) == nil,
		Locked:      map[fieldidentity.PolicyMember]bool{},
	}
	for _, m := range p.LockedMembers {
		v.Locked[m] = true
	}
	apps, err := h.Store.Applications(ctx)
	if err != nil {
		return v, usecase.Internal("REPO", "list_applications failed", err)
	}
	names := map[string][2]string{}
	for _, a := range apps {
		names[string(a.ID)] = [2]string{a.Code, a.Name}
		if ac.CanAccessApplication(string(a.ID)) {
			v.AppOptions = append(v.AppOptions, kit.Option{Value: string(a.ID), Label: a.Name + " (" + a.Code + ")"})
		}
	}
	rows, err := h.Store.PolicyOverrides(ctx, p.ClientID)
	if err != nil {
		return v, usecase.Internal("REPO", "list_field_policy_overrides failed", err)
	}
	for n := range rows {
		o := &rows[n]
		if !ac.CanAccessApplication(o.ApplicationID) {
			continue
		}
		row := policyOverrideRow{
			AppID: o.ApplicationID, AppCode: names[o.ApplicationID][0], AppName: names[o.ApplicationID][1],
			ClearDialog: fieldOverrideClear + "-" + strconv.Itoa(n),
		}
		if row.AppCode == "" {
			row.AppCode, row.AppName = o.ApplicationID, o.ApplicationID
		}
		serves, err := h.Store.ApplicationServes(ctx, p.ClientID, o.ApplicationID)
		if err != nil {
			return v, usecase.Internal("REPO", "find_client_config failed", err)
		}
		for _, m := range fieldidentity.OverridableMembers() {
			if x, ok := o.Get(m); ok {
				item := memberLabel(m) + ": " + memberValue(x)
				switch {
				case !serves:
					item += " (not applied: the application is not active and enabled for this client)"
				case p.Locked(m):
					item += " (locked by the client, not applied)"
				}
				row.Items = append(row.Items, item)
			}
		}
		v.Rows = append(v.Rows, row)
	}
	return v, nil
}

// overrideDone answers a change of an override or the locks: the page body
// again (with the refusal over its form when refused) and, when saved, a toast.
func (h *FieldPolicy) overrideDone(w http.ResponseWriter, r *http.Request, ac *auth.AuthContext, clientID string, changeErr error, flash, detail string) {
	ctx := r.Context()
	v, err := h.read(ctx, ac, clientID)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	if changeErr != nil {
		var fe formErrors
		fe.apply(r, changeErr, nil)
		msg := fe.Form
		for _, m := range fe.Fields {
			msg = m
		}
		if ue := usecase.AsError(changeErr); ue != nil && msg == "" {
			msg = sentence(ue.Message)
		}
		v.Overrides.Error = "The change was not saved: " + msg
	}
	if !ds.IsDatastar(r) {
		if changeErr != nil {
			h.render(w, r, http.StatusUnprocessableEntity, v)
			return
		}
		h.Shell.SetFlash(w, flash)
		http.Redirect(w, r, fieldPolicyPath+"?"+fieldClientParam+"="+url.QueryEscape(clientID), http.StatusSeeOther) //nolint:gosec // G710: the target is a page constant; the client id is query-escaped and was checked
		return
	}
	sse := ds.NewSSE(w, r)
	if err := sse.Patch(fieldPolicyBody(v)); err != nil {
		slog.DebugContext(ctx, "ui: field policy patch not delivered", "err", err)
	}
	if changeErr == nil {
		if err := portalFlash(sse, flash, detail); err != nil {
			slog.DebugContext(ctx, "ui: field policy flash not delivered", "err", err)
		}
	}
}

// saveOverride sets or clears one member of an application's override (the
// API's PATCH /api/field-policies/{clientId}/applications/{appCode}).
func (h *FieldPolicy) saveOverride(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	clientID := strings.TrimSpace(r.PostFormValue(fieldClientParam))
	if err := auth.CanManageFieldPolicyOverrides(ac, clientID); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	appID := strings.TrimSpace(r.PostFormValue("application"))
	member := fieldidentity.PolicyMember(strings.TrimSpace(r.PostFormValue("member")))
	cmd := fieldidentity.PatchPolicyOverrideCommand{ClientID: clientID, ApplicationID: appID}
	var bad error
	switch setting := r.PostFormValue("setting"); {
	case appID == "":
		bad = usecase.Validation("POLICY_INVALID", "Choose an application.")
	case !member.Valid():
		bad = usecase.Validation("POLICY_INVALID", "Choose a setting applications can override.")
	case setting == "follow":
		cmd.Clear = []fieldidentity.PolicyMember{member}
	case (setting == "on" || setting == "off") && member.Bool():
		cmd.Set = map[fieldidentity.PolicyMember]any{member: setting == "on"}
	case setting == "seconds" && !member.Bool():
		n, err := strconv.Atoi(strings.TrimSpace(r.PostFormValue("seconds")))
		if err != nil {
			bad = usecase.Validation("POLICY_INVALID", "Enter a whole number of seconds.")
			break
		}
		cmd.Set = map[fieldidentity.PolicyMember]any{member: n}
	case member.Bool():
		bad = usecase.Validation("POLICY_INVALID", memberLabel(member)+" is on or off: choose On, Off or Follow the client.")
	default:
		bad = usecase.Validation("POLICY_INVALID", memberLabel(member)+" is a number of seconds: choose A number of seconds or Follow the client.")
	}
	if bad == nil {
		bad = h.Ops.PatchPolicyOverride(ctx, cmd)
	}
	h.overrideDone(w, r, ac, clientID, bad, flashFieldOverrideSaved, "Override saved")
}

// clearOverride clears every member of an application's override (the API's
// DELETE).
func (h *FieldPolicy) clearOverride(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	clientID := strings.TrimSpace(r.PostFormValue(fieldClientParam))
	if err := auth.CanManageFieldPolicyOverrides(ac, clientID); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	err := h.Ops.PatchPolicyOverride(ctx, fieldidentity.PatchPolicyOverrideCommand{
		ClientID: clientID, ApplicationID: strings.TrimSpace(r.PostFormValue("application")), ClearAll: true,
	})
	h.overrideDone(w, r, ac, clientID, err, flashFieldOverrideCleared, "The application follows the client's policy")
}

// saveLocks replaces the client's locks (the API's PUT
// /api/field-policies/{clientId}/locks; platform administrators only).
func (h *FieldPolicy) saveLocks(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	clientID := strings.TrimSpace(r.PostFormValue(fieldClientParam))
	if err := auth.CanLockFieldPolicy(ac, clientID); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	cmd := fieldidentity.SetPolicyLocksCommand{ClientID: clientID, LockedMembers: []fieldidentity.PolicyMember{}}
	for _, raw := range r.PostForm["locked"] {
		if m := fieldidentity.PolicyMember(raw); m.Valid() && !slices.Contains(cmd.LockedMembers, m) {
			cmd.LockedMembers = append(cmd.LockedMembers, m)
		}
	}
	// A lock can be destructive: locking unlockKeysEnabled while only an
	// application's override keeps unlock keys on revokes every key of the
	// client. Ask first, as the page's other destructive changes do.
	if r.PostFormValue("confirmRevoke") == "" {
		revokes, err := h.Ops.LockingRevokesKeys(ctx, clientID, cmd.LockedMembers)
		if err != nil {
			h.Shell.Fail(w, r, usecase.Internal("REPO", "check_field_policy_locks failed", err))
			return
		}
		if revokes {
			h.confirmLocks(w, r, ac, clientID, cmd.LockedMembers)
			return
		}
	}
	err := h.Ops.SetPolicyLocks(ctx, cmd)
	h.overrideDone(w, r, ac, clientID, err, flashFieldLocksSaved, "Locks saved")
}

// confirmLocks shows the locks form again as submitted, with the warning that
// saving revokes every unlock key of the client and a button that confirms.
func (h *FieldPolicy) confirmLocks(w http.ResponseWriter, r *http.Request, ac *auth.AuthContext, clientID string, locked []fieldidentity.PolicyMember) {
	ctx := r.Context()
	v, err := h.read(ctx, ac, clientID)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	v.Overrides.Locked = map[fieldidentity.PolicyMember]bool{}
	for _, m := range locked {
		v.Overrides.Locked[m] = true
	}
	v.Overrides.ConfirmRevoke = true
	if !ds.IsDatastar(r) {
		h.render(w, r, http.StatusConflict, v)
		return
	}
	sse := ds.NewSSE(w, r)
	if err := sse.Patch(fieldPolicyBody(v)); err != nil {
		slog.DebugContext(ctx, "ui: field policy patch not delivered", "err", err)
	}
}
