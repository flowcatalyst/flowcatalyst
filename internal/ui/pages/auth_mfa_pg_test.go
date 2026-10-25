//go:build integration

package pages

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"
	"github.com/go-webauthn/webauthn/protocol"
	"github.com/pquerna/otp/totp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth/login"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth/loginbackoff"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth/mfatoken"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth/provider"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/loginattempt"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/mfa"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/role"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apiroute"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/email"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/encryption"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/webauthn"
	webauthnapi "github.com/flowcatalyst/flowcatalyst-go/internal/platform/webauthn/api"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/webauthn/webauthntest"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/guard"
)

// The two-factor pages against Postgres with the real services: the mfa
// service (TOTP with its replay guard, recovery codes, trusted devices), the
// token issuer, the session provider, the sign-in backoff store and the
// passkey repository. The page code is the code under test; nothing here
// replaces a rule.

type pgMailer struct{ last email.Message }

func (m *pgMailer) Send(_ context.Context, msg email.Message) error { m.last = msg; return nil }

type pgRig struct {
	t        *testing.T
	page     *AuthMFA
	mfa      *mfa.Service
	issuer   *mfatoken.Issuer
	attempts *loginattempt.Repository
	creds    *webauthn.Repository
	ann, bob string
	handler  http.Handler
}

func newPGRig(t *testing.T, tag string) *pgRig {
	t.Helper()
	ctx := context.Background()
	pool := testpg.Pool(t)
	mk := func(id, name string) string {
		_, err := pool.Exec(ctx,
			`INSERT INTO iam_principals (id, type, scope, name, active, email)
			 VALUES ($1, 'USER', 'ANCHOR', $2, TRUE, $3)`, id, name, name+"@"+tag+".test")
		require.NoError(t, err)
		return id
	}
	key, err := encryption.GenerateKey()
	require.NoError(t, err)
	enc, err := encryption.New(key)
	require.NoError(t, err)
	rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	pemKey := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(rsaKey)})

	principals := principal.NewRepository(pool)
	prov, err := provider.NewProvider(provider.Config{Issuer: "flowcatalyst", Audience: "flowcatalyst", SigningKey: pemKey}, principals, role.NewRepository(pool))
	require.NoError(t, err)

	r := &pgRig{
		t: t, mfa: mfa.NewService(mfa.NewRepository(pool), enc, &pgMailer{}, mfa.DefaultConfig()),
		issuer: mfatoken.NewIssuer(rsaKey, "flowcatalyst"), attempts: loginattempt.NewRepository(pool),
		creds: webauthn.NewRepository(pool),
		ann:   mk("prn_"+tag+"ann", "ann"), bob: mk("prn_"+tag+"bob", "bob"),
	}
	r.page = &AuthMFA{
		Flow: &login.TwoFactor{
			MFA: r.mfa, Tokens: r.issuer, Policy: fakePolicy{ev: mfaRequired(true, "TOTP", "EMAIL_PIN")}, Principals: principals,
			Attempts: r.attempts, Backoff: loginbackoff.Policy{FreeAttempts: 1, BaseDelaySecs: 60, MaxDelaySecs: 60, GlobalWindowSecs: 3600, GlobalCeiling: 100, GlobalLockSecs: 900},
			Sessions: prov, SessionTTL: time.Hour,
		},
		Principals: principals,
		Passkeys:   r.creds, PasskeyOps: PasskeyOps{Repo: r.creds, UoW: testpg.NewUoW(t)}, Shell: shellForTest(),
	}
	mux := chi.NewRouter()
	mux.Route("/ui", func(rt chi.Router) {
		rt.Use(guard.SecurityHeaders)
		r.page.MountPublic(rt)
		rt.Group(func(rt chi.Router) {
			rt.Use(func(next http.Handler) http.Handler {
				return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
					if id := req.Header.Get("X-Test-Principal"); id != "" {
						req = req.WithContext(auth.WithContext(req.Context(), &auth.AuthContext{
							PrincipalID: id, Scope: auth.ScopeAnchor, Roles: []string{"r"}, PrincipalType: auth.PrincipalTypeUser,
						}))
					}
					next.ServeHTTP(w, req)
				})
			}, guard.RequireSession)
			r.page.MountSession(rt)
		})
	})
	r.handler = mux
	return r
}

