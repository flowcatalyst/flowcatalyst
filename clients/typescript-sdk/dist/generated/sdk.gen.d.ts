import type { Client, Options as Options2, TDataShape } from './client';
import type { DeleteApiAdminClientsByIdData, DeleteApiAdminClientsByIdErrors, DeleteApiAdminClientsByIdResponses, DeleteApiAdminEventTypesByIdData, DeleteApiAdminEventTypesByIdErrors, DeleteApiAdminEventTypesByIdResponses, DeleteApiAdminOauthClientsByIdData, DeleteApiAdminOauthClientsByIdErrors, DeleteApiAdminOauthClientsByIdResponses, DeleteApiAdminPrincipalsByIdClientAccessByClientIdData, DeleteApiAdminPrincipalsByIdClientAccessByClientIdErrors, DeleteApiAdminPrincipalsByIdClientAccessByClientIdResponses, DeleteApiAdminPrincipalsByIdData, DeleteApiAdminPrincipalsByIdErrors, DeleteApiAdminPrincipalsByIdResponses, DeleteApiAdminPrincipalsByIdRolesByRoleNameData, DeleteApiAdminPrincipalsByIdRolesByRoleNameErrors, DeleteApiAdminPrincipalsByIdRolesByRoleNameResponses, DeleteApiAdminRolesByNameData, DeleteApiAdminRolesByNameErrors, DeleteApiAdminRolesByNamePermissionsByPermissionData, DeleteApiAdminRolesByNamePermissionsByPermissionErrors, DeleteApiAdminRolesByNamePermissionsByPermissionResponses, DeleteApiAdminRolesByNameResponses, DeleteApiAdminSubscriptionsByIdData, DeleteApiAdminSubscriptionsByIdErrors, DeleteApiAdminSubscriptionsByIdResponses, GetApiAdminAuditLogsApplicationIdsData, GetApiAdminAuditLogsApplicationIdsResponses, GetApiAdminAuditLogsByIdData, GetApiAdminAuditLogsByIdErrors, GetApiAdminAuditLogsByIdResponses, GetApiAdminAuditLogsClientIdsData, GetApiAdminAuditLogsClientIdsResponses, GetApiAdminAuditLogsData, GetApiAdminAuditLogsEntityByEntityTypeByEntityIdData, GetApiAdminAuditLogsEntityByEntityTypeByEntityIdResponses, GetApiAdminAuditLogsEntityTypesData, GetApiAdminAuditLogsEntityTypesResponses, GetApiAdminAuditLogsOperationsData, GetApiAdminAuditLogsOperationsResponses, GetApiAdminAuditLogsPrincipalByPrincipalIdData, GetApiAdminAuditLogsPrincipalByPrincipalIdResponses, GetApiAdminAuditLogsRecentData, GetApiAdminAuditLogsRecentResponses, GetApiAdminAuditLogsResponses, GetApiAdminClientsByIdApplicationsData, GetApiAdminClientsByIdApplicationsErrors, GetApiAdminClientsByIdApplicationsResponses, GetApiAdminClientsByIdData, GetApiAdminClientsByIdentifierByIdentifierData, GetApiAdminClientsByIdentifierByIdentifierErrors, GetApiAdminClientsByIdentifierByIdentifierResponses, GetApiAdminClientsByIdErrors, GetApiAdminClientsByIdResponses, GetApiAdminClientsData, GetApiAdminClientsResponses, GetApiAdminClientsSearchData, GetApiAdminClientsSearchResponses, GetApiAdminDispatchJobsByEventByEventIdData, GetApiAdminDispatchJobsByEventByEventIdResponses, GetApiAdminDispatchJobsByIdAttemptsData, GetApiAdminDispatchJobsByIdAttemptsErrors, GetApiAdminDispatchJobsByIdAttemptsResponses, GetApiAdminDispatchJobsByIdData, GetApiAdminDispatchJobsByIdErrors, GetApiAdminDispatchJobsByIdRawData, GetApiAdminDispatchJobsByIdRawErrors, GetApiAdminDispatchJobsByIdRawResponses, GetApiAdminDispatchJobsByIdResponses, GetApiAdminDispatchJobsData, GetApiAdminDispatchJobsFilterOptions2Data, GetApiAdminDispatchJobsFilterOptions2Responses, GetApiAdminDispatchJobsFilterOptionsData, GetApiAdminDispatchJobsFilterOptionsResponses, GetApiAdminDispatchJobsRawData, GetApiAdminDispatchJobsRawResponses, GetApiAdminDispatchJobsResponses, GetApiAdminEventsByIdData, GetApiAdminEventsByIdErrors, GetApiAdminEventsByIdResponses, GetApiAdminEventsData, GetApiAdminEventsFilterOptionsData, GetApiAdminEventsFilterOptionsResponses, GetApiAdminEventsRawData, GetApiAdminEventsRawResponses, GetApiAdminEventsResponses, GetApiAdminEventTypesByCodeByCodeData, GetApiAdminEventTypesByCodeByCodeErrors, GetApiAdminEventTypesByCodeByCodeResponses, GetApiAdminEventTypesByIdData, GetApiAdminEventTypesByIdErrors, GetApiAdminEventTypesByIdResponses, GetApiAdminEventTypesData, GetApiAdminEventTypesResponses, GetApiAdminFilterOptionsClientsData, GetApiAdminFilterOptionsClientsResponses, GetApiAdminFilterOptionsData, GetApiAdminFilterOptionsDispatchPoolsData, GetApiAdminFilterOptionsDispatchPoolsResponses, GetApiAdminFilterOptionsEventTypesData, GetApiAdminFilterOptionsEventTypesFiltersAggregatesData, GetApiAdminFilterOptionsEventTypesFiltersAggregatesResponses, GetApiAdminFilterOptionsEventTypesFiltersApplicationsData, GetApiAdminFilterOptionsEventTypesFiltersApplicationsResponses, GetApiAdminFilterOptionsEventTypesFiltersSubdomainsData, GetApiAdminFilterOptionsEventTypesFiltersSubdomainsResponses, GetApiAdminFilterOptionsEventTypesResponses, GetApiAdminFilterOptionsResponses, GetApiAdminFilterOptionsSubscriptionsData, GetApiAdminFilterOptionsSubscriptionsResponses, GetApiAdminMonitoringCircuitBreakersData, GetApiAdminMonitoringCircuitBreakersResponses, GetApiAdminMonitoringDashboardData, GetApiAdminMonitoringDashboardResponses, GetApiAdminMonitoringInFlightMessagesData, GetApiAdminMonitoringInFlightMessagesResponses, GetApiAdminMonitoringPoolStatsData, GetApiAdminMonitoringPoolStatsResponses, GetApiAdminMonitoringStandbyStatusData, GetApiAdminMonitoringStandbyStatusResponses, GetApiAdminOauthClientsByClientIdData, GetApiAdminOauthClientsByClientIdErrors, GetApiAdminOauthClientsByClientIdResponses, GetApiAdminOauthClientsByIdData, GetApiAdminOauthClientsByIdErrors, GetApiAdminOauthClientsByIdResponses, GetApiAdminOauthClientsData, GetApiAdminOauthClientsResponses, GetApiAdminPrincipalsByIdApplicationAccessData, GetApiAdminPrincipalsByIdApplicationAccessErrors, GetApiAdminPrincipalsByIdApplicationAccessResponses, GetApiAdminPrincipalsByIdAvailableApplicationsData, GetApiAdminPrincipalsByIdAvailableApplicationsErrors, GetApiAdminPrincipalsByIdAvailableApplicationsResponses, GetApiAdminPrincipalsByIdClientAccessData, GetApiAdminPrincipalsByIdClientAccessErrors, GetApiAdminPrincipalsByIdClientAccessResponses, GetApiAdminPrincipalsByIdData, GetApiAdminPrincipalsByIdErrors, GetApiAdminPrincipalsByIdResponses, GetApiAdminPrincipalsByIdRolesData, GetApiAdminPrincipalsByIdRolesErrors, GetApiAdminPrincipalsByIdRolesResponses, GetApiAdminPrincipalsCheckEmailDomainData, GetApiAdminPrincipalsCheckEmailDomainResponses, GetApiAdminPrincipalsData, GetApiAdminPrincipalsResponses, GetApiAdminRolesByApplicationByApplicationIdData, GetApiAdminRolesByApplicationByApplicationIdResponses, GetApiAdminRolesByCodeByCodeData, GetApiAdminRolesByCodeByCodeErrors, GetApiAdminRolesByCodeByCodeResponses, GetApiAdminRolesByNameData, GetApiAdminRolesByNameErrors, GetApiAdminRolesByNameResponses, GetApiAdminRolesBySourceBySourceData, GetApiAdminRolesBySourceBySourceErrors, GetApiAdminRolesBySourceBySourceResponses, GetApiAdminRolesData, GetApiAdminRolesFiltersApplicationsData, GetApiAdminRolesFiltersApplicationsResponses, GetApiAdminRolesPermissionsByPermissionData, GetApiAdminRolesPermissionsByPermissionErrors, GetApiAdminRolesPermissionsByPermissionResponses, GetApiAdminRolesPermissionsData, GetApiAdminRolesPermissionsResponses, GetApiAdminRolesResponses, GetApiAdminSubscriptionsByIdData, GetApiAdminSubscriptionsByIdErrors, GetApiAdminSubscriptionsByIdResponses, GetApiAdminSubscriptionsData, GetApiAdminSubscriptionsResponses, GetAuthCheckDomainData, GetAuthCheckDomainResponses, GetAuthMeData, GetAuthMeErrors, GetAuthMeResponses, PostApiAdminClientsByIdActivateData, PostApiAdminClientsByIdActivateErrors, PostApiAdminClientsByIdActivateResponses, PostApiAdminClientsByIdApplicationsByAppIdDisableData, PostApiAdminClientsByIdApplicationsByAppIdDisableErrors, PostApiAdminClientsByIdApplicationsByAppIdDisableResponses, PostApiAdminClientsByIdApplicationsByAppIdEnableData, PostApiAdminClientsByIdApplicationsByAppIdEnableErrors, PostApiAdminClientsByIdApplicationsByAppIdEnableResponses, PostApiAdminClientsByIdDeactivateData, PostApiAdminClientsByIdDeactivateErrors, PostApiAdminClientsByIdDeactivateResponses, PostApiAdminClientsByIdNotesData, PostApiAdminClientsByIdNotesErrors, PostApiAdminClientsByIdNotesResponses, PostApiAdminClientsByIdSuspendData, PostApiAdminClientsByIdSuspendErrors, PostApiAdminClientsByIdSuspendResponses, PostApiAdminClientsData, PostApiAdminClientsErrors, PostApiAdminClientsResponses, PostApiAdminDispatchJobsBatchData, PostApiAdminDispatchJobsBatchErrors, PostApiAdminDispatchJobsBatchResponses, PostApiAdminDispatchJobsData, PostApiAdminDispatchJobsErrors, PostApiAdminDispatchJobsResponses, PostApiAdminEventsBatchData, PostApiAdminEventsBatchErrors, PostApiAdminEventsBatchResponses, PostApiAdminEventsData, PostApiAdminEventsErrors, PostApiAdminEventsResponses, PostApiAdminEventTypesByIdSchemasData, PostApiAdminEventTypesByIdSchemasErrors, PostApiAdminEventTypesByIdSchemasResponses, PostApiAdminEventTypesData, PostApiAdminEventTypesErrors, PostApiAdminEventTypesResponses, PostApiAdminEventTypesSyncData, PostApiAdminEventTypesSyncErrors, PostApiAdminEventTypesSyncResponses, PostApiAdminOauthClientsActivateData, PostApiAdminOauthClientsActivateErrors, PostApiAdminOauthClientsActivateResponses, PostApiAdminOauthClientsData, PostApiAdminOauthClientsDeactivateData, PostApiAdminOauthClientsDeactivateErrors, PostApiAdminOauthClientsDeactivateResponses, PostApiAdminOauthClientsErrors, PostApiAdminOauthClientsRegenerateSecretData, PostApiAdminOauthClientsRegenerateSecretErrors, PostApiAdminOauthClientsRegenerateSecretResponses, PostApiAdminOauthClientsResponses, PostApiAdminOauthClientsRotateSecretData, PostApiAdminOauthClientsRotateSecretErrors, PostApiAdminOauthClientsRotateSecretResponses, PostApiAdminPrincipalsByIdActivateData, PostApiAdminPrincipalsByIdActivateErrors, PostApiAdminPrincipalsByIdActivateResponses, PostApiAdminPrincipalsByIdClientAccessData, PostApiAdminPrincipalsByIdClientAccessErrors, PostApiAdminPrincipalsByIdClientAccessResponses, PostApiAdminPrincipalsByIdDeactivateData, PostApiAdminPrincipalsByIdDeactivateErrors, PostApiAdminPrincipalsByIdDeactivateResponses, PostApiAdminPrincipalsByIdResetPasswordData, PostApiAdminPrincipalsByIdResetPasswordErrors, PostApiAdminPrincipalsByIdResetPasswordResponses, PostApiAdminPrincipalsByIdRolesData, PostApiAdminPrincipalsByIdRolesErrors, PostApiAdminPrincipalsByIdRolesResponses, PostApiAdminPrincipalsUsersData, PostApiAdminPrincipalsUsersErrors, PostApiAdminPrincipalsUsersResponses, PostApiAdminRolesByNamePermissionsData, PostApiAdminRolesByNamePermissionsErrors, PostApiAdminRolesByNamePermissionsResponses, PostApiAdminRolesData, PostApiAdminRolesErrors, PostApiAdminRolesResponses, PostApiAdminSubscriptionsByIdPauseData, PostApiAdminSubscriptionsByIdPauseErrors, PostApiAdminSubscriptionsByIdPauseResponses, PostApiAdminSubscriptionsByIdResumeData, PostApiAdminSubscriptionsByIdResumeErrors, PostApiAdminSubscriptionsByIdResumeResponses, PostApiAdminSubscriptionsData, PostApiAdminSubscriptionsErrors, PostApiAdminSubscriptionsResponses, PostApiAdminSubscriptionsSyncData, PostApiAdminSubscriptionsSyncErrors, PostApiAdminSubscriptionsSyncResponses, PostAuthLoginData, PostAuthLoginErrors, PostAuthLoginResponses, PostAuthLogoutData, PostAuthLogoutResponses, PostAuthRefreshData, PostAuthRefreshErrors, PostAuthRefreshResponses, PutApiAdminClientsByIdApplicationsData, PutApiAdminClientsByIdApplicationsErrors, PutApiAdminClientsByIdApplicationsResponses, PutApiAdminClientsByIdData, PutApiAdminClientsByIdErrors, PutApiAdminClientsByIdResponses, PutApiAdminEventTypesByIdData, PutApiAdminEventTypesByIdErrors, PutApiAdminEventTypesByIdResponses, PutApiAdminOauthClientsByIdData, PutApiAdminOauthClientsByIdErrors, PutApiAdminOauthClientsByIdResponses, PutApiAdminPrincipalsByIdApplicationAccessData, PutApiAdminPrincipalsByIdApplicationAccessErrors, PutApiAdminPrincipalsByIdApplicationAccessResponses, PutApiAdminPrincipalsByIdData, PutApiAdminPrincipalsByIdErrors, PutApiAdminPrincipalsByIdResponses, PutApiAdminPrincipalsByIdRolesData, PutApiAdminPrincipalsByIdRolesErrors, PutApiAdminPrincipalsByIdRolesResponses, PutApiAdminRolesByNameData, PutApiAdminRolesByNameErrors, PutApiAdminRolesByNameResponses, PutApiAdminSubscriptionsByIdData, PutApiAdminSubscriptionsByIdErrors, PutApiAdminSubscriptionsByIdResponses } from './types.gen';
export type Options<TData extends TDataShape = TDataShape, ThrowOnError extends boolean = boolean> = Options2<TData, ThrowOnError> & {
    /**
     * You can provide a client instance returned by `createClient()` instead of
     * individual options. This might be also useful if you want to implement a
     * custom client.
     */
    client?: Client;
    /**
     * You can pass arbitrary values through the `meta` object. This can be
     * used to access values that aren't defined as part of the SDK function.
     */
    meta?: Record<string, unknown>;
};
/**
 * List audit logs with filters (matches Java AuditLogAdminResource)
 */
