package client

import (
	"context"
	"net/url"
)

// PutFieldPolicyRequest — PUT /api/field-policies/{clientId}. A nil member
// takes the platform default (shown per field).
//
// A PUT REPLACES the policy: every member left out goes back to its default.
// Build the request from the stored policy, not from scratch:
// FieldPolicyResponse.ToRequest gives one that changes nothing, and
// FieldPoliciesResource.Update reads, lets you change what you mean to and
// writes. In particular a request that leaves UnlockKeysEnabled out switches
// unlock keys off for the client, revokes all its keys and signs those
// workers out.
type PutFieldPolicyRequest struct {
	PinMinLength       *int `json:"pinMinLength,omitempty"`      // 4 to 12 (6)
	SoftLockThreshold  *int `json:"softLockThreshold,omitempty"` // 1 to 20 (5)
	SoftLockSecs       *int `json:"softLockSecs,omitempty"`      // 30 to 86400 (900)
	HardLockThreshold  *int `json:"hardLockThreshold,omitempty"` // softLockThreshold to 50 (10)
	SharedIdleSecs     *int `json:"sharedIdleSecs,omitempty"`    // 60 to 86400 (900)
	PersonalIdleSecs   *int `json:"personalIdleSecs,omitempty"`  // 60 to 86400 (7200)
	ShiftMaxSecs       *int `json:"shiftMaxSecs,omitempty"`      // 3600 to 86400 (50400)
	RequireAttestation bool `json:"requireAttestation"`
	PinReauthDays      *int `json:"pinReauthDays,omitempty"` // 0 to 90 (0: no periodic PIN for unlock keys)
	// A PUT replaces the policy: the two flags are false when omitted.
	RequireEnrolledDevice bool `json:"requireEnrolledDevice,omitempty"`
	UnenrolledIdleSecs    *int `json:"unenrolledIdleSecs,omitempty"` // 60 to 86400 (900)
	BadgeOnlyLogin        bool `json:"badgeOnlyLogin,omitempty"`
	ImpersonationMaxSecs  *int `json:"impersonationMaxSecs,omitempty"` // 300 to 43200 (14400)
	// WorkerCodeDigits is the number of digits after the three letters of a
	// worker code the platform generates, 3 to 6 (3); codes generated
	// afterwards only.
	WorkerCodeDigits *int `json:"workerCodeDigits,omitempty"`
	// A PUT replaces the policy, so a field left out goes back to its default:
	// nil PhotosEnabled is true (field-worker photos on for the client) and
	// WorkerMaySetOwnPhoto false. A request built without reading the policy
	// first therefore switches an opted-in client's WorkerMaySetOwnPhoto off:
	// Get the policy and send every field back. Switching PhotosEnabled off
	// only hides the photos; the stored ones are kept (DeletePhotos deletes
	// them).
	PhotosEnabled        *bool `json:"photosEnabled,omitempty"`
	WorkerMaySetOwnPhoto bool  `json:"workerMaySetOwnPhoto,omitempty"`
	// UnlockKeysEnabled lets workers register unlock keys (biometric sign-in on
	// a PERSONAL device; default false; switching it off revokes the client's
	// keys). UnlockKeyAttestation is NONE (default, "" sends it) or REQUIRED.
	// Left out (false, ""), a PUT switches unlock keys off and revokes them.
	UnlockKeysEnabled    bool   `json:"unlockKeysEnabled,omitempty"`
	UnlockKeyAttestation string `json:"unlockKeyAttestation,omitempty"`
}

// FieldPolicyResponse is a client's effective field policy.
type FieldPolicyResponse struct {
	ClientID           string `json:"clientId"`
	PinMinLength       int    `json:"pinMinLength"`
	SoftLockThreshold  int    `json:"softLockThreshold"`
	SoftLockSecs       int    `json:"softLockSecs"`
	HardLockThreshold  int    `json:"hardLockThreshold"`
	SharedIdleSecs     int    `json:"sharedIdleSecs"`
	PersonalIdleSecs   int    `json:"personalIdleSecs"`
	ShiftMaxSecs       int    `json:"shiftMaxSecs"`
	RequireAttestation bool   `json:"requireAttestation"`
	PinReauthDays      int    `json:"pinReauthDays"`
	// Sign-in options (docs/field-worker-sign-in-options-spec.md).
	RequireEnrolledDevice bool `json:"requireEnrolledDevice"`
	UnenrolledIdleSecs    int  `json:"unenrolledIdleSecs"`
	BadgeOnlyLogin        bool `json:"badgeOnlyLogin"`
	ImpersonationMaxSecs  int  `json:"impersonationMaxSecs"`
	// WorkerCodeDigits: digits of a generated worker code.
	WorkerCodeDigits int `json:"workerCodeDigits"`
	// PhotosEnabled: field-worker photos for the client (default true);
	// WorkerMaySetOwnPhoto: a worker may set and remove their own from the
	// device (default false).
	PhotosEnabled        bool `json:"photosEnabled"`
	WorkerMaySetOwnPhoto bool `json:"workerMaySetOwnPhoto"`
	// UnlockKeysEnabled: workers may register unlock keys (default false);
	// UnlockKeyAttestation: NONE (recorded only, default) or REQUIRED.
	UnlockKeysEnabled    bool   `json:"unlockKeysEnabled"`
	UnlockKeyAttestation string `json:"unlockKeyAttestation"`
	// LockedMembers are the members the client does not let its applications
	// override (set by platform administrators; read-only here).
	LockedMembers []string `json:"lockedMembers"`
	// IsDefault is true when the client has no stored policy and these are
	// the platform defaults.
	IsDefault bool   `json:"isDefault"`
	UpdatedAt string `json:"updatedAt,omitempty"`
}

