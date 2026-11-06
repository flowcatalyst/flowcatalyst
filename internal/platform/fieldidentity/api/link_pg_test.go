//go:build integration

package api

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/fieldidentity"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apicommon"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httpcompat"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/flowcatalyst-go/internal/tsid"
	"github.com/flowcatalyst/go-sdk/usecase"
)

// The optional link from a field worker to a platform user
// (docs/field-native-session-spec.md section 5): who can be linked, one worker
// per user per client, the same user in another client, replace and unlink,
// the events, the response and the permission and client box.

// principalRow inserts a platform principal and returns its id and email.
// scope is ANCHOR, PARTNER or CLIENT (home is its home client, "" for none).
func principalRow(t *testing.T, typ, scope, home string, active bool) (id, email string) {
	t.Helper()
	id = tsid.Generate(tsid.Principal)
	email = fmt.Sprintf("%s@link.example.test", id)
	var homeArg any
	if home != "" {
		homeArg = home
	}
	_, err := testpg.Pool(t).Exec(context.Background(),
		`INSERT INTO iam_principals (id, type, scope, client_id, name, active, email, email_domain)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, 'link.example.test')`,
		id, typ, scope, homeArg, "Link "+id, active, email)
	require.NoError(t, err)
	return id, email
}

func grantClient(t *testing.T, principalID, clientID string) {
	t.Helper()
	_, err := testpg.Pool(t).Exec(context.Background(),
		`INSERT INTO iam_client_access_grants (id, principal_id, client_id, granted_by) VALUES ($1, $2, $3, 'prn_optestrunner1')`,
		tsid.GenerateUntyped(), principalID, clientID)
	require.NoError(t, err)
}

func (f *fixture) link(ctx context.Context, workerID, principalID string) (*apicommon.Out[FieldWorkerResponse], error) {
	return f.s.linkWorker(ctx, &linkWorkerInput{ID: workerID, Body: LinkFieldWorkerRequest{PrincipalID: principalID}})
}

// requireStatus asserts err is an API error with the status and error code.
func requireStatus(t *testing.T, err error, status int, code string) {
	t.Helper()
	var em *httpcompat.ErrorModel
	require.ErrorAs(t, err, &em, "expected an API error, got %v", err)
	assert.Equal(t, status, em.GetStatus(), "status (err: %v)", err)
	assert.Equal(t, code, em.Code)
}

func TestLinkWorkerHappyPath(t *testing.T) {
	f := newFixture(t, hasher(t))
	w := f.ensure(f.actx, "LK-1", "Linker", f.codeA).Body
	assert.Nil(t, w.LinkedPrincipal, "a new worker is not linked")

	pid, email := principalRow(t, "USER", "CLIENT", f.clientID, true)
	out, err := f.link(f.actx, w.ID, pid)
	require.NoError(t, err)
	require.NotNil(t, out.Body.LinkedPrincipal)
	assert.Equal(t, FieldLinkedPrincipalRef{ID: pid, Name: "Link " + pid, Email: email}, *out.Body.LinkedPrincipal)
	assert.Equal(t, 1, f.eventCount(fieldidentity.WorkerLinkedType, w.ID), "linked event")

	// The link is stored, read back by get and found by the list filter.
	got, err := f.s.getWorker(f.actx, &apicommon.IDInput{ID: w.ID})
	require.NoError(t, err)
	require.NotNil(t, got.Body.LinkedPrincipal)
	assert.Equal(t, pid, got.Body.LinkedPrincipal.ID)
	list, err := f.s.listWorkers(f.actx, &listWorkersInput{ClientID: f.clientID, LinkedPrincipalID: pid})
	require.NoError(t, err)
	require.Len(t, list.Body.Workers, 1)
	assert.Equal(t, w.ID, list.Body.Workers[0].ID)
	require.NotNil(t, list.Body.Workers[0].LinkedPrincipal)
	assert.Equal(t, email, list.Body.Workers[0].LinkedPrincipal.Email)
	other, err := f.s.listWorkers(f.actx, &listWorkersInput{ClientID: f.clientID, LinkedPrincipalID: "prn_nobodyhere001"})
	require.NoError(t, err)
	assert.Empty(t, other.Body.Workers)

	// Linking again to the same user is a no-op; a different user replaces it.
	_, err = f.link(f.actx, w.ID, pid)
	require.NoError(t, err)
	pid2, _ := principalRow(t, "USER", "CLIENT", f.clientID, true)
	out, err = f.link(f.actx, w.ID, pid2)
	require.NoError(t, err)
	assert.Equal(t, pid2, out.Body.LinkedPrincipal.ID)

	// Unlink clears it; unlinking again is fine; the events say who was unlinked.
	un, err := f.s.unlinkWorker(f.actx, &apicommon.IDInput{ID: w.ID})
	require.NoError(t, err)
	assert.Nil(t, un.Body.LinkedPrincipal)
	_, err = f.s.unlinkWorker(f.actx, &apicommon.IDInput{ID: w.ID})
	require.NoError(t, err)
	assert.Equal(t, 2, f.eventCount(fieldidentity.WorkerUnlinkedType, w.ID))
	var previous string
	require.NoError(t, testpg.Pool(t).QueryRow(context.Background(),
		`SELECT data::jsonb->>'principalId' FROM msg_events WHERE type = $1 AND subject LIKE '%' || $2 ORDER BY created_at LIMIT 1`,
		fieldidentity.WorkerUnlinkedType, w.ID).Scan(&previous))
	assert.Equal(t, pid2, previous)
}