export declare const getApiAdminAuditLogs: <ThrowOnError extends boolean = false>(options?: Options<GetApiAdminAuditLogsData, ThrowOnError>) => import("./client").RequestResult<GetApiAdminAuditLogsResponses, unknown, ThrowOnError, "fields">;
/**
 * Get distinct application IDs
 */
export declare const getApiAdminAuditLogsApplicationIds: <ThrowOnError extends boolean = false>(options?: Options<GetApiAdminAuditLogsApplicationIdsData, ThrowOnError>) => import("./client").RequestResult<GetApiAdminAuditLogsApplicationIdsResponses, unknown, ThrowOnError, "fields">;
/**
 * Get distinct client IDs
 */
export declare const getApiAdminAuditLogsClientIds: <ThrowOnError extends boolean = false>(options?: Options<GetApiAdminAuditLogsClientIdsData, ThrowOnError>) => import("./client").RequestResult<GetApiAdminAuditLogsClientIdsResponses, unknown, ThrowOnError, "fields">;
/**
 * Get distinct entity types
 */
export declare const getApiAdminAuditLogsEntityTypes: <ThrowOnError extends boolean = false>(options?: Options<GetApiAdminAuditLogsEntityTypesData, ThrowOnError>) => import("./client").RequestResult<GetApiAdminAuditLogsEntityTypesResponses, unknown, ThrowOnError, "fields">;
/**
 * Get audit logs for a specific entity
 */