// do sends a request and returns the recorder; as is the signed-in principal.
func (r *pgRig) do(method, target string, form url.Values, as string, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	r.t.Helper()
	var body io.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	}
	req := httptest.NewRequest(method, target, body)
	req.Header.Set("Accept", "text/html")
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Sec-Fetch-Site", "same-origin")
	}
	if as != "" {
		req.Header.Set("X-Test-Principal", as)
	}
	req.RemoteAddr = "203.0.113.9:1234"
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	r.handler.ServeHTTP(rec, req)
	return rec
}

func (r *pgRig) token(id string, purpose mfatoken.Purpose) *http.Cookie {
	r.t.Helper()
	tok, err := r.issuer.Mint(id, purpose, time.Minute)
	require.NoError(r.t, err)
	return &http.Cookie{Name: authChallengeCookie, Value: tok}
}

var (
	secretOnPage = regexp.MustCompile(`id="mfa-totp-secret">([A-Z2-7]+)<`)
	uriOnPage    = regexp.MustCompile(`name="uri" value="([^"]+)"`)
	codeOnPage   = regexp.MustCompile(`<li><code>([A-Z0-9]{5}-[A-Z0-9]{5})</code></li>`)
)

// A user the domain compels to enroll gets a session only by enrolling with a
// real authenticator code; the QR page and the recovery codes appear once; the
// next sign-in is a challenge that the same factor, a recovery code (once) and
// nothing else satisfies.
func TestEnrollThenChallengeAgainstPostgres(t *testing.T) {
	r := newPGRig(t, "pgenr")
	enroll := r.token(r.ann, mfatoken.PurposeEnroll)

	// There is no step past the enrolment without a confirmed factor.
	rec := r.do(http.MethodPost, "/ui/auth/2fa/enroll/done", url.Values{}, "", enroll)
	assert.GreaterOrEqual(t, rec.Code, 400)
	assert.Nil(t, mfaCookie(rec, "fc_session"))

	// Begin: the key and a QR code for it, in this response.
	rec = r.do(http.MethodPost, "/ui/auth/2fa/enroll/totp", url.Values{}, "", enroll)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	secret := secretOnPage.FindStringSubmatch(rec.Body.String())
	uri := uriOnPage.FindStringSubmatch(rec.Body.String())
	require.NotNil(t, secret, rec.Body.String())
	require.NotNil(t, uri)
	assert.Contains(t, rec.Body.String(), "<svg ")
	assert.Equal(t, "no-store", rec.Header().Get("Cache-Control"))
	assert.NotContains(t, rec.Header().Get("Set-Cookie"), secret[1])

	// A wrong code keeps the user where they are.
	rec = r.do(http.MethodPost, "/ui/auth/2fa/enroll/totp/confirm", url.Values{"code": {"000000"}, "uri": {htmlUnescape(uri[1])}}, "", enroll)
	assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	assert.Nil(t, mfaCookie(rec, "fc_session"))

	// The right code: the session, with the recovery codes once.
	code, err := totp.GenerateCode(secret[1], time.Now())
	require.NoError(t, err)
	rec = r.do(http.MethodPost, "/ui/auth/2fa/enroll/totp/confirm", url.Values{"code": {code}, "uri": {htmlUnescape(uri[1])}, "next": {"/ui/users"}}, "", enroll)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	recovery := codeOnPage.FindAllStringSubmatch(rec.Body.String(), -1)
	require.Len(t, recovery, 10, rec.Body.String())
	assert.NotContains(t, rec.Body.String(), secret[1])
	assert.Contains(t, rec.Body.String(), `href="/ui/users"`)
	session := mfaCookie(rec, "fc_session")
	require.NotNil(t, session)
	assert.NotEmpty(t, session.Value)

	// The enroll token is spent: with a usable factor it enrols nothing more
	// and signs nobody in, even with a factor of its own.
	for _, step := range []string{"/ui/auth/2fa/enroll/totp", "/ui/auth/2fa/enroll/email"} {
		rec = r.do(http.MethodPost, step, url.Values{}, "", enroll)
		mfaWantRedirect(t, rec, guard.LoginPath)
		assert.Nil(t, mfaCookie(rec, "fc_session"))
	}
	methods, err := r.mfa.ConfirmedMethods(context.Background(), r.ann)
	require.NoError(t, err)
	assert.Equal(t, []mfa.MethodType{mfa.MethodTOTP}, methods)

	// The same recovery codes are on no later page.
	rec = r.do(http.MethodGet, "/ui/auth/security", nil, r.ann)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.NotContains(t, rec.Body.String(), recovery[0][1])
	assert.NotContains(t, rec.Body.String(), secret[1])
	assert.Contains(t, rec.Body.String(), "10 unused codes remaining.")

	// Next sign-in: the challenge. The enroll token no longer buys a session
	// without a usable factor... it has one now, so the challenge is the gate
	// users meet; a pending token with a wrong code is refused.
	pending := r.token(r.ann, mfatoken.PurposePending)
	rec = r.do(http.MethodPost, "/ui/auth/2fa/verify", url.Values{"method": {"TOTP"}, "code": {"000000"}}, "", pending)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.Nil(t, mfaCookie(rec, "fc_session"))

	// A code for another step: valid within the skew, not the one just used.
	next, err := totp.GenerateCode(secret[1], time.Now().Add(30*time.Second))
	require.NoError(t, err)
	rec = r.do(http.MethodPost, "/ui/auth/2fa/verify", url.Values{"method": {"TOTP"}, "code": {next}, "remember": {"1"}}, "", pending)
	mfaWantRedirect(t, rec, "/ui/")
	assert.NotNil(t, mfaCookie(rec, "fc_session"))
	device := mfaCookie(rec, testTrustedDeviceCookie)
	require.NotNil(t, device, "the domain allows remembering and the box was ticked")

	// A recovery code works once.
	form := url.Values{"method": {"RECOVERY_CODE"}, "code": {recovery[0][1]}}
	rec = r.do(http.MethodPost, "/ui/auth/2fa/verify", form, "", r.token(r.ann, mfatoken.PurposePending))
	mfaWantRedirect(t, rec, "/ui/")
	rec = r.do(http.MethodPost, "/ui/auth/2fa/verify", form, "", r.token(r.ann, mfatoken.PurposePending))
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.Nil(t, mfaCookie(rec, "fc_session"))

	// The remembered device is listed for Ann, and only she can revoke it.
	devices, err := r.mfa.ListTrustedDevices(context.Background(), r.ann)
	require.NoError(t, err)
	require.Len(t, devices, 1)
	rec = r.do(http.MethodGet, "/ui/auth/security", nil, r.ann)
	assert.Contains(t, rec.Body.String(), "/ui/auth/security/2fa/devices/"+devices[0].ID+"/revoke")
	r.do(http.MethodPost, "/ui/auth/security/2fa/devices/"+devices[0].ID+"/revoke", url.Values{}, r.bob)
	left, err := r.mfa.ListTrustedDevices(context.Background(), r.ann)
	require.NoError(t, err)
	assert.Len(t, left, 1, "another user's session revoked Ann's device")
	r.do(http.MethodPost, "/ui/auth/security/2fa/devices/"+devices[0].ID+"/revoke", url.Values{}, r.ann)
	left, _ = r.mfa.ListTrustedDevices(context.Background(), r.ann)
	assert.Empty(t, left)
}

