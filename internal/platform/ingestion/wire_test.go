package ingestion_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/ingestion"
)

func TestDurationParsesIso8601(t *testing.T) {
	cases := map[string]time.Duration{
		"PT10S":    10 * time.Second,
		"PT5M":     5 * time.Minute,
		"PT1H":     time.Hour,
		"PT1H30M":  90 * time.Minute,
		"P1D":      24 * time.Hour,
		"P1DT2H":   26 * time.Hour,
		"PT0.5S":   500 * time.Millisecond,
		"pt30s":    30 * time.Second,
		"PT0.001S": time.Millisecond,
	}
	for in, want := range cases {
		got, err := ingestion.ParseDuration(in)
		require.NoError(t, err, in)
		assert.Equal(t, want, got, in)
	}
	for _, bad := range []string{"", "P", "PT", "P1DT", "10S", "PT-5S", "PT5", "1H", "PT1.S", "PT5M1H"} {
		_, err := ingestion.ParseDuration(bad)
		assert.Error(t, err, "%q", bad)
	}
}

func TestDurationFormatsAsJavaDoes(t *testing.T) {
	cases := map[time.Duration]string{
		0:                                "PT0S",
		10 * time.Second:                 "PT10S",
		5 * time.Minute:                  "PT5M",
		time.Hour:                        "PT1H",
		26 * time.Hour:                   "PT26H",
		90*time.Minute + 30*time.Second:  "PT1H30M30S",
		1500 * time.Millisecond:          "PT1.5S",
		time.Hour + 250*time.Millisecond: "PT1H0.25S",
	}
	for in, want := range cases {
		assert.Equal(t, want, ingestion.Duration(in).String())
		back, err := ingestion.ParseDuration(want)
		require.NoError(t, err)
		assert.Equal(t, in, back, "round trip of %s", want)
	}
}

func TestDurationJSONIsAString(t *testing.T) {
	var d ingestion.Duration
	require.NoError(t, json.Unmarshal([]byte(`"PT90S"`), &d))
	assert.Equal(t, 90*time.Second, time.Duration(d))
	assert.Error(t, json.Unmarshal([]byte(`30`), &d), "a bare number is not a duration")
	out, _ := json.Marshal(ingestion.Duration(time.Minute))
	assert.Equal(t, `"PT1M"`, string(out))
}

func TestUnknownFieldsAreRejectedEverywhere(t *testing.T) {
	const source = `{"kind":"sqs","region":"eu-west-1","queue":"orders"}`
	cases := map[string]string{
		"top level":                  `{"name":"x","bogus":1,"source":` + source + `,"split":{"kind":"none"},"target":` + goodTarget + `}`,
		"in a source":                `{"name":"x","source":{"kind":"sqs","region":"eu-west-1","queue":"o","bogus":1},"split":{"kind":"none"},"target":` + goodTarget + `}`,
		"in a split":                 `{"name":"x","source":` + source + `,"split":{"kind":"csv","header":true,"bogus":1},"target":` + goodTarget + `}`,
		"in the target":              `{"name":"x","source":` + source + `,"split":{"kind":"none"},"target":{"code":"a:b:c:d","bogus":1}}`,
		"in nested auth":             `{"name":"x","source":{"kind":"http","auth":{"kind":"bearer","token":{"name":"t","bogus":1}}},"split":{"kind":"none"},"target":` + goodTarget + `}`,
		"in a union inside a source": `{"name":"x","source":{"kind":"http","auth":{"kind":"bearer","token":{"name":"t"},"bogus":1}},"split":{"kind":"none"},"target":` + goodTarget + `}`,
		"unknown source kind":        `{"name":"x","source":{"kind":"ftp"},"split":{"kind":"none"},"target":` + goodTarget + `}`,
		"source without kind":        `{"name":"x","source":{"region":"eu-west-1"},"split":{"kind":"none"},"target":` + goodTarget + `}`,
		"trailing data":              `{"name":"x"} {"name":"y"}`,
	}
	for name, doc := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := ingestion.ParseDefinition([]byte(doc))
			assert.Error(t, err)
		})
	}
}

func TestDefinitionJSONRoundTrips(t *testing.T) {
	for _, c := range sourceCases {
		t.Run(c.name, func(t *testing.T) {
			d := parse(t, definition(c.source, c.split))
			out, err := json.Marshal(d)
			require.NoError(t, err)
			back, err := ingestion.ParseDefinition(out)
			require.NoError(t, err)
			assert.True(t, d.Equal(back))
			assert.Equal(t, d, back)
		})
	}
}

func TestDefinitionEqual(t *testing.T) {
	const source = `{"kind":"sqs","region":"eu-west-1","queue":"orders"}`
	a := parse(t, definition(source, `{"kind":"none"}`))
	assert.True(t, a.Equal(parse(t, definition(source, `{"kind":"none"}`))))
	assert.False(t, a.Equal(parse(t, definition(source, `{"kind":"csv","header":false}`))))
	assert.False(t, a.Equal(parse(t, definition(`{"kind":"sqs","region":"eu-west-2","queue":"orders"}`, `{"kind":"none"}`))))
}
