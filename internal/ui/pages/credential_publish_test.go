package pages

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	platformauth "github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/credentialpublish"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/serviceaccount"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/go-sdk/usecase"
)

// The publishing cfg of the tests: Parameter Store under /flowcatalyst/np/.
func pubTestConfig(defaultOn bool) credentialpublish.Config {
	return credentialpublish.Config{
		Store: credentialpublish.StoreAWSParameterStore, Prefix: "/flowcatalyst/np/",
		ServiceAccountPath: credentialpublish.DefaultServiceAccountPath, OAuthClientPath: credentialpublish.DefaultOAuthClientPath,
		DefaultOn: defaultOn,
	}
}

type pubEnableCall struct {
	owner credentialpublish.Owner
	path  *string
}

// fakePub is CredentialPublishing over canned state.
type fakePub struct {
	cfg      credentialpublish.Config
	pubs     map[credentialpublish.Owner]*credentialpublish.Publication
	items    map[credentialpublish.Owner][]credentialpublish.Item
	linked   map[string]string
	enabled  []pubEnableCall
	disabled []credentialpublish.Owner
	retried  []credentialpublish.Owner
	rotated  []string
	err      error
}

func newFakePub(defaultOn bool) *fakePub {
	return &fakePub{
		cfg:  pubTestConfig(defaultOn),
		pubs: map[credentialpublish.Owner]*credentialpublish.Publication{}, items: map[credentialpublish.Owner][]credentialpublish.Item{},
		linked: map[string]string{},
	}
}

func (f *fakePub) Config() credentialpublish.Config { return f.cfg }

func (f *fakePub) Status(_ context.Context, o credentialpublish.Owner) (*credentialpublish.Publication, []credentialpublish.Item, error) {
	return f.pubs[o], f.items[o], nil
}

func (f *fakePub) LinkedServiceAccount(_ context.Context, id string) (string, error) {
	return f.linked[id], nil
}

func (f *fakePub) Enable(_ context.Context, o credentialpublish.Owner, path *string) error {
	f.enabled = append(f.enabled, pubEnableCall{o, path})
	return f.err
}

func (f *fakePub) Disable(_ context.Context, o credentialpublish.Owner) error {
	f.disabled = append(f.disabled, o)
	return f.err
}

func (f *fakePub) Retry(_ context.Context, o credentialpublish.Owner) error {
	f.retried = append(f.retried, o)
	return f.err
}

func (f *fakePub) RotateClientSecret(_ context.Context, id string) (RotatedOAuthSecret, error) {
	f.rotated = append(f.rotated, id)
	return RotatedOAuthSecret{ClientID: "oauth_orders_public", Secret: "rotated-and-published"}, f.err
}

// turnOn records the account as published under base with the given items.
func (f *fakePub) turnOn(o credentialpublish.Owner, base string, items ...credentialpublish.Item) {
	f.pubs[o] = &credentialpublish.Publication{Owner: o, Store: "aws-ps", BasePath: base}
	f.items[o] = items
}

func pubItem(cred, base, state string) credentialpublish.Item {
	return credentialpublish.Item{Credential: cred, Path: base + cred, State: state}
}

const saPubBase = "/flowcatalyst/np/service-accounts/orders-svc/"

var saOwner1 = credentialpublish.Owner{Type: credentialpublish.OwnerServiceAccount, ID: "sa_1"}

func (h *saHarness) pubPage(p CredentialPublishing) *ServiceAccounts {
	page := h.page()
	page.Publish = p
	page.OAuthClients = fakeSAOAuthRows{}
	return page
}

func (h *saHarness) doPub(t *testing.T, p CredentialPublishing, q req) string {
	t.Helper()
	return serveMounted(t, saAdmin(), q, h.pubPage(p)).Body.String()
}

