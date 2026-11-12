//go:build integration

package fieldauth

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/fieldidentity"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/ratelimit"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	sdkauth "github.com/flowcatalyst/go-sdk/auth"
	"github.com/flowcatalyst/go-sdk/usecaseop"
)

// ── badges ───────────────────────────────────────────────────────────────

func TestBadgeLoginWithPIN(t *testing.T) {
	e := newEnv(t, envOpts{limits: roomyLimits()})
	workerID := e.worker("bg-1", goodPIN, false, e.codeA)
	token := e.badge(workerID)
	d := e.enrolled(fieldidentity.ModePersonal, e.codeA)
	un := e.newDevice()

	r := d.loginBadge(e.codeA, token, goodPIN)
	require.Equal(t, http.StatusOK, r.Code, r.Body.String())
	assert.Equal(t, []any{"badge", "pin"}, tokenPayload(t, r.str("accessToken"))["amr"])
	assert.Equal(t, workerID, tokenPayload(t, r.str("accessToken"))["sub"])
	assert.Equal(t, 1, e.q(`SELECT COUNT(*) FROM field_badges WHERE identity_id = $1 AND last_used_at IS NOT NULL`, workerID))
	assert.Equal(t, 1, e.q(`SELECT COUNT(*) FROM field_login_attempts WHERE identity_id = $1 AND auth_method = 'BADGE_PIN' AND outcome = 'SUCCESS'`, workerID))

	// Lower case (a scanner or typing) is the same badge; an unenrolled key
	// signs in with it, no client identifier needed (the badge carries it).
	lower := []byte(token)
	for i := range lower {
		if lower[i] >= 'A' && lower[i] <= 'Z' {
			lower[i] += 'a' - 'A'
		}
	}
	r = un.loginBadge(e.codeA, string(lower), goodPIN)
	require.Equal(t, http.StatusOK, r.Code, r.Body.String())
	assert.NotContains(t, tokenPayload(t, r.str("accessToken")), "device_id")

	// A wrong PIN is the normal failure path and counts towards the lock.
	r = d.loginBadge(e.codeA, token, "000111")
	assert.Equal(t, http.StatusUnauthorized, r.Code)
	assert.Equal(t, "INVALID_CREDENTIALS", r.str("error"))
	assert.Equal(t, 1, e.q(`SELECT failed_since_success FROM field_identities WHERE id = $1`, workerID))

	// The token is stored hashed and is nowhere else.
	assert.Equal(t, 0, e.q(`SELECT COUNT(*) FROM field_badges WHERE token_hash = $1`, token), "never stored in clear")
	assert.Equal(t, 1, e.q(`SELECT COUNT(*) FROM field_badges WHERE token_hash = $1`, fieldidentity.HashToken(token)))
	e.assertNoLeak(token, string(lower))
}

func TestBadgeOnlyNeedsPolicyAndAnEnrolledDevice(t *testing.T) {
	e := newEnv(t, envOpts{limits: roomyLimits()})
	workerID := e.worker("bo-1", goodPIN, false, e.codeA)
	token := e.badge(workerID)
	d := e.enrolled(fieldidentity.ModePersonal, e.codeA)
	un := e.newDevice()

	// Without the policy a badge needs its PIN, enrolled or not.
	for name, dev := range map[string]*device{"enrolled": d, "unenrolled": un} {
		r := dev.loginBadge(e.codeA, token, "")
		assert.Equal(t, http.StatusBadRequest, r.Code, "%s: %s", name, r.Body.String())
		assert.Equal(t, "PIN_REQUIRED", r.str("error"), name)
	}
	assert.Equal(t, 0, e.q(`SELECT failed_since_success FROM field_identities WHERE id = $1`, workerID), "PIN_REQUIRED costs nothing")

	e.policy("badge_only_login", true)
	// Enrolled and allowed: signs in, amr is the badge alone.
	r := d.loginBadge(e.codeA, token, "")
	require.Equal(t, http.StatusOK, r.Code, r.Body.String())
	assert.Equal(t, []any{"badge"}, tokenPayload(t, r.str("accessToken"))["amr"])
	assert.Equal(t, 1, e.q(`SELECT COUNT(*) FROM field_login_attempts WHERE identity_id = $1 AND auth_method = 'BADGE' AND outcome = 'SUCCESS'`, workerID))
	// A photocopied badge on an unenrolled key still needs the PIN, whatever the policy.
	r = un.loginBadge(e.codeA, token, "")
	assert.Equal(t, http.StatusBadRequest, r.Code, r.Body.String())
	assert.Equal(t, "PIN_REQUIRED", r.str("error"))

	// A locked worker is refused on badge-only, soft or hard.
	_, err := testpg.Pool(t).Exec(context.Background(), `UPDATE field_identities SET hard_locked = TRUE WHERE id = $1`, workerID)
	require.NoError(t, err)
	r = d.loginBadge(e.codeA, token, "")
	assert.Equal(t, http.StatusLocked, r.Code, r.Body.String())
	assert.Equal(t, "WORKER_LOCKED", r.str("error"))
	_, err = testpg.Pool(t).Exec(context.Background(),
		`UPDATE field_identities SET hard_locked = FALSE, locked_until = NOW() + INTERVAL '10 minutes' WHERE id = $1`, workerID)
	require.NoError(t, err)
	r = d.loginBadge(e.codeA, token, "")
	assert.Equal(t, http.StatusTooManyRequests, r.Code, r.Body.String())
	// A suspended worker is refused like a wrong credential.
	_, err = testpg.Pool(t).Exec(context.Background(),
		`UPDATE field_identities SET locked_until = NULL, status = 'DISABLED' WHERE id = $1`, workerID)
	require.NoError(t, err)
	r = d.loginBadge(e.codeA, token, "")
	assert.Equal(t, http.StatusUnauthorized, r.Code)
	assert.Equal(t, "INVALID_CREDENTIALS", r.str("error"))
}

