package fieldidentity

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/flowcatalyst/go-sdk/usecasepgx"
)

// Repos bundles the field-identity repositories. Each aggregate has its own
// repository because usecasepgx.Persist is one Persist/Delete pair per type.
type Repos struct {
	Identities *IdentityRepo
	Devices    *DeviceRepo
	Codes      *CodeRepo
	Policies   *PolicyRepo
	Sessions   *SessionRepo
	Badges     *BadgeRepo
	SignIns    *SignInCodeRepo
}

// NewRepos wires every field-identity repository against the pool.
func NewRepos(pool *pgxpool.Pool) *Repos {
	return &Repos{
		Identities: &IdentityRepo{pool: pool},
		Devices:    &DeviceRepo{pool: pool},
		Codes:      &CodeRepo{pool: pool},
		Policies:   &PolicyRepo{pool: pool},
		Sessions:   &SessionRepo{pool: pool},
		Badges:     &BadgeRepo{pool: pool},
		SignIns:    &SignInCodeRepo{pool: pool},
	}
}

func emptyToNil(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func escapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}

// ── identities ───────────────────────────────────────────────────────────

// IdentityRepo stores field_identities + field_identity_apps. Implements
// usecasepgx.Persist[Identity].
type IdentityRepo struct{ pool *pgxpool.Pool }

const identityColumns = `fi.id, fi.client_id, fi.worker_code, fi.name, fi.email, fi.phone, fi.status,
	fi.pin_hash, fi.pin_scheme, fi.pepper_version, fi.pin_set_at, fi.pin_change_required,
	fi.failed_since_success, fi.locked_until, fi.hard_locked, fi.last_login_at,
	fi.can_impersonate, fi.is_test, fi.linked_principal_id, fi.created_at, fi.updated_at`

const identitySelect = `SELECT ` + identityColumns + ` FROM field_identities fi`

// FindByID returns the worker (with grants), nil if absent.
func (r *IdentityRepo) FindByID(ctx context.Context, id string) (*Identity, error) {
	return r.one(ctx, identitySelect+` WHERE fi.id = $1`, id)
}

// FindByClientAndCode returns the worker for (client, code), nil if absent.
func (r *IdentityRepo) FindByClientAndCode(ctx context.Context, clientID, code string) (*Identity, error) {
	return r.one(ctx, identitySelect+` WHERE fi.client_id = $1 AND fi.worker_code = $2`,
		clientID, NormalizeWorkerCode(code))
}

// FindByClientAndPrincipal returns the worker of the client linked to the
// platform user, nil if none.
func (r *IdentityRepo) FindByClientAndPrincipal(ctx context.Context, clientID, principalID string) (*Identity, error) {
	return r.one(ctx, identitySelect+` WHERE fi.client_id = $1 AND fi.linked_principal_id = $2`,
		clientID, principalID)
}

// PrincipalRef names a platform user a worker is linked to.
type PrincipalRef struct {
	ID    string
	Name  string
	Email string
}