// Wrong codes feed the same (email, IP) backoff the password step uses; while
// it holds, even the right code is refused and no session is issued.
func TestChallengeBackoffAgainstPostgres(t *testing.T) {
	r := newPGRig(t, "pgbof")
	ctx := context.Background()
	enr, err := r.mfa.BeginTOTPEnrollment(ctx, r.ann, "ann@pgbof.test")
	require.NoError(t, err)
	code, err := totp.GenerateCode(enr.Secret, time.Now())
	require.NoError(t, err)
	ok, err := r.mfa.ConfirmTOTPEnrollment(ctx, r.ann, code)
	require.NoError(t, err)
	require.True(t, ok)

	pending := r.token(r.ann, mfatoken.PurposePending)
	var last *httptest.ResponseRecorder
	for range 3 {
		last = r.do(http.MethodPost, "/ui/auth/2fa/verify", url.Values{"method": {"TOTP"}, "code": {"000000"}}, "", pending)
	}
	assert.Equal(t, http.StatusTooManyRequests, last.Code, last.Body.String())
	assert.NotEmpty(t, last.Header().Get("Retry-After"))

	good, err := totp.GenerateCode(enr.Secret, time.Now().Add(30*time.Second))
	require.NoError(t, err)
	rec := r.do(http.MethodPost, "/ui/auth/2fa/verify", url.Values{"method": {"TOTP"}, "code": {good}}, "", pending)
	assert.Equal(t, http.StatusTooManyRequests, rec.Code)
	assert.Nil(t, mfaCookie(rec, "fc_session"), "a session was issued while the backoff held")
}

