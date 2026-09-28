// Package function is the function-runner platform aggregate: the function
// itself (identity, ownership, pool/warm/limits settings) plus the child
// records a function owns — versions, aliases, and settings — and the
// runner-fleet bookkeeping (fn_runners, fn_pool_revisions). See
// docs/function-runner-plan.md §4 (model) and §8.1 (schema).
package function

import (
	"regexp"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/tsid"
)

// NamePattern is the function name rule (plan §4, owner decision §13.1):
// a lowercase letter followed by up to 62 lowercase alphanumerics/hyphens.
var NamePattern = regexp.MustCompile(`^[a-z][a-z0-9-]{0,62}$`)

// ValidName reports whether name matches NamePattern.
func ValidName(name string) bool { return NamePattern.MatchString(name) }

// BuildAddress composes the two-label function address (owner decision
// §13.1: {applicationCode}.{name}, no service label).
func BuildAddress(applicationCode, name string) string {
	return applicationCode + "." + name
}

// Limits are the per-function resource caps (plan §7.1). Stored as the
// fn_functions.limits JSONB column.
type Limits struct {
	MemoryMB       int32 `json:"memoryMb"`
	MaxConcurrency int32 `json:"maxConcurrency"`
	TimeoutMs      int32 `json:"timeoutMs"`
	MaxBodyBytes   int64 `json:"maxBodyBytes"`
}

// Default limits, plan §7.1.
const (
	DefaultMemoryMB       int32 = 64
	DefaultMaxConcurrency int32 = 16
	DefaultTimeoutMs      int32 = 30_000
	DefaultMaxBodyBytes   int64 = 1 << 20 // 1 MiB
)

// Ceilings a function's limits may not exceed. The plan names the defaults
// (§7.1) but leaves the per-function ceilings as "platform config" without
// naming values; these are package constants until platform config grows a
// dedicated knob (spec silence — see the WP3 report).
const (
	MaxMemoryMB       int32 = 1024 // 1 GiB
	MaxMaxConcurrency int32 = 256
	MaxTimeoutMs      int32 = 300_000  // 5 minutes
	MaxMaxBodyBytes   int64 = 16 << 20 // 16 MiB — matches the runner's http.fetch response cap (plan §6.3)
)

// DefaultLimits returns the limits a new function is created with.
func DefaultLimits() Limits {
	return Limits{
		MemoryMB:       DefaultMemoryMB,
		MaxConcurrency: DefaultMaxConcurrency,
		TimeoutMs:      DefaultTimeoutMs,
		MaxBodyBytes:   DefaultMaxBodyBytes,
	}
}

// Function is the aggregate root. Table: fn_functions.
type Function struct {
	ID            string
	ApplicationID string
	// ApplicationCode is read-side only: populated by the repository from a
	// join against app_applications, never persisted on this row and never
	// written back. See docs/function-runner-plan.md WP3 scope note.
	ApplicationCode string
	ClientID        *string
	Name            string
	Address         string
	Description     *string
	// Pool overrides the function's implied dispatch pool
	// (fn-{address}, materialised at promote — WP8). Nil means "use the
	// implied pool".
	Pool      *string
	Warm      bool
	Limits    Limits
	CreatedBy *string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// IDStr satisfies usecase.HasID.
func (f Function) IDStr() string { return f.ID }

// New constructs a Function with default limits. applicationCode is used
// only to compute Address and populate the read-side field — it is not
// persisted.
func New(applicationID, applicationCode, name string) *Function {
	now := time.Now().UTC()
	return &Function{
		ID:              tsid.Generate(tsid.Function),
		ApplicationID:   applicationID,
		ApplicationCode: applicationCode,
		Name:            name,
		Address:         BuildAddress(applicationCode, name),
		Warm:            false,
		Limits:          DefaultLimits(),
		CreatedAt:       now,
		UpdatedAt:       now,
	}
}

// VersionStatus is the lifecycle state of a function version.
type VersionStatus string

const (
	VersionPublished VersionStatus = "PUBLISHED"
	VersionReady     VersionStatus = "READY"
	VersionFailed    VersionStatus = "FAILED"
	VersionRetired   VersionStatus = "RETIRED"
)

// ParseVersionStatus parses a stored status value. Returns ok=false for
// anything other than the four known constants — callers MUST reject on
// ok=false rather than coerce (X-06: a loud read error, never a silent
// default). Follows the (T, bool) shape of dispatchpool.ParseStatus.
func ParseVersionStatus(s string) (VersionStatus, bool) {
	switch VersionStatus(s) {
	case VersionPublished, VersionReady, VersionFailed, VersionRetired:
		return VersionStatus(s), true
	default:
		return "", false
	}
}

// Version is an immutable published artifact. Table: fn_versions.
type Version struct {
	ID          string
	FunctionID  string
	Number      int32
	Digest      string
	SizeBytes   int64
	ABI         int32
	Describe    []byte // raw describe JSON, plan §5.4
	Status      VersionStatus
	Failure     []byte // raw failure JSON, nil unless Status == VersionFailed
	ReadyAt     *time.Time
	PublishedBy *string
	CreatedAt   time.Time
}

// IDStr satisfies usecase.HasID.
func (v Version) IDStr() string { return v.ID }

// Alias is a named pointer to a version. Table: fn_aliases, PK
// (function_id, name).
type Alias struct {
	FunctionID string
	Name       string
	VersionID  string
	UpdatedAt  time.Time
	UpdatedBy  *string
}

// SettingKind is the kind of a function setting value.
type SettingKind string

const (
	SettingConfig SettingKind = "CONFIG"
	SettingSecret SettingKind = "SECRET"
	SettingDB     SettingKind = "DB"
)

// ParseSettingKind parses a stored kind value. Returns ok=false for
// anything other than the three known constants (X-06).
func ParseSettingKind(s string) (SettingKind, bool) {
	switch SettingKind(s) {
	case SettingConfig, SettingSecret, SettingDB:
		return SettingKind(s), true
	default:
		return "", false
	}
}

// Setting is one platform-held config/secret/DB value. Table: fn_settings,
// PK (function_id, kind, key). SECRET and DB values are encrypted (or an
// external secret-manager reference) at rest — see repository.go.
type Setting struct {
	FunctionID string
	Kind       SettingKind
	Key        string
	Value      string
	UpdatedAt  time.Time
}
