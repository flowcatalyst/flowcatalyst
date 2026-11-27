package pages

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/go-chi/chi/v5"

	"github.com/flowcatalyst/flowcatalyst-go/internal/common"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ids"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/audit"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchjob"
	djops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchjob/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/internal/schemacodegen"
	"github.com/flowcatalyst/framework/go/ui/ds"
	"github.com/flowcatalyst/framework/go/ui/kit"
	"github.com/flowcatalyst/go-sdk/usecase"
)

// ── drawer ─────────────────────────────────────────────────────────────

type djKV struct{ Key, Value string }

type djAttempt struct {
	Label           string
	Outcome         string
	OutcomeSeverity kit.TagSeverity
	At              time.Time
	Duration        string
	Sent, Timestamp string
	Headers, Error  string
	Response        string
	AttemptOfTheRun string
}

type djDetail struct {
	ID, Title                  string
	Status                     string
	StatusSeverity             kit.TagSeverity
	Descriptor, Code, ClientID string
	Group, Mode, Target        string
	Subscription, Event        string
	// SubName, SubCode and SubHref name the subscription, ConnCode and
	// ConnHref its connection (best effort: empty for a caller who may not
	// read them, then the bare id shows); ConnShown says the job has a
	// subscription, so the Connection line is drawn.
	SubName, SubCode, SubHref string
	ConnCode, ConnHref        string
	ConnShown                 bool
	Attempts                  string
	ScheduledFor, Created     time.Time
	Completed                 time.Time
	LastError                 string
	Metadata                  []djKV
	Payload                   string
	History                   []djAttempt
	ActionBase                string
	CanSign                   bool
	// CanRequeue shows the drawer's Requeue (dispatch-job:requeue, and a status
	// the API resets); CanRedrive its Redrive (a COMPLETED job, redrive-completed).
	CanRequeue, CanRedrive bool
	// CanRequeueInFlight shows the drawer's Requeue of a job in flight, behind
	// its own confirmation (a delivery under way may land as well); InFlight
	// picks the Cancel job confirmation that says so.
	CanRequeueInFlight, InFlight bool
	// CanAudit shows the Audit section (audit-log view); AuditHref loads it.
	CanAudit  bool
	AuditHref string
	// CanEditPayload is true while the job's payload may be edited (PENDING or
	// FAILED: the API's rule); EditText and ContentType fill the editor.
	CanEditPayload bool
	// CanCancel and CanComplete show the drawer's Cancel job / Mark completed:
	// the job is FAILED (the API's rule) and the caller holds dispatch-job:cancel
	// / dispatch-job:complete.
	CanCancel, CanComplete bool
	EditText               string
	ContentType            string
	// Plan is the sign action's answer, when the page renders it (without
	// Datastar); a live drawer patches it into its slot instead.
	Plan *djPlan
}

// editorLanguage is the payload editor's language: JSON for a JSON content type.
func (d djDetail) editorLanguage() string {
	if strings.Contains(strings.ToLower(d.ContentType), "json") {
		return "json"
	}
	return ""
}

func (d djDetail) drawerProps() kit.DrawerProps {
	return kit.DrawerProps{ID: djDrawerID, Signal: djDrawerSig, Label: "Dispatch job", Size: kit.DrawerTwoThirds, Title: d.Title, Subtitle: d.ID}
}

// djDrawerSeverity is the drawer's statusSeverity (it differs from the
// list's: CANCELLED and EXPIRED are secondary, anything else not named is
// info).
func djDrawerSeverity(status string) kit.TagSeverity {
	switch status {
	case "COMPLETED":
		return kit.TagSuccess
	case "FAILED":
		return kit.TagDanger
	case "PROCESSING":
		return kit.TagWarn
	case "CANCELLED", "EXPIRED":
		return kit.TagSecondary
	}
	return kit.TagInfo
}

