package pages

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/event"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
)

type fakeEventStore struct {
	events []event.Event
	params []event.FilterParams
	scopes []string
}

func (f *fakeEventStore) FindWithFilters(_ context.Context, p event.FilterParams) ([]event.Event, error) {
	f.params = append(f.params, p)
	var out []event.Event
	for _, e := range f.events {
		if len(p.Types) > 0 && !slices.Contains(p.Types, e.Type) {
			continue
		}
		if p.AccessibleClientIDs != nil && e.ClientID != nil && !slices.Contains(*p.AccessibleClientIDs, *e.ClientID) {
			continue
		}
		if p.Source != nil && e.Source != *p.Source {
			continue
		}
		if p.Cursor != nil && (e.CreatedAt.After(p.Cursor.CreatedAt) || (e.CreatedAt.Equal(p.Cursor.CreatedAt) && e.ID >= p.Cursor.ID)) {
			continue
		}
		out = append(out, e)
	}
	return out[:min(len(out), p.Limit)], nil
}

func (f *fakeEventStore) FindByID(_ context.Context, id string) (*event.Event, error) {
	for i := range f.events {
		if f.events[i].ID == id {
			e := f.events[i]
			return &e, nil
		}
	}
	return nil, nil
}

func (f *fakeEventStore) ContextByID(_ context.Context, id string) ([]event.ContextEntry, error) {
	if id == "0EVENTID0000001" {
		return []event.ContextEntry{{Key: "tenant", Value: "acme <x>"}}, nil
	}
	return []event.ContextEntry{}, nil
}

func (f *fakeEventStore) DistinctValues(_ context.Context, column string, _ int, scope *[]string) ([]string, error) {
	f.scopes = append(f.scopes, distinctScopeKey(scope))
	if column == "type" {
		return []string{"orders:sales:order:created", "orders:billing:invoice:paid", "crm:contacts:person:updated"}, nil
	}
	return nil, nil
}

var evBase = time.Date(2026, 10, 6, 8, 30, 0, 0, time.UTC)

func ev(id, typ, client string, minutesAgo int) event.Event {
	at := evBase.Add(-time.Duration(minutesAgo) * time.Minute)
	e := event.Event{ID: id, Type: typ, Source: "orders-service", Subject: "order/42", Time: at, CreatedAt: at, DeduplicationID: "dedup-" + id}
	if client != "" {
		e.ClientID = &client
	}
	return e
}

func newEvStore() *fakeEventStore {
	app, group, corr := "orders", "order-42", "corr-1"
	first := ev("0EVENTID0000001", "orders:sales:order:created", "clt_1", 1)
	first.Application, first.MessageGroup, first.CorrelationID = &app, &group, &corr
	first.Data = json.RawMessage(`{"z":1,"orderId":42,"note":"<b>"}`)
	return &fakeEventStore{events: []event.Event{
		first,
		ev("0EVENTID0000002", "crm:contacts:person:updated", "", 2),
		ev("0EVENTID0000003", "orders:billing:invoice:paid", "clt_2", 3),
	}}
}

func evRec(t *testing.T, store *fakeEventStore, ac *auth.AuthContext, q req) *httptest.ResponseRecorder {
	t.Helper()
	return serveMounted(t, ac, q, &Events{Events: store, Clients: newCliHarness().store, Shell: shellForTest()})
}

const evViewPermTest = "platform:messaging:event:view"

func TestEventsList(t *testing.T) {
	store := newEvStore()
	body := evRec(t, store, anchorAdmin(), req{target: "/ui/events"}).Body.String()
	mustContain(t, body, "<title>Events · FlowCatalyst</title>", "Browse events from the event store",
		`href="/ui/events/0EVENTID0000001"`, ">0EVENTID00...<", `title="0EVENTID0000001"`, "fc-tag-info", "orders:sales:order:created",
		"orders-service", "order/42", `title="clt_1">clt_1<`, `datetime="2026-10-06T08:29:00Z"`, "Showing the 3 most recent events",
		`id="ev-filter-types-field"`, `value="crm"`, `name="size"`, `href="/ui/events" class="fc-nav-item" title="Events" aria-current="page"`)

	if rec := evRec(t, store, connUser("platform:messaging:dispatch-job:view"), req{target: "/ui/events"}); rec.Code != http.StatusForbidden {
		t.Errorf("no permission: %d", rec.Code)
	}
	scoped := &auth.AuthContext{PrincipalID: "p", Scope: auth.ScopeClient, Clients: []string{"clt_1"}, Roles: []string{"r"}, Permissions: []string{evViewPermTest}, PrincipalType: auth.PrincipalTypeUser}
	own := evRec(t, store, scoped, req{target: "/ui/events"}).Body.String()
	mustContain(t, own, "0EVENTID0000001", "0EVENTID0000002")
	mustNotContain(t, own, "0EVENTID0000003")
	if !slices.Contains(store.scopes, "c:clt_1") {
		t.Errorf("facet scopes %v: the client user's event types were not scoped", store.scopes)
	}
	if rec := evRec(t, store, scoped, req{target: "/ui/events/0EVENTID0000003"}); rec.Code != http.StatusForbidden {
		t.Errorf("another tenant's event: %d", rec.Code)
	}

	live := evRec(t, store, anchorAdmin(), req{target: "/ui/events?types=crm:contacts:person:updated&applications=crm&size=50", datastar: true}).Body.String()
	mustContain(t, live, "0EVENTID0000002", "data: mode replace", `id="ev-filter-subdomains-options"`, `value="contacts"`, "history.replaceState")
	mustNotContain(t, live, "0EVENTID0000001", `value="billing"`)
	if p := store.params[len(store.params)-1]; p.Limit != 50 || strings.Join(p.Applications, ",") != "crm" {
		t.Errorf("params %+v", p)
	}
}

