package ingestion

import (
	"context"
	"encoding/json"
	"fmt"
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
	// Kinds are the kinds the runtime reports it carries (see Carries).
	Kinds []RuntimeKind
	// URL is informational: the platform never calls it.
	URL *string
	// Clients and Applications confine the answer to the caller's reach, nil
	// for an axis it reaches in full (ListReach).
	Clients, Applications *[]string
	Now                   time.Time
	// Lease is how long the leases this poll takes or renews last.
	Lease time.Duration
}

// PollResult is what a runtime should run and the tests it should try.
type PollResult struct {
	// Ingestions are the live, running ingestions of kinds the runtime carries:
	// every shared one, and the polled ones whose lease it now holds.
	Ingestions []ListItem
	Tests      []TestRequest
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
	carried := carriedNames(reported)

	res := &PollResult{}
	err = pgx.BeginFunc(ctx, r.pool, func(tx pgx.Tx) error {
		q := r.q.WithTx(tx)
		if err := q.IngestionRuntimeUpsert(ctx, dbq.IngestionRuntimeUpsertParams{
			RuntimeID: req.RuntimeID, Kinds: kinds, ContractVersion: int32(req.ContractVersion), Url: req.URL, Now: req.Now,
		}); err != nil {
			return fmt.Errorf("upsert runtime: %w", err)
		}
		live, err := liveIngestions(ctx, tx, req.Clients, req.Applications)
		if err != nil {
			return err
		}
		// Fair share: the polled (single-owner) ingestions are spread evenly over
		// the runtimes that are alive, so a second runtime takes half of what the
		// first one holds. A runtime over its share stops listing the surplus and
		// marks those leases released; a lease is never handed over directly, the
		// other runtime claims it once it has expired, by which time the first
		// has stopped it. The share counts every polled ingestion, whichever
		// runtime can run it, as the reference does (the runtimes of a kind a
		// runtime does not carry simply hold what they can).
		polled := 0
		for _, item := range live {
			if item.Definition.Source.SingleOwner() {
				polled++
			}
		}
		runtimes, err := q.IngestionRuntimesLive(ctx, dbq.IngestionRuntimesLiveParams{Since: req.Now.Add(-req.Lease), Until: req.Now})
		if err != nil {
			return fmt.Errorf("count live runtimes: %w", err)
		}
		share := FairShare(polled, int(runtimes))
		mine, err := q.IngestionLeasesHeldBy(ctx, dbq.IngestionLeasesHeldByParams{RuntimeID: req.RuntimeID, Now: req.Now})
		if err != nil {
			return fmt.Errorf("leases held: %w", err)
		}

		held := []string{} // not nil: sqlc sends a nil slice as NULL
		claim := func(id string) (bool, error) {
			n, err := q.IngestionLeaseClaim(ctx, dbq.IngestionLeaseClaimParams{
				IngestionID: id, RuntimeID: req.RuntimeID, Now: req.Now, ExpiresAt: req.Now.Add(req.Lease),
			})
			if err != nil {
				return false, fmt.Errorf("claim lease: %w", err)
			}
			if n == 0 {
				return false, nil // another runtime holds it
			}
			held = append(held, id)
			return true, nil
		}
		// What this runtime already holds is renewed first, up to its share; then
		// it claims unleased ones up to the share.
		var runnable []ListItem
		for _, item := range live {
			if slices.Contains(carried, RunKind(item.Definition.Source)) {
				runnable = append(runnable, item)
			}
		}
		keep := map[string]bool{}
		for _, item := range runnable {
			if item.Definition.Source.SingleOwner() && slices.Contains(mine, item.ID) && len(held) < share {
				if ok, err := claim(item.ID); err != nil {
					return err
				} else if ok {
					keep[item.ID] = true
				}
			}
		}
		for _, item := range runnable {
			if !item.Definition.Source.SingleOwner() || keep[item.ID] || len(held) >= share {
				continue
			}
			if ok, err := claim(item.ID); err != nil {
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

// liveIngestions reads the ingestions runtimes run: active, with a live
// version (idx_ing_ingestions_active), confined to the caller's reach, in id
// order so that runtimes taking leases together take them in the same order.
func liveIngestions(ctx context.Context, tx pgx.Tx, clients, applications *[]string) ([]ListItem, error) {
	var flt repocommon.Filter
	flt.Cond("i.active AND i.version > 0")
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

// FairShare is how many of the polled ingestions one of the live runtimes may
// hold: an even split, rounded up so that none is left without a holder.
func FairShare(polled, runtimes int) int {
	return (polled + max(runtimes, 1) - 1) / max(runtimes, 1)
}
