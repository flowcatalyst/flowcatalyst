//go:build integration

// Package lastadmin_test pins the last all-applications-administrator guard.
// It owns its own embedded Postgres (testpg.RunMain is per package) because
// the rule is a global count: the tests reset the set of administrators to a
// known state, which they could not do in a package whose other tests create
// anchor users concurrently. Tests here are deliberately NOT parallel.
package lastadmin_test

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/ids"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/client"
	clientops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/client/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/role"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecaseop"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecasepgx"
)

func TestMain(m *testing.M) { testpg.RunMain(m) }

type fx struct {
	repo  *principal.Repository
	apps  *application.Repository
	uow   *usecasepgx.UnitOfWork
	seq   int
	label string
}

// reset demotes every existing administrator (raw SQL, bypassing the guard) so
// the test starts with zero, then returns a fixture.
func reset(t *testing.T) *fx {
	t.Helper()
	pool := testpg.Pool(t)
	_, err := pool.Exec(context.Background(),
		`UPDATE iam_principals SET all_applications = FALSE
		  WHERE type = 'USER' AND scope = 'ANCHOR' AND all_applications`)
	require.NoError(t, err)
	return &fx{
		repo:  principal.NewRepository(pool),
		apps:  application.NewRepository(pool),
		uow:   testpg.NewUoW(t),
		label: nextLabel(),
	}
}

// admin seeds an active ANCHOR USER with all_applications = true through the
// public create operation.
func (f *fx) admin(t *testing.T, name string) ids.PrincipalID {
	t.Helper()
	f.seq++
	ev, err := usecaseop.Run(testpg.AnchorCtx(), f.uow, operations.CreateUser(f.repo),
		operations.CreateCommand{Email: name + f.label + "@lastadmin.example", Scope: "ANCHOR"}, testpg.TestEC())
	require.NoError(t, err)
	got, err := f.repo.FindByID(context.Background(), ev.UserID)
	require.NoError(t, err)
	require.True(t, got.IsAllApplicationsAdmin(), "seeded admin must satisfy the rule")
	return ev.UserID
}

func (f *fx) assign(allApps bool, id ids.PrincipalID) error {
	_, err := usecaseop.Run(testpg.AnchorCtx(), f.uow, operations.AssignApplicationAccess(f.repo, f.apps),
		operations.AssignApplicationAccessCommand{UserID: id, ApplicationIDs: []string{}, AllApplications: &allApps}, testpg.TestEC())
	return err
}

func requireLastAdmin(t *testing.T, err error) {
	t.Helper()
	testpg.RequireUsecaseError(t, err, usecase.KindBusinessRule, principal.CodeLastAllApplicationsAdmin)
	assert.Contains(t, usecase.AsError(err).Message, "Grant all-applications access to another")
}

func (f *fx) stillAdmin(t *testing.T, id ids.PrincipalID) {
	t.Helper()
	got, err := f.repo.FindByID(context.Background(), id)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.True(t, got.IsAllApplicationsAdmin(), "refused write must leave the admin intact")
}

func run[C any, E usecase.DomainEvent](f *fx, op usecaseop.Operation[C, E], cmd C) error {
	_, err := usecaseop.Run(testpg.AnchorCtx(), f.uow, op, cmd, testpg.TestEC())
	return err
}

func TestAssign_LastAdminRefused_ThenAllowedWithAnotherAdmin(t *testing.T) {
	f := reset(t)
	a := f.admin(t, "a")
	requireLastAdmin(t, f.assign(false, a))
	f.stillAdmin(t, a)

	b := f.admin(t, "b")
	require.NoError(t, f.assign(false, a), "another active admin exists")
	// b is now the last one.
	requireLastAdmin(t, f.assign(false, b))
	f.stillAdmin(t, b)
}

