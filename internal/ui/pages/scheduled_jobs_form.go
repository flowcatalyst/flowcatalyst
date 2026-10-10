package pages

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/a-h/templ"

	sjops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/scheduledjob/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/ds"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/kit"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/shell"
)

// The create and edit dialogs (the SPA's ScheduledJobCreateDrawer and the
// detail drawer's inline edit). Cron expressions are a textarea, one per
// line (the SPA's add/remove rows); the payload is JSON text.

const (
	sjFormDialog = "sj-form"
	sjFormSignal = "sjForm"
)

var sjFieldCodes = map[string]string{
	"CODE_REQUIRED":       "code",
	"INVALID_CODE_FORMAT": "code",
	"CODE_EXISTS":         "code",
	"NAME_REQUIRED":       "name",
	"CRONS_REQUIRED":      "crons",
	"INVALID_CRON":        "crons",
	"CRON_INVALID_SHAPE":  "crons",
	"INVALID_TARGET_URL":  "targetUrl",
}

type sjForm struct {
	Edit            bool
	JobID, Subtitle string

	Code, Name, Description string
	Scope, ApplicationID    string
	Timezone, Crons         string
	TargetURL               string
	MaxAttempts, Timeout    string
	Payload                 string
	Concurrent, Tracks      bool

	Errs formErrors
	Open bool

	ScopeOptions, AppOptions []kit.Option
}

func (f sjForm) id(key string) string { return fieldID(sjFormDialog, key) }

func (f sjForm) keys() []string {
	if f.Edit {
		return []string{"name", "description", "timezone", "deliveryMaxAttempts", "timeoutSeconds", "targetUrl", "crons", "payload", "options"}
	}
	return []string{"code", "name", "description", "scope", "applicationId", "timezone", "crons", "targetUrl", "deliveryMaxAttempts", "timeoutSeconds", "options", "payload"}
}

func (f sjForm) focusID(key string) string {
	if key == "options" {
		return f.id("concurrent")
	}
	return f.id(key)
}

func (f sjForm) firstInvalid() string {
	for _, k := range f.keys() {
		if f.Errs.Fields[k] != "" {
			return f.focusID(k)
		}
	}
	return ""
}

func (f sjForm) fields() []templ.Component {
	out := make([]templ.Component, 0, len(f.keys()))
	for _, k := range f.keys() {
		out = append(out, sjField(f, k))
	}
	return out
}

func (f sjForm) dialog() kit.FormDialogProps {
	p := kit.FormDialogProps{
		ID: sjFormDialog, Signal: sjFormSignal, Width: kit.WidthWide, Action: sjPath + "/new", Open: f.Open, Error: f.Errs.Form,
		Title: "New Scheduled Job", Description: "Register a cron-driven job the platform will fire on schedule.", SubmitLabel: "Create",
	}
	if f.Edit {
		p.Action = sjURL(f.JobID) + "/edit"
		p.Title, p.Description, p.SubmitLabel, p.RequireChange = "Edit Scheduled Job", f.Subtitle, "Save", true
		p.Hidden = []kit.HiddenField{drawerField(sjDrawerSig)}
	}
	return p
}

func submittedSjForm(r *http.Request, edit bool) sjForm {
	return sjForm{
		Edit: edit, Code: strings.TrimSpace(r.PostFormValue("code")), Name: r.PostFormValue("name"),
		Description: r.PostFormValue("description"), Scope: r.PostFormValue("scope"), ApplicationID: r.PostFormValue("applicationId"),
		Timezone: strings.TrimSpace(r.PostFormValue("timezone")), Crons: r.PostFormValue("crons"), TargetURL: r.PostFormValue("targetUrl"),
		MaxAttempts: r.PostFormValue("deliveryMaxAttempts"), Timeout: r.PostFormValue("timeoutSeconds"), Payload: r.PostFormValue("payload"),
		Concurrent: r.PostFormValue("concurrent") != "", Tracks: r.PostFormValue("tracksCompletion") != "",
	}
}

// cronLines is the SPA's cleanCrons: trimmed, blanks dropped.
func cronLines(raw string) []string {
	var out []string
	for line := range strings.SplitSeq(raw, "\n") {
		if c := strings.TrimSpace(line); c != "" {
			out = append(out, c)
		}
	}
	return out
}

// parsed reads what the SPA checks before sending: name, at least one cron,
// the payload as JSON (or empty), the numbers.
func (f *sjForm) parsed() (crons []string, payload json.RawMessage, maxAttempts, timeout *int32) {
	if strings.TrimSpace(f.Name) == "" {
		f.Errs.field("name", "Name is required.")
	}
	if crons = cronLines(f.Crons); len(crons) == 0 {
		f.Errs.field("crons", "At least one cron expression is required.")
	}
	if p := strings.TrimSpace(f.Payload); p != "" {
		if !json.Valid([]byte(p)) {
			f.Errs.field("payload", "Payload must be valid JSON or empty.")
		} else {
			payload = json.RawMessage(p)
		}
	}
	var ok bool
	if maxAttempts, ok = optionalInt32(f.MaxAttempts); !ok {
		f.Errs.field("deliveryMaxAttempts", notWholeNumber)
	}
	if timeout, ok = optionalInt32(f.Timeout); !ok {
		f.Errs.field("timeoutSeconds", notWholeNumber)
	}
	return crons, payload, maxAttempts, timeout
}

