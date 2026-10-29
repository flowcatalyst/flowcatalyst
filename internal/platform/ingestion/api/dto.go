// dto.go contains the wire-format types for the ingestion API.
package api

import (
	"encoding/json"
	"fmt"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/ingestion"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/ingestion/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httpcompat"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/jsontime"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecase"
)

// IngestionDefinition is the editable part of an ingestion, as the API reads and
// writes it: the file form without its identity. Source and Split are tagged
// unions ({"kind": ..., fields}); docs/ingestion-design.md section 2 has the
// kinds and their settings, and GET /api/ingestion-source-kinds the kinds a
// runtime adds. Secrets are names, never values.
type IngestionDefinition struct {
	Name               string           `json:"name"`
	KeepFailedPayloads bool             `json:"keepFailedPayloads,omitempty" doc:"Keep the first 2 KB of a payload that failed. Off unless the client has agreed."`
	Source             json.RawMessage  `json:"source" doc:"Where the data comes from: {kind: http|sftp|s3|sqs|rabbitmq|database-poll|api-poll|custom, ...}"`
	Split              json.RawMessage  `json:"split" doc:"How a payload becomes records: {kind: none|csv|json|xml, ...}"`
	Target             ingestion.Target `json:"target"`
}

// parseDefinition reads the body strictly into a Definition; an unknown field
// in source, split or target is a 400. A function, not a method: huma cannot
// build a response schema for a struct that embeds a type with methods.
func parseDefinition(b IngestionDefinition) (ingestion.Definition, error) {
	raw, err := json.Marshal(b)
	if err != nil {
		return ingestion.Definition{}, usecase.Validation("INGESTION_INVALID", err.Error())
	}
	def, err := ingestion.ParseDefinition(raw)
	if err != nil {
		return ingestion.Definition{}, usecase.Validation("INGESTION_INVALID", err.Error())
	}
	return def, nil
}

func definitionBody(d ingestion.Definition) (IngestionDefinition, error) {
	source, err := json.Marshal(d.Source)
	if err != nil {
		return IngestionDefinition{}, fmt.Errorf("marshal source: %w", err)
	}
	split, err := json.Marshal(d.Split)
	if err != nil {
		return IngestionDefinition{}, fmt.Errorf("marshal split: %w", err)
	}
	return IngestionDefinition{Name: d.Name, KeepFailedPayloads: d.KeepFailedPayloads, Source: source, Split: split, Target: d.Target}, nil
}

// CreateIngestionRequest is the wire body for POST /api/ingestions.
type CreateIngestionRequest struct {
	Application string  `json:"application" doc:"Code of the owning application, which must be of type INTEGRATION"`
	Client      string  `json:"client" doc:"The client's id (clt_...) or identifier"`
	Code        string  `json:"code" doc:"Lowercase letters, digits and hyphens, unique within the application and client"`
	Live        *bool   `json:"live,omitempty" doc:"Make the first version live. Default true; without the promote permission the first version is saved as a draft whatever this says (the answer's save says so), and someone who holds promote makes it live. false always saves a draft."`
	Note        *string `json:"note,omitempty"`
	IngestionDefinition
}

// UpdateIngestionRequest is the wire body for PUT /api/ingestions/{id}: the
// definition saved as the next version, deduplicated by digest. With the
// promote permission it is made live; without it, it is saved as a draft that
// someone who holds promote makes live (the answer's save says which).
type UpdateIngestionRequest struct {
	ExpectedVersion int     `json:"expectedVersion" doc:"The live version the caller read (0 when none is live); a different current version is a 409"`
	Note            *string `json:"note,omitempty"`
	IngestionDefinition
}

// SaveIngestionVersionRequest is the wire body for POST /api/ingestions/{id}/versions.
type SaveIngestionVersionRequest struct {
	Live *bool   `json:"live,omitempty" doc:"Also make the saved version live (needs the promote permission). Default false."`
	Note *string `json:"note,omitempty"`
	IngestionDefinition
}

