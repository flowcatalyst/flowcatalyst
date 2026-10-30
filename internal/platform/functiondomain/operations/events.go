package operations

import (
	"encoding/json"
	"time"

	"github.com/flowcatalyst/go-sdk/usecase"
)

// The domain events are Java's and Rust's (docs/platform-bug-fixes.md F11):
// their codes, source and data. Migration 074 renamed the catalogue rows and
// subscription bindings of Go's older platform:admin:function-domain:* codes.
const (
	DomainClaimedType  = "platform:function:domain:claimed"
	DomainReleasedType = "platform:function:domain:released"
	Source             = "platform:function"
)

// PlatformOwner is the owner of a platform (client-less) claim on the wire.
const PlatformOwner = "platform"

// OwnerOf is a claim's owner on the wire: the owning client's id, or
// PlatformOwner when it has none.
func OwnerOf(clientID *string) string {
	if clientID == nil {
		return PlatformOwner
	}
	return *clientID
}

func subjectFor(id string) string { return "platform.function-domain." + id }
func groupFor(id string) string   { return "platform:function-domain:" + id }

// DomainClaimed is emitted when a zone is successfully claimed: {domainId,
// hostname, owner}; hostname is the claimed zone.
type DomainClaimed struct {
	Metadata usecase.EventMetadata
	DomainID string
	Hostname string
	Owner    string
}

func (e DomainClaimed) EventID() string       { return e.Metadata.EventID }
func (e DomainClaimed) EventType() string     { return DomainClaimedType }
func (e DomainClaimed) SpecVersion() string   { return "1.0" }
func (e DomainClaimed) Source() string        { return Source }
func (e DomainClaimed) Subject() string       { return subjectFor(e.DomainID) }
func (e DomainClaimed) Time() time.Time       { return e.Metadata.OccurredAt }
func (e DomainClaimed) PrincipalID() string   { return e.Metadata.PrincipalID }
func (e DomainClaimed) CorrelationID() string { return e.Metadata.CorrelationID }
func (e DomainClaimed) CausationID() string   { return e.Metadata.CausationID }
func (e DomainClaimed) ExecutionID() string   { return e.Metadata.ExecutionID }
func (e DomainClaimed) MessageGroup() string  { return groupFor(e.DomainID) }
func (e DomainClaimed) ToDataJSON() ([]byte, error) {
	return json.Marshal(struct {
		DomainID string `json:"domainId"`
		Hostname string `json:"hostname"`
		Owner    string `json:"owner"`
	}{e.DomainID, e.Hostname, e.Owner})
}

// DomainReleased is emitted when a claim is deleted: {domainId, hostname,
// owner}, as [DomainClaimed].
type DomainReleased struct {
	Metadata usecase.EventMetadata
	DomainID string
	Hostname string
	Owner    string
}

func (e DomainReleased) EventID() string       { return e.Metadata.EventID }
func (e DomainReleased) EventType() string     { return DomainReleasedType }
func (e DomainReleased) SpecVersion() string   { return "1.0" }
func (e DomainReleased) Source() string        { return Source }
func (e DomainReleased) Subject() string       { return subjectFor(e.DomainID) }
func (e DomainReleased) Time() time.Time       { return e.Metadata.OccurredAt }
func (e DomainReleased) PrincipalID() string   { return e.Metadata.PrincipalID }
func (e DomainReleased) CorrelationID() string { return e.Metadata.CorrelationID }
func (e DomainReleased) CausationID() string   { return e.Metadata.CausationID }
func (e DomainReleased) ExecutionID() string   { return e.Metadata.ExecutionID }
func (e DomainReleased) MessageGroup() string  { return groupFor(e.DomainID) }
func (e DomainReleased) ToDataJSON() ([]byte, error) {
	return json.Marshal(struct {
		DomainID string `json:"domainId"`
		Hostname string `json:"hostname"`
		Owner    string `json:"owner"`
	}{e.DomainID, e.Hostname, e.Owner})
}
