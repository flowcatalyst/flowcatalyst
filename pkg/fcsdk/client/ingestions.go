package client

import (
	"context"
	"encoding/json"
	"net/url"
	"strings"
)

// IngestionTargetMode is how a failed record treats the ones behind it.
type IngestionTargetMode string

// The modes of an ingestion target.
const (
	TargetModeImmediate    IngestionTargetMode = "IMMEDIATE"
	TargetModeNextOnError  IngestionTargetMode = "NEXT_ON_ERROR"
	TargetModeBlockOnError IngestionTargetMode = "BLOCK_ON_ERROR"
)

// ─── Shared shapes ───────────────────────────────────────────────────

// IngestionTarget is the dispatch job each record becomes. Pool is a dispatch
// pool code; Connection and ServiceAccount are optional codes and may not both
// be set (a connection's account signs). Neither set is unsigned delivery.
type IngestionTarget struct {
	Code           string              `json:"code"`
	TargetURL      string              `json:"targetUrl"`
	Pool           string              `json:"pool"`
	Connection     string              `json:"connection,omitempty"`
	ServiceAccount string              `json:"serviceAccount,omitempty"`
	Mode           IngestionTargetMode `json:"mode"`
	BatchSize      int64               `json:"batchSize"`
}

// IngestionDefinition is the part of an ingestion a version holds, without its
// identity: what a test of an unsaved definition takes.
type IngestionDefinition struct {
	Name               string          `json:"name"`
	Source             IngestionSource `json:"source"`
	Split              IngestionSplit  `json:"split"`
	Target             IngestionTarget `json:"target"`
	KeepFailedPayloads bool            `json:"keepFailedPayloads,omitempty"`
}

// ─── Request DTOs ────────────────────────────────────────────────────

// CreateIngestionRequest — POST /api/ingestions. Application is the code of an
// INTEGRATION application, Client a client id (clt_...) or identifier, Code
// lowercase letters, digits and hyphens, unique within both.
type CreateIngestionRequest struct {
	Application        string          `json:"application"`
	Client             string          `json:"client"`
	Code               string          `json:"code"`
	Name               string          `json:"name"`
	Note               string          `json:"note,omitempty"`
	Source             IngestionSource `json:"source"`
	Split              IngestionSplit  `json:"split"`
	Target             IngestionTarget `json:"target"`
	KeepFailedPayloads bool            `json:"keepFailedPayloads,omitempty"`
	// Live makes the first version live; the platform's default is true, false
	// saves it to be made live later.
	Live *bool `json:"live,omitempty"`
}

// UpdateIngestionRequest — PUT /api/ingestions/{id}.
type UpdateIngestionRequest struct {
	// ExpectedVersion is the live version the caller read (0 when none is
	// live); a different current version is a 409.
	ExpectedVersion    int64           `json:"expectedVersion"`
	Name               string          `json:"name"`
	Note               string          `json:"note,omitempty"`
	Source             IngestionSource `json:"source"`
	Split              IngestionSplit  `json:"split"`
	Target             IngestionTarget `json:"target"`
	KeepFailedPayloads bool            `json:"keepFailedPayloads,omitempty"`
}

// SaveIngestionVersionRequest — POST /api/ingestions/{id}/versions. A saved
// version is not run until it is live.
type SaveIngestionVersionRequest struct {
	Name               string          `json:"name"`
	Note               string          `json:"note,omitempty"`
	Source             IngestionSource `json:"source"`
	Split              IngestionSplit  `json:"split"`
	Target             IngestionTarget `json:"target"`
	KeepFailedPayloads bool            `json:"keepFailedPayloads,omitempty"`
	// Live also makes the saved version live (needs the promote permission).
	Live *bool `json:"live,omitempty"`
}

// TestIngestionRequest — POST /api/ingestion-tests. Give IngestionID to test a
// saved ingestion's live definition, or Application, Client and Definition to
// test one without saving it.
type TestIngestionRequest struct {
	IngestionID string               `json:"ingestionId,omitempty"`
	Application string               `json:"application,omitempty"`
	Client      string               `json:"client,omitempty"`
	Definition  *IngestionDefinition `json:"definition,omitempty"`
}

