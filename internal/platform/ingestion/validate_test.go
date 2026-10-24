package ingestion_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/ingestion"
)

const goodTarget = `{"code":"acme:orders:order:received","targetUrl":"https://hooks.example.com/orders","pool":"orders","mode":"IMMEDIATE","batchSize":100}`

// definition builds the JSON of a definition around a source and a split.
func definition(source, split string) string {
	return `{"name":"Orders","source":` + source + `,"split":` + split + `,"target":` + goodTarget + `}`
}

func parse(t *testing.T, doc string) ingestion.Definition {
	t.Helper()
	d, err := ingestion.ParseDefinition([]byte(doc))
	require.NoError(t, err, doc)
	return d
}

// fields is the set of field paths with a problem.
func fields(p ingestion.Problems) []string {
	out := make([]string, len(p))
	for i, e := range p {
		out[i] = e.Field
	}
	return out
}

// sourceCase is a source that is valid, and the edits that each break it at
// one field.
type sourceCase struct {
	name   string
	source string
	split  string
	// bad maps an edit (old -> new replacement of the source JSON) to the
	// field it must be reported at.
	bad []struct{ from, to, field string }
}

var sourceCases = []sourceCase{
	{
		name:   "http bearer",
		source: `{"kind":"http","auth":{"kind":"bearer","token":{"name":"orders-token"}},"rateLimit":{"requests":100,"per":"PT1S"}}`,
		split:  `{"kind":"json","arrayPath":"$"}`,
		bad: []struct{ from, to, field string }{
			{`"orders-token"`, `"Orders Token"`, "source.auth.token.name"},
			{`"requests":100`, `"requests":0`, "source.rateLimit.requests"},
			{`"PT1S"`, `"PT0.0001S"`, "source.rateLimit.per"},
			{`"requests":100,"per":"PT1S"`, `"requests":1000000,"per":"PT0.1S"`, "source.rateLimit"},
		},
	},
	{
		name:   "http basic",
		source: `{"kind":"http","auth":{"kind":"basic","username":"feed","password":{"name":"feed-password"}}}`,
		split:  `{"kind":"none"}`,
		bad: []struct{ from, to, field string }{
			{`"feed"`, `"fe:ed"`, "source.auth.username"},
			{`"name":"feed-password"`, `"name":""`, "source.auth.password"},
		},
	},
	{
		name:   "http flowcatalyst token",
		source: `{"kind":"http","auth":{"kind":"flowcatalyst-token","application":"orders","principalId":"prn_0ABC123"}}`,
		split:  `{"kind":"none"}`,
		bad: []struct{ from, to, field string }{
			{`"orders"`, `"or:ders"`, "source.auth.application"},
			{`prn_0ABC123`, `sac_0ABC123`, "source.auth.principalId"},
		},
	},
	{
		name:   "sftp",
		source: `{"kind":"sftp","host":"sftp.example.com","port":22,"directory":"/in/orders","username":"feed","credential":{"name":"sftp-password"},"include":"*.csv","every":"PT5M"}`,
		split:  `{"kind":"csv","header":true}`,
		bad: []struct{ from, to, field string }{
			{`sftp.example.com`, `sftp example`, "source.host"},
			{`"port":22`, `"port":70000`, "source.port"},
			{`/in/orders`, `/in/../orders?`, "source.directory"},
			{`"username":"feed"`, `"username":""`, "source.username"},
			{`sftp-password`, `SFTP_PASSWORD`, "source.credential.name"},
			{`"PT5M"`, `"PT0S"`, "source.every"},
			{`"PT5M"`, `"P2D"`, "source.every"},
		},
	},
	{
		name:   "s3",
		source: `{"kind":"s3","region":"eu-west-1","bucket":"acme-orders","prefix":"incoming/orders/","every":"PT1M","credential":{"accessKeyId":"AKIA1234","secretAccessKey":{"name":"aws-secret"}}}`,
		split:  `{"kind":"xml","element":"order"}`,
		bad: []struct{ from, to, field string }{
			{`eu-west-1`, `europe`, "source.region"},
			{`acme-orders`, `Acme_Orders`, "source.bucket"},
			{`incoming/orders/`, `incoming/orders`, "source.prefix"},
			{`AKIA1234`, `no`, "source.credential.accessKeyId"},
			{`"name":"aws-secret"`, `"name":""`, "source.credential.secretAccessKey"},
		},
	},
	{
		name:   "sqs by runtime identity",
		source: `{"kind":"sqs","region":"eu-west-1","queue":"orders.fifo"}`,
		split:  `{"kind":"none"}`,
		bad: []struct{ from, to, field string }{
			{`orders.fifo`, `https://sqs.eu-west-1.amazonaws.com/1/orders`, "source.queue"},
			{`eu-west-1`, `eu`, "source.region"},
		},
	},
	{
		name:   "rabbitmq",
		source: `{"kind":"rabbitmq","uri":"amqps://mq.example.com","queue":"orders","username":"feed","password":{"name":"mq-password"}}`,
		split:  `{"kind":"none"}`,
		bad: []struct{ from, to, field string }{
			{`amqps://mq.example.com`, `amqps://feed:pw@mq.example.com`, "source.uri"},
			{`amqps://mq.example.com`, `http://mq.example.com`, "source.uri"},
			{`"queue":"orders"`, `"queue":""`, "source.queue"},
			{`mq-password`, `files:`, "source.password.name"},
		},
	},
	{
		name:   "database poll watermark",
		source: `{"kind":"database-poll","jdbcUrl":"jdbc:postgresql://db/orders","username":"reader","password":{"name":"db-password"},"table":"public.orders","track":{"kind":"watermark","column":"updated_at","type":"TIMESTAMP"},"maxRows":500,"every":"PT30S"}`,
		split:  `{"kind":"none"}`,
		bad: []struct{ from, to, field string }{
			{`jdbc:postgresql`, `postgresql`, "source.jdbcUrl"},
			{`public.orders`, `public.orders; drop`, "source.table"},
			{`updated_at`, `updated at`, "source.track.column"},
			{`TIMESTAMP`, `DATE`, "source.track.type"},
			{`"maxRows":500`, `"maxRows":10001`, "source.maxRows"},
		},
	},
	{
		name:   "database poll status",
		source: `{"kind":"database-poll","jdbcUrl":"jdbc:postgresql://db/orders","username":"reader","password":{"name":"db-password"},"table":"orders","track":{"kind":"status","column":"sent","keyColumn":"id","pending":"0","processed":"1"},"maxRows":500,"every":"PT30S"}`,
		split:  `{"kind":"none"}`,
		bad: []struct{ from, to, field string }{
			{`"processed":"1"`, `"processed":"0"`, "source.track.processed"},
			{`"keyColumn":"id"`, `"keyColumn":"i d"`, "source.track.keyColumn"},
			{`"pending":"0"`, `"pending":""`, "source.track.pending"},
		},
	},
	{
		name: "api poll with login and cursor",
		source: `{"kind":"api-poll","url":"https://api.example.com/v1/orders?limit=100","method":"GET","headers":[{"name":"X-Tenant","value":"acme"}],` +
			`"auth":{"kind":"login","url":"https://api.example.com/login","format":"JSON","usernameField":"user","passwordField":"pass","username":"feed","password":{"name":"api-password"},"tokenPath":"$.token","lifetime":"PT1H","header":"Authorization","prefix":"Bearer "},` +
			`"results":{"kind":"json-path","path":"$.items"},"cursor":{"from":"$.items[-1].updatedAt","param":"since"},"every":"PT10S","idleDelay":"PT1M"}`,
		split: `{"kind":"json","arrayPath":"$.items"}`,
		bad: []struct{ from, to, field string }{
			{`https://api.example.com/v1/orders?limit=100`, `ftp://api.example.com/v1`, "source.url"},
			{`https://api.example.com/v1/orders?limit=100`, `https://u:p@api.example.com/v1`, "source.url"},
			{`https://api.example.com/v1/orders?limit=100`, `https://api.example.com/a/../b`, "source.url"},
			{`"method":"GET"`, `"method":"PUT"`, "source.method"},
			{`"name":"X-Tenant"`, `"name":"Authorization"`, "source.headers.0.name"},
			{`"value":"acme"`, `"value":"a\nb"`, "source.headers.0.value"},
			{`"format":"JSON"`, `"format":"XML"`, "source.auth.format"},
			{`"tokenPath":"$.token"`, `"tokenPath":"token"`, "source.auth.tokenPath"},
			{`"lifetime":"PT1H"`, `"lifetime":"PT1S"`, "source.auth.lifetime"},
			{`"path":"$.items"`, `"path":"items"`, "source.results.path"},
			{`"param":"since"`, `"param":"a b"`, "source.cursor.param"},
			{`"idleDelay":"PT1M"`, `"idleDelay":"P3D"`, "source.idleDelay"},
		},
	},
	{
		name: "api poll with oauth and header results",
		source: `{"kind":"api-poll","url":"https://api.example.com/orders","method":"POST","body":"{\"all\":true}",` +
			`"auth":{"kind":"oauth-client-credentials","tokenUrl":"https://auth.example.com/token","clientId":"fc","clientSecret":{"name":"oauth-secret"},"credentialsIn":"BASIC_HEADER"},` +
			`"results":{"kind":"header","name":"X-More","equals":"yes"},"every":"PT10S","idleDelay":"PT1M"}`,
		split: `{"kind":"none"}`,
		bad: []struct{ from, to, field string }{
			{`"method":"POST"`, `"method":"GET"`, "source.body"},
			{`"credentialsIn":"BASIC_HEADER"`, `"credentialsIn":"QUERY"`, "source.auth.credentialsIn"},
			{`"tokenUrl":"https://auth.example.com/token"`, `"tokenUrl":"auth"`, "source.auth.tokenUrl"},
			{`"name":"X-More"`, `"name":"X More"`, "source.results.name"},
		},
	},
	{
		name:   "api poll with api key and records",
		source: `{"kind":"api-poll","url":"https://api.example.com/orders","method":"GET","auth":{"kind":"api-key","header":"X-Api-Key","value":{"name":"api-key"}},"results":{"kind":"records"},"every":"PT10S","idleDelay":"PT1M"}`,
		split:  `{"kind":"json","arrayPath":"$"}`,
		bad: []struct{ from, to, field string }{
			{`"header":"X-Api-Key"`, `"header":"X Api Key"`, "source.auth.header"},
			{`"name":"api-key"`, `"name":"Api Key"`, "source.auth.value.name"},
		},
	},
	{
		name:   "custom kind",
		source: `{"kind":"custom","type":"mix","settings":{"apiUrl":"https://api.mix.example","every":"PT1M","registrations":["ab 12","cd-34"],"enabled":true,"password":"mix-password"}}`,
		split:  `{"kind":"json","arrayPath":"$"}`,
		bad: []struct{ from, to, field string }{
			{`"type":"mix"`, `"type":"Mix"`, "source.type"},
			{`"type":"mix"`, `"type":"http"`, "source.type"},
			{`"enabled":true`, `"enabled":{"a":1}`, "source.settings.enabled"},
			{`"enabled":true`, `"kind":"x"`, "source.settings.kind"},
			{`"registrations":["ab 12","cd-34"]`, `"registrations":[1]`, "source.settings.registrations"},
		},
	},
	{
		name:   "custom camel-route",
		source: `{"kind":"custom","type":"camel-route","settings":{"yaml":"from:\n  uri: timer:tick\n  steps:\n    - log: hi\n"}}`,
		split:  `{"kind":"none"}`,
	},
}

