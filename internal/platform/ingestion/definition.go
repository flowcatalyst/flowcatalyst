package ingestion

import (
	"bytes"
	"encoding/json"
)

// Definition is the editable part of an ingestion, and what one version
// snapshots: everything but the identity (application, client, code) and the
// paused flag.
type Definition struct {
	Name string `json:"name"`
	// KeepFailedPayloads lets a failure keep the first 2 KB of the payload it
	// failed on. Off unless the client has agreed.
	KeepFailedPayloads bool   `json:"keepFailedPayloads"`
	Source             Source `json:"source"`
	Split              Split  `json:"split"`
	Target             Target `json:"target"`
}

// UnmarshalJSON reads a definition strictly: an unknown field, anywhere in it,
// is an error.
func (d *Definition) UnmarshalJSON(data []byte) error {
	var aux struct {
		Name               string          `json:"name"`
		KeepFailedPayloads bool            `json:"keepFailedPayloads"`
		Source             json.RawMessage `json:"source"`
		Split              json.RawMessage `json:"split"`
		Target             Target          `json:"target"`
	}
	if err := strictDecode(data, &aux); err != nil {
		return err
	}
	*d = Definition{Name: aux.Name, KeepFailedPayloads: aux.KeepFailedPayloads, Target: aux.Target}
	var err error
	if aux.Source != nil && string(aux.Source) != "null" {
		if d.Source, err = UnmarshalSource(aux.Source); err != nil {
			return err
		}
	}
	if aux.Split != nil && string(aux.Split) != "null" {
		if d.Split, err = UnmarshalSplit(aux.Split); err != nil {
			return err
		}
	}
	return nil
}

// ParseDefinition reads the JSON form of a definition, as the API takes it.
func ParseDefinition(data []byte) (Definition, error) {
	var d Definition
	err := json.Unmarshal(data, &d)
	return d, err
}

// Equal reports whether two definitions say the same thing; an update that
// changes nothing writes nothing.
func (d Definition) Equal(other Definition) bool {
	a, errA := json.Marshal(d)
	b, errB := json.Marshal(other)
	return errA == nil && errB == nil && bytes.Equal(a, b)
}
