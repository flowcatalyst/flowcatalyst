package httpcompat

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
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
