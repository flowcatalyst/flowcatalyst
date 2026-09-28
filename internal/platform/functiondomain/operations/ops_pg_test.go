//go:build integration

package operations_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/functiondomain"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/functiondomain/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/flowcatalyst-go/internal/tsid"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecaseop"
)

func TestMain(m *testing.M) { testpg.RunMain(m) }

// clientCtx returns a context carrying a CLIENT-scoped AuthContext confined
// to clientIDs, holding perm.
func clientCtx(clientIDs []string, perms ...string) context.Context {
	return testpg.WithAuth(context.Background(), &auth.AuthContext{
		PrincipalID: "prn_optestrunner1",
		Scope:       auth.ScopeClient,
		Clients:     clientIDs,
		Permissions: perms,
	})
}

// newClientID mints a properly-shaped client id (fng_domains.client_id is
// VARCHAR(17), the real TSID width — a hand-rolled test string would
// overflow it).
func newClientID() string { return tsid.GenerateWithPrefix("clt") }

func TestCreateDomain_HappyPath(t *testing.T) {
	t.Parallel()
	repo := functiondomain.NewRepository(testpg.Pool(t))
	uow := testpg.NewUoW(t)
	op := operations.CreateDomain(repo)

	cid := newClientID()
	event, err := usecaseop.Run(testpg.AnchorCtx(), uow, op, operations.CreateCommand{
		Zone: "Acme-" + short(t) + ".Example.com", ClientID: &cid,
	}, testpg.TestEC())
	require.NoError(t, err)
	assert.NotEmpty(t, event.DomainID)

	d, err := repo.FindByID(context.Background(), event.DomainID)
	require.NoError(t, err)
	require.NotNil(t, d)
	assert.Equal(t, "acme-"+short(t)+".example.com", d.Zone, "zone is lowercased")
	assert.Equal(t, cid, *d.ClientID)
}

// TestCreateDomain_RejectsOverlap_DifferentOwner_ParentThenChild proves the
// overlap refusal in the "existing zone covers the new one" direction.
func TestCreateDomain_RejectsOverlap_DifferentOwner_ParentThenChild(t *testing.T) {
	t.Parallel()
	repo := functiondomain.NewRepository(testpg.Pool(t))
	uow := testpg.NewUoW(t)
	op := operations.CreateDomain(repo)

	zone := "parent-" + short(t) + ".test"
	ownerA := newClientID()
	ownerB := newClientID()

	_, err := usecaseop.Run(testpg.AnchorCtx(), uow, op, operations.CreateCommand{Zone: zone, ClientID: &ownerA}, testpg.TestEC())
	require.NoError(t, err)

	_, err = usecaseop.Run(testpg.AnchorCtx(), uow, op, operations.CreateCommand{
		Zone: "sub.example." + zone, ClientID: &ownerB,
	}, testpg.TestEC())
	testpg.RequireUsecaseError(t, err, usecase.KindConflict, "ZONE_OVERLAP")
}

// TestCreateDomain_RejectsOverlap_DifferentOwner_ChildThenParent proves the
// refusal in the other direction: claiming a broader zone that covers an
// existing, narrower claim by a different owner.
func TestCreateDomain_RejectsOverlap_DifferentOwner_ChildThenParent(t *testing.T) {
	t.Parallel()
	repo := functiondomain.NewRepository(testpg.Pool(t))
	uow := testpg.NewUoW(t)
	op := operations.CreateDomain(repo)

	base := "child-" + short(t) + ".test"
	sub := "api." + base
	ownerA := newClientID()
	ownerB := newClientID()

	_, err := usecaseop.Run(testpg.AnchorCtx(), uow, op, operations.CreateCommand{Zone: sub, ClientID: &ownerA}, testpg.TestEC())
	require.NoError(t, err)

	_, err = usecaseop.Run(testpg.AnchorCtx(), uow, op, operations.CreateCommand{Zone: base, ClientID: &ownerB}, testpg.TestEC())
	testpg.RequireUsecaseError(t, err, usecase.KindConflict, "ZONE_OVERLAP")
}

// TestCreateDomain_AllowsOverlap_SameOwner proves the SAME owner may claim
// an overlapping zone (the brief's refusal rule names only "a DIFFERENT
// owner").
func TestCreateDomain_AllowsOverlap_SameOwner(t *testing.T) {
	t.Parallel()
	repo := functiondomain.NewRepository(testpg.Pool(t))
	uow := testpg.NewUoW(t)
	op := operations.CreateDomain(repo)

	base := "same-" + short(t) + ".test"
	owner := newClientID()

	_, err := usecaseop.Run(testpg.AnchorCtx(), uow, op, operations.CreateCommand{Zone: base, ClientID: &owner}, testpg.TestEC())
	require.NoError(t, err)

	_, err = usecaseop.Run(testpg.AnchorCtx(), uow, op, operations.CreateCommand{Zone: "api." + base, ClientID: &owner}, testpg.TestEC())
	assert.NoError(t, err, "same owner may claim an overlapping zone")
}

// TestCreateDomain_RejectsInvalidZone
func TestCreateDomain_RejectsInvalidZone(t *testing.T) {
	t.Parallel()
	repo := functiondomain.NewRepository(testpg.Pool(t))
	uow := testpg.NewUoW(t)
	op := operations.CreateDomain(repo)

	_, err := usecaseop.Run(testpg.AnchorCtx(), uow, op, operations.CreateCommand{Zone: "*.bad.com"}, testpg.TestEC())
	testpg.RequireUsecaseError(t, err, usecase.KindValidation, "INVALID_ZONE_FORMAT")
}

