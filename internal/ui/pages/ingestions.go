package pages

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"slices"
	"strconv"

	"github.com/a-h/templ"
	"github.com/go-chi/chi/v5"

	"github.com/flowcatalyst/flowcatalyst-go/internal/ids"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/ingestion"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/ds"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/kit"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/shell"
	"github.com/flowcatalyst/go-sdk/usecase"
)

// IngestionStore is the slice of the ingestion repository the page reads (the
// calls GET /api/ingestions, the by-id reads under it and the source kinds
// make).
type IngestionStore interface {
	List(ctx context.Context, f ingestion.ListFilter) (*ingestion.Page, error)
	FindByID(ctx context.Context, id string) (*ingestion.Ingestion, error)
	ListVersions(ctx context.Context, ingestionID string) ([]ingestion.Version, error)
	LiveKinds(ctx context.Context) ([]ingestion.RuntimeKind, error)
	AnyLiveRuntimeCarries(ctx context.Context) (func(kind string) bool, error)
	// NoticesOf is the undismissed notices (rejected failures) of the
	// ingestions shown.
	NoticesOf(ctx context.Context, ids []string) (map[string]ingestion.Notices, error)
	// ListSecretFetches is when each runtime last fetched each of the
	// ingestion's secrets (GET /api/ingestions/{id}/secrets).
	ListSecretFetches(ctx context.Context, ingestionID string) ([]ingestion.SecretFetch, error)
	// MissingSecrets is which of the named secrets have no value at the client,
	// as the resume readiness check looks them up.
	MissingSecrets(ctx context.Context, applicationCode string, clientID ids.ClientID, names []string) ([]string, error)

	// The environment's settings of an ingestion whose source names a config
	// entry (docs/ingestion-design.md section 13), as the config endpoints read
	// them. Resolve reads the entry a definition names, ConfigNamed finds the
	// definition (live, else the newest saved) that names one, ConfigEntry
	// reads an entry, SchemaOf the settings a live runtime reports for an
	// extension kind, and NamingConfig the ingestions that name an entry.
	Resolve(ctx context.Context, applicationCode string, clientID ids.ClientID, code string, def ingestion.Definition) (ingestion.Resolved, error)
	ConfigNamed(ctx context.Context, i *ingestion.Ingestion) (def ingestion.Definition, key string, ok bool, err error)
	ConfigEntry(ctx context.Context, applicationCode string, clientID ids.ClientID, key string) (*ingestion.ConfigEntry, error)
	SchemaOf(ctx context.Context, kind string) (ingestion.KindSchema, error)
	NamingConfig(ctx context.Context, applicationID ids.ApplicationID, clientID ids.ClientID, key string) ([]*ingestion.Ingestion, error)
}

// Ingestions is the ingestions page (/ui/ingestions), mirroring the Nuxt
// control plane's IngestionsPage, IngestionDrawer, IngestionFormPage and
// IngestionVersions: the list, the create and change dialogs, and the record
// drawer with its tabs. Reads make the API's checks (CanReadIngestions, the
// ingestion's reach); every write makes the permission check of the API
// endpoint it mirrors and runs its use case (IngestionCommands). Its Activity,
// Failures and History tabs are IngestionTabs'.
type Ingestions struct {
	Store IngestionStore
	Ops   IngestionCommands
	// Apps backs the application filter and the create form's application
	// select (the integration applications the caller reaches).
	Apps ApplicationStore
	// Clients backs the create form's client select; ClientNames the client
	// names and the list's client filter.
	Clients     ClientDirectory
	ClientNames ClientStore
	Shell       *shell.Shell
}

const (
	ingsPath   = "/ui/ingestions"
	ingsFormID = "ings-list"

	ingsDrawerID   = "ings-drawer"
	ingsDrawerSig  = "selIng"
	ingsTabSig     = "ingTab"
	ingsDialogSlot = "ings-dialog-slot"

	ingsLiveGet = "@get('/ui/ingestions', {contentType: 'form'})"

	ingsFacetApp    = "application"
	ingsFacetClient = "client"
	ingsFacetKind   = "kind"

	ingsPauseDialog   = "ings-pause"
	ingsResumeDialog  = "ings-resume"
	ingsDeleteDialog  = "ings-delete"
	ingsLiveDialog    = "ings-live"
	ingsDismissDialog = "ings-dismiss"

	// ingsTabOverview is the drawer's first tab; ingsTabFailures the one that
	// lists the failures and dismisses them.
	ingsTabOverview = "overview"
	ingsTabFailures = "failures"

	// ingsPageSize is the API's default page.
	ingsPageSize = 50

	ingsIntegration = "INTEGRATION"
)

// ingsTabs are the drawer's tabs in order; the value is the ?tab= parameter.
var ingsTabs = []kit.TabItem{
	{Value: ingsTabOverview, Label: "Overview"},
	{Value: "definition", Label: "Definition"},
	{Value: "versions", Label: "Versions"},
	{Value: "activity", Label: "Activity"},
	{Value: ingsTabFailures, Label: "Failures"},
	{Value: "history", Label: "History"},
}

// ingsLazyTabs are the tabs whose content is IngestionTabs': filled when the
// tab is first shown.
var ingsLazyTabs = []string{"activity", "failures", "history"}

// ingsListSpec is the ingestions list's URL state. The API pages by cursor, in
// creation order, and does not sort.
var ingsListSpec = &listSpec{
	path:     ingsPath,
	sortKeys: []string{"created"},
	statusOK: func(s string) bool { return s == "ACTIVE" || s == "PAUSED" },
	facets:   []string{ingsFacetApp, ingsFacetClient, ingsFacetKind},
}

var ingsStatusOptions = []kit.Option{{Value: "ACTIVE", Label: "Active"}, {Value: "PAUSED", Label: "Paused"}}

