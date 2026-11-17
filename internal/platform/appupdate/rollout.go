package appupdate

import (
	"crypto/sha256"
	"encoding/binary"
	"strings"

	"github.com/google/uuid"
)

// NormalizeClientID returns the installation id a phone sends as EAS-Client-ID
// in canonical lower-case form (iOS sends upper case, Android lower), or "" when
// it is not a UUID.
func NormalizeClientID(raw string) string {
	raw = strings.TrimSpace(raw)
	if len(raw) != 36 {
		return ""
	}
	u, err := uuid.Parse(raw)
	if err != nil {
		return ""
	}
	return u.String()
}

// Bucket places an installation in [0, 100) for one update: the first eight
// bytes of SHA-256("<updateID>:<clientID>") modulo 100. Keyed by the update, so
// each rollout samples different phones; fixed for a phone and an update, so
// its answer does not flap and raising a percentage keeps every phone that was in.
func Bucket(updateID, clientID string) int {
	sum := sha256.Sum256([]byte(updateID + ":" + clientID))
	return int(binary.BigEndian.Uint64(sum[:8]) % 100)
}

// InRollout reports whether the installation clientID (normalised; "" when the
// phone sent none) receives update u. A full rollout reaches every phone, a
// phone without an id included; a partial one only phones whose bucket is
// below the percentage.
func InRollout(u *Update, clientID string) bool {
	if u.RolloutPercent >= 100 {
		return true
	}
	if clientID == "" || u.RolloutPercent <= 0 {
		return false
	}
	return Bucket(u.ID, clientID) < u.RolloutPercent
}

// Select is the answer for one installation, given the branch's newest rows,
// newest first (at most two are needed: only the newest row of a branch may be
// partially rolled out). nil means no update is available.
//
// The newest row answers when the phone is in its rollout (a rollback row is
// always at 100%); otherwise the row before it does, whatever its percentage
// (it was complete when the newest was published).
func Select(rows []Update, clientID string) *Update {
	if len(rows) == 0 {
		return nil
	}
	if InRollout(&rows[0], clientID) {
		return &rows[0]
	}
	if len(rows) > 1 {
		return &rows[1]
	}
	return nil
}

// ServedPercent is the share of phones on a branch that row i of rows (newest
// first) is served to: the newest its rollout, the one before it the rest
// while the newest is partial, any other none.
func ServedPercent(rows []Update, i int) int {
	switch {
	case i == 0 && len(rows) > 0:
		return rows[0].RolloutPercent
	case i == 1 && len(rows) > 1 && rows[0].RolloutPercent < 100:
		return 100 - rows[0].RolloutPercent
	}
	return 0
}
