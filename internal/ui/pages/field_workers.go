package pages

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/a-h/templ"
	"github.com/go-chi/chi/v5"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/fieldidentity"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/shell"
	"github.com/flowcatalyst/framework/go/ui/ds"
	"github.com/flowcatalyst/framework/go/ui/kit"
	"github.com/flowcatalyst/go-sdk/usecase"
)

// FieldWorkers is the field workers page (/ui/field-workers): a client's
// field workers with their lock state and app grants, the record drawer
// (details, apps, sign-in history) and the supervisor actions of the admin
// API (ensure, grant and revoke apps, suspend, activate, unlock, reset PIN,
// delete), the badge section (issue, reissue, revoke, a printable card shown
// once), the bulk "Print badges" sheet, edit (name, email, the test and
// supervisor flags) and "Sign in as…" (a single-use console sign-in code).
// Reads make the API's check (CanReadFieldWorkers); every change
// CanManageFieldWorkers for the worker's client, as the API's handlers.
type FieldWorkers struct {
	Store   FieldStore
	Ops     FieldCommands
	Clients ClientStore
	Shell   *shell.Shell
}

const (
	fieldWorkersPath  = "/ui/field-workers"
	fieldWorkersForm  = "field-workers-list"
	fieldWorkersLive  = "@get('/ui/field-workers', {contentType: 'form'})"
	fieldWorkersTable = "field-workers-region"
	fieldAppFacet     = "app"
	fieldTestFacet    = "test"

	fwDrawerID   = "fw-drawer"
	fwDrawerSig  = "selFieldWorker"
	fwDialogSlot = "fw-dialog-slot"

	fwSuspendID  = "fw-suspend"
	fwActivateID = "fw-activate"
	fwUnlockID   = "fw-unlock"
	fwResetPinID = "fw-reset-pin"
	fwDeleteID   = "fw-delete"
	fwRevokeID   = "fw-revoke"
	fwUnlinkID   = "fw-unlink"

	fwBadgeIssueID   = "fw-badge-issue"
	fwBadgeReissueID = "fw-badge-reissue"
	fwBadgeRevokeID  = "fw-badge-revoke"
	fwPrintBadgesID  = "fw-print-badges"
	fwBadgeDialogID  = "fw-badge-reveal"
	// fwSelSig counts the ticked rows (the "Print badges" label).
	fwSelSig = "fwSel"

	fwSignInsID  = "fw-history"
	fwSignInRows = 10
)

var fieldWorkerList = &listSpec{
	path:        fieldWorkersPath,
	sortKeys:    []string{"created"}, // the API orders newest first
	statusOK:    func(s string) bool { return s == "ACTIVE" || s == "DISABLED" },
	facets:      []string{fieldClientParam, fieldAppFacet, fieldTestFacet},
	rows:        []int{25, 50, 100},
	defaultRows: 25,
}

var fieldWorkerStatusOptions = []kit.Option{{Value: "ACTIVE", Label: "Active"}, {Value: "DISABLED", Label: "Suspended"}}

// fieldWorkerTestOptions is the isTest filter of GET /api/field-workers.
var fieldWorkerTestOptions = []kit.Option{{Value: "true", Label: "Test identities"}, {Value: "false", Label: "Real workers"}}

// Mount registers the page's routes (POSTs behind parseForm).
func (h *FieldWorkers) Mount(r chi.Router) {
	r.Get("/field-workers", h.list)
	r.Get("/field-workers/new", h.newForm)
	r.Get("/field-workers/{id}", h.detail)
	r.Get("/field-workers/{id}/sign-ins", h.signIns)
	r.Get("/field-workers/{id}/grant", h.grantForm)
	r.Get("/field-workers/{id}/edit", h.editForm)
	r.Get("/field-workers/{id}/sign-in-as", h.signInAsForm)
	r.Get("/field-workers/{id}/link", h.linkForm)
	post := r.With(parseForm)
	post.Post("/field-workers/new", h.create)
	post.Post("/field-workers/badges/print", h.printBadges)
	post.Post("/field-workers/{id}/edit", h.edit)
	post.Post("/field-workers/{id}/badge", h.issueBadge)
	post.Post("/field-workers/{id}/badge/revoke", h.revokeBadge)
	post.Post("/field-workers/{id}/sign-in-as", h.signInAs)
	post.Post("/field-workers/{id}/link", h.link)
	post.Post("/field-workers/{id}/unlink", h.unlink)
	post.Post("/field-workers/{id}/grant", h.grant)
	post.Post("/field-workers/{id}/suspend", h.suspend)
	post.Post("/field-workers/{id}/activate", h.activate)
	post.Post("/field-workers/{id}/unlock", h.unlock)
	post.Post("/field-workers/{id}/reset-pin", h.resetPin)
	post.Post("/field-workers/{id}/delete", h.delete)
	post.Post("/field-workers/{id}/apps/{code}/revoke", h.revoke)
}

// ── view ───────────────────────────────────────────────────────────────

type fieldWorkerApp struct {
	Code, Name, Title string
	Granted           string // "granted <time> (<source>)", without the code
	RevokeAction      string
}

type fieldWorkerRow struct {
	ID, Code, Name, Email string
	Status                string
	StatusSeverity        kit.TagSeverity
	Lock, LockTitle       string
	LockUntil             time.Time
	LockSeverity          kit.TagSeverity
	Locked, Suspended     bool
	NoPin                 bool
	IsTest, Supervisor    bool
	Apps                  []fieldWorkerApp
	LastLogin, Created    time.Time
	DetailHref, Base      string
}

type fieldWorkersView struct {
	Query         listQuery
	Client        string
	ClientName    string
	ClientOptions []kit.Option
	AppOptions    []kit.Option
	App           string
	Test          string // "true", "false" or "" (the isTest filter)
	Rows          []fieldWorkerRow
	Total         int
	Page, Pages   int
	CanManage     bool
	Dialog        templ.Component
	Drawer        *fieldWorkerDetail
}

func fieldWorkerRowOf(i *fieldidentity.Identity, idx fieldApps, canManage bool, now time.Time) fieldWorkerRow {
	base := fieldWorkersPath + "/" + url.PathEscape(i.ID)
	row := fieldWorkerRow{
		ID: i.ID, Code: i.WorkerCode, Name: i.Name, Email: i.Email,
		Status: "Active", StatusSeverity: kit.TagSuccess, Suspended: i.Status == fieldidentity.StatusDisabled,
		NoPin: !i.HasPIN(), DetailHref: base, Base: base, Locked: i.IsLocked(now),
		IsTest: i.IsTest, Supervisor: i.CanImpersonate,
	}
	if row.Suspended {
		row.Status, row.StatusSeverity = "Suspended", kit.TagDanger
	}
	switch {
	case i.HardLocked:
		row.Lock, row.LockSeverity, row.LockTitle = "Hard locked", kit.TagDanger, "Locked until a supervisor unlocks or resets the PIN"
	case i.LockedUntil != nil && i.LockedUntil.After(now):
		row.Lock, row.LockSeverity, row.LockTitle = "Soft locked", kit.TagWarn, "Soft locked until "+fieldTime(*i.LockedUntil)
		row.LockUntil = *i.LockedUntil
	}
	if i.LastLoginAt != nil {
		row.LastLogin = *i.LastLoginAt
	}
	row.Created = i.CreatedAt
	for _, g := range i.Apps {
		code, name := idx.ref(g.AppID)
		granted := "granted " + fieldTime(g.GrantedAt) + " (" + g.Source + ")"
		a := fieldWorkerApp{Code: code, Name: name, Title: code + " · " + granted, Granted: granted}
		if canManage {
			a.RevokeAction = base + "/apps/" + url.PathEscape(code) + "/revoke"
		}
		row.Apps = append(row.Apps, a)
	}
	return row
}

