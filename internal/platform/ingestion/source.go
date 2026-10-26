package ingestion

import (
	"encoding/json"
	"slices"
)

// Kind names of the built-in sources, as "kind" writes them. A runtime
// extension's kind is none of these; it is stored as KindCustom with its type.
const (
	KindHTTP         = "http"
	KindSFTP         = "sftp"
	KindS3           = "s3"
	KindSQS          = "sqs"
	KindRabbitMQ     = "rabbitmq"
	KindDatabasePoll = "database-poll"
	KindAPIPoll      = "api-poll"
	KindCustom       = "custom"
)

// BuiltInKinds lists the nine kinds the platform itself knows (custom included).
func BuiltInKinds() []string {
	return []string{KindHTTP, KindSFTP, KindS3, KindSQS, KindRabbitMQ, KindDatabasePoll, KindAPIPoll, KindCustom}
}

// IsBuiltInKind reports whether name is one of BuiltInKinds.
func IsBuiltInKind(name string) bool { return slices.Contains(BuiltInKinds(), name) }

// Source is where an ingestion's data comes from: one of HTTP, Sftp, S3, Sqs,
// RabbitMQ, DatabasePoll, APIPoll or Custom. Secrets are always SecretRef
// names, never values.
type Source interface {
	// Kind is the "kind" the source is written with.
	Kind() string
	// SingleOwner reports whether exactly one runtime may run it at a time
	// (a polled source would be read twice by two runtimes; a queue or an
	// HTTP endpoint is shared safely). A custom source may poll, and nothing
	// says it can run twice at once, so it is single-owner.
	SingleOwner() bool
	isSource()
}

// HTTP is JSON posted to the endpoint the runtime exposes for the ingestion:
// POST /ingest/{client}/{code}.
type HTTP struct {
	Auth      HTTPAuth   `json:"auth"`
	RateLimit *RateLimit `json:"rateLimit,omitempty"`
}

// RateLimit accepts at most Requests per Per, counted per runtime.
type RateLimit struct {
	Requests int      `json:"requests"`
	Per      Duration `json:"per"`
}

// Sftp polls files in a directory on an SFTP server.
type Sftp struct {
	Host       string    `json:"host"`
	Port       int       `json:"port"`
	Directory  string    `json:"directory"`
	Username   string    `json:"username"`
	Credential SecretRef `json:"credential"`
	// KnownHosts is the server's host key as a known_hosts line. Without it the
	// runtime refuses to start the source unless AcceptAnyHostKey is set.
	KnownHosts *string `json:"knownHosts,omitempty"`
	// AcceptAnyHostKey connects without checking who the server is: whoever
	// answers at the address receives the credential. It widens where the
	// secret goes, so turning it on needs bind-secrets (CheckAuthor).
	AcceptAnyHostKey bool `json:"acceptAnyHostKey,omitempty"`
	// Include is a file name pattern such as *.csv; nil takes every file.
	Include *string  `json:"include,omitempty"`
	Every   Duration `json:"every"`
}

// S3 polls the objects directly under a prefix of a bucket.
type S3 struct {
	Region string `json:"region"`
	Bucket string `json:"bucket"`
	// Prefix is folder names each ending in "/"; nil is the top of the bucket.
	Prefix *string  `json:"prefix,omitempty"`
	Every  Duration `json:"every"`
	// Credential is the access key to read with; nil uses the runtime's own identity.
	Credential *AWSCredential `json:"credential,omitempty"`
}

// Sqs consumes messages from a queue, named, not its URL.
type Sqs struct {
	Region     string         `json:"region"`
	Queue      string         `json:"queue"`
	Credential *AWSCredential `json:"credential,omitempty"`
}

// AWSCredential is an access key; the secret half is a name.
type AWSCredential struct {
	AccessKeyID     string    `json:"accessKeyId"`
	SecretAccessKey SecretRef `json:"secretAccessKey"`
}

// RabbitMQ consumes messages from a queue. URI is amqp:// or amqps:// without a login.
type RabbitMQ struct {
	URI      string    `json:"uri"`
	Queue    string    `json:"queue"`
	Username string    `json:"username"`
	Password SecretRef `json:"password"`
}

// DatabasePoll reads the rows of a table or view; each row is one record, so
// the split is always none.
type DatabasePoll struct {
	JDBCURL  string    `json:"jdbcUrl"`
	Username string    `json:"username"`
	Password SecretRef `json:"password"`
	Table    string    `json:"table"`
	Track    Track     `json:"track"`
	MaxRows  int       `json:"maxRows"`
	Every    Duration  `json:"every"`
}

