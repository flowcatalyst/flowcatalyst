package ingestion_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/ingestion"
)

// fieldsOf reads a kind's fields the way the platform reads a runtime's report.
func fieldsOf(t *testing.T, doc string) []ingestion.SettingField {
	t.Helper()
	f, ok := ingestion.ParseSettingFields(json.RawMessage(doc))
	require.True(t, ok, doc)
	return f
}

func settings(t *testing.T, doc string) map[string]any {
	t.Helper()
	var m map[string]any
	require.NoError(t, json.Unmarshal([]byte(doc), &m))
	return m
}

func TestParseSettingFieldsOfWhatARuntimeReports(t *testing.T) {
	f := fieldsOf(t, `[{"name":"every","label":"Every","type":"DURATION","required":true,"min":"PT30S","max":"PT1H"},
		{"name":"mode","label":"Mode","type":"CHOICE","required":false,"defaultValue":"a","choices":["a","b"]}]`)
	require.Len(t, f, 2)
	assert.Equal(t, "DURATION", f[0].Type)
	assert.Equal(t, new("PT30S"), f[0].Min)
	assert.Equal(t, []string{"a", "b"}, f[1].Choices)

	for _, none := range []string{``, `null`, `[]`, `{}`, `"x"`} {
		_, ok := ingestion.ParseSettingFields(json.RawMessage(none))
		assert.False(t, ok, "%q: no schema to check against", none)
	}
}

func TestCheckSettingsEachFieldType(t *testing.T) {
	fields := fieldsOf(t, `[
		{"name":"text","type":"TEXT","pattern":"[a-z]{2,5}"},
		{"name":"lines","type":"MULTILINE"},
		{"name":"mode","type":"CHOICE","choices":["fast","slow"]},
		{"name":"count","type":"INTEGER","min":"1","max":"10"},
		{"name":"flag","type":"BOOLEAN"},
		{"name":"every","type":"DURATION","min":"PT30S","max":"PT1H"},
		{"name":"site","type":"URL"},
		{"name":"key","type":"SECRET"},
		{"name":"regs","type":"LIST","pattern":"[A-Z]+","min":"1","max":"3"}
	]`)
	good := `{"text":"abc","lines":"a\nb","mode":"fast","count":5,"flag":true,"every":"PT5M","site":"https://x.example.com/a","key":"erp-key","regs":["A","BB"]}`
	assert.Empty(t, ingestion.CheckSettings(fields, settings(t, good)))

	for _, c := range []struct{ name, doc, field, message string }{
		{"text not matching", `{"text":"ABC"}`, "config.text", "must match [a-z]{2,5}"},
		{"text not text", `{"text":5}`, "config.text", "must be text"},
		{"multiline not text", `{"lines":5}`, "config.lines", "must be text"},
		{"choice not offered", `{"mode":"medium"}`, "config.mode", "must be one of fast, slow"},
		{"choice not text", `{"mode":1}`, "config.mode", "must be one of"},
		{"integer fraction", `{"count":1.5}`, "config.count", "must be a whole number"},
		{"integer text", `{"count":"five"}`, "config.count", "must be a whole number"},
		{"integer below", `{"count":0}`, "config.count", "must be 1 to 10"},
		{"integer above", `{"count":11}`, "config.count", "must be 1 to 10"},
		{"boolean text", `{"flag":"true"}`, "config.flag", "must be true or false"},
		{"duration not iso", `{"every":"5 minutes"}`, "config.every", "must be a duration such as PT30S"},
		{"duration not text", `{"every":300}`, "config.every", "must be a duration such as PT30S"},
		{"duration below", `{"every":"PT1S"}`, "config.every", "must be PT30S to PT1H"},
		{"duration above", `{"every":"PT2H"}`, "config.every", "must be PT30S to PT1H"},
		{"url not http", `{"site":"ftp://x.example.com"}`, "config.site", "must be an http or https address"},
		{"url no host", `{"site":"https://"}`, "config.site", "must be an http or https address"},
		{"secret not text", `{"key":["a"]}`, "config.key", "must be a secret's name"},
		{"list not a list", `{"regs":"A"}`, "config.regs", "must be a list of text"},
		{"list too short", `{"regs":[]}`, "config.regs", "items"},
		{"list too long", `{"regs":["A","B","C","D"]}`, "config.regs", "items"},
		{"list item not matching", `{"regs":["A","b"]}`, "config.regs", "item 2 (b) must match [A-Z]+"},
		{"list item not text", `{"regs":["A",2]}`, "config.regs", "item 2 (2) must be text"},
		{"a setting the kind does not have", `{"nope":1}`, "config.nope", "is not a setting"},
	} {
		t.Run(c.name, func(t *testing.T) {
			p := ingestion.CheckSettings(fields, settings(t, c.doc))
			require.Len(t, p, 1, p.String())
			assert.Equal(t, c.field, p[0].Field)
			assert.Contains(t, p[0].Message, c.message)
		})
	}
}

func TestCheckSettingsIntegerGivenAsDigits(t *testing.T) {
	f := fieldsOf(t, `[{"name":"count","type":"INTEGER","min":"1"}]`)
	assert.Empty(t, ingestion.CheckSettings(f, settings(t, `{"count":"7"}`)), "a form may send digits as text")
	assert.NotEmpty(t, ingestion.CheckSettings(f, settings(t, `{"count":"0"}`)))
}

func TestCheckSettingsRequiredAndDefaults(t *testing.T) {
	f := fieldsOf(t, `[
		{"name":"a","type":"TEXT","required":true},
		{"name":"b","type":"TEXT","required":true,"defaultValue":"x"},
		{"name":"c","type":"MULTILINE","required":true},
		{"name":"d","type":"TEXT"}
	]`)
	p := ingestion.CheckSettings(f, settings(t, `{}`))
	assert.Equal(t, []string{"config.a", "config.c"}, fields(p), "a required setting with a default is not missing")

	p = ingestion.CheckSettings(f, settings(t, `{"a":"  ","c":"","b":null}`))
	assert.Equal(t, []string{"config.a"}, fields(p), "a blank text is not given, but a blank multiline is a value")

	assert.Empty(t, ingestion.CheckSettings(f, settings(t, `{"a":"x","c":"y","d":null}`)))
}

func TestCheckSettingsRuleReplacesThePlainMessage(t *testing.T) {
	f := fieldsOf(t, `[{"name":"every","type":"DURATION","min":"PT30S","rule":"must be between 30 seconds and an hour"}]`)
	p := ingestion.CheckSettings(f, settings(t, `{"every":"PT1S"}`))
	require.Len(t, p, 1)
	assert.Equal(t, "must be between 30 seconds and an hour", p[0].Message)
}

func TestCheckSettingsListsEveryProblemAtOnce(t *testing.T) {
	f := fieldsOf(t, `[{"name":"a","type":"INTEGER","required":true},{"name":"b","type":"BOOLEAN"}]`)
	p := ingestion.CheckSettings(f, settings(t, `{"b":"x","z":1}`))
	assert.ElementsMatch(t, []string{"config.a", "config.b", "config.z"}, fields(p))
}

func TestCheckSettingsLeavesAPatternItCannotCompileToTheRuntime(t *testing.T) {
	f := fieldsOf(t, `[{"name":"a","type":"TEXT","pattern":"(?<name>x)\\k<name>"}]`)
	assert.Empty(t, ingestion.CheckSettings(f, settings(t, `{"a":"anything"}`)))
}
