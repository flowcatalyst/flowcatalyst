package postgres

import "testing"

// T14 (docs/spec/router-deferral-handback.md, R5): Postgres answers true —
// Nack's visible_at update genuinely holds a row back for its delay, and R4
// additionally blocks a delayed group head's successors from claiming ahead
// of it (see postgres_deferral_handback_test.go, build tag integration, for
// T9-T11). Needs no database: HonoursDelayedReturn touches neither q.pool
// nor q.cfg.
func TestHonoursDelayedReturnIsTrue(t *testing.T) {
	q := &Queue{}
	if !q.HonoursDelayedReturn() {
		t.Fatal("Postgres must answer true — see queue.Consumer.HonoursDelayedReturn's doc comment")
	}
}
