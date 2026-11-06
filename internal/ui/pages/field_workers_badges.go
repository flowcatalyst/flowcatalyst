package pages

import (
	"context"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/a-h/templ"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/fieldidentity"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/framework/go/ui/ds"
	"github.com/flowcatalyst/framework/go/ui/kit"
	"github.com/flowcatalyst/go-sdk/usecase"
)

// The badge and console sign-in code parts of the field workers page
// (docs/field-worker-sign-in-options-spec.md sections 3 and 4.2). Each action
// makes the admin API's check (badges: manage, as POST /api/field-workers/{id}/badges
// and POST /api/field-workers/badges; sign-in code: CanSignInAsFieldWorker)
// and runs the same operation. A badge token and a sign-in code are in the
// answer to the post that made them only: never in a URL, a redirect, a flash,
// a signal, a log or a fragment the page can fetch again; the response is
// no-store and "Done" removes the dialog.

// fieldBadgeCard is one printable badge: who it is for and the token it
// carries (as text and as the QR).
type fieldBadgeCard struct {
	Name, Code, Client, Token string
}

// fieldBadgeReveal is what the dialog after issuing shows, once.
type fieldBadgeReveal struct {
	Cards []fieldBadgeCard
	// Replaced is set when a worker's earlier badge was cancelled by the issue.
	Replaced bool
}

func (b fieldBadgeReveal) title() string {
	if len(b.Cards) == 1 {
		return "Badge for " + b.Cards[0].Code
	}
	return "Badges to print"
}

// fieldBadgeQR is the badge token drawn as an inline SVG; the QR content is
// the bare token, so a scanner app or a keyboard-wedge scanner yields exactly
// the token. The label names the worker, never the token.
func fieldBadgeQR(c fieldBadgeCard) templ.Component {
	q, err := qrSVGLabeled(c.Token, "Badge QR code for "+c.Code)
	if err != nil {
		return templ.Raw(`<p class="text-sm">The QR code could not be drawn.</p>`)
	}
	return q
}

// ── issue / reissue ───────────────────────────────────────────────────────

// issueBadge mirrors POST /api/field-workers/{id}/badges: it issues a badge
// and cancels the worker's earlier one (one ACTIVE badge per worker). The
// token is generated here, as the API does, and only its hash is stored.
func (h *FieldWorkers) issueBadge(w http.ResponseWriter, r *http.Request) {
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
	token, err := fieldidentity.NewBadgeToken()
	if err != nil {
		h.Shell.Fail(w, r, usecase.Internal("TOKEN_GENERATE", "could not generate a badge token", err))
		return
	}
	prev, err := h.Store.ActiveBadge(ctx, i.ID)
	if err != nil {
		h.Shell.Fail(w, r, usecase.Internal("REPO", "find_field_badge failed", err))
		return
	}
	w = noStore{w}
	res, err := h.Ops.IssueBadge(ctx, fieldidentity.IssueBadgeCommand{WorkerRef: refOfWorker(i), Token: token})
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	reveal := fieldBadgeReveal{
		Cards:    []fieldBadgeCard{{Name: i.Name, Code: i.WorkerCode, Client: h.clientLabel(ctx, ac, i.ClientID), Token: token}},
		Replaced: prev != nil && res.Badge != nil,
	}
	h.revealBadges(w, r, ac, i.ClientID, id, reveal, flashFieldBadgeIssued, i.WorkerCode+" has a new badge; it is shown once")
}

// revealBadges answers a badge issue: the dialog with the card(s), then the
// refreshed list and drawer. A native post (no JavaScript) gets the page with
// the dialog, never a redirect that could carry a token.
func (h *FieldWorkers) revealBadges(w http.ResponseWriter, r *http.Request, ac *auth.AuthContext, clientID, id string, reveal fieldBadgeReveal, flash, detail string) {
	ctx := r.Context()
	if !ds.IsDatastar(r) {
		v, err := h.view(ctx, ac, parseListQuery(nil, fieldWorkerList).withFacet(fieldClientParam, []string{clientID}))
		if err != nil {
			h.Shell.Fail(w, r, err)
			return
		}
		v.Dialog = fieldBadgeDialog(reveal)
		h.renderPage(w, r, http.StatusOK, v)
		return
	}
	sse := ds.NewSSE(w, r)
	if err := patchDialogSlot(sse, fwDialogSlot, fieldBadgeDialog(reveal)); err != nil {
		slog.DebugContext(ctx, "ui: field badge not delivered", "err", err)
	}
	h.afterChange(sse, r, ac, clientID, id, false)
	// The flash names the worker, never the token.
	if err := portalFlash(sse, flash, detail); err != nil {
		slog.DebugContext(ctx, "ui: field badge flash not delivered", "err", err)
	}
}

// ── revoke ────────────────────────────────────────────────────────────────