func (h *ScheduledJobs) createForm(w http.ResponseWriter, r *http.Request) {
	ac := auth.FromContext(r.Context())
	if err := auth.CanCreateScheduledJobs(ac); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	// The SPA's defaults.
	h.showForm(w, r, ac, sjForm{Open: true, Timezone: "UTC", MaxAttempts: "3"})
}

func (h *ScheduledJobs) create(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	f := submittedSjForm(r, false)
	if err := auth.CanCreateScheduledJobs(ac); err != nil {
		f.Errs.apply(r, err, nil)
		h.formFailed(w, r, ac, f)
		return
	}
	if f.Code == "" {
		f.Errs.field("code", "Code is required.")
	}
	if f.Scope == "" {
		f.Errs.field("scope", "Choose Platform or a client: this can't be changed after creation.")
	}
	crons, payload, maxAttempts, timeout := f.parsed()
	if f.Errs.any() {
		h.formFailed(w, r, ac, f)
		return
	}
	cmd := sjops.CreateCommand{
		Code: f.Code, Name: f.Name, Crons: crons, Timezone: f.Timezone, Description: optionalText(strings.TrimSpace(f.Description)),
		Payload: payload, Concurrent: f.Concurrent, TracksCompletion: f.Tracks, TimeoutSeconds: timeout, DeliveryMaxAttempts: maxAttempts,
		TargetURL: optionalText(strings.TrimSpace(f.TargetURL)), ApplicationID: optionalText(f.ApplicationID),
	}
	if f.Scope != sjPlatform {
		cmd.ClientID = &f.Scope
	}
	newID, err := h.Ops.Create(ctx, cmd)
	if err != nil {
		f.Errs.apply(r, err, sjFieldCodes)
		h.formFailed(w, r, ac, f)
		return
	}
	if !ds.IsDatastar(r) {
		h.Shell.SetFlash(w, flashSjCreated)
		http.Redirect(w, r, sjPath, http.StatusSeeOther)
		return
	}
	sse := ds.NewSSE(w, r)
	if err := sse.CloseDialog(sjFormDialog); err != nil {
		slog.DebugContext(ctx, "ui: scheduled job form close not delivered", "err", err)
	}
	h.afterChange(sse, r, ac, newID, false)
	if j, err := h.loadJob(ctx, ac, newID); err != nil {
		h.Shell.FailSSE(sse, r, err)
	} else if d, err := h.detailOf(ctx, ac, j); err != nil {
		h.Shell.FailSSE(sse, r, err)
	} else if err := openSjDrawer(sse, d); err != nil {
		slog.DebugContext(ctx, "ui: scheduled job drawer patch not delivered", "err", err)
	}
	msg := FlashMessages[flashSjCreated]
	msg.Summary, msg.Detail = "Created", "Scheduled job '"+f.Code+"' created"
	if err := shell.Flash(sse, msg); err != nil {
		slog.DebugContext(ctx, "ui: scheduled job flash not delivered", "err", err)
	}
}

func (h *ScheduledJobs) editForm(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	if err := firstError(auth.CanReadScheduledJobs(ac), auth.CanUpdateScheduledJobs(ac)); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	id, err := sjID(r)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	j, err := h.loadJob(ctx, ac, id)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	f := sjForm{
		Edit: true, JobID: j.ID, Subtitle: j.Name + " (" + j.Code + ")", Open: true,
		Name: j.Name, Description: deref(j.Description), Timezone: j.Timezone, Crons: strings.Join(j.Crons, "\n"),
		TargetURL: deref(j.TargetURL), MaxAttempts: strconv.Itoa(int(j.DeliveryMaxAttempts)), Payload: prettyJSON(j.Payload),
		Concurrent: j.Concurrent, Tracks: j.TracksCompletion,
	}
	if j.TimeoutSeconds != nil {
		f.Timeout = strconv.Itoa(int(*j.TimeoutSeconds))
	}
	h.showForm(w, r, ac, f)
}

