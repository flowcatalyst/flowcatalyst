//go:build integration

package fieldauth

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/asn1"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth/authservice"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/client"
	clientops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/client/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/fieldidentity"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/fieldidentity/pin"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/ratelimit"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/go-sdk/usecaseop"
)

const (
	baseURL = "https://fc.example.test"
	issuer  = "https://fc.example.test"
)

var seq atomic.Int64

// env is one isolated client with two applications and a running protocol.
type env struct {
	t        *testing.T
	s        *State
	router   chi.Router
	clientID string
	ident    string // client identifier
	appA     string // application ids
	appB     string
	codeA    string // application codes
	codeB    string
	actx     context.Context
	privPEM  []byte
	auth     *authservice.AuthService
}

type envOpts struct {
	noAppKey bool
	limits   *ratelimit.Policies
}

func newEnv(t *testing.T, o envOpts) *env {
	t.Helper()
	pool := testpg.Pool(t)
	n := seq.Add(1)
	clients := client.NewRepository(pool)
	uow := testpg.NewUoW(t)
	identifier := fmt.Sprintf("fa-co-%d-%d", n, time.Now().UnixNano()%100000)
	ev, err := usecaseop.Run(context.Background(), uow, clientops.CreateClient(clients),
		clientops.CreateCommand{Name: fmt.Sprintf("Field Auth Co %d", n), Identifier: identifier}, testpg.TestEC())
	require.NoError(t, err)

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	privPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	pubDER, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	require.NoError(t, err)
	pubPEM := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubDER})
	svc, err := authservice.NewWithRSA(authservice.Config{Issuer: issuer, Audience: "flowcatalyst"}, string(privPEM), string(pubPEM))
	require.NoError(t, err)

	var hasher *pin.Hasher
	if !o.noAppKey {
		hasher, err = pin.NewHasher("field-auth-test-app-key")
		require.NoError(t, err)
	}
	limits := ratelimit.PoliciesFromEnv()
	if o.limits != nil {
		limits = *o.limits
	}
	codeA, codeB := fmt.Sprintf("fldauta%d", n), fmt.Sprintf("fldautb%d", n)
	e := &env{
		t: t, clientID: ev.ClientID, ident: identifier, codeA: codeA, codeB: codeB,
		appA: testpg.EnsureApplication(t, codeA), appB: testpg.EnsureApplication(t, codeB),
		actx: adminCtx(), privPEM: privPEM, auth: svc,
	}
	e.s = &State{
		Ops: &fieldidentity.Ops{
			R: fieldidentity.NewRepos(pool), Clients: clients, Apps: application.NewRepository(pool), Hasher: hasher,
		},
		UoW: uow, Auth: svc, BaseURL: baseURL, RateLimit: ratelimit.NewPostgresStore(pool), Limits: limits,
	}
	e.router = chi.NewRouter()
	e.s.RegisterRoutes(e.router)
	return e
}

func authContext() *auth.AuthContext {
	return &auth.AuthContext{
		PrincipalID: "prn_optestrunner1", Scope: auth.ScopeAnchor, Permissions: []string{"platform:*:*:*"},
		AllApplications: true, PrincipalType: auth.PrincipalTypeUser,
	}
}

func adminCtx() context.Context { return testpg.WithAuth(context.Background(), authContext()) }

// worker creates a worker holding the apps, with a working PIN (no forced
// change unless keepPCR).
func (e *env) worker(code, pinValue string, keepPCR bool, apps ...string) string {
	e.t.Helper()
	ev, err := usecaseop.Run(e.actx, e.s.UoW, e.s.Ops.EnsureWorker(), fieldidentity.EnsureWorkerCommand{
		ClientID: e.clientID, WorkerCode: code, Name: "Worker " + code, AppCodes: apps, PIN: pinValue,
	}, auth.NewExecutionContext(e.actx))
	require.NoError(e.t, err)
	id := ev.ID
	if !keepPCR {
		_, err = testpg.Pool(e.t).Exec(context.Background(),
			`UPDATE field_identities SET pin_change_required = FALSE WHERE id = $1`, id)
		require.NoError(e.t, err)
	}
	return id
}