// IngestionSetItem is one ingestion of a set: a definition with its identity.
// Active is the platform's default (true) when nil; an apply sets it, so a
// pause made since is undone.
type IngestionSetItem struct {
	Application        string          `json:"application"`
	Client             string          `json:"client"`
	Code               string          `json:"code"`
	Name               string          `json:"name"`
	Active             *bool           `json:"active,omitempty"`
	Source             IngestionSource `json:"source"`
	Split              IngestionSplit  `json:"split"`
	Target             IngestionTarget `json:"target"`
	KeepFailedPayloads bool            `json:"keepFailedPayloads,omitempty"`
}

// SetFile is a definition file as it is in the repository, kept with an apply.
type SetFile struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

// ApplyIngestionSetRequest — PUT /api/ingestion-sets/{name}. No two items may
// share an application, client and code; Files (at most 2 MB in all) are kept
// with the apply.
type ApplyIngestionSetRequest struct {
	Items []IngestionSetItem `json:"items"`
	Files []SetFile          `json:"files,omitempty"`
}

// ─── Response DTOs ───────────────────────────────────────────────────

// IngestionResponse is the platform's ingestion aggregate. Status is ACTIVE or
// PAUSED. ManagedBy is the id of the set that owns it (read-only until the set
// is released).
type IngestionResponse struct {
	ID                 string          `json:"id"`
	ApplicationID      string          `json:"applicationId"`
	ApplicationCode    string          `json:"applicationCode"`
	ClientID           string          `json:"clientId"`
	ClientIdentifier   string          `json:"clientIdentifier"`
	Code               string          `json:"code"`
	Status             string          `json:"status"`
	Version            int64           `json:"version"`
	LatestVersion      int64           `json:"latestVersion"`
	CreatedAt          string          `json:"createdAt"`
	UpdatedAt          string          `json:"updatedAt"`
	Name               string          `json:"name"`
	Source             IngestionSource `json:"source"`
	Split              IngestionSplit  `json:"split"`
	Target             IngestionTarget `json:"target"`
	KeepFailedPayloads bool            `json:"keepFailedPayloads,omitempty"`
	ManagedBy          string          `json:"managedBy,omitempty"`
	RuntimeNotice      string          `json:"runtimeNotice,omitempty"`
	// Notices counts undismissed notices (failures the platform refused,
	// category "rejected"); LatestNotice is the newest one's message.
	Notices        int64  `json:"notices,omitempty"`
	LatestNotice   string `json:"latestNotice,omitempty"`
	LatestNoticeAt string `json:"latestNoticeAt,omitempty"`
	// Warnings names what the definition says that will not run as meant.
	Warnings []string `json:"warnings,omitempty"`
	// Save is what a create or update did (set on those answers only).
	Save *IngestionSaveOutcome `json:"save,omitempty"`
}

// IngestionSaveOutcome is what a create, update or version save did. Outcome
// is SAVED, MADE_LIVE, DRAFT, DRAFT_EXISTS or UNCHANGED; AwaitingPromote is
// true for a draft that someone with promote must make live.
type IngestionSaveOutcome struct {
	Outcome         string `json:"outcome"`
	Version         int64  `json:"version"`
	AwaitingPromote bool   `json:"awaitingPromote"`
	Message         string `json:"message,omitempty"`
}

