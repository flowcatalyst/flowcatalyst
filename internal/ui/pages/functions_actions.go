package pages

import (
	"cmp"
	"fmt"
	"log/slog"
	"maps"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"github.com/a-h/templ"
	"github.com/go-chi/chi/v5"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/function"
	fnops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/function/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/ds"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/kit"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/shell"
)

// The drawer's actions and their dialogs: versions and aliases, settings,
// routes, and the typed-address delete.

const (
	fnsAliasDialog   = "fns-alias-form"
	fnsSettingDialog = "fns-setting-form"
	fnsRouteDialog   = "fns-route-form"
	fnsDeleteDialog  = "fns-delete-form"
)

// done answers a successful action: for Datastar the dialog closed, the table
// of the list the user is on, the drawer when it shows this function (closed
// after a delete) and a flash, whose detail is the key's unless detail is
// given (text the server built: wiring counts, a key it validated); for a
// native post a redirect with the flash key.
func (h *Functions) done(w http.ResponseWriter, r *http.Request, ac *auth.AuthContext, id, dialog, flash, detail string, deleted bool) {
	if !ds.IsDatastar(r) {
		h.nativeDone(w, r, id, flash, deleted)
		return
	}
	ctx := r.Context()
	sse := ds.NewSSE(w, r)
	if dialog != "" {
		if err := sse.CloseDialog(dialog); err != nil {
			slog.DebugContext(ctx, "ui: function dialog close not delivered", "dialog", dialog, "err", err)
		}
	}
	h.afterChange(sse, r, ac, id, deleted)
	msg, ok := FlashMessages[flash]
	if !ok {
		slog.ErrorContext(ctx, "ui: unknown flash key", "key", flash)
		return
	}
	if detail != "" {
		msg.Detail = detail
	}
	if err := shell.Flash(sse, msg); err != nil {
		slog.DebugContext(ctx, "ui: function flash not delivered", "err", err)
	}
}

// requireID reads the function id of a drawer action from the path.
func (h *Functions) requireID(w http.ResponseWriter, r *http.Request) (string, bool) {
	id, err := fnsID(r)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return "", false
	}
	return id, true
}

// versionNumber reads the {number} path parameter.
func versionNumber(r *http.Request) (int32, error) {
	n, err := strconv.ParseInt(chi.URLParam(r, "number"), 10, 32)
	if err != nil || n < 1 {
		return 0, httperror.NotFound("FunctionVersion", chi.URLParam(r, "number"))
	}
	return int32(n), nil
}

// ── versions and aliases ───────────────────────────────────────────────

// promote points the live alias at a ready version (the SPA's Promote to
// live: immediate, no confirmation). The promote wiring is reported.
func (h *Functions) promote(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	if err := auth.CanPromoteFunctions(ac); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	id, ok := h.requireID(w, r)
	if !ok {
		return
	}
	number, err := versionNumber(r)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	wiring, err := h.Ops.PutAlias(ctx, id, function.LiveAlias, number)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	detail := "Version " + strconv.Itoa(int(number)) + " is now live"
	if t := wiring.text(); t != "" {
		detail += " — " + t
	}
	h.done(w, r, ac, id, "", flashFnPromoted, detail, false)
}

// removeAlias removes an alias; removing live unwires the function's
// subscriptions and schedules (the demote).
func (h *Functions) removeAlias(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	if err := auth.CanPromoteFunctions(ac); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	id, ok := h.requireID(w, r)
	if !ok {
		return
	}
	name := chi.URLParam(r, "name")
	if !function.ValidAliasName(name) {
		h.Shell.Fail(w, r, httperror.NotFound("FunctionAlias", name))
		return
	}
	wiring, err := h.Ops.DeleteAlias(ctx, id, name)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	detail := "Alias removed"
	if t := wiring.text(); t != "" {
		detail += " — " + t
	}
	h.done(w, r, ac, id, "", flashFnAliasRemoved, detail, false)
}