// enrolCode mints an enrolment code through the admin use case.
func (e *env) enrolCode(mode fieldidentity.Mode, assignedWorker string, apps ...string) string {
	e.t.Helper()
	code, err := fieldidentity.NewEnrolmentCodeValue()
	require.NoError(e.t, err)
	_, err = usecaseop.RunTx(e.actx, e.s.UoW, e.s.Ops.CreateEnrolmentCode(), fieldidentity.CreateEnrolmentCodeCommand{
		ClientID: e.clientID, Mode: string(mode), AppCodes: apps, AssignedWorkerID: assignedWorker, Code: code,
	}, auth.NewExecutionContext(e.actx))
	require.NoError(e.t, err)
	return code
}

// ── a device ─────────────────────────────────────────────────────────────

// device is a test client holding an ES256 key.
type device struct {
	t     *testing.T
	e     *env
	key   *ecdsa.PrivateKey
	nonce string
	id    string // device id once enrolled
	// lastProof is the most recent proof header, for replay tests.
	lastProof string
}

func (e *env) newDevice() *device {
	k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(e.t, err)
	return &device{t: e.t, e: e, key: k}
}

func (d *device) jwk() map[string]string {
	pt, err := d.key.PublicKey.Bytes() // 0x04 || X || Y
	require.NoError(d.t, err)
	x, y := pt[1:33], pt[33:65]
	return map[string]string{
		"kty": "EC", "crv": "P-256",
		"x": base64.RawURLEncoding.EncodeToString(x), "y": base64.RawURLEncoding.EncodeToString(y),
	}
}

