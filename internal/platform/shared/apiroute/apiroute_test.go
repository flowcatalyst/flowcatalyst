package apiroute_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apiroute"
)

type widgetResponse struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type getInput struct {
	ID string `path:"id"`
}

type getOutput struct {
	Body widgetResponse
}

type createInput struct {
	Body widgetResponse
}

type emptyInput struct{}

type emptyOutput struct{}

func get(_ context.Context, _ *getInput) (*getOutput, error)       { return &getOutput{}, nil }
func create(_ context.Context, _ *createInput) (*getOutput, error) { return &getOutput{}, nil }
func update(_ context.Context, _ *getInput) (*emptyOutput, error)  { return &emptyOutput{}, nil }
func del(_ context.Context, _ *getInput) (*emptyOutput, error)     { return &emptyOutput{}, nil }
func list(_ context.Context, _ *emptyInput) (*getOutput, error)    { return &getOutput{}, nil }

func newAPI() huma.API {
	return humachi.New(chi.NewMux(), huma.DefaultConfig("test", "dev"))
}

func specJSON(t *testing.T, api huma.API) []byte {
	t.Helper()
	b, err := json.Marshal(api.OpenAPI())
	if err != nil {
		t.Fatalf("marshal spec: %v", err)
	}
	return b
}

// TestRegistrarMatchesHumaRegister is the drop-in guarantee in test
// form: registering the same handlers via apiroute and via literal
// six-field huma.Register calls must produce byte-identical specs.
func TestRegistrarMatchesHumaRegister(t *testing.T) {
	t.Parallel()

	viaHelpers := newAPI()
	g := apiroute.New(viaHelpers, "widgets")
	apiroute.Get(g, "listWidgets", "/api/widgets", "List widgets", list)
	apiroute.Post(g, "createWidget", "/api/widgets", "Create a widget", http.StatusCreated, create)
	apiroute.Get(g, "getWidget", "/api/widgets/{id}", "Get a widget by id", get)
	apiroute.Put(g, "updateWidget", "/api/widgets/{id}", "Update a widget", http.StatusNoContent, update)
	apiroute.Delete(g, "deleteWidget", "/api/widgets/{id}", "Delete a widget", http.StatusNoContent, del)

	viaRaw := newAPI()
	for _, reg := range []func(){
		func() {
			huma.Register(viaRaw, huma.Operation{
				OperationID:   "listWidgets",
				Method:        http.MethodGet,
				Path:          "/api/widgets",
				Summary:       "List widgets",
				Tags:          []string{"widgets"},
				DefaultStatus: http.StatusOK,
			}, list)
		},
		func() {
			huma.Register(viaRaw, huma.Operation{
				OperationID:   "createWidget",
				Method:        http.MethodPost,
				Path:          "/api/widgets",
				Summary:       "Create a widget",
				Tags:          []string{"widgets"},
				DefaultStatus: http.StatusCreated,
			}, create)
		},
		func() {
			huma.Register(viaRaw, huma.Operation{
				OperationID:   "getWidget",
				Method:        http.MethodGet,
				Path:          "/api/widgets/{id}",
				Summary:       "Get a widget by id",
				Tags:          []string{"widgets"},
				DefaultStatus: http.StatusOK,
			}, get)
		},
		func() {
			huma.Register(viaRaw, huma.Operation{
				OperationID:   "updateWidget",
				Method:        http.MethodPut,
				Path:          "/api/widgets/{id}",
				Summary:       "Update a widget",
				Tags:          []string{"widgets"},
				DefaultStatus: http.StatusNoContent,
			}, update)
		},
		func() {
			huma.Register(viaRaw, huma.Operation{
				OperationID:   "deleteWidget",
				Method:        http.MethodDelete,
				Path:          "/api/widgets/{id}",
				Summary:       "Delete a widget",
				Tags:          []string{"widgets"},
				DefaultStatus: http.StatusNoContent,
			}, del)
		},
	} {
		reg()
	}

	got, want := specJSON(t, viaHelpers), specJSON(t, viaRaw)
	if string(got) != string(want) {
		t.Errorf("apiroute spec diverges from raw huma.Register:\n got: %s\nwant: %s", got, want)
	}
}
