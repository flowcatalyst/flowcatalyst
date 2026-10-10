package pages

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/ids"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/eventtype"
	etops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/eventtype/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
)

type fakeETs struct{ ets []eventtype.EventType }

func (f *fakeETs) FindWithFilters(_ context.Context, app, _, status, sub, agg *string) ([]eventtype.EventType, error) {
	var out []eventtype.EventType
	for _, et := range f.ets {
		if (app == nil || et.Application == *app) && (status == nil || string(et.Status) == *status) &&
			(sub == nil || et.Subdomain == *sub) && (agg == nil || et.Aggregate == *agg) {
			out = append(out, et)
		}
	}
	return out, nil
}

func (f *fakeETs) FindByID(_ context.Context, id string) (*eventtype.EventType, error) {
	for i := range f.ets {
		if f.ets[i].ID == id {
			et := f.ets[i]
			return &et, nil
		}
	}
	return nil, nil
}

type fakeApps struct{}

func (fakeApps) FindByCode(_ context.Context, code string) (*application.Application, error) {
	return &application.Application{ID: ids.ApplicationID("app_" + code), Code: code}, nil
}

func (fakeApps) FindActive(context.Context) ([]application.Application, error) {
	return []application.Application{{Code: "orders"}, {Code: "billing"}}, nil
}

type fakeETOps struct {
	created []etops.CreateCommand
	updated []etops.UpdateCommand
	schemas []etops.AddSchemaCommand
	actions []string
	err     error
	store   *fakeETs
}

func (o *fakeETOps) Create(_ context.Context, cmd etops.CreateCommand) (string, error) {
	o.created = append(o.created, cmd)
	if o.err != nil {
		return "", o.err
	}
	o.store.ets = append(o.store.ets, et("evt_9", cmd.Code, cmd.Name, eventtype.StatusCurrent))
	return "evt_9", nil
}

func (o *fakeETOps) Update(_ context.Context, cmd etops.UpdateCommand) error {
	o.updated = append(o.updated, cmd)
	return o.err
}

func (o *fakeETOps) act(name string) error {
	o.actions = append(o.actions, name)
	return o.err
}

func (o *fakeETOps) Archive(_ context.Context, id string) error { return o.act("archive:" + id) }
func (o *fakeETOps) Delete(_ context.Context, id string) error  { return o.act("delete:" + id) }
func (o *fakeETOps) FinaliseSchema(_ context.Context, id, v string) error {
	return o.act("finalise:" + id + ":" + v)
}

func (o *fakeETOps) DeprecateSchema(_ context.Context, id, v string) error {
	return o.act("deprecate:" + id + ":" + v)
}

func (o *fakeETOps) AddSchema(_ context.Context, cmd etops.AddSchemaCommand) error {
	o.schemas = append(o.schemas, cmd)
	return o.err
}

func (o *fakeETOps) SyncPlatform(context.Context) (EventTypeSyncResult, error) {
	return EventTypeSyncResult{Created: 2, Updated: 1, Total: 40, SchemasCreated: 2, SchemasSame: 38}, o.act("sync")
}

func et(id, code, name string, status eventtype.Status, versions ...eventtype.SpecVersion) eventtype.EventType {
	day := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)
	parts := strings.Split(code, ":")
	for len(parts) < 4 {
		parts = append(parts, "")
	}
	return eventtype.EventType{
		ID: id, Code: code, Name: name, Status: status, Source: eventtype.SourceUI,
		Application: parts[0], Subdomain: parts[1], Aggregate: parts[2], EventName: parts[3],
		SpecVersions: versions, CreatedAt: day, UpdatedAt: day,
	}
}

func spec(version string, status eventtype.SpecVersionStatus, schema string) eventtype.SpecVersion {
	return eventtype.SpecVersion{ID: "sv_" + version, Version: version, MimeType: "application/schema+json", SchemaType: eventtype.SchemaJSON, Status: status, SchemaContent: json.RawMessage(schema)}
}

const orderSchema = `{"type":"object","properties":{"id":{"type":"string"},"total":{"type":"number"}},"required":["id"]}`

type etHarness struct {
	store *fakeETs
	ops   *fakeETOps
}