// Mount registers the page's routes (POSTs behind parseForm). GET routes
// never change state: dialogs are fetched with GET, written with POST.
func (h *Ingestions) Mount(r chi.Router) {
	r.Get("/ingestions", h.list)
	r.Get("/ingestions/new", h.createForm)
	r.Get("/ingestions/{id}", h.detail)
	r.Get("/ingestions/{id}/edit", h.editForm)
	r.Get("/ingestions/{id}/config", h.configForm)
	r.Get("/ingestions/{id}/compare", h.compare)
	r.Get("/ingestions/{id}/tabs/{tab}", h.lazyTab)
	r.Get("/ingestions/{id}/definition.yaml", h.export)
	post := r.With(parseForm)
	post.Post("/ingestions/new", h.create)
	post.Post("/ingestions/{id}/edit", h.update)
	post.Post("/ingestions/{id}/config", h.saveConfig)
	post.Post("/ingestions/{id}/config/delete", h.deleteConfig)
	post.Post("/ingestions/{id}/pause", h.pause)
	post.Post("/ingestions/{id}/resume", h.resume)
	post.Post("/ingestions/{id}/delete", h.delete)
	post.Post("/ingestions/{id}/versions/{number}/live", h.makeLive)
	post.Post("/ingestions/{id}/failures/{failureId}/dismiss", h.dismissFailure)
}

// ── permissions ────────────────────────────────────────────────────────

// ingsPerms are what the viewer is offered; the server decides again on every
// post, with the API endpoint's check: create, update (which includes pause
// and resume), delete and promote.
type ingsPerms struct {
	Create, Update, Delete, Promote bool
}

func ingsPermsFor(ac *auth.AuthContext) ingsPerms {
	return ingsPerms{
		Create:  auth.CanCreateIngestions(ac) == nil,
		Update:  auth.CanUpdateIngestions(ac) == nil,
		Delete:  auth.CanDeleteIngestions(ac) == nil,
		Promote: auth.CanPromoteIngestions(ac) == nil,
	}
}

// kindPerms are the permissions a kind needs to be created or changed: what
// the live runtimes report, and camel-route's own requirement whatever they
// report.
type ingsKindPerms map[string]string

func (p ingsKindPerms) of(kind string) string {
	if perm := p[kind]; perm != "" {
		return perm
	}
	if kind == "camel-route" {
		return auth.PermIngestionRoute
	}
	return ""
}

func (h *Ingestions) kindPerms(ctx context.Context) (ingsKindPerms, []ingestion.RuntimeKind, error) {
	reported, err := h.Store.LiveKinds(ctx)
	if err != nil {
		return nil, nil, usecase.Internal("REPO", "live kinds failed", err)
	}
	p := ingsKindPerms{}
	for _, k := range reported {
		p[k.Kind] = k.RequiredPermission
	}
	return p, reported, nil
}

// editBlock is why a change to an ingestion of this kind is not offered to a
// caller who may update ingestions: the set that owns it, or the permission
// its kind needs. "" when it is.
func ingsEditBlock(ac *auth.AuthContext, i *ingestion.Ingestion, kinds ingsKindPerms) string {
	if i.ManagedBy != nil {
		return "Managed by a set: change the set's files and apply them again."
	}
	return ingsKindBlock(ac, i, kinds)
}

// ingsKindBlock is the part of editBlock that is the kind's: an extension kind
// that declares a permission is for whoever holds it. A set owns the
// definition, not the environment's settings, so the settings form asks only
// this.
func ingsKindBlock(ac *auth.AuthContext, i *ingestion.Ingestion, kinds ingsKindPerms) string {
	kind := ingsKindOf(i.Definition.Source)
	if ingsIsExtension(i.Definition.Source) {
		if perm := kinds.of(kind); !ingestion.HoldsKindPermission(ac, perm) {
			if perm == auth.PermIngestionRoute && ac.HasPermission(perm) {
				return "A " + kind + " ingestion is written by a pipeline service account or by an ingestion administrator, at anchor tier."
			}
			return "Changing a " + kind + " ingestion needs " + perm + "."
		}
	}
	return ""
}

// ingsIsExtension reports whether the source runs as an extension's kind,
// whether it holds its settings or names the config entry that does.
func ingsIsExtension(src ingestion.Source) bool {
	switch s := src.(type) {
	case ingestion.Custom:
		return true
	case ingestion.Configured:
		return s.Base == ingestion.KindCustom
	}
	return false
}

// ── URL state ──────────────────────────────────────────────────────────

// ingsQuery is one view of the list: the search, the filters and the keyset
// position.
type ingsQuery struct {
	listQuery
	After string
}

func parseIngsQuery(v url.Values) ingsQuery {
	q := ingsQuery{listQuery: parseListQuery(v, ingsListSpec)}
	if after := v.Get("after"); kit.ValidID(after) {
		q.After = after
	}
	return q
}

func (q ingsQuery) values() url.Values {
	v := q.listQuery.values()
	if q.After != "" {
		v.Set("after", q.After)
	}
	return v
}

// URL is the canonical URL of this view.
func (q ingsQuery) URL() string { return encodeURL(ingsPath, q.values()) }

func (q ingsQuery) first() ingsQuery {
	q.After = ""
	return q
}

// clearURL is the view without search and filters.
func (q ingsQuery) clearURL() string { return ingsPath }

// one is the single value of a facet ("" for none).
func (q ingsQuery) one(name string) string {
	if vals := q.facet(name); len(vals) > 0 {
		return vals[0]
	}
	return ""
}

// ── list ───────────────────────────────────────────────────────────────