// UnmarshalJSON reads the source and split as their typed members.
func (r *IngestionResponse) UnmarshalJSON(data []byte) error {
	type plain IngestionResponse
	var aux struct {
		plain
		Source json.RawMessage `json:"source"`
		Split  json.RawMessage `json:"split"`
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	src, spl, err := decodeParts(aux.Source, aux.Split)
	if err != nil {
		return err
	}
	*r = IngestionResponse(aux.plain)
	r.Source, r.Split = src, spl
	return nil
}

// IngestionListResponse — GET /api/ingestions, cursor-paged: while HasMore is
// true, pass NextCursor as ListIngestionsFilters.After for the next page.
type IngestionListResponse struct {
	Ingestions []IngestionResponse `json:"ingestions"`
	HasMore    bool                `json:"hasMore"`
	NextCursor string              `json:"nextCursor,omitempty"`
}

// IngestionVersionResponse is one saved version of an ingestion.
type IngestionVersionResponse struct {
	ID                 string          `json:"id"`
	Version            int64           `json:"version"`
	Live               bool            `json:"live"`
	CreatedAt          string          `json:"createdAt"`
	CreatedBy          string          `json:"createdBy,omitempty"`
	AppliedFrom        string          `json:"appliedFrom,omitempty"`
	Name               string          `json:"name"`
	Note               string          `json:"note,omitempty"`
	Source             IngestionSource `json:"source"`
	Split              IngestionSplit  `json:"split"`
	Target             IngestionTarget `json:"target"`
	KeepFailedPayloads bool            `json:"keepFailedPayloads,omitempty"`
	// Draft is a version saved but not live; Digest identifies its
	// definition (equal definitions have equal digests).
	Draft  bool                  `json:"draft,omitempty"`
	Digest string                `json:"digest,omitempty"`
	Save   *IngestionSaveOutcome `json:"save,omitempty"`
}

// UnmarshalJSON reads the source and split as their typed members.
func (r *IngestionVersionResponse) UnmarshalJSON(data []byte) error {
	type plain IngestionVersionResponse
	var aux struct {
		plain
		Source json.RawMessage `json:"source"`
		Split  json.RawMessage `json:"split"`
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	src, spl, err := decodeParts(aux.Source, aux.Split)
	if err != nil {
		return err
	}
	*r = IngestionVersionResponse(aux.plain)
	r.Source, r.Split = src, spl
	return nil
}

// UnmarshalJSON reads the source and split as their typed members.
func (i *IngestionSetItem) UnmarshalJSON(data []byte) error {
	type plain IngestionSetItem
	var aux struct {
		plain
		Source json.RawMessage `json:"source"`
		Split  json.RawMessage `json:"split"`
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	src, spl, err := decodeParts(aux.Source, aux.Split)
	if err != nil {
		return err
	}
	*i = IngestionSetItem(aux.plain)
	i.Source, i.Split = src, spl
	return nil
}

// IngestionVersionListResponse — GET /api/ingestions/{id}/versions.
type IngestionVersionListResponse struct {
	Versions []IngestionVersionResponse `json:"versions"`
}

// IngestionHistoryEntry is one operation in an ingestion's audit history.
// Details is the command the operation ran with.
type IngestionHistoryEntry struct {
	ID            string          `json:"id"`
	Operation     string          `json:"operation"`
	PerformedAt   string          `json:"performedAt"`
	PrincipalID   string          `json:"principalId,omitempty"`
	PrincipalName string          `json:"principalName,omitempty"`
	Details       json.RawMessage `json:"details,omitempty"`
}

// IngestionHistoryResponse — GET /api/ingestions/{id}/history, offset-paged.
type IngestionHistoryResponse struct {
	Entries []IngestionHistoryEntry `json:"entries"`
	HasMore bool                    `json:"hasMore"`
}

// IngestionFailureResponse is one failure the runtime reported. Excerpt is the
// start of the payload, only for an ingestion that keeps failed payloads.
type IngestionFailureResponse struct {
	ID           string `json:"id"`
	Kind         string `json:"kind"`
	Message      string `json:"message"`
	AfterRecords int64  `json:"afterRecords"`
	OccurredAt   string `json:"occurredAt"`
	Excerpt      string `json:"excerpt,omitempty"`
	RuntimeID    string `json:"runtimeId,omitempty"`
	Version      int64  `json:"version,omitempty"`
	// Category is "payload" (the source's data was at fault) or "rejected"
	// (the platform refused the runtime's jobs permanently: a notice);
	// empty for other failures.
	Category    string `json:"category,omitempty"`
	CreatedAt   string `json:"createdAt,omitempty"`
	DismissedAt string `json:"dismissedAt,omitempty"`
	DismissedBy string `json:"dismissedBy,omitempty"`
}

// IngestionFailureListResponse — GET /api/ingestions/{id}/failures,
// cursor-paged like IngestionListResponse.
type IngestionFailureListResponse struct {
	Failures   []IngestionFailureResponse `json:"failures"`
	HasMore    bool                       `json:"hasMore"`
	NextCursor string                     `json:"nextCursor,omitempty"`
}

// IngestionSourceKindResponse is a source kind the platform or a live runtime
// carries. Settings is the kind's settings as the runtime describes them.
type IngestionSourceKindResponse struct {
	Kind        string `json:"kind"`
	BuiltIn     bool   `json:"builtIn"`
	SingleOwner bool   `json:"singleOwner"`
	Description string `json:"description,omitempty"`
	// RequiredPermission is what creating or changing an ingestion of this
	// kind needs.
	RequiredPermission string          `json:"requiredPermission,omitempty"`
	Settings           json.RawMessage `json:"settings,omitempty"`
}

// IngestionSourceKindListResponse — GET /api/ingestion-source-kinds.
type IngestionSourceKindListResponse struct {
	Kinds []IngestionSourceKindResponse `json:"kinds"`
}

// IngestionTestResponse is a connection test. Status is PENDING, RUNNING, DONE
// or EXPIRED; once DONE, Outcome is RESULTS, NO_RESULTS, REFUSED, FAILED or
// NOT_SUPPORTED. Notice is set while no live runtime carries the kind: the
// test waits, nothing has failed.
type IngestionTestResponse struct {
	ID          string   `json:"id"`
	Kind        string   `json:"kind"`
	Status      string   `json:"status"`
	CreatedAt   string   `json:"createdAt"`
	ExpiresAt   string   `json:"expiresAt"`
	IngestionID string   `json:"ingestionId,omitempty"`
	RuntimeID   string   `json:"runtimeId,omitempty"`
	CompletedAt string   `json:"completedAt,omitempty"`
	Outcome     string   `json:"outcome,omitempty"`
	Records     int64    `json:"records,omitempty"`
	Excerpt     string   `json:"excerpt,omitempty"`
	Notice      string   `json:"notice,omitempty"`
	Steps       []string `json:"steps,omitempty"`
}

// SetChange is what an apply does (or would do) to one ingestion. Action is
// CREATE, CHANGE, ADOPT, UNCHANGED or REMOVE; Changed names the fields.
type SetChange struct {
	Application string   `json:"application"`
	Client      string   `json:"client"`
	Code        string   `json:"code"`
	Action      string   `json:"action"`
	Changed     []string `json:"changed"`
	IngestionID string   `json:"ingestionId,omitempty"`
}

// IngestionSetResponse is a set and how many ingestions it owns.
type IngestionSetResponse struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Ingestions int64  `json:"ingestions"`
	CreatedAt  string `json:"createdAt"`
	UpdatedAt  string `json:"updatedAt"`
}

// IngestionSetListResponse — GET /api/ingestion-sets.
type IngestionSetListResponse struct {
	Sets []IngestionSetResponse `json:"sets"`
}

// IngestionSetPlanResponse — the answer to an apply or a reapply. ApplyID is
// absent for a dry run and for an apply that changed nothing.
type IngestionSetPlanResponse struct {
	Name    string      `json:"name"`
	DryRun  bool        `json:"dryRun"`
	Changes []SetChange `json:"changes"`
	ApplyID string      `json:"applyId,omitempty"`
}

// IngestionSetApplyResponse is one recorded apply. Items and Files count what
// it was given; GoingBackTo is the earlier apply it applied again.
type IngestionSetApplyResponse struct {
	ID          string      `json:"id"`
	Set         string      `json:"set"`
	AppliedAt   string      `json:"appliedAt"`
	Items       int64       `json:"items"`
	Files       int64       `json:"files"`
	Changes     []SetChange `json:"changes"`
	AppliedBy   string      `json:"appliedBy,omitempty"`
	GoingBackTo string      `json:"goingBackTo,omitempty"`
}

// IngestionSetApplyListResponse — GET /api/ingestion-sets/{name}/applies.
type IngestionSetApplyListResponse struct {
	Applies []IngestionSetApplyResponse `json:"applies"`
}

// IngestionSetArchiveResponse is an apply as it was archived: its items and
// files.
type IngestionSetArchiveResponse struct {
	Set         string             `json:"set"`
	ApplyID     string             `json:"applyId"`
	AppliedAt   string             `json:"appliedAt"`
	AppliedBy   string             `json:"appliedBy"`
	Items       []IngestionSetItem `json:"items"`
	Files       []SetFile          `json:"files"`
	GoingBackTo string             `json:"goingBackTo,omitempty"`
}

// IngestionSetReleaseResponse — POST /api/ingestion-sets/{name}/release.
type IngestionSetReleaseResponse struct {
	Name     string `json:"name"`
	Released int64  `json:"released"`
}

// ─── Filters ─────────────────────────────────────────────────────────

// ListIngestionsFilters narrows GET /api/ingestions. Empty fields are omitted;
// ClientID takes an id or an identifier. After is the cursor from a previous
// page's NextCursor; Limit is 1 to 200.
type ListIngestionsFilters struct {
	ApplicationCode string
	ClientID        string
	Q               string
	Status          string
	Kind            string
	After           string
	Limit           *uint32
}

// ─── Resources ───────────────────────────────────────────────────────

// IngestionsResource — /api/ingestions/*, /api/ingestion-source-kinds and
// /api/ingestion-tests/*. The runtime's own API (/api/ingestion-runtime/v1/*)
// is not here: it is the runtime's client, not an application's.
type IngestionsResource struct {
	c *FlowCatalystClient
}

// Ingestions returns the ingestions resource accessor.
func (c *FlowCatalystClient) Ingestions() *IngestionsResource {
	return &IngestionsResource{c: c}
}

// escapeSegments joins path segments, each escaped.
func escapeSegments(first string, rest []string) string {
	segs := make([]string, 0, 1+len(rest))
	segs = append(segs, url.PathEscape(first))
	for _, s := range rest {
		segs = append(segs, url.PathEscape(s))
	}
	return strings.Join(segs, "/")
}

func ingestionPath(id string, rest ...string) string {
	return "/api/ingestions/" + escapeSegments(id, rest)
}

// List — GET /api/ingestions. Page on with the answer's NextCursor.
func (r *IngestionsResource) List(ctx context.Context, f *ListIngestionsFilters) (*IngestionListResponse, error) {
	q := NewQuery()
	if f != nil {
		q.String("applicationCode", f.ApplicationCode).
			String("clientId", f.ClientID).
			String("q", f.Q).
			String("status", f.Status).
			String("kind", f.Kind).
			String("after", f.After).
			Uint32("limit", f.Limit)
	}
	var out IngestionListResponse
	if err := r.c.Get(ctx, "/api/ingestions"+q.Encode(), &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Create — POST /api/ingestions. The platform answers 201 with the whole
// ingestion; a code already used is a 409 INGESTION_CODE_EXISTS.
func (r *IngestionsResource) Create(ctx context.Context, req *CreateIngestionRequest) (*IngestionResponse, error) {
	var out IngestionResponse
	if err := r.c.Post(ctx, "/api/ingestions", req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Get — GET /api/ingestions/{id}.
func (r *IngestionsResource) Get(ctx context.Context, id string) (*IngestionResponse, error) {
	var out IngestionResponse
	if err := r.c.Get(ctx, ingestionPath(id), &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Update — PUT /api/ingestions/{id}. Returns the updated ingestion.
func (r *IngestionsResource) Update(ctx context.Context, id string, req *UpdateIngestionRequest) (*IngestionResponse, error) {
	var out IngestionResponse
	if err := r.c.Put(ctx, ingestionPath(id), req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Delete — DELETE /api/ingestions/{id}.
func (r *IngestionsResource) Delete(ctx context.Context, id string) error {
	return r.c.Delete(ctx, ingestionPath(id), nil)
}

// Pause — POST /api/ingestions/{id}/pause. Returns the updated ingestion.
func (r *IngestionsResource) Pause(ctx context.Context, id string) (*IngestionResponse, error) {
	var out IngestionResponse
	if err := r.c.Post(ctx, ingestionPath(id, "pause"), nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Resume — POST /api/ingestions/{id}/resume. Returns the updated ingestion.
func (r *IngestionsResource) Resume(ctx context.Context, id string) (*IngestionResponse, error) {
	var out IngestionResponse
	if err := r.c.Post(ctx, ingestionPath(id, "resume"), nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ListVersions — GET /api/ingestions/{id}/versions.
func (r *IngestionsResource) ListVersions(ctx context.Context, id string) (*IngestionVersionListResponse, error) {
	var out IngestionVersionListResponse
	if err := r.c.Get(ctx, ingestionPath(id, "versions"), &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// SaveVersion — POST /api/ingestions/{id}/versions. The platform answers 201
// with the saved version.
func (r *IngestionsResource) SaveVersion(ctx context.Context, id string, req *SaveIngestionVersionRequest) (*IngestionVersionResponse, error) {
	var out IngestionVersionResponse
	if err := r.c.Post(ctx, ingestionPath(id, "versions"), req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// MakeVersionLive — POST /api/ingestions/{id}/versions/{version}/live.
// Returns the updated ingestion.
func (r *IngestionsResource) MakeVersionLive(ctx context.Context, id, version string) (*IngestionResponse, error) {
	var out IngestionResponse
	if err := r.c.Post(ctx, ingestionPath(id, "versions", version, "live"), nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Definition — GET /api/ingestions/{id}/definition: the YAML document that
// `fcdev ingest apply` reads back.
func (r *IngestionsResource) Definition(ctx context.Context, id string) (string, error) {
	var out yamlBody
	if err := r.c.Get(ctx, ingestionPath(id, "definition"), &out); err != nil {
		return "", err
	}
	return string(out), nil
}

// History — GET /api/ingestions/{id}/history, offset-paged. Nil limit and
// offset are omitted.
func (r *IngestionsResource) History(ctx context.Context, id string, limit, offset *uint32) (*IngestionHistoryResponse, error) {
	q := NewQuery().Uint32("limit", limit).Uint32("offset", offset).Encode()
	var out IngestionHistoryResponse
	if err := r.c.Get(ctx, ingestionPath(id, "history")+q, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// IngestionFailureFilters narrows Failures. Category is "payload" or
// "rejected"; Undismissed keeps only failures nobody has dismissed (with
// Category "rejected": the ingestion's open notices).
type IngestionFailureFilters struct {
	After       string
	Limit       *uint32
	Category    string
	Undismissed bool
}

// Failures — GET /api/ingestions/{id}/failures, newest first, cursor-paged:
// pass the previous page's NextCursor as After. Empty fields are omitted.
func (r *IngestionsResource) Failures(ctx context.Context, id string, f IngestionFailureFilters) (*IngestionFailureListResponse, error) {
	q := NewQuery().String("after", f.After).Uint32("limit", f.Limit).String("category", f.Category)
	if f.Undismissed {
		q = q.String("undismissed", "true")
	}
	var out IngestionFailureListResponse
	if err := r.c.Get(ctx, ingestionPath(id, "failures")+q.Encode(), &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// DismissFailure — POST /api/ingestions/{id}/failures/{failureId}/dismiss:
// marks a failure (a notice) dismissed; it stays in the history.
func (r *IngestionsResource) DismissFailure(ctx context.Context, id, failureID string) (*IngestionFailureResponse, error) {
	var out IngestionFailureResponse
	if err := r.c.Post(ctx, ingestionPath(id, "failures", failureID, "dismiss"), nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// IngestionSecretFetch is when a runtime last fetched a bound secret.
type IngestionSecretFetch struct {
	RuntimeID     string `json:"runtimeId"`
	Version       int64  `json:"version,omitempty"`
	PrincipalID   string `json:"principalId,omitempty"`
	LastFetchedAt string `json:"lastFetchedAt"`
	FetchCount    int64  `json:"fetchCount"`
}

// IngestionSecretResponse is a secret name the ingestion binds (or bound),
// with the runtimes' last fetches of it. Values are never returned.
type IngestionSecretResponse struct {
	Name    string                 `json:"name"`
	Bound   bool                   `json:"bound"`
	Fetches []IngestionSecretFetch `json:"fetches"`
}

// IngestionSecretListResponse — GET /api/ingestions/{id}/secrets.
type IngestionSecretListResponse struct {
	Secrets []IngestionSecretResponse `json:"secrets"`
}

// Secrets — GET /api/ingestions/{id}/secrets: the secret names the live
// definition binds and when each runtime last fetched them.
func (r *IngestionsResource) Secrets(ctx context.Context, id string) (*IngestionSecretListResponse, error) {
	var out IngestionSecretListResponse
	if err := r.c.Get(ctx, ingestionPath(id, "secrets"), &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// SourceKinds — GET /api/ingestion-source-kinds: the built-in kinds plus the
// kinds the live runtimes report.
func (r *IngestionsResource) SourceKinds(ctx context.Context) (*IngestionSourceKindListResponse, error) {
	var out IngestionSourceKindListResponse
	if err := r.c.Get(ctx, "/api/ingestion-source-kinds", &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// TestConnection — POST /api/ingestion-tests. Queues a test (the platform
// answers 202); poll GetTest for the result.
func (r *IngestionsResource) TestConnection(ctx context.Context, req *TestIngestionRequest) (*IngestionTestResponse, error) {
	var out IngestionTestResponse
	if err := r.c.Post(ctx, "/api/ingestion-tests", req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetTest — GET /api/ingestion-tests/{id}.
func (r *IngestionsResource) GetTest(ctx context.Context, id string) (*IngestionTestResponse, error) {
	var out IngestionTestResponse
	if err := r.c.Get(ctx, "/api/ingestion-tests/"+url.PathEscape(id), &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// IngestionSetsResource — /api/ingestion-sets/*. A set owns its ingestions
// (read-only to edits) until it is released.
type IngestionSetsResource struct {
	c *FlowCatalystClient
}

// IngestionSets returns the ingestion-sets resource accessor.
func (c *FlowCatalystClient) IngestionSets() *IngestionSetsResource {
	return &IngestionSetsResource{c: c}
}

func ingestionSetPath(name string, rest ...string) string {
	return "/api/ingestion-sets/" + escapeSegments(name, rest)
}

// List — GET /api/ingestion-sets.
func (r *IngestionSetsResource) List(ctx context.Context) (*IngestionSetListResponse, error) {
	var out IngestionSetListResponse
	if err := r.c.Get(ctx, "/api/ingestion-sets", &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Apply — PUT /api/ingestion-sets/{name}. One transaction: any unresolvable
// item fails the whole apply, and an ingestion the set owns but no longer
// lists is deleted. dryRun checks and reports the changes, writing nothing;
// nil omits the flag.
func (r *IngestionSetsResource) Apply(ctx context.Context, name string, req *ApplyIngestionSetRequest, dryRun *bool) (*IngestionSetPlanResponse, error) {
	q := NewQuery().Bool("dryRun", dryRun).Encode()
	var out IngestionSetPlanResponse
	if err := r.c.Put(ctx, ingestionSetPath(name)+q, req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ListApplies — GET /api/ingestion-sets/{name}/applies.
func (r *IngestionSetsResource) ListApplies(ctx context.Context, name string) (*IngestionSetApplyListResponse, error) {
	var out IngestionSetApplyListResponse
	if err := r.c.Get(ctx, ingestionSetPath(name, "applies"), &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetApply — GET /api/ingestion-sets/{name}/applies/{applyId}: the apply's
// archived items and files.
func (r *IngestionSetsResource) GetApply(ctx context.Context, name, applyID string) (*IngestionSetArchiveResponse, error) {
	var out IngestionSetArchiveResponse
	if err := r.c.Get(ctx, ingestionSetPath(name, "applies", applyID), &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Reapply — POST /api/ingestion-sets/{name}/applies/{applyId}/reapply applies
// an archived apply's items again as a new apply. dryRun as in Apply.
func (r *IngestionSetsResource) Reapply(ctx context.Context, name, applyID string, dryRun *bool) (*IngestionSetPlanResponse, error) {
	q := NewQuery().Bool("dryRun", dryRun).Encode()
	var out IngestionSetPlanResponse
	if err := r.c.Post(ctx, ingestionSetPath(name, "applies", applyID, "reapply")+q, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Release — POST /api/ingestion-sets/{name}/release ends the set's ownership
// and changes nothing else.
func (r *IngestionSetsResource) Release(ctx context.Context, name string) (*IngestionSetReleaseResponse, error) {
	var out IngestionSetReleaseResponse
	if err := r.c.Post(ctx, ingestionSetPath(name, "release"), nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
