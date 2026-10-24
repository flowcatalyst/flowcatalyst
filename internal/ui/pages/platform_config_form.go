package pages

import (
	"context"
	"errors"
	"log/slog"
	"maps"
	"net/http"
	"strings"

	"github.com/a-h/templ"

	"github.com/flowcatalyst/flowcatalyst-go/internal/ids"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/platformconfig"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/platformconfig/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/ds"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/kit"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecase"
)

const (
	cfgFormDialog = "cfg-form"
	cfgFormSignal = "cfgForm"
	// cfgSecretSig is the signal the type toggle sets: it picks which value
	// control is shown.
	cfgSecretSig = "cfgSecret"
	cfgSecretPw  = "cfgSecretPw"
)

var cfgFieldCodes = map[string]string{
	"INVALID_VALUE_TYPE": "valueType",
}

// cfgForm is the add or edit form (the SPA's dialog). It never holds a
// secret: the secret is read from the post when it is saved, and the form it
// answers a failed save with has no value to echo.
type cfgForm struct {
	App  string
	Edit bool

	Section, Property, Client string
	// Secret is the chosen type (PLAIN when false).
	Secret      bool
	PlainValue  string
	Description string

	Errs formErrors
	Open bool
}

func (f cfgForm) id(key string) string         { return fieldID(cfgFormDialog, key) }
func (f cfgForm) fieldError(key string) string { return f.Errs.Fields[key] }

func (f cfgForm) keys() []string {
	return []string{"section", "property", "client", "valueType", "value", "description"}
}

func (f cfgForm) firstInvalid() string {
	for _, k := range f.keys() {
		if f.fieldError(k) != "" {
			if k == "value" && f.Secret {
				return f.id("secretValue")
			}
			return f.id(k)
		}
	}
	return ""
}

func (f cfgForm) fields() []templ.Component {
	out := make([]templ.Component, 0, 6)
	for _, k := range f.keys() {
		out = append(out, cfgField(f, k))
	}
	return out
}

func (f cfgForm) dialog() kit.FormDialogProps {
	p := kit.FormDialogProps{
		ID: cfgFormDialog, Signal: cfgFormSignal, Action: cfgPath + "/save", Open: f.Open, Error: f.Errs.Form,
		Title: "Add property", SubmitLabel: "Save",
		Hidden: []kit.HiddenField{{Name: "app", Value: f.App}},
	}
	if f.Edit {
		p.Title, p.Description, p.RequireChange = "Edit property", f.App, true
		p.Hidden = append(p.Hidden, kit.HiddenField{Name: "edit", Value: "1"})
	}
	return p
}

// signals declares the type toggle's signal, true when the form starts as a
// secret.
func (f cfgForm) signals() string {
	if f.Secret {
		return "{" + cfgSecretSig + ": true}"
	}
	return "{" + cfgSecretSig + ": false}"
}

func submittedCfgForm(r *http.Request) cfgForm {
	return cfgForm{
		App:         strings.TrimSpace(r.PostFormValue("app")),
		Edit:        r.PostFormValue("edit") != "",
		Section:     strings.TrimSpace(r.PostFormValue("section")),
		Property:    strings.TrimSpace(r.PostFormValue("property")),
		Client:      strings.TrimSpace(r.PostFormValue("client")),
		Secret:      r.PostFormValue("valueType") == string(platformconfig.ValueSecret),
		PlainValue:  r.PostFormValue("plainValue"),
		Description: strings.TrimSpace(r.PostFormValue("description")),
	}
}

// ── add and edit ───────────────────────────────────────────────────────

func (h *PlatformConfig) createForm(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	app := strings.TrimSpace(r.URL.Query().Get("app"))
	if err := h.Config.AuthorizeWrite(ctx, ac, app); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	h.showForm(w, r, ac, cfgForm{App: app, Open: true})
}

func (h *PlatformConfig) editForm(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	q := r.URL.Query()
	app := strings.TrimSpace(q.Get("app"))
	// The SPA edits the row it listed: view to see it, update to save it.
	if err := h.Config.AuthorizeWrite(ctx, ac, app); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	cid, err := h.Config.ResolveClientID(ctx, q.Get("client"))
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	rows, err := h.Config.List(ctx, ac, app)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	var c *platformconfig.Config
	for i := range rows {
		if rows[i].Section == q.Get("section") && rows[i].Property == q.Get("property") && deref(ids.StringPtr(rows[i].ClientID)) == cid {
			c = &rows[i]
		}
	}
	if c == nil {
		h.Shell.Fail(w, r, httperror.NotFound("Config", app+"/"+q.Get("section")+"/"+q.Get("property")))
		return
	}
	f := cfgForm{
		App: app, Edit: true, Open: true, Section: c.Section, Property: c.Property, Client: cid,
		Secret: c.ValueType == platformconfig.ValueSecret, Description: deref(c.Description),
	}
	if ident, ok := h.clientName(ctx, cid); ok {
		f.Client = ident
	}
	// A secret's value is write-only: never prefilled.
	if !f.Secret {
		f.PlainValue = c.Value
	}
	h.showForm(w, r, ac, f)
}

