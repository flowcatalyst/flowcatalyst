// wiring.go materialises the platform objects a function's `live` version
// owns (docs/function-runner-plan.md §8.5, work package 8): one dispatch
// pool, one subscription per describe `subscriptions[]` entry, and one
// scheduled job per describe `schedules[]` entry. Called from PutAlias (when
// `live` moves) and DeleteAlias (when `live` is removed), always inside the
// caller's already-open transaction (alias.go), so a promote's wiring change
// is atomic with the pointer move that triggered it.
//
// Identity (plan §8.5 says "decide and document" — spec silence):
//   - subscription: (function_id, eventType, path). msg_subscriptions.code
//     (unique per application/client) is a deterministic digest of
//     (function address, eventType, path) — see subscriptionCode — so the
//     SAME describe entry always reconciles to the SAME row, and a changed
//     eventType or path is a different row (old deleted, new created), never
//     an in-place identity change.
//   - scheduled job: (function_id, path, cron). Each describe `schedules[]`
//     entry is exactly one cron expression (unlike the admin ScheduledJob
//     aggregate's Crons list, which can hold several), so `code` is a digest
//     of (function address, path, cron) — see scheduledJobCode.
//
// Reconcile is create-what's-new / update-what-changed (re-persisted
// unconditionally — simpler than a field-by-field diff, and cheap) /
// delete-what's-no-longer-listed, scoped to rows already owned by this
// function (FindByFunctionID) so it can never touch another function's, or a
// UI/API/CODE-authored, row. Every write still goes through the sibling
// modules' own repositories + event types via usecasepgx.CommitScoped /
// CommitDeleteScoped, so each reconciled row gets its own domain event +
// audit row on this same transaction, same as if a human had used the
// ordinary subscription/scheduled-job APIs.
package operations

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/common"
	"github.com/flowcatalyst/flowcatalyst-go/internal/functions/abi"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ids"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchpool"
	dispatchpoolops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchpool/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/function"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/scheduledjob"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/serviceaccount"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/subscription"
	subscriptionops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/subscription/operations"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

// WiringDeps bundles the repositories + runner URL template the promote
// reconciliation composes. Built once at wire time
// (internal/server/wire_routes.go) and captured by PutAlias/DeleteAlias.
type WiringDeps struct {
	Subscriptions   *subscription.Repository
	ScheduledJobs   *scheduledjob.Repository
	DispatchPools   *dispatchpool.Repository
	ServiceAccounts *serviceaccount.Repository
	// RunnerURLTemplate is FC_FUNCTIONS_RUNNER_URL: a template containing
	// "{pool}", e.g. "http://127.0.0.1:8095". Plan §8.5: the full delivery
	// target is "<runner URL with {pool} substituted>/fn/<address><path>".
	RunnerURLTemplate string
}

// wiringSummary is folded into [FunctionPromoted]'s payload.
type wiringSummary struct {
	DispatchPoolCode string

	SubscriptionsCreated int
	SubscriptionsUpdated int
	SubscriptionsDeleted int

	SchedulesCreated int
	SchedulesUpdated int
	SchedulesDeleted int
}

// wiringCommand is the audit-log "command" recorded against every row this
// reconcile writes. There's no single wire-level command for a reconciled
// row (the write is a side effect of PutAlias/DeleteAlias, which already
// records its OWN command against the alias row), so this small value names
// what actually drove the write.
type wiringCommand struct {
	FunctionID string `json:"functionId"`
	Reason     string `json:"reason"`
}

