//go:build integration

package operations_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/process"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/process/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/go-sdk/usecase"
)

// The SPA drawer's process rules are the use cases' (bug 23 in
// docs/ui/README.md; docs/platform-bug-fixes.md): an archived process is not
// edited, a current one is not deleted.
func TestProcessEditAndDeleteFollowTheStatus(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := process.NewRepository(testpg.Pool(t))
	uow := testpg.NewUoW(t)
	seeded := mustCreate(t, repo, uow, "prguard:orders:flow", "Guarded")
	name := "Renamed"

	_, err := runAuthorized(uow, operations.DeleteProcess(repo), operations.DeleteCommand{ID: seeded.ProcessID})
	testpg.RequireUsecaseError(t, err, usecase.KindBusinessRule, "PROCESS_NOT_ARCHIVED")

	_, err = runAuthorized(uow, operations.ArchiveProcess(repo), operations.ArchiveCommand{ID: seeded.ProcessID})
	require.NoError(t, err)
	_, err = runAuthorized(uow, operations.UpdateProcess(repo), operations.UpdateCommand{ID: seeded.ProcessID, Name: &name})
	testpg.RequireUsecaseError(t, err, usecase.KindBusinessRule, "PROCESS_ARCHIVED")
	got, err := repo.FindByID(ctx, seeded.ProcessID)
	require.NoError(t, err)
	assert.Equal(t, "Guarded", got.Name, "a refused edit changes nothing")

	_, err = runAuthorized(uow, operations.DeleteProcess(repo), operations.DeleteCommand{ID: seeded.ProcessID})
	require.NoError(t, err)
}

// A blank description is stored as none (bug 25): the edit sent "" and the
// use case stored the empty string.
func TestUpdateProcess_BlankDescriptionClearsIt(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := process.NewRepository(testpg.Pool(t))
	uow := testpg.NewUoW(t)
	desc := "Has one"
	ev, err := runAuthorized(uow, operations.CreateProcess(repo), operations.CreateCommand{Code: "prblank:orders:flow", Name: "Blank", Description: &desc})
	require.NoError(t, err)

	blank := "  "
	_, err = runAuthorized(uow, operations.UpdateProcess(repo), operations.UpdateCommand{ID: ev.ProcessID, Description: &blank})
	require.NoError(t, err)
	got, err := repo.FindByID(ctx, ev.ProcessID)
	require.NoError(t, err)
	assert.Nil(t, got.Description)
}
