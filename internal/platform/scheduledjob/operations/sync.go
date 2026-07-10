package operations

import (
	"bytes"
	"context"
	"slices"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/scheduledjob"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecaseop"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

// ScheduledJobSyncEntry is one job definition in an SDK scheduled-job sync.
type ScheduledJobSyncEntry struct {
	Code                string
	Name                string
	Description         *string
	Crons               []string
	Timezone            string
	Payload             []byte
	Concurrent          bool
	TracksCompletion    bool
	TimeoutSeconds      *int32
	DeliveryMaxAttempts int32
	TargetURL           *string
}

// SyncScheduledJobsCommand syncs scheduled-job definitions for one scope.
// ClientID nil = platform-scoped jobs (client_id IS NULL); otherwise the
// jobs belong to that client. ApplicationCode is the {appCode} from the URL,
// carried for audit/event provenance. ApplicationID is that same {appCode}
// resolved to the Application's id — persisted onto newly-created jobs (see
// Execute) so the list UI can filter/display by application; immutable
// thereafter, same as ClientID.
type SyncScheduledJobsCommand struct {
	ApplicationCode string
	ApplicationID   string
	ClientID        *string
	Jobs            []ScheduledJobSyncEntry
	ArchiveUnlisted bool
}

// SyncScheduledJobs upserts scheduled-job definitions within a single
// transaction. Mirrors the Rust SyncScheduledJobsUseCase exactly:
//
//   - Existing jobs are loaded for the target scope (the client when ClientID
//     is set, else the platform-scoped set).
//   - Each payload job is matched by code: an existing job is updated only if
//     a field actually changed (a no-op job is neither persisted nor reported);
//     a sync also re-activates an archived/paused job that reappears. A missing
//     job is created.
//   - ArchiveUnlisted archives (soft) ACTIVE jobs absent from the payload.
//
// Authorization: the coarse "may sync scheduled jobs" permission and the app
// resolution are the controller's job; the use case enforces the per-resource
// rule — the caller must have access to the target client (or be anchor/
// super-admin when targeting platform-scoped jobs, ClientID nil).
//
// Returns the affected job IDs split into created/updated/archived, carried
// on the [ScheduledJobsSynced] rollup. Per-row Created/Updated/Archived events
// are emitted alongside, atomic via [usecaseop.Sync].
func SyncScheduledJobs(repo *scheduledjob.Repository) usecaseop.Operation[SyncScheduledJobsCommand, ScheduledJobsSynced] {
	return usecaseop.Operation[SyncScheduledJobsCommand, ScheduledJobsSynced]{
		Name: "SyncScheduledJobs",
		Validate: func(_ context.Context, cmd SyncScheduledJobsCommand) error {
			for _, j := range cmd.Jobs {
				if strings.TrimSpace(j.Code) == "" || strings.TrimSpace(j.Name) == "" || len(j.Crons) == 0 {
					return usecase.Validation("INVALID_SYNC_ENTRY",
						"Sync entry '"+j.Code+"' must have code, name, and at least one cron")
				}
			}
			return nil
		},
		Authorize: func(ctx context.Context, cmd SyncScheduledJobsCommand) error {
			ac := auth.FromContext(ctx)
			// Resource-level scope check: the caller must have access to the
			// target client (or be anchor/super-admin when targeting platform-
			// scoped jobs). Mirrors the Rust handler.
			if cid := cmd.ClientID; cid != nil {
				if !ac.CanAccessClient(*cid) {
					return httperror.Forbidden("No access to client: " + *cid)
				}
				return nil
			}
			if !ac.IsAnchor() && !ac.IsSuperAdmin() {
				return httperror.Forbidden("Only anchor users can sync platform-scoped scheduled jobs")
			}
			return nil
		},
		Execute: func(ctx context.Context, cmd SyncScheduledJobsCommand, ec usecase.ExecutionContext) (usecaseop.Plan[ScheduledJobsSynced], error) {
			filter := scheduledjob.ListFilters{}
			if cmd.ClientID != nil {
				filter.ClientID = cmd.ClientID
			} else {
				platform := "" // pointer-to-"" selects platform-scoped (client_id IS NULL)
				filter.ClientID = &platform
			}
			existing, err := repo.FindWithFilters(ctx, filter)
			if err != nil {
				return nil, usecase.Internal("REPO", "find existing scheduled jobs failed", err)
			}
			existingByCode := make(map[string]*scheduledjob.ScheduledJob, len(existing))
			for i := range existing {
				existingByCode[existing[i].Code] = &existing[i]
			}

			var (
				saves    []usecasepgx.SyncSaveItem[scheduledjob.ScheduledJob]
				created  []string
				updated  []string
				archived []string
				pid      = ec.PrincipalID
			)

			for _, entry := range cmd.Jobs {
				if cur, ok := existingByCode[entry.Code]; ok {
					delete(existingByCode, entry.Code)
					changed := false
					if cur.Name != entry.Name {
						cur.Name = entry.Name
						changed = true
					}
					if !strPtrEqual(cur.Description, entry.Description) {
						cur.Description = entry.Description
						changed = true
					}
					if !slices.Equal(cur.Crons, entry.Crons) {
						cur.Crons = entry.Crons
						changed = true
					}
					if cur.Timezone != entry.Timezone {
						cur.Timezone = entry.Timezone
						changed = true
					}
					if !bytes.Equal(cur.Payload, entry.Payload) {
						cur.Payload = entry.Payload
						changed = true
					}
					if cur.Concurrent != entry.Concurrent {
						cur.Concurrent = entry.Concurrent
						changed = true
					}
					if cur.TracksCompletion != entry.TracksCompletion {
						cur.TracksCompletion = entry.TracksCompletion
						changed = true
					}
					if !int32PtrEqual(cur.TimeoutSeconds, entry.TimeoutSeconds) {
						cur.TimeoutSeconds = entry.TimeoutSeconds
						changed = true
					}
					if cur.DeliveryMaxAttempts != entry.DeliveryMaxAttempts {
						cur.DeliveryMaxAttempts = entry.DeliveryMaxAttempts
						changed = true
					}
					if !strPtrEqual(cur.TargetURL, entry.TargetURL) {
						cur.TargetURL = entry.TargetURL
						changed = true
					}
					// A sync re-activates archived/paused jobs that reappear.
					if cur.Status != scheduledjob.StatusActive {
						cur.Status = scheduledjob.StatusActive
						changed = true
					}
					if changed {
						cur.UpdatedBy = &pid
						cur.Version++
						saves = append(saves, usecasepgx.SyncSaveItem[scheduledjob.ScheduledJob]{
							Aggregate: cur,
							Event: ScheduledJobUpdated{commonEvent{
								Metadata:       usecase.NewEventMetadata(ec, ScheduledJobUpdatedType, Source, subjectFor(cur.ID)),
								ScheduledJobID: cur.ID,
								Code:           cur.Code,
							}},
						})
						updated = append(updated, cur.ID)
					}
					continue
				}

				j := scheduledjob.New(entry.Code, entry.Name, entry.Crons)
				j.Timezone = entry.Timezone
				j.Concurrent = entry.Concurrent
				j.TracksCompletion = entry.TracksCompletion
				j.DeliveryMaxAttempts = entry.DeliveryMaxAttempts
				j.Description = entry.Description
				j.Payload = entry.Payload
				j.TimeoutSeconds = entry.TimeoutSeconds
				j.TargetURL = entry.TargetURL
				j.ClientID = cmd.ClientID
				if cmd.ApplicationID != "" {
					j.ApplicationID = &cmd.ApplicationID
				}
				j.CreatedBy = &pid
				saves = append(saves, usecasepgx.SyncSaveItem[scheduledjob.ScheduledJob]{
					Aggregate: j,
					Event: ScheduledJobCreated{commonEvent{
						Metadata:       usecase.NewEventMetadata(ec, ScheduledJobCreatedType, Source, subjectFor(j.ID)),
						ScheduledJobID: j.ID,
						Code:           j.Code,
					}},
				})
				created = append(created, j.ID)
			}

			if cmd.ArchiveUnlisted {
				for _, cur := range existingByCode {
					if cur.Status != scheduledjob.StatusActive {
						continue
					}
					cur.Archive()
					saves = append(saves, usecasepgx.SyncSaveItem[scheduledjob.ScheduledJob]{
						Aggregate: cur,
						Event: ScheduledJobArchived{commonEvent{
							Metadata:       usecase.NewEventMetadata(ec, ScheduledJobArchivedType, Source, subjectFor(cur.ID)),
							ScheduledJobID: cur.ID,
							Code:           cur.Code,
						}},
					})
					archived = append(archived, cur.ID)
				}
			}

			rollup := ScheduledJobsSynced{
				Metadata:        usecase.NewEventMetadata(ec, ScheduledJobsSyncedType, Source, "platform.scheduledjobs.synced."+cmd.ApplicationCode),
				ApplicationCode: cmd.ApplicationCode,
				ClientID:        cmd.ClientID,
				Created:         created,
				Updated:         updated,
				Archived:        archived,
			}
			return usecaseop.Sync(repo, saves, nil, rollup), nil
		},
	}
}

func strPtrEqual(a, b *string) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

func int32PtrEqual(a, b *int32) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}