export declare const getApiAdminAuditLogsEntityByEntityTypeByEntityId: <ThrowOnError extends boolean = false>(options: Options<GetApiAdminAuditLogsEntityByEntityTypeByEntityIdData, ThrowOnError>) => import("./client").RequestResult<GetApiAdminAuditLogsEntityByEntityTypeByEntityIdResponses, unknown, ThrowOnError, "fields">;
/**
 * Get distinct operations
 */
export declare const getApiAdminAuditLogsOperations: <ThrowOnError extends boolean = false>(options?: Options<GetApiAdminAuditLogsOperationsData, ThrowOnError>) => import("./client").RequestResult<GetApiAdminAuditLogsOperationsResponses, unknown, ThrowOnError, "fields">;
/**
 * Get audit logs for a principal
 */
export declare const getApiAdminAuditLogsPrincipalByPrincipalId: <ThrowOnError extends boolean = false>(options: Options<GetApiAdminAuditLogsPrincipalByPrincipalIdData, ThrowOnError>) => import("./client").RequestResult<GetApiAdminAuditLogsPrincipalByPrincipalIdResponses, unknown, ThrowOnError, "fields">;
/**
 * Get recent audit logs
 */
export declare const getApiAdminAuditLogsRecent: <ThrowOnError extends boolean = false>(options?: Options<GetApiAdminAuditLogsRecentData, ThrowOnError>) => import("./client").RequestResult<GetApiAdminAuditLogsRecentResponses, unknown, ThrowOnError, "fields">;
/**
 * Get audit log by ID
 */
export declare const getApiAdminAuditLogsById: <ThrowOnError extends boolean = false>(options: Options<GetApiAdminAuditLogsByIdData, ThrowOnError>) => import("./client").RequestResult<GetApiAdminAuditLogsByIdResponses, GetApiAdminAuditLogsByIdErrors, ThrowOnError, "fields">;
/**
 * List clients
 */
export declare const getApiAdminClients: <ThrowOnError extends boolean = false>(options?: Options<GetApiAdminClientsData, ThrowOnError>) => import("./client").RequestResult<GetApiAdminClientsResponses, unknown, ThrowOnError, "fields">;
/**
 * Create a new client
 */
export declare const postApiAdminClients: <ThrowOnError extends boolean = false>(options: Options<PostApiAdminClientsData, ThrowOnError>) => import("./client").RequestResult<PostApiAdminClientsResponses, PostApiAdminClientsErrors, ThrowOnError, "fields">;
/**
 * Get client by identifier
 */
export declare const getApiAdminClientsByIdentifierByIdentifier: <ThrowOnError extends boolean = false>(options: Options<GetApiAdminClientsByIdentifierByIdentifierData, ThrowOnError>) => import("./client").RequestResult<GetApiAdminClientsByIdentifierByIdentifierResponses, GetApiAdminClientsByIdentifierByIdentifierErrors, ThrowOnError, "fields">;
/**
 * Search clients
 */
export declare const getApiAdminClientsSearch: <ThrowOnError extends boolean = false>(options?: Options<GetApiAdminClientsSearchData, ThrowOnError>) => import("./client").RequestResult<GetApiAdminClientsSearchResponses, unknown, ThrowOnError, "fields">;
/**
 * Delete client (soft delete)
 */
export declare const deleteApiAdminClientsById: <ThrowOnError extends boolean = false>(options: Options<DeleteApiAdminClientsByIdData, ThrowOnError>) => import("./client").RequestResult<DeleteApiAdminClientsByIdResponses, DeleteApiAdminClientsByIdErrors, ThrowOnError, "fields">;
/**
 * Get client by ID
 */
export declare const getApiAdminClientsById: <ThrowOnError extends boolean = false>(options: Options<GetApiAdminClientsByIdData, ThrowOnError>) => import("./client").RequestResult<GetApiAdminClientsByIdResponses, GetApiAdminClientsByIdErrors, ThrowOnError, "fields">;
/**
 * Update client
 */
export declare const putApiAdminClientsById: <ThrowOnError extends boolean = false>(options: Options<PutApiAdminClientsByIdData, ThrowOnError>) => import("./client").RequestResult<PutApiAdminClientsByIdResponses, PutApiAdminClientsByIdErrors, ThrowOnError, "fields">;
/**
 * Activate a client
 *
 * Transitions a suspended or pending client to active status.
 */
export declare const postApiAdminClientsByIdActivate: <ThrowOnError extends boolean = false>(options: Options<PostApiAdminClientsByIdActivateData, ThrowOnError>) => import("./client").RequestResult<PostApiAdminClientsByIdActivateResponses, PostApiAdminClientsByIdActivateErrors, ThrowOnError, "fields">;
/**
 * Get client applications
 */
export declare const getApiAdminClientsByIdApplications: <ThrowOnError extends boolean = false>(options: Options<GetApiAdminClientsByIdApplicationsData, ThrowOnError>) => import("./client").RequestResult<GetApiAdminClientsByIdApplicationsResponses, GetApiAdminClientsByIdApplicationsErrors, ThrowOnError, "fields">;
/**
 * Update client applications (bulk)
 */
export declare const putApiAdminClientsByIdApplications: <ThrowOnError extends boolean = false>(options: Options<PutApiAdminClientsByIdApplicationsData, ThrowOnError>) => import("./client").RequestResult<PutApiAdminClientsByIdApplicationsResponses, PutApiAdminClientsByIdApplicationsErrors, ThrowOnError, "fields">;
/**
 * Disable application for client
 */
