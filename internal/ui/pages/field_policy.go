package pages

import (
	"context"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/fieldidentity"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/shell"
	"github.com/flowcatalyst/framework/go/ui/ds"
	"github.com/flowcatalyst/framework/go/ui/kit"
	"github.com/flowcatalyst/go-sdk/usecase"
)

// FieldPolicy is the field policy page (/ui/field-policy): a client's
// effective policy (the platform defaults until it has its own), with the
// edit form. Reads make GET /api/field-policies/{clientId}'s check
// (CanReadFieldDevices); a save PUT's (CanManageFieldDevices), and the
// operation refuses an application-confined caller.
type FieldPolicy struct {
	Store   FieldStore
	Ops     FieldCommands
	Clients ClientStore
	Shell   *shell.Shell
}

const (
	fieldPolicyPath   = "/ui/field-policy"
	fieldPolicyRegion = "field-policy-region"
	fieldPolicyForm   = "field-policy-form"
)

// Mount registers the page's routes.
func (h *FieldPolicy) Mount(r chi.Router) {
	r.Get("/field-policy", h.page)
	r.With(parseForm).Post("/field-policy", h.save)
}

// policyField is one number of the policy with the range the API enforces
// (fieldidentity.PutPolicyCommand.Build).
type policyField struct {
	Key, Label, Help string
	Min, Max         int
	Unit             string
	get              func(fieldidentity.Policy) int
}

var policyFields = []policyField{
	{Key: "pinMinLength", Label: "Minimum PIN length", Min: 4, Max: 12, Unit: "digits", Help: "4 to 12 (default 6). Below 6 the hard-lock threshold may be at most 5.", get: func(p fieldidentity.Policy) int { return p.PINMinLength }},
	{Key: "softLockThreshold", Label: "Soft lock after", Min: 1, Max: 20, Unit: "failed attempts", Help: "1 to 20 (default 5).", get: func(p fieldidentity.Policy) int { return p.SoftLockThreshold }},
	{Key: "softLockSecs", Label: "Soft lock lasts", Min: 30, Max: 86400, Unit: "seconds", Help: "30 to 86400 (default 900).", get: func(p fieldidentity.Policy) int { return p.SoftLockSecs }},
	{Key: "hardLockThreshold", Label: "Hard lock after", Min: 1, Max: 50, Unit: "failed attempts", Help: "From the soft-lock threshold to 50 (default 10). A hard lock needs a supervisor to unlock.", get: func(p fieldidentity.Policy) int { return p.HardLockThreshold }},
	{Key: "sharedIdleSecs", Label: "Shared device idle timeout", Min: 60, Max: 86400, Unit: "seconds", Help: "60 to 86400 (default 900).", get: func(p fieldidentity.Policy) int { return p.SharedIdleSecs }},
	{Key: "personalIdleSecs", Label: "Personal device idle timeout", Min: 60, Max: 86400, Unit: "seconds", Help: "60 to 86400 (default 7200).", get: func(p fieldidentity.Policy) int { return p.PersonalIdleSecs }},
	{Key: "shiftMaxSecs", Label: "Maximum shift length", Min: 3600, Max: 86400, Unit: "seconds", Help: "3600 to 86400 (default 50400). A session on a shared device is also capped at 12 hours.", get: func(p fieldidentity.Policy) int { return p.ShiftMaxSecs }},
	{Key: "pinReauthDays", Label: "PIN re-entry on a personal device", Min: 1, Max: 90, Unit: "days", Help: "1 to 90 (default 7).", get: func(p fieldidentity.Policy) int { return p.PINReauthDays }},
}

type fieldPolicyView struct {
	Client        string
	ClientName    string
	ClientOptions []kit.Option
	Policy        fieldidentity.Policy
	CanManage     bool
	// Values are the form's inputs as shown (the saved ones, or what was
	// submitted when a save was refused).
	Values      map[string]string
	Attestation bool
	Errs        formErrors
	UpdatedAt   time.Time
	Status      int
}

func newPolicyView(p fieldidentity.Policy) fieldPolicyView {
	v := fieldPolicyView{Policy: p, Values: map[string]string{}, Attestation: p.RequireAttestation, UpdatedAt: p.UpdatedAt}
	for _, f := range policyFields {
		v.Values[f.Key] = strconv.Itoa(f.get(p))
	}
	return v
}

func (v fieldPolicyView) clientSelect() kit.FilterSelectProps {
	return kit.FilterSelectProps{Name: fieldClientParam, Label: "Client", Placeholder: "Select a client", Options: v.ClientOptions, Selected: v.Client, AutoSubmit: true}
}

func (h *FieldPolicy) read(ctx context.Context, ac *auth.AuthContext, requested string) (fieldPolicyView, error) {
	opts, err := portalClientOptions(ctx, h.Clients, ac)
	if err != nil {
		return fieldPolicyView{}, err
	}
	clientID, opts := portalSelectedClient(ac, opts, requested)
	v := fieldPolicyView{Client: clientID, ClientOptions: opts, ClientName: portalClientName(opts, clientID)}
	if clientID == "" {
		return v, nil
	}
	if err := auth.CanReadFieldDevices(ac, clientID); err != nil {
		return fieldPolicyView{}, err
	}
	p, err := h.Store.Policy(ctx, clientID)
	if err != nil {
		return fieldPolicyView{}, usecase.Internal("REPO", "find_field_policy failed", err)
	}
	nv := newPolicyView(p)
	nv.Client, nv.ClientOptions, nv.ClientName = clientID, opts, v.ClientName
	nv.CanManage = auth.CanManageFieldDevices(ac, clientID) == nil && !ac.IsApplicationScoped()
	return nv, nil
}

