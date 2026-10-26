package ingestion

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/repocommon"
	"github.com/flowcatalyst/flowcatalyst-go/internal/sqlc/dbq"
)

// The runtime contract's versions (docs/ingestion-design.md section 7). The
// platform serves the current version and the one before it; a poll naming any
// other is refused with the supported range. Additive changes do not bump it.
const (
	ContractVersion    = 2
	MinContractVersion = ContractVersion - 1
)

// DefaultLease is how long a lease lasts without being renewed
// (FC_INGESTION_LEASE_SECONDS). A runtime renews it by polling, every minute.
const DefaultLease = 3 * time.Minute

// maxTestsPerPoll bounds the test requests one poll claims.
const maxTestsPerPoll = 10

// SupportedContract reports whether the platform serves contract version v.
func SupportedContract(v int) bool { return v >= MinContractVersion && v <= ContractVersion }

// PollRequest is one runtime's poll.
type PollRequest struct {
	RuntimeID string
	// ContractVersion is checked by the caller (SupportedContract).
	ContractVersion int
	// Kinds are the kinds the runtime's extensions add (and camel-route); see
	// CarriedKinds.
	Kinds []RuntimeKind
	// BuiltinKinds are the built-in kinds the runtime declares it carries; nil
	// when it declared none (the v2 baseline, BaselineBuiltInKinds).
	BuiltinKinds []string
	// URL is informational: the platform never calls it.
	URL *string
	// Clients and Applications confine the answer to the caller's reach, nil
	// for an axis it reaches in full (ListReach).
	Clients, Applications *[]string
	// Now is the time the poll is answered as of. Zero is the database's
	// clock, which is what the runtime API uses: every replica then times
	// leases by one clock, whatever its own says. Tests pass a time of their
	// own.
	Now time.Time
	// Lease is how long the leases this poll takes or renews last.
	Lease time.Duration
}

// PollResult is what a runtime should run and the tests it should try.
type PollResult struct {
	// Ingestions are the live, running ingestions of kinds the runtime carries:
	// every shared one, and the polled ones whose lease it now holds.
	Ingestions []ListItem
	Tests      []TestRequest
	// AsOf is the time the poll was answered as of (PollRequest.Now, or the
	// database's clock).
	AsOf time.Time
}

// clock is now, or the database's clock when now is zero: read in the
// caller's transaction, so every statement in it agrees on the time.
func clock(ctx context.Context, q *dbq.Queries, now time.Time) (time.Time, error) {
	if !now.IsZero() {
		return now, nil
	}
	t, err := q.IngestionDBNow(ctx)
	if err != nil {
		return time.Time{}, fmt.Errorf("database clock: %w", err)
	}
	return t.UTC(), nil
}

