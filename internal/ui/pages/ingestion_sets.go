package pages

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/a-h/templ"
	"github.com/go-chi/chi/v5"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/function/artifact"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/ingestion"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/ingestion/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/ds"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/kit"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/shell"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecaseop"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecasepgx"
)

// IngestionSetStore is what the sets pages read; IngestionSetReader is the
// real one.
type IngestionSetStore interface {
	ListSets(ctx context.Context) ([]ingestion.Set, error)
	FindSetByName(ctx context.Context, name string) (*ingestion.Set, error)
	ListSetApplies(ctx context.Context, setID string) ([]ingestion.SetApply, error)
	FindSetApply(ctx context.Context, setID, applyID string) (*ingestion.SetApply, error)
	// ApplyFiles are the files an apply was given, from the set archive.
	ApplyFiles(ctx context.Context, apply ingestion.SetApply) ([]ingestion.SetFile, error)
}

// IngestionSetReader reads sets through the repository and their archives
// through the archive store.
type IngestionSetReader struct {
	*ingestion.Repository
	Archive artifact.Store
}

// ApplyFiles reads what the apply was given from its archive; an archive that
// is no longer kept is a not-found error, as for GET
// /api/ingestion-sets/{name}/applies/{applyId}.
func (s IngestionSetReader) ApplyFiles(ctx context.Context, apply ingestion.SetApply) ([]ingestion.SetFile, error) {
	archived, err := readApplyArchive(ctx, s.Archive, apply)
	if err != nil {
		return nil, err
	}
	return archived.Files, nil
}

func readApplyArchive(ctx context.Context, store artifact.Store, apply ingestion.SetApply) (*ingestion.SetArchive, error) {
	noApply := usecase.NotFound("INGESTION_SET_APPLY_NOT_FOUND", "the set does not keep apply "+apply.ID)
	if apply.ArchiveDigest == "" {
		return nil, noApply
	}
	archived, err := ingestion.ReadSetArchive(ctx, store, apply.ArchiveDigest)
	switch {
	case errors.Is(err, artifact.ErrNotFound):
		return nil, noApply
	case err != nil:
		return nil, usecase.Internal("INGESTION_SET_ARCHIVE_UNAVAILABLE", "the set archive could not be read; try again", err)
	}
	return archived, nil
}

// IngestionSetPlan is what an apply did, or a dry run says it would do.
type IngestionSetPlan struct {
	// ApplyID is the recorded apply; nil for a dry run and for an apply that
	// changed nothing.
	ApplyID *string
	Changes []ingestion.SetChange
}

// IngestionSetCommands are the two operations the sets pages run.
type IngestionSetCommands interface {
	// Reapply applies the set again with what the apply was given (a new
	// apply, going back to the earlier one); dryRun only says what it would do.
	Reapply(ctx context.Context, name, applyID string, dryRun bool) (IngestionSetPlan, error)
	// Release lets go of the set: its ingestions stay, edited one at a time
	// again. It returns how many were released.
	Release(ctx context.Context, name string) (int, error)
}

// IngestionSetOps runs ApplyIngestionSet (for a reapply) and
// ReleaseIngestionSet through the unit of work, as POST
// /api/ingestion-sets/{name}/applies/{applyId}/reapply and
// /api/ingestion-sets/{name}/release do; the permission checks are the page's.
type IngestionSetOps struct {
	Repo     *ingestion.Repository
	Resolver ingestion.Resolver
	Archive  artifact.Store
	UoW      *usecasepgx.UnitOfWork
}

