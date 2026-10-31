package ingestion_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/ingestion"
)

const (
	httpBearer = `{"kind":"http","auth":{"kind":"bearer","token":{"name":"orders-token"}}`
	eventCode  = `"code":"acme:orders:order:received"`
)

// withTarget is a definition around a source, a split and a target.
func withTarget(source, split, target string) string {
	return `{"name":"Orders","source":` + source + `,"split":` + split + `,"target":` + target + `}`
}

func eventTarget(extra string) string {
	return `{"kind":"EVENT",` + eventCode + `,"batchSize":100` + extra + `}`
}

func TestTargetWithoutKindIsADispatchJob(t *testing.T) {
	d := parse(t, definition(httpBearer+`}`, `{"kind":"none"}`))
	assert.Equal(t, ingestion.TargetDispatchJob, d.Target.EffectiveKind())
	assert.False(t, d.Target.IsEvent())
	assert.False(t, d.Target.DataOnly)
	assert.Empty(t, d.Validate())

	// Written without a kind, as it was read; with one, it is the same definition.
	raw, err := json.Marshal(d)
	require.NoError(t, err)
	assert.NotContains(t, string(raw), `"kind":"DISPATCH_JOB"`)
	explicit := parse(t, withTarget(httpBearer+`}`, `{"kind":"none"}`, strings.Replace(goodTarget, `{"code"`, `{"kind":"DISPATCH_JOB","code"`, 1)))
	assert.True(t, d.Equal(explicit))
	assert.Empty(t, explicit.Validate())
}

func TestDispatchJobTargetDataOnly(t *testing.T) {
	d := parse(t, withTarget(httpBearer+`}`, `{"kind":"none"}`, strings.Replace(goodTarget, `"batchSize":100`, `"batchSize":100,"dataOnly":true`, 1)))
	assert.True(t, d.Target.DataOnly)
	assert.Empty(t, d.Validate())
	assert.False(t, d.Equal(parse(t, definition(httpBearer+`}`, `{"kind":"none"}`))), "dataOnly is part of the definition")
}

func TestDispatchJobTargetRefusesWrapNonJson(t *testing.T) {
	d := parse(t, withTarget(httpBearer+`}`, `{"kind":"none"}`, strings.Replace(goodTarget, `"batchSize":100`, `"batchSize":100,"wrapNonJson":true`, 1)))
	assert.Equal(t, []string{"target.wrapNonJson"}, fields(d.Validate()))
}

func TestUnknownTargetKindIsRefused(t *testing.T) {
	d := parse(t, withTarget(httpBearer+`}`, `{"kind":"none"}`, strings.Replace(goodTarget, `{"code"`, `{"kind":"QUEUE","code"`, 1)))
	assert.Equal(t, []string{"target.kind"}, fields(d.Validate()))
}

func TestEventTargetAcceptsWhatItNeeds(t *testing.T) {
	for name, doc := range map[string]string{
		"wrapped, anything":    withTarget(`{"kind":"sqs","region":"eu-west-1","queue":"q"}`, `{"kind":"none"}`, eventTarget(`,"wrapNonJson":true`)),
		"json split":           withTarget(`{"kind":"sqs","region":"eu-west-1","queue":"q"}`, `{"kind":"json","arrayPath":"$"}`, eventTarget(``)),
		"json split, wrapped":  withTarget(`{"kind":"sqs","region":"eu-west-1","queue":"q"}`, `{"kind":"json","arrayPath":"$"}`, eventTarget(`,"wrapNonJson":true`)),
		"http json body":       withTarget(httpBearer+`,"accept":{"body":"JSON"}}`, `{"kind":"none"}`, eventTarget(``)),
		"http json object":     withTarget(httpBearer+`,"accept":{"contentTypes":["application/json"],"body":"JSON_OBJECT"}}`, `{"kind":"none"}`, eventTarget(``)),
		"http xml, wrapped":    withTarget(httpBearer+`,"accept":{"body":"XML"}}`, `{"kind":"none"}`, eventTarget(`,"wrapNonJson":true`)),
		"http, no accept, wrp": withTarget(httpBearer+`}`, `{"kind":"none"}`, eventTarget(`,"wrapNonJson":true`)),
	} {
		d := parse(t, doc)
		assert.True(t, d.Target.IsEvent(), name)
		assert.Empty(t, d.Validate(), name)
	}
}

