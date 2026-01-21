package permission

import "context"

// Repository defines the interface for permission data access.
// All implementations must be wrapped with instrumentation.
type Repository interface {
	FindAll(ctx context.Context) ([]*Permission, error)
	FindByID(ctx context.Context, id string) (*Permission, error)
	FindByCode(ctx context.Context, code string) (*Permission, error)
	Insert(ctx context.Context, perm *Permission) error
}