// reconcileWiring materialises the platform objects fn owns to match d, or
// — d.Subscriptions and d.Schedules both empty (the zero abi.Describe{}, or
// any describe that happens to declare neither) — removes every
// function-owned subscription and scheduled job. The dispatch pool is
// created once there is at least one subscription or schedule to route
// through it, and is never deleted here: plan §8.5, "deleted only when the
// function is deleted" (see DeleteFunction's cascade in delete.go).
func reconcileWiring(
	ctx context.Context,
	s *usecasepgx.TxScopedUnitOfWork,
	deps WiringDeps,
	f *function.Function,
	d *abi.Describe,
	ec usecase.ExecutionContext,
) (wiringSummary, error) {
	if d == nil {
		d = &abi.Describe{}
	}
	var sum wiringSummary
	poolCode := f.DispatchPoolCode()
	cmd := wiringCommand{FunctionID: f.ID, Reason: "promote wiring reconcile"}

	needsPool := len(d.Subscriptions) > 0 || len(d.Schedules) > 0

	// A function's webhook endpoints can never verify an unsigned delivery
	// (plan §8.5): resolve the application's signing account BEFORE any
	// write, so a missing account fails the whole promote atomically — the
	// transaction rolls back, nothing partially wires. This is the same
	// lookup serviceaccount.NewCachedOutboundCredsResolver makes
	// (FindFirstByApplicationID), so a subscription's explicit
	// ServiceAccountID (set below) resolves to the identical account a
	// cached resolver keyed on the function's application would — see
	// internal/server/delivery_creds.go's step 1 ("the subscription's own
	// serviceAccountId").
	var signingAccountID *string
	if needsPool {
		sa, err := deps.ServiceAccounts.FindFirstByApplicationID(ctx, f.ApplicationID)
		if err != nil {
			return sum, usecase.Internal("REPO", "find_first_by_application_id(service account) failed", err)
		}
		if sa == nil {
			return sum, usecase.BusinessRule("NO_SIGNING_ACCOUNT",
				"application "+f.ApplicationCode+" has no active service account; a function's webhook endpoints can never verify an unsigned delivery")
		}
		id := sa.ID
		signingAccountID = &id
	}

	var poolID string
	if needsPool {
		id, err := ensureDispatchPool(ctx, s, deps.DispatchPools, f, poolCode, ec, cmd)
		if err != nil {
			return sum, err
		}
		poolID = id
		sum.DispatchPoolCode = poolCode
	}

	created, updated, deleted, err := reconcileSubscriptions(ctx, s, deps, f, d, dispatchPoolRef{id: poolID, code: poolCode}, signingAccountID, ec, cmd)
	if err != nil {
		return sum, err
	}
	sum.SubscriptionsCreated, sum.SubscriptionsUpdated, sum.SubscriptionsDeleted = created, updated, deleted

	created, updated, deleted, err = reconcileSchedules(ctx, s, deps, f, d, poolCode, ec, cmd)
	if err != nil {
		return sum, err
	}
	sum.SchedulesCreated, sum.SchedulesUpdated, sum.SchedulesDeleted = created, updated, deleted

	return sum, nil
}

// targetURL builds the runner delivery URL for one describe entry's path.
func targetURL(template, pool, address, path string) string {
	base := strings.TrimSuffix(strings.ReplaceAll(template, "{pool}", pool), "/")
	return base + "/fn/" + address + path
}

// ensureDispatchPool creates fn's dispatch pool (fn.DispatchPoolCode) if
// it doesn't already exist by code, scoped to the function's client. Never
// mutates an existing pool's settings — an operator's rate-limit/concurrency
// tuning on a function's pool survives every promote. Reuses dispatchpool's
// own CreateDispatchPool event type for consistency with pools created
// through the ordinary API.
func ensureDispatchPool(ctx context.Context, s *usecasepgx.TxScopedUnitOfWork, repo *dispatchpool.Repository, f *function.Function, code string, ec usecase.ExecutionContext, cmd any) (string, error) {
	existing, err := repo.FindByCode(ctx, code, f.ClientID)
	if err != nil {
		return "", usecase.Internal("REPO", "find_by_code(dispatch pool) failed", err)
	}
	if existing != nil {
		return existing.ID, nil
	}
	p := dispatchpool.New(code, "Function: "+f.Address)
	p.ClientID = ids.PtrOf[ids.ClientID](f.ClientID)
	desc := "Owned by function " + f.Address + " (fn_id=" + f.ID + "); deleted only when the function is deleted."
	p.Description = &desc
	event := dispatchpoolops.DispatchPoolCreated{
		Metadata: usecase.NewEventMetadata(ec, dispatchpoolops.DispatchPoolCreatedType, dispatchpoolops.Source, "platform.dispatchpool."+p.ID),
		PoolID:   p.ID,
		Code:     p.Code,
		Name:     p.Name,
	}
	if r := usecasepgx.CommitScoped(ctx, s, p, repo, event, cmd); !usecase.IsSuccess(r) {
		_, e := usecase.Into(r)
		return "", e
	}
	return p.ID, nil
}

