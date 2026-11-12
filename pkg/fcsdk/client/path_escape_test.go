package client_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/go-sdk/client"
)

// hostileIDs are caller-supplied values that must stay inside ONE path
// segment: separators, query and fragment delimiters, traversal, and an
// already-escaped sequence (the % must itself be escaped, not decoded).
var hostileIDs = []string{
	"a/b",
	"a?b=c",
	"a#b",
	"a/../b",
	"../../x",
	"a..b",
	"50%2Fb",
	"%",
	"a b",
	"a;b",
}

type recordedRequest struct{ method, requestURI string }

func recordingServer(t *testing.T) (*httptest.Server, func() []recordedRequest) {
	t.Helper()
	var mu sync.Mutex
	var seen []recordedRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		seen = append(seen, recordedRequest{r.Method, r.RequestURI})
		mu.Unlock()
		// Empty 200: callers that decode a body get EOF, which is fine; we
		// only assert on the request line.
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)
	return srv, func() []recordedRequest {
		mu.Lock()
		defer mu.Unlock()
		return append([]recordedRequest(nil), seen...)
	}
}

// Every instance method on ScheduledJobs puts the instance id in the path;
// a hostile id must arrive as exactly one escaped segment, never as extra
// segments, a query, or a fragment.
func TestScheduledJobInstanceIDsAreEscapedAsOneSegment(t *testing.T) {
	ctx := context.Background()
	methods := []struct {
		name   string
		suffix string
		call   func(sj *client.ScheduledJobsResource, id string)
	}{
		{"GetInstance", "", func(sj *client.ScheduledJobsResource, id string) { _, _ = sj.GetInstance(ctx, id) }},
		{"ListInstanceLogs", "/logs", func(sj *client.ScheduledJobsResource, id string) { _, _ = sj.ListInstanceLogs(ctx, id) }},
		{"LogForInstance", "/log", func(sj *client.ScheduledJobsResource, id string) {
			_ = sj.LogForInstance(ctx, id, &client.InstanceLogRequest{Message: "m"})
		}},
		{"CompleteInstance", "/complete", func(sj *client.ScheduledJobsResource, id string) {
			_ = sj.CompleteInstance(ctx, id, &client.InstanceCompleteRequest{Status: client.CompletionStatusSuccess})
		}},
	}
	for _, m := range methods {
		for _, id := range hostileIDs {
			t.Run(m.name+"/"+id, func(t *testing.T) {
				srv, seen := recordingServer(t)
				m.call(client.New(srv.URL).ScheduledJobs(), id)
				got := seen()
				require.Len(t, got, 1)
				want := "/api/scheduled-jobs/instances/" + url.PathEscape(id) + m.suffix
				assert.Equal(t, want, got[0].requestURI)
				assert.NotContains(t, got[0].requestURI, "?")
				assert.NotContains(t, got[0].requestURI, "#")
				// "/api/scheduled-jobs/instances/<id>[/suffix]" => 4 or 5 segments.
				segs := strings.Split(strings.TrimPrefix(got[0].requestURI, "/"), "/")
				wantSegs := 4
				if m.suffix != "" {
					wantSegs = 5
				}
				assert.Len(t, segs, wantSegs, got[0].requestURI)
			})
		}
	}
}

// The same rule holds for the other resources that build a path from caller
// data (every site that was concatenated unescaped before). A representative
// method per file/shape.
func TestOtherPathSegmentsAreEscaped(t *testing.T) {
	ctx := context.Background()
	const id = "a/b?c#d..%"
	e := url.PathEscape(id)
	cases := []struct {
		name string
		want string
		call func(c *client.FlowCatalystClient)
	}{
		{"ScheduledJobs.ListInstances", "/api/scheduled-jobs/" + e + "/instances", func(c *client.FlowCatalystClient) { _, _ = c.ScheduledJobs().ListInstances(ctx, id, nil) }},
		{"ScheduledJobs.Get", "/api/scheduled-jobs/" + e, func(c *client.FlowCatalystClient) { _, _ = c.ScheduledJobs().Get(ctx, id) }},
		{"Applications.GetByCode", "/api/applications/by-code/" + e, func(c *client.FlowCatalystClient) { _, _ = c.Applications().GetByCode(ctx, id) }},
		{"Applications.Get", "/api/applications/" + e, func(c *client.FlowCatalystClient) { _, _ = c.Applications().Get(ctx, id) }},
		{"Applications.GetClientConfig", "/api/applications/" + e + "/clients/" + e, func(c *client.FlowCatalystClient) { _, _ = c.Applications().GetClientConfig(ctx, id, id) }},
		{"Clients.GetByIdentifier", "/api/clients/by-identifier/" + e, func(c *client.FlowCatalystClient) { _, _ = c.Clients().GetByIdentifier(ctx, id) }},
		{"Roles.Get", "/api/roles/" + e, func(c *client.FlowCatalystClient) { _, _ = c.Roles().Get(ctx, id) }},
		{"Principals.Get", "/api/principals/" + e, func(c *client.FlowCatalystClient) { _, _ = c.Principals().Get(ctx, id) }},
		{"Subscriptions.Get", "/api/subscriptions/" + e, func(c *client.FlowCatalystClient) { _, _ = c.Subscriptions().Get(ctx, id) }},
		{"Connections.Get", "/api/connections/" + e, func(c *client.FlowCatalystClient) { _, _ = c.Connections().Get(ctx, id) }},
		{"DispatchPools.Get", "/api/dispatch-pools/" + e, func(c *client.FlowCatalystClient) { _, _ = c.DispatchPools().Get(ctx, id) }},
		{"EventTypes.GetByCode", "/api/event-types/by-code/" + e, func(c *client.FlowCatalystClient) { _, _ = c.EventTypes().GetByCode(ctx, id) }},
		{"Processes.Get", "/api/processes/" + e, func(c *client.FlowCatalystClient) { _, _ = c.Processes().Get(ctx, id) }},
		{"ServiceAccounts.Get", "/api/service-accounts/" + e, func(c *client.FlowCatalystClient) { _, _ = c.ServiceAccounts().Get(ctx, id) }},
		{"AuditLogs.Get", "/api/audit-logs/" + e, func(c *client.FlowCatalystClient) { _, _ = c.AuditLogs().Get(ctx, id) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv, seen := recordingServer(t)
			tc.call(client.New(srv.URL))
			got := seen()
			require.Len(t, got, 1)
			assert.Equal(t, tc.want, got[0].requestURI)
		})
	}
}
