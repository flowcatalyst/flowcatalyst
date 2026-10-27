package ingestion_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/ingestion"
)

// withoutURL is a definition around source whose target gives no delivery URL
// (the entry's value does).
func withoutURL(source string) string {
	return `{"name":"Orders","source":` + source + `,"split":{"kind":"none"},"target":{"code":"acme:orders:order:received","pool":"orders","mode":"IMMEDIATE","batchSize":100}}`
}

func entry(value string) *ingestion.ConfigEntry {
	return &ingestion.ConfigEntry{Value: value, UpdatedAt: time.Date(2026, 10, 26, 9, 0, 0, 0, time.UTC)}
}

func TestReferenceParsesInJSONForBuiltInAndExtensionKinds(t *testing.T) {
	for _, c := range []struct {
		name, source, kind, runKind, key string
		own                              bool
	}{
		{"built-in with a key", `{"kind":"sftp","config":"mix-positions"}`, "sftp", "sftp", "mix-positions", false},
		{"built-in, own code", `{"kind":"api-poll","config":true}`, "api-poll", "api-poll", "orders", true},
		{"extension with a key", `{"kind":"custom","type":"acme-orders","config":"shared"}`, "custom", "acme-orders", "shared", false},
		{"extension, own code", `{"kind":"custom","type":"acme-orders","config":true}`, "custom", "acme-orders", "orders", true},
	} {
		t.Run(c.name, func(t *testing.T) {
			d := parse(t, withoutURL(c.source))
			ref, ok := ingestion.ConfigOf(d)
			require.True(t, ok)
			assert.Equal(t, c.own, ref.Config.Own)
			assert.Equal(t, c.key, ref.Config.KeyFor("orders"))
			assert.Equal(t, c.kind, d.Source.Kind(), "it reports the kind it stands for")
			assert.Equal(t, c.runKind, ingestion.RunKind(d.Source), "a runtime must carry the underlying kind")
			assert.Equal(t, c.key, ingestion.ConfigKeyOf(d, "orders"))
			assert.Empty(t, d.Validate(), "a reference with no delivery URL is valid until the entry gives it")

			out, err := json.Marshal(d.Source)
			require.NoError(t, err)
			assert.JSONEq(t, c.source, string(out), "it is written back as it was read")
		})
	}
}

func TestReferenceSingleOwnerFollowsTheUnderlyingKind(t *testing.T) {
	for kind, single := range map[string]bool{"http": false, "sqs": false, "rabbitmq": false, "sftp": true, "s3": true, "database-poll": true, "api-poll": true} {
		d := parse(t, withoutURL(`{"kind":"`+kind+`","config":"k"}`))
		assert.Equal(t, single, d.Source.SingleOwner(), kind)
	}
	assert.True(t, parse(t, withoutURL(`{"kind":"custom","type":"acme","config":"k"}`)).Source.SingleOwner())
}

func TestReferenceNeverHoldsSettingsBeside(t *testing.T) {
	d := parse(t, withoutURL(`{"kind":"sftp","config":"k","host":"sftp.example.com","port":22}`))
	p := d.Validate()
	require.NotEmpty(t, p)
	assert.Contains(t, fields(p), "source.config")
	assert.Contains(t, p.String(), "host, port")

	custom := parse(t, withoutURL(`{"kind":"custom","type":"acme","config":"k","settings":{"a":"b"}}`))
	assert.Contains(t, fields(custom.Validate()), "source.config")

	// Written back, the extra settings are not kept: the definition is refused first.
	out, err := json.Marshal(d.Source)
	require.NoError(t, err)
	assert.JSONEq(t, `{"kind":"sftp","config":"k"}`, string(out))
}

func TestReferenceKeyFollowsTheRulesOfACode(t *testing.T) {
	for key, valid := range map[string]bool{"mix-positions": true, "a": true, "9lives": true, "Mix": false, "-x": false, "x-": false, "a_b": false, "": false, strings.Repeat("a", 65): false} {
		d := parse(t, withoutURL(`{"kind":"sqs","config":`+quote(key)+`}`))
		assert.Equal(t, valid, len(d.Validate()) == 0, "key %q: %v", key, d.Validate())
	}
}

func quote(s string) string { b, _ := json.Marshal(s); return string(b) }