// APIPoll makes one request per poll to an HTTP API and splits the answer.
type APIPoll struct {
	URL     string   `json:"url"`
	Method  string   `json:"method"`
	Headers []Header `json:"headers,omitempty"`
	Body    *string  `json:"body,omitempty"`
	Auth    APIAuth  `json:"auth"`
	Results Results  `json:"results"`
	Cursor  *Cursor  `json:"cursor,omitempty"`
	// Every is the wait after an answer with results, IdleDelay after one without.
	Every     Duration `json:"every"`
	IdleDelay Duration `json:"idleDelay"`
}

// Header is sent on every request; none of them is secret.
type Header struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// Cursor is what in an answer moves the next request on. From is a path such
// as $.next, Param the query parameter it is sent in.
type Cursor struct {
	From    string  `json:"from"`
	Param   string  `json:"param"`
	Initial *string `json:"initial,omitempty"`
}

// Custom is a kind a runtime extension supplies (and camel-route, which the
// Camel runtime reports). The platform stores the settings and knows nothing
// else about them; a secret setting is the secret's name.
type Custom struct {
	Type     string         `json:"type"`
	Settings map[string]any `json:"settings,omitempty"`
}

func (HTTP) Kind() string         { return KindHTTP }
func (Sftp) Kind() string         { return KindSFTP }
func (S3) Kind() string           { return KindS3 }
func (Sqs) Kind() string          { return KindSQS }
func (RabbitMQ) Kind() string     { return KindRabbitMQ }
func (DatabasePoll) Kind() string { return KindDatabasePoll }
func (APIPoll) Kind() string      { return KindAPIPoll }
func (Custom) Kind() string       { return KindCustom }

func (HTTP) SingleOwner() bool         { return false }
func (Sftp) SingleOwner() bool         { return true }
func (S3) SingleOwner() bool           { return true }
func (Sqs) SingleOwner() bool          { return false }
func (RabbitMQ) SingleOwner() bool     { return false }
func (DatabasePoll) SingleOwner() bool { return true }
func (APIPoll) SingleOwner() bool      { return true }
func (Custom) SingleOwner() bool       { return true }

func (HTTP) isSource()         {}
func (Sftp) isSource()         {}
func (S3) isSource()           {}
func (Sqs) isSource()          {}
func (RabbitMQ) isSource()     {}
func (DatabasePoll) isSource() {}
func (APIPoll) isSource()      {}
func (Custom) isSource()       {}

func (s HTTP) MarshalJSON() ([]byte, error) {
	type plain HTTP
	return marshalTagged(s.Kind(), plain(s))
}

func (s Sftp) MarshalJSON() ([]byte, error) {
	type plain Sftp
	return marshalTagged(s.Kind(), plain(s))
}

func (s S3) MarshalJSON() ([]byte, error) { type plain S3; return marshalTagged(s.Kind(), plain(s)) }

func (s Sqs) MarshalJSON() ([]byte, error) { type plain Sqs; return marshalTagged(s.Kind(), plain(s)) }

func (s RabbitMQ) MarshalJSON() ([]byte, error) {
	type plain RabbitMQ
	return marshalTagged(s.Kind(), plain(s))
}

func (s DatabasePoll) MarshalJSON() ([]byte, error) {
	type plain DatabasePoll
	return marshalTagged(s.Kind(), plain(s))
}

func (s APIPoll) MarshalJSON() ([]byte, error) {
	type plain APIPoll
	return marshalTagged(s.Kind(), plain(s))
}

func (s Custom) MarshalJSON() ([]byte, error) {
	type plain Custom
	return marshalTagged(s.Kind(), plain(s))
}

// The unions inside a source decode through these, since encoding/json cannot
// pick a member of an interface by itself.

func (s *HTTP) UnmarshalJSON(data []byte) error {
	var aux struct {
		Auth      json.RawMessage `json:"auth"`
		RateLimit *RateLimit      `json:"rateLimit"`
	}
	if err := strictDecode(data, &aux); err != nil {
		return err
	}
	*s = HTTP{RateLimit: aux.RateLimit}
	if aux.Auth == nil {
		return nil // validation reports the missing auth
	}
	auth, err := decodeHTTPAuth(aux.Auth)
	s.Auth = auth
	return err
}

func (s *DatabasePoll) UnmarshalJSON(data []byte) error {
	type plain DatabasePoll
	var aux struct {
		plain
		Track json.RawMessage `json:"track"`
	}
	if err := strictDecode(data, &aux); err != nil {
		return err
	}
	*s = DatabasePoll(aux.plain)
	if aux.Track == nil {
		return nil
	}
	track, err := decodeTrack(aux.Track)
	s.Track = track
	return err
}

