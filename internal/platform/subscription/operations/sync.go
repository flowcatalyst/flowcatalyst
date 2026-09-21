package operations

import (
	"context"
	"slices"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/connection"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchpool"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/subscription"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecaseop"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

// SyncEventTypeBindingInput is one event-type binding in a synced subscription.
type SyncEventTypeBindingInput struct {
	EventTypeCode string
	Filter        *string
}

// SyncSubscriptionInput is one subscription definition in an SDK sync payload.
//
// Mode is accepted for wire compatibility but intentionally NOT applied:
// the sync has never set the subscription's dispatch
// mode (synced subscriptions take the entity default), so applying it here
// would diverge the router's per-subscription dispatch behaviour. See the
// create/update branches below.
type SyncSubscriptionInput struct {
	Code         string
	Name         string
	Description  *string
	Target       string
	ConnectionID *string
	// ConnectionCode names the connection by its code instead of its id — an
	// id is minted per environment, so a code-first definition can only ever
	// carry the code. The lookup is EXPLICIT about which namespace the code
	// lives in, with no silent fallback between them (ruling 2026-09-21 #4):
	// by default the code must name a connection OWNED BY THIS APPLICATION;
	// SharedConnection true looks it up among the application-less (shared)
	// connections instead. Within the chosen namespace, a client-scoped sync
	// (Command.ClientID set) prefers that client's own connection, falling
	// back to a global one; a client-less sync may only resolve a global
	// connection.
	ConnectionCode *string
	// SharedConnection resolves ConnectionCode among the shared
	// (application-less) connections rather than this application's own.
	// Requires ConnectionCode (SHARED_CONNECTION_REQUIRES_CODE without one).
	// Without this explicit switch, a bare code would have to guess which
	// namespace it named — and if it silently fell back from one to the
	// other, an application later minting its OWN connection with the same
	// code as a pre-existing shared one would silently switch which
	// credentials sign a subscription's deliveries, with nobody touching the
	// subscription itself.
	SharedConnection bool
	EventTypes       []SyncEventTypeBindingInput
	DispatchPoolCode *string
	Mode             *string
	MaxRetries       *int32
	TimeoutSeconds   *int32
	DataOnly         bool
}

// SyncSubscriptionsCommand syncs one application's API-sourced subscriptions,
// scoped to a single (application, client) pair. ApplicationID is the
// resolved application id the sync is scoped to (the controller resolves it
// from the {appCode}); the use case authorizes against it. ClientID, when
// set, is already resolved to an id by the controller (the wire accepts
// either the client's id or its identifier). ClientID nil scopes the sync to
// the application's global, client-less subscriptions — today's behaviour,
// kept unchanged for callers that never send a client.
type SyncSubscriptionsCommand struct {
	ApplicationID   string
	ApplicationCode string
	ClientID        *string
	Subscriptions   []SyncSubscriptionInput
	RemoveUnlisted  bool
}

// SyncSubscriptions bulk-upserts an application's subscription catalogue,
// scoped to (ApplicationID, ClientID), within a single transaction:
//
//   - Validates app code; each subscription needs code, name, target, and at
//     least one event-type binding.
//   - A connection is named by connectionCode (stable across environments,
//     resolved within an explicit namespace — see [SyncSubscriptionInput])
//     or connectionId; whichever is provided must resolve (404
//     CONNECTION_NOT_FOUND), and if both are, they must agree
//     (CONNECTION_MISMATCH). However it is named, the resolved connection's
//     client must be absent or equal to this sync's ClientID
//     (CONNECTION_SCOPE_MISMATCH) — a global subscription may never point at
//     a client-owned connection.
//   - Matches existing rows by code, scoped to (this application, this
//     client — NULL matches only NULL). Only API- and CODE-sourced rows are
//     updated/removed; UI-authored rows are untouched. New rows are created
//     with source=API.
//   - dispatchPoolCode is resolved to (id, code) via the global pool lookup;
//     an unresolvable code is silently left unset.
//   - maxRetries / timeoutSeconds are only overwritten when present.
//   - RemoveUnlisted hard-deletes API/CODE rows absent from the payload,
//     scoped to the same (application, client) key — never a sibling
//     client's rows, the application's global rows, or another
//     application's rows.
//
// Authorization: the coarse "may sync subscriptions" permission and the app
// resolution (code→id) are the controller's job; the use case enforces
// application access (CanAccessApplication) and, when ClientID is set,
// client access (CanAccessClient) — mirrors connection.SyncConnections; a
// client-less sync needs no anchor tier because ownership, not reach, is the
// fence (see that use case's Authorize for the fuller argument).
//
// Emits per-row [SubscriptionCreated]/[SubscriptionUpdated]/[SubscriptionDeleted]
// events plus one [SubscriptionsSynced] rollup, atomic via [usecaseop.Sync].
func SyncSubscriptions(
	subRepo *subscription.Repository,
	connRepo *connection.Repository,
	poolRepo *dispatchpool.Repository,
) usecaseop.Operation[SyncSubscriptionsCommand, SubscriptionsSynced] {
	return usecaseop.Operation[SyncSubscriptionsCommand, SubscriptionsSynced]{
		Name: "SyncSubscriptions",
		Validate: func(_ context.Context, cmd SyncSubscriptionsCommand) error {
			if strings.TrimSpace(cmd.ApplicationCode) == "" {
				return usecase.Validation("APPLICATION_CODE_REQUIRED", "Application code is required")
			}
			for _, in := range cmd.Subscriptions {
				if strings.TrimSpace(in.Code) == "" {
					return usecase.Validation("CODE_REQUIRED", "Subscription code is required")
				}
				if strings.TrimSpace(in.Name) == "" {
					return usecase.Validation("NAME_REQUIRED", "Subscription name is required")
				}
				if strings.TrimSpace(in.Target) == "" {
					return usecase.Validation("TARGET_REQUIRED", "Target endpoint URL is required")
				}
				if len(in.EventTypes) == 0 {
					return usecase.Validation("EVENT_TYPES_REQUIRED", "At least one event type is required")
				}
				if in.SharedConnection && (in.ConnectionCode == nil || strings.TrimSpace(*in.ConnectionCode) == "") {
					return usecase.Validation("SHARED_CONNECTION_REQUIRES_CODE",
						"Subscription '"+in.Code+"': sharedConnection requires connectionCode")
				}
			}
			return nil
		},
		Authorize: func(ctx context.Context, cmd SyncSubscriptionsCommand) error {
			ac := auth.FromContext(ctx)
			if !ac.CanAccessApplication(cmd.ApplicationID) {
				return httperror.Forbidden("Not authorised for application '" + cmd.ApplicationCode + "'")
			}
			// Ruling (2026-09-21): mirrors connection.SyncConnections — a
			// client-less (global) subscription sync needs no anchor tier. It can
			// only ever create, update, or remove rows that belong to ITS OWN
			// application AND were authored by a prior sync (source API/CODE) —
			// never a UI row or another application's row — so ownership, not
			// reach, already fences it in. See SyncConnections' Authorize for the
			// fuller argument (it contrasts with the scheduled-job sync, where a
			// client-less job genuinely has platform-wide reach).
			if cmd.ClientID != nil && !ac.CanAccessClient(*cmd.ClientID) {
				return httperror.Forbidden("No access to client: " + *cmd.ClientID)
			}
			return nil
		},
		Execute: func(ctx context.Context, cmd SyncSubscriptionsCommand, ec usecase.ExecutionContext) (usecaseop.Plan[SubscriptionsSynced], error) {
			// Resolve each subscription's connection to an id, and check that
			// wherever it came from, its scope is consistent with this
			// subscription's client (ruling 2026-09-21 #5). Work on a copy so the
			// caller's command is left as it was sent.
			subs := slices.Clone(cmd.Subscriptions)
			for i := range subs {
				in := &subs[i]
				if in.ConnectionCode != nil && strings.TrimSpace(*in.ConnectionCode) != "" {
					code := strings.TrimSpace(*in.ConnectionCode)

					// The namespace is explicit, never guessed: a bare code names a
					// connection owned by THIS application; sharedConnection switches
					// to the application-less (shared) connections. No fallback
					// between the two — see SyncSubscriptionInput.SharedConnection.
					var namespaceAppCode *string
					if !in.SharedConnection {
						appCode := cmd.ApplicationCode
						namespaceAppCode = &appCode
					}

					// Within that namespace: a client-scoped sync prefers its own
					// client's connection, falling back to a global one; a
					// client-less sync only ever resolves a global connection (the
					// fallback lookup below IS that resolution when cmd.ClientID is
					// nil, since FindByCode(..., nil) only matches a NULL client_id).
					var c *connection.Connection
					var err error
					if cmd.ClientID != nil {
						c, err = connRepo.FindByCode(ctx, code, namespaceAppCode, cmd.ClientID)
						if err != nil {
							return nil, usecase.Internal("REPO", "find_by_code(connection) failed", err)
						}
					}
					if c == nil {
						c, err = connRepo.FindByCode(ctx, code, namespaceAppCode, nil)
						if err != nil {
							return nil, usecase.Internal("REPO", "find_by_code(connection) failed", err)
						}
					}
					if c == nil {
						return nil, usecase.NotFound("CONNECTION_NOT_FOUND", "Connection with code '"+code+"' not found")
					}
					if in.ConnectionID != nil && *in.ConnectionID != c.ID {
						return nil, usecase.Validation("CONNECTION_MISMATCH",
							"Subscription '"+in.Code+"': connectionId and connectionCode name different connections")
					}
					in.ConnectionID = &c.ID
					continue
				}
				if in.ConnectionID == nil {
					continue
				}
				c, err := connRepo.FindByID(ctx, *in.ConnectionID)
				if err != nil {
					return nil, usecase.Internal("REPO", "find_by_id(connection) failed", err)
				}
				if c == nil {
					return nil, usecase.NotFound("CONNECTION_NOT_FOUND", "Connection '"+*in.ConnectionID+"' not found")
				}
				// Scope consistency applies however the connection was named
				// (ruling 2026-09-21 #5): its client must be absent, or equal to
				// this subscription's client. A connectionCode lookup above can
				// never violate this (it only ever resolves this client's own
				// connection or a global one); a bare connectionId can name
				// anything, so it needs the explicit check.
				if c.ClientID != nil && (cmd.ClientID == nil || *c.ClientID != *cmd.ClientID) {
					return nil, usecase.Validation("CONNECTION_SCOPE_MISMATCH",
						"Subscription '"+in.Code+"': connection '"+*in.ConnectionID+"' is scoped to a different client")
				}
				// The same goes for the application axis. A connection signs
				// deliveries with its application's credentials, so an id must
				// not reach across: a caller with access to this application
				// only could otherwise borrow another application's. Shared
				// (application-less) connections stay usable by anyone.
				if c.ApplicationCode != nil && *c.ApplicationCode != cmd.ApplicationCode {
					return nil, usecase.Validation("CONNECTION_SCOPE_MISMATCH",
						"Subscription '"+in.Code+"': connection '"+*in.ConnectionID+"' belongs to a different application")
				}
			}

			existing, err := subRepo.FindByApplicationAndClient(ctx, cmd.ApplicationCode, cmd.ClientID)
			if err != nil {
				return nil, usecase.Internal("REPO", "find_by_application_and_client failed", err)
			}
			existingByCode := make(map[string]*subscription.Subscription, len(existing))
			for i := range existing {
				existingByCode[existing[i].Code] = &existing[i]
			}

			var (
				saves       []usecasepgx.SyncSaveItem[subscription.Subscription]
				deletes     []usecasepgx.SyncDeleteItem[subscription.Subscription]
				syncedCodes = make([]string, 0, len(cmd.Subscriptions))
				syncedSet   = make(map[string]struct{}, len(cmd.Subscriptions))
				created     uint32
				updated     uint32
				deleted     uint32
			)

			for _, in := range subs {
				syncedCodes = append(syncedCodes, in.Code)
				syncedSet[in.Code] = struct{}{}

				bindings := make([]subscription.EventTypeBinding, 0, len(in.EventTypes))
				for _, et := range in.EventTypes {
					b := subscription.NewEventTypeBinding(et.EventTypeCode)
					b.Filter = et.Filter
					bindings = append(bindings, b)
				}

				if cur, ok := existingByCode[in.Code]; ok {
					if cur.Source != subscription.SourceAPI && cur.Source != subscription.SourceCode {
						continue // never touch UI-authored rows
					}
					cur.Name = in.Name
					cur.Description = in.Description
					cur.Endpoint = in.Target
					cur.ConnectionID = in.ConnectionID
					cur.EventTypes = bindings
					cur.DataOnly = in.DataOnly
					if in.MaxRetries != nil {
						cur.MaxRetries = *in.MaxRetries
					}
					if in.TimeoutSeconds != nil {
						cur.TimeoutSeconds = *in.TimeoutSeconds
					}
					resolveDispatchPool(ctx, poolRepo, in.DispatchPoolCode, &cur.DispatchPoolID, &cur.DispatchPoolCode)
					saves = append(saves, usecasepgx.SyncSaveItem[subscription.Subscription]{
						Aggregate: cur,
						Event: SubscriptionUpdated{
							Metadata:       usecase.NewEventMetadata(ec, SubscriptionUpdatedType, Source, subjectFor(cur.ID)),
							SubscriptionID: cur.ID,
							Name:           cur.Name,
						},
					})
					updated++
					continue
				}

				sub := subscription.New(in.Code, in.Name, in.Target)
				sub.ConnectionID = in.ConnectionID
				appCode := cmd.ApplicationCode
				sub.ApplicationCode = &appCode
				sub.ClientID = cmd.ClientID
				sub.Source = subscription.SourceAPI
				sub.Description = in.Description
				sub.EventTypes = bindings
				sub.DataOnly = in.DataOnly
				pid := ec.PrincipalID
				sub.CreatedBy = &pid
				if in.MaxRetries != nil {
					sub.MaxRetries = *in.MaxRetries
				}
				if in.TimeoutSeconds != nil {
					sub.TimeoutSeconds = *in.TimeoutSeconds
				}
				resolveDispatchPool(ctx, poolRepo, in.DispatchPoolCode, &sub.DispatchPoolID, &sub.DispatchPoolCode)
				saves = append(saves, usecasepgx.SyncSaveItem[subscription.Subscription]{
					Aggregate: sub,
					Event: SubscriptionCreated{
						Metadata:       usecase.NewEventMetadata(ec, SubscriptionCreatedType, Source, subjectFor(sub.ID)),
						SubscriptionID: sub.ID,
						Code:           sub.Code,
						Name:           sub.Name,
					},
				})
				created++
			}

			if cmd.RemoveUnlisted {
				for i := range existing {
					cur := &existing[i]
					if cur.Source != subscription.SourceAPI && cur.Source != subscription.SourceCode {
						continue
					}
					if _, present := syncedSet[cur.Code]; present {
						continue
					}
					deletes = append(deletes, usecasepgx.SyncDeleteItem[subscription.Subscription]{
						Aggregate: cur,
						Event: SubscriptionDeleted{
							Metadata:       usecase.NewEventMetadata(ec, SubscriptionDeletedType, Source, subjectFor(cur.ID)),
							SubscriptionID: cur.ID,
							Code:           cur.Code,
						},
					})
					deleted++
				}
			}

			rollup := SubscriptionsSynced{
				Metadata:        usecase.NewEventMetadata(ec, SubscriptionsSyncedType, Source, "platform.subscriptions."+cmd.ApplicationCode),
				ApplicationCode: cmd.ApplicationCode,
				ClientID:        cmd.ClientID,
				Created:         created,
				Updated:         updated,
				Deleted:         deleted,
				SyncedCodes:     syncedCodes,
			}
			return usecaseop.Sync(subRepo, saves, deletes, rollup), nil
		},
	}
}

// resolveDispatchPool resolves a pool code to (id, code) via the global pool
// lookup and writes them through the supplied pointers. An empty/nil code or
// an unresolvable code leaves the targets untouched (a missing pool is
// silently ignored).
func resolveDispatchPool(ctx context.Context, poolRepo *dispatchpool.Repository, code *string, idOut, codeOut **string) {
	if code == nil || strings.TrimSpace(*code) == "" {
		return
	}
	pool, err := poolRepo.FindByCode(ctx, *code, nil)
	if err != nil || pool == nil {
		return
	}
	id := pool.ID
	c := pool.Code
	*idOut = &id
	*codeOut = &c
}