func (d *device) jkt() string {
	j := d.jwk()
	canonical := `{"crv":"P-256","kty":"EC","x":"` + j["x"] + `","y":"` + j["y"] + `"}`
	sum := sha256.Sum256([]byte(canonical))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func b64json(t *testing.T, v any) string {
	raw, err := json.Marshal(v)
	require.NoError(t, err)
	return base64.RawURLEncoding.EncodeToString(raw)
}

// proofOpts overrides parts of a proof to build bad ones.
type proofOpts struct {
	htm, htu, nonce, ath string
	iat                  time.Time
	signWith             *ecdsa.PrivateKey
	jwkOf                *device
}

func (d *device) proof(method, path, accessToken string, o proofOpts) string {
	d.t.Helper()
	if o.htm == "" {
		o.htm = method
	}
	if o.htu == "" {
		o.htu = baseURL + path
	}
	if o.nonce == "" {
		o.nonce = d.nonce
	}
	if o.iat.IsZero() {
		o.iat = time.Now()
	}
	claims := map[string]any{"htm": o.htm, "htu": o.htu, "iat": o.iat.Unix(), "jti": fmt.Sprintf("j-%d", seq.Add(1))}
	if o.nonce != "" {
		claims["nonce"] = o.nonce
	}
	if accessToken != "" {
		claims["ath"] = AccessTokenHash(accessToken)
	}
	if o.ath != "" {
		claims["ath"] = o.ath
	}
	jwkSource := d
	if o.jwkOf != nil {
		jwkSource = o.jwkOf
	}
	header := map[string]any{"alg": "ES256", "typ": "dpop+jwt", "jwk": jwkSource.jwk()}
	input := b64json(d.t, header) + "." + b64json(d.t, claims)
	digest := sha256.Sum256([]byte(input))
	signer := d.key
	if o.signWith != nil {
		signer = o.signWith
	}
	der, err := ecdsa.SignASN1(rand.Reader, signer, digest[:])
	require.NoError(d.t, err)
	var rs struct{ R, S *big.Int }
	_, err = asn1.Unmarshal(der, &rs)
	require.NoError(d.t, err)
	sig := append(rs.R.FillBytes(make([]byte, 32)), rs.S.FillBytes(make([]byte, 32))...)
	return input + "." + base64.RawURLEncoding.EncodeToString(sig)
}

// resp is a recorded response.
type resp struct {
	*httptest.ResponseRecorder
}

func (r resp) json() map[string]any {
	var m map[string]any
	_ = json.Unmarshal(r.Body.Bytes(), &m)
	return m
}

func (r resp) str(key string) string {
	v, _ := r.json()[key].(string)
	return v
}

// fetchNonce gets a fresh nonce through /field/auth/nonce.
func (d *device) fetchNonce() string {
	d.t.Helper()
	d.nonce = ""
	rec := d.raw(http.MethodPost, "/field/auth/nonce", nil, d.proof(http.MethodPost, "/field/auth/nonce", "", proofOpts{}), "")
	require.Equal(d.t, http.StatusOK, rec.Code, rec.Body.String())
	n := rec.str("nonce")
	require.NotEmpty(d.t, n)
	require.Equal(d.t, n, rec.Header().Get("DPoP-Nonce"), "the body nonce is also the header nonce")
	d.nonce = n
	return n
}

// raw sends one request with the given proof header and optional access token
// (Authorization: DPoP). It records the response's DPoP-Nonce as the device's
// next nonce.
func (d *device) raw(method, path string, body any, proofHeader, accessToken string) resp {
	d.t.Helper()
	var buf bytes.Buffer
	if body != nil {
		require.NoError(d.t, json.NewEncoder(&buf).Encode(body))
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	if proofHeader != "" {
		req.Header.Set("DPoP", proofHeader)
	}
	if accessToken != "" {
		req.Header.Set("Authorization", "DPoP "+accessToken)
	}
	rec := httptest.NewRecorder()
	d.e.router.ServeHTTP(rec, req)
	d.lastProof = proofHeader
	if n := rec.Header().Get("DPoP-Nonce"); n != "" {
		d.nonce = n
	}
	return resp{rec}
}

// call is the normal path: a proof built with the device's current nonce (the
// previous response's DPoP-Nonce, or a fetched one on first use).
func (d *device) call(path string, body any) resp { return d.callBearer(path, body, "") }

func (d *device) callBearer(path string, body any, accessToken string) resp {
	d.t.Helper()
	if d.nonce == "" {
		d.fetchNonce()
	}
	return d.raw(http.MethodPost, path, body, d.proof(http.MethodPost, path, accessToken, proofOpts{}), accessToken)
}

// enrol enrols the device with a code, recording its id.
func (d *device) enrol(code string) resp {
	d.t.Helper()
	r := d.call("/field/devices/enrol", map[string]any{
		"code": code, "publicKeyJwk": d.jwk(), "platform": "android", "model": "Test Phone 1",
	})
	if r.Code == http.StatusCreated {
		d.id = r.str("deviceId")
	}
	return r
}

func (d *device) login(app, worker, pinValue string) resp {
	return d.call("/field/auth/login", map[string]any{"appCode": app, "workerCode": worker, "pin": pinValue})
}

func (d *device) refresh(token string) resp {
	return d.call("/field/auth/refresh", map[string]any{"refreshToken": token})
}

// enrolled returns a device enrolled for apps (PERSONAL, unassigned).
func (e *env) enrolled(mode fieldidentity.Mode, apps ...string) *device {
	e.t.Helper()
	d := e.newDevice()
	r := d.enrol(e.enrolCode(mode, "", apps...))
	require.Equal(e.t, http.StatusCreated, r.Code, r.Body.String())
	return d
}

// signedIn is a worker signed in on a device.
type signedIn struct {
	access, refresh string
	body            map[string]any
}

func mustLogin(t *testing.T, d *device, app, worker, pinValue string) signedIn {
	t.Helper()
	r := d.login(app, worker, pinValue)
	require.Equal(t, http.StatusOK, r.Code, r.Body.String())
	return signedIn{access: r.str("accessToken"), refresh: r.str("refreshToken"), body: r.json()}
}

func (e *env) q(sql string, args ...any) int {
	e.t.Helper()
	var n int
	require.NoError(e.t, testpg.Pool(e.t).QueryRow(context.Background(), sql, args...).Scan(&n))
	return n
}

func (e *env) eventCount(eventType string) int {
	return e.q(`SELECT COUNT(*) FROM msg_events WHERE type = $1 AND data::text LIKE '%' || $2 || '%'`, eventType, e.clientID)
}

func (e *env) assertNoLeak(secrets ...string) {
	e.t.Helper()
	for _, s := range secrets {
		if s == "" {
			continue
		}
		leaked := e.q(`SELECT (SELECT COUNT(*) FROM msg_events WHERE data::text LIKE '%' || $1 || '%')
		   + (SELECT COUNT(*) FROM aud_logs WHERE operation_json::text LIKE '%' || $1 || '%')
		   + (SELECT COUNT(*) FROM field_login_attempts WHERE failure_reason LIKE '%' || $1 || '%')`, s)
		require.Zero(e.t, leaked, "a secret leaked into events, audit or the attempt log: %q", strings.Repeat("*", len(s)))
	}
}
