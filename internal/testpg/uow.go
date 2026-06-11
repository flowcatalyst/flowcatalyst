//go:build integration

package testpg

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/platformsink"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

// NewUoW returns a UnitOfWork wired to the shared embedded-PG pool and
// the production platform sink (events → msg_events, audit → aud_logs),
// exactly as the server wires it. Operation integration tests pass this
// into the use case under test.
func NewUoW(t *testing.T) *usecasepgx.UnitOfWork {
	t.Helper()
	return usecasepgx.New(Pool(t), platformsink.New())
}

// TestEC returns an ExecutionContext for an arbitrary test principal.
// msg_events / aud_logs carry the principal id without a foreign key,
// so no iam_principals row is needed.
func TestEC() usecase.ExecutionContext {
	return usecase.NewExecutionContext("prn_optestrunner1")
}

// RequireUsecaseError asserts err is a *usecase.Error with the given
// kind and code. httperror.NotFound also lands here (it returns a
// *usecase.Error with Code "<Resource>_NOT_FOUND").
func RequireUsecaseError(t *testing.T, err error, kind usecase.Kind, code string) {
	t.Helper()
	ue := usecase.AsError(err)
	require.NotNil(t, ue, "expected *usecase.Error, got: %v", err)
	assert.Equal(t, kind, ue.Kind, "error kind (full err: %v)", err)
	assert.Equal(t, code, ue.Code, "error code (full err: %v)", err)
}
