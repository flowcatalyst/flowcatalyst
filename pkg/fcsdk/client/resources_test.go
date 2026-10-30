package client_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/go-sdk/client"
)

// newMockSrv records the last seen request and returns the body the
// caller supplies. URL = srv.URL.
func newMockSrv(t *testing.T, respJSON string) (*httptest.Server, *seenRequest) {
	t.Helper()
	seen := &seenRequest{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen.method = r.Method
		seen.path = r.URL.Path
		seen.query = r.URL.Query()
		if r.Body != nil {
			b, _ := io.ReadAll(r.Body)
			seen.body = string(b)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(respJSON))
	}))
	t.Cleanup(srv.Close)
	return srv, seen
}

type seenRequest struct {
	method string
	path   string
	query  url.Values
	body   string
}

func TestAuditLogsListBuildsQuery(t *testing.T) {
	srv, seen := newMockSrv(t, `{"auditLogs":[],"hasMore":false}`)
	c := client.New(srv.URL)

	pageSize := uint32(25)
	_, err := c.AuditLogs().List(context.Background(), &client.AuditLogFilters{
		EntityType:     "Principal",
		Operation:      "ROLE_ASSIGNED",
		ApplicationIDs: []string{"app_1", "app_2"},
		ClientIDs:      []string{"clt_1"},
		After:          "cursor-1",
		PageSize:       &pageSize,
	})
	require.NoError(t, err)
	assert.Equal(t, http.MethodGet, seen.method)
	assert.Equal(t, "/api/audit-logs", seen.path)
	assert.Equal(t, "Principal", seen.query.Get("entityType"))
	assert.Equal(t, "ROLE_ASSIGNED", seen.query.Get("operation"))
	assert.Equal(t, "app_1,app_2", seen.query.Get("applicationIds"))
	assert.Equal(t, "clt_1", seen.query.Get("clientIds"))
	assert.Equal(t, "cursor-1", seen.query.Get("after"))
	assert.Equal(t, "25", seen.query.Get("pageSize"))
	assert.False(t, seen.query.Has("page"))
}

func TestAuditLogsGetDeserializes(t *testing.T) {
	srv, _ := newMockSrv(t, `{"id":"aud_1","operation":"CREATE","entityType":"Principal","entityId":"prn_1","performedAt":"2026-01-01T00:00:00Z"}`)
	c := client.New(srv.URL)

	r, err := c.AuditLogs().Get(context.Background(), "aud_1")
	require.NoError(t, err)
	assert.Equal(t, "aud_1", r.ID)
	assert.Equal(t, "CREATE", r.Operation)
	assert.Equal(t, "Principal", r.EntityType)
}

func TestPermissionsListDeserializes(t *testing.T) {
	srv, seen := newMockSrv(t, `{"permissions":[{"permission":"orders:read","application":"orders","context":"client","aggregate":"Order","action":"read","description":"Read orders"}],"total":1}`)
	c := client.New(srv.URL)

	r, err := c.Permissions().List(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "/api/roles/permissions", seen.path)
	require.Len(t, r.Permissions, 1)
	assert.Equal(t, "orders:read", r.Permissions[0].Permission)
}

func TestMeClientApplicationsBuildsPath(t *testing.T) {
	srv, seen := newMockSrv(t, `{"applications":[],"clientId":"clt_1"}`)
	c := client.New(srv.URL)

	_, err := c.Me().ClientApplications(context.Background(), "clt_1")
	require.NoError(t, err)
	assert.Equal(t, "/api/me/clients/clt_1/applications", seen.path)
}

func TestClientsCreateSendsBody(t *testing.T) {
	srv, seen := newMockSrv(t, `{"id":"clt_new"}`)
	c := client.New(srv.URL)

	r, err := c.Clients().Create(context.Background(), &client.CreateClientRequest{
		Identifier: "acme", Name: "Acme",
	})
	require.NoError(t, err)
	assert.Equal(t, "clt_new", r.ID)
	assert.Equal(t, http.MethodPost, seen.method)
	var body client.CreateClientRequest
	require.NoError(t, json.Unmarshal([]byte(seen.body), &body))
	assert.Equal(t, "acme", body.Identifier)
}