// The email factor: the code goes to the user's address, the wrong code does
// not sign in, the right one does.
func TestEmailChallengeAgainstPostgres(t *testing.T) {
	r := newPGRig(t, "pgeml")
	ctx := context.Background()
	mailer := &pgMailer{}
	key, _ := encryption.GenerateKey()
	enc, _ := encryption.New(key)
	svc := mfa.NewService(mfa.NewRepository(testpg.Pool(t)), enc, mailer, mfa.DefaultConfig())
	r.page.Flow.MFA = svc
	require.NoError(t, svc.BeginEmailEnrollment(ctx, r.ann, "ann@pgeml.test"))
	pin := regexp.MustCompile(`\d{6}`).FindString(mailer.last.HTMLBody)
	ok, err := svc.ConfirmEmailEnrollment(ctx, r.ann, pin)
	require.NoError(t, err)
	require.True(t, ok)

	pending := r.token(r.ann, mfatoken.PurposePending)
	rec := r.do(http.MethodPost, "/ui/auth/2fa/email", url.Values{}, "", pending)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Equal(t, "ann@pgeml.test", mailer.last.To)
	pin = regexp.MustCompile(`\d{6}`).FindString(mailer.last.HTMLBody)
	require.NotEmpty(t, pin)
	// Asking again at once sends nothing: the code just sent is the one to use.
	mailer.last = email.Message{}
	rec = r.do(http.MethodPost, "/ui/auth/2fa/email", url.Values{}, "", pending)
	assert.Equal(t, http.StatusTooManyRequests, rec.Code, rec.Body.String())
	assert.NotEmpty(t, rec.Header().Get("Retry-After"))
	assert.Contains(t, rec.Body.String(), "A code was sent recently.")
	assert.Empty(t, mailer.last.To, "a second code was emailed within the resend interval")
	rec = r.do(http.MethodPost, "/ui/auth/2fa/verify", url.Values{"method": {"EMAIL_PIN"}, "code": {"000000"}}, "", pending)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	rec = r.do(http.MethodPost, "/ui/auth/2fa/verify", url.Values{"method": {"EMAIL_PIN"}, "code": {pin}}, "", pending)
	mfaWantRedirect(t, rec, "/ui/")
	assert.NotNil(t, mfaCookie(rec, "fc_session"))
}

// ── passkeys ───────────────────────────────────────────────────────────

const (
	testRPID   = "localhost"
	testOrigin = "http://localhost:8080"
)

