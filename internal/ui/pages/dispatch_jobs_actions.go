package pages

import (
	"cmp"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/common"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ids"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchjob"
	djops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchjob/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/shell"
	"github.com/flowcatalyst/framework/go/ui/ds"
	"github.com/flowcatalyst/framework/go/ui/kit"
	"github.com/flowcatalyst/go-sdk/usecase"
)

// ── requeue (ids) ──────────────────────────────────────────────────────

// requeueOne is the row's and the drawer's Requeue.
func (h *DispatchJobs) requeueOne(w http.ResponseWriter, r *http.Request) {
	id, err := djID(r)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	h.requeue(w, r, []string{id})
}

// redriveOne is the row's and the drawer's Redrive of a COMPLETED job: the
// same two-step dialog as ticked rows that include COMPLETED ones.
func (h *DispatchJobs) redriveOne(w http.ResponseWriter, r *http.Request) {
	id, err := djID(r)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	if err := auth.CanWritePermission(auth.FromContext(r.Context()), djRedrivePerm); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	h.requeue(w, r, []string{id})
}

// requeueSelected is "Requeue selected": the ticked rows' ids.
func (h *DispatchJobs) requeueSelected(w http.ResponseWriter, r *http.Request) {
	var picked []string
	for _, id := range r.PostForm["ids"] {
		if id = strings.TrimSpace(id); id != "" && !slices.Contains(picked, id) {
			picked = append(picked, id)
		}
	}
	h.requeue(w, r, picked)
}

// djSelection is the dialog of a requeue of ticked rows that include COMPLETED
// jobs (RequeueSelectedDialog) or jobs in flight: a plain requeue leaves
// COMPLETED ones alone, redriving them is the separate, dangerous option behind
// a typed confirmation, and requeueing the ones in flight is a checkbox.
type djSelection struct {
	IDs    []string
	Drawer string
	// Completed counts the COMPLETED jobs the caller may redrive (0 without
	// dispatch-job:redrive-completed); InFlight the QUEUED or PROCESSING ones.
	Completed, InFlight int
	// Consent is the in-flight checkbox as the caller last left it.
	Consent bool
	// Confirm is the second step (the word and the count); Error is its
	// message after a wrong entry.
	Confirm bool
	Error   string
}

// others is how many of the ticked rows a plain requeue would reset.
func (s djSelection) others() int { return len(s.IDs) - s.Completed }

// requeue runs a requeue of ids. A caller who may redrive and has COMPLETED
// jobs among them, or any caller with jobs in flight among them who has not
// consented, goes through the dialog (step "" opens it, "confirm" shows the
// typed confirmation, "redrive" runs it); everyone else requeues at once and is
// told what the status guard left alone.
func (h *DispatchJobs) requeue(w http.ResponseWriter, r *http.Request, picked []string) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	if err := auth.CanWritePermission(ac, djRequeuePerm); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	step := r.PostFormValue("step")
	redrivePerm := auth.CanWritePermission(ac, djRedrivePerm)
	if (step == "confirm" || step == "redrive") && redrivePerm != nil {
		h.Shell.Fail(w, r, redrivePerm)
		return
	}
	opts := RequeueOptions{InFlight: formFlag(r.PostFormValue("inflight"))}
	if len(picked) > 0 && step != "plain" {
		sel, err := h.selection(ctx, ac, picked, r.PostFormValue("drawer"))
		if err != nil {
			h.Shell.Fail(w, r, err)
			return
		}
		if redrivePerm != nil {
			sel.Completed = 0
		}
		sel.Consent = opts.InFlight
		switch {
		case step == "redrive":
			if msg := redriveEntryError(r.PostFormValue("word"), r.PostFormValue("count"), sel.Completed); msg != "" {
				sel.Confirm, sel.Error = true, msg
				h.showSelectionDialog(w, r, sel)
				return
			}
			opts.Redrive = true
			h.runRequeue(w, r, ac, picked, opts)
		case step == "confirm" && sel.Completed > 0:
			sel.Confirm = true
			h.showSelectionDialog(w, r, sel)
		case sel.Completed > 0, sel.InFlight > 0 && !opts.InFlight:
			h.showSelectionDialog(w, r, sel)
		case step != "" && sel.Completed == 0 && redrivePerm == nil:
			h.Shell.Fail(w, r, usecase.Validation("NOTHING_TO_REDRIVE", "None of the selected jobs is COMPLETED any more."))
		default:
			h.runRequeue(w, r, ac, picked, opts)
		}
		return
	}
	h.runRequeue(w, r, ac, picked, opts)
}