func TestAssign_NilFlagAndGrantAreUnaffected(t *testing.T) {
	f := reset(t)
	a := f.admin(t, "a")
	_, err := usecaseop.Run(testpg.AnchorCtx(), f.uow, operations.AssignApplicationAccess(f.repo, f.apps),
		operations.AssignApplicationAccessCommand{UserID: a, ApplicationIDs: []string{}}, testpg.TestEC())
	require.NoError(t, err, "nil flag leaves the admin intact")
	require.NoError(t, f.assign(true, a))
}

func TestServicePrincipalDoesNotCount(t *testing.T) {
	f := reset(t)
	a := f.admin(t, "a")
	// An anchor SERVICE principal with all_applications = TRUE (the default).
	svc := principal.NewService("sa_la"+f.label, "svc")
	tx, err := testpg.Pool(t).Begin(context.Background())
	require.NoError(t, err)
	require.NoError(t, f.repo.Persist(context.Background(), svc, usecasepgx.WrapTxForBootstrap(tx)))
	require.NoError(t, tx.Commit(context.Background()))
	require.True(t, svc.AllApplications && svc.Scope == principal.ScopeAnchor && !svc.IsAllApplicationsAdmin())

	requireLastAdmin(t, f.assign(false, a))
}

func TestInactiveAdminDoesNotCount(t *testing.T) {
	f := reset(t)
	a := f.admin(t, "a")
	b := f.admin(t, "b")
	// Deactivate b (allowed: a remains), then a is the last ACTIVE admin even
	// though b still carries all_applications = true.
	require.NoError(t, run(f, operations.DeactivateUser(f.repo), operations.DeactivateCommand{ID: b}))
	requireLastAdmin(t, f.assign(false, a))
}

func TestDeactivate_Update_Delete_Refused_ForLastAdmin(t *testing.T) {
	f := reset(t)
	a := f.admin(t, "a")

	requireLastAdmin(t, run(f, operations.DeactivateUser(f.repo), operations.DeactivateCommand{ID: a}))
	off := false
	requireLastAdmin(t, run(f, operations.UpdateUser(f.repo), operations.UpdateCommand{ID: a, Active: &off}))
	requireLastAdmin(t, run(f, operations.DeleteUser(f.repo), operations.DeleteCommand{ID: a}))
	f.stillAdmin(t, a)

	// Harmless update (name only) is fine.
	name := "Renamed"
	require.NoError(t, run(f, operations.UpdateUser(f.repo), operations.UpdateCommand{ID: a, Name: &name}))

	// With a second admin all three succeed.
	b := f.admin(t, "b")
	require.NoError(t, run(f, operations.DeactivateUser(f.repo), operations.DeactivateCommand{ID: a}))
	require.NoError(t, run(f, operations.ActivateUser(f.repo), operations.ActivateCommand{ID: a}))
	require.NoError(t, run(f, operations.DeleteUser(f.repo), operations.DeleteCommand{ID: a}))
	f.stillAdmin(t, b)
}

func TestScopeChange_Refused_ForLastAdmin(t *testing.T) {
	f := reset(t)
	a := f.admin(t, "a")
	clients := client.NewRepository(testpg.Pool(t))
	grants := principal.NewClientAccessGrantRepo(testpg.Pool(t))
	// TO_PARTNER leaves ANCHOR -> refused before the client lookup matters? The
	// client check runs first, so use a real client.
	cid := mustClient(t, f)
	err := run(f, operations.SetClientAssociation(f.repo, clients, grants),
		operations.SetClientAssociationCommand{UserID: a, ClientID: cid, Mode: operations.ModeChangeClient})
	requireLastAdmin(t, err)
	err = run(f, operations.SetClientAssociation(f.repo, clients, grants),
		operations.SetClientAssociationCommand{UserID: a, ClientID: cid, Mode: operations.ModeToPartner})
	requireLastAdmin(t, err)
	f.stillAdmin(t, a)
}

