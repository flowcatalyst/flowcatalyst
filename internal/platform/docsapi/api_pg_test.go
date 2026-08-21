//go:build integration

package docsapi

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/appdocs"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	appops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/application/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apicommon"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecaseop"
)

func TestMain(m *testing.M) { testpg.RunMain(m) }

// The grouped list serves synced application pages next to the published
// platform set, and the app get endpoint returns the stored Markdown.
func TestGroupedListWithApplicationDocs(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	uow := testpg.NewUoW(t)

	appCode := "docsapiapp"
	_, err := usecaseop.Run(ctx, uow, appops.CreateApplication(application.NewRepository(pool)),
		appops.CreateCommand{Code: appCode, Name: "Docs API App"}, testpg.TestEC())
	require.NoError(t, err)
	apps := application.NewRepository(pool)
	appRow, err := apps.FindByCode(ctx, appCode)
	require.NoError(t, err)
	require.NotNil(t, appRow)

	repo := appdocs.NewRepository(pool)
	_, err = repo.ReplaceForApplication(ctx, appRow.ID, []appdocs.Input{
		{Slug: "guide", Title: "Integration Guide", Content: "# Integration Guide\nBody."},
	})
	require.NoError(t, err)

	s := &State{AppDocs: repo, Apps: apps}
	authCtx := auth.WithContext(ctx, &auth.AuthContext{
		PrincipalID: "prn_docsapipg", Scope: auth.ScopeAnchor,
	})

	list, err := s.list(authCtx, &apicommon.Empty{})
	require.NoError(t, err)
	assert.NotEmpty(t, list.Body.Platform, "published platform pages present")
	var found bool
	for _, g := range list.Body.Applications {
		if g.ApplicationCode == appCode {
			found = true
			require.Len(t, g.Docs, 1)
			assert.Equal(t, "Integration Guide", g.Docs[0].Title)
		}
	}
	require.True(t, found, "synced app must appear in the grouped list")

	doc, err := s.getApplication(authCtx, &getApplicationDocInput{AppCode: appCode, Slug: "guide"})
	require.NoError(t, err)
	assert.Contains(t, doc.Body.Content, "Body.")

	_, err = s.getApplication(authCtx, &getApplicationDocInput{AppCode: appCode, Slug: "missing"})
	require.Error(t, err)
	_, err = s.getApplication(authCtx, &getApplicationDocInput{AppCode: "no-such-app", Slug: "guide"})
	require.Error(t, err)
}
