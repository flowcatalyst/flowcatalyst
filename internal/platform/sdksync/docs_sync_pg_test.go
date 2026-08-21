//go:build integration

package sdksync

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/appdocs"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	appops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/application/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecaseop"
)

// TestSyncAppDocs_ReplaceAndRead pins the docs sync round trip: a sync
// creates the set in payload order, a second sync replaces it (unlisted
// pages are removed, kept slugs update in place), and the docsapi read
// surface serves the grouped list + page content to an authorised admin.
func TestSyncAppDocs_ReplaceAndRead(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	uow := testpg.NewUoW(t)

	appCode := "docsyncapp"
	_, err := usecaseop.Run(ctx, uow, appops.CreateApplication(application.NewRepository(pool)),
		appops.CreateCommand{Code: appCode, Name: "Doc Sync App"}, testpg.TestEC())
	require.NoError(t, err)

	s := &State{
		Apps:    application.NewRepository(pool),
		AppDocs: appdocs.NewRepository(pool),
		UoW:     uow,
	}
	authCtx := auth.WithContext(ctx, &auth.AuthContext{
		PrincipalID: "p_docsync", Scope: auth.ScopeAnchor, AllApplications: true,
	})

	title := "Getting Started"
	out, err := s.syncAppDocs(authCtx, &syncDocsInput{AppCode: appCode, Body: syncDocsRequest{Docs: []syncDocInputRequest{
		{Slug: "getting-started", Title: &title, Content: "# Getting Started\nHello."},
		{Slug: "webhooks", Content: "# Webhook Reference\nDetails."},
	}}})
	require.NoError(t, err)
	assert.EqualValues(t, 2, out.Body.Created)
	assert.Equal(t, []string{"getting-started", "webhooks"}, out.Body.SyncedCodes)

	// Replace: keep one (updated), drop one, add one.
	out, err = s.syncAppDocs(authCtx, &syncDocsInput{AppCode: appCode, Body: syncDocsRequest{Docs: []syncDocInputRequest{
		{Slug: "webhooks", Content: "# Webhook Reference v2\nMore."},
		{Slug: "faq", Content: "# FAQ\nQ&A."},
	}}})
	require.NoError(t, err)
	assert.EqualValues(t, 1, out.Body.Created)
	assert.EqualValues(t, 1, out.Body.Updated)
	assert.EqualValues(t, 1, out.Body.Deleted)

	// Read side through the store: replaced set, in payload order, titles
	// re-derived on update. (The admin read API over this store is covered
	// in docsapi's own integration test.)
	repo := appdocs.NewRepository(pool)
	appRow, err := s.Apps.FindByCode(ctx, appCode)
	require.NoError(t, err)
	require.NotNil(t, appRow)
	summaries, err := repo.ListByApplication(ctx, appRow.ID)
	require.NoError(t, err)
	require.Len(t, summaries, 2)
	assert.Equal(t, "webhooks", summaries[0].Slug, "sync order preserved")
	assert.Equal(t, "Webhook Reference v2", summaries[0].Title, "title re-derived on update")
	faq, err := repo.GetByApplicationSlug(ctx, appRow.ID, "faq")
	require.NoError(t, err)
	require.NotNil(t, faq)
	assert.Contains(t, faq.Content, "Q&A")

	// Bad slugs refuse before touching the store.
	_, err = s.syncAppDocs(authCtx, &syncDocsInput{AppCode: appCode, Body: syncDocsRequest{Docs: []syncDocInputRequest{
		{Slug: "Not A Slug", Content: "x"},
	}}})
	require.Error(t, err)
}
