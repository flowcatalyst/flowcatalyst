package pages

import (
	"cmp"
	"encoding/json"
	"fmt"
	"log/slog"
	"maps"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/ingestion"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/kit"
)

// The ingestion form is drawn from data: each source kind is a list of
// fields, each field says where in the definition's JSON it lives (Path), how
// it is typed, and when it applies (When). One reader turns the posted values
// into the definition JSON the API takes, one turns a stored definition back
// into form values, and a validation error's path finds its field. A kind a
// runtime reports describes its settings the same way, so the platform
// renders it without knowing it.

type ingsType int

const (
	ingsText ingsType = iota
	ingsNumber
	ingsDuration
	// ingsSecret is the NAME of a secret, written as {"name": ...}; never a value.
	ingsSecret
	ingsChoice
	// ingsSwitch is a boolean at its Path, or, with Presence, whether the
	// object at its Path exists (its other fields are When this is on).
	ingsSwitch
	ingsArea
	// ingsList is one value per line, a list of text.
	ingsList
	// ingsCode is text in the code editor island (Lang yaml or json).
	ingsCode
	// ingsPairs is one "name: value" a line (Sep splits it): headers are a list
	// of {name, value}, a map's entries (Map) a JSON object.
	ingsPairs
	// ingsJSON is a JSON object in the code editor, for settings no schema describes.
	ingsJSON
)

// ingsOn is what a switch posts.
const ingsOn = "on"

type ingsWhen struct{ Field, Is string }

type ingsField struct {
	// Name is the control's name, unique in the form; Path the dotted path of
	// its value in the definition JSON ("source.auth.token").
	Name, Path               string
	Label, Help, Placeholder string
	Type                     ingsType
	Required                 bool
	// Optional: a blank value leaves the key out (the kind's default applies).
	Optional bool
	NoTrim   bool
	Options  []kit.Option
	Default  string
	Min      string
	Lang     string
	Sep      string
	Map      bool
	Presence bool
	// Plain: a secret setting of an extension is the secret's name as text,
	// not a {"name"} reference.
	Plain bool
	When  *ingsWhen
	// control: other fields depend on this one, so it is bound to a signal.
	control bool
}

// ingsKind is one source kind the form offers.
type ingsKind struct {
	// Name is the kind as the list filter and the definition's "kind" write
	// it; for an extension the type of a custom source.
	Name, Label, Description string
	BuiltIn, SingleOwner     bool
	// Perm is the permission creating or changing an ingestion of the kind
	// needs ("" for none).
	Perm string
	// Reported is false for a kind of a stored ingestion that no live
	// runtime reports now.
	Reported bool
	Fields   []ingsField
}

// ingsKindLabels are the Nuxt pages' names for the built-in kinds.
var ingsKindLabels = map[string]string{
	ingestion.KindHTTP:         "HTTP push",
	ingestion.KindSFTP:         "SFTP",
	ingestion.KindS3:           "S3",
	ingestion.KindSQS:          "SQS",
	ingestion.KindRabbitMQ:     "RabbitMQ",
	ingestion.KindDatabasePoll: "Database poll",
	ingestion.KindAPIPoll:      "API poll",
}

// ingsSignal is the signal that carries the value of a control other fields
// depend on: "http.auth" is ingsHttpAuth.
func ingsSignal(name string) string {
	var b strings.Builder
	b.WriteString("ing")
	for part := range strings.FieldsFuncSeq(name, func(r rune) bool { return r < '0' || r > '9' && r < 'A' || r > 'Z' && r < 'a' || r > 'z' }) {
		b.WriteString(strings.ToUpper(part[:1]) + part[1:])
	}
	return b.String()
}

func ingsIs(field, value string) *ingsWhen { return &ingsWhen{Field: field, Is: value} }

func ingsOpts(pairs ...string) []kit.Option {
	out := make([]kit.Option, 0, len(pairs)/2)
	for i := 0; i+1 < len(pairs); i += 2 {
		out = append(out, kit.Option{Value: pairs[i], Label: pairs[i+1]})
	}
	return out
}

// secret is a secret-name field with the name the Nuxt pages suggest.
func ingsSecretField(name, path, label, def string, when *ingsWhen) ingsField {
	return ingsField{
		Name: name, Path: path, Label: label, Type: ingsSecret, Required: true, Default: def, When: when,
		Help: "The secret's name, never its value. It is read for this ingestion's client.",
	}
}

func ingsEvery(name, path, label, def string) ingsField {
	return ingsField{
		Name: name, Path: path, Label: label, Type: ingsDuration, Required: true, Default: def,
		Help: "Such as 30s, 5m, 2h, 1d or PT30S.",
	}
}

// ingsAWSKey is the optional access key of an SQS or S3 source.
func ingsAWSKey(prefix string) []ingsField {
	return []ingsField{
		{
			Name: prefix + ".key", Path: "source.credential", Type: ingsSwitch, Presence: true, Label: "Read with its own access key",
			Help: "Off, the runtime's own AWS identity is used.",
		},
		{Name: prefix + ".accessKeyId", Path: "source.credential.accessKeyId", Label: "Access key id", Type: ingsText, Required: true, When: ingsIs(prefix+".key", ingsOn)},
		ingsSecretField(prefix+".secretAccessKey", "source.credential.secretAccessKey", "Secret access key (secret name)", "aws-secret-access-key", ingsIs(prefix+".key", ingsOn)),
	}
}