// IngestionResponse mirrors ingestion.Ingestion with its application's code
// and its client's identifier.
type IngestionResponse struct {
	ID               string `json:"id"`
	ApplicationID    string `json:"applicationId"`
	ApplicationCode  string `json:"applicationCode"`
	ClientID         string `json:"clientId"`
	ClientIdentifier string `json:"clientIdentifier"`
	Code             string `json:"code"`
	Status           string `json:"status" enum:"ACTIVE,PAUSED"`
	// Version is the live version, 0 while none is live; LatestVersion the
	// highest saved.
	Version       int `json:"version"`
	LatestVersion int `json:"latestVersion"`
	IngestionDefinition
	ManagedBy *string `json:"managedBy,omitempty" doc:"Id of the set that owns the ingestion; it is read-only until the set is released"`
	// RuntimeNotice is set while a running ingestion's kind is carried by no
	// live runtime: nothing has failed, nobody is there to run it yet.
	RuntimeNotice *string `json:"runtimeNotice,omitempty"`
	// Save is what the create or update that answered did; absent from every
	// other answer.
	Save *IngestionSaveOutcome `json:"save,omitempty" doc:"What this create or update did: saved and made live, made an existing version live, saved as a draft awaiting promote, matched an existing draft, or changed nothing"`
	// Warnings are what the definition says that saves but will not run as
	// meant (an sftp source that checks no host key).
	Warnings []string `json:"warnings,omitempty" doc:"What the definition says that will not run as meant, such as an sftp source that neither names the server's host key nor accepts any"`
	// Config is where the settings stand in this environment.
	Config IngestionConfigStatus `json:"config" doc:"Where the settings stand: INLINE (in the definition), or the config entry the source names: SET, MISSING (the ingestion does not run until it is set) or INVALID"`
	// Notices counts the failures of category rejected nobody has dismissed:
	// the platform refused the runtime's jobs permanently, and nothing more
	// is made until someone acts and dismisses them.
	Notices        int              `json:"notices" doc:"Undismissed notices: rejected failures (GET .../failures?category=rejected&undismissed=true)"`
	LatestNotice   *string          `json:"latestNotice,omitempty" doc:"The newest undismissed notice's message"`
	LatestNoticeAt *httpcompat.Time `json:"latestNoticeAt,omitempty"`
	CreatedAt      httpcompat.Time  `json:"createdAt"`
	UpdatedAt      httpcompat.Time  `json:"updatedAt"`
}

// IngestionConfigStatus is the state of the config entry a source names
// (docs/ingestion-design.md section 13): the entry is platform config of the
// ingestion's application, section ingest, at its client, under Key.
type IngestionConfigStatus struct {
	Key      string                 `json:"key,omitempty" doc:"The entry's key; absent while the settings are inline"`
	Status   string                 `json:"status" enum:"INLINE,SET,MISSING,INVALID"`
	Problems []ingestion.FieldError `json:"problems,omitempty" doc:"Why the entry is INVALID"`
	// UpdatedAt is when the entry was last written.
	UpdatedAt *httpcompat.Time `json:"updatedAt,omitempty"`
}

func configStatus(r ingestion.Resolved) IngestionConfigStatus {
	out := IngestionConfigStatus{Key: r.Key, Status: string(r.Status), Problems: r.Problems}
	if r.UpdatedAt != nil {
		out.UpdatedAt = new(jsontime.New(*r.UpdatedAt))
	}
	return out
}

// withConfig adds where the settings stand, and the warnings of the effective
// definition (an entry that is not set among them). A function, not a method,
// as parseDefinition.
func withConfig(r *IngestionResponse, res ingestion.Resolved) {
	r.Config = configStatus(res)
	r.Warnings = res.Warnings()
}

