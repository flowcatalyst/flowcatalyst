package pages

import (
	"log/slog"
	"net/http"
	"strings"

	"github.com/a-h/templ"

	procops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/process/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/ds"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/kit"
)

// The create and edit dialogs (the SPA's ProcessCreatePage and
// ProcessEditPage, full pages there).

const (
	procFormDialog = "proc-form"
	procFormSignal = "procForm"
)

var procFieldCodes = map[string]string{
	"CODE_REQUIRED":       "code",
	"INVALID_CODE_FORMAT": "code",
	"CODE_EXISTS":         "code",
	"NAME_REQUIRED":       "name",
}

// procBodyPlaceholder is the SPA's example diagram.
const procBodyPlaceholder = `graph TD
  A[Fulfilment Created] --> B{Geocoded?}
  B -- No --> C[Enrich locations via dispatch job]
  C --> D[Reactive Aggregate: build shipment]
  B -- Yes --> D
  D --> E[Shipment Created]`

type procForm struct {
	Edit      bool
	ProcessID string
	Subtitle  string

	Code, Name, Description, Tags, Body string

	Errs formErrors
	Open bool
}

func (f procForm) id(key string) string         { return fieldID(procFormDialog, key) }
func (f procForm) fieldError(key string) string { return f.Errs.Fields[key] }

func (f procForm) keys() []string {
	k := []string{"name", "description", "tags", "body"}
	if f.Edit {
		return k
	}
	return append([]string{"code"}, k...)
}

func (f procForm) firstInvalid() string {
	for _, k := range f.keys() {
		if f.fieldError(k) != "" {
			return f.id(k)
		}
	}
	return ""
}

func (f procForm) fields() []templ.Component {
	out := make([]templ.Component, 0, len(f.keys()))
	for _, k := range f.keys() {
		out = append(out, procField(f, k))
	}
	return out
}

func (f procForm) dialog() kit.FormDialogProps {
	p := kit.FormDialogProps{
		ID: procFormDialog, Signal: procFormSignal, Width: kit.WidthWide, Action: procsPath + "/new", Open: f.Open, Error: f.Errs.Form,
		Title: "New process", Description: "Document a workflow with a Mermaid diagram. The platform stores the source verbatim.", SubmitLabel: "Create",
	}
	if f.Edit {
		p.Action = procURL(f.ProcessID) + "/edit"
		p.Title, p.Description, p.SubmitLabel, p.RequireChange = "Edit process", f.Subtitle, "Save", true
		p.Hidden = []kit.HiddenField{drawerField(procDrawerSig)}
	}
	return p
}

func submittedProcForm(r *http.Request, edit bool) procForm {
	return procForm{
		Edit: edit, Code: strings.TrimSpace(r.PostFormValue("code")), Name: r.PostFormValue("name"),
		Description: r.PostFormValue("description"), Tags: r.PostFormValue("tags"), Body: r.PostFormValue("body"),
	}
}

// procTags reads the SPA's comma-separated tags: trimmed, blanks dropped.
// The result is never nil, so an edit that empties the field clears the
// tags (the SPA sends []).
func procTags(raw string) []string {
	out := []string{}
	for t := range strings.SplitSeq(raw, ",") {
		if t = strings.TrimSpace(t); t != "" {
			out = append(out, t)
		}
	}
	return out
}

func (h *Processes) createForm(w http.ResponseWriter, r *http.Request) {
	ac := auth.FromContext(r.Context())
	if err := auth.CanCreateProcesses(ac); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	h.showForm(w, r, ac, procForm{Open: true})
}

func (h *Processes) create(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	f := submittedProcForm(r, false)
	if err := auth.CanCreateProcesses(ac); err != nil {
		f.Errs.apply(r, err, nil)
		h.formFailed(w, r, ac, f)
		return
	}
	// The SPA's check before it posts ("Code and name are required.").
	if f.Code == "" {
		f.Errs.field("code", "Code is required")
	}
	if strings.TrimSpace(f.Name) == "" {
		f.Errs.field("name", "Name is required")
	}
	if f.Errs.any() {
		h.formFailed(w, r, ac, f)
		return
	}
	// The SPA's POST body: code and name trimmed, the description only when
	// set, the source verbatim, the tags.
	newID, err := h.Ops.Create(ctx, procops.CreateCommand{
		Code: f.Code, Name: strings.TrimSpace(f.Name), Description: optionalText(strings.TrimSpace(f.Description)),
		Body: f.Body, Tags: procTags(f.Tags),
	})
	if err != nil {
		f.Errs.apply(r, err, procFieldCodes)
		h.formFailed(w, r, ac, f)
		return
	}
	if !ds.IsDatastar(r) {
		h.Shell.SetFlash(w, flashProcCreated)
		http.Redirect(w, r, procsPath, http.StatusSeeOther)
		return
	}
	sse := ds.NewSSE(w, r)
	if err := sse.CloseDialog(procFormDialog); err != nil {
		slog.DebugContext(ctx, "ui: process form close not delivered", "err", err)
	}
	h.afterChange(sse, r, ac, newID, false)
	// The SPA opens the new process's drawer.
	if p, err := h.loadProcess(ctx, ac, newID); err != nil {
		h.Shell.FailSSE(sse, r, err)
	} else if err := openProcDrawer(sse, procDetailOf(ac, p)); err != nil {
		slog.DebugContext(ctx, "ui: process drawer patch not delivered", "err", err)
	}
	if err := flashKey(sse, flashProcCreated); err != nil {
		slog.DebugContext(ctx, "ui: process flash not delivered", "err", err)
	}
}

