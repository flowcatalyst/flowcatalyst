package client

import (
	"encoding/json"
	"fmt"
	"strconv"
)

// The definition of an ingestion is two tagged unions, a source and a split,
// each written as {"kind": ..., fields}. They are typed here member by member.
// A member the platform adds later (or a runtime extension's kind) reads as
// Unrecognized, which keeps its JSON and writes it back unchanged, so reading
// and updating an ingestion never fails on a kind this SDK does not know.
//
// Secrets in a definition are names (SecretRef), never values; nothing in this
// SDK handles a secret value for ingestions. Durations are ISO 8601 strings
// ("PT30S", "PT5M").

// SecretRef names a secret. The runtime resolves the name against the
// ingestion's own client.
type SecretRef struct {
	Name string `json:"name"`
}

// IngestionSource is where an ingestion's data comes from: one of
// HTTPSource, SFTPSource, S3Source, SQSSource, RabbitMQSource,
// DatabasePollSource, APIPollSource, CustomSource, or Unrecognized.
type IngestionSource interface{ sourceKind() string }

// IngestionSplit is how a payload becomes records: one of NoSplit, CSVSplit,
// JSONSplit, XMLSplit, or Unrecognized.
type IngestionSplit interface{ splitKind() string }

// HTTPAuth is how the caller of an HTTP source authenticates:
// HTTPFlowCatalystTokenAuth, HTTPBasicAuth, HTTPBearerAuth, or Unrecognized.
type HTTPAuth interface{ httpAuthKind() string }

// DatabaseTrack tells a database poll's rows read from rows not yet read:
// WatermarkTrack, StatusTrack, or Unrecognized.
type DatabaseTrack interface{ trackKind() string }

// APIAuth is how an api-poll logs in to the API: APINoAuth, APIBasicAuth,
// APIBearerAuth, APIKeyAuth, APIClientCredentialsAuth, APILoginAuth, or
// Unrecognized.
type APIAuth interface{ apiAuthKind() string }

// APIResults is how an api-poll tells an answer has results:
// APIRecordsResults, APIJSONPathResults, APIHeaderResults, or Unrecognized.
type APIResults interface{ resultsKind() string }

// ─── Sources ─────────────────────────────────────────────────────────

// HTTPSource is JSON posted to the endpoint the runtime exposes for the
// ingestion.
type HTTPSource struct {
	Auth      HTTPAuth   `json:"auth"`
	RateLimit *RateLimit `json:"rateLimit,omitempty"`
}

// RateLimit accepts at most Requests per Per (ISO 8601), counted per runtime.
type RateLimit struct {
	Requests int    `json:"requests"`
	Per      string `json:"per"`
}

// SFTPSource polls files in a directory on an SFTP server.
type SFTPSource struct {
	Host       string    `json:"host"`
	Port       int       `json:"port"`
	Directory  string    `json:"directory"`
	Username   string    `json:"username"`
	Credential SecretRef `json:"credential"`
	// KnownHosts is the server's host key as a known_hosts line; nil connects
	// without checking who the server is.
	KnownHosts *string `json:"knownHosts,omitempty"`
	// Include is a file name pattern such as *.csv; nil takes every file.
	Include *string `json:"include,omitempty"`
	Every   string  `json:"every"`
}

// AWSCredential is an access key; the secret half is a name.
type AWSCredential struct {
	AccessKeyID     string    `json:"accessKeyId"`
	SecretAccessKey SecretRef `json:"secretAccessKey"`
}

// S3Source polls the objects directly under a prefix of a bucket.
type S3Source struct {
	Region string `json:"region"`
	Bucket string `json:"bucket"`
	// Prefix is folder names each ending in "/"; nil is the top of the bucket.
	Prefix *string `json:"prefix,omitempty"`
	Every  string  `json:"every"`
	// Credential is the access key to read with; nil uses the runtime's own identity.
	Credential *AWSCredential `json:"credential,omitempty"`
}

