// Package fieldidentity is the field-worker identity plane
// (docs/field-worker-identity-design.md, docs/field-worker-identity-p1-spec.md):
// workers identified by (client, worker code) with a PIN, signing in from
// enrolled devices that hold a key. It is a SEPARATE population from
// iam_principals and portal_identities.
//
// This package holds the entities, storage, domain operations and events.
// The admin HTTP surface is in the api subpackage; the device-facing /field/*
// protocol belongs to the fieldauth package.
package fieldidentity

import (
	"slices"
	"strings"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/tsid"
)

// Status is the state of a worker. Only ACTIVE may sign in.
type Status string

const (
	StatusActive   Status = "ACTIVE"
	StatusDisabled Status = "DISABLED"
)

// DeviceStatus is the state of an enrolled device.
type DeviceStatus string

const (
	DeviceActive    DeviceStatus = "ACTIVE"
	DeviceSuspended DeviceStatus = "SUSPENDED"
	// DeviceRevoked is terminal.
	DeviceRevoked DeviceStatus = "REVOKED"
)

// Mode is how a device is used.
type Mode string

const (
	// ModePersonal is one worker's own device; it may be bound to that worker.
	ModePersonal Mode = "PERSONAL"
	// ModeShared is a pool device used by many workers in turn.
	ModeShared Mode = "SHARED"
)

// Valid reports whether m is a known mode.
func (m Mode) Valid() bool { return m == ModePersonal || m == ModeShared }

// Grant sources.
const (
	SourceAdmin = "ADMIN"
	SourceApp   = "APP"
)

// Revocation reasons stored in field_sessions.revoked_reason.
const (
	ReasonRotated         = "ROTATED"
	ReasonReuse           = "REUSE"
	ReasonLogout          = "LOGOUT"
	ReasonDeviceRevoked   = "DEVICE_REVOKED"
	ReasonWorkerSuspended = "WORKER_SUSPENDED"
	ReasonAdmin           = "ADMIN"
	ReasonPinChanged      = "PIN_CHANGED"
)

// NormalizeWorkerCode upper-cases and trims a worker code (the
// (client, worker_code) key is stored normalised).
func NormalizeWorkerCode(code string) string { return strings.ToUpper(strings.TrimSpace(code)) }

// AppGrant is one worker's right to sign in to one application.
type AppGrant struct {
	AppID     string    `json:"appId"`
	Source    string    `json:"source"`
	GrantedAt time.Time `json:"grantedAt"`
}

// sessionRevoke is a pending bulk session revocation applied by Persist in the
// same transaction as the aggregate change.
type sessionRevoke struct {
	reason string
	appID  string // empty = every session of the worker
}

// Identity is one field worker in one client.
type Identity struct {
	ID         string `json:"id"`
	ClientID   string `json:"clientId"`
	WorkerCode string `json:"workerCode"`
	Name       string `json:"name"`
	Email      string `json:"email,omitempty"`
	Status     Status `json:"status"`

	// PINHash is nil until a PIN is set; never serialised.
	PINHash            *string    `json:"-"`
	PINScheme          string     `json:"pinScheme"`
	PepperVersion      int        `json:"-"`
	PINSetAt           *time.Time `json:"pinSetAt,omitempty"`
	PINChangeRequired  bool       `json:"pinChangeRequired"`
	FailedSinceSuccess int        `json:"failedSinceSuccess"`
	LockedUntil        *time.Time `json:"lockedUntil,omitempty"`
	HardLocked         bool       `json:"hardLocked"`
	LastLoginAt        *time.Time `json:"lastLoginAt,omitempty"`

	Apps      []AppGrant `json:"apps"`
	CreatedAt time.Time  `json:"createdAt"`
	UpdatedAt time.Time  `json:"updatedAt"`

	fresh    bool // constructed by NewIdentity, not yet persisted
	revoked  []string
	sessions []sessionRevoke
}

func (i *Identity) isNew() bool { return i.fresh }

