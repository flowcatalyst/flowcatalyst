<?php

namespace FlowCatalyst\Generated\Exception;

class GetApiAdminServiceAccountsByIdRoleNotFoundException extends NotFoundException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminServiceAccountsIdRolesGetResponse404
     */
    private $apiAdminServiceAccountsIdRolesGetResponse404;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminServiceAccountsIdRolesGetResponse404 $apiAdminServiceAccountsIdRolesGetResponse404, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminServiceAccountsIdRolesGetResponse404 = $apiAdminServiceAccountsIdRolesGetResponse404;
        $this->response = $response;
    }
    public function getApiAdminServiceAccountsIdRolesGetResponse404(): \FlowCatalyst\Generated\Model\ApiAdminServiceAccountsIdRolesGetResponse404
    {
        return $this->apiAdminServiceAccountsIdRolesGetResponse404;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}