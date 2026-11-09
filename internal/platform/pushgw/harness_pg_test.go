//go:build integration

package pushgw

import (
	"bufio"
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/framework/go/fw"
	"github.com/flowcatalyst/framework/go/fwtest"

	"github.com/flowcatalyst/flowcatalyst-go/internal/ids"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth/authservice"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth/provider"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/cors"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/fieldidentity"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/role"
	platformmw "github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/middleware"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/flowcatalyst-go/internal/tsid"
)

func TestMain(m *testing.M) { testpg.RunMain(m) }

const (
	publicBase   = "https://fc.example.test"
	streamSecret = "0123456789abcdef0123456789abcdef-stream-secret"
	pushPath     = "/api/push"
)

var (
	dbSeq  atomic.Int64
	keyPEM struct {
		once     sync.Once
		priv, pu []byte
	}
)

func rsaPEMs(t *testing.T) (priv, pub []byte) {
	keyPEM.once.Do(func() {
		k, err := rsa.GenerateKey(rand.Reader, 2048)
		require.NoError(t, err)
		keyPEM.priv = pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(k)})
		der, err := x509.MarshalPKIXPublicKey(&k.PublicKey)
		require.NoError(t, err)
		keyPEM.pu = pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der})
	})
	return keyPEM.priv, keyPEM.pu
}

// world is one isolated, fully migrated database, the platform's token validators over one signing
// key, and the gateways started on it.
type world struct {
	t     *testing.T
	pool  *pgxpool.Pool
	url   string
	prov  *provider.Provider
	auth  *authservice.AuthService
	field *fieldidentity.Ops
	apps  map[string]string // code -> application id
}

func newWorld(t *testing.T, appCodes ...string) *world {
	t.Helper()
	tpl := testpg.MigratedTemplate(t, "pushgw_template")
	name := fmt.Sprintf("pushgw_t%d", dbSeq.Add(1))
	pool := testpg.CloneDB(t, tpl, name, nil)

	priv, pub := rsaPEMs(t)
	svc, err := authservice.NewWithRSA(authservice.Config{
		Issuer: publicBase, Audience: publicBase,
		AccessTokenExpirySecs: 3600, SessionTokenExpirySecs: 3600, RefreshTokenExpirySecs: 3600, IDTokenExpirySecs: 300,
	}, string(priv), string(pub))
	require.NoError(t, err)
	prov, err := provider.NewProvider(provider.Config{Issuer: publicBase, Audience: publicBase, SigningKey: priv},
		principal.NewRepository(pool), role.NewRepository(pool))
	require.NoError(t, err)

	cc := pool.Config().ConnConfig
	u := url.URL{
		Scheme: "postgres", User: url.UserPassword(cc.User, cc.Password),
		Host: fmt.Sprintf("%s:%d", cc.Host, cc.Port), Path: "/" + cc.Database, RawQuery: "sslmode=disable",
	}
	w := &world{
		t: t, pool: pool, url: u.String(), prov: prov, auth: svc,
		field: &fieldidentity.Ops{R: fieldidentity.NewRepos(pool)}, apps: map[string]string{},
	}
	for _, code := range appCodes {
		id := tsid.Generate(tsid.Application)
		_, err := pool.Exec(t.Context(), `INSERT INTO app_applications (id, code, name) VALUES ($1, $2, $2)`, id, code)
		require.NoError(t, err)
		w.apps[code] = id
	}
	return w
}

// ── fixtures: rules, keys, settings ──────────────────────────────────────

func (w *world) rule(app, pattern string, public bool, requires ...string) {
	w.t.Helper()
	if requires == nil {
		requires = []string{}
	}
	_, err := w.pool.Exec(w.t.Context(),
		`INSERT INTO push_rules (application_id, pattern, requires, public) VALUES ($1, $2, $3, $4)
		 ON CONFLICT (application_id, pattern) DO UPDATE SET requires = $3, public = $4`, w.apps[app], pattern, requires, public)
	require.NoError(w.t, err)
}

func (w *world) ticketKey(app, kid string, jwk map[string]string) {
	w.t.Helper()
	raw, _ := json.Marshal(jwk)
	_, err := w.pool.Exec(w.t.Context(),
		`INSERT INTO push_ticket_keys (application_id, kid, jwk) VALUES ($1, $2, $3) ON CONFLICT (application_id, kid) DO UPDATE SET jwk = $3`,
		w.apps[app], kid, raw)
	require.NoError(w.t, err)
}

