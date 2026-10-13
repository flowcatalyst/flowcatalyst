package operations_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/function/operations"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecase"
)

// The function events Go shares with Java (FunctionEvents) and Rust
// (fc-platform-functions events.rs) carry their code, envelope and data
// (docs/platform-bug-fixes.md F11): source platform:function, subject
// platform.function.{id}, group platform:function:{id}, camelCase data with
// absent optionals omitted. The Go-only events keep their platform:admin
// codes and source.
func TestFunctionEvents_SharedWithJavaAndRust(t *testing.T) {
	desc := "does things"
	client := "clt_0000000000001"
	prev := "fnv_0000000000001"
	cases := []struct {
		name  string
		event usecase.DomainEvent
		code  string
		data  string
	}{
		{
			"created, platform-owned",
			operations.FunctionCreated{
				FunctionID: "fng_0000000000001", Address: "billing.invoices", ApplicationID: "app_0000000000001",
			},
			"platform:function:function:created",
			`{"functionId":"fng_0000000000001","address":"billing.invoices","applicationId":"app_0000000000001"}`,
		},
		{
			"created, client-owned",
			operations.FunctionCreated{
				FunctionID: "fng_0000000000001", Address: "billing.invoices", ApplicationID: "app_0000000000001", ClientID: &client,
			},
			"platform:function:function:created",
			`{"functionId":"fng_0000000000001","address":"billing.invoices","applicationId":"app_0000000000001","clientId":"clt_0000000000001"}`,
		},
		{
			"updated",
			operations.FunctionUpdated{
				FunctionID: "fng_0000000000001", Address: "billing.invoices", Description: &desc, Status: operations.FunctionStatusActive,
			},
			"platform:function:function:updated",
			`{"functionId":"fng_0000000000001","address":"billing.invoices","description":"does things","status":"ACTIVE"}`,
		},
		{
			"updated, no description",
			operations.FunctionUpdated{
				FunctionID: "fng_0000000000001", Address: "billing.invoices", Status: operations.FunctionStatusActive,
			},
			"platform:function:function:updated",
			`{"functionId":"fng_0000000000001","address":"billing.invoices","status":"ACTIVE"}`,
		},
		{
			"deleted",
			operations.FunctionDeleted{FunctionID: "fng_0000000000001", Address: "billing.invoices"},
			"platform:function:function:deleted",
			`{"functionId":"fng_0000000000001","address":"billing.invoices"}`,
		},
		{
			"version published",
			operations.VersionPublished{
				FunctionID: "fng_0000000000001", Address: "billing.invoices", VersionID: "fnv_0000000000002", Version: 2,
				Digest: "abc123", Pool: "default",
			},
			"platform:function:version:published",
			`{"functionId":"fng_0000000000001","address":"billing.invoices","versionId":"fnv_0000000000002","version":2,"digest":"abc123","pool":"default"}`,
		},
		{
			"version retired",
			operations.VersionRetired{
				FunctionID: "fng_0000000000001", Address: "billing.invoices", VersionID: "fnv_0000000000002", Version: 2,
			},
			"platform:function:version:retired",
			`{"functionId":"fng_0000000000001","address":"billing.invoices","versionId":"fnv_0000000000002","version":2}`,
		},
		{
			"alias changed, first",
			operations.AliasChanged{
				FunctionID: "fng_0000000000001", Address: "billing.invoices", Alias: "live", VersionID: "fnv_0000000000002", Version: 2,
			},
			"platform:function:alias:changed",
			`{"functionId":"fng_0000000000001","address":"billing.invoices","alias":"live","versionId":"fnv_0000000000002","version":2}`,
		},
		{
			"alias changed, moved",
			operations.AliasChanged{
				FunctionID: "fng_0000000000001", Address: "billing.invoices", Alias: "canary", VersionID: "fnv_0000000000002", Version: 2,
				PreviousVersionID: &prev,
			},
			"platform:function:alias:changed",
			`{"functionId":"fng_0000000000001","address":"billing.invoices","alias":"canary","versionId":"fnv_0000000000002","version":2,"previousVersionId":"fnv_0000000000001"}`,
		},
		{
			"alias removed",
			operations.AliasRemoved{
				FunctionID: "fng_0000000000001", Address: "billing.invoices", Alias: "canary", VersionID: "fnv_0000000000002", Version: 2,
			},
			"platform:function:alias:removed",
			`{"functionId":"fng_0000000000001","address":"billing.invoices","alias":"canary","versionId":"fnv_0000000000002","version":2}`,
		},
		{
			"config updated",
			operations.ConfigUpdated{
				FunctionID: "fng_0000000000001", Address: "billing.invoices", Keys: []string{"A", "B"},
			},
			"platform:function:config:updated",
			`{"functionId":"fng_0000000000001","address":"billing.invoices","keys":["A","B"]}`,
		},
		{
			"config updated, none left",
			operations.ConfigUpdated{
				FunctionID: "fng_0000000000001", Address: "billing.invoices",
			},
			"platform:function:config:updated",
			`{"functionId":"fng_0000000000001","address":"billing.invoices","keys":[]}`,
		},
		{
			"secret set",
			operations.SecretSet{FunctionID: "fng_0000000000001", Address: "billing.invoices", Key: "API_KEY"},
			"platform:function:secret:set",
			`{"functionId":"fng_0000000000001","address":"billing.invoices","key":"API_KEY"}`,
		},
		{
			"secret deleted",
			operations.SecretDeleted{FunctionID: "fng_0000000000001", Address: "billing.invoices", Key: "API_KEY"},
			"platform:function:secret:deleted",
			`{"functionId":"fng_0000000000001","address":"billing.invoices","key":"API_KEY"}`,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.code, c.event.EventType())
			assert.Equal(t, "platform:function", c.event.Source())
			assert.Equal(t, "platform.function.fng_0000000000001", c.event.Subject())
			assert.Equal(t, "platform:function:fng_0000000000001", c.event.MessageGroup())
			data, err := c.event.ToDataJSON()
			require.NoError(t, err)
			assert.JSONEq(t, c.data, string(data))
		})
	}
}

