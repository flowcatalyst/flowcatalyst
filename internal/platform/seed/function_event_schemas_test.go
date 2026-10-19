package seed

import (
	"encoding/json"
	"testing"

	jsonschema "github.com/google/jsonschema-go/jsonschema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	fnops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/function/operations"
	domainops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/functiondomain/operations"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecase"
)

// Every function event's real data (ToDataJSON), with its optional fields
// present and absent, validates against the schema the seed catalogues for
// its code (docs/platform-bug-fixes.md F11).
func TestFunctionEventDataValidatesAgainstTheSeededSchema(t *testing.T) {
	schemas := map[string]json.RawMessage{}
	for _, d := range PlatformEventTypes() {
		schemas[d.Code] = d.Schema
	}
	desc, client, prev := "d", "clt_0000000000001", "fnv_0000000000001"
	events := []usecase.DomainEvent{
		fnops.FunctionCreated{FunctionID: "f", Address: "a.b", ApplicationID: "app"},
		fnops.FunctionCreated{FunctionID: "f", Address: "a.b", ApplicationID: "app", ClientID: &client},
		fnops.FunctionUpdated{FunctionID: "f", Address: "a.b", Status: fnops.FunctionStatusActive},
		fnops.FunctionUpdated{FunctionID: "f", Address: "a.b", Description: &desc, Status: fnops.FunctionStatusActive},
		fnops.FunctionDeleted{FunctionID: "f", Address: "a.b"},
		fnops.VersionPublished{FunctionID: "f", Address: "a.b", VersionID: "v", Version: 1, Digest: "d", Pool: "default"},
		fnops.VersionRetired{FunctionID: "f", Address: "a.b", VersionID: "v", Version: 1},
		fnops.AliasChanged{FunctionID: "f", Address: "a.b", Alias: "live", VersionID: "v", Version: 1},
		fnops.AliasChanged{FunctionID: "f", Address: "a.b", Alias: "live", VersionID: "v", Version: 2, PreviousVersionID: &prev},
		fnops.AliasRemoved{FunctionID: "f", Address: "a.b", Alias: "canary", VersionID: "v", Version: 1},
		fnops.ConfigUpdated{FunctionID: "f", Address: "a.b", Keys: []string{"A"}},
		fnops.ConfigUpdated{FunctionID: "f", Address: "a.b"},
		fnops.SecretSet{FunctionID: "f", Address: "a.b", Key: "K"},
		fnops.SecretDeleted{FunctionID: "f", Address: "a.b", Key: "K"},
		fnops.FunctionPromoted{FunctionID: "f", Address: "a.b", Number: 1, DispatchPoolCode: "fn-a-b", SubscriptionsCreated: 1},
		fnops.FunctionPromoted{FunctionID: "f", Address: "a.b", DispatchPoolCode: "fn-a-b"},
		fnops.FunctionScheduleWired{FunctionID: "f", ScheduledJobID: "s", Code: "c", Action: "deleted"},
		fnops.VersionPruned{FunctionID: "f", Address: "a.b", VersionID: "v", Version: 3, Digest: "d"},
		fnops.RouteSet{FunctionID: "f", RouteID: "r", Hostname: "h.com", PathPrefix: "/"},
		fnops.RouteDeleted{FunctionID: "f", RouteID: "r", Hostname: "h.com", PathPrefix: "/"},
		fnops.DBSettingSet{FunctionID: "f", Key: "maindb"},
		fnops.DBSettingDeleted{FunctionID: "f", Key: "maindb"},
		domainops.DomainClaimed{DomainID: "d", Hostname: "acme.com", Owner: domainops.OwnerOf(nil)},
		domainops.DomainClaimed{DomainID: "d", Hostname: "acme.com", Owner: domainops.OwnerOf(&client)},
		domainops.DomainReleased{DomainID: "d", Hostname: "acme.com", Owner: domainops.OwnerOf(&client)},
	}
	for _, e := range events {
		raw, err := e.ToDataJSON()
		require.NoError(t, err)
		t.Run(e.EventType()+" "+string(raw), func(t *testing.T) {
			rawSchema, ok := schemas[e.EventType()]
			require.True(t, ok, "%s is not catalogued", e.EventType())
			require.NotEmpty(t, rawSchema, "%s has no schema", e.EventType())
			var schema jsonschema.Schema
			require.NoError(t, json.Unmarshal(rawSchema, &schema))
			resolved, err := schema.Resolve(nil)
			require.NoError(t, err)
			var data any
			require.NoError(t, json.Unmarshal(raw, &data))
			assert.NoError(t, resolved.Validate(data))
		})
	}
}

// For the codes Go shares with Java and Rust, the seeded schema is Java's,
// except where Go has nothing to send: Go functions have no function-level
// runtime (Go's runtime is per version), so platform:function:function:created
// leaves out Java's required runtime (F11).
func TestSharedFunctionEventSchemasMatchJava(t *testing.T) {
	java := map[string]json.RawMessage{
		"platform:function:function:created": obj(
			reqStr("functionId"), reqStr("address"), reqStr("applicationId"), optStr("clientId")),
		"platform:function:function:updated": obj(
			reqStr("functionId"), reqStr("address"), optStr("description"), reqStr("status")),
		"platform:function:function:deleted": obj(reqStr("functionId"), reqStr("address")),
		"platform:function:version:published": obj(
			reqStr("functionId"), reqStr("address"), reqStr("versionId"), reqU32("version"),
			reqStr("digest"), reqStr("pool"), optStr("signerIssuer"), optStr("signerSubject")),
		"platform:function:version:retired": obj(
			reqStr("functionId"), reqStr("address"), reqStr("versionId"), reqU32("version")),
		"platform:function:alias:changed": obj(
			reqStr("functionId"), reqStr("address"), reqStr("alias"), reqStr("versionId"),
			reqU32("version"), optStr("previousVersionId")),
		"platform:function:alias:removed": obj(
			reqStr("functionId"), reqStr("address"), reqStr("alias"), reqStr("versionId"), reqU32("version")),
		"platform:function:config:updated":  obj(reqStr("functionId"), reqStr("address"), reqStrArray("keys")),
		"platform:function:secret:set":      obj(reqStr("functionId"), reqStr("address"), reqStr("key")),
		"platform:function:secret:deleted":  obj(reqStr("functionId"), reqStr("address"), reqStr("key")),
		"platform:function:domain:claimed":  obj(reqStr("domainId"), reqStr("hostname"), reqStr("owner")),
		"platform:function:domain:released": obj(reqStr("domainId"), reqStr("hostname"), reqStr("owner")),
	}
	seeded := platformEventSchemas()
	for code, want := range java {
		require.Contains(t, seeded, code)
		assert.JSONEq(t, string(want), string(seeded[code]), code)
	}
}
