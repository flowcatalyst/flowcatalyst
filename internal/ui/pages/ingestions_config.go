package pages

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/a-h/templ"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/ingestion"
	ingops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/ingestion/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/ds"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/kit"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/shell"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecase"
)

// The environment's settings of an ingestion (docs/ingestion-design.md section
// 13): a source that names a config entry holds no settings in its definition;
// they are platform config ingest/<key> of the application, at the
// ingestion's client. The drawer shows the entry and a form to write it; the
// form is drawn from the same field model as the definition's, and a write
// goes through the same use case as PUT /api/ingestions/{id}/config, so every
// check of the API applies.

const (
	ingsConfigDialog = "ings-config"
	ingsConfigSignal = "ingsConfig"
	ingsConfigBodyID = "ings-config-body"
	// ingsConfigDeleteDialog confirms deleting the entry; its message names the
	// ingestions that stop, so it is drawn with the drawer, not shared.
	ingsConfigDeleteDialog = "ings-config-delete"
)

// ingsConfigRow is one setting the drawer shows. A secret setting holds the
// secret's name, never a value, and is shown as that name.
type ingsConfigRow struct {
	Label, Value string
	Secret       bool
}

// ingsConfigView is the drawer's "Settings (this environment)" section.
type ingsConfigView struct {
	// Key is the entry's key; Status where it stands in this environment.
	Key    string
	Status ingestion.ConfigStatus
	// Problems say why an entry is INVALID.
	Problems []string
	Rows     []ingsConfigRow
	// TargetURL is the delivery URL the entry gives; TargetInDefinition says the
	// definition gives it instead.
	TargetURL          string
	TargetInDefinition bool
	Updated, UpdatedAt string
	// Unchecked: an extension kind no live runtime reports, so the platform
	// cannot check the value against its settings (SCHEMA_UNCHECKED).
	Unchecked bool
	// Draft: only the newest saved version, not the live one, names the entry.
	Draft bool
	// Others are the codes of the other ingestions of this application and
	// client that name the same entry.
	Others []string
	// Hidden: an entry exists but the viewer cannot update ingestions, so its
	// settings are not shown.
	Hidden bool
	// CanEdit offers the form; Block says why a caller who may update is not
	// offered it. CanDelete offers Delete (there is an entry to delete).
	CanEdit, CanDelete bool
	Block              string
	// Client is the client's name, for the messages.
	Client string
}

// Needs reports whether the ingestion waits for its settings: the live
// definition names an entry that is not set, or not valid.
func (c *ingsConfigView) Needs() bool {
	return c != nil && !c.Draft && (c.Status == ingestion.ConfigMissing || c.Status == ingestion.ConfigInvalid)
}

// Reason is why the ingestion needs its settings, as the list and the drawer
// say it.
func (c *ingsConfigView) Reason() string {
	return ingsNeedsReason(c.Key, c.Client, c.Status, c.Problems)
}

func ingsNeedsReason(key, client string, status ingestion.ConfigStatus, problems []string) string {
	if status == ingestion.ConfigInvalid {
		return "config " + ingestion.ConfigSection + "/" + key + " is not valid for client " + client + ": " + strings.Join(problems, "; ")
	}
	return "config " + ingestion.ConfigSection + "/" + key + " is not set for client " + client
}

func (c *ingsConfigView) statusText() string {
	switch c.Status {
	case ingestion.ConfigSet:
		return "Set"
	case ingestion.ConfigInvalid:
		return "Invalid"
	case ingestion.ConfigMissing:
		return "Not set"
	case ingestion.ConfigInline:
	}
	return ""
}

func (c *ingsConfigView) statusVariant() kit.BadgeVariant {
	switch c.Status {
	case ingestion.ConfigSet:
		return kit.BadgeSuccess
	case ingestion.ConfigInvalid:
		return kit.BadgeDestructive
	case ingestion.ConfigMissing:
		return kit.BadgeWarn
	case ingestion.ConfigInline:
	}
	return kit.BadgeSecondary
}

// stops are the ingestions that stop when the entry is deleted.
func (c *ingsConfigView) stops(self string) []string {
	return append([]string{self}, c.Others...)
}