// attemptSeverity is the SPA's: success, a deferral (429 or a 2xx that was
// not acknowledged) is a warning, anything else danger.
func attemptSeverity(a *dispatchjob.Attempt) kit.TagSeverity {
	if a.Success {
		return kit.TagSuccess
	}
	code := 0
	if a.ResponseCode != nil {
		code = *a.ResponseCode
	}
	if code == http.StatusTooManyRequests || (code >= 200 && code < 300) {
		return kit.TagWarn
	}
	return kit.TagDanger
}

// requestLine is the SPA's summary of what was sent.
func requestLine(r *dispatchjob.RequestSummary) string {
	if r == nil {
		return "not recorded"
	}
	if r.UnsignedReason != "" {
		return "UNSIGNED — " + r.UnsignedReason
	}
	var parts []string
	if r.Signature {
		parts = append(parts, "signed by "+cmp.Or(r.SignedBy, "?"))
	}
	if r.Bearer {
		parts = append(parts, "bearer")
	}
	if len(parts) == 0 {
		return "no credentials"
	}
	return strings.Join(parts, " + ")
}

// prettyText indents a JSON document (the SPA's formatJson); anything else
// is shown as stored, and nothing as "-".
// jobPayloadText is the drawer's payload: prettyText, or a plain statement when
// it could not be read from the payload store.
func jobPayloadText(j *dispatchjob.DispatchJob) string {
	if j.PayloadUnavailable {
		return "Unavailable: the payload could not be read from the payload store."
	}
	return prettyText(j.Payload)
}

func prettyText(s *string) string {
	if s == nil || *s == "" {
		return "-"
	}
	if p, ok := schemacodegen.PrettyJSON(*s); ok {
		return p
	}
	return *s
}

func toAttempts(rows []dispatchjob.Attempt) []djAttempt {
	out := make([]djAttempt, 0, len(rows))
	// Latest first; the position, not the stored number, is the label
	// (every requeue starts a new run numbered from 1).
	for i := len(rows) - 1; i >= 0; i-- {
		a := &rows[i]
		at := djAttempt{
			Label: "#" + strconv.Itoa(i+1), At: a.AttemptedAt, OutcomeSeverity: attemptSeverity(a),
			Sent:            requestLine(a.Request),
			AttemptOfTheRun: "Attempt " + strconv.Itoa(int(a.AttemptNumber)) + " of its run — a requeue starts a new run numbered from 1",
		}
		switch {
		case a.ResponseCode != nil && *a.ResponseCode != 0:
			at.Outcome = strconv.Itoa(*a.ResponseCode)
		case a.ErrorType != nil && *a.ErrorType != "":
			at.Outcome = string(*a.ErrorType)
		default:
			at.Outcome = "no response"
		}
		if a.DurationMillis != nil {
			at.Duration = strconv.FormatInt(*a.DurationMillis, 10) + "ms"
		}
		if a.Request != nil {
			at.Timestamp = a.Request.Timestamp
			at.Headers = strings.Join(a.Request.Headers, ", ")
		}
		at.Error = deref(a.ErrorMessage)
		if a.ResponseBody != nil && *a.ResponseBody != "" {
			at.Response = prettyText(a.ResponseBody)
		}
		out = append(out, at)
	}
	return out
}

func timeOr(t *time.Time) time.Time {
	if t == nil {
		return time.Time{}
	}
	return *t
}

// loadJob reads a job with the API's checks: not found, then the job's
// client scope.
func (h *DispatchJobs) loadJob(ctx context.Context, ac *auth.AuthContext, id string) (*dispatchjob.DispatchJob, error) {
	j, err := h.Jobs.FindByID(ctx, id)
	if err != nil {
		return nil, usecase.Internal("REPO", "find_by_id failed", err)
	}
	if j == nil {
		return nil, httperror.NotFound("DispatchJob", id)
	}
	if err := auth.CheckScopeAccess(ac, ids.StringPtr(j.ClientID)); err != nil {
		return nil, err
	}
	return j, nil
}

