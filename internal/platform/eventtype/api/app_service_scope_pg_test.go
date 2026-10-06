//go:build integration

package api

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	appops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/application/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/eventtype"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/eventtype/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecaseop"
)

func TestMain(m *testing.M) { testpg.RunMain(m) }

// An application's service account lists its event types and pushes their
// schemas through the plain event-type endpoints (the SDK's schema sync does
// exactly this). It holds the application-service permissions, not the
// messaging ones, and is confined to the applications it is bound to.
func TestApplicationServiceAccount_EventTypes_ConfinedToOwnApplication(t *testing.T) {
	pool := testpg.Pool(t)
	apps := application.NewRepository(pool)
	repo := eventtype.NewRepository(pool)
	uow := testpg.NewUoW(t)

	mine, err := usecaseop.Run(testpg.AnchorCtx(), uow, appops.CreateApplication(apps),
		appops.CreateCommand{Code: "etscope-mine", Name: "Mine"}, testpg.TestEC())
	require.NoError(t, err)
	_, err = usecaseop.Run(testpg.AnchorCtx(), uow, appops.CreateApplication(apps),
		appops.CreateCommand{Code: "etscope-other", Name: "Other"}, testpg.TestEC())
	require.NoError(t, err)

	own, err := usecaseop.Run(testpg.AnchorCtx(), uow, operations.CreateEventType(repo),
		operations.CreateCommand{Code: "etscope-mine:orders:order:created", Name: "Order created"}, testpg.TestEC())
	require.NoError(t, err)
	foreign, err := usecaseop.Run(testpg.AnchorCtx(), uow, operations.CreateEventType(repo),
		operations.CreateCommand{Code: "etscope-other:orders:order:created", Name: "Order created"}, testpg.TestEC())
	require.NoError(t, err)

	s := &State{Repo: repo, UoW: uow, Apps: apps}
	svc := auth.WithContext(context.Background(), &auth.AuthContext{
		PrincipalID: "prn_etscope_svc", Scope: auth.ScopeAnchor,
		Applications: []string{mine.ApplicationID},
		Permissions: []string{
			"platform:application-service:event-type:view",
			"platform:application-service:event-type:create",
			"platform:application-service:event-type:update",
		},
	})
	schema := json.RawMessage(`{"type":"object"}`)

	t.Run("list shows only its own application's event types", func(t *testing.T) {
		out, err := s.list(svc, &listInput{Status: "CURRENT"})
		require.NoError(t, err)
		codes := make([]string, 0, len(out.Body.Items))
		for _, it := range out.Body.Items {
			codes = append(codes, it.Code)
		}
		assert.Contains(t, codes, "etscope-mine:orders:order:created")
		assert.NotContains(t, codes, "etscope-other:orders:order:created")
		for _, c := range codes {
			assert.Truef(t, len(c) > 13 && c[:13] == "etscope-mine:", "unexpected event type %q in a confined list", c)
		}
	})

	t.Run("list filtered to another application is empty", func(t *testing.T) {
		out, err := s.list(svc, &listInput{Application: "etscope-other"})
		require.NoError(t, err)
		assert.Empty(t, out.Body.Items)
	})

	t.Run("reads its own event type by id and by code", func(t *testing.T) {
		_, err := s.getByID(svc, &getByIDInput{ID: own.EventTypeID})
		require.NoError(t, err)
		_, err = s.getByCode(svc, &getByCodeInput{Code: "etscope-mine:orders:order:created"})
		require.NoError(t, err)
	})

	t.Run("is refused another application's event type", func(t *testing.T) {
		_, err := s.getByID(svc, &getByIDInput{ID: foreign.EventTypeID})
		require.Error(t, err)
		_, err = s.getByCode(svc, &getByCodeInput{Code: "etscope-other:orders:order:created"})
		require.Error(t, err)
	})

	t.Run("adds a schema version to its own event type", func(t *testing.T) {
		out, err := s.addSchema(svc, &addSchemaInput{ID: own.EventTypeID, Body: AddSchemaRequest{Version: "1.0.0", Schema: schema}})
		require.NoError(t, err)
		assert.NotEmpty(t, out.Body.SpecVersions)
	})

	t.Run("cannot add a schema version to another application's event type", func(t *testing.T) {
		_, err := s.addSchema(svc, &addSchemaInput{ID: foreign.EventTypeID, Body: AddSchemaRequest{Version: "1.0.0", Schema: schema}})
		require.Error(t, err)
		after, err := repo.FindByID(context.Background(), foreign.EventTypeID)
		require.NoError(t, err)
		assert.Empty(t, after.SpecVersions, "a refused schema push must not be stored")
	})

	t.Run("a view-only service account cannot add a schema", func(t *testing.T) {
		viewOnly := auth.WithContext(context.Background(), &auth.AuthContext{
			PrincipalID: "prn_etscope_ro", Scope: auth.ScopeAnchor,
			Applications: []string{mine.ApplicationID},
			Permissions:  []string{"platform:application-service:event-type:view"},
		})
		_, err := s.addSchema(viewOnly, &addSchemaInput{ID: own.EventTypeID, Body: AddSchemaRequest{Version: "1.1.0", Schema: schema}})
		require.Error(t, err)
	})

	t.Run("the messaging permissions still read and write any application", func(t *testing.T) {
		admin := auth.WithContext(context.Background(), &auth.AuthContext{
			PrincipalID: "prn_etscope_admin", Scope: auth.ScopeAnchor,
			Permissions: []string{"platform:*:*:*"},
		})
		_, err := s.getByID(admin, &getByIDInput{ID: foreign.EventTypeID})
		require.NoError(t, err)
		_, err = s.addSchema(admin, &addSchemaInput{ID: foreign.EventTypeID, Body: AddSchemaRequest{Version: "1.0.0", Schema: schema}})
		require.NoError(t, err)
	})
}
