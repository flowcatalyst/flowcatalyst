//go:build integration

package api

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apicommon"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecase"
)

// refRow is one read-projection row for the event-type / subscription / connection
// filters. Only the read table is seeded: every list-shaped read serves from it.
type refRow struct {
	id, code, status    string
	sub, conn, clientID *string
	created             time.Time
}

func seedRefRows(t *testing.T, pool *pgxpool.Pool, rows ...refRow) {
	t.Helper()
	for _, r := range rows {
		if r.created.IsZero() {
			r.created = time.Now().UTC().Add(-time.Hour)
		}
		if r.status == "" {
			r.status = "FAILED"
		}
		_, err := pool.Exec(context.Background(), `
			INSERT INTO msg_dispatch_jobs_read (id, code, target_url, subscription_id, connection_id, client_id,
			                                    kind, protocol, mode, status, max_retries, created_at, updated_at)
			VALUES ($1, $2, 'http://example.invalid/hook', $3, $4, $5, 'EVENT', 'HTTP_WEBHOOK', 'IMMEDIATE', $6, 3, $7, $7)`,
			r.id, r.code, r.sub, r.conn, r.clientID, r.status, r.created)
		require.NoError(t, err)
	}
}

func seedSubscription(t *testing.T, pool *pgxpool.Pool, id, code, name string, clientID, connID *string) {
	t.Helper()
	_, err := pool.Exec(context.Background(), `
		INSERT INTO msg_subscriptions (id, code, name, target, client_id, connection_id)
		VALUES ($1, $2, $3, 'http://example.invalid/hook', $4, $5)`, id, code, name, clientID, connID)
	require.NoError(t, err)
}

func seedConnection(t *testing.T, pool *pgxpool.Pool, id, code string, clientID *string) {
	t.Helper()
	_, err := pool.Exec(context.Background(), `
		INSERT INTO msg_connections (id, code, name, service_account_id, client_id)
		VALUES ($1, $2, $2, 'sac_rfref0000001', $3)`, id, code, clientID)
	require.NoError(t, err)
}

func listIDs(t *testing.T, s *State, query string) []string {
	t.Helper()
	r := serveAs(t, s, viewPerm).Get("/api/dispatch-jobs?" + query)
	require.Equal(t, http.StatusOK, r.Code, r.Body.String())
	return idsOf(t, r.Body.Bytes())
}

// A job's code is its event type: eventType / eventTypes filter that column, alone
// and with the window and the status; more than ten types is a 400.
func TestRefs_EventTypeFilters(t *testing.T) {
	pool := testpg.Pool(t)
	s := newState(pool)
	old := time.Now().UTC().Add(-72 * time.Hour)
	seedRefRows(t, pool,
		refRow{id: "rfet0000001", code: "rfet:orders:order:created", status: "FAILED"},
		refRow{id: "rfet0000002", code: "rfet:orders:order:created", status: "COMPLETED"},
		refRow{id: "rfet0000003", code: "rfet:orders:order:shipped", status: "FAILED"},
		refRow{id: "rfet0000004", code: "rfet:orders:order:cancelled", status: "FAILED"},
		refRow{id: "rfet0000005", code: "rfet:orders:order:created", status: "FAILED", created: old},
	)
	created, shipped, cancelled := "rfet:orders:order:created", "rfet:orders:order:shipped", "rfet:orders:order:cancelled"

	assert.ElementsMatch(t, []string{"rfet0000001", "rfet0000002"}, listIDs(t, s, "eventType="+created), "exact, inside the default window")
	assert.ElementsMatch(t, []string{"rfet0000001", "rfet0000002", "rfet0000003"}, listIDs(t, s, "eventTypes="+created+","+shipped))
	assert.ElementsMatch(t, []string{"rfet0000001", "rfet0000003"}, listIDs(t, s, "eventTypes="+created+","+shipped+"&status=FAILED"), "with a status")
	assert.ElementsMatch(t, []string{"rfet0000001", "rfet0000002", "rfet0000005"},
		listIDs(t, s, "eventType="+created+"&since="+rfc(time.Now().Add(-5*24*time.Hour))), "with a wider window")
	assert.ElementsMatch(t, []string{"rfet0000003"}, listIDs(t, s, "eventType="+created+"&eventTypes="+shipped+"&codes="+shipped),
		"eventType+eventTypes union into one list; codes must also hold")
	assert.Empty(t, listIDs(t, s, "eventType=rfet:orders:order:nope"))
	assert.ElementsMatch(t, []string{"rfet0000001"}, listIDs(t, s, "codes="+created+"&eventType="+created+"&status=FAILED"))
	_ = cancelled

	// More than ten types is refused, on the list and the bulk reads alike.
	many := ""
	for i := range 11 {
		many += fmt.Sprintf("rfet:t%d,", i)
	}
	r := serveAs(t, s, viewPerm).Get("/api/dispatch-jobs?eventTypes=" + many)
	assert.Equal(t, http.StatusBadRequest, r.Code, r.Body.String())
	assert.Contains(t, r.Body.String(), "TOO_MANY_EVENT_TYPES")
	_, err := s.requeueByFilterPreview(callerWith(viewPerm), &apicommon.In[RequeueByFilterPreviewRequest]{
		Body: RequeueByFilterPreviewRequest{DispatchJobFilter{EventTypes: many}},
	})
	testpg.RequireUsecaseError(t, err, usecase.KindValidation, "TOO_MANY_EVENT_TYPES")

	// The preview counts exactly what the list shows.
	pv := preview(t, s, callerWith(viewPerm), DispatchJobFilter{EventType: created, Status: "FAILED"})
	assert.EqualValues(t, 1, pv.Count)
}

