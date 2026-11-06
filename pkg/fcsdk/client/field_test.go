package client_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/go-sdk/client"
)

const workerJSON = `{"id":"fwk_1","clientId":"clt_1","workerCode":"w1","name":"W","email":"w@x.io","status":"ACTIVE","hasPin":true,"pinChangeRequired":true,"locked":false,"hardLocked":false,"failedSinceSuccess":2,"apps":[{"id":"app_1","code":"orders","name":"Orders","source":"API","grantedAt":"2026-01-01T00:00:00Z"}],"createdAt":"2026-01-01T00:00:00Z","updatedAt":"2026-01-02T00:00:00Z","created":true}`

func TestFieldWorkersEnsure(t *testing.T) {
	srv, seen := newMockSrv(t, workerJSON)
	c := client.New(srv.URL)
	r, err := c.FieldWorkers().Ensure(context.Background(), &client.EnsureFieldWorkerRequest{
		ClientID: "clt_1", WorkerCode: "w1", Name: "W", AppCodes: []string{"orders"}, Pin: new("123456"),
	})
	require.NoError(t, err)
	assert.Equal(t, http.MethodPost, seen.method)
	assert.Equal(t, "/api/field-workers", seen.path)
	assert.JSONEq(t, `{"clientId":"clt_1","workerCode":"w1","name":"W","appCodes":["orders"],"pin":"123456"}`, seen.body)
	assert.Equal(t, "fwk_1", r.ID)
	assert.True(t, r.HasPin)
	assert.True(t, r.PinChangeRequired)
	assert.Equal(t, 2, r.FailedSinceSuccess)
	require.NotNil(t, r.Created)
	assert.True(t, *r.Created)
	require.Len(t, r.Apps, 1)
	assert.Equal(t, "orders", r.Apps[0].Code)
	assert.Equal(t, "2026-01-01T00:00:00Z", r.Apps[0].GrantedAt)
}

func TestFieldWorkersListBuildsQuery(t *testing.T) {
	srv, seen := newMockSrv(t, `{"workers":[`+workerJSON+`],"total":7,"page":1,"size":5}`)
	c := client.New(srv.URL)
	r, err := c.FieldWorkers().List(context.Background(), "clt_1", &client.FieldWorkerFilters{
		Q: "al", AppCode: "orders", Status: "DISABLED", Page: new(uint32(1)), PageSize: new(uint32(5)),
	})
	require.NoError(t, err)
	assert.Equal(t, http.MethodGet, seen.method)
	assert.Equal(t, "/api/field-workers", seen.path)
	assert.Equal(t, "clt_1", seen.query.Get("clientId"))
	assert.Equal(t, "al", seen.query.Get("q"))
	assert.Equal(t, "orders", seen.query.Get("appCode"))
	assert.Equal(t, "DISABLED", seen.query.Get("status"))
	assert.Equal(t, "1", seen.query.Get("page"))
	assert.Equal(t, "5", seen.query.Get("pageSize"))
	assert.Equal(t, int64(7), r.Total)
	require.Len(t, r.Workers, 1)
	assert.Equal(t, "w1", r.Workers[0].WorkerCode)

	_, err = c.FieldWorkers().List(context.Background(), "clt_1", nil)
	require.NoError(t, err)
	assert.Equal(t, "clt_1", seen.query.Get("clientId"))
	assert.False(t, seen.query.Has("q"))
	assert.False(t, seen.query.Has("page"))
}

func TestFieldWorkersGetUpdateDelete(t *testing.T) {
	srv, seen := newMockSrv(t, workerJSON)
	c := client.New(srv.URL)
	ctx := context.Background()

	r, err := c.FieldWorkers().Get(ctx, "fwk_1")
	require.NoError(t, err)
	assert.Equal(t, http.MethodGet, seen.method)
	assert.Equal(t, "/api/field-workers/fwk_1", seen.path)
	assert.Equal(t, "W", r.Name)

	r, err = c.FieldWorkers().Update(ctx, "fwk_1", &client.UpdateFieldWorkerRequest{Name: "W2", Email: new("n@x.io")})
	require.NoError(t, err)
	assert.Equal(t, http.MethodPut, seen.method)
	assert.Equal(t, "/api/field-workers/fwk_1", seen.path)
	assert.JSONEq(t, `{"name":"W2","email":"n@x.io"}`, seen.body)
	assert.Equal(t, "fwk_1", r.ID)

	srv2, seen2 := newMockSrv(t, `{"message":"Field worker deleted"}`)
	require.NoError(t, client.New(srv2.URL).FieldWorkers().Delete(ctx, "fwk_1"))
	assert.Equal(t, http.MethodDelete, seen2.method)
	assert.Equal(t, "/api/field-workers/fwk_1", seen2.path)
}

