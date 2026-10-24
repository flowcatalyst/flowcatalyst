package ingestion

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecase"
)

// FieldError is one problem with a definition, at a dotted field path such as
// "source.auth.password".
type FieldError struct {
	Field   string `json:"field"`
	Code    string `json:"code"` // INVALID or NOT_NULL
	Message string `json:"message"`
}

// Problems is every field error found; empty means valid.
type Problems []FieldError

func (p Problems) String() string {
	parts := make([]string, len(p))
	for i, e := range p {
		parts[i] = e.Field + " " + e.Message
	}
	return strings.Join(parts, "; ")
}

// Err is nil when there are no problems, otherwise a 400 usecase error
// (INGESTION_INVALID) carrying the field errors in its details.
func (p Problems) Err() error {
	if len(p) == 0 {
		return nil
	}
	return usecase.Validation("INGESTION_INVALID", p.String()).WithDetails(map[string]any{"fields": []FieldError(p)})
}

func full(p string) *regexp.Regexp { return regexp.MustCompile("^(?:" + p + ")$") }

var (
	codePattern = full(`[a-z0-9]([a-z0-9-]{0,62}[a-z0-9])?`)
	// A reference to another record by code: pool, connection, service account.
	refPattern  = full(`[A-Za-z0-9][A-Za-z0-9_.:-]{0,99}`)
	jobCode     = full(`[a-z0-9-]+(:[a-z0-9-]+){3}`)
	hostPattern = full(`[A-Za-z0-9.-]{1,253}`)
	dirPattern  = full(`/?[A-Za-z0-9._-]+(/[A-Za-z0-9._-]+)*`)
	region      = full(`[a-z]{2}(-[a-z]+)+-\d`)
	sqsQueue    = full(`[A-Za-z0-9_-]{1,80}(\.fifo)?`)
	bucket      = full(`[a-z0-9][a-z0-9.-]{1,61}[a-z0-9]`)
	s3Prefix    = full(`[A-Za-z0-9!_*'()-][A-Za-z0-9!_.*'()-]*/([A-Za-z0-9!_.*'()-]+/)*`)
	tableName   = full(`[A-Za-z_][A-Za-z0-9_]*(\.[A-Za-z_][A-Za-z0-9_]*)?`)
	columnName  = full(`[A-Za-z_][A-Za-z0-9_]*`)
	jsonPath    = full(`\$(\.[A-Za-z0-9_-]+)*`)
	xmlName     = full(`[A-Za-z_][A-Za-z0-9._:-]*`)
	// $ then .field or [n] steps; a negative index counts from the end.
	apiPath    = full(`\$(\.[A-Za-z0-9_-]+|\[-?\d{1,6}\])*`)
	headerName = full("[A-Za-z0-9!#$%&'*+.^_`|~-]{1,100}")
	queryName  = full(`[A-Za-z0-9._~\[\]-]{1,100}`)
	// A path the runtime's client sends as it is: plain characters, no "//",
	// no . or .. segments.
	urlPath    = full(`(/[A-Za-z0-9._~!$&'()*+,;=:@-]*)*`)
	dotSegment = regexp.MustCompile(`(^|/)\.{1,2}(/|$)`)
	// An application's code in a token's applications claim: no ':'.
	applicationCode = full(`[A-Za-z0-9_-]{1,64}`)
	principalID     = full(`prn_[A-Za-z0-9]{1,64}`)
	accessKeyID     = full(`[A-Za-z0-9]{4,128}`)
	secretName      = full(`[a-z0-9]+(-[a-z0-9]+)*`)
	secretStore     = full(`[a-z][a-z0-9-]*`)
	// A custom source's type: lowercase words joined by '-'.
	customType  = full(`[a-z][a-z0-9]*(-[a-z0-9]+)*`)
	settingName = full(`[a-zA-Z][a-zA-Z0-9]*`)

	reservedHeaders = map[string]bool{
		"authorization": true, "host": true, "content-length": true,
		"transfer-encoding": true, "connection": true, "cookie": true,
	}
)

const (
	fastestPoll = time.Second
	slowestPoll = 24 * time.Hour
)

// ValidCode reports whether code is a valid ingestion code: lowercase letters,
// digits and '-', not at either end, at most 64.
func ValidCode(code string) bool { return codePattern.MatchString(code) }

// ValidateCode checks the identity part of an ingestion.
func ValidateCode(code string) Problems {
	var v checker
	v.matches("code", code, codePattern, "must be lowercase letters, digits and '-', not at either end, at most 64")
	return v.problems
}

