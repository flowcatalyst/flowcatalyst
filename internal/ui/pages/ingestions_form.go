package pages

import (
	"cmp"
	"encoding/json"
	"errors"
	"log/slog"
	"maps"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"github.com/a-h/templ"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/ingestion"
	ingops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/ingestion/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/ds"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/kit"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecase"
)

// The create and change dialog (the Nuxt control plane's IngestionFormPage):
// the form is drawn from the source kinds (ingestions_kinds.go), so it is one
// dialog whatever the kind. A change is a new saved version (POST
// /api/ingestions/{id}/versions): the live one keeps running until it is made
// live, here with the switch or from the Versions tab.

const (
	ingsFormDialog = "ings-form"
	ingsFormSignal = "ingsForm"
	ingsBodyID     = "ings-form-body"
	ingsKindSig    = "ingKind"
)

// ingsIdentityKeys are the form's own controls, ahead of the definition's.
var ingsIdentityKeys = []string{"application", "client", "code", "name", "kind"}

// ingsForm is the create or change form: what is rendered, or what was
// submitted with its errors.
type ingsForm struct {
	Edit bool
	// ID, Code and From are the ingestion being changed and the version the
	// form started from (0: the live one, else the newest).
	ID, Code string
	From     int
	Latest   int
	Live     int
	// Values are the controls' values by name; Cat the kinds the form draws.
	Values map[string]string
	Cat    ingsCatalogue
	Errs   formErrors
	Open   bool

	// The create form's choices.
	Apps          []kit.Option
	CanPickApp    bool
	ClientOptions []kit.Option
	CanPickClient bool
	// KindOptions are the kinds the caller may choose.
	KindOptions []kit.Option
	// CanPromote: the caller may make a version live (the switch is offered
	// on a change).
	CanPromote bool
}

func (f ingsForm) id(key string) string {
	return fieldID(ingsFormDialog, strings.ReplaceAll(key, ".", "-"))
}
func (f ingsForm) fieldError(key string) string { return f.Errs.Fields[key] }

func (f ingsForm) describedBy(key string) string {
	if f.fieldError(key) != "" {
		return f.id(key) + "-err"
	}
	return ""
}

func (f ingsForm) kind() *ingsKind { return f.Cat.find(f.Values["kind"]) }

// order is the controls in the order they are shown, for focus on the first
// invalid one.
func (f ingsForm) order() []string {
	keys := slices.Clone(ingsIdentityKeys)
	if k := f.kind(); k != nil {
		for _, fld := range k.Fields {
			keys = append(keys, fld.Name)
		}
	}
	for _, fld := range ingsSplitFields {
		keys = append(keys, fld.Name)
	}
	for _, fld := range ingsTargetFields {
		keys = append(keys, fld.Name)
	}
	return append(keys, "note")
}

func (f ingsForm) firstInvalid() string {
	for _, k := range f.order() {
		if f.fieldError(k) != "" {
			return f.id(k)
		}
	}
	return ""
}

func (f ingsForm) action() string {
	if f.Edit {
		return ingsURL(f.ID) + "/edit"
	}
	return ingsPath + "/new"
}

func (f ingsForm) dialogProps() kit.FormDialogProps {
	p := kit.FormDialogProps{
		ID: ingsFormDialog, Signal: ingsFormSignal, Width: kit.WidthWide, Action: f.action(), Open: f.Open, Error: f.Errs.Form,
		Title: "New ingestion", Description: "Where a client's data comes from, how it is split, and the dispatch job each record becomes", SubmitLabel: "Create ingestion",
	}
	if f.Edit {
		p.Title, p.Description, p.SubmitLabel, p.RequireChange = "Change "+f.Code, f.versionText(), "Save version", true
		p.Hidden = ingsHidden()
	}
	return p
}

