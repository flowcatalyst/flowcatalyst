package pages

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"github.com/a-h/templ"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/eventtype"
	etops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/eventtype/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/ds"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/kit"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/shell"
)

// The create and edit dialogs (the SPA's EventTypeCreateDrawer and the
// detail drawer's inline edit) and the add-schema page
// (EventTypeAddSchemaPage).

const (
	etFormDialog   = "et-form"
	etFormSignal   = "etForm"
	etSchemaFormID = "et-schema"
)

// etCodePattern is the SPA's CODE_PATTERN for each code segment, as an
// HTML pattern (the v-flag syntax browsers use needs the hyphen escaped).
const etCodePattern = `[a-z][a-z0-9.\-]*`

var etFieldCodes = map[string]string{
	"CODE_REQUIRED":       "code",
	"INVALID_CODE_FORMAT": "code",
	"CODE_EXISTS":         "code",
	"NAME_REQUIRED":       "name",
}

type etForm struct {
	Edit     bool
	ETID     string
	Subtitle string

	App, Subdomain, Aggregate, Event string
	Name, Description                string
	ClientScoped                     bool

	Errs formErrors
	Open bool

	// AppCodes are the active applications the application field suggests
	// (empty when the user may not list applications).
	AppCodes []kit.Option
}

func (f etForm) id(key string) string         { return fieldID(etFormDialog, key) }
func (f etForm) fieldError(key string) string { return f.Errs.Fields[key] }

func (f etForm) keys() []string {
	if f.Edit {
		return []string{"name", "description"}
	}
	return []string{"code", "name", "description", "clientScoped"}
}

// focusID is the control a field's error sends focus to (the code group's
// first input).
func (f etForm) focusID(key string) string {
	if key == "code" {
		return f.id("application")
	}
	return f.id(key)
}

func (f etForm) firstInvalid() string {
	for _, k := range f.keys() {
		if f.fieldError(k) != "" {
			return f.focusID(k)
		}
	}
	return ""
}

func (f etForm) fields() []templ.Component {
	out := make([]templ.Component, 0, 4)
	for _, k := range f.keys() {
		out = append(out, etField(f, k))
	}
	return out
}

func (f etForm) dialog() kit.FormDialogProps {
	p := kit.FormDialogProps{
		ID: etFormDialog, Signal: etFormSignal, Width: kit.WidthWide, Action: etPath + "/new", Open: f.Open, Error: f.Errs.Form,
		Title: "Create Event Type", Description: "Define a new event type with its code and metadata", SubmitLabel: "Create Event Type",
	}
	if f.Edit {
		p.Action = etURL(f.ETID) + "/edit"
		p.Title, p.Description, p.SubmitLabel, p.RequireChange = "Edit Event Type", f.Subtitle, "Save", true
		p.Hidden = []kit.HiddenField{drawerField(etDrawerSig)}
	}
	return p
}

func (f etForm) code() string {
	return f.App + ":" + f.Subdomain + ":" + f.Aggregate + ":" + f.Event
}

func submittedETForm(r *http.Request, edit bool) etForm {
	return etForm{
		Edit:         edit,
		App:          strings.TrimSpace(r.PostFormValue("application")),
		Subdomain:    strings.TrimSpace(r.PostFormValue("subdomain")),
		Aggregate:    strings.TrimSpace(r.PostFormValue("aggregate")),
		Event:        strings.TrimSpace(r.PostFormValue("event")),
		Name:         r.PostFormValue("name"),
		Description:  r.PostFormValue("description"),
		ClientScoped: r.PostFormValue("clientScoped") != "",
	}
}

// ── create ─────────────────────────────────────────────────────────────

func (h *EventTypes) createForm(w http.ResponseWriter, r *http.Request) {
	ac := auth.FromContext(r.Context())
	if err := auth.CanCreateEventTypes(ac); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	h.showForm(w, r, ac, etForm{Open: true})
}