func TestPublishingControlsAreHiddenWhenNotConfigured(t *testing.T) {
	h := newSAHarness()
	// Not wired at all, and wired but with no store configured: nothing shows.
	for name, p := range map[string]CredentialPublishing{"nil": nil, "unconfigured": &fakePub{}} {
		drawer := h.doPub(t, p, req{target: "/ui/service-accounts/sa_1", datastar: true})
		mustNotContain(t, drawer, "Parameter Store", "Publish credentials", "pub-stop")
		dlg := h.doPub(t, p, req{target: "/ui/service-accounts/new", datastar: true})
		mustNotContain(t, dlg, "Parameter Store", "publishCredentials", "publishBasePath")
		if name == "unconfigured" {
			// A create then asks the use case for nothing: the default stands.
			h.ops.created = nil
			h.doPub(t, p, req{method: http.MethodPost, target: "/ui/service-accounts/new", datastar: true, form: url.Values{
				"name": {"X"}, "code": {"x"}, "scope": {"ANCHOR"}, "publishCredentials": {"true"},
			}})
			if len(h.ops.created) != 1 || h.ops.created[0].PublishCredentials != nil || h.ops.created[0].PublishBasePath != nil {
				t.Errorf("an unconfigured deployment sent a publishing choice: %+v", h.ops.created)
			}
		}
	}
	// The OAuth client and application pages say nothing either.
	oc := newOCHarness()
	page := oc.page()
	page.Publish = &fakePub{}
	body := serveMounted(t, anchorAdmin(), req{target: "/ui/oauth-clients/oc_1", datastar: true}, page).Body.String()
	mustNotContain(t, body, "Parameter Store", "Publish credentials")
	create := serveMounted(t, anchorAdmin(), req{target: "/ui/oauth-clients/new", datastar: true}, page).Body.String()
	mustNotContain(t, create, "Parameter Store", "publishCredentials")
	ah := newAppHarness()
	apage := &Applications{Apps: ah.store, Ops: ah.ops, Logins: fakeLogins{"app_1": true}, Docs: fakeDocs{}, Shell: shellForTest()}
	abody := serveMounted(t, anchorAdmin(), req{target: "/ui/applications/app_2", datastar: true}, apage).Body.String()
	mustContain(t, abody, "Provision Service Account")
	mustNotContain(t, abody, "Parameter Store", "publishCredentials")
}

func TestPublishCheckboxFollowsTheDeploymentDefault(t *testing.T) {
	h := newSAHarness()
	off := h.doPub(t, newFakePub(false), req{target: "/ui/service-accounts/new", datastar: true})
	mustContain(t, off, "Publish credentials to Parameter Store", `name="publishCredentials"`, `name="publishBasePath"`, "Custom path")
	if ticked(boxTag(t, off)) {
		t.Errorf("the box starts ticked while the default is off: %s", boxTag(t, off))
	}
	on := h.doPub(t, newFakePub(true), req{target: "/ui/service-accounts/new", datastar: true})
	if !ticked(boxTag(t, on)) {
		t.Errorf("the box starts unticked while the default is on: %s", boxTag(t, on))
	}
	// The same on the OAuth client dialog.
	oc := newOCHarness()
	page := oc.page()
	page.Publish = newFakePub(true)
	dlg := serveMounted(t, anchorAdmin(), req{target: "/ui/oauth-clients/new", datastar: true}, page).Body.String()
	if !ticked(boxTag(t, dlg)) {
		t.Errorf("oauth client box: %s", boxTag(t, dlg))
	}
}

// ticked reports whether the checkbox <input> carries the checked attribute
// (its class also holds the checked: variants).
func ticked(tag string) bool {
	return strings.Contains(tag, ` checked `) || strings.Contains(tag, ` checked>`) || strings.Contains(tag, ` checked=`)
}

// boxTag is the publish checkbox's <input>.
func boxTag(t *testing.T, body string) string {
	t.Helper()
	i := strings.Index(body, `name="publishCredentials"`)
	if i < 0 {
		t.Fatalf("no publish checkbox in %s", body)
	}
	start := strings.LastIndex(body[:i], "<input")
	end := i + strings.Index(body[i:], ">")
	return body[start : end+1]
}

func TestPublishPathPreview(t *testing.T) {
	h := newSAHarness()
	body := h.doPub(t, newFakePub(false), req{target: "/ui/service-accounts/new", datastar: true})
	// The service account's follows the code typed, and the application
	// account rule (':' to '.') is applied in the browser.
	mustContain(t, body, "Parameters are written under", `data-text="`, "/flowcatalyst/np/service-accounts/fcph/",
		"$saFormCode || &#39;&lt;code&gt;&#39;).replaceAll(&#39;:&#39;, &#39;.&#39;)", "$saFormPubPath.trim()", `data-bind:sa-form-pub-path`)
	// An application's is fixed: app:<code>, with the ':' written '.'.
	ah := newAppHarness()
	ah.ops.store = ah.store
	apage := &Applications{Apps: ah.store, Ops: ah.ops, Logins: fakeLogins{"app_1": true}, Docs: fakeDocs{}, Publish: newFakePub(false), Shell: shellForTest()}
	abody := serveMounted(t, anchorAdmin(), req{target: "/ui/applications/app_2", datastar: true}, apage).Body.String()
	mustContain(t, abody, "Publish credentials to Parameter Store", "/flowcatalyst/np/service-accounts/app.billing/")
	// An OAuth client's id is not known before it is made.
	oc := newOCHarness()
	page := oc.page()
	page.Publish = newFakePub(false)
	obody := serveMounted(t, anchorAdmin(), req{target: "/ui/oauth-clients/new", datastar: true}, page).Body.String()
	mustContain(t, obody, "/flowcatalyst/np/oauth-clients/&lt;client id&gt;/", "$ocFormType === &#39;CONFIDENTIAL&#39;")
}

