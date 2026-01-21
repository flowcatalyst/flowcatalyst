package warning

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
)

// Handler provides HTTP endpoints for the warning service
type Handler struct {
	service Service
}

// NewHandler creates a new warning HTTP handler
func NewHandler(service Service) *Handler {
	return &Handler{service: service}
}

// RegisterRoutes registers warning routes on the given router
func (h *Handler) RegisterRoutes(r chi.Router) {
	r.Route("/warnings", func(r chi.Router) {
		r.Get("/", h.List)
		r.Get("/unacknowledged", h.ListUnacknowledged)
		r.Get("/severity/{severity}", h.ListBySeverity)
		r.Post("/{id}/acknowledge", h.Acknowledge)
		r.Delete("/", h.ClearAll)
		r.Delete("/old", h.ClearOld)
	})
}

// List returns all warnings
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	warnings := h.service.GetAllWarnings()
	writeJSON(w, http.StatusOK, warnings)
}

// ListUnacknowledged returns unacknowledged warnings
func (h *Handler) ListUnacknowledged(w http.ResponseWriter, r *http.Request) {
	warnings := h.service.GetUnacknowledgedWarnings()
	writeJSON(w, http.StatusOK, warnings)
}

// ListBySeverity returns warnings filtered by severity
func (h *Handler) ListBySeverity(w http.ResponseWriter, r *http.Request) {
	severity := chi.URLParam(r, "severity")
	warnings := h.service.GetWarningsBySeverity(severity)
	writeJSON(w, http.StatusOK, warnings)
}

// Acknowledge acknowledges a warning
func (h *Handler) Acknowledge(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if h.service.AcknowledgeWarning(id) {
		w.WriteHeader(http.StatusNoContent)
	} else {
		http.Error(w, "Warning not found", http.StatusNotFound)
	}
}

// ClearAll clears all warnings
func (h *Handler) ClearAll(w http.ResponseWriter, r *http.Request) {
	h.service.ClearAllWarnings()
	w.WriteHeader(http.StatusNoContent)
}

// ClearOld clears warnings older than the specified hours
func (h *Handler) ClearOld(w http.ResponseWriter, r *http.Request) {
	hoursStr := r.URL.Query().Get("hours")
	hours := 24 // default 24 hours
	if hoursStr != "" {
		if h, err := strconv.Atoi(hoursStr); err == nil && h > 0 {
			hours = h
		}
	}
	h.service.ClearOldWarnings(hours)
	w.WriteHeader(http.StatusNoContent)
}

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}