func (h *DispatchJobs) detailOf(ctx context.Context, ac *auth.AuthContext, j *dispatchjob.DispatchJob) (djDetail, error) {
	h.Jobs.ResolvePayloadForView(ctx, j)
	attempts, err := h.Jobs.AttemptsByJob(ctx, j.ID)
	if err != nil {
		return djDetail{}, usecase.Internal("REPO", "attempts failed", err)
	}
	d := djDetail{
		ID: j.ID, Title: cmp.Or(deref(j.Descriptor), j.Code, "Dispatch job"),
		Status: string(j.Status), StatusSeverity: djDrawerSeverity(string(j.Status)),
		Descriptor: cmp.Or(deref(j.Descriptor), "-"), Code: j.Code, ClientID: cmp.Or(deref(j.ClientID), "-"),
		Group: cmp.Or(deref(j.MessageGroup), "-"), Mode: string(j.Mode), Target: j.TargetURL,
		Subscription: cmp.Or(deref(j.SubscriptionID), "-"), Event: cmp.Or(deref(j.EventID), "-"),
		Attempts:     fmt.Sprintf("%d / %d", j.AttemptCount, j.MaxRetries),
		ScheduledFor: timeOr(j.ScheduledFor), Created: j.CreatedAt, Completed: timeOr(j.CompletedAt),
		LastError: deref(j.LastError), Payload: jobPayloadText(j),
		History: toAttempts(attempts), ActionBase: djURL(j.ID),
		CanSign:            auth.CanWritePermission(ac, djRawPerm) == nil && ac.IsAnchor(),
		CanRequeue:         auth.CanWritePermission(ac, djRequeuePerm) == nil && slices.Contains(dispatchjob.RequeueableStatuses, j.Status),
		CanRequeueInFlight: slices.Contains(dispatchjob.InFlightStatuses, j.Status) && auth.CanWritePermission(ac, djRequeuePerm) == nil,
		InFlight:           slices.Contains(dispatchjob.InFlightStatuses, j.Status),
		CanRedrive:         j.Status == common.DispatchCompleted && auth.CanWritePermission(ac, djRequeuePerm) == nil && auth.CanWritePermission(ac, djRedrivePerm) == nil,
		CanAudit:           h.Audit != nil && auth.CanWritePermission(ac, djAuditPerm) == nil, AuditHref: djURL(j.ID) + "/audit",
		// The editor opens on the stored text, pretty-printed for a JSON job.
		CanEditPayload: slices.Contains(dispatchjob.EditableStatuses, j.Status) && auth.CanWritePermission(ac, djEditPayloadPerm) == nil,
		ContentType:    j.PayloadContentType,
		CanCancel:      slices.Contains(dispatchjob.CancellableStatuses, j.Status) && auth.CanWritePermission(ac, djCancelPerm) == nil,
		CanComplete:    j.Status == common.DispatchFailed && auth.CanWritePermission(ac, djCompletePerm) == nil,
	}
	if j.Payload != nil {
		d.EditText = *j.Payload
		if pretty, ok := schemacodegen.PrettyJSON(*j.Payload); ok && strings.Contains(strings.ToLower(j.PayloadContentType), "json") {
			d.EditText = pretty
		}
	}
	for _, m := range j.Metadata {
		d.Metadata = append(d.Metadata, djKV{Key: m.Key, Value: m.Value})
	}
	h.nameRefs(ctx, ac, j, &d)
	return d, nil
}