// versionText says what the change will be saved as.
func (f ingsForm) versionText() string {
	text := "Saved as version " + strconv.Itoa(f.Latest+1) + "; runtimes keep running "
	if f.Live == 0 {
		text = "Saved as version " + strconv.Itoa(f.Latest+1) + "; nothing is live yet"
	} else {
		text += "version " + strconv.Itoa(f.Live) + " until a version is made live."
	}
	if f.From != 0 && f.From != f.Live {
		text = "Starting from version " + strconv.Itoa(f.From) + ". " + text
	}
	return text
}

// signals declares the signals the form's conditional controls read: the kind
// and every control other fields depend on, with the value the form starts
// with.
func (f ingsForm) signals() string {
	sig := map[string]any{ingsKindSig: f.Values["kind"]}
	add := func(fields []ingsField) {
		for _, fld := range fields {
			if !fld.control {
				continue
			}
			if fld.Type == ingsSwitch {
				sig[ingsSignal(fld.Name)] = f.Values[fld.Name] == ingsOn
			} else {
				sig[ingsSignal(fld.Name)] = f.Values[fld.Name]
			}
		}
	}
	add(ingsSplitFields)
	for _, k := range f.Cat {
		add(k.Fields)
	}
	b, _ := json.Marshal(sig)
	return string(b)
}

// whenExpr is the condition a field's When names, over the form's signals.
func (f ingsForm) whenExpr(w *ingsWhen) string {
	if w.Is == ingsOn {
		return "$" + ingsSignal(w.Field)
	}
	return "$" + ingsSignal(w.Field) + " === '" + w.Is + "'"
}

// gate makes a group of controls appear and apply together: shown (and posted)
// while show holds, hidden and disabled otherwise, so what is not chosen is
// neither posted nor checked by the browser. visible is the state it is
// rendered in.
func ingsGate(show string, visible bool) templ.Attributes {
	a := templ.Attributes{"data-show": show, "data-attr:disabled": "!(" + show + ")"}
	if !visible {
		a["style"] = "display: none"
		a["disabled"] = true
	}
	return a
}

func (f ingsForm) kindGate(k ingsKind) templ.Attributes {
	return ingsGate("$"+ingsKindSig+" === '"+k.Name+"'", f.Values["kind"] == k.Name)
}

func (f ingsForm) fieldGate(fld ingsField) templ.Attributes {
	return ingsGate(f.whenExpr(fld.When), fld.active(f.Values))
}

// splitGate hides the split for a database poll: each row is one record.
func (f ingsForm) splitGate() templ.Attributes {
	return ingsGate("$"+ingsKindSig+" !== '"+ingestion.KindDatabasePoll+"'", f.Values["kind"] != ingestion.KindDatabasePoll)
}

func (f ingsForm) controlAttrs(fld ingsField, extra templ.Attributes) templ.Attributes {
	a := templ.Attributes{}
	maps.Copy(a, extra)
	if fld.control {
		a["data-bind:"+kit.SignalAttr(ingsSignal(fld.Name))] = true
	}
	return a
}

func (f ingsForm) fieldProps(fld ingsField) kit.FieldProps {
	id := f.id(fld.Name)
	return kit.FieldProps{Label: fld.Label, For: id, Required: fld.Required, Description: fld.Help, Error: f.fieldError(fld.Name), ErrorID: id + "-err", Attrs: templ.Attributes{"id": id + "-field"}}
}

// ── building a form ────────────────────────────────────────────────────

func newIngsForm(edit bool, cat ingsCatalogue) ingsForm {
	return ingsForm{Edit: edit, Cat: cat, Values: ingsDefaults(cat)}
}

// submittedIngsForm reads a posted form. What the browser did not post (a
// disabled kind, an unticked switch) is absent; the controls of the kinds
// and branches not chosen get their defaults back, so a form shown again
// after an error is whole.
func submittedIngsForm(r *http.Request, edit bool, cat ingsCatalogue) ingsForm {
	f := newIngsForm(edit, cat)
	posted := map[string]string{}
	for k, vs := range r.PostForm {
		if len(vs) > 0 {
			posted[k] = vs[0]
		}
	}
	for _, key := range []string{"application", "client", "code", "note"} {
		posted[key] = strings.TrimSpace(posted[key])
	}
	active := map[string]bool{}
	for _, fld := range ingsBuildDefinition(posted, cat).Fields {
		active[fld.Name] = true
	}
	fields := slices.Concat(ingsSplitFields, ingsTargetFields)
	for _, k := range cat {
		fields = append(fields, k.Fields...)
	}
	for _, fld := range fields {
		if _, isPosted := posted[fld.Name]; !isPosted && !active[fld.Name] && fld.Default != "" {
			posted[fld.Name] = fld.Default
		}
	}
	f.Values = posted
	f.Open = true
	return f
}

