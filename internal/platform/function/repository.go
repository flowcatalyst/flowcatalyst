package function

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/encryption"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/repocommon"
	"github.com/flowcatalyst/flowcatalyst-go/internal/sqlc/dbq"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

// Repository is the Postgres-backed repository for the function aggregate
// and its child records. Table: fn_functions (+ fn_versions, fn_aliases,
// fn_settings, fn_runners, fn_pool_revisions).
type Repository struct {
	pool *pgxpool.Pool // retained for FindWithFilters / CountWithFilters
	q    *dbq.Queries
	// enc encrypts SECRET/DB setting values at rest, same convention as
	// serviceaccount.Repository's webhook credentials. Resolved once at
	// construction from FLOWCATALYST_APP_KEY; nil when unconfigured, which
	// makes a write of a plaintext SECRET/DB value fail rather than store
	// plaintext (see encryption.EncryptSecretRef).
	enc *encryption.Service
}

// NewRepository wires a repo.
func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool, q: dbq.New(pool), enc: encryption.MustFromEnv()}
}

// ── Function ────────────────────────────────────────────────────────────

// FindByID loads a function by id, with ApplicationCode populated read-side
// from a join against app_applications (LEFT JOIN: a dangling
// application_id, though not expected in practice, must not turn a function
// read into an error — ApplicationCode is display-only, never persisted).
func (r *Repository) FindByID(ctx context.Context, id string) (*Function, error) {
	res, err := r.q.FunctionFindByID(ctx, id)
	row, err := repocommon.One(res, err, "function repo")
	if row == nil || err != nil {
		return nil, err
	}
	return rowToFunction(row.FnFunction, row.ApplicationCode)
}

// FindByAddress loads a function by its unique address.
func (r *Repository) FindByAddress(ctx context.Context, address string) (*Function, error) {
	res, err := r.q.FunctionFindByAddress(ctx, address)
	row, err := repocommon.One(res, err, "function repo")
	if row == nil || err != nil {
		return nil, err
	}
	return rowToFunction(row.FnFunction, row.ApplicationCode)
}

// ListFilters drives FindWithFilters / CountWithFilters. Non-nil fields are
// ANDed together.
type ListFilters struct {
	ApplicationID *string
	ClientID      *string
	AddressPrefix *string
	// AccessibleClientIDs, when non-nil, restricts results to platform-scoped
	// rows (client_id IS NULL) plus rows whose client_id is in the set — the
	// same visibility rule as auth.FilterClientScoped, applied in SQL so
	// COUNT and LIMIT/OFFSET stay consistent across pages for a non-anchor
	// caller (see scheduledjob.ListFilters.AccessibleClientIDs, the existing
	// sibling this mirrors). Anchors pass nil (no scoping — they see all).
	AccessibleClientIDs *[]string
	Limit               *int64
	Offset              *int64
}

// FindWithFilters returns functions matching f, ordered by address, with
// ApplicationCode populated read-side via a join. Hand-rolled dynamic
// query — see docs/sqlc.md "Dynamic queries".
func (r *Repository) FindWithFilters(ctx context.Context, f ListFilters) ([]Function, error) {
	var flt repocommon.Filter
	flt.EqPtr("f.application_id", f.ApplicationID)
	flt.EqPtr("f.client_id", f.ClientID)
	if f.AddressPrefix != nil {
		flt.Clause("f.address LIKE $%d", *f.AddressPrefix+"%")
	}
	if f.AccessibleClientIDs != nil {
		flt.Clause("(f.client_id IS NULL OR f.client_id = ANY($%d))", *f.AccessibleClientIDs)
	}

	q := `SELECT f.id, f.application_id, f.client_id, f.name, f.address,
			f.description, f.pool, f.warm, f.limits, f.created_by,
			f.created_at, f.updated_at, a.code AS application_code
		FROM fn_functions f
		LEFT JOIN app_applications a ON a.id = f.application_id` + flt.Where() + ` ORDER BY f.address`
	if f.Limit != nil {
		q += fmt.Sprintf(" LIMIT $%d", flt.Arg(*f.Limit))
	}
	if f.Offset != nil {
		q += fmt.Sprintf(" OFFSET $%d", flt.Arg(*f.Offset))
	}

	rows, err := r.pool.Query(ctx, q, flt.Args()...)
	if err != nil {
		return nil, err
	}
	collected, err := pgx.CollectRows(rows, pgx.RowToStructByName[functionJoinRow])
	if err != nil {
		return nil, err
	}
	out := make([]Function, 0, len(collected))
	for _, row := range collected {
		fn, err := rowToFunction(row.FnFunction, row.ApplicationCode)
		if err != nil {
			return nil, err
		}
		out = append(out, *fn)
	}
	return out, nil
}

