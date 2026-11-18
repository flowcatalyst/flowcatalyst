package main

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/flowcatalyst/flowcatalyst-go/internal/outbox"
	outboxpg "github.com/flowcatalyst/flowcatalyst-go/internal/outbox/postgres"
)

func TestRedactDBURL(t *testing.T) {
	t.Parallel()
	for in, want := range map[string]string{
		"postgres://app:s3cret@db.internal:5432/app?sslmode=require":  "postgres://app:xxxxx@db.internal:5432/app?sslmode=require",
		"postgresql://app@db.internal/app":                            "postgresql://app@db.internal/app",
		"postgres://db.internal/app?user=app&password=s3cret":         "postgres://db.internal/app?password=xxxxx&user=app",
		"postgres://app:p%zz@db.internal/app":                         "postgres://app:xxxxx@db.internal/app",
		"host=db.internal user=app password=s3cret dbname=app":        "host=db.internal user=app password=xxxxx dbname=app",
		"host=db.internal password = 'se cr\\'et' dbname=app":         "host=db.internal password = xxxxx dbname=app",
		"postgres://app:pa@ss@db.internal/app":                        "postgres://app:xxxxx@db.internal/app",
		"postgres://app:s3cret@db.internal/app?options=-c%20x%3Dy":    "postgres://app:xxxxx@db.internal/app?options=-c%20x%3Dy",
		"postgres://app:s3cret@/app?host=/var/run/postgresql":         "postgres://app:xxxxx@/app?host=/var/run/postgresql",
		"postgres:///app?host=/tmp&user=app&password=s3cret&sslmode=": "postgres:///app?host=/tmp&user=app&password=xxxxx",
	} {
		got := redactDBURL(in)
		assert.Equal(t, want, got, in)
		assert.NotContains(t, got, "s3cret", in)
	}
}

func TestOutboxHealthVerdicts(t *testing.T) {
	t.Parallel()
	now := time.Now()
	stall := 10 * time.Second
	ago := func(d time.Duration) time.Time { return now.Add(-d) }
	base := func() healthInput {
		return healthInput{
			now: now, stall: stall, requireToken: true, phase: phaseRunning, phaseSince: ago(time.Hour),
			outbox: "app.public.outbox_messages",
		}
	}
	leading := &outboxpg.LeaderStatus{Role: outboxpg.RoleLeader, LastContact: ago(time.Second)}
	polling := outbox.Activity{LastIteration: ago(time.Second), LastClaimOK: ago(time.Second)}

	for _, c := range []struct {
		name    string
		in      func(*healthInput)
		healthy bool
		reason  string
	}{
		{"leader polling", func(in *healthInput) { in.leader, in.act = leading, polling }, true, ""},
		{"leadership off, polling", func(in *healthInput) { in.act = polling }, true, ""},
		{"standby reaching the database", func(in *healthInput) {
			in.leader = &outboxpg.LeaderStatus{Role: outboxpg.RoleStandby, LastContact: ago(time.Second)}
		}, true, ""},
		{"taking over", func(in *healthInput) {
			in.leader = &outboxpg.LeaderStatus{Role: outboxpg.RoleTakingOver, LastContact: ago(time.Second)}
		}, true, ""},
		{"standby, database unreachable", func(in *healthInput) {
			in.leader = &outboxpg.LeaderStatus{Role: outboxpg.RoleConnecting, LastContact: ago(time.Minute), LastError: "connection refused"}
		}, false, "cannot reach the source database: connection refused"},
		{"hung leader", func(in *healthInput) {
			in.leader = leading
			in.act = outbox.Activity{LastIteration: ago(time.Minute), LastClaimOK: ago(time.Minute)}
		}, false, "the poll loop has not completed a cycle"},
		{"hung loop, leadership off", func(in *healthInput) {
			in.act = outbox.Activity{LastIteration: ago(time.Minute), LastClaimOK: ago(time.Minute)}
		}, false, "the poll loop has not completed a cycle"},
		{"claims failing", func(in *healthInput) {
			in.leader = leading
			in.act = outbox.Activity{
				LastIteration: ago(time.Second), LastClaimOK: ago(time.Minute),
				LastClaimError: ago(time.Second), ClaimError: "permission denied for table outbox_messages",
			}
		}, false, "no successful claim: permission denied"},
		{"claims timing out on a locked table", func(in *healthInput) {
			in.leader = leading
			in.act = outbox.Activity{
				LastIteration: ago(time.Second), LastClaimOK: ago(time.Minute),
				LastClaimError: ago(time.Second), ClaimError: "context deadline exceeded", ClaimTimedOut: true,
			}
		}, true, ""},
		{"token unobtainable", func(in *healthInput) {
			in.leader, in.act = leading, polling
			in.token = tokenState{checked: true, err: "token endpoint 500", failingSince: ago(time.Minute)}
		}, false, "cannot obtain a platform token: token endpoint 500"},
		{"token failing briefly", func(in *healthInput) {
			in.leader, in.act = leading, polling
			in.token = tokenState{checked: true, err: "token endpoint 500", failingSince: ago(time.Second)}
		}, true, ""},
		{"token unobtainable, not required", func(in *healthInput) {
			in.leader, in.act, in.requireToken = leading, polling, false
			in.token = tokenState{checked: true, err: "token endpoint 500", failingSince: ago(time.Minute)}
		}, true, ""},
		{"just started leading", func(in *healthInput) {
			in.phaseSince = ago(time.Second)
			in.leader = leading
		}, true, ""},
		{"starting", func(in *healthInput) { in.phase = phaseStarting }, false, "connecting"},
		{"waiting for the table", func(in *healthInput) {
			in.phase, in.phaseSince, in.lastContact = phaseWaitingTable, ago(time.Hour), ago(time.Second)
		}, true, ""},
		{"waiting for the table, database gone", func(in *healthInput) {
			in.phase, in.phaseSince, in.lastContact = phaseWaitingTable, ago(time.Hour), ago(time.Minute)
		}, false, "cannot check the source database"},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			in := base()
			c.in(&in)
			r := verdict(in)
			assert.Equal(t, c.healthy, r.Healthy, "%+v", r)
			if c.reason != "" {
				assert.Contains(t, r.Reason, c.reason)
			}
		})
	}
}

func TestOutboxHealthCommandWithoutAPoller(t *testing.T) {
	cmd := newOutboxCmd()
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{"health", "--env-file", "", "--health-addr", "127.0.0.1:1", "--timeout", "500ms"})
	err := cmd.ExecuteContext(context.Background())
	assert.ErrorContains(t, err, "does not answer")
}