// subscriptionId(s), subscriptionCode and subscriptionName: ids filter directly; codes and
// names are resolved to ids first (exact, or a prefix with a trailing *); a name that
// matches nothing lists nothing (no error); more than 200 matches is a 400.
func TestRefs_SubscriptionFilters(t *testing.T) {
	pool := testpg.Pool(t)
	s := newState(pool)
	seedSubscription(t, pool, "sub_rfsa0000001", "rfsub-alpha", "RF Alpha Orders", nil, nil)
	seedSubscription(t, pool, "sub_rfsb0000001", "rfsub-beta", "RF Beta Orders", nil, nil)
	seedSubscription(t, pool, "sub_rfsc0000001", "other-gamma", "Gamma", nil, nil)
	seedRefRows(t, pool,
		refRow{id: "rfsu0000001", code: "rfsu:a:b:c", sub: new("sub_rfsa0000001"), status: "FAILED"},
		refRow{id: "rfsu0000002", code: "rfsu:a:b:c", sub: new("sub_rfsa0000001"), status: "COMPLETED"},
		refRow{id: "rfsu0000003", code: "rfsu:a:b:d", sub: new("sub_rfsb0000001"), status: "FAILED"},
		refRow{id: "rfsu0000004", code: "rfsu:a:b:d", sub: new("sub_rfsc0000001"), status: "FAILED"},
		refRow{id: "rfsu0000005", code: "rfsu:a:b:d", status: "FAILED"},
	)
	assert.ElementsMatch(t, []string{"rfsu0000001", "rfsu0000002"}, listIDs(t, s, "subscriptionId=sub_rfsa0000001"))
	assert.ElementsMatch(t, []string{"rfsu0000001", "rfsu0000002", "rfsu0000003"}, listIDs(t, s, "subscriptionIds=sub_rfsa0000001,sub_rfsb0000001"))
	assert.ElementsMatch(t, []string{"rfsu0000001", "rfsu0000002"}, listIDs(t, s, "subscriptionCode=rfsub-alpha"), "exact code")
	assert.ElementsMatch(t, []string{"rfsu0000001", "rfsu0000002", "rfsu0000003"}, listIDs(t, s, "subscriptionCode=rfsub-*"), "prefix code")
	assert.Empty(t, listIDs(t, s, "subscriptionCode=rfsub-"), "no star: exact, and no subscription has that code")
	assert.ElementsMatch(t, []string{"rfsu0000001", "rfsu0000002"}, listIDs(t, s, "subscriptionName=rf%20alpha%20orders"), "exact name, case-insensitive")
	assert.ElementsMatch(t, []string{"rfsu0000001", "rfsu0000002", "rfsu0000003"}, listIDs(t, s, "subscriptionName=RF%20*"), "prefix name")
	assert.ElementsMatch(t, []string{"rfsu0000001"}, listIDs(t, s, "subscriptionName=RF%20*&status=FAILED&subscriptionCode=rfsub-alpha"), "with status, code and name together")
	assert.ElementsMatch(t, []string{"rfsu0000001"}, listIDs(t, s, "subscriptionCode=rfsub-*&eventType=rfsu:a:b:c&status=FAILED"), "with an event type and status")
	assert.Empty(t, listIDs(t, s, "subscriptionName=No%20Such%20Subscription"), "nothing resolves: an empty list, not an error")
	assert.Empty(t, listIDs(t, s, "subscriptionCode=%25*"), "% is literal in a prefix, not a wildcard")
	assert.Empty(t, listIDs(t, s, "subscriptionCode=rfsub-alpha&subscriptionName=Gamma"), "code and name must name the same subscription")

	// The preview counts what the list shows, and resolves the name too.
	pv := preview(t, s, callerWith(viewPerm), DispatchJobFilter{SubscriptionName: "RF Alpha Orders"})
	assert.EqualValues(t, 2, pv.Count)
	pv = preview(t, s, callerWith(viewPerm), DispatchJobFilter{SubscriptionName: "No Such"})
	assert.EqualValues(t, 0, pv.Count)

	// A prefix that matches too many subscriptions is refused, never truncated.
	_, err := pool.Exec(context.Background(), `
		INSERT INTO msg_subscriptions (id, code, name, target)
		SELECT 'sub_rfmany' || lpad(n::text, 7, '0'), 'rfmany-' || n, 'RF many ' || n, 'http://x' FROM generate_series(1, 201) n`)
	require.NoError(t, err)
	r := serveAs(t, s, viewPerm).Get("/api/dispatch-jobs?subscriptionCode=rfmany-*")
	assert.Equal(t, http.StatusBadRequest, r.Code, r.Body.String())
	assert.Contains(t, r.Body.String(), "TOO_MANY_MATCHES")
	_, err = s.requeueByFilterPreview(callerWith(viewPerm), &apicommon.In[RequeueByFilterPreviewRequest]{
		Body: RequeueByFilterPreviewRequest{DispatchJobFilter{SubscriptionName: "RF many *"}},
	})
	testpg.RequireUsecaseError(t, err, usecase.KindValidation, "TOO_MANY_MATCHES")

	// More than 50 subscription ids on the query is refused.
	ids := ""
	for i := range 51 {
		ids += fmt.Sprintf("sub_x%d,", i)
	}
	r = serveAs(t, s, viewPerm).Get("/api/dispatch-jobs?subscriptionIds=" + ids)
	assert.Equal(t, http.StatusBadRequest, r.Code, r.Body.String())
	assert.Contains(t, r.Body.String(), "TOO_MANY_SUBSCRIPTION_IDS")
}