func ingsBuiltinKinds() []ingsKind {
	kinds := []ingsKind{
		{Name: ingestion.KindHTTP, Description: "Callers post JSON to POST /ingest/{client}/{code} on each runtime.", Fields: []ingsField{
			{
				Name: "http.auth", Path: "source.auth.kind", Label: "Callers authenticate with", Type: ingsChoice, Default: "bearer",
				Options: ingsOpts("bearer", "A fixed bearer token", "basic", "A user name and password (basic)", "flowcatalyst-token", "A FlowCatalyst access token"),
			},
			ingsSecretField("http.token", "source.auth.token", "Token (secret name)", "push-token", ingsIs("http.auth", "bearer")),
			{Name: "http.username", Path: "source.auth.username", Label: "User name", Type: ingsText, Required: true, When: ingsIs("http.auth", "basic")},
			ingsSecretField("http.password", "source.auth.password", "Password (secret name)", "push-password", ingsIs("http.auth", "basic")),
			{
				Name: "http.application", Path: "source.auth.application", Label: "Application", Type: ingsText, Required: true, When: ingsIs("http.auth", "flowcatalyst-token"),
				Help: "The caller's token must reach this client and this application (its code).",
			},
			{
				Name: "http.principal", Path: "source.auth.principalId", Label: "Only this principal", Type: ingsText, Optional: true, When: ingsIs("http.auth", "flowcatalyst-token"),
				Placeholder: "any principal with access", Help: "Optional. A principal id (prn_...); a service account's sac_ id is not in its tokens.",
			},
			{
				Name: "http.limited", Path: "source.rateLimit", Type: ingsSwitch, Presence: true, Label: "Limit how fast requests are accepted",
				Help: "A caller over the limit gets 429.",
			},
			{Name: "http.requests", Path: "source.rateLimit.requests", Label: "Requests", Type: ingsNumber, Default: "60", Min: "1", When: ingsIs("http.limited", ingsOn)},
			{Name: "http.per", Path: "source.rateLimit.per", Label: "Per", Type: ingsDuration, Default: "PT1M", When: ingsIs("http.limited", ingsOn), Help: "Such as 1s, 1m or PT1M."},
		}},
		{Name: ingestion.KindSFTP, SingleOwner: true, Description: "Polls the files in a directory of an SFTP server.", Fields: []ingsField{
			{Name: "sftp.host", Path: "source.host", Label: "Host", Type: ingsText, Required: true, Placeholder: "sftp.example.com"},
			{Name: "sftp.port", Path: "source.port", Label: "Port", Type: ingsNumber, Required: true, Default: "22", Min: "1"},
			{
				Name: "sftp.directory", Path: "source.directory", Label: "Directory", Type: ingsText, Required: true, Placeholder: "/out",
				Help: "Files done move to .done/ beside it, unreadable ones to .rejected/.",
			},
			{Name: "sftp.include", Path: "source.include", Label: "Files", Type: ingsText, Optional: true, Placeholder: "*.csv", Help: "A pattern such as *.csv; empty for every file."},
			{Name: "sftp.username", Path: "source.username", Label: "User name", Type: ingsText, Required: true},
			ingsSecretField("sftp.credential", "source.credential", "Password or private key (secret name)", "sftp-password", nil),
			{
				Name: "sftp.knownHosts", Path: "source.knownHosts", Label: "Server's host key", Type: ingsArea, Optional: true,
				Help: "Its known_hosts line. Without it the runtime refuses to connect, unless any host key is accepted below.",
			},
			{
				Name: "sftp.acceptAnyHostKey", Path: "source.acceptAnyHostKey", Type: ingsSwitch, Label: "Accept any host key",
				Help: "Connects without checking who the server is, so whoever answers at the address gets the credential. Needs platform:ingestion:ingestion:bind-secrets; not with a host key.",
			},
			ingsEvery("sftp.every", "source.every", "Poll every", "PT1M"),
		}},
		{Name: ingestion.KindS3, SingleOwner: true, Description: "Polls the objects directly under a prefix of a bucket.", Fields: append([]ingsField{
			{Name: "s3.region", Path: "source.region", Label: "Region", Type: ingsText, Required: true, Default: "eu-west-1", Placeholder: "eu-west-1"},
			{Name: "s3.bucket", Path: "source.bucket", Label: "Bucket", Type: ingsText, Required: true},
			{
				Name: "s3.prefix", Path: "source.prefix", Label: "Prefix", Type: ingsText, Optional: true, Placeholder: "orders/",
				Help: "Folders ending in /, such as orders/; empty for the top of the bucket. Objects done move under .done/.",
			},
			ingsEvery("s3.every", "source.every", "Poll every", "PT5M"),
		}, ingsAWSKey("s3")...)},
		{Name: ingestion.KindSQS, Description: "Consumes the messages of an SQS queue.", Fields: append([]ingsField{
			{Name: "sqs.region", Path: "source.region", Label: "Region", Type: ingsText, Required: true, Default: "eu-west-1", Placeholder: "eu-west-1"},
			{Name: "sqs.queue", Path: "source.queue", Label: "Queue", Type: ingsText, Required: true, Help: "The queue's name, not its URL."},
		}, ingsAWSKey("sqs")...)},
		{Name: ingestion.KindRabbitMQ, Description: "Consumes the messages of a RabbitMQ queue.", Fields: []ingsField{
			{
				Name: "rabbitmq.uri", Path: "source.uri", Label: "Broker", Type: ingsText, Required: true, Placeholder: "amqps://mq.example.com:5671/vhost",
				Help: "amqp:// or amqps://, without a login.",
			},
			{Name: "rabbitmq.queue", Path: "source.queue", Label: "Queue", Type: ingsText, Required: true},
			{Name: "rabbitmq.username", Path: "source.username", Label: "User name", Type: ingsText, Required: true},
			ingsSecretField("rabbitmq.password", "source.password", "Password (secret name)", "rabbitmq-password", nil),
		}},
		{Name: ingestion.KindDatabasePoll, SingleOwner: true, Description: "Reads the rows of a table or view; each row is one record.", Fields: []ingsField{
			{
				Name: "dbpoll.jdbcUrl", Path: "source.jdbcUrl", Label: "Database", Type: ingsText, Required: true, Default: "jdbc:postgresql://", Placeholder: "jdbc:postgresql://db.example.com/erp",
				Help: "A JDBC URL. Only PostgreSQL's driver is in the runtime.",
			},
			{Name: "dbpoll.username", Path: "source.username", Label: "User name", Type: ingsText, Required: true},
			ingsSecretField("dbpoll.password", "source.password", "Password (secret name)", "db-password", nil),
			{Name: "dbpoll.table", Path: "source.table", Label: "Table", Type: ingsText, Required: true, Help: "A table or view, optionally schema.table."},
			{
				Name: "dbpoll.track", Path: "source.track.kind", Label: "New rows are told by", Type: ingsChoice, Default: "watermark",
				Options: ingsOpts("watermark", "A column that only grows (watermark)", "status", "A status column (pending, processed)"),
			},
			{
				Name: "dbpoll.wmColumn", Path: "source.track.column", Label: "Column", Type: ingsText, Required: true, Default: "id", When: ingsIs("dbpoll.track", "watermark"),
				Help: "The highest value read is kept by the platform; nothing is written to the table.",
			},
			{
				Name: "dbpoll.wmType", Path: "source.track.type", Label: "It holds", Type: ingsChoice, Default: "NUMBER", When: ingsIs("dbpoll.track", "watermark"),
				Options: ingsOpts("NUMBER", "A number", "TIMESTAMP", "A timestamp"),
			},
			{Name: "dbpoll.statusColumn", Path: "source.track.column", Label: "Status column", Type: ingsText, Required: true, When: ingsIs("dbpoll.track", "status")},
			{
				Name: "dbpoll.keyColumn", Path: "source.track.keyColumn", Label: "Key column", Type: ingsText, Required: true, When: ingsIs("dbpoll.track", "status"),
				Help: "Identifies a row, usually the primary key.",
			},
			{Name: "dbpoll.pending", Path: "source.track.pending", Label: "Pending value", Type: ingsText, Required: true, When: ingsIs("dbpoll.track", "status")},
			{Name: "dbpoll.processed", Path: "source.track.processed", Label: "Processed value", Type: ingsText, Required: true, When: ingsIs("dbpoll.track", "status")},
			{Name: "dbpoll.maxRows", Path: "source.maxRows", Label: "Rows per poll", Type: ingsNumber, Required: true, Default: "500", Min: "1"},
			ingsEvery("dbpoll.every", "source.every", "Poll every", "PT1M"),
		}},
		{Name: ingestion.KindAPIPoll, SingleOwner: true, Description: "Asks an HTTP API for data and splits the answer.", Fields: ingsAPIPollFields()},
	}
	for i := range kinds {
		kinds[i].BuiltIn, kinds[i].Reported = true, true
		kinds[i].Label = ingsKindLabels[kinds[i].Name]
		ingsMarkControls(kinds[i].Fields)
	}
	return kinds
}

