//go:build integration

package operations_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/functions/fnfixture"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/function"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/function/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/go-sdk/usecaseop"
)

// storedEvent is one msg_events row a function use case wrote.
type storedEvent struct {
	Type, Source, Group string
	Data                map[string]any
}

// functionEvents returns the events stored for the fixture's function, oldest
// first (the event id is a monotonic TSID).
func (f *testFunctionFixture) functionEvents() []storedEvent {
	f.t.Helper()
	rows, err := f.pool.Query(context.Background(),
		`SELECT type, source, COALESCE(message_group, ''), data::text FROM msg_events
		  WHERE subject = $1 ORDER BY id`, "platform.function."+f.fn.ID)
	require.NoError(f.t, err)
	defer rows.Close()
	var out []storedEvent
	for rows.Next() {
		var e storedEvent
		var data string
		require.NoError(f.t, rows.Scan(&e.Type, &e.Source, &e.Group, &data))
		require.NoError(f.t, json.Unmarshal([]byte(data), &e.Data))
		out = append(out, e)
	}
	require.NoError(f.t, rows.Err())
	return out
}

// eventsSince returns the events stored after the first n.
func (f *testFunctionFixture) eventsSince(n int) []storedEvent {
	f.t.Helper()
	all := f.functionEvents()
	require.GreaterOrEqual(f.t, len(all), n)
	return all[n:]
}

func dataOf(t *testing.T, s string) map[string]any {
	t.Helper()
	var m map[string]any
	require.NoError(t, json.Unmarshal([]byte(s), &m))
	return m
}

// Create and update store Java's and Rust's function:created and
// function:updated: source platform:function, the function's group, data
// {functionId, address, applicationId, clientId?} and {functionId, address,
// description?, status}.
func TestFunctionEvents_CreateAndUpdateStoreTheSharedData(t *testing.T) {
	t.Parallel()
	f := newTestFunctionFixture(t, "app_optevtcru0001", "optevtcruapp", "cru-fn")

	desc := "now described"
	_, err := runAuthorized(f.uow, operations.UpdateFunction(f.repo), operations.UpdateCommand{ID: f.fn.ID, Description: &desc})
	require.NoError(t, err)

	got := f.functionEvents()
	require.Len(t, got, 2)
	assert.Equal(t, storedEvent{
		Type: "platform:function:function:created", Source: "platform:function", Group: "platform:function:" + f.fn.ID,
		Data: dataOf(t, `{"functionId":"`+f.fn.ID+`","address":"optevtcruapp.cru-fn","applicationId":"app_optevtcru0001"}`),
	}, got[0])
	assert.Equal(t, storedEvent{
		Type: "platform:function:function:updated", Source: "platform:function", Group: "platform:function:" + f.fn.ID,
		Data: dataOf(t, `{"functionId":"`+f.fn.ID+`","address":"optevtcruapp.cru-fn","description":"now described","status":"ACTIVE"}`),
	}, got[1])
}

// Publish and retire store version:published and version:retired, naming
// the version by id and number; published carries the runner pool.
func TestFunctionEvents_PublishAndRetireStoreTheSharedData(t *testing.T) {
	t.Parallel()
	f := newTestFunctionFixture(t, "app_optevtpub0001", "optevtpubapp", "pub-fn")
	digest := putArtifactBytes(t, f.artifacts, fnfixture.Wasm)
	res := f.publish(digest, "")
	_, err := usecaseop.RunTx(testpg.AnchorCtx(), f.uow, operations.RetireVersion(f.repo),
		operations.RetireCommand{FunctionID: f.fn.ID, Number: res.Version.Number}, testpg.TestEC())
	require.NoError(t, err)

	got := f.eventsSince(1)
	require.Len(t, got, 2)
	assert.Equal(t, "platform:function:version:published", got[0].Type)
	assert.Equal(t, "platform:function", got[0].Source)
	assert.Equal(t, dataOf(t, `{"functionId":"`+f.fn.ID+`","address":"optevtpubapp.pub-fn","versionId":"`+res.Version.ID+
		`","version":1,"digest":"`+digest+`","pool":"default"}`), got[0].Data)
	assert.Equal(t, "platform:function:version:retired", got[1].Type)
	assert.Equal(t, dataOf(t, `{"functionId":"`+f.fn.ID+`","address":"optevtpubapp.pub-fn","versionId":"`+res.Version.ID+
		`","version":1}`), got[1].Data)
}