// revokeBadge mirrors POST /api/field-workers/{id}/badges/{badgeId}/revoke for
// the worker's ACTIVE badge.
func (h *FieldWorkers) revokeBadge(w http.ResponseWriter, r *http.Request) {
	h.runAction(w, r, flashFieldBadgeRevoked, func(ctx context.Context, i *fieldidentity.Identity) (string, error) {
		b, err := h.Store.ActiveBadge(ctx, i.ID)
		if err != nil {
			return "", usecase.Internal("REPO", "find_field_badge failed", err)
		}
		if b == nil {
			return "", httperror.NotFound("FieldBadge", i.ID)
		}
		err = h.Ops.RevokeBadge(ctx, fieldidentity.BadgeRef{WorkerRef: refOfWorker(i), BadgeID: b.ID})
		return i.WorkerCode + "'s badge no longer signs in", err
	})
}

// ── print a sheet ─────────────────────────────────────────────────────────

// printBadges mirrors POST /api/field-workers/badges: it issues a badge for
// each ticked worker in one operation (all or nothing) and answers with the
// sheet to print. Every worker is loaded with the manage check first, so a
// worker the caller cannot reach (or that is gone) fails the whole request
// and nothing is issued.
func (h *FieldWorkers) printBadges(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	ids := formSet(r.PostForm["ids"])
	if len(ids) == 0 || len(ids) > fieldidentity.MaxBulkBadges {
		h.Shell.Fail(w, r, usecase.Validation("WORKER_IDS_INVALID", "Tick 1 to 200 workers to print badges for."))
		return
	}
	idents := make([]*fieldidentity.Identity, 0, len(ids))
	for _, id := range ids {
		if !kit.ValidID(id) {
			h.Shell.Fail(w, r, httperror.NotFound("FieldWorker", id))
			return
		}
		i, err := h.loadWorker(ctx, ac, id, true)
		if err != nil {
			h.Shell.Fail(w, r, err)
			return
		}
		idents = append(idents, i)
	}
	tokens := make([]string, len(ids))
	for n := range tokens {
		t, err := fieldidentity.NewBadgeToken()
		if err != nil {
			h.Shell.Fail(w, r, usecase.Internal("TOKEN_GENERATE", "could not generate a badge token", err))
			return
		}
		tokens[n] = t
	}
	w = noStore{w}
	issued, err := h.Ops.IssueBadges(ctx, fieldidentity.IssueBadgesCommand{WorkerIDs: ids, Tokens: tokens})
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	clients := map[string]string{}
	reveal := fieldBadgeReveal{Replaced: true}
	for n, it := range issued {
		c, ok := clients[it.Worker.ClientID]
		if !ok {
			c = h.clientLabel(ctx, ac, it.Worker.ClientID)
			clients[it.Worker.ClientID] = c
		}
		reveal.Cards = append(reveal.Cards, fieldBadgeCard{Name: it.Worker.Name, Code: it.Worker.WorkerCode, Client: c, Token: tokens[n]})
	}
	h.revealBadges(w, r, ac, idents[0].ClientID, "", reveal, flashFieldBadgesIssued, "")
}

// ── sign in as… (console sign-in code) ─────────────────────────────────────

const (
	fwSignInForm = "fw-sign-in-form"
	fwSignInSig  = "fwSignInForm"
	fwCodeDialog = "fw-sign-in-code"
)

type fieldSignInAsForm struct {
	WorkerID, Code, Name string
	Choices              []kit.Option
	App                  string
	Errs                 formErrors
	Open                 bool
}

func (f fieldSignInAsForm) id(key string) string { return fieldID(fwSignInForm, key) }

func (f fieldSignInAsForm) dialog() kit.FormDialogProps {
	return kit.FormDialogProps{
		ID: fwSignInForm, Signal: fwSignInSig, Action: fieldWorkersPath + "/" + url.PathEscape(f.WorkerID) + "/sign-in-as", Open: f.Open, Error: f.Errs.Form,
		Title:       "Sign in as " + f.Code,
		Description: "Make a single-use code that signs " + f.Code + " in to an application on whatever device uses it. It is valid for ten minutes, and the sign-in is audited under your name.",
		SubmitLabel: "Create code", Hidden: []kit.HiddenField{drawerField(fwDrawerSig)},
	}
}

func (f fieldSignInAsForm) describedBy() string {
	if f.Errs.Fields["app"] != "" {
		return f.id("app") + "-err"
	}
	return ""
}

func (f fieldSignInAsForm) fields() []templ.Component {
	return []templ.Component{fieldSignInAsField(f)}
}

func (f fieldSignInAsForm) firstInvalid() string {
	if f.Errs.Fields["app"] != "" {
		return f.id("app")
	}
	return ""
}

var fieldSignInAsCodes = map[string]string{"APP_CODE_REQUIRED": "app", "UNKNOWN_APPLICATION": "app", "APPLICATION_FORBIDDEN": "app", "NO_APP_ACCESS": "app"}

// fieldCodeReveal-like data of the dialog after creating a sign-in code.
type fieldSignInCodeReveal struct {
	Code, WorkerCode, WorkerName, AppCode string
	ExpiresAt                             time.Time
}

