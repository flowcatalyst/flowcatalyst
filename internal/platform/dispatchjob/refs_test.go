package dispatchjob

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNameMatch(t *testing.T) {
	var args []any
	assert.Equal(t, "code = $1", nameMatch("code", "orders", false, &args))
	assert.Equal(t, "lower(name) = lower($2)", nameMatch("name", "Orders", true, &args))
	assert.Equal(t, `code LIKE $3 ESCAPE '\'`, nameMatch("code", "ord_%*", false, &args))
	assert.Equal(t, `lower(name) LIKE lower($4) ESCAPE '\'`, nameMatch("name", "Or*", true, &args))
	assert.Equal(t, []any{"orders", "Orders", `ord\_\%%`, "Or%"}, args, "a prefix escapes percent, underscore and backslash; a trailing * is the only wildcard")
}

// A by-name reference that was never resolved must not widen the query: it selects nothing.
func TestUnresolvedReferenceFailsClosed(t *testing.T) {
	name := "Orders"
	q, _ := ListSQL(FilterParams{SubscriptionName: &name})
	assert.True(t, strings.Contains(q, "FALSE"), q)

	q, _ = ListSQL(FilterParams{})
	assert.False(t, strings.Contains(q, "FALSE"), q)
}

func TestFilterParamsRenderTheNewFilters(t *testing.T) {
	q, args := ListSQL(FilterParams{
		EventTypes:      []string{"a:b:c:d", "a:b:c:e"},
		SubscriptionIDs: []string{"sub_1"},
		refs:            &resolvedRefs{hasC: true, connIDs: []string{"con_1"}},
	})
	assert.Contains(t, q, "code = ANY($1)")
	assert.Contains(t, q, "subscription_id = ANY($2)")
	assert.Contains(t, q, "(connection_id = ANY($3) OR subscription_id = ANY($4))")
	assert.Equal(t, []any{[]string{"a:b:c:d", "a:b:c:e"}, []string{"sub_1"}, []string{"con_1"}, []string{}, 100}, args)
}
