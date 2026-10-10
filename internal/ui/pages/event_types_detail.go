package pages

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/a-h/templ"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/eventtype"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/internal/schemacodegen"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/ds"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/kit"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/shell"
)

// The record drawer (the SPA's EventTypeDetailDrawer), the schema viewer
// (SchemaViewerDialog), the drawer's actions, Sync Platform Events and
// Export Schemas.

type etSpecRow struct {
	Version, MimeType, SchemaType, Status string
	Severity                              kit.TagSeverity
	ViewHref, ActionBase                  string
	HasSchema, Finalising, Current        bool
}

type etDetail struct {
	ID, Code, Name, Description string
	ClientScoped                bool
	Status                      string
	StatusSeverity              kit.TagSeverity
	Archived, IsCurrent         bool
	Specs                       []etSpecRow
	// CanArchive/CanDelete are the SPA drawer's guards (its buttons are
	// disabled otherwise); the use cases do not enforce them.
	CanArchive, CanDelete   bool
	EditHref, AddSchemaHref string
	ActionBase              string
	Perms                   etPerms
}

// schemaTypeLabel is the SPA's formatSchemaType.
func schemaTypeLabel(t eventtype.SchemaType) string {
	switch t {
	case eventtype.SchemaJSON:
		return "JSON Schema"
	case eventtype.SchemaProto:
		return "Protocol Buffers"
	case eventtype.SchemaXSD:
		return "XML Schema"
	}
	return string(t)
}

func toETDetail(et *eventtype.EventType, perms etPerms) etDetail {
	d := etDetail{
		ID: et.ID, Code: et.Code, Name: et.Name, Description: deref(et.Description), ClientScoped: et.ClientScoped,
		Status: string(et.Status), StatusSeverity: etStatusSeverity(et.Status),
		Archived: et.Status == eventtype.StatusArchived, IsCurrent: et.Status == eventtype.StatusCurrent,
		EditHref: etURL(et.ID) + "/edit", AddSchemaHref: etURL(et.ID) + "/add-schema", ActionBase: etURL(et.ID),
		Perms: perms,
	}
	allDeprecated, allFinalising := true, true
	for _, sv := range et.SpecVersions {
		allDeprecated = allDeprecated && sv.Status == eventtype.SpecDeprecated
		allFinalising = allFinalising && sv.Status == eventtype.SpecFinalising
		d.Specs = append(d.Specs, etSpecRow{
			Version: sv.Version, MimeType: sv.MimeType, SchemaType: schemaTypeLabel(sv.SchemaType),
			Status: string(sv.Status), Severity: specSeverity(sv.Status),
			ViewHref: etVersionURL(et.ID, sv.Version), ActionBase: etVersionURL(et.ID, sv.Version),
			HasSchema:  len(sv.SchemaContent) > 0,
			Finalising: sv.Status == eventtype.SpecFinalising, Current: sv.Status == eventtype.SpecCurrent,
		})
	}
	// The SPA's canArchive: CURRENT and every version deprecated (or none);
	// canDelete: ARCHIVED, or CURRENT with every version still finalising.
	d.CanArchive = d.IsCurrent && allDeprecated
	d.CanDelete = d.Archived || (d.IsCurrent && allFinalising)
	return d
}

const (
	cannotArchive = "Deprecate every schema version first."
	cannotDelete  = "Only an archived event type, or one whose schema versions are all still finalising, can be deleted."
)

// etWhy is a disabled action's reason ("" when the action is allowed).
func etWhy(allowed bool, reason string) string {
	if allowed {
		return ""
	}
	return reason
}

func (d etDetail) canEdit() bool { return d.Perms.Update && !d.Archived }

func (d etDetail) drawerProps() kit.DrawerProps {
	return kit.DrawerProps{ID: etDrawerID, Signal: etDrawerSig, Label: "Event type", Size: kit.DrawerWide, Title: d.Name, Subtitle: d.Code, Extras: etTag(d.Status, d.StatusSeverity)}
}

