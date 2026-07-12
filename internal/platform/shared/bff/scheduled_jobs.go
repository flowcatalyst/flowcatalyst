package bff

import (
	"context"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/client"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/scheduledjob"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
)

// ScheduledJobsState holds the deps the /bff/scheduled-jobs/* endpoints
// reach into. Read-only — mutations go through /api/scheduled-jobs.
type ScheduledJobsState struct {
	Jobs         *scheduledjob.Repository
	Instances    *scheduledjob.InstanceRepository
	Clients      *client.Repository
	Applications *application.Repository
}

// RegisterScheduledJobs mounts the dashboard's `/bff/scheduled-jobs/*`
// endpoints. Mirrors crates/fc-platform/src/shared/bff_scheduled_jobs_api.rs.
//
// Routes:
//
//	GET /bff/scheduled-jobs                              — paginated job list
//	GET /bff/scheduled-jobs/filter-options               — clients + statuses
//	GET /bff/scheduled-jobs/instances/{instanceId}       — single instance
//	GET /bff/scheduled-jobs/instances/{instanceId}/logs  — instance logs
//	GET /bff/scheduled-jobs/{id}                         — single job
//	GET /bff/scheduled-jobs/{id}/instances               — instances of a job
//
// The static `/instances` and `/filter-options` segments are mounted
// BEFORE the wildcard `/{id}` so chi resolves them first.
func RegisterScheduledJobs(r chi.Router, s *ScheduledJobsState) {
	r.Route("/bff/scheduled-jobs", func(r chi.Router) {
		r.Get("/", s.listJobs)
		r.Get("/filter-options", s.filterOptions)
		r.Get("/instances/{instanceId}", s.getInstance)
		r.Get("/instances/{instanceId}/logs", s.listInstanceLogs)
		r.Get("/{id}", s.getJob)
		r.Get("/{id}/instances", s.listInstances)
	})
}

// ── Wire DTOs ────────────────────────────────────────────────────────────

// bffScheduledJobResponse matches Rust's BffScheduledJobResponse.
type bffScheduledJobResponse struct {
	ID                  string     `json:"id"`
	ClientID            *string    `json:"clientId,omitempty"`
	ClientName          *string    `json:"clientName,omitempty"`
	ApplicationID       *string    `json:"applicationId,omitempty"`
	ApplicationName     *string    `json:"applicationName,omitempty"`
	Code                string     `json:"code"`
	Name                string     `json:"name"`
	Description         *string    `json:"description,omitempty"`
	Status              string     `json:"status"`
	Crons               []string   `json:"crons"`
	Timezone            string     `json:"timezone"`
	Payload             any        `json:"payload,omitempty"`
	Concurrent          bool       `json:"concurrent"`
	TracksCompletion    bool       `json:"tracksCompletion"`
	TimeoutSeconds      *int32     `json:"timeoutSeconds,omitempty"`
	DeliveryMaxAttempts int32      `json:"deliveryMaxAttempts"`
	TargetURL           *string    `json:"targetUrl,omitempty"`
	LastFiredAt         *time.Time `json:"lastFiredAt,omitempty"`
	CreatedAt           time.Time  `json:"createdAt"`
	UpdatedAt           time.Time  `json:"updatedAt"`
	Version             int32      `json:"version"`
	HasActiveInstance   bool       `json:"hasActiveInstance"`
}

// bffScheduledJobInstanceResponse matches Rust's BffScheduledJobInstanceResponse.
type bffScheduledJobInstanceResponse struct {
	ID               string     `json:"id"`
	ScheduledJobID   string     `json:"scheduledJobId"`
	JobCode          string     `json:"jobCode"`
	ClientID         *string    `json:"clientId,omitempty"`
	TriggerKind      string     `json:"triggerKind"`
	ScheduledFor     *time.Time `json:"scheduledFor,omitempty"`
	FiredAt          time.Time  `json:"firedAt"`
	DeliveredAt      *time.Time `json:"deliveredAt,omitempty"`
	CompletedAt      *time.Time `json:"completedAt,omitempty"`
	Status           string     `json:"status"`
	DeliveryAttempts int32      `json:"deliveryAttempts"`
	DeliveryError    *string    `json:"deliveryError,omitempty"`
	CompletionStatus *string    `json:"completionStatus,omitempty"`
	CompletionResult any        `json:"completionResult,omitempty"`
	CorrelationID    *string    `json:"correlationId,omitempty"`
	CreatedAt        time.Time  `json:"createdAt"`
}