// signInTarget loads the worker for a sign-in code with the API's gate.
func (h *FieldWorkers) signInTarget(ctx context.Context, ac *auth.AuthContext, id string) (*fieldidentity.Identity, error) {
	return h.loadWorkerWith(ctx, ac, id, signInAsGate)
}

func (h *FieldWorkers) signInAsForm(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	id, err := fieldWorkerID(r)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	i, err := h.signInTarget(ctx, ac, id)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	idx, err := loadFieldApps(ctx, h.Store)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	choices := h.signInApps(ac, i, idx)
	if i.Status != fieldidentity.StatusActive {
		err = usecase.Conflict(fieldidentity.CodeWorkerUnavailable, "The worker is suspended: activate them before signing in as them.")
	} else if len(choices) == 0 {
		err = usecase.Conflict(fieldidentity.CodeNoAppAccess, "The worker has no application you can sign in to.")
	}
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	f := fieldSignInAsForm{WorkerID: i.ID, Code: i.WorkerCode, Name: i.Name, Choices: choices, App: choices[0].Value, Open: true}
	if ds.IsDatastar(r) {
		if err := patchDialogSlot(ds.NewSSE(w, r), fwDialogSlot, fieldSignInAsDialog(f)); err != nil {
			slog.DebugContext(ctx, "ui: field sign-in form patch not delivered", "err", err)
		}
		return
	}
	v, err := h.view(ctx, ac, parseListQuery(nil, fieldWorkerList).withFacet(fieldClientParam, []string{i.ClientID}))
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	v.Dialog = fieldSignInAsDialog(f)
	h.renderPage(w, r, http.StatusOK, v)
}

// signInAs mirrors POST /api/field-workers/{id}/sign-in-codes: the
// impersonate permission, or manage for a test identity; a human principal
// unless the target is a test identity. The code is generated here, as the
// API does, and only its hash is stored.
func (h *FieldWorkers) signInAs(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	id, err := fieldWorkerID(r)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	i, err := h.signInTarget(ctx, ac, id)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	f := fieldSignInAsForm{WorkerID: i.ID, Code: i.WorkerCode, Name: i.Name, App: strings.TrimSpace(r.PostFormValue("app"))}
	if idx, ierr := loadFieldApps(ctx, h.Store); ierr == nil {
		f.Choices = h.signInApps(ac, i, idx)
	}
	fail := func(err error) {
		f.Errs.apply(r, err, fieldSignInAsCodes)
		if ds.IsDatastar(r) {
			patchFormFailure(ds.NewSSE(w, r), r, fwSignInForm, f.fields(), f.Errs.Form, f.firstInvalid())
			return
		}
		f.Open = true
		v, verr := h.view(ctx, ac, parseListQuery(nil, fieldWorkerList).withFacet(fieldClientParam, []string{i.ClientID}))
		if verr != nil {
			v = fieldWorkersView{}
		}
		v.Dialog = fieldSignInAsDialog(f)
		h.renderPage(w, r, cmpOr(f.Errs.Status, http.StatusUnprocessableEntity), v)
	}
	code, err := fieldidentity.NewSignInCodeValue()
	if err != nil {
		fail(usecase.Internal("CODE_GENERATE", "could not generate a sign-in code", err))
		return
	}
	w = noStore{w}
	res, err := h.Ops.CreateSignInCode(ctx, fieldidentity.CreateSignInCodeCommand{
		WorkerRef: refOfWorker(i), AppCode: f.App, Code: code,
	})
	if err != nil {
		fail(err)
		return
	}
	reveal := fieldSignInCodeReveal{Code: code, WorkerCode: i.WorkerCode, WorkerName: i.Name, AppCode: res.AppCode, ExpiresAt: res.ExpiresAt}
	if !ds.IsDatastar(r) {
		v, err := h.view(ctx, ac, parseListQuery(nil, fieldWorkerList).withFacet(fieldClientParam, []string{i.ClientID}))
		if err != nil {
			h.Shell.Fail(w, r, err)
			return
		}
		v.Dialog = fieldSignInCodeDialog(reveal)
		h.renderPage(w, r, http.StatusOK, v)
		return
	}
	sse := ds.NewSSE(w, r)
	if err := patchDialogSlot(sse, fwDialogSlot, fieldSignInCodeDialog(reveal)); err != nil {
		slog.DebugContext(ctx, "ui: field sign-in code not delivered", "err", err)
	}
	// The flash names the worker and the application, never the code.
	if err := portalFlash(sse, flashFieldSignInCode, "Sign-in code for "+i.WorkerCode+" in "+res.AppCode+"; it is shown once"); err != nil {
		slog.DebugContext(ctx, "ui: field sign-in flash not delivered", "err", err)
	}
}

// fieldSignInQR draws the code as an inline SVG: the QR content is the bare
// code, as for a badge.
func fieldSignInQR(c fieldSignInCodeReveal) templ.Component {
	q, err := qrSVGLabeled(c.Code, "Sign-in code QR code for "+c.WorkerCode)
	if err != nil {
		return templ.Raw(`<p class="text-sm text-muted-foreground">The QR code could not be drawn; type the code in the app.</p>`)
	}
	return q
}
