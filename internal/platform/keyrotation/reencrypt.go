// Package keyrotation moves every value stored encrypted under
// FLOWCATALYST_APP_KEY to the current key, so that after a rotation the old
// key (kept in FLOWCATALYST_APP_KEY_PREVIOUS meanwhile) can be dropped. It is
// the engine behind `fc-server reencrypt`; docs/key-rotation.md is the
// operator procedure.
//
// What it covers — every column the platform writes through
// encryption.Service.Encrypt / EncryptBound / EncryptSecretRef (Targets):
//
//	app_platform_configs.value           SECRET config values, bound to their row (EncryptBound)
//	iam_service_accounts.wh_auth_token_ref, wh_signing_secret_ref   webhook credentials
//	oauth_identity_providers.oidc_client_secret_ref                 IdP client secrets
//	tnt_client_auth_configs.oidc_client_secret_ref                  client auth-config OIDC secrets
//	iam_user_mfa_methods.secret_encrypted                           TOTP shared secrets
//	fng_settings.value (SECRET, DB)                                 function secrets / DSNs
//	oauth_clients.client_secret_ref, previous_secret_ref            legacy (pre-hash) encrypted client secrets
//	iam_principals.dev_client_secret_ref                            legacy encrypted developer secrets
//
// What it cannot cover, and why that is fine:
//
//   - "hashed:v1:" verify-only secrets (OAuth clients, service accounts,
//     developer credentials) are an HMAC of a plaintext the platform never
//     holds; VerifySecret re-keys each on its next successful use. Counted as
//     "hashed" so the operator knows they exist.
//   - External references (aws-sm://, vault://, literal:, …) are not
//     encrypted with the app key at all.
//   - Field-worker PIN hashes (argon2id over a peppered HMAC) re-hash on the
//     worker's next sign-in; see fieldidentity/pin.
//   - Dispatch-job tokens are short-lived and verified under every configured
//     key; nothing is stored.
//
// Safe while the platform serves traffic: rows are read in primary-key order
// in batches, and each one is rewritten on its own, autocommitted, by a
// compare-and-set on the exact ciphertext that was read, so a value written
// concurrently (by a process already on the new key) is never overwritten —
// it is counted as "changed meanwhile" and left alone. Idempotent and
// resumable: a value the current key already opens in its current form is
// skipped, so an interrupted run is simply run again.
package keyrotation

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/flowcatalyst/flowcatalyst-go/internal/ids"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/platformconfig"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/encryption"
)

// encryptedPrefix is the marker EncryptSecretRef writes in front of an inline
// ciphertext. A rewrite keeps a value's marker exactly as it found it.
const encryptedPrefix = "encrypted:"

// Prefix says how a column marks its ciphertext.
type Prefix int

const (
	// PrefixRequired: only "encrypted:"-prefixed values are ciphertext; any
	// other value is a plaintext the column predates encryption with (it is
	// encrypted lazily elsewhere) or an external reference. Counted as not
	// encrypted, never touched.
	PrefixRequired Prefix = iota
	// PrefixOptional: ciphertext may also be stored bare (an older writer).
	// A bare value that no key opens is counted unreadable.
	PrefixOptional
)

// Target is one encrypted column.
type Target struct {
	Table  string
	Column string
	// Keys is the primary key, in the order rows are walked.
	Keys []string
	// Where is an extra static filter (no user input), "" for none.
	Where  string
	Prefix Prefix
	// AADColumns are read with each row and passed to AAD; set for values
	// sealed with EncryptBound.
	AADColumns []string
	AAD        func(vals []any) ([]byte, error)
}

// Name is "table.column".
func (t Target) Name() string { return t.Table + "." + t.Column }

