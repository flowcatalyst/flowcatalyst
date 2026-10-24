package ingestion

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/flowcatalyst/flowcatalyst-go/internal/ids"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/repocommon"
	"github.com/flowcatalyst/flowcatalyst-go/internal/sqlc/dbq"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecasepgx"
)

// Repository is the Postgres-backed repo. Tables: ing_ingestions, ing_versions.
type Repository struct {
	pool *pgxpool.Pool // retained for the filtered list
	q    *dbq.Queries
}

// NewRepository wires a repo.
func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool, q: dbq.New(pool)}
}

// FindByID loads by primary key.
func (r *Repository) FindByID(ctx context.Context, id string) (*Ingestion, error) {
	res, err := r.q.IngestionFindByID(ctx, id)
	row, err := repocommon.One(res, err, "ingestion repo")
	if row == nil || err != nil {
		return nil, err
	}
	return rowToIngestion(*row)
}

// FindByCode loads by the unique (application, client, code).
func (r *Repository) FindByCode(ctx context.Context, applicationID ids.ApplicationID, clientID ids.ClientID, code string) (*Ingestion, error) {
	res, err := r.q.IngestionFindByCode(ctx, dbq.IngestionFindByCodeParams{
		ApplicationID: applicationID, ClientID: clientID, Code: code,
	})
	row, err := repocommon.One(res, err, "ingestion repo")
	if row == nil || err != nil {
		return nil, err
	}
	return rowToIngestion(*row)
}

// ListFilter drives List. Non-nil fields are ANDed together.
type ListFilter struct {
	ApplicationCode *string
	ClientID        *string
	// Q matches the code or the name, ignoring case.
	Q *string
	// Status is ACTIVE or PAUSED.
	Status *string
	// Kind is a source kind; for a custom source it matches the type too.
	Kind *string
	// AccessibleClientIDs, when non-nil, restricts results to those clients:
	// the reach of a caller below anchor tier (anchors pass nil).
	AccessibleClientIDs *[]string
	// AccessibleApplicationIDs, when non-nil, restricts results to those
	// applications: the reach of an application-scoped service account.
	AccessibleApplicationIDs *[]string
	// After is the Next of the previous page; empty starts at the beginning.
	After string
	// Limit is the page size: 50 when zero, at most 200.
	Limit int
}

// ListItem is an ingestion with its application's code and its client's
// identifier, read with it for display.
type ListItem struct {
	Ingestion
	ApplicationCode  string `json:"applicationCode"`
	ClientIdentifier string `json:"clientIdentifier"`
}

// Page is one page of a list. Next is empty on the last page.
type Page struct {
	Items []ListItem
	Next  string
}

// listRow is every ing_ingestions column plus the read-side joins; pgx matches
// by db tag across the embedded struct, as sqlc's own rows do.
type listRow struct {
	dbq.IngIngestion
	ApplicationCode  string `db:"application_code"`
	ClientIdentifier string `db:"client_identifier"`
}

