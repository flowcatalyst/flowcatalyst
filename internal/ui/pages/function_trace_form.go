package pages

import (
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/a-h/templ"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/function"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/function/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/ds"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/kit"
)

// The enable dialog (the SPA's "Enable tracing" dialog of
// FunctionTraceControls): how long to stay on, or until switched off (a
// declared context, with platform:function:trace:indefinite, at most the
// configured number of days, confirmed), and the sample rate.

const (
	traceEnableDialog = "trace-enable"
	traceEnableSignal = "traceEnable"
	// traceIndefSignal is the "until switched off" checkbox, which disables
	// the duration select and shows the confirmation.
	traceIndefSignal = "traceIndef"
)

// traceFieldCodes map the enable use case's error codes to the dialog's
// fields; the rest goes to the form-level region.
var traceFieldCodes = map[string]string{
	operations.TraceWindowTooLong:        "minutes",
	operations.TraceIndefiniteNotAllowed: "indefinite",
	"TRACE_MINUTES_INVALID":       "minutes",
	"TRACE_SAMPLE_RATE_INVALID":   "sampleRate",
}

// traceDurations is the SPA's 5, 15, 30 and 60 minutes limited to the
// context's cap; a cap below 5 offers just the cap.
func traceDurations(maxMinutes int) []int {
	var out []int
	for _, m := range []int{5, 15, 30, 60} {
		if m <= maxMinutes {
			out = append(out, m)
		}
	}
	if len(out) == 0 {
		out = []int{max(1, maxMinutes)}
	}
	return out
}

type traceForm struct {
	FnID, Context string
	MaxMinutes    int
	Minutes       string
	Indefinite    bool
	// IndefiniteMaxDays is the ceiling of an "until switched off" window for
	// the context (0 for http); AllowIndefinite offers the choice (a ceiling
	// and the viewer's platform:function:trace:indefinite). Confirm is the
	// "I understand" checkbox the choice needs.
	IndefiniteMaxDays int
	AllowIndefinite   bool
	Confirm           bool
	SampleRate        string
	// OnPage is the log page (not the drawer) the dialog was opened on.
	OnPage bool
	Errs   formErrors
	Open   bool
}

func (f traceForm) id(key string) string { return fieldID(traceEnableDialog, key) }

func (f traceForm) keys() []string {
	if f.AllowIndefinite {
		return []string{"minutes", "indefinite", "indefiniteConfirm", "sampleRate"}
	}
	return []string{"minutes", "sampleRate"}
}

func (f traceForm) focusID(key string) string { return f.id(key) }

func (f traceForm) firstInvalid() string {
	for _, k := range f.keys() {
		if f.Errs.Fields[k] != "" {
			return f.focusID(k)
		}
	}
	return ""
}

func (f traceForm) fields() []templ.Component {
	out := make([]templ.Component, 0, len(f.keys()))
	for _, k := range f.keys() {
		out = append(out, traceField(f, k))
	}
	return out
}

func (f traceForm) dialog() kit.FormDialogProps {
	page := ""
	if f.OnPage {
		page = "traces"
	}
	return kit.FormDialogProps{
		ID: traceEnableDialog, Signal: traceEnableSignal, Width: kit.WidthNarrow, Open: f.Open, Error: f.Errs.Form,
		Action: traceContextPath(f.FnID, f.Context) + "/enable", Title: "Enable tracing: " + f.Context,
		Description: "Capture this context's records for a while, then it turns itself off.", SubmitLabel: "Enable",
		Hidden: []kit.HiddenField{{Name: "page", Value: page}},
	}
}

func (f traceForm) durationOptions() []kit.Option {
	var opts []kit.Option
	for _, m := range traceDurations(f.MaxMinutes) {
		opts = append(opts, kit.Option{Value: strconv.Itoa(m), Label: strconv.Itoa(m) + " minutes"})
	}
	return opts
}

func (f traceForm) describedBy(key string) string {
	if f.Errs.Fields[key] != "" {
		return f.id(key) + "-err"
	}
	return ""
}