// Targets lists every column the platform stores encrypted with the app key.
// Adding a new encrypted column? Add it here (and to the package comment).
func Targets() []Target {
	id := []string{"id"}
	return []Target{
		{
			Table: "app_platform_configs", Column: "value", Keys: id,
			Where: "value_type = 'SECRET'", Prefix: PrefixRequired,
			AADColumns: []string{"application_code", "section", "property", "client_id"},
			AAD: func(v []any) ([]byte, error) {
				app, ok1 := v[0].(string)
				section, ok2 := v[1].(string)
				property, ok3 := v[2].(string)
				if !ok1 || !ok2 || !ok3 {
					return nil, fmt.Errorf("unexpected coordinate types %T %T %T", v[0], v[1], v[2])
				}
				var client *ids.ClientID
				if c, ok := v[3].(string); ok {
					cid := ids.ClientID(c)
					client = &cid
				}
				return platformconfig.SecretAAD(app, section, property, client), nil
			},
		},
		{Table: "iam_service_accounts", Column: "wh_auth_token_ref", Keys: id, Prefix: PrefixRequired},
		{Table: "iam_service_accounts", Column: "wh_signing_secret_ref", Keys: id, Prefix: PrefixRequired},
		{Table: "oauth_identity_providers", Column: "oidc_client_secret_ref", Keys: id, Prefix: PrefixOptional},
		{Table: "tnt_client_auth_configs", Column: "oidc_client_secret_ref", Keys: id, Prefix: PrefixOptional},
		{Table: "iam_user_mfa_methods", Column: "secret_encrypted", Keys: id, Prefix: PrefixOptional},
		{
			Table: "fng_settings", Column: "value", Keys: []string{"function_id", "kind", "key"},
			Where: "kind IN ('SECRET', 'DB')", Prefix: PrefixRequired,
		},
		{Table: "oauth_clients", Column: "client_secret_ref", Keys: id, Prefix: PrefixOptional},
		{Table: "oauth_clients", Column: "previous_secret_ref", Keys: id, Prefix: PrefixOptional},
		{Table: "iam_principals", Column: "dev_client_secret_ref", Keys: id, Prefix: PrefixOptional},
	}
}

// Options control a run.
type Options struct {
	// DryRun counts what would be re-encrypted without writing anything.
	DryRun bool
	// BatchSize is the number of rows read per query (default 500).
	BatchSize int
}

// Result is one target's counts. Every non-empty value lands in exactly one
// of Current, Reencrypted, Raced, Unreadable, Hashed, External, NotEncrypted.
type Result struct {
	Target string
	// Scanned is the number of non-empty values read.
	Scanned int
	// Current values are already encrypted under the current key in the
	// current form.
	Current int
	// Reencrypted values were moved to the current key (with DryRun: would be).
	Reencrypted int
	// Raced values changed between the read and the compare-and-set and were
	// left as the concurrent writer stored them.
	Raced int
	// Unreadable values look encrypted but no configured key opens them.
	Unreadable int
	// Hashed values are verify-only "hashed:" secrets, re-keyed on next use.
	Hashed int
	// External values are secret-manager references, not app-key ciphertext.
	External int
	// NotEncrypted values are plaintext in a column that encrypts lazily.
	NotEncrypted int
}

// Run re-encrypts every target. It stops at the first database error (a
// rerun resumes, skipping what is already current); a value that cannot be
// decrypted is counted, logged by row key (never by value), and skipped.
func Run(ctx context.Context, pool *pgxpool.Pool, enc *encryption.Service, opts Options) ([]Result, error) {
	return RunTargets(ctx, pool, enc, Targets(), opts)
}

// RunTargets is Run over the given targets.
func RunTargets(ctx context.Context, pool *pgxpool.Pool, enc *encryption.Service, targets []Target, opts Options) ([]Result, error) {
	if enc == nil {
		return nil, encryption.ErrNotConfigured
	}
	if opts.BatchSize <= 0 {
		opts.BatchSize = 500
	}
	out := make([]Result, 0, len(targets))
	for _, t := range targets {
		r, err := runTarget(ctx, pool, enc, t, opts)
		out = append(out, r)
		if err != nil {
			return out, fmt.Errorf("%s: %w", t.Name(), err)
		}
	}
	return out, nil
}

type row struct {
	keys  []any
	value string
	aad   []any
}

func runTarget(ctx context.Context, pool *pgxpool.Pool, enc *encryption.Service, t Target, opts Options) (Result, error) {
	res := Result{Target: t.Name()}
	var cursor []any
	for {
		rows, err := readBatch(ctx, pool, t, cursor, opts.BatchSize)
		if err != nil {
			return res, err
		}
		for _, r := range rows {
			if err := handle(ctx, pool, enc, t, r, opts, &res); err != nil {
				return res, err
			}
		}
		if len(rows) < opts.BatchSize {
			return res, nil
		}
		cursor = rows[len(rows)-1].keys
	}
}

