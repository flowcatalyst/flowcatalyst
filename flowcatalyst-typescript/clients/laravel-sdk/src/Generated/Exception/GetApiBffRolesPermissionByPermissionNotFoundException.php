<?php

namespace FlowCatalyst\Generated\Exception;

class GetApiBffRolesPermissionByPermissionNotFoundException extends NotFoundException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiBffRolesPermissionsPermissionGetResponse404
     */
    private $apiBffRolesPermissionsPermissionGetResponse404;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiBffRolesPermissionsPermissionGetResponse404 $apiBffRolesPermissionsPermissionGetResponse404, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiBffRolesPermissionsPermissionGetResponse404 = $apiBffRolesPermissionsPermissionGetResponse404;
        $this->response = $response;
    }
    public function getApiBffRolesPermissionsPermissionGetResponse404(): \FlowCatalyst\Generated\Model\ApiBffRolesPermissionsPermissionGetResponse404
    {
        return $this->apiBffRolesPermissionsPermissionGetResponse404;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}