// indefiniteLabel is the choice as the SPA words it.
func (f traceForm) indefiniteLabel() string {
	return "Until switched off (max " + strconv.Itoa(f.IndefiniteMaxDays) + " days)"
}

// confirmWarning is what the confirmation asks the operator to accept.
func (f traceForm) confirmWarning() string {
	return "This keeps recording every sampled call until someone switches it off, for at most " +
		strconv.Itoa(f.IndefiniteMaxDays) + " days; then it switches off by itself."
}

// checkboxAttrs adds the field's error wiring to a checkbox's attributes.
func (f traceForm) checkboxAttrs(key string, a templ.Attributes) templ.Attributes {
	if f.Errs.Fields[key] != "" {
		a["aria-invalid"] = "true"
		a["aria-describedby"] = f.describedBy(key)
	}
	return a
}

// confirmStyle hides the confirmation until "until switched off" is ticked
// (data-show takes over once the page's script runs).
func (f traceForm) confirmStyle() string {
	if f.Indefinite {
		return ""
	}
	return "display: none"
}

func (f traceForm) signals() string {
	return "{" + traceIndefSignal + ": " + strconv.FormatBool(f.Indefinite) + "}"
}

// submittedTraceForm reads the dialog's post.
func submittedTraceForm(r *http.Request, fnID, name string) traceForm {
	return traceForm{
		FnID: fnID, Context: name, Minutes: r.PostFormValue("minutes"), Indefinite: r.PostFormValue("indefinite") != "",
		Confirm: r.PostFormValue("indefiniteConfirm") != "",
		SampleRate: strings.TrimSpace(r.PostFormValue("sampleRate")), OnPage: r.PostFormValue("page") == "traces",
	}
}

// mayEnable is PUT /api/functions/{id}/trace/{context}'s rule: manage, and
// view-bodies too for http (it captures bodies).
func mayEnable(ac *auth.AuthContext, name string) error {
	if err := auth.CanManageFunctionTrace(ac); err != nil {
		return err
	}
	if name == function.TraceHTTPContext {
		return auth.RequireFunctionTraceBodies(ac)
	}
	return nil
}

// withContext sets the context's caps on the form (they are the dialog's
// duration choices and whether "until switched off" is offered) and returns
// its state; an unknown context is not found.
func (h *FunctionTrace) withContext(r *http.Request, f *traceForm) (operations.TraceContextState, error) {
	states, err := h.Store.Contexts(r.Context(), f.FnID)
	if err != nil {
		return operations.TraceContextState{}, err
	}
	for _, st := range states {
		if st.Name == f.Context {
			f.MaxMinutes, f.IndefiniteMaxDays = st.MaxMinutes, st.IndefiniteMaxDays
			f.AllowIndefinite = st.IndefiniteMaxDays > 0 && traceGatesOf(auth.FromContext(r.Context())).Indefinite
			return st, nil
		}
	}
	return operations.TraceContextState{}, httperror.NotFound("Trace context", f.Context)
}

func (h *FunctionTrace) enableForm(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	id, err := traceFunctionID(r)
	var name string
	if err == nil {
		name, err = traceContextName(r)
	}
	if err == nil {
		err = firstError(auth.CanViewFunctionTrace(ac), mayEnable(ac, name))
	}
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	f := traceForm{FnID: id, Context: name, SampleRate: "1", OnPage: r.URL.Query().Get("page") == "traces", Open: true}
	st, err := h.withContext(r, &f)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	// The SPA opens on the context's first duration, or on "until switched
	// off" when it already is (still to be confirmed).
	f.Minutes, f.Indefinite = strconv.Itoa(traceDurations(f.MaxMinutes)[0]), st.Indefinite && f.AllowIndefinite
	h.showForm(w, r, ac, f)
}