func TestPublishTemplateThatNeedsAnApplicationSaysSo(t *testing.T) {
	h := newSAHarness()
	p := newFakePub(false)
	p.cfg.ServiceAccountPath = "{prefix}apps/{app}/{credential}"
	body := h.doPub(t, p, req{target: "/ui/service-accounts/new", datastar: true})
	mustContain(t, body, "has no value here", "Custom path")
}

func TestServiceAccountCreatePublishesAsAsked(t *testing.T) {
	h := newSAHarness()
	p := newFakePub(false)
	// What the create queued: the reveal names each parameter.
	created := credentialpublish.Owner{Type: credentialpublish.OwnerServiceAccount, ID: "sa_9"}
	base := "/flowcatalyst/np/service-accounts/shipping/"
	p.items[created] = []credentialpublish.Item{
		pubItem(credentialpublish.CredClientID, base, credentialpublish.StatePendingPut),
		pubItem(credentialpublish.CredClientSecret, base, credentialpublish.StatePendingPut),
		pubItem(credentialpublish.CredAuthToken, base, credentialpublish.StatePublished),
		{Credential: credentialpublish.CredSigningSecret, Path: base + "signing-secret", State: credentialpublish.StatePendingDelete},
	}
	body := h.doPub(t, p, req{method: http.MethodPost, target: "/ui/service-accounts/new", datastar: true, multipart: true, form: url.Values{
		"name": {"Shipping"}, "code": {"shipping"}, "scope": {"ANCHOR"}, "publishCredentials": {"true"}, "publishBasePath": {" /flowcatalyst/np/custom/ "},
	}})
	c := h.ops.created[0]
	if c.PublishCredentials == nil || !*c.PublishCredentials || deref(c.PublishBasePath) != "/flowcatalyst/np/custom/" {
		t.Errorf("publishing choice = %v %v", c.PublishCredentials, c.PublishBasePath)
	}
	mustContain(t, body, "Also published to", base+"client-id", base+"client-secret", base+"auth-token")
	// Only what was queued or written is named.
	mustNotContain(t, body, `Also published to <code class="fc-mono-chip break-all">`+base+"signing-secret")

	// An unticked box is sent as such, whatever the default.
	h.ops.created = nil
	h.doPub(t, newFakePub(true), req{method: http.MethodPost, target: "/ui/service-accounts/new", datastar: true, form: url.Values{
		"name": {"Plain"}, "code": {"plain"}, "scope": {"ANCHOR"},
	}})
	if c := h.ops.created[0]; c.PublishCredentials == nil || *c.PublishCredentials || c.PublishBasePath != nil {
		t.Errorf("unticked = %v %v", c.PublishCredentials, c.PublishBasePath)
	}
}

func TestServiceAccountCreatePublishRefusalsLandOnTheirFields(t *testing.T) {
	h := newSAHarness()
	h.ops.err = usecase.Validation(credentialpublish.CodePathInvalid, "publishBasePath: the path must lie under /flowcatalyst/np/")
	body := h.doPub(t, newFakePub(false), req{method: http.MethodPost, target: "/ui/service-accounts/new", datastar: true, form: url.Values{
		"name": {"Shipping"}, "code": {"shipping"}, "scope": {"ANCHOR"}, "publishCredentials": {"true"}, "publishBasePath": {"/elsewhere/"},
	}})
	// On the path field, without the wire field's name, not in the form banner.
	mustContain(t, body, `id="sa-form-publish-path-err"`, "The path must lie under /flowcatalyst/np/", `value="/elsewhere/"`)
	mustNotContain(t, body, "PublishBasePath:", "publishBasePath: the")
	if strings.Contains(body, `id="sa-form-error"`) && strings.Contains(body, "must lie under") && !strings.Contains(body, `role="alert">The path must lie`) {
		t.Errorf("the refusal is not on the field: %s", body)
	}
}

