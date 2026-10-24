package pages

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"github.com/a-h/templ"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/function"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/functiondomain"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/ds"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/kit"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecase"
)

// The record drawer (the SPA's FunctionDetailDrawer): the details, the
// versions and aliases, what the live version declares, the settings and the
// public routes, one tab each.

type fnsVersionRow struct {
	Number                  int32
	Runtime                 string
	Status                  string
	Variant                 kit.BadgeVariant
	Failure                 string
	Digest, DigestFull      string
	Published               string
	PublishedISO            string
	Aliases                 []string
	Ready, Retired, Aliased bool
	// Live is whether the live alias points here.
	Live bool
}

type fnsAliasRow struct {
	Name         string
	Version      int32
	Updated, ISO string
	Live         bool
}

// fnsSettingRow is one config key, secret or database binding: declared by
// the version, set on the platform, or both. A secret's or database's value
// is not a field: it is never read.
type fnsSettingRow struct {
	Key        string
	Set        bool
	Value      string
	EditHref   string
	RemoveHref string
}

type fnsRouteRow struct {
	ID, Hostname, PathPrefix string
	Serves                   string
	Prefixes                 string
	EditHref, RemoveHref     string
}

type fnsDetail struct {
	ID, Address, Application string
	Client, Pool             string
	Warm                     bool
	Description              string
	// ServiceAccountCode is the function's own webhook verifier account, empty
	// for the application's.
	ServiceAccountID, ServiceAccountCode string
	Memory, Concurrency, Timeout, Body   string
	Created, Updated                     string
	Versions                             []fnsVersionRow
	Aliases                              []fnsAliasRow
	// DeclaredFrom says which version Declared is from.
	DeclaredFrom string
	Declared     *fnsDeclared
	Config       []fnsSettingRow
	Secrets, DBs []fnsSettingRow
	// Missing is whether a declared key has no value (a promote is refused).
	Missing bool
	Routes  []fnsRouteRow
	// Zones are the hostname zones claimed by the function's owner (shown
	// to a viewer who may manage domains).
	Zones    []string
	ShowZone bool
	// Diagnostics is the trace switches (the function trace page's), when
	// the page has them.
	Diagnostics templ.Component
	Tab         string
	Perms       fnsPerms
	// Base is the function's address (path) for links and actions.
	Base string
}

func (d fnsDetail) drawerProps() kit.DrawerProps {
	return kit.DrawerProps{ID: fnsDrawerID, Signal: fnsDrawerSig, Label: "Function", Size: kit.DrawerWide, Title: d.Address, Subtitle: d.Application + " · " + d.Client, Extras: fnsTags(d.Pool, d.Warm)}
}

// fnsTabOf is the tab a ?tab= value names (the first for anything else).
func fnsTabOf(v string) string {
	if slices.ContainsFunc(fnsTabs, func(t kit.TabItem) bool { return t.Value == v }) {
		return v
	}
	return fnsTabOverview
}

func fnsStamp(f *function.Function) (created, updated string) {
	const layout = "2006-01-02 15:04:05 UTC"
	return f.CreatedAt.UTC().Format(layout), f.UpdatedAt.UTC().Format(layout)
}

