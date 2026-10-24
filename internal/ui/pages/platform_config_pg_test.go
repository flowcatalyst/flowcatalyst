//go:build integration

package pages

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/audit"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/client"
	clientops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/client/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/platformconfig"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/encryption"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecaseop"
)

// revealAudits is the number of SECRET_REVEALED audit rows for a config row.
func revealAudits(t *testing.T, configID string) int {
	t.Helper()
	var n int
	require.NoError(t, testpg.Pool(t).QueryRow(context.Background(),
		`SELECT count(*) FROM aud_logs WHERE entity_id = $1 AND operation = 'SECRET_REVEALED'`, configID).Scan(&n))
	return n
}

// The Configuration page end to end against Postgres, through the same
// platformconfig.Service the API runs: save (plain and secret), the secret
// never in a response, the audited reveal, the permission matrix on forged
// posts, the unknown client, delete.
func TestConfigPageAgainstPostgres(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	key, err := encryption.GenerateKey()
	require.NoError(t, err)
	enc, err := encryption.New(key)
	require.NoError(t, err)
	repo := platformconfig.NewRepository(pool).WithEncryption(enc)
	clients := client.NewRepository(pool)
	uow := testpg.NewUoW(t)
	_, err = usecaseop.Run(testpg.AnchorCtx(), uow, clientops.CreateClient(clients), clientops.CreateCommand{Name: "UI Config Co", Identifier: "ui-cfgco"}, testpg.TestEC())
	require.NoError(t, err)
	clientID := *clientIDOf(t, clients, "ui-cfgco")
	const app = "uicfgapp"
	testpg.EnsureApplication(t, app)

	svc := &platformconfig.Service{Repo: repo, UoW: uow, Apps: application.NewRepository(pool), Clients: clients, Audit: audit.NewRepository(pool)}
	newPage := func(s *platformconfig.Service) *PlatformConfig {
		return &PlatformConfig{Config: s, Ops: ConfigOps{Repo: repo, UoW: uow}, Apps: application.NewRepository(pool), ClientIdentifier: client.NewCachedIdentifierResolver(clients, time.Minute), Shell: shellForTest()}
	}
	as := func(page *PlatformConfig, ac *auth.AuthContext, q req) (string, string) {
		t.Helper()
		rec := serveMounted(t, ac, q, page)
		require.Contains(t, []int{http.StatusOK, http.StatusSeeOther}, rec.Code, rec.Body.String())
		return rec.Header().Get("Cache-Control"), rec.Body.String()
	}
	admin := cfgAdmin()
	admin.Permissions = []string{"platform:*:*:*"}
	page := newPage(svc)
	do := func(q req) string {
		t.Helper()
		_, body := as(page, admin, q)
		return body
	}
	post := func(path string, form url.Values) req {
		return req{method: http.MethodPost, target: path, datastar: true, multipart: true, form: form}
	}
	save := func(kv ...string) url.Values {
		v := url.Values{"app": {app}, "section": {"sftp"}, "property": {"password"}, "client": {""}, "valueType": {"PLAIN"}, "plainValue": {"x"}, "description": {""}}
		for i := 0; i+1 < len(kv); i += 2 {
			v.Set(kv[i], kv[i+1])
		}
		return v
	}
	find := func(section, property string, cid *string) *platformconfig.Config {
		t.Helper()
		scope := platformconfig.ScopeGlobal
		if cid != nil {
			scope = platformconfig.ScopeClient
		}
		c, err := repo.FindByCoordinate(ctx, app, section, property, scope, cid)
		require.NoError(t, err)
		return c
	}

	// Plain create, then a secret create (global and for the client, by identifier).
	body := do(post("/ui/platform/config/save", save("section", "web", "property", "title", "plainValue", "Hello", "description", "the title")))
	require.Contains(t, body, "The property was saved", body)
	plain := find("web", "title", nil)
	require.NotNil(t, plain)
	assert.Equal(t, platformconfig.ValuePlain, plain.ValueType)
	assert.Equal(t, "Hello", plain.Value)

	const secret = "pg-TOPSECRET-1"
	body = do(post("/ui/platform/config/save", save("valueType", "SECRET", "secretValue", secret, "plainValue", "")))
	require.Contains(t, body, "The property was saved", body)
	body = do(post("/ui/platform/config/save", save("valueType", "SECRET", "secretValue", secret+"-acme", "client", "UI-CfgCo")))
	require.Contains(t, body, "The property was saved", body)
	global, scoped := find("sftp", "password", nil), find("sftp", "password", &clientID)
	require.NotNil(t, global)
	require.NotNil(t, scoped, "the identifier resolved to the client's id, as the API does")
	assert.True(t, strings.HasPrefix(global.Value, platformconfig.EncryptedPrefix), "stored encrypted")

	// An unknown client is refused: no row.
	refused := do(post("/ui/platform/config/save", save("property", "ghost", "client", "no-such-client")))
	assert.Contains(t, refused, "No client with this identifier.")
	assert.Nil(t, find("sftp", "ghost", nil))

	// The secret is in no page, dialog or save response.
	list := do(req{target: "/ui/platform/config?app=" + app})
	assert.Contains(t, list, "••••••••")
	assert.Contains(t, list, "ui-cfgco", "the client's identifier names the scope")
	edit := do(req{target: "/ui/platform/config/edit?app=" + app + "&section=sftp&property=password", datastar: true})
	assert.Contains(t, edit, "Enter a new value to replace the secret")
	for name, b := range map[string]string{"list": list, "edit": edit, "save": body} {
		assert.NotContains(t, b, secret, name)
		assert.NotContains(t, b, platformconfig.EncryptedPrefix, name)
	}

	// Reveal: the audited path. Plaintext once, no-store, one audit row each.
	revealReq := req{method: http.MethodPost, target: "/ui/platform/config/reveal?app=" + app + "&section=sftp&property=password", datastar: true}
	cc, body := as(page, admin, revealReq)
	assert.Equal(t, "no-store", cc)
	assert.Equal(t, 1, strings.Count(body, secret+"<"), "the value is shown once, as text")
	assert.Equal(t, 1, revealAudits(t, global.ID))
	revealReq.target += "&client=" + clientID
	_, body = as(page, admin, revealReq)
	assert.Contains(t, body, secret+"-acme")
	assert.Equal(t, 1, revealAudits(t, scoped.ID), "the client's value is its own row")
	assert.NotContains(t, do(req{target: "/ui/platform/config?app=" + app}), secret)

	// A new secret replaces the old one (edit), and the reveal shows it.
	do(post("/ui/platform/config/save", save("edit", "1", "valueType", "SECRET", "secretValue", "rotated-2")))
	_, body = as(page, admin, req{method: http.MethodPost, target: "/ui/platform/config/reveal?app=" + app + "&section=sftp&property=password", datastar: true})
	assert.Contains(t, body, "rotated-2")
	assert.Equal(t, 2, revealAudits(t, global.ID))

	// A reveal that cannot be audited is refused: no plaintext, no row.
	noAudit := newPage(&platformconfig.Service{Repo: repo, UoW: uow, Apps: svc.Apps, Clients: clients})
	_, body = as(noAudit, admin, revealReq)
	assert.NotContains(t, body, secret)
	assert.Contains(t, body, "fc-flash-error")
	assert.Equal(t, 1, revealAudits(t, scoped.ID))

	// Permission matrix against the real rules.
	viewer := cfgUser("platform:admin:config:view")
	vbody := as2(t, page, viewer, req{target: "/ui/platform/config?app=" + app})
	assert.Contains(t, vbody, "sftp")
	for _, absent := range []string{"Add property", "/ui/platform/config/edit", "Reveal…", "Delete…"} {
		assert.NotContains(t, vbody, absent)
	}
	updater := cfgUser("platform:admin:config:view", "platform:admin:config:update")
	ubody := as2(t, page, updater, req{target: "/ui/platform/config?app=" + app})
	assert.Contains(t, ubody, "Add property")
	assert.NotContains(t, ubody, "Reveal…")

	before := find("sftp", "password", nil).UpdatedAt
	forged := []req{
		post("/ui/platform/config/save", save("plainValue", "hacked")),
		post("/ui/platform/config/delete?app="+app+"&section=sftp&property=password", nil),
		post("/ui/platform/config/reveal?app="+app+"&section=sftp&property=password", nil),
	}
	for _, q := range forged {
		b := as2(t, page, viewer, q)
		assert.NotContains(t, b, "rotated-2")
	}
	_, b := as(page, updater, forged[2])
	assert.NotContains(t, b, "rotated-2", "update does not imply reveal")
	after := find("sftp", "password", nil)
	require.NotNil(t, after, "a view-only caller cannot delete")
	assert.True(t, before.Equal(after.UpdatedAt), "nor change")
	assert.True(t, strings.HasPrefix(after.Value, platformconfig.EncryptedPrefix))
	assert.Equal(t, 2, revealAudits(t, global.ID), "refused reveals leave no audit row")

	// Delete (idempotent), through the confirm dialog's post.
	body = do(post("/ui/platform/config/delete?app="+app+"&section=web&property=title", nil))
	assert.Contains(t, body, "The property was deleted")
	assert.Nil(t, find("web", "title", nil))
	do(post("/ui/platform/config/delete?app="+app+"&section=web&property=title", nil))
}

// as2 serves a request as ac and returns the body.
func as2(t *testing.T, page *PlatformConfig, ac *auth.AuthContext, q req) string {
	t.Helper()
	return serveMounted(t, ac, q, page).Body.String()
}
