package ingestion

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/flowcatalyst/flowcatalyst-go/internal/envutil"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/repocommon"
	"github.com/flowcatalyst/flowcatalyst-go/internal/sqlc/dbq"
	"github.com/flowcatalyst/go-sdk/usecase"
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

// MinLease is the shortest lease the platform starts with: a runtime polls
// every minute and stops a single-owner route before its lease would end, so
// a shorter lease leaves it no room for one missed or slow poll.
// FC_INGESTION_LEASE_SECONDS below it is a startup configuration error.
const MinLease = 2 * time.Minute

// Placement defaults (docs/ingestion-design.md section 7).
const (
	// DefaultUnhealthyCooldown is how long a FAILED or FAILING route state
	// keeps an ingestion away from the runtime that reported it, while another
	// live runtime has not failed it (FC_INGESTION_UNHEALTHY_COOLDOWN).
	DefaultUnhealthyCooldown = 10 * time.Minute
	// DefaultRecentWindow is how far back a runtime returning after a gap
	// looks for the fleet it is part of: its new claims of a poll are capped
	// at its share over the runtimes seen that recently
	// (FC_INGESTION_RECENT_WINDOW).
	DefaultRecentWindow = 10 * time.Minute
)

// UnhealthyCooldown is FC_INGESTION_UNHEALTHY_COOLDOWN (a Go duration such as
// 10m), or DefaultUnhealthyCooldown when unset, unparseable or not positive.
func UnhealthyCooldown() time.Duration {
	return envDuration("FC_INGESTION_UNHEALTHY_COOLDOWN", DefaultUnhealthyCooldown)
}

// RecentWindow is FC_INGESTION_RECENT_WINDOW (a Go duration such as 10m), or
// DefaultRecentWindow when unset, unparseable or not positive.
func RecentWindow() time.Duration {
	return envDuration("FC_INGESTION_RECENT_WINDOW", DefaultRecentWindow)
}

func envDuration(name string, def time.Duration) time.Duration {
	if d, err := time.ParseDuration(envutil.Or(name, "")); err == nil && d > 0 {
		return d
	}
	return def
}

// runtimeInUse is the 409 for a poll or release from a process other than the
// live one that last polled as the runtime id.
func runtimeInUse(runtimeID string) error {
	return usecase.Conflict("RUNTIME_ID_IN_USE",
		fmt.Sprintf("runtime id %s is in use by another live process", runtimeID))
}

// liveAt reports whether a runtime that last polled at lastSeen, and said
// goodbye at left (nil: never), is live at now: it polled within one lease and
// has not left since that poll.
func liveAt(lastSeen time.Time, left *time.Time, now time.Time, lease time.Duration) bool {
	return lastSeen.After(now.Add(-lease)) && (left == nil || left.Before(lastSeen))
}

// previousPoll reads (and locks) what the runtime's row said before this call
// writes it; nil when the runtime has never polled (or was pruned).
func previousPoll(ctx context.Context, q *dbq.Queries, runtimeID string) (*dbq.IngestionRuntimeFindForUpdateRow, error) {
	row, err := q.IngestionRuntimeFindForUpdate(ctx, runtimeID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("runtime row: %w", err)
	}
	return &row, nil
}