// buildDetail reads the function's children with the API's by-id reads
// (versions, aliases, settings, routes: each CanReadFunctions with the
// function's client scope, checked by loadFunction) and shapes the drawer.
func (h *Functions) buildDetail(ctx context.Context, ac *auth.AuthContext, f *function.Function, tab string) (fnsDetail, error) {
	d := fnsDetail{
		ID: f.ID, Address: f.Address, Application: f.ApplicationCode, Pool: cmp.Or(deref(f.Pool), "default"), Warm: f.Warm,
		Description: deref(f.Description), Tab: fnsTabOf(tab), Perms: fnsPermsFor(ac), Base: fnsURL(f.ID),
		Memory: strconv.Itoa(int(f.Limits.MemoryMB)) + " MB", Concurrency: strconv.Itoa(int(f.Limits.MaxConcurrency)),
		Timeout: strconv.Itoa(int(f.Limits.TimeoutMs)) + " ms", Body: strconv.FormatInt(f.Limits.MaxBodyBytes, 10) + " bytes",
	}
	d.Created, d.Updated = fnsStamp(f)
	names, err := h.clientNames(ctx)
	if err != nil {
		return fnsDetail{}, err
	}
	d.Client = names.label(f.ClientID)
	if f.ServiceAccountID != nil {
		d.ServiceAccountID = *f.ServiceAccountID
		d.ServiceAccountCode = d.ServiceAccountID
		// Display data, as the API: a failed lookup leaves the id.
		if codes, err := h.Functions.ServiceAccountCodes(ctx, []string{d.ServiceAccountID}); err == nil && codes[d.ServiceAccountID] != "" {
			d.ServiceAccountCode = codes[d.ServiceAccountID]
		}
	}
	versions, err := h.Functions.ListVersionsByFunction(ctx, f.ID)
	if err != nil {
		return fnsDetail{}, usecase.Internal("REPO", "list_versions_by_function failed", err)
	}
	aliases, err := h.Functions.ListAliases(ctx, f.ID)
	if err != nil {
		return fnsDetail{}, usecase.Internal("REPO", "list_aliases failed", err)
	}
	settings, err := h.Functions.ListSettings(ctx, f.ID)
	if err != nil {
		return fnsDetail{}, usecase.Internal("REPO", "list_settings failed", err)
	}
	routes, err := h.Functions.ListRoutesByFunction(ctx, f.ID)
	if err != nil {
		return fnsDetail{}, usecase.Internal("REPO", "list_routes_by_function failed", err)
	}
	slices.SortFunc(versions, func(a, b function.Version) int { return cmp.Compare(b.Number, a.Number) })
	d.fillVersions(versions, aliases)
	d.fillSettings(settings)
	d.fillRoutes(routes)
	if d.Perms.Zones && h.Zones != nil {
		zones, err := h.Zones.FindWithFilters(ctx, functiondomain.ListFilters{})
		if err != nil {
			return fnsDetail{}, usecase.Internal("REPO", "list zones failed", err)
		}
		d.ShowZone = true
		for _, z := range zones {
			if functiondomain.SameOwner(z.ClientID, f.ClientID) {
				d.Zones = append(d.Zones, z.Zone)
			}
		}
	}
	if h.Trace != nil {
		if d.Diagnostics, err = h.Trace.Switches(ctx, f.ID, true); err != nil {
			return fnsDetail{}, err
		}
	}
	return d, nil
}

// fillVersions shapes the versions (newest first) and the aliases, and picks
// the version the "what it declares" tab reads: the live one, else the newest.
func (d *fnsDetail) fillVersions(versions []function.Version, aliases []function.Alias) {
	numberByID := make(map[string]int32, len(versions))
	for i := range versions {
		numberByID[versions[i].ID] = versions[i].Number
	}
	var liveNumber int32
	var hasLive bool
	for _, a := range aliases {
		row := fnsAliasRow{Name: a.Name, Version: numberByID[a.VersionID], Updated: a.UpdatedAt.UTC().Format("2006-01-02 15:04"), ISO: a.UpdatedAt.UTC().Format("2006-01-02T15:04:05Z"), Live: a.Name == function.LiveAlias}
		if row.Live {
			liveNumber, hasLive = row.Version, true
		}
		d.Aliases = append(d.Aliases, row)
	}
	slices.SortFunc(d.Aliases, func(a, b fnsAliasRow) int { return strings.Compare(a.Name, b.Name) })
	for i := range versions {
		v := &versions[i]
		row := fnsVersionRow{
			Number: v.Number, Runtime: cmp.Or(v.Runtime, function.DefaultRuntime), Status: string(v.Status), Variant: fnsVersionVariant(v.Status),
			Digest: fnsShortDigest(v.Digest), DigestFull: v.Digest,
			Ready: v.Status == function.VersionReady, Retired: v.Status == function.VersionRetired,
			Live: hasLive && v.Number == liveNumber,
		}
		if v.Status == function.VersionFailed {
			row.Failure = cmp.Or(fnsFailureText(v.Failure), "Load failed")
		}
		published := v.CreatedAt
		if v.ReadyAt != nil {
			published = *v.ReadyAt
		}
		row.Published, row.PublishedISO = published.UTC().Format("2006-01-02 15:04"), published.UTC().Format("2006-01-02T15:04:05Z")
		for _, a := range d.Aliases {
			if a.Version == v.Number {
				row.Aliases = append(row.Aliases, a.Name)
			}
		}
		row.Aliased = len(row.Aliases) > 0
		d.Versions = append(d.Versions, row)
	}
	// The SPA's liveOrNewestVersion.
	for i := range versions {
		if hasLive && versions[i].Number == liveNumber || !hasLive && i == 0 {
			desc := parseFnsDescribe(versions[i].Describe)
			d.Declared = &desc
			d.DeclaredFrom = "From version " + strconv.Itoa(int(versions[i].Number))
			if hasLive {
				d.DeclaredFrom += " (live)"
			} else {
				d.DeclaredFrom += " — no version promoted yet"
			}
			return
		}
	}
}