func (h *EventTypes) detail(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	if err := auth.CanReadEventTypes(ac); err != nil {
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
	d := toETDetail(et, etPermsFor(ac))
	if ds.IsDatastar(r) {
		if err := openETDrawer(ds.NewSSE(w, r), d); err != nil {
			slog.DebugContext(ctx, "ui: event type drawer patch not delivered", "err", err)
		}
		return
	}
	view, err := h.listView(ctx, ac, parseListQuery(nil, etList))
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	view.Drawer = &d
	h.renderPage(w, r, http.StatusOK, view)
}

func openETDrawer(sse *ds.SSE, d etDetail) error {
	return errors.Join(
		sse.Patch(etDrawerContent(d), ds.Inner("#"+etDrawerID)),
		sse.PatchSignals(map[string]any{etDrawerSig: d.ID}),
		sse.Focus(etDrawerID+"-title"),
	)
}

// ── schema viewer ──────────────────────────────────────────────────────

const etViewerID = "et-schema-viewer"

// etViewerTabs are the SPA's tabs; each panel is generated on the server
// by schemacodegen (the Go port of @flowcatalyst/schema-codegen).
var etViewerTabs = []kit.TabItem{
	{Value: "schema", Label: "Schema"},
	{Value: "example", Label: "Example"},
	{Value: "typescript", Label: "TypeScript"},
	{Value: "php", Label: "PHP"},
	{Value: "python", Label: "Python"},
	{Value: "java", Label: "Java"},
}

type etViewer struct {
	Title          string
	Status         string
	StatusSeverity kit.TagSeverity
	// Panels are highlighted HTML (escaped text and hl-* spans only) per tab.
	Panels map[string]string
}

// viewerSeverity is the viewer's own status colouring (CURRENT success,
// FINALISING info, anything else warn).
func viewerSeverity(s eventtype.SpecVersionStatus) kit.TagSeverity {
	switch s {
	case eventtype.SpecCurrent:
		return kit.TagSuccess
	case eventtype.SpecFinalising:
		return kit.TagInfo
	case eventtype.SpecDeprecated:
	}
	return kit.TagWarn
}

func toETViewer(et *eventtype.EventType, sv eventtype.SpecVersion) etViewer {
	src := string(sv.SchemaContent)
	return etViewer{
		Title:  et.Code + " — v" + sv.Version,
		Status: string(sv.Status), StatusSeverity: viewerSeverity(sv.Status),
		Panels: map[string]string{
			"schema":     schemacodegen.HighlightJSON(schemacodegen.FormatSchema(src)),
			"example":    schemacodegen.HighlightJSON(schemacodegen.ExampleJSON(src)),
			"typescript": schemacodegen.HighlightTypeScript(schemacodegen.TypeScriptText(src, et.Code)),
			"php":        schemacodegen.HighlightPHP(schemacodegen.PHPText(src, et.Code)),
			"python":     schemacodegen.HighlightPython(schemacodegen.PythonText(src, et.Code)),
			"java":       schemacodegen.HighlightJava(schemacodegen.JavaText(src, et.Code)),
		},
	}
}

func (v etViewer) tabsWithPanels() kit.TabsProps {
	p := kit.TabsProps{Signal: "etSchemaTab", Tabs: etViewerTabs, ListClass: "flex-wrap", Panels: map[string]templ.Component{}}
	for _, t := range etViewerTabs {
		// The highlighter's output is escaped text plus hl-* spans only.
		code := templ.Raw(v.Panels[t.Value]) //nolint:gosec // G203: schemacodegen escapes the text it highlights
		p.Panels[t.Value] = templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
			return kit.CodeBlock(kit.CodeBlockProps{ID: "et-code-" + t.Value, Label: t.Label}).Render(templ.WithChildren(ctx, code), w)
		})
	}
	return p
}

// etDangerAttrs: an enabled danger-zone button opens its confirmation; a
// disabled one points at the reason shown under it.
func etDangerAttrs(enabled bool, dialogID, action, whyID string) templ.Attributes {
	if enabled {
		return confirmTrigger(dialogID, action)
	}
	return templ.Attributes{"aria-describedby": whyID}
}

// copyExpr copies the visible tab's code (the SPA's copy button), then
// says so beside the button for two seconds.
const etCopyExpr = "navigator.clipboard.writeText(document.getElementById('et-code-' + $etSchemaTab).textContent).then(() => { $etCopied = true; setTimeout(() => $etCopied = false, 2000) })"

func (h *EventTypes) schemaViewer(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	if err := auth.CanReadEventTypes(ac); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	id, err := etID(r)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	version, err := etVersion(r)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	et, err := h.loadEventType(ctx, ac, id)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	var sv *eventtype.SpecVersion
	for i := range et.SpecVersions {
		if et.SpecVersions[i].Version == version {
			sv = &et.SpecVersions[i]
		}
	}
	if sv == nil || len(sv.SchemaContent) == 0 {
		h.Shell.Fail(w, r, httperror.NotFound("SchemaVersion", version))
		return
	}
	viewer := etViewerDialog(toETViewer(et, *sv))
	if ds.IsDatastar(r) {
		if err := ds.NewSSE(w, r).Patch(viewer, ds.Inner("#"+etDialogSlot)); err != nil {
			slog.DebugContext(ctx, "ui: schema viewer patch not delivered", "err", err)
		}
		return
	}
	view, err := h.listView(ctx, ac, parseListQuery(nil, etList))
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	d := toETDetail(et, etPermsFor(ac))
	view.Drawer, view.Dialog = &d, viewer
	h.renderPage(w, r, http.StatusOK, view)
}

