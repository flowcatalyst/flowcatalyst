<?php

namespace FlowCatalyst\Generated;

class Client extends \FlowCatalyst\Generated\Runtime\Client\Client
{
    /**
     * @param array{
     *    "page"?: int, //Page number (0-based, matches Java)
     *    "pageSize"?: int, //Page size (default 50)
     *    "entityType"?: string, //Filter by entity type
     *    "entityId"?: string, //Filter by entity ID
     *    "operation"?: string, //Filter by operation (Java calls this "operation", maps to action internally)
     *    "principalId"?: string, //Filter by principal ID
     * } $queryParameters
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\AuditLogListResponse : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiAdminAuditLogs(array $queryParameters = [], string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiAdminAuditLogs($queryParameters), $fetch);
    }
    /**
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApplicationIdsResponse : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiAdminAuditLogsApplicationIds(string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiAdminAuditLogsApplicationIds(), $fetch);
    }
    /**
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ClientIdsResponse : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiAdminAuditLogsClientIds(string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiAdminAuditLogsClientIds(), $fetch);
    }
    /**
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\EntityTypesResponse : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiAdminAuditLogsEntityTypes(string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiAdminAuditLogsEntityTypes(), $fetch);
    }
    /**
     * @param string $entityType Entity type
     * @param string $entityId Entity ID
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\EntityAuditLogsResponse : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiAdminAuditLogsEntityByEntityTypeByEntityId(string $entityType, string $entityId, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiAdminAuditLogsEntityByEntityTypeByEntityId($entityType, $entityId), $fetch);
    }
    /**
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\OperationsResponse : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiAdminAuditLogsOperations(string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiAdminAuditLogsOperations(), $fetch);
    }
    /**
     * @param string $principalId Principal ID
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\AuditLogResponse[] : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiAdminAuditLogsPrincipalByPrincipalId(string $principalId, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiAdminAuditLogsPrincipalByPrincipalId($principalId), $fetch);
    }
    /**
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\AuditLogResponse[] : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiAdminAuditLogsRecent(string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiAdminAuditLogsRecent(), $fetch);
    }
    /**
     * @param string $id Audit log ID
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\GetApiAdminAuditLogsByIdNotFoundException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\AuditLogDetailResponse : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiAdminAuditLogsById(string $id, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiAdminAuditLogsById($id), $fetch);
    }
    /**
     * @param array{
     *    "page"?: int, //Page number
     *    "limit"?: int, //Items per page
     *    "status"?: string, //Filter by status
     * } $queryParameters
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ClientListResponse : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiAdminClients(array $queryParameters = [], string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiAdminClients($queryParameters), $fetch);
    }
    /**
     * @param null|\FlowCatalyst\Generated\Model\CreateClientRequest $requestBody
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\PostApiAdminClientsBadRequestException
     * @throws \FlowCatalyst\Generated\Exception\PostApiAdminClientsConflictException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ClientResponse : \Psr\Http\Message\ResponseInterface)
     */
    public function postApiAdminClients(?\FlowCatalyst\Generated\Model\CreateClientRequest $requestBody = null, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\PostApiAdminClients($requestBody), $fetch);
    }
    /**
     * @param string $identifier Client identifier/slug
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\GetApiAdminClientsByIdentifierByIdentifierNotFoundException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ClientResponse : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiAdminClientsByIdentifierByIdentifier(string $identifier, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiAdminClientsByIdentifierByIdentifier($identifier), $fetch);
    }
    /**
     * @param array{
     *    "q"?: string, //Search term
     * } $queryParameters
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ClientListResponse : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiAdminClientsSearch(array $queryParameters = [], string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiAdminClientsSearch($queryParameters), $fetch);
    }
    /**
     * @param string $id Client ID
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\DeleteApiAdminClientsByIdNotFoundException
     *
     * @return ($fetch is 'object' ? null : \Psr\Http\Message\ResponseInterface)
     */
    public function deleteApiAdminClientsById(string $id, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\DeleteApiAdminClientsById($id), $fetch);
    }
    /**
     * @param string $id Client ID
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\GetApiAdminClientsByIdNotFoundException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ClientResponse : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiAdminClientsById(string $id, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiAdminClientsById($id), $fetch);
    }
    /**
     * @param string $id Client ID
     * @param null|\FlowCatalyst\Generated\Model\UpdateClientRequest $requestBody
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\PutApiAdminClientsByIdNotFoundException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ClientResponse : \Psr\Http\Message\ResponseInterface)
     */
    public function putApiAdminClientsById(string $id, ?\FlowCatalyst\Generated\Model\UpdateClientRequest $requestBody = null, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\PutApiAdminClientsById($id, $requestBody), $fetch);
    }
    /**
     * Transitions a suspended or pending client to active status.
     * @param string $id Client ID
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\PostApiAdminClientsByIdActivateForbiddenException
     * @throws \FlowCatalyst\Generated\Exception\PostApiAdminClientsByIdActivateNotFoundException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\StatusChangeResponse : \Psr\Http\Message\ResponseInterface)
     */
    public function postApiAdminClientsByIdActivate(string $id, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\PostApiAdminClientsByIdActivate($id), $fetch);
    }
    /**
     * @param string $id Client ID
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\GetApiAdminClientsByIdApplicationsNotFoundException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ClientApplicationsResponse : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiAdminClientsByIdApplications(string $id, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiAdminClientsByIdApplications($id), $fetch);
    }
    /**
     * @param string $id Client ID
     * @param null|\FlowCatalyst\Generated\Model\UpdateClientApplicationsRequest $requestBody
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\PutApiAdminClientsByIdApplicationsNotFoundException
     *
     * @return ($fetch is 'object' ? null : \Psr\Http\Message\ResponseInterface)
     */
    public function putApiAdminClientsByIdApplications(string $id, ?\FlowCatalyst\Generated\Model\UpdateClientApplicationsRequest $requestBody = null, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\PutApiAdminClientsByIdApplications($id, $requestBody), $fetch);
    }
    /**
     * @param string $id Client ID
     * @param string $applicationId Application ID
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\PostApiAdminClientsByIdApplicationsByAppIdDisableNotFoundException
     *
     * @return ($fetch is 'object' ? null : \Psr\Http\Message\ResponseInterface)
     */
    public function postApiAdminClientsByIdApplicationsByAppIdDisable(string $id, string $applicationId, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\PostApiAdminClientsByIdApplicationsByAppIdDisable($id, $applicationId), $fetch);
    }
    /**
     * @param string $id Client ID
     * @param string $applicationId Application ID
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\PostApiAdminClientsByIdApplicationsByAppIdEnableNotFoundException
     *
     * @return ($fetch is 'object' ? null : \Psr\Http\Message\ResponseInterface)
     */
    public function postApiAdminClientsByIdApplicationsByAppIdEnable(string $id, string $applicationId, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\PostApiAdminClientsByIdApplicationsByAppIdEnable($id, $applicationId), $fetch);
    }
    /**
     * Deactivates/soft-deletes a client. Requires a reason.
     * @param string $id Client ID
     * @param null|\FlowCatalyst\Generated\Model\StatusChangeRequest $requestBody
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\PostApiAdminClientsByIdDeactivateForbiddenException
     * @throws \FlowCatalyst\Generated\Exception\PostApiAdminClientsByIdDeactivateNotFoundException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\StatusChangeResponse : \Psr\Http\Message\ResponseInterface)
     */
    public function postApiAdminClientsByIdDeactivate(string $id, ?\FlowCatalyst\Generated\Model\StatusChangeRequest $requestBody = null, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\PostApiAdminClientsByIdDeactivate($id, $requestBody), $fetch);
    }
    /**
     * @param string $id Client ID
     * @param null|\FlowCatalyst\Generated\Model\AddNoteRequest $requestBody
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\PostApiAdminClientsByIdNotesNotFoundException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\AddNoteResponse : \Psr\Http\Message\ResponseInterface)
     */
    public function postApiAdminClientsByIdNotes(string $id, ?\FlowCatalyst\Generated\Model\AddNoteRequest $requestBody = null, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\PostApiAdminClientsByIdNotes($id, $requestBody), $fetch);
    }
    /**
     * Suspends a client (e.g., for billing issues). Requires a reason.
     * @param string $id Client ID
     * @param null|\FlowCatalyst\Generated\Model\StatusChangeRequest $requestBody
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\PostApiAdminClientsByIdSuspendForbiddenException
     * @throws \FlowCatalyst\Generated\Exception\PostApiAdminClientsByIdSuspendNotFoundException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\StatusChangeResponse : \Psr\Http\Message\ResponseInterface)
     */
    public function postApiAdminClientsByIdSuspend(string $id, ?\FlowCatalyst\Generated\Model\StatusChangeRequest $requestBody = null, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\PostApiAdminClientsByIdSuspend($id, $requestBody), $fetch);
    }
    /**
     * @param array{
     *    "pagination": array,
     *    "application"?: string, //Filter by application
     *    "clientId"?: string, //Filter by client ID
     *    "status"?: string, //Filter by status
     * } $queryParameters
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\EventTypeListResponse : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiAdminEventTypes(array $queryParameters = [], string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiAdminEventTypes($queryParameters), $fetch);
    }
    /**
     * @param null|\FlowCatalyst\Generated\Model\CreateEventTypeRequest $requestBody
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\PostApiAdminEventTypesBadRequestException
     * @throws \FlowCatalyst\Generated\Exception\PostApiAdminEventTypesConflictException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\EventTypeResponse : \Psr\Http\Message\ResponseInterface)
     */
    public function postApiAdminEventTypes(?\FlowCatalyst\Generated\Model\CreateEventTypeRequest $requestBody = null, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\PostApiAdminEventTypes($requestBody), $fetch);
    }
    /**
     * @param string $code Event type code
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\GetApiAdminEventTypesByCodeByCodeNotFoundException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\EventTypeResponse : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiAdminEventTypesByCodeByCode(string $code, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiAdminEventTypesByCodeByCode($code), $fetch);
    }
    /**
     * @param null|\FlowCatalyst\Generated\Model\SyncEventTypesRequest $requestBody
     * @param array{
     *    "removeUnlisted"?: bool, //Remove items not in the sync list
     * } $queryParameters
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\PostApiAdminEventTypesSyncBadRequestException
     * @throws \FlowCatalyst\Generated\Exception\PostApiAdminEventTypesSyncNotFoundException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\SyncResultResponse : \Psr\Http\Message\ResponseInterface)
     */
    public function postApiAdminEventTypesSync(?\FlowCatalyst\Generated\Model\SyncEventTypesRequest $requestBody = null, array $queryParameters = [], string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\PostApiAdminEventTypesSync($requestBody, $queryParameters), $fetch);
    }
    /**
     * @param string $id Event type ID
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\DeleteApiAdminEventTypesByIdNotFoundException
     *
     * @return ($fetch is 'object' ? null : \Psr\Http\Message\ResponseInterface)
     */
    public function deleteApiAdminEventTypesById(string $id, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\DeleteApiAdminEventTypesById($id), $fetch);
    }
    /**
     * @param string $id Event type ID
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\GetApiAdminEventTypesByIdNotFoundException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\EventTypeResponse : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiAdminEventTypesById(string $id, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiAdminEventTypesById($id), $fetch);
    }
    /**
     * @param string $id Event type ID
     * @param null|\FlowCatalyst\Generated\Model\UpdateEventTypeRequest $requestBody
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\PutApiAdminEventTypesByIdNotFoundException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\EventTypeResponse : \Psr\Http\Message\ResponseInterface)
     */
    public function putApiAdminEventTypesById(string $id, ?\FlowCatalyst\Generated\Model\UpdateEventTypeRequest $requestBody = null, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\PutApiAdminEventTypesById($id, $requestBody), $fetch);
    }
    /**
     * @param string $id Event type ID
     * @param null|\FlowCatalyst\Generated\Model\AddSchemaVersionRequest $requestBody
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\PostApiAdminEventTypesByIdSchemasNotFoundException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\EventTypeResponse : \Psr\Http\Message\ResponseInterface)
     */
    public function postApiAdminEventTypesByIdSchemas(string $id, ?\FlowCatalyst\Generated\Model\AddSchemaVersionRequest $requestBody = null, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\PostApiAdminEventTypesByIdSchemas($id, $requestBody), $fetch);
    }
    /**
     * @param array{
     *    "pagination": array,
     *    "active"?: bool, //Filter by active status
     * } $queryParameters
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\OAuthClientResponse[] : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiAdminOauthClients(array $queryParameters = [], string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiAdminOauthClients($queryParameters), $fetch);
    }
    /**
     * @param null|\FlowCatalyst\Generated\Model\CreateOAuthClientRequest $requestBody
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\PostApiAdminOauthClientsBadRequestException
     * @throws \FlowCatalyst\Generated\Exception\PostApiAdminOauthClientsConflictException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\OAuthClientResponse : \Psr\Http\Message\ResponseInterface)
     */
    public function postApiAdminOauthClients(?\FlowCatalyst\Generated\Model\CreateOAuthClientRequest $requestBody = null, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\PostApiAdminOauthClients($requestBody), $fetch);
    }
    /**
     * @param string $clientId OAuth client_id (public identifier)
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\GetApiAdminOauthClientsByClientIdNotFoundException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\OAuthClientResponse : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiAdminOauthClientsByClientId(string $clientId, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiAdminOauthClientsByClientId($clientId), $fetch);
    }
    /**
     * @param string $id OAuth client ID
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\DeleteApiAdminOauthClientsByIdNotFoundException
     *
     * @return ($fetch is 'object' ? null : \Psr\Http\Message\ResponseInterface)
     */
    public function deleteApiAdminOauthClientsById(string $id, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\DeleteApiAdminOauthClientsById($id), $fetch);
    }
    /**
     * @param string $id OAuth client ID
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\GetApiAdminOauthClientsByIdNotFoundException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\OAuthClientResponse : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiAdminOauthClientsById(string $id, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiAdminOauthClientsById($id), $fetch);
    }
    /**
     * @param string $id OAuth client ID
     * @param null|\FlowCatalyst\Generated\Model\UpdateOAuthClientRequest $requestBody
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\PutApiAdminOauthClientsByIdNotFoundException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\OAuthClientResponse : \Psr\Http\Message\ResponseInterface)
     */
    public function putApiAdminOauthClientsById(string $id, ?\FlowCatalyst\Generated\Model\UpdateOAuthClientRequest $requestBody = null, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\PutApiAdminOauthClientsById($id, $requestBody), $fetch);
    }
    /**
     * @param string $id OAuth client ID
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\PostApiAdminOauthClientsActivateNotFoundException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\SuccessResponse : \Psr\Http\Message\ResponseInterface)
     */
    public function postApiAdminOauthClientsActivate(string $id, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\PostApiAdminOauthClientsActivate($id), $fetch);
    }
    /**
     * @param string $id OAuth client ID
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\PostApiAdminOauthClientsDeactivateNotFoundException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\SuccessResponse : \Psr\Http\Message\ResponseInterface)
     */
    public function postApiAdminOauthClientsDeactivate(string $id, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\PostApiAdminOauthClientsDeactivate($id), $fetch);
    }
    /**
     * @param string $id OAuth client ID
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\PostApiAdminOauthClientsRegenerateSecretNotFoundException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\RegenerateSecretResponse : \Psr\Http\Message\ResponseInterface)
     */
    public function postApiAdminOauthClientsRegenerateSecret(string $id, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\PostApiAdminOauthClientsRegenerateSecret($id), $fetch);
    }
    /**
     * @param string $id OAuth client ID
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\PostApiAdminOauthClientsRotateSecretNotFoundException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\RegenerateSecretResponse : \Psr\Http\Message\ResponseInterface)
     */
    public function postApiAdminOauthClientsRotateSecret(string $id, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\PostApiAdminOauthClientsRotateSecret($id), $fetch);
    }
    /**
     * @param array{
     *    "page"?: int, //Page number
     *    "limit"?: int, //Items per page
     *    "type"?: string, //Filter by type
     *    "scope"?: string, //Filter by scope
     *    "client_id"?: string, //Filter by client ID
     * } $queryParameters
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\PrincipalListResponse : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiAdminPrincipals(array $queryParameters = [], string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiAdminPrincipals($queryParameters), $fetch);
    }
    /**
     * @param null|\FlowCatalyst\Generated\Model\CreateUserRequest $requestBody
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\PostApiAdminPrincipalsUsersBadRequestException
     * @throws \FlowCatalyst\Generated\Exception\PostApiAdminPrincipalsUsersConflictException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\CreatedResponse : \Psr\Http\Message\ResponseInterface)
     */
    public function postApiAdminPrincipalsUsers(?\FlowCatalyst\Generated\Model\CreateUserRequest $requestBody = null, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\PostApiAdminPrincipalsUsers($requestBody), $fetch);
    }
    /**
     * @param array{
     *    "domain": string, //Email domain to check
     * } $queryParameters
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\CheckEmailDomainResponse : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiAdminPrincipalsCheckEmailDomain(array $queryParameters = [], string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiAdminPrincipalsCheckEmailDomain($queryParameters), $fetch);
    }
    /**
     * @param string $id Principal ID
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\DeleteApiAdminPrincipalsByIdNotFoundException
     *
     * @return ($fetch is 'object' ? null : \Psr\Http\Message\ResponseInterface)
     */
    public function deleteApiAdminPrincipalsById(string $id, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\DeleteApiAdminPrincipalsById($id), $fetch);
    }
    /**
     * @param string $id Principal ID
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\GetApiAdminPrincipalsByIdNotFoundException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\PrincipalResponse : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiAdminPrincipalsById(string $id, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiAdminPrincipalsById($id), $fetch);
    }
    /**
     * @param string $id Principal ID
     * @param null|\FlowCatalyst\Generated\Model\UpdatePrincipalRequest $requestBody
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\PutApiAdminPrincipalsByIdNotFoundException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\PrincipalResponse : \Psr\Http\Message\ResponseInterface)
     */
    public function putApiAdminPrincipalsById(string $id, ?\FlowCatalyst\Generated\Model\UpdatePrincipalRequest $requestBody = null, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\PutApiAdminPrincipalsById($id, $requestBody), $fetch);
    }
    /**
     * Reactivates a deactivated principal.
     * @param string $id Principal ID
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\PostApiAdminPrincipalsByIdActivateForbiddenException
     * @throws \FlowCatalyst\Generated\Exception\PostApiAdminPrincipalsByIdActivateNotFoundException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\StatusChangeResponse : \Psr\Http\Message\ResponseInterface)
     */
    public function postApiAdminPrincipalsByIdActivate(string $id, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\PostApiAdminPrincipalsByIdActivate($id), $fetch);
    }
    /**
     * Returns all applications the principal has been granted access to.
     * @param string $id Principal ID
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\GetApiAdminPrincipalsByIdApplicationAccessNotFoundException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApplicationAccessListResponse : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiAdminPrincipalsByIdApplicationAccess(string $id, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiAdminPrincipalsByIdApplicationAccess($id), $fetch);
    }
    /**
     * Replaces all application access with the provided list.
     * @param string $id Principal ID
     * @param null|\FlowCatalyst\Generated\Model\SetApplicationAccessRequest $requestBody
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\PutApiAdminPrincipalsByIdApplicationAccessNotFoundException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\SetApplicationAccessResponse : \Psr\Http\Message\ResponseInterface)
     */
    public function putApiAdminPrincipalsByIdApplicationAccess(string $id, ?\FlowCatalyst\Generated\Model\SetApplicationAccessRequest $requestBody = null, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\PutApiAdminPrincipalsByIdApplicationAccess($id, $requestBody), $fetch);
    }
    /**
     * ANCHOR users see all active applications.
     * CLIENT users see only applications enabled for their accessible client configs.
     * @param string $id Principal ID
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\GetApiAdminPrincipalsByIdAvailableApplicationsNotFoundException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\AvailableApplicationsResponse : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiAdminPrincipalsByIdAvailableApplications(string $id, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiAdminPrincipalsByIdAvailableApplications($id), $fetch);
    }
    /**
     * @param string $id Principal ID
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\GetApiAdminPrincipalsByIdClientAccessNotFoundException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ClientAccessListResponse : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiAdminPrincipalsByIdClientAccess(string $id, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiAdminPrincipalsByIdClientAccess($id), $fetch);
    }
    /**
     * @param string $id Principal ID
     * @param null|\FlowCatalyst\Generated\Model\GrantClientAccessRequest $requestBody
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\PostApiAdminPrincipalsByIdClientAccessNotFoundException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ClientAccessGrantResponse : \Psr\Http\Message\ResponseInterface)
     */
    public function postApiAdminPrincipalsByIdClientAccess(string $id, ?\FlowCatalyst\Generated\Model\GrantClientAccessRequest $requestBody = null, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\PostApiAdminPrincipalsByIdClientAccess($id, $requestBody), $fetch);
    }
    /**
     * @param string $id Principal ID
     * @param string $clientId Client ID to revoke
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\DeleteApiAdminPrincipalsByIdClientAccessByClientIdNotFoundException
     *
     * @return ($fetch is 'object' ? null : \Psr\Http\Message\ResponseInterface)
     */
    public function deleteApiAdminPrincipalsByIdClientAccessByClientId(string $id, string $clientId, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\DeleteApiAdminPrincipalsByIdClientAccessByClientId($id, $clientId), $fetch);
    }
    /**
     * Deactivates an active principal.
     * @param string $id Principal ID
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\PostApiAdminPrincipalsByIdDeactivateForbiddenException
     * @throws \FlowCatalyst\Generated\Exception\PostApiAdminPrincipalsByIdDeactivateNotFoundException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\StatusChangeResponse : \Psr\Http\Message\ResponseInterface)
     */
    public function postApiAdminPrincipalsByIdDeactivate(string $id, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\PostApiAdminPrincipalsByIdDeactivate($id), $fetch);
    }
    /**
     * Resets the password for an internal auth user. Does not work for OIDC users.
     * @param string $id Principal ID
     * @param null|\FlowCatalyst\Generated\Model\ResetPasswordRequest $requestBody
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\PostApiAdminPrincipalsByIdResetPasswordBadRequestException
     * @throws \FlowCatalyst\Generated\Exception\PostApiAdminPrincipalsByIdResetPasswordForbiddenException
     * @throws \FlowCatalyst\Generated\Exception\PostApiAdminPrincipalsByIdResetPasswordNotFoundException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\StatusChangeResponse : \Psr\Http\Message\ResponseInterface)
     */
    public function postApiAdminPrincipalsByIdResetPassword(string $id, ?\FlowCatalyst\Generated\Model\ResetPasswordRequest $requestBody = null, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\PostApiAdminPrincipalsByIdResetPassword($id, $requestBody), $fetch);
    }
    /**
     * @param string $id Principal ID
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\GetApiAdminPrincipalsByIdRolesNotFoundException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\RolesListResponse : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiAdminPrincipalsByIdRoles(string $id, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiAdminPrincipalsByIdRoles($id), $fetch);
    }
    /**
     * @param string $id Principal ID
     * @param null|\FlowCatalyst\Generated\Model\AssignRoleRequest $requestBody
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\PostApiAdminPrincipalsByIdRolesNotFoundException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\PrincipalResponse : \Psr\Http\Message\ResponseInterface)
     */
    public function postApiAdminPrincipalsByIdRoles(string $id, ?\FlowCatalyst\Generated\Model\AssignRoleRequest $requestBody = null, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\PostApiAdminPrincipalsByIdRoles($id, $requestBody), $fetch);
    }
    /**
     * @param string $id Principal ID
     * @param null|\FlowCatalyst\Generated\Model\BatchAssignRolesRequest $requestBody
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\PutApiAdminPrincipalsByIdRolesNotFoundException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\BatchAssignRolesResponse : \Psr\Http\Message\ResponseInterface)
     */
    public function putApiAdminPrincipalsByIdRoles(string $id, ?\FlowCatalyst\Generated\Model\BatchAssignRolesRequest $requestBody = null, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\PutApiAdminPrincipalsByIdRoles($id, $requestBody), $fetch);
    }
    /**
     * @param string $id Principal ID
     * @param string $role Role to remove
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\DeleteApiAdminPrincipalsByIdRolesByRoleNameNotFoundException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\PrincipalResponse : \Psr\Http\Message\ResponseInterface)
     */
    public function deleteApiAdminPrincipalsByIdRolesByRoleName(string $id, string $role, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\DeleteApiAdminPrincipalsByIdRolesByRoleName($id, $role), $fetch);
    }
    /**
     * @param array{
     *    "pagination": array,
     *    "applicationCode"?: string, //Filter by application code
     *    "source"?: string, //Filter by source
     *    "clientManaged"?: bool, //Filter client-managed roles only
     * } $queryParameters
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\RoleListResponse : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiAdminRoles(array $queryParameters = [], string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiAdminRoles($queryParameters), $fetch);
    }
    /**
     * @param null|\FlowCatalyst\Generated\Model\CreateRoleRequest $requestBody
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\PostApiAdminRolesBadRequestException
     * @throws \FlowCatalyst\Generated\Exception\PostApiAdminRolesConflictException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\RoleResponse : \Psr\Http\Message\ResponseInterface)
     */
    public function postApiAdminRoles(?\FlowCatalyst\Generated\Model\CreateRoleRequest $requestBody = null, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\PostApiAdminRoles($requestBody), $fetch);
    }
    /**
     * @param string $applicationId Application ID
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\RoleResponse[] : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiAdminRolesByApplicationByApplicationId(string $applicationId, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiAdminRolesByApplicationByApplicationId($applicationId), $fetch);
    }
    /**
     * @param string $code Role code
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\GetApiAdminRolesByCodeByCodeNotFoundException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\RoleResponse : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiAdminRolesByCodeByCode(string $code, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiAdminRolesByCodeByCode($code), $fetch);
    }
    /**
     * @param string $source Role source (CODE, DATABASE, SDK)
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\GetApiAdminRolesBySourceBySourceBadRequestException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\RoleResponse[] : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiAdminRolesBySourceBySource(string $source, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiAdminRolesBySourceBySource($source), $fetch);
    }
    /**
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApplicationOptionsResponse : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiAdminRolesFiltersApplications(string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiAdminRolesFiltersApplications(), $fetch);
    }
    /**
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\PermissionListResponse : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiAdminRolesPermissions(string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiAdminRolesPermissions(), $fetch);
    }
    /**
     * @param string $permission Permission string
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\GetApiAdminRolesPermissionsByPermissionNotFoundException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\PermissionResponse : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiAdminRolesPermissionsByPermission(string $permission, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiAdminRolesPermissionsByPermission($permission), $fetch);
    }
    /**
     * @param string $roleName Role name (code) or ID
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\DeleteApiAdminRolesByNameNotFoundException
     *
     * @return ($fetch is 'object' ? null : \Psr\Http\Message\ResponseInterface)
     */
    public function deleteApiAdminRolesByName(string $roleName, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\DeleteApiAdminRolesByName($roleName), $fetch);
    }
    /**
     * The frontend calls this with the role name (e.g., "platform:super-admin"),
     * so we try by code first if it contains ":", otherwise by ID.
     * @param string $roleName Role name (code) or ID
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\GetApiAdminRolesByNameNotFoundException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\RoleResponse : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiAdminRolesByName(string $roleName, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiAdminRolesByName($roleName), $fetch);
    }
    /**
     * @param string $roleName Role name (code) or ID
     * @param null|\FlowCatalyst\Generated\Model\UpdateRoleRequest $requestBody
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\PutApiAdminRolesByNameNotFoundException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\RoleResponse : \Psr\Http\Message\ResponseInterface)
     */
    public function putApiAdminRolesByName(string $roleName, ?\FlowCatalyst\Generated\Model\UpdateRoleRequest $requestBody = null, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\PutApiAdminRolesByName($roleName, $requestBody), $fetch);
    }
    /**
     * @param string $roleName Role name (code) or ID
     * @param null|\FlowCatalyst\Generated\Model\GrantPermissionRequest $requestBody
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\PostApiAdminRolesByNamePermissionsNotFoundException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\RoleResponse : \Psr\Http\Message\ResponseInterface)
     */
    public function postApiAdminRolesByNamePermissions(string $roleName, ?\FlowCatalyst\Generated\Model\GrantPermissionRequest $requestBody = null, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\PostApiAdminRolesByNamePermissions($roleName, $requestBody), $fetch);
    }
    /**
     * @param string $roleName Role name (code) or ID
     * @param string $permission Permission to revoke
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\DeleteApiAdminRolesByNamePermissionsByPermissionNotFoundException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\RoleResponse : \Psr\Http\Message\ResponseInterface)
     */
    public function deleteApiAdminRolesByNamePermissionsByPermission(string $roleName, string $permission, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\DeleteApiAdminRolesByNamePermissionsByPermission($roleName, $permission), $fetch);
    }
    /**
     * @param array{
     *    "pagination": array,
     *    "clientId"?: string, //Filter by client ID
     *    "status"?: string, //Filter by status
     * } $queryParameters
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\SubscriptionListResponse : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiAdminSubscriptions(array $queryParameters = [], string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiAdminSubscriptions($queryParameters), $fetch);
    }
    /**
     * @param null|\FlowCatalyst\Generated\Model\CreateSubscriptionRequest $requestBody
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\PostApiAdminSubscriptionsBadRequestException
     * @throws \FlowCatalyst\Generated\Exception\PostApiAdminSubscriptionsConflictException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\SubscriptionResponse : \Psr\Http\Message\ResponseInterface)
     */
    public function postApiAdminSubscriptions(?\FlowCatalyst\Generated\Model\CreateSubscriptionRequest $requestBody = null, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\PostApiAdminSubscriptions($requestBody), $fetch);
    }
    /**
     * @param null|\FlowCatalyst\Generated\Model\SyncSubscriptionsRequest $requestBody
     * @param array{
     *    "removeUnlisted"?: bool, //Remove items not in the sync list
     * } $queryParameters
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\PostApiAdminSubscriptionsSyncBadRequestException
     * @throws \FlowCatalyst\Generated\Exception\PostApiAdminSubscriptionsSyncNotFoundException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\SyncResultResponse : \Psr\Http\Message\ResponseInterface)
     */
    public function postApiAdminSubscriptionsSync(?\FlowCatalyst\Generated\Model\SyncSubscriptionsRequest $requestBody = null, array $queryParameters = [], string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\PostApiAdminSubscriptionsSync($requestBody, $queryParameters), $fetch);
    }
    /**
     * @param string $id Subscription ID
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\DeleteApiAdminSubscriptionsByIdNotFoundException
     *
     * @return ($fetch is 'object' ? null : \Psr\Http\Message\ResponseInterface)
     */
    public function deleteApiAdminSubscriptionsById(string $id, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\DeleteApiAdminSubscriptionsById($id), $fetch);
    }
    /**
     * @param string $id Subscription ID
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\GetApiAdminSubscriptionsByIdNotFoundException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\SubscriptionResponse : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiAdminSubscriptionsById(string $id, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiAdminSubscriptionsById($id), $fetch);
    }
    /**
     * @param string $id Subscription ID
     * @param null|\FlowCatalyst\Generated\Model\UpdateSubscriptionRequest $requestBody
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\PutApiAdminSubscriptionsByIdNotFoundException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\SubscriptionResponse : \Psr\Http\Message\ResponseInterface)
     */
    public function putApiAdminSubscriptionsById(string $id, ?\FlowCatalyst\Generated\Model\UpdateSubscriptionRequest $requestBody = null, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\PutApiAdminSubscriptionsById($id, $requestBody), $fetch);
    }
    /**
     * @param string $id Subscription ID
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\PostApiAdminSubscriptionsByIdPauseNotFoundException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\SubscriptionResponse : \Psr\Http\Message\ResponseInterface)
     */
    public function postApiAdminSubscriptionsByIdPause(string $id, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\PostApiAdminSubscriptionsByIdPause($id), $fetch);
    }
    /**
     * @param string $id Subscription ID
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\PostApiAdminSubscriptionsByIdResumeNotFoundException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\SubscriptionResponse : \Psr\Http\Message\ResponseInterface)
     */
    public function postApiAdminSubscriptionsByIdResume(string $id, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\PostApiAdminSubscriptionsByIdResume($id), $fetch);
    }
    /**
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\CircuitBreakersResponse : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiAdminMonitoringCircuitBreakers(string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiAdminMonitoringCircuitBreakers(), $fetch);
    }
    /**
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\DashboardMetrics : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiAdminMonitoringDashboard(string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiAdminMonitoringDashboard(), $fetch);
    }
    /**
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\InFlightMessagesResponse : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiAdminMonitoringInFlightMessages(string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiAdminMonitoringInFlightMessages(), $fetch);
    }
    /**
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\PoolStatsResponse : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiAdminMonitoringPoolStats(string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiAdminMonitoringPoolStats(), $fetch);
    }
    /**
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\StandbyStatus : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiAdminMonitoringStandbyStatus(string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiAdminMonitoringStandbyStatus(), $fetch);
    }
    /**
     * Determines how a user with the given email should authenticate:
     * - Internal: username/password
     * - OIDC: external identity provider
     *
     * This is called before showing the login form to determine
     * if the user should be redirected to an external IDP.
     * @param array{
     *    "email": string, //Email address to check
     * } $queryParameters
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\DomainCheckResponse : \Psr\Http\Message\ResponseInterface)
     */
    public function getAuthCheckDomain(array $queryParameters = [], string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetAuthCheckDomain($queryParameters), $fetch);
    }
    /**
     * Authenticates a user with email and password credentials.
     * Returns an access token on success and sets a session cookie.
     * @param null|\FlowCatalyst\Generated\Model\LoginRequest $requestBody
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\PostAuthLoginUnauthorizedException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\LoginResponse : \Psr\Http\Message\ResponseInterface)
     */
    public function postAuthLogin(?\FlowCatalyst\Generated\Model\LoginRequest $requestBody = null, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\PostAuthLogin($requestBody), $fetch);
    }
    /**
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     *
     * @return ($fetch is 'object' ? null : \Psr\Http\Message\ResponseInterface)
     */
    public function postAuthLogout(string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\PostAuthLogout(), $fetch);
    }
    /**
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\GetAuthMeUnauthorizedException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\CurrentUserResponse : \Psr\Http\Message\ResponseInterface)
     */
    public function getAuthMe(string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetAuthMe(), $fetch);
    }
    /**
     * Exchange a refresh token for a new access token.
     * The refresh token is rotated (old one invalidated, new one issued).
     * @param null|\FlowCatalyst\Generated\Model\RefreshTokenRequest $requestBody
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\PostAuthRefreshUnauthorizedException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\TokenRefreshResponse : \Psr\Http\Message\ResponseInterface)
     */
    public function postAuthRefresh(?\FlowCatalyst\Generated\Model\RefreshTokenRequest $requestBody = null, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\PostAuthRefresh($requestBody), $fetch);
    }
    /**
     * @param array{
     *    "pagination": array,
     *    "eventId"?: string, //Filter by event ID
     *    "correlationId"?: string, //Filter by correlation ID
     *    "subscriptionId"?: string, //Filter by subscription ID
     *    "clientId"?: string, //Filter by client ID
     *    "status"?: string, //Filter by status
     * } $queryParameters
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\DispatchJobResponse[] : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiAdminDispatchJobs(array $queryParameters = [], string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiAdminDispatchJobs($queryParameters), $fetch);
    }
    /**
     * Creates and queues a new dispatch job for webhook delivery.
     * @param null|\FlowCatalyst\Generated\Model\CreateDispatchJobRequest $requestBody
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\PostApiAdminDispatchJobsBadRequestException
     * @throws \FlowCatalyst\Generated\Exception\PostApiAdminDispatchJobsForbiddenException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\DispatchJobResponse : \Psr\Http\Message\ResponseInterface)
     */
    public function postApiAdminDispatchJobs(?\FlowCatalyst\Generated\Model\CreateDispatchJobRequest $requestBody = null, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\PostApiAdminDispatchJobs($requestBody), $fetch);
    }
    /**
     * Creates multiple dispatch jobs in a single operation. Maximum batch size is 100 jobs.
     * @param null|\FlowCatalyst\Generated\Model\BatchCreateDispatchJobsRequest $requestBody
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\PostApiAdminDispatchJobsBatchBadRequestException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\BatchCreateDispatchJobsResponse : \Psr\Http\Message\ResponseInterface)
     */
    public function postApiAdminDispatchJobsBatch(?\FlowCatalyst\Generated\Model\BatchCreateDispatchJobsRequest $requestBody = null, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\PostApiAdminDispatchJobsBatch($requestBody), $fetch);
    }
    /**
     * @param string $eventId Event ID
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\DispatchJobResponse[] : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiAdminDispatchJobsByEventByEventId(string $eventId, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiAdminDispatchJobsByEventByEventId($eventId), $fetch);
    }
    /**
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\DispatchJobFilterOptionsResponse : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiAdminDispatchJobsFilterOptions(string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiAdminDispatchJobsFilterOptions(), $fetch);
    }
    /**
     * @param array{
     *    "page"?: int,
     *    "size"?: int,
     * } $queryParameters
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\PaginatedDispatchJobsResponse : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiAdminDispatchJobsRaw(array $queryParameters = [], string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiAdminDispatchJobsRaw($queryParameters), $fetch);
    }
    /**
     * @param string $id Dispatch job ID
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\GetApiAdminDispatchJobsByIdNotFoundException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\DispatchJobResponse : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiAdminDispatchJobsById(string $id, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiAdminDispatchJobsById($id), $fetch);
    }
    /**
     * Retrieves the full history of webhook delivery attempts for a job.
     * @param string $id Dispatch job ID
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\GetApiAdminDispatchJobsByIdAttemptsNotFoundException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\DispatchAttemptResponse[] : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiAdminDispatchJobsByIdAttempts(string $id, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiAdminDispatchJobsByIdAttempts($id), $fetch);
    }
    /**
     * Returns the full DispatchJob entity serialized directly as JSON (not the DTO).
     * @param string $id Dispatch job ID
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\GetApiAdminDispatchJobsByIdRawNotFoundException
     *
     * @return ($fetch is 'object' ? null : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiAdminDispatchJobsByIdRaw(string $id, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiAdminDispatchJobsByIdRaw($id), $fetch);
    }
    /**
     * @param array{
     *    "pagination": array,
     *    "eventType"?: string, //Filter by event type
     *    "correlationId"?: string, //Filter by correlation ID
     *    "clientId"?: string, //Filter by client ID
     * } $queryParameters
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\EventResponse[] : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiAdminEvents(array $queryParameters = [], string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiAdminEvents($queryParameters), $fetch);
    }
    /**
     * Creates a new event in the event store. If a deduplicationId is provided and
     * an event with that ID already exists, the existing event is returned (idempotent operation).
     * Dispatch jobs are automatically created for matching subscriptions.
     * @param null|\FlowCatalyst\Generated\Model\CreateEventRequest $requestBody
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\PostApiAdminEventsBadRequestException
     * @throws \FlowCatalyst\Generated\Exception\PostApiAdminEventsForbiddenException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\CreateEventResponse : \Psr\Http\Message\ResponseInterface)
     */
    public function postApiAdminEvents(?\FlowCatalyst\Generated\Model\CreateEventRequest $requestBody = null, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\PostApiAdminEvents($requestBody), $fetch);
    }
    /**
     * Creates multiple events in a single operation. Maximum batch size is 100 events.
     * Dispatch jobs are automatically created for matching subscriptions.
     * Events with duplicate deduplicationIds are returned from the existing store.
     * @param null|\FlowCatalyst\Generated\Model\BatchCreateEventsRequest $requestBody
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\PostApiAdminEventsBatchBadRequestException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\BatchCreateResponse : \Psr\Http\Message\ResponseInterface)
     */
    public function postApiAdminEventsBatch(?\FlowCatalyst\Generated\Model\BatchCreateEventsRequest $requestBody = null, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\PostApiAdminEventsBatch($requestBody), $fetch);
    }
    /**
     * @param array{
     *    "page"?: int,
     *    "size"?: int,
     * } $queryParameters
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\PaginatedEventsResponse : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiAdminEventsRaw(array $queryParameters = [], string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiAdminEventsRaw($queryParameters), $fetch);
    }
    /**
     * @param string $id Event ID
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\GetApiAdminEventsByIdNotFoundException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\EventResponse : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiAdminEventsById(string $id, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiAdminEventsById($id), $fetch);
    }
    /**
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\AllFilterOptions : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiAdminFilterOptions(string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiAdminFilterOptions(), $fetch);
    }
    /**
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ClientFilterOptions : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiAdminFilterOptionsClients(string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiAdminFilterOptionsClients(), $fetch);
    }
    /**
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\DispatchJobsFilterOptions : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiAdminDispatchJobsFilterOptions(string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiAdminDispatchJobsFilterOptions(), $fetch);
    }
    /**
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\DispatchPoolFilterOptions : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiAdminFilterOptionsDispatchPools(string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiAdminFilterOptionsDispatchPools(), $fetch);
    }
    /**
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\EventTypeFilterOptions : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiAdminFilterOptionsEventTypes(string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiAdminFilterOptionsEventTypes(), $fetch);
    }
    /**
     * @param array{
     *    "application[]"?: array, //Filter by application(s)
     *    "subdomain[]"?: array, //Filter by subdomain(s)
     * } $queryParameters
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\AggregatesResponse : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiAdminFilterOptionsEventTypesFiltersAggregates(array $queryParameters = [], string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiAdminFilterOptionsEventTypesFiltersAggregates($queryParameters), $fetch);
    }
    /**
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApplicationsResponse : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiAdminFilterOptionsEventTypesFiltersApplications(string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiAdminFilterOptionsEventTypesFiltersApplications(), $fetch);
    }
    /**
     * @param array{
     *    "application[]"?: array, //Filter by application(s)
     *    "subdomain[]"?: array, //Filter by subdomain(s)
     * } $queryParameters
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\SubdomainsResponse : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiAdminFilterOptionsEventTypesFiltersSubdomains(array $queryParameters = [], string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiAdminFilterOptionsEventTypesFiltersSubdomains($queryParameters), $fetch);
    }
    /**
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\EventsFilterOptions : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiAdminEventsFilterOptions(string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiAdminEventsFilterOptions(), $fetch);
    }
    /**
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\SubscriptionFilterOptions : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiAdminFilterOptionsSubscriptions(string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiAdminFilterOptionsSubscriptions(), $fetch);
    }
    public static function create($httpClient = null, array $additionalPlugins = [], array $additionalNormalizers = [])
    {
        if (null === $httpClient) {
            $httpClient = \Http\Discovery\Psr18ClientDiscovery::find();
            $plugins = [];
            if (count($additionalPlugins) > 0) {
                $plugins = array_merge($plugins, $additionalPlugins);
            }
            $httpClient = new \Http\Client\Common\PluginClient($httpClient, $plugins);
        }
        $requestFactory = \Http\Discovery\Psr17FactoryDiscovery::findRequestFactory();
        $streamFactory = \Http\Discovery\Psr17FactoryDiscovery::findStreamFactory();
        $normalizers = [new \Symfony\Component\Serializer\Normalizer\ArrayDenormalizer(), new \FlowCatalyst\Generated\Normalizer\JaneObjectNormalizer()];
        if (count($additionalNormalizers) > 0) {
            $normalizers = array_merge($normalizers, $additionalNormalizers);
        }
        $serializer = new \Symfony\Component\Serializer\Serializer($normalizers, [new \Symfony\Component\Serializer\Encoder\JsonEncoder(new \Symfony\Component\Serializer\Encoder\JsonEncode(), new \Symfony\Component\Serializer\Encoder\JsonDecode(['json_decode_associative' => true]))]);
        return new static($httpClient, $requestFactory, $serializer, $streamFactory);
    }
}