func (h *FunctionTrace) showForm(w http.ResponseWriter, r *http.Request, ac *auth.AuthContext, f traceForm) {
	if ds.IsDatastar(r) {
		if err := patchDialogSlot(ds.NewSSE(w, r), traceDialogSlot, traceFormDialogC(f)); err != nil {
			slog.DebugContext(r.Context(), "ui: trace form patch not delivered", "err", err)
		}
		return
	}
	fn, err := h.function(r.Context(), ac, f.FnID)
	if err == nil {
		var v traceLogView
		if v, err = h.logView(r.Context(), ac, fn, parseTraceQuery(nil), ""); err == nil {
			f.Open = true
			v.Dialog = traceFormDialogC(f)
			h.renderPage(w, r, cmpOr(f.Errs.Status, http.StatusOK), v)
			return
		}
	}
	h.Shell.Fail(w, r, err)
}

func (h *FunctionTrace) formFailed(w http.ResponseWriter, r *http.Request, ac *auth.AuthContext, f traceForm) {
	if f.MaxMinutes == 0 {
		// A refusal before the contexts were read (a missing permission): the
		// dialog only needs the cap to draw its choices.
		if _, err := h.withContext(r, &f); err != nil {
			f.MaxMinutes = function.DefaultTraceMaxMinutes
		}
	}
	if ds.IsDatastar(r) {
		patchFormFailure(ds.NewSSE(w, r), r, traceEnableDialog, f.fields(), f.Errs.Form, f.firstInvalid())
		return
	}
	f.Open = true
	h.showForm(w, r, ac, f)
}

func (h *FunctionTrace) enable(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	id, err := traceFunctionID(r)
	var name string
	if err == nil {
		name, err = traceContextName(r)
	}
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	f := submittedTraceForm(r, id, name)
	if err := mayEnable(ac, name); err != nil {
		f.Errs.apply(r, err, nil)
		h.formFailed(w, r, ac, f)
		return
	}
	if _, err := h.withContext(r, &f); err != nil {
		f.Errs.apply(r, err, nil)
		h.formFailed(w, r, ac, f)
		return
	}
	cmd := operations.EnableTraceCommand{FunctionID: id, Context: name, Indefinite: f.Indefinite}
	if rate, perr := strconv.Atoi(f.SampleRate); perr != nil || rate < function.MinTraceSampleRate || rate > function.MaxTraceSampleRate {
		f.Errs.field("sampleRate", "Enter a whole number from 1 to 1000.")
	} else {
		cmd.SampleRate = rate
	}
	switch {
	// The choice is not on the dialog in these two cases (a forged post), so
	// the refusal is the form's.
	case f.Indefinite && !f.AllowIndefinite && f.IndefiniteMaxDays == 0:
		f.Errs.Form, f.Errs.Status = "The "+name+" context always turns itself off; choose how long to stay on.", http.StatusUnprocessableEntity
	case f.Indefinite && !f.AllowIndefinite:
		f.Errs.Form, f.Errs.Status = "You do not have permission to leave tracing on until it is switched off.", http.StatusForbidden
	case f.Indefinite && !f.Confirm:
		f.Errs.field("indefiniteConfirm", "Confirm that it stays on until it is switched off.")
	}
	if !f.Indefinite {
		if mins, perr := strconv.Atoi(f.Minutes); perr != nil || mins < 1 {
			f.Errs.field("minutes", "Choose how long to stay on.")
		} else {
			cmd.Minutes = mins
		}
	}
	if f.Errs.any() {
		h.formFailed(w, r, ac, f)
		return
	}
	if err := h.Ops.Enable(ctx, cmd); err != nil {
		f.Errs.apply(r, err, traceFieldCodes)
		h.formFailed(w, r, ac, f)
		return
	}
	detail := name + " is live for " + strconv.Itoa(cmd.Minutes) + " minutes"
	if f.Indefinite {
		detail = name + " is on until it is switched off (at most " + strconv.Itoa(f.IndefiniteMaxDays) + " days)"
	}
	msg := FlashMessages[flashTraceOn]
	msg.Detail = detail
	h.afterSwitch(w, r, ac, id, f.OnPage, flashTraceOn, &msg, true)
}