func TestLinkWorkerRefusesUnlinkablePrincipals(t *testing.T) {
	f := newFixture(t, hasher(t))
	w := f.ensure(f.actx, "LK-2", "Linker", f.codeA).Body
	otherClient := newFixture(t, hasher(t)).clientID

	service, _ := principalRow(t, "SERVICE", "CLIENT", f.clientID, true)
	inactive, _ := principalRow(t, "USER", "CLIENT", f.clientID, false)
	elsewhere, _ := principalRow(t, "USER", "CLIENT", otherClient, true)
	partnerNoGrant, _ := principalRow(t, "USER", "PARTNER", "", true)
	for name, pid := range map[string]string{
		"a service account":                      service,
		"an inactive user":                       inactive,
		"a user of another client":               elsewhere,
		"a partner without access to the client": partnerNoGrant,
		"an unknown principal":                   "prn_doesnotexist1",
	} {
		_, err := f.link(f.actx, w.ID, pid)
		requireStatus(t, err, http.StatusUnprocessableEntity, fieldidentity.CodeInvalidLink)
		got, gerr := f.s.getWorker(f.actx, &apicommon.IDInput{ID: w.ID})
		require.NoError(t, gerr)
		assert.Nil(t, got.Body.LinkedPrincipal, "%s was linked", name)
	}
	assert.Zero(t, f.eventCount(fieldidentity.WorkerLinkedType, w.ID), "a refused link emits nothing")

	// A blank principal id is a plain validation error.
	_, err := f.link(f.actx, w.ID, "  ")
	requireErr(t, err, usecase.KindValidation, "PRINCIPAL_ID_REQUIRED")

	// What can be linked: the client's own user, an anchor, a partner granted the client.
	own, _ := principalRow(t, "USER", "CLIENT", f.clientID, true)
	anchor, _ := principalRow(t, "USER", "ANCHOR", "", true)
	partner, _ := principalRow(t, "USER", "PARTNER", "", true)
	grantClient(t, partner, f.clientID)
	for i, pid := range []string{own, anchor, partner} {
		_, err := f.link(f.actx, w.ID, pid)
		require.NoError(t, err, "linkable principal %d", i)
	}
}

