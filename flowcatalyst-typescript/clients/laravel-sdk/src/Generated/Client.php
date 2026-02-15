<?php

namespace FlowCatalyst\Generated;

class Client extends \FlowCatalyst\Generated\Runtime\Client\Client
{
    /**
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     *
     * @return ($fetch is 'object' ? null : \Psr\Http\Message\ResponseInterface)
     */
    public function getWellKnownJwk(string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetWellKnownJwk(), $fetch);
    }
    /**
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     *
     * @return ($fetch is 'object' ? null : \Psr\Http\Message\ResponseInterface)
     */
    public function getWellKnownOpenidConfiguration(string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetWellKnownOpenidConfiguration(), $fetch);
    }
    /**
     * @param array{
     *    "activeOnly"?: bool,
     *    "type"?: string,
     * } $queryParameters
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\ListApplicationsBadRequestException
     * @throws \FlowCatalyst\Generated\Exception\ListApplicationsUnauthorizedException
     * @throws \FlowCatalyst\Generated\Exception\ListApplicationsForbiddenException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApplicationListResponse : \Psr\Http\Message\ResponseInterface)
     */
    public function listApplications(array $queryParameters = [], string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\ListApplications($queryParameters), $fetch);
    }
    /**
     * @param null|\FlowCatalyst\Generated\Model\CreateApplicationRequest $requestBody
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\CreateApplicationBadRequestException
     * @throws \FlowCatalyst\Generated\Exception\CreateApplicationConflictException
     * @throws \FlowCatalyst\Generated\Exception\CreateApplicationUnauthorizedException
     * @throws \FlowCatalyst\Generated\Exception\CreateApplicationForbiddenException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApplicationResponse : \Psr\Http\Message\ResponseInterface)
     */
    public function createApplication(?\FlowCatalyst\Generated\Model\CreateApplicationRequest $requestBody = null, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\CreateApplication($requestBody), $fetch);
    }
    /**
     * @param string $code
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\GetApplicationByCodeNotFoundException
     * @throws \FlowCatalyst\Generated\Exception\GetApplicationByCodeUnauthorizedException
     * @throws \FlowCatalyst\Generated\Exception\GetApplicationByCodeForbiddenException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApplicationResponse : \Psr\Http\Message\ResponseInterface)
     */
    public function getApplicationByCode(string $code, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApplicationByCode($code), $fetch);
    }
    /**
     * Permanently deletes an application. The application must be deactivated first.
     * @param string $id
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\DeleteApplicationBadRequestException
     * @throws \FlowCatalyst\Generated\Exception\DeleteApplicationNotFoundException
     * @throws \FlowCatalyst\Generated\Exception\DeleteApplicationUnauthorizedException
     * @throws \FlowCatalyst\Generated\Exception\DeleteApplicationForbiddenException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApplicationStatusResponse : \Psr\Http\Message\ResponseInterface)
     */
    public function deleteApplication(string $id, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\DeleteApplication($id), $fetch);
    }
    /**
     * @param string $id
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\GetApplicationNotFoundException
     * @throws \FlowCatalyst\Generated\Exception\GetApplicationUnauthorizedException
     * @throws \FlowCatalyst\Generated\Exception\GetApplicationForbiddenException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApplicationResponse : \Psr\Http\Message\ResponseInterface)
     */
    public function getApplication(string $id, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApplication($id), $fetch);
    }
    /**
     * @param string $id
     * @param null|\FlowCatalyst\Generated\Model\UpdateApplicationRequest $requestBody
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\UpdateApplicationBadRequestException
     * @throws \FlowCatalyst\Generated\Exception\UpdateApplicationNotFoundException
     * @throws \FlowCatalyst\Generated\Exception\UpdateApplicationUnauthorizedException
     * @throws \FlowCatalyst\Generated\Exception\UpdateApplicationForbiddenException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApplicationResponse : \Psr\Http\Message\ResponseInterface)
     */
    public function updateApplication(string $id, ?\FlowCatalyst\Generated\Model\UpdateApplicationRequest $requestBody = null, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\UpdateApplication($id, $requestBody), $fetch);
    }
    /**
     * @param string $id
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\ActivateApplicationBadRequestException
     * @throws \FlowCatalyst\Generated\Exception\ActivateApplicationNotFoundException
     * @throws \FlowCatalyst\Generated\Exception\ActivateApplicationUnauthorizedException
     * @throws \FlowCatalyst\Generated\Exception\ActivateApplicationForbiddenException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApplicationStatusResponse : \Psr\Http\Message\ResponseInterface)
     */
    public function activateApplication(string $id, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\ActivateApplication($id), $fetch);
    }
    /**
     * @param string $id
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\GetApplicationClientConfigsNotFoundException
     * @throws \FlowCatalyst\Generated\Exception\GetApplicationClientConfigsUnauthorizedException
     * @throws \FlowCatalyst\Generated\Exception\GetApplicationClientConfigsForbiddenException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ClientConfigListResponse : \Psr\Http\Message\ResponseInterface)
     */
    public function getApplicationClientConfigs(string $id, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApplicationClientConfigs($id), $fetch);
    }
    /**
     * @param string $clientId
     * @param string $id
     * @param null|\FlowCatalyst\Generated\Model\ClientConfigRequest $requestBody
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\ConfigureApplicationForClientBadRequestException
     * @throws \FlowCatalyst\Generated\Exception\ConfigureApplicationForClientNotFoundException
     * @throws \FlowCatalyst\Generated\Exception\ConfigureApplicationForClientUnauthorizedException
     * @throws \FlowCatalyst\Generated\Exception\ConfigureApplicationForClientForbiddenException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ClientConfigResponse : \Psr\Http\Message\ResponseInterface)
     */
    public function configureApplicationForClient(string $clientId, string $id, ?\FlowCatalyst\Generated\Model\ClientConfigRequest $requestBody = null, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\ConfigureApplicationForClient($clientId, $id, $requestBody), $fetch);
    }
    /**
     * @param string $clientId
     * @param string $id
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\DisableApplicationForClientBadRequestException
     * @throws \FlowCatalyst\Generated\Exception\DisableApplicationForClientNotFoundException
     * @throws \FlowCatalyst\Generated\Exception\DisableApplicationForClientUnauthorizedException
     * @throws \FlowCatalyst\Generated\Exception\DisableApplicationForClientForbiddenException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ClientApplicationStatusResponse : \Psr\Http\Message\ResponseInterface)
     */
    public function disableApplicationForClient(string $clientId, string $id, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\DisableApplicationForClient($clientId, $id), $fetch);
    }
    /**
     * @param string $clientId
     * @param string $id
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\EnableApplicationForClientBadRequestException
     * @throws \FlowCatalyst\Generated\Exception\EnableApplicationForClientNotFoundException
     * @throws \FlowCatalyst\Generated\Exception\EnableApplicationForClientUnauthorizedException
     * @throws \FlowCatalyst\Generated\Exception\EnableApplicationForClientForbiddenException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ClientApplicationStatusResponse : \Psr\Http\Message\ResponseInterface)
     */
    public function enableApplicationForClient(string $clientId, string $id, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\EnableApplicationForClient($clientId, $id), $fetch);
    }
    /**
     * @param string $id
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\DeactivateApplicationBadRequestException
     * @throws \FlowCatalyst\Generated\Exception\DeactivateApplicationNotFoundException
     * @throws \FlowCatalyst\Generated\Exception\DeactivateApplicationUnauthorizedException
     * @throws \FlowCatalyst\Generated\Exception\DeactivateApplicationForbiddenException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApplicationStatusResponse : \Psr\Http\Message\ResponseInterface)
     */
    public function deactivateApplication(string $id, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\DeactivateApplication($id), $fetch);
    }
    /**
     * Creates a service account and OAuth client for an application that doesn't have one. The client secret is only returned once and cannot be retrieved later.
     * @param string $id
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\ProvisionApplicationServiceAccountBadRequestException
     * @throws \FlowCatalyst\Generated\Exception\ProvisionApplicationServiceAccountNotFoundException
     * @throws \FlowCatalyst\Generated\Exception\ProvisionApplicationServiceAccountUnauthorizedException
     * @throws \FlowCatalyst\Generated\Exception\ProvisionApplicationServiceAccountForbiddenException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ProvisionServiceAccountResponse : \Psr\Http\Message\ResponseInterface)
     */
    public function provisionApplicationServiceAccount(string $id, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\ProvisionApplicationServiceAccount($id), $fetch);
    }
    /**
     * @param string $id
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\GetApplicationRolesNotFoundException
     * @throws \FlowCatalyst\Generated\Exception\GetApplicationRolesUnauthorizedException
     * @throws \FlowCatalyst\Generated\Exception\GetApplicationRolesForbiddenException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApplicationRolesResponse : \Psr\Http\Message\ResponseInterface)
     */
    public function getApplicationRoles(string $id, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApplicationRoles($id), $fetch);
    }
    /**
     * Returns audit logs with optional filtering by entity type, entity ID, principal, or operation
     * @param array{
     *    "entityId"?: string, //Filter by entity ID
     *    "entityType"?: string, //Filter by entity type (e.g., 'IdentityProvider', 'Role')
     *    "operation"?: string, //Filter by operation name
     *    "page"?: int, //Page number (0-based)
     *    "pageSize"?: int, //Page size
     *    "principalId"?: string, //Filter by principal ID
     * } $queryParameters
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\ListAuditLogsUnauthorizedException
     * @throws \FlowCatalyst\Generated\Exception\ListAuditLogsForbiddenException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\AuditLogListResponse : \Psr\Http\Message\ResponseInterface)
     */
    public function listAuditLogs(array $queryParameters = [], string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\ListAuditLogs($queryParameters), $fetch);
    }
    /**
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\GetAuditLogEntityTypesUnauthorizedException
     * @throws \FlowCatalyst\Generated\Exception\GetAuditLogEntityTypesForbiddenException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\EntityTypesResponse : \Psr\Http\Message\ResponseInterface)
     */
    public function getAuditLogEntityTypes(string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetAuditLogEntityTypes(), $fetch);
    }
    /**
     * Returns all audit logs for a specific entity
     * @param string $entityId
     * @param string $entityType
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\GetEntityAuditLogsUnauthorizedException
     * @throws \FlowCatalyst\Generated\Exception\GetEntityAuditLogsForbiddenException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\EntityAuditLogsResponse : \Psr\Http\Message\ResponseInterface)
     */
    public function getEntityAuditLogs(string $entityId, string $entityType, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetEntityAuditLogs($entityId, $entityType), $fetch);
    }
    /**
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\GetAuditLogOperationsUnauthorizedException
     * @throws \FlowCatalyst\Generated\Exception\GetAuditLogOperationsForbiddenException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\OperationsResponse : \Psr\Http\Message\ResponseInterface)
     */
    public function getAuditLogOperations(string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetAuditLogOperations(), $fetch);
    }
    /**
     * @param string $id
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\GetAuditLogNotFoundException
     * @throws \FlowCatalyst\Generated\Exception\GetAuditLogUnauthorizedException
     * @throws \FlowCatalyst\Generated\Exception\GetAuditLogForbiddenException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\AuditLogDetailDto : \Psr\Http\Message\ResponseInterface)
     */
    public function getAuditLog(string $id, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetAuditLog($id), $fetch);
    }
    /**
     * Returns all clients regardless of status
     * @param array{
     *    "status"?: string, //Filter by status
     * } $queryParameters
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\ListClientsUnauthorizedException
     * @throws \FlowCatalyst\Generated\Exception\ListClientsForbiddenException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ClientListResponse : \Psr\Http\Message\ResponseInterface)
     */
    public function listClients(array $queryParameters = [], string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\ListClients($queryParameters), $fetch);
    }
    /**
     * @param null|\FlowCatalyst\Generated\Model\CreateClientRequest $requestBody
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\CreateClientBadRequestException
     * @throws \FlowCatalyst\Generated\Exception\CreateClientUnauthorizedException
     * @throws \FlowCatalyst\Generated\Exception\CreateClientForbiddenException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ClientDto : \Psr\Http\Message\ResponseInterface)
     */
    public function createClient(?\FlowCatalyst\Generated\Model\CreateClientRequest $requestBody = null, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\CreateClient($requestBody), $fetch);
    }
    /**
     * @param string $identifier
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\GetClientByIdentifierNotFoundException
     * @throws \FlowCatalyst\Generated\Exception\GetClientByIdentifierUnauthorizedException
     * @throws \FlowCatalyst\Generated\Exception\GetClientByIdentifierForbiddenException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ClientDto : \Psr\Http\Message\ResponseInterface)
     */
    public function getClientByIdentifier(string $identifier, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetClientByIdentifier($identifier), $fetch);
    }
    /**
     * Search clients by name or identifier
     * @param array{
     *    "limit"?: int, //Max results to return
     *    "q"?: string, //Search query (name or identifier)
     *    "status"?: string, //Filter by status
     * } $queryParameters
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\SearchClientsUnauthorizedException
     * @throws \FlowCatalyst\Generated\Exception\SearchClientsForbiddenException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ClientListResponse : \Psr\Http\Message\ResponseInterface)
     */
    public function searchClients(array $queryParameters = [], string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\SearchClients($queryParameters), $fetch);
    }
    /**
     * @param string $id
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\GetClientBadRequestException
     * @throws \FlowCatalyst\Generated\Exception\GetClientNotFoundException
     * @throws \FlowCatalyst\Generated\Exception\GetClientUnauthorizedException
     * @throws \FlowCatalyst\Generated\Exception\GetClientForbiddenException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ClientDto : \Psr\Http\Message\ResponseInterface)
     */
    public function getClient(string $id, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetClient($id), $fetch);
    }
    /**
     * @param string $id
     * @param null|\FlowCatalyst\Generated\Model\UpdateClientRequest $requestBody
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\UpdateClientBadRequestException
     * @throws \FlowCatalyst\Generated\Exception\UpdateClientNotFoundException
     * @throws \FlowCatalyst\Generated\Exception\UpdateClientUnauthorizedException
     * @throws \FlowCatalyst\Generated\Exception\UpdateClientForbiddenException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ClientDto : \Psr\Http\Message\ResponseInterface)
     */
    public function updateClient(string $id, ?\FlowCatalyst\Generated\Model\UpdateClientRequest $requestBody = null, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\UpdateClient($id, $requestBody), $fetch);
    }
    /**
     * @param string $id
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\ActivateClientBadRequestException
     * @throws \FlowCatalyst\Generated\Exception\ActivateClientNotFoundException
     * @throws \FlowCatalyst\Generated\Exception\ActivateClientUnauthorizedException
     * @throws \FlowCatalyst\Generated\Exception\ActivateClientForbiddenException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ClientStatusResponse : \Psr\Http\Message\ResponseInterface)
     */
    public function activateClient(string $id, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\ActivateClient($id), $fetch);
    }
    /**
     * Returns all applications with their enabled/disabled status for this client
     * @param string $id
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\GetClientApplicationsBadRequestException
     * @throws \FlowCatalyst\Generated\Exception\GetClientApplicationsNotFoundException
     * @throws \FlowCatalyst\Generated\Exception\GetClientApplicationsUnauthorizedException
     * @throws \FlowCatalyst\Generated\Exception\GetClientApplicationsForbiddenException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ClientApplicationsResponse : \Psr\Http\Message\ResponseInterface)
     */
    public function getClientApplications(string $id, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetClientApplications($id), $fetch);
    }
    /**
     * Sets which applications are enabled for this client
     * @param string $id
     * @param null|\FlowCatalyst\Generated\Model\UpdateClientApplicationsRequest $requestBody
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\UpdateClientApplicationsBadRequestException
     * @throws \FlowCatalyst\Generated\Exception\UpdateClientApplicationsNotFoundException
     * @throws \FlowCatalyst\Generated\Exception\UpdateClientApplicationsUnauthorizedException
     * @throws \FlowCatalyst\Generated\Exception\UpdateClientApplicationsForbiddenException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApplicationsUpdatedResponse : \Psr\Http\Message\ResponseInterface)
     */
    public function updateClientApplications(string $id, ?\FlowCatalyst\Generated\Model\UpdateClientApplicationsRequest $requestBody = null, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\UpdateClientApplications($id, $requestBody), $fetch);
    }
    /**
     * @param string $applicationId
     * @param string $id
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\DisableClientApplicationBadRequestException
     * @throws \FlowCatalyst\Generated\Exception\DisableClientApplicationNotFoundException
     * @throws \FlowCatalyst\Generated\Exception\DisableClientApplicationUnauthorizedException
     * @throws \FlowCatalyst\Generated\Exception\DisableClientApplicationForbiddenException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ClientApplicationStatusResponse1 : \Psr\Http\Message\ResponseInterface)
     */
    public function disableClientApplication(string $applicationId, string $id, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\DisableClientApplication($applicationId, $id), $fetch);
    }
    /**
     * @param string $applicationId
     * @param string $id
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\EnableClientApplicationBadRequestException
     * @throws \FlowCatalyst\Generated\Exception\EnableClientApplicationNotFoundException
     * @throws \FlowCatalyst\Generated\Exception\EnableClientApplicationUnauthorizedException
     * @throws \FlowCatalyst\Generated\Exception\EnableClientApplicationForbiddenException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ClientApplicationStatusResponse1 : \Psr\Http\Message\ResponseInterface)
     */
    public function enableClientApplication(string $applicationId, string $id, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\EnableClientApplication($applicationId, $id), $fetch);
    }
    /**
     * @param string $id
     * @param null|\FlowCatalyst\Generated\Model\StatusChangeRequest $requestBody
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\DeactivateClientBadRequestException
     * @throws \FlowCatalyst\Generated\Exception\DeactivateClientNotFoundException
     * @throws \FlowCatalyst\Generated\Exception\DeactivateClientUnauthorizedException
     * @throws \FlowCatalyst\Generated\Exception\DeactivateClientForbiddenException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ClientStatusResponse : \Psr\Http\Message\ResponseInterface)
     */
    public function deactivateClient(string $id, ?\FlowCatalyst\Generated\Model\StatusChangeRequest $requestBody = null, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\DeactivateClient($id, $requestBody), $fetch);
    }
    /**
     * @param string $id
     * @param null|\FlowCatalyst\Generated\Model\AddNoteRequest $requestBody
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\AddClientAuditNoteBadRequestException
     * @throws \FlowCatalyst\Generated\Exception\AddClientAuditNoteNotFoundException
     * @throws \FlowCatalyst\Generated\Exception\AddClientAuditNoteUnauthorizedException
     * @throws \FlowCatalyst\Generated\Exception\AddClientAuditNoteForbiddenException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\NoteAddedResponse : \Psr\Http\Message\ResponseInterface)
     */
    public function addClientAuditNote(string $id, ?\FlowCatalyst\Generated\Model\AddNoteRequest $requestBody = null, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\AddClientAuditNote($id, $requestBody), $fetch);
    }
    /**
     * @param string $id
     * @param null|\FlowCatalyst\Generated\Model\StatusChangeRequest $requestBody
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\SuspendClientBadRequestException
     * @throws \FlowCatalyst\Generated\Exception\SuspendClientNotFoundException
     * @throws \FlowCatalyst\Generated\Exception\SuspendClientUnauthorizedException
     * @throws \FlowCatalyst\Generated\Exception\SuspendClientForbiddenException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ClientStatusResponse : \Psr\Http\Message\ResponseInterface)
     */
    public function suspendClient(string $id, ?\FlowCatalyst\Generated\Model\StatusChangeRequest $requestBody = null, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\SuspendClient($id, $requestBody), $fetch);
    }
    /**
     * @param string $appCode
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\GetApiAdminConfigAccessByAppCodeForbiddenException
     *
     * @return ($fetch is 'object' ? null : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiAdminConfigAccessByAppCode(string $appCode, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiAdminConfigAccessByAppCode($appCode), $fetch);
    }
    /**
     * @param string $appCode
     * @param null|\FlowCatalyst\Generated\Model\GrantAccessRequest $requestBody
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\PostApiAdminConfigAccessByAppCodeBadRequestException
     * @throws \FlowCatalyst\Generated\Exception\PostApiAdminConfigAccessByAppCodeForbiddenException
     * @throws \FlowCatalyst\Generated\Exception\PostApiAdminConfigAccessByAppCodeConflictException
     *
     * @return ($fetch is 'object' ? null : \Psr\Http\Message\ResponseInterface)
     */
    public function postApiAdminConfigAccessByAppCode(string $appCode, ?\FlowCatalyst\Generated\Model\GrantAccessRequest $requestBody = null, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\PostApiAdminConfigAccessByAppCode($appCode, $requestBody), $fetch);
    }
    /**
     * @param string $appCode
     * @param string $roleCode
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\DeleteApiAdminConfigAccessByAppCodeByRoleCodeForbiddenException
     * @throws \FlowCatalyst\Generated\Exception\DeleteApiAdminConfigAccessByAppCodeByRoleCodeNotFoundException
     *
     * @return ($fetch is 'object' ? null : \Psr\Http\Message\ResponseInterface)
     */
    public function deleteApiAdminConfigAccessByAppCodeByRoleCode(string $appCode, string $roleCode, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\DeleteApiAdminConfigAccessByAppCodeByRoleCode($appCode, $roleCode), $fetch);
    }
    /**
     * @param string $appCode
     * @param string $roleCode
     * @param null|\FlowCatalyst\Generated\Model\UpdateAccessRequest $requestBody
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\PutApiAdminConfigAccessByAppCodeByRoleCodeForbiddenException
     * @throws \FlowCatalyst\Generated\Exception\PutApiAdminConfigAccessByAppCodeByRoleCodeNotFoundException
     * @throws \FlowCatalyst\Generated\Exception\PutApiAdminConfigAccessByAppCodeByRoleCodeBadRequestException
     *
     * @return ($fetch is 'object' ? null : \Psr\Http\Message\ResponseInterface)
     */
    public function putApiAdminConfigAccessByAppCodeByRoleCode(string $appCode, string $roleCode, ?\FlowCatalyst\Generated\Model\UpdateAccessRequest $requestBody = null, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\PutApiAdminConfigAccessByAppCodeByRoleCode($appCode, $roleCode, $requestBody), $fetch);
    }
    /**
     * @param string $appCode
     * @param array{
     *    "clientId"?: string,
     *    "scope"?: string,
     * } $queryParameters
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\GetApiAdminConfigByAppCodeForbiddenException
     *
     * @return ($fetch is 'object' ? null : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiAdminConfigByAppCode(string $appCode, array $queryParameters = [], string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiAdminConfigByAppCode($appCode, $queryParameters), $fetch);
    }
    /**
     * @param string $appCode
     * @param string $section
     * @param array{
     *    "clientId"?: string,
     *    "scope"?: string,
     * } $queryParameters
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\GetApiAdminConfigByAppCodeBySectionForbiddenException
     *
     * @return ($fetch is 'object' ? null : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiAdminConfigByAppCodeBySection(string $appCode, string $section, array $queryParameters = [], string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiAdminConfigByAppCodeBySection($appCode, $section, $queryParameters), $fetch);
    }
    /**
     * @param string $appCode
     * @param string $property
     * @param string $section
     * @param array{
     *    "clientId"?: string,
     *    "scope"?: string,
     * } $queryParameters
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\DeleteApiAdminConfigByAppCodeBySectionByPropertyForbiddenException
     * @throws \FlowCatalyst\Generated\Exception\DeleteApiAdminConfigByAppCodeBySectionByPropertyNotFoundException
     *
     * @return ($fetch is 'object' ? null : \Psr\Http\Message\ResponseInterface)
     */
    public function deleteApiAdminConfigByAppCodeBySectionByProperty(string $appCode, string $property, string $section, array $queryParameters = [], string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\DeleteApiAdminConfigByAppCodeBySectionByProperty($appCode, $property, $section, $queryParameters), $fetch);
    }
    /**
     * @param string $appCode
     * @param string $property
     * @param string $section
     * @param array{
     *    "clientId"?: string,
     *    "scope"?: string,
     * } $queryParameters
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\GetApiAdminConfigByAppCodeBySectionByPropertyForbiddenException
     * @throws \FlowCatalyst\Generated\Exception\GetApiAdminConfigByAppCodeBySectionByPropertyNotFoundException
     *
     * @return ($fetch is 'object' ? null : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiAdminConfigByAppCodeBySectionByProperty(string $appCode, string $property, string $section, array $queryParameters = [], string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiAdminConfigByAppCodeBySectionByProperty($appCode, $property, $section, $queryParameters), $fetch);
    }
    /**
     * @param string $appCode
     * @param string $property
     * @param string $section
     * @param null|\FlowCatalyst\Generated\Model\SetConfigRequest $requestBody
     * @param array{
     *    "clientId"?: string,
     *    "scope"?: string,
     * } $queryParameters
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\PutApiAdminConfigByAppCodeBySectionByPropertyBadRequestException
     * @throws \FlowCatalyst\Generated\Exception\PutApiAdminConfigByAppCodeBySectionByPropertyForbiddenException
     *
     * @return ($fetch is 'object' ? null : \Psr\Http\Message\ResponseInterface)
     */
    public function putApiAdminConfigByAppCodeBySectionByProperty(string $appCode, string $property, string $section, ?\FlowCatalyst\Generated\Model\SetConfigRequest $requestBody = null, array $queryParameters = [], string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\PutApiAdminConfigByAppCodeBySectionByProperty($appCode, $property, $section, $requestBody, $queryParameters), $fetch);
    }
    /**
     * Returns dispatch pools with optional filtering
     * @param array{
     *    "anchorLevel"?: bool, //If true, return only anchor-level pools (clientId is null)
     *    "clientId"?: string, //Filter by client ID
     *    "status"?: string, //Filter by status
     * } $queryParameters
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\GetApiAdminDispatchPoolUnauthorizedException
     * @throws \FlowCatalyst\Generated\Exception\GetApiAdminDispatchPoolForbiddenException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\DispatchPoolListResponse : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiAdminDispatchPool(array $queryParameters = [], string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiAdminDispatchPool($queryParameters), $fetch);
    }
    /**
     * @param null|\FlowCatalyst\Generated\Model\CreateDispatchPoolRequest $requestBody
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\PostApiAdminDispatchPoolBadRequestException
     * @throws \FlowCatalyst\Generated\Exception\PostApiAdminDispatchPoolUnauthorizedException
     * @throws \FlowCatalyst\Generated\Exception\PostApiAdminDispatchPoolForbiddenException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\DispatchPoolDto : \Psr\Http\Message\ResponseInterface)
     */
    public function postApiAdminDispatchPool(?\FlowCatalyst\Generated\Model\CreateDispatchPoolRequest $requestBody = null, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\PostApiAdminDispatchPool($requestBody), $fetch);
    }
    /**
     * Archives the pool (soft delete)
     * @param string $id
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\DeleteApiAdminDispatchPoolByIdNotFoundException
     * @throws \FlowCatalyst\Generated\Exception\DeleteApiAdminDispatchPoolByIdBadRequestException
     * @throws \FlowCatalyst\Generated\Exception\DeleteApiAdminDispatchPoolByIdUnauthorizedException
     * @throws \FlowCatalyst\Generated\Exception\DeleteApiAdminDispatchPoolByIdForbiddenException
     *
     * @return ($fetch is 'object' ? null : \Psr\Http\Message\ResponseInterface)
     */
    public function deleteApiAdminDispatchPoolById(string $id, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\DeleteApiAdminDispatchPoolById($id), $fetch);
    }
    /**
     * @param string $id
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\GetApiAdminDispatchPoolByIdNotFoundException
     * @throws \FlowCatalyst\Generated\Exception\GetApiAdminDispatchPoolByIdUnauthorizedException
     * @throws \FlowCatalyst\Generated\Exception\GetApiAdminDispatchPoolByIdForbiddenException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\DispatchPoolDto : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiAdminDispatchPoolById(string $id, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiAdminDispatchPoolById($id), $fetch);
    }
    /**
     * @param string $id
     * @param null|\FlowCatalyst\Generated\Model\UpdateDispatchPoolRequest $requestBody
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\PutApiAdminDispatchPoolByIdNotFoundException
     * @throws \FlowCatalyst\Generated\Exception\PutApiAdminDispatchPoolByIdBadRequestException
     * @throws \FlowCatalyst\Generated\Exception\PutApiAdminDispatchPoolByIdUnauthorizedException
     * @throws \FlowCatalyst\Generated\Exception\PutApiAdminDispatchPoolByIdForbiddenException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\DispatchPoolDto : \Psr\Http\Message\ResponseInterface)
     */
    public function putApiAdminDispatchPoolById(string $id, ?\FlowCatalyst\Generated\Model\UpdateDispatchPoolRequest $requestBody = null, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\PutApiAdminDispatchPoolById($id, $requestBody), $fetch);
    }
    /**
     * Re-enable a suspended pool
     * @param string $id
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\PostApiAdminDispatchPoolsByIdActivateNotFoundException
     * @throws \FlowCatalyst\Generated\Exception\PostApiAdminDispatchPoolsByIdActivateUnauthorizedException
     * @throws \FlowCatalyst\Generated\Exception\PostApiAdminDispatchPoolsByIdActivateForbiddenException
     *
     * @return ($fetch is 'object' ? null : \Psr\Http\Message\ResponseInterface)
     */
    public function postApiAdminDispatchPoolsByIdActivate(string $id, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\PostApiAdminDispatchPoolsByIdActivate($id), $fetch);
    }
    /**
     * Temporarily disable the pool
     * @param string $id
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\PostApiAdminDispatchPoolsByIdSuspendNotFoundException
     * @throws \FlowCatalyst\Generated\Exception\PostApiAdminDispatchPoolsByIdSuspendUnauthorizedException
     * @throws \FlowCatalyst\Generated\Exception\PostApiAdminDispatchPoolsByIdSuspendForbiddenException
     *
     * @return ($fetch is 'object' ? null : \Psr\Http\Message\ResponseInterface)
     */
    public function postApiAdminDispatchPoolsByIdSuspend(string $id, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\PostApiAdminDispatchPoolsByIdSuspend($id), $fetch);
    }
    /**
     * Returns all email domain mappings. Filter by identity provider or scope type.
     * @param array{
     *    "identityProviderId"?: string,
     *    "scopeType"?: string,
     * } $queryParameters
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\ListEmailDomainMappingsUnauthorizedException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\EmailDomainMappingListResponse : \Psr\Http\Message\ResponseInterface)
     */
    public function listEmailDomainMappings(array $queryParameters = [], string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\ListEmailDomainMappings($queryParameters), $fetch);
    }
    /**
     * @param null|\FlowCatalyst\Generated\Model\CreateEmailDomainMappingRequest $requestBody
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\CreateEmailDomainMappingBadRequestException
     * @throws \FlowCatalyst\Generated\Exception\CreateEmailDomainMappingNotFoundException
     * @throws \FlowCatalyst\Generated\Exception\CreateEmailDomainMappingConflictException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\EmailDomainMappingDto : \Psr\Http\Message\ResponseInterface)
     */
    public function createEmailDomainMapping(?\FlowCatalyst\Generated\Model\CreateEmailDomainMappingRequest $requestBody = null, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\CreateEmailDomainMapping($requestBody), $fetch);
    }
    /**
     * @param string $domain
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\GetEmailDomainMappingByDomainNotFoundException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\EmailDomainMappingDto : \Psr\Http\Message\ResponseInterface)
     */
    public function getEmailDomainMappingByDomain(string $domain, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetEmailDomainMappingByDomain($domain), $fetch);
    }
    /**
     * @param string $id
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\DeleteEmailDomainMappingNotFoundException
     *
     * @return ($fetch is 'object' ? null : \Psr\Http\Message\ResponseInterface)
     */
    public function deleteEmailDomainMapping(string $id, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\DeleteEmailDomainMapping($id), $fetch);
    }
    /**
     * @param string $id
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\GetEmailDomainMappingNotFoundException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\EmailDomainMappingDto : \Psr\Http\Message\ResponseInterface)
     */
    public function getEmailDomainMapping(string $id, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetEmailDomainMapping($id), $fetch);
    }
    /**
     * @param string $id
     * @param null|\FlowCatalyst\Generated\Model\UpdateEmailDomainMappingRequest $requestBody
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\UpdateEmailDomainMappingBadRequestException
     * @throws \FlowCatalyst\Generated\Exception\UpdateEmailDomainMappingNotFoundException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\EmailDomainMappingDto : \Psr\Http\Message\ResponseInterface)
     */
    public function updateEmailDomainMapping(string $id, ?\FlowCatalyst\Generated\Model\UpdateEmailDomainMappingRequest $requestBody = null, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\UpdateEmailDomainMapping($id, $requestBody), $fetch);
    }
    /**
     * Returns all identity providers. Filter by type if needed.
     * @param array{
     *    "type"?: string,
     * } $queryParameters
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\ListIdentityProvidersUnauthorizedException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\IdentityProviderListResponse : \Psr\Http\Message\ResponseInterface)
     */
    public function listIdentityProviders(array $queryParameters = [], string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\ListIdentityProviders($queryParameters), $fetch);
    }
    /**
     * @param null|\FlowCatalyst\Generated\Model\CreateIdentityProviderRequest $requestBody
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\CreateIdentityProviderBadRequestException
     * @throws \FlowCatalyst\Generated\Exception\CreateIdentityProviderConflictException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\IdentityProviderDto : \Psr\Http\Message\ResponseInterface)
     */
    public function createIdentityProvider(?\FlowCatalyst\Generated\Model\CreateIdentityProviderRequest $requestBody = null, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\CreateIdentityProvider($requestBody), $fetch);
    }
    /**
     * @param string $code
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\GetIdentityProviderByCodeNotFoundException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\IdentityProviderDto : \Psr\Http\Message\ResponseInterface)
     */
    public function getIdentityProviderByCode(string $code, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetIdentityProviderByCode($code), $fetch);
    }
    /**
     * @param string $id
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\DeleteIdentityProviderBadRequestException
     * @throws \FlowCatalyst\Generated\Exception\DeleteIdentityProviderNotFoundException
     *
     * @return ($fetch is 'object' ? null : \Psr\Http\Message\ResponseInterface)
     */
    public function deleteIdentityProvider(string $id, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\DeleteIdentityProvider($id), $fetch);
    }
    /**
     * @param string $id
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\GetIdentityProviderNotFoundException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\IdentityProviderDto : \Psr\Http\Message\ResponseInterface)
     */
    public function getIdentityProvider(string $id, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetIdentityProvider($id), $fetch);
    }
    /**
     * @param string $id
     * @param null|\FlowCatalyst\Generated\Model\UpdateIdentityProviderRequest $requestBody
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\UpdateIdentityProviderNotFoundException
     * @throws \FlowCatalyst\Generated\Exception\UpdateIdentityProviderBadRequestException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\IdentityProviderDto : \Psr\Http\Message\ResponseInterface)
     */
    public function updateIdentityProvider(string $id, ?\FlowCatalyst\Generated\Model\UpdateIdentityProviderRequest $requestBody = null, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\UpdateIdentityProvider($id, $requestBody), $fetch);
    }
    /**
     * @param array{
     *    "active"?: bool, //Filter by active status
     *    "applicationId"?: string, //Filter by associated application
     * } $queryParameters
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\ListOAuthClientsUnauthorizedException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ClientListResponse2 : \Psr\Http\Message\ResponseInterface)
     */
    public function listOAuthClients(array $queryParameters = [], string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\ListOAuthClients($queryParameters), $fetch);
    }
    /**
     * For confidential clients, the secret is returned once in the response and cannot be retrieved again.
     * @param null|\FlowCatalyst\Generated\Model\CreateClientRequest2 $requestBody
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\CreateOAuthClientBadRequestException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\CreateClientResponse : \Psr\Http\Message\ResponseInterface)
     */
    public function createOAuthClient(?\FlowCatalyst\Generated\Model\CreateClientRequest2 $requestBody = null, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\CreateOAuthClient($requestBody), $fetch);
    }
    /**
     * @param string $clientId
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\GetOAuthClientByClientIdNotFoundException
     *
     * @return ($fetch is 'object' ? null : \Psr\Http\Message\ResponseInterface)
     */
    public function getOAuthClientByClientId(string $clientId, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetOAuthClientByClientId($clientId), $fetch);
    }
    /**
     * @param string $id
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\DeleteOAuthClientNotFoundException
     *
     * @return ($fetch is 'object' ? null : \Psr\Http\Message\ResponseInterface)
     */
    public function deleteOAuthClient(string $id, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\DeleteOAuthClient($id), $fetch);
    }
    /**
     * @param string $id
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\GetOAuthClientNotFoundException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ClientDto2 : \Psr\Http\Message\ResponseInterface)
     */
    public function getOAuthClient(string $id, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetOAuthClient($id), $fetch);
    }
    /**
     * @param string $id
     * @param null|\FlowCatalyst\Generated\Model\UpdateClientRequest2 $requestBody
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\UpdateOAuthClientNotFoundException
     * @throws \FlowCatalyst\Generated\Exception\UpdateOAuthClientBadRequestException
     *
     * @return ($fetch is 'object' ? null : \Psr\Http\Message\ResponseInterface)
     */
    public function updateOAuthClient(string $id, ?\FlowCatalyst\Generated\Model\UpdateClientRequest2 $requestBody = null, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\UpdateOAuthClient($id, $requestBody), $fetch);
    }
    /**
     * @param string $id
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\ActivateOAuthClientNotFoundException
     *
     * @return ($fetch is 'object' ? null : \Psr\Http\Message\ResponseInterface)
     */
    public function activateOAuthClient(string $id, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\ActivateOAuthClient($id), $fetch);
    }
    /**
     * @param string $id
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\DeactivateOAuthClientNotFoundException
     *
     * @return ($fetch is 'object' ? null : \Psr\Http\Message\ResponseInterface)
     */
    public function deactivateOAuthClient(string $id, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\DeactivateOAuthClient($id), $fetch);
    }
    /**
     * Generates a new secret. The new secret is returned once and cannot be retrieved again.
     * @param string $id
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\RotateOAuthClientSecretBadRequestException
     * @throws \FlowCatalyst\Generated\Exception\RotateOAuthClientSecretNotFoundException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\RotateSecretResponse : \Psr\Http\Message\ResponseInterface)
     */
    public function rotateOAuthClientSecret(string $id, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\RotateOAuthClientSecret($id), $fetch);
    }
    /**
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\ListCorsOriginsUnauthorizedException
     * @throws \FlowCatalyst\Generated\Exception\ListCorsOriginsForbiddenException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\CorsOriginListResponse : \Psr\Http\Message\ResponseInterface)
     */
    public function listCorsOrigins(string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\ListCorsOrigins(), $fetch);
    }
    /**
     * @param null|\FlowCatalyst\Generated\Model\CreateCorsOriginRequest $requestBody
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\AddCorsOriginBadRequestException
     * @throws \FlowCatalyst\Generated\Exception\AddCorsOriginConflictException
     * @throws \FlowCatalyst\Generated\Exception\AddCorsOriginUnauthorizedException
     * @throws \FlowCatalyst\Generated\Exception\AddCorsOriginForbiddenException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\CorsOriginDto : \Psr\Http\Message\ResponseInterface)
     */
    public function addCorsOrigin(?\FlowCatalyst\Generated\Model\CreateCorsOriginRequest $requestBody = null, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\AddCorsOrigin($requestBody), $fetch);
    }
    /**
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\GetAllowedCorsOriginsUnauthorizedException
     * @throws \FlowCatalyst\Generated\Exception\GetAllowedCorsOriginsForbiddenException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\AllowedOriginsResponse : \Psr\Http\Message\ResponseInterface)
     */
    public function getAllowedCorsOrigins(string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetAllowedCorsOrigins(), $fetch);
    }
    /**
     * @param string $id
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\DeleteCorsOriginBadRequestException
     * @throws \FlowCatalyst\Generated\Exception\DeleteCorsOriginNotFoundException
     * @throws \FlowCatalyst\Generated\Exception\DeleteCorsOriginUnauthorizedException
     * @throws \FlowCatalyst\Generated\Exception\DeleteCorsOriginForbiddenException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\CorsOriginDeletedResponse : \Psr\Http\Message\ResponseInterface)
     */
    public function deleteCorsOrigin(string $id, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\DeleteCorsOrigin($id), $fetch);
    }
    /**
     * @param string $id
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\GetCorsOriginNotFoundException
     * @throws \FlowCatalyst\Generated\Exception\GetCorsOriginUnauthorizedException
     * @throws \FlowCatalyst\Generated\Exception\GetCorsOriginForbiddenException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\CorsOriginDto : \Psr\Http\Message\ResponseInterface)
     */
    public function getCorsOrigin(string $id, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetCorsOrigin($id), $fetch);
    }
    /**
     * List users and service accounts with optional filters
     * @param array{
     *    "active"?: bool, //Filter by active status
     *    "clientId"?: string, //Filter by client ID
     *    "email"?: string, //Filter by exact email match
     *    "type"?: string, //Filter by type (USER/SERVICE)
     * } $queryParameters
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\ListPrincipalsUnauthorizedException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\PrincipalListResponse1 : \Psr\Http\Message\ResponseInterface)
     */
    public function listPrincipals(array $queryParameters = [], string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\ListPrincipals($queryParameters), $fetch);
    }
    /**
     * Returns auth provider info and warnings for an email domain
     * @param array{
     *    "email"?: string, //Email address to check
     * } $queryParameters
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\CheckEmailDomainBadRequestException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\EmailDomainCheckResponse : \Psr\Http\Message\ResponseInterface)
     */
    public function checkEmailDomain(array $queryParameters = [], string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\CheckEmailDomain($queryParameters), $fetch);
    }
    /**
     * @param null|\FlowCatalyst\Generated\Model\CreateUserRequest1 $requestBody
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\CreateUserBadRequestException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\PrincipalDto1 : \Psr\Http\Message\ResponseInterface)
     */
    public function createUser(?\FlowCatalyst\Generated\Model\CreateUserRequest1 $requestBody = null, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\CreateUser($requestBody), $fetch);
    }
    /**
     * @param string $id
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\GetPrincipalNotFoundException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\PrincipalDetailDto : \Psr\Http\Message\ResponseInterface)
     */
    public function getPrincipal(string $id, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetPrincipal($id), $fetch);
    }
    /**
     * @param string $id
     * @param null|\FlowCatalyst\Generated\Model\UpdatePrincipalRequest1 $requestBody
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\UpdatePrincipalNotFoundException
     * @throws \FlowCatalyst\Generated\Exception\UpdatePrincipalBadRequestException
     *
     * @return ($fetch is 'object' ? null : \Psr\Http\Message\ResponseInterface)
     */
    public function updatePrincipal(string $id, ?\FlowCatalyst\Generated\Model\UpdatePrincipalRequest1 $requestBody = null, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\UpdatePrincipal($id, $requestBody), $fetch);
    }
    /**
     * @param string $id
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\ActivatePrincipalNotFoundException
     *
     * @return ($fetch is 'object' ? null : \Psr\Http\Message\ResponseInterface)
     */
    public function activatePrincipal(string $id, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\ActivatePrincipal($id), $fetch);
    }
    /**
     * @param string $id
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\GetPrincipalApplicationAccessNotFoundException
     *
     * @return ($fetch is 'object' ? null : \Psr\Http\Message\ResponseInterface)
     */
    public function getPrincipalApplicationAccess(string $id, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetPrincipalApplicationAccess($id), $fetch);
    }
    /**
     * Sets the complete list of applications for a principal. Applications not in the list will be removed.
     * @param string $id
     * @param null|\FlowCatalyst\Generated\Model\AssignApplicationAccessRequest $requestBody
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\AssignPrincipalApplicationAccessBadRequestException
     * @throws \FlowCatalyst\Generated\Exception\AssignPrincipalApplicationAccessNotFoundException
     *
     * @return ($fetch is 'object' ? null : \Psr\Http\Message\ResponseInterface)
     */
    public function assignPrincipalApplicationAccess(string $id, ?\FlowCatalyst\Generated\Model\AssignApplicationAccessRequest $requestBody = null, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\AssignPrincipalApplicationAccess($id, $requestBody), $fetch);
    }
    /**
     * Returns applications that are enabled for at least one of the user's accessible clients
     * @param string $id
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\GetPrincipalAvailableApplicationsNotFoundException
     *
     * @return ($fetch is 'object' ? null : \Psr\Http\Message\ResponseInterface)
     */
    public function getPrincipalAvailableApplications(string $id, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetPrincipalAvailableApplications($id), $fetch);
    }
    /**
     * @param string $id
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\GetPrincipalClientAccessNotFoundException
     *
     * @return ($fetch is 'object' ? null : \Psr\Http\Message\ResponseInterface)
     */
    public function getPrincipalClientAccess(string $id, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetPrincipalClientAccess($id), $fetch);
    }
    /**
     * @param string $id
     * @param null|\FlowCatalyst\Generated\Model\GrantClientAccessRequest $requestBody
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\GrantPrincipalClientAccessBadRequestException
     * @throws \FlowCatalyst\Generated\Exception\GrantPrincipalClientAccessNotFoundException
     *
     * @return ($fetch is 'object' ? null : \Psr\Http\Message\ResponseInterface)
     */
    public function grantPrincipalClientAccess(string $id, ?\FlowCatalyst\Generated\Model\GrantClientAccessRequest $requestBody = null, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GrantPrincipalClientAccess($id, $requestBody), $fetch);
    }
    /**
     * @param string $clientId
     * @param string $id
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\RevokePrincipalClientAccessNotFoundException
     *
     * @return ($fetch is 'object' ? null : \Psr\Http\Message\ResponseInterface)
     */
    public function revokePrincipalClientAccess(string $clientId, string $id, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\RevokePrincipalClientAccess($clientId, $id), $fetch);
    }
    /**
     * @param string $id
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\DeactivatePrincipalNotFoundException
     *
     * @return ($fetch is 'object' ? null : \Psr\Http\Message\ResponseInterface)
     */
    public function deactivatePrincipal(string $id, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\DeactivatePrincipal($id), $fetch);
    }
    /**
     * @param string $id
     * @param null|\FlowCatalyst\Generated\Model\ResetPasswordRequest $requestBody
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\ResetPrincipalPasswordBadRequestException
     * @throws \FlowCatalyst\Generated\Exception\ResetPrincipalPasswordNotFoundException
     *
     * @return ($fetch is 'object' ? null : \Psr\Http\Message\ResponseInterface)
     */
    public function resetPrincipalPassword(string $id, ?\FlowCatalyst\Generated\Model\ResetPasswordRequest $requestBody = null, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\ResetPrincipalPassword($id, $requestBody), $fetch);
    }
    /**
     * @param string $id
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\GetPrincipalRolesNotFoundException
     *
     * @return ($fetch is 'object' ? null : \Psr\Http\Message\ResponseInterface)
     */
    public function getPrincipalRoles(string $id, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetPrincipalRoles($id), $fetch);
    }
    /**
     * @param string $id
     * @param null|\FlowCatalyst\Generated\Model\AssignRoleRequest $requestBody
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\AssignPrincipalRoleBadRequestException
     * @throws \FlowCatalyst\Generated\Exception\AssignPrincipalRoleNotFoundException
     *
     * @return ($fetch is 'object' ? null : \Psr\Http\Message\ResponseInterface)
     */
    public function assignPrincipalRole(string $id, ?\FlowCatalyst\Generated\Model\AssignRoleRequest $requestBody = null, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\AssignPrincipalRole($id, $requestBody), $fetch);
    }
    /**
     * Sets the complete list of roles for a principal. Roles not in the list will be removed.
     * @param string $id
     * @param null|\FlowCatalyst\Generated\Model\AssignRolesRequest1 $requestBody
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\AssignPrincipalRolesBadRequestException
     * @throws \FlowCatalyst\Generated\Exception\AssignPrincipalRolesNotFoundException
     *
     * @return ($fetch is 'object' ? null : \Psr\Http\Message\ResponseInterface)
     */
    public function assignPrincipalRoles(string $id, ?\FlowCatalyst\Generated\Model\AssignRolesRequest1 $requestBody = null, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\AssignPrincipalRoles($id, $requestBody), $fetch);
    }
    /**
     * @param string $id
     * @param string $roleName
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\RemovePrincipalRoleNotFoundException
     *
     * @return ($fetch is 'object' ? null : \Psr\Http\Message\ResponseInterface)
     */
    public function removePrincipalRole(string $id, string $roleName, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\RemovePrincipalRole($id, $roleName), $fetch);
    }
    /**
     * Returns all roles from the database. Filter by application code or source.
     * @param array{
     *    "application"?: string,
     *    "source"?: string,
     * } $queryParameters
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\ListRolesBadRequestException
     * @throws \FlowCatalyst\Generated\Exception\ListRolesUnauthorizedException
     * @throws \FlowCatalyst\Generated\Exception\ListRolesForbiddenException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\RoleListResponse2 : \Psr\Http\Message\ResponseInterface)
     */
    public function listRoles(array $queryParameters = [], string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\ListRoles($queryParameters), $fetch);
    }
    /**
     * Creates a new role with source=DATABASE. Role name is auto-prefixed with application code.
     * @param null|\FlowCatalyst\Generated\Model\CreateRoleRequest1 $requestBody
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\CreateRoleBadRequestException
     * @throws \FlowCatalyst\Generated\Exception\CreateRoleNotFoundException
     * @throws \FlowCatalyst\Generated\Exception\CreateRoleConflictException
     * @throws \FlowCatalyst\Generated\Exception\CreateRoleUnauthorizedException
     * @throws \FlowCatalyst\Generated\Exception\CreateRoleForbiddenException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\RoleDto1 : \Psr\Http\Message\ResponseInterface)
     */
    public function createRole(?\FlowCatalyst\Generated\Model\CreateRoleRequest1 $requestBody = null, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\CreateRole($requestBody), $fetch);
    }
    /**
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\ListPermissionsUnauthorizedException
     * @throws \FlowCatalyst\Generated\Exception\ListPermissionsForbiddenException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\PermissionListResponse : \Psr\Http\Message\ResponseInterface)
     */
    public function listPermissions(string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\ListPermissions(), $fetch);
    }
    /**
     * @param string $permission
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\GetPermissionNotFoundException
     * @throws \FlowCatalyst\Generated\Exception\GetPermissionUnauthorizedException
     * @throws \FlowCatalyst\Generated\Exception\GetPermissionForbiddenException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\PermissionDto : \Psr\Http\Message\ResponseInterface)
     */
    public function getPermission(string $permission, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetPermission($permission), $fetch);
    }
    /**
     * Deletes a role. Only DATABASE and SDK sourced roles can be deleted.
     * @param string $roleName
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\DeleteRoleBadRequestException
     * @throws \FlowCatalyst\Generated\Exception\DeleteRoleNotFoundException
     * @throws \FlowCatalyst\Generated\Exception\DeleteRoleUnauthorizedException
     * @throws \FlowCatalyst\Generated\Exception\DeleteRoleForbiddenException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\DeleteResponse : \Psr\Http\Message\ResponseInterface)
     */
    public function deleteRole(string $roleName, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\DeleteRole($roleName), $fetch);
    }
    /**
     * @param string $roleName
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\GetRoleNotFoundException
     * @throws \FlowCatalyst\Generated\Exception\GetRoleUnauthorizedException
     * @throws \FlowCatalyst\Generated\Exception\GetRoleForbiddenException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\RoleDto1 : \Psr\Http\Message\ResponseInterface)
     */
    public function getRole(string $roleName, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetRole($roleName), $fetch);
    }
    /**
     * Updates a role. CODE-sourced roles can only have clientManaged updated.
     * @param string $roleName
     * @param null|\FlowCatalyst\Generated\Model\UpdateRoleRequest1 $requestBody
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\UpdateRoleBadRequestException
     * @throws \FlowCatalyst\Generated\Exception\UpdateRoleNotFoundException
     * @throws \FlowCatalyst\Generated\Exception\UpdateRoleUnauthorizedException
     * @throws \FlowCatalyst\Generated\Exception\UpdateRoleForbiddenException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\RoleDto1 : \Psr\Http\Message\ResponseInterface)
     */
    public function updateRole(string $roleName, ?\FlowCatalyst\Generated\Model\UpdateRoleRequest1 $requestBody = null, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\UpdateRole($roleName, $requestBody), $fetch);
    }
    /**
     * List all service accounts with optional filters
     * @param array{
     *    "active"?: bool, //Filter by active status
     *    "applicationId"?: string, //Filter by application ID
     *    "clientId"?: string, //Filter by client ID
     * } $queryParameters
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\GetApiAdminServiceAccountUnauthorizedException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ServiceAccountListResponse : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiAdminServiceAccount(array $queryParameters = [], string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiAdminServiceAccount($queryParameters), $fetch);
    }
    /**
     * Creates a service account and returns credentials. Credentials are shown only once.
     * @param null|\FlowCatalyst\Generated\Model\CreateServiceAccountRequest $requestBody
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\PostApiAdminServiceAccountBadRequestException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\CreateServiceAccountResponse : \Psr\Http\Message\ResponseInterface)
     */
    public function postApiAdminServiceAccount(?\FlowCatalyst\Generated\Model\CreateServiceAccountRequest $requestBody = null, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\PostApiAdminServiceAccount($requestBody), $fetch);
    }
    /**
     * @param string $code
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     *
     * @return ($fetch is 'object' ? null : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiAdminServiceAccountsCodeByCode(string $code, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiAdminServiceAccountsCodeByCode($code), $fetch);
    }
    /**
     * @param string $id
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     *
     * @return ($fetch is 'object' ? null : \Psr\Http\Message\ResponseInterface)
     */
    public function deleteApiAdminServiceAccountById(string $id, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\DeleteApiAdminServiceAccountById($id), $fetch);
    }
    /**
     * @param string $id
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\GetApiAdminServiceAccountByIdNotFoundException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ServiceAccountDto : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiAdminServiceAccountById(string $id, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiAdminServiceAccountById($id), $fetch);
    }
    /**
     * @param string $id
     * @param null|\FlowCatalyst\Generated\Model\UpdateServiceAccountRequest $requestBody
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\PutApiAdminServiceAccountByIdBadRequestException
     *
     * @return ($fetch is 'object' ? null : \Psr\Http\Message\ResponseInterface)
     */
    public function putApiAdminServiceAccountById(string $id, ?\FlowCatalyst\Generated\Model\UpdateServiceAccountRequest $requestBody = null, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\PutApiAdminServiceAccountById($id, $requestBody), $fetch);
    }
    /**
     * Replace the auth token with a custom value
     * @param string $id
     * @param null|\FlowCatalyst\Generated\Model\UpdateAuthTokenRequest $requestBody
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\PutApiAdminServiceAccountsByIdAuthTokenBadRequestException
     *
     * @return ($fetch is 'object' ? null : \Psr\Http\Message\ResponseInterface)
     */
    public function putApiAdminServiceAccountsByIdAuthToken(string $id, ?\FlowCatalyst\Generated\Model\UpdateAuthTokenRequest $requestBody = null, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\PutApiAdminServiceAccountsByIdAuthToken($id, $requestBody), $fetch);
    }
    /**
     * Generate a new signing secret. Returns the new secret (shown only once).
     * @param string $id
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     *
     * @return ($fetch is 'object' ? null : \Psr\Http\Message\ResponseInterface)
     */
    public function postApiAdminServiceAccountsByIdRegenerateSecret(string $id, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\PostApiAdminServiceAccountsByIdRegenerateSecret($id), $fetch);
    }
    /**
     * Generate a new random auth token. Returns the new token (shown only once).
     * @param string $id
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     *
     * @return ($fetch is 'object' ? null : \Psr\Http\Message\ResponseInterface)
     */
    public function postApiAdminServiceAccountsByIdRegenerateToken(string $id, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\PostApiAdminServiceAccountsByIdRegenerateToken($id), $fetch);
    }
    /**
     * @param string $id
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     *
     * @return ($fetch is 'object' ? null : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiAdminServiceAccountsByIdRole(string $id, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiAdminServiceAccountsByIdRole($id), $fetch);
    }
    /**
     * Replace all roles with the provided list (declarative assignment)
     * @param string $id
     * @param null|\FlowCatalyst\Generated\Model\AssignRolesRequest2 $requestBody
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\PutApiAdminServiceAccountsByIdRoleBadRequestException
     *
     * @return ($fetch is 'object' ? null : \Psr\Http\Message\ResponseInterface)
     */
    public function putApiAdminServiceAccountsByIdRole(string $id, ?\FlowCatalyst\Generated\Model\AssignRolesRequest2 $requestBody = null, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\PutApiAdminServiceAccountsByIdRole($id, $requestBody), $fetch);
    }
    /**
     * Returns subscriptions with optional filtering
     * @param array{
     *    "anchorLevel"?: bool, //If true, return only anchor-level subscriptions
     *    "clientId"?: string, //Filter by client ID
     *    "dispatchPoolId"?: string, //Filter by dispatch pool
     *    "source"?: string, //Filter by source
     *    "status"?: string, //Filter by status
     * } $queryParameters
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\GetApiAdminSubscriptionUnauthorizedException
     * @throws \FlowCatalyst\Generated\Exception\GetApiAdminSubscriptionForbiddenException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\SubscriptionListResponse1 : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiAdminSubscription(array $queryParameters = [], string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiAdminSubscription($queryParameters), $fetch);
    }
    /**
     * @param null|\FlowCatalyst\Generated\Model\CreateSubscriptionRequest $requestBody
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\PostApiAdminSubscriptionBadRequestException
     * @throws \FlowCatalyst\Generated\Exception\PostApiAdminSubscriptionUnauthorizedException
     * @throws \FlowCatalyst\Generated\Exception\PostApiAdminSubscriptionForbiddenException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\SubscriptionDto1 : \Psr\Http\Message\ResponseInterface)
     */
    public function postApiAdminSubscription(?\FlowCatalyst\Generated\Model\CreateSubscriptionRequest $requestBody = null, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\PostApiAdminSubscription($requestBody), $fetch);
    }
    /**
     * @param string $id
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\DeleteApiAdminSubscriptionByIdNotFoundException
     * @throws \FlowCatalyst\Generated\Exception\DeleteApiAdminSubscriptionByIdUnauthorizedException
     * @throws \FlowCatalyst\Generated\Exception\DeleteApiAdminSubscriptionByIdForbiddenException
     *
     * @return ($fetch is 'object' ? null : \Psr\Http\Message\ResponseInterface)
     */
    public function deleteApiAdminSubscriptionById(string $id, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\DeleteApiAdminSubscriptionById($id), $fetch);
    }
    /**
     * @param string $id
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\GetApiAdminSubscriptionByIdNotFoundException
     * @throws \FlowCatalyst\Generated\Exception\GetApiAdminSubscriptionByIdUnauthorizedException
     * @throws \FlowCatalyst\Generated\Exception\GetApiAdminSubscriptionByIdForbiddenException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\SubscriptionDto1 : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiAdminSubscriptionById(string $id, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiAdminSubscriptionById($id), $fetch);
    }
    /**
     * @param string $id
     * @param null|\FlowCatalyst\Generated\Model\UpdateSubscriptionRequest $requestBody
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\PutApiAdminSubscriptionByIdNotFoundException
     * @throws \FlowCatalyst\Generated\Exception\PutApiAdminSubscriptionByIdBadRequestException
     * @throws \FlowCatalyst\Generated\Exception\PutApiAdminSubscriptionByIdUnauthorizedException
     * @throws \FlowCatalyst\Generated\Exception\PutApiAdminSubscriptionByIdForbiddenException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\SubscriptionDto1 : \Psr\Http\Message\ResponseInterface)
     */
    public function putApiAdminSubscriptionById(string $id, ?\FlowCatalyst\Generated\Model\UpdateSubscriptionRequest $requestBody = null, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\PutApiAdminSubscriptionById($id, $requestBody), $fetch);
    }
    /**
     * Stop creating dispatch jobs for this subscription
     * @param string $id
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\PostApiAdminSubscriptionsByIdPauseNotFoundException
     * @throws \FlowCatalyst\Generated\Exception\PostApiAdminSubscriptionsByIdPauseUnauthorizedException
     * @throws \FlowCatalyst\Generated\Exception\PostApiAdminSubscriptionsByIdPauseForbiddenException
     *
     * @return ($fetch is 'object' ? null : \Psr\Http\Message\ResponseInterface)
     */
    public function postApiAdminSubscriptionsByIdPause(string $id, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\PostApiAdminSubscriptionsByIdPause($id), $fetch);
    }
    /**
     * Re-enable dispatch job creation
     * @param string $id
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\PostApiAdminSubscriptionsByIdResumeNotFoundException
     * @throws \FlowCatalyst\Generated\Exception\PostApiAdminSubscriptionsByIdResumeUnauthorizedException
     * @throws \FlowCatalyst\Generated\Exception\PostApiAdminSubscriptionsByIdResumeForbiddenException
     *
     * @return ($fetch is 'object' ? null : \Psr\Http\Message\ResponseInterface)
     */
    public function postApiAdminSubscriptionsByIdResume(string $id, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\PostApiAdminSubscriptionsByIdResume($id), $fetch);
    }
    /**
     * Returns all applications
     * @param array{
     *    "activeOnly"?: bool,
     * } $queryParameters
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApplicationListResponse1 : \Psr\Http\Message\ResponseInterface)
     */
    public function listApps(array $queryParameters = [], string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\ListApps($queryParameters), $fetch);
    }
    /**
     * @param null|\FlowCatalyst\Generated\Model\CreateApplicationRequest1 $requestBody
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\CreateAppBadRequestException
     * @throws \FlowCatalyst\Generated\Exception\CreateAppConflictException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApplicationResponse1 : \Psr\Http\Message\ResponseInterface)
     */
    public function createApp(?\FlowCatalyst\Generated\Model\CreateApplicationRequest1 $requestBody = null, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\CreateApp($requestBody), $fetch);
    }
    /**
     * @param string $code
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\GetAppByCodeNotFoundException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApplicationResponse1 : \Psr\Http\Message\ResponseInterface)
     */
    public function getAppByCode(string $code, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetAppByCode($code), $fetch);
    }
    /**
     * Returns all anchor-level dispatch pools (clientId = null).
     * @param string $appCode
     * @param array{
     *    "status"?: string,
     * } $queryParameters
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\ListApplicationDispatchPoolsNotFoundException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\PoolListResponse : \Psr\Http\Message\ResponseInterface)
     */
    public function listApplicationDispatchPools(string $appCode, array $queryParameters = [], string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\ListApplicationDispatchPools($appCode, $queryParameters), $fetch);
    }
    /**
     * Bulk sync dispatch pools. Creates new pools, updates existing ones. Set removeUnlisted=true to archive pools not in the sync list.
     * @param string $appCode
     * @param null|\FlowCatalyst\Generated\Model\SyncPoolsRequest $requestBody
     * @param array{
     *    "removeUnlisted"?: bool,
     * } $queryParameters
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\SyncApplicationDispatchPoolsNotFoundException
     * @throws \FlowCatalyst\Generated\Exception\SyncApplicationDispatchPoolsBadRequestException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\SyncResponse : \Psr\Http\Message\ResponseInterface)
     */
    public function syncApplicationDispatchPools(string $appCode, ?\FlowCatalyst\Generated\Model\SyncPoolsRequest $requestBody = null, array $queryParameters = [], string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\SyncApplicationDispatchPools($appCode, $requestBody, $queryParameters), $fetch);
    }
    /**
     * Returns all event types registered for this application (matching code prefix).
     * @param string $appCode
     * @param array{
     *    "source"?: string,
     * } $queryParameters
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\ListApplicationEventTypesNotFoundException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\EventTypeListResponse1 : \Psr\Http\Message\ResponseInterface)
     */
    public function listApplicationEventTypes(string $appCode, array $queryParameters = [], string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\ListApplicationEventTypes($appCode, $queryParameters), $fetch);
    }
    /**
     * Bulk sync event types from an external application. Creates new event types, updates existing API-sourced event types. Set removeUnlisted=true to remove API-sourced event types not in the sync list.
     * @param string $appCode
     * @param null|\FlowCatalyst\Generated\Model\SyncEventTypesRequest $requestBody
     * @param array{
     *    "removeUnlisted"?: bool,
     * } $queryParameters
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\SyncApplicationEventTypesNotFoundException
     * @throws \FlowCatalyst\Generated\Exception\SyncApplicationEventTypesBadRequestException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\SyncResponse1 : \Psr\Http\Message\ResponseInterface)
     */
    public function syncApplicationEventTypes(string $appCode, ?\FlowCatalyst\Generated\Model\SyncEventTypesRequest $requestBody = null, array $queryParameters = [], string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\SyncApplicationEventTypes($appCode, $requestBody, $queryParameters), $fetch);
    }
    /**
     * Returns all roles registered for this application.
     * @param string $appCode
     * @param array{
     *    "source"?: string,
     * } $queryParameters
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\ListApplicationRolesNotFoundException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\RoleListResponse3 : \Psr\Http\Message\ResponseInterface)
     */
    public function listApplicationRoles(string $appCode, array $queryParameters = [], string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\ListApplicationRoles($appCode, $queryParameters), $fetch);
    }
    /**
     * Creates a single role for the application with source=SDK.
     * @param string $appCode
     * @param null|\FlowCatalyst\Generated\Model\CreateRoleRequest2 $requestBody
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\CreateApplicationRoleNotFoundException
     * @throws \FlowCatalyst\Generated\Exception\CreateApplicationRoleConflictException
     * @throws \FlowCatalyst\Generated\Exception\CreateApplicationRoleBadRequestException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\RoleDto2 : \Psr\Http\Message\ResponseInterface)
     */
    public function createApplicationRole(string $appCode, ?\FlowCatalyst\Generated\Model\CreateRoleRequest2 $requestBody = null, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\CreateApplicationRole($appCode, $requestBody), $fetch);
    }
    /**
     * Bulk sync roles from an external application. Creates new roles, updates existing SDK roles. Set removeUnlisted=true to remove SDK roles not in the sync list.
     * @param string $appCode
     * @param null|\FlowCatalyst\Generated\Model\SyncRolesRequest $requestBody
     * @param array{
     *    "removeUnlisted"?: bool,
     * } $queryParameters
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\SyncApplicationRolesNotFoundException
     * @throws \FlowCatalyst\Generated\Exception\SyncApplicationRolesBadRequestException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\SyncResponse2 : \Psr\Http\Message\ResponseInterface)
     */
    public function syncApplicationRoles(string $appCode, ?\FlowCatalyst\Generated\Model\SyncRolesRequest $requestBody = null, array $queryParameters = [], string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\SyncApplicationRoles($appCode, $requestBody, $queryParameters), $fetch);
    }
    /**
     * Deletes an SDK-sourced role. Cannot delete CODE or DATABASE sourced roles.
     * @param string $appCode
     * @param string $roleName
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\DeleteApplicationRoleBadRequestException
     * @throws \FlowCatalyst\Generated\Exception\DeleteApplicationRoleNotFoundException
     *
     * @return ($fetch is 'object' ? null : \Psr\Http\Message\ResponseInterface)
     */
    public function deleteApplicationRole(string $appCode, string $roleName, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\DeleteApplicationRole($appCode, $roleName), $fetch);
    }
    /**
     * Returns all anchor-level subscriptions (clientId = null).
     * @param string $appCode
     * @param array{
     *    "source"?: string,
     * } $queryParameters
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\ListApplicationSubscriptionsNotFoundException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\SubscriptionListResponse : \Psr\Http\Message\ResponseInterface)
     */
    public function listApplicationSubscriptions(string $appCode, array $queryParameters = [], string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\ListApplicationSubscriptions($appCode, $queryParameters), $fetch);
    }
    /**
     * Bulk sync subscriptions from an external application. Creates new subscriptions, updates existing API-sourced subscriptions. Set removeUnlisted=true to remove API-sourced subscriptions not in the sync list.
     * @param string $appCode
     * @param null|\FlowCatalyst\Generated\Model\SyncSubscriptionsRequest $requestBody
     * @param array{
     *    "removeUnlisted"?: bool,
     * } $queryParameters
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\SyncApplicationSubscriptionsBadRequestException
     * @throws \FlowCatalyst\Generated\Exception\SyncApplicationSubscriptionsNotFoundException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\SyncResponse3 : \Psr\Http\Message\ResponseInterface)
     */
    public function syncApplicationSubscriptions(string $appCode, ?\FlowCatalyst\Generated\Model\SyncSubscriptionsRequest $requestBody = null, array $queryParameters = [], string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\SyncApplicationSubscriptions($appCode, $requestBody, $queryParameters), $fetch);
    }
    /**
     * @param string $id
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\DeleteAppBadRequestException
     * @throws \FlowCatalyst\Generated\Exception\DeleteAppNotFoundException
     *
     * @return ($fetch is 'object' ? null : \Psr\Http\Message\ResponseInterface)
     */
    public function deleteApp(string $id, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\DeleteApp($id), $fetch);
    }
    /**
     * @param string $id
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\GetAppBadRequestException
     * @throws \FlowCatalyst\Generated\Exception\GetAppNotFoundException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApplicationResponse1 : \Psr\Http\Message\ResponseInterface)
     */
    public function getApp(string $id, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApp($id), $fetch);
    }
    /**
     * @param string $id
     * @param null|\FlowCatalyst\Generated\Model\UpdateApplicationRequest1 $requestBody
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\UpdateAppBadRequestException
     * @throws \FlowCatalyst\Generated\Exception\UpdateAppNotFoundException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApplicationResponse1 : \Psr\Http\Message\ResponseInterface)
     */
    public function updateApp(string $id, ?\FlowCatalyst\Generated\Model\UpdateApplicationRequest1 $requestBody = null, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\UpdateApp($id, $requestBody), $fetch);
    }
    /**
     * @param string $id
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\ActivateAppBadRequestException
     * @throws \FlowCatalyst\Generated\Exception\ActivateAppNotFoundException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApplicationResponse1 : \Psr\Http\Message\ResponseInterface)
     */
    public function activateApp(string $id, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\ActivateApp($id), $fetch);
    }
    /**
     * @param string $id
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\DeactivateAppBadRequestException
     * @throws \FlowCatalyst\Generated\Exception\DeactivateAppNotFoundException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApplicationResponse1 : \Psr\Http\Message\ResponseInterface)
     */
    public function deactivateApp(string $id, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\DeactivateApp($id), $fetch);
    }
    /**
     * Creates multiple audit log entries in a single operation. Maximum batch size is 100.
     * @param null|\FlowCatalyst\Generated\Model\CreateAuditLogRequest[] $requestBody
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\PostApiAuditLogsBatchBadRequestException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\BatchAuditLogResponse : \Psr\Http\Message\ResponseInterface)
     */
    public function postApiAuditLogsBatch(?array $requestBody = null, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\PostApiAuditLogsBatch($requestBody), $fetch);
    }
    /**
     * List raw dispatch jobs from the transactional collection (debug/admin only)
     * @param array{
     *    "page"?: int,
     *    "size"?: int,
     * } $queryParameters
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\PagedRawDispatchJobResponse : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiBffDebugDispatchJob(array $queryParameters = [], string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiBffDebugDispatchJob($queryParameters), $fetch);
    }
    /**
     * Get a single raw dispatch job by its ID (debug/admin only)
     * @param string $id
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\GetApiBffDebugDispatchJobByIdNotFoundException
     *
     * @return ($fetch is 'object' ? null : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiBffDebugDispatchJobById(string $id, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiBffDebugDispatchJobById($id), $fetch);
    }
    /**
     * List raw events from the transactional collection (debug/admin only)
     * @param array{
     *    "page"?: int,
     *    "size"?: int,
     * } $queryParameters
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\PagedRawEventResponse : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiBffDebugEvent(array $queryParameters = [], string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiBffDebugEvent($queryParameters), $fetch);
    }
    /**
     * Get a single raw event by its ID (debug/admin only)
     * @param string $id
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\GetApiBffDebugEventByIdNotFoundException
     *
     * @return ($fetch is 'object' ? null : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiBffDebugEventById(string $id, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiBffDebugEventById($id), $fetch);
    }
    /**
     * Search dispatch jobs with optional filters and pagination. Multi-value parameters (clientIds, applications, etc.) support comma-separated values for OR filtering. Use 'null' in clientIds to include platform jobs (no client).
     * @param array{
     *    "aggregates"?: string,
     *    "applications"?: string,
     *    "clientIds"?: string,
     *    "codes"?: string,
     *    "createdAfter"?: string,
     *    "createdBefore"?: string,
     *    "dispatchPoolId"?: string,
     *    "kind"?: string,
     *    "messageGroup"?: string,
     *    "page"?: int,
     *    "size"?: int,
     *    "source"?: string,
     *    "statuses"?: string,
     *    "subdomains"?: string,
     *    "subscriptionId"?: string,
     * } $queryParameters
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\PagedDispatchJobReadResponse : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiBffDispatchJob(array $queryParameters = [], string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiBffDispatchJob($queryParameters), $fetch);
    }
    /**
     * Get available filter values for cascading filters. Each level is narrowed by selections at higher levels (client → application → subdomain → aggregate → code).
     * @param array{
     *    "aggregates"?: string,
     *    "applications"?: string,
     *    "clientIds"?: string,
     *    "subdomains"?: string,
     * } $queryParameters
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\FilterOptionsResponse1 : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiBffDispatchJobsFilterOption(array $queryParameters = [], string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiBffDispatchJobsFilterOption($queryParameters), $fetch);
    }
    /**
     * Get a single dispatch job by its ID
     * @param string $id
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\GetApiBffDispatchJobByIdNotFoundException
     *
     * @return ($fetch is 'object' ? null : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiBffDispatchJobById(string $id, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiBffDispatchJobById($id), $fetch);
    }
    /**
     * Search events with optional filters and pagination. Multi-value parameters (clientIds, applications, etc.) support comma-separated values for OR filtering. Use 'null' in clientIds to include platform events (no client).
     * @param array{
     *    "aggregates"?: string,
     *    "applications"?: string,
     *    "clientIds"?: string,
     *    "correlationId"?: string,
     *    "messageGroup"?: string,
     *    "page"?: int,
     *    "size"?: int,
     *    "source"?: string,
     *    "subdomains"?: string,
     *    "subject"?: string,
     *    "timeAfter"?: string,
     *    "timeBefore"?: string,
     *    "types"?: string,
     * } $queryParameters
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\PagedEventResponse : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiBffEvent(array $queryParameters = [], string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiBffEvent($queryParameters), $fetch);
    }
    /**
     * Get available filter values for cascading filters. Each level is narrowed by selections at higher levels (client → application → subdomain → aggregate → type).
     * @param array{
     *    "aggregates"?: string,
     *    "applications"?: string,
     *    "clientIds"?: string,
     *    "subdomains"?: string,
     * } $queryParameters
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\FilterOptionsResponse2 : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiBffEventsFilterOption(array $queryParameters = [], string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiBffEventsFilterOption($queryParameters), $fetch);
    }
    /**
     * Get a single event by its ID
     * @param string $id
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\GetApiBffEventByIdNotFoundException
     *
     * @return ($fetch is 'object' ? null : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiBffEventById(string $id, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiBffEventById($id), $fetch);
    }
    /**
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\GetApiClientUnauthorizedException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ClientListResponse3 : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiClient(string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiClient(), $fetch);
    }
    /**
     * Returns a specific client if the caller has access to it.
     * @param string $id
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\GetApiClientByIdBadRequestException
     * @throws \FlowCatalyst\Generated\Exception\GetApiClientByIdUnauthorizedException
     * @throws \FlowCatalyst\Generated\Exception\GetApiClientByIdForbiddenException
     * @throws \FlowCatalyst\Generated\Exception\GetApiClientByIdNotFoundException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ClientResponse : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiClientById(string $id, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiClientById($id), $fetch);
    }
    /**
     * Returns applications enabled for the specified client.
     * @param string $id
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\GetApiClientsByIdApplicationBadRequestException
     * @throws \FlowCatalyst\Generated\Exception\GetApiClientsByIdApplicationUnauthorizedException
     * @throws \FlowCatalyst\Generated\Exception\GetApiClientsByIdApplicationForbiddenException
     * @throws \FlowCatalyst\Generated\Exception\GetApiClientsByIdApplicationNotFoundException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApplicationListResponse2 : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiClientsByIdApplication(string $id, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiClientsByIdApplication($id), $fetch);
    }
    /**
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\PlatformConfigResponse : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiConfigPlatform(string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiConfigPlatform(), $fetch);
    }
    /**
     * Search for dispatch jobs with optional filters and pagination
     * @param array{
     *    "clientId"?: string,
     *    "code"?: string,
     *    "createdAfter"?: string,
     *    "createdBefore"?: string,
     *    "dispatchPoolId"?: string,
     *    "kind"?: string,
     *    "messageGroup"?: string,
     *    "page"?: int,
     *    "size"?: int,
     *    "source"?: string,
     *    "status"?: string,
     *    "subscriptionId"?: string,
     * } $queryParameters
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\PagedDispatchJobResponse : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiDispatchJob(array $queryParameters = [], string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiDispatchJob($queryParameters), $fetch);
    }
    /**
     * Creates and queues a new dispatch job for webhook delivery
     * @param null|\FlowCatalyst\Generated\Model\CreateDispatchJobRequest $requestBody
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\PostApiDispatchJobBadRequestException
     * @throws \FlowCatalyst\Generated\Exception\PostApiDispatchJobInternalServerErrorException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\DispatchJobResponse : \Psr\Http\Message\ResponseInterface)
     */
    public function postApiDispatchJob(?\FlowCatalyst\Generated\Model\CreateDispatchJobRequest $requestBody = null, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\PostApiDispatchJob($requestBody), $fetch);
    }
    /**
     * Creates multiple dispatch jobs in a single operation. Maximum batch size is 100 jobs.
     * @param null|\FlowCatalyst\Generated\Model\CreateDispatchJobRequest[] $requestBody
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\PostApiDispatchJobsBatchBadRequestException
     * @throws \FlowCatalyst\Generated\Exception\PostApiDispatchJobsBatchInternalServerErrorException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\BatchDispatchJobResponse : \Psr\Http\Message\ResponseInterface)
     */
    public function postApiDispatchJobsBatch(?array $requestBody = null, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\PostApiDispatchJobsBatch($requestBody), $fetch);
    }
    /**
     * Retrieves detailed information about a specific dispatch job
     * @param string $id
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\GetApiDispatchJobByIdNotFoundException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\DispatchJobResponse : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiDispatchJobById(string $id, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiDispatchJobById($id), $fetch);
    }
    /**
     * Retrieves the full history of webhook delivery attempts for a job
     * @param string $id
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\GetApiDispatchJobsByIdAttemptNotFoundException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\DispatchAttemptResponse : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiDispatchJobsByIdAttempt(string $id, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiDispatchJobsByIdAttempt($id), $fetch);
    }
    /**
     * Internal endpoint that executes webhook dispatch and records attempts. Requires HMAC-SHA256 authentication via Bearer token.
     * @param null|\FlowCatalyst\Generated\Model\ProcessRequest $requestBody
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\PostApiDispatchProcessUnauthorizedException
     * @throws \FlowCatalyst\Generated\Exception\PostApiDispatchProcessInternalServerErrorException
     * @throws \FlowCatalyst\Generated\Exception\PostApiDispatchProcessBadRequestException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ProcessResponse : \Psr\Http\Message\ResponseInterface)
     */
    public function postApiDispatchProcess(?\FlowCatalyst\Generated\Model\ProcessRequest $requestBody = null, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\PostApiDispatchProcess($requestBody), $fetch);
    }
    /**
     * Returns all event types with optional filtering. Supports multi-value filtering with comma-separated values.
     * @param array{
     *    "aggregate"?: array,
     *    "application"?: array,
     *    "status"?: string,
     *    "subdomain"?: array,
     * } $queryParameters
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\EventTypeListResponse : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiEventType(array $queryParameters = [], string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiEventType($queryParameters), $fetch);
    }
    /**
     * @param null|\FlowCatalyst\Generated\Model\CreateEventTypeRequest $requestBody
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\PostApiEventTypeBadRequestException
     * @throws \FlowCatalyst\Generated\Exception\PostApiEventTypeConflictException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\EventTypeResponse : \Psr\Http\Message\ResponseInterface)
     */
    public function postApiEventType(?\FlowCatalyst\Generated\Model\CreateEventTypeRequest $requestBody = null, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\PostApiEventType($requestBody), $fetch);
    }
    /**
     * @param array{
     *    "application"?: array,
     *    "subdomain"?: array,
     * } $queryParameters
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\FilterOptionsResponse : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiEventTypesFiltersAggregate(array $queryParameters = [], string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiEventTypesFiltersAggregate($queryParameters), $fetch);
    }
    /**
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\FilterOptionsResponse : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiEventTypesFiltersApplication(string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiEventTypesFiltersApplication(), $fetch);
    }
    /**
     * @param array{
     *    "application"?: array,
     * } $queryParameters
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\FilterOptionsResponse : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiEventTypesFiltersSubdomain(array $queryParameters = [], string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiEventTypesFiltersSubdomain($queryParameters), $fetch);
    }
    /**
     * @param string $id
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\DeleteApiEventTypeByIdBadRequestException
     * @throws \FlowCatalyst\Generated\Exception\DeleteApiEventTypeByIdNotFoundException
     *
     * @return ($fetch is 'object' ? null : \Psr\Http\Message\ResponseInterface)
     */
    public function deleteApiEventTypeById(string $id, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\DeleteApiEventTypeById($id), $fetch);
    }
    /**
     * @param string $id
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\GetApiEventTypeByIdNotFoundException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\EventTypeResponse : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiEventTypeById(string $id, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiEventTypeById($id), $fetch);
    }
    /**
     * @param string $id
     * @param null|\FlowCatalyst\Generated\Model\UpdateEventTypeRequest $requestBody
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\PatchApiEventTypeByIdBadRequestException
     * @throws \FlowCatalyst\Generated\Exception\PatchApiEventTypeByIdNotFoundException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\EventTypeResponse : \Psr\Http\Message\ResponseInterface)
     */
    public function patchApiEventTypeById(string $id, ?\FlowCatalyst\Generated\Model\UpdateEventTypeRequest $requestBody = null, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\PatchApiEventTypeById($id, $requestBody), $fetch);
    }
    /**
     * @param string $id
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\PostApiEventTypesByIdArchiveBadRequestException
     * @throws \FlowCatalyst\Generated\Exception\PostApiEventTypesByIdArchiveNotFoundException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\EventTypeResponse : \Psr\Http\Message\ResponseInterface)
     */
    public function postApiEventTypesByIdArchive(string $id, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\PostApiEventTypesByIdArchive($id), $fetch);
    }
    /**
     * @param string $id
     * @param null|\FlowCatalyst\Generated\Model\AddSchemaRequest $requestBody
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\PostApiEventTypesByIdSchemaBadRequestException
     * @throws \FlowCatalyst\Generated\Exception\PostApiEventTypesByIdSchemaNotFoundException
     * @throws \FlowCatalyst\Generated\Exception\PostApiEventTypesByIdSchemaConflictException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\EventTypeResponse : \Psr\Http\Message\ResponseInterface)
     */
    public function postApiEventTypesByIdSchema(string $id, ?\FlowCatalyst\Generated\Model\AddSchemaRequest $requestBody = null, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\PostApiEventTypesByIdSchema($id, $requestBody), $fetch);
    }
    /**
     * @param string $id
     * @param string $version
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\PostApiEventTypesByIdSchemasByVersionDeprecateBadRequestException
     * @throws \FlowCatalyst\Generated\Exception\PostApiEventTypesByIdSchemasByVersionDeprecateNotFoundException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\EventTypeResponse : \Psr\Http\Message\ResponseInterface)
     */
    public function postApiEventTypesByIdSchemasByVersionDeprecate(string $id, string $version, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\PostApiEventTypesByIdSchemasByVersionDeprecate($id, $version), $fetch);
    }
    /**
     * @param string $id
     * @param string $version
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\PostApiEventTypesByIdSchemasByVersionFinaliseBadRequestException
     * @throws \FlowCatalyst\Generated\Exception\PostApiEventTypesByIdSchemasByVersionFinaliseNotFoundException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\EventTypeResponse : \Psr\Http\Message\ResponseInterface)
     */
    public function postApiEventTypesByIdSchemasByVersionFinalise(string $id, string $version, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\PostApiEventTypesByIdSchemasByVersionFinalise($id, $version), $fetch);
    }
    /**
     * Creates a new event in the event store. If a deduplicationId is provided and an event with that ID already exists, the existing event is returned (idempotent operation). Dispatch jobs are automatically created for matching subscriptions.
     * @param null|\FlowCatalyst\Generated\Model\CreateEventRequest $requestBody
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\PostApiEventBadRequestException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\CreateEventResponse : \Psr\Http\Message\ResponseInterface)
     */
    public function postApiEvent(?\FlowCatalyst\Generated\Model\CreateEventRequest $requestBody = null, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\PostApiEvent($requestBody), $fetch);
    }
    /**
     * Creates multiple events in a single operation. Maximum batch size is 100 events. Dispatch jobs are automatically created for matching subscriptions.
     * @param null|\FlowCatalyst\Generated\Model\CreateEventRequest[] $requestBody
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\PostApiEventsBatchBadRequestException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\BatchEventResponse : \Psr\Http\Message\ResponseInterface)
     */
    public function postApiEventsBatch(?array $requestBody = null, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\PostApiEventsBatch($requestBody), $fetch);
    }
    /**
     * @param string $id
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\GetApiEventByIdNotFoundException
     *
     * @return ($fetch is 'object' ? null : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiEventById(string $id, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiEventById($id), $fetch);
    }
    /**
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\HealthResponse : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiHealth(string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiHealth(), $fetch);
    }
    /**
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\GetMyClientsUnauthorizedException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\MyClientsResponse : \Psr\Http\Message\ResponseInterface)
     */
    public function getMyClients(string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetMyClients(), $fetch);
    }
    /**
     * Returns client details if the user has access to it
     * @param string $clientId
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\GetMyClientUnauthorizedException
     * @throws \FlowCatalyst\Generated\Exception\GetMyClientForbiddenException
     * @throws \FlowCatalyst\Generated\Exception\GetMyClientNotFoundException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\MyClientDto : \Psr\Http\Message\ResponseInterface)
     */
    public function getMyClient(string $clientId, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetMyClient($clientId), $fetch);
    }
    /**
     * Returns applications enabled for a client the user has access to
     * @param string $clientId
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\GetMyClientApplicationsUnauthorizedException
     * @throws \FlowCatalyst\Generated\Exception\GetMyClientApplicationsForbiddenException
     * @throws \FlowCatalyst\Generated\Exception\GetMyClientApplicationsNotFoundException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\MyApplicationsResponse : \Psr\Http\Message\ResponseInterface)
     */
    public function getMyClientApplications(string $clientId, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetMyClientApplications($clientId), $fetch);
    }
    /**
     * Returns theme configuration for the login page. No authentication required.
     * @param array{
     *    "clientId"?: string,
     * } $queryParameters
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     *
     * @return ($fetch is 'object' ? null : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiPublicLoginTheme(array $queryParameters = [], string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiPublicLoginTheme($queryParameters), $fetch);
    }
    /**
     * @param null|string $requestBody
     * @param array{
     *    "X-FlowCatalyst-Causation-ID"?: string,
     *    "X-FlowCatalyst-Code"?: string,
     *    "X-FlowCatalyst-Correlation-ID"?: string,
     *    "X-FlowCatalyst-ID"?: string,
     *    "X-FlowCatalyst-Kind"?: string,
     *    "X-FlowCatalyst-Signature"?: string,
     *    "X-FlowCatalyst-Subject"?: string,
     *    "X-FlowCatalyst-Timestamp"?: string,
     * } $headerParameters
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     *
     * @return ($fetch is 'object' ? null : \Psr\Http\Message\ResponseInterface)
     */
    public function postApiSampleWebhook(?string $requestBody = null, array $headerParameters = [], string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\PostApiSampleWebhook($requestBody, $headerParameters), $fetch);
    }
    /**
     * @param null|string $requestBody
     * @param array{
     *    "X-FlowCatalyst-ID"?: string,
     * } $headerParameters
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     *
     * @return ($fetch is 'object' ? null : \Psr\Http\Message\ResponseInterface)
     */
    public function postApiSampleWebhookFailPermanent(?string $requestBody = null, array $headerParameters = [], string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\PostApiSampleWebhookFailPermanent($requestBody, $headerParameters), $fetch);
    }
    /**
     * @param null|string $requestBody
     * @param array{
     *    "X-FlowCatalyst-ID"?: string,
     * } $headerParameters
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     *
     * @return ($fetch is 'object' ? null : \Psr\Http\Message\ResponseInterface)
     */
    public function postApiSampleWebhookFailTransient(?string $requestBody = null, array $headerParameters = [], string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\PostApiSampleWebhookFailTransient($requestBody, $headerParameters), $fetch);
    }
    /**
     * @param null|string $requestBody
     * @param array{
     *    "X-FlowCatalyst-ID"?: string,
     * } $headerParameters
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     *
     * @return ($fetch is 'object' ? null : \Psr\Http\Message\ResponseInterface)
     */
    public function postApiSampleWebhookSlow(?string $requestBody = null, array $headerParameters = [], string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\PostApiSampleWebhookSlow($requestBody, $headerParameters), $fetch);
    }
    /**
     * @param array{
     *    "status"?: string, //Filter by status
     * } $queryParameters
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\SdkListClientsUnauthorizedException
     * @throws \FlowCatalyst\Generated\Exception\SdkListClientsForbiddenException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ClientListResponse1 : \Psr\Http\Message\ResponseInterface)
     */
    public function sdkListClients(array $queryParameters = [], string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\SdkListClients($queryParameters), $fetch);
    }
    /**
     * @param null|\FlowCatalyst\Generated\Model\CreateClientRequest1 $requestBody
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\SdkCreateClientBadRequestException
     * @throws \FlowCatalyst\Generated\Exception\SdkCreateClientUnauthorizedException
     * @throws \FlowCatalyst\Generated\Exception\SdkCreateClientForbiddenException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ClientDto1 : \Psr\Http\Message\ResponseInterface)
     */
    public function sdkCreateClient(?\FlowCatalyst\Generated\Model\CreateClientRequest1 $requestBody = null, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\SdkCreateClient($requestBody), $fetch);
    }
    /**
     * @param string $id
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\SdkGetClientNotFoundException
     * @throws \FlowCatalyst\Generated\Exception\SdkGetClientUnauthorizedException
     * @throws \FlowCatalyst\Generated\Exception\SdkGetClientForbiddenException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ClientDto1 : \Psr\Http\Message\ResponseInterface)
     */
    public function sdkGetClient(string $id, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\SdkGetClient($id), $fetch);
    }
    /**
     * @param string $id
     * @param null|\FlowCatalyst\Generated\Model\UpdateClientRequest1 $requestBody
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\SdkUpdateClientNotFoundException
     * @throws \FlowCatalyst\Generated\Exception\SdkUpdateClientUnauthorizedException
     * @throws \FlowCatalyst\Generated\Exception\SdkUpdateClientForbiddenException
     * @throws \FlowCatalyst\Generated\Exception\SdkUpdateClientBadRequestException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ClientDto1 : \Psr\Http\Message\ResponseInterface)
     */
    public function sdkUpdateClient(string $id, ?\FlowCatalyst\Generated\Model\UpdateClientRequest1 $requestBody = null, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\SdkUpdateClient($id, $requestBody), $fetch);
    }
    /**
     * @param string $id
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\SdkActivateClientNotFoundException
     * @throws \FlowCatalyst\Generated\Exception\SdkActivateClientUnauthorizedException
     * @throws \FlowCatalyst\Generated\Exception\SdkActivateClientForbiddenException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\StatusResponse : \Psr\Http\Message\ResponseInterface)
     */
    public function sdkActivateClient(string $id, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\SdkActivateClient($id), $fetch);
    }
    /**
     * @param string $id
     * @param null|\FlowCatalyst\Generated\Model\StatusChangeRequest1 $requestBody
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\SdkDeactivateClientNotFoundException
     * @throws \FlowCatalyst\Generated\Exception\SdkDeactivateClientUnauthorizedException
     * @throws \FlowCatalyst\Generated\Exception\SdkDeactivateClientForbiddenException
     * @throws \FlowCatalyst\Generated\Exception\SdkDeactivateClientBadRequestException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\StatusResponse : \Psr\Http\Message\ResponseInterface)
     */
    public function sdkDeactivateClient(string $id, ?\FlowCatalyst\Generated\Model\StatusChangeRequest1 $requestBody = null, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\SdkDeactivateClient($id, $requestBody), $fetch);
    }
    /**
     * @param string $id
     * @param null|\FlowCatalyst\Generated\Model\StatusChangeRequest1 $requestBody
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\SdkSuspendClientNotFoundException
     * @throws \FlowCatalyst\Generated\Exception\SdkSuspendClientUnauthorizedException
     * @throws \FlowCatalyst\Generated\Exception\SdkSuspendClientForbiddenException
     * @throws \FlowCatalyst\Generated\Exception\SdkSuspendClientBadRequestException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\StatusResponse : \Psr\Http\Message\ResponseInterface)
     */
    public function sdkSuspendClient(string $id, ?\FlowCatalyst\Generated\Model\StatusChangeRequest1 $requestBody = null, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\SdkSuspendClient($id, $requestBody), $fetch);
    }
    /**
     * @param array{
     *    "active"?: bool, //Filter by active status
     *    "clientId"?: string, //Filter by client ID
     *    "email"?: string, //Filter by exact email match
     *    "type"?: string, //Filter by type (USER/SERVICE)
     * } $queryParameters
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\SdkListPrincipalsUnauthorizedException
     * @throws \FlowCatalyst\Generated\Exception\SdkListPrincipalsForbiddenException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\PrincipalListResponse : \Psr\Http\Message\ResponseInterface)
     */
    public function sdkListPrincipals(array $queryParameters = [], string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\SdkListPrincipals($queryParameters), $fetch);
    }
    /**
     * @param null|\FlowCatalyst\Generated\Model\CreateServicePrincipalRequest $requestBody
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\SdkCreateServicePrincipalBadRequestException
     * @throws \FlowCatalyst\Generated\Exception\SdkCreateServicePrincipalUnauthorizedException
     * @throws \FlowCatalyst\Generated\Exception\SdkCreateServicePrincipalForbiddenException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\StatusResponse1 : \Psr\Http\Message\ResponseInterface)
     */
    public function sdkCreateServicePrincipal(?\FlowCatalyst\Generated\Model\CreateServicePrincipalRequest $requestBody = null, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\SdkCreateServicePrincipal($requestBody), $fetch);
    }
    /**
     * @param null|\FlowCatalyst\Generated\Model\CreateUserRequest $requestBody
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\SdkCreateUserPrincipalBadRequestException
     * @throws \FlowCatalyst\Generated\Exception\SdkCreateUserPrincipalUnauthorizedException
     * @throws \FlowCatalyst\Generated\Exception\SdkCreateUserPrincipalForbiddenException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\PrincipalDto : \Psr\Http\Message\ResponseInterface)
     */
    public function sdkCreateUserPrincipal(?\FlowCatalyst\Generated\Model\CreateUserRequest $requestBody = null, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\SdkCreateUserPrincipal($requestBody), $fetch);
    }
    /**
     * @param string $id
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\SdkGetPrincipalNotFoundException
     * @throws \FlowCatalyst\Generated\Exception\SdkGetPrincipalUnauthorizedException
     * @throws \FlowCatalyst\Generated\Exception\SdkGetPrincipalForbiddenException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\PrincipalDto : \Psr\Http\Message\ResponseInterface)
     */
    public function sdkGetPrincipal(string $id, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\SdkGetPrincipal($id), $fetch);
    }
    /**
     * @param string $id
     * @param null|\FlowCatalyst\Generated\Model\UpdatePrincipalRequest $requestBody
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\SdkUpdatePrincipalNotFoundException
     * @throws \FlowCatalyst\Generated\Exception\SdkUpdatePrincipalUnauthorizedException
     * @throws \FlowCatalyst\Generated\Exception\SdkUpdatePrincipalForbiddenException
     * @throws \FlowCatalyst\Generated\Exception\SdkUpdatePrincipalBadRequestException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\PrincipalDto : \Psr\Http\Message\ResponseInterface)
     */
    public function sdkUpdatePrincipal(string $id, ?\FlowCatalyst\Generated\Model\UpdatePrincipalRequest $requestBody = null, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\SdkUpdatePrincipal($id, $requestBody), $fetch);
    }
    /**
     * @param string $id
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\SdkActivatePrincipalNotFoundException
     * @throws \FlowCatalyst\Generated\Exception\SdkActivatePrincipalUnauthorizedException
     * @throws \FlowCatalyst\Generated\Exception\SdkActivatePrincipalForbiddenException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\StatusResponse1 : \Psr\Http\Message\ResponseInterface)
     */
    public function sdkActivatePrincipal(string $id, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\SdkActivatePrincipal($id), $fetch);
    }
    /**
     * @param string $id
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\SdkGetPrincipalClientsNotFoundException
     * @throws \FlowCatalyst\Generated\Exception\SdkGetPrincipalClientsUnauthorizedException
     * @throws \FlowCatalyst\Generated\Exception\SdkGetPrincipalClientsForbiddenException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ClientAccessListResponse : \Psr\Http\Message\ResponseInterface)
     */
    public function sdkGetPrincipalClients(string $id, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\SdkGetPrincipalClients($id), $fetch);
    }
    /**
     * @param string $clientId
     * @param string $id
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\SdkRevokeClientAccessNotFoundException
     * @throws \FlowCatalyst\Generated\Exception\SdkRevokeClientAccessUnauthorizedException
     * @throws \FlowCatalyst\Generated\Exception\SdkRevokeClientAccessForbiddenException
     *
     * @return ($fetch is 'object' ? null : \Psr\Http\Message\ResponseInterface)
     */
    public function sdkRevokeClientAccess(string $clientId, string $id, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\SdkRevokeClientAccess($clientId, $id), $fetch);
    }
    /**
     * @param string $clientId
     * @param string $id
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\SdkGrantClientAccessBadRequestException
     * @throws \FlowCatalyst\Generated\Exception\SdkGrantClientAccessNotFoundException
     * @throws \FlowCatalyst\Generated\Exception\SdkGrantClientAccessUnauthorizedException
     * @throws \FlowCatalyst\Generated\Exception\SdkGrantClientAccessForbiddenException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ClientAccessGrantDto : \Psr\Http\Message\ResponseInterface)
     */
    public function sdkGrantClientAccess(string $clientId, string $id, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\SdkGrantClientAccess($clientId, $id), $fetch);
    }
    /**
     * @param string $id
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\SdkDeactivatePrincipalNotFoundException
     * @throws \FlowCatalyst\Generated\Exception\SdkDeactivatePrincipalUnauthorizedException
     * @throws \FlowCatalyst\Generated\Exception\SdkDeactivatePrincipalForbiddenException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\StatusResponse1 : \Psr\Http\Message\ResponseInterface)
     */
    public function sdkDeactivatePrincipal(string $id, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\SdkDeactivatePrincipal($id), $fetch);
    }
    /**
     * @param string $id
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\SdkGetPrincipalRolesNotFoundException
     * @throws \FlowCatalyst\Generated\Exception\SdkGetPrincipalRolesUnauthorizedException
     * @throws \FlowCatalyst\Generated\Exception\SdkGetPrincipalRolesForbiddenException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\RoleListResponse : \Psr\Http\Message\ResponseInterface)
     */
    public function sdkGetPrincipalRoles(string $id, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\SdkGetPrincipalRoles($id), $fetch);
    }
    /**
     * @param string $id
     * @param null|\FlowCatalyst\Generated\Model\AssignRolesRequest $requestBody
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\SdkAssignPrincipalRolesBadRequestException
     * @throws \FlowCatalyst\Generated\Exception\SdkAssignPrincipalRolesNotFoundException
     * @throws \FlowCatalyst\Generated\Exception\SdkAssignPrincipalRolesUnauthorizedException
     * @throws \FlowCatalyst\Generated\Exception\SdkAssignPrincipalRolesForbiddenException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\RolesAssignedResponse : \Psr\Http\Message\ResponseInterface)
     */
    public function sdkAssignPrincipalRoles(string $id, ?\FlowCatalyst\Generated\Model\AssignRolesRequest $requestBody = null, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\SdkAssignPrincipalRoles($id, $requestBody), $fetch);
    }
    /**
     * @param string $id
     * @param string $roleName
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\SdkRemovePrincipalRoleNotFoundException
     * @throws \FlowCatalyst\Generated\Exception\SdkRemovePrincipalRoleUnauthorizedException
     * @throws \FlowCatalyst\Generated\Exception\SdkRemovePrincipalRoleForbiddenException
     *
     * @return ($fetch is 'object' ? null : \Psr\Http\Message\ResponseInterface)
     */
    public function sdkRemovePrincipalRole(string $id, string $roleName, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\SdkRemovePrincipalRole($id, $roleName), $fetch);
    }
    /**
     * @param string $id
     * @param string $roleName
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\SdkAddPrincipalRoleBadRequestException
     * @throws \FlowCatalyst\Generated\Exception\SdkAddPrincipalRoleNotFoundException
     * @throws \FlowCatalyst\Generated\Exception\SdkAddPrincipalRoleUnauthorizedException
     * @throws \FlowCatalyst\Generated\Exception\SdkAddPrincipalRoleForbiddenException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\RoleAssignmentDto : \Psr\Http\Message\ResponseInterface)
     */
    public function sdkAddPrincipalRole(string $id, string $roleName, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\SdkAddPrincipalRole($id, $roleName), $fetch);
    }
    /**
     * @param array{
     *    "application"?: string, //Filter by application code
     *    "source"?: string, //Filter by source (CODE, DATABASE, SDK)
     * } $queryParameters
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\SdkListRolesUnauthorizedException
     * @throws \FlowCatalyst\Generated\Exception\SdkListRolesForbiddenException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\RoleListResponse1 : \Psr\Http\Message\ResponseInterface)
     */
    public function sdkListRoles(array $queryParameters = [], string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\SdkListRoles($queryParameters), $fetch);
    }
    /**
     * @param null|\FlowCatalyst\Generated\Model\CreateRoleRequest $requestBody
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\SdkCreateRoleBadRequestException
     * @throws \FlowCatalyst\Generated\Exception\SdkCreateRoleNotFoundException
     * @throws \FlowCatalyst\Generated\Exception\SdkCreateRoleConflictException
     * @throws \FlowCatalyst\Generated\Exception\SdkCreateRoleUnauthorizedException
     * @throws \FlowCatalyst\Generated\Exception\SdkCreateRoleForbiddenException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\RoleDto : \Psr\Http\Message\ResponseInterface)
     */
    public function sdkCreateRole(?\FlowCatalyst\Generated\Model\CreateRoleRequest $requestBody = null, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\SdkCreateRole($requestBody), $fetch);
    }
    /**
     * @param string $roleName
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\SdkDeleteRoleBadRequestException
     * @throws \FlowCatalyst\Generated\Exception\SdkDeleteRoleNotFoundException
     * @throws \FlowCatalyst\Generated\Exception\SdkDeleteRoleUnauthorizedException
     * @throws \FlowCatalyst\Generated\Exception\SdkDeleteRoleForbiddenException
     *
     * @return ($fetch is 'object' ? null : \Psr\Http\Message\ResponseInterface)
     */
    public function sdkDeleteRole(string $roleName, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\SdkDeleteRole($roleName), $fetch);
    }
    /**
     * @param string $roleName
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\SdkGetRoleNotFoundException
     * @throws \FlowCatalyst\Generated\Exception\SdkGetRoleUnauthorizedException
     * @throws \FlowCatalyst\Generated\Exception\SdkGetRoleForbiddenException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\RoleDto : \Psr\Http\Message\ResponseInterface)
     */
    public function sdkGetRole(string $roleName, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\SdkGetRole($roleName), $fetch);
    }
    /**
     * @param string $roleName
     * @param null|\FlowCatalyst\Generated\Model\UpdateRoleRequest $requestBody
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\SdkUpdateRoleNotFoundException
     * @throws \FlowCatalyst\Generated\Exception\SdkUpdateRoleUnauthorizedException
     * @throws \FlowCatalyst\Generated\Exception\SdkUpdateRoleForbiddenException
     * @throws \FlowCatalyst\Generated\Exception\SdkUpdateRoleBadRequestException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\RoleDto : \Psr\Http\Message\ResponseInterface)
     */
    public function sdkUpdateRole(string $roleName, ?\FlowCatalyst\Generated\Model\UpdateRoleRequest $requestBody = null, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\SdkUpdateRole($roleName, $requestBody), $fetch);
    }
    /**
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\PlatformStats : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiStat(string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiStat(), $fetch);
    }
    /**
     * Determines whether to show password login or redirect to SSO
     * @param null|\FlowCatalyst\Generated\Model\DomainCheckRequest $requestBody
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\PostAuthCheckDomainBadRequestException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\DomainCheckResponse : \Psr\Http\Message\ResponseInterface)
     */
    public function postAuthCheckDomain(?\FlowCatalyst\Generated\Model\DomainCheckRequest $requestBody = null, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\PostAuthCheckDomain($requestBody), $fetch);
    }
    /**
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\GetAuthClientAccessibleUnauthorizedException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\AccessibleClientsResponse : \Psr\Http\Message\ResponseInterface)
     */
    public function getAuthClientAccessible(string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetAuthClientAccessible(), $fetch);
    }
    /**
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\GetAuthClientCurrentUnauthorizedException
     *
     * @return ($fetch is 'object' ? null : \Psr\Http\Message\ResponseInterface)
     */
    public function getAuthClientCurrent(string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetAuthClientCurrent(), $fetch);
    }
    /**
     * @param null|\FlowCatalyst\Generated\Model\SwitchClientRequest $requestBody
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\PostAuthClientSwitchUnauthorizedException
     * @throws \FlowCatalyst\Generated\Exception\PostAuthClientSwitchForbiddenException
     * @throws \FlowCatalyst\Generated\Exception\PostAuthClientSwitchBadRequestException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\SwitchClientResponse : \Psr\Http\Message\ResponseInterface)
     */
    public function postAuthClientSwitch(?\FlowCatalyst\Generated\Model\SwitchClientRequest $requestBody = null, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\PostAuthClientSwitch($requestBody), $fetch);
    }
    /**
     * @param null|\FlowCatalyst\Generated\Model\LoginRequest $requestBody
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\PostAuthLoginUnauthorizedException
     * @throws \FlowCatalyst\Generated\Exception\PostAuthLoginBadRequestException
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
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\SessionUserResponse : \Psr\Http\Message\ResponseInterface)
     */
    public function getAuthMe(string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetAuthMe(), $fetch);
    }
    /**
     * Handles callback from external IDP
     * @param array{
     *    "code"?: string,
     *    "error"?: string,
     *    "error_description"?: string,
     *    "state"?: string,
     * } $queryParameters
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     *
     * @return ($fetch is 'object' ? null : \Psr\Http\Message\ResponseInterface)
     */
    public function getAuthOidcCallback(array $queryParameters = [], string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetAuthOidcCallback($queryParameters), $fetch);
    }
    /**
     * Redirects to external IDP for authentication
     * @param array{
     *    "domain": string, //Email domain to authenticate
     *    "oauth_client_id"?: string,
     *    "oauth_code_challenge"?: string,
     *    "oauth_code_challenge_method"?: string,
     *    "oauth_nonce"?: string,
     *    "oauth_redirect_uri"?: string,
     *    "oauth_scope"?: string,
     *    "oauth_state"?: string,
     *    "return_url"?: string, //URL to return to after login
     * } $queryParameters
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     *
     * @return ($fetch is 'object' ? null : \Psr\Http\Message\ResponseInterface)
     */
    public function getAuthOidcLogin(array $queryParameters = [], string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetAuthOidcLogin($queryParameters), $fetch);
    }
    /**
     * @param array{
     *    "aggregate"?: array,
     *    "application"?: array,
     *    "status"?: string,
     *    "subdomain"?: array,
     * } $queryParameters
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     *
     * @return ($fetch is 'object' ? null : \Psr\Http\Message\ResponseInterface)
     */
    public function getBffEventType(array $queryParameters = [], string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetBffEventType($queryParameters), $fetch);
    }
    /**
     * @param null|\FlowCatalyst\Generated\Model\CreateEventTypeRequest1 $requestBody
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\PostBffEventTypeBadRequestException
     *
     * @return ($fetch is 'object' ? null : \Psr\Http\Message\ResponseInterface)
     */
    public function postBffEventType(?\FlowCatalyst\Generated\Model\CreateEventTypeRequest1 $requestBody = null, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\PostBffEventType($requestBody), $fetch);
    }
    /**
     * @param array{
     *    "application"?: array,
     *    "subdomain"?: array,
     * } $queryParameters
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     *
     * @return ($fetch is 'object' ? null : \Psr\Http\Message\ResponseInterface)
     */
    public function getBffEventTypesFiltersAggregate(array $queryParameters = [], string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetBffEventTypesFiltersAggregate($queryParameters), $fetch);
    }
    /**
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     *
     * @return ($fetch is 'object' ? null : \Psr\Http\Message\ResponseInterface)
     */
    public function getBffEventTypesFiltersApplication(string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetBffEventTypesFiltersApplication(), $fetch);
    }
    /**
     * @param array{
     *    "application"?: array,
     * } $queryParameters
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     *
     * @return ($fetch is 'object' ? null : \Psr\Http\Message\ResponseInterface)
     */
    public function getBffEventTypesFiltersSubdomain(array $queryParameters = [], string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetBffEventTypesFiltersSubdomain($queryParameters), $fetch);
    }
    /**
     * @param string $id
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     *
     * @return ($fetch is 'object' ? null : \Psr\Http\Message\ResponseInterface)
     */
    public function deleteBffEventTypeById(string $id, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\DeleteBffEventTypeById($id), $fetch);
    }
    /**
     * @param string $id
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     *
     * @return ($fetch is 'object' ? null : \Psr\Http\Message\ResponseInterface)
     */
    public function getBffEventTypeById(string $id, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetBffEventTypeById($id), $fetch);
    }
    /**
     * @param string $id
     * @param null|\FlowCatalyst\Generated\Model\UpdateEventTypeRequest1 $requestBody
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     *
     * @return ($fetch is 'object' ? null : \Psr\Http\Message\ResponseInterface)
     */
    public function patchBffEventTypeById(string $id, ?\FlowCatalyst\Generated\Model\UpdateEventTypeRequest1 $requestBody = null, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\PatchBffEventTypeById($id, $requestBody), $fetch);
    }
    /**
     * @param string $id
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     *
     * @return ($fetch is 'object' ? null : \Psr\Http\Message\ResponseInterface)
     */
    public function postBffEventTypesByIdArchive(string $id, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\PostBffEventTypesByIdArchive($id), $fetch);
    }
    /**
     * @param string $id
     * @param null|\FlowCatalyst\Generated\Model\AddSchemaRequest1 $requestBody
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\PostBffEventTypesByIdSchemaBadRequestException
     *
     * @return ($fetch is 'object' ? null : \Psr\Http\Message\ResponseInterface)
     */
    public function postBffEventTypesByIdSchema(string $id, ?\FlowCatalyst\Generated\Model\AddSchemaRequest1 $requestBody = null, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\PostBffEventTypesByIdSchema($id, $requestBody), $fetch);
    }
    /**
     * @param string $id
     * @param string $version
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     *
     * @return ($fetch is 'object' ? null : \Psr\Http\Message\ResponseInterface)
     */
    public function postBffEventTypesByIdSchemasByVersionDeprecate(string $id, string $version, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\PostBffEventTypesByIdSchemasByVersionDeprecate($id, $version), $fetch);
    }
    /**
     * @param string $id
     * @param string $version
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     *
     * @return ($fetch is 'object' ? null : \Psr\Http\Message\ResponseInterface)
     */
    public function postBffEventTypesByIdSchemasByVersionFinalise(string $id, string $version, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\PostBffEventTypesByIdSchemasByVersionFinalise($id, $version), $fetch);
    }
    /**
     * @param array{
     *    "application"?: string,
     *    "source"?: string,
     * } $queryParameters
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     *
     * @return ($fetch is 'object' ? null : \Psr\Http\Message\ResponseInterface)
     */
    public function getBffRole(array $queryParameters = [], string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetBffRole($queryParameters), $fetch);
    }
    /**
     * @param null|\FlowCatalyst\Generated\Model\CreateRoleRequest3 $requestBody
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\PostBffRoleBadRequestException
     *
     * @return ($fetch is 'object' ? null : \Psr\Http\Message\ResponseInterface)
     */
    public function postBffRole(?\FlowCatalyst\Generated\Model\CreateRoleRequest3 $requestBody = null, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\PostBffRole($requestBody), $fetch);
    }
    /**
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     *
     * @return ($fetch is 'object' ? null : \Psr\Http\Message\ResponseInterface)
     */
    public function getBffRolesFiltersApplication(string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetBffRolesFiltersApplication(), $fetch);
    }
    /**
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     *
     * @return ($fetch is 'object' ? null : \Psr\Http\Message\ResponseInterface)
     */
    public function getBffRolesPermission(string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetBffRolesPermission(), $fetch);
    }
    /**
     * @param string $permission
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     *
     * @return ($fetch is 'object' ? null : \Psr\Http\Message\ResponseInterface)
     */
    public function getBffRolesPermissionByPermission(string $permission, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetBffRolesPermissionByPermission($permission), $fetch);
    }
    /**
     * @param string $roleName
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     *
     * @return ($fetch is 'object' ? null : \Psr\Http\Message\ResponseInterface)
     */
    public function deleteBffRoleByRoleName(string $roleName, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\DeleteBffRoleByRoleName($roleName), $fetch);
    }
    /**
     * @param string $roleName
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     *
     * @return ($fetch is 'object' ? null : \Psr\Http\Message\ResponseInterface)
     */
    public function getBffRoleByRoleName(string $roleName, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetBffRoleByRoleName($roleName), $fetch);
    }
    /**
     * @param string $roleName
     * @param null|\FlowCatalyst\Generated\Model\UpdateRoleRequest2 $requestBody
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\PutBffRoleByRoleNameBadRequestException
     *
     * @return ($fetch is 'object' ? null : \Psr\Http\Message\ResponseInterface)
     */
    public function putBffRoleByRoleName(string $roleName, ?\FlowCatalyst\Generated\Model\UpdateRoleRequest2 $requestBody = null, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\PutBffRoleByRoleName($roleName, $requestBody), $fetch);
    }
    /**
     * @param array{
     *    "client_id"?: string, //OAuth client ID
     *    "code_challenge"?: string, //PKCE code challenge
     *    "code_challenge_method"?: string, //PKCE challenge method (S256 or plain)
     *    "nonce"?: string, //OIDC nonce for replay protection
     *    "redirect_uri"?: string, //URI to redirect after authorization
     *    "response_type"?: string, //Must be 'code'
     *    "scope"?: string, //Requested scopes (space-separated)
     *    "state"?: string, //Client state for CSRF protection
     * } $queryParameters
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     *
     * @return ($fetch is 'object' ? null : \Psr\Http\Message\ResponseInterface)
     */
    public function getOauthAuthorize(array $queryParameters = [], string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetOauthAuthorize($queryParameters), $fetch);
    }
    /**
     * @param null|\FlowCatalyst\Generated\Model\OauthTokenPostBody $requestBody
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\PostOauthTokenBadRequestException
     *
     * @return ($fetch is 'object' ? null : \Psr\Http\Message\ResponseInterface)
     */
    public function postOauthToken(?\FlowCatalyst\Generated\Model\OauthTokenPostBody $requestBody = null, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\PostOauthToken($requestBody), $fetch);
    }
    public static function create($httpClient = null, array $additionalPlugins = [], array $additionalNormalizers = [])
    {
        if (null === $httpClient) {
            $httpClient = \Http\Discovery\Psr18ClientDiscovery::find();
            $plugins = [];
            $uri = \Http\Discovery\Psr17FactoryDiscovery::findUriFactory()->createUri('http://localhost');
            $plugins[] = new \Http\Client\Common\Plugin\AddHostPlugin($uri);
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