func TestRevokedUnknownAndForeignBadges(t *testing.T) {
	lim := roomyLimits()
	lim.FieldPinDevice = ratelimit.Policy{Window: time.Hour, Limit: 3}
	e := newEnv(t, envOpts{limits: lim})
	other := newEnv(t, envOpts{limits: roomyLimits()})
	workerID := e.worker("rb-1", goodPIN, false, e.codeA)
	first := e.badge(workerID)
	second := e.badge(workerID) // reissue revokes the first
	foreignWorker := other.worker("rb-2", goodPIN, false, other.codeA)
	foreign := other.badge(foreignWorker)
	d := e.enrolled(fieldidentity.ModePersonal, e.codeA)

	assert.Equal(t, 1, e.q(`SELECT COUNT(*) FROM field_badges WHERE identity_id = $1 AND status = 'ACTIVE'`, workerID), "one active badge per worker")
	r := d.loginBadge(e.codeA, first, goodPIN)
	assert.Equal(t, http.StatusUnauthorized, r.Code, r.Body.String())
	assert.Equal(t, "INVALID_CREDENTIALS", r.str("error"))
	assert.Equal(t, 1, e.q(`SELECT COUNT(*) FROM field_login_attempts WHERE failure_reason = 'BADGE_REVOKED'`))
	r = d.loginBadge(e.codeA, "FWB1NOTAREALBADGE", goodPIN)
	assert.Equal(t, "INVALID_CREDENTIALS", r.str("error"))
	// Another client's badge on this client's enrolled device.
	r = d.loginBadge(e.codeA, foreign, goodPIN)
	assert.Equal(t, http.StatusUnauthorized, r.Code, r.Body.String())
	assert.Equal(t, "INVALID_CREDENTIALS", r.str("error"))
	// Each of those counted against the device bucket (limit 3): it is full now.
	r = d.loginBadge(e.codeA, second, goodPIN)
	assert.Equal(t, http.StatusTooManyRequests, r.Code, r.Body.String())

	// A named client that is not the badge's: refused as a wrong credential.
	d2 := e.enrolled(fieldidentity.ModePersonal, e.codeA)
	r = d2.call("/field/auth/login", map[string]any{
		"appCode": e.codeA, "badgeToken": second, "pin": goodPIN, "clientIdentifier": other.ident,
	})
	assert.Equal(t, http.StatusUnauthorized, r.Code, r.Body.String())
	r = d2.call("/field/auth/login", map[string]any{
		"appCode": e.codeA, "badgeToken": second, "pin": goodPIN, "clientIdentifier": e.ident,
	})
	assert.Equal(t, http.StatusOK, r.Code, r.Body.String())
}

// ── test identities ──────────────────────────────────────────────────────

