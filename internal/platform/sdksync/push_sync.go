package sdksync

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/audit"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/pushgw"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/tsid"
	"github.com/flowcatalyst/go-sdk/usecase"
)

// Push gateway sync: POST /api/applications/{appCode}/push/sync
// (docs/push-gateway-spec.md section 8). An application replaces its topic rules and its ticket
// keys in one call: the app's repo is the source of truth, the payload IS the set (there is no
// removeUnlisted flag). The gateway tasks reload within a second of the commit (NOTIFY
// fw_push_config), and a 60 second poll is the fallback.

type syncPushRuleRequest struct {
	Pattern  string   `json:"pattern" doc:"{application}:{segment}(:{segment})*, the first segment the application's own code; a segment may be a {placeholder}"`
	Requires []string `json:"requires,omitempty" doc:"Permissions the subscriber must all hold"`
	Public   bool     `json:"public,omitempty" doc:"A deliberately open topic: anyone may subscribe, with or without credentials. Required for a rule with neither {principal} nor requires."`
}

type syncPushKeyRequest struct {
	Kty string `json:"kty" doc:"OKP"`
	Crv string `json:"crv" doc:"Ed25519"`
	X   string `json:"x" doc:"The public key, base64url"`
	Kid string `json:"kid" doc:"The key id tickets name in their header"`
	Use string `json:"use,omitempty"`
	Alg string `json:"alg,omitempty"`
}

type syncPushKeysRequest struct {
	Keys []syncPushKeyRequest `json:"keys" doc:"At most 4 public Ed25519 keys: the current and the previous during a rotation"`
}

type syncPushRequest struct {
	Rules []syncPushRuleRequest `json:"rules"`
	Keys  syncPushKeysRequest   `json:"keys"`
}

type syncPushInput struct {
	AppCode string `path:"appCode" doc:"Application code"`
	Body    syncPushRequest
}

// PushSyncCounts is what a push sync did to rules or keys.
type PushSyncCounts struct {
	Created uint32 `json:"created"`
	Updated uint32 `json:"updated"`
	Deleted uint32 `json:"deleted"`
}

// PushSyncResponse is the result of a push sync.
type PushSyncResponse struct {
	ApplicationCode string         `json:"applicationCode"`
	Rules           PushSyncCounts `json:"rules"`
	Keys            PushSyncCounts `json:"keys"`
	SyncedPatterns  []string       `json:"syncedPatterns"`
	SyncedKids      []string       `json:"syncedKids"`
}

type syncPushOutput struct{ Body PushSyncResponse }

func (s *State) syncPush(ctx context.Context, in *syncPushInput) (*syncPushOutput, error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanSyncPush(ac); err != nil {
		return nil, err
	}
	if s.Push == nil {
		return nil, usecase.Internal("PUSH", "push sync is not wired", nil)
	}
	app, err := s.resolveApp(ctx, in.AppCode)
	if err != nil {
		return nil, err
	}
	if err := s.requireAppAccess(ac, app); err != nil {
		return nil, err
	}

	rules := make([]pushgw.SyncRule, 0, len(in.Body.Rules))
	for _, r := range in.Body.Rules {
		rules = append(rules, pushgw.SyncRule{Pattern: r.Pattern, Requires: r.Requires, Public: r.Public})
	}
	keys := make([]pushgw.SyncKey, 0, len(in.Body.Keys.Keys))
	for _, k := range in.Body.Keys.Keys {
		// the stored key is the public JWK as the application sent it, without private members
		jwk, _ := json.Marshal(map[string]string{"kty": k.Kty, "crv": k.Crv, "x": k.X, "kid": k.Kid})
		keys = append(keys, pushgw.SyncKey{Kid: k.Kid, JWK: jwk})
	}
	if err := pushgw.ValidateSync(app.Code, rules, keys); err != nil {
		if ve, ok := errors.AsType[*pushgw.ValidationError](err); ok {
			return nil, usecase.Validation(ve.Code, ve.Message)
		}
		return nil, err
	}

	res, err := s.Push.ReplaceRulesAndKeys(ctx, string(app.ID), rules, keys)
	if err != nil {
		return nil, usecase.Internal("PUSH", "push sync failed", err)
	}
	out := PushSyncResponse{
		ApplicationCode: app.Code,
		Rules:           counts(res.Rules),
		Keys:            counts(res.Keys),
		SyncedPatterns:  make([]string, 0, len(rules)),
		SyncedKids:      make([]string, 0, len(keys)),
	}
	for _, r := range rules {
		out.SyncedPatterns = append(out.SyncedPatterns, r.Pattern)
	}
	for _, k := range keys {
		out.SyncedKids = append(out.SyncedKids, k.Kid)
	}
	s.auditPush(ctx, ac, string(app.ID), "PUSH_CONFIG_SYNCED", map[string]any{
		"rules": out.Rules, "keys": out.Keys, "kids": out.SyncedKids,
	})
	return &syncPushOutput{Body: out}, nil
}

func counts(c pushgw.Counts) PushSyncCounts {
	return PushSyncCounts{Created: uint32(c.Created), Updated: uint32(c.Updated), Deleted: uint32(c.Deleted)} //nolint:gosec // counts of a bounded sync
}

// auditPush records a push configuration change in the audit trail (best effort: a sync must not
// fail for it, as for the other direct audit writes).
func (s *State) auditPush(ctx context.Context, ac *auth.AuthContext, appID, operation string, detail map[string]any) {
	if s.Audit == nil {
		return
	}
	actor := ac.PrincipalID
	id := appID
	raw, _ := json.Marshal(detail)
	if err := s.Audit.Insert(ctx, &audit.Log{
		ID: tsid.Generate(tsid.AuditLog), EntityType: "APPLICATION", EntityID: id, Operation: operation,
		OperationJSON: raw, PrincipalID: &actor, ApplicationID: &id, PerformedAt: time.Now().UTC(),
	}); err != nil {
		slog.Warn("push gateway: audit write failed", "operation", operation, "err", err)
	}
}
