<?php

namespace FlowCatalyst\Generated\Exception;

class GetApiAdminRolesPermissionByPermissionNotFoundException extends NotFoundException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminRolesPermissionsPermissionGetResponse404
     */
    private $apiAdminRolesPermissionsPermissionGetResponse404;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminRolesPermissionsPermissionGetResponse404 $apiAdminRolesPermissionsPermissionGetResponse404, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminRolesPermissionsPermissionGetResponse404 = $apiAdminRolesPermissionsPermissionGetResponse404;
        $this->response = $response;
    }
    public function getApiAdminRolesPermissionsPermissionGetResponse404(): \FlowCatalyst\Generated\Model\ApiAdminRolesPermissionsPermissionGetResponse404
    {
        return $this->apiAdminRolesPermissionsPermissionGetResponse404;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}