// heldByAnother reports whether instance may not act as the runtime: another
// instance polled as it within one lease and has not left since.
func heldByAnother(prev *dbq.IngestionRuntimeFindForUpdateRow, instance string, now time.Time, lease time.Duration) bool {
	return prev != nil && prev.InstanceID != nil && *prev.InstanceID != instance &&
		liveAt(prev.LastSeenAt, prev.LeftAt, now, lease)
}

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
	// InstanceID is the process the poll comes from (the contract's optional
	// instanceId, new on every start). While another instance that polled as
	// this runtime is live the poll is refused (RUNTIME_ID_IN_USE). Nil: no
	// check, and none is stored.
	InstanceID *string
	// UnhealthyCooldown and RecentWindow are the placement windows; zero reads
	// FC_INGESTION_UNHEALTHY_COOLDOWN and FC_INGESTION_RECENT_WINDOW.
	UnhealthyCooldown, RecentWindow time.Duration
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
//
// A poll from a process other than the live one that last polled as the
// runtime id is refused (RUNTIME_ID_IN_USE) and changes nothing. A runtime
// does not keep or take an ingestion it recently failed while another live
// runtime that carries the kind has not (health-aware placement), and one
// returning after a gap takes at most its share over the recently seen fleet
// in new claims (docs/ingestion-design.md section 7).
func (r *Repository) Poll(ctx context.Context, req PollRequest) (*PollResult, error) {
	if req.UnhealthyCooldown <= 0 {
		req.UnhealthyCooldown = UnhealthyCooldown()
	}
	if req.RecentWindow <= 0 {
		req.RecentWindow = RecentWindow()
	}
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
		// The instance guard comes before anything is written: a refused poll
		// does not count as the runtime being seen, and takes nothing.
		prev, err := previousPoll(ctx, q, req.RuntimeID)
		if err != nil {
			return err
		}
		if req.InstanceID != nil && heldByAnother(prev, *req.InstanceID, now, req.Lease) {
			return runtimeInUse(req.RuntimeID)
		}
		// A runtime that was not live before this poll (new, silent for a
		// lease, or gone with a goodbye) returns after a gap.
		returning := prev == nil || !liveAt(prev.LastSeenAt, prev.LeftAt, now, req.Lease)
		if err := q.IngestionRuntimeUpsert(ctx, dbq.IngestionRuntimeUpsertParams{
			RuntimeID: req.RuntimeID, Kinds: kinds, BuiltinKinds: builtin, ContractVersion: int32(req.ContractVersion),
			Url: req.URL, InstanceID: req.InstanceID, Now: now,
		}); err != nil {
			return fmt.Errorf("upsert runtime: %w", err)
		}
		if err := rememberRequirements(ctx, q, req.RuntimeID, reported, now); err != nil {
			return err
		}
		live, err := liveIngestions(ctx, tx, req.Clients, req.Applications, nil)
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
		polled := polledKinds(live)
		liveCarriers, err := carriersSeen(ctx, q, now.Add(-req.Lease), now)
		if err != nil {
			return err
		}
		shares := fairShares(polled, liveCarriers)
		// A runtime returning after a gap claims at most its share over the
		// runtimes seen recently: after a platform outage longer than a lease
		// every runtime returns at once, and the first to poll would otherwise
		// count only itself as live and claim everything.
		var claimCap map[string]int
		if returning {
			recent, err := carriersSeen(ctx, q, now.Add(-req.RecentWindow), now)
			if err != nil {
				return err
			}
			claimCap = make(map[string]int, len(polled))
			for kind, n := range polled {
				claimCap[kind] = min(FairShare(n, len(recent[kind])), shares[kind])
			}
		}
		avoid, err := unhealthyHere(ctx, q, req.RuntimeID, now.Add(-req.UnhealthyCooldown), live, liveCarriers)
		if err != nil {
			return err
		}
		// Ingestions another runtime recently failed and this one has not: it
		// may hold them beyond its share, so one avoided there is never left
		// unheld because every healthy runtime is already at its share. They
		// count toward the share, so the next renewal sheds a healthy one
		// instead, which the avoiding runtime can take: the fleet stays even.
		rescue, err := failingElsewhere(ctx, q, req.RuntimeID, now.Add(-req.UnhealthyCooldown))
		if err != nil {
			return err
		}
		mine, err := q.IngestionLeasesHeldBy(ctx, dbq.IngestionLeasesHeldByParams{RuntimeID: req.RuntimeID, Now: now})
		if err != nil {
			return fmt.Errorf("leases held: %w", err)
		}

		held := []string{} // not nil: sqlc sends a nil slice as NULL
		heldOf, claimedOf := map[string]int{}, map[string]int{}
		claim := func(item ListItem, renewal bool) (bool, error) {
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
			if !renewal {
				claimedOf[RunKind(item.Definition.Source)]++
			}
			return true, nil
		}
		var runnable []ListItem
		for _, item := range live {
			if slices.Contains(carried, RunKind(item.Definition.Source)) {
				runnable = append(runnable, item)
			}
		}
		// What this runtime already holds is renewed first: a rescued one
		// (failing elsewhere) whatever the share, then the rest up to its share
		// of the kind; then it claims unleased ones of the kind up to that share
		// (and, returning after a gap, up to its cap), or beyond it for a rescued
		// one. An ingestion it should avoid is neither renewed nor claimed: its
		// lease is released below, as any surplus is.
		keep := map[string]bool{}
		under := func(item ListItem) bool {
			kind := RunKind(item.Definition.Source)
			return heldOf[kind] < shares[kind]
		}
		capped := func(item ListItem) bool {
			kind := RunKind(item.Definition.Source)
			return claimCap != nil && claimedOf[kind] >= claimCap[kind]
		}
		for _, item := range runnable {
			if item.Definition.Source.SingleOwner() && slices.Contains(mine, item.ID) && !avoid[item.ID] && rescue[item.ID] {
				if ok, err := claim(item, true); err != nil {
					return err
				} else if ok {
					keep[item.ID] = true
				}
			}
		}
		for _, item := range runnable {
			if item.Definition.Source.SingleOwner() && slices.Contains(mine, item.ID) && !keep[item.ID] && !avoid[item.ID] && under(item) {
				if ok, err := claim(item, true); err != nil {
					return err
				} else if ok {
					keep[item.ID] = true
				}
			}
		}
		for _, item := range runnable {
			if !item.Definition.Source.SingleOwner() || keep[item.ID] || avoid[item.ID] || (!under(item) && !rescue[item.ID]) || capped(item) {
				continue
			}
			if ok, err := claim(item, false); err != nil {
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

// polledKinds counts the live polled (single-owner) ingestions of each kind.
func polledKinds(live []ListItem) map[string]int {
	polled := map[string]int{}
	for _, item := range live {
		if item.Definition.Source.SingleOwner() {
			polled[RunKind(item.Definition.Source)]++
		}
	}
	return polled
}

// carriersSeen lists, for each kind, the runtimes that polled after since (and
// not after until), have not left since, and carry the kind. Over one lease
// window these are the live runtimes, the asker included (it has just been
// upserted). A runtime whose kinds row does not parse is left out (logged): it
// cannot run anything until it polls again anyway.
func carriersSeen(ctx context.Context, q *dbq.Queries, since, until time.Time) (map[string][]string, error) {
	runtimes, err := q.IngestionRuntimesLiveKinds(ctx, dbq.IngestionRuntimesLiveKindsParams{Since: since, Until: until})
	if err != nil {
		return nil, fmt.Errorf("live runtimes: %w", err)
	}
	carriers := map[string][]string{}
	for _, rt := range runtimes {
		names, err := decodeCarried(rt.Kinds, rt.BuiltinKinds)
		if err != nil {
			slog.Warn("ingestion runtime reported kinds that do not parse", "runtime_id", rt.RuntimeID, "err", err)
			continue
		}
		for _, k := range names {
			carriers[k] = append(carriers[k], rt.RuntimeID)
		}
	}
	return carriers, nil
}

// fairShares is, for each kind of polled (single-owner) ingestion, how many of
// that kind one runtime may hold: the live ingestions of the kind spread
// evenly over the live runtimes that carry it. Counting per kind keeps a mixed
// fleet fair: a runtime that alone carries a kind takes all of it, whatever the
// other runtimes hold of other kinds, and two runtimes that both carry a kind
// split it.
func fairShares(polled map[string]int, liveCarriers map[string][]string) map[string]int {
	shares := make(map[string]int, len(polled))
	for kind, n := range polled {
		shares[kind] = FairShare(n, len(liveCarriers[kind]))
	}
	return shares
}

// unhealthyHere is the polled ingestions the runtime should neither renew nor
// claim: those it reported FAILED or FAILING after since, while another live
// runtime that carries the kind has not. An ingestion every live carrier has
// failed recently stays where it is, so a definition broken everywhere stays
// put and visible rather than moving around. A runtime that reports STARTED
// again is not avoided: a route has one state per runtime, the latest.
func unhealthyHere(ctx context.Context, q *dbq.Queries, runtimeID string, since time.Time, live []ListItem, liveCarriers map[string][]string) (map[string]bool, error) {
	rows, err := q.IngestionRouteStatesUnhealthy(ctx, dbq.IngestionRouteStatesUnhealthyParams{Since: since, RuntimeID: runtimeID})
	if err != nil {
		return nil, fmt.Errorf("unhealthy route states: %w", err)
	}
	if len(rows) == 0 {
		return nil, nil
	}
	failedBy := map[string]map[string]bool{} // ingestion -> runtimes that recently failed it
	for _, row := range rows {
		if failedBy[row.IngestionID] == nil {
			failedBy[row.IngestionID] = map[string]bool{}
		}
		failedBy[row.IngestionID][row.RuntimeID] = true
	}
	avoid := map[string]bool{}
	for _, item := range live {
		by, failed := failedBy[item.ID]
		if !failed || !item.Definition.Source.SingleOwner() {
			continue
		}
		for _, other := range liveCarriers[RunKind(item.Definition.Source)] {
			if other != runtimeID && !by[other] {
				avoid[item.ID] = true
				break
			}
		}
	}
	return avoid, nil
}

// failingElsewhere is the ingestions another runtime reported FAILED or
// FAILING after since while this one has not: this runtime may hold them
// beyond its fair share (see Poll).
func failingElsewhere(ctx context.Context, q *dbq.Queries, runtimeID string, since time.Time) (map[string]bool, error) {
	ids, err := q.IngestionRouteStatesFailingElsewhere(ctx, dbq.IngestionRouteStatesFailingElsewhereParams{Since: since, RuntimeID: runtimeID})
	if err != nil {
		return nil, fmt.Errorf("route states failing elsewhere: %w", err)
	}
	out := make(map[string]bool, len(ids))
	for _, id := range ids {
		out[id] = true
	}
	return out, nil
}

// liveIngestions reads the ingestions runtimes run: active, with a live
// version (idx_ing_ingestions_active), of a client that is active (a
// suspended or deactivated client's ingestions run nowhere), confined to the
// caller's reach (and to ids when not nil), in id order.
func liveIngestions(ctx context.Context, tx pgx.Tx, clients, applications, ids *[]string) ([]ListItem, error) {
	var flt repocommon.Filter
	flt.Cond("i.active AND i.version > 0")
	flt.Cond("c.status = 'ACTIVE'")
	if clients != nil {
		flt.Clause("i.client_id = ANY($%d)", *clients)
	}
	if applications != nil {
		flt.Clause("i.application_id = ANY($%d)", *applications)
	}
	if ids != nil {
		flt.Clause("i.id = ANY($%d)", *ids)
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