// CountWithFilters returns the total function count for f (ignoring
// Limit/Offset). No join needed: every filter is a column on fn_functions.
func (r *Repository) CountWithFilters(ctx context.Context, f ListFilters) (int64, error) {
	var flt repocommon.Filter
	flt.EqPtr("application_id", f.ApplicationID)
	flt.EqPtr("client_id", f.ClientID)
	if f.AddressPrefix != nil {
		flt.Clause("address LIKE $%d", *f.AddressPrefix+"%")
	}
	if f.AccessibleClientIDs != nil {
		flt.Clause("(client_id IS NULL OR client_id = ANY($%d))", *f.AccessibleClientIDs)
	}
	q := `SELECT COUNT(*) FROM fn_functions` + flt.Where()
	rows, err := r.pool.Query(ctx, q, flt.Args()...)
	if err != nil {
		return 0, err
	}
	return pgx.CollectOneRow(rows, pgx.RowTo[int64])
}

// Persist implements usecasepgx.Persist[Function]. ApplicationCode is
// read-side only and is never written.
func (r *Repository) Persist(ctx context.Context, f *Function, tx *usecasepgx.DbTx) error {
	limits, err := json.Marshal(f.Limits)
	if err != nil {
		return fmt.Errorf("function repo: marshal limits: %w", err)
	}
	return r.q.WithTx(tx.Inner()).FunctionUpsert(ctx, dbq.FunctionUpsertParams{
		ID:            f.ID,
		ApplicationID: f.ApplicationID,
		ClientID:      f.ClientID,
		Name:          f.Name,
		Address:       f.Address,
		Description:   f.Description,
		Pool:          f.Pool,
		Warm:          f.Warm,
		Limits:        limits,
		CreatedBy:     f.CreatedBy,
		CreatedAt:     f.CreatedAt,
		UpdatedAt:     time.Now().UTC(),
	})
}

// Delete removes the function row. fn_versions, fn_aliases, and fn_settings
// cascade via their FK ON DELETE CASCADE (migration 059). Artifact deletion
// is a WP4 TODO (docs/function-runner-plan.md §8.2: "Delete cascades
// versions, aliases, wiring and artifacts" — the artifact store has no
// knowledge of which digests are now orphaned until a version-aware caller
// checks reference counts, which is out of WP3's scope).
func (r *Repository) Delete(ctx context.Context, f *Function, tx *usecasepgx.DbTx) error {
	return r.q.WithTx(tx.Inner()).FunctionDelete(ctx, f.ID)
}

// functionJoinRow is the row shape for FindWithFilters: every fn_functions
// column plus the read-side application_code join. pgx.RowToStructByName
// matches by db tag/field name across the embedded struct, same technique
// sqlc's own generated rows use.
type functionJoinRow struct {
	dbq.FnFunction
	ApplicationCode *string `db:"application_code"`
}

// rowToFunction hydrates the entity from its row plus the read-side
// application code (empty string when the join found no application row —
// display-only, never treated as a read failure per the "bare column, no
// FK" convention this table follows).
func rowToFunction(row dbq.FnFunction, applicationCode *string) (*Function, error) {
	var limits Limits
	if err := json.Unmarshal(row.Limits, &limits); err != nil {
		slog.Error("function row has unparseable limits", "id", row.ID, "err", err)
		return nil, usecase.Internal("CORRUPT_FUNCTION_LIMITS",
			fmt.Sprintf("function %s has unparseable limits", row.ID), err)
	}
	code := ""
	if applicationCode != nil {
		code = *applicationCode
	}
	return &Function{
		ID:              row.ID,
		ApplicationID:   row.ApplicationID,
		ApplicationCode: code,
		ClientID:        row.ClientID,
		Name:            row.Name,
		Address:         row.Address,
		Description:     row.Description,
		Pool:            row.Pool,
		Warm:            row.Warm,
		Limits:          limits,
		CreatedBy:       row.CreatedBy,
		CreatedAt:       row.CreatedAt,
		UpdatedAt:       row.UpdatedAt,
	}, nil
}

// ── Versions ────────────────────────────────────────────────────────────

// InsertVersion inserts a new version row.
func (r *Repository) InsertVersion(ctx context.Context, v *Version) error {
	return r.q.FunctionVersionInsert(ctx, dbq.FunctionVersionInsertParams{
		ID:          v.ID,
		FunctionID:  v.FunctionID,
		Number:      v.Number,
		Digest:      v.Digest,
		SizeBytes:   v.SizeBytes,
		Abi:         v.ABI,
		Describe:    v.Describe,
		Status:      string(v.Status),
		Failure:     v.Failure,
		ReadyAt:     v.ReadyAt,
		PublishedBy: v.PublishedBy,
		CreatedAt:   v.CreatedAt,
	})
}

