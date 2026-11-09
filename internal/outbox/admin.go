package outbox

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
)

// AdminHandler returns an HTTP handler exposing the operational state machine so
// an operator can inspect message-group states and pause / resume / unblock /
// skip a group. StartOutboxProcessor serves it on FC_OUTBOX_ADMIN_PORT when set,
// making the GroupDistributor's programmatic controls operable over HTTP:
//
//	GET  /outbox/groups               — non-default (Paused/Blocked) group states
//	GET  /outbox/groups/blocked       — Blocked groups only
//	GET  /outbox/holds                — groups/types/rows waiting out a 429 or a retryable failure
//	POST /outbox/groups/{group}/pause
//	POST /outbox/groups/{group}/resume
//	POST /outbox/groups/{group}/unblock  — clear + re-queue the poison (retry)
//	POST /outbox/groups/{group}/skip     — clear + leave the poison failed (marked skipped)
func (p *Processor) AdminHandler() http.Handler {
	r := chi.NewRouter()
	r.Get("/outbox/groups", func(w http.ResponseWriter, _ *http.Request) {
		writeAdminJSON(w, http.StatusOK, map[string]any{"groups": p.GroupStates()})
	})
	r.Get("/outbox/groups/blocked", func(w http.ResponseWriter, _ *http.Request) {
		writeAdminJSON(w, http.StatusOK, map[string]any{"blocked": p.BlockedGroups()})
	})
	r.Get("/outbox/holds", func(w http.ResponseWriter, _ *http.Request) {
		writeAdminJSON(w, http.StatusOK, map[string]any{"holds": p.Holds()})
	})
	r.Post("/outbox/groups/{group}/pause", func(w http.ResponseWriter, req *http.Request) {
		p.PauseGroup(chi.URLParam(req, "group"))
		writeAdminJSON(w, http.StatusOK, map[string]string{"status": "PAUSED"})
	})
	r.Post("/outbox/groups/{group}/resume", func(w http.ResponseWriter, req *http.Request) {
		p.ResumeGroup(chi.URLParam(req, "group"))
		writeAdminJSON(w, http.StatusOK, map[string]string{"status": "RUNNING"})
	})
	r.Post("/outbox/groups/{group}/unblock", func(w http.ResponseWriter, req *http.Request) {
		ok, err := p.UnblockGroup(req.Context(), chi.URLParam(req, "group"))
		writeAdminOutcome(w, ok, err, "UNBLOCKED")
	})
	r.Post("/outbox/groups/{group}/skip", func(w http.ResponseWriter, req *http.Request) {
		ok, err := p.SkipGroup(req.Context(), chi.URLParam(req, "group"))
		writeAdminOutcome(w, ok, err, "SKIPPED")
	})
	return r
}

// writeAdminOutcome answers an unblock/skip: 404 when the group is not blocked, 500 when the
// operator's decision could not be stored (the group stays blocked), else the new status.
func writeAdminOutcome(w http.ResponseWriter, ok bool, err error, status string) {
	switch {
	case !ok:
		writeAdminJSON(w, http.StatusNotFound, map[string]string{"error": "group not blocked"})
	case err != nil:
		writeAdminJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
	default:
		writeAdminJSON(w, http.StatusOK, map[string]string{"status": status})
	}
}

func writeAdminJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