// view reads the page for the request's query with the API's read check.
func (h *FieldWorkers) view(ctx context.Context, ac *auth.AuthContext, q listQuery) (fieldWorkersView, error) {
	opts, err := portalClientOptions(ctx, h.Clients, ac)
	if err != nil {
		return fieldWorkersView{}, err
	}
	clientID, opts := portalSelectedClient(ac, opts, first(q.facet(fieldClientParam)))
	wanted := first(q.facet(fieldAppFacet))
	wantedTest := first(q.facet(fieldTestFacet))
	q = q.withFacet(fieldClientParam, nil).withFacet(fieldAppFacet, nil).withFacet(fieldTestFacet, nil)
	v := fieldWorkersView{Query: q, Client: clientID, ClientOptions: opts, ClientName: portalClientName(opts, clientID), Page: 1, Pages: 1}
	if clientID == "" {
		return v, nil
	}
	if err := auth.CanReadFieldWorkers(ac, clientID); err != nil {
		return fieldWorkersView{}, err
	}
	v.CanManage = auth.CanManageFieldWorkers(ac, clientID) == nil
	idx, err := loadFieldApps(ctx, h.Store)
	if err != nil {
		return fieldWorkersView{}, err
	}
	filter := fieldidentity.IdentityFilter{
		ClientID: clientID, Query: q.Q, Status: fieldidentity.Status(q.Status), AppIDs: fieldCallerApps(ac),
	}
	// The app filter is one of the applications the caller may work with; a
	// hand-edited value is dropped from the canonical URL.
	for _, a := range idx.reachable(ac, false) {
		v.AppOptions = append(v.AppOptions, kit.Option{Value: a.Code, Label: a.Name + " (" + a.Code + ")"})
		if a.Code == wanted {
			filter.AppID, v.App = string(a.ID), wanted
		}
	}
	// The isTest filter takes the API's two values; anything else is dropped.
	if wantedTest == "true" || wantedTest == "false" {
		isTest := wantedTest == "true"
		filter.IsTest, v.Test = &isTest, wantedTest
	}
	q = q.withFacet(fieldClientParam, []string{clientID})
	if v.App != "" {
		q = q.withFacet(fieldAppFacet, []string{v.App})
	}
	if v.Test != "" {
		q = q.withFacet(fieldTestFacet, []string{v.Test})
	}
	rows, total, err := h.search(ctx, &q, filter)
	if err != nil {
		return fieldWorkersView{}, err
	}
	v.Query, v.Total, v.Page = q, int(total), q.Page
	_, v.Pages, _, _ = q.pageBounds(v.Total)
	now := time.Now().UTC()
	for n := range rows {
		v.Rows = append(v.Rows, fieldWorkerRowOf(&rows[n], idx, v.CanManage, now))
	}
	return v, nil
}

// search reads the page of workers q asks for, moving q to the last page
// when it is past the end.
func (h *FieldWorkers) search(ctx context.Context, q *listQuery, filter fieldidentity.IdentityFilter) ([]fieldidentity.Identity, int64, error) {
	filter.Offset, filter.Limit = (q.Page-1)*q.Rows, q.Rows
	rows, total, err := h.Store.Workers(ctx, filter)
	if err != nil {
		return nil, 0, usecase.Internal("REPO", "search_field_workers failed", err)
	}
	if page, _, _, _ := q.pageBounds(int(total)); page != q.Page {
		q.Page = page
		filter.Offset = (page - 1) * q.Rows
		if rows, total, err = h.Store.Workers(ctx, filter); err != nil {
			return nil, 0, usecase.Internal("REPO", "search_field_workers failed", err)
		}
	}
	return rows, total, nil
}

func (h *FieldWorkers) renderPage(w http.ResponseWriter, r *http.Request, status int, v fieldWorkersView) {
	h.Shell.RenderStatus(w, r, status, shell.Page{Title: "Field Workers", Path: fieldWorkersPath}, fieldWorkersPage(v))
}

// patchTable replaces the table and the pager; the ticked rows are gone with
// the old table, so the selection count restarts.
func (v fieldWorkersView) patchTable(sse *ds.SSE) error {
	return errors.Join(sse.Patch(fieldWorkersRegion(v), ds.WithMode(ds.ModeReplace)), sse.Patch(kit.Paginator(v.pager())),
		sse.PatchSignals(map[string]any{fwSelSig: 0}))
}

func (v fieldWorkersView) url() string { return v.Query.URL() }

func (v fieldWorkersView) filterCount() int {
	n := 0
	if v.App != "" {
		n++
	}
	if v.Test != "" {
		n++
	}
	if v.Query.Status != "" {
		n++
	}
	return n
}

// clearURL is the view without search and filters, on the same client.
func (v fieldWorkersView) clearURL() string {
	return listQuery{spec: fieldWorkerList, Sort: v.Query.Sort, Dir: v.Query.Dir, Page: 1, Rows: v.Query.Rows}.
		withFacet(fieldClientParam, []string{v.Client}).URL()
}

func (v fieldWorkersView) filtered() bool { return v.Query.Q != "" || v.filterCount() > 0 }

func (v fieldWorkersView) confirm(id, title, message, label string, tone kit.ConfirmTone) kit.ConfirmDialogProps {
	return kit.ConfirmDialogProps{
		ID: id, Action: fieldWorkersPath, Title: title, Message: message, ConfirmLabel: label, Tone: tone,
		Hidden:    []kit.HiddenField{drawerField(fwDrawerSig)},
		FormAttrs: templ.Attributes{"data-on:submit": kit.SubmitConfirm},
	}
}

func (v fieldWorkersView) drawerSignals() string {
	if v.Drawer == nil {
		return "{" + fwDrawerSig + ": '', fieldCopied: '', " + fwSelSig + ": 0}"
	}
	return "{" + fwDrawerSig + ": '" + kit.ID(v.Drawer.ID) + "', fieldCopied: '', " + fwSelSig + ": 0}"
}

// workerMenu is the actions a worker's state allows, shared by the row menu
// and the drawer: Suspend or Activate, Unlock (only while locked), Reset PIN,
// Delete.
type fieldAction struct {
	Icon, Label, Hint string
	Dialog, Action    string
	Destructive       bool
}

func workerActions(base string, suspended, locked bool) []fieldAction {
	var out []fieldAction
	if suspended {
		out = append(out, fieldAction{"circle-check", "Activate…", "Let this worker sign in again.", fwActivateID, base + "/activate", false})
	} else {
		out = append(out, fieldAction{"ban", "Suspend…", "Block sign-in and end every session.", fwSuspendID, base + "/suspend", false})
	}
	if locked {
		out = append(out, fieldAction{"lock", "Unlock…", "Clear the soft and hard locks and the failed-attempt counter.", fwUnlockID, base + "/unlock", false})
	}
	out = append(out,
		fieldAction{"key-round", "Reset PIN…", "Set a new temporary PIN (shown once) and end every session.", fwResetPinID, base + "/reset-pin", false},
		fieldAction{"trash-2", "Delete…", "Remove this worker and end their sessions. Cannot be undone.", fwDeleteID, base + "/delete", true},
	)
	return out
}

