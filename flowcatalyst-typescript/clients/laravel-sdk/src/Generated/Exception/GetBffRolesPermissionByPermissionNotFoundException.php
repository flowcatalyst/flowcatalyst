<?php

namespace FlowCatalyst\Generated\Exception;

class GetBffRolesPermissionByPermissionNotFoundException extends NotFoundException
{
    /**
     * @var \FlowCatalyst\Generated\Model\BffRolesPermissionsPermissionGetResponse404
     */
    private $bffRolesPermissionsPermissionGetResponse404;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\BffRolesPermissionsPermissionGetResponse404 $bffRolesPermissionsPermissionGetResponse404, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->bffRolesPermissionsPermissionGetResponse404 = $bffRolesPermissionsPermissionGetResponse404;
        $this->response = $response;
    }
    public function getBffRolesPermissionsPermissionGetResponse404(): \FlowCatalyst\Generated\Model\BffRolesPermissionsPermissionGetResponse404
    {
        return $this->bffRolesPermissionsPermissionGetResponse404;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}