func ingsAPIPollFields() []ingsField {
	api := func(local string) string { return "api." + local }
	auth := func(v string) *ingsWhen { return ingsIs("api.auth", v) }
	return []ingsField{
		{Name: api("method"), Path: "source.method", Label: "Method", Type: ingsChoice, Default: "GET", Options: ingsOpts("GET", "GET", "POST", "POST")},
		{
			Name: api("url"), Path: "source.url", Label: "Address", Type: ingsText, Required: true, Placeholder: "https://api.example.com/v1/orders?status=open",
			Help: "With any fixed query parameters.",
		},
		{
			Name: api("headers"), Path: "source.headers", Label: "Headers", Type: ingsPairs, Sep: ":", Optional: true, Placeholder: "Accept: application/json",
			Help: "One name: value a line, sent on every request; none of them secret. The login sets Authorization.",
		},
		{Name: api("body"), Path: "source.body", Label: "Body", Type: ingsArea, Optional: true, When: ingsIs("api.method", "POST"), Help: "JSON, sent with every request."},
		{
			Name: api("auth"), Path: "source.auth.kind", Label: "It logs in with", Type: ingsChoice, Default: "none",
			Options: ingsOpts("none", "No login", "basic", "A user name and password (basic)", "bearer", "A fixed bearer token", "api-key", "A key in a header",
				"oauth-client-credentials", "OAuth client credentials", "login", "The API's own login endpoint"),
		},
		{Name: api("basicUser"), Path: "source.auth.username", Label: "User name", Type: ingsText, Required: true, When: auth("basic")},
		ingsSecretField(api("basicPassword"), "source.auth.password", "Password (secret name)", "api-password", auth("basic")),
		ingsSecretField(api("bearerToken"), "source.auth.token", "Token (secret name)", "api-token", auth("bearer")),
		{Name: api("keyHeader"), Path: "source.auth.header", Label: "Header", Type: ingsText, Required: true, Default: "X-Api-Key", When: auth("api-key")},
		ingsSecretField(api("keyValue"), "source.auth.value", "Key (secret name)", "api-key", auth("api-key")),
		{Name: api("oauthTokenUrl"), Path: "source.auth.tokenUrl", Label: "Token endpoint", Type: ingsText, Required: true, Placeholder: "https://auth.example.com/oauth/token", When: auth("oauth-client-credentials")},
		{Name: api("oauthClientId"), Path: "source.auth.clientId", Label: "Client id", Type: ingsText, Required: true, When: auth("oauth-client-credentials")},
		ingsSecretField(api("oauthSecret"), "source.auth.clientSecret", "Client secret (secret name)", "api-client-secret", auth("oauth-client-credentials")),
		{Name: api("oauthScope"), Path: "source.auth.scope", Label: "Scope", Type: ingsText, Optional: true, When: auth("oauth-client-credentials")},
		{Name: api("oauthAudience"), Path: "source.auth.audience", Label: "Audience", Type: ingsText, Optional: true, When: auth("oauth-client-credentials")},
		{
			Name: api("oauthIn"), Path: "source.auth.credentialsIn", Label: "Send the client id and secret in", Type: ingsChoice, Default: "BASIC_HEADER", When: auth("oauth-client-credentials"),
			Options: ingsOpts("BASIC_HEADER", "A basic authorization header", "BODY", "The request body"),
		},
		{Name: api("loginUrl"), Path: "source.auth.url", Label: "Login endpoint", Type: ingsText, Required: true, Placeholder: "https://api.example.com/login", When: auth("login")},
		{Name: api("loginFormat"), Path: "source.auth.format", Label: "Posted as", Type: ingsChoice, Default: "JSON", Options: ingsOpts("JSON", "JSON", "FORM", "A form"), When: auth("login")},
		{Name: api("loginUserField"), Path: "source.auth.usernameField", Label: "User name field", Type: ingsText, Required: true, Default: "username", When: auth("login")},
		{Name: api("loginPassField"), Path: "source.auth.passwordField", Label: "Password field", Type: ingsText, Required: true, Default: "password", When: auth("login")},
		{Name: api("loginUser"), Path: "source.auth.username", Label: "User name", Type: ingsText, Required: true, When: auth("login")},
		ingsSecretField(api("loginPassword"), "source.auth.password", "Password (secret name)", "api-password", auth("login")),
		{
			Name: api("loginExtra"), Path: "source.auth.extraFields", Label: "Other fields", Type: ingsPairs, Sep: "=", Map: true, Optional: true, Placeholder: "grant_type=password",
			Help: "One name=value a line.", When: auth("login"),
		},
		{Name: api("loginTokenPath"), Path: "source.auth.tokenPath", Label: "Token in the answer at", Type: ingsText, Required: true, Default: "$.token", When: auth("login")},
		{Name: api("loginExpires"), Path: "source.auth.expiresInPath", Label: "Seconds it lasts at", Type: ingsText, Optional: true, Placeholder: "$.expires_in", When: auth("login"), Help: "Optional."},
		{Name: api("loginLifetime"), Path: "source.auth.lifetime", Label: "Otherwise kept for", Type: ingsDuration, Required: true, Default: "PT15M", When: auth("login"), Help: "Such as 15m or PT15M."},
		{Name: api("loginHeader"), Path: "source.auth.header", Label: "Sent in header", Type: ingsText, Required: true, Default: "Authorization", When: auth("login")},
		{Name: api("loginPrefix"), Path: "source.auth.prefix", Label: "Before the token", Type: ingsText, NoTrim: true, Default: "Bearer ", When: auth("login"), Help: "With its space."},
		{
			Name: api("results"), Path: "source.results.kind", Label: "An answer has results when", Type: ingsChoice, Default: "records",
			Options: ingsOpts("records", "The split makes at least one record", "json-path", "A path in the answer holds something", "header", "The answer carries a header"),
			Help:    "An answer without results is not dispatched.",
		},
		{
			Name: api("resultsPath"), Path: "source.results.path", Label: "Path", Type: ingsText, Required: true, Default: "$.items", When: ingsIs("api.results", "json-path"),
			Help: "A non-empty array, or a value other than null, false or empty: $.items, $.hasMore.",
		},
		{Name: api("resultsHeader"), Path: "source.results.name", Label: "Header", Type: ingsText, Required: true, Default: "X-Has-More", When: ingsIs("api.results", "header")},
		{Name: api("resultsEquals"), Path: "source.results.equals", Label: "With the value", Type: ingsText, Optional: true, When: ingsIs("api.results", "header"), Help: "Optional; ignoring case."},
		{
			Name: api("cursor"), Path: "source.cursor", Type: ingsSwitch, Presence: true, Label: "Move each request on with a value from the last answer",
			Help: "A next-page token, or the last item's id or time. Kept between polls.",
		},
		{Name: api("cursorFrom"), Path: "source.cursor.from", Label: "From the answer at", Type: ingsText, Required: true, Default: "$.next", When: ingsIs("api.cursor", ingsOn)},
		{Name: api("cursorParam"), Path: "source.cursor.param", Label: "Sent as parameter", Type: ingsText, Required: true, Default: "cursor", When: ingsIs("api.cursor", ingsOn)},
		{Name: api("cursorInitial"), Path: "source.cursor.initial", Label: "First value", Type: ingsText, Optional: true, When: ingsIs("api.cursor", ingsOn), Help: "Optional."},
		ingsEvery(api("every"), "source.every", "After results, ask again in", "PT5S"),
		ingsEvery(api("idleDelay"), "source.idleDelay", "After none, wait", "PT60S"),
	}
}