// PrincipalRefs resolves platform users by id for display. An id that no
// longer resolves (the principal was deleted; the link has no foreign key) is
// absent from the map.
func (r *IdentityRepo) PrincipalRefs(ctx context.Context, ids []string) (map[string]PrincipalRef, error) {
	out := make(map[string]PrincipalRef, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := r.pool.Query(ctx, `SELECT id, name, email FROM iam_principals WHERE id = ANY($1)`, ids)
	if err != nil {
		return nil, fmt.Errorf("field_identity principal refs: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var p PrincipalRef
		var email *string
		if err := rows.Scan(&p.ID, &p.Name, &email); err != nil {
			return nil, err
		}
		p.Email = deref(email)
		out[p.ID] = p
	}
	return out, rows.Err()
}

// PrincipalByEmail resolves a platform principal by email address
// (case-insensitive), nil if none; the link form's lookup.
func (r *IdentityRepo) PrincipalByEmail(ctx context.Context, email string) (*PrincipalRef, error) {
	var p PrincipalRef
	var mail *string
	err := r.pool.QueryRow(ctx, `SELECT id, name, email FROM iam_principals WHERE lower(email) = lower($1)`,
		strings.TrimSpace(email)).Scan(&p.ID, &p.Name, &mail)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("field_identity principal by email: %w", err)
	}
	p.Email = deref(mail)
	return &p, nil
}

// IdentityFilter narrows a client's workers for the admin list.
type IdentityFilter struct {
	ClientID string
	// Query is a case-insensitive prefix matched against name and worker code.
	Query string
	// AppID restricts to workers granted that application.
	AppID string
	// Status restricts to ACTIVE or DISABLED ("" = all).
	Status Status
	// IsTest, when non-nil, restricts to test (true) or real (false) identities.
	IsTest *bool
	// LinkedPrincipalID restricts to the worker linked to that platform user.
	LinkedPrincipalID string
	// AppIDs, when non-nil, restricts to workers granted at least one of
	// these applications (an application-confined caller's own set).
	AppIDs []string
	Offset int
	Limit  int
}

// Search lists a client's workers, newest first, with the total match count.
func (r *IdentityRepo) Search(ctx context.Context, f IdentityFilter) ([]Identity, int64, error) {
	where := []string{"fi.client_id = $1"}
	args := []any{f.ClientID}
	add := func(v any) string {
		args = append(args, v)
		return "$" + strconv.Itoa(len(args))
	}
	if q := strings.ToLower(strings.TrimSpace(f.Query)); q != "" {
		n := add(escapeLike(q) + "%")
		where = append(where, `(lower(fi.name) LIKE `+n+` OR lower(fi.worker_code) LIKE `+n+`)`)
	}
	if f.AppID != "" {
		where = append(where, `EXISTS (SELECT 1 FROM field_identity_apps g
			WHERE g.identity_id = fi.id AND g.application_id = `+add(f.AppID)+`)`)
	}
	if f.Status != "" {
		where = append(where, `fi.status = `+add(string(f.Status)))
	}
	if f.IsTest != nil {
		where = append(where, `fi.is_test = `+add(*f.IsTest))
	}
	if f.LinkedPrincipalID != "" {
		where = append(where, `fi.linked_principal_id = `+add(f.LinkedPrincipalID))
	}
	if f.AppIDs != nil {
		where = append(where, `EXISTS (SELECT 1 FROM field_identity_apps g
			WHERE g.identity_id = fi.id AND g.application_id = ANY(`+add(f.AppIDs)+`))`)
	}
	cond := strings.Join(where, " AND ")

	var total int64
	if err := r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM field_identities fi WHERE `+cond, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("field_identity search count: %w", err)
	}
	lim, off := add(f.Limit), add(f.Offset)
	rows, err := r.pool.Query(ctx,
		identitySelect+` WHERE `+cond+` ORDER BY fi.created_at DESC, fi.id DESC LIMIT `+lim+` OFFSET `+off, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("field_identity search: %w", err)
	}
	out, err := scanIdentities(rows)
	if err != nil {
		return nil, 0, err
	}
	if err := r.attachApps(ctx, out); err != nil {
		return nil, 0, err
	}
	return out, total, nil
}

// Persist implements usecasepgx.Persist[Identity]. A newly constructed worker
// is inserted (a lost race on the (client, code) key leaves the winner's row
// untouched and the grants land on it); a loaded worker is updated by id.
// Grants, explicit grant revocations and queued session revocations are then
// applied in the same transaction.
func (r *IdentityRepo) Persist(ctx context.Context, i *Identity, tx *usecasepgx.DbTx) error {
	now := time.Now().UTC()
	rowID := i.ID
	if i.ID != "" && i.isNew() {
		err := tx.Inner().QueryRow(ctx,
			`INSERT INTO field_identities
			     (id, client_id, worker_code, name, email, phone, status, pin_hash, pin_scheme, pepper_version,
			      pin_set_at, pin_change_required, failed_since_success, locked_until, hard_locked,
			      last_login_at, can_impersonate, is_test, linked_principal_id, created_at, updated_at)
			 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21)
			 ON CONFLICT (client_id, worker_code) DO UPDATE SET updated_at = field_identities.updated_at
			 RETURNING id`,
			i.ID, i.ClientID, i.WorkerCode, i.Name, emptyToNil(i.Email), emptyToNil(i.Phone), string(i.Status), i.PINHash,
			i.PINScheme, i.PepperVersion, i.PINSetAt, i.PINChangeRequired, i.FailedSinceSuccess,
			i.LockedUntil, i.HardLocked, i.LastLoginAt, i.CanImpersonate, i.IsTest, emptyToNil(i.LinkedPrincipalID),
			i.CreatedAt, now).Scan(&rowID)
		if err != nil {
			return err
		}
		i.fresh = false
	} else {
		tag, err := tx.Inner().Exec(ctx,
			`UPDATE field_identities SET name=$2, email=$3, phone=$4, status=$5, pin_hash=$6, pin_scheme=$7,
			     pepper_version=$8, pin_set_at=$9, pin_change_required=$10, failed_since_success=$11,
			     locked_until=$12, hard_locked=$13, last_login_at=$14, can_impersonate=$15, is_test=$16,
			     linked_principal_id=$17, updated_at=$18
			 WHERE id = $1`,
			i.ID, i.Name, emptyToNil(i.Email), emptyToNil(i.Phone), string(i.Status), i.PINHash, i.PINScheme,
			i.PepperVersion, i.PINSetAt, i.PINChangeRequired, i.FailedSinceSuccess,
			i.LockedUntil, i.HardLocked, i.LastLoginAt, i.CanImpersonate, i.IsTest, emptyToNil(i.LinkedPrincipalID), now)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return errors.New("field identity vanished before update")
		}
	}
	if len(i.revoked) > 0 {
		if _, err := tx.Inner().Exec(ctx,
			`DELETE FROM field_identity_apps WHERE identity_id = $1 AND application_id = ANY($2)`,
			rowID, i.revoked); err != nil {
			return err
		}
		i.revoked = nil
	}
	for _, g := range i.Apps {
		if _, err := tx.Inner().Exec(ctx,
			`INSERT INTO field_identity_apps (identity_id, application_id, source, granted_at)
			 VALUES ($1,$2,$3,$4) ON CONFLICT DO NOTHING`,
			rowID, g.AppID, g.Source, g.GrantedAt); err != nil {
			return err
		}
	}
	for _, s := range i.sessions {
		cond := `identity_id = $1`
		if s.actor {
			cond = `actor_id = $1 AND actor_type = '` + ActorFieldWorker + `'`
		}
		if err := revokeSessions(ctx, tx, cond, rowID, s.reason, s.appID); err != nil {
			return err
		}
	}
	i.sessions = nil
	return nil
}

// Delete removes the worker: its sessions (and the sessions it holds open as an
// actor), its device assignments and unused assigned enrolment codes are
// cleared first; badges and sign-in codes go with the row (cascade). Sign-in
// history is kept.
func (r *IdentityRepo) Delete(ctx context.Context, i *Identity, tx *usecasepgx.DbTx) error {
	for _, q := range []string{
		`DELETE FROM field_sessions WHERE identity_id = $1 OR (actor_id = $1 AND actor_type = '` + ActorFieldWorker + `')`,
		`UPDATE field_devices SET assigned_identity_id = NULL, updated_at = NOW() WHERE assigned_identity_id = $1`,
		`DELETE FROM field_enrolment_codes WHERE assigned_identity_id = $1 AND used_at IS NULL`,
		`DELETE FROM field_identities WHERE id = $1`,
	} {
		if _, err := tx.Inner().Exec(ctx, q, i.ID); err != nil {
			return err
		}
	}
	return nil
}

func (r *IdentityRepo) one(ctx context.Context, sql string, args ...any) (*Identity, error) {
	rows, err := r.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("field_identity repo: %w", err)
	}
	all, err := scanIdentities(rows)
	if err != nil || len(all) == 0 {
		return nil, err
	}
	if err := r.attachApps(ctx, all[:1]); err != nil {
		return nil, err
	}
	return &all[0], nil
}

func (r *IdentityRepo) attachApps(ctx context.Context, idents []Identity) error {
	return attachAppsQ(ctx, r.pool, idents)
}

// querier is the read surface shared by a pool and a transaction.
type querier interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

func attachAppsQ(ctx context.Context, q querier, idents []Identity) error {
	if len(idents) == 0 {
		return nil
	}
	ids := make([]string, len(idents))
	index := make(map[string]int, len(idents))
	for n := range idents {
		ids[n] = idents[n].ID
		index[idents[n].ID] = n
		idents[n].Apps = []AppGrant{}
	}
	rows, err := q.Query(ctx,
		`SELECT identity_id, application_id, source, granted_at FROM field_identity_apps
		 WHERE identity_id = ANY($1) ORDER BY granted_at, application_id`, ids)
	if err != nil {
		return fmt.Errorf("field_identity apps: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var identityID string
		var g AppGrant
		if err := rows.Scan(&identityID, &g.AppID, &g.Source, &g.GrantedAt); err != nil {
			return err
		}
		if n, ok := index[identityID]; ok {
			idents[n].Apps = append(idents[n].Apps, g)
		}
	}
	return rows.Err()
}

func scanIdentities(rows pgx.Rows) ([]Identity, error) {
	defer rows.Close()
	out := []Identity{}
	for rows.Next() {
		var i Identity
		var email, phone, linked *string
		var status string
		if err := rows.Scan(&i.ID, &i.ClientID, &i.WorkerCode, &i.Name, &email, &phone, &status,
			&i.PINHash, &i.PINScheme, &i.PepperVersion, &i.PINSetAt, &i.PINChangeRequired,
			&i.FailedSinceSuccess, &i.LockedUntil, &i.HardLocked, &i.LastLoginAt,
			&i.CanImpersonate, &i.IsTest, &linked, &i.CreatedAt, &i.UpdatedAt); err != nil {
			return nil, err
		}
		i.Email = deref(email)
		i.Phone = deref(phone)
		i.LinkedPrincipalID = deref(linked)
		i.Status = Status(status)
		i.Apps = []AppGrant{}
		out = append(out, i)
	}
	return out, rows.Err()
}

// revokeSessions marks the live sessions matching cond (a predicate over
// field_sessions with the owner id as $1) revoked for reason; appID narrows
// to one application.
func revokeSessions(ctx context.Context, tx *usecasepgx.DbTx, cond, ownerID, reason, appID string) error {
	q := `UPDATE field_sessions SET revoked_at = NOW(), revoked_reason = $2
	      WHERE (` + cond + `) AND revoked_at IS NULL`
	args := []any{ownerID, reason}
	if appID != "" {
		q += ` AND application_id = $3`
		args = append(args, appID)
	}
	_, err := tx.Inner().Exec(ctx, q, args...)
	return err
}

// ── devices ──────────────────────────────────────────────────────────────

// DeviceRepo stores field_devices + field_device_apps. Implements
// usecasepgx.Persist[Device].
type DeviceRepo struct{ pool *pgxpool.Pool }

const deviceColumns = `fd.id, fd.client_id, fd.mode, fd.label, fd.platform, fd.model, fd.jkt,
	fd.public_key_jwk, fd.attestation_level, fd.assigned_identity_id, fd.status, fd.enrolled_by,
	fd.enrolled_at, fd.last_seen_at, fd.updated_at`

const deviceSelect = `SELECT ` + deviceColumns + ` FROM field_devices fd`

// FindByID returns the device (with its applications), nil if absent.
func (r *DeviceRepo) FindByID(ctx context.Context, id string) (*Device, error) {
	return r.one(ctx, deviceSelect+` WHERE fd.id = $1`, id)
}

// FindByJKT returns the device holding the key thumbprint, nil if absent.
func (r *DeviceRepo) FindByJKT(ctx context.Context, jkt string) (*Device, error) {
	return r.one(ctx, deviceSelect+` WHERE fd.jkt = $1`, jkt)
}

// DeviceFilter narrows a client's devices for the admin list.
type DeviceFilter struct {
	ClientID string
	Status   DeviceStatus
	Mode     Mode
	// AppIDs, when non-nil, restricts to devices enrolled for at least one
	// of these applications.
	AppIDs []string
	Offset int
	Limit  int
}

// Search lists a client's devices, newest first, with the total.
func (r *DeviceRepo) Search(ctx context.Context, f DeviceFilter) ([]Device, int64, error) {
	where := []string{"fd.client_id = $1"}
	args := []any{f.ClientID}
	add := func(v any) string {
		args = append(args, v)
		return "$" + strconv.Itoa(len(args))
	}
	if f.Status != "" {
		where = append(where, `fd.status = `+add(string(f.Status)))
	}
	if f.Mode != "" {
		where = append(where, `fd.mode = `+add(string(f.Mode)))
	}
	if f.AppIDs != nil {
		where = append(where, `EXISTS (SELECT 1 FROM field_device_apps a
			WHERE a.device_id = fd.id AND a.application_id = ANY(`+add(f.AppIDs)+`))`)
	}
	cond := strings.Join(where, " AND ")
	var total int64
	if err := r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM field_devices fd WHERE `+cond, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("field_device search count: %w", err)
	}
	lim, off := add(f.Limit), add(f.Offset)
	rows, err := r.pool.Query(ctx,
		deviceSelect+` WHERE `+cond+` ORDER BY fd.enrolled_at DESC, fd.id DESC LIMIT `+lim+` OFFSET `+off, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("field_device search: %w", err)
	}
	out, err := scanDevices(rows)
	if err != nil {
		return nil, 0, err
	}
	if err := r.attachApps(ctx, out); err != nil {
		return nil, 0, err
	}
	return out, total, nil
}

