package principal

import "context"

// Repository defines the interface for principal data access.
// All implementations must be wrapped with instrumentation.
type Repository interface {
	// Query operations
	FindByID(ctx context.Context, id string) (*Principal, error)
	FindByEmail(ctx context.Context, email string) (*Principal, error)
	FindByClientID(ctx context.Context, clientID string, skip, limit int64) ([]*Principal, error)
	FindByType(ctx context.Context, principalType PrincipalType, skip, limit int64) ([]*Principal, error)
	FindActive(ctx context.Context, skip, limit int64) ([]*Principal, error)
	FindAll(ctx context.Context, skip, limit int64) ([]*Principal, error)
	Count(ctx context.Context) (int64, error)
	CountByType(ctx context.Context, principalType PrincipalType) (int64, error)
	ExistsByEmail(ctx context.Context, email string) (bool, error)

	// Write operations
	Insert(ctx context.Context, principal *Principal) error
	Update(ctx context.Context, principal *Principal) error
	UpdateRoles(ctx context.Context, id string, roles []RoleAssignment) error
	UpdateLastLogin(ctx context.Context, id string) error
	SetActive(ctx context.Context, id string, active bool) error
	Delete(ctx context.Context, id string) error
}