func TestTestIdentityTokenCarriesTestClaim(t *testing.T) {
	e := newEnv(t, envOpts{limits: roomyLimits()})
	admin := e.adminAPI()
	real := e.worker("ti-real", goodPIN, false, e.codeA)
	test := e.worker("ti-test", goodPIN, false, e.codeA)
	_, err := testpg.Pool(t).Exec(context.Background(), `UPDATE field_identities SET is_test = TRUE WHERE id = $1`, test)
	require.NoError(t, err)
	d := e.enrolled(fieldidentity.ModePersonal, e.codeA)

	assert.NotContains(t, tokenPayload(t, mustLogin(t, d, e.codeA, "ti-real", goodPIN).access), "test")
	in := mustLogin(t, d, e.codeA, "ti-test", goodPIN) // otherwise a normal identity (PIN, locks)
	assert.Equal(t, true, tokenPayload(t, in.access)["test"])
	_ = real

	jwks := e.jwksServer()
	v := sdkauth.NewFieldTokenVerifier(sdkauth.FieldTokenVerifierConfig{Issuer: issuer, AppCode: e.codeA, JWKSURL: jwks.URL})
	appURL := "https://app.example.test/api/orders"
	p, err := v.Verify(context.Background(), "DPoP "+in.access, d.proof(http.MethodPost, "/x", in.access, proofOpts{htu: appURL}), http.MethodPost, appURL)
	require.NoError(t, err)
	assert.True(t, p.Test)
	// Introspection reports it.
	m := adminPost(t, admin, "/api/field-tokens/introspect", map[string]any{"token": in.access})
	assert.Equal(t, true, m["active"])
	assert.Equal(t, true, m["test"])
}

// ── act-as (supervisor) ──────────────────────────────────────────────────

// actAsEnv is a supervisor signed in on an enrolled device, and a target.
type actAsEnv struct {
	e        *env
	d        *device
	supID    string
	targetID string
	sup      signedIn
}

func newActAsEnv(t *testing.T, limits *ratelimit.Policies) *actAsEnv {
	t.Helper()
	e := newEnv(t, envOpts{limits: limits})
	supID := e.supervisor("sup-1", goodPIN, e.codeA)
	targetID := e.worker("tgt-1", "739204", true, e.codeA) // a pending PIN change the actor must not inherit
	d := e.enrolled(fieldidentity.ModePersonal, e.codeA)
	return &actAsEnv{e: e, d: d, supID: supID, targetID: targetID, sup: mustLogin(t, d, e.codeA, "sup-1", goodPIN)}
}

func (a *actAsEnv) actAs(body map[string]any) resp {
	return a.d.callBearer("/field/auth/act-as", body, a.sup.access)
}

func TestActAsHappyPath(t *testing.T) {
	a := newActAsEnv(t, roomyLimits())
	e := a.e
	admin := e.adminAPI()
	supFamily := tokenPayload(t, a.sup.access)["sid"].(string)

	r := a.actAs(map[string]any{"workerCode": "tgt-1", "pin": goodPIN})
	require.Equal(t, http.StatusOK, r.Code, r.Body.String())
	acc := r.str("accessToken")
	claims := tokenPayload(t, acc)
	assert.Equal(t, a.targetID, claims["sub"], "apps see the worker")
	assert.Equal(t, "TGT-1", claims["worker_code"])
	assert.Equal(t, map[string]any{"sub": a.supID, "typ": "field_worker", "name": "Worker sup-1"}, claims["act"])
	assert.Equal(t, []any{"pin", "imp"}, claims["amr"])
	assert.NotContains(t, claims, "pcr", "the worker's pending PIN change is not forced on the actor")
	assert.Equal(t, false, r.json()["pinChangeRequired"])
	assert.Equal(t, a.d.id, claims["device_id"])
	assert.NotEqual(t, supFamily, claims["sid"], "a new family; the supervisor's own session is untouched")
	assert.Equal(t, 1, e.q(`SELECT COUNT(*) FROM field_sessions WHERE family_id = $1 AND revoked_at IS NULL`, supFamily))
	assert.Equal(t, 1, e.q(`SELECT COUNT(*) FROM field_sessions WHERE family_id = $1 AND actor_type = 'FIELD_WORKER' AND actor_id = $2`,
		claims["sid"], a.supID))

	// Apps read the actor; introspection reports it.
	jwks := e.jwksServer()
	v := sdkauth.NewFieldTokenVerifier(sdkauth.FieldTokenVerifierConfig{Issuer: issuer, AppCode: e.codeA, JWKSURL: jwks.URL})
	appURL := "https://app.example.test/api/orders"
	p, err := v.Verify(context.Background(), "DPoP "+acc, a.d.proof(http.MethodPost, "/x", acc, proofOpts{htu: appURL}), http.MethodPost, appURL)
	require.NoError(t, err)
	require.NotNil(t, p.Actor)
	assert.Equal(t, a.supID, p.Actor.ID)
	assert.Equal(t, "field_worker", p.Actor.Type)
	m := adminPost(t, admin, "/api/field-tokens/introspect", map[string]any{"token": acc})
	assert.Equal(t, true, m["active"])
	assert.Equal(t, map[string]any{"sub": a.supID, "typ": "field_worker"}, m["act"])

	// The record: attempt row (target + actor), event, and an audit row that
	// reads "the supervisor impersonated the worker".
	assert.Equal(t, 1, e.q(`SELECT COUNT(*) FROM field_login_attempts WHERE auth_method = 'ACT_AS' AND outcome = 'SUCCESS'
		AND identity_id = $1 AND actor_id = $2`, a.targetID, a.supID))
	assert.Equal(t, 1, e.q(`SELECT COUNT(*) FROM msg_events WHERE type = $1 AND data::text LIKE '%' || $2 || '%' AND data::text LIKE '%ACT_AS%'`,
		fieldidentity.WorkerImpersonatedType, a.targetID))
	assert.Equal(t, 1, e.q(`SELECT COUNT(*) FROM aud_logs WHERE principal_id = $1 AND entity_id = $2 AND entity_type = 'Field-worker'
		AND operation = 'ImpersonationInfo' AND operation_json::text LIKE '%ACT_AS%'`, a.supID, a.targetID),
		"the audit row: actor = principal, worker = entity")

	// Refresh keeps the actor, the amr and no pcr.
	rr := a.d.refresh(r.str("refreshToken"))
	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
	rc := tokenPayload(t, rr.str("accessToken"))
	assert.Equal(t, claims["act"], rc["act"])
	assert.Equal(t, []any{"pin", "imp"}, rc["amr"])
	assert.NotContains(t, rc, "pcr")
	e.assertNoLeak(goodPIN, "739204")
}