// retire retires a version; a version an alias still points at is refused by
// the use case (RETIRE_IN_USE), shown as an error flash.
func (h *Functions) retire(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	if err := auth.CanWriteFunctions(ac); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	id, ok := h.requireID(w, r)
	if !ok {
		return
	}
	number, err := versionNumber(r)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	if err := h.Ops.RetireVersion(ctx, id, number); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	h.done(w, r, ac, id, "", flashFnRetired, "Version "+strconv.Itoa(int(number))+" retired", false)
}

// fnsAliasForm is the point-alias dialog (the SPA's Point Alias dialog).
type fnsAliasForm struct {
	fnsBase
	FunctionID    string
	Name, Version string
	// Versions are the ready versions the alias may point at.
	Versions []kit.Option
	Open     bool
}

func newFnsAliasForm(id string) fnsAliasForm {
	return fnsAliasForm{fnsBase: newFnsBase(fnsAliasDialog, "name", "version"), FunctionID: id}
}

func (f fnsAliasForm) fields() []templ.Component {
	return []templ.Component{fnsAliasField(f, "name"), fnsAliasField(f, "version")}
}

func (f fnsAliasForm) component() templ.Component { return fnsAliasDialogC(f) }

var fnsAliasFieldCodes = map[string]string{
	"INVALID_ALIAS_NAME": "name",
	"NAME_REQUIRED":      "name",
	"NUMBER_REQUIRED":    "version",
	"VERSION_NOT_FOUND":  "version",
	"VERSION_NOT_READY":  "version",
}

func (h *Functions) readyVersions(r *http.Request, id string) ([]kit.Option, error) {
	versions, err := h.Functions.ListVersionsByFunction(r.Context(), id)
	if err != nil {
		return nil, err
	}
	slices.SortFunc(versions, func(a, b function.Version) int { return cmp.Compare(b.Number, a.Number) })
	var opts []kit.Option
	for _, v := range versions {
		if v.Status == function.VersionReady {
			opts = append(opts, kit.Option{Value: strconv.Itoa(int(v.Number)), Label: "Version " + strconv.Itoa(int(v.Number))})
		}
	}
	return opts, nil
}

// aliasForm opens the point-alias dialog; ?version=N is the version row's
// Point alias action, which starts at the live alias.
func (h *Functions) aliasForm(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	if err := auth.CanPromoteFunctions(ac); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	id, ok := h.requireID(w, r)
	if !ok {
		return
	}
	if _, err := h.loadFunction(ctx, ac, id); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	f := newFnsAliasForm(id)
	f.Open = true
	if v := r.URL.Query().Get("version"); v != "" {
		f.Name, f.Version = function.LiveAlias, v
	}
	opts, err := h.readyVersions(r, id)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	f.Versions = opts
	h.showDialog(w, r, ac, id, f, http.StatusOK)
}

func (h *Functions) putAlias(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	id, ok := h.requireID(w, r)
	if !ok {
		return
	}
	f := newFnsAliasForm(id)
	f.Open = true
	f.Name, f.Version = strings.TrimSpace(r.PostFormValue("name")), strings.TrimSpace(r.PostFormValue("version"))
	fail := func() {
		if opts, err := h.readyVersions(r, id); err == nil {
			f.Versions = opts
		}
		h.formFailed(w, r, ac, id, f)
	}
	if err := auth.CanPromoteFunctions(ac); err != nil {
		f.Errs.apply(r, err, nil)
		fail()
		return
	}
	if f.Name == "" {
		f.Errs.field("name", "Alias name is required.")
	}
	number, err := strconv.ParseInt(f.Version, 10, 32)
	if err != nil || number < 1 {
		f.Errs.field("version", "Select a version.")
	}
	if f.Errs.any() {
		fail()
		return
	}
	wiring, err := h.Ops.PutAlias(ctx, id, f.Name, int32(number))
	if err != nil {
		f.Errs.apply(r, err, fnsAliasFieldCodes)
		fail()
		return
	}
	detail := f.Name + " → version " + f.Version
	if t := wiring.text(); t != "" {
		detail += " — " + t
	}
	h.done(w, r, ac, id, fnsAliasDialog, flashFnAliasSaved, detail, false)
}