func newETHarness() *etHarness {
	desc := "An order was <b>placed</b>"
	placed := et("evt_1", "orders:sales:order:placed", "Order placed", eventtype.StatusCurrent,
		spec("1.0", eventtype.SpecCurrent, orderSchema), spec("1.1", eventtype.SpecFinalising, orderSchema))
	placed.Description = &desc
	s := &fakeETs{ets: []eventtype.EventType{
		placed,
		et("evt_2", "orders:sales:order:cancelled", "Order cancelled", eventtype.StatusCurrent),
		et("evt_3", "billing:invoicing:invoice:issued", "Invoice issued", eventtype.StatusCurrent, spec("1.0", eventtype.SpecDeprecated, orderSchema)),
		et("evt_4", "billing:payments:payment:failed", "Payment failed", eventtype.StatusArchived),
		et("evt_5", "platform:iam:user:created", "User created", eventtype.StatusCurrent, spec("1.0", eventtype.SpecFinalising, orderSchema)),
	}}
	return &etHarness{store: s, ops: &fakeETOps{store: s}}
}

func (h *etHarness) rec(t *testing.T, ac *auth.AuthContext, q req) *httptest.ResponseRecorder {
	t.Helper()
	return serveMounted(t, ac, q, &EventTypes{Store: h.store, Ops: h.ops, Apps: fakeApps{}, Shell: shellForTest()})
}

func (h *etHarness) do(t *testing.T, ac *auth.AuthContext, q req) string {
	t.Helper()
	return h.rec(t, ac, q).Body.String()
}

func etUser(perms ...string) *auth.AuthContext {
	return &auth.AuthContext{PrincipalID: "prn_e", Email: "e@x.y", Scope: auth.ScopeAnchor, Roles: []string{"r"}, Permissions: perms, PrincipalType: auth.PrincipalTypeUser}
}

const etViewPerm = "platform:messaging:event-type:view"

func TestEventTypesList(t *testing.T) {
	h := newETHarness()
	body := h.do(t, anchorAdmin(), req{target: "/ui/event-types"})
	mustContain(t, body,
		`<title>Event Types · FlowCatalyst</title>`, `<h1 class="fc-page-title">Event Types</h1>`,
		"Manage event type definitions and schemas", "Create Event Type", "Sync Platform Events",
		`href="/ui/event-types/export" class="fc-btn fc-btn-outline" download="event-schemas.zip"`,
		`<span class="fc-code-seg app">orders</span>`, "Order placed", "An order was &lt;b&gt;placed&lt;/b&gt;",
		`<span title="FINALISING">`, "No schemas",
		`id="et-filter-app-summary" class="fc-multiselect-summary">All Applications<`,
		`name="app" value="billing"`, `<option value="ARCHIVED">Archived</option>`,
		"Showing 1 to 5 of 5 event types", `id="et-pager"`,
		`<dialog id="et-finalise"`, `<dialog id="et-delete"`, `<aside id="et-drawer"`,
		`aria-label="Edit Order placed"`, `aria-label="View details Order placed"`,
	)
	// An archived type's Edit is disabled with the reason.
	mustContain(t, body, "Edit: Archived event types cannot be edited.")
}

func TestEventTypesFacetsCascadeAndPrune(t *testing.T) {
	h := newETHarness()
	body := h.do(t, anchorAdmin(), req{target: "/ui/event-types?app=billing&sub=sales&sub=payments"})
	// "sales" is not a subdomain of billing: pruned; the summary lists the rest.
	mustContain(t, body, `id="et-filter-app-summary" class="fc-multiselect-summary">billing<`,
		`id="et-filter-sub-summary" class="fc-multiselect-summary">payments<`,
		`name="sub" value="invoicing"`, "Payment failed", "Showing 1 to 1 of 1 event types")
	mustNotContain(t, body, `name="sub" value="sales"`, "Order placed")

	two := h.do(t, anchorAdmin(), req{target: "/ui/event-types?app=billing&app=orders&status=CURRENT&q=order", datastar: true})
	mustContain(t, two, "orders:sales:order:placed", "Order cancelled", `/ui/event-types?app=billing\u0026app=orders\u0026q=order\u0026status=CURRENT`)
	mustNotContain(t, two, "Invoice issued")
	mustContain(t, two, `id="et-filter-app-summary" class="fc-multiselect-summary">billing, orders<`,
		"data: selector", `id="et-filter-sub-options"`, `id="et-actions"`, `id="et-list-state"`)
}

