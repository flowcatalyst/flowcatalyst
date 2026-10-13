package client_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/client"
)

func TestDispatchPoolsListDecodesPoolsAndTotal(t *testing.T) {
	srv, seen := newMockSrv(t, `{"pools":[{"id":"dp_1","code":"c","name":"n","concurrency":5,"rateLimit":60,"status":"ACTIVE","clientIdentifier":"acme","createdAt":"2026-01-01T00:00:00Z","updatedAt":"2026-01-01T00:00:00Z"}],"total":1}`)
	c := client.New(srv.URL)

	r, err := c.DispatchPools().List(context.Background(), "clt_1", "ACTIVE")
	require.NoError(t, err)
	assert.Equal(t, "clt_1", seen.query.Get("clientId"))
	assert.Equal(t, "ACTIVE", seen.query.Get("status"))
	require.Len(t, r.Pools, 1)
	assert.Equal(t, uint64(1), r.Total)
	require.NotNil(t, r.Pools[0].RateLimit)
	assert.Equal(t, uint32(60), *r.Pools[0].RateLimit)
	assert.Equal(t, "acme", r.Pools[0].ClientIdentifier)
}

func TestAuditLogsListDecodesCursor(t *testing.T) {
	srv, _ := newMockSrv(t, `{"auditLogs":[{"id":"aud_1","operation":"CREATE","entityType":"Principal","entityId":"prn_1","operationJson":"{}","performedAt":"2026-01-01T00:00:00Z"}],"hasMore":true,"nextCursor":"cur-2"}`)
	c := client.New(srv.URL)

	r, err := c.AuditLogs().List(context.Background(), nil)
	require.NoError(t, err)
	assert.True(t, r.HasMore)
	assert.Equal(t, "cur-2", r.NextCursor)
	require.Len(t, r.AuditLogs, 1)
	assert.Equal(t, "{}", r.AuditLogs[0].OperationJSON)
}

func TestEventTypesGetDecodesEventName(t *testing.T) {
	srv, _ := newMockSrv(t, `{"id":"et_1","code":"a:b:c:d","name":"n","status":"CURRENT","application":"a","subdomain":"b","aggregate":"c","eventName":"d","source":"SDK","clientId":"clt_1","createdBy":"prn_1","specVersions":[{"version":"1.0","status":"FINALISED","schema":{"type":"object"},"createdAt":"2026-01-01T00:00:00Z"}],"createdAt":"2026-01-01T00:00:00Z","updatedAt":"2026-01-01T00:00:00Z"}`)
	c := client.New(srv.URL)

	r, err := c.EventTypes().Get(context.Background(), "et_1")
	require.NoError(t, err)
	assert.Equal(t, "d", r.EventName)
	assert.Equal(t, "SDK", r.Source)
	assert.Equal(t, "clt_1", r.ClientID)
	require.Len(t, r.SpecVersions, 1)
	assert.Equal(t, "2026-01-01T00:00:00Z", r.SpecVersions[0].CreatedAt)
}

func TestPermissionsGetDecodesNameAndCategory(t *testing.T) {
	srv, _ := newMockSrv(t, `{"permission":"orders:sales:order:view","name":"View order","category":"sales","description":"d"}`)
	c := client.New(srv.URL)

	r, err := c.Permissions().Get(context.Background(), "orders:sales:order:view")
	require.NoError(t, err)
	assert.Equal(t, "View order", r.Name)
	assert.Equal(t, "sales", r.Category)
}

func TestScheduledJobsListDecodesTotalPages(t *testing.T) {
	srv, _ := newMockSrv(t, `{"data":[],"page":1,"size":20,"total":41,"total_pages":3}`)
	c := client.New(srv.URL)

	r, err := c.ScheduledJobs().List(context.Background(), nil)
	require.NoError(t, err)
	assert.Equal(t, uint32(3), r.TotalPages)
}

func TestApplicationsListClientsDecodesItems(t *testing.T) {
	srv, _ := newMockSrv(t, `{"items":[{"id":"cfg_1","applicationId":"app_1","clientId":"clt_1","enabled":true,"configJson":{"a":1},"createdAt":"2026-01-01T00:00:00Z","updatedAt":"2026-01-01T00:00:00Z"}]}`)
	c := client.New(srv.URL)

	r, err := c.Applications().ListClients(context.Background(), "app_1")
	require.NoError(t, err)
	require.Len(t, r.ClientConfigs, 1)
	assert.JSONEq(t, `{"a":1}`, string(r.ClientConfigs[0].ConfigJSON))
}