// ── settings ───────────────────────────────────────────────────────────

// fnsSettingForm is the dialog that sets a config value, a secret or a
// database DSN. A secret or DSN is write-only: the dialog never has a value to
// show, and a failed save does not echo the one that was typed.
type fnsSettingForm struct {
	fnsBase
	FunctionID string
	Kind       function.SettingKind
	Key        string
	// KeyFixed makes the key read-only: it is a declared key or the one being
	// replaced.
	KeyFixed bool
	// Value is a config value's text; empty for a secret or database.
	Value string
	Open  bool
}

func newFnsSettingForm(id string, kind function.SettingKind) fnsSettingForm {
	return fnsSettingForm{fnsBase: newFnsBase(fnsSettingDialog, "key", "value"), FunctionID: id, Kind: kind}
}

func (f fnsSettingForm) secret() bool { return f.Kind != function.SettingConfig }

func (f fnsSettingForm) fields() []templ.Component {
	return []templ.Component{fnsSettingField(f, "key"), fnsSettingField(f, "value")}
}

func (f fnsSettingForm) component() templ.Component { return fnsSettingDialogC(f) }

func (f fnsSettingForm) action() string {
	return fnsURL(f.FunctionID) + "/settings/" + fnsKindPath(f.Kind)
}

func (f fnsSettingForm) keyLabel() string {
	if f.Kind == function.SettingDB {
		return "Name"
	}
	return "Key"
}

// keyAttrs are the key input's attributes: read-only when the key is the one
// being set (still posted with the form).
func (f fnsSettingForm) keyAttrs() templ.Attributes {
	a := templ.Attributes{"autocomplete": "off", "spellcheck": "false"}
	if f.KeyFixed {
		a["readonly"] = true
	} else {
		a["autofocus"] = true
	}
	return a
}

func (f fnsSettingForm) valueHelp() string {
	if f.secret() {
		return "Write-only: this value is stored encrypted and is not shown again once saved."
	}
	return ""
}

var fnsSettingFieldCodes = map[string]string{
	"INVALID_KEY_FORMAT":        "key",
	"KEY_REQUIRED":              "key",
	"VALUE_REQUIRED":            "value",
	"ENCRYPTION_NOT_CONFIGURED": "value",
	"UNSUPPORTED_SECRET_SCHEME": "value",
}

// kindFromPath reads {kind} and makes the API endpoint's permission check for
// it; ok=false means the answer was sent.
func (h *Functions) kindFromPath(w http.ResponseWriter, r *http.Request, ac *auth.AuthContext) (function.SettingKind, bool) {
	kind, valid := fnsKindOf(chi.URLParam(r, "kind"))
	if !valid {
		h.Shell.Fail(w, r, httperror.NotFound("FunctionSetting", chi.URLParam(r, "kind")))
		return "", false
	}
	if err := fnsKindCheck(kind)(ac); err != nil {
		h.Shell.Fail(w, r, err)
		return "", false
	}
	return kind, true
}

// settingForm opens the dialog for a key: ?key= names a declared or set key
// (read-only in the dialog), none is the Add button. A config value is shown
// for editing; a secret's or database's never is.
func (h *Functions) settingForm(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	kind, ok := h.kindFromPath(w, r, ac)
	if !ok {
		return
	}
	id, ok := h.requireID(w, r)
	if !ok {
		return
	}
	if _, err := h.loadFunction(ctx, ac, id); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	f := newFnsSettingForm(id, kind)
	f.Open = true
	f.Key = strings.TrimSpace(r.URL.Query().Get("key"))
	f.KeyFixed = f.Key != ""
	if kind == function.SettingConfig && f.Key != "" {
		settings, err := h.Functions.ListSettings(ctx, id)
		if err != nil {
			h.Shell.Fail(w, r, err)
			return
		}
		for _, s := range settings {
			if s.Kind == kind && s.Key == f.Key {
				f.Value = s.Value
			}
		}
	}
	h.showDialog(w, r, ac, id, f, http.StatusOK)
}