// ListVersionsByFunction returns every version for functionID, newest number
// first.
func (r *Repository) ListVersionsByFunction(ctx context.Context, functionID string) ([]Version, error) {
	rows, err := r.q.FunctionVersionListByFunction(ctx, functionID)
	if err != nil {
		return nil, err
	}
	out := make([]Version, 0, len(rows))
	for _, row := range rows {
		v, err := rowToVersion(row)
		if err != nil {
			return nil, err
		}
		out = append(out, *v)
	}
	return out, nil
}

// GetVersionByNumber loads one version by its (function, number) key.
func (r *Repository) GetVersionByNumber(ctx context.Context, functionID string, number int32) (*Version, error) {
	res, err := r.q.FunctionVersionGetByNumber(ctx, dbq.FunctionVersionGetByNumberParams{
		FunctionID: functionID, Number: number,
	})
	row, err := repocommon.One(res, err, "function version repo")
	if row == nil || err != nil {
		return nil, err
	}
	return rowToVersion(*row)
}

// GetVersionByDigest loads one version by its (function, digest) key —
// "publishing the same digest twice returns the existing version" (plan §4).
func (r *Repository) GetVersionByDigest(ctx context.Context, functionID, digest string) (*Version, error) {
	res, err := r.q.FunctionVersionGetByDigest(ctx, dbq.FunctionVersionGetByDigestParams{
		FunctionID: functionID, Digest: digest,
	})
	row, err := repocommon.One(res, err, "function version repo")
	if row == nil || err != nil {
		return nil, err
	}
	return rowToVersion(*row)
}

// SetVersionStatus updates status only (e.g. → RETIRED).
func (r *Repository) SetVersionStatus(ctx context.Context, id string, status VersionStatus) error {
	return r.q.FunctionVersionSetStatus(ctx, dbq.FunctionVersionSetStatusParams{
		ID: id, Status: string(status),
	})
}

// SetVersionReady marks a version READY (a runner reported it LOADED — plan
// §6.5) and stamps ready_at.
func (r *Repository) SetVersionReady(ctx context.Context, id string, readyAt time.Time) error {
	return r.q.FunctionVersionSetReady(ctx, dbq.FunctionVersionSetReadyParams{
		ID: id, ReadyAt: &readyAt,
	})
}

// SetVersionFailure marks a version FAILED with the given failure detail.
func (r *Repository) SetVersionFailure(ctx context.Context, id string, failure json.RawMessage) error {
	return r.q.FunctionVersionSetFailure(ctx, dbq.FunctionVersionSetFailureParams{
		ID: id, Failure: failure,
	})
}

func rowToVersion(row dbq.FnVersion) (*Version, error) {
	status, ok := ParseVersionStatus(row.Status)
	if !ok {
		slog.Error("function version row has unrecognised status", "id", row.ID, "status", row.Status)
		return nil, usecase.Internal("CORRUPT_FUNCTION_VERSION_STATUS",
			fmt.Sprintf("function version %s has an unrecognised status", row.ID), nil)
	}
	return &Version{
		ID:          row.ID,
		FunctionID:  row.FunctionID,
		Number:      row.Number,
		Digest:      row.Digest,
		SizeBytes:   row.SizeBytes,
		ABI:         row.Abi,
		Describe:    row.Describe,
		Status:      status,
		Failure:     row.Failure,
		ReadyAt:     row.ReadyAt,
		PublishedBy: row.PublishedBy,
		CreatedAt:   row.CreatedAt,
	}, nil
}

// ── Aliases ─────────────────────────────────────────────────────────────

// UpsertAlias points (functionID, name) at versionID.
func (r *Repository) UpsertAlias(ctx context.Context, a *Alias) error {
	return r.q.FunctionAliasUpsert(ctx, dbq.FunctionAliasUpsertParams{
		FunctionID: a.FunctionID,
		Name:       a.Name,
		VersionID:  a.VersionID,
		UpdatedAt:  a.UpdatedAt,
		UpdatedBy:  a.UpdatedBy,
	})
}

// DeleteAlias removes one alias.
func (r *Repository) DeleteAlias(ctx context.Context, functionID, name string) error {
	return r.q.FunctionAliasDelete(ctx, dbq.FunctionAliasDeleteParams{
		FunctionID: functionID, Name: name,
	})
}

// ListAliases returns every alias for functionID.
func (r *Repository) ListAliases(ctx context.Context, functionID string) ([]Alias, error) {
	rows, err := r.q.FunctionAliasList(ctx, functionID)
	if err != nil {
		return nil, err
	}
	out := make([]Alias, 0, len(rows))
	for _, row := range rows {
		out = append(out, Alias{
			FunctionID: row.FunctionID,
			Name:       row.Name,
			VersionID:  row.VersionID,
			UpdatedAt:  row.UpdatedAt,
			UpdatedBy:  row.UpdatedBy,
		})
	}
	return out, nil
}

