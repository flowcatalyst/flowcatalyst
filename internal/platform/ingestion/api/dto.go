// dto.go contains the wire-format types for the ingestion API.
package api

import (
	"encoding/json"
	"fmt"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/ingestion"
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
	Live        *bool   `json:"live,omitempty" doc:"Make the first version live. Default true; false saves it to be made live later."`
	Note        *string `json:"note,omitempty"`
	IngestionDefinition
}

// UpdateIngestionRequest is the wire body for PUT /api/ingestions/{id}: the
// definition saved as the next version and made live.
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
	ManagedBy *string         `json:"managedBy,omitempty" doc:"Id of the set that owns the ingestion; it is read-only until the set is released"`
	CreatedAt httpcompat.Time `json:"createdAt"`
	UpdatedAt httpcompat.Time `json:"updatedAt"`
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
	IngestionDefinition
	Note        *string         `json:"note,omitempty"`
	AppliedFrom *string         `json:"appliedFrom,omitempty" doc:"Id of the set apply that produced the version"`
	CreatedBy   *string         `json:"createdBy,omitempty"`
	CreatedAt   httpcompat.Time `json:"createdAt"`
}

func fromVersion(v ingestion.Version, live bool) (IngestionVersionResponse, error) {
	body, err := definitionBody(v.Definition)
	if err != nil {
		return IngestionVersionResponse{}, err
	}
	return IngestionVersionResponse{
		ID: v.ID, Version: v.Number, Live: live, IngestionDefinition: body,
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
	Message      string          `json:"message"`
	AfterRecords int64           `json:"afterRecords"`
	Excerpt      *string         `json:"excerpt,omitempty" doc:"The start of the payload, only for an ingestion that keeps failed payloads"`
	OccurredAt   httpcompat.Time `json:"occurredAt"`
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
