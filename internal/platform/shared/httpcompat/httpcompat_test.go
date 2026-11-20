package httpcompat

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/humatest"
	"github.com/stretchr/testify/assert"

	"github.com/flowcatalyst/go-sdk/usecase"
)

func TestErrorModelFromUseCaseError(t *testing.T) {
	cases := []struct {
		name       string
		err        error
		wantCode   string
		wantStatus int
	}{
		{
			name:       "validation",
			err:        usecase.Validation("CODE_REQUIRED", "code is required"),
			wantCode:   "CODE_REQUIRED",
			wantStatus: http.StatusBadRequest, // suffix doesn't match _NOT_FOUND/_EXISTS, but VALIDATION code matches the direct case
		},
		{
			name:       "not found uses suffix",
			err:        usecase.NotFound("USER_NOT_FOUND", "user not found"),
			wantCode:   "USER_NOT_FOUND",
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "exists uses suffix",
			err:        usecase.Conflict("EMAIL_EXISTS", "email exists"),
			wantCode:   "EMAIL_EXISTS",
			wantStatus: http.StatusConflict,
		},
		{
			name:       "forbidden",
			err:        usecase.Authorization("FORBIDDEN", "no access"),
			wantCode:   "FORBIDDEN",
			wantStatus: http.StatusForbidden,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			em := newError(0, "", c.err).(*ErrorModel)
			assert.Equal(t, c.wantCode, em.Code)
			assert.Equal(t, c.wantStatus, em.GetStatus())
		})
	}
}

func TestNewErrorFallback(t *testing.T) {
	em := newError(0, "huma validation failure: missing field").(*ErrorModel)
	assert.Equal(t, "VALIDATION", em.Code)
	assert.Equal(t, "huma validation failure: missing field", em.Message)
	assert.Equal(t, http.StatusBadRequest, em.GetStatus())
}

// A body over a route's cap is a 413 BODY_TOO_LARGE, through huma and from
// BodyTooLarge alike, with the limit in the message.
func TestBodyTooLargeIs413(t *testing.T) {
	Init()
	_, api := humatest.New(t, huma.DefaultConfig("test", "dev"))
	huma.Register(api, huma.Operation{
		OperationID: "capped", Method: http.MethodPost, Path: "/capped", MaxBodyBytes: 32,
	}, func(_ context.Context, _ *deepIn) (*deepOut, error) { return &deepOut{}, nil })
	rec := api.Post("/capped", map[string]any{"name": strings.Repeat("x", 64)})
	assert.Equal(t, http.StatusRequestEntityTooLarge, rec.Code, rec.Body.String())
	assert.Contains(t, rec.Body.String(), `"error":"BODY_TOO_LARGE"`)
	assert.Contains(t, rec.Body.String(), "request body is too large limit=32 bytes")

	em := BodyTooLarge(32)
	assert.Equal(t, http.StatusRequestEntityTooLarge, em.GetStatus())
	assert.Equal(t, BodyTooLargeCode, em.Code)
	assert.Equal(t, "request body is too large limit=32 bytes", em.Message)

	// Any other status huma reports without a usecase error is still a 400.
	assert.Equal(t, http.StatusBadRequest, newError(http.StatusUnprocessableEntity, "bad").GetStatus())
}

type deepInner struct {
	N int `json:"n"`
}
type deepBody struct {
	Name  string               `json:"name"`
	Inner deepInner            `json:"inner"`
	List  []deepInner          `json:"list"`
	Map   map[string]deepInner `json:"map,omitempty"`
}
type (
	deepIn  struct{ Body deepBody }
	deepOut struct {
		Body struct {
			OK bool `json:"ok"`
		}
	}
)

// RelaxRequestBodies loosens the top level only; the deep version loosens
// nested objects, list items and typed maps too, for the named operations.
func TestRelaxRequestBodiesDeep(t *testing.T) {
	_, api := humatest.New(t, huma.DefaultConfig("t", "1"))
	huma.Register(api, huma.Operation{OperationID: "deepOp", Method: "POST", Path: "/deep"}, func(_ context.Context, _ *deepIn) (*deepOut, error) {
		o := &deepOut{}
		o.Body.OK = true
		return o, nil
	})
	RelaxRequestBodies(api)

	topOnly := `{"name":"a","extraTop":1,"inner":{"n":1},"list":[]}`
	nested := `{"name":"a","extraTop":1,"inner":{"n":1,"extraInner":2},"list":[{"n":1,"extraItem":3}],"map":{"k":{"n":1,"extraMapped":4}}}`
	if resp := api.Post("/deep", strings.NewReader(topOnly)); resp.Code != 200 {
		t.Fatalf("top-level unknown field after RelaxRequestBodies: %d %s", resp.Code, resp.Body.String())
	}
	if resp := api.Post("/deep", strings.NewReader(nested)); resp.Code == 200 {
		t.Fatalf("nested unknown fields were accepted before the deep relax: %s", resp.Body.String())
	}

	RelaxRequestBodiesDeep(api, "deepOp")
	if resp := api.Post("/deep", strings.NewReader(nested)); resp.Code != 200 {
		t.Fatalf("unknown fields at every level after the deep relax: %d %s", resp.Code, resp.Body.String())
	}
	// naming no operation, or an unknown one, changes nothing and does not panic
	RelaxRequestBodiesDeep(api)
	RelaxRequestBodiesDeep(api, "noSuchOp")
}