// Every alias move stores alias:changed (with the version it left, when it
// pointed somewhere) and every removal alias:removed (with the version it
// named), `live` included; `live` also stores the Go-only promoted summary.
// Removing an alias that does not exist removes nothing and stores nothing.
func TestFunctionEvents_AliasesStoreChangedAndRemoved(t *testing.T) {
	t.Parallel()
	f := newTestFunctionFixture(t, "app_optevtali0001", "optevtaliapp", "ali-fn")
	t.Cleanup(func() {
		_, _ = f.pool.Exec(context.Background(), `DELETE FROM msg_dispatch_pools WHERE code = 'fn-optevtaliapp-ali-fn'`)
	})
	v1 := f.publish(putArtifactBytes(t, f.artifacts, fnfixture.Wasm), "").Version
	f.markReady(v1.Number)
	v2 := f.publish(putArtifactBytes(t, f.artifacts, jsArtifact(`{"abi":1,"endpoints":[{"path":"/x","auth":"none"}]}`)), "js").Version
	f.markReady(v2.Number)
	require.NoError(t, setConfig(f, "GREETING", "hi"))
	start := len(f.functionEvents())

	_, err := f.putAlias("canary", v1.Number)
	require.NoError(t, err)
	_, err = f.putAlias("canary", v2.Number)
	require.NoError(t, err)
	_, err = f.putAlias("live", v1.Number)
	require.NoError(t, err)
	_, err = f.deleteAlias("canary")
	require.NoError(t, err)
	_, err = f.deleteAlias("nonesuch")
	require.NoError(t, err)
	_, err = f.deleteAlias("live")
	require.NoError(t, err)

	base := `"functionId":"` + f.fn.ID + `","address":"optevtaliapp.ali-fn"`
	var types []string
	got := f.eventsSince(start)
	for _, e := range got {
		types = append(types, e.Type)
	}
	require.Equal(t, []string{
		"platform:function:alias:changed",
		"platform:function:alias:changed",
		"platform:function:alias:changed",
		"platform:admin:function:promoted",
		"platform:function:alias:removed",
		"platform:function:alias:removed",
		"platform:admin:function:promoted",
	}, types)
	assert.Equal(t, dataOf(t, `{`+base+`,"alias":"canary","versionId":"`+v1.ID+`","version":1}`), got[0].Data)
	assert.Equal(t, dataOf(t, `{`+base+`,"alias":"canary","versionId":"`+v2.ID+`","version":2,"previousVersionId":"`+v1.ID+`"}`), got[1].Data)
	assert.Equal(t, dataOf(t, `{`+base+`,"alias":"live","versionId":"`+v1.ID+`","version":1}`), got[2].Data)
	assert.Equal(t, "platform:admin", got[3].Source)
	assert.EqualValues(t, 1, got[3].Data["number"])
	assert.Equal(t, dataOf(t, `{`+base+`,"alias":"canary","versionId":"`+v2.ID+`","version":2}`), got[4].Data)
	assert.Equal(t, dataOf(t, `{`+base+`,"alias":"live","versionId":"`+v1.ID+`","version":1}`), got[5].Data)
	assert.EqualValues(t, 0, got[6].Data["number"])
	for _, e := range got {
		assert.Equal(t, "platform:function:"+f.fn.ID, e.Group)
	}
}

// A SECRET setting stores secret:set / secret:deleted with its key; a CONFIG
// setting stores config:updated listing every CONFIG key the function holds
// after the change; a DB setting keeps the Go-only function-setting codes.
// No event carries a value.
func TestFunctionEvents_SettingsStoreSecretConfigAndDB(t *testing.T) {
	t.Parallel()
	f := newTestFunctionFixture(t, "app_optevtset0001", "optevtsetapp", "set-fn")
	put := func(kind function.SettingKind, key string) {
		t.Helper()
		_, err := usecaseop.RunTx(testpg.AnchorCtx(), f.uow, operations.PutSetting(f.repo),
			operations.PutSettingCommand{FunctionID: f.fn.ID, Kind: kind, Key: key, Value: "sentinel-value"}, testpg.TestEC())
		require.NoError(t, err)
	}
	del := func(kind function.SettingKind, key string) {
		t.Helper()
		_, err := usecaseop.RunTx(testpg.AnchorCtx(), f.uow, operations.DeleteSetting(f.repo),
			operations.DeleteSettingCommand{FunctionID: f.fn.ID, Kind: kind, Key: key}, testpg.TestEC())
		require.NoError(t, err)
	}
	start := len(f.functionEvents())

	put(function.SettingSecret, "API_KEY")
	put(function.SettingConfig, "B")
	put(function.SettingConfig, "A")
	put(function.SettingDB, "maindb")
	del(function.SettingConfig, "B")
	del(function.SettingSecret, "API_KEY")
	del(function.SettingDB, "maindb")
	del(function.SettingConfig, "A")

	base := `"functionId":"` + f.fn.ID + `","address":"optevtsetapp.set-fn"`
	want := []storedEvent{
		{"platform:function:secret:set", "platform:function", "", dataOf(t, `{`+base+`,"key":"API_KEY"}`)},
		{"platform:function:config:updated", "platform:function", "", dataOf(t, `{`+base+`,"keys":["B"]}`)},
		{"platform:function:config:updated", "platform:function", "", dataOf(t, `{`+base+`,"keys":["A","B"]}`)},
		{"platform:admin:function-setting:set", "platform:admin", "", dataOf(t, `{"functionId":"`+f.fn.ID+`","kind":"DB","key":"maindb"}`)},
		{"platform:function:config:updated", "platform:function", "", dataOf(t, `{`+base+`,"keys":["A"]}`)},
		{"platform:function:secret:deleted", "platform:function", "", dataOf(t, `{`+base+`,"key":"API_KEY"}`)},
		{"platform:admin:function-setting:deleted", "platform:admin", "", dataOf(t, `{"functionId":"`+f.fn.ID+`","kind":"DB","key":"maindb"}`)},
		{"platform:function:config:updated", "platform:function", "", dataOf(t, `{`+base+`,"keys":[]}`)},
	}
	got := f.eventsSince(start)
	for i := range got {
		got[i].Group = ""
	}
	assert.Equal(t, want, got)
}