// ToRequest returns the PUT request that stores this policy again unchanged:
// every member is carried, so Put(ToRequest()) after Get changes nothing.
// Change the members you mean to and leave the rest.
func (p *FieldPolicyResponse) ToRequest() *PutFieldPolicyRequest {
	return &PutFieldPolicyRequest{
		PinMinLength:          new(p.PinMinLength),
		SoftLockThreshold:     new(p.SoftLockThreshold),
		SoftLockSecs:          new(p.SoftLockSecs),
		HardLockThreshold:     new(p.HardLockThreshold),
		SharedIdleSecs:        new(p.SharedIdleSecs),
		PersonalIdleSecs:      new(p.PersonalIdleSecs),
		ShiftMaxSecs:          new(p.ShiftMaxSecs),
		RequireAttestation:    p.RequireAttestation,
		PinReauthDays:         new(p.PinReauthDays),
		RequireEnrolledDevice: p.RequireEnrolledDevice,
		UnenrolledIdleSecs:    new(p.UnenrolledIdleSecs),
		BadgeOnlyLogin:        p.BadgeOnlyLogin,
		ImpersonationMaxSecs:  new(p.ImpersonationMaxSecs),
		WorkerCodeDigits:      new(p.WorkerCodeDigits),
		PhotosEnabled:         new(p.PhotosEnabled),
		WorkerMaySetOwnPhoto:  p.WorkerMaySetOwnPhoto,
		UnlockKeysEnabled:     p.UnlockKeysEnabled,
		UnlockKeyAttestation:  p.UnlockKeyAttestation,
	}
}

// DeleteClientPhotosResponse says how many photos DeletePhotos removed.
type DeleteClientPhotosResponse struct {
	Removed int `json:"removed"`
}

// FieldPoliciesResource — /api/field-policies/*.
type FieldPoliciesResource struct {
	c *FlowCatalystClient
}

// Get — GET /api/field-policies/{clientId}.
func (r *FieldPoliciesResource) Get(ctx context.Context, clientID string) (*FieldPolicyResponse, error) {
	var out FieldPolicyResponse
	if err := r.c.Get(ctx, "/api/field-policies/"+url.PathEscape(clientID), &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Put — PUT /api/field-policies/{clientId}. Returns the resulting policy.
//
// The PUT replaces the whole policy: a member of req left out goes back to its
// default, and leaving UnlockKeysEnabled out switches unlock keys off for the
// client, revokes its keys and signs those workers out. Prefer Update, or
// start from FieldPolicyResponse.ToRequest.
func (r *FieldPoliciesResource) Put(ctx context.Context, clientID string, req *PutFieldPolicyRequest) (*FieldPolicyResponse, error) {
	var out FieldPolicyResponse
	if err := r.c.Put(ctx, "/api/field-policies/"+url.PathEscape(clientID), req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Update reads the client's policy, lets change alter the request that would
// store it unchanged, and writes the result: a member change does not touch
// keeps its stored value, new members of the platform included. The read and
// the write are two calls, so a policy edited by someone else between them is
// overwritten with what was read. Returns the resulting policy.
func (r *FieldPoliciesResource) Update(ctx context.Context, clientID string, change func(req *PutFieldPolicyRequest)) (*FieldPolicyResponse, error) {
	cur, err := r.Get(ctx, clientID)
	if err != nil {
		return nil, err
	}
	req := cur.ToRequest()
	if change != nil {
		change(req)
	}
	return r.Put(ctx, clientID, req)
}

// DeletePhotos — DELETE /api/field-policies/{clientId}/photos. Deletes every
// stored photo of the client's field workers in one transaction, whatever the
// client's PhotosEnabled says (switching photos off only hides them). Needs
// platform:iam:field-worker:manage for the client; an application-confined
// service account is refused (403 APPLICATION_FORBIDDEN): it removes its own
// workers' photos one at a time (FieldWorkers().RemovePhoto). It cannot be
// undone; repeating it removes 0.
func (r *FieldPoliciesResource) DeletePhotos(ctx context.Context, clientID string) (*DeleteClientPhotosResponse, error) {
	var out DeleteClientPhotosResponse
	if err := r.c.Delete(ctx, "/api/field-policies/"+url.PathEscape(clientID)+"/photos", &out); err != nil {
		return nil, err
	}
	return &out, nil
}
