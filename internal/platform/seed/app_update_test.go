package seed

import (
	"encoding/json"
	"slices"
	"testing"

	jsonschema "github.com/google/jsonschema-go/jsonschema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	auops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/appupdate/operations"
	"github.com/flowcatalyst/go-sdk/usecase"
)

// Every app-update event's real data, optional fields present and absent,
// validates against its seeded schema.
func TestAppUpdateEventDataValidatesAgainstTheSeededSchema(t *testing.T) {
	schemas := map[string]json.RawMessage{}
	for _, d := range PlatformEventTypes() {
		schemas[d.Code] = d.Schema
	}
	b := auops.BranchData{UpdateID: "upd_0000000000001", ApplicationID: "app_0000000000001", ApplicationCode: "acme",
		AppKey: "driver-app", Platform: "android", RuntimeVersion: "1.0.0", Channel: "production"}
	msg := "fix"
	events := []usecase.DomainEvent{
		auops.UpdatePublished{BranchData: b, ManifestID: "m", CreatedAt: "2026-11-17T10:00:00.000Z", RolloutPercent: 100, LaunchAssetSHA256: "s", AssetCount: 2},
		auops.UpdatePublished{BranchData: b, ManifestID: "m", CreatedAt: "t", RolloutPercent: 5, LaunchAssetSHA256: "s", Message: &msg, SourceCommit: &msg},
		auops.RolloutChanged{BranchData: b, PreviousPercent: 10, RolloutPercent: 50},
		auops.UpdateRepublished{BranchData: b, RepublishedFrom: "upd_x", ManifestID: "m", CreatedAt: "t", Reason: auops.ReasonRollback},
		auops.UpdateRepublished{BranchData: b, RepublishedFrom: "upd_x", ManifestID: "m", CreatedAt: "t", Reason: auops.ReasonRepublish, Message: &msg},
		auops.RolledBackToEmbedded{BranchData: b, CommitTime: "t"},
		auops.RolledBackToEmbedded{BranchData: b, CommitTime: "t", Message: &msg},
	}
	for _, e := range events {
		raw, err := e.ToDataJSON()
		require.NoError(t, err)
		t.Run(e.EventType()+" "+string(raw), func(t *testing.T) {
			rawSchema, ok := schemas[e.EventType()]
			require.True(t, ok, "%s is not catalogued", e.EventType())
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

// docs/app-updates-design.md section 10: an application's service account
// publishes, rolls out and rolls back its own application's updates; the
// operator role holds the same; the viewer reads. Existing deployments get the
// grants from migration 118.
func TestAppUpdatePermissionsInBuiltInRoles(t *testing.T) {
	all := []string{"platform:app-update:update:view", "platform:app-update:update:publish", "platform:app-update:rollout:manage"}
	roles := map[string][]string{}
	for _, r := range PlatformRoles() {
		roles[r.Name] = r.Permissions
	}
	for _, name := range []string{"platform:application-service", "platform:app-update-admin"} {
		for _, p := range all {
			assert.True(t, slices.Contains(roles[name], p), "%s lacks %s", name, p)
		}
	}
	assert.True(t, slices.Contains(roles["platform:viewer"], all[0]))
	for _, p := range all[1:] {
		assert.False(t, slices.Contains(roles["platform:viewer"], p))
		assert.False(t, slices.Contains(roles["platform:client-admin"], p), "a client's administrators do not act on updates")
	}
}
