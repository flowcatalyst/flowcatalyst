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
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

func TestMain(m *testing.M) { testpg.RunMain(m) }

// mustCreate seeds a process through the public operation — the same path
// production uses. Codes are hand-unique per test: the fixture never
// truncates between tests, so tests own their rows and never assert
// table-wide.
func mustCreate(t *testing.T, repo *process.Repository, uow *usecasepgx.UnitOfWork, code, name string) operations.ProcessCreated {
	t.Helper()
	committed, err := operations.CreateProcess(context.Background(), repo, uow,
		operations.CreateCommand{Code: code, Name: name}, testpg.TestEC())
	require.NoError(t, err)
	return committed.Event()
}

// ── Create ────────────────────────────────────────────────────────────────

func TestCreateProcess_HappyPath(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := process.NewRepository(testpg.Pool(t))
	uow := testpg.NewUoW(t)

	desc := "How orders get fulfilled"
	committed, err := operations.CreateProcess(ctx, repo, uow, operations.CreateCommand{
		Code:        "prcreate:orders:fulfilment",
		Name:        "Order Fulfilment",
		Description: &desc,
		Body:        "graph TD; A-->B",
		Tags:        []string{"orders", "core"},
	}, testpg.TestEC())
	require.NoError(t, err)

	ev := committed.Event()
	assert.NotEmpty(t, ev.ProcessID)
	assert.Equal(t, "prcreate:orders:fulfilment", ev.Code)
	assert.Equal(t, "Order Fulfilment", ev.Name)

	got, err := repo.FindByID(ctx, ev.ProcessID)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, process.StatusCurrent, got.Status)
	assert.Equal(t, process.SourceUI, got.Source)
	assert.Equal(t, "prcreate", got.Application)
	assert.Equal(t, "orders", got.Subdomain)
	assert.Equal(t, "fulfilment", got.ProcessName)
	assert.Equal(t, "mermaid", got.DiagramType, "diagram type defaults to mermaid")
	assert.Equal(t, "graph TD; A-->B", got.Body)
	require.NotNil(t, got.Description)
	assert.Equal(t, desc, *got.Description)
	assert.Equal(t, []string{"orders", "core"}, got.Tags)
}

// Process codes are THREE segments (application:subdomain:process-name) —
// unlike event types, which use four. A 4-segment code is invalid here.
func TestCreateProcess_Validation(t *testing.T) {
	t.Parallel()
	repo := process.NewRepository(testpg.Pool(t))
	uow := testpg.NewUoW(t)

	cases := []struct {
		name string
		cmd  operations.CreateCommand
		code string
	}{
		{"empty code", operations.CreateCommand{Name: "X"}, "CODE_REQUIRED"},
		{"four segments", operations.CreateCommand{Code: "a:b:c:d", Name: "X"}, "INVALID_CODE_FORMAT"},
		{"blank segment", operations.CreateCommand{Code: "a: :c", Name: "X"}, "INVALID_CODE_FORMAT"},
		{"empty name", operations.CreateCommand{Code: "a:b:c"}, "NAME_REQUIRED"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := operations.CreateProcess(context.Background(), repo, uow, tc.cmd, testpg.TestEC())
			testpg.RequireUsecaseError(t, err, usecase.KindValidation, tc.code)
		})
	}
}

// Conflict is pinned by seeding through the operation itself: the first
// create IS the seed for the second.
func TestCreateProcess_DuplicateCode_Conflict(t *testing.T) {
	t.Parallel()
	repo := process.NewRepository(testpg.Pool(t))
	uow := testpg.NewUoW(t)
	mustCreate(t, repo, uow, "prdup:orders:flow", "First")

	_, err := operations.CreateProcess(context.Background(), repo, uow,
		operations.CreateCommand{Code: "prdup:orders:flow", Name: "Second"}, testpg.TestEC())
	testpg.RequireUsecaseError(t, err, usecase.KindConflict, "CODE_EXISTS")
}

// ── Update ────────────────────────────────────────────────────────────────