func TestClientsSearchEncodesQ(t *testing.T) {
	srv, seen := newMockSrv(t, `{"clients":[]}`)
	c := client.New(srv.URL)

	_, err := c.Clients().Search(context.Background(), "ac me")
	require.NoError(t, err)
	assert.Equal(t, "/api/clients/search", seen.path)
	assert.Equal(t, "ac me", seen.query.Get("q"))
}

func TestConnectionsListEncodesFilters(t *testing.T) {
	srv, seen := newMockSrv(t, `{"connections":[]}`)
	c := client.New(srv.URL)

	_, err := c.Connections().List(context.Background(), "clt_1", "ACTIVE")
	require.NoError(t, err)
	assert.Equal(t, "/api/connections", seen.path)
	assert.Equal(t, "clt_1", seen.query.Get("clientId"))
	assert.Equal(t, "ACTIVE", seen.query.Get("status"))
	assert.Empty(t, seen.query.Get("serviceAccountId"))
}

func TestConnectionsPauseIsPost(t *testing.T) {
	srv, seen := newMockSrv(t, `{"id":"con_1","code":"c","name":"n","status":"PAUSED","serviceAccountId":"sa_1","source":"UI","createdAt":"","updatedAt":""}`)
	c := client.New(srv.URL)

	r, err := c.Connections().Pause(context.Background(), "con_1")
	require.NoError(t, err)
	assert.Equal(t, "PAUSED", r.Status)
	assert.Equal(t, http.MethodPost, seen.method)
	assert.Equal(t, "/api/connections/con_1/pause", seen.path)
}

func TestRouterUsesRouterBaseURL(t *testing.T) {
	apiSrv, _ := newMockSrv(t, `{"messageId":"msg_1","inPipeline":false}`)
	var routerSeen *seenRequest
	routerSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		routerSeen = &seenRequest{method: r.Method, path: r.URL.Path, query: r.URL.Query()}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"messageId":"msg_1","inPipeline":false}`))
	}))
	defer routerSrv.Close()

	c := client.New(apiSrv.URL, client.WithRouterBaseURL(routerSrv.URL))
	_, err := c.Router().InPipeline(context.Background(), "msg_1")
	require.NoError(t, err)
	require.NotNil(t, routerSeen)
	assert.Equal(t, http.MethodGet, routerSeen.method)
	assert.Equal(t, "/monitoring/in-flight-messages/check", routerSeen.path)
	assert.Equal(t, "msg_1", routerSeen.query.Get("messageId"))
}

func TestRouterInPipelineBatch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/monitoring/in-flight-messages/check-batch", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"msg_1":true,"msg_2":false}`))
	}))
	defer srv.Close()
	c := client.New(srv.URL) // single URL: router falls back to base

	out, err := c.Router().InPipelineBatch(context.Background(), []string{"msg_1", "msg_2"})
	require.NoError(t, err)
	assert.True(t, out["msg_1"])
	assert.False(t, out["msg_2"])
}

func TestScheduledJobsCreateAndFire(t *testing.T) {
	srv, seen := newMockSrv(t, `{"id":"sjb_1"}`)
	c := client.New(srv.URL)

	_, err := c.ScheduledJobs().Create(context.Background(), &client.CreateScheduledJobRequest{
		Code: "daily-sync", Name: "Daily Sync", Crons: []string{"0 0 0 * * *"},
	})
	require.NoError(t, err)
	assert.Equal(t, "/api/scheduled-jobs", seen.path)
	var body client.CreateScheduledJobRequest
	require.NoError(t, json.Unmarshal([]byte(seen.body), &body))
	assert.Equal(t, []string{"0 0 0 * * *"}, body.Crons)
}

