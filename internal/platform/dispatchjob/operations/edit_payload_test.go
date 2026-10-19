package operations

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchjob"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecase"
)

func TestEditPayloadValidate(t *testing.T) {
	t.Parallel()
	op := EditDispatchJobPayload(dispatchjob.NewRepository(nil))
	cases := []struct {
		name string
		cmd  EditPayloadCommand
		code string // "" = valid
	}{
		{"ok", EditPayloadCommand{ID: "x", Payload: `{"a":1}`}, ""},
		{"no id", EditPayloadCommand{Payload: "{}"}, "ID_REQUIRED"},
		{"empty payload", EditPayloadCommand{ID: "x"}, "PAYLOAD_REQUIRED"},
		{"too large", EditPayloadCommand{ID: "x", Payload: strings.Repeat("a", MaxEditedPayloadBytes+1)}, "PAYLOAD_TOO_LARGE"},
		{"at the cap", EditPayloadCommand{ID: "x", Payload: strings.Repeat("a", MaxEditedPayloadBytes), PayloadContentType: new("text/plain")}, ""},
		{"not utf8", EditPayloadCommand{ID: "x", Payload: "a\xffb"}, "PAYLOAD_NOT_UTF8"},
		{"blank content type", EditPayloadCommand{ID: "x", Payload: "<a/>", PayloadContentType: new("  ")}, "CONTENT_TYPE_INVALID"},
		{"content type without slash", EditPayloadCommand{ID: "x", Payload: "<a/>", PayloadContentType: new("xml")}, "CONTENT_TYPE_INVALID"},
		{"xml", EditPayloadCommand{ID: "x", Payload: "<a/>", PayloadContentType: new("application/xml")}, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := op.Validate(context.Background(), c.cmd)
			if c.code == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			var ue *usecase.Error
			if assert.ErrorAs(t, err, &ue) {
				assert.Equal(t, c.code, ue.Code)
			}
		})
	}
}

func TestIsJSONType(t *testing.T) {
	t.Parallel()
	for ct, want := range map[string]bool{
		"application/json":                true,
		"Application/JSON; charset=utf-8": true,
		"application/vnd.api+json":        true,
		"application/xml":                 false,
		"text/plain":                      false,
	} {
		assert.Equal(t, want, isJSONType(ct), ct)
	}
}

func TestDigest(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad", digest("abc"))
}