// dispatchPoolRef names the function's dispatch pool both ways: dispatch
// jobs are created with the subscription's pool ID (the code alone left
// every job without a pool), and the code is what operators see.
type dispatchPoolRef struct{ id, code string }

// ── Subscriptions ───────────────────────────────────────────────────────

// subscriptionCode derives a deterministic, short (well under the code
// column's VARCHAR(100)) identity code for one describe subscriptions[]
// entry — see the package doc comment's identity rule.
func subscriptionCode(address, eventType, path string) string {
	return stableCode("fn-sub-", address, eventType, path)
}

func reconcileSubscriptions(
	ctx context.Context,
	s *usecasepgx.TxScopedUnitOfWork,
	deps WiringDeps,
	f *function.Function,
	d *abi.Describe,
	pool dispatchPoolRef,
	signingAccountID *string,
	ec usecase.ExecutionContext,
	cmd any,
) (created, updated, deleted int, err error) {
	current, err := deps.Subscriptions.FindByFunctionID(ctx, f.ID)
	if err != nil {
		return 0, 0, 0, usecase.Internal("REPO", "find_by_function_id(subscription) failed", err)
	}
	currentByCode := make(map[string]*subscription.Subscription, len(current))
	for i := range current {
		currentByCode[current[i].Code] = &current[i]
	}

	want := make(map[string]struct{}, len(d.Subscriptions))
	for _, sub := range d.Subscriptions {
		code := subscriptionCode(f.Address, sub.EventType, sub.Path)
		want[code] = struct{}{}
		endpoint := targetURL(deps.RunnerURLTemplate, f.RunnerPool(), f.Address, sub.Path)

		if cur, ok := currentByCode[code]; ok {
			if !applyDesiredSubscription(cur, sub, pool, endpoint, signingAccountID) {
				updated++ // still counted as reconciled, even when nothing changed
				continue
			}
			event := subscriptionops.SubscriptionUpdated{
				Metadata:       usecase.NewEventMetadata(ec, subscriptionops.SubscriptionUpdatedType, subscriptionops.Source, "platform.subscription."+cur.ID),
				SubscriptionID: cur.ID,
				Name:           cur.Name,
			}
			if r := usecasepgx.CommitScoped(ctx, s, cur, deps.Subscriptions, event, cmd); !usecase.IsSuccess(r) {
				_, e := usecase.Into(r)
				return 0, 0, 0, e
			}
			updated++
			continue
		}

		ns := subscription.New(code, "Function: "+f.Address+sub.Path, endpoint)
		appCode := f.ApplicationCode
		ns.ApplicationCode = &appCode
		ns.ClientID = ids.PtrOf[ids.ClientID](f.ClientID)
		ns.ClientScoped = f.ClientID != nil
		ns.Source = subscription.SourceFunction
		ns.FunctionID = &f.ID
		ns.EventTypes = []subscription.EventTypeBinding{subscription.NewEventTypeBinding(sub.EventType)}
		applyDesiredSubscription(ns, sub, pool, endpoint, signingAccountID)
		pid := ec.PrincipalID
		ns.CreatedBy = &pid

		event := subscriptionops.SubscriptionCreated{
			Metadata:       usecase.NewEventMetadata(ec, subscriptionops.SubscriptionCreatedType, subscriptionops.Source, "platform.subscription."+ns.ID),
			SubscriptionID: ns.ID,
			Code:           ns.Code,
			Name:           ns.Name,
		}
		if r := usecasepgx.CommitScoped(ctx, s, ns, deps.Subscriptions, event, cmd); !usecase.IsSuccess(r) {
			_, e := usecase.Into(r)
			return 0, 0, 0, e
		}
		created++
	}

	for code, cur := range currentByCode {
		if _, ok := want[code]; ok {
			continue
		}
		event := subscriptionops.SubscriptionDeleted{
			Metadata:       usecase.NewEventMetadata(ec, subscriptionops.SubscriptionDeletedType, subscriptionops.Source, "platform.subscription."+cur.ID),
			SubscriptionID: cur.ID,
			Code:           cur.Code,
		}
		if r := usecasepgx.CommitDeleteScoped(ctx, s, cur, deps.Subscriptions, event, cmd); !usecase.IsSuccess(r) {
			_, e := usecase.Into(r)
			return 0, 0, 0, e
		}
		deleted++
	}

	return created, updated, deleted, nil
}

