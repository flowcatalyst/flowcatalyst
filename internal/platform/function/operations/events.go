package operations

import (
	"encoding/json"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
)

const (
	FunctionCreatedType          = "platform:admin:function:created"
	FunctionUpdatedType          = "platform:admin:function:updated"
	FunctionDeletedType          = "platform:admin:function:deleted"
	FunctionVersionPublishedType = "platform:admin:function-version:published"
	FunctionVersionRetiredType   = "platform:admin:function-version:retired"
	FunctionAliasSetType         = "platform:admin:function-alias:set"
	FunctionAliasDeletedType     = "platform:admin:function-alias:deleted"
	FunctionSettingSetType       = "platform:admin:function-setting:set"
	FunctionSettingDeletedType   = "platform:admin:function-setting:deleted"
	FunctionPromotedType         = "platform:admin:function:promoted"
	FunctionScheduleWiredType    = "platform:admin:function-schedule:wired"
	RouteSetType                 = "platform:admin:function-route:set"
	RouteDeletedType             = "platform:admin:function-route:deleted"
	Source                       = "platform:admin"
)

func subjectFor(id string) string { return "platform.function." + id }
func groupFor(id string) string   { return "platform:function:" + id }

// FunctionCreated is emitted on successful creation.
type FunctionCreated struct {
	Metadata   usecase.EventMetadata
	FunctionID string
	Address    string
	Name       string
}

func (e FunctionCreated) EventID() string       { return e.Metadata.EventID }
func (e FunctionCreated) EventType() string     { return FunctionCreatedType }
func (e FunctionCreated) SpecVersion() string   { return "1.0" }
func (e FunctionCreated) Source() string        { return Source }
func (e FunctionCreated) Subject() string       { return subjectFor(e.FunctionID) }
func (e FunctionCreated) Time() time.Time       { return e.Metadata.OccurredAt }
func (e FunctionCreated) PrincipalID() string   { return e.Metadata.PrincipalID }
func (e FunctionCreated) CorrelationID() string { return e.Metadata.CorrelationID }
func (e FunctionCreated) CausationID() string   { return e.Metadata.CausationID }
func (e FunctionCreated) ExecutionID() string   { return e.Metadata.ExecutionID }
func (e FunctionCreated) MessageGroup() string  { return groupFor(e.FunctionID) }
func (e FunctionCreated) ToDataJSON() ([]byte, error) {
	return json.Marshal(struct {
		FunctionID string `json:"functionId"`
		Address    string `json:"address"`
		Name       string `json:"name"`
	}{e.FunctionID, e.Address, e.Name})
}

// FunctionUpdated is emitted on a settings update (description, pool, warm,
// limits).
type FunctionUpdated struct {
	Metadata   usecase.EventMetadata
	FunctionID string
	Address    string
}

func (e FunctionUpdated) EventID() string       { return e.Metadata.EventID }
func (e FunctionUpdated) EventType() string     { return FunctionUpdatedType }
func (e FunctionUpdated) SpecVersion() string   { return "1.0" }
func (e FunctionUpdated) Source() string        { return Source }
func (e FunctionUpdated) Subject() string       { return subjectFor(e.FunctionID) }
func (e FunctionUpdated) Time() time.Time       { return e.Metadata.OccurredAt }
func (e FunctionUpdated) PrincipalID() string   { return e.Metadata.PrincipalID }
func (e FunctionUpdated) CorrelationID() string { return e.Metadata.CorrelationID }
func (e FunctionUpdated) CausationID() string   { return e.Metadata.CausationID }
func (e FunctionUpdated) ExecutionID() string   { return e.Metadata.ExecutionID }
func (e FunctionUpdated) MessageGroup() string  { return groupFor(e.FunctionID) }
func (e FunctionUpdated) ToDataJSON() ([]byte, error) {
	return json.Marshal(struct {
		FunctionID string `json:"functionId"`
		Address    string `json:"address"`
	}{e.FunctionID, e.Address})
}

// FunctionDeleted is emitted on deletion.
type FunctionDeleted struct {
	Metadata   usecase.EventMetadata
	FunctionID string
	Address    string
}

