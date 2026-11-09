//go:build integration

package sdksync

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	appops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/application/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/audit"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/pushgw"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/go-sdk/usecase"
	"github.com/flowcatalyst/go-sdk/usecaseop"
)

func newPushSyncState(t *testing.T) (*State, map[string]string) {
	t.Helper()
	pool := testpg.Pool(t)
	uow := testpg.NewUoW(t)
	apps := application.NewRepository(pool)
	ids := map[string]string{}
	for _, code := range []string{"pushsyncalpha", "pushsyncbeta"} {
		a, err := apps.FindByCode(t.Context(), code)
		require.NoError(t, err)
		if a == nil { // the package's database is shared: create once, start every test clean
			_, err = usecaseop.Run(t.Context(), uow, appops.CreateApplication(apps), appops.CreateCommand{Code: code, Name: code}, testpg.TestEC())
			require.NoError(t, err)
			a, err = apps.FindByCode(t.Context(), code)
			require.NoError(t, err)
		}
		ids[code] = string(a.ID)
		for _, q := range []string{
			`DELETE FROM push_rules WHERE application_id = $1`, `DELETE FROM push_ticket_keys WHERE application_id = $1`,
			`DELETE FROM push_app_settings WHERE application_id = $1`, `DELETE FROM aud_logs WHERE entity_id = $1`,
		} {
			_, err = pool.Exec(t.Context(), q, a.ID)
			require.NoError(t, err)
		}
	}
	return &State{Apps: apps, Push: pushgw.NewConfigRepo(pool), Audit: audit.NewRepository(pool), UoW: uow}, ids
}

func ctxOf(ac *auth.AuthContext) context.Context { return auth.WithContext(context.Background(), ac) }

func ed25519JWK(x string) syncPushKeyRequest {
	return syncPushKeyRequest{Kty: "OKP", Crv: "Ed25519", X: base64.RawURLEncoding.EncodeToString([]byte(x + "0123456789012345678901234567890")[:32])}
}

func appSA(ids ...string) *auth.AuthContext {
	return &auth.AuthContext{
		PrincipalID: "prn_svc", Scope: auth.ScopeAnchor, Applications: ids,
		Permissions: []string{"platform:application-service:push:sync"}, PrincipalType: auth.PrincipalTypeService,
	}
}

func syncReq(app string, rules []syncPushRuleRequest, keys ...syncPushKeyRequest) *syncPushInput {
	return &syncPushInput{AppCode: app, Body: syncPushRequest{Rules: rules, Keys: syncPushKeysRequest{Keys: keys}}}
}

func TestSyncPush_ReplaceSemantics(t *testing.T) {
	s, ids := newPushSyncState(t)
	ctx := ctxOf(appSA(ids["pushsyncalpha"]))
	k1, k2 := ed25519JWK("a"), ed25519JWK("b")
	k1.Kid, k2.Kid = "k1", "k2"

	out, err := s.syncPush(ctx, syncReq("pushsyncalpha", []syncPushRuleRequest{
		{Pattern: "pushsyncalpha:order:{id}", Requires: []string{"shop:order:view"}},
		{Pattern: "pushsyncalpha:worker:{principal}:jobs"},
		{Pattern: "pushsyncalpha:board", Public: true},
	}, k1))
	require.NoError(t, err)
	assert.Equal(t, PushSyncCounts{Created: 3}, out.Body.Rules)
	assert.Equal(t, PushSyncCounts{Created: 1}, out.Body.Keys)
	assert.Equal(t, []string{"k1"}, out.Body.SyncedKids)
	assert.Len(t, out.Body.SyncedPatterns, 3)

	// the second sync IS the set: one rule kept (changed), one dropped, one added; a key added
	out, err = s.syncPush(ctx, syncReq("pushsyncalpha", []syncPushRuleRequest{
		{Pattern: "pushsyncalpha:order:{id}", Requires: []string{"shop:order:edit"}},
		{Pattern: "pushsyncalpha:other:{id}", Requires: []string{"x:y:z"}},
	}, k1, k2))
	require.NoError(t, err)
	assert.Equal(t, PushSyncCounts{Created: 1, Updated: 1, Deleted: 2}, out.Body.Rules)
	assert.Equal(t, PushSyncCounts{Created: 1, Updated: 1}, out.Body.Keys)

	rules, err := s.Push.Rules(ctx, ids["pushsyncalpha"])
	require.NoError(t, err)
	require.Len(t, rules, 2)
	assert.Equal(t, "pushsyncalpha:order:{id}", rules[0].Pattern)
	assert.Equal(t, []string{"shop:order:edit"}, rules[0].Requires)

	// an empty sync empties the set
	out, err = s.syncPush(ctx, syncReq("pushsyncalpha", nil))
	require.NoError(t, err)
	assert.Equal(t, PushSyncCounts{Deleted: 2}, out.Body.Rules)
	assert.Equal(t, PushSyncCounts{Deleted: 2}, out.Body.Keys)

	// another application's set is untouched throughout
	_, err = s.syncPush(ctxOf(appSA(ids["pushsyncbeta"])), syncReq("pushsyncbeta", []syncPushRuleRequest{{Pattern: "pushsyncbeta:a:{id}", Requires: []string{"x:y:z"}}}))
	require.NoError(t, err)
	_, err = s.syncPush(ctx, syncReq("pushsyncalpha", nil))
	require.NoError(t, err)
	bRules, err := s.Push.Rules(ctx, ids["pushsyncbeta"])
	require.NoError(t, err)
	assert.Len(t, bRules, 1)
}