func TestEventTargetWrapNonJsonIsRequiredUnlessRecordsAreKnownJSON(t *testing.T) {
	const message = `target.wrapNonJson: these records are not known to be JSON; set wrapNonJson to true to deliver each as {"contentType", "body"}`
	for name, doc := range map[string]string{
		"sqs, no split":          withTarget(`{"kind":"sqs","region":"eu-west-1","queue":"q"}`, `{"kind":"none"}`, eventTarget(``)),
		"csv split":              withTarget(`{"kind":"sqs","region":"eu-west-1","queue":"q"}`, `{"kind":"csv","header":true}`, eventTarget(``)),
		"xml split":              withTarget(`{"kind":"sqs","region":"eu-west-1","queue":"q"}`, `{"kind":"xml","element":"row"}`, eventTarget(``)),
		"http, no accept":        withTarget(httpBearer+`}`, `{"kind":"none"}`, eventTarget(``)),
		"http, body not checked": withTarget(httpBearer+`,"accept":{"body":"NONE"}}`, `{"kind":"none"}`, eventTarget(``)),
		"http xml":               withTarget(httpBearer+`,"accept":{"body":"XML"}}`, `{"kind":"none"}`, eventTarget(``)),
		"http json, csv split":   withTarget(httpBearer+`,"accept":{"body":"JSON"}}`, `{"kind":"csv","header":false}`, eventTarget(``)),
		"http json, wrap false":  withTarget(httpBearer+`,"accept":{"contentTypes":["text/plain"]}}`, `{"kind":"none"}`, eventTarget(`,"wrapNonJson":false`)),
	} {
		p := parse(t, doc).Validate()
		require.Len(t, p, 1, name)
		assert.Equal(t, "target.wrapNonJson "+strings.TrimPrefix(message, "target.wrapNonJson: "), p.String(), name)
	}
}

func TestEventTargetRefusesDispatchJobSettings(t *testing.T) {
	for field, extra := range map[string]string{
		"target.targetUrl":      `,"targetUrl":"https://hooks.example.com/orders"`,
		"target.pool":           `,"pool":"orders"`,
		"target.connection":     `,"connection":"orders-conn"`,
		"target.serviceAccount": `,"serviceAccount":"orders-sa"`,
		"target.mode":           `,"mode":"IMMEDIATE"`,
		"target.dataOnly":       `,"dataOnly":true`,
	} {
		d := parse(t, withTarget(httpBearer+`,"accept":{"body":"JSON"}}`, `{"kind":"none"}`, eventTarget(extra)))
		assert.Equal(t, []string{field}, fields(d.Validate()), field)
	}
}

func TestEventTargetCodeAndBatchSize(t *testing.T) {
	bad := withTarget(httpBearer+`,"accept":{"body":"JSON"}}`, `{"kind":"none"}`, `{"kind":"EVENT","code":"orders","batchSize":0}`)
	assert.ElementsMatch(t, []string{"target.code", "target.batchSize"}, fields(parse(t, bad).Validate()))
}

func TestHTTPAcceptIsValidated(t *testing.T) {
	ok := func(accept string) ingestion.Problems {
		return parse(t, definition(httpBearer+`,"accept":`+accept+`}`, `{"kind":"none"}`)).Validate()
	}
	assert.Empty(t, ok(`{}`))
	assert.Empty(t, ok(`{"contentTypes":[],"body":"NONE"}`))
	assert.Empty(t, ok(`{"contentTypes":["application/xml","text/xml","application/vnd.api+json","Application/JSON"],"body":"XML"}`))
	for _, body := range []string{"JSON", "JSON_OBJECT", "XML", "NONE"} {
		assert.Empty(t, ok(`{"body":"`+body+`"}`), body)
	}
	assert.Equal(t, []string{"source.accept.body"}, fields(ok(`{"body":"YAML"}`)))
	assert.Equal(t, []string{"source.accept.body"}, fields(ok(`{"body":"json"}`)))
	for _, ct := range []string{"xml", "application/", "/json", "application/json; charset=utf-8", "text/*", "a b/c"} {
		assert.Equal(t, []string{"source.accept.contentTypes.0"}, fields(ok(`{"contentTypes":["`+ct+`"]}`)), ct)
	}
	assert.Equal(t, []string{"source.accept.contentTypes.1"}, fields(ok(`{"contentTypes":["text/xml","nope"]}`)))
}