func TestServiceAccountDrawerSectionOff(t *testing.T) {
	h := newSAHarness()
	body := h.doPub(t, newFakePub(false), req{target: "/ui/service-accounts/sa_1", datastar: true})
	mustContain(t, body, "Parameter Store", "Not published", "Publish credentials", `href="/ui/service-accounts/sa_1/publish"`)
	mustNotContain(t, body, "Stop publishing", "Retry now")
	// Without the update permission the button is not offered.
	limited := serveMounted(t, saUser(saPermView), req{target: "/ui/service-accounts/sa_1", datastar: true}, h.pubPage(newFakePub(false))).Body.String()
	mustContain(t, limited, "Not published")
	mustNotContain(t, limited, `href="/ui/service-accounts/sa_1/publish"`)

	// The dialog shows the default path and the custom path.
	dlg := h.doPub(t, newFakePub(false), req{target: "/ui/service-accounts/sa_1/publish", datastar: true})
	mustContain(t, dlg, "Publish credentials", saPubBase, `name="publishBasePath"`, "lie under /flowcatalyst/np/")
	dotted := h.doPub(t, newFakePub(false), req{target: "/ui/service-accounts/sa_3/publish", datastar: true})
	mustContain(t, dotted, "/flowcatalyst/np/service-accounts/app-bound/")
}

func TestServiceAccountEnablePublishing(t *testing.T) {
	h := newSAHarness()
	p := newFakePub(false)
	body := h.doPub(t, p, req{method: http.MethodPost, target: "/ui/service-accounts/sa_1/publish", datastar: true, form: url.Values{
		"drawer": {"sa_1"}, "publishBasePath": {" /flowcatalyst/np/orders/ "},
	}})
	if len(p.enabled) != 1 || p.enabled[0].owner != saOwner1 || deref(p.enabled[0].path) != "/flowcatalyst/np/orders/" {
		t.Fatalf("enable calls = %+v", p.enabled)
	}
	mustContain(t, body, "Credential publishing switched on", "sa-drawer")
	// No custom path: the template's.
	p.enabled = nil
	h.doPub(t, p, req{method: http.MethodPost, target: "/ui/service-accounts/sa_1/publish", datastar: true, form: url.Values{"drawer": {"sa_1"}}})
	if len(p.enabled) != 1 || p.enabled[0].path != nil {
		t.Errorf("enable without a path = %+v", p.enabled)
	}
	// A refused path is shown on the field.
	p.err = usecase.Validation(credentialpublish.CodePathInvalid, "publishBasePath: the path must lie under /flowcatalyst/np/")
	refused := h.doPub(t, p, req{method: http.MethodPost, target: "/ui/service-accounts/sa_1/publish", datastar: true, form: url.Values{"publishBasePath": {"/x/"}}})
	mustContain(t, refused, `id="pub-form-path-err"`, "The path must lie under /flowcatalyst/np/")
	// Nothing is done when publishing is not configured, or without permission.
	p2 := newFakePub(false)
	p2.cfg = credentialpublish.Config{}
	h.doPub(t, p2, req{method: http.MethodPost, target: "/ui/service-accounts/sa_1/publish", datastar: true, form: url.Values{}})
	if len(p2.enabled) != 0 {
		t.Error("enabled while not configured")
	}
	p3 := newFakePub(false)
	serveMounted(t, saUser(saPermView), req{method: http.MethodPost, target: "/ui/service-accounts/sa_1/publish", datastar: true, form: url.Values{}}, h.pubPage(p3))
	if len(p3.enabled) != 0 {
		t.Error("enabled without the update permission")
	}
}