func TestFieldWorkersResetPin(t *testing.T) {
	srv, seen := newMockSrv(t, `{"temporaryPin":"482913","pinChangeRequired":true}`)
	c := client.New(srv.URL)

	r, err := c.FieldWorkers().ResetPin(context.Background(), "fwk_1", nil)
	require.NoError(t, err)
	assert.Equal(t, http.MethodPost, seen.method)
	assert.Equal(t, "/api/field-workers/fwk_1/pin-reset", seen.path)
	assert.JSONEq(t, `{}`, seen.body)
	require.NotNil(t, r.TemporaryPin)
	assert.Equal(t, "482913", *r.TemporaryPin)
	assert.True(t, r.PinChangeRequired)

	srv2, seen2 := newMockSrv(t, `{"pinChangeRequired":true}`)
	r, err = client.New(srv2.URL).FieldWorkers().ResetPin(context.Background(), "fwk_1", &client.ResetFieldWorkerPinRequest{Pin: new("246810")})
	require.NoError(t, err)
	assert.JSONEq(t, `{"pin":"246810"}`, seen2.body)
	assert.Nil(t, r.TemporaryPin)
}

func TestFieldWorkersActions(t *testing.T) {
	srv, seen := newMockSrv(t, `{"message":"ok"}`)
	c := client.New(srv.URL)
	ctx := context.Background()
	w := c.FieldWorkers()

	cases := []struct {
		name         string
		call         func() error
		method, path string
		body         string
	}{
		{"unlock", func() error { return w.Unlock(ctx, "fwk_1") }, http.MethodPost, "/api/field-workers/fwk_1/unlock", ""},
		{"suspend", func() error { return w.Suspend(ctx, "fwk_1") }, http.MethodPost, "/api/field-workers/fwk_1/suspend", ""},
		{"activate", func() error { return w.Activate(ctx, "fwk_1") }, http.MethodPost, "/api/field-workers/fwk_1/activate", ""},
		{"grant", func() error { return w.GrantApp(ctx, "fwk_1", "orders") }, http.MethodPost, "/api/field-workers/fwk_1/apps", `{"appCode":"orders"}`},
		{"revoke", func() error { return w.RevokeApp(ctx, "fwk_1", "orders") }, http.MethodDelete, "/api/field-workers/fwk_1/apps/orders", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.NoError(t, tc.call())
			assert.Equal(t, tc.method, seen.method)
			assert.Equal(t, tc.path, seen.path)
			if tc.body == "" {
				assert.Empty(t, seen.body)
			} else {
				assert.JSONEq(t, tc.body, seen.body)
			}
		})
	}
}

func TestFieldWorkersSignIns(t *testing.T) {
	srv, seen := newMockSrv(t, `{"signIns":[{"id":"fsi_1","outcome":"FAILURE","failureReason":"BAD_PIN","authMethod":"PIN","deviceId":"fdv_1","appCode":"orders","ipAddress":"10.0.0.1","attemptedAt":"2026-01-01T00:00:00Z"}],"total":1,"page":2,"size":10}`)
	c := client.New(srv.URL)
	r, err := c.FieldWorkers().SignIns(context.Background(), "fwk_1", new(uint32(2)), new(uint32(10)))
	require.NoError(t, err)
	assert.Equal(t, http.MethodGet, seen.method)
	assert.Equal(t, "/api/field-workers/fwk_1/sign-ins", seen.path)
	assert.Equal(t, "2", seen.query.Get("page"))
	assert.Equal(t, "10", seen.query.Get("pageSize"))
	require.Len(t, r.SignIns, 1)
	assert.Equal(t, "BAD_PIN", r.SignIns[0].FailureReason)
	assert.Equal(t, "10.0.0.1", r.SignIns[0].IPAddress)
	assert.Equal(t, 2, r.Page)
}

const deviceJSON = `{"id":"fdv_1","clientId":"clt_1","mode":"PERSONAL","label":"Pixel","platform":"android","model":"P8","jkt":"abc","attestationLevel":"TEE","assignedWorkerId":"fwk_1","status":"ACTIVE","apps":[{"id":"app_1","code":"orders","name":"Orders"}],"activeSessions":3,"enrolledAt":"2026-01-01T00:00:00Z","lastSeenAt":"2026-01-02T00:00:00Z"}`

