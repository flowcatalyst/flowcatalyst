package pushgw

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/flowcatalyst/framework/go/push"
)

// configPoll is how often the caches reload when no NOTIFY arrives (spec 8: "a 60-second poll is
// the fallback").
const configPoll = 60 * time.Second

// snapshot is everything the module reads for every subscribe and listen, by application code.
type snapshot struct {
	rules  map[string][]push.Rule
	keys   map[string]map[string]ed25519.PublicKey // application, kid
	limits map[string]push.AppLimits
}

// Store holds the rules, ticket keys and limits of the applications, read from the platform
// database (push_rules, push_ticket_keys, push_app_settings) and cached. It implements the
// module's providers (push.RuleProvider, push.TicketKeyResolver, push.AppLimitProvider); each
// answer is a read of an immutable snapshot, which Reload replaces.
type Store struct {
	pool *pgxpool.Pool
	log  *slog.Logger
	snap atomic.Pointer[snapshot]
}

// NewStore returns a Store with an empty snapshot (every application unknown); call Reload.
func NewStore(pool *pgxpool.Pool, log *slog.Logger) *Store {
	s := &Store{pool: pool, log: log}
	s.snap.Store(&snapshot{})
	return s
}

var (
	_ push.RuleProvider      = (*Store)(nil)
	_ push.TicketKeyResolver = (*Store)(nil)
	_ push.AppLimitProvider  = (*Store)(nil)
)

// Rules implements push.RuleProvider.
func (s *Store) Rules(application string) []push.Rule { return s.snap.Load().rules[application] }

// PublicKey implements push.TicketKeyResolver.
func (s *Store) PublicKey(issuer, kid string) (ed25519.PublicKey, bool) {
	k, ok := s.snap.Load().keys[issuer][kid]
	return k, ok
}

// AppLimits implements push.AppLimitProvider.
func (s *Store) AppLimits(application string) push.AppLimits {
	return s.snap.Load().limits[application]
}

// Reload reads the tables and replaces the snapshot. A row the module would refuse (a bad
// pattern, an open rule, a key that is not an Ed25519 JWK) is left out and logged, never fatal: one
// application's bad sync must not stop the others'. Only active applications are served.
func (s *Store) Reload(ctx context.Context) error {
	next := &snapshot{
		rules:  map[string][]push.Rule{},
		keys:   map[string]map[string]ed25519.PublicKey{},
		limits: map[string]push.AppLimits{},
	}

	rows, err := s.pool.Query(ctx, `
		SELECT a.code, r.pattern, r.requires, r.public
		  FROM push_rules r JOIN app_applications a ON a.id = r.application_id
		 WHERE a.active
		 ORDER BY a.code, r.pattern`)
	if err != nil {
		return fmt.Errorf("read push_rules: %w", err)
	}
	for rows.Next() {
		var app, pattern string
		var requires []string
		var public bool
		if err := rows.Scan(&app, &pattern, &requires, &public); err != nil {
			rows.Close()
			return fmt.Errorf("scan push_rules: %w", err)
		}
		var rule push.Rule
		var rerr error
		if public {
			rule, rerr = push.NewPublicRule(pattern)
		} else {
			opts := make([]push.RuleOption, len(requires))
			for i, p := range requires {
				opts[i] = push.Requires(p)
			}
			rule, rerr = push.NewRule(pattern, opts...)
		}
		if rerr != nil {
			s.log.Warn("push gateway: a synced rule is refused and left out", "application", app, "pattern", pattern, "err", rerr)
			continue
		}
		next.rules[app] = append(next.rules[app], rule)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return fmt.Errorf("read push_rules: %w", err)
	}

	rows, err = s.pool.Query(ctx, `
		SELECT a.code, k.kid, k.jwk
		  FROM push_ticket_keys k JOIN app_applications a ON a.id = k.application_id
		 WHERE a.active`)
	if err != nil {
		return fmt.Errorf("read push_ticket_keys: %w", err)
	}
	for rows.Next() {
		var app, kid string
		var raw []byte
		if err := rows.Scan(&app, &kid, &raw); err != nil {
			rows.Close()
			return fmt.Errorf("scan push_ticket_keys: %w", err)
		}
		pub, kerr := parseEd25519JWK(raw)
		if kerr != nil {
			s.log.Warn("push gateway: a synced ticket key is refused and left out", "application", app, "kid", kid, "err", kerr)
			continue
		}
		if next.keys[app] == nil {
			next.keys[app] = map[string]ed25519.PublicKey{}
		}
		next.keys[app][kid] = pub
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return fmt.Errorf("read push_ticket_keys: %w", err)
	}

	rows, err = s.pool.Query(ctx, `
		SELECT a.code, coalesce(s.max_streams, 0), coalesce(s.max_data_bytes, 0)
		  FROM push_app_settings s JOIN app_applications a ON a.id = s.application_id`)
	if err != nil {
		return fmt.Errorf("read push_app_settings: %w", err)
	}
	for rows.Next() {
		var app string
		var l push.AppLimits
		if err := rows.Scan(&app, &l.MaxStreams, &l.MaxDataBytes); err != nil {
			rows.Close()
			return fmt.Errorf("scan push_app_settings: %w", err)
		}
		next.limits[app] = l
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return fmt.Errorf("read push_app_settings: %w", err)
	}

	s.snap.Store(next)
	return nil
}

// run keeps the snapshot current until ctx ends: a reload at once when ChannelConfig is
// notified, and every configPoll as the fallback.
func (s *Store) run(ctx context.Context) {
	wake := make(chan struct{}, 1)
	poke := func() {
		select {
		case wake <- struct{}{}:
		default:
		}
	}
	go listenLoop(ctx, s.log, "config", s.pool, []string{ChannelConfig},
		func(string, string) { poke() },
		poke) // after (re)connecting, catch up on what was missed
	t := time.NewTicker(configPoll)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-wake:
		case <-t.C:
		}
		if err := s.Reload(ctx); err != nil && ctx.Err() == nil {
			s.log.Warn("push gateway: reloading rules and keys failed; the previous snapshot stays", "err", err)
		}
	}
}

// parseEd25519JWK reads a public Ed25519 key: {"kty":"OKP","crv":"Ed25519","x":"<base64url>"}.
func parseEd25519JWK(raw []byte) (ed25519.PublicKey, error) {
	var j struct {
		Kty, Crv, X string
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("not a JSON object: %w", err)
	}
	for _, k := range []string{"kty", "crv", "x", "d"} {
		v, ok := m[k]
		if !ok {
			continue
		}
		var s string
		if err := json.Unmarshal(v, &s); err != nil {
			return nil, fmt.Errorf("member %s is not a string", k)
		}
		switch k {
		case "kty":
			j.Kty = s
		case "crv":
			j.Crv = s
		case "x":
			j.X = s
		case "d":
			return nil, fmt.Errorf("the JWK carries private key material")
		}
	}
	if j.Kty != "OKP" || j.Crv != "Ed25519" {
		return nil, fmt.Errorf("not an Ed25519 OKP key")
	}
	x, err := base64.RawURLEncoding.DecodeString(j.X)
	if err != nil || len(x) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("x is not the base64url of a 32-byte public key")
	}
	return ed25519.PublicKey(x), nil
}