func TestServiceAccountDrawerSectionStates(t *testing.T) {
	h := newSAHarness()
	p := newFakePub(false)
	retry := time.Date(2026, 11, 25, 12, 31, 0, 0, time.UTC)
	done := time.Date(2026, 11, 25, 12, 1, 0, 0, time.UTC)
	failing := pubItem(credentialpublish.CredAuthToken, saPubBase, credentialpublish.StatePendingPut)
	failing.Attempts, failing.LastError, failing.NextAttemptAt = 3, "AccessDeniedException: no", &retry
	published := pubItem(credentialpublish.CredClientID, saPubBase, credentialpublish.StatePublished)
	published.PublishedAt = &done
	p.turnOn(saOwner1, saPubBase,
		published,
		pubItem(credentialpublish.CredClientSecret, saPubBase, credentialpublish.StateNotPublishable),
		failing,
		pubItem(credentialpublish.CredSigningSecret, saPubBase, credentialpublish.StatePendingPut),
	)
	body := h.doPub(t, p, req{target: "/ui/service-accounts/sa_1", datastar: true})
	mustContain(t, body,
		"Client ID", "Client secret", "Auth token", "Signing secret",
		saPubBase+"client-id", "aws-ps://"+saPubBase+"client-id", `data-copy="`+saPubBase+"client-id"+`"`, `data-copy="aws-ps://`+saPubBase+`client-id"`,
		"Published", `datetime="2026-11-25T12:01:00Z"`,
		"Publishing…",
		"Failing: AccessDeniedException: no, retrying at", `datetime="2026-11-25T12:31:00Z"`,
		"Not published: rotate the client secret to publish it", "Rotate and publish", `data-action="/ui/service-accounts/sa_1/publish/rotate"`,
		"Retry now", `action="/ui/service-accounts/sa_1/publish/retry"`, "Stop publishing")
	mustNotContain(t, body, "Removing…")

	// With nothing failing there is no retry; a removal is shown as one.
	q := newFakePub(false)
	gone := pubItem(credentialpublish.CredClientID, saPubBase, credentialpublish.StatePendingDelete)
	q.turnOn(saOwner1, saPubBase, gone)
	calm := h.doPub(t, q, req{target: "/ui/service-accounts/sa_1", datastar: true})
	mustContain(t, calm, "Removing…")
	mustNotContain(t, calm, "Retry now")

	// After a disable the items remain with no publication: "Not published"
	// and the removals in progress, and the button to publish again.
	r := newFakePub(false)
	r.items[saOwner1] = []credentialpublish.Item{gone}
	off := h.doPub(t, r, req{target: "/ui/service-accounts/sa_1", datastar: true})
	mustContain(t, off, "Not published", "Removing…", "Publish credentials")
	mustNotContain(t, off, "Stop publishing")

	// "Rotate and publish" needs the rotation's permission as well.
	noRotate := serveMounted(t, saUser(saPermView, saPermUpdate), req{target: "/ui/service-accounts/sa_1", datastar: true}, h.pubPage(p)).Body.String()
	mustContain(t, noRotate, "Not published: rotate the client secret to publish it")
	mustNotContain(t, noRotate, "Rotate and publish")
}

func TestServiceAccountStopPublishingConfirmListsThePaths(t *testing.T) {
	h := newSAHarness()
	p := newFakePub(false)
	p.turnOn(saOwner1, saPubBase,
		pubItem(credentialpublish.CredClientID, saPubBase, credentialpublish.StatePublished),
		pubItem(credentialpublish.CredClientSecret, saPubBase, credentialpublish.StateNotPublishable),
		pubItem(credentialpublish.CredAuthToken, saPubBase, credentialpublish.StatePublished),
	)
	body := h.doPub(t, p, req{target: "/ui/service-accounts/sa_1", datastar: true})
	// The secret that was never published is not among the parameters deleted.
	mustContain(t, body, `<dialog id="pub-stop"`, "Deletes these 2 parameters from Parameter Store. The credentials in FlowCatalyst are not changed.",
		`<li><code class="fc-mono-chip break-all">`+saPubBase+`client-id</code></li>`, `<li><code class="fc-mono-chip break-all">`+saPubBase+`auth-token</code></li>`,
		`action="/ui/service-accounts/sa_1/publish/stop"`)
	mustNotContain(t, body, `<li><code class="fc-mono-chip break-all">`+saPubBase+`client-secret</code></li>`)

	h.doPub(t, p, req{method: http.MethodPost, target: "/ui/service-accounts/sa_1/publish/stop", datastar: true, form: url.Values{"drawer": {"sa_1"}}})
	if len(p.disabled) != 1 || p.disabled[0] != saOwner1 {
		t.Errorf("disabled = %+v", p.disabled)
	}
	h.doPub(t, p, req{method: http.MethodPost, target: "/ui/service-accounts/sa_1/publish/retry", datastar: true, form: url.Values{"drawer": {"sa_1"}}})
	if len(p.retried) != 1 || p.retried[0] != saOwner1 {
		t.Errorf("retried = %+v", p.retried)
	}
	// Without the update permission neither runs.
	serveMounted(t, saUser(saPermView), req{method: http.MethodPost, target: "/ui/service-accounts/sa_1/publish/stop", datastar: true, form: url.Values{}}, h.pubPage(p))
	if len(p.disabled) != 1 {
		t.Error("stopped without the update permission")
	}
}