func TestFieldDevicesCreateEnrolmentCode(t *testing.T) {
	srv, seen := newMockSrv(t, `{"code":"ABCD-1234","qrPayload":"fcfield:1:https://fc.example:ABCD-1234","expiresAt":"2026-01-01T00:15:00Z"}`)
	c := client.New(srv.URL)
	r, err := c.FieldDevices().CreateEnrolmentCode(context.Background(), &client.CreateEnrolmentCodeRequest{
		ClientID: "clt_1", Mode: "PERSONAL", AppCodes: []string{"orders"}, AssignedWorkerID: new("fwk_1"), Label: new("Pixel"),
	})
	require.NoError(t, err)
	assert.Equal(t, http.MethodPost, seen.method)
	assert.Equal(t, "/api/field-devices/enrolment-codes", seen.path)
	assert.JSONEq(t, `{"clientId":"clt_1","mode":"PERSONAL","appCodes":["orders"],"assignedWorkerId":"fwk_1","label":"Pixel"}`, seen.body)
	assert.Equal(t, "ABCD-1234", r.Code)
	assert.Equal(t, "fcfield:1:https://fc.example:ABCD-1234", r.QrPayload)
	assert.Equal(t, "2026-01-01T00:15:00Z", r.ExpiresAt)
}

func TestFieldDevicesListGet(t *testing.T) {
	srv, seen := newMockSrv(t, `{"devices":[`+deviceJSON+`],"total":1,"page":0,"size":100}`)
	c := client.New(srv.URL)
	ctx := context.Background()

	r, err := c.FieldDevices().List(ctx, "clt_1", &client.FieldDeviceFilters{Status: "ACTIVE", Mode: "PERSONAL", Page: new(uint32(0)), PageSize: new(uint32(100))})
	require.NoError(t, err)
	assert.Equal(t, http.MethodGet, seen.method)
	assert.Equal(t, "/api/field-devices", seen.path)
	assert.Equal(t, "clt_1", seen.query.Get("clientId"))
	assert.Equal(t, "ACTIVE", seen.query.Get("status"))
	assert.Equal(t, "PERSONAL", seen.query.Get("mode"))
	assert.Equal(t, "0", seen.query.Get("page"))
	assert.Equal(t, "100", seen.query.Get("pageSize"))
	require.Len(t, r.Devices, 1)
	assert.Equal(t, "TEE", r.Devices[0].AttestationLevel)

	srv2, seen2 := newMockSrv(t, deviceJSON)
	d, err := client.New(srv2.URL).FieldDevices().Get(ctx, "fdv_1")
	require.NoError(t, err)
	assert.Equal(t, http.MethodGet, seen2.method)
	assert.Equal(t, "/api/field-devices/fdv_1", seen2.path)
	assert.Equal(t, 3, d.ActiveSessions)
	require.NotNil(t, d.AssignedWorkerID)
	assert.Equal(t, "fwk_1", *d.AssignedWorkerID)
	assert.Equal(t, "orders", d.Apps[0].Code)
}

func TestFieldDevicesActions(t *testing.T) {
	srv, seen := newMockSrv(t, `{"message":"ok"}`)
	c := client.New(srv.URL)
	ctx := context.Background()
	d := c.FieldDevices()

	require.NoError(t, d.Suspend(ctx, "fdv_1"))
	assert.Equal(t, http.MethodPost, seen.method)
	assert.Equal(t, "/api/field-devices/fdv_1/suspend", seen.path)
	require.NoError(t, d.Activate(ctx, "fdv_1"))
	assert.Equal(t, "/api/field-devices/fdv_1/activate", seen.path)
	require.NoError(t, d.Revoke(ctx, "fdv_1"))
	assert.Equal(t, http.MethodPost, seen.method)
	assert.Equal(t, "/api/field-devices/fdv_1/revoke", seen.path)
}

const policyJSON = `{"clientId":"clt_1","pinMinLength":6,"softLockThreshold":5,"softLockSecs":900,"hardLockThreshold":10,"sharedIdleSecs":900,"personalIdleSecs":7200,"shiftMaxSecs":50400,"requireAttestation":true,"pinReauthDays":7,"isDefault":false,"updatedAt":"2026-01-01T00:00:00Z"}`