// Validate checks the shape of a definition, every problem at once. Whether
// the pool, connection or service account exists, and whether the target
// address is allowed, is the Resolver's to say: shape only here.
func (d Definition) Validate() Problems {
	var v checker
	v.require("name", strings.TrimSpace(d.Name) != "" && len(d.Name) <= 200, "must be 1 to 200 characters")
	v.target(d.Target)
	v.split(d.Split)
	v.source(d.Source, d.Split)
	return v.problems
}

type checker struct {
	problems Problems
}

func (v *checker) add(field, code, message string) {
	v.problems = append(v.problems, FieldError{Field: field, Code: code, Message: message})
}

func (v *checker) require(field string, ok bool, message string) {
	if !ok {
		v.add(field, "INVALID", message)
	}
}

func (v *checker) matches(field, value string, p *regexp.Regexp, message string) {
	v.require(field, p.MatchString(value), message)
}

func (v *checker) target(t Target) {
	v.matches("target.code", t.Code, jobCode, "must be application:subdomain:aggregate:action")
	v.require("target.targetUrl", webAddress(t.TargetURL), "must be an http or https address")
	v.matches("target.pool", t.Pool, refPattern, "must be a dispatch pool's code")
	if t.Connection != nil {
		v.matches("target.connection", *t.Connection, refPattern, "must be a connection's code")
	}
	if t.ServiceAccount != nil {
		v.matches("target.serviceAccount", *t.ServiceAccount, refPattern, "must be a service account's code")
	}
	v.require("target.serviceAccount", t.Connection == nil || t.ServiceAccount == nil,
		"may not be set with a connection: the connection's service account signs")
	switch t.Mode {
	case ModeImmediate, ModeNextOnError, ModeBlockOnError:
	default:
		v.add("target.mode", "INVALID", "must be IMMEDIATE, NEXT_ON_ERROR or BLOCK_ON_ERROR")
	}
	v.require("target.batchSize", t.BatchSize >= 1 && t.BatchSize <= 1000, "must be 1 to 1000")
}

func (v *checker) split(s Split) {
	switch s := s.(type) {
	case nil:
		v.add("split", "NOT_NULL", "is required")
	case JSON:
		v.matches("split.arrayPath", s.ArrayPath, jsonPath, "must be $ or $.field.field")
	case XML:
		v.matches("split.element", s.Element, xmlName, "must be an element name")
	}
}

func (v *checker) source(src Source, split Split) {
	switch s := src.(type) {
	case nil:
		v.add("source", "NOT_NULL", "is required")
	case HTTP:
		v.httpAuth(s.Auth)
		if r := s.RateLimit; r != nil {
			per := time.Duration(r.Per)
			v.require("source.rateLimit.requests", r.Requests >= 1 && r.Requests <= 1_000_000, "must be 1 to 1000000")
			v.require("source.rateLimit.per", per >= time.Millisecond && per <= 24*time.Hour, "must be 1ms to 1 day")
			v.require("source.rateLimit", r.Requests < 1 || per/time.Duration(r.Requests) >= time.Microsecond,
				"may not allow more than one request a microsecond")
		}
	case Sftp:
		v.matches("source.host", s.Host, hostPattern, "must be a host name")
		v.require("source.port", s.Port >= 1 && s.Port <= 65535, "must be 1 to 65535")
		v.matches("source.directory", s.Directory, dirPattern, "may hold only letters, digits, '.', '_', '-' and '/'")
		v.require("source.username", strings.TrimSpace(s.Username) != "", "is required")
		v.secret("source.credential", s.Credential)
		v.every("source.every", s.Every)
	case RabbitMQ:
		u := parseURL(s.URI)
		v.require("source.uri", u != nil && (u.Scheme == "amqp" || u.Scheme == "amqps") && u.Host != "" && u.User == nil,
			"must be amqp://host or amqps://host, without a login")
		v.require("source.queue", strings.TrimSpace(s.Queue) != "" && len(s.Queue) <= 255, "is required")
		v.require("source.username", strings.TrimSpace(s.Username) != "", "is required")
		v.secret("source.password", s.Password)
	case Sqs:
		v.matches("source.region", s.Region, region, "must be an AWS region")
		v.matches("source.queue", s.Queue, sqsQueue, "must be a queue name, not its URL")
		v.credential(s.Credential)
	case S3:
		v.matches("source.region", s.Region, region, "must be an AWS region")
		v.matches("source.bucket", s.Bucket, bucket, "must be a bucket name")
		if s.Prefix != nil {
			v.matches("source.prefix", *s.Prefix, s3Prefix, "must be folder names, each ending in '/', not starting with '.'")
		}
		v.every("source.every", s.Every)
		v.credential(s.Credential)
	case DatabasePoll:
		v.databasePoll(s, split)
	case APIPoll:
		v.apiPoll(s)
	case Custom:
		v.custom(s)
	}
}

