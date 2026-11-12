package processing

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchjob"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/subscription"
)

// The function-target decision rests on the stored subscription, and a job
// that merely names a function subscription with a target of its own does not
// qualify.
func TestFunctionTargetResolver(t *testing.T) {
	fn := "fn_1"
	subs := map[string]*subscription.Subscription{
		"sub_fn":  {ID: "sub_fn", Source: subscription.SourceFunction, FunctionID: &fn, Endpoint: "http://127.0.0.1:8095/fn/a"},
		"sub_api": {ID: "sub_api", Source: subscription.SourceAPI, Endpoint: "http://127.0.0.1:8095/fn/a"},
	}
	lookups := 0
	r := NewFunctionTargetResolver(func(_ context.Context, id string) (*subscription.Subscription, error) {
		lookups++
		if id == "boom" {
			return nil, errors.New("db down")
		}
		return subs[id], nil
	})
	job := func(sub, target string) *dispatchjob.DispatchJob {
		j := &dispatchjob.DispatchJob{TargetURL: target}
		if sub != "" {
			j.SubscriptionID = &sub
		}
		return j
	}
	ctx := context.Background()
	assert.True(t, r(ctx, job("sub_fn", "http://127.0.0.1:8095/fn/a")), "a function subscription's own job")
	assert.False(t, r(ctx, job("sub_fn", "http://127.0.0.1:8095/other")), "a customer-chosen target on a function subscription id")
	assert.False(t, r(ctx, job("sub_api", "http://127.0.0.1:8095/fn/a")), "a customer-authored subscription")
	assert.False(t, r(ctx, job("", "http://127.0.0.1:8095/fn/a")), "no subscription")
	assert.False(t, r(ctx, job("nope", "http://127.0.0.1:8095/fn/a")), "unknown subscription")
	assert.False(t, r(ctx, job("boom", "x")), "a failed lookup falls to the customer policy")
	before := lookups
	assert.True(t, r(ctx, job("sub_fn", "http://127.0.0.1:8095/fn/a")))
	assert.Equal(t, before, lookups, "a known subscription is served from the memo")
}
