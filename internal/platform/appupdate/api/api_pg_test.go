//go:build integration

package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	aut "github.com/flowcatalyst/flowcatalyst-go/internal/platform/appupdate/appupdatetest"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
)

func TestMain(m *testing.M) { testpg.RunMain(m) }

func branchA(w *aut.World) aut.Branch {
	return aut.Branch{App: w.AppA, AppKey: "driver-app", Platform: "android", RuntimeVersion: "2.0.0", Channel: "production"}
}

func errCode(t *testing.T, body []byte) string {
	t.Helper()
	var e struct {
		Error string `json:"error"`
	}
	require.NoError(t, json.Unmarshal(body, &e), string(body))
	return e.Error
}

func count(t *testing.T, sql string, args ...any) int {
	t.Helper()
	var n int
	require.NoError(t, testpg.Pool(t).QueryRow(context.Background(), sql, args...).Scan(&n))
	return n
}

// Publish writes the update, its assets, the event and the audit row.
func TestPublish_RecordsEventAndAudit(t *testing.T) {
	w := aut.NewWorld(t, aut.Options{})
	u := w.Publish("sa-a", branchA(w), aut.NewExport("audit", 3), 100)
	assert.Equal(t, 1, count(t, `SELECT count(*) FROM app_updates WHERE id = $1 AND kind = 'UPDATE' AND published_by = 'prn_ausaa00000001'`, u.ID))
	assert.Equal(t, 4, count(t, `SELECT count(*) FROM app_update_assets WHERE update_id = $1`, u.ID))
	assert.Equal(t, 1, count(t, `SELECT count(*) FROM msg_events WHERE type = 'platform:app-update:update:published' AND subject = $1`, "platform.appupdate."+u.ID))
	var op string
	var opJSON []byte
	require.NoError(t, testpg.Pool(t).QueryRow(context.Background(),
		`SELECT operation, operation_json FROM aud_logs WHERE entity_id = $1`, u.ID).Scan(&op, &opJSON))
	assert.Equal(t, "PublishCommand", op)
	var audited map[string]any
	require.NoError(t, json.Unmarshal(opJSON, &audited))
	assert.Equal(t, float64(3), audited["assetCount"], "a summary, not the asset list")
	assert.Equal(t, true, audited["hasExpoClient"])
	assert.NotContains(t, string(opJSON), "slug", "the app config is not copied into the audit log")
}

// A phone is never told about an update whose files are not all stored: a
// publish naming a file the application has not stored is refused and writes
// nothing.
func TestPublish_AllFilesOrNothing(t *testing.T) {
	w := aut.NewWorld(t, aut.Options{})
	b := branchA(w)
	e := aut.NewExport("atomic", 2)
	// Upload all but one asset.
	partial := aut.Export{Bundle: e.Bundle, Assets: e.Assets[:1]}
	w.Upload("sa-a", b.App, partial)
	st, body := w.Do("sa-a", http.MethodPost, "/api/app-updates/updates", aut.PublishBody(b, e, 100, nil))
	require.Equal(t, http.StatusConflict, st, string(body))
	assert.Equal(t, "FILES_MISSING", errCode(t, body))
	assert.Contains(t, string(body), aut.SHA256Hex(e.Assets[1].Body))
	assert.Equal(t, 0, count(t, `SELECT count(*) FROM app_updates WHERE application_id = $1`, w.AppAID))
	assert.Equal(t, 0, count(t, `SELECT count(*) FROM msg_events WHERE type LIKE 'platform:app-update:%' AND data::text LIKE '%'||$1||'%'`, w.AppAID))
	a := aut.NewPhone(w, b).Ask(t)
	assert.Equal(t, "noUpdateAvailable", a.Directive["type"])

	// Another application having the file does not count for this one.
	w.Upload("sa-b", w.AppB, aut.Export{Bundle: e.Bundle, Assets: e.Assets})
	st, body = w.Do("sa-a", http.MethodPost, "/api/app-updates/updates", aut.PublishBody(b, e, 100, nil))
	require.Equal(t, http.StatusConflict, st, string(body))
	// Uploading the last file completes it; a re-upload stores nothing twice.
	assert.Equal(t, 1, w.Upload("sa-a", b.App, e))
	assert.Equal(t, 0, w.Upload("sa-a", b.App, e))
	st, body = w.Do("sa-a", http.MethodPost, "/api/app-updates/updates", aut.PublishBody(b, e, 100, nil))
	require.Equal(t, http.StatusCreated, st, string(body))
}

