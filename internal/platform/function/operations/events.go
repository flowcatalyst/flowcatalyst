package operations

import (
	"encoding/json"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/function"
	"github.com/flowcatalyst/go-sdk/usecase"
)

// Event-type codes (docs/platform-bug-fixes.md F11, owner rulings 2026-10-12
// and 2026-10-13). The function events Java (FunctionEvents) and Rust
// (fc-platform-functions events.rs) also emit use their platform:function:*
// codes, their source and their data; migrations 073 and 074 renamed the
// catalogue rows and subscription bindings of Go's older
// platform:admin:function* codes. The events Java and Rust have no
// counterpart for keep their platform:admin:function* codes and source
// (seed/emitted_catalogue_test.go lists them, with why).
const (
	FunctionCreatedType  = "platform:function:function:created"
	FunctionUpdatedType  = "platform:function:function:updated"
	FunctionDeletedType  = "platform:function:function:deleted"
	VersionPublishedType = "platform:function:version:published"
	VersionRetiredType   = "platform:function:version:retired"
	VersionPrunedType    = "platform:function:version:pruned"
	AliasChangedType     = "platform:function:alias:changed"
	AliasRemovedType     = "platform:function:alias:removed"
	ConfigUpdatedType    = "platform:function:config:updated"
	SecretSetType        = "platform:function:secret:set"
	SecretDeletedType    = "platform:function:secret:deleted"
	// Trace contexts (docs/function-trace-design.md section 1): Go-originated,
	// in the shared family because the function API is one resource tree.
	TraceEnabledType  = "platform:function:trace:enabled"
	TraceDisabledType = "platform:function:trace:disabled"
	// Source is the envelope source of the events shared with Java and Rust.
	Source = "platform:function"

	// Go-only: the promote wiring summary, the scheduled jobs it wires, the
	// public routes, and DB settings (Java and Rust have no DB kind).
	FunctionPromotedType      = "platform:admin:function:promoted"
	FunctionScheduleWiredType = "platform:admin:function-schedule:wired"
	RouteSetType              = "platform:admin:function-route:set"
	RouteDeletedType          = "platform:admin:function-route:deleted"
	DBSettingSetType          = "platform:admin:function-setting:set"
	DBSettingDeletedType      = "platform:admin:function-setting:deleted"
	// AdminSource is the envelope source of the Go-only events.
	AdminSource = "platform:admin"

	// FunctionStatusActive is function:updated's status. Java's and Rust's
	// functions are ACTIVE or DISABLED; a Go function has no disabled state,
	// so it is always ACTIVE.
	FunctionStatusActive = "ACTIVE"
)

func subjectFor(id string) string { return "platform.function." + id }
func groupFor(id string) string   { return "platform:function:" + id }

// FunctionCreated is emitted on successful creation: {functionId, address,
// applicationId, clientId?}. Java and Rust also send the function's runtime;
// a Go function has none (Go's runtime is per version), so it is absent.
type FunctionCreated struct {
	Metadata      usecase.EventMetadata
	FunctionID    string
	Address       string
	ApplicationID string
	// ClientID is the owning client; nil (absent) for a platform function.
	ClientID *string
	// ServiceAccountID is the webhook verifier account chosen at create;
	// absent when the function uses its application's.
	ServiceAccountID *string
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
		FunctionID       string  `json:"functionId"`
		Address          string  `json:"address"`
		ApplicationID    string  `json:"applicationId"`
		ClientID         *string `json:"clientId,omitempty"`
		ServiceAccountID *string `json:"serviceAccountId,omitempty"`
	}{e.FunctionID, e.Address, e.ApplicationID, e.ClientID, e.ServiceAccountID})
}