func TestHTTPAcceptIsStrict(t *testing.T) {
	_, err := ingestion.ParseDefinition([]byte(definition(httpBearer+`,"accept":{"contentType":["a/b"]}}`, `{"kind":"none"}`)))
	require.Error(t, err)
	_, err = ingestion.ParseDefinition([]byte(withTarget(httpBearer+`}`, `{"kind":"none"}`, strings.Replace(goodTarget, `{"code"`, `{"dataOnlyy":true,"code"`, 1))))
	require.Error(t, err)
}

func TestAcceptNoneIsTheSameAsNoAccept(t *testing.T) {
	plain := parse(t, definition(httpBearer+`}`, `{"kind":"none"}`))
	assert.True(t, plain.Equal(parse(t, definition(httpBearer+`,"accept":{"body":"NONE"}}`, `{"kind":"none"}`))))
	assert.False(t, plain.Equal(parse(t, definition(httpBearer+`,"accept":{"body":"XML"}}`, `{"kind":"none"}`))))
}

const eventFile = `application: ingest
client: acme
code: orders-push
name: Orders pushed
source:
  kind: http
  auth:
    kind: bearer
    token:
      name: orders-token
  accept:
    contentTypes:
      - application/json
    body: JSON_OBJECT
split:
  kind: none
target:
  kind: EVENT
  code: acme:orders:order:received
  batchSize: 100
`

const dataOnlyFile = `application: ingest
client: acme
code: orders-raw
name: Orders raw
source:
  kind: http
  auth:
    kind: bearer
    token:
      name: orders-token
  accept:
    contentTypes:
      - application/xml
      - text/xml
    body: XML
split:
  kind: none
target:
  kind: DISPATCH_JOB
  code: acme:orders:order:received
  targetUrl: https://hooks.example.com/orders
  pool: orders
  mode: IMMEDIATE
  batchSize: 100
  dataOnly: true
`

func TestFileRoundTripOfTheNewFields(t *testing.T) {
	for name, doc := range map[string]string{"event": eventFile, "data only": dataOnlyFile} {
		files, err := ingestion.ParseFiles([]byte(doc))
		require.NoError(t, err, name)
		require.Len(t, files, 1)
		assert.Empty(t, files[0].Validate(), name)

		out, err := files[0].YAML()
		require.NoError(t, err)
		assert.Equal(t, doc, string(out), name)

		again, err := ingestion.ParseFiles(out)
		require.NoError(t, err)
		assert.Equal(t, files, again, name)

		// And the JSON form the API takes.
		raw, err := json.Marshal(files[0])
		require.NoError(t, err)
		var back ingestion.File
		require.NoError(t, json.Unmarshal(raw, &back))
		assert.Equal(t, files[0], back, name)
	}

	files, err := ingestion.ParseFiles([]byte(eventFile))
	require.NoError(t, err)
	h, ok := files[0].Source.(ingestion.HTTP)
	require.True(t, ok)
	require.NotNil(t, h.Accept)
	assert.Equal(t, []string{"application/json"}, h.Accept.ContentTypes)
	assert.Equal(t, ingestion.BodyJSONObject, h.Accept.Body)
	assert.True(t, files[0].Target.IsEvent())
}

func TestFileRefusesAnUnknownTargetField(t *testing.T) {
	_, err := ingestion.ParseFiles([]byte(strings.Replace(eventFile, "batchSize: 100", "batchSize: 100\n  wrapJson: true", 1)))
	require.Error(t, err)
}

// A config entry's targetUrl is a delivery address; an event target has none,
// so the entry's does not apply (and the definition needs none).
func TestEventTargetIgnoresTheEntrysTargetURL(t *testing.T) {
	d := parse(t, withTarget(`{"kind":"sqs","config":"orders"}`, `{"kind":"none"}`, eventTarget(`,"wrapNonJson":true`)))
	assert.Empty(t, d.Validate())
	r := ingestion.ApplyConfig(d, "orders", entry(`{"region":"eu-west-1","queue":"orders-q","targetUrl":"https://hooks.example.com/orders"}`))
	assert.Equal(t, ingestion.ConfigSet, r.Status, "%v", r.Problems)
	assert.Empty(t, r.Definition.Target.TargetURL)
	r = ingestion.ApplyConfig(d, "orders", entry(`{"region":"eu-west-1","queue":"orders-q"}`))
	assert.Equal(t, ingestion.ConfigSet, r.Status, "%v", r.Problems)
}