func (s *APIPoll) UnmarshalJSON(data []byte) error {
	type plain APIPoll
	var aux struct {
		plain
		Auth    json.RawMessage `json:"auth"`
		Results json.RawMessage `json:"results"`
	}
	if err := strictDecode(data, &aux); err != nil {
		return err
	}
	*s = APIPoll(aux.plain)
	var err error
	if aux.Auth != nil {
		if s.Auth, err = decodeAPIAuth(aux.Auth); err != nil {
			return err
		}
	}
	if aux.Results != nil {
		if s.Results, err = decodeResults(aux.Results); err != nil {
			return err
		}
	}
	return nil
}

// UnmarshalSource reads a source object by its kind, strictly. An object with a
// "config" field is a reference to a config entry (Configured), not a source
// with settings.
func UnmarshalSource(data []byte) (Source, error) {
	var fields map[string]json.RawMessage
	if json.Unmarshal(data, &fields) == nil && fields != nil {
		if _, named := fields["config"]; named {
			return decodeConfigured(fields)
		}
	}
	return decodeTagged(data, "source", map[string]func([]byte) (Source, error){
		KindHTTP:         variant[Source, HTTP],
		KindSFTP:         variant[Source, Sftp],
		KindS3:           variant[Source, S3],
		KindSQS:          variant[Source, Sqs],
		KindRabbitMQ:     variant[Source, RabbitMQ],
		KindDatabasePoll: variant[Source, DatabasePoll],
		KindAPIPoll:      variant[Source, APIPoll],
		KindCustom:       variant[Source, Custom],
	})
}

// Track tells a database poll's rows read from rows not yet read.
type Track interface {
	trackKind() string
}

// Watermark reads rows in the order of a column that only grows and keeps the
// highest value read outside the table. Type is NUMBER or TIMESTAMP.
type Watermark struct {
	Column string `json:"column"`
	Type   string `json:"type"`
}

// StatusTrack reads rows whose status column holds Pending and sets it to
// Processed once they are dispatch jobs.
type StatusTrack struct {
	Column    string `json:"column"`
	KeyColumn string `json:"keyColumn"`
	Pending   string `json:"pending"`
	Processed string `json:"processed"`
}

func (Watermark) trackKind() string   { return "watermark" }
func (StatusTrack) trackKind() string { return "status" }

func (t Watermark) MarshalJSON() ([]byte, error) {
	type plain Watermark
	return marshalTagged(t.trackKind(), plain(t))
}

func (t StatusTrack) MarshalJSON() ([]byte, error) {
	type plain StatusTrack
	return marshalTagged(t.trackKind(), plain(t))
}

func decodeTrack(data []byte) (Track, error) {
	return decodeTagged(data, "track", map[string]func([]byte) (Track, error){
		"watermark": variant[Track, Watermark],
		"status":    variant[Track, StatusTrack],
	})
}

// HTTPAuth is how the caller of an http source authenticates.
type HTTPAuth interface {
	httpAuthKind() string
}

// FlowCatalystToken accepts the caller's own platform access token, which
// must reach the client and the Application (a code). PrincipalID (prn_...)
// narrows it to one principal.
type FlowCatalystToken struct {
	Application string  `json:"application"`
	PrincipalID *string `json:"principalId,omitempty"`
}

// BasicAuth checks a username and a password held as a secret.
type BasicAuth struct {
	Username string    `json:"username"`
	Password SecretRef `json:"password"`
}

// BearerAuth checks a bearer token held as a secret.
type BearerAuth struct {
	Token SecretRef `json:"token"`
}

func (FlowCatalystToken) httpAuthKind() string { return "flowcatalyst-token" }
func (BasicAuth) httpAuthKind() string         { return "basic" }
func (BearerAuth) httpAuthKind() string        { return "bearer" }

func (a FlowCatalystToken) MarshalJSON() ([]byte, error) {
	type plain FlowCatalystToken
	return marshalTagged(a.httpAuthKind(), plain(a))
}

func (a BasicAuth) MarshalJSON() ([]byte, error) {
	type plain BasicAuth
	return marshalTagged(a.httpAuthKind(), plain(a))
}

func (a BearerAuth) MarshalJSON() ([]byte, error) {
	type plain BearerAuth
	return marshalTagged(a.httpAuthKind(), plain(a))
}

func decodeHTTPAuth(data []byte) (HTTPAuth, error) {
	return decodeTagged(data, "auth", map[string]func([]byte) (HTTPAuth, error){
		"flowcatalyst-token": variant[HTTPAuth, FlowCatalystToken],
		"basic":              variant[HTTPAuth, BasicAuth],
		"bearer":             variant[HTTPAuth, BearerAuth],
	})
}

// APIAuth is how an api-poll logs in to the API.
type APIAuth interface {
	apiAuthKind() string
}

// NoAuth sends no credentials.
type NoAuth struct{}

// APIBasic sends basic credentials.
type APIBasic struct {
	Username string    `json:"username"`
	Password SecretRef `json:"password"`
}