func (h *Functions) saveSetting(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	id, ok := h.requireID(w, r)
	if !ok {
		return
	}
	kind, valid := fnsKindOf(chi.URLParam(r, "kind"))
	if !valid {
		h.Shell.Fail(w, r, httperror.NotFound("FunctionSetting", chi.URLParam(r, "kind")))
		return
	}
	f := newFnsSettingForm(id, kind)
	f.Open = true
	f.Key, f.KeyFixed = strings.TrimSpace(r.PostFormValue("key")), r.PostFormValue("keyFixed") != ""
	value := r.PostFormValue("value")
	// A config value is echoed on failure; a secret or database never is.
	if kind == function.SettingConfig {
		f.Value = value
	}
	if err := fnsKindCheck(kind)(ac); err != nil {
		f.Errs.apply(r, err, nil)
		h.formFailed(w, r, ac, id, f)
		return
	}
	if f.Key == "" {
		f.Errs.field("key", "Key is required.")
	}
	if value == "" {
		f.Errs.field("value", "A value is required.")
	}
	if f.Errs.any() {
		h.formFailed(w, r, ac, id, f)
		return
	}
	if err := h.Ops.PutSetting(ctx, id, kind, f.Key, value); err != nil {
		f.Errs.apply(r, err, fnsSettingFieldCodes)
		h.formFailed(w, r, ac, id, f)
		return
	}
	h.done(w, r, ac, id, fnsSettingDialog, flashFnSettingSaved, fmt.Sprintf("Set %s %q", fnsKindLabel(kind), f.Key), false)
}

func (h *Functions) removeSetting(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	kind, ok := h.kindFromPath(w, r, ac)
	if !ok {
		return
	}
	id, ok := h.requireID(w, r)
	if !ok {
		return
	}
	key := strings.TrimSpace(r.FormValue("key"))
	if err := h.Ops.DeleteSetting(ctx, id, kind, key); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	h.done(w, r, ac, id, "", flashFnSettingRemoved, fmt.Sprintf("Removed %s %q", fnsKindLabel(kind), key), false)
}

// ── routes ─────────────────────────────────────────────────────────────

// fnsRouteForm is the public-route dialog: a hostname covered by a claimed
// zone, a path prefix and what it serves (live, or one alias; aliasPrefixes
// opt a live route into prefixed hostnames).
type fnsRouteForm struct {
	fnsBase
	FunctionID                  string
	Edit                        bool
	Hostname, PathPrefix, Alias string
	AliasPrefixes               string
	Aliases                     []kit.Option
	Open                        bool
}

func newFnsRouteForm(id string) fnsRouteForm {
	return fnsRouteForm{fnsBase: newFnsBase(fnsRouteDialog, "hostname", "pathPrefix", "alias", "aliasPrefixes"), FunctionID: id}
}

func (f fnsRouteForm) fields() []templ.Component {
	out := make([]templ.Component, 0, len(f.keys))
	for _, k := range f.keys {
		out = append(out, fnsRouteField(f, k))
	}
	return out
}

func (f fnsRouteForm) title() string {
	if f.Edit {
		return "Edit Route"
	}
	return "Add Route"
}

// editField marks a post as the edit of an existing route: its hostname and
// path prefix are what the route is, so they are read-only then.
func (f fnsRouteForm) editField() kit.HiddenField {
	if f.Edit {
		return kit.HiddenField{Name: "edit", Value: "1"}
	}
	return kit.HiddenField{Name: "edit", Value: ""}
}

func (f fnsRouteForm) keyAttrs(extra templ.Attributes) templ.Attributes {
	a := templ.Attributes{"autocomplete": "off", "spellcheck": "false"}
	maps.Copy(a, extra)
	if f.Edit {
		a["readonly"] = true
	}
	return a
}

