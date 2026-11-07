//go:build integration

package runtimeapi_test

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/ingestion"
)

// The instance guard on the wire: a second process polling as a live runtime's
// id is answered 409 RUNTIME_ID_IN_USE in the platform's error envelope.
func TestPoll_RuntimeIDInUse(t *testing.T) {
	e := newEnv(t)
	rt := "rt-" + unique()
	poll := func(instance string) response {
		return e.do(http.MethodPost, base+"/poll", map[string]any{
			"runtimeId": rt, "contractVersion": ingestion.ContractVersion, "instanceId": instance,
		})
	}
	first := poll("6f1c2a9e-1b7d-4c55-9a8e-0d1e2f3a4b5c")
	require.Equal(t, http.StatusOK, first.code, first.raw)
	second := poll("0a9b8c7d-6e5f-4a3b-8c2d-1e0f9a8b7c6d")
	assert.Equal(t, http.StatusConflict, second.code, second.raw)
	assert.Equal(t, "RUNTIME_ID_IN_USE", second.errCode())
	assert.Equal(t, "runtime id "+rt+" is in use by another live process", second.str("message"))
	assert.Equal(t, http.StatusOK, poll("6f1c2a9e-1b7d-4c55-9a8e-0d1e2f3a4b5c").code, "the first keeps polling")

	bad := poll("short")
	assert.Equal(t, http.StatusBadRequest, bad.code, "an instanceId is 8 to 64 of [A-Za-z0-9-]: %s", bad.raw)
}

// The release route: the leases the runtime holds end now and are counted,
// an imposter is refused, and goodbye marks the runtime gone.
func TestRelease_Route(t *testing.T) {
	e := newEnv(t)
	i := e.save("release", sftpSource, true)
	rt := "rt-" + unique()
	instance := "6f1c2a9e-1b7d-4c55-9a8e-0d1e2f3a4b5c"
	r := e.do(http.MethodPost, base+"/poll", map[string]any{"runtimeId": rt, "contractVersion": ingestion.ContractVersion, "instanceId": instance})
	require.Equal(t, http.StatusOK, r.code, r.raw)
	require.Equal(t, 1, e.count(`SELECT count(*) FROM ing_leases WHERE ingestion_id = $1 AND runtime_id = $2`, i.ID, rt))

	imposter := e.do(http.MethodPost, base+"/leases/release", map[string]any{
		"runtimeId": rt, "instanceId": "0a9b8c7d-6e5f-4a3b-8c2d-1e0f9a8b7c6d", "ingestionIds": []string{i.ID}, "goodbye": true,
	})
	assert.Equal(t, http.StatusConflict, imposter.code, imposter.raw)
	assert.Equal(t, "RUNTIME_ID_IN_USE", imposter.errCode())

	ok := e.do(http.MethodPost, base+"/leases/release", map[string]any{
		"runtimeId": rt, "instanceId": instance, "ingestionIds": []string{i.ID, "ing_nottheirs00"}, "goodbye": true,
	})
	require.Equal(t, http.StatusOK, ok.code, ok.raw)
	assert.EqualValues(t, 1, ok.json["released"])
	assert.Equal(t, 1, e.count(`SELECT count(*) FROM ing_leases WHERE ingestion_id = $1 AND released AND expires_at <= $2`, i.ID, e.now))
	assert.Equal(t, 1, e.count(`SELECT count(*) FROM ing_runtimes WHERE runtime_id = $1 AND left_at IS NOT NULL`, rt))

	empty := e.do(http.MethodPost, base+"/leases/release", map[string]any{"runtimeId": rt, "ingestionIds": []string{}, "goodbye": true})
	assert.Equal(t, http.StatusOK, empty.code, "a goodbye with nothing to release: %s", empty.raw)

	e.who = e.principal(permView)
	assert.Equal(t, http.StatusForbidden, e.do(http.MethodPost, base+"/leases/release", map[string]any{"runtimeId": rt, "ingestionIds": []string{}, "goodbye": false}).code,
		"the runtime role only, as poll")
}

// Release never answers 404: the runtime reads a 404 as "older platform
// without this route" and stops calling it. A runtime the platform has never
// seen, ingestions that do not exist or that it does not hold, with or without
// an instance and a goodbye, are 200 {released: 0}.
func TestRelease_UnknownIsNeverNotFound(t *testing.T) {
	e := newEnv(t)
	held := e.save("held-elsewhere", sftpSource, true)
	holder := "rt-" + unique()
	require.Equal(t, http.StatusOK, e.poll(holder, ingestion.ContractVersion).code)

	unknown := "rt-never-seen-" + unique()
	for _, body := range []map[string]any{
		{"runtimeId": unknown, "ingestionIds": []string{"ing_doesnotexist0"}, "goodbye": false},
		{"runtimeId": unknown, "instanceId": "6f1c2a9e-1b7d-4c55-9a8e-0d1e2f3a4b5c", "ingestionIds": []string{"ing_doesnotexist0", held.ID}, "goodbye": true},
		{"runtimeId": unknown, "ingestionIds": []string{}, "goodbye": true},
		{"runtimeId": holder, "ingestionIds": []string{"ing_doesnotexist0"}, "goodbye": false},
	} {
		r := e.do(http.MethodPost, base+"/leases/release", body)
		require.Equal(t, http.StatusOK, r.code, "%v: %s", body, r.raw)
		assert.EqualValues(t, 0, r.json["released"], "%v", body)
	}
	assert.Equal(t, 1, e.count(`SELECT count(*) FROM ing_leases WHERE ingestion_id = $1 AND runtime_id = $2 AND NOT released`, held.ID, holder),
		"another runtime's lease is untouched")
	assert.Equal(t, 0, e.count(`SELECT count(*) FROM ing_runtimes WHERE runtime_id = $1`, unknown), "a release creates no runtime")
}

// A running polled ingestion that no runtime has held for a lease and two
// minutes reads as not running, while a runtime carries its kind.
func TestIngestion_UnheldNotice(t *testing.T) {
	e := newEnv(t)
	i := e.save("unheld", sftpSource, true)
	ctx := context.Background()
	_, err := e.pool.Exec(ctx, `UPDATE ing_ingestions SET updated_at = now() - interval '10 minutes' WHERE id = $1`, i.ID)
	require.NoError(t, err)
	carrier := "rt-carrier-" + unique()
	_, err = e.pool.Exec(ctx, `INSERT INTO ing_runtimes (runtime_id, contract_version, first_seen_at, last_seen_at) VALUES ($1, 2, now(), now())`, carrier)
	require.NoError(t, err)

	r := e.do(http.MethodGet, "/api/ingestions/"+i.ID, nil)
	require.Equal(t, http.StatusOK, r.code, r.raw)
	assert.True(t, strings.HasPrefix(r.str("runtimeNotice"), "not running: no runtime has held it since "), r.str("runtimeNotice"))

	list := e.do(http.MethodGet, "/api/ingestions?applicationCode="+e.app.Code, nil)
	require.Equal(t, http.StatusOK, list.code, list.raw)
	rows := list.list("ingestions")
	require.Len(t, rows, 1)
	assert.Contains(t, rows[0]["runtimeNotice"], "not running: no runtime has held it since ")
}