func (v *checker) httpAuth(a HTTPAuth) {
	switch a := a.(type) {
	case nil:
		v.add("source.auth", "NOT_NULL", "is required")
	case FlowCatalystToken:
		v.matches("source.auth.application", a.Application, applicationCode, "must be an application's code")
		if a.PrincipalID != nil {
			v.matches("source.auth.principalId", *a.PrincipalID, principalID,
				"must be a principal id, prn_...; a service account's sac_ id is not in its tokens")
		}
	case BasicAuth:
		v.require("source.auth.username", strings.TrimSpace(a.Username) != "" && !strings.Contains(a.Username, ":"),
			"is required and may not contain ':'")
		v.secret("source.auth.password", a.Password)
	case BearerAuth:
		v.secret("source.auth.token", a.Token)
	}
}

func (v *checker) databasePoll(p DatabasePoll, split Split) {
	v.require("source.jdbcUrl", strings.HasPrefix(p.JDBCURL, "jdbc:"), "must start with 'jdbc:'")
	v.require("source.username", strings.TrimSpace(p.Username) != "", "is required")
	v.secret("source.password", p.Password)
	v.matches("source.table", p.Table, tableName, "must be a plain name, or schema.table")
	switch t := p.Track.(type) {
	case nil:
		v.add("source.track", "NOT_NULL", "is required")
	case Watermark:
		v.matches("source.track.column", t.Column, columnName, "must be a plain column name")
		v.require("source.track.type", t.Type == "NUMBER" || t.Type == "TIMESTAMP", "must be NUMBER or TIMESTAMP")
	case StatusTrack:
		v.matches("source.track.column", t.Column, columnName, "must be a plain column name")
		v.matches("source.track.keyColumn", t.KeyColumn, columnName, "must be a plain column name")
		v.require("source.track.pending", t.Pending != "", "is required")
		v.require("source.track.processed", t.Processed != "" && t.Processed != t.Pending,
			"is required and must differ from the pending value")
	}
	v.require("source.maxRows", p.MaxRows >= 1 && p.MaxRows <= 10_000, "must be 1 to 10000")
	v.every("source.every", p.Every)
	if split != nil {
		_, none := split.(None)
		v.require("split", none, "must be 'none' for a database poll: each row is one record")
	}
}

func (v *checker) apiPoll(p APIPoll) {
	v.require("source.url", apiAddress(p.URL), "must be an http or https address, without a login or a #fragment")
	v.require("source.method", p.Method == "GET" || p.Method == "POST", "must be GET or POST")
	for i, h := range p.Headers {
		field := fmt.Sprintf("source.headers.%d", i)
		v.matches(field+".name", h.Name, headerName, "must be a header name")
		v.require(field+".name", !reservedHeaders[strings.ToLower(h.Name)], "is set by the runtime; credentials go in the login")
		v.require(field+".value", !strings.ContainsAny(h.Value, "\r\n\x00"), "may not hold a line break")
	}
	v.require("source.body", p.Body == nil || p.Method == "POST", "is sent with a POST only")
	v.apiAuth(p.Auth)
	switch r := p.Results.(type) {
	case nil:
		v.add("source.results", "NOT_NULL", "is required")
	case JSONPathResults:
		v.matches("source.results.path", r.Path, apiPath, "must be $ followed by .field or [n] steps")
	case HeaderResults:
		v.matches("source.results.name", r.Name, headerName, "must be a header name")
	}
	if c := p.Cursor; c != nil {
		v.matches("source.cursor.from", c.From, apiPath, "must be $ followed by .field or [n] steps")
		v.matches("source.cursor.param", c.Param, queryName, "must be a query parameter name")
	}
	v.every("source.every", p.Every)
	v.every("source.idleDelay", p.IdleDelay)
}