// IngestionSaveOutcome is what a save of a definition did. A save is
// deduplicated by the digest of the definition: what an existing version
// already says is never saved twice.
type IngestionSaveOutcome struct {
	Outcome string `json:"outcome" enum:"UNCHANGED,SAVED,MADE_LIVE,DRAFT,DRAFT_EXISTS" doc:"UNCHANGED: the live version already says it, nothing written. SAVED: saved as a new version and made live. MADE_LIVE: a saved version that was not live already says it and was made live, nothing new saved. DRAFT: saved as a new version that is not live. DRAFT_EXISTS: a saved version that is not live already says it, nothing saved"`
	// Version is the version the outcome names.
	Version int `json:"version" doc:"The live version (UNCHANGED, SAVED, MADE_LIVE) or the draft (DRAFT, DRAFT_EXISTS)"`
	// AwaitingPromote is true for DRAFT and DRAFT_EXISTS.
	AwaitingPromote bool   `json:"awaitingPromote" doc:"The definition is in a version that is not live; someone who holds platform:ingestion:ingestion:promote makes it live"`
	Message         string `json:"message"`
}

func saveOutcome(r operations.SaveResult) *IngestionSaveOutcome {
	return &IngestionSaveOutcome{Outcome: string(r.Outcome), Version: r.Version, AwaitingPromote: r.AwaitingPromote(), Message: r.Message()}
}

// withNotices adds what the ingestion's undismissed notices add up to. A
// function, not a method, as parseDefinition.
func withNotices(r *IngestionResponse, n ingestion.Notices) {
	r.Notices = n.Count
	if n.Count > 0 {
		r.LatestNotice = &n.LatestMessage
		r.LatestNoticeAt = new(jsontime.New(n.LatestAt))
	}
}

func fromEntity(i *ingestion.Ingestion, applicationCode, clientIdentifier string) (IngestionResponse, error) {
	body, err := definitionBody(i.Definition)
	if err != nil {
		return IngestionResponse{}, err
	}
	status := "ACTIVE"
	if !i.Active {
		status = "PAUSED"
	}
	return IngestionResponse{
		ID:                  i.ID,
		ApplicationID:       string(i.ApplicationID),
		ApplicationCode:     applicationCode,
		ClientID:            string(i.ClientID),
		ClientIdentifier:    clientIdentifier,
		Code:                i.Code,
		Status:              status,
		Version:             i.Version,
		LatestVersion:       i.LatestVersion,
		IngestionDefinition: body,
		ManagedBy:           i.ManagedBy,
		Warnings:            i.Definition.Warnings(),
		CreatedAt:           jsontime.New(i.CreatedAt),
		UpdatedAt:           jsontime.New(i.UpdatedAt),
	}, nil
}

// IngestionListResponse is the wire shape for GET /api/ingestions.
type IngestionListResponse struct {
	Ingestions []IngestionResponse `json:"ingestions"`
	NextCursor *string             `json:"nextCursor,omitempty" doc:"Pass as after to read the next page; absent on the last"`
	HasMore    bool                `json:"hasMore"`
}

// IngestionVersionResponse is one saved version of an ingestion.
type IngestionVersionResponse struct {
	ID      string `json:"id"`
	Version int    `json:"version"`
	// Live is true for the version runtimes receive.
	Live bool `json:"live"`
	// Draft is true for a version saved after the live one (or while none is
	// live) that is not live: it waits for promote.
	Draft bool `json:"draft" doc:"A version saved after the live one (or while none is live) that is not live: it waits for someone who holds promote to make it live"`
	// Digest is the sha256 of the definition's canonical form.
	Digest string `json:"digest,omitempty" doc:"sha256 (hex) of the definition's canonical form; a save whose definition has the digest of a saved version reuses that version"`
	IngestionDefinition
	// Save is what the save that answered did (POST .../versions only).
	Save        *IngestionSaveOutcome `json:"save,omitempty" doc:"What this save did (the answer of POST .../versions only)"`
	Warnings    []string              `json:"warnings,omitempty" doc:"What the saved definition says that will not run as meant, such as CONFIG_MISSING for a config entry that is not set (the answer of POST .../versions only)"`
	Note        *string               `json:"note,omitempty"`
	AppliedFrom *string               `json:"appliedFrom,omitempty" doc:"Id of the set apply that produced the version"`
	CreatedBy   *string               `json:"createdBy,omitempty"`
	CreatedAt   httpcompat.Time       `json:"createdAt"`
}

