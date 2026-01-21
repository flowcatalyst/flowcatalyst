package role

import "context"

// Repository defines the interface for role data access.
// All implementations must be wrapped with instrumentation.
type Repository interface {
	FindAll(ctx context.Context) ([]*Role, error)
	FindByID(ctx context.Context, id string) (*Role, error)
	FindByCode(ctx context.Context, code string) (*Role, error)
	Insert(ctx context.Context, role *Role) error
	Update(ctx context.Context, role *Role) error
	Delete(ctx context.Context, id string) error
}