func (w *world) notifyConfig() {
	w.t.Helper()
	require.NoError(w.t, NotifyConfigChanged(w.t.Context(), w.pool))
}

// ── principals and credentials ───────────────────────────────────────────

type who struct {
	scope   principal.UserScope
	service bool
	clients []string // the client ids a CLIENT user has (one) or a PARTNER user is assigned
	perms   []string // the granted permissions (the token's scope claim)
	id      string   // the principal id, when the test needs a particular one
}

// bearer mints an API access token for an in-memory principal.
func (w *world) bearer(x who) (token, principalID string) {
	w.t.Helper()
	var p *principal.Principal
	if x.service {
		p = principal.NewService(string(ids.NewPrincipalID()), "svc")
	} else {
		p = principal.NewUser(fmt.Sprintf("u%d@example.test", dbSeq.Add(1)), x.scope)
	}
	if !x.service {
		p.Scope = x.scope
	}
	if x.id != "" {
		p.ID = ids.PrincipalID(x.id)
	}
	switch x.scope {
	case principal.ScopeClient:
		id := ids.ClientID(x.clients[0])
		p.ClientID = &id
	case principal.ScopePartner:
		for _, c := range x.clients {
			p.AssignedClients = append(p.AssignedClients, ids.ClientID(c))
		}
	case principal.ScopeAnchor:
		// an anchor reaches every client: no client of its own
	}
	tok, err := w.auth.GenerateAccessTokenWithScope(p, x.perms)
	require.NoError(w.t, err)
	return tok, string(p.ID)
}

// cookie mints the fc_session cookie of an anchor user stored in the database, holding perms
// through a role (a session carries identity only; permissions are read from the roles).
func (w *world) cookie(perms ...string) (token, principalID string) {
	w.t.Helper()
	pid := ids.NewPrincipalID()
	_, err := w.pool.Exec(w.t.Context(),
		`INSERT INTO iam_principals (id, type, scope, name, active, email) VALUES ($1, 'USER', 'ANCHOR', 'Cookie', TRUE, $2)`,
		pid, strings.ToLower(string(pid))+"@cookie.example.test")
	require.NoError(w.t, err)
	if len(perms) > 0 {
		roleID, roleName := tsid.Generate(tsid.Role), "pushtest:role-"+strings.ToLower(string(pid))
		_, err = w.pool.Exec(w.t.Context(), `INSERT INTO iam_roles (id, name, display_name) VALUES ($1, $2, 'R')`, roleID, roleName)
		require.NoError(w.t, err)
		for _, p := range perms {
			_, err = w.pool.Exec(w.t.Context(), `INSERT INTO iam_role_permissions (role_id, permission) VALUES ($1, $2)`, roleID, p)
			require.NoError(w.t, err)
		}
		_, err = w.pool.Exec(w.t.Context(),
			`INSERT INTO iam_principal_roles (principal_id, role_name, assignment_source) VALUES ($1, $2, 'TEST')`, pid, roleName)
		require.NoError(w.t, err)
	}
	tok, err := w.prov.MintSessionToken(w.t.Context(), string(pid), 0)
	require.NoError(w.t, err)
	return tok, string(pid)
}

// ── the field plane ──────────────────────────────────────────────────────

// device is a field device key.
type device struct{ key *ecdsa.PrivateKey }

func newDevice(t *testing.T) *device {
	k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	return &device{key: k}
}

func (d *device) jwk() map[string]string {
	pt, _ := d.key.PublicKey.Bytes()
	return map[string]string{
		"kty": "EC", "crv": "P-256",
		"x": base64.RawURLEncoding.EncodeToString(pt[1:33]), "y": base64.RawURLEncoding.EncodeToString(pt[33:65]),
	}
}