type ingsRow struct {
	ID, Code, Name      string
	Application, Client string
	Kind, Where         string
	Active              bool
	Draft               bool
	Set                 string
	Notice              bool
	// NeedsSettings: the source names a config entry this environment has not
	// set (or not validly), so the ingestion does not run; NeedsReason says why.
	NeedsSettings bool
	NeedsReason   string
	// Notices counts the undismissed rejected failures; LatestNotice is the
	// newest one's message.
	Notices              int
	LatestNotice         string
	Version, Latest      int
	Updated, UpdatedISO  string
	DetailHref, EditHref string
	// EditBlock is why Edit is shown disabled ("" when it is not).
	EditBlock string
}

type ingsView struct {
	Query ingsQuery
	Rows  []ingsRow
	// Apps, Clients and Kinds are the filters' options.
	Apps, Clients, Kinds []kit.Option
	OlderHref            string
	Perms                ingsPerms
	// Dialog is a form dialog rendered with the page (a navigation to a
	// dialog's address, or a native post re-rendered with errors); Drawer the
	// record the drawer opens with.
	Dialog templ.Component
	Drawer *ingsDetail
}

func (h *Ingestions) list(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	if err := auth.CanReadIngestions(ac); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	view, err := h.listView(ctx, ac, parseIngsQuery(r.URL.Query()))
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	if ds.IsDatastar(r) {
		sse := ds.NewSSE(w, r)
		if err := view.patch(sse); err != nil {
			slog.DebugContext(ctx, "ui: ingestions patch not delivered", "err", err)
		}
		return
	}
	h.renderPage(w, r, http.StatusOK, view)
}

func (h *Ingestions) renderPage(w http.ResponseWriter, r *http.Request, status int, view ingsView) {
	h.Shell.RenderStatus(w, r, status, shell.Page{Title: "Ingestions", Path: ingsPath}, ingsPage(view))
}

// patch replaces the table and the pager, the toolbar's live state and the
// address.
func (v ingsView) patch(sse *ds.SSE) error {
	return errors.Join(
		sse.Patch(ingsTable(v)), sse.Patch(kit.CursorPager(v.pager())),
		sse.Patch(kit.TableToolbarState(v.toolbar().State())), sse.ReplaceURL(v.Query.URL()),
	)
}

// filterOptions are the application filter's (the integration applications
// the caller reaches, by code) and the client filter's (the SPA's ClientFilter:
// active clients the caller can access, by id).
func (h *Ingestions) filterOptions(ctx context.Context, ac *auth.AuthContext) (apps, clients []kit.Option, err error) {
	if h.Apps != nil {
		kind := ingsIntegration
		as, err := h.Apps.FindWithFilters(ctx, &kind, nil)
		if err != nil {
			return nil, nil, usecase.Internal("REPO", "find applications failed", err)
		}
		for _, a := range as {
			if ac.CanAccessApplication(string(a.ID)) {
				apps = append(apps, kit.Option{Value: a.Code, Label: a.Name + " (" + a.Code + ")"})
			}
		}
	}
	cs, err := allClients(ctx, h.ClientNames)
	if err != nil {
		return nil, nil, err
	}
	return apps, clientFilterOptions(cs, ac), nil
}

func (h *Ingestions) listView(ctx context.Context, ac *auth.AuthContext, q ingsQuery) (ingsView, error) {
	v := ingsView{Perms: ingsPermsFor(ac)}
	var err error
	if v.Apps, v.Clients, err = h.filterOptions(ctx, ac); err != nil {
		return ingsView{}, err
	}
	kinds, reported, err := h.kindPerms(ctx)
	if err != nil {
		return ingsView{}, err
	}
	for _, k := range ingsNewCatalogue(reported) {
		if k.Name != ingestion.KindCustom {
			v.Kinds = append(v.Kinds, kit.Option{Value: k.Name, Label: k.Label})
		}
	}
	q.listQuery = q.withFacet(ingsFacetApp, prune(q.facet(ingsFacetApp), v.Apps)).
		withFacet(ingsFacetClient, prune(q.facet(ingsFacetClient), v.Clients)).
		withFacet(ingsFacetKind, prune(q.facet(ingsFacetKind), v.Kinds))
	// GET /api/ingestions: the filters in SQL, a caller below anchor confined
	// to its clients and an application's service account to its application.
	f := ingestion.ListFilter{After: q.After, Limit: ingsPageSize}
	f.AccessibleClientIDs, f.AccessibleApplicationIDs = ingestion.ListReach(ac)
	if q.Q != "" {
		f.Q = &q.Q
	}
	if q.Status != "" {
		f.Status = &q.Status
	}
	if app := q.one(ingsFacetApp); app != "" {
		f.ApplicationCode = &app
	}
	if c := q.one(ingsFacetClient); c != "" {
		f.ClientID = &c
	}
	if k := q.one(ingsFacetKind); k != "" {
		f.Kind = &k
	}
	page, err := h.Store.List(ctx, f)
	if err != nil {
		return ingsView{}, err
	}
	carries, err := h.Store.AnyLiveRuntimeCarries(ctx)
	if err != nil {
		return ingsView{}, usecase.Internal("REPO", "live runtimes failed", err)
	}
	names, err := h.clientNames(ctx)
	if err != nil {
		return ingsView{}, err
	}
	shown := make([]string, 0, len(page.Items))
	for i := range page.Items {
		shown = append(shown, page.Items[i].ID)
	}
	notices, err := h.Store.NoticesOf(ctx, shown)
	if err != nil {
		return ingsView{}, usecase.Internal("REPO", "notices failed", err)
	}
	for i := range page.Items {
		item := &page.Items[i]
		row := toIngsRow(ac, item, names, kinds, carries)
		n := notices[item.ID]
		row.Notices, row.LatestNotice = n.Count, n.LatestMessage
		if _, byConfig := item.Definition.Source.(ingestion.Configured); byConfig {
			res, err := h.Store.Resolve(ctx, item.ApplicationCode, item.ClientID, item.Code, item.Definition)
			if err != nil {
				return ingsView{}, usecase.Internal("REPO", "config entry lookup failed", err)
			}
			// The entry's settings are shown only to someone who can update ingestions.
			if auth.CanUpdateIngestions(ac) != nil {
				res = res.Redacted(item.Definition)
			}
			row.Where = ingsWhere(item.Definition.Source, item.Code, res)
			if !res.Runnable() {
				row.NeedsSettings = true
				row.NeedsReason = ingsNeedsReason(res.Key, row.Client, res.Status, ingsProblemTexts(res.Problems))
			}
		}
		v.Rows = append(v.Rows, row)
	}
	if page.Next != "" {
		next := q
		next.After = page.Next
		v.OlderHref = next.URL()
	}
	v.Query = q
	return v, nil
}