// Reapply reads the apply's archive and applies it again.
func (o IngestionSetOps) Reapply(ctx context.Context, name, applyID string, dryRun bool) (IngestionSetPlan, error) {
	set, err := o.Repo.FindSetByName(ctx, name)
	if err != nil {
		return IngestionSetPlan{}, usecase.Internal("REPO", "find set failed", err)
	}
	if set == nil {
		return IngestionSetPlan{}, usecase.NotFound("INGESTION_SET_NOT_FOUND", "Ingestion set not found: "+name)
	}
	apply, err := o.Repo.FindSetApply(ctx, set.ID, applyID)
	if err != nil {
		return IngestionSetPlan{}, usecase.Internal("REPO", "find set apply failed", err)
	}
	if apply == nil {
		return IngestionSetPlan{}, usecase.NotFound("INGESTION_SET_APPLY_NOT_FOUND", "the set does not keep apply "+applyID)
	}
	archived, err := readApplyArchive(ctx, o.Archive, *apply)
	if err != nil {
		return IngestionSetPlan{}, err
	}
	cmd := operations.ApplySetCommand{Name: name, Items: archived.Items, Files: archived.Files, DryRun: dryRun, GoingBackTo: &apply.ID}
	res, err := usecaseop.RunTx(ctx, o.UoW, operations.ApplySet(o.Repo, o.Resolver, o.Archive), cmd, auth.NewExecutionContext(ctx))
	if err != nil {
		return IngestionSetPlan{}, err
	}
	for _, digest := range res.Forgotten {
		if err := o.Archive.Delete(ctx, digest); err != nil {
			slog.WarnContext(ctx, "ingestion set archive is no longer kept and could not be removed", "set", res.Set, "digest", digest, "err", err)
		}
	}
	return IngestionSetPlan{ApplyID: res.ApplyID, Changes: res.Changes}, nil
}

// Release lets go of the set.
func (o IngestionSetOps) Release(ctx context.Context, name string) (int, error) {
	res, err := usecaseop.RunTx(ctx, o.UoW, operations.ReleaseSet(o.Repo), operations.ReleaseSetCommand{Name: name}, auth.NewExecutionContext(ctx))
	return res.Released, err
}

// IngestionSets is the sets area (/ui/ingestion-sets): the sets that own
// ingestions, one set's kept applies with the files each was given, going
// back to an earlier apply and releasing a set (the Nuxt control plane's
// SetsPage). Applying a set is not done here, only with `fcdev ingest apply`.
// Every route makes the API handlers' checks: the ingestion view permission
// and a caller that reaches every client and application (a set spans them);
// going back and releasing need the set-apply permission.
type IngestionSets struct {
	Sets IngestionSetStore
	Ops  IngestionSetCommands
	// Names resolves a principal id to the name shown for who applied; nil
	// shows the id.
	Names func(ctx context.Context, principalID string) string
	Shell *shell.Shell
}

const (
	ingSetsBase      = "/ui/ingestion-sets"
	ingSetsListID    = "ing-sets"
	ingSetDetailID   = "ing-set-detail"
	ingSetDialogName = "ing-set-dialog"
	ingSetDialogBox  = "ing-set-dialog-slot"
	ingSetRelease    = "ing-set-release"
)

// Mount registers the routes (POSTs behind parseForm).
func (h *IngestionSets) Mount(r chi.Router) {
	r.Get("/ingestion-sets", h.list)
	r.Get("/ingestion-sets/{name}", h.detail)
	r.Get("/ingestion-sets/{name}/applies/{applyId}/files", h.files)
	r.Get("/ingestion-sets/{name}/applies/{applyId}/reapply", h.reapplyForm)
	post := r.With(parseForm)
	post.Post("/ingestion-sets/{name}/applies/{applyId}/reapply", h.reapply)
	post.Post("/ingestion-sets/{name}/release", h.release)
}

// canReadIngestionSets is the API's canReadSets: sets span clients and
// applications, so only a caller that reaches all of them lists or reads them.
func canReadIngestionSets(ac *auth.AuthContext) error {
	if err := auth.CanReadIngestions(ac); err != nil {
		return err
	}
	if !ac.IsAnchor() || ac.IsApplicationScoped() {
		return usecase.Authorization("INGESTION_SET_ANCHOR_ONLY", "sets span clients and applications: only a caller that reaches all of them lists or reads them")
	}
	return nil
}

func ingSetURL(name string) string { return ingSetsBase + "/" + name }

// ingSetName reads and checks the {name} path parameter.
func ingSetName(r *http.Request) (string, error) {
	name := chi.URLParam(r, "name")
	if !ingestion.ValidSetName(name) {
		return "", usecase.NotFound("INGESTION_SET_NOT_FOUND", "Ingestion set not found: "+name)
	}
	return name, nil
}