func (c *ingsConfigView) deleteMessage(self string) string {
	which := "The ingestion " + self + " stops"
	if len(c.Others) > 0 {
		which = "These ingestions stop: " + strings.Join(c.stops(self), ", ")
	}
	return "Delete " + ingestion.ConfigSection + "/" + c.Key + " for client " + c.Client + "? " + which +
		" and takes no jobs until the settings are set again; nothing else changes."
}

func (d ingsDetail) configFormHref() string { return d.Base + "/config" }

// ingsWhere says where the data comes from, in one line: the effective
// source's summary while its entry is set, otherwise where the settings are
// kept.
func ingsWhere(src ingestion.Source, code string, res ingestion.Resolved) string {
	c, ok := src.(ingestion.Configured)
	if !ok {
		return ingsSourceSummary(src)
	}
	if res.Runnable() {
		return ingsSourceSummary(res.Definition.Source)
	}
	return "Settings in config " + ingestion.ConfigSection + "/" + c.Config.KeyFor(code)
}

// configView reads the drawer's settings section for ingestion i whose
// application is appCode. nil when neither the live version nor the newest
// saved one names an entry. live is the live definition resolved (the section
// reuses it when it is the one that names the entry).
func (h *Ingestions) configView(ctx context.Context, ac *auth.AuthContext, i *ingestion.Ingestion, appCode, clientName string, live ingestion.Resolved,
	kinds ingsKindPerms, reported []ingestion.RuntimeKind,
) (*ingsConfigView, error) {
	def, key, named, err := h.Store.ConfigNamed(ctx, i)
	if err != nil {
		return nil, usecase.Internal("REPO", "find_version failed", err)
	}
	if !named {
		return nil, nil //nolint:nilnil // no entry is named: there is no section
	}
	liveNames := ingestion.ConfigKeyOf(i.Definition, i.Code) != ""
	res := live
	if !liveNames {
		if res, err = h.Store.Resolve(ctx, appCode, i.ClientID, i.Code, def); err != nil {
			return nil, usecase.Internal("REPO", "config entry lookup failed", err)
		}
	}
	entry, err := h.Store.ConfigEntry(ctx, appCode, i.ClientID, key)
	if err != nil {
		return nil, usecase.Internal("REPO", "config entry lookup failed", err)
	}
	named2, _ := ingestion.ConfigOf(def)
	kindName := ingsKindOf(def.Source)
	v := &ingsConfigView{
		Key: key, Status: res.Status, Draft: !liveNames, Client: clientName, TargetInDefinition: def.Target.TargetURL != "",
		CanDelete: false,
	}
	for _, p := range res.Problems {
		v.Problems = append(v.Problems, p.Field+" "+p.Message)
	}
	if res.UpdatedAt != nil {
		v.Updated, v.UpdatedAt = res.UpdatedAt.UTC().Format("2006-01-02 15:04"), res.UpdatedAt.UTC().Format("2006-01-02T15:04:05Z")
	}
	if named2.Base == ingestion.KindCustom {
		schema, err := h.Store.SchemaOf(ctx, named2.Type)
		if err != nil {
			return nil, usecase.Internal("REPO", "live kinds failed", err)
		}
		v.Unchecked = !schema.Checked
	}
	sharing, err := h.Store.NamingConfig(ctx, i.ApplicationID, i.ClientID, key)
	if err != nil {
		return nil, usecase.Internal("REPO", "ingestions naming config failed", err)
	}
	for _, o := range sharing {
		if o.ID != i.ID {
			v.Others = append(v.Others, o.Code)
		}
	}
	// The settings name accounts and endpoints: only someone who can update
	// ingestions sees them; the rest see that an entry exists and its status.
	canSee := auth.CanUpdateIngestions(ac) == nil
	if entry != nil && !entry.Secret && !canSee {
		v.Hidden = true
	}
	if entry != nil && !entry.Secret && canSee {
		var value map[string]any
		if json.Unmarshal([]byte(entry.Value), &value) == nil && value != nil {
			if kind := ingsNewCatalogue(reported, kindName).find(kindName); kind != nil {
				v.Rows = ingsConfigRows(*kind, value)
			}
			v.TargetURL, _ = value["targetUrl"].(string)
		}
	}
	// The entry is the environment's part, so the update permission is enough
	// for an ingestion a set owns; the kind's own permission still applies.
	if auth.CanUpdateIngestions(ac) == nil {
		if v.Block = ingsKindBlock(ac, i, kinds); v.Block == "" {
			v.CanEdit, v.CanDelete = true, entry != nil
		}
	}
	return v, nil
}

