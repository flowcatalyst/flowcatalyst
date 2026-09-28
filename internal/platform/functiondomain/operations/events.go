package operations

import (
	"encoding/json"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
)

const (
	DomainClaimedType  = "platform:admin:function-domain:claimed"
	DomainReleasedType = "platform:admin:function-domain:released"
	Source             = "platform:admin"
)

func subjectFor(id string) string { return "platform.function-domain." + id }
func groupFor(id string) string   { return "platform:function-domain:" + id }

// DomainClaimed is emitted when a zone is successfully claimed.
type DomainClaimed struct {
	Metadata usecase.EventMetadata
	DomainID string
	Zone     string
	ClientID *string
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
		DomainID string  `json:"domainId"`
		Zone     string  `json:"zone"`
		ClientID *string `json:"clientId,omitempty"`
	}{e.DomainID, e.Zone, e.ClientID})
}

// DomainReleased is emitted when a claim is deleted.
type DomainReleased struct {
	Metadata usecase.EventMetadata
	DomainID string
	Zone     string
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
		Zone     string `json:"zone"`
	}{e.DomainID, e.Zone})
}
