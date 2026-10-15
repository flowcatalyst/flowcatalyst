package operations

import (
	"context"
	"fmt"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchpool"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/validate"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecaseop"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecasepgx"
)

// SyncDispatchPoolInput is one pool definition in an SDK sync payload.
// RateLimit nil means concurrency-only (no rate limiter).
type SyncDispatchPoolInput struct {
	Code        string
	Name        string
	Description *string
	RateLimit   *int32
	Concurrency int32
}

// SyncDispatchPoolsCommand syncs dispatch pools from an application SDK.
// Pools are GLOBAL (matched by code, not application-scoped); ApplicationCode
// is carried for audit/event provenance and ApplicationID is the resolved
// application the sync is scoped to (the use case authorizes against it — the
// caller must be able to act for that application).
type SyncDispatchPoolsCommand struct {
	ApplicationID   string
	ApplicationCode string
	Pools           []SyncDispatchPoolInput
	RemoveUnlisted  bool
}

// SyncDispatchPools bulk-upserts dispatch pools within a single transaction.
// Semantics:
//
//   - Validates each pool code against validate.CodeUnderscorePattern;
//     name required;
//     rateLimit (when set) ≥ 1; concurrency ≥ 1.
//   - Matches existing pools by code over ALL pools (pools are global).
//   - RemoveUnlisted ARCHIVES (soft, not hard-delete) pools absent from the
//     payload that aren't already archived.
//
// Authorization: the coarse "may sync dispatch pools" permission and the app
// resolution (code→id) are the controller's job; the use case enforces the
// per-resource rule — the caller must have access to the target application —
// plus an anchor-only gate on RemoveUnlisted (X-02(d); see the Authorize func).
//
// Ownership (F19). Pools are global rows matched by code, and the built-in
// application-service role holds the sync permission, so each pool records the
// application that owns it (application_id; nil for admin-created and
// pre-ownership pools). An all-applications caller (a platform admin) may sync
// any pool: it creates new pools owned by the target application, hands
// unowned pools it touches to that application, and never takes an owned pool
// from its owner. Any other caller (an application service account, even at
// anchor tier) may only create pools and change pools the target application
// owns; a code that belongs to another application, to a client, or to nobody
// is refused (DISPATCH_POOL_NOT_OWNED), all-or-nothing. RemoveUnlisted
// archives pools absent from the payload: for an all-applications anchor that
// is the platform-wide sweep it always was (X-02(d)); for an anchor-tier
// application-scoped caller it is limited to the pools the target application
// owns; any other caller is refused (ANCHOR_REQUIRED_FOR_PLATFORM_SWEEP).
//
// Emits per-row [DispatchPoolCreated]/[DispatchPoolUpdated]/[DispatchPoolArchived]
// events plus one [DispatchPoolsSynced] rollup, atomic via [usecaseop.Sync].
func SyncDispatchPools(repo *dispatchpool.Repository) usecaseop.Operation[SyncDispatchPoolsCommand, DispatchPoolsSynced] {
	return usecaseop.Operation[SyncDispatchPoolsCommand, DispatchPoolsSynced]{
		Name: "SyncDispatchPools",
		Validate: func(_ context.Context, cmd SyncDispatchPoolsCommand) error {
			if strings.TrimSpace(cmd.ApplicationCode) == "" {
				return usecase.Validation("APPLICATION_CODE_REQUIRED", "Application code is required")
			}
			for _, in := range cmd.Pools {
				if strings.TrimSpace(in.Code) == "" || !validate.CodeUnderscorePattern.MatchString(in.Code) {
					return usecase.Validation("INVALID_POOL_CODE", fmt.Sprintf(
						"Pool code '%s' is invalid. Must start with lowercase letter, contain only lowercase alphanumeric, hyphens, underscores.", in.Code))
				}
				if strings.TrimSpace(in.Name) == "" {
					return usecase.Validation("NAME_REQUIRED", "Pool name is required")
				}
				if in.RateLimit != nil && *in.RateLimit < 1 {
					return usecase.Validation("INVALID_RATE_LIMIT", "Rate limit, when set, must be at least 1")
				}
				if in.Concurrency < 1 {
					return usecase.Validation("INVALID_CONCURRENCY", "Concurrency must be at least 1")
				}
			}
			return nil
		},
		Authorize: func(ctx context.Context, cmd SyncDispatchPoolsCommand) error {
			ac := auth.FromContext(ctx)
			if !ac.CanAccessApplication(cmd.ApplicationID) {
				return httperror.Forbidden("Not authorised for application '" + cmd.ApplicationCode + "'")
			}
			// X-02(d): a RemoveUnlisted sweep is anchor-tier only. What it may
			// archive is narrowed in Execute by pool ownership (F19).
			if cmd.RemoveUnlisted && !ac.IsAnchor() && !ac.IsSuperAdmin() {
				return usecase.Authorization("ANCHOR_REQUIRED_FOR_PLATFORM_SWEEP",
					"Only anchor users may sweep (removeUnlisted) dispatch pools — they are platform-global")
			}
			return nil
		},
		Execute: func(ctx context.Context, cmd SyncDispatchPoolsCommand, ec usecase.ExecutionContext) (usecaseop.Plan[DispatchPoolsSynced], error) {
			existing, err := repo.FindWithFilters(ctx, nil, nil) // all pools (global)
			if err != nil {
				return nil, usecase.Internal("REPO", "find_all failed", err)
			}
			// The sync's pools are the client-less ones: (code, client_id) is
			// unique, so a client's pool may share a code with a platform one
			// and must never be mistaken for it.
			existingByCode := make(map[string]*dispatchpool.DispatchPool, len(existing))
			for i := range existing {
				if existing[i].ClientID == nil {
					existingByCode[existing[i].Code] = &existing[i]
				}
			}
			// An all-applications caller (a platform admin) may change any
			// pool; every other caller only the pools the application owns.
			allApps := auth.FromContext(ctx).AllApplications
			owns := func(p *dispatchpool.DispatchPool) bool {
				return p.ApplicationID != nil && *p.ApplicationID == cmd.ApplicationID
			}
			// A code held by a client's pool cannot be created or synced here.
			clientCodes := map[string]struct{}{}
			for i := range existing {
				if existing[i].ClientID != nil {
					clientCodes[existing[i].Code] = struct{}{}
				}
			}

			var (
				saves       []usecasepgx.SyncSaveItem[dispatchpool.DispatchPool]
				syncedCodes = make([]string, 0, len(cmd.Pools))
				syncedSet   = make(map[string]struct{}, len(cmd.Pools))
				created     uint32
				updated     uint32
				deleted     uint32
			)

			for _, in := range cmd.Pools {
				syncedCodes = append(syncedCodes, in.Code)
				syncedSet[in.Code] = struct{}{}

				if cur, ok := existingByCode[in.Code]; ok {
					if !allApps && !owns(cur) {
						return nil, usecase.Authorization("DISPATCH_POOL_NOT_OWNED", fmt.Sprintf(
							"Dispatch pool '%s' is not owned by application '%s' (it belongs to another application or the platform); only a platform administrator may sync it",
							in.Code, cmd.ApplicationCode))
					}
					if cur.ApplicationID == nil {
						appID := cmd.ApplicationID
						cur.ApplicationID = &appID
					}
					cur.Name = in.Name
					cur.Description = in.Description
					cur.RateLimit = in.RateLimit
					cur.Concurrency = in.Concurrency
					saves = append(saves, usecasepgx.SyncSaveItem[dispatchpool.DispatchPool]{
						Aggregate: cur,
						Event: DispatchPoolUpdated{
							Metadata: usecase.NewEventMetadata(ec, DispatchPoolUpdatedType, Source, subjectFor(cur.ID)),
							PoolID:   cur.ID,
							Name:     cur.Name,
						},
					})
					updated++
					continue
				}

				if _, taken := clientCodes[in.Code]; taken && !allApps {
					return nil, usecase.Authorization("DISPATCH_POOL_NOT_OWNED", fmt.Sprintf(
						"Dispatch pool code '%s' is not available to application '%s'", in.Code, cmd.ApplicationCode))
				}
				p := dispatchpool.New(in.Code, in.Name)
				appID := cmd.ApplicationID
				p.ApplicationID = &appID
				p.Description = in.Description
				p.RateLimit = in.RateLimit
				p.Concurrency = in.Concurrency
				saves = append(saves, usecasepgx.SyncSaveItem[dispatchpool.DispatchPool]{
					Aggregate: p,
					Event: DispatchPoolCreated{
						Metadata: usecase.NewEventMetadata(ec, DispatchPoolCreatedType, Source, subjectFor(p.ID)),
						PoolID:   p.ID,
						Code:     p.Code,
						Name:     p.Name,
					},
				})
				created++
			}

			if cmd.RemoveUnlisted {
				for i := range existing {
					cur := &existing[i]
					if _, present := syncedSet[cur.Code]; present {
						continue
					}
					if cur.Status == dispatchpool.StatusArchived {
						continue
					}
					if !allApps && !owns(cur) {
						continue // an application sweeps only its own pools
					}
					cur.Archive()
					saves = append(saves, usecasepgx.SyncSaveItem[dispatchpool.DispatchPool]{
						Aggregate: cur,
						Event: DispatchPoolArchived{
							Metadata: usecase.NewEventMetadata(ec, DispatchPoolArchivedType, Source, subjectFor(cur.ID)),
							PoolID:   cur.ID,
							Code:     cur.Code,
						},
					})
					deleted++
				}
			}

			rollup := DispatchPoolsSynced{
				Metadata:        usecase.NewEventMetadata(ec, DispatchPoolsSyncedType, Source, "platform.dispatchpools."+cmd.ApplicationCode),
				ApplicationCode: cmd.ApplicationCode,
				Created:         created,
				Updated:         updated,
				Deleted:         deleted,
				SyncedCodes:     syncedCodes,
			}
			return usecaseop.Sync(repo, saves, nil, rollup), nil
		},
	}
}