// IDStr satisfies usecase.HasID.
func (i Identity) IDStr() string { return i.ID }

// NewIdentity constructs an ACTIVE worker with no PIN.
func NewIdentity(clientID, workerCode, name string) *Identity {
	now := time.Now().UTC()
	return &Identity{
		ID:            tsid.Generate(tsid.FieldWorker),
		ClientID:      clientID,
		WorkerCode:    NormalizeWorkerCode(workerCode),
		Name:          strings.TrimSpace(name),
		Status:        StatusActive,
		PINScheme:     "PEPPER_V1",
		PepperVersion: 1,
		Apps:          []AppGrant{},
		CreatedAt:     now,
		UpdatedAt:     now,
		fresh:         true,
	}
}

// HasPIN reports whether a PIN has been set.
func (i *Identity) HasPIN() bool { return i.PINHash != nil && *i.PINHash != "" }

// HasApp reports whether the worker is granted the application.
func (i *Identity) HasApp(appID string) bool {
	return slices.ContainsFunc(i.Apps, func(g AppGrant) bool { return g.AppID == appID })
}

// Grant adds the application grant, reporting whether it was new.
func (i *Identity) Grant(appID, source string) bool {
	if appID == "" || i.HasApp(appID) {
		return false
	}
	i.revoked = slices.DeleteFunc(i.revoked, func(id string) bool { return id == appID })
	i.Apps = append(i.Apps, AppGrant{AppID: appID, Source: source, GrantedAt: time.Now().UTC()})
	return true
}

// Revoke removes the application grant, reporting whether one existed. Persist
// deletes exactly that grant row.
func (i *Identity) Revoke(appID string) bool {
	before := len(i.Apps)
	i.Apps = slices.DeleteFunc(i.Apps, func(g AppGrant) bool { return g.AppID == appID })
	if len(i.Apps) == before {
		return false
	}
	i.revoked = append(i.revoked, appID)
	return true
}

// SetPIN stores a freshly hashed PIN: native scheme, change required when the
// PIN was chosen by someone other than the worker, lock state cleared.
func (i *Identity) SetPIN(hash string, changeRequired bool) {
	now := time.Now().UTC()
	i.PINHash = &hash
	i.PINScheme = "PEPPER_V1"
	i.PepperVersion = 1
	i.PINSetAt = &now
	i.PINChangeRequired = changeRequired
	i.ClearLock()
}

// ClearLock clears the soft and hard locks and the failure counter.
func (i *Identity) ClearLock() {
	i.FailedSinceSuccess = 0
	i.LockedUntil = nil
	i.HardLocked = false
}

// IsLocked reports whether the worker is hard locked or soft locked at now.
func (i *Identity) IsLocked(now time.Time) bool {
	return i.HardLocked || (i.LockedUntil != nil && i.LockedUntil.After(now))
}

// RevokeSessions queues a revocation of every session of the worker (or, with
// appID set, of that application's sessions) that Persist applies in the
// transaction.
func (i *Identity) RevokeSessions(reason, appID string) {
	i.sessions = append(i.sessions, sessionRevoke{reason: reason, appID: appID})
}

// SessionRevocations reports whether any session revocation is pending.
func (i *Identity) SessionRevocations() int { return len(i.sessions) }

// Device is one enrolled, key-bound device.
type Device struct {
	ID               string `json:"id"`
	ClientID         string `json:"clientId"`
	Mode             Mode   `json:"mode"`
	Label            string `json:"label,omitempty"`
	Platform         string `json:"platform,omitempty"`
	Model            string `json:"model,omitempty"`
	JKT              string `json:"jkt"`
	PublicKeyJWK     string `json:"-"`
	AttestationLevel string `json:"attestationLevel"`
	// AttestationChain is the raw chain reported at enrolment (unverified in
	// P1); write-only.
	AttestationChain   string       `json:"-"`
	AssignedIdentityID *string      `json:"assignedIdentityId,omitempty"`
	Status             DeviceStatus `json:"status"`
	EnrolledBy         string       `json:"enrolledBy,omitempty"`
	EnrolledAt         time.Time    `json:"enrolledAt"`
	LastSeenAt         *time.Time   `json:"lastSeenAt,omitempty"`
	UpdatedAt          time.Time    `json:"updatedAt"`
	AppIDs             []string     `json:"appIds"`

	sessions *sessionRevoke
}

