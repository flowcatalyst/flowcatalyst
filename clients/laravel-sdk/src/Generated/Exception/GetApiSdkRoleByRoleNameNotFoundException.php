<?php

namespace FlowCatalyst\Generated\Exception;

class GetApiSdkRoleByRoleNameNotFoundException extends NotFoundException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiSdkRolesRoleNameGetResponse404
     */
    private $apiSdkRolesRoleNameGetResponse404;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiSdkRolesRoleNameGetResponse404 $apiSdkRolesRoleNameGetResponse404, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiSdkRolesRoleNameGetResponse404 = $apiSdkRolesRoleNameGetResponse404;
        $this->response = $response;
    }
    public function getApiSdkRolesRoleNameGetResponse404(): \FlowCatalyst\Generated\Model\ApiSdkRolesRoleNameGetResponse404
    {
        return $this->apiSdkRolesRoleNameGetResponse404;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}