func (h *PlatformConfig) clientName(ctx context.Context, clientID string) (string, bool) {
	if clientID == "" || h.ClientIdentifier == nil {
		return "", false
	}
	return h.ClientIdentifier(ctx, clientID)
}

func (h *PlatformConfig) save(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	f := submittedCfgForm(r)
	// PUT /api/config/...: the confinement and the update gate come first, so
	// an unauthorised caller learns nothing about clients; the use case
	// decides for callers below anchor tier by their access grants.
	if err := h.Config.AuthorizeUpdate(ctx, ac, f.App); err != nil {
		f.Errs.apply(r, err, nil)
		h.formFailed(w, r, ac, f)
		return
	}
	// The SPA's checks: section and property, then a value.
	if f.Section == "" {
		f.Errs.field("section", "Section is required.")
	}
	if f.Property == "" {
		f.Errs.field("property", "Property is required.")
	}
	value, valueType := f.PlainValue, platformconfig.ValuePlain
	if f.Secret {
		value, valueType = r.PostFormValue("secretValue"), platformconfig.ValueSecret
	}
	if value == "" {
		if f.Secret {
			f.Errs.field("value", "Enter the secret value.")
		} else {
			f.Errs.field("value", "A value is required.")
		}
	}
	if f.Errs.any() {
		h.formFailed(w, r, ac, f)
		return
	}
	cid, err := h.Config.ResolveClientID(ctx, f.Client)
	if err != nil {
		if ue, ok := errors.AsType[*usecase.Error](err); ok && ue.HTTPStatus() == http.StatusNotFound {
			f.Errs.field("client", noSuchClient)
		} else {
			f.Errs.apply(r, err, nil)
		}
		h.formFailed(w, r, ac, f)
		return
	}
	vt := string(valueType)
	cmd := operations.SetPropertyCommand{
		ApplicationCode: f.App, Section: f.Section, Property: f.Property,
		Value: value, ValueType: &vt, Description: optionalText(f.Description),
	}
	if cid != "" {
		cmd.ClientID = &cid
	}
	if err := h.Ops.Set(ctx, cmd); err != nil {
		f.Errs.apply(r, err, cfgFieldCodes)
		h.formFailed(w, r, ac, f)
		return
	}
	h.changed(w, r, ac, f.App, flashCfgSaved, true)
}

// ── rendering ──────────────────────────────────────────────────────────

func (h *PlatformConfig) showForm(w http.ResponseWriter, r *http.Request, ac *auth.AuthContext, f cfgForm) {
	if ds.IsDatastar(r) {
		if err := patchDialogSlot(ds.NewSSE(w, r), cfgDialogSlot, cfgFormDialogC(f)); err != nil {
			slog.DebugContext(r.Context(), "ui: configuration form patch not delivered", "err", err)
		}
		return
	}
	view, err := h.listView(r.Context(), ac, parseListQuery(nil, cfgList), f.App)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	view.Dialog = cfgFormDialogC(f)
	h.renderPage(w, r, http.StatusOK, view)
}

func (h *PlatformConfig) formFailed(w http.ResponseWriter, r *http.Request, ac *auth.AuthContext, f cfgForm) {
	if ds.IsDatastar(r) {
		patchFormFailure(ds.NewSSE(w, r), r, cfgFormDialog, f.fields(), f.Errs.Form, f.firstInvalid())
		return
	}
	view, err := h.listView(r.Context(), ac, parseListQuery(nil, cfgList), f.App)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	f.Open = true
	view.Dialog = cfgFormDialogC(f)
	status := f.Errs.Status
	if status == 0 {
		status = http.StatusUnprocessableEntity
	}
	h.renderPage(w, r, status, view)
}

func (f cfgForm) fieldProps(key, label, help string, required bool) kit.FieldProps {
	return kit.FieldProps{Label: label, For: f.id(key), Required: required, Description: help, Error: f.fieldError(key), Attrs: templ.Attributes{"id": f.id(key) + "-field"}}
}

func (f cfgForm) describedBy(key string) string {
	if f.fieldError(key) != "" {
		return f.id(key) + "-err"
	}
	return ""
}

// inputProps is a text input of the form. In an edit the coordinates are
// read-only (they are what the property is), still posted with the form.
func (f cfgForm) inputProps(key, value string, p kit.InputProps) kit.InputProps {
	p.ID, p.Name, p.Value = f.id(key), key, value
	p.Invalid = f.fieldError(key) != ""
	p.DescribedBy = f.describedBy(key)
	return p
}

func (f cfgForm) coordinateAttrs(extra templ.Attributes) templ.Attributes {
	a := templ.Attributes{"autocomplete": "off", "spellcheck": "false"}
	maps.Copy(a, extra)
	if f.Edit {
		a["readonly"] = true
	}
	return a
}

func (f cfgForm) valueType() string {
	if f.Secret {
		return string(platformconfig.ValueSecret)
	}
	return string(platformconfig.ValuePlain)
}

var cfgTypeOptions = []kit.Option{
	{Value: string(platformconfig.ValuePlain), Label: "Plain text"},
	{Value: string(platformconfig.ValueSecret), Label: "Secret"},
}

func (f cfgForm) secretPlaceholder() string {
	if f.Edit {
		return "Enter a new value to replace the secret"
	}
	return ""
}