// List returns ingestions matching f, oldest id first, a page at a time.
// Hand-rolled dynamic query — see docs/sqlc.md "Dynamic queries".
func (r *Repository) List(ctx context.Context, f ListFilter) (*Page, error) {
	var flt repocommon.Filter
	flt.EqPtr("a.code", f.ApplicationCode)
	flt.EqPtr("i.client_id", f.ClientID)
	if f.Q != nil && strings.TrimSpace(*f.Q) != "" {
		like := "%" + escapeLike(strings.TrimSpace(*f.Q)) + "%"
		flt.Clause(`(i.code ILIKE $%d ESCAPE '\' OR i.name ILIKE $%[1]d ESCAPE '\')`, like)
	}
	if f.Status != nil {
		switch *f.Status {
		case "ACTIVE":
			flt.Cond("i.active")
		case "PAUSED":
			flt.Cond("NOT i.active")
		default:
			return nil, usecase.Validation("INGESTION_FILTER_INVALID", "status must be ACTIVE or PAUSED")
		}
	}
	if f.Kind != nil {
		flt.Clause(`(i.source->>'kind' = $%[1]d OR (i.source->>'kind' = 'custom' AND i.source->>'type' = $%[1]d))`, *f.Kind)
	}
	if f.AccessibleClientIDs != nil {
		flt.Clause("i.client_id = ANY($%d)", *f.AccessibleClientIDs)
	}
	if f.AccessibleApplicationIDs != nil {
		flt.Clause("i.application_id = ANY($%d)", *f.AccessibleApplicationIDs)
	}
	if f.After != "" {
		flt.Clause("i.id > $%d", f.After)
	}
	limit := f.Limit
	if limit <= 0 {
		limit = 50
	}
	limit = min(limit, 200)

	// One more than the page, to know whether there is a next one.
	q := `SELECT i.*, a.code AS application_code, c.identifier AS client_identifier
		FROM ing_ingestions i
		JOIN app_applications a ON a.id = i.application_id
		JOIN tnt_clients c ON c.id = i.client_id` + flt.Where() +
		fmt.Sprintf(" ORDER BY i.id LIMIT $%d", flt.Arg(limit+1))
	rows, err := r.pool.Query(ctx, q, flt.Args()...)
	if err != nil {
		return nil, fmt.Errorf("ingestion repo: list: %w", err)
	}
	collected, err := pgx.CollectRows(rows, pgx.RowToStructByName[listRow])
	if err != nil {
		return nil, fmt.Errorf("ingestion repo: list: %w", err)
	}
	page := &Page{Items: make([]ListItem, 0, min(len(collected), limit))}
	for i, row := range collected {
		if i == limit {
			page.Next = collected[limit-1].ID
			break
		}
		ing, err := rowToIngestion(row.IngIngestion)
		if err != nil {
			return nil, err
		}
		page.Items = append(page.Items, ListItem{Ingestion: *ing, ApplicationCode: row.ApplicationCode, ClientIdentifier: row.ClientIdentifier})
	}
	return page, nil
}

func escapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}

// Persist implements usecasepgx.Persist[Ingestion]: it inserts a new
// ingestion, or updates one that was read, and writes the versions saved since.
// An update is guarded on the version pointer and the latest number the read
// saw; if either has moved, it is a conflict (INGESTION_VERSION_CONFLICT), not
// a lost update.
func (r *Repository) Persist(ctx context.Context, i *Ingestion, tx *usecasepgx.DbTx) error {
	q := r.q.WithTx(tx.Inner())
	source, err := json.Marshal(i.Definition.Source)
	if err != nil {
		return fmt.Errorf("ingestion repo: marshal source: %w", err)
	}
	split, err := json.Marshal(i.Definition.Split)
	if err != nil {
		return fmt.Errorf("ingestion repo: marshal split: %w", err)
	}
	target, err := json.Marshal(i.Definition.Target)
	if err != nil {
		return fmt.Errorf("ingestion repo: marshal target: %w", err)
	}

	if !i.loaded {
		err = q.IngestionInsert(ctx, dbq.IngestionInsertParams{
			ID: i.ID, ApplicationID: i.ApplicationID, ClientID: i.ClientID, Code: i.Code,
			Name: i.Definition.Name, Active: i.Active,
			Version: int32(i.Version), LatestVersion: int32(i.LatestVersion),
			Source: source, Split: split, Target: target,
			KeepFailedPayloads: i.Definition.KeepFailedPayloads, ManagedBy: i.ManagedBy,
			CreatedAt: i.CreatedAt, UpdatedAt: i.UpdatedAt,
		})
		if isCodeTaken(err) {
			return usecase.Conflict("INGESTION_CODE_EXISTS",
				fmt.Sprintf("an ingestion with code %q already exists for this application and client", i.Code))
		}
		if err != nil {
			return fmt.Errorf("ingestion repo: insert: %w", err)
		}
	} else {
		n, err := q.IngestionUpdate(ctx, dbq.IngestionUpdateParams{
			ID: i.ID, LoadedVersion: int32(i.loadedVersion), LoadedLatest: int32(i.loadedLatest),
			Name: i.Definition.Name, Active: i.Active,
			Version: int32(i.Version), LatestVersion: int32(i.LatestVersion),
			Source: source, Split: split, Target: target,
			KeepFailedPayloads: i.Definition.KeepFailedPayloads, ManagedBy: i.ManagedBy,
			UpdatedAt: i.UpdatedAt,
		})
		if err != nil {
			return fmt.Errorf("ingestion repo: update: %w", err)
		}
		if n == 0 {
			return usecase.Conflict("INGESTION_VERSION_CONFLICT",
				"the ingestion was changed since it was read; read it again and retry")
		}
	}

	for _, v := range i.pending {
		def, err := json.Marshal(v.Definition)
		if err != nil {
			return fmt.Errorf("ingestion repo: marshal version: %w", err)
		}
		if err := q.IngestionVersionInsert(ctx, dbq.IngestionVersionInsertParams{
			ID: v.ID, IngestionID: v.IngestionID, Version: int32(v.Number), Definition: def,
			Note: v.Note, AppliedFrom: v.AppliedFrom, CreatedBy: v.CreatedBy, CreatedAt: v.CreatedAt,
		}); err != nil {
			return fmt.Errorf("ingestion repo: insert version: %w", err)
		}
	}
	return nil
}