func (v fieldWorkersView) rowActions(row fieldWorkerRow) kit.RowActionsProps {
	p := kit.RowActionsProps{ID: "fw-" + row.ID + "-actions", RowLabel: row.Code}
	p.Menu = append(p.Menu, kit.RowAction{Icon: "eye", Label: "View details", Href: row.DetailHref, Attrs: templ.Attributes{"data-on:click__prevent": liveLink}})
	if !v.CanManage {
		return p
	}
	p.Menu = append(p.Menu, kit.RowAction{Icon: "pencil", Label: "Edit…", Href: row.Base + "/edit", Attrs: templ.Attributes{"data-on:click__prevent": liveLink, "aria-haspopup": "dialog"}})
	for _, a := range workerActions(row.Base, row.Suspended, row.Locked) {
		p.Menu = append(p.Menu, kit.RowAction{Icon: a.Icon, Label: a.Label, Destructive: a.Destructive, Attrs: confirmTrigger(a.Dialog, a.Action)})
	}
	return p
}

// ── list ───────────────────────────────────────────────────────────────

func (h *FieldWorkers) list(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	v, err := h.view(ctx, auth.FromContext(ctx), parseListQuery(r.URL.Query(), fieldWorkerList))
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	if ds.IsDatastar(r) {
		sse := ds.NewSSE(w, r)
		if err := errors.Join(v.patchTable(sse), sse.Patch(kit.TableToolbarState(v.toolbar().State())), sse.ReplaceURL(v.url())); err != nil {
			slog.DebugContext(ctx, "ui: field workers patch not delivered", "err", err)
		}
		return
	}
	h.renderPage(w, r, http.StatusOK, v)
}

// ── loading ────────────────────────────────────────────────────────────

func fieldWorkerID(r *http.Request) (string, error) {
	id := chi.URLParam(r, "id")
	if !kit.ValidID(id) {
		return "", httperror.NotFound("FieldWorker", id)
	}
	return id, nil
}

// loadWorker loads a worker for a by-id request with the API's checks: a
// missing permission is a 403; an unknown id, another client's worker or (for
// a confined caller) a worker outside its applications is a 404.
func (h *FieldWorkers) loadWorker(ctx context.Context, ac *auth.AuthContext, id string, write bool) (*fieldidentity.Identity, error) {
	gate := auth.CanReadFieldWorkers
	if write {
		gate = auth.CanManageFieldWorkers
	}
	return h.loadWorkerWith(ctx, ac, id, func(ac *auth.AuthContext, _ *fieldidentity.Identity, clientID string) error {
		return gate(ac, clientID)
	})
}

// loadWorkerWith is loadWorker with the permission chosen by the caller: the
// gate gets the worker (nil when unknown) so the permission may depend on it
// (a test identity needs only manage for a sign-in code), as the API's
// loadWorkerWith.
func (h *FieldWorkers) loadWorkerWith(ctx context.Context, ac *auth.AuthContext, id string,
	gate func(ac *auth.AuthContext, ident *fieldidentity.Identity, clientID string) error,
) (*fieldidentity.Identity, error) {
	ident, err := h.Store.Worker(ctx, id)
	if err != nil {
		return nil, usecase.Internal("REPO", "find_field_worker failed", err)
	}
	clientID := ""
	if ident != nil {
		clientID = ident.ClientID
	}
	if err := fieldGate(func(ac *auth.AuthContext, c string) error { return gate(ac, ident, c) }, ac, clientID, ident != nil, "FieldWorker", id); err != nil {
		return nil, err
	}
	if ac.IsApplicationScoped() && !hasReachableGrant(ac, ident) {
		return nil, httperror.NotFound("FieldWorker", id)
	}
	return ident, nil
}

// refOfWorker addresses a loaded worker, boxed to its own client (the API's
// refOf).
func refOfWorker(i *fieldidentity.Identity) fieldidentity.WorkerRef {
	return fieldidentity.WorkerRef{ID: i.ID, ClientID: i.ClientID}
}

// signInAsGate is the API's gate for a console sign-in code.
func signInAsGate(ac *auth.AuthContext, ident *fieldidentity.Identity, clientID string) error {
	return auth.CanSignInAsFieldWorker(ac, clientID, ident != nil && ident.IsTest)
}

func hasReachableGrant(ac *auth.AuthContext, i *fieldidentity.Identity) bool {
	for _, g := range i.Apps {
		if ac.CanAccessApplication(g.AppID) {
			return true
		}
	}
	return false
}

// ── drawer ─────────────────────────────────────────────────────────────

type fieldSignIn struct {
	At                                   time.Time
	Device, App, Outcome, Reason, Method string
	// Actor is who signed in as the worker (Method ACT_AS or SIGN_IN_CODE):
	// a supervisor's name and code, a principal's name, else the id.
	Actor   string
	Success bool
}

// fieldBadgeView is the worker's ACTIVE badge as the drawer shows it (never
// the token: that is shown once, at issue).
type fieldBadgeView struct {
	Created, LastUsed time.Time
}

// fieldLinkView is the platform user a worker is linked to, as the drawer shows
// it. Name and Email are empty when the user no longer exists (the link has no
// foreign key).
type fieldLinkView struct {
	ID, Name, Email string
}

type fieldWorkerDetail struct {
	ID, Code, Name, Email, ClientID string
	Status                          string
	StatusSeverity                  kit.TagSeverity
	Suspended, Locked               bool
	Lock, LockTitle                 string
	LockUntil                       time.Time
	LockSeverity                    kit.TagSeverity
	HasPin, PinChangeRequired       bool
	FailedSinceSuccess              int
	LastLogin, Created, Updated     time.Time
	Apps                            []fieldWorkerApp
	CanManage, CanGrant             bool
	IsTest, Supervisor              bool
	Badge                           *fieldBadgeView
	// Link is the platform user the worker is linked to, nil when none.
	// LinkHref and UnlinkAction are set for a manager.
	Link                   *fieldLinkView
	LinkHref, UnlinkAction string
	// CanSignIn is the API's rule for a console sign-in code (impersonate, or
	// manage for a test identity; a human principal for a real worker) on an
	// active worker with an application to sign in to.
	CanSignIn               bool
	Base, GrantHref         string
	EditHref, SignInAs      string
	SignIns                 []fieldSignIn
	SignInTotal             int
	SignInPage, SignInPages int
	PrevHref, NextHref      string
}

func (d fieldWorkerDetail) drawerProps() kit.DrawerProps {
	title := d.Name
	if title == "" {
		title = d.Code
	}
	return kit.DrawerProps{ID: fwDrawerID, Signal: fwDrawerSig, Label: "Field worker", Title: title, Subtitle: d.Code, Extras: fieldWorkerStatusTag(d.Status, d.StatusSeverity)}
}