func TestPrincipalsFindByEmailSendsQAndNarrowsToExactMatch(t *testing.T) {
	srv, seen := newMockSrv(t, `{"principals":[`+
		`{"id":"prn_1","type":"USER","scope":"CLIENT","name":"A","active":true,"email":"a@x.io","hasDeveloperCredential":false,"createdAt":"","updatedAt":""},`+
		`{"id":"prn_2","type":"USER","scope":"CLIENT","name":"B","active":true,"email":"aa@x.io","hasDeveloperCredential":false,"createdAt":"","updatedAt":""}],"total":2}`)
	c := client.New(srv.URL)

	r, err := c.Principals().FindByEmail(context.Background(), "A@X.io")
	require.NoError(t, err)
	assert.Equal(t, "A@X.io", seen.query.Get("q"))
	assert.False(t, seen.query.Has("email"))
	require.Len(t, r.Principals, 1)
	assert.Equal(t, "prn_1", r.Principals[0].ID)
	assert.Equal(t, uint64(1), r.Total)
}

func TestPrincipalsCreateUserSendsInviteFields(t *testing.T) {
	srv, seen := newMockSrv(t, `{"id":"prn_1","type":"USER","scope":"ANCHOR","name":"N","active":true,"hasDeveloperCredential":false,"inviteLink":"https://x/invite","createdAt":"","updatedAt":""}`)
	c := client.New(srv.URL)

	yes := true
	r, err := c.Principals().CreateUser(context.Background(), &client.CreateUserRequest{
		Email: "a@x.io", Name: "N", Scope: "ANCHOR",
		SendInvitation: &yes, ReturnInviteLink: &yes, InviteRedirectURI: "https://app/cb",
	})
	require.NoError(t, err)
	assert.Equal(t, "https://x/invite", r.InviteLink)

	var body map[string]any
	require.NoError(t, json.Unmarshal([]byte(seen.body), &body))
	assert.Equal(t, "ANCHOR", body["scope"])
	assert.Equal(t, true, body["sendInvitation"])
	assert.Equal(t, true, body["returnInviteLink"])
	assert.Equal(t, "https://app/cb", body["inviteRedirectUri"])
}

func TestPrincipalsUpdateSendsEmailNotFirstLast(t *testing.T) {
	srv, seen := newMockSrv(t, `{"id":"prn_1","type":"USER","scope":"CLIENT","name":"N","active":true,"hasDeveloperCredential":false,"createdAt":"","updatedAt":""}`)
	c := client.New(srv.URL)

	email := "new@x.io"
	_, err := c.Principals().Update(context.Background(), "prn_1", &client.UpdatePrincipalRequest{Email: &email})
	require.NoError(t, err)
	assert.JSONEq(t, `{"email":"new@x.io"}`, seen.body)
}

func TestSubscriptionsUpdateSendsReplaceableMembers(t *testing.T) {
	srv, seen := newMockSrv(t, `{}`)
	c := client.New(srv.URL)

	pool := "dp_1"
	bindings := []client.EventTypeBinding{{EventTypeCode: "a:b:c:d", SpecVersion: "1.0"}}
	err := c.Subscriptions().Update(context.Background(), "sub_1", &client.UpdateSubscriptionRequest{
		DispatchPoolID: &pool, EventTypes: &bindings,
	})
	require.NoError(t, err)
	assert.JSONEq(t, `{"dispatchPoolId":"dp_1","eventTypes":[{"eventTypeCode":"a:b:c:d","specVersion":"1.0"}]}`, seen.body)
}

func TestRolesUpdateSendsPermissions(t *testing.T) {
	srv, seen := newMockSrv(t, ``)
	c := client.New(srv.URL)

	err := c.Roles().Update(context.Background(), "orders:admin", &client.UpdateRoleRequest{Permissions: []string{"a:b:c:d"}})
	require.NoError(t, err)
	assert.JSONEq(t, `{"permissions":["a:b:c:d"]}`, seen.body)
}