export declare const postApiAdminClientsByIdApplicationsByAppIdDisable: <ThrowOnError extends boolean = false>(options: Options<PostApiAdminClientsByIdApplicationsByAppIdDisableData, ThrowOnError>) => import("./client").RequestResult<PostApiAdminClientsByIdApplicationsByAppIdDisableResponses, PostApiAdminClientsByIdApplicationsByAppIdDisableErrors, ThrowOnError, "fields">;
/**
 * Enable application for client
 */
export declare const postApiAdminClientsByIdApplicationsByAppIdEnable: <ThrowOnError extends boolean = false>(options: Options<PostApiAdminClientsByIdApplicationsByAppIdEnableData, ThrowOnError>) => import("./client").RequestResult<PostApiAdminClientsByIdApplicationsByAppIdEnableResponses, PostApiAdminClientsByIdApplicationsByAppIdEnableErrors, ThrowOnError, "fields">;
/**
 * Deactivate a client (soft delete)
 *
 * Deactivates/soft-deletes a client. Requires a reason.
 */
export declare const postApiAdminClientsByIdDeactivate: <ThrowOnError extends boolean = false>(options: Options<PostApiAdminClientsByIdDeactivateData, ThrowOnError>) => import("./client").RequestResult<PostApiAdminClientsByIdDeactivateResponses, PostApiAdminClientsByIdDeactivateErrors, ThrowOnError, "fields">;
/**
 * Add note to client
 */
export declare const postApiAdminClientsByIdNotes: <ThrowOnError extends boolean = false>(options: Options<PostApiAdminClientsByIdNotesData, ThrowOnError>) => import("./client").RequestResult<PostApiAdminClientsByIdNotesResponses, PostApiAdminClientsByIdNotesErrors, ThrowOnError, "fields">;
/**
 * Suspend a client
 *
 * Suspends a client (e.g., for billing issues). Requires a reason.
 */
export declare const postApiAdminClientsByIdSuspend: <ThrowOnError extends boolean = false>(options: Options<PostApiAdminClientsByIdSuspendData, ThrowOnError>) => import("./client").RequestResult<PostApiAdminClientsByIdSuspendResponses, PostApiAdminClientsByIdSuspendErrors, ThrowOnError, "fields">;
/**
 * List event types
 */
export declare const getApiAdminEventTypes: <ThrowOnError extends boolean = false>(options: Options<GetApiAdminEventTypesData, ThrowOnError>) => import("./client").RequestResult<GetApiAdminEventTypesResponses, unknown, ThrowOnError, "fields">;
/**
 * Create a new event type
 */
export declare const postApiAdminEventTypes: <ThrowOnError extends boolean = false>(options: Options<PostApiAdminEventTypesData, ThrowOnError>) => import("./client").RequestResult<PostApiAdminEventTypesResponses, PostApiAdminEventTypesErrors, ThrowOnError, "fields">;
/**
 * Get event type by code
 */
export declare const getApiAdminEventTypesByCodeByCode: <ThrowOnError extends boolean = false>(options: Options<GetApiAdminEventTypesByCodeByCodeData, ThrowOnError>) => import("./client").RequestResult<GetApiAdminEventTypesByCodeByCodeResponses, GetApiAdminEventTypesByCodeByCodeErrors, ThrowOnError, "fields">;
/**
 * Sync event types
 */
export declare const postApiAdminEventTypesSync: <ThrowOnError extends boolean = false>(options: Options<PostApiAdminEventTypesSyncData, ThrowOnError>) => import("./client").RequestResult<PostApiAdminEventTypesSyncResponses, PostApiAdminEventTypesSyncErrors, ThrowOnError, "fields">;
/**
 * Delete event type (archive)
 */
export declare const deleteApiAdminEventTypesById: <ThrowOnError extends boolean = false>(options: Options<DeleteApiAdminEventTypesByIdData, ThrowOnError>) => import("./client").RequestResult<DeleteApiAdminEventTypesByIdResponses, DeleteApiAdminEventTypesByIdErrors, ThrowOnError, "fields">;
/**
 * Get event type by ID
 */
export declare const getApiAdminEventTypesById: <ThrowOnError extends boolean = false>(options: Options<GetApiAdminEventTypesByIdData, ThrowOnError>) => import("./client").RequestResult<GetApiAdminEventTypesByIdResponses, GetApiAdminEventTypesByIdErrors, ThrowOnError, "fields">;
/**
 * Update event type
 */
export declare const putApiAdminEventTypesById: <ThrowOnError extends boolean = false>(options: Options<PutApiAdminEventTypesByIdData, ThrowOnError>) => import("./client").RequestResult<PutApiAdminEventTypesByIdResponses, PutApiAdminEventTypesByIdErrors, ThrowOnError, "fields">;
/**
 * Add schema version to event type
 */
export declare const postApiAdminEventTypesByIdSchemas: <ThrowOnError extends boolean = false>(options: Options<PostApiAdminEventTypesByIdSchemasData, ThrowOnError>) => import("./client").RequestResult<PostApiAdminEventTypesByIdSchemasResponses, PostApiAdminEventTypesByIdSchemasErrors, ThrowOnError, "fields">;
/**
 * List OAuth clients
 */
export declare const getApiAdminOauthClients: <ThrowOnError extends boolean = false>(options: Options<GetApiAdminOauthClientsData, ThrowOnError>) => import("./client").RequestResult<GetApiAdminOauthClientsResponses, unknown, ThrowOnError, "fields">;
/**
 * Create a new OAuth client
 */
export declare const postApiAdminOauthClients: <ThrowOnError extends boolean = false>(options: Options<PostApiAdminOauthClientsData, ThrowOnError>) => import("./client").RequestResult<PostApiAdminOauthClientsResponses, PostApiAdminOauthClientsErrors, ThrowOnError, "fields">;
/**
 * Get OAuth client by client_id (public identifier)
 */
export declare const getApiAdminOauthClientsByClientId: <ThrowOnError extends boolean = false>(options: Options<GetApiAdminOauthClientsByClientIdData, ThrowOnError>) => import("./client").RequestResult<GetApiAdminOauthClientsByClientIdResponses, GetApiAdminOauthClientsByClientIdErrors, ThrowOnError, "fields">;
/**
 * Delete OAuth client
 */
export declare const deleteApiAdminOauthClientsById: <ThrowOnError extends boolean = false>(options: Options<DeleteApiAdminOauthClientsByIdData, ThrowOnError>) => import("./client").RequestResult<DeleteApiAdminOauthClientsByIdResponses, DeleteApiAdminOauthClientsByIdErrors, ThrowOnError, "fields">;
/**
 * Get OAuth client by ID
 */
export declare const getApiAdminOauthClientsById: <ThrowOnError extends boolean = false>(options: Options<GetApiAdminOauthClientsByIdData, ThrowOnError>) => import("./client").RequestResult<GetApiAdminOauthClientsByIdResponses, GetApiAdminOauthClientsByIdErrors, ThrowOnError, "fields">;
/**
 * Update OAuth client
 */
export declare const putApiAdminOauthClientsById: <ThrowOnError extends boolean = false>(options: Options<PutApiAdminOauthClientsByIdData, ThrowOnError>) => import("./client").RequestResult<PutApiAdminOauthClientsByIdResponses, PutApiAdminOauthClientsByIdErrors, ThrowOnError, "fields">;
/**
 * Activate OAuth client
 */
export declare const postApiAdminOauthClientsActivate: <ThrowOnError extends boolean = false>(options: Options<PostApiAdminOauthClientsActivateData, ThrowOnError>) => import("./client").RequestResult<PostApiAdminOauthClientsActivateResponses, PostApiAdminOauthClientsActivateErrors, ThrowOnError, "fields">;
/**
 * Deactivate OAuth client
 */