// ingsMarkControls flags the fields other fields depend on, so they are bound to
// a signal.
func ingsMarkControls(fields []ingsField) {
	for _, f := range fields {
		if f.When == nil {
			continue
		}
		for i := range fields {
			if fields[i].Name == f.When.Field {
				fields[i].control = true
			}
		}
	}
}

// ── split and target ───────────────────────────────────────────────────

// ingsSplitFields are the split's fields (the Nuxt form's Split section).
var ingsSplitFields = func() []ingsField {
	fields := []ingsField{
		{
			Name: "split.kind", Path: "split.kind", Label: "Split", Type: ingsChoice, Default: "json",
			Options: ingsOpts("none", "One record per payload", "csv", "CSV lines", "json", "JSON array elements", "xml", "XML elements"),
		},
		{Name: "split.header", Path: "split.header", Label: "The first line is a header", Type: ingsSwitch, Default: ingsOn, When: ingsIs("split.kind", "csv")},
		{
			Name: "split.arrayPath", Path: "split.arrayPath", Label: "Array path", Type: ingsText, Required: true, Default: "$", When: ingsIs("split.kind", "json"),
			Help: "$ for a top-level array, or $.field.field.",
		},
		{Name: "split.element", Path: "split.element", Label: "Element", Type: ingsText, Required: true, Default: "record", When: ingsIs("split.kind", "xml")},
	}
	ingsMarkControls(fields)
	return fields
}()