func (d fieldWorkerDetail) actions() []fieldAction {
	if !d.CanManage {
		return nil
	}
	return workerActions(d.Base, d.Suspended, d.Locked)
}

// detailFor builds the drawer for a worker, with page signPage (0-based) of
// the sign-in history.
func (h *FieldWorkers) detailFor(ctx context.Context, ac *auth.AuthContext, i *fieldidentity.Identity, signPage int) (fieldWorkerDetail, error) {
	idx, err := loadFieldApps(ctx, h.Store)
	if err != nil {
		return fieldWorkerDetail{}, err
	}
	now := time.Now().UTC()
	canManage := auth.CanManageFieldWorkers(ac, i.ClientID) == nil
	row := fieldWorkerRowOf(i, idx, canManage, now)
	d := fieldWorkerDetail{
		ID: i.ID, Code: i.WorkerCode, Name: i.Name, Email: i.Email, ClientID: i.ClientID,
		Status: row.Status, StatusSeverity: row.StatusSeverity, Suspended: row.Suspended, Locked: row.Locked,
		Lock: row.Lock, LockTitle: row.LockTitle, LockUntil: row.LockUntil, LockSeverity: row.LockSeverity,
		HasPin: i.HasPIN(), PinChangeRequired: i.PINChangeRequired, FailedSinceSuccess: i.FailedSinceSuccess,
		LastLogin: row.LastLogin, Created: i.CreatedAt, Updated: i.UpdatedAt, Apps: row.Apps,
		CanManage: canManage, Base: row.Base, GrantHref: row.Base + "/grant", EditHref: row.Base + "/edit", SignInAs: row.Base + "/sign-in-as",
		IsTest: i.IsTest, Supervisor: i.CanImpersonate,
	}
	if i.LinkedPrincipalID != "" {
		refs, err := h.Store.PrincipalRefs(ctx, []string{i.LinkedPrincipalID})
		if err != nil {
			return fieldWorkerDetail{}, usecase.Internal("REPO", "find_principals failed", err)
		}
		p := refs[i.LinkedPrincipalID]
		d.Link = &fieldLinkView{ID: i.LinkedPrincipalID, Name: p.Name, Email: p.Email}
	}
	if canManage {
		d.LinkHref, d.UnlinkAction = row.Base+"/link", row.Base+"/unlink"
	}
	if b, err := h.Store.ActiveBadge(ctx, i.ID); err != nil {
		return fieldWorkerDetail{}, usecase.Internal("REPO", "find_field_badge failed", err)
	} else if b != nil {
		d.Badge = &fieldBadgeView{Created: b.CreatedAt}
		if b.LastUsedAt != nil {
			d.Badge.LastUsed = *b.LastUsedAt
		}
	}
	d.CanSignIn = !row.Suspended && len(h.signInApps(ac, i, idx)) > 0 && signInAsGate(ac, i, i.ClientID) == nil
	for _, a := range idx.reachable(ac, true) {
		if !i.HasApp(string(a.ID)) {
			d.CanGrant = true
			break
		}
	}
	signPage = max(signPage, 0)
	attempts, total, err := h.Store.SignIns(ctx, i.ID, signPage*fwSignInRows, fwSignInRows)
	if err != nil {
		return fieldWorkerDetail{}, usecase.Internal("REPO", "list_field_sign_ins failed", err)
	}
	actors := map[string]string{}
	for _, a := range attempts {
		s := fieldSignIn{At: a.AttemptedAt, Device: a.DeviceID, Outcome: "Success", Reason: a.FailureReason, Method: a.AuthMethod, Success: a.Outcome == fieldidentity.OutcomeSuccess}
		if a.ActorID != "" {
			s.Actor = h.actorLabel(ctx, a.ActorID, actors)
		}
		if !s.Success {
			s.Outcome = "Failure"
		}
		if a.ApplicationID != "" {
			s.App, _ = idx.ref(a.ApplicationID)
		}
		d.SignIns = append(d.SignIns, s)
	}
	d.SignInTotal, d.SignInPage = int(total), signPage+1
	d.SignInPages = max((d.SignInTotal+fwSignInRows-1)/fwSignInRows, 1)
	if signPage > 0 {
		d.PrevHref = d.Base + "/sign-ins?page=" + strconv.Itoa(signPage-1)
	}
	if d.SignInPage < d.SignInPages {
		d.NextHref = d.Base + "/sign-ins?page=" + strconv.Itoa(signPage+1)
	}
	return d, nil
}

func signPageOf(r *http.Request) int {
	n, err := strconv.Atoi(r.URL.Query().Get("page"))
	if err != nil || n < 0 || n > 100000 {
		return 0
	}
	return n
}

// actorLabel names the actor of an impersonated sign-in: a supervisor by name
// and worker code, a platform principal by name, else the id (cached per
// request, so a page of attempts looks each actor up once).
func (h *FieldWorkers) actorLabel(ctx context.Context, id string, seen map[string]string) string {
	if l, ok := seen[id]; ok {
		return l
	}
	label := id
	if w, err := h.Store.Worker(ctx, id); err == nil && w != nil {
		label = w.Name + " (" + w.WorkerCode + ")"
	} else if name, found, err := h.Store.PrincipalName(ctx, id); err == nil && found && name != "" {
		label = name
	}
	seen[id] = label
	return label
}

// signInApps are the worker's granted, active applications the caller may
// work with: the ones a sign-in code can be made for.
func (h *FieldWorkers) signInApps(ac *auth.AuthContext, i *fieldidentity.Identity, idx fieldApps) []kit.Option {
	var out []kit.Option
	for _, a := range idx.reachable(ac, true) {
		if i.HasApp(string(a.ID)) {
			out = append(out, kit.Option{Value: a.Code, Label: a.Name + " (" + a.Code + ")"})
		}
	}
	return out
}

func (h *FieldWorkers) detail(w http.ResponseWriter, r *http.Request) {
	h.showDetail(w, r, false)
}

func (h *FieldWorkers) signIns(w http.ResponseWriter, r *http.Request) {
	h.showDetail(w, r, true)
}

func (h *FieldWorkers) showDetail(w http.ResponseWriter, r *http.Request, onlySignIns bool) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	id, err := fieldWorkerID(r)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	i, err := h.loadWorker(ctx, ac, id, false)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	d, err := h.detailFor(ctx, ac, i, signPageOf(r))
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	if ds.IsDatastar(r) {
		sse := ds.NewSSE(w, r)
		var perr error
		if onlySignIns {
			perr = sse.Patch(fieldSignInsRegion(d), ds.WithMode(ds.ModeReplace))
		} else {
			perr = openFieldWorkerDrawer(sse, d)
		}
		if perr != nil {
			slog.DebugContext(ctx, "ui: field worker drawer patch not delivered", "err", perr)
		}
		return
	}
	v, err := h.view(ctx, ac, parseListQuery(nil, fieldWorkerList).withFacet(fieldClientParam, []string{i.ClientID}))
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	v.Drawer = &d
	h.renderPage(w, r, http.StatusOK, v)
}

func openFieldWorkerDrawer(sse *ds.SSE, d fieldWorkerDetail) error {
	return errors.Join(
		sse.Patch(fieldWorkerDrawerContent(d), ds.Inner("#"+fwDrawerID)),
		sse.PatchSignals(map[string]any{fwDrawerSig: d.ID}),
		sse.Focus(fwDrawerID+"-title"),
	)
}

