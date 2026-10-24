package runtimeapi_test

import (
	"encoding/json"
	"net/http"
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
		Permissions: []string{"platform:ingestion:runtime:operate"},
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
		Permissions: []string{"platform:ingestion:ingestion:view", "platform:ingestion:ingestion:update"},
	})
	routes := []struct {
		method, path string
		body         any
	}{
		{http.MethodPost, "/api/ingestion-runtime/v1/poll", map[string]any{"runtimeId": "rt-1", "contractVersion": ingestion.ContractVersion}},
		{http.MethodPost, "/api/ingestion-runtime/v1/report", map[string]any{"runtimeId": "rt-1"}},
		{http.MethodGet, "/api/ingestion-runtime/v1/state/ing_1/watermark?runtimeId=rt-1", nil},
		{http.MethodPut, "/api/ingestion-runtime/v1/state/ing_1/watermark?runtimeId=rt-1", map[string]any{"value": "x"}},
	}
	for _, rt := range routes {
		var args []any
		if rt.body != nil {
			args = append(args, rt.body)
		}
		rec := api.Do(rt.method, rt.path, args...)
		assert.Equal(t, http.StatusForbidden, rec.Code, "%s %s: %s", rt.method, rt.path, rec.Body.String())
	}
}
