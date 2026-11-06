//go:build integration

package api

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/fieldidentity"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apicommon"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/go-sdk/usecase"
	"github.com/flowcatalyst/go-sdk/usecaseop"
)

func runBulk(f *fixture, ctx context.Context, ids, tokens []string) ([]fieldidentity.IssuedBadge, error) {
	return usecaseop.RunTx(ctx, f.s.UoW, f.s.Ops.IssueBadges(),
		fieldidentity.IssueBadgesCommand{WorkerIDs: ids, Tokens: tokens}, auth.NewExecutionContext(ctx))
}

func (f *fixture) count(sql string, args ...any) int {
	f.t.Helper()
	var n int
	require.NoError(f.t, testpg.Pool(f.t).QueryRow(context.Background(), sql, args...).Scan(&n))
	return n
}

func TestIssueBadgeShownOnceAndSupersedesThePrevious(t *testing.T) {
	f := newFixture(t, hasher(t))
	w := f.ensure(f.actx, "B-1", "Badge Worker", f.codeA).Body

	first, err := f.s.issueBadge(f.actx, &issueBadgeInput{ID: w.ID, Body: IssueFieldBadgeRequest{}})
	require.NoError(t, err)
	tok := first.Body.Token
	assert.True(t, strings.HasPrefix(tok, "FWB1"))
	assert.Len(t, tok, 30)
	assert.Equal(t, "ACTIVE", first.Body.Badge.Status)
	assert.Equal(t, w.ID, first.Body.Badge.WorkerID)
	assert.Equal(t, 1, f.count(`SELECT COUNT(*) FROM field_badges WHERE token_hash = $1`, fieldidentity.HashToken(tok)))
	assert.Equal(t, 0, f.count(`SELECT COUNT(*) FROM field_badges WHERE token_hash = $1`, tok))
	// The token is in no event and no audit row.
	assert.Equal(t, 0, f.count(`SELECT (SELECT COUNT(*) FROM msg_events WHERE data::text LIKE '%' || $1 || '%')
		+ (SELECT COUNT(*) FROM aud_logs WHERE operation_json::text LIKE '%' || $1 || '%')`, tok))

	second, err := f.s.issueBadge(f.actx, &issueBadgeInput{ID: w.ID, Body: IssueFieldBadgeRequest{Label: new("Reprint")}})
	require.NoError(t, err)
	assert.NotEqual(t, tok, second.Body.Token)

	list, err := f.s.listBadges(f.actx, &apicommon.IDInput{ID: w.ID})
	require.NoError(t, err)
	require.Len(t, list.Body.Badges, 2)
	assert.Equal(t, second.Body.Badge.ID, list.Body.Badges[0].ID, "newest first")
	assert.Equal(t, "ACTIVE", list.Body.Badges[0].Status)
	assert.Equal(t, "REVOKED", list.Body.Badges[1].Status, "the earlier badge was revoked in the same transaction")
	assert.NotNil(t, list.Body.Badges[1].RevokedAt)
	assert.Equal(t, 2, f.count(`SELECT COUNT(*) FROM msg_events WHERE type = $1 AND data::text LIKE '%' || $2 || '%'`, fieldidentity.WorkerBadgeIssuedType, w.ID))
	assert.Equal(t, 1, f.count(`SELECT COUNT(*) FROM msg_events WHERE type = $1 AND data::jsonb->'revokedBadgeIds' ? $2`,
		fieldidentity.WorkerBadgeIssuedType, first.Body.Badge.ID))

	// The worker shows its active badge.
	got, err := f.s.getWorker(f.actx, &apicommon.IDInput{ID: w.ID})
	require.NoError(t, err)
	require.NotNil(t, got.Body.Badge)
	assert.Equal(t, second.Body.Badge.ID, got.Body.Badge.ID)

	// Revoke: terminal, idempotent, one event.
	rv, err := f.s.revokeBadge(f.actx, &revokeBadgeInput{ID: w.ID, BadgeID: second.Body.Badge.ID})
	require.NoError(t, err)
	assert.Equal(t, "REVOKED", rv.Body.Status)
	_, err = f.s.revokeBadge(f.actx, &revokeBadgeInput{ID: w.ID, BadgeID: second.Body.Badge.ID})
	require.NoError(t, err)
	assert.Equal(t, 1, f.count(`SELECT COUNT(*) FROM msg_events WHERE type = $1 AND data::text LIKE '%' || $2 || '%'`, fieldidentity.WorkerBadgeRevokedType, w.ID))
	got, err = f.s.getWorker(f.actx, &apicommon.IDInput{ID: w.ID})
	require.NoError(t, err)
	assert.Nil(t, got.Body.Badge)
	// A badge of another worker is not found under this one.
	w2 := f.ensure(f.actx, "B-2", "Other", f.codeA).Body
	_, err = f.s.revokeBadge(f.actx, &revokeBadgeInput{ID: w2.ID, BadgeID: first.Body.Badge.ID})
	requireErr(t, err, usecase.KindNotFound, "FieldBadge_NOT_FOUND")

	// Deleting the worker deletes its badges.
	_, err = f.s.deleteWorker(f.actx, &apicommon.IDInput{ID: w.ID})
	require.NoError(t, err)
	assert.Equal(t, 0, f.count(`SELECT COUNT(*) FROM field_badges WHERE identity_id = $1`, w.ID))
}

