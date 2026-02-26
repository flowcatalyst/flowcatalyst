<?php

namespace FlowCatalyst\Generated\Exception;

class PutApiAdminServiceAccountsByIdRoleNotFoundException extends NotFoundException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminServiceAccountsIdRolesPutResponse404
     */
    private $apiAdminServiceAccountsIdRolesPutResponse404;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminServiceAccountsIdRolesPutResponse404 $apiAdminServiceAccountsIdRolesPutResponse404, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminServiceAccountsIdRolesPutResponse404 = $apiAdminServiceAccountsIdRolesPutResponse404;
        $this->response = $response;
    }
    public function getApiAdminServiceAccountsIdRolesPutResponse404(): \FlowCatalyst\Generated\Model\ApiAdminServiceAccountsIdRolesPutResponse404
    {
        return $this->apiAdminServiceAccountsIdRolesPutResponse404;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}