// formFlag reads a checkbox or hidden flag: "1", "on" or "true".
func formFlag(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "on", "true":
		return true
	}
	return false
}

// selection counts the COMPLETED and in-flight jobs among the ticked ones the
// caller can reach (the use case drops the rest).
func (h *DispatchJobs) selection(ctx context.Context, ac *auth.AuthContext, picked []string, drawer string) (djSelection, error) {
	jobs, err := h.Jobs.FindByIDs(ctx, picked)
	if err != nil {
		return djSelection{}, usecase.Internal("REPO", "find_by_ids failed", err)
	}
	sel := djSelection{IDs: picked, Drawer: drawer}
	for i := range jobs {
		if !auth.CanAccessScope(ac, ids.StringPtr(jobs[i].ClientID)) {
			continue
		}
		switch {
		case jobs[i].Status == common.DispatchCompleted:
			sel.Completed++
		case slices.Contains(dispatchjob.InFlightStatuses, jobs[i].Status):
			sel.InFlight++
		}
	}
	return sel, nil
}

// redriveEntryError checks the second step: the word typed exactly and the
// number of jobs the run resets retyped (thousands separators tolerated).
func redriveEntryError(word, count string, want int) string {
	n, err := strconv.Atoi(strings.NewReplacer(",", "", " ", "").Replace(strings.TrimSpace(count)))
	switch {
	case want <= 0:
		return "There is nothing to redrive."
	case word != djRedriveWord:
		return "Type " + djRedriveWord + " exactly as shown."
	case err != nil || n != want:
		return "Type the number of jobs shown: " + thousands(int64(want)) + "."
	}
	return ""
}

func (h *DispatchJobs) showSelectionDialog(w http.ResponseWriter, r *http.Request, sel djSelection) {
	if !ds.IsDatastar(r) {
		// The dialog exists only as an answer: show the list with it open.
		ac := auth.FromContext(r.Context())
		view, err := h.listView(r.Context(), ac, djReferer(r))
		if err != nil {
			h.Shell.Fail(w, r, err)
			return
		}
		view.Dialog = djSelectionDialog(sel)
		h.renderPage(w, r, http.StatusOK, view)
		return
	}
	if err := patchDialogSlot(ds.NewSSE(w, r), djDialogSlot, djSelectionDialog(sel)); err != nil {
		slog.DebugContext(r.Context(), "ui: requeue dialog not delivered", "err", err)
	}
}

func (h *DispatchJobs) runRequeue(w http.ResponseWriter, r *http.Request, ac *auth.AuthContext, picked []string, opts RequeueOptions) {
	ctx := r.Context()
	var res RequeueResult
	if len(picked) > 0 {
		var err error
		if res, err = h.Ops.Requeue(ctx, picked, opts); err != nil {
			h.Shell.Fail(w, r, err)
			return
		}
	}
	if !ds.IsDatastar(r) {
		h.Shell.SetFlash(w, flashDjRequeued)
		http.Redirect(w, r, djReferer(r).URL(), http.StatusSeeOther) //nolint:gosec // G710: djPath plus a query re-encoded from parsed list state
		return
	}
	sse := ds.NewSSE(w, r)
	h.refreshList(sse, r, ac)
	if drawer := r.PostFormValue("drawer"); drawer != "" && slices.Contains(picked, drawer) {
		h.refreshDrawer(sse, r, ac, drawer)
	}
	if err := errors.Join(sse.CloseDialog(djSelDialog), shell.Flash(sse, idsOutcome(res, len(picked) == 1))); err != nil {
		slog.DebugContext(ctx, "ui: requeue outcome not delivered", "err", err)
	}
}

// idsOutcome is the SPA's toast for a requeue by ids: what was reset, what the
// guard left alone. A single job that was not reset is an error (the drawer
// and row say why).
func idsOutcome(res RequeueResult, single bool) kit.FlashMessageProps {
	skipped := skipNote(int64(res.SkippedInFlight), int64(res.SkippedCompleted))
	if res.Requeued == 0 {
		if single {
			return kit.FlashMessageProps{Kind: kit.FlashError, Summary: "Not requeued", Detail: "No selected job could be requeued." + skipped}
		}
		return kit.FlashMessageProps{Kind: kit.FlashInfo, Summary: "Nothing requeued", Detail: "No selected job could be requeued." + skipped}
	}
	n := int64(res.Requeued)
	return kit.FlashMessageProps{
		Kind: kit.FlashSuccess, Summary: "Requeued " + counted(n, "job") + redrivenNote(int64(res.Redriven)),
		Detail: counted(n, "dispatch job") + " reset to PENDING" + skipped + inFlightNote(int64(res.InFlight)),
	}
}