func TestScheduledJobsListPaginatedShape(t *testing.T) {
	srv, _ := newMockSrv(t, `{"data":[{"id":"sjb_1","code":"x","name":"X","status":"ACTIVE","crons":["0 0 * * * *"],"timezone":"UTC","concurrent":false,"tracksCompletion":false,"deliveryMaxAttempts":3,"createdAt":"","updatedAt":"","version":1}],"page":1,"size":50,"total":1,"totalPages":1}`)
	c := client.New(srv.URL)

	r, err := c.ScheduledJobs().List(context.Background(), nil)
	require.NoError(t, err)
	assert.Equal(t, uint32(1), r.Page)
	require.Len(t, r.Data, 1)
	assert.Equal(t, "sjb_1", r.Data[0].ID)
}

func TestOpenAPISyncPostsSpec(t *testing.T) {
	srv, seen := newMockSrv(t, `{"applicationCode":"orders","specId":"sp_1","version":"1.0","status":"PUBLISHED","hasBreaking":false,"unchanged":false}`)
	c := client.New(srv.URL)

	spec := json.RawMessage(`{"openapi":"3.0.0","info":{"title":"Orders","version":"1.0.0"},"paths":{}}`)
	r, err := c.OpenAPI().Sync(context.Background(), "orders", spec)
	require.NoError(t, err)
	assert.Equal(t, "/api/applications/orders/openapi/sync", seen.path)
	assert.Equal(t, "PUBLISHED", r.Status)

	var body client.SyncOpenAPISpecRequest
	require.NoError(t, json.Unmarshal([]byte(seen.body), &body))
	assert.Contains(t, string(body.Spec), `"openapi":"3.0.0"`)
}

// ─── B: deferred methods on existing resources ───────────────────────

func TestApplicationsProvisionServiceAccount(t *testing.T) {
	srv, seen := newMockSrv(t, `{"message":"ok","serviceAccount":{"name":"Svc","principalId":"prn_svc","oauthClient":{"id":"oc_1","clientId":"cid","clientSecret":"s3cret"}}}`)
	c := client.New(srv.URL)

	r, err := c.Applications().ProvisionServiceAccount(context.Background(), "app_1")
	require.NoError(t, err)
	assert.Equal(t, "/api/applications/app_1/provision-service-account", seen.path)
	assert.Equal(t, http.MethodPost, seen.method)
	assert.Equal(t, "prn_svc", r.ServiceAccount.PrincipalID)
	assert.Equal(t, "s3cret", r.ServiceAccount.OAuthClient.ClientSecret)
}

func TestApplicationsListRolesUsesByID(t *testing.T) {
	srv, seen := newMockSrv(t, `{"roles":["orders:admin"]}`)
	c := client.New(srv.URL)

	r, err := c.Applications().ListRoles(context.Background(), "app_1")
	require.NoError(t, err)
	assert.Equal(t, "/api/applications/by-id/app_1/roles", seen.path)
	require.Len(t, r, 1)
	assert.Equal(t, "orders:admin", r[0])
}

func TestApplicationsEnableForClient(t *testing.T) {
	srv, seen := newMockSrv(t, ``)
	c := client.New(srv.URL)

	require.NoError(t, c.Applications().EnableForClient(context.Background(), "app_1", "clt_1"))
	assert.Equal(t, "/api/applications/app_1/clients/clt_1/enable", seen.path)
}