func TestSyncPush_Authorization(t *testing.T) {
	s, ids := newPushSyncState(t)
	rule := []syncPushRuleRequest{{Pattern: "pushsyncalpha:a:{id}", Requires: []string{"x:y:z"}}}

	for name, ac := range map[string]*auth.AuthContext{
		"no authentication":        nil,
		"no permission":            {PrincipalID: "p", Scope: auth.ScopeAnchor, AllApplications: true, Permissions: []string{"platform:application-service:docs:sync"}},
		"the console's view right": {PrincipalID: "p", Scope: auth.ScopeAnchor, AllApplications: true, Permissions: []string{"platform:admin:push:manage"}},
	} {
		_, err := s.syncPush(ctxOf(ac), syncReq("pushsyncalpha", rule))
		require.Error(t, err, name)
	}

	// an application-scoped service account syncs its own application only
	_, err := s.syncPush(ctxOf(appSA(ids["pushsyncbeta"])), syncReq("pushsyncalpha", rule))
	require.Error(t, err)
	assert.Equal(t, usecase.KindAuthorization, usecase.AsError(err).Kind, "another application is forbidden")
	got, _ := s.Push.Rules(t.Context(), ids["pushsyncalpha"])
	assert.Empty(t, got, "and nothing was written")

	// a caller of all applications may sync any
	_, err = s.syncPush(ctxOf(&auth.AuthContext{PrincipalID: "p", Scope: auth.ScopeAnchor, AllApplications: true, Permissions: []string{"platform:*:*:*"}}), syncReq("pushsyncalpha", rule))
	assert.NoError(t, err)

	// an unknown application is a 404
	_, err = s.syncPush(ctxOf(appSA(ids["pushsyncalpha"])), syncReq("nosuchapp", rule))
	require.Error(t, err)
	assert.Equal(t, usecase.KindNotFound, usecase.AsError(err).Kind)
}

func TestSyncPush_Refusals(t *testing.T) {
	s, ids := newPushSyncState(t)
	ctx := ctxOf(appSA(ids["pushsyncalpha"]))
	good := ed25519JWK("g")
	good.Kid = "kg"
	_, err := s.syncPush(ctx, syncReq("pushsyncalpha", []syncPushRuleRequest{{Pattern: "pushsyncalpha:keep:{id}", Requires: []string{"x:y:z"}}}, good))
	require.NoError(t, err)

	five := make([]syncPushKeyRequest, 5)
	for i := range five {
		five[i] = ed25519JWK(string(rune('a' + i)))
		five[i].Kid = string(rune('a' + i))
	}
	rsaKey := syncPushKeyRequest{Kty: "RSA", Crv: "Ed25519", X: "AAAA", Kid: "r"}

	for name, c := range map[string]struct {
		in   *syncPushInput
		code string
	}{
		"another application's pattern": {syncReq("pushsyncalpha", []syncPushRuleRequest{{Pattern: "pushsyncbeta:a:{id}", Requires: []string{"x:y:z"}}}), "PATTERN_NOT_OWNED"},
		"an open rule":                  {syncReq("pushsyncalpha", []syncPushRuleRequest{{Pattern: "pushsyncalpha:a:{id}"}}), "RULE_OPEN"},
		"five keys":                     {syncReq("pushsyncalpha", nil, five...), "TOO_MANY_KEYS"},
		"not an Ed25519 key":            {syncReq("pushsyncalpha", nil, rsaKey), "KEY_INVALID"},
	} {
		_, err := s.syncPush(ctx, c.in)
		require.Error(t, err, name)
		ue := usecase.AsError(err)
		require.NotNil(t, ue, name)
		assert.Equal(t, c.code, ue.Code, name)
	}
	// every refusal left the earlier set standing
	rules, _ := s.Push.Rules(t.Context(), ids["pushsyncalpha"])
	require.Len(t, rules, 1)
	assert.Equal(t, "pushsyncalpha:keep:{id}", rules[0].Pattern)
	kids, _ := s.Push.Kids(t.Context(), ids["pushsyncalpha"])
	assert.Equal(t, []string{"kg"}, kids)

	// an open rule is accepted when declared public
	_, err = s.syncPush(ctx, syncReq("pushsyncalpha", []syncPushRuleRequest{{Pattern: "pushsyncalpha:open:{id}", Public: true}}))
	assert.NoError(t, err)
}