func fnsVersionVariant(s function.VersionStatus) kit.BadgeVariant {
	switch s {
	case function.VersionPublished:
		return kit.BadgeInfo
	case function.VersionReady:
		return kit.BadgeSuccess
	case function.VersionFailed:
		return kit.BadgeDestructive
	case function.VersionRetired:
	}
	return kit.BadgeSecondary
}

// fnsShortDigest is the SPA's versionDigestShort.
func fnsShortDigest(d string) string {
	if len(d) > 12 {
		return d[:12] + "…"
	}
	return d
}

// fnsFailureText is the SPA's describeFailure for a FAILED version's stored
// failure document.
func fnsFailureText(raw []byte) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return string(raw)
	}
	switch f := v.(type) {
	case string:
		return f
	case map[string]any:
		if m, ok := f["message"].(string); ok {
			return m
		}
		if m, ok := f["reason"].(string); ok {
			return m
		}
	}
	return string(raw)
}

// fillSettings shapes the three setting groups: the keys the declared version
// names plus the keys that are set, sorted. A secret's and a database's value
// is not part of the rows (the repository does not return it either).
func (d *fnsDetail) fillSettings(settings []function.Setting) {
	var declared fnsDeclared
	if d.Declared != nil {
		declared = *d.Declared
	}
	build := func(kind function.SettingKind, declaredKeys []string) []fnsSettingRow {
		have := map[string]function.Setting{}
		all := slices.Clone(declaredKeys)
		for _, s := range settings {
			if s.Kind == kind {
				have[s.Key] = s
				all = append(all, s.Key)
			}
		}
		slices.Sort(all)
		all = slices.Compact(all)
		seg := d.Base + "/settings/" + fnsKindPath(kind)
		rows := make([]fnsSettingRow, 0, len(all))
		for _, k := range all {
			s, set := have[k]
			row := fnsSettingRow{Key: k, Set: set, EditHref: seg + "/new?key=" + url.QueryEscape(k)}
			if set {
				row.EditHref = seg + "/edit?key=" + url.QueryEscape(k)
				row.RemoveHref = seg + "/delete?key=" + url.QueryEscape(k)
				if kind == function.SettingConfig {
					row.Value = s.Value
				}
			}
			// The SPA's computeMissingSettings: declared and not set.
			d.Missing = d.Missing || !set && slices.Contains(declaredKeys, k)
			rows = append(rows, row)
		}
		return rows
	}
	d.Config = build(function.SettingConfig, declared.Config)
	d.Secrets = build(function.SettingSecret, declared.Secrets)
	d.DBs = build(function.SettingDB, declared.DB)
}

func (d *fnsDetail) fillRoutes(routes []function.Route) {
	for _, r := range routes {
		row := fnsRouteRow{
			ID: r.ID, Hostname: r.Hostname, PathPrefix: r.PathPrefix, Serves: cmp.Or(deref(r.Alias), function.LiveAlias),
			Prefixes: strings.Join(r.AliasPrefixes, ", "),
			EditHref: d.Base + "/routes/" + r.ID + "/edit", RemoveHref: d.Base + "/routes/" + r.ID + "/delete",
		}
		d.Routes = append(d.Routes, row)
	}
}

// ── describe (docs/function-runner-plan.md §5.4), read leniently ───────

type (
	fnsEndpoint     struct{ Method, Path, Auth string }
	fnsSubscription struct{ EventType, Path, Mode string }
	fnsSchedule     struct{ Cron, Path, Timezone string }
)

// fnsDeclared is a version's describe document, shaped for display. It never
// fails: a malformed or absent document (a FAILED version may carry none)
// reads as all-empty, as the SPA's parseDescribe.
type fnsDeclared struct {
	Endpoints                             []fnsEndpoint
	Subscriptions                         []fnsSubscription
	Schedules                             []fnsSchedule
	Config, Secrets, DB, HTTPAllow, Emits []string
	Dispatches                            []string
}

