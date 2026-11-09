package pushgw

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/flowcatalyst/framework/go/push"
)

// Limits of a sync (docs/push-gateway-spec.md section 8).
const (
	// MaxSyncedKeys is how many ticket keys an application may sync: the current one and the
	// previous during a rotation, with room for one more.
	MaxSyncedKeys = 4
	// MaxSyncedRules bounds the rules of one application.
	MaxSyncedRules = 200
)

// Defaults of the per-application limits (spec section 10) when push_app_settings holds no value.
const (
	DefaultPublishRate  = 500
	DefaultPublishBurst = 2000
	DefaultMaxStreams   = push.DefaultAppMaxStreams
	DefaultMaxData      = 16384
	// MaxDataCeiling is the largest max-data-bytes an operator may set (framework section 1).
	MaxDataCeiling = 65536
)

// SyncRule is one rule of an application's sync.
type SyncRule struct {
	Pattern  string
	Requires []string
	Public   bool
}

// SyncKey is one ticket key of an application's sync: the public Ed25519 JWK, with its kid.
type SyncKey struct {
	Kid string
	JWK json.RawMessage
}

// ValidationError is a sync the platform refuses; Code is the machine code of the answer.
type ValidationError struct{ Code, Message string }

func (e *ValidationError) Error() string { return e.Code + ": " + e.Message }

func refuse(code, format string, args ...any) error {
	return &ValidationError{Code: code, Message: fmt.Sprintf(format, args...)}
}

// ValidateSync checks the rules and keys of an application's sync before anything is written: the
// pattern of every rule starts with the application's own code (an application cannot grant
// another's topics), it parses as the module's pattern, an open rule (neither {principal} nor
// requires) is refused unless public, and the keys are at most four public Ed25519 JWKs with a
// distinct kid each.
func ValidateSync(appCode string, rules []SyncRule, keys []SyncKey) error {
	if len(rules) > MaxSyncedRules {
		return refuse("TOO_MANY_RULES", "an application may sync at most %d rules", MaxSyncedRules)
	}
	seen := map[string]bool{}
	for _, r := range rules {
		first, _, _ := strings.Cut(r.Pattern, ":")
		if first != appCode {
			return refuse("PATTERN_NOT_OWNED", "the pattern %q does not start with the application code %q", r.Pattern, appCode)
		}
		if seen[r.Pattern] {
			return refuse("PATTERN_DUPLICATE", "the pattern %q appears more than once", r.Pattern)
		}
		seen[r.Pattern] = true
		if r.Public && len(r.Requires) > 0 {
			return refuse("RULE_INVALID", "the public rule %q cannot also require permissions", r.Pattern)
		}
		var err error
		if r.Public {
			_, err = push.NewPublicRule(r.Pattern)
		} else {
			opts := make([]push.RuleOption, len(r.Requires))
			for i, p := range r.Requires {
				opts[i] = push.Requires(p)
			}
			_, err = push.NewRule(r.Pattern, opts...)
		}
		switch {
		case errors.Is(err, push.ErrOpenRule):
			return refuse("RULE_OPEN", "the rule %q has neither {principal} nor requires; declare a deliberately open topic with public", r.Pattern)
		case err != nil:
			return refuse("RULE_INVALID", "the rule %q: %v", r.Pattern, err)
		}
	}
	if len(keys) > MaxSyncedKeys {
		return refuse("TOO_MANY_KEYS", "an application may sync at most %d ticket keys", MaxSyncedKeys)
	}
	kids := map[string]bool{}
	for _, k := range keys {
		if k.Kid == "" || len(k.Kid) > 64 {
			return refuse("KEY_INVALID", "a ticket key needs a kid of 1 to 64 characters")
		}
		if kids[k.Kid] {
			return refuse("KEY_DUPLICATE", "the kid %q appears more than once", k.Kid)
		}
		kids[k.Kid] = true
		if _, err := parseEd25519JWK(k.JWK); err != nil {
			return refuse("KEY_INVALID", "the key %q is not a public Ed25519 JWK: %v", k.Kid, err)
		}
	}
	return nil
}

// Counts is what a replace did to one table.
type Counts struct{ Created, Updated, Deleted int }

// SyncResult is what ReplaceRulesAndKeys did.
type SyncResult struct{ Rules, Keys Counts }

// ConfigRepo reads and writes the gateway's per-application configuration tables (push_rules,
// push_ticket_keys, push_app_settings) and tells every gateway task to reload.
type ConfigRepo struct{ pool *pgxpool.Pool }