// connectionId / connectionCode find the jobs delivered through a connection: the jobs
// of its subscriptions and the jobs that name it themselves (fn.dispatch).
func TestRefs_ConnectionFilters(t *testing.T) {
	pool := testpg.Pool(t)
	s := newState(pool)
	seedConnection(t, pool, "con_rfc00000001", "rfcon-main", nil)
	seedConnection(t, pool, "con_rfc00000002", "rfcon-other", nil)
	seedSubscription(t, pool, "sub_rfca0000001", "rfcon-sub-a", "RFC A", nil, new("con_rfc00000001"))
	seedSubscription(t, pool, "sub_rfca0000002", "rfcon-sub-b", "RFC B", nil, new("con_rfc00000001"))
	seedSubscription(t, pool, "sub_rfcb0000001", "rfcon-sub-c", "RFC C", nil, new("con_rfc00000002"))
	seedRefRows(t, pool,
		refRow{id: "rfcn0000001", code: "rfcn:a:b:c", sub: new("sub_rfca0000001"), status: "FAILED"},
		refRow{id: "rfcn0000002", code: "rfcn:a:b:c", sub: new("sub_rfca0000002"), status: "COMPLETED"},
		refRow{id: "rfcn0000003", code: "rfcn:a:b:c", sub: new("sub_rfcb0000001"), status: "FAILED"},
		refRow{id: "rfcn0000004", code: "rfcn:fn:x:dispatch", conn: new("con_rfc00000001"), status: "FAILED"},
		refRow{id: "rfcn0000005", code: "rfcn:fn:x:dispatch", conn: new("con_rfc00000002"), status: "FAILED"},
		refRow{id: "rfcn0000006", code: "rfcn:a:b:c", status: "FAILED"},
	)
	main := []string{"rfcn0000001", "rfcn0000002", "rfcn0000004"}
	assert.ElementsMatch(t, main, listIDs(t, s, "connectionId=con_rfc00000001"))
	assert.ElementsMatch(t, main, listIDs(t, s, "connectionCode=rfcon-main"))
	assert.ElementsMatch(t, []string{"rfcn0000001", "rfcn0000004"}, listIDs(t, s, "connectionCode=rfcon-main&status=FAILED"), "with a status")
	assert.ElementsMatch(t, []string{"rfcn0000003", "rfcn0000005", "rfcn0000001", "rfcn0000002", "rfcn0000004"}, listIDs(t, s, "connectionCode=rfcon-*"), "prefix")
	assert.ElementsMatch(t, []string{"rfcn0000001"}, listIDs(t, s, "connectionCode=rfcon-main&subscriptionCode=rfcon-sub-a"), "connection and subscription together")
	assert.Empty(t, listIDs(t, s, "connectionCode=no-such-connection"), "unknown code: empty, not an error")
	assert.Empty(t, listIDs(t, s, "connectionId=con_nosuch00001"))
	assert.ElementsMatch(t, []string{"rfcn0000004"}, listIDs(t, s, "connectionId=con_rfc00000001&eventType=rfcn:fn:x:dispatch"))
	assert.Empty(t, listIDs(t, s, "connectionCode=rfcon-other&connectionId=con_rfc00000001"), "a code and an id that disagree match nothing")

	pv := preview(t, s, callerWith(viewPerm), DispatchJobFilter{ConnectionCode: "rfcon-main"})
	assert.EqualValues(t, 3, pv.Count)
}