// ingsTargetFields are the dispatch job each record becomes.
var ingsTargetFields = []ingsField{
	{
		Name: "target.code", Path: "target.code", Label: "Code", Type: ingsText, Required: true, Placeholder: "application:subdomain:aggregate:action",
		Help: "application:subdomain:aggregate:action, in lower case.",
	},
	{Name: "target.targetUrl", Path: "target.targetUrl", Label: "Target URL", Type: ingsText, Required: true, Placeholder: "https://app.example.com/hooks/orders"},
	{Name: "target.pool", Path: "target.pool", Label: "Dispatch pool", Type: ingsText, Required: true, Help: "The pool's code; it must be active and usable by this client."},
	{
		Name: "target.connection", Path: "target.connection", Label: "Connection", Type: ingsText, Optional: true,
		Help: "Optional: a connection's code. Its service account signs each delivery, and pausing it holds this ingestion's jobs.",
	},
	{
		Name: "target.serviceAccount", Path: "target.serviceAccount", Label: "Signed by", Type: ingsText, Optional: true,
		Help: "Optional: a service account's code, for signing without a connection. Not with a connection.",
	},
	{
		Name: "target.mode", Path: "target.mode", Label: "Mode", Type: ingsChoice, Default: ingestion.ModeImmediate,
		Options: ingsOpts(ingestion.ModeImmediate, "Immediate (no ordering)", ingestion.ModeNextOnError, "In order, move on after a failure", ingestion.ModeBlockOnError, "In order, block on a failure"),
	},
	{Name: "target.batchSize", Path: "target.batchSize", Label: "Batch size", Type: ingsNumber, Required: true, Default: "100", Min: "1", Help: "Jobs sent to the platform per call (1 to 1000)."},
}

// ── extension kinds ────────────────────────────────────────────────────

// ingsKindName is a runtime kind's name (the report's own rule): it ends up
// in element ids and expressions, so nothing else is drawn.
var ingsKindName = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,49}$`)

// ingsSettingName is a setting's name as a definition may hold it (the
// validator's own rule); a reported field that does not fit is skipped, so a
// runtime's report never reaches an attribute or a path unchecked.
var ingsSettingName = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9]{0,63}$`)

// ingsSettingSpec is one field of a kind's settings as a runtime describes it
// (the shape the Nuxt control plane's source kinds use).
type ingsSettingSpec struct {
	Name         string   `json:"name"`
	Label        string   `json:"label"`
	Description  string   `json:"description"`
	Type         string   `json:"type"`
	Required     bool     `json:"required"`
	DefaultValue *string  `json:"defaultValue"`
	Choices      []string `json:"choices"`
	Min          string   `json:"min"`
}

