package permission

import (
	"context"

	"go.flowcatalyst.tech/internal/common/repository"
)

const collectionName = "auth_permissions"

// instrumentedRepository wraps a Repository with metrics and logging
type instrumentedRepository struct {
	inner Repository
}

// newInstrumentedRepository creates an instrumented wrapper around a Repository
func newInstrumentedRepository(inner Repository) Repository {
	return &instrumentedRepository{inner: inner}
}

func (r *instrumentedRepository) FindAll(ctx context.Context) ([]*Permission, error) {
	return repository.Instrument(ctx, collectionName, "FindAll", func() ([]*Permission, error) {
		return r.inner.FindAll(ctx)
	})
}

func (r *instrumentedRepository) FindByID(ctx context.Context, id string) (*Permission, error) {
	return repository.Instrument(ctx, collectionName, "FindByID", func() (*Permission, error) {
		return r.inner.FindByID(ctx, id)
	})
}

func (r *instrumentedRepository) FindByCode(ctx context.Context, code string) (*Permission, error) {
	return repository.Instrument(ctx, collectionName, "FindByCode", func() (*Permission, error) {
		return r.inner.FindByCode(ctx, code)
	})
}

func (r *instrumentedRepository) Insert(ctx context.Context, perm *Permission) error {
	return repository.InstrumentVoid(ctx, collectionName, "Insert", func() error {
		return r.inner.Insert(ctx, perm)
	})
}