func TestBulkIssueBadgesIsAllOrNothing(t *testing.T) {
	f := newFixture(t, hasher(t))
	other := newFixture(t, hasher(t))
	a := f.ensure(f.actx, "BK-1", "A", f.codeA).Body
	b := f.ensure(f.actx, "BK-2", "B", f.codeA).Body
	theirs := other.ensure(other.actx, "BK-3", "C", other.codeA).Body

	// A client administrator of f's client only.
	cctx := callerCtx([]string{f.clientID}, allFieldPerms, nil, true)

	// A foreign worker anywhere in the batch: 404, and nothing at all is issued.
	_, err := f.s.issueBadges(cctx, &apicommon.In[BulkIssueFieldBadgesRequest]{Body: BulkIssueFieldBadgesRequest{WorkerIDs: []string{a.ID, theirs.ID}}})
	requireErr(t, err, usecase.KindNotFound, "FieldWorker_NOT_FOUND")
	_, err = f.s.issueBadges(cctx, &apicommon.In[BulkIssueFieldBadgesRequest]{Body: BulkIssueFieldBadgesRequest{WorkerIDs: []string{a.ID, "fwu_doesnotexist"}}})
	requireErr(t, err, usecase.KindNotFound, "FieldWorker_NOT_FOUND")
	assert.Equal(t, 0, f.count(`SELECT COUNT(*) FROM field_badges WHERE identity_id = ANY($1)`, []string{a.ID, b.ID, theirs.ID}))
	// The use case boxes on its own too: the transaction rolls back whole.
	tokens := []string{"FWB1AAAAAAAAAAAAAAAAAAAAAAAAAA", "FWB1BBBBBBBBBBBBBBBBBBBBBBBBBB"}
	_, err = runBulk(f, cctx, []string{a.ID, theirs.ID}, tokens)
	requireErr(t, err, usecase.KindAuthorization, "SCOPE_FORBIDDEN")
	assert.Equal(t, 0, f.count(`SELECT COUNT(*) FROM field_badges WHERE identity_id = ANY($1)`, []string{a.ID, b.ID, theirs.ID}))
	// Shape.
	_, err = f.s.issueBadges(cctx, &apicommon.In[BulkIssueFieldBadgesRequest]{Body: BulkIssueFieldBadgesRequest{}})
	requireErr(t, err, usecase.KindValidation, "WORKER_IDS_INVALID")
	_, err = f.s.issueBadges(cctx, &apicommon.In[BulkIssueFieldBadgesRequest]{Body: BulkIssueFieldBadgesRequest{WorkerIDs: []string{a.ID, a.ID}}})
	requireErr(t, err, usecase.KindValidation, "WORKER_IDS_INVALID")

	// All mine: one badge each, tokens distinct, one event per worker.
	ok, err := f.s.issueBadges(cctx, &apicommon.In[BulkIssueFieldBadgesRequest]{Body: BulkIssueFieldBadgesRequest{WorkerIDs: []string{a.ID, b.ID}}})
	require.NoError(t, err)
	require.Len(t, ok.Body.Issued, 2)
	assert.Equal(t, "BK-1", ok.Body.Issued[0].WorkerCode)
	assert.Equal(t, "B", ok.Body.Issued[1].Name)
	assert.NotEqual(t, ok.Body.Issued[0].Token, ok.Body.Issued[1].Token)
	for _, it := range ok.Body.Issued {
		assert.Equal(t, 1, f.count(`SELECT COUNT(*) FROM field_badges WHERE identity_id = $1 AND status = 'ACTIVE' AND token_hash = $2`,
			it.WorkerID, fieldidentity.HashToken(it.Token)))
		assert.Equal(t, 1, f.count(`SELECT COUNT(*) FROM msg_events WHERE type = $1 AND data::text LIKE '%' || $2 || '%'`, fieldidentity.WorkerBadgeIssuedType, it.WorkerID))
	}
	// Badge management needs the manage permission.
	view := callerCtx([]string{f.clientID}, []string{"platform:iam:field-worker:view"}, nil, true)
	_, err = f.s.issueBadge(view, &issueBadgeInput{ID: a.ID})
	requireErr(t, err, usecase.KindAuthorization, "PERMISSION_REQUIRED")
	_, err = f.s.issueBadges(view, &apicommon.In[BulkIssueFieldBadgesRequest]{Body: BulkIssueFieldBadgesRequest{WorkerIDs: []string{a.ID}}})
	requireErr(t, err, usecase.KindAuthorization, "PERMISSION_REQUIRED")
}