export declare const postApiAdminOauthClientsDeactivate: <ThrowOnError extends boolean = false>(options: Options<PostApiAdminOauthClientsDeactivateData, ThrowOnError>) => import("./client").RequestResult<PostApiAdminOauthClientsDeactivateResponses, PostApiAdminOauthClientsDeactivateErrors, ThrowOnError, "fields">;
/**
 * Regenerate OAuth client secret
 */
export declare const postApiAdminOauthClientsRegenerateSecret: <ThrowOnError extends boolean = false>(options: Options<PostApiAdminOauthClientsRegenerateSecretData, ThrowOnError>) => import("./client").RequestResult<PostApiAdminOauthClientsRegenerateSecretResponses, PostApiAdminOauthClientsRegenerateSecretErrors, ThrowOnError, "fields">;
/**
 * Rotate OAuth client secret (alias for regenerate-secret, matches TS API)
 */
export declare const postApiAdminOauthClientsRotateSecret: <ThrowOnError extends boolean = false>(options: Options<PostApiAdminOauthClientsRotateSecretData, ThrowOnError>) => import("./client").RequestResult<PostApiAdminOauthClientsRotateSecretResponses, PostApiAdminOauthClientsRotateSecretErrors, ThrowOnError, "fields">;
/**
 * List principals
 */
export declare const getApiAdminPrincipals: <ThrowOnError extends boolean = false>(options?: Options<GetApiAdminPrincipalsData, ThrowOnError>) => import("./client").RequestResult<GetApiAdminPrincipalsResponses, unknown, ThrowOnError, "fields">;
/**
 * Create a new user principal
 */
export declare const postApiAdminPrincipalsUsers: <ThrowOnError extends boolean = false>(options: Options<PostApiAdminPrincipalsUsersData, ThrowOnError>) => import("./client").RequestResult<PostApiAdminPrincipalsUsersResponses, PostApiAdminPrincipalsUsersErrors, ThrowOnError, "fields">;
/**
 * Check email domain configuration
 */
export declare const getApiAdminPrincipalsCheckEmailDomain: <ThrowOnError extends boolean = false>(options: Options<GetApiAdminPrincipalsCheckEmailDomainData, ThrowOnError>) => import("./client").RequestResult<GetApiAdminPrincipalsCheckEmailDomainResponses, unknown, ThrowOnError, "fields">;
/**
 * Delete principal (deactivate)
 */
export declare const deleteApiAdminPrincipalsById: <ThrowOnError extends boolean = false>(options: Options<DeleteApiAdminPrincipalsByIdData, ThrowOnError>) => import("./client").RequestResult<DeleteApiAdminPrincipalsByIdResponses, DeleteApiAdminPrincipalsByIdErrors, ThrowOnError, "fields">;
/**
 * Get principal by ID
 */
export declare const getApiAdminPrincipalsById: <ThrowOnError extends boolean = false>(options: Options<GetApiAdminPrincipalsByIdData, ThrowOnError>) => import("./client").RequestResult<GetApiAdminPrincipalsByIdResponses, GetApiAdminPrincipalsByIdErrors, ThrowOnError, "fields">;
/**
 * Update principal
 */
export declare const putApiAdminPrincipalsById: <ThrowOnError extends boolean = false>(options: Options<PutApiAdminPrincipalsByIdData, ThrowOnError>) => import("./client").RequestResult<PutApiAdminPrincipalsByIdResponses, PutApiAdminPrincipalsByIdErrors, ThrowOnError, "fields">;
/**
 * Activate a principal
 *
 * Reactivates a deactivated principal.
 */
export declare const postApiAdminPrincipalsByIdActivate: <ThrowOnError extends boolean = false>(options: Options<PostApiAdminPrincipalsByIdActivateData, ThrowOnError>) => import("./client").RequestResult<PostApiAdminPrincipalsByIdActivateResponses, PostApiAdminPrincipalsByIdActivateErrors, ThrowOnError, "fields">;
/**
 * Get application access for a principal
 *
 * Returns all applications the principal has been granted access to.
 */
export declare const getApiAdminPrincipalsByIdApplicationAccess: <ThrowOnError extends boolean = false>(options: Options<GetApiAdminPrincipalsByIdApplicationAccessData, ThrowOnError>) => import("./client").RequestResult<GetApiAdminPrincipalsByIdApplicationAccessResponses, GetApiAdminPrincipalsByIdApplicationAccessErrors, ThrowOnError, "fields">;
/**
 * Set application access for a principal (batch replace)
 *
 * Replaces all application access with the provided list.
 */
export declare const putApiAdminPrincipalsByIdApplicationAccess: <ThrowOnError extends boolean = false>(options: Options<PutApiAdminPrincipalsByIdApplicationAccessData, ThrowOnError>) => import("./client").RequestResult<PutApiAdminPrincipalsByIdApplicationAccessResponses, PutApiAdminPrincipalsByIdApplicationAccessErrors, ThrowOnError, "fields">;
/**
 * Get available applications for a principal
 *
 * ANCHOR users see all active applications.
 * CLIENT users see only applications enabled for their accessible client configs.
 */
export declare const getApiAdminPrincipalsByIdAvailableApplications: <ThrowOnError extends boolean = false>(options: Options<GetApiAdminPrincipalsByIdAvailableApplicationsData, ThrowOnError>) => import("./client").RequestResult<GetApiAdminPrincipalsByIdAvailableApplicationsResponses, GetApiAdminPrincipalsByIdAvailableApplicationsErrors, ThrowOnError, "fields">;
/**
 * Get client access grants for a principal
 */
export declare const getApiAdminPrincipalsByIdClientAccess: <ThrowOnError extends boolean = false>(options: Options<GetApiAdminPrincipalsByIdClientAccessData, ThrowOnError>) => import("./client").RequestResult<GetApiAdminPrincipalsByIdClientAccessResponses, GetApiAdminPrincipalsByIdClientAccessErrors, ThrowOnError, "fields">;
/**
 * Grant client access to principal
 */
export declare const postApiAdminPrincipalsByIdClientAccess: <ThrowOnError extends boolean = false>(options: Options<PostApiAdminPrincipalsByIdClientAccessData, ThrowOnError>) => import("./client").RequestResult<PostApiAdminPrincipalsByIdClientAccessResponses, PostApiAdminPrincipalsByIdClientAccessErrors, ThrowOnError, "fields">;
/**
 * Revoke client access from principal
 */
export declare const deleteApiAdminPrincipalsByIdClientAccessByClientId: <ThrowOnError extends boolean = false>(options: Options<DeleteApiAdminPrincipalsByIdClientAccessByClientIdData, ThrowOnError>) => import("./client").RequestResult<DeleteApiAdminPrincipalsByIdClientAccessByClientIdResponses, DeleteApiAdminPrincipalsByIdClientAccessByClientIdErrors, ThrowOnError, "fields">;
/**
 * Deactivate a principal
 *
 * Deactivates an active principal.
 */
export declare const postApiAdminPrincipalsByIdDeactivate: <ThrowOnError extends boolean = false>(options: Options<PostApiAdminPrincipalsByIdDeactivateData, ThrowOnError>) => import("./client").RequestResult<PostApiAdminPrincipalsByIdDeactivateResponses, PostApiAdminPrincipalsByIdDeactivateErrors, ThrowOnError, "fields">;
/**
 * Reset a user's password
 *
 * Resets the password for an internal auth user. Does not work for OIDC users.
 */
export declare const postApiAdminPrincipalsByIdResetPassword: <ThrowOnError extends boolean = false>(options: Options<PostApiAdminPrincipalsByIdResetPasswordData, ThrowOnError>) => import("./client").RequestResult<PostApiAdminPrincipalsByIdResetPasswordResponses, PostApiAdminPrincipalsByIdResetPasswordErrors, ThrowOnError, "fields">;
/**
 * Get roles assigned to a principal
 */