func (e FunctionDeleted) EventID() string       { return e.Metadata.EventID }
func (e FunctionDeleted) EventType() string     { return FunctionDeletedType }
func (e FunctionDeleted) SpecVersion() string   { return "1.0" }
func (e FunctionDeleted) Source() string        { return Source }
func (e FunctionDeleted) Subject() string       { return subjectFor(e.FunctionID) }
func (e FunctionDeleted) Time() time.Time       { return e.Metadata.OccurredAt }
func (e FunctionDeleted) PrincipalID() string   { return e.Metadata.PrincipalID }
func (e FunctionDeleted) CorrelationID() string { return e.Metadata.CorrelationID }
func (e FunctionDeleted) CausationID() string   { return e.Metadata.CausationID }
func (e FunctionDeleted) ExecutionID() string   { return e.Metadata.ExecutionID }
func (e FunctionDeleted) MessageGroup() string  { return groupFor(e.FunctionID) }
func (e FunctionDeleted) ToDataJSON() ([]byte, error) {
	return json.Marshal(struct {
		FunctionID string `json:"functionId"`
		Address    string `json:"address"`
	}{e.FunctionID, e.Address})
}

// FunctionVersionPublished is emitted when publish stores a new version (not
// on the idempotent same-digest replay, which persists nothing).
type FunctionVersionPublished struct {
	Metadata   usecase.EventMetadata
	FunctionID string
	Address    string
	Number     int32
	Digest     string
	Runtime    string
}

func (e FunctionVersionPublished) EventID() string       { return e.Metadata.EventID }
func (e FunctionVersionPublished) EventType() string     { return FunctionVersionPublishedType }
func (e FunctionVersionPublished) SpecVersion() string   { return "1.0" }
func (e FunctionVersionPublished) Source() string        { return Source }
func (e FunctionVersionPublished) Subject() string       { return subjectFor(e.FunctionID) }
func (e FunctionVersionPublished) Time() time.Time       { return e.Metadata.OccurredAt }
func (e FunctionVersionPublished) PrincipalID() string   { return e.Metadata.PrincipalID }
func (e FunctionVersionPublished) CorrelationID() string { return e.Metadata.CorrelationID }
func (e FunctionVersionPublished) CausationID() string   { return e.Metadata.CausationID }
func (e FunctionVersionPublished) ExecutionID() string   { return e.Metadata.ExecutionID }
func (e FunctionVersionPublished) MessageGroup() string  { return groupFor(e.FunctionID) }
func (e FunctionVersionPublished) ToDataJSON() ([]byte, error) {
	return json.Marshal(struct {
		FunctionID string `json:"functionId"`
		Address    string `json:"address"`
		Number     int32  `json:"number"`
		Digest     string `json:"digest"`
		Runtime    string `json:"runtime"`
	}{e.FunctionID, e.Address, e.Number, e.Digest, e.Runtime})
}

// FunctionVersionRetired is emitted when a version is retired.
type FunctionVersionRetired struct {
	Metadata   usecase.EventMetadata
	FunctionID string
	Address    string
	Number     int32
}

func (e FunctionVersionRetired) EventID() string       { return e.Metadata.EventID }
func (e FunctionVersionRetired) EventType() string     { return FunctionVersionRetiredType }
func (e FunctionVersionRetired) SpecVersion() string   { return "1.0" }
func (e FunctionVersionRetired) Source() string        { return Source }
func (e FunctionVersionRetired) Subject() string       { return subjectFor(e.FunctionID) }
func (e FunctionVersionRetired) Time() time.Time       { return e.Metadata.OccurredAt }
func (e FunctionVersionRetired) PrincipalID() string   { return e.Metadata.PrincipalID }
func (e FunctionVersionRetired) CorrelationID() string { return e.Metadata.CorrelationID }
func (e FunctionVersionRetired) CausationID() string   { return e.Metadata.CausationID }
func (e FunctionVersionRetired) ExecutionID() string   { return e.Metadata.ExecutionID }
func (e FunctionVersionRetired) MessageGroup() string  { return groupFor(e.FunctionID) }
func (e FunctionVersionRetired) ToDataJSON() ([]byte, error) {
	return json.Marshal(struct {
		FunctionID string `json:"functionId"`
		Address    string `json:"address"`
		Number     int32  `json:"number"`
	}{e.FunctionID, e.Address, e.Number})
}

// FunctionAliasSet is emitted when an alias is created or repointed. Promote
// (setting the `live` alias) uses [FunctionPromoted] instead, which carries
// the wiring diff; FunctionAliasSet is for every other alias name, and for
// `live`'s own pointer-move bookkeeping alongside FunctionPromoted.
type FunctionAliasSet struct {
	Metadata   usecase.EventMetadata
	FunctionID string
	Address    string
	Alias      string
	Number     int32
}