// readBatch reads the next batch of non-empty values after cursor, in
// primary-key order.
func readBatch(ctx context.Context, pool *pgxpool.Pool, t Target, cursor []any, limit int) ([]row, error) {
	cols := append(append(append([]string{}, t.Keys...), t.Column), t.AADColumns...)
	where := []string{t.Column + " IS NOT NULL", t.Column + " <> ''"}
	if t.Where != "" {
		where = append(where, "("+t.Where+")")
	}
	args := []any{}
	if cursor != nil {
		ph := make([]string, len(cursor))
		for i, v := range cursor {
			args = append(args, v)
			ph[i] = fmt.Sprintf("$%d", i+1)
		}
		where = append(where, "("+strings.Join(t.Keys, ", ")+") > ("+strings.Join(ph, ", ")+")")
	}
	args = append(args, limit)
	// #nosec G202 -- table, column and filter names come from Targets(), never from input.
	q := "SELECT " + strings.Join(cols, ", ") + " FROM " + t.Table +
		" WHERE " + strings.Join(where, " AND ") +
		" ORDER BY " + strings.Join(t.Keys, ", ") +
		fmt.Sprintf(" LIMIT $%d", len(args))
	rs, err := pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rs, func(r pgx.CollectableRow) (row, error) {
		vals, err := r.Values()
		if err != nil {
			return row{}, err
		}
		nk := len(t.Keys)
		v, ok := vals[nk].(string)
		if !ok {
			return row{}, fmt.Errorf("%s is %T, not text", t.Column, vals[nk])
		}
		return row{keys: vals[:nk], value: v, aad: vals[nk+1:]}, nil
	})
}

func handle(ctx context.Context, pool *pgxpool.Pool, enc *encryption.Service, t Target, r row, opts Options, res *Result) error {
	res.Scanned++
	v := r.value
	switch {
	case encryption.IsHashed(v):
		res.Hashed++
		return nil
	case encryption.IsExternalReference(v):
		res.External++
		return nil
	case t.Prefix == PrefixRequired && !strings.HasPrefix(v, encryptedPrefix):
		res.NotEncrypted++
		return nil
	}

	var aad []byte
	if t.AAD != nil {
		var err error
		if aad, err = t.AAD(r.aad); err != nil {
			return fmt.Errorf("row %v: %w", r.keys, err)
		}
	}
	var (
		pt      string
		current bool
		err     error
	)
	if aad != nil {
		pt, current, err = enc.DecryptBoundCurrent(v, aad)
	} else {
		pt, current, err = enc.DecryptCurrent(v)
	}
	if err != nil {
		res.Unreadable++
		slog.WarnContext(ctx, "reencrypt: value no configured key opens; left as it is",
			"target", t.Name(), "row", fmt.Sprint(r.keys))
		return nil
	}
	if current {
		res.Current++
		return nil
	}

	var sealed string
	if aad != nil {
		sealed, err = enc.EncryptBound(pt, aad)
	} else {
		sealed, err = enc.Encrypt(pt)
	}
	if err != nil {
		return err
	}
	if strings.HasPrefix(v, encryptedPrefix) {
		sealed = encryptedPrefix + sealed
	}
	if opts.DryRun {
		res.Reencrypted++
		return nil
	}
	changed, err := compareAndSet(ctx, pool, t, r.keys, v, sealed)
	if err != nil {
		return err
	}
	if changed {
		res.Reencrypted++
	} else {
		res.Raced++
	}
	return nil
}

// compareAndSet replaces old with sealed on one row, only while the column
// still holds old. Autocommitted on its own: a long run holds no locks.
func compareAndSet(ctx context.Context, pool *pgxpool.Pool, t Target, keys []any, old, sealed string) (bool, error) {
	conds := make([]string, len(t.Keys))
	args := []any{sealed, old}
	for i, k := range t.Keys {
		args = append(args, keys[i])
		conds[i] = fmt.Sprintf("%s = $%d", k, len(args))
	}
	// #nosec G202 -- table and column names come from Targets(), never from input.
	q := "UPDATE " + t.Table + " SET " + t.Column + " = $1 WHERE " + t.Column + " = $2 AND " + strings.Join(conds, " AND ")
	tag, err := pool.Exec(ctx, q, args...)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() == 1, nil
}