// SQSSource consumes messages from a queue, named, not its URL.
type SQSSource struct {
	Region     string         `json:"region"`
	Queue      string         `json:"queue"`
	Credential *AWSCredential `json:"credential,omitempty"`
}

// RabbitMQSource consumes messages from a queue. URI is amqp:// or amqps://
// without a login.
type RabbitMQSource struct {
	URI      string    `json:"uri"`
	Queue    string    `json:"queue"`
	Username string    `json:"username"`
	Password SecretRef `json:"password"`
}

// DatabasePollSource reads the rows of a table or view; each row is one
// record, so the split is always NoSplit.
type DatabasePollSource struct {
	JDBCURL  string        `json:"jdbcUrl"`
	Username string        `json:"username"`
	Password SecretRef     `json:"password"`
	Table    string        `json:"table"`
	Track    DatabaseTrack `json:"track"`
	MaxRows  int           `json:"maxRows"`
	Every    string        `json:"every"`
}

// APIPollSource makes one request per poll to an HTTP API and splits the
// answer.
type APIPollSource struct {
	URL     string      `json:"url"`
	Method  string      `json:"method"`
	Headers []APIHeader `json:"headers,omitempty"`
	Body    *string     `json:"body,omitempty"`
	Auth    APIAuth     `json:"auth"`
	Results APIResults  `json:"results"`
	Cursor  *APICursor  `json:"cursor,omitempty"`
	// Every is the wait after an answer with results, IdleDelay after one
	// without.
	Every     string `json:"every"`
	IdleDelay string `json:"idleDelay"`
}

// APIHeader is sent on every request; none of them is secret.
type APIHeader struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// APICursor is what in an answer moves the next request on. From is a path
// such as $.next, Param the query parameter it is sent in.
type APICursor struct {
	From    string  `json:"from"`
	Param   string  `json:"param"`
	Initial *string `json:"initial,omitempty"`
}

// CustomSource is a kind a runtime extension supplies (and camel-route, which
// the Camel runtime reports): the typed escape hatch. The platform stores the
// settings and knows nothing else about them; a secret setting is the
// secret's name.
type CustomSource struct {
	Type     string         `json:"type"`
	Settings map[string]any `json:"settings,omitempty"`
}

// ─── HTTP authentication ─────────────────────────────────────────────

// HTTPFlowCatalystTokenAuth accepts the caller's own platform access token,
// which must reach the client and the Application (a code). PrincipalID
// (prn_...) narrows it to one principal.
type HTTPFlowCatalystTokenAuth struct {
	Application string  `json:"application"`
	PrincipalID *string `json:"principalId,omitempty"`
}

// HTTPBasicAuth checks a username and a password held as a secret.
type HTTPBasicAuth struct {
	Username string    `json:"username"`
	Password SecretRef `json:"password"`
}

// HTTPBearerAuth checks a bearer token held as a secret.
type HTTPBearerAuth struct {
	Token SecretRef `json:"token"`
}

// ─── Database tracking ───────────────────────────────────────────────