// FunctionUpdated is emitted on a settings update (description, pool, warm,
// limits): {functionId, address, description?, status}.
type FunctionUpdated struct {
	Metadata    usecase.EventMetadata
	FunctionID  string
	Address     string
	Description *string
	// Status is always FunctionStatusActive.
	Status string
	// ServiceAccountID is the function's webhook verifier account after the
	// update (nil: the application's); PreviousServiceAccountID the one
	// before. Reported only when they differ.
	ServiceAccountID         *string
	PreviousServiceAccountID *string
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
		FunctionID  string  `json:"functionId"`
		Address     string  `json:"address"`
		Description *string `json:"description,omitempty"`
		Status      string  `json:"status"`
		// serviceAccountChange is present only when the verifier account
		// changed: {from, to}, each an id or null (null = the application's).
		ServiceAccountChange *saChange `json:"serviceAccountChange,omitempty"`
	}{e.FunctionID, e.Address, e.Description, e.Status, serviceAccountChange(e.PreviousServiceAccountID, e.ServiceAccountID)})
}

type saChange struct {
	From *string `json:"from"`
	To   *string `json:"to"`
}

func serviceAccountChange(from, to *string) *saChange {
	if (from == nil && to == nil) || (from != nil && to != nil && *from == *to) {
		return nil
	}
	return &saChange{From: from, To: to}
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

// VersionPublished is emitted when publish stores a new version (not on the
// idempotent same-digest replay, which persists nothing): {functionId,
// address, versionId, version, digest, pool}. pool is the runner pool. Java's
// and Rust's signer pair is never sent (Go does not sign versions).
type VersionPublished struct {
	Metadata   usecase.EventMetadata
	FunctionID string
	Address    string
	VersionID  string
	Version    int32
	Digest     string
	Pool       string
}

func (e VersionPublished) EventID() string       { return e.Metadata.EventID }
func (e VersionPublished) EventType() string     { return VersionPublishedType }
func (e VersionPublished) SpecVersion() string   { return "1.0" }
func (e VersionPublished) Source() string        { return Source }
func (e VersionPublished) Subject() string       { return subjectFor(e.FunctionID) }
func (e VersionPublished) Time() time.Time       { return e.Metadata.OccurredAt }
func (e VersionPublished) PrincipalID() string   { return e.Metadata.PrincipalID }
func (e VersionPublished) CorrelationID() string { return e.Metadata.CorrelationID }
func (e VersionPublished) CausationID() string   { return e.Metadata.CausationID }
func (e VersionPublished) ExecutionID() string   { return e.Metadata.ExecutionID }
func (e VersionPublished) MessageGroup() string  { return groupFor(e.FunctionID) }
func (e VersionPublished) ToDataJSON() ([]byte, error) {
	return json.Marshal(struct {
		FunctionID string `json:"functionId"`
		Address    string `json:"address"`
		VersionID  string `json:"versionId"`
		Version    int32  `json:"version"`
		Digest     string `json:"digest"`
		Pool       string `json:"pool"`
	}{e.FunctionID, e.Address, e.VersionID, e.Version, e.Digest, e.Pool})
}

// VersionRetired is emitted when a version is retired: {functionId, address,
// versionId, version}.
type VersionRetired struct {
	Metadata   usecase.EventMetadata
	FunctionID string
	Address    string
	VersionID  string
	Version    int32
}

func (e VersionRetired) EventID() string       { return e.Metadata.EventID }
func (e VersionRetired) EventType() string     { return VersionRetiredType }
func (e VersionRetired) SpecVersion() string   { return "1.0" }
func (e VersionRetired) Source() string        { return Source }
func (e VersionRetired) Subject() string       { return subjectFor(e.FunctionID) }
func (e VersionRetired) Time() time.Time       { return e.Metadata.OccurredAt }
func (e VersionRetired) PrincipalID() string   { return e.Metadata.PrincipalID }
func (e VersionRetired) CorrelationID() string { return e.Metadata.CorrelationID }
func (e VersionRetired) CausationID() string   { return e.Metadata.CausationID }
func (e VersionRetired) ExecutionID() string   { return e.Metadata.ExecutionID }
func (e VersionRetired) MessageGroup() string  { return groupFor(e.FunctionID) }
func (e VersionRetired) ToDataJSON() ([]byte, error) {
	return json.Marshal(struct {
		FunctionID string `json:"functionId"`
		Address    string `json:"address"`
		VersionID  string `json:"versionId"`
		Version    int32  `json:"version"`
	}{e.FunctionID, e.Address, e.VersionID, e.Version})
}

// VersionPruned is emitted once per version the retention limit deleted
// (Java and Rust are to emit the same event): {functionId, address, versionId,
// version, digest}. The version row is gone when the event is read; the
// event is the record of it.
type VersionPruned struct {
	Metadata   usecase.EventMetadata
	FunctionID string
	Address    string
	VersionID  string
	Version    int32
	Digest     string
}

func (e VersionPruned) EventID() string       { return e.Metadata.EventID }
func (e VersionPruned) EventType() string     { return VersionPrunedType }
func (e VersionPruned) SpecVersion() string   { return "1.0" }
func (e VersionPruned) Source() string        { return Source }
func (e VersionPruned) Subject() string       { return subjectFor(e.FunctionID) }
func (e VersionPruned) Time() time.Time       { return e.Metadata.OccurredAt }
func (e VersionPruned) PrincipalID() string   { return e.Metadata.PrincipalID }
func (e VersionPruned) CorrelationID() string { return e.Metadata.CorrelationID }
func (e VersionPruned) CausationID() string   { return e.Metadata.CausationID }
func (e VersionPruned) ExecutionID() string   { return e.Metadata.ExecutionID }
func (e VersionPruned) MessageGroup() string  { return groupFor(e.FunctionID) }
func (e VersionPruned) ToDataJSON() ([]byte, error) {
	return json.Marshal(struct {
		FunctionID string `json:"functionId"`
		Address    string `json:"address"`
		VersionID  string `json:"versionId"`
		Version    int32  `json:"version"`
		Digest     string `json:"digest"`
	}{e.FunctionID, e.Address, e.VersionID, e.Version, e.Digest})
}

// AliasChanged is emitted whenever an alias is pointed at a version, `live`
// (promote, which also emits [FunctionPromoted]) included: {functionId,
// address, alias, versionId, version, previousVersionId?}.
// previousVersionId is the version the alias pointed at before; absent when
// it is new.
type AliasChanged struct {
	Metadata          usecase.EventMetadata
	FunctionID        string
	Address           string
	Alias             string
	VersionID         string
	Version           int32
	PreviousVersionID *string
}

func (e AliasChanged) EventID() string       { return e.Metadata.EventID }
func (e AliasChanged) EventType() string     { return AliasChangedType }
func (e AliasChanged) SpecVersion() string   { return "1.0" }
func (e AliasChanged) Source() string        { return Source }
func (e AliasChanged) Subject() string       { return subjectFor(e.FunctionID) }
func (e AliasChanged) Time() time.Time       { return e.Metadata.OccurredAt }
func (e AliasChanged) PrincipalID() string   { return e.Metadata.PrincipalID }
func (e AliasChanged) CorrelationID() string { return e.Metadata.CorrelationID }
func (e AliasChanged) CausationID() string   { return e.Metadata.CausationID }
func (e AliasChanged) ExecutionID() string   { return e.Metadata.ExecutionID }
func (e AliasChanged) MessageGroup() string  { return groupFor(e.FunctionID) }
func (e AliasChanged) ToDataJSON() ([]byte, error) {
	return json.Marshal(struct {
		FunctionID        string  `json:"functionId"`
		Address           string  `json:"address"`
		Alias             string  `json:"alias"`
		VersionID         string  `json:"versionId"`
		Version           int32   `json:"version"`
		PreviousVersionID *string `json:"previousVersionId,omitempty"`
	}{e.FunctionID, e.Address, e.Alias, e.VersionID, e.Version, e.PreviousVersionID})
}

// AliasRemoved is emitted when an alias is removed: {functionId, address,
// alias, versionId, version}, the version it pointed at. Removing `live` also
// unwires and emits [FunctionPromoted] (Java and Rust refuse to remove
// `live`). Removing an alias that does not exist emits nothing.
type AliasRemoved struct {
	Metadata   usecase.EventMetadata
	FunctionID string
	Address    string
	Alias      string
	VersionID  string
	Version    int32
}

func (e AliasRemoved) EventID() string       { return e.Metadata.EventID }
func (e AliasRemoved) EventType() string     { return AliasRemovedType }
func (e AliasRemoved) SpecVersion() string   { return "1.0" }
func (e AliasRemoved) Source() string        { return Source }
func (e AliasRemoved) Subject() string       { return subjectFor(e.FunctionID) }
func (e AliasRemoved) Time() time.Time       { return e.Metadata.OccurredAt }
func (e AliasRemoved) PrincipalID() string   { return e.Metadata.PrincipalID }
func (e AliasRemoved) CorrelationID() string { return e.Metadata.CorrelationID }
func (e AliasRemoved) CausationID() string   { return e.Metadata.CausationID }
func (e AliasRemoved) ExecutionID() string   { return e.Metadata.ExecutionID }
func (e AliasRemoved) MessageGroup() string  { return groupFor(e.FunctionID) }
func (e AliasRemoved) ToDataJSON() ([]byte, error) {
	return json.Marshal(struct {
		FunctionID string `json:"functionId"`
		Address    string `json:"address"`
		Alias      string `json:"alias"`
		VersionID  string `json:"versionId"`
		Version    int32  `json:"version"`
	}{e.FunctionID, e.Address, e.Alias, e.VersionID, e.Version})
}

// ConfigUpdated is emitted when a CONFIG setting is set or deleted:
// {functionId, address, keys}, every CONFIG key the function holds after the
// change, sorted; never a value. Java and Rust replace the whole config map
// and list its keys; Go writes one key at a time and lists the same thing.
type ConfigUpdated struct {
	Metadata   usecase.EventMetadata
	FunctionID string
	Address    string
	Keys       []string
}

func (e ConfigUpdated) EventID() string       { return e.Metadata.EventID }
func (e ConfigUpdated) EventType() string     { return ConfigUpdatedType }
func (e ConfigUpdated) SpecVersion() string   { return "1.0" }
func (e ConfigUpdated) Source() string        { return Source }
func (e ConfigUpdated) Subject() string       { return subjectFor(e.FunctionID) }
func (e ConfigUpdated) Time() time.Time       { return e.Metadata.OccurredAt }
func (e ConfigUpdated) PrincipalID() string   { return e.Metadata.PrincipalID }
func (e ConfigUpdated) CorrelationID() string { return e.Metadata.CorrelationID }
func (e ConfigUpdated) CausationID() string   { return e.Metadata.CausationID }
func (e ConfigUpdated) ExecutionID() string   { return e.Metadata.ExecutionID }
func (e ConfigUpdated) MessageGroup() string  { return groupFor(e.FunctionID) }
func (e ConfigUpdated) ToDataJSON() ([]byte, error) {
	return json.Marshal(struct {
		FunctionID string   `json:"functionId"`
		Address    string   `json:"address"`
		Keys       []string `json:"keys"`
	}{e.FunctionID, e.Address, nonNil(e.Keys)})
}

// SecretSet is emitted when a SECRET setting is written: {functionId,
// address, key}. The value is never in it.
type SecretSet struct {
	Metadata   usecase.EventMetadata
	FunctionID string
	Address    string
	Key        string
}

func (e SecretSet) EventID() string       { return e.Metadata.EventID }
func (e SecretSet) EventType() string     { return SecretSetType }
func (e SecretSet) SpecVersion() string   { return "1.0" }
func (e SecretSet) Source() string        { return Source }
func (e SecretSet) Subject() string       { return subjectFor(e.FunctionID) }
func (e SecretSet) Time() time.Time       { return e.Metadata.OccurredAt }
func (e SecretSet) PrincipalID() string   { return e.Metadata.PrincipalID }
func (e SecretSet) CorrelationID() string { return e.Metadata.CorrelationID }
func (e SecretSet) CausationID() string   { return e.Metadata.CausationID }
func (e SecretSet) ExecutionID() string   { return e.Metadata.ExecutionID }
func (e SecretSet) MessageGroup() string  { return groupFor(e.FunctionID) }
func (e SecretSet) ToDataJSON() ([]byte, error) {
	return json.Marshal(struct {
		FunctionID string `json:"functionId"`
		Address    string `json:"address"`
		Key        string `json:"key"`
	}{e.FunctionID, e.Address, e.Key})
}

// SecretDeleted is emitted when a SECRET setting is removed: {functionId,
// address, key}.
type SecretDeleted struct {
	Metadata   usecase.EventMetadata
	FunctionID string
	Address    string
	Key        string
}

func (e SecretDeleted) EventID() string       { return e.Metadata.EventID }
func (e SecretDeleted) EventType() string     { return SecretDeletedType }
func (e SecretDeleted) SpecVersion() string   { return "1.0" }
func (e SecretDeleted) Source() string        { return Source }
func (e SecretDeleted) Subject() string       { return subjectFor(e.FunctionID) }
func (e SecretDeleted) Time() time.Time       { return e.Metadata.OccurredAt }
func (e SecretDeleted) PrincipalID() string   { return e.Metadata.PrincipalID }
func (e SecretDeleted) CorrelationID() string { return e.Metadata.CorrelationID }
func (e SecretDeleted) CausationID() string   { return e.Metadata.CausationID }
func (e SecretDeleted) ExecutionID() string   { return e.Metadata.ExecutionID }
func (e SecretDeleted) MessageGroup() string  { return groupFor(e.FunctionID) }
func (e SecretDeleted) ToDataJSON() ([]byte, error) {
	return json.Marshal(struct {
		FunctionID string `json:"functionId"`
		Address    string `json:"address"`
		Key        string `json:"key"`
	}{e.FunctionID, e.Address, e.Key})
}

// FunctionPromoted is emitted when the `live` alias moves (promote) or is
// removed (unwire), carrying the wiring reconciliation summary (WP8, plan
// §8.5). Go-only: the alias move itself is [AliasChanged] / [AliasRemoved],
// emitted first.
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
func (e FunctionPromoted) Source() string        { return AdminSource }
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
func (e FunctionScheduleWired) Source() string        { return AdminSource }
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
func (e RouteSet) Source() string        { return AdminSource }
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
func (e RouteDeleted) Source() string        { return AdminSource }
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

// DBSettingSet is emitted when a DB setting (a database connection, keyed by
// its name) is written. Go-only (Java and Rust have no DB kind), so it keeps
// the function-setting code and data it had for every kind: {functionId,
// kind: "DB", key}. The value is never in it.
type DBSettingSet struct {
	Metadata   usecase.EventMetadata
	FunctionID string
	Key        string
}

func (e DBSettingSet) EventID() string       { return e.Metadata.EventID }
func (e DBSettingSet) EventType() string     { return DBSettingSetType }
func (e DBSettingSet) SpecVersion() string   { return "1.0" }
func (e DBSettingSet) Source() string        { return AdminSource }
func (e DBSettingSet) Subject() string       { return subjectFor(e.FunctionID) }
func (e DBSettingSet) Time() time.Time       { return e.Metadata.OccurredAt }
func (e DBSettingSet) PrincipalID() string   { return e.Metadata.PrincipalID }
func (e DBSettingSet) CorrelationID() string { return e.Metadata.CorrelationID }
func (e DBSettingSet) CausationID() string   { return e.Metadata.CausationID }
func (e DBSettingSet) ExecutionID() string   { return e.Metadata.ExecutionID }
func (e DBSettingSet) MessageGroup() string  { return groupFor(e.FunctionID) }
func (e DBSettingSet) ToDataJSON() ([]byte, error) {
	return json.Marshal(struct {
		FunctionID string `json:"functionId"`
		Kind       string `json:"kind"`
		Key        string `json:"key"`
	}{e.FunctionID, string(function.SettingDB), e.Key})
}

// DBSettingDeleted is emitted when a DB setting is removed: {functionId,
// kind: "DB", key}. Go-only, like [DBSettingSet].
type DBSettingDeleted struct {
	Metadata   usecase.EventMetadata
	FunctionID string
	Key        string
}

func (e DBSettingDeleted) EventID() string       { return e.Metadata.EventID }
func (e DBSettingDeleted) EventType() string     { return DBSettingDeletedType }
func (e DBSettingDeleted) SpecVersion() string   { return "1.0" }
func (e DBSettingDeleted) Source() string        { return AdminSource }
func (e DBSettingDeleted) Subject() string       { return subjectFor(e.FunctionID) }
func (e DBSettingDeleted) Time() time.Time       { return e.Metadata.OccurredAt }
func (e DBSettingDeleted) PrincipalID() string   { return e.Metadata.PrincipalID }
func (e DBSettingDeleted) CorrelationID() string { return e.Metadata.CorrelationID }
func (e DBSettingDeleted) CausationID() string   { return e.Metadata.CausationID }
func (e DBSettingDeleted) ExecutionID() string   { return e.Metadata.ExecutionID }
func (e DBSettingDeleted) MessageGroup() string  { return groupFor(e.FunctionID) }
func (e DBSettingDeleted) ToDataJSON() ([]byte, error) {
	return json.Marshal(struct {
		FunctionID string `json:"functionId"`
		Kind       string `json:"kind"`
		Key        string `json:"key"`
	}{e.FunctionID, string(function.SettingDB), e.Key})
}

// nonNil sends an empty list as [], never null.
func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// TraceEnabled is emitted when a trace context is switched on:
// {functionId, address, context, enabledUntil, indefinite, sampleRate}.
// indefinite is always present: true for a window switched on "until switched
// off", whose enabledUntil is then its hard ceiling.
type TraceEnabled struct {
	Metadata     usecase.EventMetadata
	FunctionID   string
	Address      string
	Context      string
	EnabledUntil time.Time
	Indefinite   bool
	SampleRate   int
}

func (e TraceEnabled) EventID() string       { return e.Metadata.EventID }
func (e TraceEnabled) EventType() string     { return TraceEnabledType }
func (e TraceEnabled) SpecVersion() string   { return "1.0" }
func (e TraceEnabled) Source() string        { return Source }
func (e TraceEnabled) Subject() string       { return subjectFor(e.FunctionID) }
func (e TraceEnabled) Time() time.Time       { return e.Metadata.OccurredAt }
func (e TraceEnabled) PrincipalID() string   { return e.Metadata.PrincipalID }
func (e TraceEnabled) CorrelationID() string { return e.Metadata.CorrelationID }
func (e TraceEnabled) CausationID() string   { return e.Metadata.CausationID }
func (e TraceEnabled) ExecutionID() string   { return e.Metadata.ExecutionID }
func (e TraceEnabled) MessageGroup() string  { return groupFor(e.FunctionID) }
func (e TraceEnabled) ToDataJSON() ([]byte, error) {
	return json.Marshal(struct {
		FunctionID   string    `json:"functionId"`
		Address      string    `json:"address"`
		Context      string    `json:"context"`
		EnabledUntil time.Time `json:"enabledUntil"`
		Indefinite   bool      `json:"indefinite"`
		SampleRate   int       `json:"sampleRate"`
	}{e.FunctionID, e.Address, e.Context, e.EnabledUntil.UTC(), e.Indefinite, e.SampleRate})
}

// TraceDisabled is emitted when a trace context is switched off:
// {functionId, address, context}.
type TraceDisabled struct {
	Metadata   usecase.EventMetadata
	FunctionID string
	Address    string
	Context    string
}

func (e TraceDisabled) EventID() string       { return e.Metadata.EventID }
func (e TraceDisabled) EventType() string     { return TraceDisabledType }
func (e TraceDisabled) SpecVersion() string   { return "1.0" }
func (e TraceDisabled) Source() string        { return Source }
func (e TraceDisabled) Subject() string       { return subjectFor(e.FunctionID) }
func (e TraceDisabled) Time() time.Time       { return e.Metadata.OccurredAt }
func (e TraceDisabled) PrincipalID() string   { return e.Metadata.PrincipalID }
func (e TraceDisabled) CorrelationID() string { return e.Metadata.CorrelationID }
func (e TraceDisabled) CausationID() string   { return e.Metadata.CausationID }
func (e TraceDisabled) ExecutionID() string   { return e.Metadata.ExecutionID }
func (e TraceDisabled) MessageGroup() string  { return groupFor(e.FunctionID) }
func (e TraceDisabled) ToDataJSON() ([]byte, error) {
	return json.Marshal(struct {
		FunctionID string `json:"functionId"`
		Address    string `json:"address"`
		Context    string `json:"context"`
	}{e.FunctionID, e.Address, e.Context})
}