// skipNote is what a requeue left alone, in words ("" when nothing): jobs in
// flight are never reset, COMPLETED ones only by a confirmed redrive.
func skipNote(inFlight, completed int64) string {
	var parts []string
	if inFlight > 0 {
		parts = append(parts, counted(inFlight, "job")+" in flight (QUEUED or PROCESSING) left alone")
	}
	if completed > 0 {
		parts = append(parts, counted(completed, "COMPLETED job")+" left alone")
	}
	if len(parts) == 0 {
		return ""
	}
	return " — " + strings.Join(parts, "; ")
}

// inFlightNote says how many of the reset jobs were in flight (" 1 of them was
// in flight."), "" when none.
func inFlightNote(n int64) string {
	switch n {
	case 0:
		return ""
	case 1:
		return " 1 of them was in flight."
	}
	return " " + thousands(n) + " of them were in flight."
}

func redrivenNote(n int64) string {
	if n == 0 {
		return ""
	}
	return " (" + counted(n, "COMPLETED job") + " redriven)"
}

// ── cancel / complete ──────────────────────────────────────────────────

// djIgnore is the dialog of "Ignore selected": how many of the ticked rows the
// caller can reach and how many of those the use case would mark CANCELLED.
type djIgnore struct {
	IDs    []string
	Drawer string
	// Cancellable is how many would be cancelled (InFlight of them QUEUED or
	// being delivered), Others how many reachable ones are left as they are
	// (already finished).
	Cancellable, InFlight, Others int
}

// cancelSelected is "Ignore selected": the ticked rows' ids, every unfinished
// one (in flight included) to CANCELLED (dispatch-job:cancel). Step ""
// opens the confirmation, "confirm" runs it.
func (h *DispatchJobs) cancelSelected(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	if err := auth.CanWritePermission(ac, djCancelPerm); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	var picked []string
	for _, id := range r.PostForm["ids"] {
		if id = strings.TrimSpace(id); id != "" && !slices.Contains(picked, id) {
			picked = append(picked, id)
		}
	}
	if r.PostFormValue("step") == "confirm" {
		h.runIgnore(w, r, ac, picked)
		return
	}
	jobs, err := h.Jobs.FindByIDs(ctx, picked)
	if err != nil {
		h.Shell.Fail(w, r, usecase.Internal("REPO", "find_by_ids failed", err))
		return
	}
	ig := djIgnore{IDs: picked, Drawer: r.PostFormValue("drawer")}
	for i := range jobs {
		if !auth.CanAccessScope(ac, ids.StringPtr(jobs[i].ClientID)) {
			continue
		}
		if slices.Contains(dispatchjob.CancellableStatuses, jobs[i].Status) {
			ig.Cancellable++
			if slices.Contains(dispatchjob.InFlightStatuses, jobs[i].Status) {
				ig.InFlight++
			}
		} else {
			ig.Others++
		}
	}
	if ig.Cancellable == 0 {
		h.Shell.Fail(w, r, usecase.Validation("NOTHING_TO_IGNORE", "None of the selected jobs can be ignored: they are all finished."))
		return
	}
	if !ds.IsDatastar(r) {
		// The dialog exists only as an answer: show the list with it open.
		view, err := h.listView(ctx, ac, djReferer(r))
		if err != nil {
			h.Shell.Fail(w, r, err)
			return
		}
		view.Dialog = djIgnoreConfirm(ig)
		h.renderPage(w, r, http.StatusOK, view)
		return
	}
	if err := patchDialogSlot(ds.NewSSE(w, r), djDialogSlot, djIgnoreConfirm(ig)); err != nil {
		slog.DebugContext(ctx, "ui: ignore dialog not delivered", "err", err)
	}
}