// Delete removes the ingestion; its versions, leases, state and route states
// go with it. Failure and metric rows are left for retention to sweep.
func (r *Repository) Delete(ctx context.Context, i *Ingestion, tx *usecasepgx.DbTx) error {
	return r.q.WithTx(tx.Inner()).IngestionDelete(ctx, i.ID)
}

// FindVersion loads one saved version, nil when the ingestion has not saved it.
func (r *Repository) FindVersion(ctx context.Context, ingestionID string, number int) (*Version, error) {
	res, err := r.q.IngestionVersionFind(ctx, dbq.IngestionVersionFindParams{IngestionID: ingestionID, Version: int32(number)})
	row, err := repocommon.One(res, err, "ingestion repo")
	if row == nil || err != nil {
		return nil, err
	}
	return rowToVersion(*row)
}

// ListVersions returns every saved version, newest first.
func (r *Repository) ListVersions(ctx context.Context, ingestionID string) ([]Version, error) {
	rows, err := r.q.IngestionVersionList(ctx, ingestionID)
	if err != nil {
		return nil, fmt.Errorf("ingestion repo: list versions: %w", err)
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

func isCodeTaken(err error) bool {
	pgErr, ok := errors.AsType[*pgconn.PgError](err)
	return ok && pgErr.Code == "23505" && pgErr.ConstraintName == "uq_ing_ingestions_code"
}

// rowToIngestion hydrates the aggregate. A definition that no longer parses
// (a hand-edited row, or a field this build does not know) is a loud read
// error, never a silent default, per the X-06 ruling; the row id is logged so
// it can be found.
func rowToIngestion(row dbq.IngIngestion) (*Ingestion, error) {
	def, err := definitionFromColumns(row)
	if err != nil {
		slog.Error("ingestion row does not parse", "id", row.ID, "err", err)
		return nil, usecase.Internal("CORRUPT_INGESTION",
			fmt.Sprintf("ingestion %s does not parse", row.ID), err)
	}
	return &Ingestion{
		ID:            row.ID,
		ApplicationID: row.ApplicationID,
		ClientID:      row.ClientID,
		Code:          row.Code,
		Active:        row.Active,
		Version:       int(row.Version),
		LatestVersion: int(row.LatestVersion),
		Definition:    def,
		ManagedBy:     row.ManagedBy,
		CreatedAt:     row.CreatedAt,
		UpdatedAt:     row.UpdatedAt,
		loadedVersion: int(row.Version),
		loadedLatest:  int(row.LatestVersion),
		loaded:        true,
	}, nil
}

func definitionFromColumns(row dbq.IngIngestion) (Definition, error) {
	src, err := UnmarshalSource(row.Source)
	if err != nil {
		return Definition{}, err
	}
	split, err := UnmarshalSplit(row.Split)
	if err != nil {
		return Definition{}, err
	}
	var target Target
	if err := strictDecode(row.Target, &target); err != nil {
		return Definition{}, fmt.Errorf("target: %w", err)
	}
	return Definition{Name: row.Name, KeepFailedPayloads: row.KeepFailedPayloads, Source: src, Split: split, Target: target}, nil
}

func rowToVersion(row dbq.IngVersion) (*Version, error) {
	var def Definition
	if err := json.Unmarshal(row.Definition, &def); err != nil {
		slog.Error("ingestion version row does not parse", "id", row.ID, "err", err)
		return nil, usecase.Internal("CORRUPT_INGESTION_VERSION",
			fmt.Sprintf("version %s of ingestion %s does not parse", row.ID, row.IngestionID), err)
	}
	return &Version{
		ID:          row.ID,
		IngestionID: row.IngestionID,
		Number:      int(row.Version),
		Definition:  def,
		Note:        row.Note,
		AppliedFrom: row.AppliedFrom,
		CreatedBy:   row.CreatedBy,
		CreatedAt:   row.CreatedAt,
	}, nil
}
