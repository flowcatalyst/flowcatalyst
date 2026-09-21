package operations

import (
	"context"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/connection"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/validate"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/subscription"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecaseop"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

// SyncConnectionEntry is one connection definition in an SDK sync payload.
type SyncConnectionEntry struct {
	Code        string
	Name        string
	Description *string
	ExternalID  *string
}

// SyncConnectionsCommand syncs one application's API/CODE-sourced
// connections, scoped to a single (application, client) pair. ApplicationID
// is the resolved application id the sync is scoped to (the controller
// resolves it from the {appCode}); the use case authorizes against it.
// ClientID, when set, is already resolved to an id by the controller (the
// wire accepts either the client's id or its identifier). ClientID nil
// scopes the sync to the application's shared, client-less connections —
// still owned by this application, just tenant-less (not to be confused
// with "global", which this codebase reserves for "no client" on a
// connection that also has no application).
type SyncConnectionsCommand struct {
	ApplicationID   string
	ApplicationCode string
	ClientID        *string
	Connections     []SyncConnectionEntry
	RemoveUnlisted  bool
}

// SyncConnections bulk-upserts an application's connection catalogue,
// scoped to (ApplicationID, ClientID), within a single transaction:
//
//   - Validates app code; each entry needs a code (the same format
//     CreateConnection enforces) and a name; a request naming the same code
//     twice is rejected outright (CODE_REQUIRED / INVALID_CODE_FORMAT /
//     NAME_REQUIRED / DUPLICATE_CODE).
//   - A synced connection's own service account always follows the
//     application's provisioned service account — never the calling
//     principal's, and there is no per-connection override. An application
//     with no provisioned service account fails the whole sync
//     (APPLICATION_SERVICE_ACCOUNT_REQUIRED).
//   - Matches existing rows by code, scoped to (this application, this
//     client — NULL matches only NULL). Only API/CODE-sourced rows are
//     updated/removed; a UI-authored row at the same key is left untouched.
//     New rows are created with source=API.
//   - RemoveUnlisted hard-deletes API/CODE rows absent from the payload,
//     scoped to the same (application, client) key — never a sibling
//     client's rows, the application's shared rows, or another
//     application's rows. Removing a connection a live subscription still
//     targets is refused whole-sync (CONNECTION_REFERENCED) rather than
//     silently orphaning it — connection/operations/delete.go (the
//     single-row delete) has no such guard today; this sync does not
//     inherit that gap.
//
// Authorization: the coarse "may sync connections" permission is the
// controller's job; the use case enforces application access
// (CanAccessApplication) and, when ClientID is set, client access
// (CanAccessClient) — see Authorize below for why a client-less sync needs
// no anchor tier, unlike the scheduled-job sync.
//
// Emits per-row [ConnectionCreated]/[ConnectionUpdated]/[ConnectionDeleted]
// events plus one [ConnectionsSynced] rollup, atomic via [usecaseop.Sync].
func SyncConnections(
	connRepo *connection.Repository,
	apps *application.Repository,
	subRepo *subscription.Repository,
) usecaseop.Operation[SyncConnectionsCommand, ConnectionsSynced] {
	return usecaseop.Operation[SyncConnectionsCommand, ConnectionsSynced]{
		Name: "SyncConnections",
		Validate: func(_ context.Context, cmd SyncConnectionsCommand) error {
			if strings.TrimSpace(cmd.ApplicationCode) == "" {
				return usecase.Validation("APPLICATION_CODE_REQUIRED", "Application code is required")
			}
			seen := make(map[string]struct{}, len(cmd.Connections))
			for _, in := range cmd.Connections {
				code := strings.ToLower(strings.TrimSpace(in.Code))
				if code == "" {
					return usecase.Validation("CODE_REQUIRED", "Connection code is required")
				}
				if !validate.CodePattern.MatchString(code) {
					return usecase.Validation("INVALID_CODE_FORMAT",
						"Code must start with lowercase letter, contain only lowercase alphanumeric and hyphens")
				}
				if strings.TrimSpace(in.Name) == "" {
					return usecase.Validation("NAME_REQUIRED", "Connection name is required")
				}
				if _, dup := seen[code]; dup {
					return usecase.Validation("DUPLICATE_CODE", "Duplicate connection code '"+code+"' in sync request")
				}
				seen[code] = struct{}{}
			}
			return nil
		},
		Authorize: func(ctx context.Context, cmd SyncConnectionsCommand) error {
			ac := auth.FromContext(ctx)
			if !ac.CanAccessApplication(cmd.ApplicationID) {
				return httperror.Forbidden("Not authorised for application '" + cmd.ApplicationCode + "'")
			}
			// Ruling (2026-09-21): unlike SyncScheduledJobs, a client-less
			// (shared) connection sync needs no anchor tier. That use case's
			// anchor gate exists because a client-less scheduled job has
			// genuinely platform-wide reach with nothing else fencing it in.
			// Here the fence is ownership, not reach: this sync can only ever
			// create, update, or remove rows that belong to ITS OWN
			// application AND were authored by a prior sync (source API/CODE)
			// — never a UI row, a shared row some other caller wrote, or
			// another application's row. That containment already makes a
			// client-less sync safe for any caller with access to this
			// application; requiring anchor on top of it would only block
			// non-anchor application-service accounts from syncing their own
			// client-less connections, for no additional safety.
			if cmd.ClientID != nil && !ac.CanAccessClient(*cmd.ClientID) {
				return httperror.Forbidden("No access to client: " + *cmd.ClientID)
			}
			return nil
		},
		Execute: func(ctx context.Context, cmd SyncConnectionsCommand, ec usecase.ExecutionContext) (usecaseop.Plan[ConnectionsSynced], error) {
			app, err := apps.FindByID(ctx, cmd.ApplicationID)
			if err != nil {
				return nil, usecase.Internal("REPO", "find_application_by_id failed", err)
			}
			if app == nil {
				return nil, httperror.NotFound("Application", cmd.ApplicationID)
			}
			if app.ServiceAccountID == nil || strings.TrimSpace(*app.ServiceAccountID) == "" {
				return nil, usecase.Validation("APPLICATION_SERVICE_ACCOUNT_REQUIRED",
					"Application '"+cmd.ApplicationCode+"' has no provisioned service account; connections cannot be synced without one")
			}
			appServiceAccountID := *app.ServiceAccountID

			existing, err := connRepo.FindByApplicationAndClient(ctx, cmd.ApplicationCode, cmd.ClientID)
			if err != nil {
				return nil, usecase.Internal("REPO", "find_by_application_and_client failed", err)
			}
			existingByCode := make(map[string]*connection.Connection, len(existing))
			for i := range existing {
				existingByCode[existing[i].Code] = &existing[i]
			}

			var (
				saves       []usecasepgx.SyncSaveItem[connection.Connection]
				syncedCodes = make([]string, 0, len(cmd.Connections))
				syncedSet   = make(map[string]struct{}, len(cmd.Connections))
				created     uint32
				updated     uint32
			)

			for _, in := range cmd.Connections {
				code := strings.ToLower(strings.TrimSpace(in.Code))
				syncedCodes = append(syncedCodes, code)
				syncedSet[code] = struct{}{}

				if cur, ok := existingByCode[code]; ok {
					if cur.Source != connection.SourceAPI && cur.Source != connection.SourceCode {
						continue // never touch UI-authored rows
					}
					cur.Name = strings.TrimSpace(in.Name)
					cur.Description = in.Description
					cur.ExternalID = in.ExternalID
					cur.ServiceAccountID = appServiceAccountID
					saves = append(saves, usecasepgx.SyncSaveItem[connection.Connection]{
						Aggregate: cur,
						Event: ConnectionUpdated{
							Metadata:     usecase.NewEventMetadata(ec, ConnectionUpdatedType, Source, subjectFor(cur.ID)),
							ConnectionID: cur.ID,
							Name:         cur.Name,
						},
					})
					updated++
					continue
				}

				c := connection.New(code, strings.TrimSpace(in.Name), appServiceAccountID)
				appCode := cmd.ApplicationCode
				c.ApplicationCode = &appCode
				c.ClientID = cmd.ClientID
				c.Source = connection.SourceAPI
				c.Description = in.Description
				c.ExternalID = in.ExternalID
				saves = append(saves, usecasepgx.SyncSaveItem[connection.Connection]{
					Aggregate: c,
					Event: ConnectionCreated{
						Metadata:     usecase.NewEventMetadata(ec, ConnectionCreatedType, Source, subjectFor(c.ID)),
						ConnectionID: c.ID,
						Code:         c.Code,
						Name:         c.Name,
					},
				})
				created++
			}

			var (
				deletes []usecasepgx.SyncDeleteItem[connection.Connection]
				deleted uint32
			)
			if cmd.RemoveUnlisted {
				var candidates []*connection.Connection
				for i := range existing {
					cur := &existing[i]
					if cur.Source != connection.SourceAPI && cur.Source != connection.SourceCode {
						continue
					}
					if _, present := syncedSet[cur.Code]; present {
						continue
					}
					candidates = append(candidates, cur)
				}
				// Check every candidate for live references BEFORE deleting any
				// of them, so one referenced connection fails the whole sync
				// rather than leaving a partial removal behind.
				for _, cur := range candidates {
					refs, err := subRepo.FindCodesByConnectionID(ctx, cur.ID)
					if err != nil {
						return nil, usecase.Internal("REPO", "find subscriptions by connection failed", err)
					}
					if len(refs) > 0 {
						return nil, usecase.Conflict("CONNECTION_REFERENCED",
							"Connection '"+cur.Code+"' cannot be removed: still referenced by subscription(s) "+strings.Join(refs, ", "))
					}
				}
				for _, cur := range candidates {
					deletes = append(deletes, usecasepgx.SyncDeleteItem[connection.Connection]{
						Aggregate: cur,
						Event: ConnectionDeleted{
							Metadata:     usecase.NewEventMetadata(ec, ConnectionDeletedType, Source, subjectFor(cur.ID)),
							ConnectionID: cur.ID,
							Code:         cur.Code,
						},
					})
					deleted++
				}
			}

			rollup := ConnectionsSynced{
				Metadata:        usecase.NewEventMetadata(ec, ConnectionsSyncedType, Source, "platform.connections."+cmd.ApplicationCode),
				ApplicationCode: cmd.ApplicationCode,
				ClientID:        cmd.ClientID,
				Created:         created,
				Updated:         updated,
				Deleted:         deleted,
				SyncedCodes:     syncedCodes,
			}
			return usecaseop.Sync(connRepo, saves, deletes, rollup), nil
		},
	}
}
