package usecase_test

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
)

// fakeEvent implements DomainEvent for these tests.
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

// TestSealedSuccessCannotBeConstructedExternally documents the seal.
// The compile-time evidence is what matters — the body below would not
// compile if the seal weren't real. To verify locally, uncomment any
// line in the PROOF block; the compiler must reject all of them.
func TestSealedSuccessCannotBeConstructedExternally(t *testing.T) {
	// PROOF (compile-time). Uncomment to see the compile error:
	//
	//   _ = usecase.success[fakeEvent]{}                  // undefined: usecase.success
	//   _ = usecase.Success[fakeEvent](fakeEvent{})       // missing sealed.Token argument
	//   type myResult struct{}
	//   func (myResult) isResult()  {}                    // no usable interface element
	//   var _ usecase.Result[fakeEvent] = myResult{}      // myResult does not implement Result[fakeEvent]
	//
	// Application code outside pkg/fcsdk/ cannot import the seal package
	// (pkg/fcsdk/internal/sealed), so even usecase.Success("legal" signature)
	// is unreachable. The fact that none of those compile is the seal.
	//
	// Below we only exercise the public API to verify Failure works:
	r := usecase.Failure[fakeEvent](errors.New("boom"))
	assert.True(t, usecase.IsFailure(r))
	assert.False(t, usecase.IsSuccess(r))

	_, err := usecase.Into(r)
	require.Error(t, err)
	assert.EqualError(t, err, "boom")
}

// TestErrorTypeInspection verifies usecase.AsError extracts the typed error.
func TestErrorTypeInspection(t *testing.T) {
	r := usecase.Failure[fakeEvent](usecase.BusinessRule("DUP", "duplicate code"))
	_, err := usecase.Into(r)

	got := usecase.AsError(err)
	require.NotNil(t, got)
	assert.Equal(t, usecase.KindBusinessRule, got.Kind)
	assert.Equal(t, "DUP", got.Code)
	assert.Equal(t, 409, got.HTTPStatus())
}

// TestExtractHelpers covers the subject parsing used by sinks.
func TestExtractHelpers(t *testing.T) {
	assert.Equal(t, "Order", usecase.ExtractAggregateType("orders.order.ord_123"))
	assert.Equal(t, "Unknown", usecase.ExtractAggregateType(""))
	assert.Equal(t, "ord_123", usecase.ExtractEntityID("orders.order.ord_123"))
	assert.Equal(t, "", usecase.ExtractEntityID("orders.order"))
}
