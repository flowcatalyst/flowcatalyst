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

// --init-schema: default true (today's behaviour), FC_OUTBOX_INIT_SCHEMA when the flag is not
// given, the flag over the env, and an env value that is not a boolean refused rather than read
// as the default (which would bring back the DDL the switch exists to turn off).
func TestOutboxInitSchemaResolution(t *testing.T) {
	cases := []struct {
		env, flag string
		want      bool
		wantErr   bool
	}{
		{want: true},
		{env: "false", want: false},
		{env: "0", want: false},
		{env: "OFF", want: false},
		{env: "true", want: true},
		{env: "false", flag: "--init-schema=true", want: true},
		{env: "true", flag: "--init-schema=false", want: false},
		{flag: "--init-schema=false", want: false},
		{env: "flase", wantErr: true},
	}
	for _, c := range cases {
		t.Run(c.env+"|"+c.flag, func(t *testing.T) {
			t.Setenv("FC_OUTBOX_INIT_SCHEMA", c.env)
			cmd := newOutboxCmd()
			var args []string
			if c.flag != "" {
				args = append(args, c.flag)
			}
			if err := cmd.ParseFlags(args); err != nil {
				t.Fatal(err)
			}
			got, err := resolveEnvFlagBool(cmd, "init-schema", "FC_OUTBOX_INIT_SCHEMA")
			if c.wantErr {
				if err == nil || !strings.Contains(err.Error(), "FC_OUTBOX_INIT_SCHEMA") {
					t.Fatalf("err = %v, want one naming FC_OUTBOX_INIT_SCHEMA", err)
				}
				return
			}
			if err != nil || got != c.want {
				t.Fatalf("got %v, %v; want %v", got, err, c.want)
			}
		})
	}
}

// An unreadable FC_OUTBOX_INIT_SCHEMA stops the command before it connects to anything.
func TestOutboxRefusesUnreadableInitSchema(t *testing.T) {
	t.Setenv("FC_OUTBOX_INIT_SCHEMA", "nope")
	cmd := newOutboxCmd()
	cmd.SetArgs([]string{"--env-file", "", "--source-db-url", "postgres://127.0.0.1:1/x"})
	var out strings.Builder
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), `FC_OUTBOX_INIT_SCHEMA="nope"`) {
		t.Fatalf("err = %v, want FC_OUTBOX_INIT_SCHEMA refused", err)
	}
}