// ingApplyID reads and checks the {applyId} path parameter: an id that could
// not be an element id is not an apply.
func ingApplyID(r *http.Request) (string, error) {
	id := chi.URLParam(r, "applyId")
	if !kit.ValidID(id) {
		return "", usecase.NotFound("INGESTION_SET_APPLY_NOT_FOUND", "the set does not keep apply "+id)
	}
	return id, nil
}

// ── views ──────────────────────────────────────────────────────────────

type ingSetRow struct {
	Name       string
	Ingestions int
	Updated    time.Time
	Current    bool
}

type ingChange struct {
	Action   string
	Severity kit.TagSeverity
	Label    string
	// Href opens the ingestion; empty for a removed one.
	Href string
}

type ingApplyRow struct {
	ID        string
	Newest    bool
	GoingBack string
	By        string
	At        time.Time
	Items     int
	Files     int
	Changes   []ingChange
	// GoBack is the confirmation's URL: offered on every apply but the newest
	// (the set already is as the newest left it), to who may apply sets.
	GoBack string
	// FilesURL loads the files the apply was given.
	FilesURL string
}

type ingSetsView struct {
	Sets     []ingSetRow
	Selected string
	Owned    int
	Applies  []ingApplyRow
	CanApply bool
}

var ingActionSeverity = map[ingestion.SetAction]kit.TagSeverity{
	ingestion.SetCreate:    kit.TagSuccess,
	ingestion.SetChanged:   kit.TagInfo,
	ingestion.SetAdopt:     kit.TagInfo,
	ingestion.SetUnchanged: kit.TagSecondary,
	ingestion.SetRemove:    kit.TagDanger,
}

// ingChanges are the changes that did something, as badges.
func ingChanges(changes []ingestion.SetChange) []ingChange {
	var out []ingChange
	for _, c := range changes {
		if c.Action == ingestion.SetUnchanged {
			continue
		}
		ch := ingChange{Action: string(c.Action), Severity: ingActionSeverity[c.Action], Label: c.Client + "/" + c.Code}
		if c.IngestionID != nil && c.Action != ingestion.SetRemove && kit.ValidID(*c.IngestionID) {
			ch.Href = ingTabURL(*c.IngestionID)
		}
		out = append(out, ch)
	}
	return out
}

func (h *IngestionSets) name(ctx context.Context, id *string) string {
	if id == nil {
		return "unknown"
	}
	if h.Names != nil {
		if n := h.Names(ctx, *id); n != "" {
			return n
		}
	}
	return *id
}

// view reads the sets and, when name is not empty, that set's applies.
func (h *IngestionSets) view(ctx context.Context, ac *auth.AuthContext, name string) (ingSetsView, error) {
	sets, err := h.Sets.ListSets(ctx)
	if err != nil {
		return ingSetsView{}, usecase.Internal("REPO", "list sets failed", err)
	}
	v := ingSetsView{Selected: name, CanApply: auth.CanApplyIngestionSets(ac) == nil}
	for _, s := range sets {
		v.Sets = append(v.Sets, ingSetRow{Name: s.Name, Ingestions: s.Ingestions, Updated: s.UpdatedAt, Current: s.Name == name})
		if s.Name == name {
			v.Owned = s.Ingestions
		}
	}
	if name == "" {
		return v, nil
	}
	set, err := h.Sets.FindSetByName(ctx, name)
	if err != nil {
		return ingSetsView{}, usecase.Internal("REPO", "find set failed", err)
	}
	if set == nil {
		return ingSetsView{}, usecase.NotFound("INGESTION_SET_NOT_FOUND", "Ingestion set not found: "+name)
	}
	applies, err := h.Sets.ListSetApplies(ctx, set.ID)
	if err != nil {
		return ingSetsView{}, usecase.Internal("REPO", "list set applies failed", err)
	}
	for n, a := range applies {
		row := ingApplyRow{
			ID: a.ID, Newest: n == 0, By: h.name(ctx, a.AppliedBy), At: a.AppliedAt, Items: a.Summary.Items, Files: a.Summary.Files,
			Changes:  ingChanges(a.Summary.Changes),
			FilesURL: ingSetURL(name) + "/applies/" + a.ID + "/files",
		}
		if a.Summary.GoingBackTo != nil {
			row.GoingBack = *a.Summary.GoingBackTo
		}
		if v.CanApply && n > 0 {
			row.GoBack = ingSetURL(name) + "/applies/" + a.ID + "/reapply"
		}
		v.Applies = append(v.Applies, row)
	}
	return v, nil
}

