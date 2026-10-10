//go:build integration

package operations_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/eventtype"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/eventtype/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
)

// The SPA drawer's archive and delete guards are the use cases' rules (bug 5
// in docs/ui/README.md; docs/platform-bug-fixes.md): archive needs every
// schema version deprecated; delete needs the event type archived, or every
// version still finalising.
func TestArchiveAndDeleteEnforceTheVersionRules(t *testing.T) {
	t.Parallel()
	repo := eventtype.NewRepository(testpg.Pool(t))
	uow := testpg.NewUoW(t)
	schema := json.RawMessage(`{"type":"object"}`)

	seeded := mustCreate(t, repo, uow, "etguard:orders:order:created", "Guards")
	_, err := runAuthorized(uow, operations.AddSchema(repo), operations.AddSchemaCommand{EventTypeID: seeded.EventTypeID, Version: "1.0", Schema: schema})
	require.NoError(t, err)
	_, err = runAuthorized(uow, operations.FinaliseEventTypeSchema(repo), operations.FinaliseSchemaCommand{EventTypeID: seeded.EventTypeID, Version: "1.0"})
	require.NoError(t, err)

	// A CURRENT version: neither archive nor delete.
	_, err = runAuthorized(uow, operations.ArchiveEventType(repo), operations.ArchiveCommand{ID: seeded.EventTypeID})
	testpg.RequireUsecaseError(t, err, usecase.KindBusinessRule, "SCHEMAS_NOT_DEPRECATED")
	_, err = runAuthorized(uow, operations.DeleteEventType(repo), operations.DeleteCommand{ID: seeded.EventTypeID})
	testpg.RequireUsecaseError(t, err, usecase.KindBusinessRule, "DELETE_NOT_ALLOWED")

	// Every version deprecated: archive, then delete.
	_, err = runAuthorized(uow, operations.DeprecateEventTypeSchema(repo), operations.DeprecateSchemaCommand{EventTypeID: seeded.EventTypeID, Version: "1.0"})
	require.NoError(t, err)
	_, err = runAuthorized(uow, operations.DeleteEventType(repo), operations.DeleteCommand{ID: seeded.EventTypeID})
	testpg.RequireUsecaseError(t, err, usecase.KindBusinessRule, "DELETE_NOT_ALLOWED")
	_, err = runAuthorized(uow, operations.ArchiveEventType(repo), operations.ArchiveCommand{ID: seeded.EventTypeID})
	require.NoError(t, err)
	_, err = runAuthorized(uow, operations.DeleteEventType(repo), operations.DeleteCommand{ID: seeded.EventTypeID})
	require.NoError(t, err)

	// Every version still finalising: delete without archiving.
	fresh := mustCreate(t, repo, uow, "etguard:orders:order:drafted", "Draft")
	_, err = runAuthorized(uow, operations.AddSchema(repo), operations.AddSchemaCommand{EventTypeID: fresh.EventTypeID, Version: "1.0", Schema: schema})
	require.NoError(t, err)
	_, err = runAuthorized(uow, operations.ArchiveEventType(repo), operations.ArchiveCommand{ID: fresh.EventTypeID})
	testpg.RequireUsecaseError(t, err, usecase.KindBusinessRule, "SCHEMAS_NOT_DEPRECATED")
	_, err = runAuthorized(uow, operations.DeleteEventType(repo), operations.DeleteCommand{ID: fresh.EventTypeID})
	require.NoError(t, err)
}