func (h *Ingestions) clientNames(ctx context.Context) (fnsNames, error) {
	n := fnsNames{}
	cs, err := allClients(ctx, h.ClientNames)
	if err != nil {
		return nil, err
	}
	for _, c := range cs {
		n[string(c.ID)] = c.Name
	}
	return n, nil
}

func ingsURL(id string) string { return ingsPath + "/" + url.PathEscape(id) }

// ingsNoticeCount is "1 notice" or "n notices".
func ingsNoticeCount(n int) string {
	if n == 1 {
		return "1 notice"
	}
	return strconv.Itoa(n) + " notices"
}

func toIngsRow(ac *auth.AuthContext, item *ingestion.ListItem, names fnsNames, kinds ingsKindPerms, carries func(string) bool) ingsRow {
	i := &item.Ingestion
	kind := ingsKindOf(i.Definition.Source)
	row := ingsRow{
		ID: i.ID, Code: i.Code, Name: i.Definition.Name, Application: item.ApplicationCode,
		Client: cmp.Or(names[string(i.ClientID)], item.ClientIdentifier), Kind: ingsKindLabel(kind), Where: ingsSourceSummary(i.Definition.Source),
		Active: i.Active, Draft: i.Version == 0, Set: deref(i.ManagedBy), Notice: i.Live() && !carries(kind),
		Version: i.Version, Latest: i.LatestVersion,
		Updated: i.UpdatedAt.UTC().Format("2006-01-02 15:04"), UpdatedISO: i.UpdatedAt.UTC().Format("2006-01-02T15:04:05Z"),
		DetailHref: ingsURL(i.ID), EditHref: ingsURL(i.ID) + "/edit", EditBlock: ingsEditBlock(ac, i, kinds),
	}
	return row
}

// rowActions: Edit inline (the Nuxt row has none; the drawer's Change is the
// first action here), View details, Pause or Resume and Remove in the menu.
func (v ingsView) rowActions(row ingsRow) kit.RowActionsProps {
	p := kit.RowActionsProps{ID: "ings-" + row.ID + "-actions", RowLabel: row.Code}
	if v.Perms.Update {
		edit := kit.RowAction{Icon: "pencil", Label: "Change", Href: row.EditHref, Attrs: templ.Attributes{"data-on:click__prevent": liveLink, "aria-haspopup": "dialog"}}
		edit.DisabledReason = row.EditBlock
		p.Actions = append(p.Actions, edit)
	}
	p.Menu = append(p.Menu, kit.RowAction{Icon: "eye", Label: "View details", Href: row.DetailHref, Attrs: templ.Attributes{"data-on:click__prevent": liveLink}})
	if v.Perms.Update {
		if row.Active {
			p.Menu = append(p.Menu, kit.RowAction{Icon: "pause", Label: "Pause…", Attrs: confirmTrigger(ingsPauseDialog, row.DetailHref+"/pause")})
		} else {
			p.Menu = append(p.Menu, kit.RowAction{Icon: "play", Label: "Resume…", Attrs: confirmTrigger(ingsResumeDialog, row.DetailHref+"/resume")})
		}
	}
	if v.Perms.Delete {
		del := kit.RowAction{Icon: "trash-2", Label: "Remove…", Destructive: true, Attrs: confirmTrigger(ingsDeleteDialog, row.DetailHref+"/delete")}
		if row.Set != "" {
			del = kit.RowAction{Icon: "trash-2", Label: "Remove…", Destructive: true, DisabledReason: "Managed by a set: release the set first."}
		}
		p.Menu = append(p.Menu, del)
	}
	return p
}

// ── source summaries ───────────────────────────────────────────────────

// ingsSourceSummary says where the data comes from, in one line (the Nuxt
// pages' sourceSummary).
func ingsSourceSummary(src ingestion.Source) string {
	switch s := src.(type) {
	case ingestion.HTTP:
		switch a := s.Auth.(type) {
		case ingestion.FlowCatalystToken:
			return "POST, flowcatalyst-token for " + a.Application
		case ingestion.BasicAuth:
			return "POST, basic"
		case ingestion.BearerAuth:
			return "POST, bearer"
		}
		return "POST"
	case ingestion.Sftp:
		return s.Username + "@" + s.Host + ":" + strconv.Itoa(s.Port) + s.Directory
	case ingestion.RabbitMQ:
		return s.URI + " / " + s.Queue
	case ingestion.Sqs:
		return s.Region + " / " + s.Queue
	case ingestion.S3:
		return s.Region + " / " + s.Bucket + deref(s.Prefix)
	case ingestion.DatabasePoll:
		kind := ""
		if s.Track != nil {
			kind = " (" + ingsTrackKind(s.Track) + ")"
		}
		return s.Table + kind
	case ingestion.APIPoll:
		return s.Method + " " + s.URL + " (" + ingsAPIAuthKind(s.Auth) + ")"
	case ingestion.Custom:
		return s.Type + " (extension)"
	}
	return ""
}