func TestActAsByWorkerIDAndLockedTargetAllowed(t *testing.T) {
	a := newActAsEnv(t, roomyLimits())
	_, err := testpg.Pool(t).Exec(context.Background(), `UPDATE field_identities SET hard_locked = TRUE WHERE id = $1`, a.targetID)
	require.NoError(t, err)
	r := a.actAs(map[string]any{"workerId": a.targetID, "pin": goodPIN})
	require.Equal(t, http.StatusOK, r.Code, "covering for a locked-out worker is a normal reason: %s", r.Body.String())
	// Exactly one of workerCode / workerId.
	assert.Equal(t, http.StatusBadRequest, a.actAs(map[string]any{"workerId": a.targetID, "workerCode": "tgt-1", "pin": goodPIN}).Code)
	assert.Equal(t, http.StatusBadRequest, a.actAs(map[string]any{"pin": goodPIN}).Code)
}

func TestActAsAbsoluteLimitIsCapped(t *testing.T) {
	a := newActAsEnv(t, roomyLimits())
	e := a.e
	supFamily := tokenPayload(t, a.sup.access)["sid"].(string)

	// Capped by the supervisor's own session.
	_, err := testpg.Pool(t).Exec(context.Background(),
		`UPDATE field_sessions SET absolute_expires_at = NOW() + INTERVAL '100 seconds' WHERE family_id = $1`, supFamily)
	require.NoError(t, err)
	r := a.actAs(map[string]any{"workerCode": "tgt-1", "pin": goodPIN})
	require.Equal(t, http.StatusOK, r.Code, r.Body.String())
	fam := tokenPayload(t, r.str("accessToken"))["sid"].(string)
	assert.Equal(t, 1, e.q(`SELECT COUNT(*) FROM field_sessions WHERE family_id = $1 AND absolute_expires_at - issued_at <= INTERVAL '101 seconds'`, fam),
		"no longer than the supervisor's own session")

	// Capped by the policy's impersonation limit.
	_, err = testpg.Pool(t).Exec(context.Background(),
		`UPDATE field_sessions SET absolute_expires_at = NOW() + INTERVAL '10 hours' WHERE family_id = $1`, supFamily)
	require.NoError(t, err)
	e.policy("impersonation_max_secs", 300)
	r = a.actAs(map[string]any{"workerCode": "tgt-1", "pin": goodPIN})
	require.Equal(t, http.StatusOK, r.Code, r.Body.String())
	fam = tokenPayload(t, r.str("accessToken"))["sid"].(string)
	assert.Equal(t, 1, e.q(`SELECT COUNT(*) FROM field_sessions WHERE family_id = $1 AND absolute_expires_at - issued_at BETWEEN INTERVAL '299 seconds' AND INTERVAL '301 seconds'`, fam))
}