func TestServiceAccountRegenerateRevealsWhereItWasPublished(t *testing.T) {
	h := newSAHarness()
	p := newFakePub(false)
	p.turnOn(saOwner1, saPubBase, pubItem(credentialpublish.CredAuthToken, saPubBase, credentialpublish.StatePendingPut))
	body := h.doPub(t, p, req{method: http.MethodPost, target: "/ui/service-accounts/sa_1/regenerate-token", datastar: true, form: url.Values{"drawer": {"sa_1"}}})
	mustContain(t, body, "fc_regenerated_token", "Also published to", saPubBase+"auth-token")
	// A secret that is not published says nothing.
	nb := h.doPub(t, newFakePub(false), req{method: http.MethodPost, target: "/ui/service-accounts/sa_1/regenerate-secret", datastar: true, form: url.Values{"drawer": {"sa_1"}}})
	mustContain(t, nb, "regenerated-signing-secret")
	mustNotContain(t, nb, "Also published to")
}

func TestServiceAccountRotateAndPublish(t *testing.T) {
	h := newSAHarness()
	p := newFakePub(false)
	p.turnOn(saOwner1, saPubBase, pubItem(credentialpublish.CredClientSecret, saPubBase, credentialpublish.StatePendingPut))
	// fakeSAOAuth returns no row id; the page runs the rotation of the client
	// the account's principal owns.
	page := h.pubPage(p)
	body := serveMounted(t, saAdmin(), req{method: http.MethodPost, target: "/ui/service-accounts/sa_1/publish/rotate", datastar: true, form: url.Values{"drawer": {"sa_1"}}}, page).Body.String()
	if len(p.rotated) != 1 || p.rotated[0] != "oac_row_1" {
		t.Fatalf("rotated = %v", p.rotated)
	}
	mustContain(t, body, "New Client Secret", "rotated-and-published", "Also published to", saPubBase+"client-secret", "Client secret rotated successfully")
	// It needs the rotation's permission: an update-only caller changes nothing.
	p.rotated = nil
	serveMounted(t, saUser(saPermView, saPermUpdate), req{method: http.MethodPost, target: "/ui/service-accounts/sa_1/publish/rotate", datastar: true, form: url.Values{}}, page)
	if len(p.rotated) != 0 {
		t.Error("rotated without the rotation permission")
	}
}

// fakeSAOAuthRows is the account's OAuth client with its row id.
type fakeSAOAuthRows struct{}

func (fakeSAOAuthRows) FindByPrincipalID(_ context.Context, id string) ([]platformauth.OAuthClient, error) {
	if id == "prn_sa1" {
		return []platformauth.OAuthClient{{ID: "oac_row_1", ClientID: "oauth_orders_public"}}, nil
	}
	return nil, nil
}

// ── OAuth clients ──────────────────────────────────────────────────────

var ocOwner1 = credentialpublish.Owner{Type: credentialpublish.OwnerOAuthClient, ID: "oc_1"}

const ocPubBase = "/flowcatalyst/np/oauth-clients/oac_orders/"

func (h *ocHarness) pubPage(p CredentialPublishing) *OAuthClients {
	page := h.page()
	page.Publish = p
	page.Accounts = &fakeSAStore{accounts: []serviceaccount.ServiceAccount{saFixture("sac_7", "billing-svc", "Billing <service>", true)}}
	return page
}

func (h *ocHarness) doPub(t *testing.T, ac *auth.AuthContext, p CredentialPublishing, q req) string {
	t.Helper()
	return serveMounted(t, ac, q, h.pubPage(p)).Body.String()
}

func TestOAuthClientCreatePublishes(t *testing.T) {
	h := newOCHarness()
	p := newFakePub(false)
	created := credentialpublish.Owner{Type: credentialpublish.OwnerOAuthClient, ID: "oc_9"}
	base := "/flowcatalyst/np/oauth-clients/oac_new_client/"
	p.items[created] = []credentialpublish.Item{
		pubItem(credentialpublish.CredClientID, base, credentialpublish.StatePendingPut),
		pubItem(credentialpublish.CredClientSecret, base, credentialpublish.StatePendingPut),
	}
	form := url.Values{
		"clientName": {"Billing"}, "clientType": {"CONFIDENTIAL"}, "redirectUris": {"https://b.example.com/cb"}, "grantTypes": {"authorization_code"},
		"publishCredentials": {"true"}, "publishBasePath": {"/flowcatalyst/np/custom/"},
	}
	body := h.doPub(t, anchorAdmin(), p, req{method: http.MethodPost, target: "/ui/oauth-clients/new", datastar: true, multipart: true, form: form})
	c := h.ops.created[0]
	if c.PublishCredentials == nil || !*c.PublishCredentials || deref(c.PublishBasePath) != "/flowcatalyst/np/custom/" {
		t.Errorf("publishing choice = %v %v", c.PublishCredentials, c.PublishBasePath)
	}
	mustContain(t, body, "brand-new-client-secret", "Also published to", base+"client-id", base+"client-secret")

	// A public client never sends the choice, ticked or not.
	h.ops.created = nil
	form.Set("clientType", "PUBLIC")
	h.doPub(t, anchorAdmin(), p, req{method: http.MethodPost, target: "/ui/oauth-clients/new", datastar: true, form: form})
	if c := h.ops.created[0]; c.PublishCredentials != nil || c.PublishBasePath != nil {
		t.Errorf("a public client sent a publishing choice: %v %v", c.PublishCredentials, c.PublishBasePath)
	}
}