func ingsTrackKind(t ingestion.Track) string {
	switch t.(type) {
	case ingestion.Watermark:
		return "watermark"
	case ingestion.StatusTrack:
		return "status"
	}
	return ""
}

func ingsAPIAuthKind(a ingestion.APIAuth) string {
	switch a.(type) {
	case ingestion.APIBasic:
		return "basic"
	case ingestion.APIBearer:
		return "bearer"
	case ingestion.APIKey:
		return "api-key"
	case ingestion.ClientCredentials:
		return "oauth-client-credentials"
	case ingestion.Login:
		return "login"
	}
	return "none"
}

func ingsSplitSummary(s ingestion.Split) string {
	switch s := s.(type) {
	case ingestion.CSV:
		if s.Header {
			return "CSV lines, the first a header"
		}
		return "CSV lines, no header"
	case ingestion.JSON:
		return "JSON array elements at " + s.ArrayPath
	case ingestion.XML:
		return "XML elements <" + s.Element + ">"
	}
	return "One record per payload"
}

// ── drawer ─────────────────────────────────────────────────────────────

type ingsVersionRow struct {
	Number int
	Live   bool
	// Draft: saved after the live version (or while none is live) and not
	// live, so it waits for someone who holds promote.
	Draft               bool
	Note, By, Applied   string
	Created, CreatedISO string
	// MakeLive: the Make live action is offered.
	MakeLive bool
	EditHref string
}

// ingsComparison is two saved versions side by side.
type ingsComparison struct {
	A, B                       int
	ALive, BLive               bool
	AYAML, BYAML               string
	Same                       bool
	ALabel, BLabel, SameNotice string
}

type ingsDetail struct {
	ID, Code, Name, Application, Client string
	ClientIdentifier                    string
	Active, Draft, Notice               bool
	Set                                 string
	// Notices and LatestNotice: the undismissed rejected failures.
	Notices      int
	LatestNotice string
	// Warnings: what the live definition says that will not run as meant.
	Warnings              []string
	Version, Latest       int
	Kind, Where, Endpoint string
	Split, SourceYAML     string
	Target                ingestion.Target
	KeepPayloads          bool
	Created, Updated      string
	YAML                  string
	Versions              []ingsVersionRow
	Compare               *ingsComparison
	// Secrets: the names the live definition binds and who last fetched
	// each (names only, never a value).
	Secrets []ingsSecretRow
	// SecretsWithheld: the settings are in a config entry the viewer cannot
	// read, so the secrets it binds are not listed.
	SecretsWithheld bool
	// Config is the environment's settings, when the source names an entry.
	Config *ingsConfigView
	Tab    string
	Perms  ingsPerms
	// CanChange: Change… and New version are offered; EditBlock says why a
	// caller who may update is not offered them.
	CanChange bool
	EditBlock string
	// CanRemove and CanLive: Remove is offered, and Make live on versions.
	CanRemove, CanLive bool
	// Base is the ingestion's address (path) for links and actions.
	Base string
}

func (d ingsDetail) drawerProps() kit.DrawerProps {
	return kit.DrawerProps{ID: ingsDrawerID, Signal: ingsDrawerSig, Label: "Ingestion", Size: kit.DrawerWide, Title: d.Code, Subtitle: d.Name + " · " + d.Application + " · " + d.Client, Extras: ingsStateTags(d.Active, d.Draft, d.Set)}
}

// ingsTabOf is the tab a ?tab= value names (the first for anything else).
func ingsTabOf(v string) string {
	if slices.ContainsFunc(ingsTabs, func(t kit.TabItem) bool { return t.Value == v }) {
		return v
	}
	return ingsTabOverview
}

// ingsID reads and checks the {id} path parameter: an id that could not be an
// element id is not an ingestion (and must never reach an expression).
func ingsID(r *http.Request) (string, error) {
	id := chi.URLParam(r, "id")
	if !kit.ValidID(id) {
		return "", httperror.NotFound("Ingestion", id)
	}
	return id, nil
}

// loadIngestion reads an ingestion with GET /api/ingestions/{id}'s checks:
// the read permission and the ingestion's reach.
func (h *Ingestions) loadIngestion(ctx context.Context, ac *auth.AuthContext, id string) (*ingestion.Ingestion, error) {
	if err := auth.CanReadIngestions(ac); err != nil {
		return nil, err
	}
	return h.reachable(ctx, ac, id)
}

// reachable is the ingestion with its reach checked, the by-id rule every
// write use case repeats. Writes call it for the page that follows the
// action, not as their own gate (the use case is that).
func (h *Ingestions) reachable(ctx context.Context, ac *auth.AuthContext, id string) (*ingestion.Ingestion, error) {
	i, err := h.Store.FindByID(ctx, id)
	if err != nil {
		return nil, usecase.Internal("REPO", "find_by_id failed", err)
	}
	if i == nil {
		return nil, httperror.NotFound("Ingestion", id)
	}
	if err := ingestion.CheckReach(ac, i); err != nil {
		return nil, err
	}
	return i, nil
}

// owners reads the application and client an ingestion is shown with: the
// application's code and name, the client's identifier and name.
func (h *Ingestions) owners(ctx context.Context, i *ingestion.Ingestion) (*application.Application, string, string, error) {
	app, err := h.Apps.FindByID(ctx, string(i.ApplicationID))
	if err != nil || app == nil {
		return nil, "", "", usecase.Internal("REPO", "application lookup failed", err)
	}
	c, err := h.ClientNames.FindByID(ctx, string(i.ClientID))
	if err != nil || c == nil {
		return nil, "", "", usecase.Internal("REPO", "client lookup failed", err)
	}
	return app, c.Identifier, c.Name, nil
}