func (e FunctionAliasSet) EventID() string       { return e.Metadata.EventID }
func (e FunctionAliasSet) EventType() string     { return FunctionAliasSetType }
func (e FunctionAliasSet) SpecVersion() string   { return "1.0" }
func (e FunctionAliasSet) Source() string        { return Source }
func (e FunctionAliasSet) Subject() string       { return subjectFor(e.FunctionID) }
func (e FunctionAliasSet) Time() time.Time       { return e.Metadata.OccurredAt }
func (e FunctionAliasSet) PrincipalID() string   { return e.Metadata.PrincipalID }
func (e FunctionAliasSet) CorrelationID() string { return e.Metadata.CorrelationID }
func (e FunctionAliasSet) CausationID() string   { return e.Metadata.CausationID }
func (e FunctionAliasSet) ExecutionID() string   { return e.Metadata.ExecutionID }
func (e FunctionAliasSet) MessageGroup() string  { return groupFor(e.FunctionID) }
func (e FunctionAliasSet) ToDataJSON() ([]byte, error) {
	return json.Marshal(struct {
		FunctionID string `json:"functionId"`
		Address    string `json:"address"`
		Alias      string `json:"alias"`
		Number     int32  `json:"number"`
	}{e.FunctionID, e.Address, e.Alias, e.Number})
}

// FunctionAliasDeleted is emitted when an alias is removed. Removing `live`
// also unwires (see FunctionPromoted's doc); this event still fires for the
// pointer removal itself.
type FunctionAliasDeleted struct {
	Metadata   usecase.EventMetadata
	FunctionID string
	Address    string
	Alias      string
}

func (e FunctionAliasDeleted) EventID() string       { return e.Metadata.EventID }
func (e FunctionAliasDeleted) EventType() string     { return FunctionAliasDeletedType }
func (e FunctionAliasDeleted) SpecVersion() string   { return "1.0" }
func (e FunctionAliasDeleted) Source() string        { return Source }
func (e FunctionAliasDeleted) Subject() string       { return subjectFor(e.FunctionID) }
func (e FunctionAliasDeleted) Time() time.Time       { return e.Metadata.OccurredAt }
func (e FunctionAliasDeleted) PrincipalID() string   { return e.Metadata.PrincipalID }
func (e FunctionAliasDeleted) CorrelationID() string { return e.Metadata.CorrelationID }
func (e FunctionAliasDeleted) CausationID() string   { return e.Metadata.CausationID }
func (e FunctionAliasDeleted) ExecutionID() string   { return e.Metadata.ExecutionID }
func (e FunctionAliasDeleted) MessageGroup() string  { return groupFor(e.FunctionID) }
func (e FunctionAliasDeleted) ToDataJSON() ([]byte, error) {
	return json.Marshal(struct {
		FunctionID string `json:"functionId"`
		Address    string `json:"address"`
		Alias      string `json:"alias"`
	}{e.FunctionID, e.Address, e.Alias})
}

// FunctionPromoted is emitted when the `live` alias moves (promote) or is
// removed (unwire), carrying the wiring reconciliation summary (WP8, plan
// §8.5).
type FunctionPromoted struct {
	Metadata usecase.EventMetadata

	FunctionID string
	Address    string
	// Number is the version now live, or 0 when `live` was removed.
	Number int32

	DispatchPoolCode string

	SubscriptionsCreated int
	SubscriptionsUpdated int
	SubscriptionsDeleted int

	SchedulesCreated int
	SchedulesUpdated int
	SchedulesDeleted int
}

func (e FunctionPromoted) EventID() string       { return e.Metadata.EventID }
func (e FunctionPromoted) EventType() string     { return FunctionPromotedType }
func (e FunctionPromoted) SpecVersion() string   { return "1.0" }
func (e FunctionPromoted) Source() string        { return Source }
func (e FunctionPromoted) Subject() string       { return subjectFor(e.FunctionID) }
func (e FunctionPromoted) Time() time.Time       { return e.Metadata.OccurredAt }
func (e FunctionPromoted) PrincipalID() string   { return e.Metadata.PrincipalID }
func (e FunctionPromoted) CorrelationID() string { return e.Metadata.CorrelationID }
func (e FunctionPromoted) CausationID() string   { return e.Metadata.CausationID }
func (e FunctionPromoted) ExecutionID() string   { return e.Metadata.ExecutionID }
func (e FunctionPromoted) MessageGroup() string  { return groupFor(e.FunctionID) }
func (e FunctionPromoted) ToDataJSON() ([]byte, error) {
	return json.Marshal(struct {
		FunctionID           string `json:"functionId"`
		Address              string `json:"address"`
		Number               int32  `json:"number"`
		DispatchPoolCode     string `json:"dispatchPoolCode"`
		SubscriptionsCreated int    `json:"subscriptionsCreated"`
		SubscriptionsUpdated int    `json:"subscriptionsUpdated"`
		SubscriptionsDeleted int    `json:"subscriptionsDeleted"`
		SchedulesCreated     int    `json:"schedulesCreated"`
		SchedulesUpdated     int    `json:"schedulesUpdated"`
		SchedulesDeleted     int    `json:"schedulesDeleted"`
	}{
		e.FunctionID, e.Address, e.Number, e.DispatchPoolCode,
		e.SubscriptionsCreated, e.SubscriptionsUpdated, e.SubscriptionsDeleted,
		e.SchedulesCreated, e.SchedulesUpdated, e.SchedulesDeleted,
	})
}

