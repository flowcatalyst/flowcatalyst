package abi

// Version is the ABI this package speaks. A module declares it by exporting
// the marker function named by ExportMarker.
const Version = 1

// Export and import names of ABI v1.
const (
	ExportMemory     = "memory"
	ExportMarker     = "fc_abi_v1"
	ExportAlloc      = "fc_alloc"
	ExportHandle     = "fc_handle"
	ExportDescribe   = "fc_describe"
	ExportInitialize = "_initialize"

	ImportModule = "fc"
	ImportCall   = "call"
	ImportTake   = "take"

	WASIModule = "wasi_snapshot_preview1"
)

// Request is the meta of the frame fc_handle receives.
type Request struct {
	ID             string              `json:"id"`
	Address        string              `json:"address"`
	Version        int                 `json:"version"`
	Method         string              `json:"method"`
	Path           string              `json:"path"`
	RawQuery       string              `json:"rawQuery"`
	Headers        map[string][]string `json:"headers"`
	Route          string              `json:"route"`
	PathParams     map[string]string   `json:"pathParams"`
	Caller         Caller              `json:"caller"`
	DeadlineUnixMs int64               `json:"deadlineUnixMs"`
}

// Caller kinds.
const (
	CallerWebhook   = "webhook"
	CallerAnonymous = "anonymous"
	CallerPrincipal = "principal"
)

// Caller is who the runner proved the caller to be. Principal fields are set
// only for kind "principal"; email and name are deliberately never passed.
type Caller struct {
	Kind            string   `json:"kind"`
	ID              string   `json:"id,omitempty"`
	Type            string   `json:"type,omitempty"`
	Tier            string   `json:"tier,omitempty"`
	Clients         []string `json:"clients,omitempty"`
	Roles           []string `json:"roles,omitempty"`
	Applications    []string `json:"applications,omitempty"`
	AllApplications bool     `json:"allApplications,omitempty"`
	Permissions     []string `json:"permissions,omitempty"`
}

// Response is the meta of the frame fc_handle returns.
type Response struct {
	Status  int                 `json:"status"`
	Headers map[string][]string `json:"headers,omitempty"`
}

// Op is a host capability a guest reaches through fc.call.
type Op uint32

// Ops of ABI v1.
const (
	OpLog        Op = 1
	OpConfigGet  Op = 2
	OpSecretGet  Op = 3
	OpHTTPFetch  Op = 4
	OpEventEmit  Op = 5
	OpDBQuery    Op = 6
	OpDBExec     Op = 7
	OpDBBegin    Op = 8
	OpDBCommit   Op = 9
	OpDBRollback Op = 10
)

func (o Op) String() string {
	switch o {
	case OpLog:
		return "log"
	case OpConfigGet:
		return "config.get"
	case OpSecretGet:
		return "secret.get"
	case OpHTTPFetch:
		return "http.fetch"
	case OpEventEmit:
		return "event.emit"
	case OpDBQuery:
		return "db.query"
	case OpDBExec:
		return "db.exec"
	case OpDBBegin:
		return "db.begin"
	case OpDBCommit:
		return "db.commit"
	case OpDBRollback:
		return "db.rollback"
	}
	return "unknown"
}

// Error codes a host call answers with. A capability failure is always one
// of these, never a trap.
const (
	CodeNotDeclared           = "NOT_DECLARED"
	CodeNotAllowed            = "NOT_ALLOWED"
	CodeDeadline              = "DEADLINE"
	CodeTooLarge              = "TOO_LARGE"
	CodeUnavailable           = "UNAVAILABLE"
	CodeBadRequest            = "BAD_REQUEST"
	CodeCapabilityUnavailable = "CAPABILITY_UNAVAILABLE"
	CodeUnknownOp             = "UNKNOWN_OP"
	CodeDBConstraint          = "DB_CONSTRAINT"
	CodeDBSyntax              = "DB_SYNTAX"
	CodeDBTimeout             = "DB_TIMEOUT"
	CodeDBUnavailable         = "DB_UNAVAILABLE"
	CodeDBError               = "DB_ERROR"
	CodeDBTxUnknown           = "DB_TX_UNKNOWN"
)

// Error is a host call's failure: the meta of an error frame, and a Go error.
type Error struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *Error) Error() string { return e.Code + ": " + e.Message }

// Errorf builds an *Error.
func Errorf(code, msg string) *Error { return &Error{Code: code, Message: msg} }

// Log is the meta of an OpLog call.
type Log struct {
	Level string         `json:"level"`
	Msg   string         `json:"msg"`
	Attrs map[string]any `json:"attrs,omitempty"`
}

// Key is the meta of an OpConfigGet / OpSecretGet call.
type Key struct {
	Key string `json:"key"`
}

// Found is the result meta of OpConfigGet / OpSecretGet; the value is the body.
type Found struct {
	Found bool `json:"found"`
}

// HTTPRequest is the meta of an OpHTTPFetch call; the request body is the body.
type HTTPRequest struct {
	Method    string              `json:"method"`
	URL       string              `json:"url"`
	Headers   map[string][]string `json:"headers,omitempty"`
	TimeoutMs int64               `json:"timeoutMs,omitempty"`
}

// HTTPResponse is the result meta of OpHTTPFetch; the response body is the body.
type HTTPResponse struct {
	Status  int                 `json:"status"`
	Headers map[string][]string `json:"headers,omitempty"`
}

// Event is the meta of an OpEventEmit call; the event data is the body.
type Event struct {
	Type          string `json:"type"`
	Source        string `json:"source,omitempty"`
	Subject       string `json:"subject,omitempty"`
	DedupID       string `json:"dedupId"`
	CorrelationID string `json:"correlationId,omitempty"`
	CausationID   string `json:"causationId,omitempty"`
	MessageGroup  string `json:"messageGroup,omitempty"`
	ContentType   string `json:"contentType,omitempty"`
}

// Emitted is the result meta of OpEventEmit.
type Emitted struct {
	EventID string `json:"eventId"`
}

// DBStatement is the meta of OpDBQuery / OpDBExec.
type DBStatement struct {
	DB     string `json:"db"`
	SQL    string `json:"sql"`
	Params []any  `json:"params,omitempty"`
	Tx     string `json:"tx,omitempty"`
}

// DBRows is the result meta of OpDBQuery; the rows (a JSON array of objects,
// keys in select order) are the body.
type DBRows struct {
	Truncated bool `json:"truncated"`
}

// DBExecuted is the result meta of OpDBExec.
type DBExecuted struct {
	RowsAffected int64 `json:"rowsAffected"`
}

// DBBegin is the meta of OpDBBegin.
type DBBegin struct {
	DB string `json:"db"`
}

// DBTx names a transaction: the result of OpDBBegin, the meta of commit and rollback.
type DBTx struct {
	Tx string `json:"tx"`
}

// Outcome header: a function's terminal "do not retry" answer is 422 with
// this header set to OutcomeReject (plan §13.2).
const (
	OutcomeHeader = "FlowCatalyst-Outcome"
	OutcomeReject = "reject"
)