// buildDetail shapes the drawer. The compare pair is the versions the
// Versions tab shows side by side (0 for none).
func (h *Ingestions) buildDetail(ctx context.Context, ac *auth.AuthContext, i *ingestion.Ingestion, tab string, a, b int) (ingsDetail, error) {
	app, clientIdentifier, clientName, err := h.owners(ctx, i)
	if err != nil {
		return ingsDetail{}, err
	}
	kinds, reported, err := h.kindPerms(ctx)
	if err != nil {
		return ingsDetail{}, err
	}
	carries, err := h.Store.AnyLiveRuntimeCarries(ctx)
	if err != nil {
		return ingsDetail{}, usecase.Internal("REPO", "live runtimes failed", err)
	}
	versions, err := h.Store.ListVersions(ctx, i.ID)
	if err != nil {
		return ingsDetail{}, usecase.Internal("REPO", "list_versions failed", err)
	}
	def := i.Definition
	kind := ingsKindOf(def.Source)
	perms := ingsPermsFor(ac)
	// Where the data comes from, the delivery, the secrets and the warnings are
	// the effective definition's: the settings of the config entry the source
	// names, when it names one. The export keeps the reference.
	live, err := h.Store.Resolve(ctx, app.Code, i.ClientID, i.Code, def)
	if err != nil {
		return ingsDetail{}, usecase.Internal("REPO", "config entry lookup failed", err)
	}
	// The entry's settings are shown only to someone who can update ingestions:
	// anyone else sees the reference where the effective definition would say
	// them (the source, the delivery URL the entry gives, the secrets it binds).
	_, byConfig := def.Source.(ingestion.Configured)
	withheld := byConfig && auth.CanUpdateIngestions(ac) != nil
	shown := live
	if withheld {
		shown = live.Redacted(def)
	}
	effective := shown.Definition
	if withheld && effective.Target.TargetURL == "" {
		effective.Target.TargetURL = "In the settings"
	}
	d := ingsDetail{
		ID: i.ID, Code: i.Code, Name: def.Name, Application: app.Code, Client: clientName, ClientIdentifier: clientIdentifier,
		Active: i.Active, Draft: i.Version == 0, Set: deref(i.ManagedBy), Notice: i.Live() && !carries(kind),
		Version: i.Version, Latest: i.LatestVersion, Kind: ingsKindLabel(kind), Where: ingsWhere(def.Source, i.Code, shown),
		Split: ingsSplitSummary(def.Split), Target: effective.Target, KeepPayloads: def.KeepFailedPayloads,
		Created: i.CreatedAt.UTC().Format("2006-01-02 15:04:05 UTC"), Updated: i.UpdatedAt.UTC().Format("2006-01-02 15:04:05 UTC"),
		Tab: ingsTabOf(tab), Perms: perms, Base: ingsURL(i.ID),
	}
	if _, ok := def.Source.(ingestion.HTTP); ok {
		d.Endpoint = "POST /ingest/" + clientIdentifier + "/" + i.Code + " on each runtime"
	}
	notices, err := h.Store.NoticesOf(ctx, []string{i.ID})
	if err != nil {
		return ingsDetail{}, usecase.Internal("REPO", "notices failed", err)
	}
	d.Notices, d.LatestNotice = notices[i.ID].Count, notices[i.ID].LatestMessage
	fetches, err := h.Store.ListSecretFetches(ctx, i.ID)
	if err != nil {
		return ingsDetail{}, usecase.Internal("REPO", "list_secret_fetches failed", err)
	}
	if withheld {
		// The names, and what was fetched under them, come from the entry.
		d.SecretsWithheld = true
	} else {
		bound := *i
		bound.Definition = effective
		d.Secrets = ingsSecretRows(&bound, fetches)
		// An editor is shown which values exist and where to set them. A value
		// is set on the platform configuration page, not here.
		if perms.Update {
			if err := h.linkSecrets(ctx, d.Secrets, app.Code, i.ClientID, clientIdentifier); err != nil {
				return ingsDetail{}, err
			}
		}
	}
	if shown.Runnable() {
		d.Warnings = shown.Definition.Warnings()
	}
	if d.Config, err = h.configView(ctx, ac, i, app.Code, clientName, live, kinds, reported); err != nil {
		return ingsDetail{}, err
	}
	d.EditBlock = ingsEditBlock(ac, i, kinds)
	d.CanChange = perms.Update && d.EditBlock == ""
	d.CanRemove = perms.Delete && i.ManagedBy == nil
	d.CanLive = perms.Promote && i.ManagedBy == nil
	file := func(def ingestion.Definition) (string, error) {
		y, err := ingestion.File{Application: app.Code, Client: clientIdentifier, Code: i.Code, Active: &i.Active, Definition: def}.YAML()
		if err != nil {
			return "", usecase.Internal("MARSHAL", "definition does not render", err)
		}
		return string(y), nil
	}
	if d.YAML, err = file(def); err != nil {
		return ingsDetail{}, err
	}
	byNumber := map[int]ingestion.Version{}
	for _, v := range versions {
		byNumber[v.Number] = v
		row := ingsVersionRow{
			Number: v.Number, Live: v.Number == i.Version, Draft: v.Number > i.Version, Note: deref(v.Note), By: deref(v.CreatedBy), Applied: deref(v.AppliedFrom),
			Created: v.CreatedAt.UTC().Format("2006-01-02 15:04"), CreatedISO: v.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
			MakeLive: d.CanLive && v.Number != i.Version,
		}
		if d.CanChange {
			row.EditHref = d.Base + "/edit?from=" + strconv.Itoa(v.Number)
		}
		d.Versions = append(d.Versions, row)
	}
	if va, ok := byNumber[a]; ok && a != 0 {
		if vb, ok := byNumber[b]; ok && b != 0 {
			cmpn := &ingsComparison{A: a, B: b, ALive: a == i.Version, BLive: b == i.Version, Same: va.Definition.Equal(vb.Definition)}
			if cmpn.AYAML, err = file(va.Definition); err != nil {
				return ingsDetail{}, err
			}
			if cmpn.BYAML, err = file(vb.Definition); err != nil {
				return ingsDetail{}, err
			}
			d.Compare = cmpn
		}
	}
	return d, nil
}