// The store refuses a body whose SHA-256 is not its address, and nothing is
// registered.
func TestPutFile_DigestMismatch(t *testing.T) {
	w := aut.NewWorld(t, aut.Options{})
	sha := aut.SHA256Hex([]byte("the real one"))
	st, body := w.Do("sa-a", http.MethodPut, "/api/app-updates/files/"+w.AppA+"/"+sha, []byte("something else"))
	require.Equal(t, http.StatusBadRequest, st, string(body))
	assert.Equal(t, "DIGEST_MISMATCH", errCode(t, body))
	assert.Equal(t, 0, count(t, `SELECT count(*) FROM app_update_files WHERE application_id = $1`, w.AppAID))
	st, body = w.Do("sa-a", http.MethodPut, "/api/app-updates/files/"+w.AppA+"/"+strings.ToUpper(sha), []byte("the real one"))
	require.Equal(t, http.StatusBadRequest, st, string(body))
	assert.Equal(t, "INVALID_SHA256", errCode(t, body))
}

// An application's service account acts on its own application only; another
// application's updates are refused (by code) or not found (by id), an unknown
// application looks like another's, a client-tier principal is refused whatever
// it holds, and each route needs its permission.
func TestPermissions(t *testing.T) {
	w := aut.NewWorld(t, aut.Options{})
	b := branchA(w)
	u := w.Publish("sa-a", b, aut.NewExport("perm", 1), 100)
	e := aut.NewExport("perm-b", 1)

	// sa-b on application A.
	st, body := w.Do("sa-b", http.MethodPost, "/api/app-updates/files/"+w.AppA+"/missing", map[string]any{"sha256": []string{aut.SHA256Hex(e.Bundle)}})
	require.Equal(t, http.StatusForbidden, st, string(body))
	assert.Equal(t, "SCOPE_FORBIDDEN", errCode(t, body))
	st, body = w.Do("sa-b", http.MethodPut, "/api/app-updates/files/"+w.AppA+"/"+aut.SHA256Hex(e.Bundle), e.Bundle)
	require.Equal(t, http.StatusForbidden, st, string(body))
	st, body = w.Do("sa-b", http.MethodPost, "/api/app-updates/updates", aut.PublishBody(b, e, 100, nil))
	require.Equal(t, http.StatusForbidden, st, string(body))
	st, body = w.Do("sa-b", http.MethodGet, "/api/app-updates/updates?applicationCode="+w.AppA, nil)
	require.Equal(t, http.StatusForbidden, st, string(body))
	st, body = w.Do("sa-b", http.MethodGet, "/api/app-updates/branches?applicationCode="+w.AppA, nil)
	require.Equal(t, http.StatusForbidden, st, string(body))
	st, body = w.Do("sa-b", http.MethodGet, "/api/app-updates/updates/"+u.ID, nil)
	require.Equal(t, http.StatusNotFound, st, string(body))
	st, body = w.Do("sa-b", http.MethodPut, "/api/app-updates/updates/"+u.ID+"/rollout", map[string]int{"rolloutPercent": 10})
	require.Equal(t, http.StatusNotFound, st, string(body))
	st, body = w.Do("sa-b", http.MethodPost, "/api/app-updates/updates/"+u.ID+"/republish", map[string]any{})
	require.Equal(t, http.StatusNotFound, st, string(body))
	st, body = w.Do("sa-b", http.MethodPost, "/api/app-updates/rollback", map[string]any{
		"applicationCode": b.App, "appKey": b.AppKey, "platform": b.Platform, "runtimeVersion": b.RuntimeVersion, "channel": b.Channel, "to": "EMBEDDED",
	})
	require.Equal(t, http.StatusForbidden, st, string(body))

	// An unknown application: the same 403 for an application-scoped caller,
	// not found for an operator.
	st, body = w.Do("sa-b", http.MethodGet, "/api/app-updates/updates?applicationCode=no-such-app-x", nil)
	require.Equal(t, http.StatusForbidden, st, string(body))
	assert.Equal(t, "SCOPE_FORBIDDEN", errCode(t, body))
	st, _ = w.Do("operator", http.MethodGet, "/api/app-updates/updates?applicationCode=no-such-app-x", nil)
	require.Equal(t, http.StatusNotFound, st)

	// Client tier: refused whatever it holds.
	st, body = w.Do("client-admin", http.MethodGet, "/api/app-updates/updates?applicationCode="+w.AppA, nil)
	require.Equal(t, http.StatusForbidden, st, string(body))
	st, body = w.Do("client-admin", http.MethodGet, "/api/app-updates/updates/"+u.ID, nil)
	require.Equal(t, http.StatusForbidden, st, string(body))

	// Permissions: a viewer reads but does not publish or roll back; nobody does nothing.
	st, _ = w.Do("viewer", http.MethodGet, "/api/app-updates/updates/"+u.ID, nil)
	require.Equal(t, http.StatusOK, st)
	st, body = w.Do("viewer", http.MethodPost, "/api/app-updates/updates", aut.PublishBody(b, e, 100, nil))
	require.Equal(t, http.StatusForbidden, st)
	assert.Equal(t, "PERMISSION_REQUIRED", errCode(t, body))
	st, _ = w.Do("viewer", http.MethodPut, "/api/app-updates/updates/"+u.ID+"/rollout", map[string]int{"rolloutPercent": 10})
	require.Equal(t, http.StatusForbidden, st)
	st, _ = w.Do("nobody", http.MethodGet, "/api/app-updates/updates/"+u.ID, nil)
	require.Equal(t, http.StatusForbidden, st)
	st, _ = w.Do("anonymous", http.MethodGet, "/api/app-updates/updates/"+u.ID, nil)
	require.Contains(t, []int{http.StatusUnauthorized, http.StatusForbidden}, st)

	// The operator acts on any application; sa-a on its own.
	w.Publish("operator", aut.Branch{App: w.AppB, AppKey: "driver-app", Platform: "ios", RuntimeVersion: "2.0.0", Channel: "production"}, e, 100)
	st, _ = w.Do("sa-a", http.MethodGet, "/api/app-updates/updates/"+u.ID, nil)
	require.Equal(t, http.StatusOK, st)
}