// The browser half of a passkey ceremony is fc-webauthn.js; its server half is
// the platform's WebAuthn endpoints, which it calls with the JSON shapes
// @simplewebauthn/browser produces. This drives those endpoints the way it
// does, with the repository's software authenticator in place of the browser's
// (headless, no browser): register a passkey while signed in, sign in with it
// (the response sets the session cookie), see it on the security page, and have
// only its owner remove it there.
func TestPasskeyCeremonyAndSecurityPageAgainstPostgres(t *testing.T) {
	r := newPGRig(t, "pgpky")
	pool := testpg.Pool(t)
	svc, err := webauthn.NewService(webauthn.Config{RPDisplayName: "FC", RPID: testRPID, RPOrigins: []string{testOrigin}},
		r.creds, webauthn.NewCeremonyRepository(pool))
	require.NoError(t, err)
	prov := r.page.Flow.Sessions.(*provider.Provider)

	api := chi.NewRouter()
	api.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			if id := req.Header.Get("X-Test-Principal"); id != "" {
				req = req.WithContext(auth.WithContext(req.Context(), &auth.AuthContext{PrincipalID: id}))
			}
			next.ServeHTTP(w, req)
		})
	})
	webauthnapi.Register(humachi.New(api, apiroute.PlatformAPIConfig("webauthn-ui-test", "test")), &webauthnapi.State{
		Service: svc, Principals: principal.NewRepository(pool), Creds: r.creds, UoW: testpg.NewUoW(t),
		Provider: prov, SessionTTL: time.Hour, LoginAttempts: r.attempts, BackoffPolicy: loginbackoff.Policy{},
	})
	srv := httptest.NewServer(api)
	t.Cleanup(srv.Close)

	jar, err := cookiejar.New(nil)
	require.NoError(t, err)
	client := &http.Client{Jar: jar}
	postJSON := func(path string, as string, body any) (int, []byte, []*http.Cookie) {
		t.Helper()
		b, err := json.Marshal(body)
		require.NoError(t, err)
		req, err := http.NewRequest(http.MethodPost, srv.URL+path, bytes.NewReader(b))
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")
		if as != "" {
			req.Header.Set("X-Test-Principal", as)
		}
		resp, err := client.Do(req)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()
		out, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, out, resp.Cookies()
	}

	// Registration, as the security page's form runs it (begin, then complete
	// with the name).
	a := webauthntest.New(t)
	status, out, _ := postJSON("/auth/webauthn/register/begin", r.ann, map[string]any{"displayName": "Work laptop"})
	require.Equal(t, http.StatusOK, status, string(out))
	var begin struct {
		StateID string `json:"stateId"`
		Options struct {
			PublicKey protocol.PublicKeyCredentialCreationOptions `json:"publicKey"`
		} `json:"options"`
	}
	require.NoError(t, json.Unmarshal(out, &begin))
	status, out, _ = postJSON("/auth/webauthn/register/complete", r.ann, map[string]any{
		"stateId": begin.StateID, "name": "Work laptop",
		"credential": json.RawMessage(a.Register(begin.Options.PublicKey.Challenge.String(), testOrigin, testRPID)),
	})
	require.Equal(t, http.StatusOK, status, string(out))

	// Sign in with it: the assertion in, the session cookie out.
	status, out, _ = postJSON("/auth/webauthn/authenticate/begin", "", map[string]any{"email": "ann@pgpky.test"})
	require.Equal(t, http.StatusOK, status, string(out))
	var abegin struct {
		StateID string `json:"stateId"`
		Options struct {
			PublicKey protocol.PublicKeyCredentialRequestOptions `json:"publicKey"`
		} `json:"options"`
	}
	require.NoError(t, json.Unmarshal(out, &abegin))
	status, out, cookies := postJSON("/auth/webauthn/authenticate/complete", "", map[string]any{
		"stateId":    abegin.StateID,
		"credential": json.RawMessage(a.Assert(abegin.Options.PublicKey.Challenge.String(), testOrigin, testRPID, []byte(r.ann))),
	})
	require.Equal(t, http.StatusOK, status, string(out))
	var sess *http.Cookie
	for _, c := range cookies {
		if c.Name == "fc_session" {
			sess = c
		}
	}
	require.NotNil(t, sess, "a passkey sign-in sets the session cookie")
	assert.True(t, sess.HttpOnly)
	// A forged assertion (wrong signature key) does not.
	other := webauthntest.New(t)
	status, out, _ = postJSON("/auth/webauthn/authenticate/begin", "", map[string]any{"email": "ann@pgpky.test"})
	require.Equal(t, http.StatusOK, status)
	require.NoError(t, json.Unmarshal(out, &abegin))
	status, out, cookies = postJSON("/auth/webauthn/authenticate/complete", "", map[string]any{
		"stateId":    abegin.StateID,
		"credential": json.RawMessage(other.Assert(abegin.Options.PublicKey.Challenge.String(), testOrigin, testRPID, []byte(r.ann))),
	})
	// (This test router lacks the platform's error mapper, which turns the
	// refusal into a 401; the refusal itself is what matters here.)
	assert.GreaterOrEqual(t, status, 400, string(out))
	assert.Contains(t, string(out), "INVALID_CREDENTIALS")
	assert.Empty(t, cookies)

	// The security page lists it for Ann alone, and only Ann removes it.
	rec := r.do(http.MethodGet, "/ui/auth/security", nil, r.ann)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Contains(t, rec.Body.String(), "Work laptop")
	keys, err := r.creds.FindByPrincipal(context.Background(), r.ann)
	require.NoError(t, err)
	require.Len(t, keys, 1)
	remove := "/ui/auth/security/passkeys/" + keys[0].ID + "/remove"
	assert.NotContains(t, r.do(http.MethodGet, "/ui/auth/security", nil, r.bob).Body.String(), "Work laptop")
	rec = r.do(http.MethodPost, remove, url.Values{}, r.bob)
	assert.Equal(t, http.StatusNotFound, rec.Code)
	still, err := r.creds.FindByID(context.Background(), keys[0].ID)
	require.NoError(t, err)
	assert.NotNil(t, still, "another user's session removed Ann's passkey")
	rec = r.do(http.MethodPost, remove, url.Values{}, r.ann)
	assert.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	gone, err := r.creds.FindByID(context.Background(), keys[0].ID)
	require.NoError(t, err)
	assert.Nil(t, gone)
}

func htmlUnescape(s string) string {
	return strings.NewReplacer("&amp;", "&", "&#39;", "'", "&#34;", `"`, "&lt;", "<", "&gt;", ">").Replace(s)
}