func (f fnsRouteForm) component() templ.Component { return fnsRouteDialogC(f) }

var fnsRouteFieldCodes = map[string]string{
	"INVALID_HOSTNAME":            "hostname",
	"ROUTE_HOST_NOT_CLAIMED":      "hostname",
	"ROUTE_TAKEN":                 "hostname",
	"INVALID_ALIAS_NAME":          "alias",
	"ALIAS_NOT_FOUND":             "alias",
	"ALIAS_PREFIXES_REQUIRE_LIVE": "aliasPrefixes",
	"INVALID_ALIAS_PREFIX":        "aliasPrefixes",
}

// withAliases offers the function's aliases other than live (live is the
// blank choice).
func (h *Functions) withAliases(r *http.Request, id string, f *fnsRouteForm) error {
	aliases, err := h.Functions.ListAliases(r.Context(), id)
	if err != nil {
		return err
	}
	for _, a := range aliases {
		if a.Name != function.LiveAlias {
			f.Aliases = append(f.Aliases, kit.Option{Value: a.Name, Label: a.Name})
		}
	}
	slices.SortFunc(f.Aliases, func(a, b kit.Option) int { return strings.Compare(a.Label, b.Label) })
	return nil
}

func (h *Functions) routeForm(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	if err := auth.CanManageFunctionRoutes(ac); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	id, ok := h.requireID(w, r)
	if !ok {
		return
	}
	if _, err := h.loadFunction(ctx, ac, id); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	f := newFnsRouteForm(id)
	f.Open, f.PathPrefix = true, "/"
	if routeID := chi.URLParam(r, "routeId"); routeID != "" {
		routes, err := h.Functions.ListRoutesByFunction(ctx, id)
		if err != nil {
			h.Shell.Fail(w, r, err)
			return
		}
		i := slices.IndexFunc(routes, func(rt function.Route) bool { return rt.ID == routeID })
		if i < 0 {
			h.Shell.Fail(w, r, httperror.NotFound("FunctionRoute", routeID))
			return
		}
		rt := routes[i]
		f.Edit, f.Hostname, f.PathPrefix, f.Alias, f.AliasPrefixes = true, rt.Hostname, rt.PathPrefix, deref(rt.Alias), strings.Join(rt.AliasPrefixes, ", ")
	}
	if err := h.withAliases(r, id, &f); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	h.showDialog(w, r, ac, id, f, http.StatusOK)
}

func (h *Functions) saveRoute(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	id, ok := h.requireID(w, r)
	if !ok {
		return
	}
	f := newFnsRouteForm(id)
	f.Open = true
	f.Edit = r.PostFormValue("edit") != ""
	f.Hostname, f.PathPrefix = strings.TrimSpace(r.PostFormValue("hostname")), strings.TrimSpace(r.PostFormValue("pathPrefix"))
	f.Alias, f.AliasPrefixes = strings.TrimSpace(r.PostFormValue("alias")), r.PostFormValue("aliasPrefixes")
	fail := func() {
		_ = h.withAliases(r, id, &f)
		h.formFailed(w, r, ac, id, f)
	}
	if err := auth.CanManageFunctionRoutes(ac); err != nil {
		f.Errs.apply(r, err, nil)
		fail()
		return
	}
	if f.Hostname == "" {
		f.Errs.field("hostname", "Hostname is required.")
		fail()
		return
	}
	cmd := fnops.PutRouteCommand{
		FunctionID: id, Hostname: f.Hostname, PathPrefix: cmp.Or(f.PathPrefix, "/"), Alias: f.Alias,
		AliasPrefixes: strings.FieldsFunc(f.AliasPrefixes, func(c rune) bool { return c == ',' || c == ' ' || c == '\n' || c == '\t' }),
	}
	if err := h.Ops.PutRoute(ctx, cmd); err != nil {
		f.Errs.apply(r, err, fnsRouteFieldCodes)
		fail()
		return
	}
	h.done(w, r, ac, id, fnsRouteDialog, flashFnRouteSaved, "", false)
}