func TestSourceKindsAreValid(t *testing.T) {
	for _, c := range sourceCases {
		t.Run(c.name, func(t *testing.T) {
			d := parse(t, definition(c.source, c.split))
			assert.Empty(t, d.Validate(), "a good %s source has no problems", c.name)
		})
	}
}

func TestSourceKindsReportEachBadField(t *testing.T) {
	for _, c := range sourceCases {
		for _, b := range c.bad {
			t.Run(c.name+" "+b.field, func(t *testing.T) {
				require.Contains(t, c.source, b.from, "the edit must apply to the source")
				broken := strings.Replace(c.source, b.from, b.to, 1)
				d := parse(t, definition(broken, c.split))
				assert.Contains(t, fields(d.Validate()), b.field)
			})
		}
	}
}

func TestEverySourceKindHasACase(t *testing.T) {
	seen := map[string]bool{}
	for _, c := range sourceCases {
		d := parse(t, definition(c.source, c.split))
		seen[d.Source.Kind()] = true
	}
	for _, kind := range ingestion.BuiltInKinds() {
		assert.True(t, seen[kind], "no test case for source kind %s", kind)
	}
}

func TestSplitRules(t *testing.T) {
	const source = `{"kind":"sqs","region":"eu-west-1","queue":"orders"}`
	cases := []struct {
		name, split, field string
	}{
		{"none", `{"kind":"none"}`, ""},
		{"csv with header", `{"kind":"csv","header":true}`, ""},
		{"csv without header", `{"kind":"csv","header":false}`, ""},
		{"json top level", `{"kind":"json","arrayPath":"$"}`, ""},
		{"json nested", `{"kind":"json","arrayPath":"$.data.items"}`, ""},
		{"json bad path", `{"kind":"json","arrayPath":"data.items"}`, "split.arrayPath"},
		{"json index is not a field", `{"kind":"json","arrayPath":"$.items[0]"}`, "split.arrayPath"},
		{"json empty path", `{"kind":"json","arrayPath":""}`, "split.arrayPath"},
		{"xml element", `{"kind":"xml","element":"order"}`, ""},
		{"xml namespaced element", `{"kind":"xml","element":"ns:order"}`, ""},
		{"xml bad element", `{"kind":"xml","element":"1order"}`, "split.element"},
		{"xml empty element", `{"kind":"xml","element":""}`, "split.element"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := parse(t, definition(source, c.split)).Validate()
			if c.field == "" {
				assert.Empty(t, p)
			} else {
				assert.Equal(t, []string{c.field}, fields(p))
			}
		})
	}
}

