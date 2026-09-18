package sqs

import "testing"

// T14 (docs/spec/router-deferral-handback.md, R5): SQS answers true —
// Nack's ChangeMessageVisibility call (R3) genuinely holds the message back
// for its delay. Needs no AWS call: HonoursDelayedReturn touches no field.
func TestHonoursDelayedReturnIsTrue(t *testing.T) {
	q := &Queue{}
	if !q.HonoursDelayedReturn() {
		t.Fatal("SQS must answer true — see queue.Consumer.HonoursDelayedReturn's doc comment")
	}
}
