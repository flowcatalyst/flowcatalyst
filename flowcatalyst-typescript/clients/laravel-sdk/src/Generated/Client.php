<?php

namespace FlowCatalyst\Generated;

class Client extends \FlowCatalyst\Generated\Runtime\Client\Client
{
    /**
     * @param array{
     *    "type"?: string,
     *    "clientId"?: string,
     *    "active"?: string,
     *    "email"?: string,
     *    "q"?: string,
     *    "page"?: string,
     *    "pageSize"?: string,
     * } $queryParameters
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiAdminPrincipalsGetResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiAdminPrincipal(array $queryParameters = [], string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiAdminPrincipal($queryParameters), $fetch);
    }
    /**
     * @param string $id
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\DeleteApiAdminPrincipalByIdNotFoundException
     *
     * @return ($fetch is 'object' ? null : \Psr\Http\Message\ResponseInterface)
     */
    public function deleteApiAdminPrincipalById(string $id, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\DeleteApiAdminPrincipalById($id), $fetch);
    }
    /**
     * @param string $id
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\GetApiAdminPrincipalByIdNotFoundException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdGetResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiAdminPrincipalById(string $id, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiAdminPrincipalById($id), $fetch);
    }
    /**
     * @param string $id
     * @param null|\FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdPutBody $requestBody
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\PutApiAdminPrincipalByIdBadRequestException
     * @throws \FlowCatalyst\Generated\Exception\PutApiAdminPrincipalByIdNotFoundException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdPutResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function putApiAdminPrincipalById(string $id, ?\FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdPutBody $requestBody = null, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\PutApiAdminPrincipalById($id, $requestBody), $fetch);
    }
    /**
     * @param null|\FlowCatalyst\Generated\Model\ApiAdminPrincipalsUsersPostBody $requestBody
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\PostApiAdminPrincipalsUserBadRequestException
     * @throws \FlowCatalyst\Generated\Exception\PostApiAdminPrincipalsUserConflictException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiAdminPrincipalsUsersPostResponse201 : \Psr\Http\Message\ResponseInterface)
     */
    public function postApiAdminPrincipalsUser(?\FlowCatalyst\Generated\Model\ApiAdminPrincipalsUsersPostBody $requestBody = null, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\PostApiAdminPrincipalsUser($requestBody), $fetch);
    }
    /**
     * @param string $id
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\PostApiAdminPrincipalsByIdActivateNotFoundException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdActivatePostResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function postApiAdminPrincipalsByIdActivate(string $id, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\PostApiAdminPrincipalsByIdActivate($id), $fetch);
    }
    /**
     * @param string $id
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\PostApiAdminPrincipalsByIdDeactivateNotFoundException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdDeactivatePostResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function postApiAdminPrincipalsByIdDeactivate(string $id, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\PostApiAdminPrincipalsByIdDeactivate($id), $fetch);
    }
    /**
     * @param string $id
     * @param null|\FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdResetPasswordPostBody $requestBody
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\PostApiAdminPrincipalsByIdResetPasswordBadRequestException
     * @throws \FlowCatalyst\Generated\Exception\PostApiAdminPrincipalsByIdResetPasswordNotFoundException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdResetPasswordPostResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function postApiAdminPrincipalsByIdResetPassword(string $id, ?\FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdResetPasswordPostBody $requestBody = null, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\PostApiAdminPrincipalsByIdResetPassword($id, $requestBody), $fetch);
    }
    /**
     * @param string $id
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\GetApiAdminPrincipalsByIdRoleNotFoundException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdRolesGetResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiAdminPrincipalsByIdRole(string $id, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiAdminPrincipalsByIdRole($id), $fetch);
    }
    /**
     * @param string $id
     * @param null|\FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdRolesPostBody $requestBody
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\PostApiAdminPrincipalsByIdRoleBadRequestException
     * @throws \FlowCatalyst\Generated\Exception\PostApiAdminPrincipalsByIdRoleNotFoundException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdRolesPostResponse201 : \Psr\Http\Message\ResponseInterface)
     */
    public function postApiAdminPrincipalsByIdRole(string $id, ?\FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdRolesPostBody $requestBody = null, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\PostApiAdminPrincipalsByIdRole($id, $requestBody), $fetch);
    }
    /**
     * @param string $id
     * @param null|\FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdRolesPutBody $requestBody
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\PutApiAdminPrincipalsByIdRoleBadRequestException
     * @throws \FlowCatalyst\Generated\Exception\PutApiAdminPrincipalsByIdRoleNotFoundException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdRolesPutResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function putApiAdminPrincipalsByIdRole(string $id, ?\FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdRolesPutBody $requestBody = null, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\PutApiAdminPrincipalsByIdRole($id, $requestBody), $fetch);
    }
    /**
     * @param string $id
     * @param string $roleName
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\DeleteApiAdminPrincipalsByIdRoleByRoleNameNotFoundException
     *
     * @return ($fetch is 'object' ? null : \Psr\Http\Message\ResponseInterface)
     */
    public function deleteApiAdminPrincipalsByIdRoleByRoleName(string $id, string $roleName, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\DeleteApiAdminPrincipalsByIdRoleByRoleName($id, $roleName), $fetch);
    }
    /**
     * @param string $id
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\GetApiAdminPrincipalsByIdClientAccessNotFoundException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdClientAccessGetResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiAdminPrincipalsByIdClientAccess(string $id, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiAdminPrincipalsByIdClientAccess($id), $fetch);
    }
    /**
     * @param string $id
     * @param null|\FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdClientAccessPostBody $requestBody
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\PostApiAdminPrincipalsByIdClientAccessBadRequestException
     * @throws \FlowCatalyst\Generated\Exception\PostApiAdminPrincipalsByIdClientAccessNotFoundException
     * @throws \FlowCatalyst\Generated\Exception\PostApiAdminPrincipalsByIdClientAccessConflictException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdClientAccessPostResponse201 : \Psr\Http\Message\ResponseInterface)
     */
    public function postApiAdminPrincipalsByIdClientAccess(string $id, ?\FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdClientAccessPostBody $requestBody = null, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\PostApiAdminPrincipalsByIdClientAccess($id, $requestBody), $fetch);
    }
    /**
     * @param string $id
     * @param string $clientId
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
     * @param array{
     *    "email"?: string,
     * } $queryParameters
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\GetApiAdminPrincipalsCheckEmailDomainBadRequestException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiAdminPrincipalsCheckEmailDomainGetResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiAdminPrincipalsCheckEmailDomain(array $queryParameters = [], string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiAdminPrincipalsCheckEmailDomain($queryParameters), $fetch);
    }
    /**
     * @param string $id
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\GetApiAdminPrincipalsByIdApplicationAccessNotFoundException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdApplicationAccessGetResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiAdminPrincipalsByIdApplicationAccess(string $id, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiAdminPrincipalsByIdApplicationAccess($id), $fetch);
    }
    /**
     * @param string $id
     * @param null|\FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdApplicationAccessPutBody $requestBody
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\PutApiAdminPrincipalsByIdApplicationAccessBadRequestException
     * @throws \FlowCatalyst\Generated\Exception\PutApiAdminPrincipalsByIdApplicationAccessNotFoundException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdApplicationAccessPutResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function putApiAdminPrincipalsByIdApplicationAccess(string $id, ?\FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdApplicationAccessPutBody $requestBody = null, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\PutApiAdminPrincipalsByIdApplicationAccess($id, $requestBody), $fetch);
    }
    /**
     * @param string $id
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\GetApiAdminPrincipalsByIdAvailableApplicationNotFoundException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdAvailableApplicationsGetResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiAdminPrincipalsByIdAvailableApplication(string $id, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiAdminPrincipalsByIdAvailableApplication($id), $fetch);
    }
    /**
     * @param array{
     *    "status"?: string,
     *    "page"?: string,
     *    "pageSize"?: string,
     * } $queryParameters
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiAdminClientsGetResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiAdminClient(array $queryParameters = [], string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiAdminClient($queryParameters), $fetch);
    }
    /**
     * @param null|\FlowCatalyst\Generated\Model\ApiAdminClientsPostBody $requestBody
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\PostApiAdminClientBadRequestException
     * @throws \FlowCatalyst\Generated\Exception\PostApiAdminClientConflictException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiAdminClientsPostResponse201 : \Psr\Http\Message\ResponseInterface)
     */
    public function postApiAdminClient(?\FlowCatalyst\Generated\Model\ApiAdminClientsPostBody $requestBody = null, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\PostApiAdminClient($requestBody), $fetch);
    }
    /**
     * @param array{
     *    "q"?: string,
     *    "status"?: string,
     *    "limit"?: string,
     * } $queryParameters
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiAdminClientsSearchGetResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiAdminClientsSearch(array $queryParameters = [], string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiAdminClientsSearch($queryParameters), $fetch);
    }
    /**
     * @param string $id
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\DeleteApiAdminClientByIdNotFoundException
     *
     * @return ($fetch is 'object' ? null : \Psr\Http\Message\ResponseInterface)
     */
    public function deleteApiAdminClientById(string $id, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\DeleteApiAdminClientById($id), $fetch);
    }
    /**
     * @param string $id
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\GetApiAdminClientByIdNotFoundException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiAdminClientsIdGetResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiAdminClientById(string $id, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiAdminClientById($id), $fetch);
    }
    /**
     * @param string $id
     * @param null|\FlowCatalyst\Generated\Model\ApiAdminClientsIdPutBody $requestBody
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\PutApiAdminClientByIdBadRequestException
     * @throws \FlowCatalyst\Generated\Exception\PutApiAdminClientByIdNotFoundException
     * @throws \FlowCatalyst\Generated\Exception\PutApiAdminClientByIdConflictException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiAdminClientsIdPutResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function putApiAdminClientById(string $id, ?\FlowCatalyst\Generated\Model\ApiAdminClientsIdPutBody $requestBody = null, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\PutApiAdminClientById($id, $requestBody), $fetch);
    }
    /**
     * @param string $identifier
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\GetApiAdminClientsByIdentifierByIdentifierNotFoundException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiAdminClientsByIdentifierIdentifierGetResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiAdminClientsByIdentifierByIdentifier(string $identifier, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiAdminClientsByIdentifierByIdentifier($identifier), $fetch);
    }
    /**
     * @param string $id
     * @param null|\FlowCatalyst\Generated\Model\ApiAdminClientsIdActivatePostBody $requestBody
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\PostApiAdminClientsByIdActivateBadRequestException
     * @throws \FlowCatalyst\Generated\Exception\PostApiAdminClientsByIdActivateNotFoundException
     * @throws \FlowCatalyst\Generated\Exception\PostApiAdminClientsByIdActivateConflictException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiAdminClientsIdActivatePostResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function postApiAdminClientsByIdActivate(string $id, ?\FlowCatalyst\Generated\Model\ApiAdminClientsIdActivatePostBody $requestBody = null, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\PostApiAdminClientsByIdActivate($id, $requestBody), $fetch);
    }
    /**
     * @param string $id
     * @param null|\FlowCatalyst\Generated\Model\ApiAdminClientsIdSuspendPostBody $requestBody
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\PostApiAdminClientsByIdSuspendBadRequestException
     * @throws \FlowCatalyst\Generated\Exception\PostApiAdminClientsByIdSuspendNotFoundException
     * @throws \FlowCatalyst\Generated\Exception\PostApiAdminClientsByIdSuspendConflictException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiAdminClientsIdSuspendPostResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function postApiAdminClientsByIdSuspend(string $id, ?\FlowCatalyst\Generated\Model\ApiAdminClientsIdSuspendPostBody $requestBody = null, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\PostApiAdminClientsByIdSuspend($id, $requestBody), $fetch);
    }
    /**
     * @param string $id
     * @param null|\FlowCatalyst\Generated\Model\ApiAdminClientsIdDeactivatePostBody $requestBody
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\PostApiAdminClientsByIdDeactivateBadRequestException
     * @throws \FlowCatalyst\Generated\Exception\PostApiAdminClientsByIdDeactivateNotFoundException
     * @throws \FlowCatalyst\Generated\Exception\PostApiAdminClientsByIdDeactivateConflictException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiAdminClientsIdDeactivatePostResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function postApiAdminClientsByIdDeactivate(string $id, ?\FlowCatalyst\Generated\Model\ApiAdminClientsIdDeactivatePostBody $requestBody = null, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\PostApiAdminClientsByIdDeactivate($id, $requestBody), $fetch);
    }
    /**
     * @param string $id
     * @param null|\FlowCatalyst\Generated\Model\ApiAdminClientsIdNotesPostBody $requestBody
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\PostApiAdminClientsByIdNoteBadRequestException
     * @throws \FlowCatalyst\Generated\Exception\PostApiAdminClientsByIdNoteNotFoundException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiAdminClientsIdNotesPostResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function postApiAdminClientsByIdNote(string $id, ?\FlowCatalyst\Generated\Model\ApiAdminClientsIdNotesPostBody $requestBody = null, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\PostApiAdminClientsByIdNote($id, $requestBody), $fetch);
    }
    /**
     * @param string $id
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\GetApiAdminClientsByIdApplicationNotFoundException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiAdminClientsIdApplicationsGetResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiAdminClientsByIdApplication(string $id, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiAdminClientsByIdApplication($id), $fetch);
    }
    /**
     * @param string $id
     * @param null|\FlowCatalyst\Generated\Model\ApiAdminClientsIdApplicationsPutBody $requestBody
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\PutApiAdminClientsByIdApplicationNotFoundException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiAdminClientsIdApplicationsPutResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function putApiAdminClientsByIdApplication(string $id, ?\FlowCatalyst\Generated\Model\ApiAdminClientsIdApplicationsPutBody $requestBody = null, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\PutApiAdminClientsByIdApplication($id, $requestBody), $fetch);
    }
    /**
     * @param string $id
     * @param string $appId
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\PostApiAdminClientsByIdApplicationsByAppIdEnableNotFoundException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiAdminClientsIdApplicationsAppIdEnablePostResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function postApiAdminClientsByIdApplicationsByAppIdEnable(string $id, string $appId, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\PostApiAdminClientsByIdApplicationsByAppIdEnable($id, $appId), $fetch);
    }
    /**
     * @param string $id
     * @param string $appId
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\PostApiAdminClientsByIdApplicationsByAppIdDisableNotFoundException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiAdminClientsIdApplicationsAppIdDisablePostResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function postApiAdminClientsByIdApplicationsByAppIdDisable(string $id, string $appId, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\PostApiAdminClientsByIdApplicationsByAppIdDisable($id, $appId), $fetch);
    }
    /**
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiAdminAnchorDomainsGetResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiAdminAnchorDomain(string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiAdminAnchorDomain(), $fetch);
    }
    /**
     * @param null|\FlowCatalyst\Generated\Model\ApiAdminAnchorDomainsPostBody $requestBody
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\PostApiAdminAnchorDomainBadRequestException
     * @throws \FlowCatalyst\Generated\Exception\PostApiAdminAnchorDomainConflictException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiAdminAnchorDomainsPostResponse201 : \Psr\Http\Message\ResponseInterface)
     */
    public function postApiAdminAnchorDomain(?\FlowCatalyst\Generated\Model\ApiAdminAnchorDomainsPostBody $requestBody = null, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\PostApiAdminAnchorDomain($requestBody), $fetch);
    }
    /**
     * @param string $id
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\DeleteApiAdminAnchorDomainByIdNotFoundException
     *
     * @return ($fetch is 'object' ? null : \Psr\Http\Message\ResponseInterface)
     */
    public function deleteApiAdminAnchorDomainById(string $id, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\DeleteApiAdminAnchorDomainById($id), $fetch);
    }
    /**
     * @param string $id
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\GetApiAdminAnchorDomainByIdNotFoundException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiAdminAnchorDomainsIdGetResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiAdminAnchorDomainById(string $id, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiAdminAnchorDomainById($id), $fetch);
    }
    /**
     * @param string $id
     * @param null|\FlowCatalyst\Generated\Model\ApiAdminAnchorDomainsIdPutBody $requestBody
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\PutApiAdminAnchorDomainByIdBadRequestException
     * @throws \FlowCatalyst\Generated\Exception\PutApiAdminAnchorDomainByIdNotFoundException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiAdminAnchorDomainsIdPutResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function putApiAdminAnchorDomainById(string $id, ?\FlowCatalyst\Generated\Model\ApiAdminAnchorDomainsIdPutBody $requestBody = null, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\PutApiAdminAnchorDomainById($id, $requestBody), $fetch);
    }
    /**
     * @param array{
     *    "page"?: string,
     *    "pageSize"?: string,
     *    "type"?: string,
     *    "activeOnly"?: string,
     * } $queryParameters
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiAdminApplicationsGetResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiAdminApplication(array $queryParameters = [], string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiAdminApplication($queryParameters), $fetch);
    }
    /**
     * @param null|\FlowCatalyst\Generated\Model\ApiAdminApplicationsPostBody $requestBody
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\PostApiAdminApplicationBadRequestException
     * @throws \FlowCatalyst\Generated\Exception\PostApiAdminApplicationConflictException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiAdminApplicationsPostResponse201 : \Psr\Http\Message\ResponseInterface)
     */
    public function postApiAdminApplication(?\FlowCatalyst\Generated\Model\ApiAdminApplicationsPostBody $requestBody = null, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\PostApiAdminApplication($requestBody), $fetch);
    }
    /**
     * @param string $id
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\DeleteApiAdminApplicationByIdNotFoundException
     *
     * @return ($fetch is 'object' ? null : \Psr\Http\Message\ResponseInterface)
     */
    public function deleteApiAdminApplicationById(string $id, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\DeleteApiAdminApplicationById($id), $fetch);
    }
    /**
     * @param string $id
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\GetApiAdminApplicationByIdNotFoundException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiAdminApplicationsIdGetResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiAdminApplicationById(string $id, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiAdminApplicationById($id), $fetch);
    }
    /**
     * @param string $id
     * @param null|\FlowCatalyst\Generated\Model\ApiAdminApplicationsIdPutBody $requestBody
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\PutApiAdminApplicationByIdBadRequestException
     * @throws \FlowCatalyst\Generated\Exception\PutApiAdminApplicationByIdNotFoundException
     * @throws \FlowCatalyst\Generated\Exception\PutApiAdminApplicationByIdConflictException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiAdminApplicationsIdPutResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function putApiAdminApplicationById(string $id, ?\FlowCatalyst\Generated\Model\ApiAdminApplicationsIdPutBody $requestBody = null, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\PutApiAdminApplicationById($id, $requestBody), $fetch);
    }
    /**
     * @param string $code
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\GetApiAdminApplicationsByCodeByCodeNotFoundException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiAdminApplicationsByCodeCodeGetResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiAdminApplicationsByCodeByCode(string $code, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiAdminApplicationsByCodeByCode($code), $fetch);
    }
    /**
     * @param string $id
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\PostApiAdminApplicationsByIdActivateNotFoundException
     * @throws \FlowCatalyst\Generated\Exception\PostApiAdminApplicationsByIdActivateConflictException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiAdminApplicationsIdActivatePostResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function postApiAdminApplicationsByIdActivate(string $id, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\PostApiAdminApplicationsByIdActivate($id), $fetch);
    }
    /**
     * @param string $id
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\PostApiAdminApplicationsByIdDeactivateNotFoundException
     * @throws \FlowCatalyst\Generated\Exception\PostApiAdminApplicationsByIdDeactivateConflictException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiAdminApplicationsIdDeactivatePostResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function postApiAdminApplicationsByIdDeactivate(string $id, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\PostApiAdminApplicationsByIdDeactivate($id), $fetch);
    }
    /**
     * @param string $id
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\GetApiAdminApplicationsByIdClientNotFoundException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiAdminApplicationsIdClientsGetResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiAdminApplicationsByIdClient(string $id, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiAdminApplicationsByIdClient($id), $fetch);
    }
    /**
     * @param string $id
     * @param null|\FlowCatalyst\Generated\Model\ApiAdminApplicationsIdClientsPostBody $requestBody
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\PostApiAdminApplicationsByIdClientBadRequestException
     * @throws \FlowCatalyst\Generated\Exception\PostApiAdminApplicationsByIdClientNotFoundException
     * @throws \FlowCatalyst\Generated\Exception\PostApiAdminApplicationsByIdClientConflictException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiAdminApplicationsIdClientsPostResponse201 : \Psr\Http\Message\ResponseInterface)
     */
    public function postApiAdminApplicationsByIdClient(string $id, ?\FlowCatalyst\Generated\Model\ApiAdminApplicationsIdClientsPostBody $requestBody = null, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\PostApiAdminApplicationsByIdClient($id, $requestBody), $fetch);
    }
    /**
     * @param string $id
     * @param string $clientId
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\DeleteApiAdminApplicationsByIdClientByClientIdNotFoundException
     *
     * @return ($fetch is 'object' ? null : \Psr\Http\Message\ResponseInterface)
     */
    public function deleteApiAdminApplicationsByIdClientByClientId(string $id, string $clientId, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\DeleteApiAdminApplicationsByIdClientByClientId($id, $clientId), $fetch);
    }
    /**
     * @param string $id
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\GetApiAdminApplicationsByIdRoleNotFoundException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiAdminApplicationsIdRolesGetResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiAdminApplicationsByIdRole(string $id, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiAdminApplicationsByIdRole($id), $fetch);
    }
    /**
     * @param string $id
     * @param null|\FlowCatalyst\Generated\Model\ApiAdminApplicationsIdProvisionServiceAccountPostBody $requestBody
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\PostApiAdminApplicationsByIdProvisionServiceAccountBadRequestException
     * @throws \FlowCatalyst\Generated\Exception\PostApiAdminApplicationsByIdProvisionServiceAccountNotFoundException
     * @throws \FlowCatalyst\Generated\Exception\PostApiAdminApplicationsByIdProvisionServiceAccountConflictException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiAdminApplicationsIdProvisionServiceAccountPostResponse201 : \Psr\Http\Message\ResponseInterface)
     */
    public function postApiAdminApplicationsByIdProvisionServiceAccount(string $id, ?\FlowCatalyst\Generated\Model\ApiAdminApplicationsIdProvisionServiceAccountPostBody $requestBody = null, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\PostApiAdminApplicationsByIdProvisionServiceAccount($id, $requestBody), $fetch);
    }
    /**
     * @param array{
     *    "page"?: string,
     *    "pageSize"?: string,
     *    "q"?: string,
     *    "source"?: string,
     *    "applicationId"?: string,
     * } $queryParameters
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiAdminRolesGetResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiAdminRole(array $queryParameters = [], string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiAdminRole($queryParameters), $fetch);
    }
    /**
     * @param null|\FlowCatalyst\Generated\Model\ApiAdminRolesPostBody $requestBody
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\PostApiAdminRoleBadRequestException
     * @throws \FlowCatalyst\Generated\Exception\PostApiAdminRoleConflictException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiAdminRolesPostResponse201 : \Psr\Http\Message\ResponseInterface)
     */
    public function postApiAdminRole(?\FlowCatalyst\Generated\Model\ApiAdminRolesPostBody $requestBody = null, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\PostApiAdminRole($requestBody), $fetch);
    }
    /**
     * @param string $name
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\DeleteApiAdminRoleByNameNotFoundException
     *
     * @return ($fetch is 'object' ? null : \Psr\Http\Message\ResponseInterface)
     */
    public function deleteApiAdminRoleByName(string $name, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\DeleteApiAdminRoleByName($name), $fetch);
    }
    /**
     * @param string $name
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\GetApiAdminRoleByNameNotFoundException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiAdminRolesNameGetResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiAdminRoleByName(string $name, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiAdminRoleByName($name), $fetch);
    }
    /**
     * @param string $name
     * @param null|\FlowCatalyst\Generated\Model\ApiAdminRolesNamePutBody $requestBody
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\PutApiAdminRoleByNameBadRequestException
     * @throws \FlowCatalyst\Generated\Exception\PutApiAdminRoleByNameNotFoundException
     * @throws \FlowCatalyst\Generated\Exception\PutApiAdminRoleByNameConflictException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiAdminRolesNamePutResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function putApiAdminRoleByName(string $name, ?\FlowCatalyst\Generated\Model\ApiAdminRolesNamePutBody $requestBody = null, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\PutApiAdminRoleByName($name, $requestBody), $fetch);
    }
    /**
     * @param string $source
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\GetApiAdminRolesBySourceBySourceBadRequestException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiAdminRolesBySourceSourceGetResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiAdminRolesBySourceBySource(string $source, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiAdminRolesBySourceBySource($source), $fetch);
    }
    /**
     * @param string $applicationId
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiAdminRolesByApplicationApplicationIdGetResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiAdminRolesByApplicationByApplicationId(string $applicationId, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiAdminRolesByApplicationByApplicationId($applicationId), $fetch);
    }
    /**
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiAdminRolesPermissionsGetResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiAdminRolesPermission(string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiAdminRolesPermission(), $fetch);
    }
    /**
     * @param string $permission
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\GetApiAdminRolesPermissionByPermissionNotFoundException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiAdminRolesPermissionsPermissionGetResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiAdminRolesPermissionByPermission(string $permission, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiAdminRolesPermissionByPermission($permission), $fetch);
    }
    /**
     * @param array{
     *    "clientId"?: string,
     *    "configType"?: string,
     * } $queryParameters
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiAdminAuthConfigsGetResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiAdminAuthConfig(array $queryParameters = [], string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiAdminAuthConfig($queryParameters), $fetch);
    }
    /**
     * @param string $id
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\DeleteApiAdminAuthConfigByIdNotFoundException
     *
     * @return ($fetch is 'object' ? null : \Psr\Http\Message\ResponseInterface)
     */
    public function deleteApiAdminAuthConfigById(string $id, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\DeleteApiAdminAuthConfigById($id), $fetch);
    }
    /**
     * @param string $id
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\GetApiAdminAuthConfigByIdNotFoundException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiAdminAuthConfigsIdGetResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiAdminAuthConfigById(string $id, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiAdminAuthConfigById($id), $fetch);
    }
    /**
     * @param string $domain
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\GetApiAdminAuthConfigsByDomainByDomainNotFoundException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiAdminAuthConfigsByDomainDomainGetResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiAdminAuthConfigsByDomainByDomain(string $domain, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiAdminAuthConfigsByDomainByDomain($domain), $fetch);
    }
    /**
     * @param null|\FlowCatalyst\Generated\Model\ApiAdminAuthConfigsInternalPostBody $requestBody
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\PostApiAdminAuthConfigsInternalBadRequestException
     * @throws \FlowCatalyst\Generated\Exception\PostApiAdminAuthConfigsInternalConflictException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiAdminAuthConfigsInternalPostResponse201 : \Psr\Http\Message\ResponseInterface)
     */
    public function postApiAdminAuthConfigsInternal(?\FlowCatalyst\Generated\Model\ApiAdminAuthConfigsInternalPostBody $requestBody = null, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\PostApiAdminAuthConfigsInternal($requestBody), $fetch);
    }
    /**
     * @param null|\FlowCatalyst\Generated\Model\ApiAdminAuthConfigsOidcPostBody $requestBody
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\PostApiAdminAuthConfigsOidcBadRequestException
     * @throws \FlowCatalyst\Generated\Exception\PostApiAdminAuthConfigsOidcConflictException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiAdminAuthConfigsOidcPostResponse201 : \Psr\Http\Message\ResponseInterface)
     */
    public function postApiAdminAuthConfigsOidc(?\FlowCatalyst\Generated\Model\ApiAdminAuthConfigsOidcPostBody $requestBody = null, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\PostApiAdminAuthConfigsOidc($requestBody), $fetch);
    }
    /**
     * @param string $id
     * @param null|\FlowCatalyst\Generated\Model\ApiAdminAuthConfigsIdOidcPutBody $requestBody
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\PutApiAdminAuthConfigsByIdOidcBadRequestException
     * @throws \FlowCatalyst\Generated\Exception\PutApiAdminAuthConfigsByIdOidcNotFoundException
     * @throws \FlowCatalyst\Generated\Exception\PutApiAdminAuthConfigsByIdOidcConflictException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiAdminAuthConfigsIdOidcPutResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function putApiAdminAuthConfigsByIdOidc(string $id, ?\FlowCatalyst\Generated\Model\ApiAdminAuthConfigsIdOidcPutBody $requestBody = null, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\PutApiAdminAuthConfigsByIdOidc($id, $requestBody), $fetch);
    }
    /**
     * @param string $id
     * @param null|\FlowCatalyst\Generated\Model\ApiAdminAuthConfigsIdConfigTypePutBody $requestBody
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\PutApiAdminAuthConfigsByIdConfigTypeBadRequestException
     * @throws \FlowCatalyst\Generated\Exception\PutApiAdminAuthConfigsByIdConfigTypeNotFoundException
     * @throws \FlowCatalyst\Generated\Exception\PutApiAdminAuthConfigsByIdConfigTypeConflictException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiAdminAuthConfigsIdConfigTypePutResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function putApiAdminAuthConfigsByIdConfigType(string $id, ?\FlowCatalyst\Generated\Model\ApiAdminAuthConfigsIdConfigTypePutBody $requestBody = null, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\PutApiAdminAuthConfigsByIdConfigType($id, $requestBody), $fetch);
    }
    /**
     * @param string $id
     * @param null|\FlowCatalyst\Generated\Model\ApiAdminAuthConfigsIdAdditionalClientsPutBody $requestBody
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\PutApiAdminAuthConfigsByIdAdditionalClientBadRequestException
     * @throws \FlowCatalyst\Generated\Exception\PutApiAdminAuthConfigsByIdAdditionalClientNotFoundException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiAdminAuthConfigsIdAdditionalClientsPutResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function putApiAdminAuthConfigsByIdAdditionalClient(string $id, ?\FlowCatalyst\Generated\Model\ApiAdminAuthConfigsIdAdditionalClientsPutBody $requestBody = null, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\PutApiAdminAuthConfigsByIdAdditionalClient($id, $requestBody), $fetch);
    }
    /**
     * @param string $id
     * @param null|\FlowCatalyst\Generated\Model\ApiAdminAuthConfigsIdGrantedClientsPutBody $requestBody
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\PutApiAdminAuthConfigsByIdGrantedClientBadRequestException
     * @throws \FlowCatalyst\Generated\Exception\PutApiAdminAuthConfigsByIdGrantedClientNotFoundException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiAdminAuthConfigsIdGrantedClientsPutResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function putApiAdminAuthConfigsByIdGrantedClient(string $id, ?\FlowCatalyst\Generated\Model\ApiAdminAuthConfigsIdGrantedClientsPutBody $requestBody = null, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\PutApiAdminAuthConfigsByIdGrantedClient($id, $requestBody), $fetch);
    }
    /**
     * @param array{
     *    "active"?: string,
     * } $queryParameters
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiAdminOauthClientsGetResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiAdminOauthClient(array $queryParameters = [], string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiAdminOauthClient($queryParameters), $fetch);
    }
    /**
     * @param null|\FlowCatalyst\Generated\Model\ApiAdminOauthClientsPostBody $requestBody
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\PostApiAdminOauthClientBadRequestException
     * @throws \FlowCatalyst\Generated\Exception\PostApiAdminOauthClientConflictException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiAdminOauthClientsPostResponse201 : \Psr\Http\Message\ResponseInterface)
     */
    public function postApiAdminOauthClient(?\FlowCatalyst\Generated\Model\ApiAdminOauthClientsPostBody $requestBody = null, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\PostApiAdminOauthClient($requestBody), $fetch);
    }
    /**
     * @param string $id
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\DeleteApiAdminOauthClientByIdNotFoundException
     *
     * @return ($fetch is 'object' ? null : \Psr\Http\Message\ResponseInterface)
     */
    public function deleteApiAdminOauthClientById(string $id, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\DeleteApiAdminOauthClientById($id), $fetch);
    }
    /**
     * @param string $id
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\GetApiAdminOauthClientByIdNotFoundException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiAdminOauthClientsIdGetResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiAdminOauthClientById(string $id, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiAdminOauthClientById($id), $fetch);
    }
    /**
     * @param string $id
     * @param null|\FlowCatalyst\Generated\Model\ApiAdminOauthClientsIdPutBody $requestBody
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\PutApiAdminOauthClientByIdBadRequestException
     * @throws \FlowCatalyst\Generated\Exception\PutApiAdminOauthClientByIdNotFoundException
     * @throws \FlowCatalyst\Generated\Exception\PutApiAdminOauthClientByIdConflictException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiAdminOauthClientsIdPutResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function putApiAdminOauthClientById(string $id, ?\FlowCatalyst\Generated\Model\ApiAdminOauthClientsIdPutBody $requestBody = null, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\PutApiAdminOauthClientById($id, $requestBody), $fetch);
    }
    /**
     * @param string $clientId
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\GetApiAdminOauthClientsByClientIdByClientIdNotFoundException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiAdminOauthClientsByClientIdClientIdGetResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiAdminOauthClientsByClientIdByClientId(string $clientId, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiAdminOauthClientsByClientIdByClientId($clientId), $fetch);
    }
    /**
     * @param string $id
     * @param null|\FlowCatalyst\Generated\Model\ApiAdminOauthClientsIdRegenerateSecretPostBody $requestBody
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\PostApiAdminOauthClientsByIdRegenerateSecretBadRequestException
     * @throws \FlowCatalyst\Generated\Exception\PostApiAdminOauthClientsByIdRegenerateSecretNotFoundException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiAdminOauthClientsIdRegenerateSecretPostResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function postApiAdminOauthClientsByIdRegenerateSecret(string $id, ?\FlowCatalyst\Generated\Model\ApiAdminOauthClientsIdRegenerateSecretPostBody $requestBody = null, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\PostApiAdminOauthClientsByIdRegenerateSecret($id, $requestBody), $fetch);
    }
    /**
     * @param string $id
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\PostApiAdminOauthClientsByIdActivateNotFoundException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiAdminOauthClientsIdActivatePostResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function postApiAdminOauthClientsByIdActivate(string $id, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\PostApiAdminOauthClientsByIdActivate($id), $fetch);
    }
    /**
     * @param string $id
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\PostApiAdminOauthClientsByIdDeactivateNotFoundException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiAdminOauthClientsIdDeactivatePostResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function postApiAdminOauthClientsByIdDeactivate(string $id, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\PostApiAdminOauthClientsByIdDeactivate($id), $fetch);
    }
    /**
     * @param string $id
     * @param null|\FlowCatalyst\Generated\Model\ApiAdminOauthClientsIdRotateSecretPostBody $requestBody
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\PostApiAdminOauthClientsByIdRotateSecretBadRequestException
     * @throws \FlowCatalyst\Generated\Exception\PostApiAdminOauthClientsByIdRotateSecretNotFoundException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiAdminOauthClientsIdRotateSecretPostResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function postApiAdminOauthClientsByIdRotateSecret(string $id, ?\FlowCatalyst\Generated\Model\ApiAdminOauthClientsIdRotateSecretPostBody $requestBody = null, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\PostApiAdminOauthClientsByIdRotateSecret($id, $requestBody), $fetch);
    }
    /**
     * @param array{
     *    "entityType"?: string,
     *    "entityId"?: string,
     *    "principalId"?: string,
     *    "operation"?: string,
     *    "page"?: string,
     *    "pageSize"?: string,
     * } $queryParameters
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiAdminAuditLogsGetResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiAdminAuditLog(array $queryParameters = [], string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiAdminAuditLog($queryParameters), $fetch);
    }
    /**
     * @param string $id
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\GetApiAdminAuditLogByIdNotFoundException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiAdminAuditLogsIdGetResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiAdminAuditLogById(string $id, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiAdminAuditLogById($id), $fetch);
    }
    /**
     * @param string $entityType
     * @param string $entityId
     * @param array{
     *    "page"?: string,
     *    "pageSize"?: string,
     * } $queryParameters
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiAdminAuditLogsEntityEntityTypeEntityIdGetResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiAdminAuditLogsEntityByEntityTypeByEntityId(string $entityType, string $entityId, array $queryParameters = [], string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiAdminAuditLogsEntityByEntityTypeByEntityId($entityType, $entityId, $queryParameters), $fetch);
    }
    /**
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiAdminAuditLogsEntityTypesGetResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiAdminAuditLogsEntityType(string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiAdminAuditLogsEntityType(), $fetch);
    }
    /**
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiAdminAuditLogsOperationsGetResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiAdminAuditLogsOperation(string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiAdminAuditLogsOperation(), $fetch);
    }
    /**
     * @param array{
     *    "status"?: string,
     *    "application"?: array,
     *    "subdomain"?: array,
     *    "aggregate"?: array,
     * } $queryParameters
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiAdminEventTypesGetResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiAdminEventType(array $queryParameters = [], string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiAdminEventType($queryParameters), $fetch);
    }
    /**
     * @param null|\FlowCatalyst\Generated\Model\ApiAdminEventTypesPostBody $requestBody
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\PostApiAdminEventTypeBadRequestException
     * @throws \FlowCatalyst\Generated\Exception\PostApiAdminEventTypeConflictException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiAdminEventTypesPostResponse201 : \Psr\Http\Message\ResponseInterface)
     */
    public function postApiAdminEventType(?\FlowCatalyst\Generated\Model\ApiAdminEventTypesPostBody $requestBody = null, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\PostApiAdminEventType($requestBody), $fetch);
    }
    /**
     * @param string $id
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\DeleteApiAdminEventTypeByIdNotFoundException
     *
     * @return ($fetch is 'object' ? null : \Psr\Http\Message\ResponseInterface)
     */
    public function deleteApiAdminEventTypeById(string $id, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\DeleteApiAdminEventTypeById($id), $fetch);
    }
    /**
     * @param string $id
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\GetApiAdminEventTypeByIdNotFoundException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiAdminEventTypesIdGetResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiAdminEventTypeById(string $id, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiAdminEventTypeById($id), $fetch);
    }
    /**
     * @param string $id
     * @param null|\FlowCatalyst\Generated\Model\ApiAdminEventTypesIdPatchBody $requestBody
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\PatchApiAdminEventTypeByIdBadRequestException
     * @throws \FlowCatalyst\Generated\Exception\PatchApiAdminEventTypeByIdNotFoundException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiAdminEventTypesIdPatchResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function patchApiAdminEventTypeById(string $id, ?\FlowCatalyst\Generated\Model\ApiAdminEventTypesIdPatchBody $requestBody = null, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\PatchApiAdminEventTypeById($id, $requestBody), $fetch);
    }
    /**
     * @param string $id
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\PostApiAdminEventTypesByIdArchiveNotFoundException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiAdminEventTypesIdArchivePostResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function postApiAdminEventTypesByIdArchive(string $id, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\PostApiAdminEventTypesByIdArchive($id), $fetch);
    }
    /**
     * @param string $id
     * @param null|\FlowCatalyst\Generated\Model\ApiAdminEventTypesIdSchemasPostBody $requestBody
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\PostApiAdminEventTypesByIdSchemaBadRequestException
     * @throws \FlowCatalyst\Generated\Exception\PostApiAdminEventTypesByIdSchemaNotFoundException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiAdminEventTypesIdSchemasPostResponse201 : \Psr\Http\Message\ResponseInterface)
     */
    public function postApiAdminEventTypesByIdSchema(string $id, ?\FlowCatalyst\Generated\Model\ApiAdminEventTypesIdSchemasPostBody $requestBody = null, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\PostApiAdminEventTypesByIdSchema($id, $requestBody), $fetch);
    }
    /**
     * @param string $id
     * @param string $version
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\PostApiAdminEventTypesByIdSchemasByVersionFinaliseNotFoundException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiAdminEventTypesIdSchemasVersionFinalisePostResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function postApiAdminEventTypesByIdSchemasByVersionFinalise(string $id, string $version, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\PostApiAdminEventTypesByIdSchemasByVersionFinalise($id, $version), $fetch);
    }
    /**
     * @param string $id
     * @param string $version
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\PostApiAdminEventTypesByIdSchemasByVersionDeprecateNotFoundException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiAdminEventTypesIdSchemasVersionDeprecatePostResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function postApiAdminEventTypesByIdSchemasByVersionDeprecate(string $id, string $version, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\PostApiAdminEventTypesByIdSchemasByVersionDeprecate($id, $version), $fetch);
    }
    /**
     * @param null|\FlowCatalyst\Generated\Model\ApiAdminEventTypesSyncPostBody $requestBody
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\PostApiAdminEventTypesSyncBadRequestException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiAdminEventTypesSyncPostResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function postApiAdminEventTypesSync(?\FlowCatalyst\Generated\Model\ApiAdminEventTypesSyncPostBody $requestBody = null, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\PostApiAdminEventTypesSync($requestBody), $fetch);
    }
    /**
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiAdminEventTypesFiltersApplicationsGetResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiAdminEventTypesFiltersApplication(string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiAdminEventTypesFiltersApplication(), $fetch);
    }
    /**
     * @param array{
     *    "application"?: array,
     * } $queryParameters
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiAdminEventTypesFiltersSubdomainsGetResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiAdminEventTypesFiltersSubdomain(array $queryParameters = [], string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiAdminEventTypesFiltersSubdomain($queryParameters), $fetch);
    }
    /**
     * @param array{
     *    "application"?: array,
     *    "subdomain"?: array,
     * } $queryParameters
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiAdminEventTypesFiltersAggregatesGetResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiAdminEventTypesFiltersAggregate(array $queryParameters = [], string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiAdminEventTypesFiltersAggregate($queryParameters), $fetch);
    }
    /**
     * @param array{
     *    "clientId"?: string,
     *    "status"?: string,
     *    "anchorLevel"?: string,
     * } $queryParameters
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiAdminDispatchPoolsGetResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiAdminDispatchPool(array $queryParameters = [], string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiAdminDispatchPool($queryParameters), $fetch);
    }
    /**
     * @param null|\FlowCatalyst\Generated\Model\ApiAdminDispatchPoolsPostBody $requestBody
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\PostApiAdminDispatchPoolBadRequestException
     * @throws \FlowCatalyst\Generated\Exception\PostApiAdminDispatchPoolConflictException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiAdminDispatchPoolsPostResponse201 : \Psr\Http\Message\ResponseInterface)
     */
    public function postApiAdminDispatchPool(?\FlowCatalyst\Generated\Model\ApiAdminDispatchPoolsPostBody $requestBody = null, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\PostApiAdminDispatchPool($requestBody), $fetch);
    }
    /**
     * @param string $id
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\DeleteApiAdminDispatchPoolByIdNotFoundException
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
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiAdminDispatchPoolsIdGetResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiAdminDispatchPoolById(string $id, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiAdminDispatchPoolById($id), $fetch);
    }
    /**
     * @param string $id
     * @param null|\FlowCatalyst\Generated\Model\ApiAdminDispatchPoolsIdPutBody $requestBody
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\PutApiAdminDispatchPoolByIdBadRequestException
     * @throws \FlowCatalyst\Generated\Exception\PutApiAdminDispatchPoolByIdNotFoundException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiAdminDispatchPoolsIdPutResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function putApiAdminDispatchPoolById(string $id, ?\FlowCatalyst\Generated\Model\ApiAdminDispatchPoolsIdPutBody $requestBody = null, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\PutApiAdminDispatchPoolById($id, $requestBody), $fetch);
    }
    /**
     * @param string $id
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\PostApiAdminDispatchPoolsByIdSuspendNotFoundException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiAdminDispatchPoolsIdSuspendPostResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function postApiAdminDispatchPoolsByIdSuspend(string $id, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\PostApiAdminDispatchPoolsByIdSuspend($id), $fetch);
    }
    /**
     * @param string $id
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\PostApiAdminDispatchPoolsByIdActivateNotFoundException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiAdminDispatchPoolsIdActivatePostResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function postApiAdminDispatchPoolsByIdActivate(string $id, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\PostApiAdminDispatchPoolsByIdActivate($id), $fetch);
    }
    /**
     * @param null|\FlowCatalyst\Generated\Model\ApiAdminDispatchPoolsSyncPostBody $requestBody
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\PostApiAdminDispatchPoolsSyncBadRequestException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiAdminDispatchPoolsSyncPostResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function postApiAdminDispatchPoolsSync(?\FlowCatalyst\Generated\Model\ApiAdminDispatchPoolsSyncPostBody $requestBody = null, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\PostApiAdminDispatchPoolsSync($requestBody), $fetch);
    }
    /**
     * @param array{
     *    "clientId"?: string,
     *    "status"?: string,
     *    "source"?: string,
     *    "dispatchPoolId"?: string,
     *    "anchorLevel"?: string,
     * } $queryParameters
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiAdminSubscriptionsGetResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiAdminSubscription(array $queryParameters = [], string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiAdminSubscription($queryParameters), $fetch);
    }
    /**
     * @param null|\FlowCatalyst\Generated\Model\ApiAdminSubscriptionsPostBody $requestBody
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\PostApiAdminSubscriptionBadRequestException
     * @throws \FlowCatalyst\Generated\Exception\PostApiAdminSubscriptionConflictException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiAdminSubscriptionsPostResponse201 : \Psr\Http\Message\ResponseInterface)
     */
    public function postApiAdminSubscription(?\FlowCatalyst\Generated\Model\ApiAdminSubscriptionsPostBody $requestBody = null, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\PostApiAdminSubscription($requestBody), $fetch);
    }
    /**
     * @param string $id
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\DeleteApiAdminSubscriptionByIdNotFoundException
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
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiAdminSubscriptionsIdGetResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiAdminSubscriptionById(string $id, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiAdminSubscriptionById($id), $fetch);
    }
    /**
     * @param string $id
     * @param null|\FlowCatalyst\Generated\Model\ApiAdminSubscriptionsIdPutBody $requestBody
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\PutApiAdminSubscriptionByIdBadRequestException
     * @throws \FlowCatalyst\Generated\Exception\PutApiAdminSubscriptionByIdNotFoundException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiAdminSubscriptionsIdPutResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function putApiAdminSubscriptionById(string $id, ?\FlowCatalyst\Generated\Model\ApiAdminSubscriptionsIdPutBody $requestBody = null, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\PutApiAdminSubscriptionById($id, $requestBody), $fetch);
    }
    /**
     * @param string $id
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\PostApiAdminSubscriptionsByIdPauseNotFoundException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiAdminSubscriptionsIdPausePostResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function postApiAdminSubscriptionsByIdPause(string $id, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\PostApiAdminSubscriptionsByIdPause($id), $fetch);
    }
    /**
     * @param string $id
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\PostApiAdminSubscriptionsByIdResumeNotFoundException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiAdminSubscriptionsIdResumePostResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function postApiAdminSubscriptionsByIdResume(string $id, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\PostApiAdminSubscriptionsByIdResume($id), $fetch);
    }
    /**
     * @param null|\FlowCatalyst\Generated\Model\ApiAdminSubscriptionsSyncPostBody $requestBody
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\PostApiAdminSubscriptionsSyncBadRequestException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiAdminSubscriptionsSyncPostResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function postApiAdminSubscriptionsSync(?\FlowCatalyst\Generated\Model\ApiAdminSubscriptionsSyncPostBody $requestBody = null, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\PostApiAdminSubscriptionsSync($requestBody), $fetch);
    }
    /**
     * @param array{
     *    "clientIds"?: string,
     *    "applications"?: string,
     *    "subdomains"?: string,
     *    "aggregates"?: string,
     *    "types"?: string,
     *    "source"?: string,
     *    "subject"?: string,
     *    "correlationId"?: string,
     *    "messageGroup"?: string,
     *    "timeAfter"?: string,
     *    "timeBefore"?: string,
     *    "page"?: string,
     *    "size"?: string,
     * } $queryParameters
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiAdminEventsGetResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiAdminEvent(array $queryParameters = [], string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiAdminEvent($queryParameters), $fetch);
    }
    /**
     * @param array{
     *    "clientIds"?: string,
     *    "applications"?: string,
     *    "subdomains"?: string,
     *    "aggregates"?: string,
     * } $queryParameters
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiAdminEventsFilterOptionsGetResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiAdminEventsFilterOption(array $queryParameters = [], string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiAdminEventsFilterOption($queryParameters), $fetch);
    }
    /**
     * @param string $id
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\GetApiAdminEventByIdNotFoundException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiAdminEventsIdGetResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiAdminEventById(string $id, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiAdminEventById($id), $fetch);
    }
    /**
     * @param array{
     *    "clientIds"?: string,
     *    "statuses"?: string,
     *    "applications"?: string,
     *    "subdomains"?: string,
     *    "aggregates"?: string,
     *    "codes"?: string,
     *    "source"?: string,
     *    "kind"?: string,
     *    "subscriptionId"?: string,
     *    "dispatchPoolId"?: string,
     *    "messageGroup"?: string,
     *    "createdAfter"?: string,
     *    "createdBefore"?: string,
     *    "page"?: string,
     *    "size"?: string,
     * } $queryParameters
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiAdminDispatchJobsGetResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiAdminDispatchJob(array $queryParameters = [], string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiAdminDispatchJob($queryParameters), $fetch);
    }
    /**
     * @param array{
     *    "clientIds"?: string,
     *    "applications"?: string,
     *    "subdomains"?: string,
     *    "aggregates"?: string,
     * } $queryParameters
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiAdminDispatchJobsFilterOptionsGetResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiAdminDispatchJobsFilterOption(array $queryParameters = [], string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiAdminDispatchJobsFilterOption($queryParameters), $fetch);
    }
    /**
     * @param string $id
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\GetApiAdminDispatchJobByIdNotFoundException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiAdminDispatchJobsIdGetResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiAdminDispatchJobById(string $id, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiAdminDispatchJobById($id), $fetch);
    }
    /**
     * @param string $id
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\GetApiAdminDispatchJobsByIdAttemptNotFoundException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiAdminDispatchJobsIdAttemptsGetResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiAdminDispatchJobsByIdAttempt(string $id, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiAdminDispatchJobsByIdAttempt($id), $fetch);
    }
    /**
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiAdminIdentityProvidersGetResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiAdminIdentityProvider(string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiAdminIdentityProvider(), $fetch);
    }
    /**
     * @param null|\FlowCatalyst\Generated\Model\ApiAdminIdentityProvidersPostBody $requestBody
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\PostApiAdminIdentityProviderBadRequestException
     * @throws \FlowCatalyst\Generated\Exception\PostApiAdminIdentityProviderConflictException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiAdminIdentityProvidersPostResponse201 : \Psr\Http\Message\ResponseInterface)
     */
    public function postApiAdminIdentityProvider(?\FlowCatalyst\Generated\Model\ApiAdminIdentityProvidersPostBody $requestBody = null, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\PostApiAdminIdentityProvider($requestBody), $fetch);
    }
    /**
     * @param string $id
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\DeleteApiAdminIdentityProviderByIdNotFoundException
     *
     * @return ($fetch is 'object' ? null : \Psr\Http\Message\ResponseInterface)
     */
    public function deleteApiAdminIdentityProviderById(string $id, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\DeleteApiAdminIdentityProviderById($id), $fetch);
    }
    /**
     * @param string $id
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\GetApiAdminIdentityProviderByIdNotFoundException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiAdminIdentityProvidersIdGetResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiAdminIdentityProviderById(string $id, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiAdminIdentityProviderById($id), $fetch);
    }
    /**
     * @param string $id
     * @param null|\FlowCatalyst\Generated\Model\ApiAdminIdentityProvidersIdPutBody $requestBody
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\PutApiAdminIdentityProviderByIdBadRequestException
     * @throws \FlowCatalyst\Generated\Exception\PutApiAdminIdentityProviderByIdNotFoundException
     * @throws \FlowCatalyst\Generated\Exception\PutApiAdminIdentityProviderByIdConflictException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiAdminIdentityProvidersIdPutResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function putApiAdminIdentityProviderById(string $id, ?\FlowCatalyst\Generated\Model\ApiAdminIdentityProvidersIdPutBody $requestBody = null, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\PutApiAdminIdentityProviderById($id, $requestBody), $fetch);
    }
    /**
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiAdminEmailDomainMappingsGetResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiAdminEmailDomainMapping(string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiAdminEmailDomainMapping(), $fetch);
    }
    /**
     * @param null|\FlowCatalyst\Generated\Model\ApiAdminEmailDomainMappingsPostBody $requestBody
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\PostApiAdminEmailDomainMappingBadRequestException
     * @throws \FlowCatalyst\Generated\Exception\PostApiAdminEmailDomainMappingConflictException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiAdminEmailDomainMappingsPostResponse201 : \Psr\Http\Message\ResponseInterface)
     */
    public function postApiAdminEmailDomainMapping(?\FlowCatalyst\Generated\Model\ApiAdminEmailDomainMappingsPostBody $requestBody = null, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\PostApiAdminEmailDomainMapping($requestBody), $fetch);
    }
    /**
     * @param string $id
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\DeleteApiAdminEmailDomainMappingByIdNotFoundException
     *
     * @return ($fetch is 'object' ? null : \Psr\Http\Message\ResponseInterface)
     */
    public function deleteApiAdminEmailDomainMappingById(string $id, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\DeleteApiAdminEmailDomainMappingById($id), $fetch);
    }
    /**
     * @param string $id
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\GetApiAdminEmailDomainMappingByIdNotFoundException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiAdminEmailDomainMappingsIdGetResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiAdminEmailDomainMappingById(string $id, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiAdminEmailDomainMappingById($id), $fetch);
    }
    /**
     * @param string $id
     * @param null|\FlowCatalyst\Generated\Model\ApiAdminEmailDomainMappingsIdPutBody $requestBody
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\PutApiAdminEmailDomainMappingByIdBadRequestException
     * @throws \FlowCatalyst\Generated\Exception\PutApiAdminEmailDomainMappingByIdNotFoundException
     * @throws \FlowCatalyst\Generated\Exception\PutApiAdminEmailDomainMappingByIdConflictException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiAdminEmailDomainMappingsIdPutResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function putApiAdminEmailDomainMappingById(string $id, ?\FlowCatalyst\Generated\Model\ApiAdminEmailDomainMappingsIdPutBody $requestBody = null, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\PutApiAdminEmailDomainMappingById($id, $requestBody), $fetch);
    }
    /**
     * @param string $domain
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\GetApiAdminEmailDomainMappingsLookupByDomainNotFoundException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiAdminEmailDomainMappingsLookupDomainGetResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiAdminEmailDomainMappingsLookupByDomain(string $domain, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiAdminEmailDomainMappingsLookupByDomain($domain), $fetch);
    }
    /**
     * @param array{
     *    "clientId"?: string,
     *    "applicationId"?: string,
     *    "active"?: string,
     * } $queryParameters
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiAdminServiceAccountsGetResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiAdminServiceAccount(array $queryParameters = [], string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiAdminServiceAccount($queryParameters), $fetch);
    }
    /**
     * @param null|\FlowCatalyst\Generated\Model\ApiAdminServiceAccountsPostBody $requestBody
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\PostApiAdminServiceAccountBadRequestException
     * @throws \FlowCatalyst\Generated\Exception\PostApiAdminServiceAccountConflictException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiAdminServiceAccountsPostResponse201 : \Psr\Http\Message\ResponseInterface)
     */
    public function postApiAdminServiceAccount(?\FlowCatalyst\Generated\Model\ApiAdminServiceAccountsPostBody $requestBody = null, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\PostApiAdminServiceAccount($requestBody), $fetch);
    }
    /**
     * @param string $id
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\DeleteApiAdminServiceAccountByIdNotFoundException
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
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiAdminServiceAccountsIdGetResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiAdminServiceAccountById(string $id, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiAdminServiceAccountById($id), $fetch);
    }
    /**
     * @param string $id
     * @param null|\FlowCatalyst\Generated\Model\ApiAdminServiceAccountsIdPutBody $requestBody
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\PutApiAdminServiceAccountByIdBadRequestException
     * @throws \FlowCatalyst\Generated\Exception\PutApiAdminServiceAccountByIdNotFoundException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiAdminServiceAccountsIdPutResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function putApiAdminServiceAccountById(string $id, ?\FlowCatalyst\Generated\Model\ApiAdminServiceAccountsIdPutBody $requestBody = null, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\PutApiAdminServiceAccountById($id, $requestBody), $fetch);
    }
    /**
     * @param string $code
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\GetApiAdminServiceAccountsCodeByCodeNotFoundException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiAdminServiceAccountsCodeCodeGetResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiAdminServiceAccountsCodeByCode(string $code, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiAdminServiceAccountsCodeByCode($code), $fetch);
    }
    /**
     * @param string $id
     * @param null|\FlowCatalyst\Generated\Model\ApiAdminServiceAccountsIdAuthTokenPutBody $requestBody
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\PutApiAdminServiceAccountsByIdAuthTokenBadRequestException
     * @throws \FlowCatalyst\Generated\Exception\PutApiAdminServiceAccountsByIdAuthTokenNotFoundException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiAdminServiceAccountsIdAuthTokenPutResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function putApiAdminServiceAccountsByIdAuthToken(string $id, ?\FlowCatalyst\Generated\Model\ApiAdminServiceAccountsIdAuthTokenPutBody $requestBody = null, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\PutApiAdminServiceAccountsByIdAuthToken($id, $requestBody), $fetch);
    }
    /**
     * @param string $id
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\PostApiAdminServiceAccountsByIdRegenerateTokenNotFoundException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiAdminServiceAccountsIdRegenerateTokenPostResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function postApiAdminServiceAccountsByIdRegenerateToken(string $id, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\PostApiAdminServiceAccountsByIdRegenerateToken($id), $fetch);
    }
    /**
     * @param string $id
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\PostApiAdminServiceAccountsByIdRegenerateAuthTokenNotFoundException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiAdminServiceAccountsIdRegenerateAuthTokenPostResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function postApiAdminServiceAccountsByIdRegenerateAuthToken(string $id, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\PostApiAdminServiceAccountsByIdRegenerateAuthToken($id), $fetch);
    }
    /**
     * @param string $id
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\PostApiAdminServiceAccountsByIdRegenerateSecretNotFoundException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiAdminServiceAccountsIdRegenerateSecretPostResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function postApiAdminServiceAccountsByIdRegenerateSecret(string $id, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\PostApiAdminServiceAccountsByIdRegenerateSecret($id), $fetch);
    }
    /**
     * @param string $id
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\PostApiAdminServiceAccountsByIdRegenerateSigningSecretNotFoundException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiAdminServiceAccountsIdRegenerateSigningSecretPostResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function postApiAdminServiceAccountsByIdRegenerateSigningSecret(string $id, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\PostApiAdminServiceAccountsByIdRegenerateSigningSecret($id), $fetch);
    }
    /**
     * @param string $id
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\GetApiAdminServiceAccountsByIdRoleNotFoundException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiAdminServiceAccountsIdRolesGetResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiAdminServiceAccountsByIdRole(string $id, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiAdminServiceAccountsByIdRole($id), $fetch);
    }
    /**
     * @param string $id
     * @param null|\FlowCatalyst\Generated\Model\ApiAdminServiceAccountsIdRolesPutBody $requestBody
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\PutApiAdminServiceAccountsByIdRoleBadRequestException
     * @throws \FlowCatalyst\Generated\Exception\PutApiAdminServiceAccountsByIdRoleNotFoundException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiAdminServiceAccountsIdRolesPutResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function putApiAdminServiceAccountsByIdRole(string $id, ?\FlowCatalyst\Generated\Model\ApiAdminServiceAccountsIdRolesPutBody $requestBody = null, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\PutApiAdminServiceAccountsByIdRole($id, $requestBody), $fetch);
    }
    /**
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiAdminPlatformCorsGetResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiAdminPlatformCor(string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiAdminPlatformCor(), $fetch);
    }
    /**
     * @param null|\FlowCatalyst\Generated\Model\ApiAdminPlatformCorsPostBody $requestBody
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\PostApiAdminPlatformCorBadRequestException
     * @throws \FlowCatalyst\Generated\Exception\PostApiAdminPlatformCorConflictException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiAdminPlatformCorsPostResponse201 : \Psr\Http\Message\ResponseInterface)
     */
    public function postApiAdminPlatformCor(?\FlowCatalyst\Generated\Model\ApiAdminPlatformCorsPostBody $requestBody = null, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\PostApiAdminPlatformCor($requestBody), $fetch);
    }
    /**
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiAdminPlatformCorsAllowedGetResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiAdminPlatformCorsAllowed(string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiAdminPlatformCorsAllowed(), $fetch);
    }
    /**
     * @param string $id
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\DeleteApiAdminPlatformCorByIdNotFoundException
     *
     * @return ($fetch is 'object' ? null : \Psr\Http\Message\ResponseInterface)
     */
    public function deleteApiAdminPlatformCorById(string $id, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\DeleteApiAdminPlatformCorById($id), $fetch);
    }
    /**
     * @param string $id
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\GetApiAdminPlatformCorByIdNotFoundException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiAdminPlatformCorsIdGetResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiAdminPlatformCorById(string $id, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiAdminPlatformCorById($id), $fetch);
    }
    /**
     * @param string $appCode
     * @param array{
     *    "scope"?: string,
     *    "clientId"?: string,
     * } $queryParameters
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\GetApiAdminConfigByAppCodeForbiddenException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiAdminConfigAppCodeGetResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiAdminConfigByAppCode(string $appCode, array $queryParameters = [], string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiAdminConfigByAppCode($appCode, $queryParameters), $fetch);
    }
    /**
     * @param string $appCode
     * @param string $section
     * @param array{
     *    "scope"?: string,
     *    "clientId"?: string,
     * } $queryParameters
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\GetApiAdminConfigByAppCodeBySectionForbiddenException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiAdminConfigAppCodeSectionGetResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiAdminConfigByAppCodeBySection(string $appCode, string $section, array $queryParameters = [], string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiAdminConfigByAppCodeBySection($appCode, $section, $queryParameters), $fetch);
    }
    /**
     * @param string $appCode
     * @param string $section
     * @param string $property
     * @param array{
     *    "scope"?: string,
     *    "clientId"?: string,
     * } $queryParameters
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\DeleteApiAdminConfigByAppCodeBySectionByPropertyForbiddenException
     * @throws \FlowCatalyst\Generated\Exception\DeleteApiAdminConfigByAppCodeBySectionByPropertyNotFoundException
     *
     * @return ($fetch is 'object' ? null : \Psr\Http\Message\ResponseInterface)
     */
    public function deleteApiAdminConfigByAppCodeBySectionByProperty(string $appCode, string $section, string $property, array $queryParameters = [], string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\DeleteApiAdminConfigByAppCodeBySectionByProperty($appCode, $section, $property, $queryParameters), $fetch);
    }
    /**
     * @param string $appCode
     * @param string $section
     * @param string $property
     * @param array{
     *    "scope"?: string,
     *    "clientId"?: string,
     * } $queryParameters
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\GetApiAdminConfigByAppCodeBySectionByPropertyForbiddenException
     * @throws \FlowCatalyst\Generated\Exception\GetApiAdminConfigByAppCodeBySectionByPropertyNotFoundException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiAdminConfigAppCodeSectionPropertyGetResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiAdminConfigByAppCodeBySectionByProperty(string $appCode, string $section, string $property, array $queryParameters = [], string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiAdminConfigByAppCodeBySectionByProperty($appCode, $section, $property, $queryParameters), $fetch);
    }
    /**
     * @param string $appCode
     * @param string $section
     * @param string $property
     * @param null|\FlowCatalyst\Generated\Model\ApiAdminConfigAppCodeSectionPropertyPutBody $requestBody
     * @param array{
     *    "scope"?: string,
     *    "clientId"?: string,
     * } $queryParameters
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\PutApiAdminConfigByAppCodeBySectionByPropertyBadRequestException
     * @throws \FlowCatalyst\Generated\Exception\PutApiAdminConfigByAppCodeBySectionByPropertyForbiddenException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiAdminConfigAppCodeSectionPropertyPutResponse200|\FlowCatalyst\Generated\Model\ApiAdminConfigAppCodeSectionPropertyPutResponse201 : \Psr\Http\Message\ResponseInterface)
     */
    public function putApiAdminConfigByAppCodeBySectionByProperty(string $appCode, string $section, string $property, ?\FlowCatalyst\Generated\Model\ApiAdminConfigAppCodeSectionPropertyPutBody $requestBody = null, array $queryParameters = [], string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\PutApiAdminConfigByAppCodeBySectionByProperty($appCode, $section, $property, $requestBody, $queryParameters), $fetch);
    }
    /**
     * @param string $appCode
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiAdminConfigAccessAppCodeGetResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiAdminConfigAccessByAppCode(string $appCode, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiAdminConfigAccessByAppCode($appCode), $fetch);
    }
    /**
     * @param string $appCode
     * @param null|\FlowCatalyst\Generated\Model\ApiAdminConfigAccessAppCodePostBody $requestBody
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\PostApiAdminConfigAccessByAppCodeConflictException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiAdminConfigAccessAppCodePostResponse201 : \Psr\Http\Message\ResponseInterface)
     */
    public function postApiAdminConfigAccessByAppCode(string $appCode, ?\FlowCatalyst\Generated\Model\ApiAdminConfigAccessAppCodePostBody $requestBody = null, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\PostApiAdminConfigAccessByAppCode($appCode, $requestBody), $fetch);
    }
    /**
     * @param string $appCode
     * @param string $roleCode
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
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
     * @param null|\FlowCatalyst\Generated\Model\ApiAdminConfigAccessAppCodeRoleCodePutBody $requestBody
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\PutApiAdminConfigAccessByAppCodeByRoleCodeNotFoundException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiAdminConfigAccessAppCodeRoleCodePutResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function putApiAdminConfigAccessByAppCodeByRoleCode(string $appCode, string $roleCode, ?\FlowCatalyst\Generated\Model\ApiAdminConfigAccessAppCodeRoleCodePutBody $requestBody = null, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\PutApiAdminConfigAccessByAppCodeByRoleCode($appCode, $roleCode, $requestBody), $fetch);
    }
    /**
     * @param array{
     *    "status"?: string,
     *    "application"?: array,
     *    "subdomain"?: array,
     *    "aggregate"?: array,
     * } $queryParameters
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\BffEventTypesGetResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function getBffEventType(array $queryParameters = [], string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetBffEventType($queryParameters), $fetch);
    }
    /**
     * @param null|\FlowCatalyst\Generated\Model\BffEventTypesPostBody $requestBody
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\PostBffEventTypeBadRequestException
     * @throws \FlowCatalyst\Generated\Exception\PostBffEventTypeConflictException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\BffEventTypesPostResponse201 : \Psr\Http\Message\ResponseInterface)
     */
    public function postBffEventType(?\FlowCatalyst\Generated\Model\BffEventTypesPostBody $requestBody = null, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\PostBffEventType($requestBody), $fetch);
    }
    /**
     * @param string $id
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\DeleteBffEventTypeByIdNotFoundException
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
     * @throws \FlowCatalyst\Generated\Exception\GetBffEventTypeByIdNotFoundException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\BffEventTypesIdGetResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function getBffEventTypeById(string $id, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetBffEventTypeById($id), $fetch);
    }
    /**
     * @param string $id
     * @param null|\FlowCatalyst\Generated\Model\BffEventTypesIdPatchBody $requestBody
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\PatchBffEventTypeByIdBadRequestException
     * @throws \FlowCatalyst\Generated\Exception\PatchBffEventTypeByIdNotFoundException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\BffEventTypesIdPatchResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function patchBffEventTypeById(string $id, ?\FlowCatalyst\Generated\Model\BffEventTypesIdPatchBody $requestBody = null, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\PatchBffEventTypeById($id, $requestBody), $fetch);
    }
    /**
     * @param string $id
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\PostBffEventTypesByIdArchiveNotFoundException
     * @throws \FlowCatalyst\Generated\Exception\PostBffEventTypesByIdArchiveConflictException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\BffEventTypesIdArchivePostResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function postBffEventTypesByIdArchive(string $id, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\PostBffEventTypesByIdArchive($id), $fetch);
    }
    /**
     * @param string $id
     * @param null|\FlowCatalyst\Generated\Model\BffEventTypesIdSchemasPostBody $requestBody
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\PostBffEventTypesByIdSchemaBadRequestException
     * @throws \FlowCatalyst\Generated\Exception\PostBffEventTypesByIdSchemaNotFoundException
     * @throws \FlowCatalyst\Generated\Exception\PostBffEventTypesByIdSchemaConflictException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\BffEventTypesIdSchemasPostResponse201 : \Psr\Http\Message\ResponseInterface)
     */
    public function postBffEventTypesByIdSchema(string $id, ?\FlowCatalyst\Generated\Model\BffEventTypesIdSchemasPostBody $requestBody = null, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\PostBffEventTypesByIdSchema($id, $requestBody), $fetch);
    }
    /**
     * @param string $id
     * @param string $version
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\PostBffEventTypesByIdSchemasByVersionFinaliseNotFoundException
     * @throws \FlowCatalyst\Generated\Exception\PostBffEventTypesByIdSchemasByVersionFinaliseConflictException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\BffEventTypesIdSchemasVersionFinalisePostResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function postBffEventTypesByIdSchemasByVersionFinalise(string $id, string $version, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\PostBffEventTypesByIdSchemasByVersionFinalise($id, $version), $fetch);
    }
    /**
     * @param string $id
     * @param string $version
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\PostBffEventTypesByIdSchemasByVersionDeprecateNotFoundException
     * @throws \FlowCatalyst\Generated\Exception\PostBffEventTypesByIdSchemasByVersionDeprecateConflictException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\BffEventTypesIdSchemasVersionDeprecatePostResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function postBffEventTypesByIdSchemasByVersionDeprecate(string $id, string $version, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\PostBffEventTypesByIdSchemasByVersionDeprecate($id, $version), $fetch);
    }
    /**
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\BffEventTypesFiltersApplicationsGetResponse200 : \Psr\Http\Message\ResponseInterface)
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
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\BffEventTypesFiltersSubdomainsGetResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function getBffEventTypesFiltersSubdomain(array $queryParameters = [], string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetBffEventTypesFiltersSubdomain($queryParameters), $fetch);
    }
    /**
     * @param array{
     *    "application"?: array,
     *    "subdomain"?: array,
     * } $queryParameters
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\BffEventTypesFiltersAggregatesGetResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function getBffEventTypesFiltersAggregate(array $queryParameters = [], string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetBffEventTypesFiltersAggregate($queryParameters), $fetch);
    }
    /**
     * @param array{
     *    "application"?: string,
     *    "source"?: string,
     * } $queryParameters
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\BffRolesGetResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function getBffRole(array $queryParameters = [], string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetBffRole($queryParameters), $fetch);
    }
    /**
     * @param null|\FlowCatalyst\Generated\Model\BffRolesPostBody $requestBody
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\PostBffRoleBadRequestException
     * @throws \FlowCatalyst\Generated\Exception\PostBffRoleConflictException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\BffRolesPostResponse201 : \Psr\Http\Message\ResponseInterface)
     */
    public function postBffRole(?\FlowCatalyst\Generated\Model\BffRolesPostBody $requestBody = null, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\PostBffRole($requestBody), $fetch);
    }
    /**
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\BffRolesFiltersApplicationsGetResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function getBffRolesFiltersApplication(string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetBffRolesFiltersApplication(), $fetch);
    }
    /**
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\BffRolesPermissionsGetResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function getBffRolesPermission(string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetBffRolesPermission(), $fetch);
    }
    /**
     * @param string $permission
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\GetBffRolesPermissionByPermissionNotFoundException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\BffRolesPermissionsPermissionGetResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function getBffRolesPermissionByPermission(string $permission, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetBffRolesPermissionByPermission($permission), $fetch);
    }
    /**
     * @param string $roleName
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\DeleteBffRoleByRoleNameNotFoundException
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
     * @throws \FlowCatalyst\Generated\Exception\GetBffRoleByRoleNameNotFoundException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\BffRolesRoleNameGetResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function getBffRoleByRoleName(string $roleName, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetBffRoleByRoleName($roleName), $fetch);
    }
    /**
     * @param string $roleName
     * @param null|\FlowCatalyst\Generated\Model\BffRolesRoleNamePutBody $requestBody
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\PutBffRoleByRoleNameBadRequestException
     * @throws \FlowCatalyst\Generated\Exception\PutBffRoleByRoleNameNotFoundException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\BffRolesRoleNamePutResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function putBffRoleByRoleName(string $roleName, ?\FlowCatalyst\Generated\Model\BffRolesRoleNamePutBody $requestBody = null, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\PutBffRoleByRoleName($roleName, $requestBody), $fetch);
    }
    /**
     * @param array{
     *    "clientIds"?: string,
     *    "applications"?: string,
     *    "subdomains"?: string,
     *    "aggregates"?: string,
     *    "types"?: string,
     *    "source"?: string,
     *    "subject"?: string,
     *    "correlationId"?: string,
     *    "messageGroup"?: string,
     *    "timeAfter"?: string,
     *    "timeBefore"?: string,
     *    "page"?: string,
     *    "size"?: string,
     * } $queryParameters
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\BffEventsGetResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function getBffEvent(array $queryParameters = [], string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetBffEvent($queryParameters), $fetch);
    }
    /**
     * @param array{
     *    "clientIds"?: string,
     *    "applications"?: string,
     *    "subdomains"?: string,
     *    "aggregates"?: string,
     * } $queryParameters
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\BffEventsFilterOptionsGetResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function getBffEventsFilterOption(array $queryParameters = [], string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetBffEventsFilterOption($queryParameters), $fetch);
    }
    /**
     * @param string $id
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\GetBffEventByIdNotFoundException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\BffEventsIdGetResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function getBffEventById(string $id, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetBffEventById($id), $fetch);
    }
    /**
     * @param array{
     *    "clientIds"?: string,
     *    "statuses"?: string,
     *    "applications"?: string,
     *    "subdomains"?: string,
     *    "aggregates"?: string,
     *    "codes"?: string,
     *    "source"?: string,
     *    "kind"?: string,
     *    "subscriptionId"?: string,
     *    "dispatchPoolId"?: string,
     *    "messageGroup"?: string,
     *    "createdAfter"?: string,
     *    "createdBefore"?: string,
     *    "page"?: string,
     *    "size"?: string,
     * } $queryParameters
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\BffDispatchJobsGetResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function getBffDispatchJob(array $queryParameters = [], string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetBffDispatchJob($queryParameters), $fetch);
    }
    /**
     * @param array{
     *    "clientIds"?: string,
     *    "applications"?: string,
     *    "subdomains"?: string,
     *    "aggregates"?: string,
     * } $queryParameters
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\BffDispatchJobsFilterOptionsGetResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function getBffDispatchJobsFilterOption(array $queryParameters = [], string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetBffDispatchJobsFilterOption($queryParameters), $fetch);
    }
    /**
     * @param string $id
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\GetBffDispatchJobByIdNotFoundException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\BffDispatchJobsIdGetResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function getBffDispatchJobById(string $id, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetBffDispatchJobById($id), $fetch);
    }
    /**
     * @param string $id
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\GetBffDispatchJobsByIdAttemptNotFoundException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\BffDispatchJobsIdAttemptsGetResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function getBffDispatchJobsByIdAttempt(string $id, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetBffDispatchJobsByIdAttempt($id), $fetch);
    }
    /**
     * @param array{
     *    "status"?: string,
     *    "application"?: array,
     *    "subdomain"?: array,
     *    "aggregate"?: array,
     * } $queryParameters
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiBffEventTypesGetResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiBffEventType(array $queryParameters = [], string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiBffEventType($queryParameters), $fetch);
    }
    /**
     * @param null|\FlowCatalyst\Generated\Model\ApiBffEventTypesPostBody $requestBody
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\PostApiBffEventTypeBadRequestException
     * @throws \FlowCatalyst\Generated\Exception\PostApiBffEventTypeConflictException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiBffEventTypesPostResponse201 : \Psr\Http\Message\ResponseInterface)
     */
    public function postApiBffEventType(?\FlowCatalyst\Generated\Model\ApiBffEventTypesPostBody $requestBody = null, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\PostApiBffEventType($requestBody), $fetch);
    }
    /**
     * @param string $id
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\DeleteApiBffEventTypeByIdNotFoundException
     *
     * @return ($fetch is 'object' ? null : \Psr\Http\Message\ResponseInterface)
     */
    public function deleteApiBffEventTypeById(string $id, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\DeleteApiBffEventTypeById($id), $fetch);
    }
    /**
     * @param string $id
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\GetApiBffEventTypeByIdNotFoundException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiBffEventTypesIdGetResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiBffEventTypeById(string $id, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiBffEventTypeById($id), $fetch);
    }
    /**
     * @param string $id
     * @param null|\FlowCatalyst\Generated\Model\ApiBffEventTypesIdPatchBody $requestBody
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\PatchApiBffEventTypeByIdBadRequestException
     * @throws \FlowCatalyst\Generated\Exception\PatchApiBffEventTypeByIdNotFoundException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiBffEventTypesIdPatchResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function patchApiBffEventTypeById(string $id, ?\FlowCatalyst\Generated\Model\ApiBffEventTypesIdPatchBody $requestBody = null, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\PatchApiBffEventTypeById($id, $requestBody), $fetch);
    }
    /**
     * @param string $id
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\PostApiBffEventTypesByIdArchiveNotFoundException
     * @throws \FlowCatalyst\Generated\Exception\PostApiBffEventTypesByIdArchiveConflictException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiBffEventTypesIdArchivePostResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function postApiBffEventTypesByIdArchive(string $id, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\PostApiBffEventTypesByIdArchive($id), $fetch);
    }
    /**
     * @param string $id
     * @param null|\FlowCatalyst\Generated\Model\ApiBffEventTypesIdSchemasPostBody $requestBody
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\PostApiBffEventTypesByIdSchemaBadRequestException
     * @throws \FlowCatalyst\Generated\Exception\PostApiBffEventTypesByIdSchemaNotFoundException
     * @throws \FlowCatalyst\Generated\Exception\PostApiBffEventTypesByIdSchemaConflictException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiBffEventTypesIdSchemasPostResponse201 : \Psr\Http\Message\ResponseInterface)
     */
    public function postApiBffEventTypesByIdSchema(string $id, ?\FlowCatalyst\Generated\Model\ApiBffEventTypesIdSchemasPostBody $requestBody = null, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\PostApiBffEventTypesByIdSchema($id, $requestBody), $fetch);
    }
    /**
     * @param string $id
     * @param string $version
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\PostApiBffEventTypesByIdSchemasByVersionFinaliseNotFoundException
     * @throws \FlowCatalyst\Generated\Exception\PostApiBffEventTypesByIdSchemasByVersionFinaliseConflictException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiBffEventTypesIdSchemasVersionFinalisePostResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function postApiBffEventTypesByIdSchemasByVersionFinalise(string $id, string $version, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\PostApiBffEventTypesByIdSchemasByVersionFinalise($id, $version), $fetch);
    }
    /**
     * @param string $id
     * @param string $version
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\PostApiBffEventTypesByIdSchemasByVersionDeprecateNotFoundException
     * @throws \FlowCatalyst\Generated\Exception\PostApiBffEventTypesByIdSchemasByVersionDeprecateConflictException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiBffEventTypesIdSchemasVersionDeprecatePostResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function postApiBffEventTypesByIdSchemasByVersionDeprecate(string $id, string $version, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\PostApiBffEventTypesByIdSchemasByVersionDeprecate($id, $version), $fetch);
    }
    /**
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiBffEventTypesFiltersApplicationsGetResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiBffEventTypesFiltersApplication(string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiBffEventTypesFiltersApplication(), $fetch);
    }
    /**
     * @param array{
     *    "application"?: array,
     * } $queryParameters
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiBffEventTypesFiltersSubdomainsGetResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiBffEventTypesFiltersSubdomain(array $queryParameters = [], string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiBffEventTypesFiltersSubdomain($queryParameters), $fetch);
    }
    /**
     * @param array{
     *    "application"?: array,
     *    "subdomain"?: array,
     * } $queryParameters
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiBffEventTypesFiltersAggregatesGetResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiBffEventTypesFiltersAggregate(array $queryParameters = [], string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiBffEventTypesFiltersAggregate($queryParameters), $fetch);
    }
    /**
     * @param array{
     *    "application"?: string,
     *    "source"?: string,
     * } $queryParameters
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiBffRolesGetResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiBffRole(array $queryParameters = [], string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiBffRole($queryParameters), $fetch);
    }
    /**
     * @param null|\FlowCatalyst\Generated\Model\ApiBffRolesPostBody $requestBody
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\PostApiBffRoleBadRequestException
     * @throws \FlowCatalyst\Generated\Exception\PostApiBffRoleConflictException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiBffRolesPostResponse201 : \Psr\Http\Message\ResponseInterface)
     */
    public function postApiBffRole(?\FlowCatalyst\Generated\Model\ApiBffRolesPostBody $requestBody = null, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\PostApiBffRole($requestBody), $fetch);
    }
    /**
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiBffRolesFiltersApplicationsGetResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiBffRolesFiltersApplication(string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiBffRolesFiltersApplication(), $fetch);
    }
    /**
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiBffRolesPermissionsGetResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiBffRolesPermission(string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiBffRolesPermission(), $fetch);
    }
    /**
     * @param string $permission
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\GetApiBffRolesPermissionByPermissionNotFoundException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiBffRolesPermissionsPermissionGetResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiBffRolesPermissionByPermission(string $permission, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiBffRolesPermissionByPermission($permission), $fetch);
    }
    /**
     * @param string $roleName
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\DeleteApiBffRoleByRoleNameNotFoundException
     *
     * @return ($fetch is 'object' ? null : \Psr\Http\Message\ResponseInterface)
     */
    public function deleteApiBffRoleByRoleName(string $roleName, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\DeleteApiBffRoleByRoleName($roleName), $fetch);
    }
    /**
     * @param string $roleName
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\GetApiBffRoleByRoleNameNotFoundException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiBffRolesRoleNameGetResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiBffRoleByRoleName(string $roleName, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiBffRoleByRoleName($roleName), $fetch);
    }
    /**
     * @param string $roleName
     * @param null|\FlowCatalyst\Generated\Model\ApiBffRolesRoleNamePutBody $requestBody
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\PutApiBffRoleByRoleNameBadRequestException
     * @throws \FlowCatalyst\Generated\Exception\PutApiBffRoleByRoleNameNotFoundException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiBffRolesRoleNamePutResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function putApiBffRoleByRoleName(string $roleName, ?\FlowCatalyst\Generated\Model\ApiBffRolesRoleNamePutBody $requestBody = null, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\PutApiBffRoleByRoleName($roleName, $requestBody), $fetch);
    }
    /**
     * @param array{
     *    "clientIds"?: string,
     *    "applications"?: string,
     *    "subdomains"?: string,
     *    "aggregates"?: string,
     *    "types"?: string,
     *    "source"?: string,
     *    "subject"?: string,
     *    "correlationId"?: string,
     *    "messageGroup"?: string,
     *    "timeAfter"?: string,
     *    "timeBefore"?: string,
     *    "page"?: string,
     *    "size"?: string,
     * } $queryParameters
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiBffEventsGetResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiBffEvent(array $queryParameters = [], string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiBffEvent($queryParameters), $fetch);
    }
    /**
     * @param array{
     *    "clientIds"?: string,
     *    "applications"?: string,
     *    "subdomains"?: string,
     *    "aggregates"?: string,
     * } $queryParameters
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiBffEventsFilterOptionsGetResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiBffEventsFilterOption(array $queryParameters = [], string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiBffEventsFilterOption($queryParameters), $fetch);
    }
    /**
     * @param string $id
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\GetApiBffEventByIdNotFoundException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiBffEventsIdGetResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiBffEventById(string $id, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiBffEventById($id), $fetch);
    }
    /**
     * @param array{
     *    "clientIds"?: string,
     *    "statuses"?: string,
     *    "applications"?: string,
     *    "subdomains"?: string,
     *    "aggregates"?: string,
     *    "codes"?: string,
     *    "source"?: string,
     *    "kind"?: string,
     *    "subscriptionId"?: string,
     *    "dispatchPoolId"?: string,
     *    "messageGroup"?: string,
     *    "createdAfter"?: string,
     *    "createdBefore"?: string,
     *    "page"?: string,
     *    "size"?: string,
     * } $queryParameters
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiBffDispatchJobsGetResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiBffDispatchJob(array $queryParameters = [], string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiBffDispatchJob($queryParameters), $fetch);
    }
    /**
     * @param array{
     *    "clientIds"?: string,
     *    "applications"?: string,
     *    "subdomains"?: string,
     *    "aggregates"?: string,
     * } $queryParameters
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiBffDispatchJobsFilterOptionsGetResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiBffDispatchJobsFilterOption(array $queryParameters = [], string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiBffDispatchJobsFilterOption($queryParameters), $fetch);
    }
    /**
     * @param string $id
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\GetApiBffDispatchJobByIdNotFoundException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiBffDispatchJobsIdGetResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiBffDispatchJobById(string $id, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiBffDispatchJobById($id), $fetch);
    }
    /**
     * @param string $id
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\GetApiBffDispatchJobsByIdAttemptNotFoundException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiBffDispatchJobsIdAttemptsGetResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiBffDispatchJobsByIdAttempt(string $id, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiBffDispatchJobsByIdAttempt($id), $fetch);
    }
    /**
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiSdkClientsGetResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiSdkClient(string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiSdkClient(), $fetch);
    }
    /**
     * @param null|\FlowCatalyst\Generated\Model\ApiSdkClientsPostBody $requestBody
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\PostApiSdkClientBadRequestException
     * @throws \FlowCatalyst\Generated\Exception\PostApiSdkClientConflictException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiSdkClientsPostResponse201 : \Psr\Http\Message\ResponseInterface)
     */
    public function postApiSdkClient(?\FlowCatalyst\Generated\Model\ApiSdkClientsPostBody $requestBody = null, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\PostApiSdkClient($requestBody), $fetch);
    }
    /**
     * @param string $id
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\GetApiSdkClientByIdNotFoundException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiSdkClientsIdGetResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiSdkClientById(string $id, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiSdkClientById($id), $fetch);
    }
    /**
     * @param string $id
     * @param null|\FlowCatalyst\Generated\Model\ApiSdkClientsIdPutBody $requestBody
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\PutApiSdkClientByIdBadRequestException
     * @throws \FlowCatalyst\Generated\Exception\PutApiSdkClientByIdNotFoundException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiSdkClientsIdPutResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function putApiSdkClientById(string $id, ?\FlowCatalyst\Generated\Model\ApiSdkClientsIdPutBody $requestBody = null, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\PutApiSdkClientById($id, $requestBody), $fetch);
    }
    /**
     * @param string $id
     * @param null|\FlowCatalyst\Generated\Model\ApiSdkClientsIdActivatePostBody $requestBody
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\PostApiSdkClientsByIdActivateNotFoundException
     * @throws \FlowCatalyst\Generated\Exception\PostApiSdkClientsByIdActivateConflictException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiSdkClientsIdActivatePostResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function postApiSdkClientsByIdActivate(string $id, ?\FlowCatalyst\Generated\Model\ApiSdkClientsIdActivatePostBody $requestBody = null, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\PostApiSdkClientsByIdActivate($id, $requestBody), $fetch);
    }
    /**
     * @param string $id
     * @param null|\FlowCatalyst\Generated\Model\ApiSdkClientsIdSuspendPostBody $requestBody
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\PostApiSdkClientsByIdSuspendNotFoundException
     * @throws \FlowCatalyst\Generated\Exception\PostApiSdkClientsByIdSuspendConflictException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiSdkClientsIdSuspendPostResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function postApiSdkClientsByIdSuspend(string $id, ?\FlowCatalyst\Generated\Model\ApiSdkClientsIdSuspendPostBody $requestBody = null, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\PostApiSdkClientsByIdSuspend($id, $requestBody), $fetch);
    }
    /**
     * @param string $id
     * @param null|\FlowCatalyst\Generated\Model\ApiSdkClientsIdDeactivatePostBody $requestBody
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\PostApiSdkClientsByIdDeactivateNotFoundException
     * @throws \FlowCatalyst\Generated\Exception\PostApiSdkClientsByIdDeactivateConflictException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiSdkClientsIdDeactivatePostResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function postApiSdkClientsByIdDeactivate(string $id, ?\FlowCatalyst\Generated\Model\ApiSdkClientsIdDeactivatePostBody $requestBody = null, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\PostApiSdkClientsByIdDeactivate($id, $requestBody), $fetch);
    }
    /**
     * @param array{
     *    "application"?: string,
     *    "source"?: string,
     * } $queryParameters
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiSdkRolesGetResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiSdkRole(array $queryParameters = [], string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiSdkRole($queryParameters), $fetch);
    }
    /**
     * @param null|\FlowCatalyst\Generated\Model\ApiSdkRolesPostBody $requestBody
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\PostApiSdkRoleBadRequestException
     * @throws \FlowCatalyst\Generated\Exception\PostApiSdkRoleConflictException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiSdkRolesPostResponse201 : \Psr\Http\Message\ResponseInterface)
     */
    public function postApiSdkRole(?\FlowCatalyst\Generated\Model\ApiSdkRolesPostBody $requestBody = null, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\PostApiSdkRole($requestBody), $fetch);
    }
    /**
     * @param string $roleName
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\DeleteApiSdkRoleByRoleNameNotFoundException
     *
     * @return ($fetch is 'object' ? null : \Psr\Http\Message\ResponseInterface)
     */
    public function deleteApiSdkRoleByRoleName(string $roleName, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\DeleteApiSdkRoleByRoleName($roleName), $fetch);
    }
    /**
     * @param string $roleName
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\GetApiSdkRoleByRoleNameNotFoundException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiSdkRolesRoleNameGetResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiSdkRoleByRoleName(string $roleName, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiSdkRoleByRoleName($roleName), $fetch);
    }
    /**
     * @param string $roleName
     * @param null|\FlowCatalyst\Generated\Model\ApiSdkRolesRoleNamePutBody $requestBody
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\PutApiSdkRoleByRoleNameBadRequestException
     * @throws \FlowCatalyst\Generated\Exception\PutApiSdkRoleByRoleNameNotFoundException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiSdkRolesRoleNamePutResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function putApiSdkRoleByRoleName(string $roleName, ?\FlowCatalyst\Generated\Model\ApiSdkRolesRoleNamePutBody $requestBody = null, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\PutApiSdkRoleByRoleName($roleName, $requestBody), $fetch);
    }
    /**
     * @param array{
     *    "clientId"?: string,
     *    "type"?: string,
     *    "active"?: string,
     *    "email"?: string,
     * } $queryParameters
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiSdkPrincipalsGetResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiSdkPrincipal(array $queryParameters = [], string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiSdkPrincipal($queryParameters), $fetch);
    }
    /**
     * @param string $id
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\GetApiSdkPrincipalByIdNotFoundException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiSdkPrincipalsIdGetResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiSdkPrincipalById(string $id, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiSdkPrincipalById($id), $fetch);
    }
    /**
     * @param string $id
     * @param null|\FlowCatalyst\Generated\Model\ApiSdkPrincipalsIdPutBody $requestBody
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\PutApiSdkPrincipalByIdBadRequestException
     * @throws \FlowCatalyst\Generated\Exception\PutApiSdkPrincipalByIdNotFoundException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiSdkPrincipalsIdPutResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function putApiSdkPrincipalById(string $id, ?\FlowCatalyst\Generated\Model\ApiSdkPrincipalsIdPutBody $requestBody = null, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\PutApiSdkPrincipalById($id, $requestBody), $fetch);
    }
    /**
     * @param null|\FlowCatalyst\Generated\Model\ApiSdkPrincipalsUserPostBody $requestBody
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\PostApiSdkPrincipalsUserBadRequestException
     * @throws \FlowCatalyst\Generated\Exception\PostApiSdkPrincipalsUserConflictException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiSdkPrincipalsUserPostResponse201 : \Psr\Http\Message\ResponseInterface)
     */
    public function postApiSdkPrincipalsUser(?\FlowCatalyst\Generated\Model\ApiSdkPrincipalsUserPostBody $requestBody = null, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\PostApiSdkPrincipalsUser($requestBody), $fetch);
    }
    /**
     * @param string $id
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\PostApiSdkPrincipalsByIdActivateNotFoundException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiSdkPrincipalsIdActivatePostResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function postApiSdkPrincipalsByIdActivate(string $id, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\PostApiSdkPrincipalsByIdActivate($id), $fetch);
    }
    /**
     * @param string $id
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\PostApiSdkPrincipalsByIdDeactivateNotFoundException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiSdkPrincipalsIdDeactivatePostResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function postApiSdkPrincipalsByIdDeactivate(string $id, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\PostApiSdkPrincipalsByIdDeactivate($id), $fetch);
    }
    /**
     * @param string $id
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\GetApiSdkPrincipalsByIdRoleNotFoundException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiSdkPrincipalsIdRolesGetResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiSdkPrincipalsByIdRole(string $id, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiSdkPrincipalsByIdRole($id), $fetch);
    }
    /**
     * @param string $id
     * @param null|\FlowCatalyst\Generated\Model\ApiSdkPrincipalsIdRolesPutBody $requestBody
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\PutApiSdkPrincipalsByIdRoleBadRequestException
     * @throws \FlowCatalyst\Generated\Exception\PutApiSdkPrincipalsByIdRoleNotFoundException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiSdkPrincipalsIdRolesPutResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function putApiSdkPrincipalsByIdRole(string $id, ?\FlowCatalyst\Generated\Model\ApiSdkPrincipalsIdRolesPutBody $requestBody = null, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\PutApiSdkPrincipalsByIdRole($id, $requestBody), $fetch);
    }
    /**
     * @param string $id
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\GetApiSdkPrincipalsByIdClientNotFoundException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiSdkPrincipalsIdClientsGetResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiSdkPrincipalsByIdClient(string $id, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiSdkPrincipalsByIdClient($id), $fetch);
    }
    /**
     * @param string $id
     * @param string $clientId
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\DeleteApiSdkPrincipalsByIdClientByClientIdNotFoundException
     *
     * @return ($fetch is 'object' ? null : \Psr\Http\Message\ResponseInterface)
     */
    public function deleteApiSdkPrincipalsByIdClientByClientId(string $id, string $clientId, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\DeleteApiSdkPrincipalsByIdClientByClientId($id, $clientId), $fetch);
    }
    /**
     * @param string $id
     * @param string $clientId
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\PostApiSdkPrincipalsByIdClientByClientIdNotFoundException
     * @throws \FlowCatalyst\Generated\Exception\PostApiSdkPrincipalsByIdClientByClientIdConflictException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiSdkPrincipalsIdClientsClientIdPostResponse201 : \Psr\Http\Message\ResponseInterface)
     */
    public function postApiSdkPrincipalsByIdClientByClientId(string $id, string $clientId, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\PostApiSdkPrincipalsByIdClientByClientId($id, $clientId), $fetch);
    }
    /**
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\GetApiMeClientUnauthorizedException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiMeClientsGetResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiMeClient(string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiMeClient(), $fetch);
    }
    /**
     * @param string $clientId
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\GetApiMeClientByClientIdUnauthorizedException
     * @throws \FlowCatalyst\Generated\Exception\GetApiMeClientByClientIdForbiddenException
     * @throws \FlowCatalyst\Generated\Exception\GetApiMeClientByClientIdNotFoundException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiMeClientsClientIdGetResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiMeClientByClientId(string $clientId, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiMeClientByClientId($clientId), $fetch);
    }
    /**
     * @param string $clientId
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\GetApiMeClientsByClientIdApplicationUnauthorizedException
     * @throws \FlowCatalyst\Generated\Exception\GetApiMeClientsByClientIdApplicationForbiddenException
     * @throws \FlowCatalyst\Generated\Exception\GetApiMeClientsByClientIdApplicationNotFoundException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiMeClientsClientIdApplicationsGetResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiMeClientsByClientIdApplication(string $clientId, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiMeClientsByClientIdApplication($clientId), $fetch);
    }
    /**
     * @param array{
     *    "clientId"?: string,
     * } $queryParameters
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiPublicLoginThemeGetResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiPublicLoginTheme(array $queryParameters = [], string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiPublicLoginTheme($queryParameters), $fetch);
    }
    /**
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiPublicPlatformGetResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiPublicPlatform(string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiPublicPlatform(), $fetch);
    }
    /**
     * @param array{
     *    "clientId"?: string,
     * } $queryParameters
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiConfigLoginThemeGetResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiConfigLoginTheme(array $queryParameters = [], string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiConfigLoginTheme($queryParameters), $fetch);
    }
    /**
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiConfigPlatformGetResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiConfigPlatform(string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiConfigPlatform(), $fetch);
    }
    /**
     * @param array{
     *    "page"?: int,
     *    "size"?: int,
     * } $queryParameters
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\BffDebugEventsGetResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function getBffDebugEvent(array $queryParameters = [], string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetBffDebugEvent($queryParameters), $fetch);
    }
    /**
     * @param string $id
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\GetBffDebugEventByIdNotFoundException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\BffDebugEventsIdGetResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function getBffDebugEventById(string $id, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetBffDebugEventById($id), $fetch);
    }
    /**
     * @param array{
     *    "page"?: int,
     *    "size"?: int,
     * } $queryParameters
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\BffDebugDispatchJobsGetResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function getBffDebugDispatchJob(array $queryParameters = [], string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetBffDebugDispatchJob($queryParameters), $fetch);
    }
    /**
     * @param string $id
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\GetBffDebugDispatchJobByIdNotFoundException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\BffDebugDispatchJobsIdGetResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function getBffDebugDispatchJobById(string $id, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetBffDebugDispatchJobById($id), $fetch);
    }
    /**
     * @param array{
     *    "page"?: int,
     *    "size"?: int,
     * } $queryParameters
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiBffDebugEventsGetResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiBffDebugEvent(array $queryParameters = [], string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiBffDebugEvent($queryParameters), $fetch);
    }
    /**
     * @param string $id
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\GetApiBffDebugEventByIdNotFoundException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiBffDebugEventsIdGetResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiBffDebugEventById(string $id, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiBffDebugEventById($id), $fetch);
    }
    /**
     * @param array{
     *    "page"?: int,
     *    "size"?: int,
     * } $queryParameters
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiBffDebugDispatchJobsGetResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiBffDebugDispatchJob(array $queryParameters = [], string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiBffDebugDispatchJob($queryParameters), $fetch);
    }
    /**
     * @param string $id
     * @param string $fetch Fetch mode to use (can be OBJECT or RESPONSE)
     * @throws \FlowCatalyst\Generated\Exception\GetApiBffDebugDispatchJobByIdNotFoundException
     *
     * @return ($fetch is 'object' ? null|\FlowCatalyst\Generated\Model\ApiBffDebugDispatchJobsIdGetResponse200 : \Psr\Http\Message\ResponseInterface)
     */
    public function getApiBffDebugDispatchJobById(string $id, string $fetch = self::FETCH_OBJECT)
    {
        return $this->executeEndpoint(new \FlowCatalyst\Generated\Endpoint\GetApiBffDebugDispatchJobById($id), $fetch);
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