func TestOAuthClientPublishRefusalLandsOnTheField(t *testing.T) {
	h := newOCHarness()
	h.ops.err = usecase.Validation(credentialpublish.CodePathInvalid, "publishBasePath: the path must lie under /flowcatalyst/np/")
	body := h.doPub(t, anchorAdmin(), newFakePub(false), req{method: http.MethodPost, target: "/ui/oauth-clients/new", datastar: true, form: url.Values{
		"clientName": {"Billing"}, "clientType": {"CONFIDENTIAL"}, "redirectUris": {"https://b.example.com/cb"}, "grantTypes": {"authorization_code"},
		"publishCredentials": {"true"}, "publishBasePath": {"/nope/"},
	}})
	mustContain(t, body, `id="oc-form-publish-path-err"`, "The path must lie under /flowcatalyst/np/")
}

func TestOAuthClientDrawerSection(t *testing.T) {
	h := newOCHarness()
	// A stand-alone confidential client: the same section.
	off := h.doPub(t, anchorAdmin(), newFakePub(false), req{target: "/ui/oauth-clients/oc_1", datastar: true})
	mustContain(t, off, "Parameter Store", "Not published", `href="/ui/oauth-clients/oc_1/publish"`)
	p := newFakePub(false)
	p.turnOn(ocOwner1, ocPubBase,
		pubItem(credentialpublish.CredClientID, ocPubBase, credentialpublish.StatePublished),
		pubItem(credentialpublish.CredClientSecret, ocPubBase, credentialpublish.StateNotPublishable),
	)
	on := h.doPub(t, anchorAdmin(), p, req{target: "/ui/oauth-clients/oc_1", datastar: true})
	mustContain(t, on, ocPubBase+"client-id", "aws-ps://"+ocPubBase+"client-id", "Not published: rotate the client secret to publish it",
		"Rotate and publish", `data-action="/ui/oauth-clients/oc_1/rotate-secret"`, "Stop publishing", "Deletes these 1 parameters")
	// A public client has nothing to publish.
	public := h.doPub(t, anchorAdmin(), newFakePub(false), req{target: "/ui/oauth-clients/oc_2", datastar: true})
	mustNotContain(t, public, "Parameter Store")
	// A client linked to a service account is published with it.
	p.linked["oc_1"] = "sac_7"
	linked := h.doPub(t, anchorAdmin(), p, req{target: "/ui/oauth-clients/oc_1", datastar: true})
	mustContain(t, linked, "Published with service account", `href="/ui/service-accounts/sac_7"`, "Billing &lt;service&gt;")
	mustNotContain(t, linked, "Stop publishing", "Publish credentials", ocPubBase+"client-id")
}

func TestOAuthClientEnableStopRetry(t *testing.T) {
	h := newOCHarness()
	p := newFakePub(false)
	dlg := h.doPub(t, anchorAdmin(), p, req{target: "/ui/oauth-clients/oc_1/publish", datastar: true})
	mustContain(t, dlg, "Publish credentials", ocPubBase, `name="publishBasePath"`)
	h.doPub(t, anchorAdmin(), p, req{method: http.MethodPost, target: "/ui/oauth-clients/oc_1/publish", datastar: true, form: url.Values{"drawer": {"oc_1"}, "publishBasePath": {"/flowcatalyst/np/x/"}}})
	if len(p.enabled) != 1 || p.enabled[0].owner != ocOwner1 || deref(p.enabled[0].path) != "/flowcatalyst/np/x/" {
		t.Errorf("enabled = %+v", p.enabled)
	}
	h.doPub(t, anchorAdmin(), p, req{method: http.MethodPost, target: "/ui/oauth-clients/oc_1/publish/stop", datastar: true, form: url.Values{"drawer": {"oc_1"}}})
	h.doPub(t, anchorAdmin(), p, req{method: http.MethodPost, target: "/ui/oauth-clients/oc_1/publish/retry", datastar: true, form: url.Values{"drawer": {"oc_1"}}})
	if len(p.disabled) != 1 || len(p.retried) != 1 {
		t.Errorf("disabled %v retried %v", p.disabled, p.retried)
	}
	// Anchor scope and the update permission, as the API.
	h.doPub(t, ocUser(ocPermView), p, req{method: http.MethodPost, target: "/ui/oauth-clients/oc_1/publish/stop", datastar: true, form: url.Values{}})
	if len(p.disabled) != 1 {
		t.Error("stopped without the update permission")
	}
}