func (h *DispatchJobs) runIgnore(w http.ResponseWriter, r *http.Request, ac *auth.AuthContext, picked []string) {
	ctx := r.Context()
	res, err := h.Ops.CancelMany(ctx, picked)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	if !ds.IsDatastar(r) {
		h.Shell.SetFlash(w, flashDjIgnored)
		http.Redirect(w, r, djReferer(r).URL(), http.StatusSeeOther) //nolint:gosec // G710: djPath plus a query re-encoded from parsed list state
		return
	}
	sse := ds.NewSSE(w, r)
	h.refreshList(sse, r, ac)
	if drawer := r.PostFormValue("drawer"); drawer != "" && slices.Contains(picked, drawer) {
		h.refreshDrawer(sse, r, ac, drawer)
	}
	if err := errors.Join(sse.CloseDialog(djIgnoreDialog), shell.Flash(sse, ignoreOutcome(res))); err != nil {
		slog.DebugContext(ctx, "ui: ignore outcome not delivered", "err", err)
	}
}

// ignoreOutcome is the toast for an ignore by ids: what was cancelled, what the
// status guard left alone.
func ignoreOutcome(res CancelResult) kit.FlashMessageProps {
	var skipped string
	if res.Skipped > 0 {
		skipped = " " + thousands(int64(res.Skipped)) + " left as they were: already finished."
	}
	if res.Cancelled == 0 {
		return kit.FlashMessageProps{Kind: kit.FlashInfo, Summary: "Nothing ignored", Detail: "No selected job could be ignored." + skipped}
	}
	summary := "Job ignored"
	if res.Cancelled > 1 {
		summary = counted(int64(res.Cancelled), "job") + " ignored"
	}
	return kit.FlashMessageProps{Kind: kit.FlashSuccess, Summary: summary, Detail: counted(int64(res.Cancelled), "dispatch job") + " marked CANCELLED." + skipped}
}

// cancel is the drawer's "Cancel job": the API's POST /dispatch-jobs/{id}/cancel
// (PENDING or FAILED -> CANCELLED; dispatch-job:cancel). The use case enforces the status
// rule and the client scope.
func (h *DispatchJobs) cancel(w http.ResponseWriter, r *http.Request) {
	h.statusAction(w, r, djCancelPerm, h.Ops.Cancel, flashDjCancelled)
}

// complete is the drawer's "Mark completed": POST /dispatch-jobs/{id}/complete
// (FAILED -> COMPLETED, the delivery happened out of band; dispatch-job:complete).
func (h *DispatchJobs) complete(w http.ResponseWriter, r *http.Request) {
	h.statusAction(w, r, djCompletePerm, h.Ops.Complete, flashDjCompleted)
}

func (h *DispatchJobs) statusAction(w http.ResponseWriter, r *http.Request, perm string, run func(context.Context, string) error, flash string) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	if err := auth.CanWritePermission(ac, perm); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	id, err := djID(r)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	if err := run(ctx, id); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	if !ds.IsDatastar(r) {
		h.Shell.SetFlash(w, flash)
		http.Redirect(w, r, djURL(id), http.StatusSeeOther) //nolint:gosec // G710: djPath plus an id kit.ValidRecordID accepted
		return
	}
	sse := ds.NewSSE(w, r)
	h.refreshList(sse, r, ac)
	h.refreshDrawer(sse, r, ac, id)
	if err := shell.Flash(sse, FlashMessages[flash]); err != nil {
		slog.DebugContext(ctx, "ui: status action flash not delivered", "err", err)
	}
}

// ── edit payload ───────────────────────────────────────────────────────

// editPayload is the drawer's "Edit payload": the API's PUT
// /dispatch-jobs/{id}/payload. Same gate (dispatch-job:edit-payload) and the
// same use case, which enforces the status rule (PENDING / FAILED), the client
// scope and the payload validation. Status and attempts are not touched:
// Requeue stays a separate action.
func (h *DispatchJobs) editPayload(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	if err := auth.CanWritePermission(ac, djEditPayloadPerm); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	id, err := djID(r)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	var contentType *string
	if ct := strings.TrimSpace(r.PostFormValue("contentType")); ct != "" {
		contentType = &ct
	}
	if err := h.Ops.EditPayload(ctx, id, r.PostFormValue("payload"), contentType); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	if !ds.IsDatastar(r) {
		h.Shell.SetFlash(w, flashDjPayloadSaved)
		http.Redirect(w, r, djURL(id), http.StatusSeeOther) //nolint:gosec // G710: djPath plus an id kit.ValidRecordID accepted
		return
	}
	sse := ds.NewSSE(w, r)
	h.refreshDrawer(sse, r, ac, id)
	if err := shell.Flash(sse, FlashMessages[flashDjPayloadSaved]); err != nil {
		slog.DebugContext(ctx, "ui: payload flash not delivered", "err", err)
	}
}

