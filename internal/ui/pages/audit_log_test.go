package pages

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"reflect"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/audit"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
)

type fakeAuditStore struct {
	logs   []audit.Log
	params []audit.CursorFilterParams
}

func (f *fakeAuditStore) FindWithCursor(_ context.Context, p audit.CursorFilterParams) ([]audit.Log, error) {
	f.params = append(f.params, p)
	var out []audit.Log
	for _, l := range f.logs {
		switch {
		case p.EntityType != nil && l.EntityType != *p.EntityType,
			p.Operation != nil && l.Operation != *p.Operation,
			len(p.ApplicationIDs) > 0 && (l.ApplicationID == nil || !slices.Contains(p.ApplicationIDs, *l.ApplicationID)),
			len(p.ClientIDs) > 0 && (l.ClientID == nil || !slices.Contains(p.ClientIDs, *l.ClientID)),
			p.After != nil && (l.PerformedAt.After(p.After.PerformedAt) || (l.PerformedAt.Equal(p.After.PerformedAt) && l.ID >= p.After.ID)):
			continue
		}
		out = append(out, l)
	}
	return out[:min(len(out), p.Limit)], nil
}

func (f *fakeAuditStore) FindByID(_ context.Context, id string) (*audit.Log, error) {
	for i := range f.logs {
		if f.logs[i].ID == id {
			l := f.logs[i]
			return &l, nil
		}
	}
	return nil, nil
}

func (f *fakeAuditStore) DistinctValues(_ context.Context, column string, _ int) ([]string, error) {
	var out []string
	for _, l := range f.logs {
		var v string
		switch column {
		case "entity_type":
			v = l.EntityType
		case "operation":
			v = l.Operation
		case "application_id":
			if l.ApplicationID != nil {
				v = *l.ApplicationID
			}
		}
		if v != "" && !slices.Contains(out, v) {
			out = append(out, v)
		}
	}
	return out, nil
}

var auditBase = time.Date(2026, 10, 6, 8, 30, 0, 0, time.UTC)

func auditLog(id, entity, op string, minutesAgo int) audit.Log {
	return audit.Log{ID: id, EntityType: entity, EntityID: "ent_" + id, Operation: op, PerformedAt: auditBase.Add(-time.Duration(minutesAgo) * time.Minute)}
}

func newAuditStore() *fakeAuditStore {
	name, pid, app, client := "Ada <Admin>", "prn_9", "app_1", "clt_1"
	first := auditLog("aud_1", "Role", "CreateRole", 1)
	first.PrincipalName, first.PrincipalID, first.ApplicationID, first.ClientID = &name, &pid, &app, &client
	first.OperationJSON = json.RawMessage(`{"z":1,"name":"<b>"}`)
	return &fakeAuditStore{logs: []audit.Log{first, auditLog("aud_2", "Principal", "ActivateUser", 2), auditLog("aud_3", "EventType", "ArchiveEventType", 3)}}
}

func auditRec(t *testing.T, store *fakeAuditStore, ac *auth.AuthContext, q req) string {
	t.Helper()
	return auditRecStatus(t, store, ac, q, http.StatusOK)
}

func auditRecStatus(t *testing.T, store *fakeAuditStore, ac *auth.AuthContext, q req, status int) string {
	t.Helper()
	page := &AuditLogPage{Store: store, Apps: newAppHarness().store, Clients: newCliHarness().store, Shell: shellForTest()}
	rec := serveMounted(t, ac, q, page)
	if rec.Code != status {
		t.Errorf("%s: status %d, want %d", q.target, rec.Code, status)
	}
	return rec.Body.String()
}

const auditViewPermTest = "platform:admin:audit-log:view"