// ── actions ────────────────────────────────────────────────────────────

type etAction struct {
	check   func(*auth.AuthContext) error
	run     func(h *EventTypes, ctx context.Context, id, version string) error
	flash   string
	deleted bool
}

var (
	archiveETAction = etAction{
		check: auth.CanUpdateEventTypes, flash: flashETArchived,
		run: func(h *EventTypes, ctx context.Context, id, _ string) error { return h.Ops.Archive(ctx, id) },
	}
	deleteETAction = etAction{
		check: auth.CanDeleteEventTypes, flash: flashETDeleted, deleted: true,
		run: func(h *EventTypes, ctx context.Context, id, _ string) error { return h.Ops.Delete(ctx, id) },
	}
	finaliseETAction = etAction{
		check: auth.CanUpdateEventTypes, flash: flashSchemaFinalised,
		run: func(h *EventTypes, ctx context.Context, id, v string) error { return h.Ops.FinaliseSchema(ctx, id, v) },
	}
	deprecateETAction = etAction{
		check: auth.CanUpdateEventTypes, flash: flashSchemaDeprecated,
		run: func(h *EventTypes, ctx context.Context, id, v string) error { return h.Ops.DeprecateSchema(ctx, id, v) },
	}
)

func (h *EventTypes) archive(w http.ResponseWriter, r *http.Request) {
	h.runAction(w, r, archiveETAction, false)
}

func (h *EventTypes) delete(w http.ResponseWriter, r *http.Request) {
	h.runAction(w, r, deleteETAction, false)
}

func (h *EventTypes) finalise(w http.ResponseWriter, r *http.Request) {
	h.runAction(w, r, finaliseETAction, true)
}

func (h *EventTypes) deprecate(w http.ResponseWriter, r *http.Request) {
	h.runAction(w, r, deprecateETAction, true)
}

// runAction: the BFF handler's check, the use case, then the list, the
// drawer (when it shows this event type; closed after a delete) and a
// flash. A refusal (e.g. deprecating a FINALISING version) is an error flash.
func (h *EventTypes) runAction(w http.ResponseWriter, r *http.Request, a etAction, versioned bool) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	if err := a.check(ac); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	id, err := etID(r)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	version := ""
	if versioned {
		if version, err = etVersion(r); err != nil {
			h.Shell.Fail(w, r, err)
			return
		}
	}
	if err := a.run(h, ctx, id, version); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	if !ds.IsDatastar(r) {
		h.Shell.SetFlash(w, a.flash)
		target := etPath
		if !a.deleted {
			target = etURL(id)
		}
		http.Redirect(w, r, target, http.StatusSeeOther) //nolint:gosec // G710: a local path built from a validated id (kit.ValidID)
		return
	}
	sse := ds.NewSSE(w, r)
	h.afterChange(sse, r, ac, id, a.deleted)
	if err := flashKey(sse, a.flash); err != nil {
		slog.DebugContext(ctx, "ui: event type flash not delivered", "err", err)
	}
}

// afterChange re-renders the list the user is on and, when the drawer shows
// this event type, the drawer (closed after a delete).
func (h *EventTypes) afterChange(sse *ds.SSE, r *http.Request, ac *auth.AuthContext, id string, deleted bool) {
	ctx := r.Context()
	if view, err := h.listView(ctx, ac, refererList(r, etList)); err != nil {
		h.Shell.FailSSE(sse, r, err)
	} else if err := errors.Join(view.patchTable(sse), sse.Patch(etActions(view))); err != nil {
		slog.DebugContext(ctx, "ui: event types table patch not delivered", "err", err)
	}
	if id == "" || r.PostFormValue("drawer") != id {
		return
	}
	if deleted {
		if err := sse.PatchSignals(map[string]any{etDrawerSig: ""}); err != nil {
			slog.DebugContext(ctx, "ui: event type drawer close not delivered", "err", err)
		}
		return
	}
	et, err := h.loadEventType(ctx, ac, id)
	if err != nil {
		h.Shell.FailSSE(sse, r, err)
		return
	}
	if err := sse.Patch(etDrawerContent(toETDetail(et, etPermsFor(ac))), ds.Inner("#"+etDrawerID)); err != nil {
		slog.DebugContext(ctx, "ui: event type drawer patch not delivered", "err", err)
	}
}