// compareOf reads the ?a and ?b version numbers (0 when absent or not one).
func compareOf(v url.Values) (a, b int) {
	a, _ = strconv.Atoi(v.Get("a"))
	b, _ = strconv.Atoi(v.Get("b"))
	return max(a, 0), max(b, 0)
}

// detail opens the drawer: patched into the page for Datastar (a row's code
// or View details), or the list page with the drawer open for a navigation to
// /ui/ingestions/{id}.
func (h *Ingestions) detail(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	id, err := ingsID(r)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	i, err := h.loadIngestion(ctx, ac, id)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	a, b := compareOf(r.URL.Query())
	d, err := h.buildDetail(ctx, ac, i, r.URL.Query().Get("tab"), a, b)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	if ds.IsDatastar(r) {
		sse := ds.NewSSE(w, r)
		if err := openIngsDrawer(sse, d); err != nil {
			slog.DebugContext(ctx, "ui: ingestion drawer patch not delivered", "err", err)
		}
		return
	}
	view, err := h.listView(ctx, ac, parseIngsQuery(nil))
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	view.Drawer = &d
	h.renderPage(w, r, http.StatusOK, view)
}

// openIngsDrawer patches the drawer's content for d, selects it and moves
// focus to its heading (drawer.md, server-loaded record).
func openIngsDrawer(sse *ds.SSE, d ingsDetail) error {
	return errors.Join(
		sse.Patch(ingsDrawerContent(d), ds.Inner("#"+ingsDrawerID)),
		sse.PatchSignals(map[string]any{ingsDrawerSig: d.ID, ingsTabSig: d.Tab}),
		sse.Focus(ingsDrawerID+"-title"),
	)
}

// ingsHidden are the hidden fields every action inside the drawer carries: the
// record the drawer shows (the drawer's signal) and the tab it is on, so the
// answer can refresh the drawer where the user is.
func ingsHidden() []kit.HiddenField {
	return []kit.HiddenField{drawerField(ingsDrawerSig), {Name: "tab", Attrs: templ.Attributes{"data-attr:value": "$" + kit.Signal(ingsTabSig)}}}
}

// afterChange re-renders what a change to ingestion id affects: the table of
// the list the user is on and the drawer when it shows this ingestion (the
// form's hidden "drawer" field), on the tab it is on. A deleted ingestion's
// drawer is closed. A caller who may write but not read gets neither: the
// reads are the API's.
func (h *Ingestions) afterChange(sse *ds.SSE, r *http.Request, ac *auth.AuthContext, id string, deleted bool) {
	ctx := r.Context()
	if auth.CanReadIngestions(ac) != nil {
		return
	}
	if view, err := h.listView(ctx, ac, h.refererQuery(r)); err != nil {
		h.Shell.FailSSE(sse, r, err)
	} else if err := view.patch(sse); err != nil {
		slog.DebugContext(ctx, "ui: ingestions table patch not delivered", "err", err)
	}
	if r.PostFormValue("drawer") != id {
		return
	}
	if deleted {
		if err := sse.PatchSignals(map[string]any{ingsDrawerSig: ""}); err != nil {
			slog.DebugContext(ctx, "ui: ingestion drawer close not delivered", "err", err)
		}
		return
	}
	i, err := h.loadIngestion(ctx, ac, id)
	if err != nil {
		h.Shell.FailSSE(sse, r, err)
		return
	}
	d, err := h.buildDetail(ctx, ac, i, r.PostFormValue("tab"), 0, 0)
	if err != nil {
		h.Shell.FailSSE(sse, r, err)
		return
	}
	if err := errors.Join(sse.Patch(ingsDrawerContent(d), ds.Inner("#"+ingsDrawerID)), sse.PatchSignals(map[string]any{ingsTabSig: d.Tab})); err != nil {
		slog.DebugContext(ctx, "ui: ingestion drawer patch not delivered", "err", err)
	}
}

// refererQuery is the list view the request came from (live filtering keeps
// the address current), so an action re-renders the table the user sees.
func (h *Ingestions) refererQuery(r *http.Request) ingsQuery {
	ref, err := url.Parse(r.Referer())
	if err != nil || ref.Path != ingsPath {
		return parseIngsQuery(nil)
	}
	return parseIngsQuery(ref.Query())
}

// nativeDone answers a successful native post: a redirect to the ingestion's
// drawer on the tab the form came from (the list after a delete) with the
// flash key.
func (h *Ingestions) nativeDone(w http.ResponseWriter, r *http.Request, id, flash string, deleted bool) {
	h.Shell.SetFlash(w, flash)
	target := ingsPath
	if !deleted {
		target = ingsURL(id) + "?tab=" + ingsTabOf(r.PostFormValue("tab"))
	}
	// The path is under /ui and the id passed kit.ValidID.
	http.Redirect(w, r, target, http.StatusSeeOther) //nolint:gosec // G710: a /ui path built from a validated id
}

// ── tabs that are IngestionTabs': filled when shown ────────────────────