func TestFieldPoliciesGetPut(t *testing.T) {
	srv, seen := newMockSrv(t, policyJSON)
	c := client.New(srv.URL)
	ctx := context.Background()

	p, err := c.FieldPolicies().Get(ctx, "clt_1")
	require.NoError(t, err)
	assert.Equal(t, http.MethodGet, seen.method)
	assert.Equal(t, "/api/field-policies/clt_1", seen.path)
	assert.Equal(t, 6, p.PinMinLength)
	assert.Equal(t, 50400, p.ShiftMaxSecs)
	assert.True(t, p.RequireAttestation)
	assert.False(t, p.IsDefault)

	p, err = c.FieldPolicies().Put(ctx, "clt_1", &client.PutFieldPolicyRequest{PinMinLength: new(8), RequireAttestation: true})
	require.NoError(t, err)
	assert.Equal(t, http.MethodPut, seen.method)
	assert.Equal(t, "/api/field-policies/clt_1", seen.path)
	assert.JSONEq(t, `{"pinMinLength":8,"requireAttestation":true}`, seen.body)
	assert.Equal(t, "clt_1", p.ClientID)
}

func TestFieldTokensIntrospect(t *testing.T) {
	srv, seen := newMockSrv(t, `{"active":true,"sub":"fwk_1","clientId":"clt_1","workerCode":"w1","deviceId":"fdv_1","appCode":"orders","amr":["pin","hwk"],"authTime":"2026-01-01T00:00:00Z","exp":"2026-01-01T00:15:00Z"}`)
	c := client.New(srv.URL)
	r, err := c.FieldTokens().Introspect(context.Background(), "tok")
	require.NoError(t, err)
	assert.Equal(t, http.MethodPost, seen.method)
	assert.Equal(t, "/api/field-tokens/introspect", seen.path)
	assert.JSONEq(t, `{"token":"tok"}`, seen.body)
	assert.True(t, r.Active)
	assert.Equal(t, "w1", r.WorkerCode)
	assert.Equal(t, []string{"pin", "hwk"}, r.Amr)
	assert.Equal(t, "2026-01-01T00:15:00Z", r.Exp)

	srv2, _ := newMockSrv(t, `{"active":false}`)
	r, err = client.New(srv2.URL).FieldTokens().Introspect(context.Background(), "bad")
	require.NoError(t, err)
	assert.False(t, r.Active)
	assert.Empty(t, r.Sub)
}

func TestFieldWorkersEnsureUpdateIdentityFlags(t *testing.T) {
	srv, seen := newMockSrv(t, workerJSON)
	c := client.New(srv.URL)
	ctx := context.Background()
	_, err := c.FieldWorkers().Ensure(ctx, &client.EnsureFieldWorkerRequest{
		ClientID: "clt_1", WorkerCode: "w1", Name: "W", CanImpersonate: new(true), IsTest: new(true),
	})
	require.NoError(t, err)
	assert.JSONEq(t, `{"clientId":"clt_1","workerCode":"w1","name":"W","canImpersonate":true,"isTest":true}`, seen.body)
	_, err = c.FieldWorkers().Update(ctx, "fwk_1", &client.UpdateFieldWorkerRequest{Name: "W", IsTest: new(false)})
	require.NoError(t, err)
	assert.JSONEq(t, `{"name":"W","isTest":false}`, seen.body)
}

func TestFieldWorkersBadges(t *testing.T) {
	const badge = `{"id":"fbg_1","workerId":"fwk_1","label":"L","status":"ACTIVE","createdAt":"2026-01-01T00:00:00Z","createdBy":"prn_1"}`
	srv, seen := newMockSrv(t, `{"badge":`+badge+`,"token":"tok"}`)
	c := client.New(srv.URL)
	ctx := context.Background()

	r, err := c.FieldWorkers().IssueBadge(ctx, "fwk_1", &client.IssueFieldBadgeRequest{Label: "L"})
	require.NoError(t, err)
	assert.Equal(t, http.MethodPost, seen.method)
	assert.Equal(t, "/api/field-workers/fwk_1/badges", seen.path)
	assert.JSONEq(t, `{"label":"L"}`, seen.body)
	assert.Equal(t, "tok", r.Token)
	assert.Equal(t, "fbg_1", r.Badge.ID)

	srv2, seen2 := newMockSrv(t, `{"badges":[`+badge+`]}`)
	l, err := client.New(srv2.URL).FieldWorkers().ListBadges(ctx, "fwk_1")
	require.NoError(t, err)
	assert.Equal(t, http.MethodGet, seen2.method)
	assert.Equal(t, "/api/field-workers/fwk_1/badges", seen2.path)
	require.Len(t, l.Badges, 1)
	assert.Equal(t, "ACTIVE", l.Badges[0].Status)

	require.NoError(t, c.FieldWorkers().RevokeBadge(ctx, "fwk_1", "fbg_1"))
	assert.Equal(t, http.MethodPost, seen.method)
	assert.Equal(t, "/api/field-workers/fwk_1/badges/fbg_1/revoke", seen.path)

	srv3, seen3 := newMockSrv(t, `{"issued":[{"workerId":"fwk_1","workerCode":"w1","name":"W","badge":`+badge+`,"token":"t1"}]}`)
	b, err := client.New(srv3.URL).FieldWorkers().BulkIssueBadges(ctx, []string{"fwk_1"})
	require.NoError(t, err)
	assert.Equal(t, "/api/field-workers/badges", seen3.path)
	assert.JSONEq(t, `{"workerIds":["fwk_1"]}`, seen3.body)
	require.Len(t, b.Issued, 1)
	assert.Equal(t, "t1", b.Issued[0].Token)
	assert.Equal(t, "w1", b.Issued[0].WorkerCode)
}