// Pool returns the platform pool the repository writes through (tests read the tables with it).
func (r *ConfigRepo) Pool() *pgxpool.Pool { return r.pool }

// NewConfigRepo returns a ConfigRepo on the platform pool.
func NewConfigRepo(pool *pgxpool.Pool) *ConfigRepo { return &ConfigRepo{pool: pool} }

// ReplaceRulesAndKeys makes the application's stored rules and ticket keys equal the given sets
// (a declarative full replace), in one transaction that also sends the NOTIFY the gateway tasks
// reload on: it is delivered at commit, so no task reloads half a sync. The input must have passed
// [ValidateSync].
func (r *ConfigRepo) ReplaceRulesAndKeys(ctx context.Context, applicationID string, rules []SyncRule, keys []SyncKey) (SyncResult, error) {
	var res SyncResult
	err := pgx.BeginFunc(ctx, r.pool, func(tx pgx.Tx) error {
		var err error
		if res.Rules, err = replaceRules(ctx, tx, applicationID, rules); err != nil {
			return err
		}
		if res.Keys, err = replaceKeys(ctx, tx, applicationID, keys); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `SELECT pg_notify($1, '')`, ChannelConfig)
		return err
	})
	return res, err
}

func replaceRules(ctx context.Context, tx pgx.Tx, applicationID string, rules []SyncRule) (Counts, error) {
	var c Counts
	existing, err := stringSet(ctx, tx, `SELECT pattern FROM push_rules WHERE application_id = $1`, applicationID)
	if err != nil {
		return c, fmt.Errorf("read push_rules: %w", err)
	}
	listed := make([]string, 0, len(rules))
	for _, rule := range rules {
		listed = append(listed, rule.Pattern)
		requires := rule.Requires
		if requires == nil {
			requires = []string{}
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO push_rules (application_id, pattern, requires, public, synced_at)
			VALUES ($1, $2, $3, $4, NOW())
			ON CONFLICT (application_id, pattern) DO UPDATE
			   SET requires = EXCLUDED.requires, public = EXCLUDED.public, synced_at = EXCLUDED.synced_at`,
			applicationID, rule.Pattern, requires, rule.Public); err != nil {
			return c, fmt.Errorf("write push_rules: %w", err)
		}
		if existing[rule.Pattern] {
			c.Updated++
		} else {
			c.Created++
		}
	}
	tag, err := tx.Exec(ctx, `DELETE FROM push_rules WHERE application_id = $1 AND NOT (pattern = ANY($2))`, applicationID, listed)
	if err != nil {
		return c, fmt.Errorf("prune push_rules: %w", err)
	}
	c.Deleted = int(tag.RowsAffected())
	return c, nil
}

func replaceKeys(ctx context.Context, tx pgx.Tx, applicationID string, keys []SyncKey) (Counts, error) {
	var c Counts
	existing, err := stringSet(ctx, tx, `SELECT kid FROM push_ticket_keys WHERE application_id = $1`, applicationID)
	if err != nil {
		return c, fmt.Errorf("read push_ticket_keys: %w", err)
	}
	listed := make([]string, 0, len(keys))
	for _, k := range keys {
		listed = append(listed, k.Kid)
		if _, err := tx.Exec(ctx, `
			INSERT INTO push_ticket_keys (application_id, kid, jwk, synced_at)
			VALUES ($1, $2, $3, NOW())
			ON CONFLICT (application_id, kid) DO UPDATE SET jwk = EXCLUDED.jwk, synced_at = EXCLUDED.synced_at`,
			applicationID, k.Kid, []byte(k.JWK)); err != nil {
			return c, fmt.Errorf("write push_ticket_keys: %w", err)
		}
		if existing[k.Kid] {
			c.Updated++
		} else {
			c.Created++
		}
	}
	tag, err := tx.Exec(ctx, `DELETE FROM push_ticket_keys WHERE application_id = $1 AND NOT (kid = ANY($2))`, applicationID, listed)
	if err != nil {
		return c, fmt.Errorf("prune push_ticket_keys: %w", err)
	}
	c.Deleted = int(tag.RowsAffected())
	return c, nil
}

func stringSet(ctx context.Context, tx pgx.Tx, sql string, args ...any) (map[string]bool, error) {
	rows, err := tx.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, err
		}
		out[s] = true
	}
	return out, rows.Err()
}

// SyncedRule is a rule as stored, for the console and tests.
type SyncedRule struct {
	Pattern  string
	Requires []string
	Public   bool
}

// Rules returns the application's stored rules, ordered by pattern.
func (r *ConfigRepo) Rules(ctx context.Context, applicationID string) ([]SyncedRule, error) {
	rows, err := r.pool.Query(ctx, `SELECT pattern, requires, public FROM push_rules WHERE application_id = $1 ORDER BY pattern`, applicationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SyncedRule
	for rows.Next() {
		var s SyncedRule
		if err := rows.Scan(&s.Pattern, &s.Requires, &s.Public); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// Kids returns the kids of the application's stored ticket keys, sorted.
func (r *ConfigRepo) Kids(ctx context.Context, applicationID string) ([]string, error) {
	rows, err := r.pool.Query(ctx, `SELECT kid FROM push_ticket_keys WHERE application_id = $1 ORDER BY kid`, applicationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	slices.Sort(out)
	return out, rows.Err()
}

// AppSettings are an application's operator overrides (push_app_settings). A nil limit takes the
// gateway default of spec section 10.
type AppSettings struct {
	MaxStreams   *int
	PublishRate  *int
	PublishBurst *int
	MaxDataBytes *int
	Paused       bool
	UpdatedAt    *time.Time
	UpdatedBy    *string
}

// Settings returns the application's overrides (the zero value when none are stored).
func (r *ConfigRepo) Settings(ctx context.Context, applicationID string) (AppSettings, error) {
	var s AppSettings
	err := r.pool.QueryRow(ctx, `
		SELECT max_streams, publish_rate, publish_burst, max_data_bytes, paused, updated_at, updated_by
		  FROM push_app_settings WHERE application_id = $1`, applicationID).
		Scan(&s.MaxStreams, &s.PublishRate, &s.PublishBurst, &s.MaxDataBytes, &s.Paused, &s.UpdatedAt, &s.UpdatedBy)
	if errors.Is(err, pgx.ErrNoRows) {
		return AppSettings{}, nil
	}
	return s, err
}

// ValidateSettings checks an operator's limits: positive, the data limit at most [MaxDataCeiling],
// and a burst of at least the rate.
func ValidateSettings(s AppSettings) error {
	for name, v := range map[string]*int{
		"maxStreams": s.MaxStreams, "publishRate": s.PublishRate, "publishBurst": s.PublishBurst, "maxDataBytes": s.MaxDataBytes,
	} {
		if v != nil && *v < 1 {
			return refuse("SETTINGS_INVALID", "%s must be at least 1 (omit it for the default)", name)
		}
	}
	if s.MaxDataBytes != nil && *s.MaxDataBytes > MaxDataCeiling {
		return refuse("SETTINGS_INVALID", "maxDataBytes is at most %d", MaxDataCeiling)
	}
	if s.PublishBurst != nil {
		rate := DefaultPublishRate
		if s.PublishRate != nil {
			rate = *s.PublishRate
		}
		if *s.PublishBurst < rate {
			return refuse("SETTINGS_INVALID", "publishBurst (%d) must be at least publishRate (%d)", *s.PublishBurst, rate)
		}
	}
	return nil
}

// SetSettings replaces the application's overrides and tells every gateway task to reload. by is
// the operator's principal id.
func (r *ConfigRepo) SetSettings(ctx context.Context, applicationID string, s AppSettings, by string) error {
	return pgx.BeginFunc(ctx, r.pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `
			INSERT INTO push_app_settings
			       (application_id, max_streams, publish_rate, publish_burst, max_data_bytes, paused, updated_at, updated_by)
			VALUES ($1, $2, $3, $4, $5, $6, NOW(), $7)
			ON CONFLICT (application_id) DO UPDATE SET
			       max_streams = EXCLUDED.max_streams, publish_rate = EXCLUDED.publish_rate,
			       publish_burst = EXCLUDED.publish_burst, max_data_bytes = EXCLUDED.max_data_bytes,
			       paused = EXCLUDED.paused, updated_at = EXCLUDED.updated_at, updated_by = EXCLUDED.updated_by`,
			applicationID, s.MaxStreams, s.PublishRate, s.PublishBurst, s.MaxDataBytes, s.Paused, by); err != nil {
			return fmt.Errorf("write push_app_settings: %w", err)
		}
		_, err := tx.Exec(ctx, `SELECT pg_notify($1, '')`, ChannelConfig)
		return err
	})
}