// Poll notes that the runtime is alive, takes or renews its leases, and
// answers with what it should run. A polled source (SingleOwner) is listed for
// one runtime at a time, the holder of its lease; a lease is never handed over
// directly: a runtime that stops running an ingestion marks its lease released
// and the lease passes to another runtime when it expires. A runtime that does
// not carry an ingestion's kind never gets it.
func (r *Repository) Poll(ctx context.Context, req PollRequest) (*PollResult, error) {
	reported := req.Kinds
	if reported == nil {
		reported = []RuntimeKind{}
	}
	kinds, err := json.Marshal(reported)
	if err != nil {
		return nil, fmt.Errorf("ingestion repo: marshal runtime kinds: %w", err)
	}
	var builtin json.RawMessage // NULL: the runtime declared no builtinKinds
	if req.BuiltinKinds != nil {
		if builtin, err = json.Marshal(req.BuiltinKinds); err != nil {
			return nil, fmt.Errorf("ingestion repo: marshal builtin kinds: %w", err)
		}
	}
	carried := CarriedKinds(reported, req.BuiltinKinds)

	res := &PollResult{}
	err = pgx.BeginFunc(ctx, r.pool, func(tx pgx.Tx) error {
		q := r.q.WithTx(tx)
		now, err := clock(ctx, q, req.Now)
		if err != nil {
			return err
		}
		req.Now, res.AsOf = now, now
		if err := q.IngestionRuntimeUpsert(ctx, dbq.IngestionRuntimeUpsertParams{
			RuntimeID: req.RuntimeID, Kinds: kinds, BuiltinKinds: builtin, ContractVersion: int32(req.ContractVersion),
			Url: req.URL, Now: now,
		}); err != nil {
			return fmt.Errorf("upsert runtime: %w", err)
		}
		if err := rememberRequirements(ctx, q, req.RuntimeID, reported, now); err != nil {
			return err
		}
		live, err := liveIngestions(ctx, tx, req.Clients, req.Applications)
		if err != nil {
			return err
		}
		// An ingestion whose settings are in a config entry is run as the
		// entry says; while the entry is missing or invalid it is not listed,
		// leased or counted.
		if live, err = resolveListed(ctx, q, live); err != nil {
			return err
		}
		// The lease section runs one poll at a time. Each poll renews what it
		// holds and then claims what is free, so two polls interleaved would
		// lock lease rows in different orders and could deadlock; serialised,
		// they cannot. A poll is once a minute per runtime: the wait is short.
		if err := q.IngestionLeasesLock(ctx); err != nil {
			return fmt.Errorf("lock leases: %w", err)
		}
		shares, err := fairShares(ctx, q, live, req)
		if err != nil {
			return err
		}
		mine, err := q.IngestionLeasesHeldBy(ctx, dbq.IngestionLeasesHeldByParams{RuntimeID: req.RuntimeID, Now: now})
		if err != nil {
			return fmt.Errorf("leases held: %w", err)
		}

		held := []string{} // not nil: sqlc sends a nil slice as NULL
		heldOf := map[string]int{}
		claim := func(item ListItem) (bool, error) {
			n, err := q.IngestionLeaseClaim(ctx, dbq.IngestionLeaseClaimParams{
				IngestionID: item.ID, RuntimeID: req.RuntimeID, Now: now, ExpiresAt: now.Add(req.Lease),
			})
			if err != nil {
				return false, fmt.Errorf("claim lease: %w", err)
			}
			if n == 0 {
				return false, nil // another runtime holds it
			}
			held = append(held, item.ID)
			heldOf[RunKind(item.Definition.Source)]++
			return true, nil
		}
		var runnable []ListItem
		for _, item := range live {
			if slices.Contains(carried, RunKind(item.Definition.Source)) {
				runnable = append(runnable, item)
			}
		}
		// What this runtime already holds is renewed first, up to its share of
		// the kind; then it claims unleased ones of the kind up to that share.
		keep := map[string]bool{}
		under := func(item ListItem) bool {
			kind := RunKind(item.Definition.Source)
			return heldOf[kind] < shares[kind]
		}
		for _, item := range runnable {
			if item.Definition.Source.SingleOwner() && slices.Contains(mine, item.ID) && under(item) {
				if ok, err := claim(item); err != nil {
					return err
				} else if ok {
					keep[item.ID] = true
				}
			}
		}
		for _, item := range runnable {
			if !item.Definition.Source.SingleOwner() || keep[item.ID] || !under(item) {
				continue
			}
			if ok, err := claim(item); err != nil {
				return err
			} else if ok {
				keep[item.ID] = true
			}
		}
		for _, item := range runnable {
			if !item.Definition.Source.SingleOwner() || keep[item.ID] {
				res.Ingestions = append(res.Ingestions, item)
			}
		}
		if _, err := q.IngestionLeaseReleaseExcept(ctx, dbq.IngestionLeaseReleaseExceptParams{
			RuntimeID: req.RuntimeID, Keep: held,
		}); err != nil {
			return fmt.Errorf("release leases: %w", err)
		}
		res.Tests, err = claimTests(ctx, q, req, carried)
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("ingestion repo: poll: %w", err)
	}
	return res, nil
}

// fairShares is, for each kind of polled (single-owner) ingestion, how many of
// that kind one runtime may hold: the live ingestions of the kind spread
// evenly over the live runtimes that carry it (those that polled within one
// lease window, the asker included). Counting per kind keeps a mixed fleet
// fair: a runtime that alone carries a kind takes all of it, whatever the
// other runtimes hold of other kinds, and two runtimes that both carry a kind
// split it. A runtime whose kinds row does not parse is left out of the count
// (logged): it cannot run anything until it polls again anyway.
func fairShares(ctx context.Context, q *dbq.Queries, live []ListItem, req PollRequest) (map[string]int, error) {
	polled := map[string]int{}
	for _, item := range live {
		if item.Definition.Source.SingleOwner() {
			polled[RunKind(item.Definition.Source)]++
		}
	}
	runtimes, err := q.IngestionRuntimesLiveKinds(ctx, dbq.IngestionRuntimesLiveKindsParams{Since: req.Now.Add(-req.Lease), Until: req.Now})
	if err != nil {
		return nil, fmt.Errorf("live runtimes: %w", err)
	}
	carriers := map[string]int{}
	for _, rt := range runtimes {
		names, err := decodeCarried(rt.Kinds, rt.BuiltinKinds)
		if err != nil {
			slog.Warn("ingestion runtime reported kinds that do not parse", "runtime_id", rt.RuntimeID, "err", err)
			continue
		}
		for _, k := range names {
			carriers[k]++
		}
	}
	shares := make(map[string]int, len(polled))
	for kind, n := range polled {
		shares[kind] = FairShare(n, carriers[kind])
	}
	return shares, nil
}

// liveIngestions reads the ingestions runtimes run: active, with a live
// version (idx_ing_ingestions_active), of a client that is active (a
// suspended or deactivated client's ingestions run nowhere), confined to the
// caller's reach, in id order.
func liveIngestions(ctx context.Context, tx pgx.Tx, clients, applications *[]string) ([]ListItem, error) {
	var flt repocommon.Filter
	flt.Cond("i.active AND i.version > 0")
	flt.Cond("c.status = 'ACTIVE'")
	if clients != nil {
		flt.Clause("i.client_id = ANY($%d)", *clients)
	}
	if applications != nil {
		flt.Clause("i.application_id = ANY($%d)", *applications)
	}
	rows, err := tx.Query(ctx, `SELECT i.*, a.code AS application_code, c.identifier AS client_identifier
		FROM ing_ingestions i
		JOIN app_applications a ON a.id = i.application_id
		JOIN tnt_clients c ON c.id = i.client_id`+flt.Where()+" ORDER BY i.id", flt.Args()...)
	if err != nil {
		return nil, fmt.Errorf("live ingestions: %w", err)
	}
	collected, err := pgx.CollectRows(rows, pgx.RowToStructByName[listRow])
	if err != nil {
		return nil, fmt.Errorf("live ingestions: %w", err)
	}
	out := make([]ListItem, 0, len(collected))
	for _, row := range collected {
		ing, err := rowToIngestion(row.IngIngestion)
		if err != nil {
			continue // logged by rowToIngestion; one bad row must not stop every runtime
		}
		out = append(out, ListItem{Ingestion: *ing, ApplicationCode: row.ApplicationCode, ClientIdentifier: row.ClientIdentifier})
	}
	return out, nil
}

// FairShare is how many of n polled ingestions of a kind one of the live
// runtimes that carry the kind may hold: an even split, rounded up so that
// none is left without a holder.
func FairShare(polled, runtimes int) int {
	return (polled + max(runtimes, 1) - 1) / max(runtimes, 1)
}