func (d *device) jkt() string {
	j := d.jwk()
	sum := sha256.Sum256([]byte(`{"crv":"P-256","kty":"EC","x":"` + j["x"] + `","y":"` + j["y"] + `"}`))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

// proof signs a DPoP proof for a request.
func (d *device) proof(t *testing.T, method, htu, token string, iat time.Time) string {
	hdr, _ := json.Marshal(map[string]any{"typ": "dpop+jwt", "alg": "ES256", "jwk": d.jwk()})
	ath := sha256.Sum256([]byte(token))
	claims, _ := json.Marshal(map[string]any{
		"htm": method, "htu": htu, "iat": iat.Unix(), "jti": fmt.Sprintf("jti-%d", dbSeq.Add(1)),
		"ath": base64.RawURLEncoding.EncodeToString(ath[:]),
	})
	signing := base64.RawURLEncoding.EncodeToString(hdr) + "." + base64.RawURLEncoding.EncodeToString(claims)
	digest := sha256.Sum256([]byte(signing))
	r, s, err := ecdsa.Sign(rand.Reader, d.key, digest[:])
	require.NoError(t, err)
	sig := make([]byte, 64)
	r.FillBytes(sig[:32])
	s.FillBytes(sig[32:])
	return signing + "." + base64.RawURLEncoding.EncodeToString(sig)
}

// fieldWorker is a signed-in field worker: a live login family on an unenrolled key.
type fieldWorker struct {
	dev     *device
	token   string
	worker  string // field identity id (the principal)
	family  string // login family (the stream's session)
	clientI string
}

func (w *world) fieldWorker(app, client string) *fieldWorker {
	w.t.Helper()
	dev := newDevice(w.t)
	worker := tsid.Generate(tsid.FieldWorker)
	family := tsid.Generate(tsid.FieldSession)
	ctx := w.t.Context()
	_, err := w.pool.Exec(ctx, `INSERT INTO field_identities (id, client_id, worker_code, name) VALUES ($1, $2, $3, 'W')`,
		worker, client, strings.ToUpper(worker))
	require.NoError(w.t, err)
	_, err = w.pool.Exec(ctx,
		`INSERT INTO field_sessions (id, family_id, identity_id, application_id, refresh_hash, idle_expires_at, absolute_expires_at, jkt)
		 VALUES ($1, $2, $3, $4, $5, NOW() + interval '1 day', NOW() + interval '1 day', $6)`,
		tsid.Generate(tsid.FieldSession), family, worker, w.apps[app], tsid.Generate(tsid.FieldSession), dev.jkt())
	require.NoError(w.t, err)
	tok, err := w.auth.GenerateFieldAccessToken(authservice.FieldSubject{
		WorkerID: worker, ClientID: client, ClientIdentifier: "ident", WorkerCode: strings.ToUpper(worker), Name: "W",
		AppCode: app, DeviceMode: "UNENROLLED", SessionID: family, AuthTime: time.Now(), JKT: dev.jkt(),
	})
	require.NoError(w.t, err)
	return &fieldWorker{dev: dev, token: tok, worker: worker, family: family, clientI: client}
}

// ── a gateway task ───────────────────────────────────────────────────────

type node struct {
	t   *testing.T
	gw  *Gateway
	srv *httptest.Server
}

type nodeOpts struct {
	env   map[string]string
	clock fw.Clock
}

func (w *world) start(o ...nodeOpts) *node {
	w.t.Helper()
	csrf, err := platformmw.SessionCSRFChecker([]string{publicBase})
	require.NoError(w.t, err)
	env := map[string]string{
		"APP_ENV": "production", "PUSH_STREAM_SECRET": streamSecret, "PUSH_RELAY_INTERVAL": "50ms",
	}
	var clock fw.Clock
	for _, x := range o {
		maps.Copy(env, x.env)
		if x.clock != nil {
			clock = x.clock
		}
	}
	gw, err := Start(w.t.Context(), Config{
		Pool: w.pool, DatabaseURL: w.url, Env: env, Clock: clock,
		Authenticator: &Authenticator{Provider: w.prov, Auth: w.auth, Field: w.field, CSRF: csrf, BaseURL: publicBase, Replay: PGReplayGuard{Pool: w.pool}},
		Origins:       cors.NewRepository(w.pool),
	})
	require.NoError(w.t, err)
	mux := http.NewServeMux()
	mux.Handle(IngestPath, gw.IngestHandler(platformmw.Authenticator(platformmw.AuthConfig{Provider: w.prov})))
	mux.Handle(pushPath+"/", gw.Handler())
	srv := httptest.NewServer(mux)
	n := &node{t: w.t, gw: gw, srv: srv}
	w.t.Cleanup(func() {
		srv.CloseClientConnections()
		srv.Close()
		_ = gw.Shutdown()
	})
	return n
}

// request is one HTTP call to a node.
type request struct {
	method string
	path   string
	header map[string]string
	body   any
	raw    string
}

type response struct {
	status int
	header http.Header
	body   []byte
}

func (r response) json() map[string]any {
	var m map[string]any
	_ = json.Unmarshal(r.body, &m)
	return m
}

func (r response) code() string {
	s, _ := r.json()["error"].(string)
	return s
}

func (n *node) do(q request) response {
	n.t.Helper()
	var body io.Reader
	switch {
	case q.raw != "":
		body = strings.NewReader(q.raw)
	case q.body != nil:
		b, err := json.Marshal(q.body)
		require.NoError(n.t, err)
		body = bytes.NewReader(b)
	}
	method := q.method
	if method == "" {
		method = http.MethodPost
	}
	req, err := http.NewRequestWithContext(n.t.Context(), method, n.srv.URL+q.path, body)
	require.NoError(n.t, err)
	if q.body != nil || q.raw != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range q.header {
		req.Header.Set(k, v)
	}
	res, err := http.DefaultClient.Do(req)
	require.NoError(n.t, err)
	defer func() { _ = res.Body.Close() }()
	b, _ := io.ReadAll(res.Body)
	return response{status: res.StatusCode, header: res.Header, body: b}
}

// subscribe posts to /api/push/subscriptions.
func (n *node) subscribe(body any, header map[string]string) response {
	n.t.Helper()
	return n.do(request{path: pushPath + "/subscriptions", header: header, body: body})
}

func bearerHeader(token string) map[string]string {
	return map[string]string{"Authorization": "Bearer " + token}
}

// fieldHeader is the headers of a field worker's subscribe.
func (f *fieldWorker) header(t *testing.T, method, path string) map[string]string {
	return map[string]string{
		"Authorization": "DPoP " + f.token,
		"DPoP":          f.dev.proof(t, method, publicBase+path, f.token, time.Now()),
	}
}

// ── streams ──────────────────────────────────────────────────────────────

type stream struct {
	t      *testing.T
	frames chan string
	closed chan struct{}
	cancel context.CancelFunc
	res    *http.Response
}

// listen opens GET /api/push/stream and parses its frames.
func (n *node) listen(token, lastEventID string) *stream {
	n.t.Helper()
	ctx, cancel := context.WithCancel(n.t.Context())
	target := n.srv.URL + pushPath + "/stream?st=" + url.QueryEscape(token)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	require.NoError(n.t, err)
	if lastEventID != "" {
		req.Header.Set("Last-Event-ID", lastEventID)
	}
	res, err := http.DefaultClient.Do(req)
	require.NoError(n.t, err)
	s := &stream{t: n.t, frames: make(chan string, 256), closed: make(chan struct{}), cancel: cancel, res: res}
	n.t.Cleanup(func() { cancel(); _ = res.Body.Close() })
	go func() {
		defer close(s.closed)
		br := bufio.NewReader(res.Body)
		var b strings.Builder
		for {
			line, err := br.ReadString('\n')
			if err != nil {
				return
			}
			b.WriteString(line)
			if line == "\n" {
				s.frames <- b.String()
				b.Reset()
			}
		}
	}()
	return s
}

// next returns the next frame, failing the test after the timeout.
func (s *stream) next() string {
	s.t.Helper()
	select {
	case f := <-s.frames:
		return f
	case <-time.After(10 * time.Second):
		s.t.Fatal("no frame within 10s")
	}
	return ""
}

// greeting reads the retry frame and the subscribed event.
func (s *stream) greeting() (retry, subscribed string) {
	s.t.Helper()
	return s.next(), s.next()
}

// nextEvent skips heartbeats and returns the next real frame.
func (s *stream) nextEvent() string {
	s.t.Helper()
	for {
		if f := s.next(); !strings.HasPrefix(f, ":") {
			return f
		}
	}
}

// ended reports whether the stream's connection closes within d, after any frames already queued.
func (s *stream) ended(d time.Duration) bool {
	select {
	case <-s.closed:
		return true
	case <-time.After(d):
		return false
	}
}

// drainFrames returns the frames queued until the connection closes or d passes.
func (s *stream) framesUntilClosed(d time.Duration) []string {
	var out []string
	deadline := time.After(d)
	for {
		select {
		case f := <-s.frames:
			out = append(out, f)
		case <-s.closed:
			for {
				select {
				case f := <-s.frames:
					out = append(out, f)
				default:
					return out
				}
			}
		case <-deadline:
			return out
		}
	}
}

// ── small helpers ────────────────────────────────────────────────────────

func readTestdata(t *testing.T, name string) map[string]any {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "conformance", "push", name))
	require.NoError(t, err)
	var m map[string]any
	require.NoError(t, json.Unmarshal(raw, &m))
	return m
}

func eventually(t *testing.T, what string, within time.Duration, f func() bool) {
	t.Helper()
	deadline := time.Now().Add(within)
	for time.Now().Before(deadline) {
		if f() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

var _ = fwtest.NewClock
