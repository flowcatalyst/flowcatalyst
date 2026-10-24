package pages

import (
	"context"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/ds"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/shell"
)

// The drawer's actions: pause, resume, remove and make a version live. Each
// asks first (a shared confirmation, rendered only for the actions the user
// may run), makes the permission check of the API endpoint it mirrors, and
// runs its use case; the use case checks reach and the owning set.

// done answers a successful action: for Datastar the table of the list the
// user is on, the drawer when it shows this ingestion (closed after a delete)
// and a flash, whose detail is the key's unless detail is given (text the
// server built); for a native post a redirect with the flash key.
func (h *Ingestions) done(w http.ResponseWriter, r *http.Request, ac *auth.AuthContext, id, flash, detail string, deleted bool) {
	if !ds.IsDatastar(r) {
		h.nativeDone(w, r, id, flash, deleted)
		return
	}
	ctx := r.Context()
	sse := ds.NewSSE(w, r)
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
		slog.DebugContext(ctx, "ui: ingestion flash not delivered", "err", err)
	}
}

// ingsAction is one of the drawer's actions.
type ingsAction struct {
	check   func(*auth.AuthContext) error
	run     func(IngestionCommands, context.Context, string) error
	flash   string
	deleted bool
}

var (
	ingsPauseAction  = ingsAction{check: auth.CanUpdateIngestions, run: IngestionCommands.Pause, flash: flashIngPaused}
	ingsResumeAction = ingsAction{check: auth.CanUpdateIngestions, run: IngestionCommands.Resume, flash: flashIngResumed}
	ingsDeleteAction = ingsAction{check: auth.CanDeleteIngestions, run: IngestionCommands.Delete, flash: flashIngDeleted, deleted: true}
)

func (h *Ingestions) pause(w http.ResponseWriter, r *http.Request) {
	h.runAction(w, r, ingsPauseAction)
}

func (h *Ingestions) resume(w http.ResponseWriter, r *http.Request) {
	h.runAction(w, r, ingsResumeAction)
}

func (h *Ingestions) delete(w http.ResponseWriter, r *http.Request) {
	h.runAction(w, r, ingsDeleteAction)
}

func (h *Ingestions) runAction(w http.ResponseWriter, r *http.Request, a ingsAction) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	if err := a.check(ac); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	id, err := ingsID(r)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	if err := a.run(h.Ops, ctx, id); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	h.done(w, r, ac, id, a.flash, "", a.deleted)
}

// makeLive moves the live pointer to a saved version, forwards or back
// (POST /api/ingestions/{id}/versions/{version}/live: the promote
// permission).
func (h *Ingestions) makeLive(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	if err := auth.CanPromoteIngestions(ac); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	id, err := ingsID(r)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	number, err := strconv.Atoi(chi.URLParam(r, "number"))
	if err != nil || number < 1 {
		h.Shell.Fail(w, r, httperror.NotFound("IngestionVersion", chi.URLParam(r, "number")))
		return
	}
	if err := h.Ops.MakeLive(ctx, id, number); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	h.done(w, r, ac, id, flashIngVersionLive, "Version "+strconv.Itoa(number)+" is live: runtimes run it on their next poll", false)
}