// withChoices loads what the form's selects offer: the integration
// applications the caller reaches and the client suggestions (create), the
// kinds the caller may choose, and whether the switch for a live version is
// offered.
func (h *Ingestions) withChoices(r *http.Request, ac *auth.AuthContext, f *ingsForm, kinds ingsKindPerms) {
	ctx := r.Context()
	f.CanPromote = auth.CanPromoteIngestions(ac) == nil
	for _, k := range f.Cat {
		if k.Name == ingestion.KindCustom {
			continue
		}
		// A kind that declares a permission is offered to those who hold it,
		// and kept when it is the one an ingestion already has.
		if perm := kinds.of(k.Name); k.BuiltIn || perm == "" || ac.HasPermission(perm) || k.Name == f.Values["kind"] {
			f.KindOptions = append(f.KindOptions, kit.Option{Value: k.Name, Label: k.Label})
		}
	}
	if f.Edit {
		return
	}
	if h.Apps != nil {
		kind := ingsIntegration
		if as, err := h.Apps.FindWithFilters(ctx, &kind, nil); err != nil {
			slog.WarnContext(ctx, "ui: applications unavailable", "err", err)
		} else {
			for _, a := range as {
				if a.Active && ac.CanAccessApplication(string(a.ID)) {
					f.Apps = append(f.Apps, kit.Option{Value: a.Code, Label: a.Name + " (" + a.Code + ")"})
				}
			}
			f.CanPickApp = true
		}
	}
	f.CanPickClient = canPickClient(h.Clients, ac)
	if opts, err := clientSuggestions(ctx, h.Clients, ac, f.Values["client"]); err != nil {
		slog.WarnContext(ctx, "ui: client suggestions unavailable", "err", err)
	} else {
		f.ClientOptions = opts
	}
}

// catalogue builds the form's kinds from the reported ones; current are the
// kinds of the ingestion being changed (its live version's, and the version
// the change starts from).
func (h *Ingestions) catalogue(r *http.Request, current ...string) (ingsCatalogue, ingsKindPerms, error) {
	kinds, reported, err := h.kindPerms(r.Context())
	if err != nil {
		return nil, nil, err
	}
	return ingsNewCatalogue(reported, current...), kinds, nil
}

// ── showing and failing ────────────────────────────────────────────────

func (f ingsForm) component() templ.Component { return ingsFormDialogC(f) }

// showDialog answers a request for the form dialog: patched into the page's
// dialog slot for Datastar, or the list page (with the ingestion's drawer when
// id names one) with the dialog open.
func (h *Ingestions) showDialog(w http.ResponseWriter, r *http.Request, ac *auth.AuthContext, id string, f ingsForm, status int) {
	if ds.IsDatastar(r) {
		if err := patchDialogSlot(ds.NewSSE(w, r), ingsDialogSlot, f.component()); err != nil {
			slog.DebugContext(r.Context(), "ui: ingestion dialog patch not delivered", "err", err)
		}
		return
	}
	ctx := r.Context()
	view, err := h.listView(ctx, ac, parseIngsQuery(nil))
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	if id != "" && auth.CanReadIngestions(ac) == nil {
		i, err := h.reachable(ctx, ac, id)
		if err != nil {
			h.Shell.Fail(w, r, err)
			return
		}
		d, err := h.buildDetail(ctx, ac, i, r.PostFormValue("tab"), 0, 0)
		if err != nil {
			h.Shell.Fail(w, r, err)
			return
		}
		view.Drawer = &d
	}
	view.Dialog = f.component()
	h.renderPage(w, r, status, view)
}