func TestDatabasePollRequiresSplitNone(t *testing.T) {
	var poll sourceCase
	for _, c := range sourceCases {
		if c.name == "database poll watermark" {
			poll = c
		}
	}
	p := parse(t, definition(poll.source, `{"kind":"csv","header":false}`)).Validate()
	assert.Equal(t, []string{"split"}, fields(p))
}

func TestSplitKindsDecodeToTheirTypes(t *testing.T) {
	s, err := ingestion.UnmarshalSplit([]byte(`{"kind":"json","arrayPath":"$.a"}`))
	require.NoError(t, err)
	assert.Equal(t, ingestion.JSON{ArrayPath: "$.a"}, s)
	_, err = ingestion.UnmarshalSplit([]byte(`{"kind":"yaml"}`))
	assert.ErrorContains(t, err, `kind "yaml" is not known`)
}

func TestTargetRules(t *testing.T) {
	const source = `{"kind":"sqs","region":"eu-west-1","queue":"orders"}`
	cases := []struct {
		name, from, to, field string
	}{
		{"job code needs four parts", `acme:orders:order:received`, `acme:orders:received`, "target.code"},
		{"job code is lowercase", `acme:orders:order:received`, `Acme:orders:order:received`, "target.code"},
		{"url needs a scheme", `https://hooks.example.com/orders`, `hooks.example.com/orders`, "target.targetUrl"},
		{"url must be http", `https://hooks.example.com/orders`, `ftp://hooks.example.com/orders`, "target.targetUrl"},
		{"pool is required", `"pool":"orders"`, `"pool":""`, "target.pool"},
		{"mode is one of three", `IMMEDIATE`, `LATER`, "target.mode"},
		{"batch size at least 1", `"batchSize":100`, `"batchSize":0`, "target.batchSize"},
		{"batch size at most 1000", `"batchSize":100`, `"batchSize":1001`, "target.batchSize"},
		{"connection and service account are exclusive", `"mode"`, `"connection":"c","serviceAccount":"s","mode"`, "target.serviceAccount"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			doc := strings.Replace(definition(source, `{"kind":"none"}`), c.from, c.to, 1)
			require.NotEqual(t, definition(source, `{"kind":"none"}`), doc)
			assert.Equal(t, []string{c.field}, fields(parse(t, doc).Validate()))
		})
	}

	t.Run("each of connection and service account alone is fine", func(t *testing.T) {
		for _, extra := range []string{`"connection":"orders-conn",`, `"serviceAccount":"orders-signer",`} {
			doc := strings.Replace(definition(source, `{"kind":"none"}`), `"mode"`, extra+`"mode"`, 1)
			assert.Empty(t, parse(t, doc).Validate())
		}
	})
}

