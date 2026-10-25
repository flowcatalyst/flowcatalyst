/**
 * Resource Classes
 *
 * High-level wrappers around the generated SDK functions.
 */

export { EventTypesResource } from "./event-types.js";
export { ProcessesResource } from "./processes.js";
export { SubscriptionsResource } from "./subscriptions.js";
export { DispatchPoolsResource } from "./dispatch-pools.js";
export { RolesResource } from "./roles.js";
export { PermissionsResource } from "./permissions.js";
export { ApplicationsResource } from "./applications.js";
export { ClientsResource } from "./clients.js";
export { PrincipalsResource } from "./principals.js";
export {
	MeResource,
	type MyClient,
	type MyClientsResponse,
	type MyApplication,
	type MyApplicationsResponse,
} from "./me.js";
export { ConnectionsResource } from "./connections.js";
export {
	PlatformConfigResource,
	type RevealedSecret,
	type PlatformConfigScope,
	type PlatformConfigActorOptions,
	type ConfigResponse,
	type ConfigListResponse,
	type SetPropertyRequest,
} from "./platform-config.js";
export {
	IngestionsResource,
	type Ingestion,
	type IngestionList,
	type IngestionDefinition,
	type IngestionSource,
	type BuiltInIngestionSource,
	type CustomIngestionSource,
	type SftpIngestionSource,
	type BuiltInSourceKind,
	type IngestionSplit,
	type IngestionTarget,
	type TargetMode,
	type IngestionStatus,
	type IngestionVersion,
	type IngestionHistory,
	type IngestionHistoryEntry,
	type IngestionFailure,
	type IngestionFailureCategory,
	type IngestionFailureList,
	type IngestionSaveOutcome,
	type IngestionSaveOutcomeKind,
	type IngestionSecret,
	type IngestionSecretFetch,
	type IngestionSecretList,
	type IngestionSourceKind,
	type IngestionTest,
	type IngestionTestStatus,
	type IngestionTestOutcome,
	type IngestionSet,
	type IngestionSetItem,
	type IngestionSetPlan,
	type IngestionSetApply,
	type IngestionSetApplyArchive,
	type IngestionSetRelease,
	type SetChange,
	type SetChangeAction,
	type SetFile,
	type ApplyIngestionSetRequest,
	type CreateIngestionRequest,
	type UpdateIngestionRequest,
	type SaveIngestionVersionRequest,
	type TestIngestionRequest,
	type ListIngestionsFilters,
	type CursorPage,
	type FailureFilters,
	type HistoryPage,
} from "./ingestions.js";
export { AuditLogsResource } from "./audit-logs.js";
export {
	ScheduledJobsResource,
	type ScheduledJob,
	type ScheduledJobInstance,
	type ScheduledJobInstanceLog,
	type ScheduledJobStatus,
	type TriggerKind,
	type InstanceStatus,
	type CompletionStatus,
	type LogLevel,
	type CreateScheduledJobRequest,
	type UpdateScheduledJobRequest,
	type ListJobsFilters,
	type ListInstancesFilters,
	type FireRequest,
	type InstanceLogRequest,
	type InstanceCompleteRequest,
	type PaginatedJobs,
	type PaginatedInstances,
} from "./scheduled-jobs.js";