func TestSync_DeactivatingLastAdmin_Refused(t *testing.T) {
	f := reset(t)
	a := f.admin(t, "a")
	got, err := f.repo.FindByID(context.Background(), a)
	require.NoError(t, err)
	err = run(f, operations.SyncPrincipals(f.repo, role.NewRepository(testpg.Pool(t)), nil),
		operations.SyncPrincipalsCommand{Principals: []operations.SyncPrincipalInput{
			{Email: got.UserIdentity.Email, Name: got.Name, Active: false},
		}})
	requireLastAdmin(t, err)
	f.stillAdmin(t, a)
}

// TestConcurrentMutualDemotion_OnlyOneSucceeds drives two real use-case runs
// (two pooled connections) that each demote the OTHER administrator. Repeated
// to give an unguarded implementation every chance to fail both-succeed.
func TestConcurrentMutualDemotion_OnlyOneSucceeds(t *testing.T) {
	for i := range 15 {
		f := reset(t)
		a := f.admin(t, "a")
		b := f.admin(t, "b")
		var wg sync.WaitGroup
		start := make(chan struct{})
		errs := make([]error, 2)
		for n, id := range []ids.PrincipalID{a, b} {
			wg.Go(func() {
				<-start
				errs[n] = f.assign(false, id)
			})
		}
		close(start)
		wg.Wait()

		ok := 0
		for _, e := range errs {
			if e == nil {
				ok++
			} else {
				requireLastAdmin(t, e)
			}
		}
		assert.Equal(t, 1, ok, "iteration %d: exactly one demotion may win (errs=%v)", i, errs)
		var admins int
		require.NoError(t, testpg.Pool(t).QueryRow(context.Background(),
			`SELECT count(*) FROM iam_principals WHERE type='USER' AND scope='ANCHOR' AND active AND all_applications`).Scan(&admins))
		assert.Equal(t, 1, admins, "iteration %d: one administrator must remain", i)
	}
}

// TestLockSerialisesAcrossTransactions is the deterministic form of the race:
// tx1 demotes A and stays open; tx2 (a second connection) tries to demote B and
// must BLOCK on the advisory lock, then be refused once tx1 commits.
func TestLockSerialisesAcrossTransactions(t *testing.T) {
	f := reset(t)
	a := f.admin(t, "a")
	b := f.admin(t, "b")
	ctx := context.Background()
	pool := testpg.Pool(t)

	pa, err := f.repo.FindByID(ctx, a)
	require.NoError(t, err)
	pb, err := f.repo.FindByID(ctx, b)
	require.NoError(t, err)
	pa.AllApplications = false
	pb.AllApplications = false

	tx1, err := pool.Begin(ctx)
	require.NoError(t, err)
	defer func() { _ = tx1.Rollback(ctx) }()
	require.NoError(t, f.repo.Persist(ctx, pa, usecasepgx.WrapTxForBootstrap(tx1)))

	tx2, err := pool.Begin(ctx)
	require.NoError(t, err)
	defer func() { _ = tx2.Rollback(ctx) }()
	done := make(chan error, 1)
	go func() { done <- f.repo.Persist(ctx, pb, usecasepgx.WrapTxForBootstrap(tx2)) }()

	select {
	case err := <-done:
		t.Fatalf("second demotion must block while the first tx holds the lock, returned: %v", err)
	case <-time.After(400 * time.Millisecond):
	}
	require.NoError(t, tx1.Commit(ctx))
	select {
	case err := <-done:
		requireLastAdmin(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("second demotion never unblocked")
	}
}

func mustClient(t *testing.T, f *fx) string {
	t.Helper()
	ev, err := usecaseop.Run(testpg.AnchorCtx(), f.uow, clientops.CreateClient(client.NewRepository(testpg.Pool(t))),
		clientops.CreateCommand{Name: "LA Client", Identifier: "la-" + f.label}, testpg.TestEC())
	require.NoError(t, err)
	return ev.ClientID
}

var labelSeq atomic.Int64

// nextLabel returns a short digits-only token unique across fixtures and runs
// (emails, client identifiers and service-account ids must be unique/short).
func nextLabel() string {
	return fmt.Sprintf("%d%d", time.Now().Unix()%100000, labelSeq.Add(1))
}