export declare const getApiAdminPrincipalsByIdRoles: <ThrowOnError extends boolean = false>(options: Options<GetApiAdminPrincipalsByIdRolesData, ThrowOnError>) => import("./client").RequestResult<GetApiAdminPrincipalsByIdRolesResponses, GetApiAdminPrincipalsByIdRolesErrors, ThrowOnError, "fields">;
/**
 * Assign role to principal
 */
export declare const postApiAdminPrincipalsByIdRoles: <ThrowOnError extends boolean = false>(options: Options<PostApiAdminPrincipalsByIdRolesData, ThrowOnError>) => import("./client").RequestResult<PostApiAdminPrincipalsByIdRolesResponses, PostApiAdminPrincipalsByIdRolesErrors, ThrowOnError, "fields">;
/**
 * Batch assign roles to principal (declarative - replaces all roles)
 */
export declare const putApiAdminPrincipalsByIdRoles: <ThrowOnError extends boolean = false>(options: Options<PutApiAdminPrincipalsByIdRolesData, ThrowOnError>) => import("./client").RequestResult<PutApiAdminPrincipalsByIdRolesResponses, PutApiAdminPrincipalsByIdRolesErrors, ThrowOnError, "fields">;
/**
 * Remove role from principal
 */
export declare const deleteApiAdminPrincipalsByIdRolesByRoleName: <ThrowOnError extends boolean = false>(options: Options<DeleteApiAdminPrincipalsByIdRolesByRoleNameData, ThrowOnError>) => import("./client").RequestResult<DeleteApiAdminPrincipalsByIdRolesByRoleNameResponses, DeleteApiAdminPrincipalsByIdRolesByRoleNameErrors, ThrowOnError, "fields">;
/**
 * List roles
 */
export declare const getApiAdminRoles: <ThrowOnError extends boolean = false>(options: Options<GetApiAdminRolesData, ThrowOnError>) => import("./client").RequestResult<GetApiAdminRolesResponses, unknown, ThrowOnError, "fields">;
/**
 * Create a new role
 */
export declare const postApiAdminRoles: <ThrowOnError extends boolean = false>(options: Options<PostApiAdminRolesData, ThrowOnError>) => import("./client").RequestResult<PostApiAdminRolesResponses, PostApiAdminRolesErrors, ThrowOnError, "fields">;
/**
 * Get roles by application ID
 */
export declare const getApiAdminRolesByApplicationByApplicationId: <ThrowOnError extends boolean = false>(options: Options<GetApiAdminRolesByApplicationByApplicationIdData, ThrowOnError>) => import("./client").RequestResult<GetApiAdminRolesByApplicationByApplicationIdResponses, unknown, ThrowOnError, "fields">;
/**
 * Get role by code (name)
 */
export declare const getApiAdminRolesByCodeByCode: <ThrowOnError extends boolean = false>(options: Options<GetApiAdminRolesByCodeByCodeData, ThrowOnError>) => import("./client").RequestResult<GetApiAdminRolesByCodeByCodeResponses, GetApiAdminRolesByCodeByCodeErrors, ThrowOnError, "fields">;
/**
 * Get roles by source (CODE, DATABASE, SDK)
 */
export declare const getApiAdminRolesBySourceBySource: <ThrowOnError extends boolean = false>(options: Options<GetApiAdminRolesBySourceBySourceData, ThrowOnError>) => import("./client").RequestResult<GetApiAdminRolesBySourceBySourceResponses, GetApiAdminRolesBySourceBySourceErrors, ThrowOnError, "fields">;
/**
 * Get applications for role filter dropdown
 */
export declare const getApiAdminRolesFiltersApplications: <ThrowOnError extends boolean = false>(options?: Options<GetApiAdminRolesFiltersApplicationsData, ThrowOnError>) => import("./client").RequestResult<GetApiAdminRolesFiltersApplicationsResponses, unknown, ThrowOnError, "fields">;
/**
 * List all permissions
 */
export declare const getApiAdminRolesPermissions: <ThrowOnError extends boolean = false>(options?: Options<GetApiAdminRolesPermissionsData, ThrowOnError>) => import("./client").RequestResult<GetApiAdminRolesPermissionsResponses, unknown, ThrowOnError, "fields">;
/**
 * Get permission by string
 */
export declare const getApiAdminRolesPermissionsByPermission: <ThrowOnError extends boolean = false>(options: Options<GetApiAdminRolesPermissionsByPermissionData, ThrowOnError>) => import("./client").RequestResult<GetApiAdminRolesPermissionsByPermissionResponses, GetApiAdminRolesPermissionsByPermissionErrors, ThrowOnError, "fields">;
/**
 * Delete role
 */
export declare const deleteApiAdminRolesByName: <ThrowOnError extends boolean = false>(options: Options<DeleteApiAdminRolesByNameData, ThrowOnError>) => import("./client").RequestResult<DeleteApiAdminRolesByNameResponses, DeleteApiAdminRolesByNameErrors, ThrowOnError, "fields">;
/**
 * Get role by ID or name (code)
 *
 * The frontend calls this with the role name (e.g., "platform:super-admin"),
 * so we try by code first if it contains ":", otherwise by ID.
 */
export declare const getApiAdminRolesByName: <ThrowOnError extends boolean = false>(options: Options<GetApiAdminRolesByNameData, ThrowOnError>) => import("./client").RequestResult<GetApiAdminRolesByNameResponses, GetApiAdminRolesByNameErrors, ThrowOnError, "fields">;
/**
 * Update role
 */
export declare const putApiAdminRolesByName: <ThrowOnError extends boolean = false>(options: Options<PutApiAdminRolesByNameData, ThrowOnError>) => import("./client").RequestResult<PutApiAdminRolesByNameResponses, PutApiAdminRolesByNameErrors, ThrowOnError, "fields">;
/**
 * Grant permission to role
 */
export declare const postApiAdminRolesByNamePermissions: <ThrowOnError extends boolean = false>(options: Options<PostApiAdminRolesByNamePermissionsData, ThrowOnError>) => import("./client").RequestResult<PostApiAdminRolesByNamePermissionsResponses, PostApiAdminRolesByNamePermissionsErrors, ThrowOnError, "fields">;
/**
 * Revoke permission from role
 */
export declare const deleteApiAdminRolesByNamePermissionsByPermission: <ThrowOnError extends boolean = false>(options: Options<DeleteApiAdminRolesByNamePermissionsByPermissionData, ThrowOnError>) => import("./client").RequestResult<DeleteApiAdminRolesByNamePermissionsByPermissionResponses, DeleteApiAdminRolesByNamePermissionsByPermissionErrors, ThrowOnError, "fields">;
/**
 * List subscriptions
 */
export declare const getApiAdminSubscriptions: <ThrowOnError extends boolean = false>(options: Options<GetApiAdminSubscriptionsData, ThrowOnError>) => import("./client").RequestResult<GetApiAdminSubscriptionsResponses, unknown, ThrowOnError, "fields">;
/**
 * Create a new subscription
 */
export declare const postApiAdminSubscriptions: <ThrowOnError extends boolean = false>(options: Options<PostApiAdminSubscriptionsData, ThrowOnError>) => import("./client").RequestResult<PostApiAdminSubscriptionsResponses, PostApiAdminSubscriptionsErrors, ThrowOnError, "fields">;
/**
 * Sync subscriptions
 */
export declare const postApiAdminSubscriptionsSync: <ThrowOnError extends boolean = false>(options: Options<PostApiAdminSubscriptionsSyncData, ThrowOnError>) => import("./client").RequestResult<PostApiAdminSubscriptionsSyncResponses, PostApiAdminSubscriptionsSyncErrors, ThrowOnError, "fields">;
/**
 * Delete subscription (archive)
 */