// applyDesiredSubscription mutates cur to match sub's declared fields,
// returning whether anything changed. An omitted mode maps to
// common.DefaultDispatchMode (NEXT_ON_ERROR) — never IMMEDIATE (plan §5.4:
// "A default must not quietly weaken ordering").
func applyDesiredSubscription(cur *subscription.Subscription, sub abi.Subscription, pool dispatchPoolRef, endpoint string, signingAccountID *string) bool {
	changed := false
	if cur.Endpoint != endpoint {
		cur.Endpoint = endpoint
		changed = true
	}
	mode := common.DefaultDispatchMode
	if sub.Mode != "" {
		mode = common.DispatchMode(sub.Mode)
	}
	if cur.Mode != mode {
		cur.Mode = mode
		changed = true
	}
	maxRetries := int32(3)
	if sub.MaxRetries != nil {
		maxRetries = int32(*sub.MaxRetries)
	}
	if cur.MaxRetries != maxRetries {
		cur.MaxRetries = maxRetries
		changed = true
	}
	timeoutSeconds := int32(30)
	if sub.TimeoutSeconds != nil {
		timeoutSeconds = int32(*sub.TimeoutSeconds)
	}
	if cur.TimeoutSeconds != timeoutSeconds {
		cur.TimeoutSeconds = timeoutSeconds
		changed = true
	}
	if cur.DataOnly != sub.DataOnly {
		cur.DataOnly = sub.DataOnly
		changed = true
	}
	if cur.DispatchPoolCode == nil || *cur.DispatchPoolCode != pool.code {
		pc := pool.code
		cur.DispatchPoolCode = &pc
		changed = true
	}
	if pool.id != "" && (cur.DispatchPoolID == nil || *cur.DispatchPoolID != pool.id) {
		pid := pool.id
		cur.DispatchPoolID = &pid
		changed = true
	}
	if !ptrStrEqual(cur.ServiceAccountID, signingAccountID) {
		cur.ServiceAccountID = signingAccountID
		changed = true
	}
	return changed
}

func ptrStrEqual(a, b *string) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

// ── Scheduled jobs ──────────────────────────────────────────────────────

// scheduledJobCode derives a deterministic identity code for one describe
// schedules[] entry (function address, path, cron) — see the package doc
// comment's identity rule. A cron change on the same path is therefore a
// delete+create (a different identity), not an in-place update: bookkeeping
// like last_fired_at for a fundamentally different firing pattern should not
// carry over silently.
func scheduledJobCode(address, path, cron string) string {
	return stableCode("fn-job-", address, path, cron)
}