func TestReferenceRefusesWhatIsNotAKeyOrTrue(t *testing.T) {
	for _, bad := range []string{`false`, `null`, `5`, `["k"]`, `{"a":1}`} {
		_, err := ingestion.ParseDefinition([]byte(withoutURL(`{"kind":"sqs","config":` + bad + `}`)))
		assert.Error(t, err, bad)
	}
	_, err := ingestion.ParseDefinition([]byte(withoutURL(`{"kind":"mix","config":"k"}`)))
	assert.Error(t, err, "in JSON an extension kind is custom with a type")
}

func TestReferenceExtensionTypeIsChecked(t *testing.T) {
	assert.Contains(t, fields(parse(t, withoutURL(`{"kind":"custom","config":"k"}`)).Validate()), "source.type")
	assert.Contains(t, fields(parse(t, withoutURL(`{"kind":"custom","type":"Bad Type","config":"k"}`)).Validate()), "source.type")
	assert.Contains(t, fields(parse(t, withoutURL(`{"kind":"custom","type":"sftp","config":"k"}`)).Validate()), "source.type")
}

func TestReferenceInTheFlatFileForm(t *testing.T) {
	const file = `application: ingest
client: acme
code: mix-feed
name: Mix feed
active: true
source:
  kind: mix
  config: mix-positions
split:
  kind: none
target:
  code: acme:orders:order:received
  pool: orders
  mode: IMMEDIATE
  batchSize: 100
`
	files, err := ingestion.ParseFiles([]byte(file))
	require.NoError(t, err)
	require.Len(t, files, 1)
	ref, ok := ingestion.ConfigOf(files[0].Definition)
	require.True(t, ok)
	assert.Equal(t, ingestion.Configured{Base: "custom", Type: "mix", Config: ingestion.ConfigRef{Key: "mix-positions"}}, ref)
	assert.Equal(t, "mix", ingestion.RunKind(files[0].Source))
	assert.Empty(t, files[0].Validate())

	out, err := files[0].YAML()
	require.NoError(t, err)
	assert.Equal(t, file, string(out), "the reference is written back flat")
	again, err := ingestion.ParseFiles(out)
	require.NoError(t, err)
	assert.Equal(t, files, again)
}

func TestReferenceInTheFileFormForBuiltInKindsAndOwnCode(t *testing.T) {
	const file = `application: ingest
client: acme
code: orders
name: Orders
active: true
source:
  kind: api-poll
  config: true
split:
  kind: json
  arrayPath: $.items
target:
  code: acme:orders:order:received
  targetUrl: https://hooks.example.com/orders
  pool: orders
  mode: IMMEDIATE
  batchSize: 100
`
	files, err := ingestion.ParseFiles([]byte(file))
	require.NoError(t, err)
	ref, ok := ingestion.ConfigOf(files[0].Definition)
	require.True(t, ok)
	assert.True(t, ref.Config.Own)
	out, err := files[0].YAML()
	require.NoError(t, err)
	assert.Equal(t, file, string(out))
}

func TestFlatFileRefusesSettingsBesideConfig(t *testing.T) {
	files, err := ingestion.ParseFiles([]byte(`application: ingest
client: acme
code: x
name: X
source:
  kind: mix
  config: k
  interval: PT5M
split: {kind: none}
target: {code: "acme:a:b:c", pool: p, mode: IMMEDIATE, batchSize: 1}
`))
	require.NoError(t, err)
	p := files[0].Validate()
	assert.Contains(t, fields(p), "source.config")
	assert.Contains(t, p.String(), "interval")
}

func TestDeliveryUrlMayBeLeftOutOnlyByAReference(t *testing.T) {
	inline := parse(t, strings.Replace(withoutURL(`{"kind":"sqs","region":"eu-west-1","queue":"q"}`), `"pool"`, `"targetUrl":"","pool"`, 1))
	assert.Contains(t, fields(inline.Validate()), "target.targetUrl")

	ref := parse(t, withoutURL(`{"kind":"sqs","config":"k"}`))
	assert.Empty(t, ref.Validate())

	bad := parse(t, strings.Replace(withoutURL(`{"kind":"sqs","config":"k"}`), `"pool"`, `"targetUrl":"ftp://x","pool"`, 1))
	assert.Contains(t, fields(bad.Validate()), "target.targetUrl", "a URL that is given is checked")
}

func TestInlineDefinitionIsItsOwnEffectiveDefinition(t *testing.T) {
	d := parse(t, definition(`{"kind":"sqs","region":"eu-west-1","queue":"q"}`, `{"kind":"none"}`))
	r := ingestion.ApplyConfig(d, "orders", nil)
	assert.Equal(t, ingestion.ConfigInline, r.Status)
	assert.True(t, r.Runnable())
	assert.True(t, r.Definition.Equal(d))
	assert.Empty(t, r.Key)
}