func TestFieldWorkersCreateSignInCode(t *testing.T) {
	srv, seen := newMockSrv(t, `{"code":"ABC123","workerId":"fwk_1","appCode":"orders","expiresAt":"2026-01-01T00:05:00Z"}`)
	r, err := client.New(srv.URL).FieldWorkers().CreateSignInCode(context.Background(), "fwk_1", "orders")
	require.NoError(t, err)
	assert.Equal(t, http.MethodPost, seen.method)
	assert.Equal(t, "/api/field-workers/fwk_1/sign-in-codes", seen.path)
	assert.JSONEq(t, `{"appCode":"orders"}`, seen.body)
	assert.Equal(t, "ABC123", r.Code)
	assert.Equal(t, "2026-01-01T00:05:00Z", r.ExpiresAt)
}

func TestFieldWorkersLinkUnlink(t *testing.T) {
	linked := `{"id":"fwk_1","clientId":"clt_1","workerCode":"w1","name":"W","status":"ACTIVE","hasPin":true,"pinChangeRequired":false,"canImpersonate":false,"isTest":false,"linkedPrincipal":{"id":"prn_1","name":"Pat","email":"pat@x.io"},"badge":null,"locked":false,"hardLocked":false,"failedSinceSuccess":0,"apps":[],"createdAt":"2026-01-01T00:00:00Z","updatedAt":"2026-01-02T00:00:00Z"}`
	srv, seen := newMockSrv(t, linked)
	c := client.New(srv.URL)
	ctx := context.Background()
	r, err := c.FieldWorkers().Link(ctx, "fwk_1", "prn_1")
	require.NoError(t, err)
	assert.Equal(t, http.MethodPut, seen.method)
	assert.Equal(t, "/api/field-workers/fwk_1/link", seen.path)
	assert.JSONEq(t, `{"principalId":"prn_1"}`, seen.body)
	require.NotNil(t, r.LinkedPrincipal)
	assert.Equal(t, client.FieldLinkedPrincipal{ID: "prn_1", Name: "Pat", Email: "pat@x.io"}, *r.LinkedPrincipal)

	srv2, seen2 := newMockSrv(t, `{"id":"fwk_1","clientId":"clt_1","workerCode":"w1","name":"W","status":"ACTIVE","linkedPrincipal":null,"apps":[]}`)
	r, err = client.New(srv2.URL).FieldWorkers().Unlink(ctx, "fwk_1")
	require.NoError(t, err)
	assert.Equal(t, http.MethodDelete, seen2.method)
	assert.Equal(t, "/api/field-workers/fwk_1/link", seen2.path)
	assert.Nil(t, r.LinkedPrincipal)

	_, err = c.FieldWorkers().List(ctx, "clt_1", &client.FieldWorkerFilters{LinkedPrincipalID: "prn_1"})
	require.NoError(t, err)
	assert.Equal(t, "prn_1", seen.query.Get("linkedPrincipalId"))
}

func TestFieldTokensIntrospectLinkedPrincipal(t *testing.T) {
	srv, _ := newMockSrv(t, `{"active":true,"sub":"fwk_1","linkedPrincipalId":"prn_1"}`)
	r, err := client.New(srv.URL).FieldTokens().Introspect(context.Background(), "tok")
	require.NoError(t, err)
	assert.Equal(t, "prn_1", r.LinkedPrincipalID)
}
