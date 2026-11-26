package client

import (
	"context"
	"net/url"
)

// CreateEnrolmentCodeRequest — POST /api/field-devices/enrolment-codes.
type CreateEnrolmentCodeRequest struct {
	ClientID string `json:"clientId"`
	// Mode is PERSONAL or SHARED.
	Mode     string   `json:"mode"`
	AppCodes []string `json:"appCodes"`
	// AssignedWorkerID is for PERSONAL devices only.
	AssignedWorkerID *string `json:"assignedWorkerId,omitempty"`
	Label            *string `json:"label,omitempty"`
}

// FieldEnrolmentCodeResponse is a freshly minted single-use code.
type FieldEnrolmentCodeResponse struct {
	Code string `json:"code"`
	// QrPayload is the string to render as a QR code
	// (fcfield:1:<baseUrl>:<code>).
	QrPayload string `json:"qrPayload"`
	ExpiresAt string `json:"expiresAt"`
}

// FieldAppRef names an application.
type FieldAppRef struct {
	ID   string `json:"id"`
	Code string `json:"code"`
	Name string `json:"name"`
}

// FieldDeviceResponse is an enrolled field device.
type FieldDeviceResponse struct {
	ID       string `json:"id"`
	ClientID string `json:"clientId"`
	// Mode is PERSONAL or SHARED.
	Mode     string `json:"mode"`
	Label    string `json:"label,omitempty"`
	Platform string `json:"platform,omitempty"`
	Model    string `json:"model,omitempty"`
	// Jkt is the RFC 7638 thumbprint of the device key.
	Jkt string `json:"jkt"`
	// AttestationLevel is STRONGBOX, TEE, SOFTWARE or NONE.
	AttestationLevel string  `json:"attestationLevel"`
	AssignedWorkerID *string `json:"assignedWorkerId,omitempty"`
	// Status is ACTIVE, SUSPENDED or REVOKED.
	Status         string        `json:"status"`
	Apps           []FieldAppRef `json:"apps"`
	ActiveSessions int           `json:"activeSessions"`
	EnrolledAt     string        `json:"enrolledAt"`
	LastSeenAt     string        `json:"lastSeenAt,omitempty"`
	// UnlockKey is the device's live unlock key, nil when none.
	UnlockKey *FieldUnlockKey `json:"unlockKey"`
}

// FieldUnlockKey is a worker's unlock key on a device (biometric sign-in on a
// PERSONAL device; docs/field-unlock-keys-spec.md in the platform repository).
type FieldUnlockKey struct {
	ID       string `json:"id"`
	WorkerID string `json:"workerId"`
	DeviceID string `json:"deviceId"`
	// Status is ACTIVE or REVOKED.
	Status        string `json:"status"`
	CreatedAt     string `json:"createdAt"`
	LastUsedAt    string `json:"lastUsedAt,omitempty"`
	PinVerifiedAt string `json:"pinVerifiedAt"`
	PinRequiredBy string `json:"pinRequiredBy,omitempty"` // absent: no periodic PIN
	RevokedAt     string `json:"revokedAt,omitempty"`
	RevokedReason string `json:"revokedReason,omitempty"`
	RevokedBy     string `json:"revokedBy,omitempty"`
	// LiveSessions is how many sessions the key signed in are still live.
	LiveSessions int64 `json:"liveSessions"`
	// Attestation is what the platform parsed from the key's attestation.
	Attestation FieldUnlockKeyAttestation `json:"attestation"`
}