// ingsSettingFields reads the settings a runtime reports for kind: a list of
// fields, or {"fields": [...]}. Anything else is no description, and the
// settings are edited as JSON.
func ingsSettingFields(kind string, raw json.RawMessage) []ingsField {
	var specs []ingsSettingSpec
	if err := json.Unmarshal(raw, &specs); err != nil {
		var wrapped struct {
			Fields []ingsSettingSpec `json:"fields"`
		}
		if json.Unmarshal(raw, &wrapped) == nil {
			specs = wrapped.Fields
		}
	}
	var out []ingsField
	for _, s := range specs {
		if !ingsSettingName.MatchString(s.Name) {
			slog.Warn("ingestion runtime reported a setting the form cannot draw", "kind", kind, "setting", s.Name)
			continue
		}
		f := ingsField{
			Name: kind + ".s." + s.Name, Path: "source.settings." + s.Name, Label: cmp.Or(s.Label, s.Name), Help: s.Description,
			Required: s.Required && s.DefaultValue == nil, Optional: true, Default: deref(s.DefaultValue), Min: s.Min,
		}
		switch strings.ToUpper(s.Type) {
		case "MULTILINE":
			f.Type = ingsArea
			switch s.Name {
			case "yaml":
				f.Type, f.Lang = ingsCode, "yaml"
			case "json":
				f.Type, f.Lang = ingsCode, "json"
			}
		case "INTEGER":
			f.Type = ingsNumber
		case "BOOLEAN":
			f.Type = ingsSwitch
			if f.Default == "true" {
				f.Default = ingsOn
			} else {
				f.Default = ""
			}
		case "DURATION":
			f.Type = ingsDuration
		case "SECRET":
			f.Type, f.Optional, f.Plain = ingsSecret, false, true
		case "CHOICE":
			f.Type = ingsChoice
			for _, c := range s.Choices {
				f.Options = append(f.Options, kit.Option{Value: c, Label: c})
			}
		case "LIST":
			f.Type, f.Help = ingsList, strings.TrimSpace(f.Help+" One per line.")
		default:
			f.Type = ingsText
		}
		out = append(out, f)
	}
	return out
}

// ingsExtensionKind is the form's kind for one a runtime reports (or camel-route
// before any runtime does).
func ingsExtensionKind(rk ingestion.RuntimeKind) ingsKind {
	k := ingsKind{Name: rk.Kind, Label: rk.Kind, Description: rk.Description, SingleOwner: true, Perm: rk.RequiredPermission, Reported: true}
	k.Fields = ingsSettingFields(rk.Kind, rk.Settings)
	if len(k.Fields) == 0 {
		k.Fields = ingsUnknownSettings(rk.Kind)
	}
	ingsMarkControls(k.Fields)
	return k
}

// ingsUnknownSettings is the one field of a kind no schema describes: its
// settings as a JSON object (camel-route's is its route, in YAML).
func ingsUnknownSettings(kind string) []ingsField {
	if kind == "camel-route" {
		return []ingsField{{
			Name: kind + ".s.yaml", Path: "source.settings.yaml", Label: "Route (YAML)", Type: ingsCode, Lang: "yaml", Required: true,
			Help: "One from: with its uri and steps. The body the steps leave is split and dispatched. The runtime checks the route when it takes the ingestion.",
		}}
	}
	return []ingsField{{
		Name: kind + ".settings", Path: "source.settings", Label: "Settings", Type: ingsJSON, Lang: "json", Optional: true,
		Help: "A JSON object. The runtime that carries the kind says what it takes.",
	}}
}

// ── the catalogue ──────────────────────────────────────────────────────

// ingsCatalogue is the kinds the form offers: the platform's own, then those
// the live runtimes report (ingestion.Repository.LiveKinds, as GET
// /api/ingestion-source-kinds lists them).
type ingsCatalogue []ingsKind

var ingsBuiltins = ingsBuiltinKinds()

// ingsNewCatalogue builds the catalogue from the reported kinds. A runtime cannot
// redefine a platform kind (the API skips it too). current are the kinds of
// an ingestion being changed: offered even when no runtime reports them now.
func ingsNewCatalogue(reported []ingestion.RuntimeKind, current ...string) ingsCatalogue {
	cat := ingsCatalogue(slices.Clone(ingsBuiltins))
	for _, rk := range reported {
		if !ingestion.IsBuiltInKind(rk.Kind) && ingsKindName.MatchString(rk.Kind) {
			cat = append(cat, ingsExtensionKind(rk))
		}
	}
	for _, name := range current {
		if name == "" || cat.find(name) != nil || !ingsKindName.MatchString(name) {
			continue
		}
		k := ingsKind{Name: name, Label: name + " (no runtime reports it)", SingleOwner: true, Fields: ingsUnknownSettings(name)}
		if name == "camel-route" {
			k.Perm = auth.PermIngestionRoute
		}
		ingsMarkControls(k.Fields)
		cat = append(cat, k)
	}
	return cat
}

func (c ingsCatalogue) find(name string) *ingsKind {
	for i := range c {
		if c[i].Name == name {
			return &c[i]
		}
	}
	return nil
}

// ingsKindOf is the name the list and the form know a source by: its kind, or
// for a custom source its type.
func ingsKindOf(src ingestion.Source) string { return ingestion.RunKind(src) }

func ingsKindLabel(name string) string { return cmp.Or(ingsKindLabels[name], name) }

// ── values ↔ definition ────────────────────────────────────────────────

// active reports whether the field applies to values.
func (f ingsField) active(values map[string]string) bool {
	return f.When == nil || values[f.When.Field] == f.When.Is
}

// ingsDefaults are the values a new ingestion's form starts with.
func ingsDefaults(cat ingsCatalogue) map[string]string {
	values := map[string]string{"kind": ingestion.KindHTTP, "live": ingsOn}
	set := func(fields []ingsField) {
		for _, f := range fields {
			if f.Default != "" {
				values[f.Name] = f.Default
			}
		}
	}
	for _, k := range cat {
		set(k.Fields)
	}
	set(ingsSplitFields)
	set(ingsTargetFields)
	return values
}