// WatermarkTrack reads rows in the order of a column that only grows and keeps
// the highest value read outside the table. Type is NUMBER or TIMESTAMP.
type WatermarkTrack struct {
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

// ─── API authentication ──────────────────────────────────────────────

// APINoAuth sends no credentials.
type APINoAuth struct{}

// APIBasicAuth sends basic credentials.
type APIBasicAuth struct {
	Username string    `json:"username"`
	Password SecretRef `json:"password"`
}

// APIBearerAuth sends a bearer token.
type APIBearerAuth struct {
	Token SecretRef `json:"token"`
}

// APIKeyAuth sends a secret in a header.
type APIKeyAuth struct {
	Header string    `json:"header"`
	Value  SecretRef `json:"value"`
}

// APIClientCredentialsAuth gets a token with the OAuth client credentials
// grant. CredentialsIn is BASIC_HEADER or BODY.
type APIClientCredentialsAuth struct {
	TokenURL      string    `json:"tokenUrl"`
	ClientID      string    `json:"clientId"`
	ClientSecret  SecretRef `json:"clientSecret"`
	Scope         *string   `json:"scope,omitempty"`
	Audience      *string   `json:"audience,omitempty"`
	CredentialsIn string    `json:"credentialsIn"`
}

// APILoginAuth posts a username and password to a login endpoint and reads the
// token from the answer. Format is FORM or JSON.
type APILoginAuth struct {
	URL           string            `json:"url"`
	Format        string            `json:"format"`
	UsernameField string            `json:"usernameField"`
	PasswordField string            `json:"passwordField"`
	Username      string            `json:"username"`
	Password      SecretRef         `json:"password"`
	ExtraFields   map[string]string `json:"extraFields,omitempty"`
	TokenPath     string            `json:"tokenPath"`
	ExpiresInPath *string           `json:"expiresInPath,omitempty"`
	Lifetime      string            `json:"lifetime"`
	Header        string            `json:"header"`
	Prefix        string            `json:"prefix"`
}

// ─── API results ─────────────────────────────────────────────────────

// APIRecordsResults has results when the split makes at least one record of
// the answer.
type APIRecordsResults struct{}

// APIJSONPathResults has results when Path ($.items) holds a non-empty array
// or a value that is not null, false or empty.
type APIJSONPathResults struct {
	Path string `json:"path"`
}

// APIHeaderResults has results when the answer carries the header, optionally
// with the value Equals (ignoring case).
type APIHeaderResults struct {
	Name   string  `json:"name"`
	Equals *string `json:"equals,omitempty"`
}

// ─── Splits ──────────────────────────────────────────────────────────

// NoSplit makes one record of the payload.
type NoSplit struct{}

// CSVSplit makes one record per line; Header drops the first line.
type CSVSplit struct {
	Header bool `json:"header"`
}

// JSONSplit makes one record per element of the array at ArrayPath: $ for a
// top-level array, or $.field.field.
type JSONSplit struct {
	ArrayPath string `json:"arrayPath"`
}

// XMLSplit makes one record per occurrence of Element.
type XMLSplit struct {
	Element string `json:"element"`
}

// ─── Unrecognized ────────────────────────────────────────────────────

// Unrecognized is a union member this SDK has no type for: a kind the
// platform or a runtime extension added later, or an object without a kind. It
// keeps the JSON as read and writes it back unchanged.
type Unrecognized struct {
	// Raw is the member's JSON, kind included.
	Raw json.RawMessage
}

// Kind is the member's "kind", empty when it has none.
func (u Unrecognized) Kind() string {
	var k struct {
		Kind string `json:"kind"`
	}
	_ = json.Unmarshal(u.Raw, &k)
	return k.Kind
}

// MarshalJSON writes the member as it was read.
func (u Unrecognized) MarshalJSON() ([]byte, error) {
	if len(u.Raw) == 0 {
		return []byte("{}"), nil
	}
	return u.Raw, nil
}

func (u Unrecognized) sourceKind() string   { return u.Kind() }
func (u Unrecognized) splitKind() string    { return u.Kind() }
func (u Unrecognized) httpAuthKind() string { return u.Kind() }
func (u Unrecognized) trackKind() string    { return u.Kind() }
func (u Unrecognized) apiAuthKind() string  { return u.Kind() }
func (u Unrecognized) resultsKind() string  { return u.Kind() }

// ─── Kind tags ───────────────────────────────────────────────────────

func (HTTPSource) sourceKind() string         { return "http" }
func (SFTPSource) sourceKind() string         { return "sftp" }
func (S3Source) sourceKind() string           { return "s3" }
func (SQSSource) sourceKind() string          { return "sqs" }
func (RabbitMQSource) sourceKind() string     { return "rabbitmq" }
func (DatabasePollSource) sourceKind() string { return "database-poll" }
func (APIPollSource) sourceKind() string      { return "api-poll" }
func (CustomSource) sourceKind() string       { return "custom" }

func (NoSplit) splitKind() string   { return "none" }
func (CSVSplit) splitKind() string  { return "csv" }
func (JSONSplit) splitKind() string { return "json" }
func (XMLSplit) splitKind() string  { return "xml" }

func (HTTPFlowCatalystTokenAuth) httpAuthKind() string { return "flowcatalyst-token" }
func (HTTPBasicAuth) httpAuthKind() string             { return "basic" }
func (HTTPBearerAuth) httpAuthKind() string            { return "bearer" }

func (WatermarkTrack) trackKind() string { return "watermark" }
func (StatusTrack) trackKind() string    { return "status" }

func (APINoAuth) apiAuthKind() string                { return "none" }
func (APIBasicAuth) apiAuthKind() string             { return "basic" }
func (APIBearerAuth) apiAuthKind() string            { return "bearer" }
func (APIKeyAuth) apiAuthKind() string               { return "api-key" }
func (APIClientCredentialsAuth) apiAuthKind() string { return "oauth-client-credentials" }
func (APILoginAuth) apiAuthKind() string             { return "login" }

func (APIRecordsResults) resultsKind() string  { return "records" }
func (APIJSONPathResults) resultsKind() string { return "json-path" }
func (APIHeaderResults) resultsKind() string   { return "header" }

// ─── Encoding ────────────────────────────────────────────────────────

// marshalTagged writes v (a struct that marshals to an object) with "kind"
// first, so a member reads {"kind":"csv","header":true}.
func marshalTagged(kind string, v any) ([]byte, error) {
	body, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	head := `{"kind":` + strconv.Quote(kind)
	if string(body) == "{}" {
		return []byte(head + "}"), nil
	}
	return []byte(head + "," + string(body[1:])), nil
}

func (s HTTPSource) MarshalJSON() ([]byte, error) {
	type plain HTTPSource
	return marshalTagged(s.sourceKind(), plain(s))
}

func (s SFTPSource) MarshalJSON() ([]byte, error) {
	type plain SFTPSource
	return marshalTagged(s.sourceKind(), plain(s))
}

func (s S3Source) MarshalJSON() ([]byte, error) {
	type plain S3Source
	return marshalTagged(s.sourceKind(), plain(s))
}

func (s SQSSource) MarshalJSON() ([]byte, error) {
	type plain SQSSource
	return marshalTagged(s.sourceKind(), plain(s))
}

func (s RabbitMQSource) MarshalJSON() ([]byte, error) {
	type plain RabbitMQSource
	return marshalTagged(s.sourceKind(), plain(s))
}

func (s DatabasePollSource) MarshalJSON() ([]byte, error) {
	type plain DatabasePollSource
	return marshalTagged(s.sourceKind(), plain(s))
}

func (s APIPollSource) MarshalJSON() ([]byte, error) {
	type plain APIPollSource
	return marshalTagged(s.sourceKind(), plain(s))
}

func (s CustomSource) MarshalJSON() ([]byte, error) {
	type plain CustomSource
	return marshalTagged(s.sourceKind(), plain(s))
}

func (s NoSplit) MarshalJSON() ([]byte, error) { return marshalTagged(s.splitKind(), struct{}{}) }

func (s CSVSplit) MarshalJSON() ([]byte, error) {
	type plain CSVSplit
	return marshalTagged(s.splitKind(), plain(s))
}

func (s JSONSplit) MarshalJSON() ([]byte, error) {
	type plain JSONSplit
	return marshalTagged(s.splitKind(), plain(s))
}

func (s XMLSplit) MarshalJSON() ([]byte, error) {
	type plain XMLSplit
	return marshalTagged(s.splitKind(), plain(s))
}

func (a HTTPFlowCatalystTokenAuth) MarshalJSON() ([]byte, error) {
	type plain HTTPFlowCatalystTokenAuth
	return marshalTagged(a.httpAuthKind(), plain(a))
}

func (a HTTPBasicAuth) MarshalJSON() ([]byte, error) {
	type plain HTTPBasicAuth
	return marshalTagged(a.httpAuthKind(), plain(a))
}

func (a HTTPBearerAuth) MarshalJSON() ([]byte, error) {
	type plain HTTPBearerAuth
	return marshalTagged(a.httpAuthKind(), plain(a))
}

func (t WatermarkTrack) MarshalJSON() ([]byte, error) {
	type plain WatermarkTrack
	return marshalTagged(t.trackKind(), plain(t))
}

func (t StatusTrack) MarshalJSON() ([]byte, error) {
	type plain StatusTrack
	return marshalTagged(t.trackKind(), plain(t))
}

func (a APINoAuth) MarshalJSON() ([]byte, error) { return marshalTagged(a.apiAuthKind(), struct{}{}) }

func (a APIBasicAuth) MarshalJSON() ([]byte, error) {
	type plain APIBasicAuth
	return marshalTagged(a.apiAuthKind(), plain(a))
}

func (a APIBearerAuth) MarshalJSON() ([]byte, error) {
	type plain APIBearerAuth
	return marshalTagged(a.apiAuthKind(), plain(a))
}

func (a APIKeyAuth) MarshalJSON() ([]byte, error) {
	type plain APIKeyAuth
	return marshalTagged(a.apiAuthKind(), plain(a))
}

func (a APIClientCredentialsAuth) MarshalJSON() ([]byte, error) {
	type plain APIClientCredentialsAuth
	return marshalTagged(a.apiAuthKind(), plain(a))
}

func (a APILoginAuth) MarshalJSON() ([]byte, error) {
	type plain APILoginAuth
	return marshalTagged(a.apiAuthKind(), plain(a))
}

func (r APIRecordsResults) MarshalJSON() ([]byte, error) {
	return marshalTagged(r.resultsKind(), struct{}{})
}

func (r APIJSONPathResults) MarshalJSON() ([]byte, error) {
	type plain APIJSONPathResults
	return marshalTagged(r.resultsKind(), plain(r))
}

func (r APIHeaderResults) MarshalJSON() ([]byte, error) {
	type plain APIHeaderResults
	return marshalTagged(r.resultsKind(), plain(r))
}

// ─── Decoding ────────────────────────────────────────────────────────

// variant decodes a union member (its "kind" is ignored) into the concrete
// type C, returned as the union interface U.
func variant[U, C any](raw []byte) (U, error) {
	var c C
	if err := json.Unmarshal(raw, &c); err != nil {
		var zero U
		return zero, err
	}
	return any(c).(U), nil
}

// decodeUnion reads a union member by its kind. JSON null (or nothing) is the
// union's zero value; an object with an unknown or missing kind is
// Unrecognized.
func decodeUnion[U any](data []byte, members map[string]func([]byte) (U, error)) (U, error) {
	var zero U
	if len(data) == 0 || string(data) == "null" {
		return zero, nil
	}
	var head struct {
		Kind string `json:"kind"`
	}
	if err := json.Unmarshal(data, &head); err != nil {
		return zero, err
	}
	decode, ok := members[head.Kind]
	if !ok {
		return any(Unrecognized{Raw: append(json.RawMessage(nil), data...)}).(U), nil
	}
	u, err := decode(data)
	if err != nil {
		return zero, fmt.Errorf("%s: %w", head.Kind, err)
	}
	return u, nil
}

// DecodeIngestionSource reads a source object by its kind.
func DecodeIngestionSource(data []byte) (IngestionSource, error) {
	return decodeUnion(data, map[string]func([]byte) (IngestionSource, error){
		"http":          variant[IngestionSource, HTTPSource],
		"sftp":          variant[IngestionSource, SFTPSource],
		"s3":            variant[IngestionSource, S3Source],
		"sqs":           variant[IngestionSource, SQSSource],
		"rabbitmq":      variant[IngestionSource, RabbitMQSource],
		"database-poll": variant[IngestionSource, DatabasePollSource],
		"api-poll":      variant[IngestionSource, APIPollSource],
		"custom":        variant[IngestionSource, CustomSource],
	})
}

// DecodeIngestionSplit reads a split object by its kind.
func DecodeIngestionSplit(data []byte) (IngestionSplit, error) {
	return decodeUnion(data, map[string]func([]byte) (IngestionSplit, error){
		"none": variant[IngestionSplit, NoSplit],
		"csv":  variant[IngestionSplit, CSVSplit],
		"json": variant[IngestionSplit, JSONSplit],
		"xml":  variant[IngestionSplit, XMLSplit],
	})
}

func decodeHTTPAuth(data []byte) (HTTPAuth, error) {
	return decodeUnion(data, map[string]func([]byte) (HTTPAuth, error){
		"flowcatalyst-token": variant[HTTPAuth, HTTPFlowCatalystTokenAuth],
		"basic":              variant[HTTPAuth, HTTPBasicAuth],
		"bearer":             variant[HTTPAuth, HTTPBearerAuth],
	})
}

func decodeTrack(data []byte) (DatabaseTrack, error) {
	return decodeUnion(data, map[string]func([]byte) (DatabaseTrack, error){
		"watermark": variant[DatabaseTrack, WatermarkTrack],
		"status":    variant[DatabaseTrack, StatusTrack],
	})
}

func decodeAPIAuth(data []byte) (APIAuth, error) {
	return decodeUnion(data, map[string]func([]byte) (APIAuth, error){
		"none":                     variant[APIAuth, APINoAuth],
		"basic":                    variant[APIAuth, APIBasicAuth],
		"bearer":                   variant[APIAuth, APIBearerAuth],
		"api-key":                  variant[APIAuth, APIKeyAuth],
		"oauth-client-credentials": variant[APIAuth, APIClientCredentialsAuth],
		"login":                    variant[APIAuth, APILoginAuth],
	})
}

func decodeResults(data []byte) (APIResults, error) {
	return decodeUnion(data, map[string]func([]byte) (APIResults, error){
		"records":   variant[APIResults, APIRecordsResults],
		"json-path": variant[APIResults, APIJSONPathResults],
		"header":    variant[APIResults, APIHeaderResults],
	})
}

// The unions inside a member decode through these, since encoding/json cannot
// pick a member of an interface by itself.

func (s *HTTPSource) UnmarshalJSON(data []byte) error {
	var aux struct {
		Auth      json.RawMessage `json:"auth"`
		RateLimit *RateLimit      `json:"rateLimit"`
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	auth, err := decodeHTTPAuth(aux.Auth)
	if err != nil {
		return fmt.Errorf("auth: %w", err)
	}
	*s = HTTPSource{Auth: auth, RateLimit: aux.RateLimit}
	return nil
}

func (s *DatabasePollSource) UnmarshalJSON(data []byte) error {
	type plain DatabasePollSource
	var aux struct {
		plain
		Track json.RawMessage `json:"track"`
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	track, err := decodeTrack(aux.Track)
	if err != nil {
		return fmt.Errorf("track: %w", err)
	}
	*s = DatabasePollSource(aux.plain)
	s.Track = track
	return nil
}

func (s *APIPollSource) UnmarshalJSON(data []byte) error {
	type plain APIPollSource
	var aux struct {
		plain
		Auth    json.RawMessage `json:"auth"`
		Results json.RawMessage `json:"results"`
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	auth, err := decodeAPIAuth(aux.Auth)
	if err != nil {
		return fmt.Errorf("auth: %w", err)
	}
	results, err := decodeResults(aux.Results)
	if err != nil {
		return fmt.Errorf("results: %w", err)
	}
	*s = APIPollSource(aux.plain)
	s.Auth, s.Results = auth, results
	return nil
}

// decodeParts reads the source and split of a definition.
func decodeParts(source, split json.RawMessage) (IngestionSource, IngestionSplit, error) {
	src, err := DecodeIngestionSource(source)
	if err != nil {
		return nil, nil, fmt.Errorf("source: %w", err)
	}
	spl, err := DecodeIngestionSplit(split)
	if err != nil {
		return nil, nil, fmt.Errorf("split: %w", err)
	}
	return src, spl, nil
}