// fromVersion renders v of an ingestion whose live version is liveVersion
// (0 while none is).
func fromVersion(v ingestion.Version, liveVersion int) (IngestionVersionResponse, error) {
	body, err := definitionBody(v.Definition)
	if err != nil {
		return IngestionVersionResponse{}, err
	}
	return IngestionVersionResponse{
		ID: v.ID, Version: v.Number, Live: v.Number == liveVersion, Draft: v.Number > liveVersion, Digest: v.Digest, IngestionDefinition: body,
		Note: v.Note, AppliedFrom: v.AppliedFrom, CreatedBy: v.CreatedBy, CreatedAt: jsontime.New(v.CreatedAt),
	}, nil
}

// IngestionVersionListResponse is the wire shape for GET /api/ingestions/{id}/versions,
// newest first.
type IngestionVersionListResponse struct {
	Versions []IngestionVersionResponse `json:"versions"`
}

// IngestionHistoryEntry is one audit-log row about an ingestion.
type IngestionHistoryEntry struct {
	ID            string          `json:"id"`
	Operation     string          `json:"operation"`
	PrincipalID   *string         `json:"principalId,omitempty"`
	PrincipalName *string         `json:"principalName,omitempty"`
	Details       json.RawMessage `json:"details,omitempty" doc:"The command the operation ran with"`
	PerformedAt   httpcompat.Time `json:"performedAt"`
}

// IngestionHistoryResponse is the wire shape for GET /api/ingestions/{id}/history,
// newest first.
type IngestionHistoryResponse struct {
	Entries []IngestionHistoryEntry `json:"entries"`
	HasMore bool                    `json:"hasMore"`
}

// IngestionFailureResponse is one failure a runtime reported.
type IngestionFailureResponse struct {
	ID           string          `json:"id"`
	RuntimeID    *string         `json:"runtimeId,omitempty"`
	Version      *int            `json:"version,omitempty"`
	Kind         string          `json:"kind"`
	Category     string          `json:"category" enum:"payload,rejected" doc:"payload: a record or payload the runtime could not take. rejected: a notice, the platform refused the runtime's jobs permanently"`
	Message      string          `json:"message"`
	AfterRecords int64           `json:"afterRecords"`
	Excerpt      *string         `json:"excerpt,omitempty" doc:"The start of the payload, only for an ingestion that keeps failed payloads"`
	OccurredAt   httpcompat.Time `json:"occurredAt"`
	// CreatedAt is when the platform recorded it; retention counts from it.
	CreatedAt   httpcompat.Time  `json:"createdAt"`
	DismissedAt *httpcompat.Time `json:"dismissedAt,omitempty"`
	DismissedBy *string          `json:"dismissedBy,omitempty" doc:"The principal who dismissed it"`
}

// IngestionFailureListResponse is the wire shape for GET /api/ingestions/{id}/failures,
// newest first.
type IngestionFailureListResponse struct {
	Failures   []IngestionFailureResponse `json:"failures"`
	NextCursor *string                    `json:"nextCursor,omitempty" doc:"Pass as after to read the next page; absent on the last"`
	HasMore    bool                       `json:"hasMore"`
}

// IngestionSourceKindResponse is a source kind a definition can use.
type IngestionSourceKindResponse struct {
	Kind        string `json:"kind"`
	Description string `json:"description,omitempty"`
	// BuiltIn kinds are the platform's own; the others are reported by a live
	// runtime and are written as {kind: custom, type: <kind>, settings: ...}.
	BuiltIn bool `json:"builtIn"`
	// SingleOwner kinds run on one runtime at a time (a lease).
	SingleOwner        bool            `json:"singleOwner"`
	RequiredPermission string          `json:"requiredPermission,omitempty" doc:"Creating or changing an ingestion of this kind needs this permission"`
	Settings           json.RawMessage `json:"settings,omitempty" doc:"The kind's settings, as the runtime describes them for the form"`
}