func TestEventsQueryAndPaging(t *testing.T) {
	q := parseEvQuery(url.Values{"types": {"a:b:c:d"}, "q": {"svc"}, "size": {"1000"}, "after": {"1759739400000000.0EV1"}})
	again := parseEvQuery(mustQuery(t, q.URL()))
	if !reflect.DeepEqual(again.values(), q.values()) || q.After == nil || q.After.ID != "0EV1" {
		t.Errorf("round trip %s vs %s", q.URL(), again.URL())
	}
	store := newEvStore()
	for i := range 60 {
		store.events = append(store.events, ev("1EV"+strconv.Itoa(100+i), "orders:sales:order:created", "", 10+i))
	}
	first := evRec(t, store, anchorAdmin(), req{target: "/ui/events?size=50"}).Body.String()
	last := store.events[49]
	mustContain(t, first, "/ui/events?after="+strconv.FormatInt(last.CreatedAt.UnixMicro(), 10)+"."+last.ID+"&amp;size=50")
	second := evRec(t, store, anchorAdmin(), req{target: "/ui/events?size=50&after=" + strconv.FormatInt(last.CreatedAt.UnixMicro(), 10) + "." + last.ID}).Body.String()
	mustContain(t, second, "Showing 13 older events", `href="/ui/events?size=50"`)
}

func TestEventDrawer(t *testing.T) {
	store := newEvStore()
	body := evRec(t, store, anchorAdmin(), req{target: "/ui/events/0EVENTID0000001", datastar: true}).Body.String()
	mustContain(t, body, "orders:sales:order:created", "0EVENTID0000001", ">orders<", "order-42", "corr-1", "dedup-0EVENTID0000001",
		"data: selector #ev-drawer\n", `id="ev-drawer-body"`, "Context Data", "tenant", "acme &lt;x&gt;")
	page := evRec(t, store, anchorAdmin(), req{target: "/ui/events/0EVENTID0000001"}).Body.String()
	mustContain(t, page, "&#34;z&#34;: 1,\n  &#34;orderId&#34;: 42,\n  &#34;note&#34;: &#34;&lt;b&gt;&#34;", `<aside id="ev-drawer"`)
	if rec := evRec(t, store, anchorAdmin(), req{target: "/ui/events/nope'quote"}); rec.Code != http.StatusNotFound {
		t.Errorf("bad id: %d", rec.Code)
	}
	if got := formatData(json.RawMessage(`"{\"a\":1}"`)); got != "{\n  \"a\": 1\n}" {
		t.Errorf("a JSON string holding JSON: %q", got)
	}
	if got := formatData(nil); got != "-" {
		t.Errorf("no data: %q", got)
	}
}

func BenchmarkEventsTable(b *testing.B) {
	for _, n := range []int{200, 1000} {
		v := evView{Query: parseEvQuery(url.Values{"size": {strconv.Itoa(n)}})}
		tmpl := newEvStore().events[0]
		for i := range n {
			e := tmpl
			e.ID = "0EV" + strconv.Itoa(1000000+i)
			v.Rows = append(v.Rows, toEvRow(&e))
		}
		b.Run(strconv.Itoa(n), func(b *testing.B) {
			var buf bytes.Buffer
			for b.Loop() {
				buf.Reset()
				if err := evTable(v).Render(context.Background(), &buf); err != nil {
					b.Fatal(err)
				}
			}
			b.ReportMetric(float64(buf.Len()), "bytes")
		})
	}
}