func TestUpdateProcess_HappyPath(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := process.NewRepository(testpg.Pool(t))
	uow := testpg.NewUoW(t)
	seeded := mustCreate(t, repo, uow, "prupd:orders:flow", "Before")

	newName := "  After  "
	newDesc := "after"
	newBody := "graph LR; X-->Y"
	newDiagram := "plantuml"
	committed, err := operations.UpdateProcess(ctx, repo, uow, operations.UpdateCommand{
		ID:          seeded.ProcessID,
		Name:        &newName,
		Description: &newDesc,
		Body:        &newBody,
		DiagramType: &newDiagram,
		Tags:        []string{"updated"},
	}, testpg.TestEC())
	require.NoError(t, err)
	assert.Equal(t, seeded.ProcessID, committed.Event().ProcessID)
	assert.Equal(t, "After", committed.Event().Name, "name is trimmed")

	got, err := repo.FindByID(ctx, seeded.ProcessID)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "After", got.Name)
	require.NotNil(t, got.Description)
	assert.Equal(t, "after", *got.Description)
	assert.Equal(t, "graph LR; X-->Y", got.Body)
	assert.Equal(t, "plantuml", got.DiagramType)
	assert.Equal(t, []string{"updated"}, got.Tags)
	assert.Equal(t, "prupd:orders:flow", got.Code, "code is immutable on update")
}

func TestUpdateProcess_Errors(t *testing.T) {
	t.Parallel()
	repo := process.NewRepository(testpg.Pool(t))
	uow := testpg.NewUoW(t)

	blank := " "
	name := "X"
	cases := []struct {
		name string
		cmd  operations.UpdateCommand
		kind usecase.Kind
		code string
	}{
		{"missing id", operations.UpdateCommand{Name: &name}, usecase.KindValidation, "ID_REQUIRED"},
		{"blank name", operations.UpdateCommand{ID: "prc_doesnotexist1", Name: &blank}, usecase.KindValidation, "NAME_REQUIRED"},
		{"unknown id", operations.UpdateCommand{ID: "prc_doesnotexist1", Name: &name}, usecase.KindNotFound, "Process_NOT_FOUND"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := operations.UpdateProcess(context.Background(), repo, uow, tc.cmd, testpg.TestEC())
			testpg.RequireUsecaseError(t, err, tc.kind, tc.code)
		})
	}
}

// ── Delete ────────────────────────────────────────────────────────────────

func TestDeleteProcess_HappyPath(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := process.NewRepository(testpg.Pool(t))
	uow := testpg.NewUoW(t)
	seeded := mustCreate(t, repo, uow, "prdel:orders:flow", "Doomed")

	committed, err := operations.DeleteProcess(ctx, repo, uow,
		operations.DeleteCommand{ID: seeded.ProcessID}, testpg.TestEC())
	require.NoError(t, err)
	assert.Equal(t, seeded.ProcessID, committed.Event().ProcessID)
	assert.Equal(t, "prdel:orders:flow", committed.Event().Code)

	got, err := repo.FindByID(ctx, seeded.ProcessID)
	require.NoError(t, err)
	assert.Nil(t, got, "deleted row must be gone")
}

func TestDeleteProcess_Errors(t *testing.T) {
	t.Parallel()
	repo := process.NewRepository(testpg.Pool(t))
	uow := testpg.NewUoW(t)

	_, err := operations.DeleteProcess(context.Background(), repo, uow,
		operations.DeleteCommand{}, testpg.TestEC())
	testpg.RequireUsecaseError(t, err, usecase.KindValidation, "ID_REQUIRED")

	_, err = operations.DeleteProcess(context.Background(), repo, uow,
		operations.DeleteCommand{ID: "prc_doesnotexist1"}, testpg.TestEC())
	testpg.RequireUsecaseError(t, err, usecase.KindNotFound, "Process_NOT_FOUND")
}

// ── Archive ───────────────────────────────────────────────────────────────
// Unlike eventtype, ArchiveProcess has no ALREADY_ARCHIVED conflict — the
// transition is unconditional.

func TestArchiveProcess_HappyPath(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := process.NewRepository(testpg.Pool(t))
	uow := testpg.NewUoW(t)

	// Deliberately TAGLESS: pins the persist-boundary tags normalization in
	// repository.go — a nil Tags slice on the reload→persist round-trip used
	// to violate msg_processes.tags NOT NULL (23502).
	created, err := operations.CreateProcess(ctx, repo, uow, operations.CreateCommand{
		Code: "prarc:orders:flow", Name: "Archive Me",
	}, testpg.TestEC())
	require.NoError(t, err)
	seeded := created.Event()

	committed, err := operations.ArchiveProcess(ctx, repo, uow,
		operations.ArchiveCommand{ID: seeded.ProcessID}, testpg.TestEC())
	require.NoError(t, err)
	assert.Equal(t, seeded.ProcessID, committed.Event().ProcessID)
	assert.Equal(t, "prarc:orders:flow", committed.Event().Code)

	got, err := repo.FindByID(ctx, seeded.ProcessID)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, process.StatusArchived, got.Status)
}

