package role

import (
	"context"

	"go.flowcatalyst.tech/internal/common/repository"
)

const collectionName = "auth_roles"

// instrumentedRepository wraps a Repository with metrics and logging
type instrumentedRepository struct {
	inner Repository
}

// newInstrumentedRepository creates an instrumented wrapper around a Repository
func newInstrumentedRepository(inner Repository) Repository {
	return &instrumentedRepository{inner: inner}
}

func (r *instrumentedRepository) FindAll(ctx context.Context) ([]*Role, error) {
	return repository.Instrument(ctx, collectionName, "FindAll", func() ([]*Role, error) {
		return r.inner.FindAll(ctx)
	})
}

func (r *instrumentedRepository) FindByID(ctx context.Context, id string) (*Role, error) {
	return repository.Instrument(ctx, collectionName, "FindByID", func() (*Role, error) {
		return r.inner.FindByID(ctx, id)
	})
}

func (r *instrumentedRepository) FindByCode(ctx context.Context, code string) (*Role, error) {
	return repository.Instrument(ctx, collectionName, "FindByCode", func() (*Role, error) {
		return r.inner.FindByCode(ctx, code)
	})
}

func (r *instrumentedRepository) Insert(ctx context.Context, role *Role) error {
	return repository.InstrumentVoid(ctx, collectionName, "Insert", func() error {
		return r.inner.Insert(ctx, role)
	})
}

func (r *instrumentedRepository) Update(ctx context.Context, role *Role) error {
	return repository.InstrumentVoid(ctx, collectionName, "Update", func() error {
		return r.inner.Update(ctx, role)
	})
}

func (r *instrumentedRepository) Delete(ctx context.Context, id string) error {
	return repository.InstrumentVoid(ctx, collectionName, "Delete", func() error {
		return r.inner.Delete(ctx, id)
	})
}