// ── Settings ────────────────────────────────────────────────────────────

// UpsertSetting writes one setting. SECRET and DB values are encrypted at
// rest through encryption.EncryptSecretRef — the same at-rest convention
// iam_service_accounts.wh_*_ref uses: a plaintext value becomes
// "encrypted:<blob>", an already-encrypted or external secret-manager
// reference passes through unchanged, and a plaintext write with no key
// configured fails (encryption.ErrNotConfigured) rather than storing
// plaintext. CONFIG values are stored as given.
func (r *Repository) UpsertSetting(ctx context.Context, s *Setting) error {
	value := s.Value
	if s.Kind == SettingSecret || s.Kind == SettingDB {
		ref, err := encryption.EncryptSecretRef(r.enc, &value)
		if err != nil {
			return fmt.Errorf("function repo: encrypt setting %s/%s: %w", s.Kind, s.Key, err)
		}
		value = *ref
	}
	return r.q.FunctionSettingUpsert(ctx, dbq.FunctionSettingUpsertParams{
		FunctionID: s.FunctionID,
		Kind:       string(s.Kind),
		Key:        s.Key,
		Value:      value,
		UpdatedAt:  time.Now().UTC(),
	})
}

// DeleteSetting removes one setting.
func (r *Repository) DeleteSetting(ctx context.Context, functionID string, kind SettingKind, key string) error {
	return r.q.FunctionSettingDelete(ctx, dbq.FunctionSettingDeleteParams{
		FunctionID: functionID, Kind: string(kind), Key: key,
	})
}

// ListSettings returns every setting for functionID. SECRET and DB values
// are NEVER decrypted or returned here — Value is always "" for those two
// kinds (plan §8.2: "Secrets are write-only and never returned"). Only
// CONFIG values carry their real Value. Callers that need a DB DSN or a
// secret's plaintext for delivery (the runner's control plane, WP5) use a
// dedicated resolve path, not this list.
func (r *Repository) ListSettings(ctx context.Context, functionID string) ([]Setting, error) {
	rows, err := r.q.FunctionSettingList(ctx, functionID)
	if err != nil {
		return nil, err
	}
	out := make([]Setting, 0, len(rows))
	for _, row := range rows {
		kind, ok := ParseSettingKind(row.Kind)
		if !ok {
			slog.Error("function setting row has unrecognised kind",
				"function_id", row.FunctionID, "kind", row.Kind)
			return nil, usecase.Internal("CORRUPT_FUNCTION_SETTING_KIND",
				fmt.Sprintf("function %s has a setting with an unrecognised kind", row.FunctionID), nil)
		}
		value := row.Value
		if kind != SettingConfig {
			value = ""
		}
		out = append(out, Setting{
			FunctionID: row.FunctionID,
			Kind:       kind,
			Key:        row.Key,
			Value:      value,
			UpdatedAt:  row.UpdatedAt,
		})
	}
	return out, nil
}

// ── Runners ─────────────────────────────────────────────────────────────

// UpsertRunnerHeartbeat records (or refreshes) a runner's heartbeat + budget
// report (plan §6.5, §7). Not transactional: heartbeats are a hot,
// frequent, best-effort write, not a use-case commit.
func (r *Repository) UpsertRunnerHeartbeat(ctx context.Context, id, pool string, report json.RawMessage) error {
	return r.q.FunctionRunnerUpsertHeartbeat(ctx, dbq.FunctionRunnerUpsertHeartbeatParams{
		ID: id, Pool: pool, HeartbeatAt: time.Now().UTC(), Report: report,
	})
}

// ── Pool revisions ──────────────────────────────────────────────────────

// BumpPoolRevision increments (creating if absent) pool's desired-state
// revision counter inside tx, returning the new value. Callers (promote,
// alias, settings-change operations — WP4/WP5/WP8) run this in the same
// transaction as the change it announces, so a long-poller reading the
// revision after seeing the transaction commit always sees the matching
// state. Issuing `NOTIFY fn_desired` alongside this bump is a WP5 (control
// plane) concern, not WP3's.
func (r *Repository) BumpPoolRevision(ctx context.Context, tx *usecasepgx.DbTx, pool string) (int64, error) {
	return r.q.WithTx(tx.Inner()).FunctionPoolRevisionBump(ctx, pool)
}

// GetPoolRevision reads a pool's current revision. ok=false when the pool
// has never had a revision bumped (no row yet) — the caller's `wait=` poll
// should treat that as revision 0, not an error.
func (r *Repository) GetPoolRevision(ctx context.Context, pool string) (int64, bool, error) {
	res, err := r.q.FunctionPoolRevisionGet(ctx, pool)
	row, err := repocommon.One(res, err, "function pool revision repo")
	if err != nil {
		return 0, false, err
	}
	if row == nil {
		return 0, false, nil
	}
	return *row, true, nil
}