// ingsConfigRows are the settings an entry holds, in the order of the kind's
// fields.
func ingsConfigRows(kind ingsKind, entry map[string]any) []ingsConfigRow {
	values := ingsEntryValues(kind, entry, false)
	var rows []ingsConfigRow
	for _, f := range kind.Fields {
		if !f.active(values) || f.Presence {
			continue
		}
		v := values[f.Name]
		if v == "" {
			continue
		}
		if f.Type == ingsSwitch {
			v = "Yes"
		}
		if utf8.RuneCountInString(v) > 400 {
			v = string([]rune(v)[:400]) + "…"
		}
		rows = append(rows, ingsConfigRow{Label: f.Label, Value: v, Secret: f.Type == ingsSecret})
	}
	return rows
}

// ── entry ↔ form values ────────────────────────────────────────────────

// ingsEntryDoc wraps an entry's settings the way a definition holds them, so
// the kind's fields (whose paths address a definition) find their values: a
// built-in kind's settings are its source, an extension's are source.settings.
func ingsEntryDoc(kind ingsKind, entry map[string]any) map[string]any {
	settings := maps.Clone(entry)
	delete(settings, "targetUrl")
	if kind.BuiltIn {
		return map[string]any{"source": settings}
	}
	return map[string]any{"source": map[string]any{"settings": settings}}
}

// ingsEntryValues reads an entry into form values; with defaults, the fields
// it does not hold start at the kind's defaults.
func ingsEntryValues(kind ingsKind, entry map[string]any, defaults bool) map[string]string {
	values := map[string]string{}
	if defaults {
		for _, f := range kind.Fields {
			if f.Default != "" {
				values[f.Name] = f.Default
			}
		}
	}
	doc := ingsEntryDoc(kind, entry)
	for _, f := range kind.Fields {
		v, found := ingsGetPath(doc, f.Path)
		if s, ok := ingsFormatValue(f, v, found); ok {
			values[f.Name] = s
		} else if f.Type == ingsSwitch {
			delete(values, f.Name)
		}
	}
	if t, _ := entry["targetUrl"].(string); t != "" {
		values[ingsConfigTargetURL] = t
	}
	return values
}

// ingsBuildEntry reads the posted values of the kind's fields into the
// entry's value: the settings as an inline source holds them, and targetUrl
// when the delivery URL is the entry's to give.
func ingsBuildEntry(values map[string]string, kind ingsKind, withTargetURL bool) (map[string]any, ingsBuilt) {
	b := ingsBuilt{Doc: map[string]any{}, Errs: map[string]string{}}
	for _, f := range kind.Fields {
		if !f.active(values) {
			continue
		}
		b.Fields = append(b.Fields, f)
		b.read(f, values[f.Name])
	}
	src, _ := b.Doc["source"].(map[string]any)
	entry := map[string]any{}
	if kind.BuiltIn {
		maps.Copy(entry, src)
	} else if settings, ok := src["settings"].(map[string]any); ok {
		maps.Copy(entry, settings)
	}
	if withTargetURL {
		if t := strings.TrimSpace(values[ingsConfigTargetURL]); t != "" {
			entry["targetUrl"] = t
		}
	}
	return entry, b
}

// ingsEntryFieldFor finds the control a problem of an entry is about. The
// platform names a built-in kind's problems by the definition's paths
// (source.host) and an extension's by the entry's (config.region).
func ingsEntryFieldFor(kind ingsKind, b ingsBuilt, path string) (string, bool) {
	switch path {
	case "config.targetUrl", "target.targetUrl":
		return ingsConfigTargetURL, true
	}
	if !kind.BuiltIn {
		if rest, ok := strings.CutPrefix(path, "config."); ok {
			path = "source.settings." + rest
		}
	}
	for _, f := range b.Fields {
		if f.Type == ingsJSON && strings.HasPrefix(path, f.Path+".") {
			return f.Name, true
		}
	}
	if f, ok := b.fieldFor(path); ok {
		return f.Name, true
	}
	return "", false
}

