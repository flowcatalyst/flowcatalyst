package scheduler

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
)

// DispatchAuthService signs dispatch-job IDs with HMAC-SHA256 so the
// router's callback to /api/dispatch/process can prove it really
// originated from a job the scheduler queued. Same construction as
// fc-platform/src/scheduler/auth.rs.
type DispatchAuthService struct{ secret []byte }

// NewDispatchAuthService wires the service with the supplied secret.
func NewDispatchAuthService(secret string) *DispatchAuthService {
	return &DispatchAuthService{secret: []byte(secret)}
}

// Sign returns the hex-encoded HMAC-SHA256 of jobID.
func (s *DispatchAuthService) Sign(jobID string) string {
	mac := hmac.New(sha256.New, s.secret)
	mac.Write([]byte(jobID))
	return hex.EncodeToString(mac.Sum(nil))
}

// Verify checks a token against a job ID in constant time.
func (s *DispatchAuthService) Verify(jobID, token string) bool {
	expected := s.Sign(jobID)
	return subtle.ConstantTimeCompare([]byte(expected), []byte(token)) == 1
}
