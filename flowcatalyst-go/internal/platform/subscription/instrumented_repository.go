package subscription

import (
	"context"

	"go.flowcatalyst.tech/internal/common/repository"
)

const collectionName = "subscriptions"

// instrumentedRepository wraps a Repository with metrics and logging
type instrumentedRepository struct {
	inner Repository
}

// newInstrumentedRepository creates an instrumented wrapper around a Repository
func newInstrumentedRepository(inner Repository) Repository {
	return &instrumentedRepository{inner: inner}
}

func (r *instrumentedRepository) FindSubscriptionByID(ctx context.Context, id string) (*Subscription, error) {
	return repository.Instrument(ctx, collectionName, "FindSubscriptionByID", func() (*Subscription, error) {
		return r.inner.FindSubscriptionByID(ctx, id)
	})
}

func (r *instrumentedRepository) FindSubscriptionByCode(ctx context.Context, code string) (*Subscription, error) {
	return repository.Instrument(ctx, collectionName, "FindSubscriptionByCode", func() (*Subscription, error) {
		return r.inner.FindSubscriptionByCode(ctx, code)
	})
}

func (r *instrumentedRepository) FindSubscriptionsByClient(ctx context.Context, clientID string) ([]*Subscription, error) {
	return repository.Instrument(ctx, collectionName, "FindSubscriptionsByClient", func() ([]*Subscription, error) {
		return r.inner.FindSubscriptionsByClient(ctx, clientID)
	})
}

func (r *instrumentedRepository) FindActiveSubscriptions(ctx context.Context) ([]*Subscription, error) {
	return repository.Instrument(ctx, collectionName, "FindActiveSubscriptions", func() ([]*Subscription, error) {
		return r.inner.FindActiveSubscriptions(ctx)
	})
}

func (r *instrumentedRepository) FindSubscriptionsByEventType(ctx context.Context, eventTypeCode string) ([]*Subscription, error) {
	return repository.Instrument(ctx, collectionName, "FindSubscriptionsByEventType", func() ([]*Subscription, error) {
		return r.inner.FindSubscriptionsByEventType(ctx, eventTypeCode)
	})
}

func (r *instrumentedRepository) FindAllSubscriptions(ctx context.Context, skip, limit int64) ([]*Subscription, error) {
	return repository.Instrument(ctx, collectionName, "FindAllSubscriptions", func() ([]*Subscription, error) {
		return r.inner.FindAllSubscriptions(ctx, skip, limit)
	})
}

func (r *instrumentedRepository) InsertSubscription(ctx context.Context, sub *Subscription) error {
	return repository.InstrumentVoid(ctx, collectionName, "InsertSubscription", func() error {
		return r.inner.InsertSubscription(ctx, sub)
	})
}

func (r *instrumentedRepository) UpdateSubscription(ctx context.Context, sub *Subscription) error {
	return repository.InstrumentVoid(ctx, collectionName, "UpdateSubscription", func() error {
		return r.inner.UpdateSubscription(ctx, sub)
	})
}

func (r *instrumentedRepository) UpdateSubscriptionStatus(ctx context.Context, id string, status SubscriptionStatus) error {
	return repository.InstrumentVoid(ctx, collectionName, "UpdateSubscriptionStatus", func() error {
		return r.inner.UpdateSubscriptionStatus(ctx, id, status)
	})
}

func (r *instrumentedRepository) DeleteSubscription(ctx context.Context, id string) error {
	return repository.InstrumentVoid(ctx, collectionName, "DeleteSubscription", func() error {
		return r.inner.DeleteSubscription(ctx, id)
	})
}