// Persist implements usecasepgx.Persist[Device].
func (r *DeviceRepo) Persist(ctx context.Context, d *Device, tx *usecasepgx.DbTx) error {
	now := time.Now().UTC()
	if _, err := tx.Inner().Exec(ctx,
		`INSERT INTO field_devices
		     (id, client_id, mode, label, platform, model, jkt, public_key_jwk, attestation_level,
		      assigned_identity_id, status, enrolled_by, enrolled_at, last_seen_at, updated_at, attestation_chain)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16)
		 ON CONFLICT (id) DO UPDATE SET
		     label = EXCLUDED.label, status = EXCLUDED.status,
		     assigned_identity_id = EXCLUDED.assigned_identity_id,
		     attestation_level = EXCLUDED.attestation_level, updated_at = EXCLUDED.updated_at`,
		d.ID, d.ClientID, string(d.Mode), emptyToNil(d.Label), emptyToNil(d.Platform), emptyToNil(d.Model),
		d.JKT, d.PublicKeyJWK, d.AttestationLevel, d.AssignedIdentityID, string(d.Status),
		emptyToNil(d.EnrolledBy), d.EnrolledAt, d.LastSeenAt, now, emptyToNil(d.AttestationChain)); err != nil {
		return err
	}
	for _, appID := range d.AppIDs {
		if _, err := tx.Inner().Exec(ctx,
			`INSERT INTO field_device_apps (device_id, application_id) VALUES ($1,$2) ON CONFLICT DO NOTHING`,
			d.ID, appID); err != nil {
			return err
		}
	}
	if d.sessions != nil {
		if err := revokeSessions(ctx, tx, `device_id = $1`, d.ID, d.sessions.reason, ""); err != nil {
			return err
		}
		d.sessions = nil
	}
	return nil
}