// IDStr satisfies usecase.HasID.
func (d Device) IDStr() string { return d.ID }

// RevokeSessions queues revocation of every session on the device.
func (d *Device) RevokeSessions(reason string) { d.sessions = &sessionRevoke{reason: reason} }

// HasApp reports whether the device is enrolled for the application.
func (d *Device) HasApp(appID string) bool { return slices.Contains(d.AppIDs, appID) }

// EnrolmentCode is a single-use, short-lived code that turns a device into an
// enrolled device. Only its SHA-256 is stored.
type EnrolmentCode struct {
	ID                 string     `json:"id"`
	ClientID           string     `json:"clientId"`
	CodeHash           string     `json:"-"`
	Mode               Mode       `json:"mode"`
	AppIDs             []string   `json:"appIds"`
	AssignedIdentityID *string    `json:"assignedIdentityId,omitempty"`
	Label              string     `json:"label,omitempty"`
	ExpiresAt          time.Time  `json:"expiresAt"`
	UsedAt             *time.Time `json:"usedAt,omitempty"`
	CreatedBy          string     `json:"createdBy,omitempty"`
	CreatedAt          time.Time  `json:"createdAt"`
}

// IDStr satisfies usecase.HasID.
func (c EnrolmentCode) IDStr() string { return c.ID }

// Session is one issued refresh token (a family is one login).
type Session struct {
	ID                string
	FamilyID          string
	IdentityID        string
	DeviceID          string
	ApplicationID     string
	RefreshHash       string
	AMR               []string
	AuthTime          time.Time
	IssuedAt          time.Time
	IdleExpiresAt     time.Time
	AbsoluteExpiresAt time.Time
	RotatedAt         *time.Time
	RevokedAt         *time.Time
	RevokedReason     string
	PINChangeRequired bool
}

// Active reports whether the session is neither revoked nor expired at now.
func (s *Session) Active(now time.Time) bool {
	return s.RevokedAt == nil && now.Before(s.IdleExpiresAt) && now.Before(s.AbsoluteExpiresAt)
}

// Policy is a client's effective field policy. A client with no row gets
// [DefaultPolicy] (IsDefault true).
type Policy struct {
	ClientID           string    `json:"clientId"`
	PINMinLength       int       `json:"pinMinLength"`
	SoftLockThreshold  int       `json:"softLockThreshold"`
	SoftLockSecs       int       `json:"softLockSecs"`
	HardLockThreshold  int       `json:"hardLockThreshold"`
	SharedIdleSecs     int       `json:"sharedIdleSecs"`
	PersonalIdleSecs   int       `json:"personalIdleSecs"`
	ShiftMaxSecs       int       `json:"shiftMaxSecs"`
	RequireAttestation bool      `json:"requireAttestation"`
	PINReauthDays      int       `json:"pinReauthDays"`
	UpdatedAt          time.Time `json:"updatedAt"`
	IsDefault          bool      `json:"isDefault"`
}

// SharedShiftMaxSecs is the absolute session limit on a SHARED device (not a
// policy column).
const SharedShiftMaxSecs = 43200

// DefaultPolicy returns the defaults applied when a client has no policy row.
func DefaultPolicy(clientID string) Policy {
	return Policy{
		ClientID: clientID, PINMinLength: 6, SoftLockThreshold: 5, SoftLockSecs: 900,
		HardLockThreshold: 10, SharedIdleSecs: 900, PersonalIdleSecs: 7200,
		ShiftMaxSecs: 50400, PINReauthDays: 7, IsDefault: true,
	}
}

// IDStr satisfies usecase.HasID (keyed by client).
func (p Policy) IDStr() string { return p.ClientID }