func TestLinkWorkerOneWorkerPerUserPerClient(t *testing.T) {
	f := newFixture(t, hasher(t))
	a := f.ensure(f.actx, "LK-A", "A", f.codeA).Body
	b := f.ensure(f.actx, "LK-B", "B", f.codeA).Body
	anchor, _ := principalRow(t, "USER", "ANCHOR", "", true)

	_, err := f.link(f.actx, a.ID, anchor)
	require.NoError(t, err)
	_, err = f.link(f.actx, b.ID, anchor)
	requireErr(t, err, usecase.KindConflict, "LINK_EXISTS")
	got, err := f.s.getWorker(f.actx, &apicommon.IDInput{ID: b.ID})
	require.NoError(t, err)
	assert.Nil(t, got.Body.LinkedPrincipal)

	// The same user is linkable in another client.
	f2 := newFixture(t, hasher(t))
	c := f2.ensure(f2.actx, "LK-C", "C", f2.codeA).Body
	_, err = f2.link(f2.actx, c.ID, anchor)
	require.NoError(t, err)

	// Unlinking frees the user in the first client.
	_, err = f.s.unlinkWorker(f.actx, &apicommon.IDInput{ID: a.ID})
	require.NoError(t, err)
	_, err = f.link(f.actx, b.ID, anchor)
	require.NoError(t, err)
}

func TestLinkWorkerPermissionsAndClientBox(t *testing.T) {
	f := newFixture(t, hasher(t))
	w := f.ensure(f.actx, "LK-P", "P", f.codeA).Body
	pid, _ := principalRow(t, "USER", "CLIENT", f.clientID, true)

	// View alone may read the link but not change it.
	viewer := userCtx(f.clientID, "platform:iam:field-worker:view")
	_, err := f.link(viewer, w.ID, pid)
	requireErr(t, err, usecase.KindAuthorization, "PERMISSION_REQUIRED")
	_, err = f.s.unlinkWorker(viewer, &apicommon.IDInput{ID: w.ID})
	requireErr(t, err, usecase.KindAuthorization, "PERMISSION_REQUIRED")

	// Manage works, in the worker's client only.
	manager := userCtx(f.clientID, permManage)
	_, err = f.link(manager, w.ID, pid)
	require.NoError(t, err)
	got, err := f.s.getWorker(userCtx(f.clientID, "platform:iam:field-worker:view"), &apicommon.IDInput{ID: w.ID})
	require.NoError(t, err)
	require.NotNil(t, got.Body.LinkedPrincipal)
	outsider := userCtx(newFixture(t, hasher(t)).clientID, permManage)
	_, err = f.link(outsider, w.ID, pid)
	requireErr(t, err, usecase.KindNotFound, "FieldWorker_NOT_FOUND")
	_, err = f.s.unlinkWorker(outsider, &apicommon.IDInput{ID: w.ID})
	requireErr(t, err, usecase.KindNotFound, "FieldWorker_NOT_FOUND")
	still, err := f.s.getWorker(f.actx, &apicommon.IDInput{ID: w.ID})
	require.NoError(t, err)
	require.NotNil(t, still.Body.LinkedPrincipal, "a refused unlink left the link")

	// An application-confined caller is limited to workers of its applications.
	confined := callerCtx([]string{f.clientID}, []string{permManage}, []string{f.appB}, false)
	_, err = f.link(confined, w.ID, pid)
	requireErr(t, err, usecase.KindNotFound, "FieldWorker_NOT_FOUND")
	_, err = f.link(f.actx, "fwu_doesnotexist01", pid)
	requireErr(t, err, usecase.KindNotFound, "FieldWorker_NOT_FOUND")
}

func TestLinkSurvivesPrincipalDeletionAndShowsUnresolvable(t *testing.T) {
	f := newFixture(t, hasher(t))
	w := f.ensure(f.actx, "LK-D", "D", f.codeA).Body
	pid, _ := principalRow(t, "USER", "CLIENT", f.clientID, true)
	_, err := f.link(f.actx, w.ID, pid)
	require.NoError(t, err)

	_, err = testpg.Pool(t).Exec(context.Background(), `DELETE FROM iam_principals WHERE id = $1`, pid)
	require.NoError(t, err)
	got, err := f.s.getWorker(f.actx, &apicommon.IDInput{ID: w.ID})
	require.NoError(t, err)
	require.NotNil(t, got.Body.LinkedPrincipal, "no foreign key: the link stays")
	assert.Equal(t, FieldLinkedPrincipalRef{ID: pid}, *got.Body.LinkedPrincipal, "id only, no name or email")
}