func TestPrincipalsSyncUsersPostsAppLessWithPasswordHash(t *testing.T) {
	srv, seen := newMockSrv(t, `{"created":1,"updated":0,"deleted":0,"syncedEmails":["a@example.com"]}`)
	c := client.New(srv.URL)

	hash := "$2y$10$abcdefghijklmnopqrstuv"
	res, err := c.Principals().SyncUsers(context.Background(), &client.SyncPrincipalsRequest{
		Principals: []client.SyncPrincipalItem{
			{Email: "a@example.com", Name: "A", Active: true, PasswordHash: &hash},
			{Email: "b@example.com", Name: "B", Active: true}, // no hash
		},
	})
	require.NoError(t, err)
	require.NotNil(t, res)

	// App-less: posts to /api/principals/sync with no appCode anywhere.
	assert.Equal(t, http.MethodPost, seen.method)
	assert.Equal(t, "/api/principals/sync", seen.path)
	assert.Contains(t, seen.body, `"passwordHash":"$2y$10$abcdefghijklmnopqrstuv"`)

	// The hash-less entry must omit passwordHash (omitempty) so a roles-only
	// user never carries a null password over the wire.
	var parsed struct {
		Principals []map[string]any `json:"principals"`
	}
	require.NoError(t, json.Unmarshal([]byte(seen.body), &parsed))
	require.Len(t, parsed.Principals, 2)
	_, hasField := parsed.Principals[1]["passwordHash"]
	assert.False(t, hasField, "entry without a hash omits passwordHash")
}

func TestPrincipalsAddRoleSendsRoleField(t *testing.T) {
	srv, seen := newMockSrv(t, `{"id":"prn_1","type":"USER","scope":"CLIENT","name":"A","active":true,"roles":["orders:admin"],"isAnchorUser":false,"grantedClientIds":[],"hasDeveloperCredential":false,"createdAt":"","updatedAt":""}`)
	c := client.New(srv.URL)

	p, err := c.Principals().AddRole(context.Background(), "prn_1", "orders:admin")
	require.NoError(t, err)
	assert.Equal(t, []string{"orders:admin"}, p.Roles)
	assert.Equal(t, "/api/principals/prn_1/roles", seen.path)
	assert.Equal(t, http.MethodPost, seen.method)
	var body client.AssignRoleRequest
	require.NoError(t, json.Unmarshal([]byte(seen.body), &body))
	assert.Equal(t, "orders:admin", body.Role)
}

func TestPrincipalsSetRolesReplaces(t *testing.T) {
	srv, seen := newMockSrv(t, `{"added":["orders:viewer"],"removed":[],"roles":[{"id":"r1","roleName":"orders:admin","assignmentSource":"MANUAL","assignedAt":"2026-01-01T00:00:00Z"}]}`)
	c := client.New(srv.URL)

	res, err := c.Principals().SetRoles(context.Background(), "prn_1", []string{"orders:admin", "orders:viewer"})
	require.NoError(t, err)
	assert.Equal(t, []string{"orders:viewer"}, res.Added)
	assert.Equal(t, http.MethodPut, seen.method)
	assert.Equal(t, "/api/principals/prn_1/roles", seen.path)
	var body client.ReplaceRolesRequest
	require.NoError(t, json.Unmarshal([]byte(seen.body), &body))
	assert.Equal(t, []string{"orders:admin", "orders:viewer"}, body.Roles)
}

func TestPrincipalsGrantAndRevokeClientAccess(t *testing.T) {
	srv, seen := newMockSrv(t, `{"id":"g1","clientId":"clt_x","grantedAt":"2026-01-01T00:00:00Z"}`)
	c := client.New(srv.URL)

	grant, err := c.Principals().GrantClientAccess(context.Background(), "prn_1", "clt_x")
	require.NoError(t, err)
	assert.Equal(t, "clt_x", grant.ClientID)
	assert.Equal(t, http.MethodPost, seen.method)
	assert.Equal(t, "/api/principals/prn_1/client-access", seen.path)
	var body client.GrantClientAccessRequest
	require.NoError(t, json.Unmarshal([]byte(seen.body), &body))
	assert.Equal(t, "clt_x", body.ClientID)

	require.NoError(t, c.Principals().RevokeClientAccess(context.Background(), "prn_1", "clt_x"))
	assert.Equal(t, http.MethodDelete, seen.method)
	assert.Equal(t, "/api/principals/prn_1/client-access/clt_x", seen.path)
}