// refreshList re-renders the list view the request came from.
func (h *DispatchJobs) refreshList(sse *ds.SSE, r *http.Request, ac *auth.AuthContext) {
	ctx := r.Context()
	view, err := h.listView(ctx, ac, djReferer(r))
	if err != nil {
		h.Shell.FailSSE(sse, r, err)
		return
	}
	if err := errors.Join(patchDjTable(sse, view), sse.Patch(kit.CursorPager(view.pager())), sse.Patch(djBulkBar(view)), sse.PatchSignals(map[string]any{djSelSig: 0})); err != nil {
		slog.DebugContext(ctx, "ui: dispatch jobs table patch not delivered", "err", err)
	}
}

func (h *DispatchJobs) refreshDrawer(sse *ds.SSE, r *http.Request, ac *auth.AuthContext, id string) {
	ctx := r.Context()
	if !kit.ValidRecordID(id) {
		return
	}
	j, err := h.loadJob(ctx, ac, id)
	if err != nil {
		h.Shell.FailSSE(sse, r, err)
		return
	}
	d, err := h.detailOf(ctx, ac, j)
	if err != nil {
		h.Shell.FailSSE(sse, r, err)
		return
	}
	if err := patchDjDrawer(sse, d); err != nil {
		slog.DebugContext(ctx, "ui: dispatch job drawer patch not delivered", "err", err)
	}
}

// ── requeue by filter ──────────────────────────────────────────────────

type djStatusCount struct {
	Status   string
	Count    int64
	Severity kit.TagSeverity
}

// djPreview is the RequeueByFilterDialog: the filter in words, what it
// matches, what this caller may requeue, and the form that acts on exactly
// that filter (its URL carries it).
type djPreview struct {
	Summary []djKV
	// Window is the created_at range the counts cover, in words.
	Window          string
	Count, Eligible int64
	OutOfScope      int64
	// Completed and InFlight are matches a requeue leaves alone: COMPLETED
	// ones (redriven only with the API's two redrive fields) and QUEUED or
	// PROCESSING ones (never).
	Completed, InFlight int64
	ToRequeue           int64
	OverCap, Changed    bool
	ByStatus            []djStatusCount
	Action              string
	// CanRedrive offers "Also redrive the COMPLETED jobs" (redrive-completed);
	// RedriveCount is the number a redrive confirms (every job it resets).
	CanRedrive   bool
	RedriveCount int64
	// RedriveHref opens the redrive step and Review returns to the first
	// (the filter's own URL).
	RedriveHref, Review string
	// Redrive is the second step (the word and the count); Error is its
	// message after a wrong entry.
	Redrive bool
	Error   string
}

func (p djPreview) title() string {
	if p.Redrive {
		return "Redrive COMPLETED jobs"
	}
	return "Requeue all matching jobs"
}

// confirmCount is the number the run is confirmed with: the eligible jobs, or
// with a redrive those plus the COMPLETED ones.
func (p djPreview) confirmCount() int64 {
	if p.Redrive {
		return p.RedriveCount
	}
	return p.Eligible
}

// djRBFOpts is how the requeue-by-filter dialog opens: after a changed
// match (nothing was reset), or at the redrive step.
type djRBFOpts struct {
	Changed, Redrive bool
	Error            string
}

// summary is summarizeFilters: the active filters as label/value lines.
func (q djQuery) summary(clientNames map[string]string) []djKV {
	var out []djKV
	add := func(label string, vals []string, labels map[string]string) {
		if len(vals) == 0 {
			return
		}
		shown := make([]string, 0, len(vals))
		for _, v := range vals {
			shown = append(shown, cmp.Or(labels[v], v))
		}
		out = append(out, djKV{Key: label, Value: strings.Join(shown, ", ")})
	}
	add("Client", q.facet(djFacetClients), clientNames)
	add("Application", q.facet(djFacetApps), nil)
	add("Subdomain", q.facet(djFacetSubs), nil)
	add("Aggregate", q.facet(djFacetAggs), nil)
	add("Code", q.facet(djFacetCodes), nil)
	add("Status", q.facet(djFacetStatuses), nil)
	add("Event type", q.EventTypes, nil)
	windowLabel := ""
	if p, ok := djWindowPresets[q.Window]; ok && q.From == "" && q.To == "" {
		windowLabel = p.label
	}
	for _, kv := range []djKV{{"Source", q.Q}, {"Message group", q.Group}, {"Subscription", q.Subscription}, {"Connection", q.Connection}, {"Window", windowLabel}, {"Created from", q.From}, {"Created to", q.To}} {
		if kv.Value != "" {
			out = append(out, kv)
		}
	}
	return out
}

