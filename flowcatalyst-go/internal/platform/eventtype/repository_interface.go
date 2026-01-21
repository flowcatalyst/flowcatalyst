package eventtype

import "context"

// Repository defines the interface for event type data access.
// All implementations must be wrapped with instrumentation.
type Repository interface {
	FindAll(ctx context.Context) ([]*EventType, error)
	FindByID(ctx context.Context, id string) (*EventType, error)
	FindByCode(ctx context.Context, code string) (*EventType, error)
	Insert(ctx context.Context, et *EventType) error
	Update(ctx context.Context, et *EventType) error
	Delete(ctx context.Context, id string) error
}