func TestRolesGrantAndRevokePermission(t *testing.T) {
	srv, seen := newMockSrv(t, `{"id":"rol_1","name":"orders:admin","shortName":"admin","displayName":"Admin","applicationCode":"orders","source":"PLATFORM","permissions":["orders:read"],"createdAt":"","updatedAt":""}`)
	c := client.New(srv.URL)

	r, err := c.Roles().GrantPermission(context.Background(), "orders:admin", "orders:read")
	require.NoError(t, err)
	assert.Equal(t, http.MethodPost, seen.method)
	assert.Equal(t, "/api/roles/orders:admin/permissions", seen.path)
	assert.Equal(t, []string{"orders:read"}, r.Permissions)

	_, err = c.Roles().RevokePermission(context.Background(), "orders:admin", "orders:read")
	require.NoError(t, err)
	assert.Equal(t, http.MethodDelete, seen.method)
	assert.Equal(t, "/api/roles/orders:admin/permissions/orders:read", seen.path)
}

func TestRolesListForApplication(t *testing.T) {
	srv, seen := newMockSrv(t, `[{"id":"rol_1","name":"orders:admin","displayName":"Admin","applicationCode":"orders","clientManaged":false,"permissions":[],"source":"PLATFORM","createdAt":"","updatedAt":""}]`)
	c := client.New(srv.URL)

	r, err := c.Roles().ListForApplication(context.Background(), "app_1")
	require.NoError(t, err)
	assert.Equal(t, "/api/roles/by-application/app_1", seen.path)
	require.Len(t, r, 1)
	assert.Equal(t, "orders:admin", r[0].Name)
}

// GET /api/scheduled-jobs/instances/{id}/logs answers a bare array of log
// lines (the platform's []ScheduledJobInstanceLogResponse). Before the fix
// ListInstanceLogs decoded it into a {logs,total} object and failed.
func TestScheduledJobsListInstanceLogsDecodesTheArray(t *testing.T) {
	srv, seen := newMockSrv(t, `[{"id":"sjl_1","instanceId":"sji_1","scheduledJobId":"sjb_1","level":"INFO","message":"starting","metadata":{"n":1},"createdAt":"2026-10-12T09:30:00.123456Z"},`+
		`{"id":"sjl_2","instanceId":"sji_1","level":"ERROR","message":"failed","createdAt":"2026-10-12T09:30:01.000000Z"}]`)
	c := client.New(srv.URL)

	r, err := c.ScheduledJobs().ListInstanceLogs(context.Background(), "sji_1")
	require.NoError(t, err)
	assert.Equal(t, "/api/scheduled-jobs/instances/sji_1/logs", seen.path)
	require.Len(t, r.Logs, 2)
	assert.Equal(t, uint64(2), r.Total)
	assert.Equal(t, "starting", r.Logs[0].Message)
	assert.Equal(t, "sjb_1", r.Logs[0].ScheduledJobID)
	assert.JSONEq(t, `{"n":1}`, string(r.Logs[0].Metadata))
	assert.Equal(t, "ERROR", r.Logs[1].Level)
}

// LogForInstance sends INFO when the request has no level: the platform
// requires one.
func TestScheduledJobsLogForInstanceDefaultsTheLevel(t *testing.T) {
	srv, seen := newMockSrv(t, ``)
	c := client.New(srv.URL)

	err := c.ScheduledJobs().LogForInstance(context.Background(), "sji_1", &client.InstanceLogRequest{Message: "hello"})
	require.NoError(t, err)
	assert.Equal(t, "/api/scheduled-jobs/instances/sji_1/log", seen.path)
	assert.JSONEq(t, `{"message":"hello","level":"INFO"}`, seen.body)
}