// TestCreateDomain_ScopeForbidden_ClientCannotClaimForAnother proves the
// resource-level authz: a CLIENT-scoped caller may only claim a zone for a
// client it can access.
func TestCreateDomain_ScopeForbidden_ClientCannotClaimForAnother(t *testing.T) {
	t.Parallel()
	repo := functiondomain.NewRepository(testpg.Pool(t))
	uow := testpg.NewUoW(t)
	op := operations.CreateDomain(repo)

	mine := newClientID()
	other := newClientID()
	ctx := clientCtx([]string{mine}, "platform:function:domain:manage")

	_, err := usecaseop.Run(ctx, uow, op, operations.CreateCommand{Zone: "x-" + short(t) + ".test", ClientID: &other}, testpg.TestEC())
	testpg.RequireUsecaseError(t, err, usecase.KindAuthorization, "SCOPE_FORBIDDEN")
}

// TestListDomains_ReachFiltering proves FindWithFilters's AccessibleClientIDs
// scoping: a platform-owned (nil client) zone is visible to anyone, a
// client-owned zone only to a caller with access to that client.
func TestListDomains_ReachFiltering(t *testing.T) {
	t.Parallel()
	repo := functiondomain.NewRepository(testpg.Pool(t))
	uow := testpg.NewUoW(t)
	op := operations.CreateDomain(repo)

	mine := newClientID()
	other := newClientID()

	platformZone := "platform-" + short(t) + ".test"
	mineZone := "mine-" + short(t) + ".test"
	otherZone := "other-" + short(t) + ".test"

	_, err := usecaseop.Run(testpg.AnchorCtx(), uow, op, operations.CreateCommand{Zone: platformZone}, testpg.TestEC())
	require.NoError(t, err)
	_, err = usecaseop.Run(testpg.AnchorCtx(), uow, op, operations.CreateCommand{Zone: mineZone, ClientID: &mine}, testpg.TestEC())
	require.NoError(t, err)
	_, err = usecaseop.Run(testpg.AnchorCtx(), uow, op, operations.CreateCommand{Zone: otherZone, ClientID: &other}, testpg.TestEC())
	require.NoError(t, err)

	rows, err := repo.FindWithFilters(context.Background(), functiondomain.ListFilters{AccessibleClientIDs: &[]string{mine}})
	require.NoError(t, err)
	zones := make(map[string]bool, len(rows))
	for _, r := range rows {
		zones[r.Zone] = true
	}
	assert.True(t, zones[platformZone], "platform-owned zone must be visible to any reach-scoped caller")
	assert.True(t, zones[mineZone], "own client's zone must be visible")
	assert.False(t, zones[otherZone], "another client's zone must not be visible")
}

// TestDeleteDomain_RefusesDomainInUse proves the DOMAIN_IN_USE refusal: a
// route hostname covered by the domain's zone blocks the delete.
func TestDeleteDomain_RefusesDomainInUse(t *testing.T) {
	t.Parallel()
	repo := functiondomain.NewRepository(testpg.Pool(t))
	uow := testpg.NewUoW(t)
	createOp := operations.CreateDomain(repo)

	zone := "inuse-" + short(t) + ".test"
	event, err := usecaseop.Run(testpg.AnchorCtx(), uow, createOp, operations.CreateCommand{Zone: zone}, testpg.TestEC())
	require.NoError(t, err)

	hosts := func(context.Context) ([]string, error) { return []string{"api." + zone}, nil }
	deleteOp := operations.DeleteDomain(repo, hosts)
	_, err = usecaseop.Run(testpg.AnchorCtx(), uow, deleteOp, operations.DeleteCommand{ID: event.DomainID}, testpg.TestEC())
	testpg.RequireUsecaseError(t, err, usecase.KindBusinessRule, "DOMAIN_IN_USE")

	// The claim must still exist.
	d, err := repo.FindByID(context.Background(), event.DomainID)
	require.NoError(t, err)
	assert.NotNil(t, d)
}

// TestDeleteDomain_SucceedsWhenUnused proves the ordinary delete path.
func TestDeleteDomain_SucceedsWhenUnused(t *testing.T) {
	t.Parallel()
	repo := functiondomain.NewRepository(testpg.Pool(t))
	uow := testpg.NewUoW(t)
	createOp := operations.CreateDomain(repo)

	zone := "unused-" + short(t) + ".test"
	event, err := usecaseop.Run(testpg.AnchorCtx(), uow, createOp, operations.CreateCommand{Zone: zone}, testpg.TestEC())
	require.NoError(t, err)

	hosts := func(context.Context) ([]string, error) { return nil, nil }
	deleteOp := operations.DeleteDomain(repo, hosts)
	_, err = usecaseop.Run(testpg.AnchorCtx(), uow, deleteOp, operations.DeleteCommand{ID: event.DomainID}, testpg.TestEC())
	require.NoError(t, err)

	d, err := repo.FindByID(context.Background(), event.DomainID)
	require.NoError(t, err)
	assert.Nil(t, d, "domain row must be gone")
}

// short derives a short, test-unique suffix from the test name so parallel
// tests never collide on the unique zone index.
func short(t *testing.T) string {
	h := 0
	for _, r := range t.Name() {
		h = h*31 + int(r)
	}
	if h < 0 {
		h = -h
	}
	return itoa(h % 100000)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}