func (h *DispatchJobs) preview(ctx context.Context, ac *auth.AuthContext, q djQuery, names map[string]string, opts djRBFOpts) (djPreview, error) {
	f, win, err := h.scoped(ctx, ac, q)
	if err != nil {
		return djPreview{}, err
	}
	sum, err := h.Ops.Preview(ctx, f)
	if err != nil {
		return djPreview{}, err
	}
	window := win.From.Format("2006-01-02 15:04") + " – " + win.To.Format("2006-01-02 15:04") + " UTC"
	if win.Defaulted {
		window += " (the last 24 hours: no dates chosen)"
	}
	p := djPreview{
		Summary: q.summary(names), Window: window, Count: sum.Count, Eligible: sum.Eligible, Completed: sum.Completed, InFlight: sum.InFlight,
		OutOfScope: sum.Count - sum.Eligible - sum.Completed - sum.InFlight,
		ToRequeue:  min(sum.Eligible, int64(djMaxRequeue)), OverCap: sum.Eligible > int64(djMaxRequeue), Changed: opts.Changed,
		Action:     encodeURL(djPath+"/requeue-by-filter", q.filterValues()),
		CanRedrive: sum.Completed > 0 && auth.CanWritePermission(ac, djRedrivePerm) == nil, RedriveCount: sum.RedriveEligible(),
		Error: opts.Error,
	}
	p.Redrive = opts.Redrive && p.CanRedrive
	p.Review = p.Action
	step := q.filterValues()
	step.Set("step", "redrive")
	p.RedriveHref = encodeURL(djPath+"/requeue-by-filter", step)
	for status, n := range sum.ByStatus {
		p.ByStatus = append(p.ByStatus, djStatusCount{Status: status, Count: n, Severity: djSeverity(status)})
	}
	slices.SortFunc(p.ByStatus, func(a, b djStatusCount) int {
		return cmp.Or(cmp.Compare(b.Count, a.Count), strings.Compare(a.Status, b.Status))
	})
	return p, nil
}

// requeueByFilterDialog opens the confirm step for the view in the query.
func (h *DispatchJobs) requeueByFilterDialog(w http.ResponseWriter, r *http.Request) {
	h.openRequeueByFilter(w, r, parseDJQuery(r.URL.Query()), false, r.URL.Query().Get("step") == "redrive")
}

// requeueErrors is "Requeue everything in error": the list switches to every
// job in an error status (whatever else it was filtered by) and the same
// confirm step opens for that view.
func (h *DispatchJobs) requeueErrors(w http.ResponseWriter, r *http.Request) {
	q := parseDJQuery(url.Values{djFacetStatuses: djErrorStatuses})
	keep := parseDJQuery(r.URL.Query())
	q.Size, q.Window, q.From, q.To = keep.Size, keep.Window, keep.From, keep.To
	h.openRequeueByFilter(w, r, q, true, false)
}

func (h *DispatchJobs) openRequeueByFilter(w http.ResponseWriter, r *http.Request, q djQuery, switchList, redrive bool) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	if err := auth.CanWritePermission(ac, djRequeuePerm); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	h.showRequeueDialog(w, r, ac, q, switchList, djRBFOpts{Redrive: redrive})
}