// FunctionScheduleWired is emitted for each msg_scheduled_jobs row the
// promote wiring reconciliation (WP8) creates, updates or deletes.
// scheduledjob/operations defines its own ScheduledJobCreated/Updated/
// Deleted, but each wraps an unexported embedded `commonEvent` struct
// (scheduledjob/operations/events.go) whose field name IS the unexported
// type name — a composite literal for it cannot be written from any other
// package, so this promote wiring code (in the function/operations package)
// uses this plain event instead of reusing theirs. subscriptionops and
// dispatchpoolops event types have ordinary exported fields and ARE reused
// directly (see reconcileSubscriptions / ensureDispatchPool).
type FunctionScheduleWired struct {
	Metadata       usecase.EventMetadata
	FunctionID     string
	ScheduledJobID string
	Code           string
	// Action is "created", "updated", or "deleted".
	Action string
}

func (e FunctionScheduleWired) EventID() string       { return e.Metadata.EventID }
func (e FunctionScheduleWired) EventType() string     { return FunctionScheduleWiredType }
func (e FunctionScheduleWired) SpecVersion() string   { return "1.0" }
func (e FunctionScheduleWired) Source() string        { return Source }
func (e FunctionScheduleWired) Subject() string       { return subjectFor(e.FunctionID) }
func (e FunctionScheduleWired) Time() time.Time       { return e.Metadata.OccurredAt }
func (e FunctionScheduleWired) PrincipalID() string   { return e.Metadata.PrincipalID }
func (e FunctionScheduleWired) CorrelationID() string { return e.Metadata.CorrelationID }
func (e FunctionScheduleWired) CausationID() string   { return e.Metadata.CausationID }
func (e FunctionScheduleWired) ExecutionID() string   { return e.Metadata.ExecutionID }
func (e FunctionScheduleWired) MessageGroup() string  { return groupFor(e.FunctionID) }
func (e FunctionScheduleWired) ToDataJSON() ([]byte, error) {
	return json.Marshal(struct {
		FunctionID     string `json:"functionId"`
		ScheduledJobID string `json:"scheduledJobId"`
		Code           string `json:"code"`
		Action         string `json:"action"`
	}{e.FunctionID, e.ScheduledJobID, e.Code, e.Action})
}

// RouteSet is emitted when a public route is created or repointed.
type RouteSet struct {
	Metadata   usecase.EventMetadata
	FunctionID string
	RouteID    string
	Hostname   string
	PathPrefix string
}

func (e RouteSet) EventID() string       { return e.Metadata.EventID }
func (e RouteSet) EventType() string     { return RouteSetType }
func (e RouteSet) SpecVersion() string   { return "1.0" }
func (e RouteSet) Source() string        { return Source }
func (e RouteSet) Subject() string       { return subjectFor(e.FunctionID) }
func (e RouteSet) Time() time.Time       { return e.Metadata.OccurredAt }
func (e RouteSet) PrincipalID() string   { return e.Metadata.PrincipalID }
func (e RouteSet) CorrelationID() string { return e.Metadata.CorrelationID }
func (e RouteSet) CausationID() string   { return e.Metadata.CausationID }
func (e RouteSet) ExecutionID() string   { return e.Metadata.ExecutionID }
func (e RouteSet) MessageGroup() string  { return groupFor(e.FunctionID) }
func (e RouteSet) ToDataJSON() ([]byte, error) {
	return json.Marshal(struct {
		FunctionID string `json:"functionId"`
		RouteID    string `json:"routeId"`
		Hostname   string `json:"hostname"`
		PathPrefix string `json:"pathPrefix"`
	}{e.FunctionID, e.RouteID, e.Hostname, e.PathPrefix})
}