export declare const deleteApiAdminSubscriptionsById: <ThrowOnError extends boolean = false>(options: Options<DeleteApiAdminSubscriptionsByIdData, ThrowOnError>) => import("./client").RequestResult<DeleteApiAdminSubscriptionsByIdResponses, DeleteApiAdminSubscriptionsByIdErrors, ThrowOnError, "fields">;
/**
 * Get subscription by ID
 */
export declare const getApiAdminSubscriptionsById: <ThrowOnError extends boolean = false>(options: Options<GetApiAdminSubscriptionsByIdData, ThrowOnError>) => import("./client").RequestResult<GetApiAdminSubscriptionsByIdResponses, GetApiAdminSubscriptionsByIdErrors, ThrowOnError, "fields">;
/**
 * Update subscription
 */
export declare const putApiAdminSubscriptionsById: <ThrowOnError extends boolean = false>(options: Options<PutApiAdminSubscriptionsByIdData, ThrowOnError>) => import("./client").RequestResult<PutApiAdminSubscriptionsByIdResponses, PutApiAdminSubscriptionsByIdErrors, ThrowOnError, "fields">;
/**
 * Pause subscription
 */
export declare const postApiAdminSubscriptionsByIdPause: <ThrowOnError extends boolean = false>(options: Options<PostApiAdminSubscriptionsByIdPauseData, ThrowOnError>) => import("./client").RequestResult<PostApiAdminSubscriptionsByIdPauseResponses, PostApiAdminSubscriptionsByIdPauseErrors, ThrowOnError, "fields">;
/**
 * Resume subscription
 */
export declare const postApiAdminSubscriptionsByIdResume: <ThrowOnError extends boolean = false>(options: Options<PostApiAdminSubscriptionsByIdResumeData, ThrowOnError>) => import("./client").RequestResult<PostApiAdminSubscriptionsByIdResumeResponses, PostApiAdminSubscriptionsByIdResumeErrors, ThrowOnError, "fields">;
/**
 * Get circuit breaker states
 */
export declare const getApiAdminMonitoringCircuitBreakers: <ThrowOnError extends boolean = false>(options?: Options<GetApiAdminMonitoringCircuitBreakersData, ThrowOnError>) => import("./client").RequestResult<GetApiAdminMonitoringCircuitBreakersResponses, unknown, ThrowOnError, "fields">;
/**
 * Get dashboard metrics
 */
export declare const getApiAdminMonitoringDashboard: <ThrowOnError extends boolean = false>(options?: Options<GetApiAdminMonitoringDashboardData, ThrowOnError>) => import("./client").RequestResult<GetApiAdminMonitoringDashboardResponses, unknown, ThrowOnError, "fields">;
/**
 * Get in-flight messages
 */
export declare const getApiAdminMonitoringInFlightMessages: <ThrowOnError extends boolean = false>(options?: Options<GetApiAdminMonitoringInFlightMessagesData, ThrowOnError>) => import("./client").RequestResult<GetApiAdminMonitoringInFlightMessagesResponses, unknown, ThrowOnError, "fields">;
/**
 * Get pool statistics with enhanced metrics
 */
export declare const getApiAdminMonitoringPoolStats: <ThrowOnError extends boolean = false>(options?: Options<GetApiAdminMonitoringPoolStatsData, ThrowOnError>) => import("./client").RequestResult<GetApiAdminMonitoringPoolStatsResponses, unknown, ThrowOnError, "fields">;
/**
 * Get standby status
 */
export declare const getApiAdminMonitoringStandbyStatus: <ThrowOnError extends boolean = false>(options?: Options<GetApiAdminMonitoringStandbyStatusData, ThrowOnError>) => import("./client").RequestResult<GetApiAdminMonitoringStandbyStatusResponses, unknown, ThrowOnError, "fields">;
/**
 * Check email domain authentication method
 *
 * Determines how a user with the given email should authenticate:
 * - Internal: username/password
 * - OIDC: external identity provider
 *
 * This is called before showing the login form to determine
 * if the user should be redirected to an external IDP.
 */
export declare const getAuthCheckDomain: <ThrowOnError extends boolean = false>(options: Options<GetAuthCheckDomainData, ThrowOnError>) => import("./client").RequestResult<GetAuthCheckDomainResponses, unknown, ThrowOnError, "fields">;
/**
 * Login with email and password
 *
 * Authenticates a user with email and password credentials.
 * Returns an access token on success and sets a session cookie.
 */
export declare const postAuthLogin: <ThrowOnError extends boolean = false>(options: Options<PostAuthLoginData, ThrowOnError>) => import("./client").RequestResult<PostAuthLoginResponses, PostAuthLoginErrors, ThrowOnError, "fields">;
/**
 * Logout / revoke token
 *
 * Invalidates the current session by clearing the session cookie.
 */
export declare const postAuthLogout: <ThrowOnError extends boolean = false>(options?: Options<PostAuthLogoutData, ThrowOnError>) => import("./client").RequestResult<PostAuthLogoutResponses, unknown, ThrowOnError, "fields">;
/**
 * Get current user info
 *
 * Returns information about the currently authenticated user.
 */
export declare const getAuthMe: <ThrowOnError extends boolean = false>(options?: Options<GetAuthMeData, ThrowOnError>) => import("./client").RequestResult<GetAuthMeResponses, GetAuthMeErrors, ThrowOnError, "fields">;
/**
 * Refresh access token
 *
 * Exchange a refresh token for a new access token.
 * The refresh token is rotated (old one invalidated, new one issued).
 */
export declare const postAuthRefresh: <ThrowOnError extends boolean = false>(options: Options<PostAuthRefreshData, ThrowOnError>) => import("./client").RequestResult<PostAuthRefreshResponses, PostAuthRefreshErrors, ThrowOnError, "fields">;
/**
 * List dispatch jobs
 */
export declare const getApiAdminDispatchJobs: <ThrowOnError extends boolean = false>(options: Options<GetApiAdminDispatchJobsData, ThrowOnError>) => import("./client").RequestResult<GetApiAdminDispatchJobsResponses, unknown, ThrowOnError, "fields">;
/**
 * Create a new dispatch job
 *
 * Creates and queues a new dispatch job for webhook delivery.
 */
export declare const postApiAdminDispatchJobs: <ThrowOnError extends boolean = false>(options: Options<PostApiAdminDispatchJobsData, ThrowOnError>) => import("./client").RequestResult<PostApiAdminDispatchJobsResponses, PostApiAdminDispatchJobsErrors, ThrowOnError, "fields">;
/**
 * Create multiple dispatch jobs in batch
 *
 * Creates multiple dispatch jobs in a single operation. Maximum batch size is 100 jobs.
 */
export declare const postApiAdminDispatchJobsBatch: <ThrowOnError extends boolean = false>(options: Options<PostApiAdminDispatchJobsBatchData, ThrowOnError>) => import("./client").RequestResult<PostApiAdminDispatchJobsBatchResponses, PostApiAdminDispatchJobsBatchErrors, ThrowOnError, "fields">;
/**
 * Get dispatch jobs for an event
 */
export declare const getApiAdminDispatchJobsByEventByEventId: <ThrowOnError extends boolean = false>(options: Options<GetApiAdminDispatchJobsByEventByEventIdData, ThrowOnError>) => import("./client").RequestResult<GetApiAdminDispatchJobsByEventByEventIdResponses, unknown, ThrowOnError, "fields">;
/**
 * Get filter options for dispatch jobs
 *
 * Returns distinct values for filter dropdowns (statuses, modes, subscriptionIds, eventTypeCodes).
 */
export declare const getApiAdminDispatchJobsFilterOptions: <ThrowOnError extends boolean = false>(options?: Options<GetApiAdminDispatchJobsFilterOptionsData, ThrowOnError>) => import("./client").RequestResult<GetApiAdminDispatchJobsFilterOptionsResponses, unknown, ThrowOnError, "fields">;
/**
 * List raw dispatch jobs (from msg_dispatch_jobs, not read projection)
 */