// ── pages ──────────────────────────────────────────────────────────────

func (h *IngestionSets) list(w http.ResponseWriter, r *http.Request) {
	h.page(w, r, "")
}

func (h *IngestionSets) detail(w http.ResponseWriter, r *http.Request) {
	name, err := ingSetName(r)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	h.page(w, r, name)
}

// page renders the sets, with the applies of name when it is not empty.
func (h *IngestionSets) page(w http.ResponseWriter, r *http.Request, name string) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	if err := canReadIngestionSets(ac); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	v, err := h.view(ctx, ac, name)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	h.Shell.Render(w, r, shell.Page{Title: "Ingestion sets", Path: ingSetsBase}, ingSetsPage(v))
}

// files loads the files an apply was given into its row.
func (h *IngestionSets) files(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if err := canReadIngestionSets(auth.FromContext(ctx)); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	name, err := ingSetName(r)
	var applyID string
	if err == nil {
		applyID, err = ingApplyID(r)
	}
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	if !ds.IsDatastar(r) {
		http.Redirect(w, r, ingSetURL(name), http.StatusSeeOther) //nolint:gosec // G710: name is a validated set name
		return
	}
	apply, err := h.apply(ctx, name, applyID)
	var files []ingestion.SetFile
	if err == nil {
		files, err = h.Sets.ApplyFiles(ctx, *apply)
	}
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	if err := ds.NewSSE(w, r).Patch(ingApplyFilesC(applyID, files)); err != nil {
		slog.DebugContext(ctx, "ui: ingestion set files patch not delivered", "err", err)
	}
}

// apply finds one kept apply of the set.
func (h *IngestionSets) apply(ctx context.Context, name, applyID string) (*ingestion.SetApply, error) {
	set, err := h.Sets.FindSetByName(ctx, name)
	if err != nil {
		return nil, usecase.Internal("REPO", "find set failed", err)
	}
	if set == nil {
		return nil, usecase.NotFound("INGESTION_SET_NOT_FOUND", "Ingestion set not found: "+name)
	}
	apply, err := h.Sets.FindSetApply(ctx, set.ID, applyID)
	if err != nil {
		return nil, usecase.Internal("REPO", "find set apply failed", err)
	}
	if apply == nil {
		return nil, usecase.NotFound("INGESTION_SET_APPLY_NOT_FOUND", "the set does not keep apply "+applyID)
	}
	return apply, nil
}

// ── going back ─────────────────────────────────────────────────────────

// ingReapplyView is the confirmation: the plan a dry run made.
type ingReapplyView struct {
	Name, ApplyID string
	Action        string
	Changes       []ingChange
	// Nothing says the set already is as that apply left it.
	Nothing bool
}

// reapplyForm opens the confirmation for going back to an apply, with the
// plan a dry run makes (the SPA's dry run first, then its confirm). The dry
// run is the API's dryRun apply, so it needs the same permission.
func (h *IngestionSets) reapplyForm(w http.ResponseWriter, r *http.Request) {
	name, applyID, err := reapplyTarget(r)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	if !ds.IsDatastar(r) {
		http.Redirect(w, r, ingSetURL(name), http.StatusSeeOther) //nolint:gosec // G710: name is a validated set name
		return
	}
	ctx := r.Context()
	plan, err := h.Ops.Reapply(ctx, name, applyID, true)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	v := ingReapplyView{
		Name: name, ApplyID: applyID, Action: ingSetURL(name) + "/applies/" + applyID + "/reapply",
		Changes: ingChanges(plan.Changes),
	}
	v.Nothing = len(v.Changes) == 0
	if err := patchDialogSlot(ds.NewSSE(w, r), ingSetDialogBox, ingReapplyDialog(v)); err != nil {
		slog.DebugContext(ctx, "ui: ingestion set dialog patch not delivered", "err", err)
	}
}

