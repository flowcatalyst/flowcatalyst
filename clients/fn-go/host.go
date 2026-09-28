package fn

import "fmt"

// opCode identifies a host capability (plan §5.3).
type opCode int32

const (
	opLog        opCode = 1
	opConfigGet  opCode = 2
	opSecretGet  opCode = 3
	opHTTPFetch  opCode = 4
	opEventEmit  opCode = 5
	opDBQuery    opCode = 6
	opDBExec     opCode = 7
	opDBBegin    opCode = 8
	opDBCommit   opCode = 9
	opDBRollback opCode = 10
)

// Error codes shared across host ops (plan §5.3).
const (
	ErrCodeNotDeclared           = "NOT_DECLARED"
	ErrCodeNotAllowed            = "NOT_ALLOWED"
	ErrCodeDeadline              = "DEADLINE"
	ErrCodeTooLarge              = "TOO_LARGE"
	ErrCodeUnavailable           = "UNAVAILABLE"
	ErrCodeBadRequest            = "BAD_REQUEST"
	ErrCodeCapabilityUnavailable = "CAPABILITY_UNAVAILABLE"
	ErrCodeDBConstraint          = "DB_CONSTRAINT"
	ErrCodeDBSyntax              = "DB_SYNTAX"
	ErrCodeDBTimeout             = "DB_TIMEOUT"
	ErrCodeDBUnavailable         = "DB_UNAVAILABLE"
	ErrCodeDBError               = "DB_ERROR"
	ErrCodeDBTxUnknown           = "DB_TX_UNKNOWN"
)

// Error is returned when a host capability reports failure: its Code is one
// of the ErrCode* constants and its Message is the host's free-text detail
// (plan §5.3: "the result frame ... is an error frame {code, message}").
type Error struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *Error) Error() string {
	if e.Message == "" {
		return "fn: " + e.Code
	}
	return fmt.Sprintf("fn: %s: %s", e.Code, e.Message)
}

// ErrRetryable is matched via errors.Is against any *Error whose Code is
// UNAVAILABLE ("retryable" per plan §5.3): errors.Is(err, fn.ErrRetryable).
var ErrRetryable = &Error{Code: ErrCodeUnavailable, Message: "retryable"}

// Is implements errors.Is support: an *Error compares equal to ErrRetryable
// when its Code is UNAVAILABLE, regardless of Message.
func (e *Error) Is(target error) bool {
	if e == nil {
		return false
	}
	if target == ErrRetryable {
		return e.Code == ErrCodeUnavailable
	}
	return false
}

// host is the seam between the portable SDK logic (registry, dispatch,
// capability wrappers) and the ABI. Exactly one implementation is linked in
// per build: abi_wasip1.go's real `fc` import glue for GOOS=wasip1, or
// abi_other.go's fake host everywhere else (used directly by tests, and so
// the package builds and runs under `go test`/`go vet` on the host OS).
type host interface {
	// call runs host capability op with meta/body as the request frame. On
	// success it returns the response meta/body (either of which may be
	// nil/empty when the op has no result payload). On a host-reported
	// capability failure it returns a non-nil hostErr and a nil err. err is
	// reserved for local/transport-level failures (e.g. a malformed frame
	// from the host) that are not part of the ABI's error-code vocabulary.
	call(op opCode, meta, body []byte) (respMeta, respBody []byte, hostErr *Error, err error)
}

// currentHost is package state set by the active build's glue file.
var currentHost host