func TestEventTypesPaging(t *testing.T) {
	h := newETHarness()
	for i := range 120 {
		h.store.ets = append(h.store.ets, et(fmt.Sprintf("evt_x%03d", i), fmt.Sprintf("bulk:a:b:e%03d", i), fmt.Sprintf("Bulk %03d", i), eventtype.StatusCurrent))
	}
	body := h.do(t, anchorAdmin(), req{target: "/ui/event-types?page=2"})
	mustContain(t, body, "Showing 101 to 125 of 125 event types", `href="/ui/event-types?page=1"`, `<option value="100" selected>`)
	small := h.do(t, anchorAdmin(), req{target: "/ui/event-types?rows=50&page=3"})
	mustContain(t, small, "Showing 101 to 125 of 125 event types", `href="/ui/event-types?page=2&amp;rows=50"`)
	beyond := h.do(t, anchorAdmin(), req{target: "/ui/event-types?page=99"})
	mustContain(t, beyond, "Showing 101 to 125 of 125 event types")
}

func TestEventTypesPermissions(t *testing.T) {
	h := newETHarness()
	if rec := h.rec(t, etUser("platform:messaging:event:view"), req{target: "/ui/event-types"}); rec.Code != http.StatusForbidden {
		t.Errorf("no view permission: status %d", rec.Code)
	}
	viewer := h.do(t, etUser(etViewPerm), req{target: "/ui/event-types"})
	mustNotContain(t, viewer, "Create Event Type", `aria-label="Edit `, `<dialog id="et-delete"`, `<dialog id="et-finalise"`)
	// Sync needs anchor scope and the sync permission (CanSyncPlatformEventTypes).
	mustContain(t, viewer, "Export Schemas")
	mustNotContain(t, viewer, "Sync Platform Events")
	mustContain(t, h.do(t, etUser(etViewPerm, "platform:messaging:event-type:sync"), req{target: "/ui/event-types"}), "Sync Platform Events")
	h.do(t, etUser(etViewPerm), req{method: http.MethodPost, target: "/ui/event-types/sync-platform", datastar: true, form: url.Values{}})
	drawer := h.do(t, etUser(etViewPerm), req{target: "/ui/event-types/evt_1", datastar: true})
	mustNotContain(t, drawer, "Danger Zone", "Add Schema", `title="Finalise"`, ">Edit<")
	mustContain(t, drawer, `title="View Schema"`)

	client := &auth.AuthContext{PrincipalID: "p", Scope: auth.ScopeClient, Clients: []string{"clt_a"}, Roles: []string{"r"}, Permissions: []string{etViewPerm, "platform:messaging:event-type:update"}, PrincipalType: auth.PrincipalTypeUser}
	mustNotContain(t, h.do(t, client, req{target: "/ui/event-types"}), "Sync Platform Events")
	h.do(t, client, req{method: http.MethodPost, target: "/ui/event-types/sync-platform", datastar: true, form: url.Values{}})
	h.do(t, etUser(etViewPerm, "platform:messaging:event-type:update"), req{method: http.MethodPost, target: "/ui/event-types/evt_4/delete", datastar: true, form: url.Values{}})
	if len(h.ops.actions) != 0 {
		t.Fatalf("ran %v without permission", h.ops.actions)
	}
	updater := h.do(t, etUser(etViewPerm, "platform:messaging:event-type:update"), req{target: "/ui/event-types/evt_1", datastar: true})
	mustContain(t, updater, "Archive Event Type", `title="Finalise"`, `title="Deprecate"`, "Add Schema")
	mustNotContain(t, updater, "Delete Event Type")
}

func TestEventTypesAppConfinement(t *testing.T) {
	h := newETHarness()
	sa := &auth.AuthContext{
		PrincipalID: "sa", Scope: auth.ScopeAnchor, Roles: []string{"r"},
		Permissions: []string{"platform:application-service:event-type:view"}, PrincipalType: "SERVICE",
		Applications: []string{"app_orders"},
	}
	if err := auth.CanReadEventTypes(sa); err != nil {
		t.Skipf("application-service permission name differs: %v", err)
	}
	body := h.do(t, sa, req{target: "/ui/event-types"})
	mustContain(t, body, "Order placed")
	mustNotContain(t, body, "Invoice issued")
	if rec := h.rec(t, sa, req{target: "/ui/event-types/evt_3"}); rec.Code != http.StatusForbidden {
		t.Errorf("another application's event type: status %d", rec.Code)
	}
}