// The events Java and Rust have no counterpart for keep their
// platform:admin:function* codes, source platform:admin, and data.
func TestFunctionEvents_GoOnly(t *testing.T) {
	cases := []struct {
		name  string
		event usecase.DomainEvent
		code  string
		data  string
	}{
		{
			"promoted",
			operations.FunctionPromoted{
				FunctionID: "fng_0000000000001", Address: "billing.invoices", Number: 2, DispatchPoolCode: "fn-billing-invoices",
				SubscriptionsCreated: 1, SchedulesDeleted: 1,
			},
			"platform:admin:function:promoted",
			`{"functionId":"fng_0000000000001","address":"billing.invoices","number":2,"dispatchPoolCode":"fn-billing-invoices",
			  "subscriptionsCreated":1,"subscriptionsUpdated":0,"subscriptionsDeleted":0,
			  "schedulesCreated":0,"schedulesUpdated":0,"schedulesDeleted":1}`,
		},
		{
			"schedule wired",
			operations.FunctionScheduleWired{
				FunctionID: "fng_0000000000001", ScheduledJobID: "sjb_0000000000001", Code: "fn-tick", Action: "created",
			},
			"platform:admin:function-schedule:wired",
			`{"functionId":"fng_0000000000001","scheduledJobId":"sjb_0000000000001","code":"fn-tick","action":"created"}`,
		},
		{
			"route set",
			operations.RouteSet{
				FunctionID: "fng_0000000000001", RouteID: "fnr_0000000000001", Hostname: "api.acme.com", PathPrefix: "/billing",
			},
			"platform:admin:function-route:set",
			`{"functionId":"fng_0000000000001","routeId":"fnr_0000000000001","hostname":"api.acme.com","pathPrefix":"/billing"}`,
		},
		{
			"route deleted",
			operations.RouteDeleted{
				FunctionID: "fng_0000000000001", RouteID: "fnr_0000000000001", Hostname: "api.acme.com", PathPrefix: "/billing",
			},
			"platform:admin:function-route:deleted",
			`{"functionId":"fng_0000000000001","routeId":"fnr_0000000000001","hostname":"api.acme.com","pathPrefix":"/billing"}`,
		},
		{
			"db setting set",
			operations.DBSettingSet{FunctionID: "fng_0000000000001", Key: "maindb"},
			"platform:admin:function-setting:set",
			`{"functionId":"fng_0000000000001","kind":"DB","key":"maindb"}`,
		},
		{
			"db setting deleted",
			operations.DBSettingDeleted{FunctionID: "fng_0000000000001", Key: "maindb"},
			"platform:admin:function-setting:deleted",
			`{"functionId":"fng_0000000000001","kind":"DB","key":"maindb"}`,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.code, c.event.EventType())
			assert.Equal(t, "platform:admin", c.event.Source())
			assert.Equal(t, "platform.function.fng_0000000000001", c.event.Subject())
			assert.Equal(t, "platform:function:fng_0000000000001", c.event.MessageGroup())
			data, err := c.event.ToDataJSON()
			require.NoError(t, err)
			assert.JSONEq(t, c.data, string(data))
		})
	}
}
