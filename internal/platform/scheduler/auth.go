package scheduler

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
)

// DispatchAuthService signs dispatch-job IDs with HMAC-SHA256 so the
// router's callback to /api/dispatch/process can prove it really
// originated from a job the scheduler queued.
//
// Key rotation: Sign always uses the current secret; Verify accepts the
// current one or any previous one. A job signed before FLOWCATALYST_APP_KEY
// was rotated, still on the queue, keeps verifying while the old key is listed
// in FLOWCATALYST_APP_KEY_PREVIOUS, and so does one signed by a scheduler that
// has not been redeployed with the new key yet.
type DispatchAuthService struct {
	secret   []byte
	previous [][]byte
}

// NewDispatchAuthService wires the service with the supplied secret, and the
// secrets it replaced (accepted by Verify, never used to sign).
func NewDispatchAuthService(secret string, previous ...string) *DispatchAuthService {
	s := &DispatchAuthService{secret: []byte(secret)}
	for _, p := range previous {
		s.previous = append(s.previous, []byte(p))
	}
	return s
}

// Sign returns the hex-encoded HMAC-SHA256 of jobID under the current secret.
func (s *DispatchAuthService) Sign(jobID string) string {
	return sign(s.secret, jobID)
}

func sign(secret []byte, jobID string) string {
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(jobID))
	return hex.EncodeToString(mac.Sum(nil))
}

// Verify checks a token against a job ID in constant time, under the current
// secret and then each previous one. previousKey reports that only a previous
// secret matched — the token was signed before a key rotation (or by a
// process still running the old key).
func (s *DispatchAuthService) Verify(jobID, token string) (ok, previousKey bool) {
	if subtle.ConstantTimeCompare([]byte(sign(s.secret, jobID)), []byte(token)) == 1 {
		return true, false
	}
	for _, p := range s.previous {
		if subtle.ConstantTimeCompare([]byte(sign(p, jobID)), []byte(token)) == 1 {
			return true, true
		}
	}
	return false, false
}
