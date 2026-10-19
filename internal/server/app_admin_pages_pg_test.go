//go:build integration

package server

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/ids"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatch"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/flowcatalyst-go/internal/tsid"
)

// F20 end to end: an application's service account, with only the built-in
// application-service permissions, reads its own events and dispatch jobs and
// administers its own users and invitations through the whole HTTP stack
// (routing, bearer auth, huma validation, the application confinement), and
// is refused everything of another application.
func TestApplicationAdminPagesEndToEnd(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	keyPath, err := EnsureSigningKeyFile(filepath.Join(t.TempDir(), "jwt.pem"))
	require.NoError(t, err)
	cfg := EnvCfg{
		JWTIssuer: "https://platform.example.com", JWTSigningKeyPath: keyPath,
		SessionTTLSecs: 3600, AccessTokenTTLSecs: 3600, RefreshTokenTTLSecs: 3600,
		FunctionsArtifactStore: "file://" + t.TempDir(),
	}
	r := chi.NewRouter()
	_, err = WirePlatform(r, pool, cfg, dispatch.Settings{})
	require.NoError(t, err)
	repos := buildRepos(pool)
	svcs, err := buildServices(cfg, pool, repos)
	require.NoError(t, err)
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)

	sfx := strings.ToLower(tsid.GenerateUntyped())[5:]
	own, other := "e2eo"+sfx, "e2ex"+sfx
	ownID, otherID := tsid.Generate(tsid.Application), tsid.Generate(tsid.Application)
	for _, a := range [][2]string{{ownID, own}, {otherID, other}} {
		_, err = pool.Exec(ctx, `INSERT INTO app_applications (id, code, name) VALUES ($1, $2, $2)`, a[0], a[1])
		require.NoError(t, err)
	}
	clientID := "clt_e2e" + sfx[:6]
	_, err = pool.Exec(ctx, `INSERT INTO tnt_clients (id, name, identifier) VALUES ($1, $1, $1)`, clientID)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `INSERT INTO app_client_configs (id, application_id, client_id, enabled) VALUES ($1, $2, $3, TRUE)`,
		tsid.Generate(tsid.AppClientConfig), ownID, clientID)
	require.NoError(t, err)
	// the application's roles
	for _, rn := range []string{own + ":clerk", other + ":clerk"} {
		_, err = pool.Exec(ctx, `INSERT INTO iam_roles (id, name, display_name, application_id, application_code, source) VALUES ($1, $2, $2, $3, $4, 'SDK')`,
			tsid.Generate(tsid.Role), rn, map[bool]string{true: ownID, false: otherID}[strings.HasPrefix(rn, own)], strings.SplitN(rn, ":", 2)[0])
		require.NoError(t, err)
	}
	// data of both applications
	now := time.Now().UTC()
	for i, app := range []string{own, other} {
		evID := "e2eev" + sfx[:5] + string(rune('0'+i))
		_, err = pool.Exec(ctx, `INSERT INTO msg_events_read (id, type, source, time, application, created_at) VALUES ($1, $2, 'test://e2e', NOW(), $3, $4)`,
			evID, app+":a:b:c", app, now)
		require.NoError(t, err)
		_, err = pool.Exec(ctx, `INSERT INTO msg_dispatch_jobs_read (id, code, target_url, application, kind, protocol, mode, status, max_retries, attempt_count, created_at, updated_at)
			VALUES ($1, $2, 'http://example.invalid/h', $3, 'EVENT', 'HTTP_WEBHOOK', 'IMMEDIATE', 'COMPLETED', 3, 1, $4, $4)`,
			"e2ejb"+sfx[:5]+string(rune('0'+i)), app+":a:b:c", app, now)
		require.NoError(t, err)
	}

	// The application's service account: the built-in role's permissions, bound
	// to its own application only.
	perms := []string{
		"platform:application-service:event:view", "platform:application-service:dispatch-job:view",
		"platform:application-service:user:view", "platform:application-service:user:create",
		"platform:application-service:invitation:view", "platform:application-service:invitation:manage",
	}
	bearer := mintScoped(t, pool, repos, svcs, ownID, perms)

	do := func(method, path, body string) (int, string) {
		t.Helper()
		var rdr io.Reader
		if body != "" {
			rdr = strings.NewReader(body)
		}
		req, err := http.NewRequest(method, srv.URL+path, rdr)
		require.NoError(t, err)
		req.Header.Set("Authorization", "Bearer "+bearer)
		if body != "" {
			req.Header.Set("Content-Type", "application/json")
		}
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(b)
	}

	// events and dispatch jobs: only the application's own
	code, body := do("GET", "/api/events?size=1000&types="+own+":a:b:c,"+other+":a:b:c", "")
	require.Equal(t, 200, code, body)
	assert.Contains(t, body, own+":a:b:c")
	assert.NotContains(t, body, other+":a:b:c")
	code, body = do("GET", "/api/dispatch-jobs?size=1000&codes="+own+":a:b:c,"+other+":a:b:c", "")
	require.Equal(t, 200, code, body)
	assert.Contains(t, body, own+":a:b:c")
	assert.NotContains(t, body, other+":a:b:c")
	code, _ = do("GET", "/api/events/e2eev"+sfx[:5]+"1", "")
	assert.Equal(t, 404, code, "another application's event is not found")
	code, _ = do("GET", "/api/dispatch-jobs/e2ejb"+sfx[:5]+"1", "")
	assert.Equal(t, 404, code)

	// users: invite (link back), list, get, invitations, resend, revoke
	code, body = do("POST", "/api/applications/"+own+"/users",
		`{"email":"e2e-`+sfx+`@example.test","name":"E2E","clientId":"`+clientID+`","roles":["`+own+`:clerk"],"returnInviteLink":true}`)
	require.Equal(t, 201, code, body)
	var created struct {
		ID         string  `json:"id"`
		InviteLink *string `json:"inviteLink"`
	}
	require.NoError(t, json.Unmarshal([]byte(body), &created))
	require.NotNil(t, created.InviteLink)
	token := (*created.InviteLink)[strings.Index(*created.InviteLink, "token=")+len("token="):]

	code, body = do("GET", "/api/applications/"+own+"/users?pageSize=10", "")
	require.Equal(t, 200, code, body)
	assert.Contains(t, body, created.ID)
	assert.NotContains(t, body, token, "the invite token is never listed")
	code, body = do("GET", "/api/applications/"+own+"/invitations", "")
	require.Equal(t, 200, code, body)
	assert.Contains(t, body, created.ID)
	assert.NotContains(t, body, token)
	assert.NotContains(t, body, "inviteLink")
	code, body = do("POST", "/api/applications/"+own+"/invitations/"+created.ID+"/resend", `{"returnInviteLink":true}`)
	require.Equal(t, 200, code, body)
	assert.Contains(t, body, "inviteLink")
	code, _ = do("DELETE", "/api/applications/"+own+"/invitations/"+created.ID, "")
	assert.Equal(t, 204, code)
	code, body = do("GET", "/api/applications/"+own+"/users/"+created.ID, "")
	require.Equal(t, 200, code, body)
	assert.Contains(t, body, `"active":false`)

	// huma validation: roles are required
	code, _ = do("POST", "/api/applications/"+own+"/users", `{"email":"x@example.test","name":"X","clientId":"`+clientID+`","roles":[]}`)
	assert.Equal(t, 400, code)

	// another application, or none: 404, the same for both (F32)
	for _, c := range []struct{ m, p, b string }{
		{"GET", "/api/applications/" + other + "/users", ""},
		{"GET", "/api/applications/" + other + "/invitations", ""},
		{"POST", "/api/applications/" + other + "/users", `{"email":"y@example.test","name":"Y","clientId":"` + clientID + `","roles":["` + other + `:clerk"]}`},
		{"GET", "/api/applications/nosuchapp" + sfx + "/users", ""},
	} {
		code, body = do(c.m, c.p, c.b)
		assert.Equal(t, 404, code, "%s %s: %s", c.m, c.p, body)
		assert.Contains(t, body, "Application_NOT_FOUND")
	}
}