func TestActAsWrongSupervisorPINCountsTowardsTheSupervisorLock(t *testing.T) {
	a := newActAsEnv(t, roomyLimits())
	e := a.e
	for i := 1; i <= 5; i++ {
		r := a.actAs(map[string]any{"workerCode": "tgt-1", "pin": "000111"})
		require.Equal(t, http.StatusUnauthorized, r.Code, r.Body.String())
		assert.Equal(t, "INVALID_CREDENTIALS", r.str("error"))
		assert.Equal(t, i, e.q(`SELECT failed_since_success FROM field_identities WHERE id = $1`, a.supID))
	}
	// Soft locked now: the right PIN is refused outright.
	r := a.actAs(map[string]any{"workerCode": "tgt-1", "pin": goodPIN})
	assert.Equal(t, http.StatusForbidden, r.Code, r.Body.String())
	assert.Equal(t, "IMPERSONATION_FORBIDDEN", r.str("error"))
	assert.Equal(t, 0, e.q(`SELECT COUNT(*) FROM field_sessions WHERE actor_id = $1`, a.supID))
	assert.GreaterOrEqual(t, e.q(`SELECT COUNT(*) FROM field_login_attempts WHERE auth_method = 'ACT_AS' AND failure_reason = 'PIN_INVALID' AND identity_id = $1`, a.supID), 5)
	e.assertNoLeak("000111")
}

func TestActAsRefusedForANonSupervisorWithoutAPINCheck(t *testing.T) {
	e := newEnv(t, envOpts{limits: roomyLimits()})
	plain := e.worker("np-1", goodPIN, false, e.codeA)
	e.worker("np-2", goodPIN, false, e.codeA)
	d := e.enrolled(fieldidentity.ModePersonal, e.codeA)
	in := mustLogin(t, d, e.codeA, "np-1", goodPIN)
	r := d.callBearer("/field/auth/act-as", map[string]any{"workerCode": "np-2", "pin": "000111"}, in.access)
	assert.Equal(t, http.StatusForbidden, r.Code, r.Body.String())
	assert.Equal(t, "IMPERSONATION_FORBIDDEN", r.str("error"))
	assert.Equal(t, 0, e.q(`SELECT failed_since_success FROM field_identities WHERE id = $1`, plain), "the supervisor is authenticated already: no PIN check")
}

func TestActAsTargetRulesAnswerUniform403(t *testing.T) {
	a := newActAsEnv(t, roomyLimits())
	e := a.e
	other := newEnv(t, envOpts{limits: roomyLimits()})
	e.supervisor("sup-2", goodPIN, e.codeA)
	e.worker("tgt-nogrant", goodPIN, false, e.codeB)
	suspended := e.worker("tgt-off", goodPIN, false, e.codeA)
	_, err := testpg.Pool(t).Exec(context.Background(), `UPDATE field_identities SET status = 'DISABLED' WHERE id = $1`, suspended)
	require.NoError(t, err)
	foreign := other.worker("tgt-1", goodPIN, false, other.codeA)

	cases := []struct {
		name   string
		body   map[string]any
		reason string
	}{
		{"another supervisor", map[string]any{"workerCode": "sup-2"}, "TARGET_IS_SUPERVISOR"},
		{"themselves", map[string]any{"workerCode": "sup-1"}, "TARGET_IS_SELF"},
		{"no grant to the app", map[string]any{"workerCode": "tgt-nogrant"}, "NO_GRANT"},
		{"suspended", map[string]any{"workerCode": "tgt-off"}, "TARGET_INACTIVE"},
		{"unknown", map[string]any{"workerCode": "nobody"}, "TARGET_NOT_FOUND"},
		{"another client's worker", map[string]any{"workerId": foreign}, "TARGET_OTHER_CLIENT"},
	}
	for _, c := range cases {
		c.body["pin"] = goodPIN
		r := a.actAs(c.body)
		assert.Equal(t, http.StatusForbidden, r.Code, "%s: %s", c.name, r.Body.String())
		assert.Equal(t, "NO_APP_ACCESS", r.str("error"), c.name)
		assert.Equal(t, 1, e.q(`SELECT COUNT(*) FROM field_login_attempts WHERE auth_method = 'ACT_AS' AND failure_reason = $1 AND actor_id = $2`,
			c.reason, a.supID), "%s: the reason is recorded, not returned", c.name)
	}
	assert.Equal(t, 0, e.q(`SELECT COUNT(*) FROM field_sessions WHERE actor_id = $1`, a.supID))
}