type bffInstanceLogResponse struct {
	ID         string    `json:"id"`
	InstanceID string    `json:"instanceId"`
	Level      string    `json:"level"`
	Message    string    `json:"message"`
	Metadata   any       `json:"metadata,omitempty"`
	CreatedAt  time.Time `json:"createdAt"`
}

type bffPaginatedResponse struct {
	Data       any    `json:"data"`
	Page       uint32 `json:"page"`
	Size       uint32 `json:"size"`
	Total      int64  `json:"total"`
	TotalPages uint32 `json:"totalPages"`
}

type bffScheduledJobsFilterOptions struct {
	Clients      []bffFilterOption `json:"clients"`
	Applications []bffFilterOption `json:"applications"`
	Statuses     []bffFilterOption `json:"statuses"`
}

type bffFilterOption struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

// ── Handlers ─────────────────────────────────────────────────────────────

// GET /bff/scheduled-jobs?clientIds=&applicationIds=&statuses=&search=&page=&size=
//
// clientIds/applicationIds/statuses are CSV multi-selects (mirrors the
// dispatch-jobs list page's filters). A "platform" entry in clientIds
// additionally matches platform-scoped jobs (client_id IS NULL).
func (s *ScheduledJobsState) listJobs(w http.ResponseWriter, r *http.Request) {
	ac := auth.FromContext(r.Context())
	if err := auth.CanReadScheduledJobs(ac); err != nil {
		httperror.Write(w, err)
		return
	}
	q := r.URL.Query()
	page, size := parsePagination(q)

	filters := scheduledjob.ListFilters{}
	if clientIDs := splitCSV(q.Get("clientIds")); len(clientIDs) > 0 {
		filters.ClientIDs = clientIDs
	}
	if applicationIDs := splitCSV(q.Get("applicationIds")); len(applicationIDs) > 0 {
		filters.ApplicationIDs = applicationIDs
	}
	if statuses := splitCSV(q.Get("statuses")); len(statuses) > 0 {
		filters.Statuses = statuses
	}
	if search := strings.TrimSpace(q.Get("search")); search != "" {
		filters.Search = &search
	}
	limit := int64(size)
	offset := int64(page) * int64(size)
	filters.Limit = &limit
	filters.Offset = &offset

	total, err := s.Jobs.CountWithFilters(r.Context(), filters)
	if err != nil {
		httperror.Write(w, usecase.Internal("REPO", "count scheduled jobs failed", err))
		return
	}
	rows, err := s.Jobs.FindWithFilters(r.Context(), filters)
	if err != nil {
		httperror.Write(w, usecase.Internal("REPO", "list scheduled jobs failed", err))
		return
	}

	// Client-access filter + denormalised client/application name lookup.
	clients, err := s.allClientsByID(r.Context())
	if err != nil {
		httperror.Write(w, usecase.Internal("REPO", "client lookup failed", err))
		return
	}
	applications, err := s.allApplicationsByID(r.Context())
	if err != nil {
		httperror.Write(w, usecase.Internal("REPO", "application lookup failed", err))
		return
	}
	out := make([]bffScheduledJobResponse, 0, len(rows))
	for i := range rows {
		j := &rows[i]
		if !canViewJob(ac, j) {
			continue
		}
		active, err := s.Instances.HasActiveInstance(r.Context(), j.ID, j.TracksCompletion)
		if err != nil {
			httperror.Write(w, usecase.Internal("REPO", "has_active_instance failed", err))
			return
		}
		out = append(out, toBffJob(j, clients, applications, active))
	}

	writeJSON(w, http.StatusOK, bffPaginatedResponse{
		Data:       out,
		Page:       page,
		Size:       size,
		Total:      total,
		TotalPages: totalPages(total, size),
	})
}