func (h *Functions) removeRoute(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	if err := auth.CanManageFunctionRoutes(ac); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	id, ok := h.requireID(w, r)
	if !ok {
		return
	}
	routeID := chi.URLParam(r, "routeId")
	if !kit.ValidID(routeID) {
		h.Shell.Fail(w, r, httperror.NotFound("FunctionRoute", routeID))
		return
	}
	if err := h.Ops.DeleteRoute(ctx, id, routeID); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	h.done(w, r, ac, id, "", flashFnRouteRemoved, "", false)
}

// ── delete ─────────────────────────────────────────────────────────────

// fnsDeleteForm is the delete confirmation. Deleting a function is
// irreversible, so the address must be typed exactly (the same safety as
// `fcdev fn delete`; surrounding whitespace is forgiven, case is not).
type fnsDeleteForm struct {
	fnsBase
	FunctionID, Address string
	Versions            int
	// Live is the version the live alias points at, 0 when never promoted.
	Live int
	Open bool
}

func (f fnsDeleteForm) fields() []templ.Component {
	return []templ.Component{fnsDeleteField(f)}
}

func (f fnsDeleteForm) versionsText() string {
	if f.Versions == 1 {
		return "1 version"
	}
	return strconv.Itoa(f.Versions) + " versions"
}

func (f fnsDeleteForm) liveText() string {
	if f.Live == 0 {
		return "none (never promoted)"
	}
	return "v" + strconv.Itoa(f.Live)
}

func (f fnsDeleteForm) component() templ.Component { return fnsDeleteDialogC(f) }

// deleteForm builds the dialog for a function: the SPA's summary of what goes
// with it (the version count and the live version).
func (h *Functions) deleteFormFor(r *http.Request, fn *function.Function) (fnsDeleteForm, error) {
	f := fnsDeleteForm{fnsBase: newFnsBase(fnsDeleteDialog, "confirm"), FunctionID: fn.ID, Address: fn.Address}
	versions, err := h.Functions.ListVersionsByFunction(r.Context(), fn.ID)
	if err != nil {
		return f, err
	}
	aliases, err := h.Functions.ListAliases(r.Context(), fn.ID)
	if err != nil {
		return f, err
	}
	f.Versions = len(versions)
	for _, a := range aliases {
		if a.Name != function.LiveAlias {
			continue
		}
		for _, v := range versions {
			if v.ID == a.VersionID {
				f.Live = int(v.Number)
			}
		}
	}
	return f, nil
}

func (h *Functions) deleteForm(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	if err := firstError(auth.CanReadFunctions(ac), auth.CanWriteFunctions(ac)); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	id, ok := h.requireID(w, r)
	if !ok {
		return
	}
	fn, err := h.loadFunction(ctx, ac, id)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	f, err := h.deleteFormFor(r, fn)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	f.Open = true
	h.showDialog(w, r, ac, id, f, http.StatusOK)
}

func (h *Functions) delete(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	id, ok := h.requireID(w, r)
	if !ok {
		return
	}
	if err := auth.CanWriteFunctions(ac); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	fn, err := h.scopedFunction(ctx, ac, id)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	f, err := h.deleteFormFor(r, fn)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	f.Open = true
	// The address must be typed exactly, checked here whatever the dialog did.
	if strings.TrimSpace(r.PostFormValue("confirm")) != fn.Address {
		f.Errs.field("confirm", "Type the function's address exactly to confirm.")
		h.formFailed(w, r, ac, id, f)
		return
	}
	if err := h.Ops.Delete(ctx, id); err != nil {
		f.Errs.apply(r, err, nil)
		h.formFailed(w, r, ac, id, f)
		return
	}
	h.done(w, r, ac, id, fnsDeleteDialog, flashFnDeleted, "Function "+fn.Address+" deleted", true)
}
