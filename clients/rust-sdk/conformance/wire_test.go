// Package conformance checks the Rust SDK's golden wire fixtures
// (clients/rust-sdk/tests/fixtures/wire) against the platform's own Go
// decoders and the Go SDK, so a change on either side that breaks the other
// fails `go test ./...`. The Rust side asserts it produces these exact bytes
// (clients/rust-sdk/tests/wire_fixtures.rs).
package conformance

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	eventapi "github.com/flowcatalyst/flowcatalyst-go/internal/platform/event/api"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/sdk"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/tsid"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/webhook"
)

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "tests", "fixtures", "wire", name))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	return bytes.TrimSpace(b)
}

// jsonKeys returns the json names a struct type declares.
func jsonKeys(t reflect.Type) map[string]bool {
	keys := map[string]bool{}
	for f := range t.Fields() {
		name, _, _ := strings.Cut(f.Tag.Get("json"), ",")
		if name != "" && name != "-" {
			keys[name] = true
		}
	}
	return keys
}

func objectKeys(t *testing.T, raw []byte) []string {
	t.Helper()
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("not a JSON object: %v", err)
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// The dispatch-job payloads decode strictly into the batch endpoint's item:
// every key exists in BatchItem, and metadata is the key/value array.
func TestDispatchJobFixturesDecodeStrictlyIntoTheBatchItem(t *testing.T) {
	for _, name := range []string{"dispatch-job-full.json", "dispatch-job-minimal.json"} {
		dec := json.NewDecoder(bytes.NewReader(fixture(t, name)))
		dec.DisallowUnknownFields()
		var item sdk.BatchItem
		if err := dec.Decode(&item); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if item.Code != "orders:billing:export" || item.TargetURL == "" || item.Payload == nil || item.Source == nil {
			t.Fatalf("%s: required members missing: %+v", name, item)
		}
	}
	dec := json.NewDecoder(bytes.NewReader(fixture(t, "dispatch-job-full.json")))
	var full sdk.BatchItem
	if err := dec.Decode(&full); err != nil {
		t.Fatal(err)
	}
	if len(full.Metadata) != 2 || full.Metadata[0].Key != "invoice" || full.Metadata[1].Value != "nightly" {
		t.Fatalf("metadata: %+v", full.Metadata)
	}
	if full.Mode != "BLOCK_ON_ERROR" || full.Sequence != 2 || full.TimeoutSeconds != 45 || full.MaxRetries != 5 {
		t.Fatalf("numbers/mode: %+v", full)
	}
}

// The event payloads use only BatchEventItem's camelCase members and carry
// what the batch endpoint requires (type, data; an absent source is defaulted
// to the type's {application}:{subdomain}, F14). The integration test
// internal/platform/event/api/encoded_data_pg_test.go sends both through the
// batch handler.
func TestEventFixturesUseTheBatchEventItemMembers(t *testing.T) {
	known := jsonKeys(reflect.TypeFor[eventapi.BatchEventItem]())
	for _, name := range []string{"event-full.json", "event-minimal.json"} {
		raw := fixture(t, name)
		for _, k := range objectKeys(t, raw) {
			if !known[k] {
				t.Errorf("%s: key %q is not a BatchEventItem member", name, k)
			}
		}
		var item eventapi.BatchEventItem
		if err := json.Unmarshal(raw, &item); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if item.Type != "orders:sales:order:created" || len(item.Data) == 0 {
			t.Fatalf("%s: %+v", name, item)
		}
	}
	var full eventapi.BatchEventItem
	if err := json.Unmarshal(fixture(t, "event-full.json"), &full); err != nil {
		t.Fatal(err)
	}
	if full.ClientCode == nil || *full.ClientCode != "acme" || len(full.Context) != 2 || full.DeduplicationID == "" {
		t.Fatalf("event-full: %+v", full)
	}
}

// The audit payload's platform-read members decode into AuditBatchItem.
// source, correlationId and metadata are the Java SDK's members that the
// endpoint ignores; nothing else may be unknown.
func TestAuditFixtureUsesTheAuditBatchItemMembers(t *testing.T) {
	known := jsonKeys(reflect.TypeFor[sdk.AuditBatchItem]())
	ignoredByPlatform := map[string]bool{"source": true, "correlationId": true, "metadata": true}
	raw := fixture(t, "audit-log-full.json")
	for _, k := range objectKeys(t, raw) {
		if !known[k] && !ignoredByPlatform[k] {
			t.Errorf("key %q is not an AuditBatchItem member", k)
		}
	}
	var item sdk.AuditBatchItem
	if err := json.Unmarshal(raw, &item); err != nil {
		t.Fatal(err)
	}
	if item.PrincipalID == "" || item.EntityType != "Order" || item.ApplicationCode == nil || item.PerformedAt == nil {
		t.Fatalf("audit: %+v", item)
	}
}

// Every signature vector matches the Go SDK's signer.
func TestWebhookVectorsMatchTheGoSigner(t *testing.T) {
	var vectors []struct {
		Name, Secret, Timestamp, Body, Signature string
	}
	if err := json.Unmarshal(fixture(t, "webhook-signatures.json"), &vectors); err != nil {
		t.Fatal(err)
	}
	if len(vectors) == 0 {
		t.Fatal("no vectors")
	}
	for _, v := range vectors {
		got := webhook.NewValidator(v.Secret).ComputeSignature(v.Timestamp, []byte(v.Body))
		if got != v.Signature {
			t.Errorf("%s: Go signs %s, fixture has %s", v.Name, got, v.Signature)
		}
	}
}

// TSID encodings match the Go SDK's.
func TestTsidVectorsMatchTheGoEncoder(t *testing.T) {
	var vectors []struct {
		Value   int64  `json:"value"`
		Encoded string `json:"encoded"`
	}
	if err := json.Unmarshal(fixture(t, "tsid-vectors.json"), &vectors); err != nil {
		t.Fatal(err)
	}
	for _, v := range vectors {
		if got := tsid.FromLong(v.Value); got != v.Encoded {
			t.Errorf("FromLong(%d) = %s, fixture %s", v.Value, got, v.Encoded)
		}
		if got, ok := tsid.ToLong(v.Encoded); !ok || got != v.Value {
			t.Errorf("ToLong(%s) = %d,%v, fixture %d", v.Encoded, got, ok, v.Value)
		}
	}
}

// The redaction vectors are the Go SDK's file byte for byte.
func TestRedactionVectorsAreTheGoSdkCopy(t *testing.T) {
	ours, err := os.ReadFile(filepath.Join("..", "tests", "fixtures", "audit-redaction-vectors.json"))
	if err != nil {
		t.Fatal(err)
	}
	theirs, err := os.ReadFile(filepath.Join("..", "..", "..", "pkg", "fcsdk", "usecase", "testdata", "audit-redaction-vectors.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(ours, theirs) {
		t.Fatal("tests/fixtures/audit-redaction-vectors.json differs from pkg/fcsdk/usecase/testdata")
	}
}

// The outbox DDL is the Java SDK's migration byte for byte.
func TestOutboxDDLIsTheJavaSdkMigration(t *testing.T) {
	for _, dialect := range []string{"postgresql", "mysql"} {
		ours, err := os.ReadFile(filepath.Join("..", "migrations", dialect, "001_create_outbox_messages.sql"))
		if err != nil {
			t.Fatal(err)
		}
		theirs, err := os.ReadFile(filepath.Join("..", "..", "java-sdk", "migrations", dialect, "001_create_outbox_messages.sql"))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(ours, theirs) {
			t.Fatalf("%s DDL differs from the Java SDK's", dialect)
		}
	}
}
