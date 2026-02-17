<?php

namespace FlowCatalyst\Generated\Exception;

class DeleteBffRoleByRoleNameNotFoundException extends NotFoundException
{
    /**
     * @var \FlowCatalyst\Generated\Model\BffRolesRoleNameDeleteResponse404
     */
    private $bffRolesRoleNameDeleteResponse404;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\BffRolesRoleNameDeleteResponse404 $bffRolesRoleNameDeleteResponse404, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->bffRolesRoleNameDeleteResponse404 = $bffRolesRoleNameDeleteResponse404;
        $this->response = $response;
    }
    public function getBffRolesRoleNameDeleteResponse404(): \FlowCatalyst\Generated\Model\BffRolesRoleNameDeleteResponse404
    {
        return $this->bffRolesRoleNameDeleteResponse404;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}