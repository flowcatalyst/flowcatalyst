package dispatchjob

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSubscriptionMessageGroup(t *testing.T) {
	assert.Nil(t, SubscriptionMessageGroup("sub_1", nil), "no event group: unordered")
	empty := ""
	assert.Nil(t, SubscriptionMessageGroup("sub_1", &empty))

	g := "order-1"
	a, b := SubscriptionMessageGroup("sub_a", &g), SubscriptionMessageGroup("sub_b", &g)
	require.NotNil(t, a)
	assert.Equal(t, "sub_a:order-1", *a)
	assert.NotEqual(t, *a, *b, "two subscribers of one event group are two groups")

	fits := strings.Repeat("x", MaxMessageGroupLen-len("sub_a:"))
	assert.Equal(t, "sub_a:"+fits, *SubscriptionMessageGroup("sub_a", &fits), "exactly at the limit: plain")

	long := fits + "y"
	sum := sha256.Sum256([]byte(long))
	d := SubscriptionMessageGroup("sub_a", &long)
	assert.Equal(t, "sub_a:sha256:"+hex.EncodeToString(sum[:]), *d)
	assert.LessOrEqual(t, len(*d), MaxMessageGroupLen)
	other := long + "z"
	assert.NotEqual(t, *d, *SubscriptionMessageGroup("sub_a", &other), "distinct long groups stay distinct")
}

func TestEventMessageGroup(t *testing.T) {
	sub := "sub_a"
	g := "order-1"
	job := &DispatchJob{SubscriptionID: &sub, MessageGroup: SubscriptionMessageGroup(sub, &g)}
	got, ok := EventMessageGroup(job)
	require.True(t, ok)
	assert.Equal(t, "order-1", *got, "the subscriber is told the event's own group")

	long := strings.Repeat("x", 300)
	job.MessageGroup = SubscriptionMessageGroup(sub, &long)
	_, ok = EventMessageGroup(job)
	assert.False(t, ok, "the digest form must be read from the event")

	bare := "order-2"
	direct := &DispatchJob{SubscriptionID: &sub, MessageGroup: &bare}
	got, ok = EventMessageGroup(direct)
	require.True(t, ok)
	assert.Equal(t, "order-2", *got, "a group without the prefix (direct, or pre-change) is unchanged")

	noSub := &DispatchJob{MessageGroup: &bare}
	got, ok = EventMessageGroup(noSub)
	require.True(t, ok)
	assert.Equal(t, "order-2", *got)

	got, ok = EventMessageGroup(&DispatchJob{SubscriptionID: &sub})
	assert.True(t, ok)
	assert.Nil(t, got)
}