// lazyTab fills an Activity, Failures or History tab the first time it is
// shown: the tab's placeholder (IngestionActivityTab and its siblings), which
// asks IngestionTabs for the panel. It makes the drawer's own checks first.
func (h *Ingestions) lazyTab(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	id, err := ingsID(r)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	tab := chi.URLParam(r, "tab")
	if !slices.Contains(ingsLazyTabs, tab) {
		h.Shell.Fail(w, r, httperror.NotFound("IngestionTab", tab))
		return
	}
	if _, err := h.loadIngestion(ctx, ac, id); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	if !ds.IsDatastar(r) {
		http.Redirect(w, r, ingsURL(id)+"?tab="+tab, http.StatusSeeOther) //nolint:gosec // G710: a /ui path built from a validated id and a listed tab
		return
	}
	if err := ds.NewSSE(w, r).Patch(ingsLazyContent(id, tab), ds.Inner("#"+ingsSlotID(tab))); err != nil {
		slog.DebugContext(ctx, "ui: ingestion tab patch not delivered", "tab", tab, "err", err)
	}
}

func ingsSlotID(tab string) string { return "ings-" + tab + "-slot" }

// ── compare ────────────────────────────────────────────────────────────

// compare shows two saved versions side by side (the Versions tab's form).
func (h *Ingestions) compare(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	id, err := ingsID(r)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	i, err := h.loadIngestion(ctx, ac, id)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	a, b := compareOf(r.URL.Query())
	if !ds.IsDatastar(r) {
		http.Redirect(w, r, fmt.Sprintf("%s?tab=versions&a=%d&b=%d", ingsURL(id), a, b), http.StatusSeeOther) //nolint:gosec // G710: a /ui path built from a validated id and numbers
		return
	}
	d, err := h.buildDetail(ctx, ac, i, "versions", a, b)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	if d.Compare == nil {
		h.Shell.Fail(w, r, usecase.Validation("VERSIONS_REQUIRED", "Choose two saved versions to compare."))
		return
	}
	if err := ds.NewSSE(w, r).Patch(ingsCompareC(d)); err != nil {
		slog.DebugContext(ctx, "ui: ingestion comparison patch not delivered", "err", err)
	}
}

// ── export ─────────────────────────────────────────────────────────────

// export downloads the ingestion as the definition file `fcdev ingest apply`
// reads (GET /api/ingestions/{id}/definition).
func (h *Ingestions) export(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	id, err := ingsID(r)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	i, err := h.loadIngestion(ctx, ac, id)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	app, clientIdentifier, _, err := h.owners(ctx, i)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	// The client is written by identifier, which ports between environments.
	yaml, err := ingestion.File{Application: app.Code, Client: clientIdentifier, Code: i.Code, Active: &i.Active, Definition: i.Definition}.YAML()
	if err != nil {
		h.Shell.Fail(w, r, usecase.Internal("MARSHAL", "definition does not render", err))
		return
	}
	w.Header().Set("Content-Type", "application/yaml")
	w.Header().Set("Content-Disposition", `attachment; filename="`+i.Code+`.yaml"`)
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(yaml)
}

// ingsSecretRow is one secret the drawer's Secrets section shows: the name,
// whether the live definition binds it, and which runtime last fetched it and
// when. A runtime's fetch is security telemetry, not an audited reveal (owner
// ruling 2026-10-25); this is where it shows.
type ingsSecretRow struct {
	Name  string
	Bound bool
	// Fetches are the runtimes that fetched it, newest first.
	Fetches []ingsSecretFetchRow
	// Href is the platform configuration page that sets the value, shown to an
	// editor only; Linked says Href and Set are filled, Set that a value exists.
	Href   string
	Linked bool
	Set    bool
}

type ingsSecretFetchRow struct {
	RuntimeID, At, AtISO string
	Version              int
	Count                int64
}

func ingsSecretRows(i *ingestion.Ingestion, fetches []ingestion.SecretFetch) []ingsSecretRow {
	bound := ingestion.BoundSecrets(i, fetches)
	rows := make([]ingsSecretRow, 0, len(bound))
	for _, b := range bound {
		row := ingsSecretRow{Name: b.Name, Bound: b.Bound}
		for _, f := range b.Fetches {
			row.Fetches = append(row.Fetches, ingsSecretFetchRow{
				RuntimeID: f.RuntimeID, Version: f.Version, Count: f.FetchCount,
				At: f.LastFetchedAt.UTC().Format("2006-01-02 15:04:05 UTC"), AtISO: f.LastFetchedAt.UTC().Format("2006-01-02T15:04:05Z"),
			})
		}
		rows = append(rows, row)
	}
	return rows
}

// linkSecrets marks which of the rows' secrets have a value at the client and
// points each at the configuration property that holds it: the edit form for a
// value that exists, the add form with the coordinates filled in for one that
// does not. A name no longer bound is left alone.
func (h *Ingestions) linkSecrets(ctx context.Context, rows []ingsSecretRow, application string, clientID ids.ClientID, client string) error {
	var names []string
	for _, r := range rows {
		if r.Bound {
			names = append(names, r.Name)
		}
	}
	missing, err := h.Store.MissingSecrets(ctx, application, clientID, names)
	if err != nil {
		return usecase.Internal("REPO", "secret lookup failed", err)
	}
	for k := range rows {
		if !rows[k].Bound {
			continue
		}
		rows[k].Linked = true
		rows[k].Set = !slices.Contains(missing, rows[k].Name)
		query := cfgQuery(application, ingestion.SecretsSection, rows[k].Name, client)
		if rows[k].Set {
			rows[k].Href = cfgPath + "/edit?" + query
		} else {
			rows[k].Href = cfgPath + "/new?" + query
		}
	}
	return nil
}

// ingsProblemTexts are an entry's problems as one line each.
func ingsProblemTexts(p ingestion.Problems) []string {
	out := make([]string, 0, len(p))
	for _, fe := range p {
		out = append(out, fe.Field+" "+fe.Message)
	}
	return out
}
