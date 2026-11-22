package serviceaccount

import "github.com/flowcatalyst/flowcatalyst-go/internal/ids"

// Signer is what decides whose work a service account may sign: its id, its
// code (for a message) and the application it belongs to, if any.
// Repository.FindSigner reads it without the account's credentials.
type Signer struct {
	ID            string
	Code          string
	ApplicationID *ids.ApplicationID
}

// SigningIdentity is who work is done as when its delivery will be signed by
// a service account the doer named, through a connection, rather than one the
// platform chose: an application, and an account that is the doer's own
// beside the application's. A function is one (its application, and its own
// service account when it sets one, the webhook verifier of F35).
type SigningIdentity struct {
	// ApplicationID is the application the work is done as.
	ApplicationID string
	// OwnAccountID is an account that is the doer's own, "" for none.
	OwnAccountID string
}

// SigningProblem reports why work done as id may not be delivered signed by
// s, or "" when it may (owner's decision D4, 2026-11-22: a function's dispatch
// may name a connection only if the function may sign as the connection's
// account). The rule is the Java port's delivery-signing guard for a caller
// that is an application (an application signs only with its own accounts):
//
//   - s nil (no such account) is no problem: an account that does not exist
//     signs nothing, and the delivery is declined with that reason;
//   - id's own account may sign;
//   - an account that belongs to id's application may sign;
//   - an account that belongs to another application may not (the receiver
//     would verify the work as that application's);
//   - an account that belongs to no application may not, unless it is id's
//     own: a shared account is not the application's to sign with.
//
// Whether the account is active is not asked: an inactive account signs
// nothing, and the delivery says so.
func (id SigningIdentity) SigningProblem(s *Signer) string {
	if s == nil {
		return ""
	}
	if id.OwnAccountID != "" && s.ID == id.OwnAccountID {
		return ""
	}
	if s.ApplicationID != nil && string(*s.ApplicationID) != "" {
		if string(*s.ApplicationID) == id.ApplicationID {
			return ""
		}
		return "service account " + s.Code + " belongs to another application"
	}
	return "service account " + s.Code + " belongs to no application, and work of an application is signed only by the application's own accounts"
}