func TestMissingEntry(t *testing.T) {
	d := parse(t, withoutURL(`{"kind":"sftp","config":true}`))
	r := ingestion.ApplyConfig(d, "orders", nil)
	assert.Equal(t, ingestion.ConfigMissing, r.Status)
	assert.Equal(t, "orders", r.Key, "config: true is the ingestion's code")
	assert.False(t, r.Runnable())
	assert.Nil(t, r.UpdatedAt)
	require.Len(t, r.Warnings(), 1)
	assert.True(t, strings.HasPrefix(r.Warnings()[0], "CONFIG_MISSING: "), r.Warnings()[0])
	_, still := r.Definition.Source.(ingestion.Configured)
	assert.True(t, still, "the definition stays the reference")
}

func TestEffectiveSftp(t *testing.T) {
	d := parse(t, withoutURL(`{"kind":"sftp","config":"vendor"}`))
	r := ingestion.ApplyConfig(d, "orders", entry(`{"host":"sftp.example.com","port":22,"directory":"/in","username":"feed","credential":{"name":"sftp-password"},"every":"PT5M","knownHosts":"sftp.example.com ssh-ed25519 AAAA","targetUrl":"https://hooks.example.com/orders"}`))
	require.Equal(t, ingestion.ConfigSet, r.Status, r.Problems.String())
	require.NotNil(t, r.UpdatedAt)
	sftp, ok := r.Definition.Source.(ingestion.Sftp)
	require.True(t, ok)
	assert.Equal(t, "sftp.example.com", sftp.Host)
	assert.Equal(t, []string{"sftp-password"}, ingestion.SecretNames(r.Definition.Source))
	assert.Equal(t, "https://hooks.example.com/orders", r.Definition.Target.TargetURL)
	assert.Equal(t, "acme:orders:order:received", r.Definition.Target.Code)
	assert.Empty(t, r.Warnings())

	raw, err := json.Marshal(r.Definition.Source)
	require.NoError(t, err)
	assert.NotContains(t, string(raw), "config", "a runtime never sees a config key")
}

func TestEffectiveApiPollAndSqs(t *testing.T) {
	poll := parse(t, `{"name":"P","source":{"kind":"api-poll","config":"k"},"split":{"kind":"json","arrayPath":"$.items"},"target":{"code":"acme:orders:order:received","targetUrl":"https://hooks.example.com/o","pool":"orders","mode":"IMMEDIATE","batchSize":10}}`)
	r := ingestion.ApplyConfig(poll, "orders", entry(`{"url":"https://api.example.com/orders","method":"GET","auth":{"kind":"login","url":"https://api.example.com/login","format":"JSON","usernameField":"u","passwordField":"p","username":"bot","password":{"name":"api-pass"},"tokenPath":"$.token","lifetime":"PT1H","header":"Authorization","prefix":"Bearer "},"results":{"kind":"records"},"every":"PT1M","idleDelay":"PT5M"}`))
	require.Equal(t, ingestion.ConfigSet, r.Status, r.Problems.String())
	assert.Equal(t, []string{"api-pass"}, ingestion.SecretNames(r.Definition.Source))
	assert.Equal(t, "https://hooks.example.com/o", r.Definition.Target.TargetURL, "the definition's own URL is kept")

	sqs := parse(t, withoutURL(`{"kind":"sqs","config":"k"}`))
	r = ingestion.ApplyConfig(sqs, "orders", entry(`{"region":"eu-west-1","queue":"orders-q","credential":{"accessKeyId":"AKIAEXAMPLE","secretAccessKey":{"name":"aws-secret"}},"targetUrl":"https://hooks.example.com/orders"}`))
	require.Equal(t, ingestion.ConfigSet, r.Status, r.Problems.String())
	assert.False(t, ingestion.UsesRuntimeIdentity(r.Definition.Source))
	assert.Equal(t, []string{"aws-secret"}, ingestion.SecretNames(r.Definition.Source))

	// Without a credential the runtime fleet's own identity reads the queue: the
	// effective source says so, so CheckAuthor can ask for runtime-identity.
	r = ingestion.ApplyConfig(sqs, "orders", entry(`{"region":"eu-west-1","queue":"orders-q","targetUrl":"https://hooks.example.com/orders"}`))
	require.Equal(t, ingestion.ConfigSet, r.Status)
	assert.True(t, ingestion.UsesRuntimeIdentity(r.Definition.Source))
}

