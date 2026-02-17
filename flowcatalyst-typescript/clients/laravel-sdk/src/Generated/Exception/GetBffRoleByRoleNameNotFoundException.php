<?php

namespace FlowCatalyst\Generated\Exception;

class GetBffRoleByRoleNameNotFoundException extends NotFoundException
{
    /**
     * @var \FlowCatalyst\Generated\Model\BffRolesRoleNameGetResponse404
     */
    private $bffRolesRoleNameGetResponse404;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\BffRolesRoleNameGetResponse404 $bffRolesRoleNameGetResponse404, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->bffRolesRoleNameGetResponse404 = $bffRolesRoleNameGetResponse404;
        $this->response = $response;
    }
    public function getBffRolesRoleNameGetResponse404(): \FlowCatalyst\Generated\Model\BffRolesRoleNameGetResponse404
    {
        return $this->bffRolesRoleNameGetResponse404;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}