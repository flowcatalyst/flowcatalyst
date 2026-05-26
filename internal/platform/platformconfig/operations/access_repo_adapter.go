package operations

import (
	"context"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/platformconfig"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

// accessRepo adapts platformconfig.Repository's PersistAccess/DeleteAccess
// methods to satisfy usecasepgx.Persist[platformconfig.Access]. The
// underlying repo can't implement Persist[Config] AND Persist[Access]
// directly because Go doesn't allow two methods with the same name on
// the same receiver; this thin wrapper resolves that.
type accessRepo struct {
	inner *platformconfig.Repository
}

func newAccessRepo(r *platformconfig.Repository) *accessRepo { return &accessRepo{inner: r} }

func (a *accessRepo) Persist(ctx context.Context, x *platformconfig.Access, tx *usecasepgx.DbTx) error {
	return a.inner.PersistAccess(ctx, x, tx)
}

func (a *accessRepo) Delete(ctx context.Context, x *platformconfig.Access, tx *usecasepgx.DbTx) error {
	return a.inner.DeleteAccess(ctx, x, tx)
}