func parseFnsDescribe(raw []byte) fnsDeclared {
	var obj map[string]any
	if json.Unmarshal(raw, &obj) != nil {
		return fnsDeclared{}
	}
	var out fnsDeclared
	for _, e := range fnsObjects(obj["endpoints"]) {
		out.Endpoints = append(out.Endpoints, fnsEndpoint{Method: cmp.Or(fnsString(e["method"]), "GET"), Path: fnsString(e["path"]), Auth: cmp.Or(fnsString(e["auth"]), "none")})
	}
	for _, e := range fnsObjects(obj["subscriptions"]) {
		out.Subscriptions = append(out.Subscriptions, fnsSubscription{EventType: fnsString(e["eventType"]), Path: fnsString(e["path"]), Mode: fnsString(e["mode"])})
	}
	for _, e := range fnsObjects(obj["schedules"]) {
		out.Schedules = append(out.Schedules, fnsSchedule{Cron: fnsString(e["cron"]), Path: fnsString(e["path"]), Timezone: fnsString(e["timezone"])})
	}
	for _, e := range fnsObjects(obj["dispatches"]) {
		out.Dispatches = append(out.Dispatches, fnsString(e["target"])+fnsString(e["path"]))
	}
	out.Config, out.Secrets, out.DB = fnsStrings(obj["config"]), fnsStrings(obj["secrets"]), fnsStrings(obj["db"])
	out.HTTPAllow, out.Emits = fnsStrings(obj["httpAllow"]), fnsStrings(obj["emits"])
	return out
}

