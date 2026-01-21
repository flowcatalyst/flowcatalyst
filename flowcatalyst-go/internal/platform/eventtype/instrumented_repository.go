package eventtype

import (
	"context"

	"go.flowcatalyst.tech/internal/common/repository"
)

const collectionName = "event_types"

// instrumentedRepository wraps a Repository with metrics and logging
type instrumentedRepository struct {
	inner Repository
}

// newInstrumentedRepository creates an instrumented wrapper around a Repository
func newInstrumentedRepository(inner Repository) Repository {
	return &instrumentedRepository{inner: inner}
}

func (r *instrumentedRepository) FindAll(ctx context.Context) ([]*EventType, error) {
	return repository.Instrument(ctx, collectionName, "FindAll", func() ([]*EventType, error) {
		return r.inner.FindAll(ctx)
	})
}

func (r *instrumentedRepository) FindByID(ctx context.Context, id string) (*EventType, error) {
	return repository.Instrument(ctx, collectionName, "FindByID", func() (*EventType, error) {
		return r.inner.FindByID(ctx, id)
	})
}

func (r *instrumentedRepository) FindByCode(ctx context.Context, code string) (*EventType, error) {
	return repository.Instrument(ctx, collectionName, "FindByCode", func() (*EventType, error) {
		return r.inner.FindByCode(ctx, code)
	})
}

func (r *instrumentedRepository) Insert(ctx context.Context, et *EventType) error {
	return repository.InstrumentVoid(ctx, collectionName, "Insert", func() error {
		return r.inner.Insert(ctx, et)
	})
}

func (r *instrumentedRepository) Update(ctx context.Context, et *EventType) error {
	return repository.InstrumentVoid(ctx, collectionName, "Update", func() error {
		return r.inner.Update(ctx, et)
	})
}

func (r *instrumentedRepository) Delete(ctx context.Context, id string) error {
	return repository.InstrumentVoid(ctx, collectionName, "Delete", func() error {
		return r.inner.Delete(ctx, id)
	})
}