// showRequeueDialog answers with the confirm step for q: patched into the
// dialog slot (with the list switched to q when switchList), or, without
// Datastar, the page at q with the dialog open.
func (h *DispatchJobs) showRequeueDialog(w http.ResponseWriter, r *http.Request, ac *auth.AuthContext, q djQuery, switchList bool, opts djRBFOpts) {
	ctx := r.Context()
	live := ds.IsDatastar(r)
	var view djView
	var names map[string]string
	if !live || switchList {
		v, err := h.listView(ctx, ac, q.first())
		if err != nil {
			h.Shell.Fail(w, r, err)
			return
		}
		view, names = v, v.ClientNames
	} else {
		cs, err := allClients(ctx, h.Clients)
		if err != nil {
			h.Shell.Fail(w, r, err)
			return
		}
		names = clientNames(cs)
	}
	p, err := h.preview(ctx, ac, q, names, opts)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	if !live {
		view.Dialog = djRequeueDialog(p)
		status := http.StatusOK
		if opts.Changed {
			status = http.StatusConflict
		}
		h.renderPage(w, r, status, view)
		return
	}
	sse := ds.NewSSE(w, r)
	var errs []error
	if switchList {
		errs = append(errs, view.patchList(sse, true))
	}
	errs = append(errs, patchDialogSlot(sse, djDialogSlot, djRequeueDialog(p)))
	if err := errors.Join(errs...); err != nil {
		slog.DebugContext(ctx, "ui: requeue-by-filter dialog not delivered", "err", err)
	}
}

// requeueByFilter runs the confirmed requeue on the filter in its URL. More
// eligible jobs than confirmed (FILTER_MATCH_CHANGED): nothing was reset, so
// the dialog shows the new counts and asks again.
func (h *DispatchJobs) requeueByFilter(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	if err := auth.CanWritePermission(ac, djRequeuePerm); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	q := parseDJQuery(r.URL.Query())
	confirm, err := strconv.ParseInt(r.PostFormValue("confirmCount"), 10, 64)
	if err != nil || confirm < 0 {
		h.Shell.Fail(w, r, usecase.Validation("INVALID_CONFIRM_COUNT", "confirmCount must be a whole number, not negative"))
		return
	}
	f, _, err := h.scoped(ctx, ac, q)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	redrive := r.PostFormValue("redrive") != ""
	if redrive {
		if err := auth.CanWritePermission(ac, djRedrivePerm); err != nil {
			h.Shell.Fail(w, r, err)
			return
		}
		// The typed word and count come first: a wrong entry runs nothing.
		if msg := redriveEntryError(r.PostFormValue("word"), r.PostFormValue("count"), int(confirm)); msg != "" {
			h.showRequeueDialog(w, r, ac, q, false, djRBFOpts{Redrive: true, Error: msg})
			return
		}
	}
	res, err := h.Ops.RequeueByFilter(ctx, f, confirm, redrive)
	if ue := usecase.AsError(err); ue != nil && ue.Code == "FILTER_MATCH_CHANGED" {
		h.showRequeueDialog(w, r, ac, q, false, djRBFOpts{Changed: true})
		return
	}
	if err != nil {
		if ds.IsDatastar(r) {
			// Batches before a failure stay committed: the list is stale
			// either way (the SPA reloads it too).
			sse := ds.NewSSE(w, r)
			h.Shell.FailSSE(sse, r, err)
			h.refreshList(sse, r, ac)
			return
		}
		h.Shell.Fail(w, r, err)
		return
	}
	if !ds.IsDatastar(r) {
		h.Shell.SetFlash(w, flashDjRequeuedByFilter)
		http.Redirect(w, r, q.URL(), http.StatusSeeOther) //nolint:gosec // G710: djPath plus a query re-encoded from parsed list state
		return
	}
	sse := ds.NewSSE(w, r)
	h.refreshList(sse, r, ac)
	if err := errors.Join(sse.CloseDialog(djRBFDialog), shell.Flash(sse, requeueOutcome(res))); err != nil {
		slog.DebugContext(ctx, "ui: requeue-by-filter outcome not delivered", "err", err)
	}
}

// requeueOutcome is the SPA's toast for a finished run.
func requeueOutcome(res djops.ResendByFilterResult) kit.FlashMessageProps {
	skipped := skipNote(res.SkippedInFlight, res.SkippedCompleted)
	summary := "Requeued " + counted(res.Requeued, "job") + redrivenNote(res.Redriven)
	switch {
	case res.Remaining > 0:
		return kit.FlashMessageProps{Kind: kit.FlashWarn, Summary: summary, Detail: thousands(res.Remaining) + " remain — run it again to continue." + skipped}
	case res.Requeued == 0:
		return kit.FlashMessageProps{Kind: kit.FlashInfo, Summary: "Nothing requeued", Detail: "No matching job could be requeued." + skipped}
	}
	return kit.FlashMessageProps{Kind: kit.FlashSuccess, Summary: summary, Detail: counted(res.Requeued, "dispatch job") + " reset to PENDING." + skipped}
}
