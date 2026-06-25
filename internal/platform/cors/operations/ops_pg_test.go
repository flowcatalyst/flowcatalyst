//go:build integration

package operations_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/cors"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/cors/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecaseop"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

func TestMain(m *testing.M) { testpg.RunMain(m) }

// runAuthorized drives op through the full use-case envelope (Validate →
// Authorize → Execute → atomic commit) as an anchor principal. These tests
// exercise validation, invariants, and persistence; the coarse anchor check is
// controller-gated (the use case's Authorize is Public). It mirrors how the
// HTTP handler runs the operation.
func runAuthorized[C any, E usecase.DomainEvent](
	uow *usecasepgx.UnitOfWork, op usecaseop.Operation[C, E], cmd C,
) (E, error) {
	return usecaseop.Run(testpg.AnchorCtx(), uow, op, cmd, testpg.TestEC())
}

// mustAdd seeds an origin through the public operation — the same path
// production uses. Origins are hand-unique per test: the fixture never
// truncates between tests, so tests own their rows and never assert
// table-wide.
func mustAdd(t *testing.T, repo *cors.Repository, uow *usecasepgx.UnitOfWork, origin string) operations.CorsOriginAdded {
	t.Helper()
	ev, err := runAuthorized(uow, operations.AddOrigin(repo),
		operations.AddCommand{Origin: origin})
	require.NoError(t, err)
	return ev
}

// ── AddOrigin ─────────────────────────────────────────────────────────────

func TestAddOrigin_HappyPath(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := cors.NewRepository(testpg.Pool(t))
	uow := testpg.NewUoW(t)

	desc := "frontend dev origin"
	ev, err := runAuthorized(uow, operations.AddOrigin(repo), operations.AddCommand{
		Origin:      "https://corsadd-happy.example.com:3000",
		Description: &desc,
	})
	require.NoError(t, err)

	assert.NotEmpty(t, ev.OriginID)
	assert.Equal(t, "https://corsadd-happy.example.com:3000", ev.Origin)

	got, err := repo.FindByID(ctx, ev.OriginID)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "https://corsadd-happy.example.com:3000", got.Origin)
	require.NotNil(t, got.Description)
	assert.Equal(t, desc, *got.Description)
	require.NotNil(t, got.CreatedBy)
	assert.Equal(t, testpg.TestEC().PrincipalID, *got.CreatedBy)
}

func TestAddOrigin_Validation(t *testing.T) {
	t.Parallel()
	repo := cors.NewRepository(testpg.Pool(t))
	uow := testpg.NewUoW(t)

	cases := []struct {
		name string
		cmd  operations.AddCommand
		code string
	}{
		{"empty origin", operations.AddCommand{}, "ORIGIN_REQUIRED"},
		{"whitespace origin", operations.AddCommand{Origin: "   "}, "ORIGIN_REQUIRED"},
		{"no scheme", operations.AddCommand{Origin: "corsadd-bad.example.com"}, "INVALID_ORIGIN_FORMAT"},
		{"wrong scheme", operations.AddCommand{Origin: "ftp://corsadd-bad.example.com"}, "INVALID_ORIGIN_FORMAT"},
		{"trailing path", operations.AddCommand{Origin: "https://corsadd-bad.example.com/path"}, "INVALID_ORIGIN_FORMAT"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := runAuthorized(uow, operations.AddOrigin(repo), tc.cmd)
			testpg.RequireUsecaseError(t, err, usecase.KindValidation, tc.code)
		})
	}
}

// Conflict is pinned by seeding through the operation itself: the first
// add IS the seed for the second.
func TestAddOrigin_Duplicate_Conflict(t *testing.T) {
	t.Parallel()
	repo := cors.NewRepository(testpg.Pool(t))
	uow := testpg.NewUoW(t)
	mustAdd(t, repo, uow, "https://corsdup.example.com")

	_, err := runAuthorized(uow, operations.AddOrigin(repo),
		operations.AddCommand{Origin: "https://corsdup.example.com"})
	testpg.RequireUsecaseError(t, err, usecase.KindConflict, "ORIGIN_ALREADY_EXISTS")
}

// ── DeleteOrigin ──────────────────────────────────────────────────────────

func TestDeleteOrigin_HappyPath(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := cors.NewRepository(testpg.Pool(t))
	uow := testpg.NewUoW(t)
	seeded := mustAdd(t, repo, uow, "https://corsdel-happy.example.com")

	ev, err := runAuthorized(uow, operations.DeleteOrigin(repo),
		operations.DeleteCommand{OriginID: seeded.OriginID})
	require.NoError(t, err)
	assert.Equal(t, seeded.OriginID, ev.OriginID)
	assert.Equal(t, "https://corsdel-happy.example.com", ev.Origin)

	got, err := repo.FindByID(ctx, seeded.OriginID)
	require.NoError(t, err)
	assert.Nil(t, got, "deleted row must be gone")
}

// DeleteOrigin has no required-id validation in the operation: an empty
// OriginID falls through FindByID and surfaces as NotFound, same as an
// unknown id.
func TestDeleteOrigin_Errors(t *testing.T) {
	t.Parallel()
	repo := cors.NewRepository(testpg.Pool(t))
	uow := testpg.NewUoW(t)

	_, err := runAuthorized(uow, operations.DeleteOrigin(repo),
		operations.DeleteCommand{})
	testpg.RequireUsecaseError(t, err, usecase.KindNotFound, "CorsOrigin_NOT_FOUND")

	_, err = runAuthorized(uow, operations.DeleteOrigin(repo),
		operations.DeleteCommand{OriginID: "cor_doesnotexist1"})
	testpg.RequireUsecaseError(t, err, usecase.KindNotFound, "CorsOrigin_NOT_FOUND")
}