// ── actions ────────────────────────────────────────────────────────────

func (h *FieldWorkers) suspend(w http.ResponseWriter, r *http.Request) {
	h.runAction(w, r, flashFieldWorkerSuspended, func(ctx context.Context, i *fieldidentity.Identity) (string, error) {
		return i.WorkerCode + " suspended", h.Ops.SuspendWorker(ctx, fieldidentity.WorkerRef{ID: i.ID, ClientID: i.ClientID})
	})
}

func (h *FieldWorkers) activate(w http.ResponseWriter, r *http.Request) {
	h.runAction(w, r, flashFieldWorkerActivated, func(ctx context.Context, i *fieldidentity.Identity) (string, error) {
		return i.WorkerCode + " activated", h.Ops.ActivateWorker(ctx, fieldidentity.WorkerRef{ID: i.ID, ClientID: i.ClientID})
	})
}

func (h *FieldWorkers) unlock(w http.ResponseWriter, r *http.Request) {
	h.runAction(w, r, flashFieldWorkerUnlocked, func(ctx context.Context, i *fieldidentity.Identity) (string, error) {
		return i.WorkerCode + " unlocked", h.Ops.UnlockWorker(ctx, fieldidentity.WorkerRef{ID: i.ID, ClientID: i.ClientID})
	})
}

func (h *FieldWorkers) delete(w http.ResponseWriter, r *http.Request) {
	h.runAction(w, r, flashFieldWorkerDeleted, func(ctx context.Context, i *fieldidentity.Identity) (string, error) {
		return i.WorkerCode + " deleted", h.Ops.DeleteWorker(ctx, fieldidentity.WorkerRef{ID: i.ID, ClientID: i.ClientID})
	})
}

// revoke removes one app grant; the API resolves the app by code.
func (h *FieldWorkers) revoke(w http.ResponseWriter, r *http.Request) {
	h.runAction(w, r, flashFieldWorkerAppRevoked, func(ctx context.Context, i *fieldidentity.Identity) (string, error) {
		code := chi.URLParam(r, "code")
		cmd := fieldidentity.WorkerAppCommand{AppCode: code}
		cmd.ID, cmd.ClientID = i.ID, i.ClientID
		err := h.Ops.RevokeWorkerApp(ctx, cmd)
		return i.WorkerCode + " no longer has access to " + code, err
	})
}

// runAction runs one confirmed action on a worker and refreshes the list and
// the drawer. The detail it returns replaces the flash key's text on a
// Datastar answer.
func (h *FieldWorkers) runAction(w http.ResponseWriter, r *http.Request, flash string, run func(ctx context.Context, i *fieldidentity.Identity) (string, error)) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	id, err := fieldWorkerID(r)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	i, err := h.loadWorker(ctx, ac, id, true)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	detail, err := run(ctx, i)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	if !ds.IsDatastar(r) {
		h.Shell.SetFlash(w, flash)
		http.Redirect(w, r, fieldWorkersPath+"?"+fieldClientParam+"="+url.QueryEscape(i.ClientID), http.StatusSeeOther) //nolint:gosec // G710: the target is a page constant; the client id is query-escaped and was checked by the manage check
		return
	}
	sse := ds.NewSSE(w, r)
	h.afterChange(sse, r, ac, i.ClientID, id, flash == flashFieldWorkerDeleted)
	if err := portalFlash(sse, flash, detail); err != nil {
		slog.DebugContext(ctx, "ui: field worker flash not delivered", "err", err)
	}
}

// afterChange re-renders the list the user is on and, when the drawer shows
// this worker, the drawer (closed when the worker is gone).
func (h *FieldWorkers) afterChange(sse *ds.SSE, r *http.Request, ac *auth.AuthContext, clientID, id string, deleted bool) {
	ctx := r.Context()
	q := refererList(r, fieldWorkerList).withFacet(fieldClientParam, []string{clientID})
	if v, err := h.view(ctx, ac, q); err != nil {
		h.Shell.FailSSE(sse, r, err)
	} else if err := v.patchTable(sse); err != nil {
		slog.DebugContext(ctx, "ui: field workers patch not delivered", "err", err)
	}
	if id == "" || r.PostFormValue("drawer") != id {
		return
	}
	if deleted {
		if err := sse.PatchSignals(map[string]any{fwDrawerSig: ""}); err != nil {
			slog.DebugContext(ctx, "ui: field worker drawer close not delivered", "err", err)
		}
		return
	}
	// The change is done; a worker the caller can no longer see (a confined
	// caller after a revoke) just closes the drawer.
	i, err := h.loadWorker(ctx, ac, id, false)
	var d fieldWorkerDetail
	if err == nil {
		d, err = h.detailFor(ctx, ac, i, 0)
	}
	if err != nil {
		if perr := sse.PatchSignals(map[string]any{fwDrawerSig: ""}); perr != nil {
			slog.DebugContext(ctx, "ui: field worker drawer close not delivered", "err", perr)
		}
		return
	}
	if err := sse.Patch(fieldWorkerDrawerContent(d), ds.Inner("#"+fwDrawerID)); err != nil {
		slog.DebugContext(ctx, "ui: field worker drawer patch not delivered", "err", err)
	}
}

// ── reset PIN ──────────────────────────────────────────────────────────

// fieldPinReveal is what the dialog after a PIN reset shows, once.
type fieldPinReveal struct{ Code, Name, PIN string }

// resetPin mirrors POST /api/field-workers/{id}/pin-reset without a
// supervisor-chosen PIN: the platform generates a temporary PIN. The PIN is in
// this response only: never in a URL, a redirect, a flash, a signal, a log or
// a fragment the page can fetch again. A native post gets the page itself
// (post/redirect/get would lose it); the response is no-store, and "Done"
// removes the dialog, so the PIN does not stay in the DOM.
func (h *FieldWorkers) resetPin(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	id, err := fieldWorkerID(r)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	i, err := h.loadWorker(ctx, ac, id, true)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	w = noStore{w}
	var cmd fieldidentity.ResetPinCommand
	cmd.ID, cmd.ClientID = i.ID, i.ClientID
	res, err := h.Ops.ResetWorkerPin(ctx, cmd)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	reveal := fieldPinReveal{Code: i.WorkerCode, Name: i.Name, PIN: res.TemporaryPIN}
	if !ds.IsDatastar(r) {
		v, err := h.view(ctx, ac, parseListQuery(nil, fieldWorkerList).withFacet(fieldClientParam, []string{i.ClientID}))
		if err != nil {
			h.Shell.Fail(w, r, err)
			return
		}
		v.Dialog = fieldPinDialog(reveal)
		h.renderPage(w, r, http.StatusOK, v)
		return
	}
	sse := ds.NewSSE(w, r)
	if err := patchDialogSlot(sse, fwDialogSlot, fieldPinDialog(reveal)); err != nil {
		slog.DebugContext(ctx, "ui: field worker PIN not delivered", "err", err)
	}
	h.afterChange(sse, r, ac, i.ClientID, id, false)
	// The flash names the worker, never the PIN.
	if err := portalFlash(sse, flashFieldWorkerPinReset, "PIN reset for "+i.WorkerCode); err != nil {
		slog.DebugContext(ctx, "ui: field worker flash not delivered", "err", err)
	}
}

