package runtimeapi_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/humatest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/ingestion"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/ingestion/runtimeapi"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apiroute"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httpcompat"
)

// apiAs mounts the routes with no repository: everything these tests reach is
// refused before any read.
func apiAs(t *testing.T, who *auth.AuthContext) humatest.TestAPI {
	t.Helper()
	httpcompat.Init()
	_, api := humatest.New(t, apiroute.PlatformAPIConfig("test", "dev"))
	api.UseMiddleware(func(ctx huma.Context, next func(huma.Context)) {
		next(huma.WithContext(ctx, auth.WithContext(ctx.Context(), who)))
	})
	runtimeapi.Register(api, &runtimeapi.State{})
	return api
}

func asRuntime() *auth.AuthContext {
	return &auth.AuthContext{
		PrincipalID: "prn_runtime000001", Scope: auth.ScopeAnchor, AllApplications: true,
		Permissions: []string{"platform:ingestion:runtime:operate"}, PrincipalType: auth.PrincipalTypeService,
	}
}

// runtimeRoutes is one request to each runtime route.
var runtimeRoutes = []struct {
	method, path string
	body         any
}{
	{http.MethodPost, "/api/ingestion-runtime/v1/poll", map[string]any{"runtimeId": "rt-1", "contractVersion": ingestion.ContractVersion}},
	{http.MethodPost, "/api/ingestion-runtime/v1/report", map[string]any{"runtimeId": "rt-1"}},
	{http.MethodGet, "/api/ingestion-runtime/v1/state/ing_1/watermark?runtimeId=rt-1", nil},
	{http.MethodPut, "/api/ingestion-runtime/v1/state/ing_1/watermark?runtimeId=rt-1", map[string]any{"value": "x"}},
	{http.MethodPost, "/api/ingestion-runtime/v1/ingestions/ing_1/jobs?runtimeId=rt-1", map[string]any{"records": []map[string]any{{"payload": "x"}}}},
	{http.MethodGet, "/api/ingestion-runtime/v1/ingestions/ing_1/secrets/sftp-password?runtimeId=rt-1", nil},
}

// Holding the runtime permission is not enough (owner ruling R1): the caller
// must be a service account at anchor tier that belongs to no application. A
// forged or mis-provisioned principal is refused on every route with a code
// that says which.
func TestRuntimeRoutes_RefuseAPrincipalThatIsNotARuntime(t *testing.T) {
	cases := []struct {
		name string
		who  func(*auth.AuthContext)
		code string
	}{
		{"a person", func(a *auth.AuthContext) { a.PrincipalType = auth.PrincipalTypeUser }, "INGESTION_RUNTIME_SERVICE_ACCOUNT_REQUIRED"},
		{"an unknown principal type", func(a *auth.AuthContext) { a.PrincipalType = "" }, "INGESTION_RUNTIME_SERVICE_ACCOUNT_REQUIRED"},
		{"client tier", func(a *auth.AuthContext) { a.Scope, a.Clients = auth.ScopeClient, []string{"clt_0000000000001"} }, "INGESTION_RUNTIME_ANCHOR_REQUIRED"},
		{"partner tier", func(a *auth.AuthContext) {
			a.Scope, a.Clients = auth.ScopePartner, []string{"clt_0000000000001", "clt_0000000000002"}
		}, "INGESTION_RUNTIME_ANCHOR_REQUIRED"},
		{"an application's account", func(a *auth.AuthContext) {
			a.AllApplications, a.Applications = false, []string{"app_0000000000001"}
		}, "INGESTION_RUNTIME_APPLICATION_BOUND"},
	}
	for _, c := range cases {
		who := asRuntime()
		c.who(who)
		api := apiAs(t, who)
		for _, rt := range runtimeRoutes {
			var args []any
			if rt.body != nil {
				args = append(args, rt.body)
			}
			rec := api.Do(rt.method, rt.path, args...)
			require.Equal(t, http.StatusForbidden, rec.Code, "%s: %s %s: %s", c.name, rt.method, rt.path, rec.Body.String())
			assert.Contains(t, rec.Body.String(), c.code, "%s: %s %s", c.name, rt.method, rt.path)
		}
	}
}

// An account bound to no application passes the application check whether
// its principal holds every application or none (an account made through the
// API without applicationId has none; fcdev's has every one).
func TestRuntimeRoutes_AnApplicationlessAccountIsARuntime(t *testing.T) {
	for _, all := range []bool{true, false} {
		who := asRuntime()
		who.AllApplications = all
		require.NoError(t, auth.RequireIngestionRuntime(who), "allApplications=%v", all)
	}
}

// A poll for a contract version the platform does not serve is a 426 that
// names the range it does.
func TestPoll_UnsupportedContractVersionIs426(t *testing.T) {
	api := apiAs(t, asRuntime())
	for _, v := range []int{0, ingestion.MinContractVersion - 1, ingestion.ContractVersion + 1} {
		rec := api.Post("/api/ingestion-runtime/v1/poll", map[string]any{"runtimeId": "rt-1", "contractVersion": v})
		require.Equal(t, http.StatusUpgradeRequired, rec.Code, "version %d: %s", v, rec.Body.String())
		var body struct {
			Error   string `json:"error"`
			Message string `json:"message"`
			Details struct {
				Min int `json:"minContractVersion"`
				Max int `json:"maxContractVersion"`
			} `json:"details"`
		}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
		assert.Equal(t, "CONTRACT_VERSION_UNSUPPORTED", body.Error)
		assert.Equal(t, ingestion.MinContractVersion, body.Details.Min)
		assert.Equal(t, ingestion.ContractVersion, body.Details.Max)
		assert.Contains(t, body.Message, "serves")
	}
}