func (h *FieldPolicy) render(w http.ResponseWriter, r *http.Request, status int, v fieldPolicyView) {
	h.Shell.RenderStatus(w, r, status, shell.Page{Title: "Field Policy", Path: fieldPolicyPath}, fieldPolicyPage(v))
}

func (h *FieldPolicy) page(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	v, err := h.read(ctx, ac, r.URL.Query().Get(fieldClientParam))
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	h.render(w, r, http.StatusOK, v)
}

// policyFieldCodes maps the operation's field name (the error detail) to the
// form's input.
func policyErrorField(err error) string {
	if ue := usecase.AsError(err); ue != nil && ue.Code == "POLICY_INVALID" {
		if f, ok := ue.Details["field"].(string); ok {
			return f
		}
	}
	return ""
}

// policyMessage restates the operation's validation message with the form's
// labels in place of the API's field names ("softLockSecs must be between 30
// and 86400" becomes "Soft lock lasts must be between 30 and 86400"). The
// API's error is unchanged; this is the UI layer only.
func policyMessage(field, msg string) string {
	label := func(f policyField) string { return f.Label }
	rest := strings.TrimPrefix(msg, field)
	if rest == msg {
		return sentence(msg)
	}
	for _, f := range policyFields {
		l := label(f)
		rest = strings.ReplaceAll(rest, f.Key, strings.ToLower(l[:1])+l[1:])
	}
	for _, f := range policyFields {
		if f.Key == field {
			return label(f) + rest
		}
	}
	return sentence(msg)
}

func (h *FieldPolicy) save(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	clientID := strings.TrimSpace(r.PostFormValue(fieldClientParam))
	if clientID == "" {
		h.Shell.Fail(w, r, usecase.Validation("CLIENT_ID_REQUIRED", "clientId is required"))
		return
	}
	// PUT /api/field-policies/{clientId}'s check, before anything is read.
	if err := auth.CanManageFieldDevices(ac, clientID); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	// The policy is client-wide: an application-confined caller may not change
	// it (the operation refuses it too).
	if ac.IsApplicationScoped() {
		h.Shell.Fail(w, r, usecase.Authorization("APPLICATION_FORBIDDEN", "an application-confined caller cannot change the client's field policy"))
		return
	}
	cur, err := h.read(ctx, ac, clientID)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	v := cur
	v.Attestation = r.PostFormValue("requireAttestation") != ""
	cmd := fieldidentity.PutPolicyCommand{ClientID: clientID, RequireAttestation: v.Attestation}
	slots := make(map[string]*int, len(policyFields))
	for _, f := range policyFields {
		raw := strings.TrimSpace(r.PostFormValue(f.Key))
		v.Values[f.Key] = raw
		if raw == "" {
			continue // blank takes the default, as the API's omitted field
		}
		n, err := strconv.Atoi(raw)
		if err != nil {
			v.Errs.field(f.Key, "Enter a whole number.")
			continue
		}
		slots[f.Key] = &n
	}
	cmd.PINMinLength, cmd.SoftLockThreshold, cmd.SoftLockSecs = slots["pinMinLength"], slots["softLockThreshold"], slots["softLockSecs"]
	cmd.HardLockThreshold, cmd.SharedIdleSecs, cmd.PersonalIdleSecs = slots["hardLockThreshold"], slots["sharedIdleSecs"], slots["personalIdleSecs"]
	cmd.ShiftMaxSecs, cmd.PINReauthDays = slots["shiftMaxSecs"], slots["pinReauthDays"]
	if !v.Errs.any() {
		if err := h.Ops.PutPolicy(ctx, cmd); err != nil {
			if f := policyErrorField(err); f != "" {
				if ue := usecase.AsError(err); ue != nil {
					v.Errs.field(f, policyMessage(f, ue.Message))
				}
			} else {
				v.Errs.apply(r, err, nil)
			}
		}
	}
	if v.Errs.any() {
		if v.Errs.Status == 0 {
			v.Errs.Status = http.StatusUnprocessableEntity
		}
		if ds.IsDatastar(r) {
			sse := ds.NewSSE(w, r)
			if err := sse.Patch(fieldPolicyBody(v)); err != nil {
				slog.DebugContext(ctx, "ui: field policy patch not delivered", "err", err)
			}
			return
		}
		h.render(w, r, v.Errs.Status, v)
		return
	}
	if !ds.IsDatastar(r) {
		h.Shell.SetFlash(w, flashFieldPolicySaved)
		http.Redirect(w, r, fieldPolicyPath+"?"+fieldClientParam+"="+url.QueryEscape(clientID), http.StatusSeeOther) //nolint:gosec // G710: the target is a page constant; the client id is query-escaped and was checked by the manage check
		return
	}
	saved, err := h.read(ctx, ac, clientID)
	sse := ds.NewSSE(w, r)
	if err != nil {
		h.Shell.FailSSE(sse, r, err)
		return
	}
	if err := sse.Patch(fieldPolicyBody(saved)); err != nil {
		slog.DebugContext(ctx, "ui: field policy patch not delivered", "err", err)
	}
	if err := portalFlash(sse, flashFieldPolicySaved, "Field policy saved for "+saved.ClientName); err != nil {
		slog.DebugContext(ctx, "ui: field policy flash not delivered", "err", err)
	}
}