// Rollout rules: only the newest update has a changeable rollout, a publish
// onto a partial rollout is refused, and a rollback supersedes it.
func TestRolloutRules(t *testing.T) {
	w := aut.NewWorld(t, aut.Options{})
	b := branchA(w)
	u1 := w.Publish("sa-a", b, aut.NewExport("r1", 1), 100)
	u2 := w.Publish("sa-a", b, aut.NewExport("r2", 1), 20)

	w.Upload("sa-a", b.App, aut.NewExport("r3", 1))
	st, body := w.Do("sa-a", http.MethodPost, "/api/app-updates/updates", aut.PublishBody(b, aut.NewExport("r3", 1), 100, nil))
	require.Equal(t, http.StatusConflict, st, string(body))
	assert.Equal(t, "ROLLOUT_IN_PROGRESS", errCode(t, body))

	st, body = w.Do("sa-a", http.MethodPut, "/api/app-updates/updates/"+u1.ID+"/rollout", map[string]int{"rolloutPercent": 50})
	require.Equal(t, http.StatusConflict, st, string(body))
	assert.Equal(t, "NOT_LATEST_UPDATE", errCode(t, body))
	st, body = w.Do("sa-a", http.MethodPut, "/api/app-updates/updates/"+u2.ID+"/rollout", map[string]int{"rolloutPercent": 101})
	require.Equal(t, http.StatusBadRequest, st, string(body), "huma refuses out of range before the handler")

	// Served shares while partial.
	var page struct {
		Data []aut.UpdateRow `json:"data"`
	}
	st, body = w.Do("sa-a", http.MethodGet, "/api/app-updates/updates?applicationCode="+w.AppA+"&appKey=driver-app", nil)
	require.Equal(t, http.StatusOK, st, string(body))
	require.NoError(t, json.Unmarshal(body, &page))
	require.Len(t, page.Data, 2)
	assert.Equal(t, u2.ID, page.Data[0].ID)
	assert.Equal(t, 20, page.Data[0].ServedPercent)
	assert.Equal(t, 80, page.Data[1].ServedPercent)
	assert.True(t, page.Data[1].Live)

	// Change it: the event is recorded; the same value again records nothing.
	st, body = w.Do("sa-a", http.MethodPut, "/api/app-updates/updates/"+u2.ID+"/rollout", map[string]int{"rolloutPercent": 60})
	require.Equal(t, http.StatusOK, st, string(body))
	st, _ = w.Do("sa-a", http.MethodPut, "/api/app-updates/updates/"+u2.ID+"/rollout", map[string]int{"rolloutPercent": 60})
	require.Equal(t, http.StatusOK, st)
	assert.Equal(t, 1, count(t, `SELECT count(*) FROM msg_events WHERE type = 'platform:app-update:update:rollout-changed' AND subject = $1`, "platform.appupdate."+u2.ID))
	assert.Equal(t, 1, count(t, `SELECT count(*) FROM aud_logs WHERE entity_id = $1 AND operation = 'SetRolloutCommand'`, u2.ID))

	// Branches show the partial current and who gets the rest.
	var br struct {
		Branches []struct {
			Current  aut.UpdateRow  `json:"current"`
			Previous *aut.UpdateRow `json:"previous"`
		} `json:"branches"`
	}
	st, body = w.Do("sa-a", http.MethodGet, "/api/app-updates/branches?applicationCode="+w.AppA, nil)
	require.Equal(t, http.StatusOK, st, string(body))
	require.NoError(t, json.Unmarshal(body, &br))
	require.Len(t, br.Branches, 1)
	assert.Equal(t, u2.ID, br.Branches[0].Current.ID)
	require.NotNil(t, br.Branches[0].Previous)
	assert.Equal(t, u1.ID, br.Branches[0].Previous.ID)
	assert.Equal(t, 40, br.Branches[0].Previous.ServedPercent)

	// A rollback to the previous update supersedes the partial rollout.
	st, body = w.Do("sa-a", http.MethodPost, "/api/app-updates/rollback", map[string]any{
		"applicationCode": b.App, "appKey": b.AppKey, "platform": b.Platform, "runtimeVersion": b.RuntimeVersion,
		"channel": b.Channel, "to": "PREVIOUS",
	})
	require.Equal(t, http.StatusCreated, st, string(body))
	assert.Equal(t, 1, count(t, `SELECT count(*) FROM msg_events WHERE type = 'platform:app-update:update:republished' AND data->>'republishedFrom' = $1 AND data->>'reason' = 'ROLLBACK'`, u1.ID))
}