// RouteDeleted is emitted when a public route is removed.
type RouteDeleted struct {
	Metadata   usecase.EventMetadata
	FunctionID string
	RouteID    string
	Hostname   string
	PathPrefix string
}

func (e RouteDeleted) EventID() string       { return e.Metadata.EventID }
func (e RouteDeleted) EventType() string     { return RouteDeletedType }
func (e RouteDeleted) SpecVersion() string   { return "1.0" }
func (e RouteDeleted) Source() string        { return Source }
func (e RouteDeleted) Subject() string       { return subjectFor(e.FunctionID) }
func (e RouteDeleted) Time() time.Time       { return e.Metadata.OccurredAt }
func (e RouteDeleted) PrincipalID() string   { return e.Metadata.PrincipalID }
func (e RouteDeleted) CorrelationID() string { return e.Metadata.CorrelationID }
func (e RouteDeleted) CausationID() string   { return e.Metadata.CausationID }
func (e RouteDeleted) ExecutionID() string   { return e.Metadata.ExecutionID }
func (e RouteDeleted) MessageGroup() string  { return groupFor(e.FunctionID) }
func (e RouteDeleted) ToDataJSON() ([]byte, error) {
	return json.Marshal(struct {
		FunctionID string `json:"functionId"`
		RouteID    string `json:"routeId"`
		Hostname   string `json:"hostname"`
		PathPrefix string `json:"pathPrefix"`
	}{e.FunctionID, e.RouteID, e.Hostname, e.PathPrefix})
}

// FunctionSettingSet is emitted when a config/secret/db setting is written.
// Value is NEVER included (settings are write-only on the wire — plan §8.2).
type FunctionSettingSet struct {
	Metadata   usecase.EventMetadata
	FunctionID string
	Kind       string
	Key        string
}

func (e FunctionSettingSet) EventID() string       { return e.Metadata.EventID }
func (e FunctionSettingSet) EventType() string     { return FunctionSettingSetType }
func (e FunctionSettingSet) SpecVersion() string   { return "1.0" }
func (e FunctionSettingSet) Source() string        { return Source }
func (e FunctionSettingSet) Subject() string       { return subjectFor(e.FunctionID) }
func (e FunctionSettingSet) Time() time.Time       { return e.Metadata.OccurredAt }
func (e FunctionSettingSet) PrincipalID() string   { return e.Metadata.PrincipalID }
func (e FunctionSettingSet) CorrelationID() string { return e.Metadata.CorrelationID }
func (e FunctionSettingSet) CausationID() string   { return e.Metadata.CausationID }
func (e FunctionSettingSet) ExecutionID() string   { return e.Metadata.ExecutionID }
func (e FunctionSettingSet) MessageGroup() string  { return groupFor(e.FunctionID) }
func (e FunctionSettingSet) ToDataJSON() ([]byte, error) {
	return json.Marshal(struct {
		FunctionID string `json:"functionId"`
		Kind       string `json:"kind"`
		Key        string `json:"key"`
	}{e.FunctionID, e.Kind, e.Key})
}

// FunctionSettingDeleted is emitted when a setting is removed.
type FunctionSettingDeleted struct {
	Metadata   usecase.EventMetadata
	FunctionID string
	Kind       string
	Key        string
}

func (e FunctionSettingDeleted) EventID() string       { return e.Metadata.EventID }
func (e FunctionSettingDeleted) EventType() string     { return FunctionSettingDeletedType }
func (e FunctionSettingDeleted) SpecVersion() string   { return "1.0" }
func (e FunctionSettingDeleted) Source() string        { return Source }
func (e FunctionSettingDeleted) Subject() string       { return subjectFor(e.FunctionID) }
func (e FunctionSettingDeleted) Time() time.Time       { return e.Metadata.OccurredAt }
func (e FunctionSettingDeleted) PrincipalID() string   { return e.Metadata.PrincipalID }
func (e FunctionSettingDeleted) CorrelationID() string { return e.Metadata.CorrelationID }
func (e FunctionSettingDeleted) CausationID() string   { return e.Metadata.CausationID }
func (e FunctionSettingDeleted) ExecutionID() string   { return e.Metadata.ExecutionID }
func (e FunctionSettingDeleted) MessageGroup() string  { return groupFor(e.FunctionID) }
func (e FunctionSettingDeleted) ToDataJSON() ([]byte, error) {
	return json.Marshal(struct {
		FunctionID string `json:"functionId"`
		Kind       string `json:"kind"`
		Key        string `json:"key"`
	}{e.FunctionID, e.Kind, e.Key})
}