// ingsValuesOf reads a stored definition into form values, over defaults for
// the fields the definition does not hold.
func ingsValuesOf(def ingestion.Definition, cat ingsCatalogue) (map[string]string, error) {
	values := ingsDefaults(cat)
	raw, err := json.Marshal(def)
	if err != nil {
		return nil, fmt.Errorf("marshal definition: %w", err)
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("unmarshal definition: %w", err)
	}
	values["name"] = ingsString(doc, "name")
	delete(values, "keepFailedPayloads")
	if b, _ := doc["keepFailedPayloads"].(bool); b {
		values["keepFailedPayloads"] = ingsOn
	}
	kind := ingsKindOf(def.Source)
	values["kind"] = kind
	fields := slices.Clone(ingsSplitFields)
	fields = append(fields, ingsTargetFields...)
	if k := cat.find(kind); k != nil {
		fields = append(fields, k.Fields...)
	}
	for _, f := range fields {
		v, found := ingsGetPath(doc, f.Path)
		if v, ok := ingsFormatValue(f, v, found); ok {
			values[f.Name] = v
		} else if f.Type == ingsSwitch {
			delete(values, f.Name)
		}
	}
	return values, nil
}

// ingsFormatValue is a stored value as the form shows it; ok=false when the
// definition holds none (the default stays).
func ingsFormatValue(f ingsField, v any, found bool) (string, bool) {
	if f.Type == ingsSwitch {
		if f.Presence {
			if found {
				return ingsOn, true
			}
			return "", false
		}
		if b, _ := v.(bool); b {
			return ingsOn, true
		}
		return "", false
	}
	if !found || v == nil {
		return "", false
	}
	switch f.Type {
	case ingsSecret:
		if f.Plain {
			s, ok := v.(string)
			return s, ok
		}
		m, _ := v.(map[string]any)
		return ingsString(m, "name"), true
	case ingsNumber:
		if n, ok := v.(float64); ok {
			return strconv.FormatInt(int64(n), 10), true
		}
	case ingsList:
		var lines []string
		for _, item := range ingsSlice(v) {
			lines = append(lines, fmt.Sprint(item))
		}
		return strings.Join(lines, "\n"), true
	case ingsPairs:
		return ingsFormatPairs(f, v), true
	case ingsJSON:
		b, err := json.MarshalIndent(v, "", "  ")
		return string(b), err == nil
	case ingsText, ingsDuration, ingsChoice, ingsArea, ingsCode:
		if s, ok := v.(string); ok {
			return s, true
		}
		if b, ok := v.(bool); ok {
			return strconv.FormatBool(b), true
		}
		if n, ok := v.(float64); ok {
			return strconv.FormatFloat(n, 'f', -1, 64), true
		}
	case ingsSwitch:
	}
	return "", false
}

func ingsSlice(v any) []any {
	s, _ := v.([]any)
	return s
}

func ingsFormatPairs(f ingsField, v any) string {
	var lines []string
	switch x := v.(type) {
	case []any:
		for _, item := range x {
			m, _ := item.(map[string]any)
			lines = append(lines, ingsString(m, "name")+f.Sep+" "+ingsString(m, "value"))
		}
	case map[string]any:
		for _, k := range slices.Sorted(maps.Keys(x)) {
			lines = append(lines, k+f.Sep+fmt.Sprint(x[k]))
		}
	}
	return strings.Join(lines, "\n")
}

func ingsString(m map[string]any, key string) string {
	s, _ := m[key].(string)
	return s
}

func ingsGetPath(doc map[string]any, path string) (any, bool) {
	var cur any = doc
	for part := range strings.SplitSeq(path, ".") {
		m, ok := cur.(map[string]any)
		if !ok {
			return nil, false
		}
		if cur, ok = m[part]; !ok {
			return nil, false
		}
	}
	return cur, true
}

func ingsSetPath(doc map[string]any, path string, v any) {
	parts := strings.Split(path, ".")
	cur := doc
	for _, part := range parts[:len(parts)-1] {
		next, ok := cur[part].(map[string]any)
		if !ok {
			next = map[string]any{}
			cur[part] = next
		}
		cur = next
	}
	cur[parts[len(parts)-1]] = v
}

// ingsBuilt is the definition JSON the posted values make, with the problems
// found while reading them (a number that is not a number, a duration that is
// not one), by control name.
type ingsBuilt struct {
	Doc    map[string]any
	Fields []ingsField
	Errs   map[string]string
}

// ingsBuildDefinition reads the values of the chosen kind into the definition
// JSON the API takes. Only the kind's active fields count: a value posted for
// another kind, or for a branch not chosen, is ignored.
func ingsBuildDefinition(values map[string]string, cat ingsCatalogue) ingsBuilt {
	b := ingsBuilt{Doc: map[string]any{}, Errs: map[string]string{}}
	kind := cat.find(values["kind"])
	if kind == nil {
		b.Errs["kind"] = "Choose where the data comes from."
		return b
	}
	b.Doc["name"] = strings.TrimSpace(values["name"])
	if values["keepFailedPayloads"] == ingsOn {
		b.Doc["keepFailedPayloads"] = true
	}
	if kind.BuiltIn {
		ingsSetPath(b.Doc, "source.kind", kind.Name)
	} else {
		ingsSetPath(b.Doc, "source.kind", ingestion.KindCustom)
		ingsSetPath(b.Doc, "source.type", kind.Name)
	}
	fields := slices.Clone(ingsSplitFields)
	if kind.Name == ingestion.KindDatabasePoll {
		// Each row is one record, so there is no split to choose.
		values = maps.Clone(values)
		values["split.kind"] = "none"
	}
	fields = append(fields, ingsTargetFields...)
	fields = append(fields, kind.Fields...)
	for _, f := range fields {
		if !f.active(values) {
			continue
		}
		b.Fields = append(b.Fields, f)
		b.read(f, values[f.Name])
	}
	return b
}