// ── the form ───────────────────────────────────────────────────────────

// ingsConfigForm is the settings form of one ingestion's entry: its kind's
// fields, drawn by the same controls as the definition's form.
type ingsConfigForm struct {
	// F carries the values, the errors and the id prefix the controls use.
	F    ingsForm
	Kind ingsKind
	ID   string
	Code string
	Key  string
	// Client is the client's name; Others the other ingestions sharing the
	// entry; TargetInDefinition says the delivery URL is the definition's.
	Client             string
	Others             []string
	TargetInDefinition bool
	// Unchecked: the platform cannot check an extension's settings.
	Unchecked bool
	// Exists: there is an entry to change (the dialog then needs a change to submit).
	Exists bool
}

func (c ingsConfigForm) targetField() ingsField {
	return ingsField{
		Name: ingsConfigTargetURL, Label: "Delivery URL", Type: ingsText, Optional: true, Placeholder: "https://app.example.com/hooks/orders",
		Help: "Where each job is delivered in this environment. The definition gives none, so it is given here.",
	}
}

func (c ingsConfigForm) dialogProps() kit.FormDialogProps {
	return kit.FormDialogProps{
		ID: ingsConfigDialog, Signal: ingsConfigSignal, Width: kit.WidthWide, Action: ingsURL(c.ID) + "/config", Open: c.F.Open, Error: c.F.Errs.Form,
		Title: "Settings for " + c.Code, Description: "Platform config " + ingestion.ConfigSection + "/" + c.Key + " for client " + c.Client + ", in this environment",
		SubmitLabel: "Save settings", RequireChange: c.Exists, Hidden: ingsHidden(),
	}
}

// signals declares the signals of the controls other fields depend on.
func (c ingsConfigForm) signals() string {
	sig := map[string]any{}
	for _, fld := range c.Kind.Fields {
		if !fld.control {
			continue
		}
		if fld.Type == ingsSwitch {
			sig[ingsSignal(fld.Name)] = c.F.Values[fld.Name] == ingsOn
		} else {
			sig[ingsSignal(fld.Name)] = c.F.Values[fld.Name]
		}
	}
	b, _ := json.Marshal(sig)
	return string(b)
}

func (c ingsConfigForm) firstInvalid() string {
	for _, fld := range c.Kind.Fields {
		if c.F.fieldError(fld.Name) != "" {
			return c.F.id(fld.Name)
		}
	}
	if c.F.fieldError(ingsConfigTargetURL) != "" {
		return c.F.id(ingsConfigTargetURL)
	}
	return ""
}

func (c ingsConfigForm) component() templ.Component { return ingsConfigDialogC(c) }

// sharedText names the other ingestions the value is checked against.
func (c ingsConfigForm) sharedText() string {
	if len(c.Others) == 0 {
		return ""
	}
	return "The entry is shared with " + strings.Join(c.Others, ", ") + ": the value must suit each of them."
}

// applyError puts a refusal of the write on the controls it is about; a
// problem that names no control is told in the form-level region.
func (c *ingsConfigForm) applyError(r *http.Request, err error, built ingsBuilt) {
	if ue, ok := errors.AsType[*usecase.Error](err); ok && ue.Code == "INGESTION_INVALID" {
		if fields, ok := ue.Details["fields"].([]ingestion.FieldError); ok {
			var rest []string
			for _, fe := range fields {
				if name, found := ingsEntryFieldFor(c.Kind, built, fe.Field); found {
					c.F.Errs.field(name, sentence(fe.Message))
				} else {
					rest = append(rest, fe.Field+" "+fe.Message)
				}
			}
			if len(rest) > 0 {
				c.F.Errs.Form = sentence(strings.Join(rest, "; "))
			}
			c.F.Errs.Status = http.StatusBadRequest
			return
		}
	}
	c.F.Errs.apply(r, err, nil)
}

