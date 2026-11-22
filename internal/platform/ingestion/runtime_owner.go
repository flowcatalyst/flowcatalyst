package ingestion

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/flowcatalyst/go-sdk/usecase"
)

// RuntimeIDNotOwned is the code of the 403 for a runtime id that another
// account registered (docs/ingestion-design.md section 7, "The runtime id's
// owner"): a runtime id belongs to the service account whose poll first
// registered it, and every runtime route refuses it to any other account.
const RuntimeIDNotOwned = "RUNTIME_ID_NOT_OWNED"

// runtimeNotOwned is that refusal. The message does not name the owner.
func runtimeNotOwned(runtimeID string) error {
	return usecase.Authorization(RuntimeIDNotOwned,
		fmt.Sprintf("runtime id %s is registered to another service account", runtimeID))
}

// IsRuntimeNotOwned reports whether err is the refusal of a runtime id that
// another account registered.
func IsRuntimeNotOwned(err error) bool {
	e := usecase.AsError(err)
	return e != nil && e.Code == RuntimeIDNotOwned
}

// ownedByAnother reports whether a runtime row whose principal is owner (nil:
// none recorded yet) belongs to an account other than principal.
func ownedByAnother(owner *string, principal string) bool {
	return owner != nil && *owner != principal
}

// errNoPrincipal is a runtime call made without the calling account: a bug in
// the caller, never a refusal, so it is not answered as one.
var errNoPrincipal = errors.New("ingestion repo: a runtime call names no calling principal")

// CheckRuntimeOwner refuses (403 RUNTIME_ID_NOT_OWNED) a runtime id that an
// account other than principal registered. An id that no account holds (never
// registered, pruned by retention, or registered before migration 125 and not
// polled since) is not refused here: only a poll registers an id, and the
// call's own checks (the kind carried, the lease) decide the rest.
func (r *Repository) CheckRuntimeOwner(ctx context.Context, runtimeID, principal string) error {
	if principal == "" {
		return errNoPrincipal
	}
	owner, err := r.q.IngestionRuntimeOwner(ctx, runtimeID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("ingestion repo: runtime owner: %w", err)
	}
	if ownedByAnother(owner, principal) {
		return runtimeNotOwned(runtimeID)
	}
	return nil
}