func (h *EventTypes) create(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	f := submittedETForm(r, false)
	if err := auth.CanCreateEventTypes(ac); err != nil {
		f.Errs.apply(r, err, nil)
		h.formFailed(w, r, ac, f)
		return
	}
	// The SPA's POST body: the joined code, name, description when set, the
	// client-scoped flag; no client (anchor-level) and no schema.
	newID, err := h.Ops.Create(ctx, etops.CreateCommand{
		Code:         f.code(),
		Name:         f.Name,
		Description:  optionalText(f.Description),
		ClientScoped: f.ClientScoped,
	})
	if err != nil {
		f.Errs.apply(r, err, etFieldCodes)
		h.formFailed(w, r, ac, f)
		return
	}
	if !ds.IsDatastar(r) {
		h.Shell.SetFlash(w, flashETCreated)
		http.Redirect(w, r, etURL(newID), http.StatusSeeOther)
		return
	}
	// As the SPA: close the form and open the new event type's drawer.
	sse := ds.NewSSE(w, r)
	if err := sse.CloseDialog(etFormDialog); err != nil {
		slog.DebugContext(ctx, "ui: event type form close not delivered", "err", err)
	}
	h.afterChange(sse, r, ac, newID, false)
	if et, err := h.loadEventType(ctx, ac, newID); err != nil {
		h.Shell.FailSSE(sse, r, err)
	} else if err := openETDrawer(sse, toETDetail(et, etPermsFor(ac))); err != nil {
		slog.DebugContext(ctx, "ui: event type drawer patch not delivered", "err", err)
	}
	if err := flashKey(sse, flashETCreated); err != nil {
		slog.DebugContext(ctx, "ui: event type flash not delivered", "err", err)
	}
}

// ── edit ───────────────────────────────────────────────────────────────

func (h *EventTypes) editForm(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	// The SPA loads the event type and saves it with PUT /bff/event-types/{id}.
	if err := firstError(auth.CanReadEventTypes(ac), auth.CanUpdateEventTypes(ac)); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	id, err := etID(r)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	et, err := h.loadEventType(ctx, ac, id)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	h.showForm(w, r, ac, etForm{Edit: true, ETID: et.ID, Subtitle: et.Code, Name: et.Name, Description: deref(et.Description), Open: true})
}

func (h *EventTypes) update(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	f := submittedETForm(r, true)
	if err := firstError(auth.CanReadEventTypes(ac), auth.CanUpdateEventTypes(ac)); err != nil {
		f.Errs.apply(r, err, nil)
		h.formFailed(w, r, ac, f)
		return
	}
	id, err := etID(r)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	et, err := h.loadEventType(ctx, ac, id)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	f.ETID, f.Subtitle = et.ID, et.Code
	// The SPA's body: name and description as typed (a blank description is
	// sent as "", which clears it).
	desc := f.Description
	if err := h.Ops.Update(ctx, etops.UpdateCommand{ID: id, Name: f.Name, Description: &desc}); err != nil {
		f.Errs.apply(r, err, etFieldCodes)
		h.formFailed(w, r, ac, f)
		return
	}
	if !ds.IsDatastar(r) {
		h.Shell.SetFlash(w, flashETUpdated)
		http.Redirect(w, r, etURL(id), http.StatusSeeOther) //nolint:gosec // G710: a local path built from a validated id (kit.ValidID)
		return
	}
	sse := ds.NewSSE(w, r)
	if err := sse.CloseDialog(etFormDialog); err != nil {
		slog.DebugContext(ctx, "ui: event type form close not delivered", "err", err)
	}
	h.afterChange(sse, r, ac, id, false)
	if err := flashKey(sse, flashETUpdated); err != nil {
		slog.DebugContext(ctx, "ui: event type flash not delivered", "err", err)
	}
}

// ── rendering ──────────────────────────────────────────────────────────

// withChoices fills the application suggestions (active applications, when
// the user may list them, as the SPA's call would succeed).
func (h *EventTypes) withChoices(r *http.Request, ac *auth.AuthContext, f *etForm) {
	if f.Edit || h.Apps == nil || auth.CanReadApplications(ac) != nil {
		return
	}
	apps, err := h.Apps.FindActive(r.Context())
	if err != nil {
		slog.WarnContext(r.Context(), "ui: applications unavailable", "err", err)
		return
	}
	codes := make([]string, 0, len(apps))
	for _, a := range apps {
		codes = append(codes, a.Code)
	}
	slices.Sort(codes)
	for _, c := range slices.Compact(codes) {
		f.AppCodes = append(f.AppCodes, kit.Option{Value: c})
	}
}

func (h *EventTypes) showForm(w http.ResponseWriter, r *http.Request, ac *auth.AuthContext, f etForm) {
	h.withChoices(r, ac, &f)
	if ds.IsDatastar(r) {
		if err := ds.NewSSE(w, r).Patch(etFormDialogC(f), ds.Inner("#"+etDialogSlot)); err != nil {
			slog.DebugContext(r.Context(), "ui: event type form patch not delivered", "err", err)
		}
		return
	}
	view, err := h.listView(r.Context(), ac, parseListQuery(nil, etList))
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	view.Dialog = etFormDialogC(f)
	h.renderPage(w, r, http.StatusOK, view)
}

func (h *EventTypes) formFailed(w http.ResponseWriter, r *http.Request, ac *auth.AuthContext, f etForm) {
	h.withChoices(r, ac, &f)
	if ds.IsDatastar(r) {
		patchFormFailure(ds.NewSSE(w, r), r, etFormDialog, f.fields(), f.Errs.Form, f.firstInvalid())
		return
	}
	view, err := h.listView(r.Context(), ac, parseListQuery(nil, etList))
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	f.Open = true
	view.Dialog = etFormDialogC(f)
	h.renderPage(w, r, cmpOr(f.Errs.Status, http.StatusUnprocessableEntity), view)
}