// formFailed answers a submit that did not go through: the form again with
// the errors on its fields and the form-level message patched into the open
// dialog (Datastar), or the page with the dialog open (a native post; status
// 422 for validation).
func (h *Ingestions) formFailed(w http.ResponseWriter, r *http.Request, ac *auth.AuthContext, id string, f ingsForm) {
	if ds.IsDatastar(r) {
		sse := ds.NewSSE(w, r)
		errs := []error{sse.Patch(ingsFormBody(f)), sse.Patch(kit.FormDialogError(ingsFormDialog, f.Errs.Form))}
		if focus := f.firstInvalid(); focus != "" {
			errs = append(errs, sse.Focus(focus))
		}
		if err := errors.Join(errs...); err != nil {
			slog.DebugContext(r.Context(), "ui: ingestion form failure patch not delivered", "err", err)
		}
		return
	}
	h.showDialog(w, r, ac, id, f, cmp.Or(f.Errs.Status, http.StatusUnprocessableEntity))
}

// ── errors → fields ────────────────────────────────────────────────────

// ingsErrorCodes map the use cases' codes to the control that caused them.
var ingsErrorCodes = map[string]string{
	"INGESTION_CODE_EXISTS":              "code",
	"INGESTION_APPLICATION_INVALID":      "application",
	"INGESTION_CLIENT_INVALID":           "client",
	"CLIENT_NOT_FOUND":                   "client",
	"CLIENT_REQUIRED":                    "client",
	"INGESTION_KIND_PERMISSION_REQUIRED": "kind",
}

// applyError records a use case's error on the form: a definition's field
// errors on the controls their paths name, a target reference on its control,
// the rest on the control the code names or in the form-level region.
func (f *ingsForm) applyError(r *http.Request, err error, built ingsBuilt) {
	ue, ok := errors.AsType[*usecase.Error](err)
	if !ok {
		f.Errs.apply(r, err, nil)
		return
	}
	switch ue.Code {
	case "INGESTION_INVALID":
		if fields, ok := ue.Details["fields"].([]ingestion.FieldError); ok {
			f.applyFieldErrors(fields, built)
			return
		}
	case "INGESTION_TARGET_INVALID":
		if name, msg, ok := ingsTargetField(ue.Message); ok {
			f.Errs.field(name, sentence(msg))
			f.Errs.Status = ue.HTTPStatus()
			return
		}
	}
	if name := ingsErrorCodes[ue.Code]; name != "" {
		f.Errs.field(name, sentence(ue.Message))
		f.Errs.Status = ue.HTTPStatus()
		return
	}
	f.Errs.apply(r, err, nil)
}

// applyFieldErrors puts each problem of a definition on the control its path
// names; one that names none is told in the form-level region.
func (f *ingsForm) applyFieldErrors(fields []ingestion.FieldError, built ingsBuilt) {
	var rest []string
	for _, fe := range fields {
		name, ok := "", false
		switch fe.Field {
		case "name", "code", "application":
			name, ok = fe.Field, true
		default:
			var fld ingsField
			if fld, ok = built.fieldFor(fe.Field); ok {
				name = fld.Name
			}
		}
		if ok {
			f.Errs.field(name, sentence(fe.Message))
		} else {
			rest = append(rest, fe.Field+" "+fe.Message)
		}
	}
	if len(rest) > 0 {
		f.Errs.Form = sentence(strings.Join(rest, "; "))
	}
	f.Errs.Status = http.StatusBadRequest
}

// ingsTargetField reads the resolver's "target.pool: no dispatch pool ..."
// into the control and the message.
func ingsTargetField(message string) (name, msg string, ok bool) {
	rest, found := strings.CutPrefix(message, "target.")
	if !found {
		return "", "", false
	}
	field, msg, _ := strings.Cut(rest, " ")
	field = strings.TrimSuffix(field, ":")
	if !slices.ContainsFunc(ingsTargetFields, func(t ingsField) bool { return t.Name == "target."+field }) {
		return "", "", false
	}
	return "target." + field, strings.TrimSpace(msg), true
}

