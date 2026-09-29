package portalidentity

import (
	"github.com/flowcatalyst/flowcatalyst-go/internal/ids"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
)

type Identity struct{ ID string }

// SetStatusCommand is a request shape: its ID is whatever the caller sent.
type SetStatusCommand struct{ ID string }

func fine(cmd SetStatusCommand, p principal.Principal, raw string, cid ids.ClientID) {
	_ = ids.PrincipalID(cmd.ID)       // request shape: allowed
	_ = ids.PrincipalID(raw)          // a plain string: allowed here
	_ = ids.PrincipalID(string(p.ID)) // its own entity's id: allowed
}

func wrong(i Identity, cid ids.ClientID, p principal.Principal) {
	_ = ids.PrincipalID(i.ID)   // want `ids.PrincipalID built from portalidentity.Identity.ID: that is not a principal.Principal`
	_ = ids.PrincipalID(cid)    // want `converting ids.ClientID to ids.PrincipalID`
	_ = ids.ApplicationID(p.ID) // want `converting ids.PrincipalID to ids.ApplicationID`
	_ = ids.PrincipalID(i.ID)   //idconv:ok ptu_ subject travels as a principal id in this legacy path
	//idconv:ok waived on the line above
	_ = ids.ClientID(i.ID)
	_ = ids.ClientID(i.ID) // want `ids.ClientID built from`
}
