package api

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestToParams_EventTypesMergeAndCap(t *testing.T) {
	p, err := (&DispatchJobFilter{EventType: "a:b:c:d", EventTypes: "a:b:c:d, a:b:c:e,"}).toParams(false)
	require.NoError(t, err)
	assert.Equal(t, []string{"a:b:c:d", "a:b:c:e"}, p.EventTypes, "eventType joins eventTypes without a duplicate")

	var many []string
	for i := range maxEventTypes + 1 {
		many = append(many, fmt.Sprintf("a:b:c:%d", i))
	}
	// The cap holds on the lenient (list) path too, not only the strict (bulk) one.
	for _, strict := range []bool{false, true} {
		_, err = (&DispatchJobFilter{EventTypes: strings.Join(many, ",")}).toParams(strict)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "at most")
	}
	_, err = (&DispatchJobFilter{EventTypes: strings.Join(many[:maxEventTypes], ",")}).toParams(false)
	assert.NoError(t, err)
}

func TestToParams_SubscriptionAndConnectionReferences(t *testing.T) {
	p, err := (&DispatchJobFilter{SubscriptionCode: " orders-* ", SubscriptionName: "", ConnectionCode: "main", ConnectionID: "con_1"}).toParams(false)
	require.NoError(t, err)
	require.NotNil(t, p.SubscriptionCode)
	assert.Equal(t, "orders-*", *p.SubscriptionCode)
	assert.Nil(t, p.SubscriptionName, "blank is absent")
	assert.Equal(t, "main", *p.ConnectionCode)
	assert.Equal(t, "con_1", *p.ConnectionID)

	var ids []string
	for i := range maxSubscriptionIDs + 1 {
		ids = append(ids, fmt.Sprintf("sub_%d", i))
	}
	_, err = (&DispatchJobFilter{SubscriptionIDs: strings.Join(ids, ",")}).toParams(false)
	require.Error(t, err)
}
