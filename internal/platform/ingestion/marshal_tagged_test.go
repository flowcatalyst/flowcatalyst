package ingestion

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// marshalTagged writes the kind first, the kind alone for no fields (a nil map
// included: owner's decision D6, 2026-11-22), and refuses fields that are not
// an object instead of splicing them into broken JSON.
func TestMarshalTagged(t *testing.T) {
	for name, c := range map[string]struct {
		v    any
		want string
	}{
		"fields":       {map[string]any{"a": 1}, `{"kind":"mix","a":1}`},
		"a struct":     {struct{ A int }{1}, `{"kind":"mix","A":1}`},
		"no fields":    {struct{}{}, `{"kind":"mix"}`},
		"an empty map": {map[string]any{}, `{"kind":"mix"}`},
		"a nil map":    {map[string]any(nil), `{"kind":"mix"}`},
	} {
		t.Run(name, func(t *testing.T) {
			got, err := marshalTagged("mix", c.v)
			require.NoError(t, err)
			assert.True(t, json.Valid(got), string(got))
			assert.JSONEq(t, c.want, string(got))
			assert.Equal(t, `{"kind":"mix"`, string(got[:13]), "the kind first")
		})
	}
	for name, v := range map[string]any{"a list": []int{1}, "a string": "x", "a number": 1} {
		t.Run(name, func(t *testing.T) {
			_, err := marshalTagged("mix", v)
			assert.Error(t, err)
		})
	}
}
