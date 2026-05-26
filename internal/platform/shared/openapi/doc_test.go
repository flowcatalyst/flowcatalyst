package openapi_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/openapi"
)

type sampleRequest struct {
	Name string `json:"name"`
}

type sampleResponse struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type errorEnvelope struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func TestDoc_OpRegistration(t *testing.T) {
	doc := openapi.NewDoc("Test", "0.0.0")

	doc.Op("GET", "/api/widgets", "listWidgets", "List widgets",
		openapi.Tag("widgets"),
		openapi.QueryParam("status", "filter by status", ""),
		openapi.Response(200, "OK", "WidgetList", []sampleResponse{}),
		openapi.Response(403, "Forbidden", "ErrorEnvelope", &errorEnvelope{}),
	)

	doc.Op("POST", "/api/widgets", "createWidget", "Create a widget",
		openapi.Tag("widgets"),
		openapi.RequestBody("CreateWidgetRequest", "Widget data", &sampleRequest{}),
		openapi.Response(201, "Created", "Widget", &sampleResponse{}),
	)

	doc.Op("DELETE", "/api/widgets/{id}", "deleteWidget", "Delete a widget",
		openapi.Tag("widgets"),
		openapi.PathParam("id", "Widget id"),
		openapi.Response(204, "No Content", "", nil),
	)

	spec := doc.Spec()
	if spec.OpenAPI != "3.0.3" {
		t.Errorf("openapi version: got %q want 3.0.3", spec.OpenAPI)
	}
	if spec.Info.Title != "Test" {
		t.Errorf("title: got %q want Test", spec.Info.Title)
	}

	widgetsPath := spec.Paths.Find("/api/widgets")
	if widgetsPath == nil {
		t.Fatal("path /api/widgets not registered")
	}
	if widgetsPath.Get == nil || widgetsPath.Get.OperationID != "listWidgets" {
		t.Errorf("GET /api/widgets: got %+v", widgetsPath.Get)
	}
	if widgetsPath.Post == nil || widgetsPath.Post.OperationID != "createWidget" {
		t.Errorf("POST /api/widgets: got %+v", widgetsPath.Post)
	}

	if widgetByID := spec.Paths.Find("/api/widgets/{id}"); widgetByID == nil || widgetByID.Delete == nil {
		t.Fatal("DELETE /api/widgets/{id} not registered")
	} else if got := widgetByID.Delete.Parameters; len(got) != 1 || got[0].Value.Name != "id" {
		t.Errorf("path param: got %+v", got)
	}

	// Schema components should hold the reflected types.
	if _, ok := spec.Components.Schemas["CreateWidgetRequest"]; !ok {
		t.Errorf("CreateWidgetRequest schema not in components")
	}
	if _, ok := spec.Components.Schemas["Widget"]; !ok {
		t.Errorf("Widget schema not in components")
	}
	if _, ok := spec.Components.Schemas["ErrorEnvelope"]; !ok {
		t.Errorf("ErrorEnvelope schema not in components")
	}
}

func TestDoc_SchemaDeduplicates(t *testing.T) {
	doc := openapi.NewDoc("Test", "0.0.0")

	doc.Op("GET", "/api/a", "getA", "",
		openapi.Response(200, "OK", "Shared", &sampleResponse{}),
	)
	doc.Op("GET", "/api/b", "getB", "",
		openapi.Response(200, "OK", "Shared", &sampleResponse{}),
	)

	spec := doc.Spec()
	// Both responses should $ref the same Shared schema, registered once.
	if len(spec.Components.Schemas) != 1 {
		t.Errorf("schemas: got %d want 1", len(spec.Components.Schemas))
	}
}

func TestRegisterRoutes_ServesJSON(t *testing.T) {
	doc := openapi.NewDoc("Test", "1.2.3")
	doc.Op("GET", "/api/health", "getHealth", "Health probe",
		openapi.Response(200, "OK", "", nil),
	)

	r := chi.NewRouter()
	openapi.RegisterRoutes(r, doc)

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/openapi.json", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); !strings.Contains(ct, "application/json") {
		t.Errorf("content-type: got %q", ct)
	}

	var out map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v (body=%q)", err, w.Body.String())
	}
	if out["openapi"] != "3.0.3" {
		t.Errorf("openapi field: got %v", out["openapi"])
	}
	info, _ := out["info"].(map[string]any)
	if info["title"] != "Test" || info["version"] != "1.2.3" {
		t.Errorf("info: got %+v", info)
	}
	paths, _ := out["paths"].(map[string]any)
	if _, ok := paths["/api/health"]; !ok {
		t.Errorf("missing /api/health in paths: got %+v", paths)
	}
}