func TestSyncPush_IsAudited(t *testing.T) {
	s, ids := newPushSyncState(t)
	k := ed25519JWK("z")
	k.Kid = "kz"
	_, err := s.syncPush(ctxOf(appSA(ids["pushsyncalpha"])), syncReq("pushsyncalpha", []syncPushRuleRequest{{Pattern: "pushsyncalpha:a:{id}", Requires: []string{"x:y:z"}}}, k))
	require.NoError(t, err)

	var op, principal string
	var detail []byte
	require.NoError(t, s.Push.Pool().QueryRow(t.Context(),
		`SELECT operation, principal_id, operation_json FROM aud_logs WHERE entity_id = $1 ORDER BY performed_at DESC LIMIT 1`, ids["pushsyncalpha"]).
		Scan(&op, &principal, &detail))
	assert.Equal(t, "PUSH_CONFIG_SYNCED", op)
	assert.Equal(t, "prn_svc", principal)
	var d map[string]any
	require.NoError(t, json.Unmarshal(detail, &d))
	assert.Equal(t, []any{"kz"}, d["kids"])
}

func TestPushSettings(t *testing.T) {
	s, ids := newPushSyncState(t)
	anchor := &auth.AuthContext{PrincipalID: "prn_op", Scope: auth.ScopeAnchor, AllApplications: true, Permissions: []string{"platform:admin:push:manage"}}
	i := func(n int) *int { return &n }

	// who may: an anchor holding the permission; not a client-scoped admin with it, not an anchor without it
	for name, ac := range map[string]*auth.AuthContext{
		"a client-scoped caller": {PrincipalID: "p", Scope: auth.ScopeClient, Clients: []string{"c"}, AllApplications: true, Permissions: []string{"platform:admin:push:manage"}},
		"an anchor without it":   {PrincipalID: "p", Scope: auth.ScopeAnchor, AllApplications: true, Permissions: []string{"platform:application-service:push:sync"}},
		"an app service account": appSA(ids["pushsyncalpha"]),
		"nobody":                 nil,
	} {
		_, err := s.putPushSettings(ctxOf(ac), &putPushSettingsInput{AppCode: "pushsyncalpha", Body: PushSettings{Paused: true}})
		require.Error(t, err, name)
		_, err = s.getPushSettings(ctxOf(ac), &pushSettingsInput{AppCode: "pushsyncalpha"})
		require.Error(t, err, name)
	}

	out, err := s.getPushSettings(ctxOf(anchor), &pushSettingsInput{AppCode: "pushsyncalpha"})
	require.NoError(t, err)
	assert.False(t, out.Body.Settings.Paused)
	assert.Equal(t, 500, out.Body.Effective.PublishRate)
	assert.Equal(t, 2000, out.Body.Effective.PublishBurst)
	assert.Equal(t, 16384, out.Body.Effective.MaxDataBytes)
	assert.Equal(t, 20000, out.Body.Effective.MaxStreams)

	out, err = s.putPushSettings(ctxOf(anchor), &putPushSettingsInput{AppCode: "pushsyncalpha", Body: PushSettings{
		MaxStreams: i(50), PublishRate: i(10), PublishBurst: i(40), MaxDataBytes: i(2048), Paused: true,
	}})
	require.NoError(t, err)
	assert.True(t, out.Body.Settings.Paused)
	assert.Equal(t, 10, out.Body.Effective.PublishRate)
	assert.Equal(t, 40, out.Body.Effective.PublishBurst)
	assert.Equal(t, 2048, out.Body.Effective.MaxDataBytes)
	assert.Equal(t, 50, out.Body.Effective.MaxStreams)
	require.NotNil(t, out.Body.UpdatedBy)
	assert.Equal(t, "prn_op", *out.Body.UpdatedBy)

	// a PUT is the whole set of overrides: what it leaves out returns to the default
	out, err = s.putPushSettings(ctxOf(anchor), &putPushSettingsInput{AppCode: "pushsyncalpha", Body: PushSettings{PublishRate: i(20)}})
	require.NoError(t, err)
	assert.False(t, out.Body.Settings.Paused)
	assert.Equal(t, 2000, out.Body.Effective.PublishBurst)
	assert.Equal(t, 20000, out.Body.Effective.MaxStreams)

	// invalid limits and unknown applications
	_, err = s.putPushSettings(ctxOf(anchor), &putPushSettingsInput{AppCode: "pushsyncalpha", Body: PushSettings{MaxDataBytes: i(70000)}})
	require.Error(t, err)
	assert.Equal(t, "SETTINGS_INVALID", usecase.AsError(err).Code)
	_, err = s.putPushSettings(ctxOf(anchor), &putPushSettingsInput{AppCode: "nosuchapp", Body: PushSettings{}})
	require.Error(t, err)
	assert.Equal(t, usecase.KindNotFound, usecase.AsError(err).Kind)

	// the changes are audited
	var n int
	require.NoError(t, s.Push.Pool().QueryRow(t.Context(),
		`SELECT count(*) FROM aud_logs WHERE entity_id = $1 AND operation = 'PUSH_SETTINGS_CHANGED' AND principal_id = 'prn_op'`, ids["pushsyncalpha"]).Scan(&n))
	assert.Equal(t, 2, n)
}