// ── ensure (create) ────────────────────────────────────────────────────

const (
	fwFormDialog = "fw-form"
	fwFormSignal = "fwForm"
	fwGrantForm  = "fw-grant-form"
	fwGrantSig   = "fwGrantForm"
)

var fieldWorkerFieldCodes = map[string]string{
	"WORKER_CODE_REQUIRED": "workerCode", "WORKER_CODE_INVALID": "workerCode",
	"NAME_REQUIRED": "name", "EMAIL_INVALID": "email",
	"UNKNOWN_APPLICATION": "apps", "APPLICATION_FORBIDDEN": "apps",
}

type fieldWorkerForm struct {
	Client, ClientName      string
	WorkerCode, Name, Email string
	Apps                    []string
	AppChoices              []kit.Option
	// IsTest marks a test identity; CanImpersonate lets the worker act as
	// others. The latter is offered (and accepted) only for a caller with
	// platform:iam:field-worker:impersonate (MayImpersonate).
	IsTest, CanImpersonate, MayImpersonate bool
	Errs                                   formErrors
	Open                                   bool
}

func (f fieldWorkerForm) id(key string) string { return fieldID(fwFormDialog, key) }

func (f fieldWorkerForm) dialog() kit.FormDialogProps {
	return kit.FormDialogProps{
		ID: fwFormDialog, Signal: fwFormSignal, Action: fieldWorkersPath + "/new", Open: f.Open, Error: f.Errs.Form,
		Title: "New field worker", Description: "Create the worker in " + f.ClientName + ", or add application access to one that already exists. The worker cannot sign in until a PIN is set (Reset PIN).",
		SubmitLabel: "Create", Hidden: []kit.HiddenField{{Name: fieldClientParam, Value: f.Client}},
	}
}

func (f fieldWorkerForm) keys() []string {
	keys := []string{"workerCode", "name", "email", "apps", "isTest"}
	if f.MayImpersonate {
		keys = append(keys, "canImpersonate")
	}
	return keys
}

func (f fieldWorkerForm) fields() []templ.Component {
	out := make([]templ.Component, 0, 6)
	for _, k := range f.keys() {
		out = append(out, fieldWorkerFormField(f, k))
	}
	return out
}

func (f fieldWorkerForm) firstInvalid() string {
	for _, k := range f.keys() {
		if f.Errs.Fields[k] != "" {
			return f.id(k)
		}
	}
	return ""
}

func (f fieldWorkerForm) describedBy(key string) string {
	if f.Errs.Fields[key] != "" {
		return f.id(key) + "-err"
	}
	return ""
}

func (f fieldWorkerForm) fieldProps(key, label, help string, required bool) kit.FieldProps {
	return kit.FieldProps{
		Label: label, For: f.id(key), Description: help, Required: required, Error: f.Errs.Fields[key],
		ErrorID: f.id(key) + "-err", Attrs: templ.Attributes{"id": f.id(key) + "-field"},
	}
}

// manageClient is the client a new-worker request names, after the manage
// check the API's POST makes (CanManageFieldWorkers on the body's clientId).
func (h *FieldWorkers) manageClient(r *http.Request, ac *auth.AuthContext) (string, error) {
	clientID := strings.TrimSpace(r.PostFormValue(fieldClientParam))
	if clientID == "" {
		clientID = strings.TrimSpace(r.URL.Query().Get(fieldClientParam))
	}
	if clientID == "" || !kit.ValidID(clientID) {
		return "", usecase.Validation("CLIENT_ID_REQUIRED", "clientId is required")
	}
	if err := auth.CanManageFieldWorkers(ac, clientID); err != nil {
		return "", err
	}
	return clientID, nil
}

func (h *FieldWorkers) clientLabel(ctx context.Context, ac *auth.AuthContext, clientID string) string {
	opts, err := portalClientOptions(ctx, h.Clients, ac)
	if err != nil {
		return clientID
	}
	return portalClientName(opts, clientID)
}

func (h *FieldWorkers) newForm(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	clientID, err := h.manageClient(r, ac)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	idx, err := loadFieldApps(ctx, h.Store)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	f := fieldWorkerForm{
		Client: clientID, ClientName: h.clientLabel(ctx, ac, clientID), AppChoices: fieldAppChoices(idx.reachable(ac, true)), Open: true,
		MayImpersonate: auth.CanImpersonateFieldWorkers(ac, clientID) == nil,
	}
	if ds.IsDatastar(r) {
		if err := patchDialogSlot(ds.NewSSE(w, r), fwDialogSlot, fieldWorkerFormDialog(f)); err != nil {
			slog.DebugContext(ctx, "ui: field worker form patch not delivered", "err", err)
		}
		return
	}
	v, err := h.view(ctx, ac, parseListQuery(nil, fieldWorkerList).withFacet(fieldClientParam, []string{clientID}))
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	v.Dialog = fieldWorkerFormDialog(f)
	h.renderPage(w, r, http.StatusOK, v)
}

func (h *FieldWorkers) formFailed(w http.ResponseWriter, r *http.Request, f fieldWorkerForm) {
	if ds.IsDatastar(r) {
		patchFormFailure(ds.NewSSE(w, r), r, fwFormDialog, f.fields(), f.Errs.Form, f.firstInvalid())
		return
	}
	f.Open = true
	v, err := h.view(r.Context(), auth.FromContext(r.Context()), parseListQuery(nil, fieldWorkerList).withFacet(fieldClientParam, []string{f.Client}))
	if err != nil {
		v = fieldWorkersView{}
	}
	v.Dialog = fieldWorkerFormDialog(f)
	h.renderPage(w, r, cmpOr(f.Errs.Status, http.StatusUnprocessableEntity), v)
}