func TestAuditLogList(t *testing.T) {
	store := newAuditStore()
	body := auditRec(t, store, anchorAdmin(), req{target: "/ui/audit-log"})
	mustContain(t, body, "<title>Audit Log · FlowCatalyst</title>", "View system activity and changes",
		`href="/ui/audit-log/aud_1"`, "Create Role", "Activate User", "fc-tag-warn", "fc-tag-success", "fc-tag-info",
		"Ada &lt;Admin&gt;", ">Unknown<", ">Orders<", "Acme &lt;Corp&gt;", "Showing the 3 most recent entries",
		`datetime="2026-10-06T08:29:00Z"`, `value="Role"`, `value="ArchiveEventType"`, `id="audit-filter-applications-field"`,
		`href="/ui/audit-log" class="fc-nav-item" title="Audit Log" aria-current="page"`)

	// The log is not tenant-scoped: the permission is the whole rule.
	client := &auth.AuthContext{PrincipalID: "p", Scope: auth.ScopeClient, Clients: []string{"clt_1"}, Roles: []string{"r"}, Permissions: []string{auditViewPermTest}, PrincipalType: auth.PrincipalTypeUser}
	auditRec(t, store, client, req{target: "/ui/audit-log"})
	// Without the application read permission the rows name the id, not the application.
	plain := auditRec(t, store, connUser(auditViewPermTest), req{target: "/ui/audit-log"})
	mustContain(t, plain, ">app_1<")
	mustNotContain(t, plain, ">Orders<")
	// The sidebar's permission string is not the API's: neither it nor an unrelated one opens the page.
	for _, perm := range []string{"platform:admin:audit:view", "platform:messaging:event:view"} {
		auditRecStatus(t, store, connUser(perm), req{target: "/ui/audit-log"}, http.StatusForbidden)
		mustContain(t, auditRec(t, store, connUser(perm), req{target: "/ui/audit-log/aud_1", datastar: true}), "fc-flash-error")
	}

	live := auditRec(t, store, anchorAdmin(), req{target: "/ui/audit-log?entityType=Role&applications=app_1&clients=clt_1", datastar: true})
	mustContain(t, live, "aud_1", "data: mode replace", "history.replaceState")
	mustNotContain(t, live, "aud_2")
	p := store.params[len(store.params)-1]
	if p.EntityType == nil || *p.EntityType != "Role" || !reflect.DeepEqual(p.ApplicationIDs, []string{"app_1"}) || !reflect.DeepEqual(p.ClientIDs, []string{"clt_1"}) || p.Limit != auditPageSize+1 {
		t.Errorf("params %+v", p)
	}
	// A filter that matches nothing offers Clear All back to the unfiltered list.
	mustContain(t, auditRec(t, store, anchorAdmin(), req{target: "/ui/audit-log?operation=Nope"}), "No audit logs found", `href="/ui/audit-log"`)
}

func TestAuditQueryAndPaging(t *testing.T) {
	q := parseAuditQuery(url.Values{"entityType": {" Role "}, "applications": {"a", "b"}, "after": {"1759739400000000.aud_7"}})
	again := parseAuditQuery(mustQuery(t, q.URL()))
	if q.EntityType != "Role" || q.After == nil || q.After.ID != "aud_7" || again.URL() != q.URL() {
		t.Errorf("round trip %s vs %s", q.URL(), again.URL())
	}
	if bad := parseAuditQuery(url.Values{"after": {"junk"}}); bad.After != nil {
		t.Error("a malformed cursor is no cursor")
	}
	store := newAuditStore()
	for i := range 120 {
		store.logs = append(store.logs, auditLog("aud_"+strconv.Itoa(1000+i), "Role", "CreateRole", 10+i))
	}
	first := auditRec(t, store, anchorAdmin(), req{target: "/ui/audit-log"})
	last := store.logs[auditPageSize-1]
	cursor := strconv.FormatInt(last.PerformedAt.UnixMicro(), 10) + "." + last.ID
	mustContain(t, first, "Showing the 100 most recent entries", "/ui/audit-log?after="+cursor)
	second := auditRec(t, store, anchorAdmin(), req{target: "/ui/audit-log?after=" + cursor})
	mustContain(t, second, "Showing 23 older entries", `href="/ui/audit-log"`)
}

func TestAuditLogDrawer(t *testing.T) {
	store := newAuditStore()
	body := auditRec(t, store, anchorAdmin(), req{target: "/ui/audit-log/aud_1", datastar: true})
	mustContain(t, body, "data: selector #audit-drawer\n", "Audit Log Details", "ent_aud_1", "Create Role", "Ada &lt;Admin&gt;", "prn_9", "Orders", "Acme &lt;Corp&gt;",
		"Operation Data", "&lt;b&gt;")
	mustNotContain(t, body, "<b>")
	page := auditRec(t, store, anchorAdmin(), req{target: "/ui/audit-log/aud_1"})
	mustContain(t, page, `<aside id="audit-drawer"`, `{selAudit: &#39;aud_1&#39;}`, "&#34;z&#34;: 1,\n  &#34;name&#34;: &#34;&lt;b&gt;&#34;")
	bare := auditRec(t, store, anchorAdmin(), req{target: "/ui/audit-log/aud_2", datastar: true})
	mustNotContain(t, bare, "Operation Data", "Principal ID")
	auditRecStatus(t, store, anchorAdmin(), req{target: "/ui/audit-log/nope'quote"}, http.StatusNotFound)
	auditRecStatus(t, store, anchorAdmin(), req{target: "/ui/audit-log/aud_404"}, http.StatusNotFound)
}

func TestAuditLogHelpers(t *testing.T) {
	for in, want := range map[string]string{"CreateApplication": "Create Application", "login": "Login", "": ""} {
		if got := operationName(in); got != want {
			t.Errorf("operationName(%q) = %q, want %q", in, got, want)
		}
	}
	if got := operationData(json.RawMessage(`not json`)); got != "not json" {
		t.Errorf("operation data that is not JSON is shown as stored: %q", got)
	}
	if got := operationData(nil); got != "" {
		t.Errorf("no data: %q", got)
	}
}
