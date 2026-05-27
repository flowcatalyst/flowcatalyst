// Package operations holds the OpenAPI-spec use cases. Each file is
// one verb; the shape mirrors the other platform operations packages.
package operations

import (
	"encoding/json"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
)

const (
	SpecSyncedType = "platform:developer:application-openapi:synced"
	Source         = "platform:developer"
)

// ApplicationOpenApiSpecSynced is emitted after a sync — whether the
// document was new (Unchanged=false) or byte-identical to the prior
// CURRENT (Unchanged=true). The audit log keeps both for completeness.
type ApplicationOpenApiSpecSynced struct {
	Metadata             usecase.EventMetadata
	ApplicationID        string
	ApplicationCode      string
	SpecID               string
	Version              string
	SpecHash             string
	ArchivedPriorVersion *string
	HasBreaking          bool
	Unchanged            bool
}

func subjectFor(specID string) string { return "platform.application-openapi." + specID }
func groupFor(appID string) string    { return "platform:application-openapi:" + appID }

func (e ApplicationOpenApiSpecSynced) EventID() string       { return e.Metadata.EventID }
func (e ApplicationOpenApiSpecSynced) EventType() string     { return SpecSyncedType }
func (e ApplicationOpenApiSpecSynced) SpecVersion() string   { return "1.0" }
func (e ApplicationOpenApiSpecSynced) Source() string        { return Source }
func (e ApplicationOpenApiSpecSynced) Subject() string       { return subjectFor(e.SpecID) }
func (e ApplicationOpenApiSpecSynced) Time() time.Time       { return e.Metadata.OccurredAt }
func (e ApplicationOpenApiSpecSynced) PrincipalID() string   { return e.Metadata.PrincipalID }
func (e ApplicationOpenApiSpecSynced) CorrelationID() string { return e.Metadata.CorrelationID }
func (e ApplicationOpenApiSpecSynced) CausationID() string   { return e.Metadata.CausationID }
func (e ApplicationOpenApiSpecSynced) ExecutionID() string   { return e.Metadata.ExecutionID }
func (e ApplicationOpenApiSpecSynced) MessageGroup() string  { return groupFor(e.ApplicationID) }
func (e ApplicationOpenApiSpecSynced) ToDataJSON() ([]byte, error) {
	return json.Marshal(struct {
		ApplicationID        string  `json:"applicationId"`
		ApplicationCode      string  `json:"applicationCode"`
		SpecID               string  `json:"specId"`
		Version              string  `json:"version"`
		SpecHash             string  `json:"specHash"`
		ArchivedPriorVersion *string `json:"archivedPriorVersion,omitempty"`
		HasBreaking          bool    `json:"hasBreaking"`
		Unchanged            bool    `json:"unchanged"`
	}{
		e.ApplicationID, e.ApplicationCode, e.SpecID, e.Version, e.SpecHash,
		e.ArchivedPriorVersion, e.HasBreaking, e.Unchanged,
	})
}