// Delete removes the device and its sessions.
func (r *DeviceRepo) Delete(ctx context.Context, d *Device, tx *usecasepgx.DbTx) error {
	for _, q := range []string{
		`DELETE FROM field_sessions WHERE device_id = $1`,
		`DELETE FROM field_devices WHERE id = $1`,
	} {
		if _, err := tx.Inner().Exec(ctx, q, d.ID); err != nil {
			return err
		}
	}
	return nil
}

// TouchLastSeen stamps last_seen_at, at most once a minute per device.
func (r *DeviceRepo) TouchLastSeen(ctx context.Context, id string) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE field_devices SET last_seen_at = NOW()
		 WHERE id = $1 AND (last_seen_at IS NULL OR last_seen_at < NOW() - INTERVAL '1 minute')`, id)
	return err
}

func (r *DeviceRepo) one(ctx context.Context, sql string, args ...any) (*Device, error) {
	rows, err := r.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("field_device repo: %w", err)
	}
	all, err := scanDevices(rows)
	if err != nil || len(all) == 0 {
		return nil, err
	}
	if err := r.attachApps(ctx, all[:1]); err != nil {
		return nil, err
	}
	return &all[0], nil
}

func (r *DeviceRepo) attachApps(ctx context.Context, devs []Device) error {
	if len(devs) == 0 {
		return nil
	}
	ids := make([]string, len(devs))
	index := make(map[string]int, len(devs))
	for n := range devs {
		ids[n] = devs[n].ID
		index[devs[n].ID] = n
		devs[n].AppIDs = []string{}
	}
	rows, err := r.pool.Query(ctx,
		`SELECT device_id, application_id FROM field_device_apps WHERE device_id = ANY($1) ORDER BY application_id`, ids)
	if err != nil {
		return fmt.Errorf("field_device apps: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var deviceID, appID string
		if err := rows.Scan(&deviceID, &appID); err != nil {
			return err
		}
		if n, ok := index[deviceID]; ok {
			devs[n].AppIDs = append(devs[n].AppIDs, appID)
		}
	}
	return rows.Err()
}

func scanDevices(rows pgx.Rows) ([]Device, error) {
	defer rows.Close()
	out := []Device{}
	for rows.Next() {
		var d Device
		var mode, status string
		var label, platform, model, enrolledBy *string
		if err := rows.Scan(&d.ID, &d.ClientID, &mode, &label, &platform, &model, &d.JKT,
			&d.PublicKeyJWK, &d.AttestationLevel, &d.AssignedIdentityID, &status, &enrolledBy,
			&d.EnrolledAt, &d.LastSeenAt, &d.UpdatedAt); err != nil {
			return nil, err
		}
		d.Mode, d.Status = Mode(mode), DeviceStatus(status)
		d.Label, d.Platform, d.Model, d.EnrolledBy = deref(label), deref(platform), deref(model), deref(enrolledBy)
		d.AppIDs = []string{}
		out = append(out, d)
	}
	return out, rows.Err()
}

// ── enrolment codes ──────────────────────────────────────────────────────

// CodeRepo stores field_enrolment_codes. Implements
// usecasepgx.Persist[EnrolmentCode].
type CodeRepo struct{ pool *pgxpool.Pool }

const codeColumns = `id, client_id, code_hash, mode, app_ids, assigned_identity_id, label,
	expires_at, used_at, created_by, created_at`

// FindByHash returns the code row for a SHA-256 hex digest, nil if absent.
func (r *CodeRepo) FindByHash(ctx context.Context, hash string) (*EnrolmentCode, error) {
	var c EnrolmentCode
	var mode string
	var label, createdBy *string
	err := r.pool.QueryRow(ctx, `SELECT `+codeColumns+` FROM field_enrolment_codes WHERE code_hash = $1`, hash).
		Scan(&c.ID, &c.ClientID, &c.CodeHash, &mode, &c.AppIDs, &c.AssignedIdentityID, &label,
			&c.ExpiresAt, &c.UsedAt, &createdBy, &c.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("field_enrolment_code repo: %w", err)
	}
	c.Mode, c.Label, c.CreatedBy = Mode(mode), deref(label), deref(createdBy)
	return &c, nil
}

// Consume atomically marks an unused, unexpired code used and returns it; nil
// when the code is unknown, used or expired. Runs inside the caller's tx so a
// failed enrolment does not burn the code.
func (r *CodeRepo) Consume(ctx context.Context, tx *usecasepgx.DbTx, hash string) (*EnrolmentCode, error) {
	var c EnrolmentCode
	var mode string
	var label, createdBy *string
	err := tx.Inner().QueryRow(ctx,
		`UPDATE field_enrolment_codes SET used_at = NOW()
		 WHERE code_hash = $1 AND used_at IS NULL AND expires_at > NOW()
		 RETURNING `+codeColumns, hash).
		Scan(&c.ID, &c.ClientID, &c.CodeHash, &mode, &c.AppIDs, &c.AssignedIdentityID, &label,
			&c.ExpiresAt, &c.UsedAt, &createdBy, &c.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	c.Mode, c.Label, c.CreatedBy = Mode(mode), deref(label), deref(createdBy)
	return &c, nil
}

// Persist implements usecasepgx.Persist[EnrolmentCode].
func (r *CodeRepo) Persist(ctx context.Context, c *EnrolmentCode, tx *usecasepgx.DbTx) error {
	appIDs := c.AppIDs
	if appIDs == nil {
		appIDs = []string{}
	}
	_, err := tx.Inner().Exec(ctx,
		`INSERT INTO field_enrolment_codes
		     (id, client_id, code_hash, mode, app_ids, assigned_identity_id, label, expires_at, used_at, created_by, created_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
		 ON CONFLICT (id) DO UPDATE SET used_at = EXCLUDED.used_at`,
		c.ID, c.ClientID, c.CodeHash, string(c.Mode), appIDs, c.AssignedIdentityID,
		emptyToNil(c.Label), c.ExpiresAt, c.UsedAt, emptyToNil(c.CreatedBy), c.CreatedAt)
	return err
}

// Delete removes the code row.
func (r *CodeRepo) Delete(ctx context.Context, c *EnrolmentCode, tx *usecasepgx.DbTx) error {
	_, err := tx.Inner().Exec(ctx, `DELETE FROM field_enrolment_codes WHERE id = $1`, c.ID)
	return err
}

// ── policies ─────────────────────────────────────────────────────────────

// PolicyRepo stores field_policies. Implements usecasepgx.Persist[Policy].
type PolicyRepo struct{ pool *pgxpool.Pool }

// Effective returns the client's policy, or the defaults (IsDefault true)
// when it has no row.
func (r *PolicyRepo) Effective(ctx context.Context, clientID string) (Policy, error) {
	p := Policy{ClientID: clientID}
	err := r.pool.QueryRow(ctx,
		`SELECT pin_min_length, soft_lock_threshold, soft_lock_secs, hard_lock_threshold, shared_idle_secs,
		        personal_idle_secs, shift_max_secs, require_attestation, pin_reauth_days,
		        require_enrolled_device, unenrolled_idle_secs, badge_only_login, impersonation_max_secs, updated_at
		 FROM field_policies WHERE client_id = $1`, clientID).
		Scan(&p.PINMinLength, &p.SoftLockThreshold, &p.SoftLockSecs, &p.HardLockThreshold, &p.SharedIdleSecs,
			&p.PersonalIdleSecs, &p.ShiftMaxSecs, &p.RequireAttestation, &p.PINReauthDays,
			&p.RequireEnrolledDevice, &p.UnenrolledIdleSecs, &p.BadgeOnlyLogin, &p.ImpersonationMaxSecs, &p.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return DefaultPolicy(clientID), nil
	}
	if err != nil {
		return Policy{}, fmt.Errorf("field_policy repo: %w", err)
	}
	return p, nil
}

// Persist implements usecasepgx.Persist[Policy] (upsert by client).
func (r *PolicyRepo) Persist(ctx context.Context, p *Policy, tx *usecasepgx.DbTx) error {
	p.UpdatedAt = time.Now().UTC()
	p.IsDefault = false
	_, err := tx.Inner().Exec(ctx,
		`INSERT INTO field_policies
		     (client_id, pin_min_length, soft_lock_threshold, soft_lock_secs, hard_lock_threshold, shared_idle_secs,
		      personal_idle_secs, shift_max_secs, require_attestation, pin_reauth_days,
		      require_enrolled_device, unenrolled_idle_secs, badge_only_login, impersonation_max_secs, updated_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)
		 ON CONFLICT (client_id) DO UPDATE SET
		     pin_min_length = EXCLUDED.pin_min_length, soft_lock_threshold = EXCLUDED.soft_lock_threshold,
		     soft_lock_secs = EXCLUDED.soft_lock_secs, hard_lock_threshold = EXCLUDED.hard_lock_threshold,
		     shared_idle_secs = EXCLUDED.shared_idle_secs, personal_idle_secs = EXCLUDED.personal_idle_secs,
		     shift_max_secs = EXCLUDED.shift_max_secs, require_attestation = EXCLUDED.require_attestation,
		     pin_reauth_days = EXCLUDED.pin_reauth_days,
		     require_enrolled_device = EXCLUDED.require_enrolled_device,
		     unenrolled_idle_secs = EXCLUDED.unenrolled_idle_secs, badge_only_login = EXCLUDED.badge_only_login,
		     impersonation_max_secs = EXCLUDED.impersonation_max_secs, updated_at = EXCLUDED.updated_at`,
		p.ClientID, p.PINMinLength, p.SoftLockThreshold, p.SoftLockSecs, p.HardLockThreshold, p.SharedIdleSecs,
		p.PersonalIdleSecs, p.ShiftMaxSecs, p.RequireAttestation, p.PINReauthDays,
		p.RequireEnrolledDevice, p.UnenrolledIdleSecs, p.BadgeOnlyLogin, p.ImpersonationMaxSecs, p.UpdatedAt)
	return err
}

// Delete removes the client's policy row (back to defaults).
func (r *PolicyRepo) Delete(ctx context.Context, p *Policy, tx *usecasepgx.DbTx) error {
	_, err := tx.Inner().Exec(ctx, `DELETE FROM field_policies WHERE client_id = $1`, p.ClientID)
	return err
}
