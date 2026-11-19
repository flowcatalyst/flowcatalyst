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
	b := auops.BranchData{
		UpdateID: "upd_0000000000001", ApplicationID: "app_0000000000001", ApplicationCode: "acme",
		AppKey: "driver-app", Platform: "android", RuntimeVersion: "1.0.0", Channel: "production",
	}
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

// Binary and install-link events validate against their seeded schemas; a
// link's events carry no token field.
func TestAppBinaryEventDataValidatesAgainstTheSeededSchema(t *testing.T) {
	schemas := map[string]json.RawMessage{}
	for _, d := range PlatformEventTypes() {
		schemas[d.Code] = d.Schema
	}
	msg, rv := "fix", "1.4.0"
	link := auops.InstallLinkData{LinkID: "lnk_0000000000001", ApplicationID: "app_0000000000001", ApplicationCode: "acme", AppKey: "driver-app", Channel: "production", Generation: 2}
	events := []usecase.DomainEvent{
		auops.BinaryPublished{
			BinaryID: "bin_0000000000001", ApplicationID: "app_0000000000001", ApplicationCode: "acme", AppKey: "driver-app",
			Platform: "android", Channel: "production", Variant: "release", SHA256: "s", SizeBytes: 50_000_000,
			PackageName: "com.example.driver", VersionCode: 42, SigningCertSHA256: "c",
		},
		auops.BinaryPublished{
			BinaryID: "bin_0000000000001", ApplicationID: "app_0000000000001", ApplicationCode: "acme", AppKey: "driver-app",
			Platform: "android", Channel: "production", Variant: "debug", SHA256: "s", SizeBytes: 1, PackageName: "p",
			VersionCode: 1 << 33, VersionName: &rv, RuntimeVersion: &rv, SigningCertSHA256: "c", Message: &msg, SourceCommit: &msg,
		},
		auops.InstallLinkCreated{InstallLinkData: link},
		auops.InstallLinkReplaced{InstallLinkData: link},
		auops.InstallLinkRemoved{InstallLinkData: link},
	}
	for _, e := range events {
		raw, err := e.ToDataJSON()
		require.NoError(t, err)
		assert.NotContains(t, string(raw), "token")
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

// Operators make install links; an application's service account, the
// viewer and a client's administrators do not (docs/app-updates-design.md
// section 14.7). Existing deployments get the grant from migration 121.
func TestInstallLinkPermissionIsTheOperatorsOnly(t *testing.T) {
	const p = "platform:app-update:install-link:manage"
	roles := map[string][]string{}
	for _, r := range PlatformRoles() {
		roles[r.Name] = r.Permissions
	}
	assert.True(t, slices.Contains(roles["platform:app-update-admin"], p))
	for _, name := range []string{"platform:application-service", "platform:viewer", "platform:client-admin"} {
		assert.False(t, slices.Contains(roles[name], p), name)
	}
}
