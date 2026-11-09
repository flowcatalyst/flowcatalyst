package subscription

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCheckBindingFilters(t *testing.T) {
	blank, f := "  ", "subject like 'orders.%'"
	ok := []EventTypeBinding{{EventTypeCode: "a:b:c:d"}, {EventTypeCode: "a:b:c:e", Filter: &blank}}
	require.NoError(t, CheckBindingFilters(ok))
	assert.Nil(t, ok[1].Filter, "a blank filter is cleared, never carried on")

	err := CheckBindingFilters([]EventTypeBinding{{EventTypeCode: "a:b:c:d", Filter: &f}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "a:b:c:d")
}

func TestNewSubscriptionDefaultsToTheEnvelope(t *testing.T) {
	assert.False(t, New("c", "n", "https://x.test").DataOnly)
}