func reconcileSchedules(
	ctx context.Context,
	s *usecasepgx.TxScopedUnitOfWork,
	deps WiringDeps,
	f *function.Function,
	d *abi.Describe,
	poolCode string,
	ec usecase.ExecutionContext,
	cmd any,
) (created, updated, deleted int, err error) {
	current, err := deps.ScheduledJobs.FindByFunctionID(ctx, f.ID)
	if err != nil {
		return 0, 0, 0, usecase.Internal("REPO", "find_by_function_id(scheduled job) failed", err)
	}
	currentByCode := make(map[string]*scheduledjob.ScheduledJob, len(current))
	for i := range current {
		currentByCode[current[i].Code] = &current[i]
	}

	want := make(map[string]struct{}, len(d.Schedules))
	for _, sc := range d.Schedules {
		code := scheduledJobCode(f.Address, sc.Path, sc.Cron)
		want[code] = struct{}{}
		tURL := targetURL(deps.RunnerURLTemplate, f.RunnerPool(), f.Address, sc.Path)
		timezone := sc.Timezone
		if timezone == "" {
			timezone = "UTC"
		}

		if cur, ok := currentByCode[code]; ok {
			changed := false
			if cur.TargetURL == nil || *cur.TargetURL != tURL {
				u := tURL
				cur.TargetURL = &u
				changed = true
			}
			if cur.Timezone != timezone {
				cur.Timezone = timezone
				changed = true
			}
			if string(cur.Payload) != string(sc.Payload) {
				cur.Payload = sc.Payload
				changed = true
			}
			if cur.ApplicationID == nil || *cur.ApplicationID != f.ApplicationID {
				aid := f.ApplicationID
				cur.ApplicationID = &aid
				changed = true
			}
			if !ptrStrEqual(ids.StringPtr(cur.ClientID), f.ClientID) {
				cur.ClientID = ids.PtrOf[ids.ClientID](f.ClientID)
				changed = true
			}
			if cur.Status != scheduledjob.StatusActive {
				cur.Status = scheduledjob.StatusActive
				changed = true
			}
			if !changed {
				updated++
				continue
			}
			cur.Version++
			event := FunctionScheduleWired{
				Metadata:       usecase.NewEventMetadata(ec, FunctionScheduleWiredType, Source, subjectFor(f.ID)),
				FunctionID:     f.ID,
				ScheduledJobID: cur.ID,
				Code:           cur.Code,
				Action:         "updated",
			}
			if r := usecasepgx.CommitScoped(ctx, s, cur, deps.ScheduledJobs, event, cmd); !usecase.IsSuccess(r) {
				_, e := usecase.Into(r)
				return 0, 0, 0, e
			}
			updated++
			continue
		}

		nj := scheduledjob.New(code, "Function: "+f.Address+sc.Path, []string{sc.Cron})
		aid := f.ApplicationID
		nj.ApplicationID = &aid
		nj.ClientID = ids.PtrOf[ids.ClientID](f.ClientID)
		nj.FunctionID = &f.ID
		nj.Timezone = timezone
		nj.Payload = sc.Payload
		u := tURL
		nj.TargetURL = &u
		pid := ec.PrincipalID
		nj.CreatedBy = &pid

		event := FunctionScheduleWired{
			Metadata:       usecase.NewEventMetadata(ec, FunctionScheduleWiredType, Source, subjectFor(f.ID)),
			FunctionID:     f.ID,
			ScheduledJobID: nj.ID,
			Code:           nj.Code,
			Action:         "created",
		}
		if r := usecasepgx.CommitScoped(ctx, s, nj, deps.ScheduledJobs, event, cmd); !usecase.IsSuccess(r) {
			_, e := usecase.Into(r)
			return 0, 0, 0, e
		}
		created++
	}

	for code, cur := range currentByCode {
		if _, ok := want[code]; ok {
			continue
		}
		event := FunctionScheduleWired{
			Metadata:       usecase.NewEventMetadata(ec, FunctionScheduleWiredType, Source, subjectFor(f.ID)),
			FunctionID:     f.ID,
			ScheduledJobID: cur.ID,
			Code:           cur.Code,
			Action:         "deleted",
		}
		if r := usecasepgx.CommitDeleteScoped(ctx, s, cur, deps.ScheduledJobs, event, cmd); !usecase.IsSuccess(r) {
			_, e := usecase.Into(r)
			return 0, 0, 0, e
		}
		deleted++
	}

	return created, updated, deleted, nil
}

// unitSeparator delimits stableCode's parts so that e.g. ("ab", "c") and
// ("a", "bc") never collide.
const unitSeparator = "\x1f"

func stableCode(prefix string, parts ...string) string {
	h := sha256.Sum256([]byte(strings.Join(parts, unitSeparator)))
	return prefix + hex.EncodeToString(h[:])[:20]
}