// Every runtime route needs the runtime permission, whatever the scope.
func TestRuntimeRoutes_RefusedWithoutPermission(t *testing.T) {
	api := apiAs(t, &auth.AuthContext{
		PrincipalID: "prn_nobody0000001", Scope: auth.ScopeAnchor, AllApplications: true,
		Permissions:   []string{"platform:ingestion:ingestion:view", "platform:ingestion:ingestion:update"},
		PrincipalType: auth.PrincipalTypeService,
	})
	for _, rt := range runtimeRoutes {
		var args []any
		if rt.body != nil {
			args = append(args, rt.body)
		}
		rec := api.Do(rt.method, rt.path, args...)
		assert.Equal(t, http.StatusForbidden, rec.Code, "%s %s: %s", rt.method, rt.path, rec.Body.String())
	}
}

// The jobs call takes bodies up to 10 MiB and the report up to 4 MiB (C4);
// every other runtime call keeps huma's 1 MiB. A body within the limit
// reaches the handler (here refused for want of the permission, 403); one
// over it is refused before anything is read ("request body is too large",
// which the platform's error envelope answers as a 400 VALIDATION).
func TestRuntimeRoutes_BodyLimits(t *testing.T) {
	api := apiAs(t, &auth.AuthContext{
		PrincipalID: "prn_nobody0000001", Scope: auth.ScopeAnchor, AllApplications: true,
		PrincipalType: auth.PrincipalTypeService,
	})
	const jobsPath, reportPath, pollPath = "/api/ingestion-runtime/v1/ingestions/ing_1/jobs?runtimeId=rt-1", "/api/ingestion-runtime/v1/report", "/api/ingestion-runtime/v1/poll"
	const tooLarge = "request body is too large limit="
	for _, c := range []struct {
		name, path string
		body       any
		limit      int // 0: within the limit
	}{
		{"jobs at 3 MiB", jobsPath, jobsBody(3 << 20), 0},
		{"jobs at 9 MiB", jobsPath, jobsBody(9 << 20), 0},
		{"jobs over 10 MiB", jobsPath, jobsBody(11 << 20), 10 << 20},
		{"report at 3 MiB", reportPath, reportBody(3 << 20), 0},
		{"report over 4 MiB", reportPath, reportBody(5 << 20), 4 << 20},
		{"poll over 1 MiB", pollPath, map[string]any{"runtimeId": "rt-1", "contractVersion": ingestion.ContractVersion, "url": "https://" + strings.Repeat("x", 2<<20)}, 1 << 20},
	} {
		rec := api.Post(c.path, c.body)
		if c.limit == 0 {
			assert.Equal(t, http.StatusForbidden, rec.Code, "%s reaches the handler: %.200s", c.name, rec.Body.String())
			continue
		}
		assert.NotEqual(t, http.StatusForbidden, rec.Code, c.name)
		assert.Contains(t, rec.Body.String(), tooLarge+strconv.Itoa(c.limit), c.name)
	}
}

func jobsBody(n int) map[string]any {
	return map[string]any{"records": []map[string]any{{"payload": strings.Repeat("x", n)}}}
}

func reportBody(n int) map[string]any {
	return map[string]any{"runtimeId": "rt-1", "routes": []map[string]any{{"ingestionId": strings.Repeat("x", n), "version": 1, "status": "STARTED"}}}
}

// FC_INGESTION_JOBS_MAX_BYTES and FC_INGESTION_REPORT_MAX_BYTES move the
// limits; the State's fields, when set, win.
func TestRuntimeRoutes_BodyLimitsAreConfigurable(t *testing.T) {
	t.Setenv("FC_INGESTION_JOBS_MAX_BYTES", "1048576")
	t.Setenv("FC_INGESTION_REPORT_MAX_BYTES", "99999999")
	httpcompat.Init()
	_, api := humatest.New(t, apiroute.PlatformAPIConfig("test", "dev"))
	who := &auth.AuthContext{PrincipalID: "prn_nobody0000001", Scope: auth.ScopeAnchor, AllApplications: true, PrincipalType: auth.PrincipalTypeService}
	api.UseMiddleware(func(ctx huma.Context, next func(huma.Context)) {
		next(huma.WithContext(ctx, auth.WithContext(ctx.Context(), who)))
	})
	runtimeapi.Register(api, &runtimeapi.State{ReportMaxBytes: 1 << 20})
	for _, rec := range []*httptest.ResponseRecorder{
		api.Post("/api/ingestion-runtime/v1/ingestions/ing_1/jobs?runtimeId=rt-1", jobsBody(2<<20)),
		api.Post("/api/ingestion-runtime/v1/report", reportBody(2<<20)),
	} {
		assert.NotEqual(t, http.StatusForbidden, rec.Code, "refused before the handler")
		assert.Contains(t, rec.Body.String(), "request body is too large limit=1048576")
	}
}