func cmpOr(v, def int) int {
	if v == 0 {
		return def
	}
	return v
}

func (f etForm) fieldProps(key, label, help string, required bool) kit.FieldProps {
	return kit.FieldProps{Label: label, For: f.focusID(key), Required: required, Description: help, Error: f.fieldError(key), Attrs: templ.Attributes{"id": f.id(key) + "-field"}}
}

func (f etForm) describedBy(key string) string {
	if f.fieldError(key) != "" {
		return f.focusID(key) + "-err"
	}
	return ""
}

// segmentInput is one of the code builder's four inputs, bound to a signal
// for the live preview.
func (f etForm) segmentInput(key, value, placeholder, signal string) kit.InputProps {
	return kit.InputProps{
		ID: f.id(key), Name: key, Value: value, Placeholder: placeholder, Required: true,
		Invalid: f.fieldError("code") != "", DescribedBy: f.describedBy("code"),
		Attrs: templ.Attributes{
			"pattern": etCodePattern, "title": "Lowercase letters, numbers, hyphens, dots only",
			"autocomplete": "off", "data-bind:" + kit.SignalAttr(signal): true,
		},
	}
}

func (f etForm) signals() string {
	q := func(s string) string { b, _ := json.Marshal(s); return string(b) }
	return "{etApp: " + q(f.App) + ", etSub: " + q(f.Subdomain) + ", etAgg: " + q(f.Aggregate) + ", etEvent: " + q(f.Event) + ", etName: " + q(f.Name) + ", etDesc: " + q(f.Description) + "}"
}

var etPreviewAttrs = []templ.Attributes{
	{"data-text": "$etApp || 'app'"},
	{"data-text": "$etSub || 'subdomain'"},
	{"data-text": "$etAgg || 'aggregate'"},
	{"data-text": "$etEvent || 'event'"},
}

func (f etForm) previewCode() string {
	or := func(v, def string) string {
		if v == "" {
			return def
		}
		return v
	}
	return or(f.App, "app") + ":" + or(f.Subdomain, "subdomain") + ":" + or(f.Aggregate, "aggregate") + ":" + or(f.Event, "event")
}

// ── add schema ─────────────────────────────────────────────────────────

type etSchemaForm struct {
	ETID, ETName string
	Version      string
	MimeType     string
	SchemaType   string
	Schema       string
	Errs         formErrors
}

// etSchemaTypes are the SPA's schema type choices.
var etSchemaTypes = []kit.Option{
	{Value: "JSON_SCHEMA", Label: "JSON Schema"},
	{Value: "PROTO", Label: "Protocol Buffers"},
	{Value: "XSD", Label: "XML Schema (XSD)"},
}

func (f etSchemaForm) id(key string) string { return fieldID(etSchemaFormID, key) }

func (f etSchemaForm) fieldProps(key, label string) kit.FieldProps {
	return kit.FieldProps{Label: label, For: f.id(key), Required: true, Error: f.Errs.Fields[key], Attrs: templ.Attributes{"id": f.id(key) + "-field"}}
}

func (f etSchemaForm) describedBy(key string) string {
	if f.Errs.Fields[key] != "" {
		return f.id(key) + "-err"
	}
	return ""
}

func (f etSchemaForm) action() string { return etURL(f.ETID) + "/add-schema" }

// etSchemaFields are the add-schema form's fields, in order.
var etSchemaFields = []string{"version", "mimeType", "schemaType", "schema"}

var etSchemaFieldCodes = map[string]string{
	"VERSION_REQUIRED":    "version",
	"VERSION_EXISTS":      "version",
	"SCHEMA_REQUIRED":     "schema",
	"INVALID_SCHEMA":      "schema",
	"INVALID_SCHEMA_TYPE": "schemaType",
	"INVALID_MIME_TYPE":   "mimeType",
}

// suggestNextVersion is the SPA's: 1.0 for the first, else the highest
// major's next minor.
func suggestNextVersion(et *eventtype.EventType) string {
	if len(et.SpecVersions) == 0 {
		return "1.0"
	}
	major, minor := -1, 0
	for _, sv := range et.SpecVersions {
		maj, mn := splitVersion(sv.Version)
		if maj > major || (maj == major && mn > minor) {
			major, minor = maj, mn
		}
	}
	return strconv.Itoa(major) + "." + strconv.Itoa(minor+1)
}

