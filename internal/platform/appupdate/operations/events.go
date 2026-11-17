// Package operations holds the app-update use cases: publish, change a
// rollout, republish and roll back (docs/app-updates-design.md).
package operations

import (
	"encoding/json"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/appupdate"
	"github.com/flowcatalyst/go-sdk/usecase"
)

// Event-type codes (catalogued in seed/event_types.go).
const (
	UpdatePublishedType      = "platform:app-update:update:published"
	RolloutChangedType       = "platform:app-update:update:rollout-changed"
	UpdateRepublishedType    = "platform:app-update:update:republished"
	RolledBackToEmbeddedType = "platform:app-update:update:rolled-back-to-embedded"
	// Source is the envelope source of every app-update event.
	Source = "platform:app-update"
)

func subjectFor(id string) string { return "platform.appupdate." + id }

// groupFor orders a branch's events: one message group per phone app.
func groupFor(applicationID, appKey string) string {
	return "platform:app-update:" + applicationID + ":" + appKey
}

// eventBase is the envelope every app-update event shares.
type eventBase struct {
	Metadata usecase.EventMetadata
	group    string
}

func (e eventBase) EventID() string       { return e.Metadata.EventID }
func (e eventBase) SpecVersion() string   { return "1.0" }
func (e eventBase) Source() string        { return Source }
func (e eventBase) Subject() string       { return e.Metadata.Subject }
func (e eventBase) Time() time.Time       { return e.Metadata.OccurredAt }
func (e eventBase) PrincipalID() string   { return e.Metadata.PrincipalID }
func (e eventBase) CorrelationID() string { return e.Metadata.CorrelationID }
func (e eventBase) CausationID() string   { return e.Metadata.CausationID }
func (e eventBase) ExecutionID() string   { return e.Metadata.ExecutionID }
func (e eventBase) MessageGroup() string  { return e.group }

func newBase(ec usecase.ExecutionContext, eventType string, u *appupdate.Update) eventBase {
	return eventBase{
		Metadata: usecase.NewEventMetadata(ec, eventType, Source, subjectFor(u.ID)),
		group:    groupFor(u.ApplicationID, u.AppKey),
	}
}

// BranchData is the branch every event names.
type BranchData struct {
	UpdateID        string `json:"updateId"`
	ApplicationID   string `json:"applicationId"`
	ApplicationCode string `json:"applicationCode"`
	AppKey          string `json:"appKey"`
	Platform        string `json:"platform"`
	RuntimeVersion  string `json:"runtimeVersion"`
	Channel         string `json:"channel"`
}

func branchData(u *appupdate.Update) BranchData {
	return BranchData{
		UpdateID: u.ID, ApplicationID: u.ApplicationID, ApplicationCode: u.ApplicationCode,
		AppKey: u.AppKey, Platform: u.Platform, RuntimeVersion: u.RuntimeVersion, Channel: u.Channel,
	}
}

// UpdatePublished: an update was registered and is live for its rollout.
type UpdatePublished struct {
	eventBase
	BranchData
	ManifestID        string  `json:"manifestId"`
	CreatedAt         string  `json:"createdAt"`
	RolloutPercent    int     `json:"rolloutPercent"`
	LaunchAssetSHA256 string  `json:"launchAssetSha256"`
	AssetCount        int     `json:"assetCount"`
	Message           *string `json:"message,omitempty"`
	SourceCommit      *string `json:"sourceCommit,omitempty"`
}

func (e UpdatePublished) EventType() string { return UpdatePublishedType }
func (e UpdatePublished) ToDataJSON() ([]byte, error) {
	return json.Marshal(struct {
		BranchData
		ManifestID        string  `json:"manifestId"`
		CreatedAt         string  `json:"createdAt"`
		RolloutPercent    int     `json:"rolloutPercent"`
		LaunchAssetSHA256 string  `json:"launchAssetSha256"`
		AssetCount        int     `json:"assetCount"`
		Message           *string `json:"message,omitempty"`
		SourceCommit      *string `json:"sourceCommit,omitempty"`
	}{e.BranchData, e.ManifestID, e.CreatedAt, e.RolloutPercent, e.LaunchAssetSHA256, e.AssetCount, e.Message, e.SourceCommit})
}

// RolloutChanged: the share of a branch's phones an update reaches changed.
type RolloutChanged struct {
	eventBase
	BranchData
	PreviousPercent int `json:"previousPercent"`
	RolloutPercent  int `json:"rolloutPercent"`
}

func (e RolloutChanged) EventType() string { return RolloutChangedType }
func (e RolloutChanged) ToDataJSON() ([]byte, error) {
	return json.Marshal(struct {
		BranchData
		PreviousPercent int `json:"previousPercent"`
		RolloutPercent  int `json:"rolloutPercent"`
	}{e.BranchData, e.PreviousPercent, e.RolloutPercent})
}

// Republish reasons.
const (
	// ReasonRollback: a rollback to the previous update.
	ReasonRollback = "ROLLBACK"
	// ReasonRepublish: an earlier update named by id was made live again.
	ReasonRepublish = "REPUBLISH"
)

// UpdateRepublished: an earlier update's files were published again as a new
// update (a rollback to the previous update, or a republish by id).
type UpdateRepublished struct {
	eventBase
	BranchData
	RepublishedFrom string  `json:"republishedFrom"`
	ManifestID      string  `json:"manifestId"`
	CreatedAt       string  `json:"createdAt"`
	Reason          string  `json:"reason"`
	Message         *string `json:"message,omitempty"`
}

func (e UpdateRepublished) EventType() string { return UpdateRepublishedType }
func (e UpdateRepublished) ToDataJSON() ([]byte, error) {
	return json.Marshal(struct {
		BranchData
		RepublishedFrom string  `json:"republishedFrom"`
		ManifestID      string  `json:"manifestId"`
		CreatedAt       string  `json:"createdAt"`
		Reason          string  `json:"reason"`
		Message         *string `json:"message,omitempty"`
	}{e.BranchData, e.RepublishedFrom, e.ManifestID, e.CreatedAt, e.Reason, e.Message})
}

// RolledBackToEmbedded: the branch's phones are told to run their embedded bundle.
type RolledBackToEmbedded struct {
	eventBase
	BranchData
	CommitTime string  `json:"commitTime"`
	Message    *string `json:"message,omitempty"`
}

func (e RolledBackToEmbedded) EventType() string { return RolledBackToEmbeddedType }
func (e RolledBackToEmbedded) ToDataJSON() ([]byte, error) {
	return json.Marshal(struct {
		BranchData
		CommitTime string  `json:"commitTime"`
		Message    *string `json:"message,omitempty"`
	}{e.BranchData, e.CommitTime, e.Message})
}