export declare const getApiAdminDispatchJobsRaw: <ThrowOnError extends boolean = false>(options?: Options<GetApiAdminDispatchJobsRawData, ThrowOnError>) => import("./client").RequestResult<GetApiAdminDispatchJobsRawResponses, unknown, ThrowOnError, "fields">;
/**
 * Get dispatch job by ID
 */
export declare const getApiAdminDispatchJobsById: <ThrowOnError extends boolean = false>(options: Options<GetApiAdminDispatchJobsByIdData, ThrowOnError>) => import("./client").RequestResult<GetApiAdminDispatchJobsByIdResponses, GetApiAdminDispatchJobsByIdErrors, ThrowOnError, "fields">;
/**
 * Get all attempts for a dispatch job
 *
 * Retrieves the full history of webhook delivery attempts for a job.
 */
export declare const getApiAdminDispatchJobsByIdAttempts: <ThrowOnError extends boolean = false>(options: Options<GetApiAdminDispatchJobsByIdAttemptsData, ThrowOnError>) => import("./client").RequestResult<GetApiAdminDispatchJobsByIdAttemptsResponses, GetApiAdminDispatchJobsByIdAttemptsErrors, ThrowOnError, "fields">;
/**
 * Get raw dispatch job data by ID
 *
 * Returns the full DispatchJob entity serialized directly as JSON (not the DTO).
 */
export declare const getApiAdminDispatchJobsByIdRaw: <ThrowOnError extends boolean = false>(options: Options<GetApiAdminDispatchJobsByIdRawData, ThrowOnError>) => import("./client").RequestResult<GetApiAdminDispatchJobsByIdRawResponses, GetApiAdminDispatchJobsByIdRawErrors, ThrowOnError, "fields">;
/**
 * List events
 */
export declare const getApiAdminEvents: <ThrowOnError extends boolean = false>(options: Options<GetApiAdminEventsData, ThrowOnError>) => import("./client").RequestResult<GetApiAdminEventsResponses, unknown, ThrowOnError, "fields">;
/**
 * Create a new event
 *
 * Creates a new event in the event store. If a deduplicationId is provided and
 * an event with that ID already exists, the existing event is returned (idempotent operation).
 * Dispatch jobs are automatically created for matching subscriptions.
 */
export declare const postApiAdminEvents: <ThrowOnError extends boolean = false>(options: Options<PostApiAdminEventsData, ThrowOnError>) => import("./client").RequestResult<PostApiAdminEventsResponses, PostApiAdminEventsErrors, ThrowOnError, "fields">;
/**
 * Batch create events
 *
 * Creates multiple events in a single operation. Maximum batch size is 100 events.
 * Dispatch jobs are automatically created for matching subscriptions.
 * Events with duplicate deduplicationIds are returned from the existing store.
 */
export declare const postApiAdminEventsBatch: <ThrowOnError extends boolean = false>(options: Options<PostApiAdminEventsBatchData, ThrowOnError>) => import("./client").RequestResult<PostApiAdminEventsBatchResponses, PostApiAdminEventsBatchErrors, ThrowOnError, "fields">;
/**
 * List raw events (from msg_events, not read projection)
 */
export declare const getApiAdminEventsRaw: <ThrowOnError extends boolean = false>(options?: Options<GetApiAdminEventsRawData, ThrowOnError>) => import("./client").RequestResult<GetApiAdminEventsRawResponses, unknown, ThrowOnError, "fields">;
/**
 * Get event by ID
 */
export declare const getApiAdminEventsById: <ThrowOnError extends boolean = false>(options: Options<GetApiAdminEventsByIdData, ThrowOnError>) => import("./client").RequestResult<GetApiAdminEventsByIdResponses, GetApiAdminEventsByIdErrors, ThrowOnError, "fields">;
/**
 * Get all filter options at once
 */
export declare const getApiAdminFilterOptions: <ThrowOnError extends boolean = false>(options?: Options<GetApiAdminFilterOptionsData, ThrowOnError>) => import("./client").RequestResult<GetApiAdminFilterOptionsResponses, unknown, ThrowOnError, "fields">;
/**
 * Get client filter options
 */
export declare const getApiAdminFilterOptionsClients: <ThrowOnError extends boolean = false>(options?: Options<GetApiAdminFilterOptionsClientsData, ThrowOnError>) => import("./client").RequestResult<GetApiAdminFilterOptionsClientsResponses, unknown, ThrowOnError, "fields">;
/**
 * Get dispatch jobs filter options
 */
export declare const getApiAdminDispatchJobsFilterOptions2: <ThrowOnError extends boolean = false>(options?: Options<GetApiAdminDispatchJobsFilterOptions2Data, ThrowOnError>) => import("./client").RequestResult<GetApiAdminDispatchJobsFilterOptions2Responses, unknown, ThrowOnError, "fields">;
/**
 * Get dispatch pool filter options
 */
export declare const getApiAdminFilterOptionsDispatchPools: <ThrowOnError extends boolean = false>(options?: Options<GetApiAdminFilterOptionsDispatchPoolsData, ThrowOnError>) => import("./client").RequestResult<GetApiAdminFilterOptionsDispatchPoolsResponses, unknown, ThrowOnError, "fields">;
/**
 * Get event type filter options
 */
export declare const getApiAdminFilterOptionsEventTypes: <ThrowOnError extends boolean = false>(options?: Options<GetApiAdminFilterOptionsEventTypesData, ThrowOnError>) => import("./client").RequestResult<GetApiAdminFilterOptionsEventTypesResponses, unknown, ThrowOnError, "fields">;
/**
 * Get aggregates for event type cascading filter (filtered by application and subdomain)
 */
export declare const getApiAdminFilterOptionsEventTypesFiltersAggregates: <ThrowOnError extends boolean = false>(options?: Options<GetApiAdminFilterOptionsEventTypesFiltersAggregatesData, ThrowOnError>) => import("./client").RequestResult<GetApiAdminFilterOptionsEventTypesFiltersAggregatesResponses, unknown, ThrowOnError, "fields">;
/**
 * Get applications for event type cascading filter
 */
export declare const getApiAdminFilterOptionsEventTypesFiltersApplications: <ThrowOnError extends boolean = false>(options?: Options<GetApiAdminFilterOptionsEventTypesFiltersApplicationsData, ThrowOnError>) => import("./client").RequestResult<GetApiAdminFilterOptionsEventTypesFiltersApplicationsResponses, unknown, ThrowOnError, "fields">;
/**
 * Get subdomains for event type cascading filter (filtered by application)
 */
export declare const getApiAdminFilterOptionsEventTypesFiltersSubdomains: <ThrowOnError extends boolean = false>(options?: Options<GetApiAdminFilterOptionsEventTypesFiltersSubdomainsData, ThrowOnError>) => import("./client").RequestResult<GetApiAdminFilterOptionsEventTypesFiltersSubdomainsResponses, unknown, ThrowOnError, "fields">;
/**
 * Get events filter options (cascading)
 */
export declare const getApiAdminEventsFilterOptions: <ThrowOnError extends boolean = false>(options?: Options<GetApiAdminEventsFilterOptionsData, ThrowOnError>) => import("./client").RequestResult<GetApiAdminEventsFilterOptionsResponses, unknown, ThrowOnError, "fields">;
/**
 * Get subscription filter options
 */
export declare const getApiAdminFilterOptionsSubscriptions: <ThrowOnError extends boolean = false>(options?: Options<GetApiAdminFilterOptionsSubscriptionsData, ThrowOnError>) => import("./client").RequestResult<GetApiAdminFilterOptionsSubscriptionsResponses, unknown, ThrowOnError, "fields">;
//# sourceMappingURL=sdk.gen.d.ts.map