// Rollback and republish edge cases.
func TestRollbacks(t *testing.T) {
	w := aut.NewWorld(t, aut.Options{})
	b := branchA(w)
	rollback := func(to string) (int, []byte) {
		return w.Do("sa-a", http.MethodPost, "/api/app-updates/rollback", map[string]any{
			"applicationCode": b.App, "appKey": b.AppKey, "platform": b.Platform, "runtimeVersion": b.RuntimeVersion,
			"channel": b.Channel, "to": to,
		})
	}
	st, body := rollback("PREVIOUS")
	require.Equal(t, http.StatusConflict, st, string(body))
	assert.Equal(t, "NOTHING_TO_ROLL_BACK", errCode(t, body))

	u1 := w.Publish("sa-a", b, aut.NewExport("b1", 1), 100)
	st, body = rollback("PREVIOUS")
	require.Equal(t, http.StatusConflict, st, string(body))
	assert.Equal(t, "NO_PREVIOUS_UPDATE", errCode(t, body))

	st, body = w.Do("sa-a", http.MethodPost, "/api/app-updates/updates/"+u1.ID+"/republish", map[string]any{})
	require.Equal(t, http.StatusConflict, st, string(body))
	assert.Equal(t, "ALREADY_LIVE", errCode(t, body))

	st, body = rollback("EMBEDDED")
	require.Equal(t, http.StatusCreated, st, string(body))
	var rb struct {
		Update  aut.UpdateRow `json:"update"`
		Created bool          `json:"created"`
	}
	require.NoError(t, json.Unmarshal(body, &rb))
	assert.Equal(t, "ROLLBACK_TO_EMBEDDED", rb.Update.Kind)
	// Again: nothing new.
	st, body = rollback("EMBEDDED")
	require.Equal(t, http.StatusOK, st, string(body))
	var again struct {
		Update  aut.UpdateRow `json:"update"`
		Created bool          `json:"created"`
	}
	require.NoError(t, json.Unmarshal(body, &again))
	assert.False(t, again.Created)
	assert.Equal(t, rb.Update.ID, again.Update.ID)
	assert.Equal(t, 1, count(t, `SELECT count(*) FROM msg_events WHERE type = 'platform:app-update:update:rolled-back-to-embedded' AND subject = $1`, "platform.appupdate."+rb.Update.ID))

	st, body = rollback("PREVIOUS")
	require.Equal(t, http.StatusConflict, st, string(body))
	assert.Equal(t, "NOTHING_TO_ROLL_BACK", errCode(t, body))
	st, body = w.Do("sa-a", http.MethodPost, "/api/app-updates/updates/"+rb.Update.ID+"/republish", map[string]any{})
	require.Equal(t, http.StatusConflict, st, string(body))
	assert.Equal(t, "NOT_AN_UPDATE", errCode(t, body))

	// Undo the rollback by republishing u1.
	st, body = w.Do("sa-a", http.MethodPost, "/api/app-updates/updates/"+u1.ID+"/republish", map[string]any{"message": "undo"})
	require.Equal(t, http.StatusCreated, st, string(body))
	var rp aut.UpdateRow
	require.NoError(t, json.Unmarshal(body, &rp))
	require.NotNil(t, rp.RepublishedFrom)
	assert.Equal(t, u1.ID, *rp.RepublishedFrom)
	assert.Equal(t, rp.ManifestID, aut.NewPhone(w, b).Ask(t).Manifest.ID)

	st, body = rollback("SIDEWAYS")
	require.Equal(t, http.StatusBadRequest, st, string(body), "the enum is checked before the handler")
}

