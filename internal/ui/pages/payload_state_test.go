package pages

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchjob"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/event"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/payloadstore"
)

// What the console shows of a payload it could not or did not read
// (docs/payload-offload-design.md, sections 7 and 11).
func TestPayloadStates(t *testing.T) {
	ref := "v1:k:0:10:zstd:00000000"

	// An event's drawer: the data, or a plain statement that it could not
	// be read.
	assert.Equal(t, formatData(json.RawMessage(`{"a":1}`)), eventData(&event.Event{Data: json.RawMessage(`{"a":1}`)}))
	assert.Equal(t, dataUnavailableText, eventData(&event.Event{DataUnavailable: true}))

	// A job's drawer likewise.
	p := `{"a":1}`
	assert.Equal(t, prettyText(&p), jobPayloadText(&dispatchjob.DispatchJob{Payload: &p}))
	assert.Contains(t, jobPayloadText(&dispatchjob.DispatchJob{PayloadUnavailable: true}), "Unavailable")

	// The raw dispatch job list and drawer: the size from the payload row, and
	// where the payload is when the page did not read it.
	assert.Equal(t, "7 bytes", dbgDjPayload(&dispatchjob.DispatchJob{Payload: &p}))
	assert.Equal(t, "7 bytes", dbgDjPayload(&dispatchjob.DispatchJob{Payload: &p, StoredPayload: &payloadstore.Stored{ID: "p", Data: &p, Size: 7}}))
	assert.Equal(t, "9000 bytes, in the payload store",
		dbgDjPayload(&dispatchjob.DispatchJob{StoredPayload: &payloadstore.Stored{ID: "p", Ref: &ref, Size: 9000}}))
	assert.Equal(t, "0 bytes", dbgDjPayload(&dispatchjob.DispatchJob{}))
}