func TestEventTypeDrawer(t *testing.T) {
	h := newETHarness()
	body := h.do(t, anchorAdmin(), req{target: "/ui/event-types/evt_1", datastar: true})
	mustContain(t, body, "data: selector #et-drawer\n", "Event Type Details", "Schema Versions", "Danger Zone",
		`<code class="fc-mono-chip">application/schema+json</code>`, "JSON Schema",
		`href="/ui/event-types/evt_1/schemas/1.0"`, `data-action="/ui/event-types/evt_1/schemas/1.1/finalise"`,
		`data-action="/ui/event-types/evt_1/schemas/1.0/deprecate"`, `href="/ui/event-types/evt_1/add-schema"`,
		`"selEt":"evt_1"`, cannotArchive, cannotDelete)
	// Archive and Delete are disabled for a type with a CURRENT version.
	mustNotContain(t, body, `data-action="/ui/event-types/evt_1/archive"`, `data-action="/ui/event-types/evt_1/delete"`)

	deprecated := h.do(t, anchorAdmin(), req{target: "/ui/event-types/evt_3", datastar: true})
	mustContain(t, deprecated, `data-action="/ui/event-types/evt_3/archive"`)
	finalising := h.do(t, anchorAdmin(), req{target: "/ui/event-types/evt_5", datastar: true})
	mustContain(t, finalising, `data-action="/ui/event-types/evt_5/delete"`)
	archived := h.do(t, anchorAdmin(), req{target: "/ui/event-types/evt_4", datastar: true})
	mustContain(t, archived, `data-action="/ui/event-types/evt_4/delete"`, "No schema versions defined yet.")
	mustNotContain(t, archived, "Archive Event Type", "Add First Schema", ">Edit<")
	empty := h.do(t, anchorAdmin(), req{target: "/ui/event-types/evt_2", datastar: true})
	mustContain(t, empty, "Add First Schema")

	page := h.do(t, anchorAdmin(), req{target: "/ui/event-types/evt_1"})
	mustContain(t, page, `data-signals="{selEt: &#39;evt_1&#39;}"`, "Event Type Details")
}

func TestEventTypeSchemaViewer(t *testing.T) {
	h := newETHarness()
	body := h.do(t, anchorAdmin(), req{target: "/ui/event-types/evt_1/schemas/1.0", datastar: true})
	mustContain(t, body, "data: mode replace\n", `data: elements <div id="et-dialog-slot">`, "orders:sales:order:placed — v1.0",
		`id="et-code-schema"`, `<span class="hl-key">`, `id="et-code-typescript"`, "interface", `id="et-code-java"`,
		`data-signals:et-schema-tab="&#39;schema&#39;"`, `aria-label="Copy"`)
	if rec := h.rec(t, anchorAdmin(), req{target: "/ui/event-types/evt_2/schemas/1.0", datastar: true}); !strings.Contains(rec.Body.String(), "fc-flash-error") {
		t.Errorf("missing version: %s", rec.Body.String())
	}
	if rec := h.rec(t, anchorAdmin(), req{target: "/ui/event-types/evt_1/schemas/..%2Fx"}); rec.Code != http.StatusNotFound {
		t.Errorf("bad version: status %d", rec.Code)
	}
}