// reapplyTarget makes the API's checks for going back (the set-apply
// permission; the use case checks reach per ingestion) and reads the set and
// the apply from the path.
func reapplyTarget(r *http.Request) (name, applyID string, err error) {
	if err = auth.CanApplyIngestionSets(auth.FromContext(r.Context())); err != nil {
		return "", "", err
	}
	if name, err = ingSetName(r); err != nil {
		return "", "", err
	}
	applyID, err = ingApplyID(r)
	return name, applyID, err
}

// reapply goes back to the apply: the set is applied again with what that
// apply was given.
func (h *IngestionSets) reapply(w http.ResponseWriter, r *http.Request) {
	name, applyID, err := reapplyTarget(r)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	plan, err := h.Ops.Reapply(r.Context(), name, applyID, false)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	if !ds.IsDatastar(r) {
		h.Shell.SetFlash(w, flashIngSetReapplied)
		http.Redirect(w, r, ingSetURL(name), http.StatusSeeOther) //nolint:gosec // G710: name is a validated set name
		return
	}
	detail := "Set " + name + " is back as apply " + applyID + " left it."
	if plan.ApplyID == nil {
		detail = "Set " + name + " already is as apply " + applyID + " left it."
	}
	h.afterChange(w, r, name, kit.FlashMessageProps{Kind: kit.FlashSuccess, Summary: "Success", Detail: detail})
}

// release lets go of the set.
func (h *IngestionSets) release(w http.ResponseWriter, r *http.Request) {
	name, err := ingSetName(r)
	if err == nil {
		err = auth.CanApplyIngestionSets(auth.FromContext(r.Context()))
	}
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	released, err := h.Ops.Release(r.Context(), name)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	if !ds.IsDatastar(r) {
		h.Shell.SetFlash(w, flashIngSetReleased)
		http.Redirect(w, r, ingSetURL(name), http.StatusSeeOther) //nolint:gosec // G710: name is a validated set name
		return
	}
	h.afterChange(w, r, name, kit.FlashMessageProps{
		Kind: kit.FlashSuccess, Summary: "Success",
		Detail: "Set " + name + " released: " + strconv.Itoa(released) + " ingestion" + ingPlural(int64(released)) + " can be edited one at a time again.",
	})
}

// afterChange answers a Datastar action: the message and, for a caller who
// may read the sets, the sets and the open set's applies again.
func (h *IngestionSets) afterChange(w http.ResponseWriter, r *http.Request, name string, msg kit.FlashMessageProps) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	sse := ds.NewSSE(w, r)
	if err := shell.Flash(sse, msg); err != nil {
		slog.DebugContext(ctx, "ui: ingestion sets flash not delivered", "err", err)
	}
	if canReadIngestionSets(ac) != nil {
		return
	}
	v, err := h.view(ctx, ac, name)
	if err != nil {
		h.Shell.FailSSE(sse, r, err)
		return
	}
	if err := errors.Join(sse.Patch(ingSetsList(v)), sse.Patch(ingSetDetail(v))); err != nil {
		slog.DebugContext(ctx, "ui: ingestion sets patch not delivered", "err", err)
	}
}

// ingSetConfirm is the release confirmation, posted to the set's own URL.
func ingSetConfirm(name string) kit.ConfirmDialogProps {
	return kit.ConfirmDialogProps{
		ID: ingSetRelease, Action: ingSetURL(name) + "/release", Title: "Release set " + name + "?",
		Message:      "Its ingestions stay as they are and are edited one at a time again. Applying the set again takes them back.",
		ConfirmLabel: "Release", Tone: kit.ToneWarn,
		FormAttrs: templ.Attributes{"data-on:submit": kit.SubmitConfirm},
	}
}
