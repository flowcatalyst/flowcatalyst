package main

import (
	"strings"
	"testing"
)

// `fcdev outbox poll` and `fcdev outbox init` were once documented; they do not
// exist. They must be rejected, not silently run the poller.
func TestOutboxRejectsUnknownSubcommands(t *testing.T) {
	for _, sub := range []string{"poll", "init"} {
		cmd := newOutboxCmd()
		cmd.SetArgs([]string{sub})
		var out strings.Builder
		cmd.SetOut(&out)
		cmd.SetErr(&out)
		err := cmd.Execute()
		if err == nil || !strings.Contains(err.Error(), "unknown command") {
			t.Errorf("fcdev outbox %s: err = %v, want unknown command", sub, err)
		}
	}
}