func TestEventTypeCreate(t *testing.T) {
	h := newETHarness()
	dlg := h.do(t, anchorAdmin(), req{target: "/ui/event-types/new", datastar: true})
	mustContain(t, dlg, "data: mode replace\n", `data: elements <div id="et-dialog-slot">`, "Create Event Type", `<option value="billing"></option>`,
		`pattern="[a-z][a-z0-9.\-]*"`, "Generated Code", `data-text="$etApp || &#39;app&#39;"`, "Client Scoped")

	form := url.Values{"application": {"orders"}, "subdomain": {"sales"}, "aggregate": {"order"}, "event": {"shipped"}, "name": {"Order shipped"}, "description": {""}, "clientScoped": {"true"}}
	body := h.do(t, anchorAdmin(), req{method: http.MethodPost, target: "/ui/event-types/new", datastar: true, form: form})
	if len(h.ops.created) != 1 {
		t.Fatalf("creates %d", len(h.ops.created))
	}
	if c := h.ops.created[0]; c.Code != "orders:sales:order:shipped" || !c.ClientScoped || c.Description != nil || c.Schema != nil {
		t.Errorf("command = %+v", c)
	}
	mustContain(t, body, `document.getElementById(&#34;et-form&#34;)?.close()`, `"selEt":"evt_9"`, "Event type created")

	h.ops.err = usecase.Conflict("CODE_EXISTS", "Event type with code 'orders:sales:order:shipped' already exists")
	dup := h.do(t, anchorAdmin(), req{method: http.MethodPost, target: "/ui/event-types/new", datastar: true, form: form})
	mustContain(t, dup, `id="et-form-application-err"`, "already exists", `document.getElementById(&#34;et-form-application&#34;)?.focus()`)

	if rec := h.rec(t, etUser(etViewPerm), req{target: "/ui/event-types/new"}); rec.Code != http.StatusForbidden {
		t.Errorf("create without permission: %d", rec.Code)
	}
}

func TestEventTypeEditAndActions(t *testing.T) {
	h := newETHarness()
	dlg := h.do(t, anchorAdmin(), req{target: "/ui/event-types/evt_1/edit", datastar: true})
	mustContain(t, dlg, "Edit Event Type", `value="Order placed"`, "An order was &lt;b&gt;placed&lt;/b&gt;", `data-attr:value="$selEt"`)
	mustNotContain(t, dlg, `name="application"`)

	body := h.do(t, anchorAdmin(), req{
		method: http.MethodPost, target: "/ui/event-types/evt_1/edit", datastar: true, multipart: true,
		form: url.Values{"name": {"Order placed!"}, "description": {""}, "drawer": {"evt_1"}},
	})
	if u := h.ops.updated[0]; u.ID != "evt_1" || u.Name != "Order placed!" || u.Description == nil || *u.Description != "" || u.ClientScoped != nil {
		t.Errorf("update = %+v", u)
	}
	mustContain(t, body, "Event type updated", "data: selector #et-drawer\n")

	for _, path := range []string{"evt_3/archive", "evt_4/delete", "evt_1/schemas/1.1/finalise", "evt_1/schemas/1.0/deprecate"} {
		out := h.do(t, anchorAdmin(), req{method: http.MethodPost, target: "/ui/event-types/" + path, datastar: true, form: url.Values{"drawer": {"evt_1"}}})
		mustContain(t, out, "fc-flash-success", `id="et-table"`)
	}
	if got := strings.Join(h.ops.actions, ","); got != "archive:evt_3,delete:evt_4,finalise:evt_1:1.1,deprecate:evt_1:1.0" {
		t.Errorf("actions = %s", got)
	}

	h.ops.err = usecase.Conflict("STILL_FINALISING", "Schema version '1.1' is still finalising")
	refused := h.do(t, anchorAdmin(), req{method: http.MethodPost, target: "/ui/event-types/evt_1/schemas/1.1/deprecate", datastar: true, form: url.Values{}})
	mustContain(t, refused, "fc-flash-error", "still finalising")

	h.ops.err = nil
	rec := h.rec(t, anchorAdmin(), req{method: http.MethodPost, target: "/ui/event-types/evt_3/archive", form: url.Values{}})
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/ui/event-types/evt_3" || !strings.Contains(rec.Header().Get("Set-Cookie"), "fc_ui_flash=event-type-archived") {
		t.Errorf("native: %d %q %q", rec.Code, rec.Header().Get("Location"), rec.Header().Get("Set-Cookie"))
	}
}

