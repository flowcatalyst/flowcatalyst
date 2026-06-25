package usecaseop_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecaseop"
)

// fakeEvent implements usecase.DomainEvent for these tests.
type fakeEvent struct{ id string }

func (e fakeEvent) EventID() string           { return e.id }
func (fakeEvent) EventType() string           { return "test:fake:created" }
func (fakeEvent) SpecVersion() string         { return "1.0" }
func (fakeEvent) Source() string              { return "test" }
func (fakeEvent) Subject() string             { return "test.fake.1" }
func (fakeEvent) Time() time.Time             { return time.Unix(0, 0).UTC() }
func (fakeEvent) PrincipalID() string         { return "test-principal" }
func (fakeEvent) CorrelationID() string       { return "corr-1" }
func (fakeEvent) CausationID() string         { return "" }
func (fakeEvent) ExecutionID() string         { return "exec-1" }
func (fakeEvent) MessageGroup() string        { return "test:fake:1" }
func (fakeEvent) ToDataJSON() ([]byte, error) { return []byte(`{}`), nil }

// newOp builds an Operation that records phase order into calls and uses the
// supplied phase errors. Execute returns an error (never a Plan) so Run never
// reaches the DB — phase ordering is all we exercise here. The atomic commit
// itself is covered by the connection pg test.
func newOp(calls *[]string, validateErr, authorizeErr, executeErr error) usecaseop.Operation[string, fakeEvent] {
	return usecaseop.Operation[string, fakeEvent]{
		Name: "TestOp",
		Validate: func(context.Context, string) error {
			*calls = append(*calls, "validate")
			return validateErr
		},
		Authorize: func(context.Context, string) error {
			*calls = append(*calls, "authorize")
			return authorizeErr
		},
		Execute: func(context.Context, string, usecase.ExecutionContext) (usecaseop.Plan[fakeEvent], error) {
			*calls = append(*calls, "execute")
			return nil, executeErr
		},
	}
}

func run(op usecaseop.Operation[string, fakeEvent]) (fakeEvent, error) {
	// uow is nil: every test path errors before plan.apply, which is the only
	// thing that dereferences the unit of work.
	return usecaseop.Run(context.Background(), nil, op, "cmd", usecase.NewExecutionContext("p"))
}

func TestRunOrdersValidateAuthorizeExecute(t *testing.T) {
	var calls []string
	_, err := run(newOp(&calls, nil, nil, errors.New("stop before commit")))
	require.Error(t, err)
	assert.Equal(t, []string{"validate", "authorize", "execute"}, calls)
}

func TestRunShortCircuitsOnValidationFailure(t *testing.T) {
	var calls []string
	_, err := run(newOp(&calls, usecase.Validation("BAD", "bad"), nil, nil))
	require.Error(t, err)
	assert.Equal(t, []string{"validate"}, calls)
	assert.Equal(t, usecase.KindValidation, usecase.AsError(err).Kind)
}

func TestRunShortCircuitsOnAuthorizationFailure(t *testing.T) {
	var calls []string
	_, err := run(newOp(&calls, nil, usecase.Authorization("DENY", "no"), nil))
	require.Error(t, err)
	assert.Equal(t, []string{"validate", "authorize"}, calls)
	assert.Equal(t, usecase.KindAuthorization, usecase.AsError(err).Kind)
}

// TestRunFailsClosedWhenAuthorizeMissing is the runtime backstop for the seal:
// an Operation literal that omits Authorize must not silently run unauthorized.
func TestRunFailsClosedWhenAuthorizeMissing(t *testing.T) {
	op := usecaseop.Operation[string, fakeEvent]{
		Name: "NoAuthz",
		Execute: func(context.Context, string, usecase.ExecutionContext) (usecaseop.Plan[fakeEvent], error) {
			t.Fatal("Execute must not run when Authorize is missing")
			return nil, nil
		},
	}
	_, err := run(op)
	require.Error(t, err)
	got := usecase.AsError(err)
	require.NotNil(t, got)
	assert.Equal(t, usecase.KindInternal, got.Kind)
	assert.Equal(t, "USECASE_MISCONFIGURED", got.Code)
}

// TestPublicIsAValidAuthorize documents that the explicit "intentionally open"
// value satisfies the required phase.
func TestPublicIsAValidAuthorize(t *testing.T) {
	op := usecaseop.Operation[string, fakeEvent]{
		Name:      "OpenOp",
		Authorize: usecaseop.Public[string],
		Execute: func(context.Context, string, usecase.ExecutionContext) (usecaseop.Plan[fakeEvent], error) {
			return nil, errors.New("stop before commit")
		},
	}
	_, err := run(op)
	require.Error(t, err)
	// Reaches Execute (not the misconfigured guard): the error is our sentinel.
	assert.Nil(t, usecase.AsError(err), "should not be a misconfigured usecase.Error")
}

// TestRunRejectsNilPlan covers the guard against an Execute that returns
// (nil, nil): there is no plan to apply, so Run must error rather than panic.
func TestRunRejectsNilPlan(t *testing.T) {
	op := usecaseop.Operation[string, fakeEvent]{
		Name:      "NilPlanOp",
		Authorize: usecaseop.Public[string],
		Execute: func(context.Context, string, usecase.ExecutionContext) (usecaseop.Plan[fakeEvent], error) {
			return nil, nil
		},
	}
	_, err := run(op)
	require.Error(t, err)
	assert.Equal(t, "USECASE_NIL_PLAN", usecase.AsError(err).Code)
}

// TestPlanSealIsCompileTime documents the seal. None of the lines in the PROOF
// block compile, which is the guarantee: outside this package you cannot
// construct a Plan or apply one — only Save/Delete/Emit/SaveAll/Sync build a
// Plan, and only Run (via the unexported apply method) commits it.
func TestPlanSealIsCompileTime(t *testing.T) {
	// PROOF (compile-time). Uncomment to see the compile error:
	//
	//   type myPlan struct{}
	//   func (myPlan) apply(context.Context, *usecasepgx.UnitOfWork, any) (fakeEvent, error) { ... }
	//   var _ usecaseop.Plan[fakeEvent] = myPlan{}   // apply is unexported: myPlan does not implement Plan
	//
	//   var p usecaseop.Plan[fakeEvent] = usecaseop.Emit(fakeEvent{})
	//   p.apply(ctx, uow, cmd)                       // p.apply undefined (unexported method)
	//
	// The fact that none of those compile is the seal. A Plan can only be
	// applied by Run.
	_ = usecaseop.Emit(fakeEvent{id: "e1"}) // the public constructor compiles fine
}
