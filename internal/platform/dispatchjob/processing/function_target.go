package processing

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchjob"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/subscription"
)

// functionTargetTTL bounds how long a subscription lookup is remembered. A
// subscription's Source never changes, so this only bounds staleness after the
// row is deleted or its endpoint edited.
const functionTargetTTL = time.Minute

// functionTargetCacheMax bounds the memo; at the limit it is emptied.
const functionTargetCacheMax = 4096

// NewFunctionTargetResolver tells the dispatch callback which jobs are
// delivered to the platform's own function runner, so they go through the
// internal delivery policy (netguard.Internal), the only one that exempts it.
//
// The decision rests on stored facts only: the job's subscription exists, was
// written by a function promote (Source FUNCTION, FunctionID set, which only
// promote wiring writes), and the job's target is that subscription's own
// endpoint. A customer can name any subscriptionId on a job it creates through
// the SDK, which is why the target must match too: a customer-chosen target is
// never the one the platform stored on the subscription. A failed lookup is
// "no", so it falls to the customer policy.
func NewFunctionTargetResolver(find func(ctx context.Context, id string) (*subscription.Subscription, error)) FunctionTargetResolver {
	type entry struct {
		sub *subscription.Subscription
		at  time.Time
	}
	var (
		mu   sync.Mutex
		memo = map[string]entry{}
	)
	return func(ctx context.Context, job *dispatchjob.DispatchJob) bool {
		if job.SubscriptionID == nil || *job.SubscriptionID == "" {
			return false
		}
		id := *job.SubscriptionID
		mu.Lock()
		e, ok := memo[id]
		mu.Unlock()
		if !ok || time.Since(e.at) > functionTargetTTL {
			sub, err := find(ctx, id)
			if err != nil {
				slog.Warn("dispatch delivery: subscription lookup failed; using the customer delivery policy", "subscription_id", id, "err", err)
				return false
			}
			e = entry{sub: sub, at: time.Now()}
			mu.Lock()
			if len(memo) >= functionTargetCacheMax {
				clear(memo)
			}
			memo[id] = e
			mu.Unlock()
		}
		s := e.sub
		return s != nil && s.Source == subscription.SourceFunction &&
			s.FunctionID != nil && *s.FunctionID != "" && s.Endpoint == job.TargetURL
	}
}
