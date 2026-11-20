package server

import (
	"context"
	"strings"
	"testing"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/payloadstore"
)

// The FC_PAYLOAD_* variables (docs/payload-offload-design.md, section 4): unset
// means the defaults; a value that is not a whole number or a codec is
// refused, naming the variable, instead of falling back.
func TestParsePayloadConfig(t *testing.T) {
	env := func(m map[string]string) func(string) string {
		return func(k string) string { return m[k] }
	}

	c, err := parsePayloadConfig(env(nil))
	if err != nil {
		t.Fatalf("defaults: %v", err)
	}
	if c != payloadstore.DefaultConfig() {
		t.Errorf("defaults = %+v, want %+v", c, payloadstore.DefaultConfig())
	}

	c, err = parsePayloadConfig(env(map[string]string{
		"FC_PAYLOAD_STORE":             " s3://bucket/fc ",
		"FC_PAYLOAD_OFFLOAD_MIN_BYTES": "0",
		"FC_PAYLOAD_INLINE_MAX_BYTES":  "2097152",
		"FC_PAYLOAD_MAX_BYTES":         "4000000",
		"FC_PAYLOAD_CODEC":             "none",
		"FC_PAYLOAD_CACHE_BYTES":       "0",
	}))
	if err != nil {
		t.Fatalf("values: %v", err)
	}
	want := payloadstore.Config{
		Store: "s3://bucket/fc", OffloadMinBytes: 0, InlineMaxBytes: 2097152,
		MaxBytes: 4000000, Codec: payloadstore.CodecNone, CacheBytes: 0,
	}
	if c != want {
		t.Errorf("values = %+v, want %+v", c, want)
	}

	for name, value := range map[string]string{
		"FC_PAYLOAD_OFFLOAD_MIN_BYTES": "5k",
		"FC_PAYLOAD_INLINE_MAX_BYTES":  "0",
		"FC_PAYLOAD_MAX_BYTES":         "-1",
		"FC_PAYLOAD_CACHE_BYTES":       "1.5",
		"FC_PAYLOAD_CODEC":             "gzip",
	} {
		if _, err := parsePayloadConfig(env(map[string]string{name: value})); err == nil || !strings.Contains(err.Error(), name) {
			t.Errorf("%s=%q: err = %v, want a refusal naming the variable", name, value, err)
		}
	}

	// A value in range for the parser but refused by the rules of section 4
	// is caught by Validate, which buildServices runs.
	c, err = parsePayloadConfig(env(map[string]string{"FC_PAYLOAD_STORE": "file:///tmp/p", "FC_PAYLOAD_OFFLOAD_MIN_BYTES": "100"}))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if err := c.Validate(); err == nil {
		t.Error("a threshold of 100 bytes must be refused")
	}

	// A config built by hand is valid: no store.
	if err := (EnvCfg{}).Payload.Validate(); err != nil {
		t.Errorf("a config built by hand is valid: %v", err)
	}
}

// FC_PAYLOAD_TABLE_WRITES, the writers' switch (section 10): off unless set,
// a boolean, and a value that is not one is refused, naming the variable.
func TestParsePayloadConfig_TableWrites(t *testing.T) {
	env := func(v string) func(string) string {
		return func(k string) string {
			if k == "FC_PAYLOAD_TABLE_WRITES" {
				return v
			}
			return ""
		}
	}
	for v, want := range map[string]bool{"": false, "true": true, "ON": true, "1": true, "false": false, "0": false} {
		c, err := parsePayloadConfig(env(v))
		if err != nil {
			t.Fatalf("%q: %v", v, err)
		}
		if c.TableWrites != want {
			t.Errorf("FC_PAYLOAD_TABLE_WRITES=%q: TableWrites = %v, want %v", v, c.TableWrites, want)
		}
	}
	if _, err := parsePayloadConfig(env("maybe")); err == nil || !strings.Contains(err.Error(), "FC_PAYLOAD_TABLE_WRITES") {
		t.Errorf("a value that is not a boolean: err = %v, want a refusal naming the variable", err)
	}
}

// FC_INGEST_BATCH_MAX_BYTES (section 8): the batch routes' request cap,
// 16 MiB unless set; a value that is not a positive whole number is refused,
// and one smaller than twice the largest payload stops startup (Validate,
// which buildServices runs through payloadstore.New).
func TestParsePayloadConfig_IngestBatchMaxBytes(t *testing.T) {
	env := func(m map[string]string) func(string) string {
		return func(k string) string { return m[k] }
	}
	c, err := parsePayloadConfig(env(nil))
	if err != nil || c.IngestBatchMaxBytes != 0 || c.Validate() != nil {
		t.Fatalf("unset: %+v, %v (zero means the 16 MiB default)", c, err)
	}
	c, err = parsePayloadConfig(env(map[string]string{"FC_INGEST_BATCH_MAX_BYTES": "33554432"}))
	if err != nil || c.IngestBatchMaxBytes != 32<<20 {
		t.Fatalf("set: %+v, %v", c, err)
	}
	for _, v := range []string{"0", "-5", "16MiB"} {
		if _, err := parsePayloadConfig(env(map[string]string{"FC_INGEST_BATCH_MAX_BYTES": v})); err == nil || !strings.Contains(err.Error(), "FC_INGEST_BATCH_MAX_BYTES") {
			t.Errorf("%q: err = %v, want a refusal naming the variable", v, err)
		}
	}

	// Under twice the largest payload: refused at startup. With a store and
	// the payload table written the largest is FC_PAYLOAD_MAX_BYTES (5 MB),
	// so 8 MiB is too small; with the switch off it is the 1 MiB inline
	// maximum, so 8 MiB is enough, and 2 MiB - 1 is not.
	written := map[string]string{
		"FC_PAYLOAD_STORE": "file:///tmp/p", "FC_PAYLOAD_TABLE_WRITES": "true", "FC_INGEST_BATCH_MAX_BYTES": "8388608",
	}
	c, err = parsePayloadConfig(env(written))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if err := c.Validate(); err == nil || !strings.Contains(err.Error(), "FC_INGEST_BATCH_MAX_BYTES") {
		t.Errorf("8 MiB against a 5 MB largest payload: %v, want a refusal", err)
	}
	written["FC_PAYLOAD_TABLE_WRITES"] = "false"
	if c, _ = parsePayloadConfig(env(written)); c.Validate() != nil {
		t.Errorf("8 MiB with the switch off: %v", c.Validate())
	}
	c, _ = parsePayloadConfig(env(map[string]string{"FC_INGEST_BATCH_MAX_BYTES": "2097151"}))
	if err := c.Validate(); err == nil {
		t.Error("2 MiB - 1 against the 1 MiB inline maximum must be refused")
	}
	// What buildServices builds the payload service with.
	if _, err := payloadstore.New(context.Background(), c, nil); err == nil || !strings.Contains(err.Error(), "FC_INGEST_BATCH_MAX_BYTES") {
		t.Errorf("payloadstore.New: %v, want startup refused", err)
	}
}