// mintScoped makes a principal holding a role with perms, bound to exactly one
// application (all_applications off plus one access row), and returns a
// bearer for it.
func mintScoped(t *testing.T, pool *pgxpool.Pool, repos *repoSet, svcs *serviceSet, appID string, perms []string) string {
	t.Helper()
	ctx := context.Background()
	pid := ids.NewPrincipalID()
	roleName := "e2escoped:r-" + strings.ToLower(string(pid))
	_, err := pool.Exec(ctx, `INSERT INTO iam_principals (id, type, scope, name, active, email, all_applications) VALUES ($1, 'USER', 'ANCHOR', 'scoped', TRUE, $2, FALSE)`,
		pid, strings.ToLower(string(pid))+"@scoped.example.com")
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `INSERT INTO iam_principal_application_access (principal_id, application_id) VALUES ($1, $2)`, pid, appID)
	require.NoError(t, err)
	roleID := tsid.Generate(tsid.Role)
	_, err = pool.Exec(ctx, `INSERT INTO iam_roles (id, name, display_name) VALUES ($1, $2, 'scoped')`, roleID, roleName)
	require.NoError(t, err)
	for _, perm := range perms {
		_, err = pool.Exec(ctx, `INSERT INTO iam_role_permissions (role_id, permission) VALUES ($1, $2)`, roleID, perm)
		require.NoError(t, err)
	}
	_, err = pool.Exec(ctx, `INSERT INTO iam_principal_roles (principal_id, role_name, assignment_source) VALUES ($1, $2, 'TEST')`, pid, roleName)
	require.NoError(t, err)
	p, err := repos.principalRepo.FindByID(ctx, pid)
	require.NoError(t, err)
	require.False(t, p.AllApplications)
	bearer, err := svcs.authSvc.GenerateAccessTokenWithScope(p, perms)
	require.NoError(t, err)
	return bearer
}