// nameRefs fills the drawer's subscription and connection links, as the SPA
// does after the job loads: best effort, with the reads' own checks, so a
// caller who may not read subscriptions or connections (or a record that is
// gone) keeps the bare id.
func (h *DispatchJobs) nameRefs(ctx context.Context, ac *auth.AuthContext, j *dispatchjob.DispatchJob, d *djDetail) {
	if j.SubscriptionID == nil || *j.SubscriptionID == "" {
		return
	}
	d.ConnShown = true
	if h.Subs == nil || auth.CanReadSubscriptions(ac) != nil {
		return
	}
	sub, err := h.Subs.FindByID(ctx, *j.SubscriptionID)
	if err != nil || sub == nil || (sub.ClientID != nil && !ac.CanAccessClient(string(*sub.ClientID))) {
		return
	}
	d.SubName, d.SubCode, d.SubHref = sub.Name, sub.Code, "/ui/subscriptions/"+url.PathEscape(sub.ID)
	if sub.ConnectionID == nil || *sub.ConnectionID == "" || h.Connections == nil || auth.CanReadConnections(ac) != nil {
		return
	}
	conn, err := h.Connections.FindByID(ctx, *sub.ConnectionID)
	if err != nil || conn == nil || (conn.ClientID != nil && !ac.CanAccessClient(string(*conn.ClientID))) {
		return
	}
	d.ConnCode, d.ConnHref = conn.Code, "/ui/connections/"+url.PathEscape(conn.ID)
}

func djID(r *http.Request) (string, error) {
	id := chi.URLParam(r, "id")
	if !kit.ValidRecordID(id) {
		return "", httperror.NotFound("DispatchJob", id)
	}
	return id, nil
}

func (h *DispatchJobs) detail(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	if err := auth.CanWritePermission(ac, djViewPerm); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	id, err := djID(r)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	j, err := h.loadJob(ctx, ac, id)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	d, err := h.detailOf(ctx, ac, j)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	if ds.IsDatastar(r) {
		if err := openDjDrawer(ds.NewSSE(w, r), d); err != nil {
			slog.DebugContext(ctx, "ui: dispatch job drawer patch not delivered", "err", err)
		}
		return
	}
	view, err := h.listView(ctx, ac, parseDJQuery(nil))
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	view.Drawer = &d
	h.renderPage(w, r, http.StatusOK, view)
}

// patchDjTable replaces the table rather than morphing it: a morph keeps
// each row's <time> and resets its text to the UTC fallback without
// re-running the localisation (local-time.md, Behaviour), and replacing
// 1,000 rows is cheaper than diffing them.
func patchDjTable(sse *ds.SSE, v djView) error {
	return sse.Patch(djTable(v), ds.WithMode(ds.ModeReplace))
}

// patchDjDrawer fills the drawer: the content morphs in (header, focus
// target), then the body is replaced so its local times are localised again.
func patchDjDrawer(sse *ds.SSE, d djDetail) error {
	return errors.Join(
		sse.Patch(djDrawerContent(d), ds.Inner("#"+djDrawerID)),
		sse.Patch(djDrawerBody(d), ds.WithMode(ds.ModeReplace)),
	)
}

func openDjDrawer(sse *ds.SSE, d djDetail) error {
	return errors.Join(
		patchDjDrawer(sse, d),
		sse.PatchSignals(map[string]any{djDrawerSig: d.ID}),
		sse.Focus(djDrawerID+"-title"),
	)
}

// ── audit ──────────────────────────────────────────────────────────────

type djAuditEntry struct {
	Operation, Actor string
	At               time.Time
}

// djAudit is the drawer's Audit section: who changed the job by hand, newest
// first. Failed is set when the rows could not be read.
type djAudit struct {
	Entries []djAuditEntry
	Failed  bool
}

// auditOperationLabel turns an audit operation name into words:
// "CancelCommand" is "Cancel", "EditPayloadCommand" is "Edit payload".
func auditOperationLabel(op string) string {
	base := strings.TrimSuffix(op, "Command")
	var b strings.Builder
	for i, r := range base {
		if i > 0 && unicode.IsUpper(r) && !unicode.IsUpper(rune(base[i-1])) {
			b.WriteByte(' ')
		}
		b.WriteRune(unicode.ToLower(r))
	}
	words := b.String()
	if words == "" {
		return op
	}
	return strings.ToUpper(words[:1]) + words[1:]
}