// loadConfigForm reads the ingestion and builds the settings form for the entry
// it names, with the checks of PUT /api/ingestions/{id}/config: the update
// permission, the ingestion's reach and its kind's permission. A set does not
// own the entry, so it does not block it.
func (h *Ingestions) loadConfigForm(r *http.Request, ac *auth.AuthContext, id string) (ingsConfigForm, *ingestion.Ingestion, error) {
	ctx := r.Context()
	if err := firstError(auth.CanReadIngestions(ac), auth.CanUpdateIngestions(ac)); err != nil {
		return ingsConfigForm{}, nil, err
	}
	i, err := h.reachable(ctx, ac, id)
	if err != nil {
		return ingsConfigForm{}, nil, err
	}
	def, key, named, err := h.Store.ConfigNamed(ctx, i)
	if err != nil {
		return ingsConfigForm{}, nil, usecase.Internal("REPO", "find_version failed", err)
	}
	if !named {
		return ingsConfigForm{}, nil, usecase.Validation("INGESTION_CONFIG_NOT_NAMED",
			"the ingestion's source holds its settings inline; name an entry with the source's settings choice first")
	}
	kindName := ingsKindOf(def.Source)
	cat, kinds, err := h.catalogue(r, kindName)
	if err != nil {
		return ingsConfigForm{}, nil, err
	}
	if block := ingsKindBlock(ac, i, kinds); block != "" {
		return ingsConfigForm{}, nil, usecase.Authorization("INGESTION_CHANGE_REFUSED", block)
	}
	kind := cat.find(kindName)
	if kind == nil {
		return ingsConfigForm{}, nil, usecase.Validation("INGESTION_KIND_UNKNOWN", "the source kind "+kindName+" is not known")
	}
	app, _, clientName, err := h.owners(ctx, i)
	if err != nil {
		return ingsConfigForm{}, nil, err
	}
	entry, err := h.Store.ConfigEntry(ctx, app.Code, i.ClientID, key)
	if err != nil {
		return ingsConfigForm{}, nil, usecase.Internal("REPO", "config entry lookup failed", err)
	}
	var value map[string]any
	if entry != nil && !entry.Secret {
		_ = json.Unmarshal([]byte(entry.Value), &value)
	}
	c := ingsConfigForm{
		F:    ingsForm{Prefix: ingsConfigDialog, Cat: ingsCatalogue{*kind}, Values: ingsEntryValues(*kind, value, true), Open: true},
		Kind: *kind, ID: i.ID, Code: i.Code, Key: key, Client: clientName, TargetInDefinition: def.Target.TargetURL != "", Exists: entry != nil,
	}
	if cn, ok := ingestion.ConfigOf(def); ok && cn.Base == ingestion.KindCustom {
		schema, err := h.Store.SchemaOf(ctx, cn.Type)
		if err != nil {
			return ingsConfigForm{}, nil, usecase.Internal("REPO", "live kinds failed", err)
		}
		c.Unchecked = !schema.Checked
	}
	sharing, err := h.Store.NamingConfig(ctx, i.ApplicationID, i.ClientID, key)
	if err != nil {
		return ingsConfigForm{}, nil, usecase.Internal("REPO", "ingestions naming config failed", err)
	}
	for _, o := range sharing {
		if o.ID != i.ID {
			c.Others = append(c.Others, o.Code)
		}
	}
	return c, i, nil
}

// configForm opens the settings dialog.
func (h *Ingestions) configForm(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	id, err := ingsID(r)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	c, _, err := h.loadConfigForm(r, ac, id)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	h.showConfigDialog(w, r, ac, id, c, http.StatusOK)
}

// showConfigDialog answers a request for the dialog: patched into the page's
// dialog slot for Datastar, or the list page with the ingestion's drawer and
// the dialog open.
func (h *Ingestions) showConfigDialog(w http.ResponseWriter, r *http.Request, ac *auth.AuthContext, id string, c ingsConfigForm, status int) {
	if ds.IsDatastar(r) {
		if err := patchDialogSlot(ds.NewSSE(w, r), ingsDialogSlot, c.component()); err != nil {
			slog.DebugContext(r.Context(), "ui: ingestion settings dialog patch not delivered", "err", err)
		}
		return
	}
	ctx := r.Context()
	view, err := h.listView(ctx, ac, parseIngsQuery(nil))
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
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
	view.Dialog = c.component()
	h.renderPage(w, r, status, view)
}

