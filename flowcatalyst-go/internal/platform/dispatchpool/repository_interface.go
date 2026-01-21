package dispatchpool

import "context"

// Repository defines the interface for dispatch pool data access.
// All implementations must be wrapped with instrumentation.
type Repository interface {
	FindByID(ctx context.Context, id string) (*DispatchPool, error)
	FindByCode(ctx context.Context, code string) (*DispatchPool, error)
	FindAll(ctx context.Context) ([]*DispatchPool, error)
	FindAllEnabled(ctx context.Context) ([]*DispatchPool, error)
	FindAllActive(ctx context.Context) ([]*DispatchPool, error)
	FindByStatus(ctx context.Context, status DispatchPoolStatus) ([]*DispatchPool, error)
	FindAnchorLevel(ctx context.Context) ([]*DispatchPool, error)
	FindAllNonArchived(ctx context.Context) ([]*DispatchPool, error)
	FindByClientID(ctx context.Context, clientID string) ([]*DispatchPool, error)
	Insert(ctx context.Context, pool *DispatchPool) error
	Update(ctx context.Context, pool *DispatchPool) error
	UpdateConfig(ctx context.Context, id string, concurrency, queueCapacity int, rateLimitPerMin *int) error
	SetEnabled(ctx context.Context, id string, enabled bool) error
	SetStatus(ctx context.Context, id string, status DispatchPoolStatus) error
	Delete(ctx context.Context, id string) error
	Count(ctx context.Context) (int64, error)
	CountEnabled(ctx context.Context) (int64, error)
	CountActive(ctx context.Context) (int64, error)
	CountByStatus(ctx context.Context, status DispatchPoolStatus) (int64, error)
	ExistsByCode(ctx context.Context, code string) (bool, error)
}