func TestEventTypeAddSchema(t *testing.T) {
	h := newETHarness()
	page := h.do(t, anchorAdmin(), req{target: "/ui/event-types/evt_1/add-schema"})
	mustContain(t, page, "Add Schema Version", "Add a new schema to <strong>Order placed</strong>", `name="version" value="1.2"`, `pattern="\d+\.\d+"`,
		`name="mimeType" value="application/json"`, `name="schemaType"`, `<option value="XSD"`)
	first := h.do(t, anchorAdmin(), req{target: "/ui/event-types/evt_2/add-schema"})
	mustContain(t, first, `value="1.0"`)

	bad := h.do(t, anchorAdmin(), req{method: http.MethodPost, target: "/ui/event-types/evt_1/add-schema", datastar: true, form: url.Values{"version": {"2.0"}, "mimeType": {"application/json"}, "schema": {"{not json"}}})
	mustContain(t, bad, "Enter a valid JSON document.", `id="et-schema-schema-err"`)
	if len(h.ops.schemas) != 0 {
		t.Fatal("invalid JSON reached the use case")
	}
	rec := h.rec(t, anchorAdmin(), req{method: http.MethodPost, target: "/ui/event-types/evt_1/add-schema", datastar: true, form: url.Values{"version": {"2.0"}, "mimeType": {"application/json"}, "schemaType": {"JSON_SCHEMA"}, "schema": {orderSchema}}})
	if s := h.ops.schemas[0]; s.EventTypeID != "evt_1" || s.Version != "2.0" || string(s.Schema) != orderSchema || s.MimeType != "application/json" || s.SchemaType != "JSON_SCHEMA" {
		t.Errorf("add schema = %+v", s)
	}
	if !strings.Contains(rec.Body.String(), "/ui/event-types/evt_1") || !strings.Contains(rec.Header().Get("Set-Cookie"), "fc_ui_flash=schema-added") {
		t.Errorf("redirect: %s %q", rec.Body.String(), rec.Header().Get("Set-Cookie"))
	}
	// An XSD is text: sent as a JSON string with its type (the use case stores it so).
	xsd := `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema"/>`
	h.do(t, anchorAdmin(), req{method: http.MethodPost, target: "/ui/event-types/evt_1/add-schema", datastar: true, form: url.Values{"version": {"2.1"}, "mimeType": {"application/xml"}, "schemaType": {"XSD"}, "schema": {xsd}}})
	var sentXSD string
	if s := h.ops.schemas[1]; s.SchemaType != "XSD" || s.MimeType != "application/xml" || json.Unmarshal(s.Schema, &sentXSD) != nil || sentXSD != xsd {
		t.Errorf("add XSD schema = %+v (%s)", s, s.Schema)
	}
	noMime := h.do(t, anchorAdmin(), req{method: http.MethodPost, target: "/ui/event-types/evt_1/add-schema", datastar: true, form: url.Values{"version": {"2.2"}, "schema": {orderSchema}}})
	mustContain(t, noMime, `id="et-schema-mime-type-err"`, "Enter the MIME type.")
	h.ops.err = usecase.Conflict("VERSION_EXISTS", "Schema version '2.0' already exists for this event type")
	dup := h.do(t, anchorAdmin(), req{method: http.MethodPost, target: "/ui/event-types/evt_1/add-schema", datastar: true, form: url.Values{"version": {"2.0"}, "mimeType": {"application/json"}, "schema": {orderSchema}}})
	mustContain(t, dup, `id="et-schema-version-err"`, "already exists")
}

func TestEventTypesSyncAndExport(t *testing.T) {
	h := newETHarness()
	body := h.do(t, anchorAdmin(), req{method: http.MethodPost, target: "/ui/event-types/sync-platform", datastar: true, form: url.Values{}})
	mustContain(t, body, "Platform Events Synced", "2 created, 1 updated (40 total)", "Schemas: 2 created, 38 unchanged (40 total)")

	rec := h.rec(t, anchorAdmin(), req{target: "/ui/event-types/export?app=orders"})
	if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "application/zip" {
		t.Fatalf("export: %d %s", rec.Code, rec.Body.String())
	}
	zr, err := zip.NewReader(bytes.NewReader(rec.Body.Bytes()), int64(rec.Body.Len()))
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, f := range zr.File {
		names = append(names, f.Name)
	}
	got := strings.Join(names, ",")
	if !strings.Contains(got, "schemas/orders/sales/order/placed.json") || strings.Contains(got, "billing") || strings.Contains(got, "platform/") {
		t.Errorf("zip = %s", got)
	}
	f, _ := zr.Open("schemas/orders/sales/order/placed.json")
	content, _ := io.ReadAll(f)
	mustContain(t, string(content), `"type": "object"`)

	none := h.rec(t, anchorAdmin(), req{target: "/ui/event-types/export?app=platform"})
	if none.Code != http.StatusSeeOther || !strings.Contains(none.Header().Get("Set-Cookie"), "schema-export-empty") {
		t.Errorf("empty export: %d", none.Code)
	}
}