func (v *checker) apiAuth(a APIAuth) {
	const badAddress = "must be an http or https address, without a login or a #fragment"
	switch a := a.(type) {
	case nil:
		v.add("source.auth", "NOT_NULL", "is required")
	case APIBasic:
		v.require("source.auth.username", strings.TrimSpace(a.Username) != "" && !strings.Contains(a.Username, ":"),
			"is required and may not contain ':'")
		v.secret("source.auth.password", a.Password)
	case APIBearer:
		v.secret("source.auth.token", a.Token)
	case APIKey:
		v.matches("source.auth.header", a.Header, headerName, "must be a header name")
		v.secret("source.auth.value", a.Value)
	case ClientCredentials:
		v.require("source.auth.tokenUrl", apiAddress(a.TokenURL), badAddress)
		v.require("source.auth.clientId", strings.TrimSpace(a.ClientID) != "", "is required")
		v.secret("source.auth.clientSecret", a.ClientSecret)
		v.require("source.auth.credentialsIn", a.CredentialsIn == "BASIC_HEADER" || a.CredentialsIn == "BODY",
			"must be BASIC_HEADER or BODY")
	case Login:
		v.require("source.auth.url", apiAddress(a.URL), badAddress)
		v.require("source.auth.format", a.Format == "FORM" || a.Format == "JSON", "must be FORM or JSON")
		v.require("source.auth.usernameField", strings.TrimSpace(a.UsernameField) != "", "is required")
		v.require("source.auth.passwordField", strings.TrimSpace(a.PasswordField) != "", "is required")
		v.require("source.auth.username", strings.TrimSpace(a.Username) != "", "is required")
		v.secret("source.auth.password", a.Password)
		v.matches("source.auth.tokenPath", a.TokenPath, apiPath, "must be $ followed by .field or [n] steps")
		if a.ExpiresInPath != nil {
			v.matches("source.auth.expiresInPath", *a.ExpiresInPath, apiPath, "must be $ followed by .field or [n] steps")
		}
		lifetime := time.Duration(a.Lifetime)
		v.require("source.auth.lifetime", lifetime >= 10*time.Second && lifetime <= 24*time.Hour, "must be 10 seconds to a day")
		v.matches("source.auth.header", a.Header, headerName, "must be a header name")
		v.require("source.auth.prefix", !strings.ContainsAny(a.Prefix, "\r\n"), "may not hold a line break")
	}
}

// custom checks what the platform can know of a runtime-supplied kind: its
// type's form, and that the settings are plain values. The kind's own fields
// are the runtime's to validate (it reports them; camel-route's YAML is
// checked by the runtime at test time and when it takes the lease).
func (v *checker) custom(c Custom) {
	if !customType.MatchString(c.Type) {
		v.add("source.type", "INVALID", "must be lowercase words joined by '-', such as camel-route")
		return
	}
	v.require("source.type", !IsBuiltInKind(c.Type), "is a built-in kind: write it with that kind")
	for name, value := range c.Settings {
		field := "source.settings." + name
		v.matches(field, name, settingName, "must be a letter then letters and digits")
		v.require(field, name != "kind", "is the kind's own key in a definition file")
		v.require(field, plainSetting(value), "must be text, a number, true or false, or a list of text")
	}
}

// plainSetting reports whether value is what an extension's setting may hold:
// text, a number, a boolean, or a list of text.
func plainSetting(value any) bool {
	switch x := value.(type) {
	case string, bool, float64, int, int64:
		return true
	case []any:
		for _, item := range x {
			if _, ok := item.(string); !ok {
				return false
			}
		}
		return true
	case []string:
		return true
	}
	return false
}

func (v *checker) credential(c *AWSCredential) {
	if c == nil {
		return
	}
	v.matches("source.credential.accessKeyId", c.AccessKeyID, accessKeyID, "must be an access key id")
	v.secret("source.credential.secretAccessKey", c.SecretAccessKey)
}

func (v *checker) every(field string, d Duration) {
	t := time.Duration(d)
	v.require(field, t >= fastestPoll && t <= slowestPoll, "must be between one second and one day")
}

// secret checks a secret is named, never given: name or store:name, the form
// the runtime's secret stores read.
func (v *checker) secret(field string, ref SecretRef) {
	if ref.Name == "" {
		v.add(field, "NOT_NULL", "must name a secret")
		return
	}
	if !validSecretName(ref.Name) {
		v.add(field+".name", "INVALID", "must be a secret's name, such as sftp-password or files:sftp-password")
	}
}

func validSecretName(s string) bool {
	store, name, hasStore := strings.Cut(s, ":")
	if !hasStore {
		name = store
	} else if len(store) > 100 || !secretStore.MatchString(store) {
		return false
	}
	return len(name) <= 100 && secretName.MatchString(name)
}

func parseURL(s string) *url.URL {
	u, err := url.Parse(s)
	if err != nil {
		return nil
	}
	return u
}

func webAddress(s string) bool {
	u := parseURL(s)
	return u != nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}

// apiAddress is a webAddress without a login or a #fragment, whose path is
// plain characters with no "//" and no . or .. segments.
func apiAddress(s string) bool {
	u := parseURL(s)
	if u == nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || u.Fragment != "" {
		return false
	}
	path := u.EscapedPath()
	return urlPath.MatchString(path) && !strings.Contains(path, "//") && !dotSegment.MatchString(path)
}
