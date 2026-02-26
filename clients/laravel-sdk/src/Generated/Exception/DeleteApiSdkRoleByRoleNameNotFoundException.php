<?php

namespace FlowCatalyst\Generated\Exception;

class DeleteApiSdkRoleByRoleNameNotFoundException extends NotFoundException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiSdkRolesRoleNameDeleteResponse404
     */
    private $apiSdkRolesRoleNameDeleteResponse404;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiSdkRolesRoleNameDeleteResponse404 $apiSdkRolesRoleNameDeleteResponse404, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiSdkRolesRoleNameDeleteResponse404 = $apiSdkRolesRoleNameDeleteResponse404;
        $this->response = $response;
    }
    public function getApiSdkRolesRoleNameDeleteResponse404(): \FlowCatalyst\Generated\Model\ApiSdkRolesRoleNameDeleteResponse404
    {
        return $this->apiSdkRolesRoleNameDeleteResponse404;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}