func (b *ingsBuilt) read(f ingsField, raw string) {
	if f.Presence {
		return
	}
	switch f.Type { //nolint:exhaustive // text keeps its line breaks, a switch its value, everything else is trimmed
	case ingsArea, ingsCode, ingsJSON:
		// Text is kept as written (a browser posts line breaks as CRLF).
		raw = strings.ReplaceAll(raw, "\r\n", "\n")
		if strings.TrimSpace(raw) == "" {
			raw = ""
		}
	case ingsSwitch:
	default:
		if !f.NoTrim {
			raw = strings.TrimSpace(raw)
		}
	}
	if raw == "" && f.Type != ingsSwitch {
		// The platform checks an extension's settings no further than their
		// shape, so a required one is checked here.
		if f.Required && strings.HasPrefix(f.Path, "source.settings.") {
			b.Errs[f.Name] = "This is required."
			return
		}
		if f.Optional || f.Type == ingsNumber || f.Type == ingsList || f.Type == ingsPairs {
			return
		}
	}
	switch f.Type {
	case ingsSwitch:
		ingsSetPath(b.Doc, f.Path, raw == ingsOn)
	case ingsNumber:
		n, err := strconv.Atoi(raw)
		if err != nil {
			b.Errs[f.Name] = notWholeNumber
			return
		}
		ingsSetPath(b.Doc, f.Path, n)
	case ingsDuration:
		d, err := ingsParseDuration(raw)
		if err != nil {
			b.Errs[f.Name] = "Enter a duration such as 30s, 5m, 2h, 1d or PT30S."
			return
		}
		ingsSetPath(b.Doc, f.Path, d)
	case ingsSecret:
		if f.Plain {
			ingsSetPath(b.Doc, f.Path, raw)
			return
		}
		ingsSetPath(b.Doc, f.Path, map[string]any{"name": raw})
	case ingsList:
		var items []string
		for line := range strings.SplitSeq(raw, "\n") {
			if line = strings.TrimSpace(line); line != "" {
				items = append(items, line)
			}
		}
		ingsSetPath(b.Doc, f.Path, items)
	case ingsPairs:
		v, problem := ingsParsePairs(f, raw)
		if problem != "" {
			b.Errs[f.Name] = problem
			return
		}
		ingsSetPath(b.Doc, f.Path, v)
	case ingsJSON:
		var obj map[string]any
		if err := json.Unmarshal([]byte(raw), &obj); err != nil || obj == nil {
			b.Errs[f.Name] = "Enter a JSON object."
			return
		}
		ingsSetPath(b.Doc, f.Path, obj)
	case ingsText, ingsChoice, ingsArea, ingsCode:
		ingsSetPath(b.Doc, f.Path, raw)
	}
}

var ingsShortDuration = regexp.MustCompile(`^(\d+)\s*([smhd])$`)

// ingsParseDuration reads 30s, 5m, 2h, 1d or an ISO 8601 duration (PT30S)
// and returns the ISO text the API takes.
func ingsParseDuration(s string) (string, error) {
	if m := ingsShortDuration.FindStringSubmatch(strings.ToLower(s)); m != nil {
		n, err := strconv.Atoi(m[1])
		if err != nil {
			return "", err
		}
		unit := map[string]time.Duration{"s": time.Second, "m": time.Minute, "h": time.Hour, "d": 24 * time.Hour}[m[2]]
		if n > int(time.Duration(1<<62)/unit) {
			return "", fmt.Errorf("%q is too long", s)
		}
		return ingestion.Duration(time.Duration(n) * unit).String(), nil
	}
	d, err := ingestion.ParseDuration(strings.ToUpper(s))
	if err != nil {
		return "", err
	}
	return ingestion.Duration(d).String(), nil
}

// ingsParsePairs reads one "name<sep> value" a line.
func ingsParsePairs(f ingsField, raw string) (v any, problem string) {
	var list []map[string]string
	entries := map[string]string{}
	for line := range strings.SplitSeq(raw, "\n") {
		if line = strings.TrimSpace(line); line == "" {
			continue
		}
		name, value, ok := strings.Cut(line, f.Sep)
		name = strings.TrimSpace(name)
		if !ok || name == "" {
			return nil, "Write each line as name" + f.Sep + " value."
		}
		list = append(list, map[string]string{"name": name, "value": strings.TrimSpace(value)})
		entries[name] = strings.TrimSpace(value)
	}
	if f.Map {
		return entries, ""
	}
	return list, ""
}

// fieldFor finds the control a validation error at path is about among the
// fields that were read.
func (b ingsBuilt) fieldFor(path string) (ingsField, bool) {
	for _, f := range b.Fields {
		switch {
		case f.Path == path:
			return f, true
		case f.Type == ingsSecret && path == f.Path+".name":
			return f, true
		case f.Type == ingsPairs && strings.HasPrefix(path, f.Path+"."):
			return f, true
		}
	}
	return ingsField{}, false
}