// auditSection fills the drawer's Audit section when it is opened (the SPA
// reads GET /api/audit-logs/entity/Dispatchjob/{id} then): the audit-log view
// permission, and the job's own checks.
func (h *DispatchJobs) auditSection(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	id, err := djID(r)
	if err == nil && h.Audit == nil {
		err = httperror.NotFound("DispatchJob", id)
	}
	if err == nil {
		if err = auth.CanWritePermission(ac, djViewPerm); err == nil {
			if err = auth.CanWritePermission(ac, djAuditPerm); err == nil {
				_, err = h.loadJob(ctx, ac, id)
			}
		}
	}
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	if !ds.IsDatastar(r) {
		http.Redirect(w, r, djURL(id), http.StatusSeeOther) //nolint:gosec // G710: djPath plus an id kit.ValidRecordID accepted
		return
	}
	entity := djAuditEntity
	rows, err := h.Audit.FindWithFilters(ctx, audit.FilterParams{EntityType: &entity, EntityID: &id, Limit: 500})
	a := djAudit{Failed: err != nil}
	if err != nil {
		slog.WarnContext(ctx, "ui: dispatch job audit not read", "id", id, "err", err)
	}
	for _, l := range rows {
		a.Entries = append(a.Entries, djAuditEntry{Operation: auditOperationLabel(l.Operation), Actor: cmp.Or(deref(l.PrincipalName), deref(l.PrincipalID), "system"), At: l.PerformedAt})
	}
	if err := ds.NewSSE(w, r).Patch(djAuditBody(a), ds.WithMode(ds.ModeReplace)); err != nil {
		slog.DebugContext(ctx, "ui: dispatch job audit patch not delivered", "err", err)
	}
}

// ── sign ───────────────────────────────────────────────────────────────

type djPlan struct {
	Sent, Target string
	Headers      []djKV
	Verify, Body string
}

// verifyCommand is the subscriber-side check, as one paste (the SPA's).
func verifyCommand(headers map[string]string) string {
	return "php artisan flowcatalyst:verify-signature --timestamp='" + headers["X-FlowCatalyst-Timestamp"] +
		"' --signature='" + headers["X-FlowCatalyst-Signature"] + "' --body-file=body.json"
}

// sign is the dry run: the delivery as it would go out now, with a real
// signature, without sending it. Gated on the raw-view permission (the API's).
func (h *DispatchJobs) sign(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	// The answer carries a live signature (and the body): no-store, not the
	// stream helper's no-cache.
	w = noStore{w}
	if err := auth.CanWritePermission(ac, djRawPerm); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	id, err := djID(r)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	j, err := h.loadJob(ctx, ac, id)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	if err := djops.RequireSignAccess(ctx, ac, h.Apps, j); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	plan, err := h.Ops.Plan(ctx, j)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	p := djPlan{Sent: requestLine(plan.Request), Verify: verifyCommand(plan.Headers), Body: plan.Body}
	if plan.Request != nil {
		p.Target = plan.Request.Target
	}
	for _, k := range slices.Sorted(maps.Keys(plan.Headers)) {
		p.Headers = append(p.Headers, djKV{Key: k, Value: plan.Headers[k]})
	}
	if !ds.IsDatastar(r) {
		// The plan exists only as an answer: without Datastar, show the
		// drawer page with it.
		d, err := h.detailOf(ctx, ac, j)
		if err != nil {
			h.Shell.Fail(w, r, err)
			return
		}
		view, err := h.listView(ctx, ac, parseDJQuery(nil))
		if err != nil {
			h.Shell.Fail(w, r, err)
			return
		}
		d.Plan = &p
		view.Drawer = &d
		h.renderPage(w, r, http.StatusOK, view)
		return
	}
	if err := ds.NewSSE(w, r).Patch(djPlanSection(p), ds.Inner("#"+djDrawerID+"-plan")); err != nil {
		slog.DebugContext(ctx, "ui: delivery plan patch not delivered", "err", err)
	}
}
