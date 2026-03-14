<?php

namespace FlowCatalyst\Generated\Exception;

class PutApiSdkRoleByRoleNameNotFoundException extends NotFoundException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiSdkRolesRoleNamePutResponse404
     */
    private $apiSdkRolesRoleNamePutResponse404;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiSdkRolesRoleNamePutResponse404 $apiSdkRolesRoleNamePutResponse404, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiSdkRolesRoleNamePutResponse404 = $apiSdkRolesRoleNamePutResponse404;
        $this->response = $response;
    }
    public function getApiSdkRolesRoleNamePutResponse404(): \FlowCatalyst\Generated\Model\ApiSdkRolesRoleNamePutResponse404
    {
        return $this->apiSdkRolesRoleNamePutResponse404;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}