func TestImpersonatedSessionCannotNestOrChangeThePIN(t *testing.T) {
	a := newActAsEnv(t, roomyLimits())
	e := a.e
	e.worker("tgt-2", goodPIN, false, e.codeA)
	r := a.actAs(map[string]any{"workerCode": "tgt-1", "pin": goodPIN})
	require.Equal(t, http.StatusOK, r.Code, r.Body.String())
	imp := r.str("accessToken")

	n := a.d.callBearer("/field/auth/act-as", map[string]any{"workerCode": "tgt-2", "pin": goodPIN}, imp)
	assert.Equal(t, http.StatusForbidden, n.Code, n.Body.String())
	assert.Equal(t, "IMPERSONATING", n.str("error"))
	p := a.d.callBearer("/field/auth/pin", map[string]any{"currentPin": "739204", "newPin": "846201"}, imp)
	assert.Equal(t, http.StatusForbidden, p.Code, p.Body.String())
	assert.Equal(t, "IMPERSONATING", p.str("error"))
	assert.Equal(t, 0, e.q(`SELECT failed_since_success FROM field_identities WHERE id = $1`, a.targetID))
}

func TestSuspendingOrDemotingTheSupervisorEndsImpersonatedSessions(t *testing.T) {
	for _, how := range []string{"suspend", "clear canImpersonate", "delete"} {
		t.Run(how, func(t *testing.T) {
			a := newActAsEnv(t, roomyLimits())
			e := a.e
			admin := e.adminAPI()
			r := a.actAs(map[string]any{"workerCode": "tgt-1", "pin": goodPIN})
			require.Equal(t, http.StatusOK, r.Code, r.Body.String())
			imp, impRefresh := r.str("accessToken"), r.str("refreshToken")
			require.True(t, e.introspectActive(admin, imp))
			fam := tokenPayload(t, imp)["sid"].(string)

			ref := fieldidentity.WorkerRef{ClientID: e.clientID, ID: a.supID}
			var err error
			switch how {
			case "suspend":
				_, err = usecaseop.RunTx(e.actx, e.s.UoW, e.s.Ops.SuspendWorker(), ref, auth.NewExecutionContext(e.actx))
			case "delete":
				_, err = usecaseop.RunTx(e.actx, e.s.UoW, e.s.Ops.DeleteWorker(), ref, auth.NewExecutionContext(e.actx))
			default:
				off := false
				_, err = usecaseop.RunTx(e.actx, e.s.UoW, e.s.Ops.UpdateWorker(),
					fieldidentity.UpdateWorkerCommand{WorkerRef: ref, Name: "Sup", CanImpersonate: &off}, auth.NewExecutionContext(e.actx))
			}
			require.NoError(t, err)

			assert.False(t, e.introspectActive(admin, imp), "the impersonated session is over")
			if how != "delete" {
				assert.Equal(t, 1, e.q(`SELECT COUNT(*) FROM field_sessions WHERE family_id = $1 AND revoked_reason = 'ACTOR_SUSPENDED'`, fam))
			}
			assert.Equal(t, http.StatusUnauthorized, a.d.refresh(impRefresh).Code)
			// The worker's own sessions are untouched by losing an actor.
			assert.Equal(t, 0, e.q(`SELECT COUNT(*) FROM field_sessions WHERE identity_id = $1 AND revoked_at IS NULL`, a.targetID))
		})
	}
}

func TestActAsWorksFromAnUnenrolledKey(t *testing.T) {
	e := newEnv(t, envOpts{limits: roomyLimits()})
	supID := e.supervisor("us-1", goodPIN, e.codeA)
	e.worker("ut-1", goodPIN, false, e.codeA)
	d := e.newDevice()
	sup := mustLoginUn(t, d, e.codeA, "us-1", goodPIN)
	r := d.callBearer("/field/auth/act-as", map[string]any{"workerCode": "ut-1", "pin": goodPIN}, sup.access)
	require.Equal(t, http.StatusOK, r.Code, r.Body.String())
	claims := tokenPayload(t, r.str("accessToken"))
	assert.NotContains(t, claims, "device_id")
	assert.Equal(t, "UNENROLLED", claims["device_mode"])
	assert.Equal(t, supID, claims["act"].(map[string]any)["sub"])

	e.policy("require_enrolled_device", true)
	r = d.callBearer("/field/auth/act-as", map[string]any{"workerCode": "ut-1", "pin": goodPIN}, sup.access)
	assert.Equal(t, http.StatusUnauthorized, r.Code, r.Body.String())
	assert.Equal(t, "UNKNOWN_DEVICE", r.str("error"))
}

// ── console sign-in codes ────────────────────────────────────────────────

