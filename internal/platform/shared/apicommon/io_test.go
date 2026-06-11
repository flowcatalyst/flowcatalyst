package apicommon_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apicommon"
)

type gadgetResponse struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Hand-rolled wrappers, as every module defines them today.
type handGetInput struct {
	ID string `path:"id"`
}

type handGetOutput struct {
	Body gadgetResponse
}

type handCreateInput struct {
	Body gadgetResponse
}

type handEmptyInput struct{}

type handEmptyOutput struct{}

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

func registerPair[I, O any](api huma.API, h func(context.Context, *I) (*O, error)) {
	huma.Register(api, huma.Operation{
		OperationID:   "getGadget",
		Method:        http.MethodGet,
		Path:          "/api/gadgets/{id}",
		Summary:       "Get a gadget by id",
		Tags:          []string{"gadgets"},
		DefaultStatus: http.StatusOK,
	}, h)
}

func registerCreatePair[I, O any](api huma.API, h func(context.Context, *I) (*O, error)) {
	huma.Register(api, huma.Operation{
		OperationID:   "createGadget",
		Method:        http.MethodPost,
		Path:          "/api/gadgets",
		Summary:       "Create a gadget",
		Tags:          []string{"gadgets"},
		DefaultStatus: http.StatusCreated,
	}, h)
}

func registerEmptyPair[I, O any](api huma.API, h func(context.Context, *I) (*O, error)) {
	huma.Register(api, huma.Operation{
		OperationID:   "purgeGadgets",
		Method:        http.MethodDelete,
		Path:          "/api/gadgets",
		Summary:       "Purge gadgets",
		Tags:          []string{"gadgets"},
		DefaultStatus: http.StatusNoContent,
	}, h)
}

// TestSharedWrappersMatchHandRolled proves Out/Empty/IDInput are
// wire-neutral: the generated spec is byte-identical to one produced
// with per-module wrapper structs.
func TestSharedWrappersMatchHandRolled(t *testing.T) {
	t.Parallel()

	shared := newAPI()
	registerPair(shared, func(_ context.Context, _ *apicommon.IDInput) (*apicommon.Out[gadgetResponse], error) {
		return &apicommon.Out[gadgetResponse]{}, nil
	})
	registerCreatePair(shared, func(_ context.Context, _ *apicommon.In[gadgetResponse]) (*apicommon.Out[gadgetResponse], error) {
		return &apicommon.Out[gadgetResponse]{}, nil
	})
	registerEmptyPair(shared, func(_ context.Context, _ *apicommon.Empty) (*apicommon.Empty, error) {
		return &apicommon.Empty{}, nil
	})

	hand := newAPI()
	registerPair(hand, func(_ context.Context, _ *handGetInput) (*handGetOutput, error) {
		return &handGetOutput{}, nil
	})
	registerCreatePair(hand, func(_ context.Context, _ *handCreateInput) (*handGetOutput, error) {
		return &handGetOutput{}, nil
	})
	registerEmptyPair(hand, func(_ context.Context, _ *handEmptyInput) (*handEmptyOutput, error) {
		return &handEmptyOutput{}, nil
	})

	got, want := specJSON(t, shared), specJSON(t, hand)
	if string(got) != string(want) {
		t.Errorf("shared wrappers diverge from hand-rolled spec:\n got: %s\nwant: %s", got, want)
	}
}

func TestOptStr(t *testing.T) {
	t.Parallel()

	if got := apicommon.OptStr(""); got != nil {
		t.Errorf("OptStr(%q) = %v, want nil", "", *got)
	}
	if got := apicommon.OptStr("x"); got == nil || *got != "x" {
		t.Errorf("OptStr(%q) = %v, want %q", "x", got, "x")
	}
}

func TestMapSlice(t *testing.T) {
	t.Parallel()

	got := apicommon.MapSlice(nil, func(s *string) string { return *s })
	if got == nil || len(got) != 0 {
		t.Errorf("MapSlice(nil) = %v, want non-nil empty slice", got)
	}
	b, err := json.Marshal(got)
	if err != nil || string(b) != "[]" {
		t.Errorf("MapSlice(nil) marshals to %s (err %v), want []", b, err)
	}

	in := []int{1, 2, 3}
	doubled := apicommon.MapSlice(in, func(i *int) int { return *i * 2 })
	if len(doubled) != 3 || doubled[0] != 2 || doubled[2] != 6 {
		t.Errorf("MapSlice(%v) = %v, want [2 4 6]", in, doubled)
	}
}