// IngestionSourceKindListResponse is the wire shape for GET /api/ingestion-source-kinds.
type IngestionSourceKindListResponse struct {
	Kinds []IngestionSourceKindResponse `json:"kinds"`
}

// TestIngestionRequest is the wire body for POST /api/ingestion-tests: a saved
// ingestion to test, or a definition not yet saved with the application and
// client it would belong to.
type TestIngestionRequest struct {
	IngestionID *string              `json:"ingestionId,omitempty" doc:"Test this saved ingestion's live definition"`
	Application string               `json:"application,omitempty" doc:"With definition: code of the owning application, which must be of type INTEGRATION"`
	Client      string               `json:"client,omitempty" doc:"With definition: the client's id (clt_...) or identifier"`
	Code        string               `json:"code,omitempty" doc:"With definition: the ingestion's code, which is the config entry's key for a source that says config: true"`
	Definition  *IngestionDefinition `json:"definition,omitempty" doc:"A definition to test without saving it"`
}

// IngestionTestResponse is a queued test and, once a runtime has answered, its
// result. A runtime tries the source once from its own network with its own
// secrets and dispatches nothing; no secret is in the result.
type IngestionTestResponse struct {
	ID          string  `json:"id"`
	IngestionID *string `json:"ingestionId,omitempty"`
	// Kind is the source kind a runtime must carry to take the test.
	Kind   string `json:"kind"`
	Status string `json:"status" enum:"PENDING,RUNNING,DONE,EXPIRED" doc:"PENDING until a runtime takes it, RUNNING while it tries, DONE with its result, EXPIRED when none answered in time"`
	// RuntimeID is the runtime that took the test.
	RuntimeID   *string          `json:"runtimeId,omitempty"`
	CreatedAt   httpcompat.Time  `json:"createdAt"`
	ExpiresAt   httpcompat.Time  `json:"expiresAt"`
	CompletedAt *httpcompat.Time `json:"completedAt,omitempty"`
	Outcome     *string          `json:"outcome,omitempty" enum:"RESULTS,NO_RESULTS,REFUSED,FAILED,NOT_SUPPORTED"`
	Steps       []string         `json:"steps,omitempty" doc:"What the runtime did, step by step, for a person"`
	Records     *int64           `json:"records,omitempty" doc:"How many records the split makes of the answer, when it answered"`
	Excerpt     *string          `json:"excerpt,omitempty" doc:"The start of the answer, only for an ingestion that keeps failed payloads"`
	Notice      *string          `json:"notice,omitempty" doc:"Set while no live runtime carries the kind: the test waits, nothing has failed"`
}

// IngestionSetItem is one ingestion of a set in the definition-file form: its
// identity, whether it runs, and the definition.
type IngestionSetItem struct {
	Application string `json:"application" doc:"Code of the owning application, which must be of type INTEGRATION"`
	Client      string `json:"client" doc:"The client's id (clt_...) or identifier"`
	Code        string `json:"code"`
	Active      *bool  `json:"active,omitempty" doc:"Whether the ingestion runs. When omitted: a new ingestion whose source names a config entry is created paused (set the entry and its secrets, then resume), any other is created active, and one that exists keeps its state, so a pause made since stays. When given, an apply enforces it."`
	IngestionDefinition
}

// file reads the item as a definition file.
func (i IngestionSetItem) file() (ingestion.File, error) {
	def, err := parseDefinition(i.IngestionDefinition)
	if err != nil {
		return ingestion.File{}, err
	}
	return ingestion.File{Application: i.Application, Client: i.Client, Code: i.Code, Active: i.Active, Definition: def}, nil
}

