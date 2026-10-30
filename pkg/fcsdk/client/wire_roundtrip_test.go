package client_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/go-sdk/client"
)

// assertWireRoundTrip decodes a full platform payload (every member of the
// OpenAPI schema, populated) into dst and re-encodes it. If the struct is
// missing a member the platform sends, or carries a member under a different
// name or type, the re-encoded JSON differs from the payload. Payloads are
// built from the schemas in api/openapi.lock.json.
func assertWireRoundTrip(t *testing.T, dst any, payload string) {
	t.Helper()
	require.NoError(t, json.Unmarshal([]byte(payload), dst), "payload must decode into the struct")
	got, err := json.Marshal(dst)
	require.NoError(t, err)
	assert.JSONEq(t, payload, string(got))
}

func TestWireRoundTrip(t *testing.T) {
	cases := []struct {
		name    string
		dst     any
		payload string
	}{
		{
			"CreateApplicationRequest", &client.CreateApplicationRequest{},
			`{"code":"orders","name":"Orders","description":"d","type":"APPLICATION","defaultBaseUrl":"https://o","iconUrl":"https://i","logo":"<svg/>","logoMimeType":"image/svg+xml","website":"https://w"}`,
		},
		{
			"UpdateApplicationRequest", &client.UpdateApplicationRequest{},
			`{"name":"Orders","description":"d","defaultBaseUrl":"https://o","iconUrl":"https://i","logo":"<svg/>","logoMimeType":"image/svg+xml","website":"https://w"}`,
		},
		{
			"ApplicationResponse", &client.ApplicationResponse{},
			`{"id":"app_1","code":"orders","name":"Orders","description":"d","type":"APPLICATION","defaultBaseUrl":"https://o","iconUrl":"https://i","logo":"<svg/>","logoMimeType":"image/svg+xml","website":"https://w","serviceAccountId":"sa_1","hasLoginClient":true,"active":true,"createdAt":"2026-01-01T00:00:00Z","updatedAt":"2026-01-02T00:00:00Z"}`,
		},
		{
			"ServiceAccountResponse", &client.ServiceAccountResponse{},
			`{"id":"sa_1","code":"c","name":"n","description":"d","active":true,"applicationId":"app_1","authType":"BEARER_TOKEN","clientIds":["clt_1"],"lastUsedAt":"2026-01-03T00:00:00Z","oauthClientId":"oc_1","principalId":"prn_1","roles":["orders:admin"],"scope":"CLIENT","createdAt":"2026-01-01T00:00:00Z","updatedAt":"2026-01-02T00:00:00Z"}`,
		},
		{
			"ClientConfigResponse", &client.ClientConfigResponse{},
			`{"id":"cfg_1","applicationId":"app_1","clientId":"clt_1","enabled":true,"baseUrlOverride":"https://x","configJson":{"a":1},"createdAt":"2026-01-01T00:00:00Z","updatedAt":"2026-01-02T00:00:00Z"}`,
		},
		{"CreatedResponse", &client.CreatedResponse{}, `{"id":"x_1"}`},
		{
			"AuditLogResponse", &client.AuditLogResponse{},
			`{"id":"aud_1","operation":"CREATE","operationJson":"{\"a\":1}","entityType":"Principal","entityId":"prn_1","principalId":"prn_2","principalName":"P","applicationId":"app_1","clientId":"clt_1","performedAt":"2026-01-01T00:00:00Z"}`,
		},
		{
			"AuditLogListResponse", &client.AuditLogListResponse{},
			`{"auditLogs":[{"id":"aud_1","operation":"CREATE","entityType":"Principal","entityId":"prn_1","performedAt":"2026-01-01T00:00:00Z"}],"hasMore":true,"nextCursor":"abc"}`,
		},
		{
			"ClientResponse", &client.ClientResponse{},
			`{"id":"clt_1","name":"N","identifier":"n","status":"SUSPENDED","statusReason":"r","statusChangedAt":"2026-01-02T00:00:00Z","notes":[{"text":"t","category":"GENERAL","addedBy":"prn_1","addedAt":"2026-01-02T00:00:00Z"}],"createdAt":"2026-01-01T00:00:00Z","updatedAt":"2026-01-02T00:00:00Z"}`,
		},
		{
			"CreateConnectionRequest", &client.CreateConnectionRequest{},
			`{"code":"c","name":"n","description":"d","serviceAccountId":"sa_1","externalId":"e","clientId":"clt_1","applicationCode":"orders"}`,
		},
		{
			"UpdateConnectionRequest", &client.UpdateConnectionRequest{},
			`{"name":"n","description":"d","externalId":"e","status":"ACTIVE","applicationCode":"orders"}`,
		},
		{
			"ConnectionResponse", &client.ConnectionResponse{},
			`{"id":"con_1","code":"c","name":"n","description":"d","externalId":"e","status":"ACTIVE","source":"SDK","serviceAccountId":"sa_1","clientId":"clt_1","clientIdentifier":"acme","applicationCode":"orders","createdAt":"2026-01-01T00:00:00Z","updatedAt":"2026-01-02T00:00:00Z"}`,
		},
		{
			"CreateDispatchPoolRequest", &client.CreateDispatchPoolRequest{},
			`{"code":"c","name":"n","description":"d","concurrency":5,"rateLimit":60,"clientId":"clt_1"}`,
		},
		{
			"UpdateDispatchPoolRequest", &client.UpdateDispatchPoolRequest{},
			`{"name":"n","description":"d","concurrency":5,"rateLimit":60}`,
		},
		{
			"DispatchPoolResponse", &client.DispatchPoolResponse{},
			`{"id":"dp_1","code":"c","name":"n","description":"d","concurrency":5,"rateLimit":60,"status":"ACTIVE","clientId":"clt_1","clientIdentifier":"acme","createdAt":"2026-01-01T00:00:00Z","updatedAt":"2026-01-02T00:00:00Z"}`,
		},
		{
			"DispatchPoolListResponse", &client.DispatchPoolListResponse{},
			`{"pools":[{"id":"dp_1","code":"c","name":"n","concurrency":5,"status":"ACTIVE","createdAt":"2026-01-01T00:00:00Z","updatedAt":"2026-01-02T00:00:00Z"}],"total":1}`,
		},
		{
			"CreateEventTypeRequest", &client.CreateEventTypeRequest{},
			`{"code":"a:b:c:d","name":"n","description":"d","schema":{"type":"object"},"clientId":"clt_1","clientScoped":true}`,
		},
		{
			"UpdateEventTypeRequest", &client.UpdateEventTypeRequest{},
			`{"name":"n","description":"d","clientScoped":true}`,
		},
		{
			"EventTypeResponse", &client.EventTypeResponse{},
			`{"id":"et_1","code":"a:b:c:d","name":"n","description":"d","status":"CURRENT","application":"a","subdomain":"b","aggregate":"c","eventName":"d","source":"SDK","clientId":"clt_1","createdBy":"prn_1","specVersions":[{"version":"1.0","status":"FINALISED","schema":{"type":"object"},"createdAt":"2026-01-01T00:00:00Z"}],"createdAt":"2026-01-01T00:00:00Z","updatedAt":"2026-01-02T00:00:00Z"}`,
		},
		{
			"PermissionResponse", &client.PermissionResponse{},
			`{"permission":"orders:sales:order:view","name":"View","category":"sales","description":"d"}`,
		},
		{
			"CreateUserRequest", &client.CreateUserRequest{},
			`{"email":"a@b.c","name":"n","password":"pw","clientId":"clt_1","scope":"CLIENT","enforcePasswordComplexity":false,"sendInvitation":true,"returnInviteLink":true,"inviteRedirectUri":"https://app/cb"}`,
		},
		{
			"UpdatePrincipalRequest", &client.UpdatePrincipalRequest{},
			`{"name":"n","email":"a@b.c","active":true}`,
		},
		{
			"PrincipalResponse", &client.PrincipalResponse{},
			`{"id":"prn_1","type":"USER","scope":"CLIENT","clientId":"clt_1","name":"n","active":true,"email":"a@b.c","idpType":"INTERNAL","roles":["r"],"isAnchorUser":true,"grantedClientIds":["clt_2"],"serviceAccountId":"sa_1","hasDeveloperCredential":true,"developerCredentialUpdatedAt":"2026-01-02T00:00:00Z","twoFactorMethods":["TOTP"],"inviteLink":"https://x","createdAt":"2026-01-01T00:00:00Z","updatedAt":"2026-01-02T00:00:00Z"}`,
		},
		{
			"UpdateRoleRequest", &client.UpdateRoleRequest{},
			`{"displayName":"d","description":"x","permissions":["a:b:c:d"],"clientManaged":true}`,
		},
		{
			"RoleResponse", &client.RoleResponse{},
			`{"id":"rol_1","name":"orders:admin","displayName":"d","description":"x","applicationCode":"orders","applicationId":"app_1","permissions":["a:b:c:d"],"source":"SDK","clientManaged":true,"createdAt":"2026-01-01T00:00:00Z","updatedAt":"2026-01-02T00:00:00Z"}`,
		},
		{
			"ScheduledJobResponse", &client.ScheduledJobResponse{},
			`{"id":"sj_1","clientId":"clt_1","applicationId":"app_1","code":"c","name":"n","description":"d","status":"ACTIVE","crons":["* * * * *"],"timezone":"UTC","payload":{"a":1},"concurrent":true,"tracksCompletion":true,"timeoutSeconds":30,"deliveryMaxAttempts":3,"targetUrl":"https://t","lastFiredAt":"2026-01-02T00:00:00Z","hasActiveInstance":true,"version":2,"createdBy":"prn_1","updatedBy":"prn_2","createdAt":"2026-01-01T00:00:00Z","updatedAt":"2026-01-02T00:00:00Z"}`,
		},
		{
			"ScheduledJobListResponse", &client.ScheduledJobListResponse{},
			`{"data":[],"page":1,"size":20,"total":41,"total_pages":3}`,
		},
		{
			"CreateSubscriptionRequest", &client.CreateSubscriptionRequest{},
			`{"code":"c","name":"n","description":"d","endpoint":"https://e","connectionId":"con_1","clientId":"clt_1","clientScoped":true,"eventTypes":[{"eventTypeCode":"a:b:c:d","eventTypeId":"et_1","filter":"f","specVersion":"1.0"}],"customConfig":[{"key":"k","value":"v"}],"dispatchPoolId":"dp_1","serviceAccountId":"sa_1","mode":"IMMEDIATE","timeoutSeconds":30,"maxRetries":3,"maxAgeSeconds":60,"delaySeconds":5,"sequence":2,"queue":"q","dataOnly":true}`,
		},
		{
			"UpdateSubscriptionRequest", &client.UpdateSubscriptionRequest{},
			`{"name":"n","description":"d","endpoint":"https://e","connectionId":"con_1","eventTypes":[{"eventTypeCode":"a:b:c:d"}],"customConfig":[{"key":"k","value":"v"}],"dispatchPoolId":"dp_1","serviceAccountId":"sa_1","mode":"IMMEDIATE","timeoutSeconds":30,"maxRetries":3,"maxAgeSeconds":60,"delaySeconds":5,"queue":"q","dataOnly":true}`,
		},
		{
			"SubscriptionResponse", &client.SubscriptionResponse{},
			`{"id":"sub_1","code":"c","name":"n","description":"d","clientId":"clt_1","clientIdentifier":"acme","clientScoped":true,"applicationCode":"orders","eventTypes":[{"eventTypeCode":"a:b:c:d","eventTypeId":"et_1","filter":"f","specVersion":"1.0"}],"endpoint":"https://e","connectionId":"con_1","queue":"q","source":"SDK","status":"ACTIVE","maxAgeSeconds":60,"dispatchPoolId":"dp_1","dispatchPoolCode":"p","delaySeconds":5,"sequence":2,"mode":"IMMEDIATE","timeoutSeconds":30,"maxRetries":3,"serviceAccountId":"sa_1","dataOnly":true,"customConfig":[{"key":"k","value":"v"}],"createdBy":"prn_1","createdAt":"2026-01-01T00:00:00Z","updatedAt":"2026-01-02T00:00:00Z"}`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) { assertWireRoundTrip(t, tc.dst, tc.payload) })
	}
}