// A client user resolves names within its own tenants (and platform rows) only: another
// tenant's subscription or connection cannot be found by name, and its jobs are not
// listed by id either.
func TestRefs_ResolutionIsTenantScoped(t *testing.T) {
	pool := testpg.Pool(t)
	s := newState(pool)
	mine, other := "clt_rfscope0001", "clt_rfscope0002"
	seedSubscription(t, pool, "sub_rfsm0000001", "rfscope-mine", "RF Scope Mine", &mine, nil)
	seedSubscription(t, pool, "sub_rfso0000001", "rfscope-other", "RF Scope Other", &other, nil)
	seedConnection(t, pool, "con_rfsm0000001", "rfscope-con-mine", &mine)
	seedConnection(t, pool, "con_rfso0000001", "rfscope-con-other", &other)
	seedRefRows(t, pool,
		refRow{id: "rfsc0000001", code: "rfsc:a:b:c", sub: new("sub_rfsm0000001"), clientID: &mine},
		refRow{id: "rfsc0000002", code: "rfsc:a:b:c", sub: new("sub_rfso0000001"), clientID: &other},
		refRow{id: "rfsc0000003", code: "rfsc:a:b:c", conn: new("con_rfso0000001"), clientID: &other},
	)
	call := func(ctx context.Context, f DispatchJobFilter) []string {
		in := &listInput{DispatchJobFilter: f}
		out, err := s.list(ctx, in)
		require.NoError(t, err)
		ids := make([]string, len(out.Body))
		for i, r := range out.Body {
			ids[i] = r.ID
		}
		return ids
	}
	cc := clientCaller(mine)
	assert.Equal(t, []string{"rfsc0000001"}, call(cc, DispatchJobFilter{SubscriptionName: "RF Scope *"}), "only my subscription resolves")
	assert.Empty(t, call(cc, DispatchJobFilter{SubscriptionCode: "rfscope-other"}), "the other tenant's code does not resolve")
	assert.Empty(t, call(cc, DispatchJobFilter{ConnectionCode: "rfscope-con-other"}))
	assert.Empty(t, call(cc, DispatchJobFilter{SubscriptionID: "sub_rfso0000001"}), "nor does its id list its jobs")
	assert.ElementsMatch(t, []string{"rfsc0000001", "rfsc0000002"}, call(anchor(), DispatchJobFilter{SubscriptionName: "RF Scope *"}), "an anchor resolves everything")
	assert.ElementsMatch(t, []string{"rfsc0000003"}, call(anchor(), DispatchJobFilter{ConnectionCode: "rfscope-con-other"}))
}

// The filter-options answer is kept briefly per tenant scope and window, and now carries
// the event types.
func TestFilterOptions_CachedAndCarriesEventTypes(t *testing.T) {
	pool := testpg.Pool(t)
	s := newState(pool)
	seedRefRows(t, pool, refRow{id: "rffo0000001", code: "rffo:first:agg:one"})
	first, err := s.filterOptions(anchor(), &filterOptionsInput{})
	require.NoError(t, err)
	assert.Contains(t, first.Body.EventTypes, "rffo:first:agg:one")
	assert.Equal(t, first.Body.Codes, first.Body.EventTypes)

	seedRefRows(t, pool, refRow{id: "rffo0000002", code: "rffo:second:agg:two"})
	again, err := s.filterOptions(anchor(), &filterOptionsInput{})
	require.NoError(t, err)
	assert.NotContains(t, again.Body.EventTypes, "rffo:second:agg:two", "served from the cache inside the TTL")

	// Another tenant scope does not share the entry.
	mine := "clt_rffo0000001"
	other, err := s.filterOptions(clientCaller(mine), &filterOptionsInput{})
	require.NoError(t, err)
	assert.Contains(t, other.Body.EventTypes, "rffo:second:agg:two")

	// A fresh State (another process, or after the TTL) sees it.
	fresh, err := newState(pool).filterOptions(anchor(), &filterOptionsInput{})
	require.NoError(t, err)
	assert.Contains(t, fresh.Body.EventTypes, "rffo:second:agg:two")
}