func itemOf(f ingestion.File) (IngestionSetItem, error) {
	def, err := definitionBody(f.Definition)
	if err != nil {
		return IngestionSetItem{}, err
	}
	return IngestionSetItem{Application: f.Application, Client: f.Client, Code: f.Code, Active: f.Active, IngestionDefinition: def}, nil
}

// ApplyIngestionSetRequest is the wire body for PUT /api/ingestion-sets/{name}:
// every ingestion the set should own. An empty list deletes whatever the set
// owns.
type ApplyIngestionSetRequest struct {
	Items []IngestionSetItem  `json:"items" doc:"The set's ingestions, no two of the same application, client and code"`
	Files []ingestion.SetFile `json:"files,omitempty" maxItems:"500" doc:"The files the items were read from, as they are in the repository; kept with the apply (at most 2 MB in all)"`
}

// IngestionSetPlanResponse is what an apply did, or what a dry run would do.
type IngestionSetPlanResponse struct {
	Name    string                `json:"name"`
	DryRun  bool                  `json:"dryRun"`
	ApplyID *string               `json:"applyId,omitempty" doc:"The recorded apply; absent for a dry run and for an apply that changed nothing"`
	Changes []ingestion.SetChange `json:"changes" doc:"Each item of the set, then each ingestion removed from it"`
}

// IngestionSetResponse is a set and how many ingestions it owns.
type IngestionSetResponse struct {
	ID         string          `json:"id"`
	Name       string          `json:"name"`
	Ingestions int             `json:"ingestions"`
	CreatedAt  httpcompat.Time `json:"createdAt"`
	UpdatedAt  httpcompat.Time `json:"updatedAt"`
}

// IngestionSetListResponse lists sets by name.
type IngestionSetListResponse struct {
	Sets []IngestionSetResponse `json:"sets"`
}

// IngestionSetApplyResponse is one kept apply of a set.
type IngestionSetApplyResponse struct {
	ID          string                `json:"id"`
	Set         string                `json:"set"`
	AppliedAt   httpcompat.Time       `json:"appliedAt"`
	AppliedBy   *string               `json:"appliedBy,omitempty"`
	GoingBackTo *string               `json:"goingBackTo,omitempty" doc:"The earlier apply it applied again"`
	Items       int                   `json:"items" doc:"How many items it was given"`
	Files       int                   `json:"files" doc:"How many files they were read from"`
	Changes     []ingestion.SetChange `json:"changes"`
}

func fromSetApply(set string, a ingestion.SetApply) IngestionSetApplyResponse {
	changes := a.Summary.Changes
	if changes == nil {
		changes = []ingestion.SetChange{}
	}
	return IngestionSetApplyResponse{
		ID: a.ID, Set: set, AppliedAt: jsontime.New(a.AppliedAt), AppliedBy: a.AppliedBy, GoingBackTo: a.Summary.GoingBackTo,
		Items: a.Summary.Items, Files: a.Summary.Files, Changes: changes,
	}
}

// IngestionSetApplyListResponse lists a set's kept applies, newest first.
type IngestionSetApplyListResponse struct {
	Applies []IngestionSetApplyResponse `json:"applies"`
}

// IngestionSetArchiveResponse is what one apply was given, as kept in the
// artifact store.
type IngestionSetArchiveResponse struct {
	Set         string              `json:"set"`
	ApplyID     string              `json:"applyId"`
	AppliedAt   httpcompat.Time     `json:"appliedAt"`
	AppliedBy   string              `json:"appliedBy"`
	GoingBackTo *string             `json:"goingBackTo,omitempty"`
	Items       []IngestionSetItem  `json:"items"`
	Files       []ingestion.SetFile `json:"files"`
}

// IngestionSetReleaseResponse is how many ingestions a set let go of.
type IngestionSetReleaseResponse struct {
	Name     string `json:"name"`
	Released int    `json:"released"`
}