// Concurrent publishes on one branch are serialised: every update gets a
// distinct, strictly later createdAt.
func TestPublish_ConcurrentCommitTimesAreStrictlyIncreasing(t *testing.T) {
	w := aut.NewWorld(t, aut.Options{})
	b := branchA(w)
	const n = 8
	exports := make([]aut.Export, n)
	for i := range n {
		exports[i] = aut.NewExport("c"+string(rune('a'+i)), 1)
		w.Upload("sa-a", b.App, exports[i])
	}
	var wg sync.WaitGroup
	statuses := make([]int, n)
	for i := range n {
		wg.Go(func() {
			st, _ := w.Do("sa-a", http.MethodPost, "/api/app-updates/updates", aut.PublishBody(b, exports[i], 100, nil))
			statuses[i] = st
		})
	}
	wg.Wait()
	for _, st := range statuses {
		require.Equal(t, http.StatusCreated, st)
	}
	rows, err := testpg.Pool(t).Query(context.Background(),
		`SELECT created_at FROM app_updates WHERE application_id = $1 ORDER BY created_at, id`, w.AppAID)
	require.NoError(t, err)
	defer rows.Close()
	var prev time.Time
	k := 0
	for rows.Next() {
		var c time.Time
		require.NoError(t, rows.Scan(&c))
		if k > 0 {
			require.True(t, c.After(prev), "strictly increasing")
		}
		prev = c
		k++
	}
	require.Equal(t, n, k)
}