func TestArchiveProcess_Errors(t *testing.T) {
	t.Parallel()
	repo := process.NewRepository(testpg.Pool(t))
	uow := testpg.NewUoW(t)

	_, err := operations.ArchiveProcess(context.Background(), repo, uow,
		operations.ArchiveCommand{}, testpg.TestEC())
	testpg.RequireUsecaseError(t, err, usecase.KindValidation, "ID_REQUIRED")

	_, err = operations.ArchiveProcess(context.Background(), repo, uow,
		operations.ArchiveCommand{ID: "prc_doesnotexist1"}, testpg.TestEC())
	testpg.RequireUsecaseError(t, err, usecase.KindNotFound, "Process_NOT_FOUND")
}

// ── Sync (app-scoped; created/updated/deleted; API-source-only removal) ───

func TestSyncProcesses_UpsertAndRemoveUnlisted(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := process.NewRepository(testpg.Pool(t))
	uow := testpg.NewUoW(t)
	ec := testpg.TestEC()

	// UI-sourced row in the same application scope: sync must NEVER touch it.
	uiRow := mustCreate(t, repo, uow, "prsync:ui:kept", "UI Kept")

	first, err := operations.SyncProcesses(ctx, repo, uow, operations.SyncProcessesCommand{
		ApplicationCode: "prsync",
		Processes: []operations.SyncProcessInput{
			{Code: "prsync:orders:flow-a", Name: "A"},
			{Code: "prsync:orders:flow-b", Name: "B"},
		},
	}, ec)
	require.NoError(t, err)
	assert.Equal(t, "prsync", first.Event().ApplicationCode)
	assert.Equal(t, uint32(2), first.Event().Created)
	assert.Equal(t, uint32(0), first.Event().Updated)
	assert.Equal(t, uint32(0), first.Event().Deleted)
	assert.Equal(t, []string{"prsync:orders:flow-a", "prsync:orders:flow-b"}, first.Event().SyncedCodes)

	second, err := operations.SyncProcesses(ctx, repo, uow, operations.SyncProcessesCommand{
		ApplicationCode: "prsync",
		Processes: []operations.SyncProcessInput{
			// Deliberately nil Tags: sync is declarative (absent tags = no
			// tags) and the persist-boundary normalization must absorb the
			// nil — this used to 23502 against the NOT NULL tags column.
			{Code: "prsync:orders:flow-a", Name: "A renamed"},
		},
		RemoveUnlisted: true,
	}, ec)
	require.NoError(t, err)
	assert.Equal(t, uint32(0), second.Event().Created)
	assert.Equal(t, uint32(1), second.Event().Updated)
	assert.Equal(t, uint32(1), second.Event().Deleted)

	kept, err := repo.FindByCode(ctx, "prsync:orders:flow-a")
	require.NoError(t, err)
	require.NotNil(t, kept)
	assert.Equal(t, "A renamed", kept.Name)
	assert.Equal(t, process.SourceAPI, kept.Source, "sync-created rows are API-sourced")

	goneB, err := repo.FindByCode(ctx, "prsync:orders:flow-b")
	require.NoError(t, err)
	assert.Nil(t, goneB, "unlisted API row must be deleted")

	stillUI, err := repo.FindByID(ctx, uiRow.ProcessID)
	require.NoError(t, err)
	require.NotNil(t, stillUI, "RemoveUnlisted must never touch UI-sourced rows")
	assert.Equal(t, "UI Kept", stillUI.Name)
}

func TestSyncProcesses_Validation(t *testing.T) {
	t.Parallel()
	repo := process.NewRepository(testpg.Pool(t))
	uow := testpg.NewUoW(t)

	_, err := operations.SyncProcesses(context.Background(), repo, uow,
		operations.SyncProcessesCommand{}, testpg.TestEC())
	testpg.RequireUsecaseError(t, err, usecase.KindValidation, "APPLICATION_CODE_REQUIRED")

	_, err = operations.SyncProcesses(context.Background(), repo, uow, operations.SyncProcessesCommand{
		ApplicationCode: "prsyncbad",
		Processes:       []operations.SyncProcessInput{{Code: "not-three-parts", Name: "X"}},
	}, testpg.TestEC())
	testpg.RequireUsecaseError(t, err, usecase.KindValidation, "INVALID_PROCESS_CODE")
}