// syncPlatform is the SPA's "Sync Platform Events" (anchor only, as
// POST /bff/event-types/sync-platform): a flash with the tally.
func (h *EventTypes) syncPlatform(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	if err := auth.RequireAnchor(ac); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	res, err := h.Ops.SyncPlatform(ctx)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	if !ds.IsDatastar(r) {
		h.Shell.SetFlash(w, flashETSynced)
		http.Redirect(w, r, etPath, http.StatusSeeOther)
		return
	}
	sse := ds.NewSSE(w, r)
	h.afterChange(sse, r, ac, "", false)
	msg := kit.FlashMessageProps{Kind: kit.FlashSuccess, Summary: "Platform Events Synced", Detail: syncDetail(res)}
	if err := shell.Flash(sse, msg); err != nil {
		slog.DebugContext(ctx, "ui: sync flash not delivered", "err", err)
	}
}

// syncDetail is the SPA's toast text for a sync result.
func syncDetail(r EventTypeSyncResult) string {
	var parts []string
	for _, p := range []struct {
		n    int
		verb string
	}{{r.Created, "created"}, {r.Updated, "updated"}, {r.Deleted, "deleted"}} {
		if p.n > 0 {
			parts = append(parts, strconv.Itoa(p.n)+" "+p.verb)
		}
	}
	out := strconv.Itoa(r.Total) + " event types up to date"
	if len(parts) > 0 {
		out = strings.Join(parts, ", ") + " (" + strconv.Itoa(r.Total) + " total)"
	}
	var schema []string
	for _, p := range []struct {
		n    int
		verb string
	}{{r.SchemasCreated, "created"}, {r.SchemasUpdated, "updated"}, {r.SchemasSame, "unchanged"}} {
		if p.n > 0 {
			schema = append(schema, strconv.Itoa(p.n)+" "+p.verb)
		}
	}
	if total := r.SchemasCreated + r.SchemasUpdated + r.SchemasSame; total > 0 {
		out += "\nSchemas: " + strings.Join(schema, ", ") + " (" + strconv.Itoa(total) + " total)"
	}
	return out
}

// export is "Export Schemas": a zip of every matching event type's CURRENT
// JSON Schema with TypeScript, PHP, Python and Java types (the SPA builds
// the same zip in the browser).
func (h *EventTypes) export(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	if err := auth.CanReadEventTypes(ac); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	all, err := h.visible(ctx, ac)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	q := parseListQuery(r.URL.Query(), etList)
	q.Rows = len(all) + 1 // every match on one page
	view := buildETView(ac, q, all)
	byID := make(map[string]eventtype.EventType, len(all))
	for _, et := range all {
		byID[et.ID] = et
	}
	in := make([]schemacodegen.ExportEventType, 0, len(view.Rows))
	for _, row := range view.Rows {
		et := byID[row.ID]
		in = append(in, schemacodegen.ExportEventType{Application: et.Application, Subdomain: et.Subdomain, Aggregate: et.Aggregate, Event: et.EventName, CurrentSchema: currentSchema(et)})
	}
	zip, exported, genErrs, err := schemacodegen.ExportZip(in)
	if err != nil {
		h.Shell.Fail(w, r, fmt.Errorf("export schemas: %w", err))
		return
	}
	if exported == 0 {
		h.Shell.SetFlash(w, flashExportEmpty)
		http.Redirect(w, r, view.Query.URL(), http.StatusSeeOther)
		return
	}
	if len(genErrs) > 0 {
		slog.InfoContext(ctx, "ui: schema export generation errors", "count", len(genErrs), "errors", genErrs)
	}
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="event-schemas.zip"`)
	w.Header().Set("Content-Length", strconv.Itoa(len(zip)))
	if _, err := w.Write(zip); err != nil {
		slog.DebugContext(ctx, "ui: schema export not delivered", "err", err)
	}
}

func etConfirm(id, title, message, label string, tone kit.ConfirmTone) kit.ConfirmDialogProps {
	return kit.ConfirmDialogProps{
		ID: id, Action: etPath, Title: title, Message: message, ConfirmLabel: label, Tone: tone,
		Hidden:    []kit.HiddenField{drawerField(etDrawerSig)},
		FormAttrs: templ.Attributes{"data-on:submit": kit.SubmitConfirm},
	}
}