// Malformed publishes are refused before anything is read.
func TestPublish_Validation(t *testing.T) {
	w := aut.NewWorld(t, aut.Options{})
	b := branchA(w)
	e := aut.NewExport("v", 1)
	w.Upload("sa-a", b.App, e)
	mut := func(f func(map[string]any)) map[string]any {
		body := aut.PublishBody(b, e, 100, nil)
		f(body)
		return body
	}
	for name, tc := range map[string]struct {
		body map[string]any
		code string
	}{
		"bad app key": {mut(func(m map[string]any) { m["appKey"] = "Driver App" }), "INVALID_APP_KEY"},
		"bad runtime": {mut(func(m map[string]any) { m["runtimeVersion"] = "" }), "INVALID_RUNTIME_VERSION"},
		"bad channel": {mut(func(m map[string]any) { m["channel"] = "Prod" }), "INVALID_CHANNEL"},
		"bad launch":  {mut(func(m map[string]any) { m["launchAsset"] = map[string]string{"sha256": "x"} }), "INVALID_LAUNCH_ASSET"},
		"bad extension": {mut(func(m map[string]any) {
			m["assets"] = []map[string]string{{"sha256": aut.SHA256Hex(e.Bundle), "fileExtension": "../x"}}
		}), "INVALID_ASSET"},
		"expoClient array":  {mut(func(m map[string]any) { m["expoClient"] = []int{1} }), "INVALID_EXPO_CLIENT"},
		"bad source commit": {mut(func(m map[string]any) { m["sourceCommit"] = "a b" }), "INVALID_SOURCE_COMMIT"},
	} {
		st, body := w.Do("sa-a", http.MethodPost, "/api/app-updates/updates", tc.body)
		require.Equal(t, http.StatusBadRequest, st, "%s: %s", name, body)
		assert.Equal(t, tc.code, errCode(t, body), name)
	}
	assert.Equal(t, 0, count(t, `SELECT count(*) FROM app_updates WHERE application_id = $1`, w.AppAID))
}

// An inactive application cannot publish.
func TestPublish_InactiveApplication(t *testing.T) {
	w := aut.NewWorld(t, aut.Options{})
	b := branchA(w)
	e := aut.NewExport("inactive", 1)
	w.Upload("sa-a", b.App, e)
	_, err := testpg.Pool(t).Exec(context.Background(), `UPDATE app_applications SET active = false WHERE id = $1`, w.AppAID)
	require.NoError(t, err)
	st, body := w.Do("sa-a", http.MethodPost, "/api/app-updates/updates", aut.PublishBody(b, e, 100, nil))
	require.Equal(t, http.StatusConflict, st, string(body))
	assert.Equal(t, "APPLICATION_INACTIVE", errCode(t, body))
}

// The detail answer lists the files in manifest order with their keys.
func TestGetUpdateDetail(t *testing.T) {
	w := aut.NewWorld(t, aut.Options{})
	e := aut.NewExport("detail", 2)
	u := w.Publish("sa-a", branchA(w), e, 100)
	st, body := w.Do("viewer", http.MethodGet, "/api/app-updates/updates/"+u.ID, nil)
	require.Equal(t, http.StatusOK, st, string(body))
	var d struct {
		Update aut.UpdateRow `json:"update"`
		Assets []struct {
			SHA256        string `json:"sha256"`
			Key           string `json:"key"`
			FileExtension string `json:"fileExtension"`
			IsLaunchAsset bool   `json:"isLaunchAsset"`
		} `json:"assets"`
		TotalBytes    int64 `json:"totalBytes"`
		HasExpoClient bool  `json:"hasExpoClient"`
	}
	require.NoError(t, json.Unmarshal(body, &d))
	require.Len(t, d.Assets, 3)
	assert.True(t, d.Assets[0].IsLaunchAsset)
	assert.Equal(t, aut.SHA256Hex(e.Bundle), d.Assets[0].SHA256)
	assert.Equal(t, ".png", d.Assets[1].FileExtension)
	assert.Len(t, d.Assets[1].Key, 32)
	assert.Equal(t, int64(len(e.Bundle)+len(e.Assets[0].Body)+len(e.Assets[1].Body)), d.TotalBytes)
	assert.True(t, d.HasExpoClient)
	assert.Equal(t, 100, d.Update.ServedPercent)
}