func TestEffectiveExtensionKind(t *testing.T) {
	d := parse(t, withoutURL(`{"kind":"custom","type":"acme-orders","config":"k"}`))
	r := ingestion.ApplyConfig(d, "orders", entry(`{"baseUrl":"https://erp.example.com","org":"42","apiKey":"erp-key","registrations":["a","b"],"targetUrl":"https://hooks.example.com/orders"}`))
	require.Equal(t, ingestion.ConfigSet, r.Status, r.Problems.String())
	c, ok := r.Definition.Source.(ingestion.Custom)
	require.True(t, ok)
	assert.Equal(t, "acme-orders", c.Type)
	assert.Equal(t, "https://erp.example.com", c.Settings["baseUrl"])
	assert.NotContains(t, c.Settings, "targetUrl", "the delivery URL is the target's, not a setting")
	assert.ElementsMatch(t, []string{"https://erp.example.com", "42", "erp-key", "a", "b"}, ingestion.SecretNames(r.Definition.Source), "every text setting counts as a secret name")

	// An extension kind's value may hold nothing but the delivery URL.
	r = ingestion.ApplyConfig(d, "orders", entry(`{"targetUrl":"https://hooks.example.com/orders"}`))
	assert.Equal(t, ingestion.ConfigSet, r.Status, r.Problems.String())
}

func TestInvalidEntries(t *testing.T) {
	sftp := parse(t, withoutURL(`{"kind":"sftp","config":"k"}`))
	const good = `"host":"sftp.example.com","port":22,"directory":"/in","username":"feed","credential":{"name":"sftp-password"},"every":"PT5M","knownHosts":"k"`
	for name, c := range map[string]struct {
		def   ingestion.Definition
		value string
		field string
	}{
		"not an object":             {sftp, `[1]`, "config"},
		"not json":                  {sftp, `nope`, "config"},
		"null":                      {sftp, `null`, "config"},
		"an unknown setting":        {sftp, `{` + good + `,"hostt":"x","targetUrl":"https://h.example.com/x"}`, "config"},
		"a bad setting":             {sftp, `{` + strings.Replace(good, `"port":22`, `"port":0`, 1) + `,"targetUrl":"https://h.example.com/x"}`, "source.port"},
		"a loopback host":           {sftp, `{` + strings.Replace(good, `sftp.example.com`, `localhost`, 1) + `,"targetUrl":"https://h.example.com/x"}`, "source.host"},
		"a kind key":                {sftp, `{"kind":"sftp",` + good + `,"targetUrl":"https://h.example.com/x"}`, "config.kind"},
		"a config key":              {sftp, `{"config":"x",` + good + `,"targetUrl":"https://h.example.com/x"}`, "config.config"},
		"no delivery url anywhere":  {sftp, `{` + good + `}`, "target.targetUrl"},
		"a delivery url not text":   {sftp, `{` + good + `,"targetUrl":5}`, "config.targetUrl"},
		"a delivery url not a url":  {sftp, `{` + good + `,"targetUrl":"nope"}`, "target.targetUrl"},
		"a delivery url in both":    {parse(t, definition(`{"kind":"sftp","config":"k"}`, `{"kind":"none"}`)), `{` + good + `,"targetUrl":"https://h.example.com/x"}`, "target.targetUrl"},
		"an extension setting list": {parse(t, withoutURL(`{"kind":"custom","type":"acme","config":"k"}`)), `{"a":[1],"targetUrl":"https://h.example.com/x"}`, "source.settings.a"},
	} {
		t.Run(name, func(t *testing.T) {
			r := ingestion.ApplyConfig(c.def, "orders", entry(c.value))
			require.Equal(t, ingestion.ConfigInvalid, r.Status)
			assert.False(t, r.Runnable())
			assert.Contains(t, fields(r.Problems), c.field, r.Problems.String())
			assert.NotNil(t, r.UpdatedAt, "an invalid entry still says when it was written")
			_, still := r.Definition.Source.(ingestion.Configured)
			assert.True(t, still, "an invalid entry does not replace the reference")
			assert.Empty(t, r.Warnings())
		})
	}
}

