package secretref

import (
	"errors"
	"fmt"

	"github.com/flowcatalyst/go-sdk/usecase"
)

// The codes of a secret reference failure. They are the wire codes of the
// refusals in docs/external-secret-references.md: the first five come out of
// Resolve and Allowed, the last two out of the write check (CheckWrite).
const (
	CodeNotFound    = "SECRET_REFERENCE_NOT_FOUND"
	CodeDenied      = "SECRET_REFERENCE_DENIED"
	CodeKeyMissing  = "SECRET_REFERENCE_KEY_MISSING"
	CodeUnavailable = "SECRET_REFERENCE_UNAVAILABLE"
	CodeNotAllowed  = "SECRET_REFERENCE_NOT_ALLOWED"
	CodeUnsupported = "SECRET_REFERENCE_UNSUPPORTED"
)

// Error is a failed resolution of one reference. It names the reference and
// the reason, never a secret value: it is logged and returned to callers.
type Error struct {
	Code string
	// Ref is the reference that failed; a reference is not a secret.
	Ref string
	// Msg says what went wrong, without a value.
	Msg string
	// Err is the underlying failure (an AWS error), if any.
	Err error
}

func (e *Error) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("secret reference %s: %s: %s: %v", e.Ref, e.Code, e.Msg, e.Err)
	}
	return fmt.Sprintf("secret reference %s: %s: %s", e.Ref, e.Code, e.Msg)
}

func (e *Error) Unwrap() error { return e.Err }

// CodeOf is the code of a secret reference error in err's chain, or "".
func CodeOf(err error) string {
	if e, ok := errors.AsType[*Error](err); ok {
		return e.Code
	}
	return ""
}

// IsUnavailable reports whether err is a retryable failure to reach the
// secret store (the 503 of the endpoints that hand a secret out).
func IsUnavailable(err error) bool { return CodeOf(err) == CodeUnavailable }

// ToUsecase turns a resolution failure into the use case error an endpoint
// answers: 409 with the reference code for not found, denied, key missing and
// not allowed (a refusal the caller can act on, not a fault of the platform's);
// KindInternal for unavailable, which the endpoints that hand a secret out
// answer 503 (see IsUnavailable). An error that is not a secret reference
// error is returned as is.
func ToUsecase(err error) error {
	e, ok := errors.AsType[*Error](err)
	if !ok {
		return err
	}
	msg := "the secret reference " + e.Ref + " did not resolve: " + e.Msg
	if e.Code == CodeUnavailable {
		return usecase.Internal(e.Code, msg, e)
	}
	return usecase.Conflict(e.Code, msg)
}
