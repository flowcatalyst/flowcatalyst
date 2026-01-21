package subscription

import "context"

// Repository defines the interface for subscription data access.
// All implementations must be wrapped with instrumentation.
type Repository interface {
	FindSubscriptionByID(ctx context.Context, id string) (*Subscription, error)
	FindSubscriptionByCode(ctx context.Context, code string) (*Subscription, error)
	FindSubscriptionsByClient(ctx context.Context, clientID string) ([]*Subscription, error)
	FindActiveSubscriptions(ctx context.Context) ([]*Subscription, error)
	FindSubscriptionsByEventType(ctx context.Context, eventTypeCode string) ([]*Subscription, error)
	FindAllSubscriptions(ctx context.Context, skip, limit int64) ([]*Subscription, error)
	InsertSubscription(ctx context.Context, sub *Subscription) error
	UpdateSubscription(ctx context.Context, sub *Subscription) error
	UpdateSubscriptionStatus(ctx context.Context, id string, status SubscriptionStatus) error
	DeleteSubscription(ctx context.Context, id string) error
}