func (h *Processes) editForm(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	if err := auth.CanUpdateProcesses(ac); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	id, err := procID(r)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	p, err := h.loadProcess(ctx, ac, id)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	h.showForm(w, r, ac, procForm{
		Edit: true, ProcessID: p.ID, Subtitle: p.Name + " (" + p.Code + ")", Open: true,
		Name: p.Name, Description: deref(p.Description), Tags: strings.Join(p.Tags, ", "), Body: p.Body,
	})
}

func (h *Processes) update(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	f := submittedProcForm(r, true)
	if err := auth.CanUpdateProcesses(ac); err != nil {
		f.Errs.apply(r, err, nil)
		h.formFailed(w, r, ac, f)
		return
	}
	id, err := procID(r)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	p, err := h.loadProcess(ctx, ac, id)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	f.ProcessID, f.Subtitle = p.ID, p.Name+" ("+p.Code+")"
	// The SPA's PUT body: name and description trimmed (a blank description
	// is sent as "" and clears it), the source verbatim, the tags.
	name, desc := strings.TrimSpace(f.Name), strings.TrimSpace(f.Description)
	if err := h.Ops.Update(ctx, procops.UpdateCommand{ID: id, Name: &name, Description: &desc, Body: &f.Body, Tags: procTags(f.Tags)}); err != nil {
		f.Errs.apply(r, err, procFieldCodes)
		h.formFailed(w, r, ac, f)
		return
	}
	if !ds.IsDatastar(r) {
		h.Shell.SetFlash(w, flashProcUpdated)
		http.Redirect(w, r, procsPath, http.StatusSeeOther)
		return
	}
	sse := ds.NewSSE(w, r)
	if err := sse.CloseDialog(procFormDialog); err != nil {
		slog.DebugContext(ctx, "ui: process form close not delivered", "err", err)
	}
	h.afterChange(sse, r, ac, id, false)
	if err := flashKey(sse, flashProcUpdated); err != nil {
		slog.DebugContext(ctx, "ui: process flash not delivered", "err", err)
	}
}

func (h *Processes) showForm(w http.ResponseWriter, r *http.Request, ac *auth.AuthContext, f procForm) {
	if ds.IsDatastar(r) {
		if err := patchDialogSlot(ds.NewSSE(w, r), procDialogSlot, procFormDialogC(f)); err != nil {
			slog.DebugContext(r.Context(), "ui: process form patch not delivered", "err", err)
		}
		return
	}
	view, err := h.listView(r.Context(), ac, parseListQuery(nil, procList))
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	view.Dialog = procFormDialogC(f)
	h.renderPage(w, r, http.StatusOK, view)
}

func (h *Processes) formFailed(w http.ResponseWriter, r *http.Request, ac *auth.AuthContext, f procForm) {
	if ds.IsDatastar(r) {
		patchFormFailure(ds.NewSSE(w, r), r, procFormDialog, f.fields(), f.Errs.Form, f.firstInvalid())
		return
	}
	view, err := h.listView(r.Context(), ac, parseListQuery(nil, procList))
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	f.Open = true
	view.Dialog = procFormDialogC(f)
	h.renderPage(w, r, cmpOr(f.Errs.Status, http.StatusUnprocessableEntity), view)
}

func (f procForm) fieldProps(key, label, help string, required bool) kit.FieldProps {
	return kit.FieldProps{Label: label, For: f.id(key), Required: required, Description: help, Error: f.fieldError(key), Attrs: templ.Attributes{"id": f.id(key) + "-field"}}
}

func (f procForm) describedBy(key string) string {
	if f.fieldError(key) != "" {
		return f.id(key) + "-err"
	}
	return ""
}

func (f procForm) inputProps(key, value, placeholder string, attrs templ.Attributes) kit.InputProps {
	return kit.InputProps{ID: f.id(key), Name: key, Value: value, Placeholder: placeholder, Invalid: f.fieldError(key) != "", DescribedBy: f.describedBy(key), Attrs: attrs}
}

func procNameAttrs(edit bool) templ.Attributes {
	a := templ.Attributes{"required": true, "autocomplete": "off"}
	if edit {
		a["autofocus"] = true
	}
	return a
}
