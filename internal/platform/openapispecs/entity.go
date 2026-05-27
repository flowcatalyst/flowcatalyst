// Package openapispecs is the per-application OpenAPI document store.
// Each application has at most one CURRENT row at a time; previous
// syncs are flipped to ARCHIVED so the lineage is auditable.
//
// Port of fc-platform/src/application_openapi_spec.
package openapispecs

import (
	"encoding/json"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/tsid"
)

// Status is the spec lifecycle state.
type Status string

const (
	StatusCurrent  Status = "CURRENT"
	StatusArchived Status = "ARCHIVED"
)

// ParseStatus is the lenient parser. Unknown → CURRENT.
func ParseStatus(s string) Status {
	if s == string(StatusArchived) {
		return StatusArchived
	}
	return StatusCurrent
}

// ChangeNotes is the structured diff persisted as JSONB alongside an
// archived row, so the UI can render rich diffs. The associated
// ChangeNotesText is a pre-rendered human summary for listings.
type ChangeNotes struct {
	AddedPaths        []string `json:"addedPaths,omitempty"`
	RemovedPaths      []string `json:"removedPaths,omitempty"`
	AddedSchemas      []string `json:"addedSchemas,omitempty"`
	RemovedSchemas    []string `json:"removedSchemas,omitempty"`
	RemovedOperations []string `json:"removedOperations,omitempty"`
	HasBreaking       bool     `json:"hasBreaking"`
}

// IsEmpty reports whether the diff has any structural changes. Used by
// the summary renderer.
func (n ChangeNotes) IsEmpty() bool {
	return len(n.AddedPaths) == 0 &&
		len(n.RemovedPaths) == 0 &&
		len(n.AddedSchemas) == 0 &&
		len(n.RemovedSchemas) == 0 &&
		len(n.RemovedOperations) == 0
}

// OpenApiSpec is one stored OpenAPI document for an application.
type OpenApiSpec struct {
	ID              string          `json:"id"`
	ApplicationID   string          `json:"applicationId"`
	Version         string          `json:"version"`
	Status          Status          `json:"status"`
	Spec            json.RawMessage `json:"spec"`
	SpecHash        string          `json:"specHash"`
	ChangeNotes     *ChangeNotes    `json:"changeNotes,omitempty"`
	ChangeNotesText *string         `json:"changeNotesText,omitempty"`
	SyncedAt        time.Time       `json:"syncedAt"`
	SyncedBy        *string         `json:"syncedBy,omitempty"`
	CreatedAt       time.Time       `json:"createdAt"`
	UpdatedAt       time.Time       `json:"updatedAt"`
}

// IDStr satisfies usecase.HasID.
func (s OpenApiSpec) IDStr() string { return s.ID }

// New constructs a CURRENT-status spec with a fresh TSID.
func New(applicationID, version string, spec json.RawMessage, specHash string) *OpenApiSpec {
	now := time.Now().UTC()
	return &OpenApiSpec{
		ID:            tsid.Generate(tsid.ApplicationOpenApiSpec),
		ApplicationID: applicationID,
		Version:       version,
		Status:        StatusCurrent,
		Spec:          spec,
		SpecHash:      specHash,
		SyncedAt:      now,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
}