// ── create ─────────────────────────────────────────────────────────────

// createForm opens the create dialog (the Nuxt page's New ingestion).
func (h *Ingestions) createForm(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	// POST /api/ingestions's check.
	if err := auth.CanCreateIngestions(ac); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	cat, kinds, err := h.catalogue(r)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	f := newIngsForm(false, cat)
	f.Open = true
	h.withChoices(r, ac, &f, kinds)
	h.showDialog(w, r, ac, "", f, http.StatusOK)
}

// parseBuilt makes the definition of the posted values, recording what could
// not be read as field errors on the form.
func (f *ingsForm) parseBuilt() (ingestion.Definition, ingsBuilt) {
	built := ingsBuildDefinition(f.Values, f.Cat)
	for name, msg := range built.Errs {
		f.Errs.field(name, msg)
	}
	if f.Errs.any() {
		return ingestion.Definition{}, built
	}
	raw, err := json.Marshal(built.Doc)
	if err != nil {
		f.Errs.Form = "The form could not be read."
		return ingestion.Definition{}, built
	}
	def, err := ingestion.ParseDefinition(raw)
	if err != nil {
		f.Errs.Form = sentence(err.Error())
	}
	return def, built
}

func (h *Ingestions) create(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	cat, kinds, err := h.catalogue(r)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	f := submittedIngsForm(r, false, cat)
	var built ingsBuilt
	fail := func() {
		h.withChoices(r, ac, &f, kinds)
		h.formFailed(w, r, ac, "", f)
	}
	if err := auth.CanCreateIngestions(ac); err != nil {
		f.Errs.apply(r, err, nil)
		fail()
		return
	}
	// The client is resolved only after the permission check, so a caller
	// without it learns nothing about which clients exist.
	var def ingestion.Definition
	if f.Values["application"] == "" {
		f.Errs.field("application", "Choose the application.")
	}
	if f.Values["client"] == "" {
		f.Errs.field("client", "Choose the client.")
	}
	def, built = f.parseBuilt()
	if f.Errs.any() {
		fail()
		return
	}
	cmd := ingops.CreateCommand{
		Application: f.Values["application"], Code: f.Values["code"], Definition: def,
		Live: f.Values["live"] == ingsOn, Note: optionalText(f.Values["note"]),
	}
	newID, err := h.Ops.Create(ctx, ingsClientRef(f.Values["client"]), cmd)
	if err != nil {
		f.applyError(r, err, built)
		fail()
		return
	}
	if !ds.IsDatastar(r) {
		h.nativeDone(w, r, newID, flashIngCreated, false)
		return
	}
	// As the Nuxt form: the new ingestion's drawer opens over the list.
	sse := ds.NewSSE(w, r)
	if err := sse.CloseDialog(ingsFormDialog); err != nil {
		slog.DebugContext(ctx, "ui: ingestion form close not delivered", "err", err)
	}
	h.afterChange(sse, r, ac, newID, false)
	if auth.CanReadIngestions(ac) == nil {
		if i, err := h.reachable(ctx, ac, newID); err != nil {
			h.Shell.FailSSE(sse, r, err)
		} else if d, err := h.buildDetail(ctx, ac, i, ingsTabOverview, 0, 0); err != nil {
			h.Shell.FailSSE(sse, r, err)
		} else if err := openIngsDrawer(sse, d); err != nil {
			slog.DebugContext(ctx, "ui: ingestion drawer patch not delivered", "err", err)
		}
	}
	if err := flashKey(sse, flashIngCreated); err != nil {
		slog.DebugContext(ctx, "ui: ingestion flash not delivered", "err", err)
	}
}

// ingsClientRef is the client as the API takes it: an id as it is, an
// identifier lower-cased (identifiers are stored so).
func ingsClientRef(v string) string {
	v = strings.TrimSpace(v)
	if strings.HasPrefix(v, "clt_") {
		return v
	}
	return strings.ToLower(v)
}