// GET /bff/scheduled-jobs/{id}
func (s *ScheduledJobsState) getJob(w http.ResponseWriter, r *http.Request) {
	ac := auth.FromContext(r.Context())
	if err := auth.CanReadScheduledJobs(ac); err != nil {
		httperror.Write(w, err)
		return
	}
	id := chi.URLParam(r, "id")
	j, err := s.Jobs.FindByID(r.Context(), id)
	if err != nil {
		httperror.Write(w, usecase.Internal("REPO", "find scheduled job failed", err))
		return
	}
	if j == nil || !canViewJob(ac, j) {
		httperror.Write(w, httperror.NotFound("ScheduledJob", id))
		return
	}
	clients, err := s.allClientsByID(r.Context())
	if err != nil {
		httperror.Write(w, usecase.Internal("REPO", "client lookup failed", err))
		return
	}
	applications, err := s.allApplicationsByID(r.Context())
	if err != nil {
		httperror.Write(w, usecase.Internal("REPO", "application lookup failed", err))
		return
	}
	active, err := s.Instances.HasActiveInstance(r.Context(), j.ID, j.TracksCompletion)
	if err != nil {
		httperror.Write(w, usecase.Internal("REPO", "has_active_instance failed", err))
		return
	}
	writeJSON(w, http.StatusOK, toBffJob(j, clients, applications, active))
}

// GET /bff/scheduled-jobs/{id}/instances?status=&triggerKind=&from=&to=&page=&size=
func (s *ScheduledJobsState) listInstances(w http.ResponseWriter, r *http.Request) {
	ac := auth.FromContext(r.Context())
	if err := auth.CanReadScheduledJobs(ac); err != nil {
		httperror.Write(w, err)
		return
	}
	id := chi.URLParam(r, "id")
	j, err := s.Jobs.FindByID(r.Context(), id)
	if err != nil {
		httperror.Write(w, usecase.Internal("REPO", "find scheduled job failed", err))
		return
	}
	if j == nil || !canViewJob(ac, j) {
		httperror.Write(w, httperror.NotFound("ScheduledJob", id))
		return
	}

	q := r.URL.Query()
	page, size := parsePagination(q)
	limit := int64(size)
	offset := int64(page) * int64(size)
	filters := scheduledjob.InstanceListFilters{
		ScheduledJobID: &id,
		Limit:          &limit,
		Offset:         &offset,
	}
	if status := q.Get("status"); status != "" {
		st := scheduledjob.ParseInstanceStatus(status)
		filters.Status = &st
	}
	if trigger := q.Get("triggerKind"); trigger != "" {
		tk := scheduledjob.ParseTriggerKind(trigger)
		filters.TriggerKind = &tk
	}
	if from, ok := parseTimeParam(q.Get("from")); ok {
		filters.From = &from
	}
	if to, ok := parseTimeParam(q.Get("to")); ok {
		filters.To = &to
	}

	total, err := s.Instances.Count(r.Context(), filters)
	if err != nil {
		httperror.Write(w, usecase.Internal("REPO", "count instances failed", err))
		return
	}
	rows, err := s.Instances.List(r.Context(), filters)
	if err != nil {
		httperror.Write(w, usecase.Internal("REPO", "list instances failed", err))
		return
	}
	out := make([]bffScheduledJobInstanceResponse, 0, len(rows))
	for i := range rows {
		out = append(out, toBffInstance(&rows[i]))
	}
	writeJSON(w, http.StatusOK, bffPaginatedResponse{
		Data:       out,
		Page:       page,
		Size:       size,
		Total:      total,
		TotalPages: totalPages(total, size),
	})
}

// GET /bff/scheduled-jobs/instances/{instanceId}
func (s *ScheduledJobsState) getInstance(w http.ResponseWriter, r *http.Request) {
	ac := auth.FromContext(r.Context())
	if err := auth.CanReadScheduledJobs(ac); err != nil {
		httperror.Write(w, err)
		return
	}
	instanceID := chi.URLParam(r, "instanceId")
	inst, err := s.Instances.FindByID(r.Context(), instanceID)
	if err != nil {
		httperror.Write(w, usecase.Internal("REPO", "find instance failed", err))
		return
	}
	if inst == nil || !canViewInstance(ac, inst) {
		httperror.Write(w, httperror.NotFound("ScheduledJobInstance", instanceID))
		return
	}
	writeJSON(w, http.StatusOK, toBffInstance(inst))
}