func TestSecretTypedEntryIsInvalid(t *testing.T) {
	d := parse(t, withoutURL(`{"kind":"sqs","config":"k"}`))
	r := ingestion.ApplyConfig(d, "orders", &ingestion.ConfigEntry{Value: `{"region":"eu-west-1","queue":"q"}`, Secret: true})
	assert.Equal(t, ingestion.ConfigInvalid, r.Status)
	assert.Contains(t, r.Problems.String(), "PLAIN")
}

func TestEffectiveDoesNotChangeTheReferenceDefinition(t *testing.T) {
	d := parse(t, withoutURL(`{"kind":"sqs","config":"k"}`))
	before, err := json.Marshal(d)
	require.NoError(t, err)
	ingestion.ApplyConfig(d, "orders", entry(`{"region":"eu-west-1","queue":"q","targetUrl":"https://h.example.com/x"}`))
	after, err := json.Marshal(d)
	require.NoError(t, err)
	assert.JSONEq(t, string(before), string(after))
	assert.Empty(t, d.Target.TargetURL)
}

func TestWarningsOfAnEffectiveDefinition(t *testing.T) {
	d := parse(t, withoutURL(`{"kind":"sftp","config":"k"}`))
	r := ingestion.ApplyConfig(d, "orders", entry(`{"host":"sftp.example.com","port":22,"directory":"/in","username":"feed","credential":{"name":"sftp-password"},"every":"PT5M","targetUrl":"https://hooks.example.com/orders"}`))
	require.Equal(t, ingestion.ConfigSet, r.Status, r.Problems.String())
	require.Len(t, r.Warnings(), 1)
	assert.Contains(t, r.Warnings()[0], "knownHosts", "an sftp source that checks no host key is warned about, wherever its settings are")
}

// A problem is shown to everyone who can view the ingestion, who may not read
// the entry, so a problem names the field and the type it wants, never the
// value the decoder choked on.
func TestProblemsOfAnEntryDoNotQuoteItsValue(t *testing.T) {
	sftp := parse(t, withoutURL(`{"kind":"sftp","config":"k"}`))
	for name, c := range map[string]struct {
		def           ingestion.Definition
		value, secret string
		says          string
	}{
		"a number where a whole one is wanted": {sftp, `{"host":"h.example.com","port":1.5,"targetUrl":"https://h.example.com/x"}`, "1.5", "settings.port must be of type int"},
		"text where a number is wanted":        {sftp, `{"host":"h.example.com","port":"s3cr3t-port","targetUrl":"https://h.example.com/x"}`, "s3cr3t-port", "settings.port must be of type int"},
		"an unknown field":                     {sftp, `{"hostt":"h.example.com","targetUrl":"https://h.example.com/x"}`, "h.example.com", `unknown field "hostt"`},
	} {
		t.Run(name, func(t *testing.T) {
			r := ingestion.ApplyConfig(c.def, "orders", entry(c.value))
			require.Equal(t, ingestion.ConfigInvalid, r.Status, r.Problems.String())
			text := r.Problems.String()
			assert.NotContains(t, text, c.secret)
			assert.Contains(t, text, c.says)
		})
	}
}

func TestRedactedResolvedKeepsTheStatusAndNotTheEntry(t *testing.T) {
	d := parse(t, withoutURL(`{"kind":"sftp","config":"k"}`))
	r := ingestion.ApplyConfig(d, "orders", entry(`{"host":"sftp.vendor.example","port":22,"directory":"/out","username":"svc","credential":{"name":"vendor-pass"},"every":"PT5M","targetUrl":"https://hooks.example.com/orders"}`))
	require.Equal(t, ingestion.ConfigSet, r.Status, r.Problems.String())

	seen := r.Redacted(d)
	assert.Equal(t, ingestion.ConfigSet, seen.Status)
	assert.True(t, seen.Runnable())
	assert.Equal(t, r.Key, seen.Key)
	assert.NotNil(t, seen.UpdatedAt)
	_, still := seen.Definition.Source.(ingestion.Configured)
	assert.True(t, still, "the reference stands in for the effective definition")
	assert.Empty(t, seen.Definition.Target.TargetURL)
	assert.Empty(t, seen.Warnings(), "an effective definition's warnings describe the entry")

	inline := ingestion.ApplyConfig(parse(t, definition(`{"kind":"sqs","region":"eu-west-1","queue":"q"}`, `{"kind":"none"}`)), "orders", nil)
	assert.Equal(t, inline, inline.Redacted(inline.Definition), "settings inline have no entry to withhold")
}
