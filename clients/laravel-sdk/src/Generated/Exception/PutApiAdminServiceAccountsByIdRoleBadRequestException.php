<?php

namespace FlowCatalyst\Generated\Exception;

class PutApiAdminServiceAccountsByIdRoleBadRequestException extends BadRequestException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminServiceAccountsIdRolesPutResponse400
     */
    private $apiAdminServiceAccountsIdRolesPutResponse400;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminServiceAccountsIdRolesPutResponse400 $apiAdminServiceAccountsIdRolesPutResponse400, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminServiceAccountsIdRolesPutResponse400 = $apiAdminServiceAccountsIdRolesPutResponse400;
        $this->response = $response;
    }
    public function getApiAdminServiceAccountsIdRolesPutResponse400(): \FlowCatalyst\Generated\Model\ApiAdminServiceAccountsIdRolesPutResponse400
    {
        return $this->apiAdminServiceAccountsIdRolesPutResponse400;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}