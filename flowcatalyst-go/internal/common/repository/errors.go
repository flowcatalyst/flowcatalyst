package repository

import "errors"

// Common repository errors
var (
	// ErrNotFound indicates the requested entity was not found
	ErrNotFound = errors.New("entity not found")

	// ErrDuplicateKey indicates a unique constraint violation
	ErrDuplicateKey = errors.New("duplicate key")

	// ErrOptimisticLock indicates a concurrent modification conflict
	ErrOptimisticLock = errors.New("optimistic lock failed")
)