func (h *ScheduledJobs) update(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	f := submittedSjForm(r, true)
	if err := firstError(auth.CanReadScheduledJobs(ac), auth.CanUpdateScheduledJobs(ac)); err != nil {
		f.Errs.apply(r, err, nil)
		h.formFailed(w, r, ac, f)
		return
	}
	id, err := sjID(r)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	j, err := h.loadJob(ctx, ac, id)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	f.JobID, f.Subtitle = j.ID, j.Name+" ("+j.Code+")"
	crons, payload, maxAttempts, timeout := f.parsed()
	if f.Errs.any() {
		h.formFailed(w, r, ac, f)
		return
	}
	// The SPA's PUT body: blank description, timezone and target URL are
	// not sent (kept), an empty payload is not sent.
	name := strings.TrimSpace(f.Name)
	if err := h.Ops.Update(ctx, sjops.UpdateCommand{
		ID: id, Name: &name, Description: optionalText(strings.TrimSpace(f.Description)), Crons: crons, Timezone: optionalText(f.Timezone),
		Payload: payload, Concurrent: &f.Concurrent, TracksCompletion: &f.Tracks, TimeoutSeconds: timeout, DeliveryMaxAttempts: maxAttempts,
		TargetURL: optionalText(strings.TrimSpace(f.TargetURL)),
	}); err != nil {
		f.Errs.apply(r, err, sjFieldCodes)
		h.formFailed(w, r, ac, f)
		return
	}
	if !ds.IsDatastar(r) {
		h.Shell.SetFlash(w, flashSjUpdated)
		http.Redirect(w, r, sjPath, http.StatusSeeOther)
		return
	}
	sse := ds.NewSSE(w, r)
	if err := sse.CloseDialog(sjFormDialog); err != nil {
		slog.DebugContext(ctx, "ui: scheduled job form close not delivered", "err", err)
	}
	h.afterChange(sse, r, ac, id, false)
	if err := flashKey(sse, flashSjUpdated); err != nil {
		slog.DebugContext(ctx, "ui: scheduled job flash not delivered", "err", err)
	}
}

// withChoices: the scope select (Platform, then the BFF's client options)
// and the active applications.
func (h *ScheduledJobs) withChoices(r *http.Request, ac *auth.AuthContext, f *sjForm) {
	if f.Edit {
		return
	}
	opts, err := h.filterOptions(r.Context(), ac)
	if err != nil {
		slog.WarnContext(r.Context(), "ui: scheduled job choices unavailable", "err", err)
		return
	}
	f.ScopeOptions = []kit.Option{{Value: sjPlatform, Label: "Platform-scoped (anchor only)"}}
	for _, o := range opts[sjFacetClients] {
		if o.Value != sjPlatform {
			f.ScopeOptions = append(f.ScopeOptions, o)
		}
	}
	f.AppOptions = opts[sjFacetApps]
}

func (h *ScheduledJobs) showForm(w http.ResponseWriter, r *http.Request, ac *auth.AuthContext, f sjForm) {
	h.withChoices(r, ac, &f)
	if ds.IsDatastar(r) {
		if err := ds.NewSSE(w, r).Patch(sjFormDialogC(f), ds.Inner("#"+sjDialogSlot)); err != nil {
			slog.DebugContext(r.Context(), "ui: scheduled job form patch not delivered", "err", err)
		}
		return
	}
	view, err := h.listView(r.Context(), ac, parseListQuery(nil, sjList))
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	view.Dialog = sjFormDialogC(f)
	h.renderPage(w, r, http.StatusOK, view)
}

func (h *ScheduledJobs) formFailed(w http.ResponseWriter, r *http.Request, ac *auth.AuthContext, f sjForm) {
	h.withChoices(r, ac, &f)
	if ds.IsDatastar(r) {
		patchFormFailure(ds.NewSSE(w, r), r, sjFormDialog, f.fields(), f.Errs.Form, f.firstInvalid())
		return
	}
	view, err := h.listView(r.Context(), ac, parseListQuery(nil, sjList))
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	f.Open = true
	view.Dialog = sjFormDialogC(f)
	h.renderPage(w, r, cmpOr(f.Errs.Status, http.StatusUnprocessableEntity), view)
}

func (f sjForm) fieldProps(key, label, help string, required bool) kit.FieldProps {
	return kit.FieldProps{Label: label, For: f.focusID(key), Required: required, Description: help, Error: f.Errs.Fields[key], Attrs: templ.Attributes{"id": f.id(key) + "-field"}}
}

func (f sjForm) describedBy(key string) string {
	if f.Errs.Fields[key] != "" {
		return f.focusID(key) + "-err"
	}
	return ""
}

func (f sjForm) inputProps(key, value, placeholder string, attrs templ.Attributes) kit.InputProps {
	return kit.InputProps{ID: f.id(key), Name: key, Value: value, Placeholder: placeholder, Invalid: f.Errs.Fields[key] != "", DescribedBy: f.describedBy(key), Attrs: attrs}
}

func sjNameAttrs(edit bool) templ.Attributes {
	a := templ.Attributes{"required": true, "autocomplete": "off"}
	if edit {
		a["autofocus"] = true
	}
	return a
}