// GET /bff/scheduled-jobs/instances/{instanceId}/logs?limit=
func (s *ScheduledJobsState) listInstanceLogs(w http.ResponseWriter, r *http.Request) {
	ac := auth.FromContext(r.Context())
	if err := auth.CanReadScheduledJobs(ac); err != nil {
		httperror.Write(w, err)
		return
	}
	instanceID := chi.URLParam(r, "instanceId")
	inst, err := s.Instances.FindByID(r.Context(), instanceID)
	if err != nil {
		httperror.Write(w, usecase.Internal("REPO", "find instance failed", err))
		return
	}
	if inst == nil || !canViewInstance(ac, inst) {
		httperror.Write(w, httperror.NotFound("ScheduledJobInstance", instanceID))
		return
	}
	limit := int64(0)
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.ParseInt(l, 10, 64); err == nil && n > 0 {
			limit = n
		}
	}
	logs, err := s.Instances.ListLogs(r.Context(), instanceID, limit)
	if err != nil {
		httperror.Write(w, usecase.Internal("REPO", "list instance logs failed", err))
		return
	}
	out := make([]bffInstanceLogResponse, 0, len(logs))
	for i := range logs {
		out = append(out, toBffInstanceLog(&logs[i]))
	}
	// Bare array: the SPA's listInstanceLogs is typed
	// `Promise<ScheduledJobInstanceLog[]>` and consumes the result directly
	// (`logs.length`, `<DataTable :value="logs">`). The other
	// /bff/scheduled-jobs list handlers keep their `{data,…}` wrapper.
	writeJSON(w, http.StatusOK, out)
}

// GET /bff/scheduled-jobs/filter-options
func (s *ScheduledJobsState) filterOptions(w http.ResponseWriter, r *http.Request) {
	ac := auth.FromContext(r.Context())
	if err := auth.CanReadScheduledJobs(ac); err != nil {
		httperror.Write(w, err)
		return
	}
	rows, err := s.Clients.FindAll(r.Context())
	if err != nil {
		httperror.Write(w, usecase.Internal("REPO", "list clients failed", err))
		return
	}
	options := []bffFilterOption{}
	if ac.IsAnchor() {
		options = append(options, bffFilterOption{Value: "platform", Label: "Platform-scoped"})
	}
	visible := []bffFilterOption{}
	for _, c := range rows {
		if c.Status != client.StatusActive {
			continue
		}
		if !ac.IsAnchor() && !ac.CanAccessClient(c.ID) {
			continue
		}
		visible = append(visible, bffFilterOption{Value: c.ID, Label: c.Name})
	}
	sort.Slice(visible, func(i, j int) bool { return visible[i].Label < visible[j].Label })
	options = append(options, visible...)

	apps, err := s.Applications.FindWithFilters(r.Context(), nil, nil)
	if err != nil {
		httperror.Write(w, usecase.Internal("REPO", "list applications failed", err))
		return
	}
	appOptions := []bffFilterOption{}
	for _, a := range apps {
		if !a.Active {
			continue
		}
		appOptions = append(appOptions, bffFilterOption{Value: a.ID, Label: a.Name})
	}
	sort.Slice(appOptions, func(i, j int) bool { return appOptions[i].Label < appOptions[j].Label })

	writeJSON(w, http.StatusOK, bffScheduledJobsFilterOptions{
		Clients:      options,
		Applications: appOptions,
		Statuses: []bffFilterOption{
			{Value: "ACTIVE", Label: "Active"},
			{Value: "PAUSED", Label: "Paused"},
			{Value: "ARCHIVED", Label: "Archived"},
		},
	})
}

// ── Helpers ──────────────────────────────────────────────────────────────

// splitCSV mirrors dispatchjob/api.splitCSV: trim, drop empties.
func splitCSV(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if v := strings.TrimSpace(p); v != "" {
			out = append(out, v)
		}
	}
	return out
}

func canViewJob(ac *auth.AuthContext, j *scheduledjob.ScheduledJob) bool {
	if j.ClientID == nil {
		return ac.IsAnchor()
	}
	return ac.CanAccessClient(*j.ClientID)
}

func canViewInstance(ac *auth.AuthContext, inst *scheduledjob.ScheduledJobInstance) bool {
	if inst.ClientID == nil {
		return ac.IsAnchor()
	}
	return ac.CanAccessClient(*inst.ClientID)
}

func (s *ScheduledJobsState) allClientsByID(ctx context.Context) (map[string]string, error) {
	rows, err := s.Clients.FindAll(ctx)
	if err != nil {
		return nil, err
	}
	out := make(map[string]string, len(rows))
	for _, c := range rows {
		out[c.ID] = c.Name
	}
	return out, nil
}

func (s *ScheduledJobsState) allApplicationsByID(ctx context.Context) (map[string]string, error) {
	rows, err := s.Applications.FindWithFilters(ctx, nil, nil)
	if err != nil {
		return nil, err
	}
	out := make(map[string]string, len(rows))
	for _, a := range rows {
		out[a.ID] = a.Name
	}
	return out, nil
}

