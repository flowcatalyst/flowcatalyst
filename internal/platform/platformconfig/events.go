package platformconfig

import (
	"encoding/json"
	"time"

	"github.com/flowcatalyst/go-sdk/usecase"
)

// PropertyDeletedType is the event of the delete-property use case. It is
// declared here rather than beside the other platform-config events
// (operations/events.go) because the use case is: Service.Delete runs it, and
// the operations package imports this one.
const PropertyDeletedType = "platform:admin:platform-config:property-deleted"

// PropertyDeleted is emitted when a platform-config property is deleted.
type PropertyDeleted struct {
	Metadata        usecase.EventMetadata
	ConfigID        string
	ApplicationCode string
	Section         string
	Property        string
	ClientID        *string
}

func (e PropertyDeleted) EventID() string       { return e.Metadata.EventID }
func (e PropertyDeleted) EventType() string     { return PropertyDeletedType }
func (e PropertyDeleted) SpecVersion() string   { return "1.0" }
func (e PropertyDeleted) Source() string        { return "platform:admin" }
func (e PropertyDeleted) Subject() string       { return "platform.platformconfig." + e.ConfigID }
func (e PropertyDeleted) Time() time.Time       { return e.Metadata.OccurredAt }
func (e PropertyDeleted) PrincipalID() string   { return e.Metadata.PrincipalID }
func (e PropertyDeleted) CorrelationID() string { return e.Metadata.CorrelationID }
func (e PropertyDeleted) CausationID() string   { return e.Metadata.CausationID }
func (e PropertyDeleted) ExecutionID() string   { return e.Metadata.ExecutionID }
func (e PropertyDeleted) MessageGroup() string  { return "platform:platformconfig:" + e.ConfigID }
func (e PropertyDeleted) ToDataJSON() ([]byte, error) {
	return json.Marshal(struct {
		ConfigID        string  `json:"configId"`
		ApplicationCode string  `json:"applicationCode"`
		Section         string  `json:"section"`
		Property        string  `json:"property"`
		ClientID        *string `json:"clientId"`
	}{e.ConfigID, e.ApplicationCode, e.Section, e.Property, e.ClientID})
}