// create mirrors POST /api/field-workers (ensure): the worker is created or,
// when it exists, only gains the named application grants.
func (h *FieldWorkers) create(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	f := fieldWorkerForm{
		Client:     strings.TrimSpace(r.PostFormValue(fieldClientParam)),
		WorkerCode: strings.TrimSpace(r.PostFormValue("workerCode")), Name: strings.TrimSpace(r.PostFormValue("name")),
		Email: strings.TrimSpace(r.PostFormValue("email")), Apps: formSet(r.PostForm["apps"]),
		IsTest: r.PostFormValue("isTest") != "", CanImpersonate: r.PostFormValue("canImpersonate") != "",
	}
	clientID, err := h.manageClient(r, ac)
	if err != nil {
		f.Errs.apply(r, err, nil)
		h.formFailed(w, r, f)
		return
	}
	f.Client, f.ClientName = clientID, h.clientLabel(ctx, ac, clientID)
	f.MayImpersonate = auth.CanImpersonateFieldWorkers(ac, clientID) == nil
	// Granting the power to act as others is the impersonate permission's call,
	// in addition to manage (the API's ensure handler); a forged switch is
	// refused, not ignored.
	if f.CanImpersonate && !f.MayImpersonate {
		f.Errs.apply(r, auth.CanImpersonateFieldWorkers(ac, clientID), nil)
		h.formFailed(w, r, f)
		return
	}
	if idx, ierr := loadFieldApps(ctx, h.Store); ierr == nil {
		f.AppChoices = fieldAppChoices(idx.reachable(ac, true))
	}
	ev, err := h.Ops.EnsureWorker(ctx, fieldidentity.EnsureWorkerCommand{
		ClientID: clientID, WorkerCode: f.WorkerCode, Name: f.Name, Email: f.Email, AppCodes: f.Apps,
		IsTest: f.IsTest, CanImpersonate: f.CanImpersonate,
	})
	if err != nil {
		f.Errs.apply(r, err, fieldWorkerFieldCodes)
		h.formFailed(w, r, f)
		return
	}
	created := ev.Type == fieldidentity.WorkerCreatedType
	flash, detail := flashFieldWorkerEnsured, "Worker "+fieldidentity.NormalizeWorkerCode(f.WorkerCode)+" already existed; applications granted"
	if created {
		flash, detail = flashFieldWorkerCreated, "Worker "+fieldidentity.NormalizeWorkerCode(f.WorkerCode)+" created. Reset PIN to let them sign in."
	}
	q := parseListQuery(nil, fieldWorkerList).withFacet(fieldClientParam, []string{clientID})
	if !ds.IsDatastar(r) {
		h.Shell.SetFlash(w, flash)
		http.Redirect(w, r, q.URL(), http.StatusSeeOther) //nolint:gosec // G710: the target is a page constant; the client id is query-escaped and was checked by the manage check
		return
	}
	sse := ds.NewSSE(w, r)
	if err := sse.CloseDialog(fwFormDialog); err != nil {
		slog.DebugContext(ctx, "ui: field worker form close not delivered", "err", err)
	}
	h.afterChange(sse, r, ac, clientID, "", false)
	if err := portalFlash(sse, flash, detail); err != nil {
		slog.DebugContext(ctx, "ui: field worker flash not delivered", "err", err)
	}
}

// ── grant app ──────────────────────────────────────────────────────────

type fieldGrantForm struct {
	WorkerID, Code string
	Choices        []kit.Option
	App            string
	Errs           formErrors
	Open           bool
}

func (f fieldGrantForm) id(key string) string { return fieldID(fwGrantForm, key) }

func (f fieldGrantForm) dialog() kit.FormDialogProps {
	return kit.FormDialogProps{
		ID: fwGrantForm, Signal: fwGrantSig, Action: fieldWorkersPath + "/" + url.PathEscape(f.WorkerID) + "/grant", Open: f.Open, Error: f.Errs.Form,
		Title: "Grant application", Description: "Let " + f.Code + " sign in to another application. Existing grants are unchanged.",
		SubmitLabel: "Grant", Hidden: []kit.HiddenField{drawerField(fwDrawerSig)},
	}
}

func (f fieldGrantForm) describedBy() string {
	if f.Errs.Fields["app"] != "" {
		return f.id("app") + "-err"
	}
	return ""
}

func (f fieldGrantForm) fields() []templ.Component { return []templ.Component{fieldGrantField(f)} }

func (f fieldGrantForm) firstInvalid() string {
	if f.Errs.Fields["app"] != "" {
		return f.id("app")
	}
	return ""
}

func (h *FieldWorkers) grantChoices(ctx context.Context, ac *auth.AuthContext, i *fieldidentity.Identity) ([]kit.Option, error) {
	idx, err := loadFieldApps(ctx, h.Store)
	if err != nil {
		return nil, err
	}
	var out []kit.Option
	for _, a := range idx.reachable(ac, true) {
		if !i.HasApp(string(a.ID)) {
			out = append(out, kit.Option{Value: a.Code, Label: a.Name + " (" + a.Code + ")"})
		}
	}
	return out, nil
}

// grantTarget loads the worker for a grant. Like the API's grant handler it
// does not use the application confinement of the by-id loader: a confined
// caller may grant its own application to a worker it cannot yet see.
func (h *FieldWorkers) grantTarget(ctx context.Context, ac *auth.AuthContext, id string) (*fieldidentity.Identity, error) {
	i, err := h.Store.Worker(ctx, id)
	if err != nil {
		return nil, usecase.Internal("REPO", "find_field_worker failed", err)
	}
	clientID := ""
	if i != nil {
		clientID = i.ClientID
	}
	if err := fieldGate(auth.CanManageFieldWorkers, ac, clientID, i != nil, "FieldWorker", id); err != nil {
		return nil, err
	}
	return i, nil
}

func (h *FieldWorkers) grantForm(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	id, err := fieldWorkerID(r)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	i, err := h.grantTarget(ctx, ac, id)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	choices, err := h.grantChoices(ctx, ac, i)
	if err == nil && len(choices) == 0 {
		err = usecase.Validation("NO_GRANTABLE_APP", "There is no other active application to grant.")
	}
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	f := fieldGrantForm{WorkerID: i.ID, Code: i.WorkerCode, Choices: choices, App: choices[0].Value, Open: true}
	if ds.IsDatastar(r) {
		if err := patchDialogSlot(ds.NewSSE(w, r), fwDialogSlot, fieldGrantDialog(f)); err != nil {
			slog.DebugContext(ctx, "ui: field grant form patch not delivered", "err", err)
		}
		return
	}
	v, err := h.view(ctx, ac, parseListQuery(nil, fieldWorkerList).withFacet(fieldClientParam, []string{i.ClientID}))
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	v.Dialog = fieldGrantDialog(f)
	h.renderPage(w, r, http.StatusOK, v)
}

func (h *FieldWorkers) grant(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	id, err := fieldWorkerID(r)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	i, err := h.grantTarget(ctx, ac, id)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	f := fieldGrantForm{WorkerID: i.ID, Code: i.WorkerCode, App: strings.TrimSpace(r.PostFormValue("app"))}
	f.Choices, _ = h.grantChoices(ctx, ac, i)
	grantCmd := fieldidentity.WorkerAppCommand{AppCode: f.App}
	grantCmd.ID, grantCmd.ClientID = i.ID, i.ClientID
	if err := h.Ops.GrantWorkerApp(ctx, grantCmd); err != nil {
		f.Errs.apply(r, err, map[string]string{"UNKNOWN_APPLICATION": "app", "APP_CODE_REQUIRED": "app", "APPLICATION_FORBIDDEN": "app"})
		if ds.IsDatastar(r) {
			patchFormFailure(ds.NewSSE(w, r), r, fwGrantForm, f.fields(), f.Errs.Form, f.firstInvalid())
			return
		}
		f.Open = true
		v, verr := h.view(ctx, ac, parseListQuery(nil, fieldWorkerList).withFacet(fieldClientParam, []string{i.ClientID}))
		if verr != nil {
			v = fieldWorkersView{}
		}
		v.Dialog = fieldGrantDialog(f)
		h.renderPage(w, r, cmpOr(f.Errs.Status, http.StatusUnprocessableEntity), v)
		return
	}
	if !ds.IsDatastar(r) {
		h.Shell.SetFlash(w, flashFieldWorkerAppGranted)
		http.Redirect(w, r, fieldWorkersPath+"?"+fieldClientParam+"="+url.QueryEscape(i.ClientID), http.StatusSeeOther) //nolint:gosec // G710: the target is a page constant; the client id is query-escaped and was checked by the manage check
		return
	}
	sse := ds.NewSSE(w, r)
	if err := sse.CloseDialog(fwGrantForm); err != nil {
		slog.DebugContext(ctx, "ui: field grant form close not delivered", "err", err)
	}
	h.afterChange(sse, r, ac, i.ClientID, id, false)
	if err := portalFlash(sse, flashFieldWorkerAppGranted, i.WorkerCode+" can now sign in to "+f.App); err != nil {
		slog.DebugContext(ctx, "ui: field worker flash not delivered", "err", err)
	}
}

