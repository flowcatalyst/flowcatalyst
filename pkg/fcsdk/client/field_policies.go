package client

import (
	"context"
	"net/url"
)

// PutFieldPolicyRequest — PUT /api/field-policies/{clientId}. A nil member
// takes the platform default (shown per field).
type PutFieldPolicyRequest struct {
	PinMinLength       *int `json:"pinMinLength,omitempty"`      // 4 to 12 (6)
	SoftLockThreshold  *int `json:"softLockThreshold,omitempty"` // 1 to 20 (5)
	SoftLockSecs       *int `json:"softLockSecs,omitempty"`      // 30 to 86400 (900)
	HardLockThreshold  *int `json:"hardLockThreshold,omitempty"` // softLockThreshold to 50 (10)
	SharedIdleSecs     *int `json:"sharedIdleSecs,omitempty"`    // 60 to 86400 (900)
	PersonalIdleSecs   *int `json:"personalIdleSecs,omitempty"`  // 60 to 86400 (7200)
	ShiftMaxSecs       *int `json:"shiftMaxSecs,omitempty"`      // 3600 to 86400 (50400)
	RequireAttestation bool `json:"requireAttestation"`
	PinReauthDays      *int `json:"pinReauthDays,omitempty"` // 1 to 90 (7)
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
	// IsDefault is true when the client has no stored policy and these are
	// the platform defaults.
	IsDefault bool   `json:"isDefault"`
	UpdatedAt string `json:"updatedAt,omitempty"`
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
func (r *FieldPoliciesResource) Put(ctx context.Context, clientID string, req *PutFieldPolicyRequest) (*FieldPolicyResponse, error) {
	var out FieldPolicyResponse
	if err := r.c.Put(ctx, "/api/field-policies/"+url.PathEscape(clientID), req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