func TestNameRules(t *testing.T) {
	const source = `{"kind":"sqs","region":"eu-west-1","queue":"orders"}`
	doc := definition(source, `{"kind":"none"}`)
	assert.Equal(t, []string{"name"}, fields(parse(t, strings.Replace(doc, `"Orders"`, `"  "`, 1)).Validate()))
	assert.Equal(t, []string{"name"}, fields(parse(t, strings.Replace(doc, `"Orders"`, `"`+strings.Repeat("x", 201)+`"`, 1)).Validate()))
}

func TestCodeRules(t *testing.T) {
	for _, ok := range []string{"orders", "a", "orders-2", "a1", strings.Repeat("a", 64)} {
		assert.Empty(t, ingestion.ValidateCode(ok), ok)
	}
	for _, bad := range []string{"", "-orders", "orders-", "Orders", "or_ders", "or ders", strings.Repeat("a", 65)} {
		assert.NotEmpty(t, ingestion.ValidateCode(bad), bad)
	}
}

func TestMissingPartsAreReportedNotPanicked(t *testing.T) {
	d, err := ingestion.ParseDefinition([]byte(`{"name":"x","target":` + goodTarget + `}`))
	require.NoError(t, err)
	assert.Equal(t, []string{"split", "source"}, fields(d.Validate()))

	d = parse(t, `{"name":"x","source":{"kind":"http"},"split":{"kind":"none"},"target":`+goodTarget+`}`)
	assert.Equal(t, []string{"source.auth"}, fields(d.Validate()))
}

func TestProblemsErrIsAValidationError(t *testing.T) {
	assert.NoError(t, ingestion.Problems(nil).Err())
	err := ingestion.ValidateCode("Bad").Err()
	require.Error(t, err)
	assert.ErrorContains(t, err, "INGESTION_INVALID")
}

func TestSingleOwnerSources(t *testing.T) {
	want := map[string]bool{
		"http": false, "sqs": false, "rabbitmq": false,
		"sftp": true, "s3": true, "database-poll": true, "api-poll": true, "custom": true,
	}
	for _, c := range sourceCases {
		d := parse(t, definition(c.source, c.split))
		assert.Equal(t, want[d.Source.Kind()], d.Source.SingleOwner(), d.Source.Kind())
	}
}
