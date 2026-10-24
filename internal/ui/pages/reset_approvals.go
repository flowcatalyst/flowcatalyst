package pages

import (
	"context"
	"log/slog"
	"maps"
	"net/http"
	"net/url"
	"time"

	"github.com/go-chi/chi/v5"

	resetapi "github.com/flowcatalyst/flowcatalyst-go/internal/platform/resetapproval/api"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/ds"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/kit"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/shell"
)

const (
	flashResetApproved = "reset-approved"
	flashResetDenied   = "reset-denied"
)

func init() {
	maps.Copy(FlashMessages, map[string]kit.FlashMessageProps{
		flashResetApproved: {Kind: kit.FlashSuccess, Summary: "Approved", Detail: "The user has been emailed a reset link."},
		flashResetDenied:   {Kind: kit.FlashSuccess, Summary: "Denied", Detail: "Request denied."},
	})
}

// ResetApprovalService is the reset approvals API the page runs: each method
// is the route's handler (resetapi.State), called in-process with the caller
// on ctx, so the permission check (user:update), the client scoping of the
// queue, the per-request client check and the single decision are the API's.
type ResetApprovalService interface {
	Pending(ctx context.Context) ([]resetapi.RequestDTO, error)
	Approve(ctx context.Context, id string) (string, error)
	Deny(ctx context.Context, id string) (string, error)
}

// ResetApprovals is the password reset approvals page (/ui/reset-approvals),
// mirroring the SPA's ResetApprovalsPage: the lost-device requests of users
// with no authenticator or passkey, each approved (the user is emailed a reset
// link and their 2FA is cleared) or denied.
type ResetApprovals struct {
	Service ResetApprovalService
	Shell   *shell.Shell
}

const (
	raPath     = "/ui/reset-approvals"
	raRegionID = "ra-region"
)

// Mount registers the page's routes (POSTs behind parseForm).
func (h *ResetApprovals) Mount(r chi.Router) {
	r.Get("/reset-approvals", h.list)
	post := r.With(parseForm)
	post.Post("/reset-approvals/{id}/approve", h.approve)
	post.Post("/reset-approvals/{id}/deny", h.deny)
}

type raRow struct {
	ID, Name, Email    string
	Requested, Expires time.Time
	ActionBase         string
}

type raView struct {
	Rows []raRow
}

func (h *ResetApprovals) view(ctx context.Context) (raView, error) {
	reqs, err := h.Service.Pending(ctx)
	if err != nil {
		return raView{}, err
	}
	var v raView
	for _, q := range reqs {
		name := q.Name
		if name == "" {
			name = q.Email
		}
		v.Rows = append(v.Rows, raRow{
			ID: q.ID, Name: name, Email: q.Email, Requested: q.CreatedAt.Underlying(), Expires: q.ExpiresAt.Underlying(),
			ActionBase: raPath + "/" + url.PathEscape(q.ID),
		})
	}
	return v, nil
}

func (h *ResetApprovals) list(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	view, err := h.view(ctx)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	if ds.IsDatastar(r) {
		// The refresh button: only the queue changes.
		if err := ds.NewSSE(w, r).Patch(raRegion(view)); err != nil {
			slog.DebugContext(ctx, "ui: reset approvals patch not delivered", "err", err)
		}
		return
	}
	h.Shell.Render(w, r, shell.Page{Title: "Password reset approvals", Path: raPath}, raPage(view))
}

func (h *ResetApprovals) approve(w http.ResponseWriter, r *http.Request) {
	h.decide(w, r, h.Service.Approve, flashResetApproved)
}

func (h *ResetApprovals) deny(w http.ResponseWriter, r *http.Request) {
	h.decide(w, r, h.Service.Deny, flashResetDenied)
}

// decide runs the API's decision for the request (which checks the caller's
// permission and its client) and answers with the queue as it now stands.
func (h *ResetApprovals) decide(w http.ResponseWriter, r *http.Request, run func(context.Context, string) (string, error), flash string) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")
	if !kit.ValidID(id) {
		h.Shell.Fail(w, r, httperror.NotFound("ResetApprovalRequest", id))
		return
	}
	if _, err := run(ctx, id); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	if !ds.IsDatastar(r) {
		h.Shell.SetFlash(w, flash)
		http.Redirect(w, r, raPath, http.StatusSeeOther)
		return
	}
	sse := ds.NewSSE(w, r)
	// The decision is made; the queue read can only fail the refresh. The
	// caller was authorised by the decision, so this read repeats a check it
	// has just passed.
	if view, err := h.view(ctx); err != nil {
		h.Shell.FailSSE(sse, r, err)
	} else if err := sse.Patch(raRegion(view)); err != nil {
		slog.DebugContext(ctx, "ui: reset approvals patch not delivered", "err", err)
	}
	if err := flashKey(sse, flash); err != nil {
		slog.DebugContext(ctx, "ui: reset approvals flash not delivered", "err", err)
	}
}