// ── change ─────────────────────────────────────────────────────────────

// editForm opens the change dialog, starting from the version ?from= names,
// else the live one, else the newest saved (the Nuxt form's rule).
func (h *Ingestions) editForm(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	if err := firstError(auth.CanReadIngestions(ac), auth.CanUpdateIngestions(ac)); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	id, err := ingsID(r)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	i, err := h.reachable(ctx, ac, id)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	def, from := i.Definition, i.Version
	if want, _ := strconv.Atoi(r.URL.Query().Get("from")); want > 0 {
		versions, err := h.Store.ListVersions(ctx, i.ID)
		if err != nil {
			h.Shell.Fail(w, r, usecase.Internal("REPO", "list_versions failed", err))
			return
		}
		if k := slices.IndexFunc(versions, func(v ingestion.Version) bool { return v.Number == want }); k >= 0 {
			def, from = versions[k].Definition, want
		}
	}
	cat, kinds, err := h.catalogue(r, ingsKindOf(i.Definition.Source), ingsKindOf(def.Source))
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	if block := ingsEditBlock(ac, i, kinds); block != "" {
		h.Shell.Fail(w, r, usecase.Authorization("INGESTION_CHANGE_REFUSED", block))
		return
	}
	f, err := ingsEditForm(i, def, from, cat)
	if err != nil {
		h.Shell.Fail(w, r, usecase.Internal("MARSHAL", "definition does not render", err))
		return
	}
	h.withChoices(r, ac, &f, kinds)
	h.showDialog(w, r, ac, id, f, http.StatusOK)
}

func ingsEditForm(i *ingestion.Ingestion, def ingestion.Definition, from int, cat ingsCatalogue) (ingsForm, error) {
	f := newIngsForm(true, cat)
	f.ID, f.Code, f.From, f.Latest, f.Live = i.ID, i.Code, from, i.LatestVersion, i.Version
	var err error
	if f.Values, err = ingsValuesOf(def, cat); err != nil {
		return ingsForm{}, err
	}
	f.Values["live"] = ""
	f.Open = true
	return f, nil
}

func (h *Ingestions) update(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	id, err := ingsID(r)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	i, err := h.reachable(ctx, ac, id)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	cat, kinds, err := h.catalogue(r, ingsKindOf(i.Definition.Source))
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	f := submittedIngsForm(r, true, cat)
	f.ID, f.Code, f.Latest, f.Live = i.ID, i.Code, i.LatestVersion, i.Version
	var built ingsBuilt
	fail := func() {
		h.withChoices(r, ac, &f, kinds)
		h.formFailed(w, r, ac, id, f)
	}
	// POST /api/ingestions/{id}/versions: update, and promote for a live one.
	live := f.Values["live"] == ingsOn
	check := auth.CanUpdateIngestions(ac)
	if check == nil && live {
		check = auth.CanPromoteIngestions(ac)
	}
	if check != nil {
		f.Errs.apply(r, check, nil)
		fail()
		return
	}
	var def ingestion.Definition
	def, built = f.parseBuilt()
	if f.Errs.any() {
		fail()
		return
	}
	cmd := ingops.SaveVersionCommand{ID: id, Definition: def, Live: live, Note: optionalText(f.Values["note"])}
	if err := h.Ops.SaveVersion(ctx, cmd); err != nil {
		f.applyError(r, err, built)
		fail()
		return
	}
	flash := flashIngVersionSaved
	if live {
		flash = flashIngVersionLive
	}
	if !ds.IsDatastar(r) {
		h.nativeDone(w, r, id, flash, false)
		return
	}
	sse := ds.NewSSE(w, r)
	if err := sse.CloseDialog(ingsFormDialog); err != nil {
		slog.DebugContext(ctx, "ui: ingestion form close not delivered", "err", err)
	}
	h.afterChange(sse, r, ac, id, false)
	if err := flashKey(sse, flash); err != nil {
		slog.DebugContext(ctx, "ui: ingestion flash not delivered", "err", err)
	}
}