func splitVersion(v string) (int, int) {
	a, b, _ := strings.Cut(v, ".")
	maj, _ := strconv.Atoi(a)
	b, _, _ = strings.Cut(b, ".")
	mn, _ := strconv.Atoi(b)
	return maj, mn
}

func (h *EventTypes) addSchemaTarget(w http.ResponseWriter, r *http.Request) (*eventtype.EventType, bool) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	// POST /bff/event-types/{id}/schemas's check (and the read to load it).
	if err := firstError(auth.CanReadEventTypes(ac), auth.CanUpdateEventTypes(ac)); err != nil {
		h.Shell.Fail(w, r, err)
		return nil, false
	}
	id, err := etID(r)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return nil, false
	}
	et, err := h.loadEventType(ctx, ac, id)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return nil, false
	}
	return et, true
}

func (h *EventTypes) addSchemaPage(w http.ResponseWriter, r *http.Request) {
	et, ok := h.addSchemaTarget(w, r)
	if !ok {
		return
	}
	f := etSchemaForm{ETID: et.ID, ETName: et.Name, Version: suggestNextVersion(et), MimeType: "application/json", SchemaType: "JSON_SCHEMA"}
	h.Shell.Render(w, r, shell.Page{Title: "Add Schema Version", Path: etPath}, etSchemaPage(f))
}

// addSchema adds a schema version with the SPA's fields: version, MIME type,
// schema type and the schema. A JSON Schema must be a JSON document and is
// sent as one; an XSD or PROTO schema is text, sent as a JSON string (the use
// case stores each that way, with the MIME type and schema type given).
func (h *EventTypes) addSchema(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	et, ok := h.addSchemaTarget(w, r)
	if !ok {
		return
	}
	f := etSchemaForm{
		ETID: et.ID, ETName: et.Name,
		Version:    strings.TrimSpace(r.PostFormValue("version")),
		MimeType:   strings.TrimSpace(r.PostFormValue("mimeType")),
		SchemaType: cmpOrString(strings.TrimSpace(r.PostFormValue("schemaType")), "JSON_SCHEMA"),
		Schema:     r.PostFormValue("schema"),
	}
	var raw json.RawMessage
	switch {
	case strings.TrimSpace(f.Schema) == "":
		f.Errs.field("schema", "Enter the schema.")
	case f.SchemaType != "JSON_SCHEMA":
		raw, _ = json.Marshal(f.Schema)
	case !json.Valid([]byte(f.Schema)):
		f.Errs.field("schema", "Enter a valid JSON document.")
	default:
		raw = json.RawMessage(f.Schema)
	}
	if f.MimeType == "" {
		f.Errs.field("mimeType", "Enter the MIME type.")
	}
	if !f.Errs.any() {
		cmd := etops.AddSchemaCommand{EventTypeID: et.ID, Version: f.Version, Schema: raw, MimeType: f.MimeType, SchemaType: f.SchemaType}
		if err := h.Ops.AddSchema(ctx, cmd); err != nil {
			f.Errs.apply(r, err, etSchemaFieldCodes)
		}
	}
	if f.Errs.any() {
		h.schemaFailed(w, r, f)
		return
	}
	h.Shell.SetFlash(w, flashSchemaAdded)
	if !ds.IsDatastar(r) {
		http.Redirect(w, r, etURL(et.ID), http.StatusSeeOther)
		return
	}
	if err := ds.NewSSE(w, r).Redirect(etURL(et.ID)); err != nil {
		slog.DebugContext(ctx, "ui: add schema redirect not delivered", "err", err)
	}
}

func (h *EventTypes) schemaFailed(w http.ResponseWriter, r *http.Request, f etSchemaForm) {
	if ds.IsDatastar(r) {
		focus := ""
		for _, k := range etSchemaFields {
			if f.Errs.Fields[k] != "" {
				focus = f.id(k)
				break
			}
		}
		fields := make([]templ.Component, 0, len(etSchemaFields))
		for _, k := range etSchemaFields {
			fields = append(fields, etSchemaField(f, k))
		}
		patchFormFailure(ds.NewSSE(w, r), r, etSchemaFormID, fields, f.Errs.Form, focus)
		return
	}
	h.Shell.RenderStatus(w, r, cmpOr(f.Errs.Status, http.StatusUnprocessableEntity), shell.Page{Title: "Add Schema Version", Path: etPath}, etSchemaPage(f))
}

func etNameAttrs(edit bool) templ.Attributes {
	a := templ.Attributes{"maxlength": "100", "autocomplete": "off", "data-bind:et-name": true}
	if edit {
		a["autofocus"] = true
	}
	return a
}

// etCount is a character counter's text before Datastar runs ("12 / 100").
func etCount(s string, limit int) string {
	return strconv.Itoa(len([]rune(s))) + " / " + strconv.Itoa(limit)
}