func toBffJob(j *scheduledjob.ScheduledJob, clientsByID, applicationsByID map[string]string, hasActive bool) bffScheduledJobResponse {
	var clientName *string
	if j.ClientID != nil {
		if n, ok := clientsByID[*j.ClientID]; ok {
			clientName = &n
		}
	}
	var applicationName *string
	if j.ApplicationID != nil {
		if n, ok := applicationsByID[*j.ApplicationID]; ok {
			applicationName = &n
		}
	}
	crons := j.Crons
	if crons == nil {
		crons = []string{}
	}
	out := bffScheduledJobResponse{
		ID:                  j.ID,
		ClientID:            j.ClientID,
		ClientName:          clientName,
		ApplicationID:       j.ApplicationID,
		ApplicationName:     applicationName,
		Code:                j.Code,
		Name:                j.Name,
		Description:         j.Description,
		Status:              string(j.Status),
		Crons:               crons,
		Timezone:            j.Timezone,
		Concurrent:          j.Concurrent,
		TracksCompletion:    j.TracksCompletion,
		TimeoutSeconds:      j.TimeoutSeconds,
		DeliveryMaxAttempts: j.DeliveryMaxAttempts,
		TargetURL:           j.TargetURL,
		LastFiredAt:         j.LastFiredAt,
		CreatedAt:           j.CreatedAt,
		UpdatedAt:           j.UpdatedAt,
		Version:             j.Version,
		HasActiveInstance:   hasActive,
	}
	if len(j.Payload) > 0 {
		out.Payload = j.Payload
	}
	return out
}

func toBffInstance(inst *scheduledjob.ScheduledJobInstance) bffScheduledJobInstanceResponse {
	out := bffScheduledJobInstanceResponse{
		ID:               inst.ID,
		ScheduledJobID:   inst.ScheduledJobID,
		JobCode:          inst.JobCode,
		ClientID:         inst.ClientID,
		TriggerKind:      string(inst.TriggerKind),
		ScheduledFor:     inst.ScheduledFor,
		FiredAt:          inst.FiredAt,
		DeliveredAt:      inst.DeliveredAt,
		CompletedAt:      inst.CompletedAt,
		Status:           string(inst.Status),
		DeliveryAttempts: inst.DeliveryAttempts,
		DeliveryError:    inst.DeliveryError,
		CompletionStatus: inst.CompletionStatus,
		CorrelationID:    inst.CorrelationID,
		CreatedAt:        inst.CreatedAt,
	}
	if len(inst.CompletionResult) > 0 {
		out.CompletionResult = inst.CompletionResult
	}
	return out
}

func toBffInstanceLog(log *scheduledjob.ScheduledJobInstanceLog) bffInstanceLogResponse {
	out := bffInstanceLogResponse{
		ID:         log.ID,
		InstanceID: log.InstanceID,
		Level:      log.Level,
		Message:    log.Message,
		CreatedAt:  log.CreatedAt,
	}
	if len(log.Metadata) > 0 {
		out.Metadata = log.Metadata
	}
	return out
}

func parsePagination(q map[string][]string) (page, size uint32) {
	get := func(k string) string {
		if v, ok := q[k]; ok && len(v) > 0 {
			return v[0]
		}
		return ""
	}
	page = 0
	size = 20
	if p, err := strconv.ParseUint(get("page"), 10, 32); err == nil {
		page = uint32(p)
	}
	if s := get("size"); s != "" {
		if n, err := strconv.ParseUint(s, 10, 32); err == nil && n > 0 {
			size = uint32(n)
		}
	} else if s := get("pageSize"); s != "" {
		if n, err := strconv.ParseUint(s, 10, 32); err == nil && n > 0 {
			size = uint32(n)
		}
	}
	if size > 200 {
		size = 200
	}
	return
}

func parseTimeParam(raw string) (time.Time, bool) {
	if raw == "" {
		return time.Time{}, false
	}
	if t, err := time.Parse(time.RFC3339Nano, raw); err == nil {
		return t.UTC(), true
	}
	if t, err := time.Parse(time.RFC3339, raw); err == nil {
		return t.UTC(), true
	}
	return time.Time{}, false
}

func totalPages(total int64, size uint32) uint32 {
	if size == 0 || total <= 0 {
		return 0
	}
	return uint32(math.Ceil(float64(total) / float64(size)))
}