func TestSignInCodeRedeemedOnceAsPrincipal(t *testing.T) {
	e := newEnv(t, envOpts{limits: roomyLimits()})
	admin := e.adminAPI()
	workerID := e.worker("sc-1", goodPIN, true, e.codeA) // pending PIN change: not forced on the actor
	principal := e.principal(true)
	code := e.signInCode(workerID, e.codeA, principal)
	d := e.newDevice() // any device; unenrolled here

	r := d.call("/field/auth/sign-in-code", map[string]any{"code": code})
	require.Equal(t, http.StatusOK, r.Code, r.Body.String())
	acc := r.str("accessToken")
	claims := tokenPayload(t, acc)
	assert.Equal(t, workerID, claims["sub"])
	assert.Equal(t, e.codeA, claims["aud"])
	assert.Equal(t, map[string]any{"sub": principal, "typ": "principal", "name": "Console Admin"}, claims["act"])
	assert.Equal(t, []any{"code", "imp"}, claims["amr"])
	assert.NotContains(t, claims, "pcr")
	assert.NotContains(t, claims, "device_id")
	assert.Equal(t, 1, e.q(`SELECT COUNT(*) FROM field_sessions WHERE family_id = $1 AND actor_type = 'PRINCIPAL' AND actor_id = $2 AND jkt = $3`,
		claims["sid"], principal, d.jkt()))
	require.True(t, e.introspectActive(admin, acc))

	// Single use, even with a lower-case paste.
	again := d.call("/field/auth/sign-in-code", map[string]any{"code": code})
	assert.Equal(t, http.StatusUnauthorized, again.Code, again.Body.String())
	assert.Equal(t, "INVALID_CODE", again.str("error"))

	// Attempt row, event and the audit row (actor = principal, entity = worker).
	assert.Equal(t, 1, e.q(`SELECT COUNT(*) FROM field_login_attempts WHERE auth_method = 'SIGN_IN_CODE' AND outcome = 'SUCCESS'
		AND identity_id = $1 AND actor_id = $2 AND device_id IS NULL`, workerID, principal))
	assert.Equal(t, 1, e.q(`SELECT COUNT(*) FROM msg_events WHERE type = $1 AND data::text LIKE '%SIGN_IN_CODE%' AND data::text LIKE '%' || $2 || '%'`,
		fieldidentity.WorkerImpersonatedType, workerID))
	assert.Equal(t, 1, e.q(`SELECT COUNT(*) FROM aud_logs WHERE principal_id = $1 AND entity_id = $2 AND entity_type = 'Field-worker'
		AND operation = 'ImpersonationInfo' AND operation_json::text LIKE '%SIGN_IN_CODE%'`, principal, workerID),
		"the audit row: the principal impersonated the worker")
	// The creation is audited as the calling admin.
	assert.Equal(t, 1, e.q(`SELECT COUNT(*) FROM aud_logs WHERE principal_id = $1 AND entity_id = $2 AND operation = 'CreateSignInCodeCommand'`,
		principal, workerID))
	e.assertNoLeak(code, goodPIN)

	// Refresh keeps the principal as actor.
	rr := d.refresh(r.str("refreshToken"))
	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
	assert.Equal(t, claims["act"], tokenPayload(t, rr.str("accessToken"))["act"])
}