// APIBearer sends a bearer token.
type APIBearer struct {
	Token SecretRef `json:"token"`
}

// APIKey sends a secret in a header.
type APIKey struct {
	Header string    `json:"header"`
	Value  SecretRef `json:"value"`
}

// ClientCredentials gets a token with the OAuth client credentials grant.
// CredentialsIn is BASIC_HEADER or BODY.
type ClientCredentials struct {
	TokenURL      string    `json:"tokenUrl"`
	ClientID      string    `json:"clientId"`
	ClientSecret  SecretRef `json:"clientSecret"`
	Scope         *string   `json:"scope,omitempty"`
	Audience      *string   `json:"audience,omitempty"`
	CredentialsIn string    `json:"credentialsIn"`
}

// Login posts a username and password to a login endpoint and reads the token
// from the answer. Format is FORM or JSON.
type Login struct {
	URL           string            `json:"url"`
	Format        string            `json:"format"`
	UsernameField string            `json:"usernameField"`
	PasswordField string            `json:"passwordField"`
	Username      string            `json:"username"`
	Password      SecretRef         `json:"password"`
	ExtraFields   map[string]string `json:"extraFields,omitempty"`
	TokenPath     string            `json:"tokenPath"`
	ExpiresInPath *string           `json:"expiresInPath,omitempty"`
	Lifetime      Duration          `json:"lifetime"`
	Header        string            `json:"header"`
	Prefix        string            `json:"prefix"`
}

func (NoAuth) apiAuthKind() string            { return "none" }
func (APIBasic) apiAuthKind() string          { return "basic" }
func (APIBearer) apiAuthKind() string         { return "bearer" }
func (APIKey) apiAuthKind() string            { return "api-key" }
func (ClientCredentials) apiAuthKind() string { return "oauth-client-credentials" }
func (Login) apiAuthKind() string             { return "login" }

func (a NoAuth) MarshalJSON() ([]byte, error) { return marshalTagged(a.apiAuthKind(), struct{}{}) }
func (a APIBasic) MarshalJSON() ([]byte, error) {
	type plain APIBasic
	return marshalTagged(a.apiAuthKind(), plain(a))
}

func (a APIBearer) MarshalJSON() ([]byte, error) {
	type plain APIBearer
	return marshalTagged(a.apiAuthKind(), plain(a))
}

func (a APIKey) MarshalJSON() ([]byte, error) {
	type plain APIKey
	return marshalTagged(a.apiAuthKind(), plain(a))
}

func (a ClientCredentials) MarshalJSON() ([]byte, error) {
	type plain ClientCredentials
	return marshalTagged(a.apiAuthKind(), plain(a))
}

func (a Login) MarshalJSON() ([]byte, error) {
	type plain Login
	return marshalTagged(a.apiAuthKind(), plain(a))
}

func decodeAPIAuth(data []byte) (APIAuth, error) {
	return decodeTagged(data, "auth", map[string]func([]byte) (APIAuth, error){
		"none":                     variant[APIAuth, NoAuth],
		"basic":                    variant[APIAuth, APIBasic],
		"bearer":                   variant[APIAuth, APIBearer],
		"api-key":                  variant[APIAuth, APIKey],
		"oauth-client-credentials": variant[APIAuth, ClientCredentials],
		"login":                    variant[APIAuth, Login],
	})
}

// Results is how an api-poll's answer is told to have results.
type Results interface {
	resultsKind() string
}

// RecordResults has results when the split makes at least one record of the answer.
type RecordResults struct{}

// JSONPathResults has results when Path ($.items) holds a non-empty array or a
// value that is not null, false or empty.
type JSONPathResults struct {
	Path string `json:"path"`
}

// HeaderResults has results when the answer carries the header, optionally
// with the value Equals (ignoring case).
type HeaderResults struct {
	Name   string  `json:"name"`
	Equals *string `json:"equals,omitempty"`
}

func (RecordResults) resultsKind() string   { return "records" }
func (JSONPathResults) resultsKind() string { return "json-path" }
func (HeaderResults) resultsKind() string   { return "header" }

func (r RecordResults) MarshalJSON() ([]byte, error) {
	return marshalTagged(r.resultsKind(), struct{}{})
}

func (r JSONPathResults) MarshalJSON() ([]byte, error) {
	type plain JSONPathResults
	return marshalTagged(r.resultsKind(), plain(r))
}

func (r HeaderResults) MarshalJSON() ([]byte, error) {
	type plain HeaderResults
	return marshalTagged(r.resultsKind(), plain(r))
}

func decodeResults(data []byte) (Results, error) {
	return decodeTagged(data, "results", map[string]func([]byte) (Results, error){
		"records":   variant[Results, RecordResults],
		"json-path": variant[Results, JSONPathResults],
		"header":    variant[Results, HeaderResults],
	})
}