func fnsObjects(v any) []map[string]any {
	list, _ := v.([]any)
	var out []map[string]any
	for _, e := range list {
		if m, ok := e.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

func fnsString(v any) string {
	s, _ := v.(string)
	return s
}

func fnsStrings(v any) []string {
	list, _ := v.([]any)
	var out []string
	for _, e := range list {
		if s, ok := e.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

// ── settings kinds ─────────────────────────────────────────────────────

// fnsKindPath is the kind's segment in the API's paths and ours.
func fnsKindPath(k function.SettingKind) string {
	switch k {
	case function.SettingSecret:
		return "secrets"
	case function.SettingDB:
		return "db"
	case function.SettingConfig:
	}
	return "config"
}

// fnsKindOf reads the {kind} path segment.
func fnsKindOf(seg string) (function.SettingKind, bool) {
	switch seg {
	case "config":
		return function.SettingConfig, true
	case "secrets":
		return function.SettingSecret, true
	case "db":
		return function.SettingDB, true
	}
	return "", false
}

// fnsKindLabel is the SPA's settingKindLabel.
func fnsKindLabel(k function.SettingKind) string {
	switch k {
	case function.SettingSecret:
		return "secret"
	case function.SettingDB:
		return "database binding"
	case function.SettingConfig:
	}
	return "config value"
}

// fnsKindCheck is the API endpoint's permission check for the kind: config
// values need manage, secrets and databases the secret-manage permission.
func fnsKindCheck(k function.SettingKind) func(*auth.AuthContext) error {
	if k == function.SettingConfig {
		return auth.CanWriteFunctions
	}
	return auth.CanManageFunctionSecrets
}

// ── the drawer's request handlers ──────────────────────────────────────

// detail opens the drawer: patched into the page for Datastar (a row's
// address or View details), or the list page with the drawer open for a
// navigation to /ui/functions/{id}.
func (h *Functions) detail(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	id, err := fnsID(r)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	f, err := h.loadFunction(ctx, ac, id)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	d, err := h.buildDetail(ctx, ac, f, r.URL.Query().Get("tab"))
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	if ds.IsDatastar(r) {
		sse := ds.NewSSE(w, r)
		if err := openFnsDrawer(sse, d); err != nil {
			slog.DebugContext(ctx, "ui: function drawer patch not delivered", "err", err)
		}
		return
	}
	view, err := h.listView(ctx, ac, parseListQuery(nil, fnsList))
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	view.Drawer = &d
	h.renderPage(w, r, http.StatusOK, view)
}

// openFnsDrawer patches the drawer's content for d, selects it and moves
// focus to its heading (drawer.md, server-loaded record).
func openFnsDrawer(sse *ds.SSE, d fnsDetail) error {
	return errors.Join(
		sse.Patch(fnsDrawerContent(d), ds.Inner("#"+fnsDrawerID)),
		sse.PatchSignals(map[string]any{fnsDrawerSig: d.ID, fnsTabSig: d.Tab}),
		sse.Focus(fnsDrawerID+"-title"),
	)
}

// fnsHidden are the hidden fields every action inside the drawer carries: the
// record the drawer shows (the drawer's signal) and the tab it is on, so the
// answer can refresh the drawer where the user is.
func fnsHidden() []kit.HiddenField {
	return []kit.HiddenField{drawerField(fnsDrawerSig), {Name: "tab", Attrs: templ.Attributes{"data-attr:value": "$" + kit.Signal(fnsTabSig)}}}
}

// afterChange re-renders what a change to function id affects: the table of
// the list the user is on and the drawer when it shows this function (the
// form's hidden "drawer" field), on the tab it is on. A deleted function's
// drawer is closed. A caller who may write but not read gets neither: the
// reads are the API's.
func (h *Functions) afterChange(sse *ds.SSE, r *http.Request, ac *auth.AuthContext, id string, deleted bool) {
	ctx := r.Context()
	if auth.CanReadFunctions(ac) != nil {
		return
	}
	if view, err := h.listView(ctx, ac, refererList(r, fnsList)); err != nil {
		h.Shell.FailSSE(sse, r, err)
	} else if err := view.patch(sse); err != nil {
		slog.DebugContext(ctx, "ui: functions table patch not delivered", "err", err)
	}
	if r.PostFormValue("drawer") != id {
		return
	}
	if deleted {
		if err := sse.PatchSignals(map[string]any{fnsDrawerSig: ""}); err != nil {
			slog.DebugContext(ctx, "ui: function drawer close not delivered", "err", err)
		}
		return
	}
	f, err := h.loadFunction(ctx, ac, id)
	if err != nil {
		h.Shell.FailSSE(sse, r, err)
		return
	}
	d, err := h.buildDetail(ctx, ac, f, r.PostFormValue("tab"))
	if err != nil {
		h.Shell.FailSSE(sse, r, err)
		return
	}
	if err := errors.Join(sse.Patch(fnsDrawerContent(d), ds.Inner("#"+fnsDrawerID)), sse.PatchSignals(map[string]any{fnsTabSig: d.Tab})); err != nil {
		slog.DebugContext(ctx, "ui: function drawer patch not delivered", "err", err)
	}
}

// nativeDone answers a successful native post: a redirect to the function's
// drawer on the tab the form came from (the list after a delete) with the
// flash key.
func (h *Functions) nativeDone(w http.ResponseWriter, r *http.Request, id, flash string, deleted bool) {
	h.Shell.SetFlash(w, flash)
	target := fnsPath
	if !deleted {
		target = fnsURL(id) + "?tab=" + fnsTabOf(r.PostFormValue("tab"))
	}
	// The path is under /ui and the id passed kit.ValidID.
	http.Redirect(w, r, target, http.StatusSeeOther) //nolint:gosec // G710: a /ui path built from a validated id
}

// wiringText is the SPA's describeWiring: "pool fn-billing-invoice-pdf · 2
// subscriptions created, 1 schedule updated"; empty when nothing changed.
func (w FunctionWiring) text() string {
	var parts []string
	if w.DispatchPoolCode != "" {
		parts = append(parts, "pool "+w.DispatchPoolCode)
	}
	for _, c := range []struct {
		noun, verb string
		n          int
	}{
		{"subscription", "created", w.SubscriptionsCreated},
		{"subscription", "updated", w.SubscriptionsUpdated},
		{"subscription", "deleted", w.SubscriptionsDeleted},
		{"schedule", "created", w.SchedulesCreated},
		{"schedule", "updated", w.SchedulesUpdated},
		{"schedule", "deleted", w.SchedulesDeleted},
	} {
		if c.n > 0 {
			noun := c.noun
			if c.n != 1 {
				noun += "s"
			}
			parts = append(parts, fmt.Sprintf("%d %s %s", c.n, noun, c.verb))
		}
	}
	return strings.Join(parts, " · ")
}