// ── edit ───────────────────────────────────────────────────────────────

const (
	fwEditForm = "fw-edit-form"
	fwEditSig  = "fwEditForm"
)

var fieldWorkerEditCodes = map[string]string{"NAME_REQUIRED": "name", "EMAIL_INVALID": "email"}

// fieldEditForm is PUT /api/field-workers/{id}: name, email and the two
// flags. The "flags" marker says the test switch was part of the form (an
// unticked switch posts nothing); "impersonateField" says the same of the
// supervisor switch, which only a caller with the impersonate permission is
// shown.
type fieldEditForm struct {
	WorkerID, Code                         string
	Name, Email                            string
	IsTest, CanImpersonate, MayImpersonate bool
	Errs                                   formErrors
	Open                                   bool
}

func (f fieldEditForm) id(key string) string { return fieldID(fwEditForm, key) }

func (f fieldEditForm) dialog() kit.FormDialogProps {
	hidden := []kit.HiddenField{drawerField(fwDrawerSig), {Name: "flags", Value: "1"}}
	if f.MayImpersonate {
		hidden = append(hidden, kit.HiddenField{Name: "impersonateField", Value: "1"})
	}
	return kit.FormDialogProps{
		ID: fwEditForm, Signal: fwEditSig, Action: fieldWorkersPath + "/" + url.PathEscape(f.WorkerID) + "/edit", Open: f.Open, Error: f.Errs.Form,
		Title: "Edit field worker", Description: "Change the name, the contact email and how " + f.Code + " is treated. The worker code cannot change.",
		SubmitLabel: "Save", Hidden: hidden,
	}
}

func (f fieldEditForm) keys() []string {
	keys := []string{"name", "email", "isTest"}
	if f.MayImpersonate {
		keys = append(keys, "canImpersonate")
	}
	return keys
}

func (f fieldEditForm) fields() []templ.Component {
	out := make([]templ.Component, 0, 4)
	for _, k := range f.keys() {
		out = append(out, fieldEditField(f, k))
	}
	return out
}

func (f fieldEditForm) firstInvalid() string {
	for _, k := range f.keys() {
		if f.Errs.Fields[k] != "" {
			return f.id(k)
		}
	}
	return ""
}

func (f fieldEditForm) describedBy(key string) string {
	if f.Errs.Fields[key] != "" {
		return f.id(key) + "-err"
	}
	return ""
}

func (f fieldEditForm) fieldProps(key, label, help string, required bool) kit.FieldProps {
	return kit.FieldProps{
		Label: label, For: f.id(key), Description: help, Required: required, Error: f.Errs.Fields[key],
		ErrorID: f.id(key) + "-err", Attrs: templ.Attributes{"id": f.id(key) + "-field"},
	}
}

func (h *FieldWorkers) editFormOf(ac *auth.AuthContext, i *fieldidentity.Identity) fieldEditForm {
	return fieldEditForm{
		WorkerID: i.ID, Code: i.WorkerCode, Name: i.Name, Email: i.Email, IsTest: i.IsTest, CanImpersonate: i.CanImpersonate,
		MayImpersonate: auth.CanImpersonateFieldWorkers(ac, i.ClientID) == nil,
	}
}

func (h *FieldWorkers) editForm(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	id, err := fieldWorkerID(r)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	i, err := h.loadWorker(ctx, ac, id, true)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	f := h.editFormOf(ac, i)
	f.Open = true
	if ds.IsDatastar(r) {
		if err := patchDialogSlot(ds.NewSSE(w, r), fwDialogSlot, fieldEditDialog(f)); err != nil {
			slog.DebugContext(ctx, "ui: field edit form patch not delivered", "err", err)
		}
		return
	}
	v, err := h.view(ctx, ac, parseListQuery(nil, fieldWorkerList).withFacet(fieldClientParam, []string{i.ClientID}))
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	v.Dialog = fieldEditDialog(f)
	h.renderPage(w, r, http.StatusOK, v)
}

// edit mirrors PUT /api/field-workers/{id}: manage, and changing
// canImpersonate also needs the impersonate permission. A forged supervisor
// switch from a caller without it is refused, not ignored.
func (h *FieldWorkers) edit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	id, err := fieldWorkerID(r)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	i, err := h.loadWorker(ctx, ac, id, true)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	f := h.editFormOf(ac, i)
	f.Name, f.Email = strings.TrimSpace(r.PostFormValue("name")), strings.TrimSpace(r.PostFormValue("email"))
	cmd := fieldidentity.UpdateWorkerCommand{WorkerRef: refOfWorker(i), Name: f.Name, Email: &f.Email}
	if r.PostForm.Has("flags") {
		f.IsTest = r.PostFormValue("isTest") != ""
		cmd.IsTest = &f.IsTest
	}
	var err2 error
	if r.PostForm.Has("impersonateField") || r.PostForm.Has("canImpersonate") {
		want := r.PostFormValue("canImpersonate") != ""
		if want != i.CanImpersonate {
			err2 = auth.CanImpersonateFieldWorkers(ac, i.ClientID)
			cmd.CanImpersonate = &want
		}
		f.CanImpersonate = want
	}
	if err2 == nil {
		err2 = h.Ops.UpdateWorker(ctx, cmd)
	}
	if err2 != nil {
		f.Errs.apply(r, err2, fieldWorkerEditCodes)
		if ds.IsDatastar(r) {
			patchFormFailure(ds.NewSSE(w, r), r, fwEditForm, f.fields(), f.Errs.Form, f.firstInvalid())
			return
		}
		f.Open = true
		v, verr := h.view(ctx, ac, parseListQuery(nil, fieldWorkerList).withFacet(fieldClientParam, []string{i.ClientID}))
		if verr != nil {
			v = fieldWorkersView{}
		}
		v.Dialog = fieldEditDialog(f)
		h.renderPage(w, r, cmpOr(f.Errs.Status, http.StatusUnprocessableEntity), v)
		return
	}
	if !ds.IsDatastar(r) {
		h.Shell.SetFlash(w, flashFieldWorkerUpdated)
		http.Redirect(w, r, fieldWorkersPath+"?"+fieldClientParam+"="+url.QueryEscape(i.ClientID), http.StatusSeeOther) //nolint:gosec // G710: the target is a page constant; the client id is query-escaped and was checked by the manage check
		return
	}
	sse := ds.NewSSE(w, r)
	if err := sse.CloseDialog(fwEditForm); err != nil {
		slog.DebugContext(ctx, "ui: field edit form close not delivered", "err", err)
	}
	h.afterChange(sse, r, ac, i.ClientID, id, false)
	if err := portalFlash(sse, flashFieldWorkerUpdated, i.WorkerCode+" updated"); err != nil {
		slog.DebugContext(ctx, "ui: field worker flash not delivered", "err", err)
	}
}