// FieldUnlockKeyAttestation is the parsed attestation of an unlock key.
type FieldUnlockKeyAttestation struct {
	// Format is ANDROID_KEY or NONE; Verdict NONE, MALFORMED, UNTRUSTED or TRUSTED.
	Format                   string `json:"format"`
	Verdict                  string `json:"verdict"`
	Problem                  string `json:"problem,omitempty"`
	SecurityLevel            string `json:"securityLevel,omitempty"`
	AttestationSecurityLevel string `json:"attestationSecurityLevel,omitempty"`
	AttestationVersion       int    `json:"attestationVersion,omitempty"`
	ChallengeMatches         bool   `json:"challengeMatches"`
	AuthPerUse               bool   `json:"authPerUse"`
	BiometricOnly            bool   `json:"biometricOnly"`
	SignP256                 bool   `json:"signP256"`
	RootFingerprint          string `json:"rootFingerprint,omitempty"`
	// Provisioning is FACTORY or REMOTE; Dates VALID or INVALID at
	// registration; Revocation what the attestation key status list said.
	Provisioning     string `json:"provisioning,omitempty"`
	Dates            string `json:"dates,omitempty"`
	DatesProblem     string `json:"datesProblem,omitempty"`
	Revocation       string `json:"revocation,omitempty"`
	CheckedAt        string `json:"checkedAt,omitempty"`
	MeetsRequirement bool   `json:"meetsRequirement"`
}

// FieldUnlockKeyListResponse — GET /api/field-workers/{id}/unlock-keys: the
// worker's unlock keys, newest first, live and revoked.
type FieldUnlockKeyListResponse struct {
	UnlockKeys []FieldUnlockKey `json:"unlockKeys"`
}

// FieldDeviceListResponse — GET /api/field-devices.
type FieldDeviceListResponse struct {
	Devices []FieldDeviceResponse `json:"devices"`
	Total   int64                 `json:"total"`
	Page    int                   `json:"page"`
	Size    int                   `json:"size"`
}

// FieldDeviceFilters — query parameters for GET /api/field-devices.
type FieldDeviceFilters struct {
	// Status is ACTIVE, SUSPENDED or REVOKED.
	Status string
	// Mode is PERSONAL or SHARED.
	Mode     string
	Page     *uint32
	PageSize *uint32
}

// FieldDevicesResource — /api/field-devices/*.
type FieldDevicesResource struct {
	c *FlowCatalystClient
}

// CreateEnrolmentCode — POST /api/field-devices/enrolment-codes.
func (r *FieldDevicesResource) CreateEnrolmentCode(ctx context.Context, req *CreateEnrolmentCodeRequest) (*FieldEnrolmentCodeResponse, error) {
	var out FieldEnrolmentCodeResponse
	if err := r.c.Post(ctx, "/api/field-devices/enrolment-codes", req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// List — GET /api/field-devices for one client, with optional filters.
func (r *FieldDevicesResource) List(ctx context.Context, clientID string, filters *FieldDeviceFilters) (*FieldDeviceListResponse, error) {
	q := NewQuery().String("clientId", clientID)
	if filters != nil {
		q = q.String("status", filters.Status).
			String("mode", filters.Mode).
			Uint32("page", filters.Page).
			Uint32("pageSize", filters.PageSize)
	}
	var out FieldDeviceListResponse
	if err := r.c.Get(ctx, "/api/field-devices"+q.Encode(), &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Get — GET /api/field-devices/{id}.
func (r *FieldDevicesResource) Get(ctx context.Context, id string) (*FieldDeviceResponse, error) {
	var out FieldDeviceResponse
	if err := r.c.Get(ctx, "/api/field-devices/"+url.PathEscape(id), &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Suspend — POST /api/field-devices/{id}/suspend. Revokes its sessions.
func (r *FieldDevicesResource) Suspend(ctx context.Context, id string) error {
	return r.c.Post(ctx, "/api/field-devices/"+url.PathEscape(id)+"/suspend", nil, nil)
}

// Activate — POST /api/field-devices/{id}/activate.
func (r *FieldDevicesResource) Activate(ctx context.Context, id string) error {
	return r.c.Post(ctx, "/api/field-devices/"+url.PathEscape(id)+"/activate", nil, nil)
}

// Revoke — POST /api/field-devices/{id}/revoke. Permanent; revokes its
// sessions.
func (r *FieldDevicesResource) Revoke(ctx context.Context, id string) error {
	return r.c.Post(ctx, "/api/field-devices/"+url.PathEscape(id)+"/revoke", nil, nil)
}