func TestSignInCodeRefusals(t *testing.T) {
	e := newEnv(t, envOpts{limits: roomyLimits()})
	workerID := e.worker("sr-1", goodPIN, false, e.codeA)
	live := e.principal(true)
	d := e.newDevice()
	unused := func(code string) int {
		return e.q(`SELECT COUNT(*) FROM field_sign_in_codes WHERE code_hash = $1 AND used_at IS NULL`, fieldidentity.HashToken(code))
	}

	// Unknown / malformed.
	r := d.call("/field/auth/sign-in-code", map[string]any{"code": "FWS1NOPE"})
	assert.Equal(t, http.StatusUnauthorized, r.Code)
	assert.Equal(t, "INVALID_CODE", r.str("error"))

	// Expired.
	expired := e.signInCode(workerID, e.codeA, live)
	_, err := testpg.Pool(t).Exec(context.Background(), `UPDATE field_sign_in_codes SET expires_at = NOW() - INTERVAL '1 second' WHERE code_hash = $1`, fieldidentity.HashToken(expired))
	require.NoError(t, err)
	r = d.call("/field/auth/sign-in-code", map[string]any{"code": expired})
	assert.Equal(t, http.StatusUnauthorized, r.Code, r.Body.String())
	assert.Equal(t, "INVALID_CODE", r.str("error"))

	// The creator has been deactivated since: INVALID_CODE, and the code is not burnt.
	creator := e.principal(true)
	byGone := e.signInCode(workerID, e.codeA, creator)
	_, err = testpg.Pool(t).Exec(context.Background(), `UPDATE iam_principals SET active = FALSE WHERE id = $1`, creator)
	require.NoError(t, err)
	r = d.call("/field/auth/sign-in-code", map[string]any{"code": byGone})
	assert.Equal(t, http.StatusUnauthorized, r.Code, r.Body.String())
	assert.Equal(t, "INVALID_CODE", r.str("error"))
	assert.Equal(t, 1, unused(byGone))

	// The worker was suspended or lost the grant since: refused, code unused.
	suspended := e.signInCode(workerID, e.codeA, live)
	_, err = testpg.Pool(t).Exec(context.Background(), `UPDATE field_identities SET status = 'DISABLED' WHERE id = $1`, workerID)
	require.NoError(t, err)
	r = d.call("/field/auth/sign-in-code", map[string]any{"code": suspended})
	assert.Equal(t, http.StatusForbidden, r.Code, r.Body.String())
	assert.Equal(t, "NO_APP_ACCESS", r.str("error"))
	assert.Equal(t, 1, unused(suspended))
	_, err = testpg.Pool(t).Exec(context.Background(), `UPDATE field_identities SET status = 'ACTIVE' WHERE id = $1`, workerID)
	require.NoError(t, err)

	// The client requires an enrolled device.
	e.policy("require_enrolled_device", true)
	r = d.call("/field/auth/sign-in-code", map[string]any{"code": suspended})
	assert.Equal(t, http.StatusUnauthorized, r.Code, r.Body.String())
	assert.Equal(t, "UNKNOWN_DEVICE", r.str("error"))
	assert.Equal(t, 1, unused(suspended))

	// An enrolled device restricted to another application.
	other := e.enrolled(fieldidentity.ModePersonal, e.codeB)
	r = other.call("/field/auth/sign-in-code", map[string]any{"code": suspended})
	assert.Equal(t, http.StatusForbidden, r.Code, r.Body.String())
	assert.Equal(t, "NO_APP_ACCESS", r.str("error"))
	// An enrolled device for the app redeems it, and the session limits are the device's.
	ok := e.enrolled(fieldidentity.ModeShared, e.codeA)
	r = ok.call("/field/auth/sign-in-code", map[string]any{"code": suspended})
	require.Equal(t, http.StatusOK, r.Code, r.Body.String())
	assert.Equal(t, ok.id, tokenPayload(t, r.str("accessToken"))["device_id"])
}

func TestSignInCodeCappedByImpersonationLimitAndCountsFailures(t *testing.T) {
	lim := roomyLimits()
	lim.FieldPinIP = ratelimit.Policy{Window: time.Hour, Limit: 2}
	e := newEnv(t, envOpts{limits: lim})
	workerID := e.worker("sl-1", goodPIN, false, e.codeA)
	live := e.principal(true)
	e.policy("impersonation_max_secs", 600)
	d := e.newDevice()
	code := e.signInCode(workerID, e.codeA, live)

	// Bad codes count against the subject (IP) bucket; at the limit even a good
	// code is 429.
	for range 2 {
		assert.Equal(t, http.StatusUnauthorized, d.call("/field/auth/sign-in-code", map[string]any{"code": "FWS1WRONGWRONGWRONGWRONGWRONG"}).Code)
	}
	r := d.call("/field/auth/sign-in-code", map[string]any{"code": code})
	assert.Equal(t, http.StatusTooManyRequests, r.Code, r.Body.String())

	fresh := e.newDevice()
	r = fresh.call("/field/auth/sign-in-code", map[string]any{"code": code})
	require.Equal(t, http.StatusOK, r.Code, r.Body.String())
	fam := tokenPayload(t, r.str("accessToken"))["sid"].(string)
	assert.Equal(t, 1, e.q(`SELECT COUNT(*) FROM field_sessions WHERE family_id = $1 AND absolute_expires_at - issued_at BETWEEN INTERVAL '599 seconds' AND INTERVAL '601 seconds'`, fam))
}

func TestSignInCodeForTestIdentityGivesTestClaim(t *testing.T) {
	e := newEnv(t, envOpts{limits: roomyLimits()})
	workerID := e.worker("st-1", goodPIN, false, e.codeA)
	_, err := testpg.Pool(t).Exec(context.Background(), `UPDATE field_identities SET is_test = TRUE WHERE id = $1`, workerID)
	require.NoError(t, err)
	code := e.signInCode(workerID, e.codeA, e.principal(true))
	r := e.newDevice().call("/field/auth/sign-in-code", map[string]any{"code": code})
	require.Equal(t, http.StatusOK, r.Code, r.Body.String())
	claims := tokenPayload(t, r.str("accessToken"))
	assert.Equal(t, true, claims["test"])
	assert.Equal(t, "principal", claims["act"].(map[string]any)["typ"])
}