// configFailed answers a submit that did not go through: the controls again
// with the errors found.
func (h *Ingestions) configFailed(w http.ResponseWriter, r *http.Request, ac *auth.AuthContext, c ingsConfigForm) {
	if ds.IsDatastar(r) {
		sse := ds.NewSSE(w, r)
		errs := []error{sse.Patch(ingsConfigBody(c)), sse.Patch(kit.FormDialogError(ingsConfigDialog, c.F.Errs.Form))}
		if focus := c.firstInvalid(); focus != "" {
			errs = append(errs, sse.Focus(focus))
		}
		if err := errors.Join(errs...); err != nil {
			slog.DebugContext(r.Context(), "ui: ingestion settings failure patch not delivered", "err", err)
		}
		return
	}
	c.F.Open = true
	status := c.F.Errs.Status
	if status == 0 {
		status = http.StatusUnprocessableEntity
	}
	h.showConfigDialog(w, r, ac, c.ID, c, status)
}

// saveConfig writes the entry the ingestion names (PUT /api/ingestions/{id}/config:
// the update permission, then the use case, which holds the value against every
// ingestion that names the entry).
func (h *Ingestions) saveConfig(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	id, err := ingsID(r)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	c, _, err := h.loadConfigForm(r, ac, id)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	posted := map[string]string{}
	for k, vs := range r.PostForm {
		if len(vs) > 0 {
			posted[k] = vs[0]
		}
	}
	// What the browser did not post (an unticked switch, a branch not chosen)
	// is absent; a branch not chosen gets its defaults back so a form shown
	// again after an error is whole.
	active := map[string]bool{}
	for _, f := range c.Kind.Fields {
		if f.active(posted) {
			active[f.Name] = true
		}
	}
	for _, f := range c.Kind.Fields {
		if _, isPosted := posted[f.Name]; !isPosted && !active[f.Name] && f.Default != "" {
			posted[f.Name] = f.Default
		}
	}
	c.F.Values = posted
	entry, built := ingsBuildEntry(posted, c.Kind, !c.TargetInDefinition)
	for name, msg := range built.Errs {
		c.F.Errs.field(name, msg)
	}
	if c.F.Errs.any() {
		h.configFailed(w, r, ac, c)
		return
	}
	ev, err := h.Ops.SetConfig(ctx, ingops.SetConfigCommand{ID: id, Value: entry})
	if err != nil {
		c.applyError(r, err, built)
		h.configFailed(w, r, ac, c)
		return
	}
	if !ds.IsDatastar(r) {
		h.nativeDone(w, r, id, flashIngConfigSet, false)
		return
	}
	sse := ds.NewSSE(w, r)
	if err := sse.CloseDialog(ingsConfigDialog); err != nil {
		slog.DebugContext(ctx, "ui: ingestion settings close not delivered", "err", err)
	}
	h.afterChange(sse, r, ac, id, false)
	msg := FlashMessages[flashIngConfigSet]
	if len(ev.Ingestions) > 1 {
		msg.Detail += "; checked against " + strings.Join(ev.Ingestions, ", ")
	}
	if len(ev.Warnings) > 0 {
		msg.Kind, msg.Summary = kit.FlashWarn, "Saved, with a warning"
		msg.Detail += ". " + strings.Join(ev.Warnings, " ")
	}
	if err := shell.Flash(sse, msg); err != nil {
		slog.DebugContext(ctx, "ui: ingestion flash not delivered", "err", err)
	}
}

// deleteConfig deletes the entry the ingestion names; every ingestion that
// names it then waits for it (DELETE /api/ingestions/{id}/config: the update
// permission).
func (h *Ingestions) deleteConfig(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	if err := auth.CanUpdateIngestions(ac); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	id, err := ingsID(r)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	ev, err := h.Ops.DeleteConfig(ctx, id)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	if !ds.IsDatastar(r) {
		h.nativeDone(w, r, id, flashIngConfigDeleted, false)
		return
	}
	sse := ds.NewSSE(w, r)
	h.afterChange(sse, r, ac, id, false)
	msg := FlashMessages[flashIngConfigDeleted]
	if len(ev.Ingestions) > 0 {
		msg.Detail = fmt.Sprintf("Waiting for settings: %s", strings.Join(ev.Ingestions, ", "))
	}
	if err := shell.Flash(sse, msg); err != nil {
		slog.DebugContext(ctx, "ui: ingestion flash not delivered", "err", err)
	}
}