func TestOAuthClientRotateRevealsWhereItWasPublished(t *testing.T) {
	h := newOCHarness()
	p := newFakePub(false)
	p.turnOn(ocOwner1, ocPubBase, pubItem(credentialpublish.CredClientSecret, ocPubBase, credentialpublish.StatePendingPut))
	body := h.doPub(t, anchorAdmin(), p, req{method: http.MethodPost, target: "/ui/oauth-clients/oc_1/rotate-secret", datastar: true, form: url.Values{"drawer": {"oc_1"}}})
	mustContain(t, body, "rotated-client-secret", "Also published to", ocPubBase+"client-secret")
	// A linked client's rotation reads the account's items.
	p2 := newFakePub(false)
	p2.linked["oc_1"] = "sac_7"
	acct := credentialpublish.Owner{Type: credentialpublish.OwnerServiceAccount, ID: "sac_7"}
	p2.turnOn(acct, "/flowcatalyst/np/service-accounts/billing/", pubItem(credentialpublish.CredClientSecret, "/flowcatalyst/np/service-accounts/billing/", credentialpublish.StatePendingPut))
	body = h.doPub(t, anchorAdmin(), p2, req{method: http.MethodPost, target: "/ui/oauth-clients/oc_1/rotate-secret", datastar: true, form: url.Values{"drawer": {"oc_1"}}})
	mustContain(t, body, "Also published to", "/flowcatalyst/np/service-accounts/billing/client-secret")
}

// ── applications ───────────────────────────────────────────────────────

func TestApplicationProvisionPublishes(t *testing.T) {
	ah := newAppHarness()
	p := newFakePub(true)
	acct := credentialpublish.Owner{Type: credentialpublish.OwnerServiceAccount, ID: "sac_prov"}
	base := "/flowcatalyst/np/service-accounts/app.billing/"
	p.items[acct] = []credentialpublish.Item{
		pubItem(credentialpublish.CredClientID, base, credentialpublish.StatePendingPut),
		pubItem(credentialpublish.CredSigningSecret, base, credentialpublish.StatePendingPut),
	}
	page := &Applications{Apps: ah.store, Ops: ah.ops, Logins: fakeLogins{"app_1": true}, Docs: fakeDocs{}, Publish: p, Shell: shellForTest()}
	// The form starts as the default says (here ticked).
	form := serveMounted(t, anchorAdmin(), req{target: "/ui/applications/app_2", datastar: true}, page).Body.String()
	if !ticked(boxTag(t, form)) {
		t.Errorf("the provisioning box starts unticked while the default is on")
	}
	body := serveMounted(t, anchorAdmin(), req{method: http.MethodPost, target: "/ui/applications/app_2/provision-service-account", datastar: true, form: url.Values{
		"drawer": {"app_2"}, "publishCredentials": {"true"}, "publishBasePath": {"/flowcatalyst/np/billing/"},
	}}, page).Body.String()
	c := ah.ops.provisioned[0]
	if c.ApplicationID != "app_2" || c.PublishCredentials == nil || !*c.PublishCredentials || deref(c.PublishBasePath) != "/flowcatalyst/np/billing/" {
		t.Errorf("provision command = %+v", c)
	}
	mustContain(t, body, "Service Account Provisioned", "Also published to", base+"client-id", base+"signing-secret")
}

func TestApplicationProvisionPublishRefusalLandsOnTheField(t *testing.T) {
	ah := newAppHarness()
	ah.ops.err = usecase.Validation(credentialpublish.CodePathInvalid, "publishBasePath: the path must lie under /flowcatalyst/np/")
	page := &Applications{Apps: ah.store, Ops: ah.ops, Logins: fakeLogins{"app_1": true}, Docs: fakeDocs{}, Publish: newFakePub(false), Shell: shellForTest()}
	body := serveMounted(t, anchorAdmin(), req{method: http.MethodPost, target: "/ui/applications/app_2/provision-service-account", datastar: true, form: url.Values{
		"drawer": {"app_2"}, "publishCredentials": {"true"}, "publishBasePath": {"/nope/"},
	}}, page).Body.String()
	mustContain(t, body, `id="app-sa-publish-path-err"`, "The path must lie under /flowcatalyst/np/", `value="/nope/"`)
	mustNotContain(t, body, "fc-flash-error")
}
